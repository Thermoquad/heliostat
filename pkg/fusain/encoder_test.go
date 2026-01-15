package fusain

import (
	"bytes"
	"math"
	"testing"
)

// payloadValuesEqual compares payload values accounting for CBOR type coercion.
// CBOR may decode uint64 as int64 or vice versa, and floats need epsilon comparison.
func payloadValuesEqual(expected, actual interface{}) bool {
	switch e := expected.(type) {
	case uint64:
		switch a := actual.(type) {
		case uint64:
			return e == a
		case int64:
			return a >= 0 && uint64(a) == e
		}
	case int64:
		switch a := actual.(type) {
		case int64:
			return e == a
		case uint64:
			return e >= 0 && uint64(e) == a
		}
	case float64:
		switch a := actual.(type) {
		case float64:
			return math.Abs(e-a) < 0.0001
		}
	case bool:
		if a, ok := actual.(bool); ok {
			return e == a
		}
	}
	return false
}

func TestEncodePacket_RoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		address    uint64
		msgType    uint8
		payloadMap map[int]interface{}
	}{
		{
			name:       "ping request with no payload",
			address:    0x0102030405060708,
			msgType:    MsgPingRequest,
			payloadMap: nil,
		},
		{
			name:    "state data",
			address: 0x1122334455667788,
			msgType: MsgStateData,
			payloadMap: map[int]interface{}{
				0: false,         // error
				1: uint64(0),     // code
				2: uint64(1),     // state (IDLE)
				3: uint64(12345), // timestamp
			},
		},
		{
			name:    "motor data",
			address: AddressBroadcast,
			msgType: MsgMotorData,
			payloadMap: map[int]interface{}{
				0: uint64(0),    // motor index
				1: uint64(1000), // timestamp
				2: uint64(2500), // rpm
				3: uint64(3000), // target
			},
		},
		{
			name:    "state command",
			address: 0xAABBCCDDEEFF0011,
			msgType: MsgStateCommand,
			payloadMap: map[int]interface{}{
				0: uint64(2), // mode (HEAT)
			},
		},
		{
			name:    "temp data with float",
			address: AddressStateless,
			msgType: MsgTempData,
			payloadMap: map[int]interface{}{
				0: uint64(0),  // thermometer index
				1: uint64(50), // timestamp
				2: 125.5,      // temperature reading
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode the packet using EncodePacket
			encoded, err := EncodePacket(tt.address, tt.msgType, tt.payloadMap)
			if err != nil {
				t.Fatalf("EncodePacket failed: %v", err)
			}

			// Verify framing
			if encoded[0] != StartByte {
				t.Errorf("packet should start with StartByte (0x%02X), got 0x%02X", StartByte, encoded[0])
			}
			if encoded[len(encoded)-1] != EndByte {
				t.Errorf("packet should end with EndByte (0x%02X), got 0x%02X", EndByte, encoded[len(encoded)-1])
			}

			// Decode the packet
			decoder := NewDecoder()
			var decoded *Packet
			for _, b := range encoded {
				p, err := decoder.DecodeByte(b)
				if err != nil {
					t.Fatalf("Decoder error: %v", err)
				}
				if p != nil {
					decoded = p
				}
			}

			if decoded == nil {
				t.Fatal("Decoder did not produce a packet")
			}

			// Verify decoded values match original
			if decoded.Address() != tt.address {
				t.Errorf("address mismatch: got 0x%016X, want 0x%016X", decoded.Address(), tt.address)
			}
			if decoded.Type() != tt.msgType {
				t.Errorf("msgType mismatch: got 0x%02X, want 0x%02X", decoded.Type(), tt.msgType)
			}

			// Verify payload values survived round-trip
			if tt.payloadMap != nil {
				decodedPayload := decoded.PayloadMap()
				if decodedPayload == nil {
					t.Error("expected payload map, got nil")
				} else {
					for key, expectedValue := range tt.payloadMap {
						actualValue, ok := decodedPayload[key]
						if !ok {
							t.Errorf("missing payload key %d", key)
							continue
						}
						if !payloadValuesEqual(expectedValue, actualValue) {
							t.Errorf("payload[%d] mismatch: got %v (%T), want %v (%T)",
								key, actualValue, actualValue, expectedValue, expectedValue)
						}
					}
				}
			} else {
				// Nil payload should decode as nil or empty map
				decodedPayload := decoded.PayloadMap()
				if decodedPayload != nil && len(decodedPayload) > 0 {
					t.Errorf("expected nil payload, got %v", decodedPayload)
				}
			}
		})
	}
}

