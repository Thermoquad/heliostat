// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var wsTestCmd = &cobra.Command{
	Use:   "ws_test",
	Short: "Test raw WebSocket connection stability",
	Long: `Test WebSocket connection to Slate without sending Fusain protocol data.

This command connects to the WebSocket and just waits, logging any data received
or errors encountered. Useful for debugging connection stability issues.

Exit codes:
  0 - Test completed normally
  1 - Test failed
  2 - Connection error`,
	RunE: runWsTest,
}

var wsTestDuration int

func init() {
	rootCmd.AddCommand(wsTestCmd)
	wsTestCmd.Flags().IntVar(&wsTestDuration, "duration", 30, "Test duration in seconds")
}

func runWsTest(cmd *cobra.Command, args []string) error {
	// Open connection (serial or WebSocket)
	conn, connInfo, err := OpenConnection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection error: %v\n", err)
		os.Exit(2)
	}
	defer conn.Close()

	fmt.Printf("WebSocket Connection Stability Test\n")
	fmt.Printf("Connection: %s\n", connInfo)
	fmt.Printf("Duration: %d seconds\n\n", wsTestDuration)

	// Start a goroutine to read from the connection
	readChan := make(chan []byte, 100)
	errChan := make(chan error, 1)

	go func() {
		buf := make([]byte, 256)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				errChan <- err
				return
			}
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				readChan <- data
			}
		}
	}()

	// Run for the specified duration
	endTime := time.Now().Add(time.Duration(wsTestDuration) * time.Second)
	bytesReceived := 0
	packetsReceived := 0

	fmt.Printf("Listening for data...\n\n")

	for time.Now().Before(endTime) {
		select {
		case data := <-readChan:
			bytesReceived += len(data)
			packetsReceived++
			fmt.Printf("[%s] Received %d bytes: %x\n",
				time.Now().Format("15:04:05.000"), len(data), data)

		case err := <-errChan:
			fmt.Printf("\n[%s] Connection error: %v\n",
				time.Now().Format("15:04:05.000"), err)
			fmt.Printf("\n--- Test Results ---\n")
			fmt.Printf("Duration: %v\n", time.Since(endTime.Add(-time.Duration(wsTestDuration)*time.Second)))
			fmt.Printf("Packets received: %d\n", packetsReceived)
			fmt.Printf("Bytes received: %d\n", bytesReceived)
			fmt.Printf("Result: FAILED (connection error)\n")
			os.Exit(1)

		case <-time.After(1 * time.Second):
			// Just a heartbeat to show the test is running
			remaining := time.Until(endTime).Seconds()
			fmt.Printf("[%s] Still connected... (%.0fs remaining)\n",
				time.Now().Format("15:04:05.000"), remaining)
		}
	}

	fmt.Printf("\n--- Test Results ---\n")
	fmt.Printf("Duration: %d seconds\n", wsTestDuration)
	fmt.Printf("Packets received: %d\n", packetsReceived)
	fmt.Printf("Bytes received: %d\n", bytesReceived)
	fmt.Printf("Result: PASSED (connection stable)\n")

	return nil
}
