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
	discoveryTimeout int
	discoveryRouter  bool
)

var discoveryCmd = &cobra.Command{
	Use:   "discovery",
	Short: "Discover devices via serial or WebSocket",
	Long: `Send DISCOVERY_REQUEST to discover Fusain devices.

Modes:
  Serial (default): Send broadcast DISCOVERY_REQUEST to discover appliances directly.
                    Appliances respond with DEVICE_ANNOUNCE (0-50ms random delay).

  Router (--router): Send stateless DISCOVERY_REQUEST to a router (e.g., Slate).
                     Router responds with DEVICE_ANNOUNCE for each known device,
                     followed by an end-of-discovery marker (all zeros).

Per the Fusain spec:
  - Appliances ignore non-broadcast DISCOVERY_REQUEST
  - Routers respond to stateless address with known devices

Examples:
  # Direct serial discovery to Helios
  heliostat discovery --port /dev/ttyUSB0

  # WebSocket router discovery (Slate)
  heliostat discovery --url ws://slate.local/fusain --router

Exit codes:
  0 - Discovery successful (at least one device found)
  1 - Discovery failed (no devices or timeout)
  2 - Connection error`,
	RunE: runDiscovery,
}

func init() {
	rootCmd.AddCommand(discoveryCmd)
	discoveryCmd.Flags().IntVar(&discoveryTimeout, "timeout", 5, "Timeout in seconds for discovery")
	discoveryCmd.Flags().BoolVar(&discoveryRouter, "router", false, "Use router mode (stateless address)")
}

func runDiscovery(cmd *cobra.Command, args []string) error {
	// Open connection (serial or WebSocket)
	conn, connInfo, err := OpenConnection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection error: %v\n", err)
		os.Exit(2)
	}
	defer conn.Close()

	mode := "appliance"
	var address uint64 = fusain.AddressBroadcast
	if discoveryRouter {
		mode = "router"
		address = fusain.AddressStateless
	}

	fmt.Printf("Heliostat - Device Discovery\n")
	fmt.Printf("Connection: %s\n", connInfo)
	fmt.Printf("Mode: %s\n", mode)
	fmt.Printf("Timeout: %d seconds\n\n", discoveryTimeout)

	decoder := fusain.NewDecoder()

	// Create DISCOVERY_REQUEST packet
	discoveryPacket := fusain.NewDiscoveryRequest(address)
	wireBytes := fusain.MustEncodePacket(discoveryPacket)

	// Send discovery request
	fmt.Printf("Sending DISCOVERY_REQUEST (address=0x%016X)...\n", address)
	_, err = conn.Write(wireBytes)
	if err != nil {
		fmt.Printf("SEND FAILED: %v\n", err)
		os.Exit(2)
	}

	// Collect DEVICE_ANNOUNCE responses
	devices := make([]discoveryDeviceInfo, 0)
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
						device := parseDiscoveryAnnounce(packet)

						// End-of-discovery marker (router mode only)
						if device.isEndMarker() {
							if discoveryRouter {
								fmt.Printf("\nEnd of discovery marker received\n")
								done <- true
								return
							}
							// Ignore zero-capability devices in appliance mode
							continue
						}

						devices = append(devices, device)
						fmt.Printf("\nDevice found:\n")
						fmt.Printf("  Address: 0x%016X\n", device.address)
						fmt.Printf("  Motors: %d\n", device.motorCount)
						fmt.Printf("  Thermometers: %d\n", device.thermometerCount)
						fmt.Printf("  Pumps: %d\n", device.pumpCount)
						fmt.Printf("  Glow plugs: %d\n", device.glowCount)

						// In appliance mode, we might get multiple devices
						// but typically just one (Helios) on a point-to-point link
						// Continue listening for more responses
					}
				}
			}
		}
	}()

	// Wait for discovery to complete or timeout
	select {
	case <-done:
		// Discovery complete (router sent end marker)
	case err := <-errChan:
		fmt.Printf("READ FAILED: %v\n", err)
		os.Exit(2)
	case <-time.After(time.Duration(discoveryTimeout) * time.Second):
		if discoveryRouter {
			fmt.Printf("\nTIMEOUT: No end-of-discovery marker received in %ds\n", discoveryTimeout)
		} else {
			// In appliance mode, timeout is expected after all devices respond
			if len(devices) > 0 {
				fmt.Printf("\nDiscovery timeout reached\n")
			} else {
				fmt.Printf("\nTIMEOUT: No devices responded in %ds\n", discoveryTimeout)
			}
		}
	}

	// Summary
	fmt.Printf("\n--- Discovery summary ---\n")
	fmt.Printf("Devices found: %d\n", len(devices))

	if len(devices) == 0 {
		if discoveryRouter {
			fmt.Printf("No devices discovered. Router may not have any connected devices.\n")
		} else {
			fmt.Printf("No devices discovered. Check connection and device power.\n")
		}
		os.Exit(1)
	}

	return nil
}

type discoveryDeviceInfo struct {
	address          uint64
	motorCount       uint64
	thermometerCount uint64
	pumpCount        uint64
	glowCount        uint64
}

func (d discoveryDeviceInfo) isEndMarker() bool {
	return d.motorCount == 0 && d.thermometerCount == 0 && d.pumpCount == 0 && d.glowCount == 0
}

func parseDiscoveryAnnounce(p *fusain.Packet) discoveryDeviceInfo {
	payloadMap := p.PayloadMap()

	motorCount, _ := fusain.GetMapUint(payloadMap, 0)
	thermometerCount, _ := fusain.GetMapUint(payloadMap, 1)
	pumpCount, _ := fusain.GetMapUint(payloadMap, 2)
	glowCount, _ := fusain.GetMapUint(payloadMap, 3)

	return discoveryDeviceInfo{
		address:          p.Address(),
		motorCount:       motorCount,
		thermometerCount: thermometerCount,
		pumpCount:        pumpCount,
		glowCount:        glowCount,
	}
}
