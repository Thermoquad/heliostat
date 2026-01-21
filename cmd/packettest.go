// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/Thermoquad/heliostat/pkg/fusain"
	"github.com/spf13/cobra"
)

var (
	packetTestTimeout int
)

var packetTestCmd = &cobra.Command{
	Use:   "packet_test",
	Short: "Test connection by waiting for a valid Fusain packet",
	Long: `Wait for a valid Fusain packet on the connection until timeout.

This command connects to a serial port or WebSocket and waits for any valid
Fusain protocol packet. It ignores invalid bytes and waits for a complete,
valid packet (passing CRC check).

Exit codes:
  0 - Packet received before timeout
  1 - Timeout reached without receiving a valid packet
  2 - Connection error

Useful for testing connectivity to Helios or Slate WebSocket bridge.`,
	RunE: runPacketTest,
}

func init() {
	rootCmd.AddCommand(packetTestCmd)
	packetTestCmd.Flags().IntVar(&packetTestTimeout, "timeout", 10, "Timeout in seconds to wait for a packet")
}

func runPacketTest(cmd *cobra.Command, args []string) error {
	// Open connection (serial or WebSocket)
	conn, connInfo, err := OpenConnection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection error: %v\n", err)
		os.Exit(2)
	}
	defer conn.Close()

	fmt.Printf("Heliostat - Packet Test\n")
	fmt.Printf("Connection: %s\n", connInfo)
	fmt.Printf("Timeout: %d seconds\n", packetTestTimeout)
	fmt.Printf("Waiting for valid Fusain packet...\n\n")

	decoder := fusain.NewDecoder()
	buf := make([]byte, 128)

	// Channel for packet reception
	packetChan := make(chan *fusain.Packet, 1)
	errChan := make(chan error, 1)

	// Reader goroutine
	go func() {
		invalidBytes := 0
		for {
			n, err := conn.Read(buf)
			if err != nil {
				errChan <- err
				return
			}

			for i := 0; i < n; i++ {
				packet, decodeErr := decoder.DecodeByte(buf[i])
				if decodeErr != nil {
					// Ignore decode errors, just count invalid bytes
					invalidBytes++
					continue
				}
				if packet != nil {
					// Got a valid packet!
					if invalidBytes > 0 {
						fmt.Printf("(skipped %d invalid bytes before sync)\n", invalidBytes)
					}
					packetChan <- packet
					return
				}
			}
		}
	}()

	// Wait for packet or timeout
	select {
	case packet := <-packetChan:
		fmt.Printf("SUCCESS: Received valid packet\n")
		fmt.Printf("  Type: %s (0x%02X)\n", fusain.FormatMessageType(packet.Type()), packet.Type())
		fmt.Printf("  Address: 0x%016X\n", packet.Address())
		fmt.Printf("  Length: %d bytes\n", packet.Length())
		fmt.Printf("  CRC: 0x%04X\n", packet.CRC())
		os.Exit(0)

	case err := <-errChan:
		fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
		os.Exit(2)

	case <-time.After(time.Duration(packetTestTimeout) * time.Second):
		fmt.Fprintf(os.Stderr, "TIMEOUT: No valid packet received within %d seconds\n", packetTestTimeout)
		os.Exit(1)
	}

	return nil
}
