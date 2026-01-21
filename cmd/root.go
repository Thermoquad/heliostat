// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"github.com/spf13/cobra"
)

var (
	// Serial connection flags
	portName string
	baudRate int

	// WebSocket connection flags
	wsURL         string
	wsUsername    string
	wsNoSSLVerify bool
)

var rootCmd = &cobra.Command{
	Use:   "heliostat",
	Short: "Helios Serial Protocol Analyzer",
	Long: `Heliostat - A CLI tool for monitoring and analyzing Helios serial protocol packets.

Provides commands for raw packet logging and advanced error detection to help
diagnose communication issues and protocol anomalies.

Connection modes:
  Serial:    --port /dev/ttyUSB0 [--baud 115200]
  WebSocket: --url ws://host/path [--username user]

For WebSocket authentication, the password is read from the FUSAIN_PASSWORD
environment variable, or prompted interactively if not set. The --password
flag is intentionally not provided to avoid leaking credentials in shell history.`,
	Version: "2.1.0",
}

func init() {
	// Serial connection flags
	rootCmd.PersistentFlags().StringVarP(&portName, "port", "p", "", "Serial port device")
	rootCmd.PersistentFlags().IntVarP(&baudRate, "baud", "b", 115200, "Baud rate (serial only)")

	// WebSocket connection flags
	rootCmd.PersistentFlags().StringVarP(&wsURL, "url", "u", "", "WebSocket URL (ws:// or wss://)")
	rootCmd.PersistentFlags().StringVar(&wsUsername, "username", "", "Username for HTTP Basic auth")
	rootCmd.PersistentFlags().BoolVar(&wsNoSSLVerify, "no-ssl-verify", false, "Skip TLS certificate verification (wss:// only)")
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}
