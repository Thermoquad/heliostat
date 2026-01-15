package fusain

import (
	"encoding/binary"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// Encoder encodes Fusain packets for transmission.
// Handles CBOR encoding, byte stuffing, and CRC calculation.
type Encoder struct{}

// NewEncoder creates a new Fusain packet encoder.
func NewEncoder() *Encoder {
	return &Encoder{}
}

// Encode encodes a Packet to wire format.
func (e *Encoder) Encode(p *Packet) ([]byte, error) {
	return EncodePacketFromValues(p.Address(), p.Type(), p.PayloadMap())
}

// EncodePacketFromValues creates a complete wire-formatted Fusain packet.
// Returns the packet bytes ready for transmission, including framing and byte stuffing.
func EncodePacketFromValues(address uint64, msgType uint8, payloadMap map[int]interface{}) ([]byte, error) {
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

// EncodePacket encodes an existing Packet struct back to wire format.
// Panics on encoding error (use Encoder.Encode for error handling).
func EncodePacket(p *Packet) []byte {
	data, err := EncodePacketFromValues(p.Address(), p.Type(), p.PayloadMap())
	if err != nil {
		panic(fmt.Sprintf("fusain: encode error: %v", err))
	}
	return data
}

// encodeCBORPayload creates the CBOR-encoded payload for a message.
func encodeCBORPayload(msgType uint8, payloadMap map[int]interface{}) ([]byte, error) {
	var msg interface{}
	if payloadMap == nil || len(payloadMap) == 0 {
		msg = []interface{}{uint64(msgType), nil}
	} else {
		msg = []interface{}{uint64(msgType), payloadMap}
	}

	data, err := cbor.Marshal(msg)
	if err != nil {
		return nil, err
	}

	return data, nil
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

// UnstuffBytes removes byte stuffing from escaped data.
// This is the inverse of stuffBytes.
func UnstuffBytes(data []byte) ([]byte, error) {
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
