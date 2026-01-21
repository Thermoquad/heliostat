package fusain

import (
	"encoding/binary"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// EncodePacket creates a complete wire-formatted Fusain packet.
// Returns the packet bytes ready for transmission, including framing and byte stuffing.
func EncodePacket(address uint64, msgType uint8, payloadMap map[int]interface{}) ([]byte, error) {
	// Build CBOR payload: [msgType, payloadMap]
	cborPayload, err := encodeCBORPayload(msgType, payloadMap)
	if err != nil {
		return nil, fmt.Errorf("failed to encode CBOR payload: %w", err)
	}

	if len(cborPayload) > MaxPayloadSize {
		return nil, fmt.Errorf("CBOR payload too large: %d bytes (max %d)", len(cborPayload), MaxPayloadSize)
	}

	// Build the data section: length + address + CBOR payload
	// This is what gets CRC'd and byte-stuffed
	dataLen := 1 + AddressSize + len(cborPayload) // length byte + 8 address bytes + payload
	data := make([]byte, dataLen)

	data[0] = uint8(len(cborPayload))
	binary.LittleEndian.PutUint64(data[1:9], address)
	copy(data[9:], cborPayload)

	// Calculate CRC over the data section
	crc := CalculateCRC(data)

	// Append CRC (big-endian)
	data = append(data, byte(crc>>8), byte(crc&0xFF))

	// Apply byte stuffing to the data section (not framing bytes)
	stuffed := stuffBytes(data)

	// Build final packet with framing
	packet := make([]byte, 0, len(stuffed)+2)
	packet = append(packet, StartByte)
	packet = append(packet, stuffed...)
	packet = append(packet, EndByte)

	return packet, nil
}

// MustEncodePacket encodes an existing Packet struct back to wire format.
// Panics on encoding error (use EncodePacket for error handling).
//
// WARNING: Do not use with untrusted packet data. An attacker could craft
// packets that decode successfully but fail to re-encode (e.g., oversized
// payload), causing a panic and application crash. Use EncodePacket() for
// untrusted data as it returns errors instead.
func MustEncodePacket(p *Packet) []byte {
	data, err := EncodePacket(p.Address(), p.Type(), p.PayloadMap())
	if err != nil {
		panic(fmt.Sprintf("fusain: encode error: %v", err))
	}
	return data
}

// encodeCBORPayload creates the CBOR-encoded payload for a message.
// Uses explicit 2-byte encoding for message type (0x18 prefix) to ensure
// consistent wire format across implementations.
func encodeCBORPayload(msgType uint8, payloadMap map[int]interface{}) ([]byte, error) {
	// Manually build CBOR to ensure consistent encoding:
	// - Array of 2 elements: 0x82
	// - Message type with 1-byte uint encoding: 0x18 <msgType>
	// - Payload map (or null): marshaled by cbor library

	var result []byte

	// Array header (2 elements)
	result = append(result, 0x82)

	// Message type with explicit 1-byte uint encoding (0x18 prefix)
	// This ensures consistent wire format regardless of msgType value
	result = append(result, 0x18, msgType)

	// Encode payload map (or null)
	var payloadBytes []byte
	var err error
	if payloadMap == nil || len(payloadMap) == 0 {
		payloadBytes, err = cbor.Marshal(nil)
	} else {
		payloadBytes, err = cbor.Marshal(payloadMap)
	}
	if err != nil {
		return nil, err
	}

	result = append(result, payloadBytes...)
	return result, nil
}

// stuffBytes applies byte stuffing to escape special bytes.
// Special bytes (START, END, ESC) are replaced with ESC + (byte XOR EscXor).
func stuffBytes(data []byte) []byte {
	// Pre-allocate with extra space for potential escapes
	result := make([]byte, 0, len(data)*2)

	for _, b := range data {
		if b == StartByte || b == EndByte || b == EscByte {
			result = append(result, EscByte, b^EscXor)
		} else {
			result = append(result, b)
		}
	}

	return result
}

// unstuffBytes removes byte stuffing from escaped data.
// This is the inverse of stuffBytes.
func unstuffBytes(data []byte) ([]byte, error) {
	result := make([]byte, 0, len(data))
	escapeNext := false

	for _, b := range data {
		if escapeNext {
			result = append(result, b^EscXor)
			escapeNext = false
		} else if b == EscByte {
			escapeNext = true
		} else {
			result = append(result, b)
		}
	}

	if escapeNext {
		return nil, fmt.Errorf("incomplete escape sequence at end of data")
	}

	return result, nil
}
