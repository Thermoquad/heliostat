// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package helios_protocol

import (
	"fmt"
	"time"
)

// Decoder implements the Helios protocol packet decoder state machine
type Decoder struct {
	state       int
	buffer      []byte
	bufferIndex int
	escapeNext  bool
	packet      *Packet
	rawBuffer   []byte // Accumulate raw bytes including framing
}

// NewDecoder creates a new protocol decoder
func NewDecoder() *Decoder {
	return &Decoder{
		state:     STATE_IDLE,
		buffer:    make([]byte, MAX_PACKET_SIZE),
		rawBuffer: make([]byte, 0, MAX_PACKET_SIZE*2),
	}
}

// Reset resets the decoder state to idle
func (d *Decoder) Reset() {
	d.state = STATE_IDLE
	d.bufferIndex = 0
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
	if b == ESC_BYTE && !d.escapeNext {
		d.escapeNext = true
		return nil, nil
	}

	originalB := b
	if d.escapeNext {
		b ^= ESC_XOR
		d.escapeNext = false
	}

	// Handle framing bytes
	if originalB == START_BYTE && !d.escapeNext {
		d.Reset()
		d.rawBuffer = append(d.rawBuffer[:0], originalB)
		d.state = STATE_LENGTH
		return nil, nil
	}

	if originalB == END_BYTE && !d.escapeNext {
		if d.state == STATE_CRC2 {
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
	case STATE_IDLE:
		// Waiting for START byte
		return nil, nil

	case STATE_LENGTH:
		if b > MAX_PAYLOAD_SIZE {
			d.Reset()
			return nil, fmt.Errorf("invalid length: %d", b)
		}
		// Protocol v1.3: Check for buffer overflow
		if d.bufferIndex >= MAX_PACKET_SIZE {
			d.Reset()
			return nil, fmt.Errorf("buffer overflow at length byte")
		}
		d.packet = &Packet{length: b, payload: make([]byte, 0, b)}
		d.buffer[d.bufferIndex] = b
		d.bufferIndex++
		d.state = STATE_TYPE
		return nil, nil

	case STATE_TYPE:
		// Protocol v1.3: Check for buffer overflow
		if d.bufferIndex >= MAX_PACKET_SIZE {
			d.Reset()
			return nil, fmt.Errorf("buffer overflow at type byte")
		}
		d.packet.msgType = b
		d.buffer[d.bufferIndex] = b
		d.bufferIndex++
		if d.packet.length == 0 {
			d.state = STATE_CRC1
		} else {
			d.state = STATE_PAYLOAD
		}
		return nil, nil

	case STATE_PAYLOAD:
		// Protocol v1.3: Check for buffer overflow before accepting byte
		if d.bufferIndex >= MAX_PACKET_SIZE {
			d.Reset()
			return nil, fmt.Errorf("buffer overflow: packet exceeds max size")
		}
		d.packet.payload = append(d.packet.payload, b)
		d.buffer[d.bufferIndex] = b
		d.bufferIndex++
		if len(d.packet.payload) >= int(d.packet.length) {
			d.state = STATE_CRC1
		}
		return nil, nil

	case STATE_CRC1:
		d.packet.crc = uint16(b) << 8
		d.state = STATE_CRC2
		return nil, nil

	case STATE_CRC2:
		d.packet.crc |= uint16(b)
		// Wait for END byte
		return nil, nil

	default:
		d.Reset()
		return nil, fmt.Errorf("invalid state: %d", d.state)
	}
}
