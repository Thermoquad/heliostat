// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad
//
// Heliostat - Fusain Protocol Analyzer
//
// A CLI tool for monitoring and analyzing Helios serial protocol packets
// with commands for raw logging and advanced error detection.

package main

import (
	"fmt"
	"os"

	"github.com/Thermoquad/heliostat/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
