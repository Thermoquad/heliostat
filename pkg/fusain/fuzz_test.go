// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package fusain

import (
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"
)

// getFuzzRounds returns the number of fuzz rounds from FUZZ_ROUNDS env var, default 1000
func getFuzzRounds() int {
	if envRounds := os.Getenv("FUZZ_ROUNDS"); envRounds != "" {
		if rounds, err := strconv.Atoi(envRounds); err == nil && rounds > 0 {
			return rounds
		}
	}
	return 1000
}

// getFuzzSeed returns the seed from FUZZ_SEED env var, or generates one from current time
func getFuzzSeed() int64 {
	if envSeed := os.Getenv("FUZZ_SEED"); envSeed != "" {
		if seed, err := strconv.ParseInt(envSeed, 10, 64); err == nil {
			return seed
		}
	}
	return time.Now().UnixNano()
}

// newFuzzRng creates a new random number generator and logs the seed for reproducibility
func newFuzzRng(t *testing.T) *rand.Rand {
	seed := getFuzzSeed()
	t.Logf("Seed: %d (reproduce with FUZZ_SEED=%d)", seed, seed)
	return rand.New(rand.NewSource(seed))
}

// ============================================================
// Decoder Fuzz Tests
// ============================================================

// TestFuzzDecoder_RandomBytes feeds random bytes to the decoder
// and verifies it doesn't crash or panic
func TestFuzzDecoder_RandomBytes(t *testing.T) {
	rounds := getFuzzRounds()
	rng := newFuzzRng(t)
	t.Logf("Running %d fuzz rounds", rounds)

	for i := 0; i < rounds; i++ {
		d := NewDecoder()

		// Generate random byte sequence of random length (1-512 bytes)
		length := rng.Intn(512) + 1
		data := make([]byte, length)
		rng.Read(data)

		// Feed all bytes to decoder - should not panic
		for _, b := range data {
			d.DecodeByte(b)
		}
	}
}

// TestFuzzDecoder_RandomPackets generates random valid-looking packets
// with random payloads
func TestFuzzDecoder_RandomPackets(t *testing.T) {
	rounds := getFuzzRounds()
	rng := newFuzzRng(t)
	t.Logf("Running %d fuzz rounds", rounds)

	for i := 0; i < rounds; i++ {
		d := NewDecoder()

		// Generate random packet
		address := rng.Uint64()
		msgType := uint8(rng.Intn(256))
		payloadLen := rng.Intn(int(MaxPayloadSize) + 1)
		payload := make([]byte, payloadLen)
		rng.Read(payload)

		// Build CRC data
		crcData := []byte{uint8(payloadLen)}
		for j := 0; j < 8; j++ {
			crcData = append(crcData, byte(address>>(j*8)))
		}
		crcData = append(crcData, msgType)
		crcData = append(crcData, payload...)
		crc := CalculateCRC(crcData)

		// Feed packet with byte stuffing
		d.DecodeByte(StartByte)
		feedByteWithStuffing(d, uint8(payloadLen))
		for j := 0; j < 8; j++ {
			feedByteWithStuffing(d, byte(address>>(j*8)))
		}
		feedByteWithStuffing(d, msgType)
		for _, b := range payload {
			feedByteWithStuffing(d, b)
		}
		feedByteWithStuffing(d, byte(crc>>8))
		feedByteWithStuffing(d, byte(crc))
		packet, err := d.DecodeByte(EndByte)

		// Packet should decode successfully
		if err != nil {
			t.Errorf("Round %d: unexpected decode error: %v", i, err)
			continue
		}
		if packet == nil {
			t.Errorf("Round %d: expected packet, got nil", i)
			continue
		}

		// Verify packet fields
		if packet.Length() != uint8(payloadLen) {
			t.Errorf("Round %d: length mismatch: expected %d, got %d", i, payloadLen, packet.Length())
		}
		if packet.Address() != address {
			t.Errorf("Round %d: address mismatch: expected 0x%016X, got 0x%016X", i, address, packet.Address())
		}
		if packet.Type() != msgType {
			t.Errorf("Round %d: type mismatch: expected 0x%02X, got 0x%02X", i, msgType, packet.Type())
		}
	}
}