func TestStuffBytes(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		expect []byte
	}{
		{
			name:   "no special bytes",
			input:  []byte{0x01, 0x02, 0x03},
			expect: []byte{0x01, 0x02, 0x03},
		},
		{
			name:   "escape start byte",
			input:  []byte{0x01, StartByte, 0x03},
			expect: []byte{0x01, EscByte, StartByte ^ EscXor, 0x03},
		},
		{
			name:   "escape end byte",
			input:  []byte{0x01, EndByte, 0x03},
			expect: []byte{0x01, EscByte, EndByte ^ EscXor, 0x03},
		},
		{
			name:   "escape escape byte",
			input:  []byte{0x01, EscByte, 0x03},
			expect: []byte{0x01, EscByte, EscByte ^ EscXor, 0x03},
		},
		{
			name:   "multiple special bytes",
			input:  []byte{StartByte, EndByte, EscByte},
			expect: []byte{EscByte, StartByte ^ EscXor, EscByte, EndByte ^ EscXor, EscByte, EscByte ^ EscXor},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stuffBytes(tt.input)
			if !bytes.Equal(result, tt.expect) {
				t.Errorf("stuffBytes(%v) = %v, want %v", tt.input, result, tt.expect)
			}
		})
	}
}

func Test_unstuffBytes(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		expect []byte
	}{
		{
			name:   "no escapes",
			input:  []byte{0x01, 0x02, 0x03},
			expect: []byte{0x01, 0x02, 0x03},
		},
		{
			name:   "unescape start byte",
			input:  []byte{0x01, EscByte, StartByte ^ EscXor, 0x03},
			expect: []byte{0x01, StartByte, 0x03},
		},
		{
			name:   "unescape end byte",
			input:  []byte{0x01, EscByte, EndByte ^ EscXor, 0x03},
			expect: []byte{0x01, EndByte, 0x03},
		},
		{
			name:   "unescape escape byte",
			input:  []byte{0x01, EscByte, EscByte ^ EscXor, 0x03},
			expect: []byte{0x01, EscByte, 0x03},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := unstuffBytes(tt.input)
			if err != nil {
				t.Fatalf("unstuffBytes error: %v", err)
			}
			if !bytes.Equal(result, tt.expect) {
				t.Errorf("unstuffBytes(%v) = %v, want %v", tt.input, result, tt.expect)
			}
		})
	}
}

func Test_unstuffBytes_IncompleteEscape(t *testing.T) {
	// Test error path: escape byte at end of data with no following byte
	input := []byte{0x01, 0x02, EscByte}

	_, err := unstuffBytes(input)
	if err == nil {
		t.Error("expected error for incomplete escape sequence, got nil")
	}
}

func TestStuffUnstuffRoundTrip(t *testing.T) {
	// Test with various byte patterns including special bytes
	inputs := [][]byte{
		{0x00, 0x01, 0x02},
		{StartByte, EndByte, EscByte},
		{0x7E, 0x7D, 0x7F, 0x00, 0xFF},
		{0xFF, 0xFE, 0xFD},
	}

	for _, input := range inputs {
		stuffed := stuffBytes(input)
		unstuffed, err := unstuffBytes(stuffed)
		if err != nil {
			t.Errorf("unstuffBytes error for input %v: %v", input, err)
			continue
		}
		if !bytes.Equal(unstuffed, input) {
			t.Errorf("roundtrip failed: input=%v, stuffed=%v, unstuffed=%v", input, stuffed, unstuffed)
		}
	}
}

func TestEncodePacket_PayloadTooLarge(t *testing.T) {
	// Create a payload that will exceed MaxPayloadSize when CBOR encoded
	largePayload := make(map[int]interface{})
	for i := 0; i < 200; i++ {
		largePayload[i] = uint64(i)
	}

	_, err := EncodePacket(0, MsgStateData, largePayload)
	if err == nil {
		t.Error("expected error for oversized payload, got nil")
	}
}

func TestDecodePacket(t *testing.T) {
	// First encode a packet
	encoded, err := EncodePacket(0x1234567890ABCDEF, MsgPingRequest, nil)
	if err != nil {
		t.Fatalf("EncodePacket failed: %v", err)
	}

	// Then decode it using the convenience function
	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatalf("DecodePacket failed: %v", err)
	}

	if decoded.Address() != 0x1234567890ABCDEF {
		t.Errorf("address mismatch: got 0x%X, want 0x1234567890ABCDEF", decoded.Address())
	}
	if decoded.Type() != MsgPingRequest {
		t.Errorf("type mismatch: got 0x%02X, want 0x%02X", decoded.Type(), MsgPingRequest)
	}
}

func TestDecodePacket_Empty(t *testing.T) {
	_, err := DecodePacket([]byte{})
	if err == nil {
		t.Error("expected error for empty packet data, got nil")
	}
}

func TestDecodePacket_Incomplete(t *testing.T) {
	// Just a start byte - incomplete packet
	_, err := DecodePacket([]byte{StartByte})
	if err == nil {
		t.Error("expected error for incomplete packet data, got nil")
	}
}

func TestNewPacketWithPayload(t *testing.T) {
	payload := map[int]interface{}{
		0: uint64(1),
		1: uint64(2),
	}

	p := NewPacketWithPayload(0xAABBCCDD, MsgStateCommand, payload)

	if p.Address() != 0xAABBCCDD {
		t.Errorf("address mismatch: got 0x%X, want 0xAABBCCDD", p.Address())
	}
	if p.Type() != MsgStateCommand {
		t.Errorf("type mismatch: got 0x%02X, want 0x%02X", p.Type(), MsgStateCommand)
	}
	if p.PayloadMap() == nil {
		t.Error("payload map should not be nil")
	}
}

