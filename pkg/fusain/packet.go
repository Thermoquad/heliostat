// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package fusain

import "time"

// Packet represents a decoded Fusain protocol packet
type Packet struct {
	length    uint8
	address   uint64
	msgType   uint8
	payload   []byte
	crc       uint16
	timestamp time.Time
}

// NewPacket creates a new packet with the given fields
func NewPacket(length uint8, address uint64, msgType uint8, payload []byte, crc uint16) *Packet {
	return &Packet{
		length:    length,
		address:   address,
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

// Address returns the packet's 64-bit device address
func (p *Packet) Address() uint64 {
	return p.address
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

// IsBroadcast returns true if the packet is addressed to all devices
func (p *Packet) IsBroadcast() bool {
	return p.address == AddressBroadcast
}

// IsStateless returns true if the packet uses the stateless address
func (p *Packet) IsStateless() bool {
	return p.address == AddressStateless
}
