// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package fusain

import "time"

// Packet represents a decoded Fusain protocol packet
type Packet struct {
	length      uint8
	address     uint64
	cborPayload []byte // Raw CBOR bytes: [msg_type, payload_map]
	crc         uint16
	timestamp   time.Time

	// Cached parsed values (lazy parsing)
	msgType    uint8
	payloadMap map[int]interface{}
	parsed     bool
	parseErr   error
}

// NewPacket creates a new packet with the given fields
func NewPacket(length uint8, address uint64, cborPayload []byte, crc uint16) *Packet {
	return &Packet{
		length:      length,
		address:     address,
		cborPayload: cborPayload,
		crc:         crc,
		timestamp:   time.Now(),
	}
}

// NewPacketWithPayload creates a new packet from message type and payload map.
// The CBOR encoding and CRC are computed automatically.
func NewPacketWithPayload(address uint64, msgType uint8, payload map[int]interface{}) *Packet {
	return &Packet{
		address:    address,
		msgType:    msgType,
		payloadMap: payload,
		parsed:     true,
		timestamp:  time.Now(),
	}
}

// ensureParsed parses the CBOR payload if not already done
func (p *Packet) ensureParsed() {
	if p.parsed {
		return
	}
	p.parsed = true
	if len(p.cborPayload) == 0 {
		return
	}
	p.msgType, p.payloadMap, p.parseErr = ParseCBORMessage(p.cborPayload)
}

// Length returns the packet's CBOR payload length
func (p *Packet) Length() uint8 {
	return p.length
}

// Address returns the packet's 64-bit device address
func (p *Packet) Address() uint64 {
	return p.address
}

// Type returns the packet's message type (parsed from CBOR)
func (p *Packet) Type() uint8 {
	p.ensureParsed()
	return p.msgType
}

// Payload returns the raw CBOR payload bytes
func (p *Packet) Payload() []byte {
	return p.cborPayload
}

// PayloadMap returns the decoded CBOR payload map (nil for empty payloads)
func (p *Packet) PayloadMap() map[int]interface{} {
	p.ensureParsed()
	return p.payloadMap
}

// ParseError returns any error from parsing the CBOR payload
func (p *Packet) ParseError() error {
	p.ensureParsed()
	return p.parseErr
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
