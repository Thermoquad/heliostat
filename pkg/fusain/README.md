# Fusain - Go Protocol Library

Reference Go implementation of the Fusain serial protocol with CBOR-encoded payloads.

## Overview

This package provides a complete Go implementation of the Fusain serial communication
protocol used between Thermoquad devices. It is the canonical Go reference implementation,
complementing the C implementation in `modules/lib/fusain/`.

## Features

- **CBOR Encoding**: Payloads use CBOR format with integer-keyed maps
- **Standalone Module**: Can be imported independently by other Go tools
- **Complete Protocol Support**: All message types, CRC, byte stuffing
- **Validation**: Anomaly detection and packet validation
- **Statistics**: Tracking for packet rates and error counts

## Installation

```go
import "github.com/Thermoquad/heliostat/pkg/fusain"
```

## Dependencies

- `github.com/fxamacker/cbor/v2` - CBOR encoding/decoding

## Usage

### Decoding Packets

```go
decoder := fusain.NewDecoder()

// Process bytes as they arrive
for _, b := range serialData {
    packet, err := decoder.DecodeByte(b)
    if err != nil {
        // Handle decode error (CRC, framing, etc.)
        decoder.Reset()
        continue
    }
    if packet != nil {
        // Complete packet received
        fmt.Printf("Type: %s\n", fusain.FormatMessageType(packet.Type()))
        fmt.Printf("Address: 0x%016X\n", packet.Address())

        // Access CBOR payload map
        payloadMap := packet.PayloadMap()
        if value, ok := fusain.GetMapUint(payloadMap, 0); ok {
            fmt.Printf("Key 0: %d\n", value)
        }
    }
}
```

### Validating Packets

```go
packet, _ := decoder.DecodeByte(b)
if packet != nil {
    errors := fusain.ValidatePacket(packet)
    for _, err := range errors {
        fmt.Printf("Validation error: %s\n", err.Message)
    }
}
```

### Formatting Output

```go
// Human-readable packet format
output := fusain.FormatPacket(packet)
fmt.Println(output)

// Message type name
typeName := fusain.FormatMessageType(packet.Type())

// Formatted payload from CBOR map
payloadStr := fusain.FormatPayloadMap(packet.Type(), packet.PayloadMap())
```

### Working with CBOR Payload Maps

```go
// Get typed values from payload map
payloadMap := packet.PayloadMap()

// Unsigned integer
if val, ok := fusain.GetMapUint(payloadMap, 0); ok {
    fmt.Printf("Key 0 (uint): %d\n", val)
}

// Signed integer
if val, ok := fusain.GetMapInt(payloadMap, 1); ok {
    fmt.Printf("Key 1 (int): %d\n", val)
}

// Float
if val, ok := fusain.GetMapFloat(payloadMap, 2); ok {
    fmt.Printf("Key 2 (float): %.2f\n", val)
}

// Boolean
if val, ok := fusain.GetMapBool(payloadMap, 3); ok {
    fmt.Printf("Key 3 (bool): %v\n", val)
}
```

### Tracking Statistics

```go
stats := fusain.NewStatistics()

// Update with each packet
stats.Update(packet, decodeErr, validationErrors)

// Get formatted summary
fmt.Println(stats.String())

// Access individual counters
fmt.Printf("Total: %d, Valid: %d\n", stats.TotalPackets, stats.ValidPackets)
```

### CRC Calculation

```go
// Calculate CRC-16-CCITT
data := []byte{0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x82, 0x2F, 0xF6}
crc := fusain.CalculateCRC(data)
```

## API Reference

### Types

#### Packet

```go
type Packet struct {
    // Fields are private, use accessor methods
}

func (p *Packet) Length() uint8
func (p *Packet) Address() uint64
func (p *Packet) Type() uint8           // Parsed from CBOR payload
func (p *Packet) Payload() []byte       // Raw CBOR bytes
func (p *Packet) PayloadMap() map[int]interface{}  // Decoded CBOR map
func (p *Packet) ParseError() error     // CBOR parse error (if any)
func (p *Packet) CRC() uint16
func (p *Packet) Timestamp() time.Time
func (p *Packet) IsBroadcast() bool
func (p *Packet) IsStateless() bool
```

#### Decoder

```go
type Decoder struct { /* ... */ }

func NewDecoder() *Decoder
func (d *Decoder) DecodeByte(b byte) (*Packet, error)
func (d *Decoder) Reset()
func (d *Decoder) GetRawBytes() []byte
```

#### CBOR Helpers

```go
func ParseCBORMessage(data []byte) (msgType uint8, payload map[int]interface{}, err error)
func GetMapUint(m map[int]interface{}, key int) (uint64, bool)
func GetMapInt(m map[int]interface{}, key int) (int64, bool)
func GetMapFloat(m map[int]interface{}, key int) (float64, bool)
func GetMapBool(m map[int]interface{}, key int) (bool, bool)
func GetMapBytes(m map[int]interface{}, key int) ([]byte, bool)
```