// TestFuzzDecoder_CorruptedPackets generates packets with random corruption
func TestFuzzDecoder_CorruptedPackets(t *testing.T) {
	rounds := getFuzzRounds()
	rng := newFuzzRng(t)
	t.Logf("Running %d fuzz rounds", rounds)

	for i := 0; i < rounds; i++ {
		d := NewDecoder()

		// Generate a valid packet first
		address := rng.Uint64()
		msgType := uint8(rng.Intn(256))
		payloadLen := rng.Intn(50) + 1 // Smaller for speed
		payload := make([]byte, payloadLen)
		rng.Read(payload)

		// Build packet bytes (without byte stuffing for simplicity)
		packetBytes := []byte{StartByte, uint8(payloadLen)}
		for j := 0; j < 8; j++ {
			packetBytes = append(packetBytes, byte(address>>(j*8)))
		}
		packetBytes = append(packetBytes, msgType)
		packetBytes = append(packetBytes, payload...)

		// Calculate correct CRC
		crcData := packetBytes[1:] // Skip StartByte
		crc := CalculateCRC(crcData)
		packetBytes = append(packetBytes, byte(crc>>8), byte(crc))
		packetBytes = append(packetBytes, EndByte)

		// Corrupt a random byte (not START or END)
		if len(packetBytes) > 2 {
			corruptIdx := rng.Intn(len(packetBytes)-2) + 1 // Skip START and END
			packetBytes[corruptIdx] ^= byte(rng.Intn(255) + 1)
		}

		// Feed corrupted packet - should not panic
		for _, b := range packetBytes {
			d.DecodeByte(b)
		}
	}
}

// TestFuzzDecoder_MissingBytes tests packets with missing bytes
func TestFuzzDecoder_MissingBytes(t *testing.T) {
	rounds := getFuzzRounds()
	rng := newFuzzRng(t)
	t.Logf("Running %d fuzz rounds", rounds)

	for i := 0; i < rounds; i++ {
		d := NewDecoder()

		// Build valid packet bytes
		address := rng.Uint64()
		msgType := uint8(rng.Intn(256))
		payloadLen := rng.Intn(20) + 1
		payload := make([]byte, payloadLen)
		rng.Read(payload)

		packetBytes := []byte{StartByte, uint8(payloadLen)}
		for j := 0; j < 8; j++ {
			packetBytes = append(packetBytes, byte(address>>(j*8)))
		}
		packetBytes = append(packetBytes, msgType)
		packetBytes = append(packetBytes, payload...)

		crcData := packetBytes[1:]
		crc := CalculateCRC(crcData)
		packetBytes = append(packetBytes, byte(crc>>8), byte(crc))
		packetBytes = append(packetBytes, EndByte)

		// Remove random bytes
		numToRemove := rng.Intn(5) + 1
		for j := 0; j < numToRemove && len(packetBytes) > 2; j++ {
			idx := rng.Intn(len(packetBytes))
			packetBytes = append(packetBytes[:idx], packetBytes[idx+1:]...)
		}

		// Feed truncated packet - should not panic
		for _, b := range packetBytes {
			d.DecodeByte(b)
		}
	}
}

