// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"fmt"
	"sync"
	"time"

	"github.com/Thermoquad/heliostat/pkg/fusain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var controlCmd = &cobra.Command{
	Use:   "control",
	Short: "Interactive TUI for controlling Helios heaters",
	Long: `Control Helios heaters via an interactive terminal UI.

This command provides a TUI for monitoring and controlling Helios devices
connected via WebSocket (through Slate) or UART (direct connection).

Features:
  - Device discovery (DEVICE_ANNOUNCE)
  - Real-time telemetry display
  - State control (idle, fan mode)
  - Statistics tracking
  - Event logging
  - Automatic reconnection on connection loss

The TUI discovers devices first before enabling control. Tab switches between
device list and control panel. Arrow keys navigate the device list.

Supports both serial and WebSocket connections.`,
	RunE: runControl,
}

func init() {
	rootCmd.AddCommand(controlCmd)
}

// connectionManager handles connection lifecycle and reconnection
type connectionManager struct {
	conn     Connection
	connInfo string
	mu       sync.RWMutex
	p        *tea.Program
	done     chan struct{}
	stopRead chan struct{}
}

func (cm *connectionManager) getConn() Connection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.conn
}

func (cm *connectionManager) setConn(conn Connection, connInfo string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.conn = conn
	cm.connInfo = connInfo
}

func runControl(cmd *cobra.Command, args []string) error {
	// Open initial connection (serial or WebSocket)
	conn, connInfo, err := OpenConnection()
	if err != nil {
		return err
	}

	// Create connection manager
	cm := &connectionManager{
		conn:     conn,
		connInfo: connInfo,
		done:     make(chan struct{}),
		stopRead: make(chan struct{}),
	}

	// Create TUI model with connection manager
	m := initialControlModel(cm, connInfo)

	// Create TUI program with alt screen and mouse support
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	cm.p = p

	// Start reader goroutines (similar to error_detection.go pattern)
	go cm.readerLoop()

	// Send initial discovery request
	sendInitialDiscoveryRequest(cm.getConn())

	// Run TUI
	if _, err := p.Run(); err != nil {
		close(cm.done) // Signal goroutines to stop
		cm.getConn().Close()
		return fmt.Errorf("TUI error: %v", err)
	}

	close(cm.done) // Signal goroutines to stop
	cm.getConn().Close()
	return nil
}

// readerLoop handles reading from connection with automatic reconnection
func (cm *connectionManager) readerLoop() {
	for {
		select {
		case <-cm.done:
			return
		default:
		}

		// Start reading from current connection
		connLost := cm.readFromConnection()

		if connLost {
			// Notify TUI about connection loss
			cm.p.Send(connectionLostMsg{})

			// Attempt to reconnect
			if !cm.reconnect() {
				return // Shutdown requested during reconnect
			}
		}
	}
}

// readFromConnection reads packets from the connection until it fails
// Returns true if connection was lost, false if shutdown requested
func (cm *connectionManager) readFromConnection() bool {
	decoder := fusain.NewDecoder()
	synchronized := false
	invalidBytesBeforeSync := 0

	// Buffered channel for batching updates
	batchChan := make(chan controlDataMsg, 100)
	syncChan := make(chan controlSyncMsg, 1)
	readerDone := make(chan struct{})

	// Reader goroutine - decodes packets and sends to batch channel
	go func() {
		defer close(readerDone)
		buf := make([]byte, 128)
		for {
			select {
			case <-cm.done:
				return
			case <-cm.stopRead:
				return
			default:
			}

			conn := cm.getConn()
			if conn == nil {
				return
			}

			n, err := conn.Read(buf)
			if err != nil {
				// Check if we're shutting down
				select {
				case <-cm.done:
					return
				default:
					// For WebSocket connections, a read error usually means
					// the connection is permanently closed
					if err == ErrConnectionClosed {
						return
					}
					// Brief pause before retry on transient errors (e.g., serial)
					time.Sleep(10 * time.Millisecond)
					continue
				}
			}

			for i := 0; i < n; i++ {
				packet, decodeErr := decoder.DecodeByte(buf[i])

				if decodeErr != nil {
					if synchronized {
						select {
						case batchChan <- controlDataMsg{
							packet:           nil,
							decodeErr:        decodeErr,
							validationErrors: nil,
						}:
						default:
						}
					} else {
						invalidBytesBeforeSync++
					}
				} else if packet != nil {
					if !synchronized {
						synchronized = true
						select {
						case syncChan <- controlSyncMsg{invalidBytes: invalidBytesBeforeSync}:
						default:
						}
					}

					validationErrors := fusain.ValidatePacket(packet)
					select {
					case batchChan <- controlDataMsg{
						packet:           packet,
						decodeErr:        nil,
						validationErrors: validationErrors,
					}:
					default:
					}
				}
			}
		}
	}()

	// Batch sender goroutine - sends batched updates to TUI at fixed rate
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-cm.done:
				return
			case <-readerDone:
				return
			case <-ticker.C:
				var batch controlBatchMsg

				// Check for sync message
				select {
				case sync := <-syncChan:
					batch.syncMsg = &sync
				default:
				}

				// Drain all available messages from batch channel
			drainLoop:
				for {
					select {
					case msg := <-batchChan:
						batch.messages = append(batch.messages, msg)
					default:
						break drainLoop
					}
				}

				// Send batch if we have anything
				if batch.syncMsg != nil || len(batch.messages) > 0 {
					cm.p.Send(batch)
				}
			}
		}
	}()

	// Wait for reader to finish (connection lost or shutdown)
	<-readerDone

	// Check if we're shutting down
	select {
	case <-cm.done:
		return false
	default:
		return true // Connection lost
	}
}

// reconnect attempts to reconnect with exponential backoff
// Returns false if shutdown was requested during reconnection
func (cm *connectionManager) reconnect() bool {
	// Close old connection
	if conn := cm.getConn(); conn != nil {
		conn.Close()
	}

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-cm.done:
			return false
		case <-time.After(backoff):
		}

		// Attempt to reconnect
		conn, connInfo, err := OpenConnection()
		if err == nil {
			cm.setConn(conn, connInfo)

			// Notify TUI about reconnection
			cm.p.Send(reconnectedMsg{connInfo: connInfo})

			// Send discovery request
			sendInitialDiscoveryRequest(conn)

			return true
		}

		// Exponential backoff
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// sendInitialDiscoveryRequest sends a discovery request to find devices
func sendInitialDiscoveryRequest(conn Connection) {
	packet := fusain.NewDiscoveryRequest(fusain.AddressBroadcast)
	wireBytes := fusain.MustEncodePacket(packet)
	conn.Write(wireBytes)
}
