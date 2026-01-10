// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"github.com/spf13/cobra"
)

var (
	// Global flags
	portName string
	baudRate int
)

var rootCmd = &cobra.Command{
	Use:   "heliostat",
	Short: "Helios Serial Protocol Analyzer",
	Long: `Heliostat - A CLI tool for monitoring and analyzing Helios serial protocol packets.

Provides commands for raw packet logging and advanced error detection to help
diagnose communication issues and protocol anomalies.`,
	Version: "2.0.0",
}

func init() {
	// Global persistent flags (available to all subcommands)
	rootCmd.PersistentFlags().StringVarP(&portName, "port", "p", "", "Serial port device (required)")
	rootCmd.PersistentFlags().IntVarP(&baudRate, "baud", "b", 115200, "Baud rate")
	rootCmd.MarkPersistentFlagRequired("port")
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}
