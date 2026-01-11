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
- **CBOR:** fxamacker/cbor/v2
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
        ├── packet.go                # Packet structure with CBOR payload
        ├── decoder.go               # State machine decoder
        ├── cbor.go                  # CBOR parsing helpers
        ├── crc.go                   # CRC-16-CCITT
        ├── formatter.go             # Packet formatting (all message types)
        ├── validator.go             # Validation and anomaly detection
        ├── statistics.go            # Statistics tracking
        ├── fusain_test.go           # Comprehensive tests
        └── fuzz_test.go             # Fuzz testing
```

> **Note:** The `pkg/fusain/` package is a **separate Go module** that can be imported
> independently by other Go tools.
>
> **Import:** `import "github.com/Thermoquad/heliostat/pkg/fusain"`

---

## Code Structure

### Package: `fusain`

Reference Go implementation of the Fusain protocol with CBOR-encoded payloads.

#### constants.go

Protocol-level constants:
- Framing bytes: `StartByte (0x7E)`, `EndByte (0x7F)`, `EscByte (0x7D)`, `EscXor (0x20)`
- Size limits: `MaxPacketSize (128)`, `MaxPayloadSize (114)`, `AddressSize (8)`
- CRC parameters: `CrcPolynomial (0x1021)`, `CrcInitial (0xFFFF)`
- Special addresses: `AddressBroadcast (0x0)`, `AddressStateless (0xFFFFFFFFFFFFFFFF)`
- Message types:
  - Configuration Commands (0x10-0x1F): MsgMotorConfig, MsgPumpConfig, MsgTempConfig, etc.
  - Control Commands (0x20-0x2F): MsgStateCommand, MsgMotorCommand, MsgPingRequest, etc.
  - Telemetry Data (0x30-0x3F): MsgStateData, MsgMotorData, MsgTempData, MsgPingResponse, etc.
  - Errors (0xE0-0xEF): MsgErrorInvalidCmd, MsgErrorStateReject
- Decoder states: `stateIdle`, `stateLength`, `stateAddress`, `statePayload`, `stateCRC1`, `stateCRC2`

#### cbor.go

CBOR parsing functions using fxamacker/cbor/v2:
- `ParseCBORMessage(data []byte) (msgType uint8, payload map[int]interface{}, err error)`
- `GetMapUint(m map[int]interface{}, key int) (uint64, bool)`
- `GetMapInt(m map[int]interface{}, key int) (int64, bool)`
- `GetMapFloat(m map[int]interface{}, key int) (float64, bool)`
- `GetMapBool(m map[int]interface{}, key int) (bool, bool)`
- `GetMapBytes(m map[int]interface{}, key int) ([]byte, bool)`

#### packet.go

**Type: `Packet`**
- Fields: `length`, `address`, `cborPayload`, `crc`, `timestamp` (plus cached parsed values)
- Methods: `Length()`, `Address()`, `Type()`, `Payload()`, `PayloadMap()`, `ParseError()`, `CRC()`, `Timestamp()`, `IsBroadcast()`, `IsStateless()`
- Constructor: `NewPacket(length, address, cborPayload, crc) *Packet`

**Lazy CBOR Parsing:** The message type and payload map are parsed from CBOR on first access
and cached for subsequent calls.

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
1. `stateIdle` - Waiting for `StartByte`
2. `stateLength` - Read CBOR payload length
3. `stateAddress` - Read 8-byte address (little-endian)
4. `statePayload` - Read CBOR payload bytes (includes message type)
5. `stateCRC1` - Read CRC high byte
6. `stateCRC2` - Read CRC low byte
7. Return to idle on `EndByte` after validating CRC

**Note:** There is no separate `stateType` - the message type is embedded in the CBOR payload.

#### crc.go

**Function: `CalculateCRC(data []byte) uint16`**
- Implements CRC-16-CCITT
- Polynomial: 0x1021, Initial: 0xFFFF
- Used for packet validation

#### formatter.go

**Functions:**
- `FormatPacket(p *Packet) string` - Human-readable packet format with timestamp
- `FormatMessageType(msgType uint8) string` - Message type name (e.g., "STATE_DATA")
- `FormatPayloadMap(msgType uint8, m map[int]interface{}) string` - Payload interpretation from CBOR map

**Payload Formatting:**
- Parses CBOR map using integer keys per CDDL specification
- Extracts fields (state, error, motor data, temperatures, etc.)
- Uses `GetMapUint()`, `GetMapInt()`, `GetMapFloat()`, `GetMapBool()` helpers

#### validator.go

**Type: `AnomalyType`**
- Enum: `AnomalyInvalidCount`, `AnomalyLengthMismatch`, `AnomalyHighRPM`, `AnomalyInvalidTemp`, `AnomalyInvalidPWM`, `AnomalyInvalidValue`, `AnomalyCRCError`, `AnomalyDecodeError`

**Type: `ValidationError`**
- Fields: `Type`, `Message`, `Details` (map with context)
- Implements `error` interface

**Function: `ValidatePacket(p *Packet) []ValidationError`**
- Returns slice of validation errors (empty if valid)
- Validates based on message type using CBOR map keys

**Validation Rules:**
Validators use CBOR map helpers to extract values. See `pkg/fusain/validator.go` for current validation rules.

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
- Live telemetry display (state, uptime, motors, temperatures)
- Scrolling error log with timestamps
- Press 'q' to quit
- Note: `--stats-interval` flag is ignored in TUI mode (statistics always update live)

**Text Mode (`--tui=false`):**
- Decode errors: Red, shows CRC mismatch or framing issue
- Validation errors: Yellow/red, shows issue type and details
- Includes packet context (state, error code) for telemetry data
- Statistics printed at intervals

#### cmd/tui.go

**TUI Model Structure:**
- Built using Bubbletea framework
- Receives messages from serial goroutine
- Updates display in real-time
- Parses telemetry from CBOR payload maps

**Messages:**
- `tickMsg` - 1-second ticker for rate calculations
- `serialDataMsg` - Packet data, decode errors, validation errors
- `syncMsg` - First packet received (synchronized)

**Telemetry Parsing:**
Uses `packet.PayloadMap()` and CBOR map helpers to extract:
- STATE_DATA: state (key 2), error code (key 1)
- PING_RESPONSE: uptime (key 0)
- MOTOR_DATA: motor index (key 0), rpm (key 2), target (key 3)
- TEMP_DATA: thermometer index (key 0), reading (key 2)

**Display Components:**
- Header: Title, port info, mode, controls
- Sync status: Waiting or synchronized (with invalid byte count)
- Statistics box: Formatted with colors (errors in red, values in green)
- Telemetry box: State, uptime, motor RPMs, temperatures
- Event log: Scrolling list of recent errors/warnings with timestamps
- Auto-sizing to terminal dimensions

---

## Protocol Details

### Fusain Serial Protocol

**Wire Format:**
```
START_BYTE (0x7E)
LENGTH (1 byte) - CBOR payload length
ADDRESS (8 bytes) - device address (little-endian)
CBOR_PAYLOAD (variable) - [msg_type, payload_map]
CRC_HIGH (1 byte)
CRC_LOW (1 byte)
END_BYTE (0x7F)
```

The message type is embedded in the CBOR payload as the first element of a 2-element array.
There is no separate TYPE byte in the framing layer.

**CBOR Payload Format:**
```
[msg_type, payload_map]
```
- `msg_type`: Unsigned integer (0x00-0xFF)
- `payload_map`: CBOR map with integer keys, or `nil` for empty payloads

**Special Addresses:**
- `0x0000000000000000` - Broadcast (all devices)
- `0xFFFFFFFFFFFFFFFF` - Stateless (routers, subscriptions)

**Byte Stuffing:**
- If data byte equals `StartByte`, `EndByte`, or `EscByte`:
  - Replace with: `EscByte` + (original ^ `EscXor`)
- Decoder handles unstuffing automatically

**CRC:** CRC-16-CCITT (big-endian) over `[LENGTH, ADDRESS, CBOR_PAYLOAD]`

**References:**
- **Specification:** `origin/documentation/source/specifications/fusain/` (canonical)
- **C Implementation:** `modules/lib/fusain/`
- **Go Implementation:** `tools/heliostat/pkg/fusain/` (this package)

### Message Types

**Configuration Commands (Controller → Appliance, 0x10-0x1F):**
- `0x10` MOTOR_CONFIG - Configure motor controller
- `0x11` PUMP_CONFIG - Configure pump controller
- `0x12` TEMPERATURE_CONFIG - Configure temperature controller
- `0x13` GLOW_CONFIG - Configure glow plug
- `0x14` DATA_SUBSCRIPTION - Subscribe to appliance data
- `0x15` DATA_UNSUBSCRIBE - Unsubscribe from data
- `0x16` TELEMETRY_CONFIG - Enable/disable telemetry
- `0x17` TIMEOUT_CONFIG - Configure communication timeout
- `0x1F` DISCOVERY_REQUEST - Request device capabilities

**Control Commands (Controller → Appliance, 0x20-0x2F):**
- `0x20` STATE_COMMAND - Set system mode/state
- `0x21` MOTOR_COMMAND - Set motor RPM
- `0x22` PUMP_COMMAND - Set pump rate
- `0x23` GLOW_COMMAND - Control glow plug
- `0x24` TEMPERATURE_COMMAND - Temperature controller control
- `0x25` SEND_TELEMETRY - Request telemetry (polling mode)
- `0x2F` PING_REQUEST - Heartbeat/connectivity check

**Telemetry Data (Appliance → Controller, 0x30-0x3F):**
- `0x30` STATE_DATA - System state and status
- `0x31` MOTOR_DATA - Motor telemetry
- `0x32` PUMP_DATA - Pump status and events
- `0x33` GLOW_DATA - Glow plug status
- `0x34` TEMPERATURE_DATA - Temperature readings
- `0x35` DEVICE_ANNOUNCE - Device capabilities
- `0x3F` PING_RESPONSE - Heartbeat response

**Errors (Bidirectional, 0xE0-0xEF):**
- `0xE0` ERROR_INVALID_CMD - Command validation failed
- `0xE1` ERROR_STATE_REJECT - Command rejected by state machine

### CBOR Payload Keys (per CDDL specification)

| Message Type | Keys |
|--------------|------|
| STATE_DATA | 0=error(bool), 1=code, 2=state, 3=timestamp |
| MOTOR_DATA | 0=motor, 1=timestamp, 2=rpm, 3=target, 4=max-rpm, 5=min-rpm, 6=pwm, 7=pwm-max |
| TEMP_DATA | 0=thermometer, 1=timestamp, 2=reading, 3=temp-rpm-control, 4=watched-motor, 5=target-temp |
| PING_RESPONSE | 0=uptime |
| DEVICE_ANNOUNCE | 0=motor-count, 1=thermometer-count, 2=pump-count, 3=glow-count |
| STATE_COMMAND | 0=mode, 1=argument (optional) |
| GLOW_COMMAND | 0=glow, 1=duration |

See the Fusain protocol specification for complete payload structures:
`origin/documentation/source/specifications/fusain/`

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

**Test Coverage:** The `pkg/fusain/` package maintains comprehensive test coverage.

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
- `github.com/fxamacker/cbor/v2` - CBOR encoding/decoding

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

1. Add constant to `pkg/fusain/constants.go` (e.g., `MsgNewType = 0xNN`)
2. Add case to `FormatMessageType()` in `formatter.go`
3. Add payload formatter to `FormatPayloadMap()` in `formatter.go` using CBOR map keys
4. If validation needed, add case to `ValidatePacket()` in `validator.go`

### Adding New Validation Rule

1. Add `AnomalyType` to `validator.go`
2. Implement validation logic using `GetMapUint()`, `GetMapInt()`, etc.
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
- CBOR payload format with integer keys

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

**Protocol:** Heliostat supports the Fusain Protocol with CBOR payloads

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

**CBOR parse errors:** May indicate corrupted data or incompatible firmware

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
- `fusain` - Fusain package changes
- `cmd` - CLI command changes
- `docs` - Documentation updates
- `deps` - Dependency updates

**Example:**
```
feat(fusain): add new message type for XYZ

Add support for XYZ message type with CBOR payload.

Changes:
- Add MsgXYZ constant to constants.go
- Add formatter for XYZ payload in formatter.go
- Add validation rules in validator.go

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
```

---

## Resources

**Go Documentation:**
- Cobra: https://github.com/spf13/cobra
- Serial Port: https://pkg.go.dev/go.bug.st/serial
- CBOR: https://github.com/fxamacker/cbor

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

**Last Updated:** 2026-01-11

**Maintainer:** Kaz Walker
