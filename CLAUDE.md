# Heliostat - AI Assistant Guide

## Project Overview

Heliostat is a Go CLI tool for monitoring and analyzing Fusain protocol packets in real-time. It provides both raw packet logging and advanced error detection capabilities.

**Purpose:** Diagnose communication issues, validate protocol implementation, and detect anomalous telemetry data from Helios burner ICU.

**Key Features:**
- Decode and display Fusain protocol packets
- Validate packet structure and detect anomalies
- Track statistics (packet rates, error rates, success rates)
- **Reference Go implementation** of Fusain protocol (`pkg/fusain/`)

**Protocol Specification:** `origin/documentation/source/specifications/fusain/` (Sphinx docs)

---

## Architecture

### Technology Stack
- **Language:** Go 1.25.5
- **CLI Framework:** Cobra (github.com/spf13/cobra)
- **Serial I/O:** go.bug.st/serial
- **Module:** github.com/Thermoquad/heliostat

### Directory Structure

```
heliostat/
├── go.mod                           # Go module definition (references pkg/fusain)
├── go.sum                           # Dependency checksums
├── main.go                          # Cobra CLI entrypoint
├── Taskfile.dist.yml                # Task runner (includes fusain tasks)
├── README.md                        # User documentation
├── CLAUDE.md                        # This file
├── cmd/                             # Cobra command definitions
│   ├── root.go                      # Root command and global flags
│   ├── raw_log.go                   # Raw log command
│   ├── error_detection.go           # Error detection command
│   └── tui.go                       # Bubbletea TUI model
└── pkg/
    └── fusain/                      # Reference Go implementation (separate module)
        ├── go.mod                   # Standalone module for external imports
        ├── Taskfile.dist.yml        # Fusain-specific tasks (test, coverage, ci)
        ├── constants.go             # Protocol constants
        ├── packet.go                # Packet structure with 8-byte addressing
        ├── decoder.go               # State machine decoder
        ├── crc.go                   # CRC-16-CCITT
        ├── formatter.go             # Packet formatting (all message types)
        ├── validator.go             # Validation and anomaly detection
        ├── statistics.go            # Statistics tracking
        └── fusain_test.go           # Comprehensive tests (100% coverage)
```

> **Note:** The `pkg/fusain/` package is a **separate Go module** that can be imported
> independently by other Go tools. It has no external dependencies (stdlib only).
>
> **Import:** `import "github.com/Thermoquad/heliostat/pkg/fusain"`

---

## Code Structure

### Package: `fusain`

Reference Go implementation of the Fusain protocol. Separate module with no external dependencies.

#### constants.go

Protocol-level constants:
- Framing bytes: `START_BYTE (0x7E)`, `END_BYTE (0x7F)`, `ESC_BYTE (0x7D)`, `ESC_XOR (0x20)`
- Size limits: `MAX_PACKET_SIZE (128)`, `MAX_PAYLOAD_SIZE (114)`, `ADDRESS_SIZE (8)`
- CRC parameters: `CRC_POLYNOMIAL (0x1021)`, `CRC_INITIAL (0xFFFF)`
- Special addresses: `ADDRESS_BROADCAST (0x0)`, `ADDRESS_STATELESS (0xFFFFFFFFFFFFFFFF)`
- Message types:
  - Configuration Commands (0x10-0x1F): MOTOR_CONFIG, PUMP_CONFIG, TEMP_CONFIG, etc.
  - Control Commands (0x20-0x2F): STATE_COMMAND, MOTOR_COMMAND, PING_REQUEST, etc.
  - Telemetry Data (0x30-0x3F): STATE_DATA, MOTOR_DATA, TEMP_DATA, PING_RESPONSE, etc.
  - Errors (0xE0-0xEF): ERROR_INVALID_CMD, ERROR_STATE_REJECT
- Decoder states: `STATE_IDLE`, `STATE_LENGTH`, `STATE_ADDRESS`, `STATE_TYPE`, `STATE_PAYLOAD`, `STATE_CRC1`, `STATE_CRC2`

#### packet.go

