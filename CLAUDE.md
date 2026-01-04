# Heliostat - AI Assistant Guide

## Project Overview

Heliostat is a Go CLI tool for monitoring and analyzing Helios serial protocol packets in real-time. It provides both raw packet logging and advanced error detection capabilities.

**Purpose:** Diagnose communication issues, validate protocol implementation, and detect anomalous telemetry data from Helios burner ICU.

**Key Features:**
- Decode and display Helios protocol packets
- Validate packet structure and detect anomalies
- Track statistics (packet rates, error rates, success rates)
- Reusable protocol package for other Go tools

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
├── go.mod                           # Go module definition
├── go.sum                           # Dependency checksums
├── main.go                          # Cobra CLI entrypoint
├── README.md                        # User documentation
├── CLAUDE.md                        # This file
├── cmd/                             # Cobra command definitions
│   ├── root.go                      # Root command and global flags
│   ├── raw_log.go                   # Raw log command
│   ├── error_detection.go           # Error detection command
│   └── tui.go                       # Bubbletea TUI model
└── pkg/
    └── helios_protocol/             # Reusable protocol package
        ├── constants.go             # Protocol constants
        ├── packet.go                # Packet structure
        ├── decoder.go               # State machine decoder
        ├── crc.go                   # CRC-16-CCITT
        ├── formatter.go             # Packet formatting
        ├── validator.go             # Validation and anomaly detection
        └── statistics.go            # Statistics tracking
```

---

## Code Structure

### Package: `helios_protocol`

Reusable Go package for Helios protocol handling. Can be imported by other tools.

**Import:**
```go
import "github.com/Thermoquad/heliostat/pkg/helios_protocol"
```

#### constants.go

Protocol-level constants:
- Framing bytes: `START_BYTE (0x7E)`, `END_BYTE (0x7F)`, `ESC_BYTE (0x7D)`, `ESC_XOR (0x20)`
- Size limits: `MAX_PACKET_SIZE (128)`, `MAX_PAYLOAD_SIZE (122)`
- CRC parameters: `CRC_POLYNOMIAL (0x1021)`, `CRC_INITIAL (0xFFFF)`
- Message types: Commands (0x10-0x16), Data (0x20-0x26), Errors (0xE0-0xE3)
- Decoder states: `STATE_IDLE`, `STATE_LENGTH`, `STATE_TYPE`, `STATE_PAYLOAD`, `STATE_CRC1`, `STATE_CRC2`

#### packet.go

**Type: `Packet`**
- Fields: `length`, `msgType`, `payload`, `crc`, `timestamp`
- Methods: `Length()`, `Type()`, `Payload()`, `CRC()`, `Timestamp()`
- Constructor: `NewPacket(length, msgType, payload, crc) *Packet`

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
3. `STATE_TYPE` - Read message type
4. `STATE_PAYLOAD` - Read payload bytes
5. `STATE_CRC1` - Read CRC high byte
6. `STATE_CRC2` - Read CRC low byte
7. Return to idle on `END_BYTE` after validating CRC

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

TELEMETRY_BUNDLE:
- `motor_count <= 10` (Slate firmware limit)
- `temp_count <= 10`
- Payload length matches: `7 + (motor_count * 16) + (temp_count * 8)`
- Motor RPM <= 6000
- Target RPM <= 6000
- PWM duty <= PWM period
- Temperature: -50°C to 1000°C

MOTOR_DATA:
- RPM <= 6000
- Target RPM <= 6000
- PWM <= pwmMax

TEMPERATURE_DATA:
- Current temp: -50°C to 1000°C
- Target temp: -50°C to 1000°C

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

### Helios Serial Protocol

**Framing:**
```
START_BYTE (0x7E)
LENGTH (1 byte) - payload length
TYPE (1 byte) - message type
PAYLOAD (0-122 bytes)
CRC_HIGH (1 byte)
CRC_LOW (1 byte)
END_BYTE (0x7F)
```

**Byte Stuffing:**
- If data byte equals `START_BYTE`, `END_BYTE`, or `ESC_BYTE`:
  - Replace with: `ESC_BYTE` + (original ^ `ESC_XOR`)
- Decoder handles unstuffing automatically

**CRC:** CRC-16-CCITT over `[LENGTH, TYPE, PAYLOAD]`

**Reference:** See `modules/lib/fusain/` for C implementation

### Message Types

**Commands (Master → ICU):**
- `0x10` SET_MODE - Change operating mode
- `0x11` SET_PUMP_RATE - Set pump rate (ms)
- `0x12` SET_TARGET_RPM - Set motor target RPM
- `0x13` PING_REQUEST - Keepalive
- `0x14` SET_TIMEOUT_CONFIG - Configure timeouts
- `0x15` EMERGENCY_STOP - Emergency shutdown
- `0x16` TELEMETRY_CONFIG - Enable/configure telemetry

**Data (ICU → Master):**
- `0x20` STATE_DATA - Current state and error
- `0x21` MOTOR_DATA - Motor telemetry (individual)
- `0x22` TEMPERATURE_DATA - Temperature telemetry (individual)
- `0x23` PUMP_DATA - Pump telemetry
- `0x24` GLOW_DATA - Glow plug telemetry
- `0x25` TELEMETRY_BUNDLE - Combined telemetry (v1.2+)
- `0x26` PING_RESPONSE - Ping acknowledgment

**Errors:**
- `0xE0` ERROR_INVALID_COMMAND - Unknown command
- `0xE1` ERROR_INVALID_CRC - CRC validation failed
- `0xE2` ERROR_INVALID_LENGTH - Payload length mismatch
- `0xE3` ERROR_TIMEOUT - Communication timeout

### Telemetry Bundle Structure (0x25)

```
Offset  Size  Field
0-3     4     state (u32, little-endian)
4       1     error (u8)
5       1     motor_count (u8)
6       1     temp_count (u8)
7+      16*N  motor data (N = motor_count)
        8*M   temperature data (M = temp_count)
