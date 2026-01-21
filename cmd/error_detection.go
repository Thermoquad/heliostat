// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"fmt"
	"time"

	"github.com/Thermoquad/heliostat/pkg/fusain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	showAll       bool
	statsInterval int
	useTUI        bool
)

var errorDetectionCmd = &cobra.Command{
	Use:   "error_detection",
	Short: "Detect and analyze malformed packets and errors",
	Long: `Track packet errors, malformed data, and anomalous values with statistics.

This command validates each packet and detects:
  - Malformed packets (invalid counts, length mismatches)
  - CRC errors and decode failures
  - Anomalous telemetry values (RPM > 6000, invalid temperatures)
  - Statistics and trends (packet rate, error rate, success rate)

By default, only errors are displayed. Use --show-all to display valid packets too.

Packets are validated in real-time, with errors highlighted immediately and
periodic statistics summaries displayed at configurable intervals.

Supports both serial and WebSocket connections.`,
	RunE: runErrorDetection,
}

func init() {
	rootCmd.AddCommand(errorDetectionCmd)
	errorDetectionCmd.Flags().BoolVar(&showAll, "show-all", false, "Show all packets (not just errors)")
	errorDetectionCmd.Flags().IntVar(&statsInterval, "stats-interval", 10, "Statistics update interval (seconds)")
	errorDetectionCmd.Flags().BoolVar(&useTUI, "tui", true, "Use terminal UI (false for text mode)")
}

func runErrorDetection(cmd *cobra.Command, args []string) error {
	// Open connection (serial or WebSocket)
	conn, connInfo, err := OpenConnection()
	if err != nil {
		return err
	}
	defer conn.Close()

	if useTUI {
		return runTUIMode(conn, connInfo)
	}
	return runTextMode(conn, connInfo)
}

// printDecodeError prints a decode error in highlighted format
func printDecodeError(err error) {
	timestamp := time.Now().Format("15:04:05.000")
	fmt.Printf("[%s] \033[1;31mDECODE ERROR:\033[0m %v\n", timestamp, err)
	fmt.Printf("  >>> DECODE FAILED <<<\n\n")
}

// printPingResponse prints a ping response with uptime
func printPingResponse(packet *fusain.Packet) {
	timestamp := packet.Timestamp().Format("15:04:05.000")
	payloadMap := packet.PayloadMap()

	// Extract uptime (CBOR key 0)
	uptime, ok := fusain.GetMapUint(payloadMap, 0)
	if !ok {
		fmt.Printf("[%s] \033[1;32mPING_RESPONSE:\033[0m No uptime in payload\n\n", timestamp)
		return
	}

	uptimeStr := formatUptime(uptime)
	fmt.Printf("[%s] \033[1;32mPING_RESPONSE:\033[0m Helios uptime: %s\n\n", timestamp, uptimeStr)
}