**Type: `Packet`**
- Fields: `length`, `address`, `msgType`, `payload`, `crc`, `timestamp`
- Methods: `Length()`, `Address()`, `Type()`, `Payload()`, `CRC()`, `Timestamp()`, `IsBroadcast()`, `IsStateless()`
- Constructor: `NewPacket(length, address, msgType, payload, crc) *Packet`

#### decoder.go

**Type: `Decoder`**
- State machine for decoding byte stream
- Handles byte stuffing (escape sequences)
- Validates CRC before returning packet

**Methods:**
- `NewDecoder() *Decoder` - Create new decoder
- `DecodeByte(b byte) (*Packet, error)` - Process single byte, returns packet when complete
- `Reset()` - Reset to idle state
- `GetRawBytes() []byte` - Get accumulated raw bytes (for debugging)

**State Machine:**
1. `STATE_IDLE` - Waiting for `START_BYTE`
2. `STATE_LENGTH` - Read payload length
3. `STATE_ADDRESS` - Read 8-byte address (little-endian)
4. `STATE_TYPE` - Read message type
5. `STATE_PAYLOAD` - Read payload bytes
6. `STATE_CRC1` - Read CRC high byte
7. `STATE_CRC2` - Read CRC low byte
8. Return to idle on `END_BYTE` after validating CRC

#### crc.go

**Function: `CalculateCRC(data []byte) uint16`**
- Implements CRC-16-CCITT
- Polynomial: 0x1021, Initial: 0xFFFF
- Used for packet validation

#### formatter.go

**Functions:**
- `FormatPacket(p *Packet) string` - Human-readable packet format with timestamp
- `FormatMessageType(msgType uint8) string` - Message type name (e.g., "TELEMETRY_BUNDLE")
- `FormatPayload(msgType uint8, payload []byte) string` - Payload interpretation

**Payload Formatting:**
- Parses payload based on message type
- Extracts fields (state, error, motor data, temperatures, etc.)
- Formats multi-byte integers (little-endian)
- Converts float64 from bits using `unsafe.Pointer`

**Helper Functions:**
- `formatDuration(ms uint64) string` - Human-readable uptime (e.g., "1 hour and 23 minutes")
- `float64frombits(b uint64) float64` - Convert uint64 bits to float64

#### validator.go

**Type: `AnomalyType`**
- Enum: `ANOMALY_INVALID_COUNT`, `ANOMALY_LENGTH_MISMATCH`, `ANOMALY_HIGH_RPM`, `ANOMALY_INVALID_TEMP`, `ANOMALY_INVALID_PWM`

**Type: `ValidationError`**
- Fields: `Type`, `Message`, `Details` (map with context)
- Implements `error` interface

**Function: `ValidatePacket(p *Packet) []ValidationError`**
- Returns slice of validation errors (empty if valid)
- Validates based on message type

**Validation Rules:**

See `pkg/fusain/validator.go` for current validation rules. Validates message structure, count limits, and value ranges for telemetry data.

#### statistics.go

**Type: `Statistics`**
- Tracks packet statistics and error rates
- Fields:
  - Counters: `TotalPackets`, `ValidPackets`, `CRCErrors`, `DecodeErrors`
  - Malformed: `MalformedPackets`, `InvalidCounts`, `LengthMismatches`
  - Anomalous: `AnomalousValues`, `HighRPM`, `InvalidTemp`, `InvalidPWM`
  - Rates: `PacketRate`, `ErrorRate` (calculated)
  - Timestamps: `StartTime`, `LastUpdateTime`

**Methods:**
- `NewStatistics() *Statistics` - Create new tracker
- `Update(packet, decodeErr, validationErrors)` - Update counters based on packet/errors
- `CalculateRates()` - Calculate packets/sec and errors/sec
- `String() string` - Formatted statistics summary
- `Reset()` - Reset all counters

### Commands: `cmd/`

#### cmd/root.go

Root command configuration using Cobra.

**Global Flags:**
- `--port, -p` - Serial port device (required)
- `--baud, -b` - Baud rate (default: 115200)

**Initialization:**
- Registers subcommands (`raw_log`, `error_detection`)
- Marks `--port` as required
- Defines version (2.0.0)

#### cmd/raw_log.go

**Command:** `heliostat raw_log --port <device>`

