# Fusain (Go) - AI Assistant Guide

> **Note:** This file documents the Go Fusain reference implementation specifically.
> Always read the [Thermoquad Organization CLAUDE.md](../../../../CLAUDE.md) first
> for organization-wide structure and conventions.

## Overview

**Fusain (Go)** is the canonical Go reference implementation of the Fusain protocol. It provides a complete, production-ready implementation for encoding, decoding, validating, and analyzing Fusain protocol packets with CBOR payloads.

**Module:** `github.com/Thermoquad/heliostat/pkg/fusain`

**Purpose:**
- Reference Go implementation for desktop tools and servers
- Protocol analyzer and validator
- Packet encoder/decoder library for external Go projects

**Protocol Specification:** `origin/documentation/source/specifications/fusain/` (Sphinx docs)

**License:** Apache-2.0 (matches C and TypeScript implementations)

---

## Key Features

- ✅ Complete Fusain protocol implementation (CBOR payloads)
- ✅ State machine decoder with byte unstuffing
- ✅ Packet encoder with byte stuffing
- ✅ Command builder functions (6 command types)
- ✅ CRC-16-CCITT validation
- ✅ Lazy CBOR parsing with caching
- ✅ Comprehensive validation and anomaly detection
- ✅ Statistics tracking (packet rates, error rates)
- ✅ Human-readable packet formatting (all message types)
- ✅ Fuzz testing for robustness
- ✅ 100% test coverage

---

## Architecture

### Standalone Go Module

The Fusain package is a **separate Go module** within the Heliostat repository, allowing it to be imported independently by other Go tools.

**Import:**
```go
import "github.com/Thermoquad/heliostat/pkg/fusain"
```

**Dependencies:**
- `github.com/fxamacker/cbor/v2` v2.9.0 - CBOR encoding/decoding
- Go 1.25.5+

### Directory Structure

```
pkg/fusain/
├── go.mod                   # Standalone module definition
├── Taskfile.dist.yml        # Task runner (test, coverage, ci)
├── CLAUDE.md                # This file
├── constants.go             # Protocol constants (message types, framing bytes)
├── packet.go                # Packet structure with lazy CBOR parsing
├── decoder.go               # State machine decoder with byte unstuffing
├── encoder.go               # Packet encoder with byte stuffing
├── commands.go              # Command builder functions (6 builders)
├── cbor.go                  # CBOR parsing helpers
├── crc.go                   # CRC-16-CCITT implementation
├── formatter.go             # Human-readable packet formatting
├── validator.go             # Validation and anomaly detection
├── statistics.go            # Statistics tracking
├── *_test.go                # Comprehensive unit tests
└── fuzz_test.go             # Fuzz testing
```

---

## API Reference

### Core Types

#### Packet

Represents a complete Fusain protocol packet with lazy CBOR parsing.

**Constructor:**
```go
func NewPacket(length uint8, address uint64, cborPayload []byte, crc uint16) *Packet
```

**Methods:**
- `Length() uint8` - CBOR payload length
- `Address() uint64` - Device address (little-endian)
- `Type() (uint8, error)` - Message type (lazy parsed from CBOR)
- `Payload() ([]byte, error)` - Raw CBOR payload
- `PayloadMap() (map[int]interface{}, error)` - Parsed CBOR payload map
- `ParseError() error` - Get CBOR parse error if any
- `CRC() uint16` - Packet CRC value
- `Timestamp() time.Time` - Packet receive timestamp
- `IsBroadcast() bool` - Check if address is broadcast (0x0)
- `IsStateless() bool` - Check if address is stateless (0xFFFFFFFFFFFFFFFF)

**Lazy Parsing:** Message type and payload map are parsed from CBOR on first access and cached for subsequent calls.

#### Decoder

State machine for decoding byte streams into Fusain packets.

**Constructor:**
```go
func NewDecoder() *Decoder
```

**Methods:**
- `DecodeByte(b byte) (*Packet, error)` - Process single byte, returns packet when complete
- `Reset()` - Reset decoder to idle state
- `GetRawBytes() []byte` - Get accumulated raw bytes (debugging)

**State Machine:**
1. `stateIdle` - Waiting for `StartByte (0x7E)`
2. `stateLength` - Read CBOR payload length
3. `stateAddress` - Read 8-byte address (little-endian)
4. `statePayload` - Read CBOR payload bytes
5. `stateCRC1` - Read CRC high byte
6. `stateCRC2` - Read CRC low byte
7. Validate CRC and return to idle on `EndByte (0x7F)`

**Byte Unstuffing:** Handles escape sequences (`EscByte 0x7D` + `EscXor 0x20`)

#### Statistics

Tracks packet statistics and error rates for monitoring.

