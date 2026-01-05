// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"fmt"
	"log"
	"time"

	"github.com/Thermoquad/heliostat/pkg/helios_protocol"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"go.bug.st/serial"
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
periodic statistics summaries displayed at configurable intervals.`,
	RunE: runErrorDetection,
}

func init() {
	rootCmd.AddCommand(errorDetectionCmd)
	errorDetectionCmd.Flags().BoolVar(&showAll, "show-all", false, "Show all packets (not just errors)")
	errorDetectionCmd.Flags().IntVar(&statsInterval, "stats-interval", 10, "Statistics update interval (seconds)")
	errorDetectionCmd.Flags().BoolVar(&useTUI, "tui", true, "Use terminal UI (false for text mode)")
}

func runErrorDetection(cmd *cobra.Command, args []string) error {
	// Open serial port
	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return fmt.Errorf("failed to open serial port %s: %v", portName, err)
	}
	defer port.Close()

	if useTUI {
		return runTUIMode(port)
	}
	return runTextMode(port)
}

// printDecodeError prints a decode error in highlighted format
func printDecodeError(err error) {
	timestamp := time.Now().Format("15:04:05.000")
	fmt.Printf("[%s] \033[1;31mDECODE ERROR:\033[0m %v\n", timestamp, err)
	fmt.Printf("  >>> DECODE FAILED <<<\n\n")
}

// printPingResponse prints a ping response with uptime
func printPingResponse(packet *helios_protocol.Packet) {
	timestamp := packet.Timestamp().Format("15:04:05.000")
	payload := packet.Payload()

	// Extract uptime (8 bytes, little-endian)
	if len(payload) < 8 {
		fmt.Printf("[%s] \033[1;32mPING_RESPONSE:\033[0m Invalid payload length (%d bytes)\n\n", timestamp, len(payload))
		return
	}

	uptime := uint64(payload[0]) | uint64(payload[1])<<8 |
		uint64(payload[2])<<16 | uint64(payload[3])<<24 |
		uint64(payload[4])<<32 | uint64(payload[5])<<40 |
		uint64(payload[6])<<48 | uint64(payload[7])<<56

	uptimeStr := formatUptime(uptime)
	fmt.Printf("[%s] \033[1;32mPING_RESPONSE:\033[0m Helios uptime: %s\n\n", timestamp, uptimeStr)
}

// printValidationErrors prints validation errors for a packet
func printValidationErrors(packet *helios_protocol.Packet, errors []helios_protocol.ValidationError) {
	timestamp := packet.Timestamp().Format("15:04:05.000")
	msgType := helios_protocol.FormatMessageType(packet.Type())

	fmt.Printf("[%s] \033[1;33mVALIDATION ERROR:\033[0m %s (0x%02X)\n", timestamp, msgType, packet.Type())
	fmt.Printf("  CRC: \033[1;32mOK\033[0m\n")

	for i, err := range errors {
		switch err.Type {
		case helios_protocol.ANOMALY_INVALID_COUNT:
			fmt.Printf("  Issue %d: \033[1;31m%s\033[0m\n", i+1, err.Message)
			if motorCount, ok := err.Details["motor_count"].(uint8); ok {
				fmt.Printf("    motor_count=%d (max 10)\n", motorCount)
			}
			if tempCount, ok := err.Details["temp_count"].(uint8); ok {
				fmt.Printf("    temp_count=%d (max 10)\n", tempCount)
			}

		case helios_protocol.ANOMALY_LENGTH_MISMATCH:
			fmt.Printf("  Issue %d: \033[1;31m%s\033[0m\n", i+1, err.Message)
			if received, ok := err.Details["received"].(int); ok {
				if expected, ok := err.Details["expected"].(int); ok {
					fmt.Printf("    Length: received=%d, expected=%d\n", received, expected)
				}
			}

		case helios_protocol.ANOMALY_HIGH_RPM:
			fmt.Printf("  Issue %d: \033[1;33m%s\033[0m\n", i+1, err.Message)
			if rpm, ok := err.Details["rpm"].(int32); ok {
				if targetRPM, ok := err.Details["target_rpm"].(int32); ok {
					fmt.Printf("    RPM=%d, target=%d (max 6000)\n", rpm, targetRPM)
				}
			}

		case helios_protocol.ANOMALY_INVALID_TEMP:
			fmt.Printf("  Issue %d: \033[1;33m%s\033[0m\n", i+1, err.Message)
			if temp, ok := err.Details["value"].(float64); ok {
				fmt.Printf("    Temperature=%.1f°C (valid: -50 to 1000°C)\n", temp)
			}

		case helios_protocol.ANOMALY_INVALID_PWM:
			fmt.Printf("  Issue %d: \033[1;33m%s\033[0m\n", i+1, err.Message)
			if duty, ok := err.Details["pwm_duty"].(int32); ok {
				if period, ok := err.Details["pwm_period"].(int32); ok {
					fmt.Printf("    PWM: duty=%d, period=%d\n", duty, period)
				}
			}

		default:
			fmt.Printf("  Issue %d: %s\n", i+1, err.Message)
		}
	}

	// Print packet header for context
	stateNames := []string{"INITIALIZING", "IDLE", "BLOWING", "PREHEAT", "PREHEAT_STAGE_2", "HEATING", "COOLING", "ERROR", "E_STOP"}
	if packet.Type() == helios_protocol.MSG_TELEMETRY_BUNDLE && len(packet.Payload()) >= 7 {
		state := uint32(packet.Payload()[0]) | uint32(packet.Payload()[1])<<8 |
			uint32(packet.Payload()[2])<<16 | uint32(packet.Payload()[3])<<24
		errorCode := packet.Payload()[4]
		stateName := "UNKNOWN"
		if int(state) < len(stateNames) {
			stateName = stateNames[state]
		}
		fmt.Printf("  State: %s (0x%02X), Error: 0x%02X\n", stateName, state, errorCode)
	}

	fmt.Printf("  >>> PACKET REJECTED <<<\n\n")
}

// runTUIMode runs error detection in TUI mode
func runTUIMode(port serial.Port) error {
	decoder := helios_protocol.NewDecoder()
	synchronized := false
	invalidBytesBeforeSync := 0

	// Create TUI program
	m := initialModel(portName, baudRate, statsInterval, showAll)
	p := tea.NewProgram(m)

	// Serial reader goroutine
	go func() {
		buf := make([]byte, 128)
		for {
			n, err := port.Read(buf)
			if err != nil {
				log.Printf("Read error: %v", err)
				continue
			}

			// Process bytes
			for i := 0; i < n; i++ {
				packet, decodeErr := decoder.DecodeByte(buf[i])

				// Handle decode errors
				if decodeErr != nil {
					if synchronized {
						// We're synced, this is a real error
						p.Send(serialDataMsg{
							packet:           nil,
							decodeErr:        decodeErr,
							validationErrors: nil,
						})
					} else {
						// Not synced yet, just count invalid bytes
						invalidBytesBeforeSync++
					}
				} else if packet != nil {
					// Successfully decoded a packet
					if !synchronized {
						// First packet! We're now synchronized
						synchronized = true
						p.Send(syncMsg{invalidBytes: invalidBytesBeforeSync})
					}

					// Validate packet
					validationErrors := helios_protocol.ValidatePacket(packet)
					p.Send(serialDataMsg{
						packet:           packet,
						decodeErr:        nil,
						validationErrors: validationErrors,
					})
				}
			}
		}
	}()

	// Run TUI
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %v", err)
	}

	return nil
}

// runTextMode runs error detection in text mode (original behavior)
func runTextMode(port serial.Port) error {
	fmt.Printf("Heliostat - Error Detection Mode\n")
	fmt.Printf("Port: %s @ %d baud\n", portName, baudRate)
	fmt.Printf("Statistics interval: %d seconds\n", statsInterval)
	if showAll {
		fmt.Printf("Mode: All packets\n")
	} else {
		fmt.Printf("Mode: Errors only\n")
	}
	fmt.Printf("Press Ctrl+C to exit\n\n")

	decoder := helios_protocol.NewDecoder()
	stats := helios_protocol.NewStatistics()
	buf := make([]byte, 128)

	// Sync tracking - ignore decode errors until first valid packet
	synchronized := false
	invalidBytesBeforeSync := 0

	// Statistics ticker
	statsTicker := time.NewTicker(time.Duration(statsInterval) * time.Second)
	defer statsTicker.Stop()

	// Channel for non-blocking serial reads
	serialBuf := make(chan []byte, 10)
	go func() {
		for {
			n, err := port.Read(buf)
			if err != nil {
				log.Printf("Read error: %v", err)
				continue
			}
			data := make([]byte, n)
			copy(data, buf[:n])
			serialBuf <- data
		}
	}()

	for {
		select {
		case data := <-serialBuf:
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
					validationErrors := helios_protocol.ValidatePacket(packet)
					stats.Update(packet, nil, validationErrors)

					// Print packet or error based on mode
					if len(validationErrors) > 0 {
						printValidationErrors(packet, validationErrors)
					} else if packet.Type() == helios_protocol.MSG_PING_RESPONSE {
						// Always print ping responses (for debugging)
						printPingResponse(packet)
					} else if showAll {
						// Print valid packet (only if --show-all flag is set)
						fmt.Print(helios_protocol.FormatPacket(packet))
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