**Behavior:**
- Open serial port
- Create decoder
- Read bytes in loop
- Decode and print packets using `FormatPacket()`
- Print decode errors

**Purpose:** Provides same functionality as original heliostat (raw packet logging)

#### cmd/error_detection.go

**Command:** `heliostat error_detection --port <device> [flags]`

**Flags:**
- `--show-all` - Show all packets (default: false, only errors shown)
- `--stats-interval <seconds>` - Statistics update interval (default: 10)
- `--tui` - Use terminal UI mode (default: true)

**Behavior:**
- Open serial port
- Create decoder and statistics tracker
- Track synchronization (ignore errors before first valid packet)
- Choose mode: TUI (default) or text mode
- Read bytes in background goroutine
- For each packet:
  - Validate using `ValidatePacket()`
  - Update statistics
  - Display errors immediately (TUI: error log, text: color-coded)
  - Display valid packets only if `--show-all`
- Update statistics display at intervals (TUI: live, text: periodic)

**TUI Mode (default):**
- Built with Bubbletea and Lipgloss
- Shows header with connection info and controls
- Sync status indicator (waiting/synchronized)
- Live statistics box (total, valid, errors, rates) - updates continuously in real-time
- Scrolling error log with timestamps
- Press 'q' to quit
- Note: `--stats-interval` flag is ignored in TUI mode (statistics always update live)

**Text Mode (`--tui=false`):**
- Decode errors: Red, shows CRC mismatch or framing issue
- Validation errors: Yellow/red, shows issue type and details
- Includes packet context (state, error code) for telemetry bundles
- Statistics printed at intervals

#### cmd/tui.go

**TUI Model Structure:**
- Built using Bubbletea framework
- Receives messages from serial goroutine
- Updates display in real-time

**Messages:**
- `tickMsg` - 1-second ticker for rate calculations
- `serialDataMsg` - Packet data, decode errors, validation errors
- `syncMsg` - First packet received (synchronized)

**Display Components:**
- Header: Title, port info, mode, controls
- Sync status: Waiting or synchronized (with invalid byte count)
- Statistics box: Formatted with colors (errors in red, values in green)
- Event log: Scrolling list of recent errors/warnings with timestamps
- Auto-sizing to terminal dimensions

---

## Protocol Details

### Fusain Serial Protocol

**Framing:**
```
START_BYTE (0x7E)
LENGTH (1 byte) - payload length
ADDRESS (8 bytes) - device address (little-endian)
TYPE (1 byte) - message type
PAYLOAD (variable, 0-114 bytes)
CRC_HIGH (1 byte)
CRC_LOW (1 byte)
END_BYTE (0x7F)
```

**Special Addresses:**
- `0x0000000000000000` - Broadcast (all devices)
- `0xFFFFFFFFFFFFFFFF` - Stateless (routers, subscriptions)

**Byte Stuffing:**
- If data byte equals `START_BYTE`, `END_BYTE`, or `ESC_BYTE`:
  - Replace with: `ESC_BYTE` + (original ^ `ESC_XOR`)
- Decoder handles unstuffing automatically

**CRC:** CRC-16-CCITT (big-endian) over `[LENGTH, ADDRESS, TYPE, PAYLOAD]`

**References:**
- **Specification:** `origin/documentation/source/specifications/fusain/` (canonical)
- **C Implementation:** `modules/lib/fusain/`
- **Go Implementation:** `tools/heliostat/pkg/fusain/` (this package)

### Message Types

**Configuration Commands (Controller → Appliance, 0x10-0x1F):**
- `0x10` MOTOR_CONFIG - Configure motor controller (48 bytes)
- `0x11` PUMP_CONFIG - Configure pump controller (16 bytes)
- `0x12` TEMPERATURE_CONFIG - Configure temperature controller (48 bytes)
- `0x13` GLOW_CONFIG - Configure glow plug (16 bytes)
- `0x14` DATA_SUBSCRIPTION - Subscribe to appliance data (8 bytes)
- `0x15` DATA_UNSUBSCRIBE - Unsubscribe from data (8 bytes)
- `0x16` TELEMETRY_CONFIG - Enable/disable telemetry (8 bytes)
- `0x17` TIMEOUT_CONFIG - Configure communication timeout (8 bytes)
- `0x1F` DISCOVERY_REQUEST - Request device capabilities (0 bytes)