**Constructor:**
```go
func NewStatistics() *Statistics
```

**Fields:**
- Counters: `TotalPackets`, `ValidPackets`, `CRCErrors`, `DecodeErrors`
- Malformed: `MalformedPackets`, `InvalidCounts`, `LengthMismatches`
- Anomalous: `AnomalousValues`, `HighRPM`, `InvalidTemp`, `InvalidPWM`
- Rates: `PacketRate`, `ErrorRate` (packets/sec, errors/sec)
- Timestamps: `StartTime`, `LastUpdateTime`

**Methods:**
- `Update(packet *Packet, decodeErr error, validationErrors []ValidationError)`
- `CalculateRates()` - Calculate packets/sec and errors/sec
- `String() string` - Formatted statistics summary
- `Reset()` - Reset all counters

#### ValidationError

Represents a validation error with context.

**Fields:**
- `Type AnomalyType` - Error category
- `Message string` - Human-readable message
- `Details map[string]interface{}` - Additional context

**Anomaly Types:**
- `AnomalyInvalidCount` - Device count out of range
- `AnomalyLengthMismatch` - Payload length mismatch
- `AnomalyHighRPM` - RPM above threshold
- `AnomalyInvalidTemp` - Temperature out of range
- `AnomalyInvalidPWM` - PWM duty cycle invalid
- `AnomalyInvalidValue` - Generic invalid value
- `AnomalyCRCError` - CRC validation failed
- `AnomalyDecodeError` - CBOR decode failed

---

### Encoding Functions

#### EncodePacket

Creates a complete wire-formatted Fusain packet ready for transmission.

```go
func EncodePacket(address uint64, msgType uint8, payloadMap map[int]interface{}) ([]byte, error)
```

**Process:**
1. Encode CBOR payload: `[msgType, payloadMap]`
2. Build data section: `length + address + CBOR payload`
3. Calculate CRC over data section
4. Apply byte stuffing to data + CRC
5. Add framing bytes: `StartByte + stuffed data + EndByte`

**Returns:** Complete packet bytes including framing and byte stuffing

#### MustEncodePacket

Encodes an existing Packet struct back to wire format. Panics on error.

```go
func MustEncodePacket(p *Packet) []byte
```

**Use Case:** Re-encode packets for retransmission or testing

---

### Command Builders

Convenience functions for creating common Fusain command packets.

#### NewStateCommand

```go
func NewStateCommand(address uint64, mode uint8, argument *int64) *Packet
```

**Message Type:** `MsgStateCommand (0x20)`

**Payload Fields:**
- `1: mode` (uint) - Target state (idle, fan, heat, emergency)
- `2: argument` (int, optional) - Mode-specific argument

#### NewPingRequest

```go
func NewPingRequest(address uint64) *Packet
```

**Message Type:** `MsgPingRequest (0x22)`

**Purpose:** Keepalive packet, expects `MsgPingResponse (0x33)`

#### NewTelemetryConfig

```go
func NewTelemetryConfig(address uint64, enabled bool, intervalMs uint32) *Packet
```

**Message Type:** `MsgTelemetryConfig (0x16)`

**Payload Fields:**
- `1: enabled` (bool) - Enable/disable telemetry
- `2: interval_ms` (uint) - Telemetry interval in milliseconds

#### NewMotorCommand

```go
func NewMotorCommand(address uint64, motor uint8, rpm int32) *Packet
```

**Message Type:** `MsgMotorCommand (0x21)`

**Payload Fields:**
- `1: device_index` (uint) - Motor index
- `2: target_rpm` (int) - Target RPM (negative for reverse)

#### NewPumpCommand

```go
func NewPumpCommand(address uint64, pump uint8, rateMs int32) *Packet
```

**Message Type:** `MsgPumpCommand (0x24)`

**Payload Fields:**
- `1: device_index` (uint) - Pump index
- `2: pulse_rate_ms` (int) - Pulse rate in milliseconds

#### NewGlowCommand

```go
func NewGlowCommand(address uint64, glow uint8, durationMs int32) *Packet
```

**Message Type:** `MsgGlowCommand (0x25)`

**Payload Fields:**
- `1: device_index` (uint) - Glow plug index
- `2: on_duration_ms` (int) - On duration in milliseconds

---

### CBOR Helpers

#### ParseCBORMessage

Parse CBOR payload into message type and payload map.

```go
func ParseCBORMessage(data []byte) (msgType uint8, payload map[int]interface{}, err error)
```

**Format:** CBOR array: `[msgType, payloadMap]`

**Returns:**
- `msgType` - Message type byte
- `payload` - Map with integer keys per CDDL specification
- `err` - Parse error if invalid CBOR