// TestFuzzDecoder_ExtraBytes tests packets with extra random bytes inserted
func TestFuzzDecoder_ExtraBytes(t *testing.T) {
	rounds := getFuzzRounds()
	rng := newFuzzRng(t)
	t.Logf("Running %d fuzz rounds", rounds)

	for i := 0; i < rounds; i++ {
		d := NewDecoder()

		// Build valid packet bytes
		address := rng.Uint64()
		msgType := uint8(rng.Intn(256))
		payloadLen := rng.Intn(20) + 1
		payload := make([]byte, payloadLen)
		rng.Read(payload)

		packetBytes := []byte{StartByte, uint8(payloadLen)}
		for j := 0; j < 8; j++ {
			packetBytes = append(packetBytes, byte(address>>(j*8)))
		}
		packetBytes = append(packetBytes, msgType)
		packetBytes = append(packetBytes, payload...)

		crcData := packetBytes[1:]
		crc := CalculateCRC(crcData)
		packetBytes = append(packetBytes, byte(crc>>8), byte(crc))
		packetBytes = append(packetBytes, EndByte)

		// Insert random bytes at random positions
		numToInsert := rng.Intn(5) + 1
		for j := 0; j < numToInsert; j++ {
			idx := rng.Intn(len(packetBytes) + 1)
			extraByte := byte(rng.Intn(256))
			packetBytes = append(packetBytes[:idx], append([]byte{extraByte}, packetBytes[idx:]...)...)
		}

		// Feed modified packet - should not panic
		for _, b := range packetBytes {
			d.DecodeByte(b)
		}
	}
}

// TestFuzzDecoder_RepeatedStart tests handling of repeated START bytes
func TestFuzzDecoder_RepeatedStart(t *testing.T) {
	rounds := getFuzzRounds()
	rng := newFuzzRng(t)
	t.Logf("Running %d fuzz rounds", rounds)

	for i := 0; i < rounds; i++ {
		d := NewDecoder()

		// Send random number of START bytes
		numStarts := rng.Intn(100) + 1
		for j := 0; j < numStarts; j++ {
			d.DecodeByte(StartByte)
		}

		// Now send a valid packet
		address := uint64(0x0102030405060708)
		msgType := uint8(MsgPingRequest)
		length := uint8(0)

		crcData := []byte{length}
		for j := 0; j < 8; j++ {
			crcData = append(crcData, byte(address>>(j*8)))
		}
		crcData = append(crcData, msgType)
		crc := CalculateCRC(crcData)

		d.DecodeByte(length)
		for j := 0; j < 8; j++ {
			d.DecodeByte(byte(address >> (j * 8)))
		}
		d.DecodeByte(msgType)
		d.DecodeByte(byte(crc >> 8))
		d.DecodeByte(byte(crc))

		packet, err := d.DecodeByte(EndByte)
		if err != nil {
			t.Errorf("Round %d: unexpected error after repeated START: %v", i, err)
		}
		if packet == nil {
			t.Errorf("Round %d: expected valid packet after repeated START", i)
		}
	}
}

// ============================================================
// CRC Fuzz Tests
// ============================================================

// TestFuzzCRC_RandomData tests CRC calculation with random data
func TestFuzzCRC_RandomData(t *testing.T) {
	rounds := getFuzzRounds()
	rng := newFuzzRng(t)
	t.Logf("Running %d fuzz rounds", rounds)

	for i := 0; i < rounds; i++ {
		// Generate random data
		length := rng.Intn(1000) + 1
		data := make([]byte, length)
		rng.Read(data)

		// Calculate CRC - should not panic
		crc1 := CalculateCRC(data)
		crc2 := CalculateCRC(data)

		// CRC should be deterministic
		if crc1 != crc2 {
			t.Errorf("Round %d: CRC not deterministic: 0x%04X != 0x%04X", i, crc1, crc2)
		}

		// Modify one byte - CRC should change
		if len(data) > 0 {
			idx := rng.Intn(len(data))
			original := data[idx]
			data[idx] ^= byte(rng.Intn(255) + 1)
			crc3 := CalculateCRC(data)
			data[idx] = original

			if crc3 == crc1 {
				// This can happen (CRC collision) but should be rare
				// Just note it, don't fail
				t.Logf("Round %d: CRC collision detected (rare but possible)", i)
			}
		}
	}
}

