// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package fusain

import (
	"fmt"
	"time"
)

// Decoder implements the Fusain protocol packet decoder state machine
type Decoder struct {
	state        int
	buffer       []byte
	bufferIndex  int
	escapeNext   bool
	addressBytes int // Counter for address bytes (0-7)
	packet       *Packet
	rawBuffer    []byte // Accumulate raw bytes including framing
}

// NewDecoder creates a new protocol decoder
func NewDecoder() *Decoder {
	return &Decoder{
		state:     stateIdle,
		buffer:    make([]byte, MaxPacketSize),
		rawBuffer: make([]byte, 0, MaxPacketSize*2),
	}
}

// Reset resets the decoder state to idle
func (d *Decoder) Reset() {
	d.state = stateIdle
	d.bufferIndex = 0
	d.addressBytes = 0
	d.escapeNext = false
	d.packet = nil
	d.rawBuffer = d.rawBuffer[:0]
}

// GetRawBytes returns the accumulated raw bytes since the last packet
func (d *Decoder) GetRawBytes() []byte {
	return d.rawBuffer
}

// DecodeByte processes a single byte through the decoder state machine
// Returns a completed packet, or nil if the packet is incomplete
// Returns an error if decoding fails
func (d *Decoder) DecodeByte(b byte) (*Packet, error) {
	// Always accumulate raw bytes for verification
	d.rawBuffer = append(d.rawBuffer, b)

	// Handle byte stuffing
	if b == EscByte && !d.escapeNext {
		d.escapeNext = true
		return nil, nil
	}

	originalB := b
	if d.escapeNext {
		b ^= EscXor
		d.escapeNext = false
	}

	// Handle framing bytes
	if originalB == StartByte && !d.escapeNext {
		d.Reset()
		d.rawBuffer = append(d.rawBuffer[:0], originalB)
		d.state = stateLength
		return nil, nil
	}

	if originalB == EndByte && !d.escapeNext {
		if d.state == stateCRC2 {
			// Packet complete - validate CRC
			packet := d.packet
			calculatedCRC := CalculateCRC(d.buffer[:d.bufferIndex])

			if packet.crc != calculatedCRC {
				err := fmt.Errorf("CRC mismatch: expected 0x%04X, got 0x%04X", calculatedCRC, packet.crc)
				d.Reset()
				return nil, err
			}

			packet.timestamp = time.Now()

			d.Reset()
			return packet, nil
		}
		d.Reset()
		return nil, fmt.Errorf("unexpected END byte in state %d", d.state)
	}

	// State machine
	switch d.state {
	case stateIdle:
		// Waiting for START byte
		return nil, nil

	case stateLength:
		if b > MaxPayloadSize {
			d.Reset()
			return nil, fmt.Errorf("invalid length: %d (max %d)", b, MaxPayloadSize)
		}
		// Check for buffer overflow
		if d.bufferIndex >= MaxPacketSize {
			d.Reset()
			return nil, fmt.Errorf("buffer overflow at length byte")
		}
		d.packet = &Packet{length: b, payload: make([]byte, 0, b)}
		d.buffer[d.bufferIndex] = b
		d.bufferIndex++
		d.addressBytes = 0
		d.state = stateAddress
		return nil, nil

	case stateAddress:
		// Check for buffer overflow
		if d.bufferIndex >= MaxPacketSize {
			d.Reset()
			return nil, fmt.Errorf("buffer overflow at address byte")
		}
		// Accumulate address bytes (little-endian)
		d.packet.address |= uint64(b) << (d.addressBytes * 8)
		d.buffer[d.bufferIndex] = b
		d.bufferIndex++
		d.addressBytes++
		if d.addressBytes >= AddressSize {
			d.state = stateType
		}
		return nil, nil

	case stateType:
		// Check for buffer overflow
		if d.bufferIndex >= MaxPacketSize {
			d.Reset()
			return nil, fmt.Errorf("buffer overflow at type byte")
		}
		d.packet.msgType = b
		d.buffer[d.bufferIndex] = b
		d.bufferIndex++
		if d.packet.length == 0 {
			d.state = stateCRC1
		} else {
			d.state = statePayload
		}
		return nil, nil

	case statePayload:
		// Check for buffer overflow before accepting byte
		if d.bufferIndex >= MaxPacketSize {
			d.Reset()
			return nil, fmt.Errorf("buffer overflow: packet exceeds max size")
		}
		d.packet.payload = append(d.packet.payload, b)
		d.buffer[d.bufferIndex] = b
		d.bufferIndex++
		if len(d.packet.payload) >= int(d.packet.length) {
			d.state = stateCRC1
		}
		return nil, nil

	case stateCRC1:
		d.packet.crc = uint16(b) << 8
		d.state = stateCRC2
		return nil, nil

	case stateCRC2:
		d.packet.crc |= uint16(b)
		// Wait for END byte
		return nil, nil

	default:
		d.Reset()
		return nil, fmt.Errorf("invalid state: %d", d.state)
	}
}
