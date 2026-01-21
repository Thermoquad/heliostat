// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"fmt"
	"log"

	"github.com/Thermoquad/heliostat/pkg/fusain"
	"github.com/spf13/cobra"
)

var rawLogCmd = &cobra.Command{
	Use:   "raw_log",
	Short: "Display raw packet log in human-readable format",
	Long: `Continuously decode and display Helios protocol packets as they arrive.

This command provides the same output as the original heliostat tool, showing
each packet with timestamp, message type, and decoded payload data.

Supports both serial and WebSocket connections.`,
	RunE: runRawLog,
}

func init() {
	rootCmd.AddCommand(rawLogCmd)
}

func runRawLog(cmd *cobra.Command, args []string) error {
	// Open connection (serial or WebSocket)
	conn, connInfo, err := OpenConnection()
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Printf("Heliostat - Raw Packet Log\n")
	fmt.Printf("Connection: %s\n", connInfo)
	fmt.Printf("Press Ctrl+C to exit\n\n")

	decoder := fusain.NewDecoder()
	buf := make([]byte, 128)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			// For WebSocket connections, a read error usually means
			// the connection is permanently closed - exit gracefully
			if err == ErrConnectionClosed {
				log.Printf("Connection closed")
				return nil
			}
			log.Printf("Read error: %v", err)
			continue
		}

		for i := 0; i < n; i++ {
			packet, err := decoder.DecodeByte(buf[i])
			if err != nil {
				fmt.Printf("[ERROR] %v\n", err)
				continue
			}
			if packet != nil {
				fmt.Print(fusain.FormatPacket(packet))
			}
		}
	}
}