// ============================================================
// Validation Fuzz Tests
// ============================================================

// TestFuzzValidation_RandomPackets tests validation with random packet contents
func TestFuzzValidation_RandomPackets(t *testing.T) {
	rounds := getFuzzRounds()
	rng := newFuzzRng(t)
	t.Logf("Running %d fuzz rounds", rounds)

	// Test each message type that has validation
	msgTypes := []uint8{
		MsgStateData,
		MsgMotorData,
		MsgTempData,
		MsgGlowCommand,
		MsgDeviceAnnounce,
	}

	for i := 0; i < rounds; i++ {
		for _, msgType := range msgTypes {
			// Generate random payload
			payloadLen := rng.Intn(int(MaxPayloadSize) + 1)
			payload := make([]byte, payloadLen)
			rng.Read(payload)

			// Create packet
			address := rng.Uint64()
			p := NewPacket(uint8(payloadLen), address, msgType, payload, 0)

			// Validate - should not panic
			errors := ValidatePacket(p)

			// Errors slice should be non-nil
			if errors == nil {
				t.Errorf("Round %d: ValidatePacket returned nil slice", i)
			}
		}
	}
}

// TestFuzzValidation_EdgeCases tests validation with edge case values
func TestFuzzValidation_EdgeCases(t *testing.T) {
	rounds := getFuzzRounds()
	rng := newFuzzRng(t)
	t.Logf("Running %d fuzz rounds", rounds)

	edgeCaseValues := []byte{0x00, 0x01, 0x7E, 0x7F, 0x7D, 0xFE, 0xFF}

	for i := 0; i < rounds; i++ {
		// Create payload filled with edge case values
		payloadLen := rng.Intn(100) + 1
		payload := make([]byte, payloadLen)
		for j := range payload {
			payload[j] = edgeCaseValues[rng.Intn(len(edgeCaseValues))]
		}

		// Test with different message types
		msgTypes := []uint8{MsgStateData, MsgMotorData, MsgTempData}
		msgType := msgTypes[rng.Intn(len(msgTypes))]

		p := NewPacket(uint8(payloadLen), 0x123456789ABCDEF0, msgType, payload, 0)

		// Validate - should not panic
		ValidatePacket(p)
	}
}

// ============================================================
// Formatter Fuzz Tests
// ============================================================

// TestFuzzFormatter_RandomPackets tests formatting with random packets
func TestFuzzFormatter_RandomPackets(t *testing.T) {
	rounds := getFuzzRounds()
	rng := newFuzzRng(t)
	t.Logf("Running %d fuzz rounds", rounds)

	for i := 0; i < rounds; i++ {
		// Generate random packet
		msgType := uint8(rng.Intn(256))
		payloadLen := rng.Intn(int(MaxPayloadSize) + 1)
		payload := make([]byte, payloadLen)
		rng.Read(payload)

		address := rng.Uint64()
		p := NewPacket(uint8(payloadLen), address, msgType, payload, 0)

		// Format - should not panic
		result := FormatPacket(p)
		if result == "" {
			t.Errorf("Round %d: FormatPacket returned empty string", i)
		}

		// FormatMessageType - should not panic
		typeStr := FormatMessageType(msgType)
		if typeStr == "" {
			t.Errorf("Round %d: FormatMessageType returned empty string", i)
		}

		// FormatPayload - should not panic
		payloadStr := FormatPayload(msgType, payload)
		if payloadStr == "" {
			t.Errorf("Round %d: FormatPayload returned empty string", i)
		}
	}
}

// ============================================================
// Helper Functions
// ============================================================

// feedByteWithStuffing sends a byte to the decoder with proper byte stuffing
func feedByteWithStuffing(d *Decoder, b byte) {
	if b == StartByte || b == EndByte || b == EscByte {
		d.DecodeByte(EscByte)
		d.DecodeByte(b ^ EscXor)
	} else {
		d.DecodeByte(b)
	}
}