func TestMustEncodePacket(t *testing.T) {
	// Test the MustEncodePacket(p *Packet) []byte function
	p := NewPacketWithPayload(0x1122334455667788, MsgStateData, map[int]interface{}{
		0: false,
		2: uint64(1),
	})

	encoded := MustEncodePacket(p)

	if encoded[0] != StartByte || encoded[len(encoded)-1] != EndByte {
		t.Error("packet framing incorrect")
	}
}

func TestMustEncodePacket_Panic(t *testing.T) {
	// Verify that MustEncodePacket panics on oversized payload as documented
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustEncodePacket should panic on oversized payload")
		}
	}()

	// Create oversized payload that will exceed MaxPayloadSize
	largePayload := make(map[int]interface{})
	for i := 0; i < 200; i++ {
		largePayload[i] = uint64(i)
	}

	p := NewPacketWithPayload(0, MsgStateData, largePayload)
	MustEncodePacket(p) // Should panic
}

func TestEncodePacket_MessageTypeBoundary(t *testing.T) {
	// Test encoding with max message type value (0xFF)
	encoded, err := EncodePacket(0x1234567890ABCDEF, 0xFF, nil)
	if err != nil {
		t.Fatalf("EncodePacket failed for msgType 0xFF: %v", err)
	}

	// Decode and verify
	decoder := NewDecoder()
	var decoded *Packet
	for _, b := range encoded {
		p, err := decoder.DecodeByte(b)
		if err != nil {
			t.Fatalf("Decoder error: %v", err)
		}
		if p != nil {
			decoded = p
		}
	}

	if decoded == nil {
		t.Fatal("Decoder did not produce a packet")
	}
	if decoded.Type() != 0xFF {
		t.Errorf("msgType mismatch: got 0x%02X, want 0xFF", decoded.Type())
	}
}

func TestEncodePacket_ZeroLengthPayload(t *testing.T) {
	// Test that nil payload produces correct length byte (0x00 for CBOR [type, nil])
	encoded, err := EncodePacket(0x1234567890ABCDEF, MsgPingRequest, nil)
	if err != nil {
		t.Fatalf("EncodePacket failed: %v", err)
	}

	// Unstuff the packet content (between START and END bytes)
	unstuffed, err := unstuffBytes(encoded[1 : len(encoded)-1])
	if err != nil {
		t.Fatalf("unstuffBytes failed: %v", err)
	}

	// First byte after unstuffing is the length byte
	// For nil payload, CBOR encodes [msgType, nil] which is small but not zero
	lengthByte := unstuffed[0]
	if lengthByte == 0 {
		t.Error("length byte should not be 0 for CBOR-encoded [msgType, nil]")
	}
	// Verify the length is reasonable (CBOR array with type and nil is ~3-4 bytes)
	if lengthByte > 10 {
		t.Errorf("length byte unexpectedly large for nil payload: %d", lengthByte)
	}
}

func TestStuffBytes_ConsecutiveSpecialBytes(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		expect []byte
	}{
		{
			name:  "consecutive start bytes",
			input: []byte{StartByte, StartByte, StartByte},
			expect: []byte{
				EscByte, StartByte ^ EscXor,
				EscByte, StartByte ^ EscXor,
				EscByte, StartByte ^ EscXor,
			},
		},
		{
			name:  "consecutive end bytes",
			input: []byte{EndByte, EndByte, EndByte},
			expect: []byte{
				EscByte, EndByte ^ EscXor,
				EscByte, EndByte ^ EscXor,
				EscByte, EndByte ^ EscXor,
			},
		},
		{
			name:  "consecutive escape bytes",
			input: []byte{EscByte, EscByte, EscByte},
			expect: []byte{
				EscByte, EscByte ^ EscXor,
				EscByte, EscByte ^ EscXor,
				EscByte, EscByte ^ EscXor,
			},
		},
		{
			name:  "alternating special bytes",
			input: []byte{StartByte, EndByte, StartByte, EndByte},
			expect: []byte{
				EscByte, StartByte ^ EscXor,
				EscByte, EndByte ^ EscXor,
				EscByte, StartByte ^ EscXor,
				EscByte, EndByte ^ EscXor,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stuffBytes(tt.input)
			if !bytes.Equal(result, tt.expect) {
				t.Errorf("stuffBytes(%v) = %v, want %v", tt.input, result, tt.expect)
			}

			// Also verify round-trip
			unstuffed, err := unstuffBytes(result)
			if err != nil {
				t.Fatalf("unstuffBytes error: %v", err)
			}
			if !bytes.Equal(unstuffed, tt.input) {
				t.Errorf("round-trip failed: got %v, want %v", unstuffed, tt.input)
			}
		})
	}
}

func TestEncodePacket_CBOREncodingError(t *testing.T) {
	// Test that unencodable CBOR types return an error
	// Channels cannot be encoded to CBOR
	invalidPayload := map[int]interface{}{
		0: make(chan int),
	}

	_, err := EncodePacket(0, MsgStateData, invalidPayload)
	if err == nil {
		t.Error("expected error for unencodable CBOR payload (channel), got nil")
	}
}