**Control Commands (Controller → Appliance, 0x20-0x2F):**
- `0x20` STATE_COMMAND - Set system mode/state (8 bytes)
- `0x21` MOTOR_COMMAND - Set motor RPM (8 bytes)
- `0x22` PUMP_COMMAND - Set pump rate (8 bytes)
- `0x23` GLOW_COMMAND - Control glow plug (8 bytes)
- `0x24` TEMPERATURE_COMMAND - Temperature controller control (20 bytes)
- `0x25` SEND_TELEMETRY - Request telemetry (polling mode, 8 bytes)
- `0x2F` PING_REQUEST - Heartbeat/connectivity check (0 bytes)

**Telemetry Data (Appliance → Controller, 0x30-0x3F):**
- `0x30` STATE_DATA - System state and status (16 bytes)
- `0x31` MOTOR_DATA - Motor telemetry (32 bytes)
- `0x32` PUMP_DATA - Pump status and events (16 bytes)
- `0x33` GLOW_DATA - Glow plug status (12 bytes)
- `0x34` TEMPERATURE_DATA - Temperature readings (32 bytes)
- `0x35` DEVICE_ANNOUNCE - Device capabilities (8 bytes)
- `0x3F` PING_RESPONSE - Heartbeat response (4 bytes)

**Errors (Bidirectional, 0xE0-0xEF):**
- `0xE0` ERROR_INVALID_CMD - Command validation failed (4 bytes)
- `0xE1` ERROR_STATE_REJECT - Command rejected by state machine (4 bytes)

### Payload Structures

See the Fusain protocol specification for detailed payload structures:
`origin/documentation/source/specifications/fusain/packet-payloads.rst`

**Key payload notes:**
- All multi-byte integers are little-endian (except CRC which is big-endian)
- DEVICE_ANNOUNCE counts are u8 (1 byte each): motor_count, temp_count, pump_count, glow_count
- Temperature and PID values use f64 (8-byte IEEE 754 floats)

---

## Development

### Building

```bash
go build
```

Produces `heliostat` binary in current directory.

### Testing

```bash
task fusain:test           # Run all tests with 1000 fuzz rounds
task fusain:test -- 10000  # Run with custom fuzz round count
task fusain:ci             # Run CI checks (format, vet, 100k fuzz rounds)
task fusain:coverage       # Generate coverage report
```

**Test Coverage:** The `pkg/fusain/` package maintains 100% test coverage.

**Manual Testing:**
1. Connect to Helios UART (e.g., `/dev/ttyUSB0`)
2. Run `heliostat raw_log --port /dev/ttyUSB0`
3. Verify packets decode correctly
4. Run `heliostat error_detection --port /dev/ttyUSB0`
5. Trigger corrupted packet (if possible) and verify detection

### Code Style

- Use `go fmt` for formatting
- Follow Go naming conventions
- Document exported functions and types
- Use descriptive variable names

### Dependencies

**Direct:**
- `github.com/spf13/cobra` - CLI framework
- `go.bug.st/serial` - Serial port I/O
- `github.com/charmbracelet/bubbletea` - Terminal UI framework
- `github.com/charmbracelet/lipgloss` - Terminal styling

**Update:**
```bash
go get -u ./...
go mod tidy
```

---

## Adding New Commands

1. Create new file in `cmd/` (e.g., `cmd/my_command.go`)
2. Define command using Cobra:
   ```go
   var myCmd = &cobra.Command{
       Use:   "my_command",
       Short: "Description",
       RunE:  runMyCommand,
   }

   func init() {
       rootCmd.AddCommand(myCmd)
   }

   func runMyCommand(cmd *cobra.Command, args []string) error {
       // Access global flags: portName, baudRate
       // Implementation...
   }
   ```
3. Use `fusain` package for decoding/formatting
4. Rebuild and test

---

## Common Tasks for AI Assistants

### Adding New Message Type