// printValidationErrors prints validation errors for a packet
func printValidationErrors(packet *fusain.Packet, errors []fusain.ValidationError) {
	timestamp := packet.Timestamp().Format("15:04:05.000")
	msgType := fusain.FormatMessageType(packet.Type())

	fmt.Printf("[%s] \033[1;33mVALIDATION ERROR:\033[0m %s (0x%02X)\n", timestamp, msgType, packet.Type())
	fmt.Printf("  CRC: \033[1;32mOK\033[0m\n")

	for i, err := range errors {
		switch err.Type {
		case fusain.AnomalyInvalidCount:
			fmt.Printf("  Issue %d: \033[1;31m%s\033[0m\n", i+1, err.Message)
			if motorCount, ok := err.Details["motor_count"].(uint64); ok {
				fmt.Printf("    motor_count=%d (max 10)\n", motorCount)
			}
			if tempCount, ok := err.Details["temp_count"].(uint64); ok {
				fmt.Printf("    temp_count=%d (max 10)\n", tempCount)
			}

		case fusain.AnomalyLengthMismatch:
			fmt.Printf("  Issue %d: \033[1;31m%s\033[0m\n", i+1, err.Message)

		case fusain.AnomalyHighRPM:
			fmt.Printf("  Issue %d: \033[1;33m%s\033[0m\n", i+1, err.Message)
			if rpm, ok := err.Details["rpm"].(int64); ok {
				if targetRPM, ok := err.Details["target_rpm"].(int64); ok {
					fmt.Printf("    RPM=%d, target=%d (max 6000)\n", rpm, targetRPM)
				}
			}

		case fusain.AnomalyInvalidTemp:
			fmt.Printf("  Issue %d: \033[1;33m%s\033[0m\n", i+1, err.Message)
			if temp, ok := err.Details["value"].(float64); ok {
				fmt.Printf("    Temperature=%.1f°C (valid: -50 to 1000°C)\n", temp)
			}

		case fusain.AnomalyInvalidPWM:
			fmt.Printf("  Issue %d: \033[1;33m%s\033[0m\n", i+1, err.Message)
			if pwm, ok := err.Details["pwm"].(uint64); ok {
				if pwmMax, ok := err.Details["pwm_max"].(uint64); ok {
					fmt.Printf("    PWM: duty=%d, max=%d\n", pwm, pwmMax)
				}
			}

		case fusain.AnomalyDecodeError:
			fmt.Printf("  Issue %d: \033[1;31m%s\033[0m\n", i+1, err.Message)

		case fusain.AnomalyInvalidValue:
			fmt.Printf("  Issue %d: \033[1;33m%s\033[0m\n", i+1, err.Message)

		default:
			fmt.Printf("  Issue %d: %s\n", i+1, err.Message)
		}
	}

	// Print packet header for context (STATE_DATA contains state and error code)
	stateNames := []string{"INITIALIZING", "IDLE", "BLOWING", "PREHEAT", "PREHEAT_STAGE_2", "HEATING", "COOLING", "ERROR", "E_STOP"}
	if packet.Type() == fusain.MsgStateData {
		payloadMap := packet.PayloadMap()
		// CBOR keys: 0=error(bool), 1=code, 2=state, 3=timestamp
		state, hasState := fusain.GetMapUint(payloadMap, 2)
		errorCode, _ := fusain.GetMapInt(payloadMap, 1)
		if hasState {
			stateName := "UNKNOWN"
			if int(state) < len(stateNames) {
				stateName = stateNames[state]
			}
			fmt.Printf("  State: %s (0x%02X), Error: 0x%02X\n", stateName, state, errorCode)
		}
	}

	fmt.Printf("  >>> PACKET REJECTED <<<\n\n")
}

// runTUIMode runs error detection in TUI mode
func runTUIMode(conn ByteReader, connInfo string) error {
	decoder := fusain.NewDecoder()
	synchronized := false
	invalidBytesBeforeSync := 0

	// Create TUI program with alt screen for flicker-free rendering
	m := initialModel(connInfo, statsInterval, showAll)
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Done channel for shutdown signaling
	done := make(chan struct{})

	// Buffered channel for batching updates (prevents TUI glitches)
	batchChan := make(chan serialDataMsg, 100)
	syncChan := make(chan syncMsg, 1)

	// Reader goroutine - decodes packets and sends to batch channel
	go func() {
		buf := make([]byte, 128)
		for {
			// Check if we should stop
			select {
			case <-done:
				return
			default:
			}

			n, err := conn.Read(buf)
			if err != nil {
				// Check if we're shutting down
				select {
				case <-done:
					return
				default:
					// For WebSocket connections, a read error usually means
					// the connection is permanently closed - exit gracefully
					if err == ErrConnectionClosed {
						return
					}
					// Brief pause before retry on transient errors (e.g., serial)
					time.Sleep(10 * time.Millisecond)
					continue
				}
			}

			// Process bytes
			for i := 0; i < n; i++ {
				packet, decodeErr := decoder.DecodeByte(buf[i])

				// Handle decode errors
				if decodeErr != nil {
					if synchronized {
						// We're synced, this is a real error
						select {
						case batchChan <- serialDataMsg{
							packet:           nil,
							decodeErr:        decodeErr,
							validationErrors: nil,
						}:
						default:
							// Channel full, drop oldest by reading and discarding
						}
					} else {
						// Not synced yet, just count invalid bytes
						invalidBytesBeforeSync++
					}
				} else if packet != nil {
					// Successfully decoded a packet
					if !synchronized {
						// First packet! We're now synchronized
						synchronized = true
						select {
						case syncChan <- syncMsg{invalidBytes: invalidBytesBeforeSync}:
						default:
						}
					}

					// Validate packet
					validationErrors := fusain.ValidatePacket(packet)
					select {
					case batchChan <- serialDataMsg{
						packet:           packet,
						decodeErr:        nil,
						validationErrors: validationErrors,
					}:
					default:
						// Channel full, drop message (TUI can't keep up)
					}
				}
			}
		}
	}()

	// Batch sender goroutine - sends batched updates to TUI at fixed rate
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond) // 20 updates/sec max
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				var batch batchDataMsg

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
					p.Send(batch)
				}
			}
		}
	}()

	// Run TUI
	if _, err := p.Run(); err != nil {
		close(done) // Signal goroutines to stop
		return fmt.Errorf("TUI error: %v", err)
	}

	close(done) // Signal goroutines to stop
	return nil
}

