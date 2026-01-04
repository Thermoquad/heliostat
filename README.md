# Heliostat - Helios Serial Protocol Analyzer

A CLI tool for monitoring and analyzing Helios serial protocol packets in real-time.

## Features

- **Raw Log Mode**: Display decoded packets in human-readable format (original heliostat behavior)
- **Error Detection Mode**: Track malformed packets, CRC errors, and anomalous values with live statistics
- **Reusable Protocol Package**: `pkg/helios_protocol` can be imported by other Go tools
- **Real-time Validation**: Detect suspicious packet data as it arrives
- **Configurable Statistics**: Periodic statistics summaries at configurable intervals

## Installation

```bash
cd tools/heliostat
go build
```

This produces the `heliostat` binary.

## Usage

### Raw Packet Log

Display all packets in human-readable format (same as original heliostat):

```bash
heliostat raw_log --port /dev/ttyUSB0
```

With custom baud rate:

```bash
heliostat raw_log --port /dev/ttyUSB0 --baud 115200
```

### Error Detection Mode

Track errors, malformed packets, and anomalous values with a live terminal UI (shows only errors by default):

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

Custom statistics interval (default 10 seconds):

```bash
heliostat error_detection --port /dev/ttyUSB0 --stats-interval 5
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
- **Invalid Counts**: `motor_count` or `temp_count` exceeding 10 (e.g., the motor_count=136 bug)
- **Length Mismatches**: Payload length doesn't match expected size based on declared counts

### Decode Errors
- **CRC Failures**: CRC-16-CCITT checksum validation failures
- **Framing Errors**: Unexpected byte stuffing or framing issues
- **Buffer Overflows**: Packets exceeding maximum size limits

### Anomalous Values
- **High RPM**: Motor RPM or target RPM exceeding 6000 (unrealistic for Helios motors)
- **Invalid Temperatures**: Values outside -50°C to 1000°C range
- **Invalid PWM**: PWM duty cycle exceeding PWM period

### Statistics Tracking
- Total packets received
- Valid packets vs. error packets (with percentages)
- Breakdown by error type
- Packet rate (packets/second)
- Error rate (errors/second)

## Example Output

### Error Detection Mode

```
Heliostat - Error Detection Mode
Port: /dev/ttyUSB0 @ 115200 baud
Statistics interval: 10 seconds
Press Ctrl+C to exit

=== Statistics (10 seconds) ===
Total Packets:       1,234
Valid Packets:       1,198 (97.1%)
CRC Errors:             12 (1.0%)
Decode Errors:           2 (0.2%)
Malformed Pkts:         22 (1.8%)
  Invalid Counts:       20
  Length Mismatch:       2
Anomalous Values:        5 (0.4%)
  High RPM (>6000):      3
  Invalid Temp:          2
Packet Rate:        123.4 pkts/sec
Error Rate:           3.6 errors/sec
================================

[15:04:05.123] VALIDATION ERROR: TELEMETRY_BUNDLE (0x25)
  CRC: OK
  Issue 1: Invalid motor_count=136 (max 10)
    motor_count=136 (max 10)
  Issue 2: Payload length mismatch: received=23, expected=15 (motors=136, temps=1)
    Length: received=23, expected=15
  State: IDLE (0x00), Error: 0x00
  >>> PACKET REJECTED <<<
```

## Protocol Package

The `pkg/helios_protocol` package provides reusable components for Helios protocol handling:

```go
import "github.com/Thermoquad/heliostat/pkg/helios_protocol"

// Create decoder
decoder := helios_protocol.NewDecoder()

// Decode bytes
packet, err := decoder.DecodeByte(byteValue)

// Validate packet
errors := helios_protocol.ValidatePacket(packet)

// Format for display
output := helios_protocol.FormatPacket(packet)

// Track statistics
stats := helios_protocol.NewStatistics()
stats.Update(packet, decodeErr, validationErrors)
fmt.Println(stats.String())
```

### Package Structure

- `constants.go` - Protocol constants (message types, framing bytes, limits)
- `packet.go` - Packet structure and methods
- `decoder.go` - State machine decoder
- `crc.go` - CRC-16-CCITT calculation
- `formatter.go` - Human-readable packet formatting
- `validator.go` - Packet validation and anomaly detection
- `statistics.go` - Statistics tracking and reporting

## Development

Build:
```bash
go build
```

Test:
```bash
go test ./...
```

Format:
```bash
go fmt ./...
```

Update dependencies:
```bash
go mod tidy
```

## License

Apache-2.0

Copyright (c) 2025 Kaz Walker, Thermoquad