#### GetMap* Helpers

Type-safe helpers for extracting values from CBOR payload maps.

```go
func GetMapUint(m map[int]interface{}, key int) (uint64, bool)
func GetMapInt(m map[int]interface{}, key int) (int64, bool)
func GetMapFloat(m map[int]interface{}, key int) (float64, bool)
func GetMapBool(m map[int]interface{}, key int) (bool, bool)
func GetMapBytes(m map[int]interface{}, key int) ([]byte, bool)
```

**Returns:** `(value, exists)` - Value and presence flag

**Use Case:** Safe extraction from CBOR maps without type assertion panics

---

### Validation

#### ValidatePacket

Validates packet contents and detects anomalies.

```go
func ValidatePacket(p *Packet) []ValidationError
```

**Returns:** Slice of validation errors (empty if valid)

**Validation Rules:**
- Device count range checks
- RPM threshold validation (max: 10000)
- Temperature range checks
- PWM duty cycle validation (0-100%)
- Payload field presence checks
- Type-specific field validation

**Use Case:** Detect anomalous telemetry data for monitoring and alerting

---

### Formatting

#### FormatPacket

```go
func FormatPacket(p *Packet) string
```

**Returns:** Human-readable packet string with timestamp

**Example Output:**
```
[2026-01-15 12:34:56.789] STATE_DATA addr=0x1234567890ABCDEF
  state=HEATING error=NONE motor_rpm=2800 target_rpm=2800
  temp_0=185.5°C temp_1=42.0°C
```

#### FormatMessageType

```go
func FormatMessageType(msgType uint8) string
```

**Returns:** Message type name (e.g., "STATE_DATA", "PING_REQUEST")

#### FormatPayloadMap

```go
func FormatPayloadMap(msgType uint8, m map[int]interface{}) string
```

**Returns:** Formatted payload fields based on message type

---

### CRC

#### CalculateCRC

```go
func CalculateCRC(data []byte) uint16
```

**Algorithm:** CRC-16-CCITT
- Polynomial: 0x1021
- Initial value: 0xFFFF
- Big-endian byte order

**Use Case:** Packet validation and creation

---

### Constants

Protocol-level constants defined in `constants.go`:

**Framing:**
- `StartByte = 0x7E`
- `EndByte = 0x7F`
- `EscByte = 0x7D`
- `EscXor = 0x20`

**Size Limits:**
- `MaxPacketSize = 128`
- `MaxPayloadSize = 114`
- `AddressSize = 8`

**Special Addresses:**
- `AddressBroadcast = 0x0`
- `AddressStateless = 0xFFFFFFFFFFFFFFFF`

**Message Types:**
- Configuration: `0x10-0x1F` (MsgMotorConfig, MsgPumpConfig, MsgTempConfig, etc.)
- Commands: `0x20-0x2F` (MsgStateCommand, MsgMotorCommand, MsgPingRequest, etc.)
- Telemetry: `0x30-0x3F` (MsgStateData, MsgMotorData, MsgTempData, etc.)
- Errors: `0xE0-0xEF` (MsgErrorInvalidCmd, MsgErrorStateReject)

---

## Building & Testing

### Task Commands

Always use Taskfile commands for building and testing:

```bash
# Run all tests
task test

# Run tests with coverage (100% required)
task coverage

# CI checks (test + coverage)
task ci

# Format code
task format

# Format check
task format-check
```

### Go Commands

For external projects importing the module:

```bash
# Install as dependency
go get github.com/Thermoquad/heliostat/pkg/fusain

# Update to latest
go get -u github.com/Thermoquad/heliostat/pkg/fusain

# Run tests locally
go test -v ./...

# Run with coverage
go test -cover ./...

# Run fuzz tests
go test -fuzz=Fuzz -fuzztime=30s
```

### Coverage Requirements

**Test coverage must be 100%** for all files. Coverage is enforced in CI.

---

## Usage Examples

### Decoding Packets

```go
import "github.com/Thermoquad/heliostat/pkg/fusain"

// Create decoder
decoder := fusain.NewDecoder()

// Process byte stream
for {
    b, err := serial.ReadByte()
    if err != nil {
        break
    }

    packet, err := decoder.DecodeByte(b)
    if err != nil {
        log.Printf("Decode error: %v", err)
        continue
    }

    if packet != nil {
        // Packet complete
        msgType, _ := packet.Type()
        fmt.Printf("Received: %s\n", fusain.FormatMessageType(msgType))

        // Validate packet
        errors := fusain.ValidatePacket(packet)
        if len(errors) > 0 {
            for _, e := range errors {
                log.Printf("Validation error: %v", e.Message)
            }
        }
    }
}
```