// runTextMode runs error detection in text mode (original behavior)
func runTextMode(conn ByteReader, connInfo string) error {
	fmt.Printf("Heliostat - Error Detection Mode\n")
	fmt.Printf("Connection: %s\n", connInfo)
	fmt.Printf("Statistics interval: %d seconds\n", statsInterval)
	if showAll {
		fmt.Printf("Mode: All packets\n")
	} else {
		fmt.Printf("Mode: Errors only\n")
	}
	fmt.Printf("Press Ctrl+C to exit\n\n")

	decoder := fusain.NewDecoder()
	stats := fusain.NewStatistics()
	buf := make([]byte, 128)

	// Sync tracking - ignore decode errors until first valid packet
	synchronized := false
	invalidBytesBeforeSync := 0

	// Statistics ticker
	statsTicker := time.NewTicker(time.Duration(statsInterval) * time.Second)
	defer statsTicker.Stop()

	// Channel for non-blocking reads
	dataBuf := make(chan []byte, 10)
	go func() {
		for {
			n, err := conn.Read(buf)
			if err != nil {
				// Connection closed or error - exit goroutine silently
				return
			}
			data := make([]byte, n)
			copy(data, buf[:n])
			dataBuf <- data
		}
	}()

	for {
		select {
		case data := <-dataBuf:
			// Process bytes
			for _, b := range data {
				packet, decodeErr := decoder.DecodeByte(b)

				// Handle decode errors
				if decodeErr != nil {
					if synchronized {
						// We're synced, this is a real error
						stats.Update(nil, decodeErr, nil)
						printDecodeError(decodeErr)
					} else {
						// Not synced yet, just count invalid bytes
						invalidBytesBeforeSync++
					}
				} else if packet != nil {
					// Successfully decoded a packet
					if !synchronized {
						// First packet! We're now synchronized
						synchronized = true
						if invalidBytesBeforeSync > 0 {
							fmt.Printf("[SYNC] Synchronized after skipping %d invalid bytes\n\n", invalidBytesBeforeSync)
						} else {
							fmt.Printf("[SYNC] Synchronized\n\n")
						}
					}

					// Validate packet
					validationErrors := fusain.ValidatePacket(packet)
					stats.Update(packet, nil, validationErrors)

					// Print packet or error based on mode
					if len(validationErrors) > 0 {
						printValidationErrors(packet, validationErrors)
					} else if packet.Type() == fusain.MsgPingResponse {
						// Always print ping responses (for debugging)
						printPingResponse(packet)
					} else if showAll {
						// Print valid packet (only if --show-all flag is set)
						fmt.Print(fusain.FormatPacket(packet))
					}
				}
			}

		case <-statsTicker.C:
			// Print statistics
			fmt.Println()
			fmt.Print(stats.String())
			fmt.Println()
		}
	}
}
