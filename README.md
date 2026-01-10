# Heliostat - Fusain Protocol Analyzer

A CLI tool for monitoring and analyzing Fusain serial protocol packets in real-time.

**Protocol Version:** Fusain v2.0

## Features

- **Raw Log Mode**: Display decoded packets in human-readable format
- **Error Detection Mode**: Track malformed packets, CRC errors, and anomalous values with live statistics
- **Reference Go Implementation**: `pkg/fusain` provides the canonical Go implementation of the Fusain protocol
- **Real-time Validation**: Detect suspicious packet data as it arrives
- **TUI Mode**: Interactive terminal UI with live statistics (default)

## Installation

```bash
cd tools/heliostat
go build
```

This produces the `heliostat` binary.

## Usage

### Raw Packet Log

Display all packets in human-readable format:

```bash
heliostat raw_log --port /dev/ttyUSB0
```

With custom baud rate:

```bash
heliostat raw_log --port /dev/ttyUSB0 --baud 115200
```

### Error Detection Mode

Track errors, malformed packets, and anomalous values with a live terminal UI:

```bash
heliostat error_detection --port /dev/ttyUSB0
```

Show all packets (not just errors):

```bash
heliostat error_detection --port /dev/ttyUSB0 --show-all
```

Use text mode instead of TUI:

```bash
heliostat error_detection --port /dev/ttyUSB0 --tui=false
```

Custom statistics interval (default 10 seconds, text mode only):

```bash
heliostat error_detection --port /dev/ttyUSB0 --tui=false --stats-interval 5
```

### Help

```bash
heliostat --help
heliostat raw_log --help
heliostat error_detection --help
```

## Error Detection Features

The `error_detection` command validates packets and detects:

### Malformed Packets
- **Invalid Counts**: `motor_count` or `temp_count` exceeding limits
- **Length Mismatches**: Payload length doesn't match expected size

### Decode Errors
- **CRC Failures**: CRC-16-CCITT checksum validation failures
- **Framing Errors**: Unexpected byte stuffing or framing issues
- **Buffer Overflows**: Packets exceeding maximum size limits

### Anomalous Values
- **High RPM**: Motor RPM or target RPM exceeding 6000
- **Invalid Temperatures**: Values outside -50°C to 1000°C range
- **Invalid PWM**: PWM duty cycle exceeding PWM period

### Statistics Tracking
- Total packets received
- Valid packets vs. error packets (with percentages)
- Breakdown by error type
- Packet rate (packets/second)
- Error rate (errors/second)

## Protocol Package

The `pkg/fusain` package provides the reference Go implementation of the Fusain protocol:

```go
import "github.com/Thermoquad/heliostat/pkg/fusain"

// Create decoder
decoder := fusain.NewDecoder()

// Decode bytes
packet, err := decoder.DecodeByte(byteValue)

// Validate packet
errors := fusain.ValidatePacket(packet)

// Format for display
output := fusain.FormatPacket(packet)

// Track statistics
stats := fusain.NewStatistics()
stats.Update(packet, decodeErr, validationErrors)
fmt.Println(stats.String())
```

### Package Structure

```
pkg/fusain/
├── constants.go    # Protocol constants (message types, framing bytes, limits)
├── packet.go       # Packet structure and methods
├── decoder.go      # State machine decoder with byte unstuffing
├── crc.go          # CRC-16-CCITT calculation
├── formatter.go    # Human-readable packet formatting
├── validator.go    # Packet validation and anomaly detection
├── statistics.go   # Statistics tracking and reporting
├── fusain_test.go  # Comprehensive unit tests
└── fuzz_test.go    # Fuzz testing
```

The package is a **standalone Go module** with no external dependencies (stdlib only),
allowing it to be imported by other Go tools.

## Development

### Build

```bash
go build
```

### Test

```bash
# Run all tests
go test ./...

# Run fusain package tests with coverage
task fusain:test

# Run with custom fuzz rounds
task fusain:test -- 10000

# CI mode (format check, vet, 100k fuzz rounds)
task fusain:ci
```

### Format

```bash
go fmt ./...
```

### Update Dependencies

```bash
go mod tidy
```

## Protocol Specification

The Fusain protocol specification is maintained in the Sphinx documentation:

- **Location:** `origin/documentation/source/specifications/fusain/`
- **Contents:** Packet format, message types, payloads, communication patterns

## Related Projects

- **Fusain Library (C):** `modules/lib/fusain/` - Embedded C implementation for Zephyr RTOS
- **Helios Firmware:** `apps/helios/` - Burner ICU that sends telemetry
- **Slate Firmware:** `apps/slate/` - Controller that receives telemetry

## License

**Heliostat** is licensed under the GNU General Public License v2.0 or later (GPL-2.0-or-later).

**Note:** The `pkg/fusain` protocol library is licensed separately under Apache-2.0 to allow
broader use in both open source and proprietary applications.

Copyright (c) 2025 Kaz Walker, Thermoquad