### Encoding Commands

```go
import "github.com/Thermoquad/heliostat/pkg/fusain"

// Create command packet
packet := fusain.NewPingRequest(0x1234567890ABCDEF)

// Encode to wire format
wireBytes, err := fusain.MustEncodePacket(packet)
if err != nil {
    log.Fatalf("Encode error: %v", err)
}

// Send over serial/network
serial.Write(wireBytes)
```

### Statistics Tracking

```go
import "github.com/Thermoquad/heliostat/pkg/fusain"

stats := fusain.NewStatistics()
decoder := fusain.NewDecoder()

for {
    b, _ := serial.ReadByte()
    packet, decodeErr := decoder.DecodeByte(b)

    if packet != nil {
        validationErrors := fusain.ValidatePacket(packet)
        stats.Update(packet, decodeErr, validationErrors)

        // Print stats every 1000 packets
        if stats.TotalPackets % 1000 == 0 {
            stats.CalculateRates()
            fmt.Println(stats.String())
        }
    }
}
```

---

## Relationship to Other Implementations

### Fusain Protocol Specification

**Location:** `origin/documentation/source/specifications/fusain/`

**Relationship:** This Go implementation follows the canonical specification. All three implementations (C, Go, TypeScript) must match the specification.

### Fusain C Library

**Location:** `modules/lib/fusain/`

**Relationship:** Embedded C implementation for Zephyr RTOS (Helios, Slate firmware).

**Shared Concepts:**
- Protocol constants (message types, framing bytes)
- CRC-16-CCITT algorithm
- Byte stuffing/unstuffing
- CBOR payload format with integer keys

**Differences:**
- C: Embedded systems, memory-constrained, uses zcbor
- Go: Desktop/server tools, reference implementation, uses fxamacker/cbor

### Fusain TypeScript Library

**Location:** `apps/roastee/packages/fusain/`

**Relationship:** Reference TypeScript implementation for web/Node.js.

**Purpose:** Enables browser-based and Node.js tools to communicate with Thermoquad devices.

**Status:** Not yet published to npm

---

## Development Conventions

### Code Style

- Follow Go standard formatting (`gofmt`, `go vet`)
- Use task `format` before committing
- 100% test coverage required
- Fuzz testing for robustness

### Naming

- `PascalCase` for exported types and functions
- `camelCase` for unexported types and functions
- `UPPER_SNAKE_CASE` for constants

### Testing

- Unit tests in `*_test.go` files
- Fuzz tests in `fuzz_test.go`
- Coverage enforced in CI
- Test all error paths and edge cases

---

## Git Workflow

Follow the organization git workflow in `../../../../CLAUDE.md`.

**Commit Scopes:**
- `fusain` - Go Fusain package changes
- `heliostat` - Heliostat tool changes (if affecting both)

**Important:** Changes to `pkg/fusain/` should be reviewed carefully as this is the reference Go implementation used by multiple tools.

---

## AI Assistant Operations

### Content Integrity Check

To verify consistency across all CLAUDE.md files in the organization, see the **Content Integrity Check** section in the [Thermoquad Organization CLAUDE.md](../../../../CLAUDE.md).

**How to Request:** Ask the AI assistant to "run a content integrity check on all CLAUDE.md files"

### Content Status Integrity Check

To validate that this CLAUDE.md accurately reflects the actual Go Fusain implementation, see the **Content Status Integrity Check** section in the [Thermoquad Organization CLAUDE.md](../../../../CLAUDE.md).

**How to Request:** Ask the AI assistant to "run a content status integrity check on go fusain"

**What Gets Checked for Go Fusain:**
- File count matches documentation (10 .go files)
- Command builder count matches documentation (6 builders)
- All documented functions exist in source
- Message type constants match Fusain specification
- Test coverage actually meets 100% requirement
- Build succeeds with `go build`
- go.mod dependencies match documented versions
- Taskfile tasks exist as documented

### CLAUDE.md Reload

To reload all organization CLAUDE.md files, see the **CLAUDE.md Reload** section in the [Thermoquad Organization CLAUDE.md](../../../../CLAUDE.md).

---

## Resources

### Protocol Specification

- **Fusain Specification:** https://thermoquad.github.io/origin/specifications/fusain/
- **CDDL Schema:** `origin/documentation/source/specifications/fusain/fusain.cddl`

### Go Resources

- **fxamacker/cbor:** https://github.com/fxamacker/cbor
- **CBOR RFC 8949:** https://datatracker.ietf.org/doc/html/rfc8949
- **CRC Tutorial:** https://users.ece.cmu.edu/~koopman/crc/

---

**Last Updated:** 2026-01-15

**Maintainer:** Kaz Walker
