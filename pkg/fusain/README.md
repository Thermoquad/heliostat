# Fusain - Go Protocol Library

Reference Go implementation of the Fusain serial protocol.

**Protocol Version:** Fusain v2.0

## Overview

This package provides a complete Go implementation of the Fusain serial communication
protocol used between Thermoquad devices. It is the canonical Go reference implementation,
complementing the C implementation in `modules/lib/fusain/`.

## Features

- **Zero Dependencies**: Uses only Go standard library
- **Standalone Module**: Can be imported independently by other Go tools
- **Complete Protocol Support**: All message types, CRC, byte stuffing
- **Validation**: Anomaly detection and packet validation
- **Statistics**: Tracking for packet rates and error counts

## Installation

```go
import "github.com/Thermoquad/heliostat/pkg/fusain"
```

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

// Formatted payload
payload := fusain.FormatPayload(packet.Type(), packet.Payload())
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
data := []byte{0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20}
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
func (p *Packet) Type() uint8
func (p *Packet) Payload() []byte
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
const START_BYTE = 0x7E
const END_BYTE = 0x7F
const ESC_BYTE = 0x7D
const ESC_XOR = 0x20

// Sizes
const MAX_PACKET_SIZE = 128
const MAX_PAYLOAD_SIZE = 114
const ADDRESS_SIZE = 8

// Special Addresses
const ADDRESS_BROADCAST = 0x0
const ADDRESS_STATELESS = 0xFFFFFFFFFFFFFFFF

// Message Types (see constants.go for full list)
const MSG_STATE_COMMAND = 0x20
const MSG_PING_REQUEST = 0x2F
const MSG_STATE_DATA = 0x30
const MSG_PING_RESPONSE = 0x3F
// ... etc
```

## Protocol Details

### Packet Format

```
[START][LENGTH][ADDRESS(8)][TYPE][PAYLOAD(0-114)][CRC(2)][END]
   │       │         │        │         │           │      │
   │       │         │        │         │           │      └─ 0x7F
   │       │         │        │         │           └─ CRC-16-CCITT (big-endian)
   │       │         │        │         └─ Variable length payload
   │       │         │        └─ Message type byte
   │       │         └─ 64-bit device address (little-endian)
   │       └─ Payload length (0-114)
   └─ 0x7E
```

### Byte Stuffing

Special bytes in data are escaped:
- `0x7E` (START) → `0x7D 0x5E`
- `0x7F` (END) → `0x7D 0x5F`
- `0x7D` (ESC) → `0x7D 0x5D`

### CRC

- Algorithm: CRC-16-CCITT
- Polynomial: 0x1021
- Initial value: 0xFFFF
- Coverage: LENGTH + ADDRESS + TYPE + PAYLOAD
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
