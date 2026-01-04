// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package helios_protocol

import "time"

// Packet represents a decoded Helios protocol packet
type Packet struct {
	length    uint8
	msgType   uint8
	payload   []byte
	crc       uint16
	timestamp time.Time
}

// NewPacket creates a new packet with the given fields
func NewPacket(length uint8, msgType uint8, payload []byte, crc uint16) *Packet {
	return &Packet{
		length:    length,
		msgType:   msgType,
		payload:   payload,
		crc:       crc,
		timestamp: time.Now(),
	}
}

// Length returns the packet's payload length
func (p *Packet) Length() uint8 {
	return p.length
}

// Type returns the packet's message type
func (p *Packet) Type() uint8 {
	return p.msgType
}

// Payload returns the packet's payload bytes
func (p *Packet) Payload() []byte {
	return p.payload
}

// CRC returns the packet's CRC value
func (p *Packet) CRC() uint16 {
	return p.crc
}

// Timestamp returns the packet's decode timestamp
func (p *Packet) Timestamp() time.Time {
	return p.timestamp
}