1. Add constant to `pkg/fusain/constants.go`
2. Add case to `FormatMessageType()` in `formatter.go`
3. Add payload formatter to `FormatPayload()` in `formatter.go`
4. If validation needed, add case to `ValidatePacket()` in `validator.go`

### Adding New Validation Rule

1. Add `AnomalyType` to `validator.go`
2. Implement validation logic in appropriate function
3. Update statistics counters in `statistics.go`
4. Add error formatting in `cmd/error_detection.go`

### Modifying Statistics Tracking

1. Add counter fields to `Statistics` struct in `statistics.go`
2. Update `Update()` method to increment counters
3. Update `String()` method to format output
4. Update `Reset()` method to reset new counters

---

## Relationship to Other Projects

### Fusain Protocol Specification

**Location:** `origin/documentation/source/specifications/fusain/`

**Relationship:** The Sphinx documentation is the canonical specification. Both C and Go
implementations follow this specification.

### Fusain Library (C)

**Location:** `modules/lib/fusain/`

**Relationship:** Heliostat's `pkg/fusain/` implements the same protocol in Go.

**Shared Concepts:**
- Protocol constants (message types, framing bytes)
- CRC-16-CCITT algorithm
- Byte stuffing/unstuffing
- State machine decoder

**Differences:**
- Fusain (C) is for embedded systems (Zephyr RTOS)
- Heliostat's `pkg/fusain/` (Go) is the reference Go implementation for desktop tools

### Slate Firmware (C)

**Location:** `apps/slate/`

**Relationship:** Slate uses Fusain to communicate with Helios. Heliostat helps debug that communication.

**Validation Alignment:**
- Heliostat's validation rules match protocol limits defined in the specification
- Validation thresholds align with Slate's filtering

### Helios Firmware (C)

**Location:** `apps/helios/`

**Relationship:** Helios is the ICU that sends telemetry. Heliostat decodes and validates that telemetry.

**Protocol Version:** Heliostat supports Fusain Protocol v2.0 (same as Helios)

---

## Troubleshooting

### Build Errors

**Import cycle:** Ensure no circular imports between package files

**Undefined reference:** Check package name matches directory name (`fusain`)

**Version conflicts:** Run `go mod tidy` to resolve dependencies

### Runtime Issues

**Serial port permission denied:** Add user to `dialout` group or use `sudo`

**No packets detected:** Verify baud rate matches Helios configuration (default 115200)

**CRC errors:** May indicate communication issue (loose connection, electrical noise)

**Malformed packets:** May indicate Helios firmware bug (use error_detection to diagnose)

---

## Git Workflow

**IMPORTANT:** This tool is part of the Thermoquad organization. Follow the git workflow documented in `../../CLAUDE.md`.

**Summary:**
1. Show changes with `git diff`
2. Explain modifications
3. Show proposed commit message
4. Wait for approval
5. Commit

**Commit Style:** Conventional Commits

**Scopes:**
- `protocol` - Protocol package changes
- `cmd` - CLI command changes
- `docs` - Documentation updates
- `deps` - Dependency updates

**Example:**
```
feat(protocol): add PWM duty validation to telemetry bundles

Add validation to detect when PWM duty exceeds PWM period in motor data.
This catches hardware configuration errors that could damage motors.

Validation applied to:
- TELEMETRY_BUNDLE motor data
- MOTOR_DATA packets

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
```

---

## Resources

**Go Documentation:**
- Cobra: https://github.com/spf13/cobra
- Serial Port: https://pkg.go.dev/go.bug.st/serial

**Protocol Reference:**
- Fusain Specification: `origin/documentation/source/specifications/fusain/` (canonical)
- Fusain library (C): `modules/lib/fusain/CLAUDE.md`
- Helios firmware: `apps/helios/CLAUDE.md`
- Slate firmware: `apps/slate/CLAUDE.md`

**Organization:**
- Thermoquad CLAUDE.md: `../../CLAUDE.md`

---

## AI Assistant Operations

To reload all organization CLAUDE.md files or run a content integrity check, see the **CLAUDE.md Reload** and **Content Integrity Check** sections in the [Thermoquad Organization CLAUDE.md](../../CLAUDE.md).

---

**Last Updated:** 2026-01-09

**Maintainer:** Kaz Walker
