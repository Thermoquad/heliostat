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
	wsDiscoveryTimeout int
)

var wsDiscoveryCmd = &cobra.Command{
	Use:   "ws_discovery",
	Short: "Test device discovery via WebSocket bridge",
	Long: `Send DISCOVERY_REQUEST to the Slate WebSocket router and display DEVICE_ANNOUNCE responses.

This command tests the device discovery handshake:
  1. Send DISCOVERY_REQUEST to the router (stateless address)
  2. Router responds with DEVICE_ANNOUNCE for each known device
  3. Router sends end-of-discovery marker (DEVICE_ANNOUNCE with all zeros)

This is useful for verifying:
  - Slate is tracking connected devices (e.g., Helios)
  - DEVICE_ANNOUNCE contains correct capabilities
  - Discovery handshake completes successfully

Exit codes:
  0 - Discovery successful (at least one device found)
  1 - Discovery failed (no devices or timeout)
  2 - Connection error`,
	RunE: runWsDiscovery,
}

func init() {
	rootCmd.AddCommand(wsDiscoveryCmd)
	wsDiscoveryCmd.Flags().IntVar(&wsDiscoveryTimeout, "timeout", 5, "Timeout in seconds for discovery")
}

func runWsDiscovery(cmd *cobra.Command, args []string) error {
	// Open connection (serial or WebSocket)
	conn, connInfo, err := OpenConnection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection error: %v\n", err)
		os.Exit(2)
	}
	defer conn.Close()

	fmt.Printf("Heliostat - Device Discovery Test\n")
	fmt.Printf("Connection: %s\n", connInfo)
	fmt.Printf("Timeout: %d seconds\n\n", wsDiscoveryTimeout)

	decoder := fusain.NewDecoder()

	// Create DISCOVERY_REQUEST packet for stateless address (router)
	discoveryPacket := fusain.NewDiscoveryRequest(fusain.AddressStateless)
	wireBytes := fusain.MustEncodePacket(discoveryPacket)

	// Send discovery request
	fmt.Printf("Sending DISCOVERY_REQUEST...\n")
	_, err = conn.Write(wireBytes)
	if err != nil {
		fmt.Printf("SEND FAILED: %v\n", err)
		os.Exit(2)
	}

	// Collect DEVICE_ANNOUNCE responses
	devices := make([]deviceInfo, 0)
	done := make(chan bool, 1)
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
					continue
				}
				if packet != nil {
					msgType := packet.Type()
					if msgType == fusain.MsgDeviceAnnounce {
						device := parseDeviceAnnounce(packet)
						if device.isEndMarker() {
							fmt.Printf("\nEnd of discovery marker received\n")
							done <- true
							return
						}
						devices = append(devices, device)
						fmt.Printf("\nDevice found:\n")
						fmt.Printf("  Address: 0x%016X\n", device.address)
						fmt.Printf("  Motors: %d\n", device.motorCount)
						fmt.Printf("  Thermometers: %d\n", device.thermometerCount)
						fmt.Printf("  Pumps: %d\n", device.pumpCount)
						fmt.Printf("  Glow plugs: %d\n", device.glowCount)
					}
					// Ignore non-discovery packets (telemetry, etc.)
				}
			}
		}
	}()

	// Wait for discovery to complete or timeout
	select {
	case <-done:
		// Discovery complete
	case err := <-errChan:
		fmt.Printf("READ FAILED: %v\n", err)
		os.Exit(2)
	case <-time.After(time.Duration(wsDiscoveryTimeout) * time.Second):
		fmt.Printf("\nTIMEOUT: No end-of-discovery marker received in %ds\n", wsDiscoveryTimeout)
	}

	// Summary
	fmt.Printf("\n--- Discovery summary ---\n")
	fmt.Printf("Devices found: %d\n", len(devices))

	if len(devices) == 0 {
		fmt.Printf("No devices discovered. Slate may not have tracked any connected devices yet.\n")
		os.Exit(1)
	}

	return nil
}

type deviceInfo struct {
	address          uint64
	motorCount       uint64
	thermometerCount uint64
	pumpCount        uint64
	glowCount        uint64
}

func (d deviceInfo) isEndMarker() bool {
	return d.motorCount == 0 && d.thermometerCount == 0 && d.pumpCount == 0 && d.glowCount == 0
}

func parseDeviceAnnounce(p *fusain.Packet) deviceInfo {
	payloadMap := p.PayloadMap()

	motorCount, _ := fusain.GetMapUint(payloadMap, 0)
	thermometerCount, _ := fusain.GetMapUint(payloadMap, 1)
	pumpCount, _ := fusain.GetMapUint(payloadMap, 2)
	glowCount, _ := fusain.GetMapUint(payloadMap, 3)

	return deviceInfo{
		address:          p.Address(),
		motorCount:       motorCount,
		thermometerCount: thermometerCount,
		pumpCount:        pumpCount,
		glowCount:        glowCount,
	}
}