#### Statistics

```go
type Statistics struct {
    TotalPackets     int64
    ValidPackets     int64
    CRCErrors        int64
    DecodeErrors     int64
    MalformedPackets int64
    // ... more counters
}

func NewStatistics() *Statistics
func (s *Statistics) Update(packet *Packet, decodeErr error, validationErrors []ValidationError)
func (s *Statistics) CalculateRates()
func (s *Statistics) String() string
func (s *Statistics) Reset()
```

#### ValidationError

```go
type ValidationError struct {
    Type    AnomalyType
    Message string
    Details map[string]interface{}
}

func ValidatePacket(p *Packet) []ValidationError
```

### Constants

```go
// Framing
const StartByte = 0x7E
const EndByte = 0x7F
const EscByte = 0x7D
const EscXor = 0x20

// Sizes
const MaxPacketSize = 128
const MaxPayloadSize = 114
const AddressSize = 8

// Special Addresses
const AddressBroadcast = 0x0
const AddressStateless = 0xFFFFFFFFFFFFFFFF

// Message Types (see constants.go for full list)
const MsgStateCommand = 0x20
const MsgPingRequest = 0x2F
const MsgStateData = 0x30
const MsgPingResponse = 0x3F
// ... etc
```

## Protocol Details

### Packet Format

```
[START][LENGTH][ADDRESS(8)][CBOR_PAYLOAD][CRC(2)][END]
   │       │         │            │          │      │
   │       │         │            │          │      └─ 0x7F
   │       │         │            │          └─ CRC-16-CCITT (big-endian)
   │       │         │            └─ CBOR array: [msg_type, payload_map]
   │       │         └─ 64-bit device address (little-endian)
   │       └─ CBOR payload length (0-114)
   └─ 0x7E
```

**Note:** The message type is embedded in the CBOR payload as the first
element of a 2-element array. There is no separate TYPE byte in the framing.

### CBOR Payload Format

All payloads are encoded as a 2-element CBOR array:

```
[msg_type, payload_map]
```

- `msg_type`: Unsigned integer (0x00-0xFF)
- `payload_map`: CBOR map with integer keys, or `nil` for empty payloads

**Example - PING_REQUEST (empty payload):**
```
0x82 0x2F 0xF6  →  [47, nil]
```

**Example - STATE_DATA:**
```
0x82 0x30 0xA4 0x00 0xF4 0x01 0x00 0x02 0x01 0x03 0x1A ...
→  [48, {0: false, 1: 0, 2: 1, 3: 12345}]
```

### CBOR Map Keys (per CDDL specification)

| Message Type | Keys |
|--------------|------|
| STATE_DATA | 0=error(bool), 1=code, 2=state, 3=timestamp |
| MOTOR_DATA | 0=motor, 1=timestamp, 2=rpm, 3=target, 4=max-rpm, 5=min-rpm, 6=pwm, 7=pwm-max |
| TEMP_DATA | 0=thermometer, 1=timestamp, 2=reading, 3=temp-rpm-control, 4=watched-motor, 5=target-temp |
| PING_RESPONSE | 0=uptime |
| DEVICE_ANNOUNCE | 0=motor-count, 1=thermometer-count, 2=pump-count, 3=glow-count |

### Byte Stuffing

Special bytes in data are escaped:
- `0x7E` (START) → `0x7D 0x5E`
- `0x7F` (END) → `0x7D 0x5F`
- `0x7D` (ESC) → `0x7D 0x5D`

### CRC

- Algorithm: CRC-16-CCITT
- Polynomial: 0x1021
- Initial value: 0xFFFF
- Coverage: LENGTH + ADDRESS + CBOR_PAYLOAD
- Byte order: Big-endian in packet

## Testing

```bash
# Run tests
go test

# Run with verbose output
go test -v

# Run fuzz tests (default 1000 rounds)
FUZZ_ROUNDS=1000 go test -run TestFuzz

# Run with coverage
go test -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Protocol Specification

The canonical Fusain protocol specification is maintained in Sphinx documentation:

**Location:** `origin/documentation/source/specifications/fusain/`

## Related Implementations

- **C Library:** `modules/lib/fusain/` - Embedded C for Zephyr RTOS
- **Go Library:** This package - Reference Go implementation

## License

This package is licensed under the Apache License, Version 2.0 (Apache-2.0).

This permissive license allows use in both open source and proprietary applications,
unlike the parent Heliostat tool which is GPL-2.0-or-later.

See [LICENSE.md](LICENSE.md) for the full license text.

Copyright (c) 2025 Kaz Walker, Thermoquad
