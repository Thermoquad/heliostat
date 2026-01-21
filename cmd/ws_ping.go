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
	wsPingTimeout int
	wsPingCount   int
)

var wsPingCmd = &cobra.Command{
	Use:   "ws_ping",
	Short: "Test WebSocket bridge by sending PING_REQUEST to Slate router",
	Long: `Send PING_REQUEST packets to the Slate WebSocket router and wait for PING_RESPONSE.

This command tests bidirectional WebSocket communication with the Slate router.
The Slate router handles PING_REQUEST locally (does not forward to Helios) and
responds with PING_RESPONSE containing the router's uptime.

This is useful for verifying:
  - WebSocket connection is established
  - HTTP Basic authentication works
  - Slate router is processing packets
  - Bidirectional packet flow works

Exit codes:
  0 - All pings successful
  1 - One or more pings failed/timed out
  2 - Connection error`,
	RunE: runWsPing,
}

func init() {
	rootCmd.AddCommand(wsPingCmd)
	wsPingCmd.Flags().IntVar(&wsPingTimeout, "timeout", 5, "Timeout in seconds for each ping")
	wsPingCmd.Flags().IntVar(&wsPingCount, "count", 3, "Number of pings to send")
}

func runWsPing(cmd *cobra.Command, args []string) error {
	// Open connection (serial or WebSocket)
	conn, connInfo, err := OpenConnection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection error: %v\n", err)
		os.Exit(2)
	}
	defer conn.Close()

	fmt.Printf("Heliostat - WebSocket Ping Test\n")
	fmt.Printf("Connection: %s\n", connInfo)
	fmt.Printf("Timeout: %d seconds per ping\n", wsPingTimeout)
	fmt.Printf("Count: %d pings\n\n", wsPingCount)

	decoder := fusain.NewDecoder()
	successCount := 0
	failCount := 0

	for i := 1; i <= wsPingCount; i++ {
		fmt.Printf("Ping %d/%d: ", i, wsPingCount)

		// Create PING_REQUEST packet for stateless address (router)
		pingPacket := fusain.NewPingRequest(fusain.AddressStateless)
		wireBytes := fusain.MustEncodePacket(pingPacket)

		// Send ping
		startTime := time.Now()
		_, err := conn.Write(wireBytes)
		if err != nil {
			fmt.Printf("SEND FAILED: %v\n", err)
			failCount++
			continue
		}

		// Wait for PING_RESPONSE
		responseChan := make(chan *fusain.Packet, 1)
		errChan := make(chan error, 1)

		go func() {
			buf := make([]byte, 128)
			for {
				n, err := conn.Read(buf)
				if err != nil {
					errChan <- err
					return
				}

				for j := 0; j < n; j++ {
					packet, decodeErr := decoder.DecodeByte(buf[j])
					if decodeErr != nil {
						// Ignore decode errors
						continue
					}
					if packet != nil {
						// Check if it's a PING_RESPONSE
						if packet.Type() == fusain.MsgPingResponse {
							responseChan <- packet
							return
						}
						// Ignore non-ping packets (telemetry, etc.)
					}
				}
			}
		}()

		// Wait for response or timeout
		select {
		case packet := <-responseChan:
			rtt := time.Since(startTime)
			payloadMap := packet.PayloadMap()
			uptime, _ := fusain.GetMapUint(payloadMap, 0)
			uptimeStr := formatUptime(uptime)
			fmt.Printf("PONG from router, uptime=%s, rtt=%v\n", uptimeStr, rtt.Round(time.Millisecond))
			successCount++

		case err := <-errChan:
			fmt.Printf("READ FAILED: %v\n", err)
			failCount++

		case <-time.After(time.Duration(wsPingTimeout) * time.Second):
			fmt.Printf("TIMEOUT (no response in %ds)\n", wsPingTimeout)
			failCount++
		}

		// Small delay between pings
		if i < wsPingCount {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Summary
	fmt.Printf("\n--- Ping statistics ---\n")
	fmt.Printf("%d pings sent, %d responses received, %.0f%% packet loss\n",
		wsPingCount, successCount, float64(failCount)/float64(wsPingCount)*100)

	if failCount > 0 {
		os.Exit(1)
	}
	return nil
}