```

**Motor Data (16 bytes):**
```
0-3     rpm (i32)
4-7     target_rpm (i32)
8-11    pwm_duty (i32, nanoseconds)
12-15   pwm_period (i32, nanoseconds)
```

**Temperature Data (8 bytes):**
```
0-7     temperature (f64, IEEE 754, little-endian)
```

---

## Development

### Building

```bash
go build
```

Produces `heliostat` binary in current directory.

### Testing

```bash
go test ./...
```

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
3. Use `helios_protocol` package for decoding/formatting
4. Rebuild and test

---

## Common Tasks for AI Assistants

### Adding New Message Type

1. Add constant to `pkg/helios_protocol/constants.go`
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

### Fusain Library (C)

**Location:** `modules/lib/fusain/`

**Relationship:** Heliostat implements the same protocol as Fusain in Go.

**Shared Concepts:**
- Protocol constants (message types, framing bytes)
- CRC-16-CCITT algorithm
- Byte stuffing/unstuffing
- State machine decoder

**Differences:**
- Fusain is for embedded C (Zephyr RTOS)
- Heliostat is for desktop analysis (Go)

### Slate Firmware (C)

**Location:** `apps/slate/`

**Relationship:** Slate uses Fusain to communicate with Helios. Heliostat helps debug that communication.

**Validation Alignment:**
- Heliostat's `motor_count <= 10` rule matches Slate's validation
- Heliostat's `temp_count <= 10` rule matches Slate's validation
- Heliostat's RPM > 6000 warning matches Slate's filtering

### Helios Firmware (C)

**Location:** `apps/helios/`

**Relationship:** Helios is the ICU that sends telemetry. Heliostat decodes and validates that telemetry.

**Protocol Version:** Heliostat supports protocol v1.3 (same as Helios)

---

## Troubleshooting

### Build Errors

**Import cycle:** Ensure no circular imports between package files

**Undefined reference:** Check package name matches directory name (`helios_protocol`)

**Version conflicts:** Run `go mod tidy` to resolve dependencies

### Runtime Issues

**Serial port permission denied:** Add user to `dialout` group or use `sudo`

**No packets detected:** Verify baud rate matches Helios configuration (default 115200)

**CRC errors:** May indicate communication issue (loose connection, electrical noise)

**Malformed packets:** May indicate Helios firmware bug (use error_detection to diagnose)

---

## Git Workflow

**IMPORTANT:** This tool is part of the Thermoquad organization. Follow the git workflow documented in `/home/kazw/Projects/Thermoquad/CLAUDE.md`.

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
- Fusain library: `modules/lib/fusain/CLAUDE.md`
- Helios firmware: `apps/helios/CLAUDE.md`
- Slate firmware: `apps/slate/CLAUDE.md`

**Organization:**
- Thermoquad CLAUDE.md: `/home/kazw/Projects/Thermoquad/CLAUDE.md`

---

**Last Updated:** 2026-01-04

**Maintainer:** Kaz Walker
