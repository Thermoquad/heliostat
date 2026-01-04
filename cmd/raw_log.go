// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"fmt"
	"log"

	"github.com/Thermoquad/heliostat/pkg/helios_protocol"
	"github.com/spf13/cobra"
	"go.bug.st/serial"
)

var rawLogCmd = &cobra.Command{
	Use:   "raw_log",
	Short: "Display raw packet log in human-readable format",
	Long: `Continuously decode and display Helios protocol packets as they arrive.

This command provides the same output as the original heliostat tool, showing
each packet with timestamp, message type, and decoded payload data.`,
	RunE: runRawLog,
}

func init() {
	rootCmd.AddCommand(rawLogCmd)
}

func runRawLog(cmd *cobra.Command, args []string) error {
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

	fmt.Printf("Heliostat - Raw Packet Log\n")
	fmt.Printf("Port: %s @ %d baud\n", portName, baudRate)
	fmt.Printf("Press Ctrl+C to exit\n\n")

	decoder := helios_protocol.NewDecoder()
	buf := make([]byte, 128)

	for {
		n, err := port.Read(buf)
		if err != nil {
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
				fmt.Print(helios_protocol.FormatPacket(packet))
			}
		}
	}
}
