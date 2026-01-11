// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package fusain

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/fxamacker/cbor/v2"
)

// ============================================================
// CBOR Test Helpers
// ============================================================

// buildCBORPayload creates a CBOR-encoded message: [msgType, payloadMap]
func buildCBORPayload(msgType uint8, payload map[int]interface{}) []byte {
	var msg interface{}
	if payload == nil {
		msg = []interface{}{uint64(msgType), nil}
	} else {
		msg = []interface{}{uint64(msgType), payload}
	}
	data, err := cbor.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return data
}

// buildCBOREmptyPayload creates a CBOR-encoded message with nil payload
func buildCBOREmptyPayload(msgType uint8) []byte {
	return buildCBORPayload(msgType, nil)
}

// ============================================================
// CRC Tests
// ============================================================

func TestCalculateCRC_Empty(t *testing.T) {
	crc := CalculateCRC([]byte{})
	if crc != crcInitial {
		t.Errorf("CRC of empty data should be initial value, got 0x%04X", crc)
	}
}

func TestCalculateCRC_KnownValues(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint16
	}{
		{
			name:     "ASCII '123456789'",
			data:     []byte("123456789"),
			expected: 0x29B1, // Standard CRC-16-CCITT check value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crc := CalculateCRC(tt.data)
			if crc != tt.expected {
				t.Errorf("CRC mismatch: expected 0x%04X, got 0x%04X", tt.expected, crc)
			}
		})
	}
}

func TestCalculateCRC_Deterministic(t *testing.T) {
	data := []byte{0x10, 0x30, 0x01, 0x02, 0x03, 0x04}
	crc1 := CalculateCRC(data)
	crc2 := CalculateCRC(data)
	if crc1 != crc2 {
		t.Errorf("CRC should be deterministic: 0x%04X != 0x%04X", crc1, crc2)
	}
}

// ============================================================
// CBOR Parsing Tests
// ============================================================

func TestParseCBORMessage_Empty(t *testing.T) {
	_, _, err := ParseCBORMessage([]byte{})
	if err == nil {
		t.Error("Expected error for empty CBOR payload")
	}
}

func TestParseCBORMessage_PingRequest(t *testing.T) {
	// [47, nil] = PING_REQUEST with no payload
	data := buildCBOREmptyPayload(MsgPingRequest)
	msgType, payload, err := ParseCBORMessage(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if msgType != MsgPingRequest {
		t.Errorf("Expected MsgPingRequest (0x2F), got 0x%02X", msgType)
	}
	if payload != nil {
		t.Errorf("Expected nil payload, got %v", payload)
	}
}

func TestParseCBORMessage_StateData(t *testing.T) {
	// [48, {0: false, 1: 0, 2: 1, 3: 12345}]
	payload := map[int]interface{}{
		0: false,         // error
		1: uint64(0),     // code
		2: uint64(1),     // state (IDLE)
		3: uint64(12345), // timestamp
	}
	data := buildCBORPayload(MsgStateData, payload)
	msgType, parsed, err := ParseCBORMessage(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if msgType != MsgStateData {
		t.Errorf("Expected MsgStateData (0x30), got 0x%02X", msgType)
	}

	errorFlag, ok := GetMapBool(parsed, 0)
	if !ok || errorFlag != false {
		t.Error("Expected error=false")
	}

	state, ok := GetMapUint(parsed, 2)
	if !ok || state != 1 {
		t.Errorf("Expected state=1, got %d", state)
	}
}

func TestGetMapHelpers(t *testing.T) {
	m := map[int]interface{}{
		0: uint64(42),
		1: int64(-10),
		2: float64(3.14),
		3: true,
		4: []byte{0x01, 0x02},
	}

	// Test GetMapUint
	u, ok := GetMapUint(m, 0)
	if !ok || u != 42 {
		t.Errorf("GetMapUint(0) = %d, %v; want 42, true", u, ok)
	}

	// Test GetMapInt
	i, ok := GetMapInt(m, 1)
	if !ok || i != -10 {
		t.Errorf("GetMapInt(1) = %d, %v; want -10, true", i, ok)
	}

	// Test GetMapFloat
	f, ok := GetMapFloat(m, 2)
	if !ok || f != 3.14 {
		t.Errorf("GetMapFloat(2) = %f, %v; want 3.14, true", f, ok)
	}

	// Test GetMapBool
	b, ok := GetMapBool(m, 3)
	if !ok || b != true {
		t.Errorf("GetMapBool(3) = %v, %v; want true, true", b, ok)
	}

	// Test GetMapBytes
	bytes, ok := GetMapBytes(m, 4)
	if !ok || len(bytes) != 2 {
		t.Errorf("GetMapBytes(4) = %v, %v; want [0x01, 0x02], true", bytes, ok)
	}

	// Test missing key
	_, ok = GetMapUint(m, 99)
	if ok {
		t.Error("GetMapUint(99) should return false for missing key")
	}

	// Test nil map
	_, ok = GetMapUint(nil, 0)
	if ok {
		t.Error("GetMapUint(nil, 0) should return false for nil map")
	}
}

// ============================================================
// Packet Tests
// ============================================================

func TestNewPacket(t *testing.T) {
	cborPayload := buildCBORPayload(MsgStateData, map[int]interface{}{
		0: false, 1: uint64(0), 2: uint64(1), 3: uint64(1000),
	})
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0x1234)

	if p.Length() != uint8(len(cborPayload)) {
		t.Errorf("Length mismatch: expected %d, got %d", len(cborPayload), p.Length())
	}
	if p.Address() != 0x123456789ABCDEF0 {
		t.Errorf("Address mismatch: expected 0x123456789ABCDEF0, got 0x%016X", p.Address())
	}
	if p.Type() != MsgStateData {
		t.Errorf("Type mismatch: expected 0x%02X, got 0x%02X", MsgStateData, p.Type())
	}
	if p.CRC() != 0x1234 {
		t.Errorf("CRC mismatch: expected 0x1234, got 0x%04X", p.CRC())
	}
}

func TestPacket_IsBroadcast(t *testing.T) {
	cborPayload := buildCBOREmptyPayload(MsgStateCommand)
	p1 := NewPacket(uint8(len(cborPayload)), AddressBroadcast, cborPayload, 0)
	if !p1.IsBroadcast() {
		t.Error("Packet with AddressBroadcast should return true for IsBroadcast()")
	}

	p2 := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)
	if p2.IsBroadcast() {
		t.Error("Packet with non-broadcast address should return false for IsBroadcast()")
	}
}

func TestPacket_IsStateless(t *testing.T) {
	cborPayload := buildCBOREmptyPayload(MsgDiscoveryRequest)
	p1 := NewPacket(uint8(len(cborPayload)), AddressStateless, cborPayload, 0)
	if !p1.IsStateless() {
		t.Error("Packet with AddressStateless should return true for IsStateless()")
	}

	p2 := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)
	if p2.IsStateless() {
		t.Error("Packet with non-stateless address should return false for IsStateless()")
	}
}

func TestPacket_Timestamp(t *testing.T) {
	cborPayload := buildCBOREmptyPayload(MsgPingRequest)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)
	ts := p.Timestamp()
	if ts.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestPacket_PayloadMap(t *testing.T) {
	payload := map[int]interface{}{
		0: uint64(1000), // uptime
	}
	cborPayload := buildCBORPayload(MsgPingResponse, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	m := p.PayloadMap()
	if m == nil {
		t.Fatal("PayloadMap should not be nil")
	}

	uptime, ok := GetMapUint(m, 0)
	if !ok || uptime != 1000 {
		t.Errorf("Expected uptime=1000, got %d", uptime)
	}
}

// ============================================================
// Decoder Tests
// ============================================================

func TestDecoder_Reset(t *testing.T) {
	d := NewDecoder()

	// Feed some bytes
	d.DecodeByte(StartByte)
	d.DecodeByte(0x04)

	// Reset and verify state
	d.Reset()

	// Should be back to idle state (ignoring non-START bytes)
	packet, err := d.DecodeByte(0x00)
	if packet != nil || err != nil {
		t.Error("After reset, decoder should be in IDLE state ignoring non-START bytes")
	}
}

func TestDecoder_GetRawBytes(t *testing.T) {
	d := NewDecoder()

	// Feed some bytes (StartByte transitions state but is also stored)
	d.DecodeByte(StartByte)
	d.DecodeByte(0x04)
	d.DecodeByte(0x01)
	d.DecodeByte(0x02)

	raw := d.GetRawBytes()
	if len(raw) == 0 {
		t.Error("GetRawBytes should return accumulated bytes")
	}
}

func TestDecoder_SimplePacket(t *testing.T) {
	d := NewDecoder()

	// Build a PING_REQUEST packet (empty CBOR payload)
	// Format: START + LENGTH + ADDRESS(8) + CBOR_PAYLOAD + CRC(2) + END
	address := uint64(0x0102030405060708)
	cborPayload := buildCBOREmptyPayload(MsgPingRequest)
	length := uint8(len(cborPayload))

	// Calculate CRC over: length, address bytes, CBOR payload
	crcData := []byte{length}
	for i := 0; i < 8; i++ {
		crcData = append(crcData, byte(address>>(i*8)))
	}
	crcData = append(crcData, cborPayload...)
	crc := CalculateCRC(crcData)

	// Feed bytes to decoder
	d.DecodeByte(StartByte)
	d.DecodeByte(length)

	// Address bytes (little-endian)
	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(address >> (i * 8)))
	}

	// CBOR payload
	for _, b := range cborPayload {
		d.DecodeByte(b)
	}

	d.DecodeByte(byte(crc >> 8)) // CRC high
	d.DecodeByte(byte(crc))      // CRC low

	packet, err := d.DecodeByte(EndByte)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if packet == nil {
		t.Fatal("Expected packet, got nil")
	}

	if packet.Length() != length {
		t.Errorf("Length mismatch: expected %d, got %d", length, packet.Length())
	}
	if packet.Address() != address {
		t.Errorf("Address mismatch: expected 0x%016X, got 0x%016X", address, packet.Address())
	}
	if packet.Type() != MsgPingRequest {
		t.Errorf("Type mismatch: expected 0x%02X, got 0x%02X", MsgPingRequest, packet.Type())
	}
}

func TestDecoder_PacketWithPayload(t *testing.T) {
	d := NewDecoder()

	address := uint64(0x123456789ABCDEF0)
	payload := map[int]interface{}{
		0: false,          // error
		1: uint64(0),      // code
		2: uint64(1),      // state
		3: uint64(123456), // timestamp
	}
	cborPayload := buildCBORPayload(MsgStateData, payload)
	length := uint8(len(cborPayload))

	// Calculate CRC
	crcData := []byte{length}
	for i := 0; i < 8; i++ {
		crcData = append(crcData, byte(address>>(i*8)))
	}
	crcData = append(crcData, cborPayload...)
	crc := CalculateCRC(crcData)

	// Feed bytes
	d.DecodeByte(StartByte)
	d.DecodeByte(length)

	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(address >> (i * 8)))
	}

	for _, b := range cborPayload {
		d.DecodeByte(b)
	}

	d.DecodeByte(byte(crc >> 8))
	d.DecodeByte(byte(crc))

	packet, err := d.DecodeByte(EndByte)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if packet == nil {
		t.Fatal("Expected packet, got nil")
	}

	if packet.Type() != MsgStateData {
		t.Errorf("Type mismatch: expected 0x%02X, got 0x%02X", MsgStateData, packet.Type())
	}

	m := packet.PayloadMap()
	state, ok := GetMapUint(m, 2)
	if !ok || state != 1 {
		t.Errorf("State mismatch: expected 1, got %d", state)
	}
}

func TestDecoder_ByteStuffing(t *testing.T) {
	d := NewDecoder()

	address := uint64(0x0102030405060708)
	// Create a CBOR payload that contains bytes needing escaping
	payload := map[int]interface{}{
		0: uint64(0x7E), // Contains StartByte value
	}
	cborPayload := buildCBORPayload(MsgPingResponse, payload)
	length := uint8(len(cborPayload))

	// Calculate CRC (over unescaped data)
	crcData := []byte{length}
	for i := 0; i < 8; i++ {
		crcData = append(crcData, byte(address>>(i*8)))
	}
	crcData = append(crcData, cborPayload...)
	crc := CalculateCRC(crcData)

	// Feed bytes
	d.DecodeByte(StartByte)
	d.DecodeByte(length)

	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(address >> (i * 8)))
	}

	// Send CBOR payload with escaping
	for _, b := range cborPayload {
		if b == StartByte || b == EndByte || b == EscByte {
			d.DecodeByte(EscByte)
			d.DecodeByte(b ^ EscXor)
		} else {
			d.DecodeByte(b)
		}
	}

	d.DecodeByte(byte(crc >> 8))
	d.DecodeByte(byte(crc))

	packet, err := d.DecodeByte(EndByte)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if packet == nil {
		t.Fatal("Expected packet, got nil")
	}

	m := packet.PayloadMap()
	uptime, ok := GetMapUint(m, 0)
	if !ok || uptime != 0x7E {
		t.Errorf("Uptime mismatch: expected 0x7E, got 0x%X", uptime)
	}
}

func TestDecoder_CRCMismatch(t *testing.T) {
	d := NewDecoder()

	address := uint64(0x0102030405060708)
	cborPayload := buildCBOREmptyPayload(MsgPingRequest)
	length := uint8(len(cborPayload))

	// Intentionally wrong CRC
	wrongCRC := uint16(0xBEEF)

	d.DecodeByte(StartByte)
	d.DecodeByte(length)

	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(address >> (i * 8)))
	}

	for _, b := range cborPayload {
		d.DecodeByte(b)
	}

	d.DecodeByte(byte(wrongCRC >> 8))
	d.DecodeByte(byte(wrongCRC))

	packet, err := d.DecodeByte(EndByte)
	if err == nil {
		t.Error("Expected CRC mismatch error, got nil")
	}
	if packet != nil {
		t.Error("Expected nil packet on CRC error")
	}
}

func TestDecoder_InvalidLength(t *testing.T) {
	d := NewDecoder()

	// Length > MaxPayloadSize should error
	d.DecodeByte(StartByte)
	_, err := d.DecodeByte(MaxPayloadSize + 1)
	if err == nil {
		t.Error("Expected error for invalid length")
	}
}

func TestDecoder_StartByteResetsState(t *testing.T) {
	d := NewDecoder()

	// Start a packet
	d.DecodeByte(StartByte)
	d.DecodeByte(0x04)
	d.DecodeByte(0x01)
	d.DecodeByte(0x02)

	// Another StartByte should reset and start fresh
	d.DecodeByte(StartByte)

	// Now feed a complete valid packet
	address := uint64(0x0102030405060708)
	cborPayload := buildCBOREmptyPayload(MsgPingRequest)
	length := uint8(len(cborPayload))

	crcData := []byte{length}
	for i := 0; i < 8; i++ {
		crcData = append(crcData, byte(address>>(i*8)))
	}
	crcData = append(crcData, cborPayload...)
	crc := CalculateCRC(crcData)

	d.DecodeByte(length)
	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(address >> (i * 8)))
	}
	for _, b := range cborPayload {
		d.DecodeByte(b)
	}
	d.DecodeByte(byte(crc >> 8))
	d.DecodeByte(byte(crc))

	packet, err := d.DecodeByte(EndByte)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if packet == nil {
		t.Fatal("Expected packet after START reset")
	}
}

// ============================================================
// Validation Tests
// ============================================================

func TestValidatePacket_StateData_Valid(t *testing.T) {
	payload := map[int]interface{}{
		0: false,         // error
		1: uint64(0),     // code = ERROR_NONE
		2: uint64(0),     // state = INITIALIZING
		3: uint64(12345), // timestamp
	}
	cborPayload := buildCBORPayload(MsgStateData, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d: %v", len(errors), errors)
	}
}

func TestValidatePacket_StateData_InvalidState(t *testing.T) {
	payload := map[int]interface{}{
		0: false,          // error
		1: uint64(0),      // code
		2: uint64(255),    // state = invalid (> SysStateEstop)
		3: uint64(123456), // timestamp
	}
	cborPayload := buildCBORPayload(MsgStateData, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(errors))
		return
	}
	if errors[0].Type != AnomalyInvalidValue {
		t.Errorf("Expected AnomalyInvalidValue, got %d", errors[0].Type)
	}
}

func TestValidatePacket_MotorData_Valid(t *testing.T) {
	payload := map[int]interface{}{
		0: uint64(0),    // motor index
		1: uint64(1000), // timestamp
		2: int64(3000),  // rpm (valid, < 6000)
		3: int64(3000),  // target
		6: uint64(1000), // pwm
		7: uint64(2000), // pwm_max
	}
	cborPayload := buildCBORPayload(MsgMotorData, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d: %v", len(errors), errors)
	}
}

func TestValidatePacket_MotorData_HighRPM(t *testing.T) {
	payload := map[int]interface{}{
		0: uint64(0),    // motor index
		1: uint64(1000), // timestamp
		2: int64(7000),  // rpm (invalid, > 6000)
		3: int64(7000),  // target
	}
	cborPayload := buildCBORPayload(MsgMotorData, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(errors))
		return
	}
	if errors[0].Type != AnomalyHighRPM {
		t.Errorf("Expected AnomalyHighRPM, got %d", errors[0].Type)
	}
}

func TestValidatePacket_MotorData_PWMExceedsMax(t *testing.T) {
	payload := map[int]interface{}{
		0: uint64(0),    // motor index
		1: uint64(1000), // timestamp
		2: int64(3000),  // rpm
		3: int64(3000),  // target
		6: uint64(2500), // pwm (> pwm_max)
		7: uint64(2000), // pwm_max
	}
	cborPayload := buildCBORPayload(MsgMotorData, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(errors))
		return
	}
	if errors[0].Type != AnomalyInvalidPWM {
		t.Errorf("Expected AnomalyInvalidPWM, got %d", errors[0].Type)
	}
}

func TestValidatePacket_TempData_Valid(t *testing.T) {
	payload := map[int]interface{}{
		0: uint64(0),    // thermometer
		1: uint64(1000), // timestamp
		2: float64(25),  // reading (valid)
		5: float64(100), // target_temperature (valid)
	}
	cborPayload := buildCBORPayload(MsgTempData, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d: %v", len(errors), errors)
	}
}

func TestValidatePacket_TempData_InvalidTemp(t *testing.T) {
	payload := map[int]interface{}{
		0: uint64(0),     // thermometer
		1: uint64(1000),  // timestamp
		2: float64(-100), // reading (invalid, < -50)
	}
	cborPayload := buildCBORPayload(MsgTempData, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(errors))
		return
	}
	if errors[0].Type != AnomalyInvalidTemp {
		t.Errorf("Expected AnomalyInvalidTemp, got %d", errors[0].Type)
	}
}

func TestValidatePacket_GlowCommand_Valid(t *testing.T) {
	payload := map[int]interface{}{
		0: uint64(0),    // glow index
		1: int64(60000), // duration (valid, < 300000)
	}
	cborPayload := buildCBORPayload(MsgGlowCommand, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d: %v", len(errors), errors)
	}
}

func TestValidatePacket_GlowCommand_InvalidDuration(t *testing.T) {
	payload := map[int]interface{}{
		0: uint64(0),     // glow index
		1: int64(400000), // duration (invalid, > 300000)
	}
	cborPayload := buildCBORPayload(MsgGlowCommand, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(errors))
		return
	}
	if errors[0].Type != AnomalyInvalidValue {
		t.Errorf("Expected AnomalyInvalidValue, got %d", errors[0].Type)
	}
}

func TestValidatePacket_DeviceAnnounce_Valid(t *testing.T) {
	payload := map[int]interface{}{
		0: uint64(2), // motor_count
		1: uint64(3), // thermometer_count
		2: uint64(1), // pump_count
		3: uint64(1), // glow_count
	}
	cborPayload := buildCBORPayload(MsgDeviceAnnounce, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d: %v", len(errors), errors)
	}
}

func TestValidatePacket_DeviceAnnounce_InvalidCount(t *testing.T) {
	payload := map[int]interface{}{
		0: uint64(20), // motor_count (invalid, > 10)
		1: uint64(1),
		2: uint64(1),
		3: uint64(1),
	}
	cborPayload := buildCBORPayload(MsgDeviceAnnounce, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(errors))
		return
	}
	if errors[0].Type != AnomalyInvalidCount {
		t.Errorf("Expected AnomalyInvalidCount, got %d", errors[0].Type)
	}
}

func TestValidatePacket_DeviceAnnounce_Stateless(t *testing.T) {
	// End-of-discovery marker with stateless address
	payload := map[int]interface{}{
		0: uint64(0),
		1: uint64(0),
		2: uint64(0),
		3: uint64(0),
	}
	cborPayload := buildCBORPayload(MsgDeviceAnnounce, payload)
	p := NewPacket(uint8(len(cborPayload)), AddressStateless, cborPayload, 0)

	errors := ValidatePacket(p)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors for stateless DEVICE_ANNOUNCE, got %d: %v", len(errors), errors)
	}
}

func TestValidationError_Error(t *testing.T) {
	err := ValidationError{
		Type:    AnomalyHighRPM,
		Message: "RPM exceeds maximum",
		Details: map[string]interface{}{"rpm": 7000},
	}
	errStr := err.Error()
	if errStr != "RPM exceeds maximum" {
		t.Errorf("Error() should return message, got '%s'", errStr)
	}
}

// ============================================================
// Formatter Tests
// ============================================================

func TestFormatMessageType(t *testing.T) {
	tests := []struct {
		msgType  uint8
		expected string
	}{
		{MsgMotorConfig, "MOTOR_CONFIG"},
		{MsgPumpConfig, "PUMP_CONFIG"},
		{MsgTempConfig, "TEMP_CONFIG"},
		{MsgGlowConfig, "GLOW_CONFIG"},
		{MsgDataSubscription, "DATA_SUBSCRIPTION"},
		{MsgDataUnsubscribe, "DATA_UNSUBSCRIBE"},
		{MsgTelemetryConfig, "TELEMETRY_CONFIG"},
		{MsgTimeoutConfig, "TIMEOUT_CONFIG"},
		{MsgDiscoveryRequest, "DISCOVERY_REQUEST"},
		{MsgStateCommand, "STATE_COMMAND"},
		{MsgMotorCommand, "MOTOR_COMMAND"},
		{MsgPumpCommand, "PUMP_COMMAND"},
		{MsgGlowCommand, "GLOW_COMMAND"},
		{MsgTempCommand, "TEMP_COMMAND"},
		{MsgSendTelemetry, "SEND_TELEMETRY"},
		{MsgPingRequest, "PING_REQUEST"},
		{MsgStateData, "STATE_DATA"},
		{MsgMotorData, "MOTOR_DATA"},
		{MsgPumpData, "PUMP_DATA"},
		{MsgGlowData, "GLOW_DATA"},
		{MsgTempData, "TEMP_DATA"},
		{MsgDeviceAnnounce, "DEVICE_ANNOUNCE"},
		{MsgPingResponse, "PING_RESPONSE"},
		{MsgErrorInvalidCmd, "ERROR_INVALID_CMD"},
		{MsgErrorStateReject, "ERROR_STATE_REJECT"},
		{0x99, "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatMessageType(tt.msgType)
			if result != tt.expected {
				t.Errorf("FormatMessageType(0x%02X) = %s, expected %s", tt.msgType, result, tt.expected)
			}
		})
	}
}

func TestFormatPayloadMap_PingRequest(t *testing.T) {
	result := FormatPayloadMap(MsgPingRequest, nil)
	if result != "  (no payload)\n" {
		t.Errorf("Expected '  (no payload)\\n', got '%s'", result)
	}
}

func TestFormatPayloadMap_PingResponse(t *testing.T) {
	m := map[int]interface{}{
		0: uint64(3600000), // uptime = 1 hour
	}
	result := FormatPayloadMap(MsgPingResponse, m)
	if !strings.Contains(result, "1 hour") {
		t.Errorf("Expected uptime formatting with '1 hour', got '%s'", result)
	}
}

func TestFormatPayloadMap_StateData(t *testing.T) {
	m := map[int]interface{}{
		0: true,          // error
		1: uint64(1),     // code = OVERHEAT
		2: uint64(5),     // state = HEATING
		3: uint64(12345), // timestamp
	}
	result := FormatPayloadMap(MsgStateData, m)
	if !strings.Contains(result, "HEATING") {
		t.Error("Should contain state name 'HEATING'")
	}
	if !strings.Contains(result, "OVERHEAT") {
		t.Error("Should contain error code 'OVERHEAT'")
	}
}

func TestFormatPayloadMap_MotorData(t *testing.T) {
	m := map[int]interface{}{
		0: uint64(1),   // motor index
		1: uint64(100), // timestamp
		2: int64(3000), // rpm
		3: int64(3000), // target
	}
	result := FormatPayloadMap(MsgMotorData, m)
	if !strings.Contains(result, "Motor 1") {
		t.Error("Should contain 'Motor 1'")
	}
}

func TestFormatPayloadMap_StateCommand(t *testing.T) {
	m := map[int]interface{}{
		0: uint64(2), // mode = HEAT
	}
	result := FormatPayloadMap(MsgStateCommand, m)
	if !strings.Contains(result, "HEAT") {
		t.Error("Should contain mode 'HEAT'")
	}
}

func TestFormatPayloadMap_DeviceAnnounce(t *testing.T) {
	m := map[int]interface{}{
		0: uint64(2), // motors
		1: uint64(3), // temps
		2: uint64(1), // pumps
		3: uint64(1), // glow
	}
	result := FormatPayloadMap(MsgDeviceAnnounce, m)
	if !strings.Contains(result, "Motors: 2") {
		t.Error("Should contain 'Motors: 2'")
	}
}

func TestFormatPacket(t *testing.T) {
	payload := map[int]interface{}{
		0: false, 1: uint64(0), 2: uint64(1), 3: uint64(1000),
	}
	cborPayload := buildCBORPayload(MsgStateData, payload)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0x1234)

	result := FormatPacket(p)
	if !strings.Contains(result, "STATE_DATA") {
		t.Error("Should contain message type")
	}
	if !strings.Contains(result, "123456789ABCDEF0") {
		t.Error("Should contain address")
	}
}

// ============================================================
// Statistics Tests
// ============================================================

func TestStatistics_NewStatistics(t *testing.T) {
	s := NewStatistics()
	if s.TotalPackets != 0 {
		t.Error("New statistics should have 0 total packets")
	}
	if s.ValidPackets != 0 {
		t.Error("New statistics should have 0 valid packets")
	}
	if s.StartTime.IsZero() {
		t.Error("StartTime should be set")
	}
}

func TestStatistics_Update_ValidPacket(t *testing.T) {
	s := NewStatistics()
	cborPayload := buildCBOREmptyPayload(MsgPingRequest)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)

	s.Update(p, nil, nil)

	if s.TotalPackets != 1 {
		t.Errorf("TotalPackets should be 1, got %d", s.TotalPackets)
	}
	if s.ValidPackets != 1 {
		t.Errorf("ValidPackets should be 1, got %d", s.ValidPackets)
	}
}

func TestStatistics_Update_CRCError(t *testing.T) {
	s := NewStatistics()
	err := &testError{msg: "CRC mismatch: expected 0x1234, got 0x5678"}

	s.Update(nil, err, nil)

	if s.TotalPackets != 1 {
		t.Errorf("TotalPackets should be 1, got %d", s.TotalPackets)
	}
	if s.CRCErrors != 1 {
		t.Errorf("CRCErrors should be 1, got %d", s.CRCErrors)
	}
}

func TestStatistics_Update_DecodeError(t *testing.T) {
	s := NewStatistics()
	err := &testError{msg: "invalid length: 200"}

	s.Update(nil, err, nil)

	if s.TotalPackets != 1 {
		t.Errorf("TotalPackets should be 1, got %d", s.TotalPackets)
	}
	if s.DecodeErrors != 1 {
		t.Errorf("DecodeErrors should be 1, got %d", s.DecodeErrors)
	}
}

func TestStatistics_Update_ValidationErrors(t *testing.T) {
	s := NewStatistics()
	cborPayload := buildCBOREmptyPayload(MsgMotorData)
	p := NewPacket(uint8(len(cborPayload)), 0x123456789ABCDEF0, cborPayload, 0)
	validationErrors := []ValidationError{
		{Type: AnomalyHighRPM, Message: "High RPM detected"},
	}

	s.Update(p, nil, validationErrors)

	if s.TotalPackets != 1 {
		t.Errorf("TotalPackets should be 1, got %d", s.TotalPackets)
	}
	if s.HighRPM != 1 {
		t.Errorf("HighRPM should be 1, got %d", s.HighRPM)
	}
	if s.AnomalousValues != 1 {
		t.Errorf("AnomalousValues should be 1, got %d", s.AnomalousValues)
	}
}

func TestStatistics_Reset(t *testing.T) {
	s := NewStatistics()
	s.TotalPackets = 100
	s.ValidPackets = 95
	s.CRCErrors = 5

	s.Reset()

	if s.TotalPackets != 0 {
		t.Error("TotalPackets should be 0 after reset")
	}
	if s.ValidPackets != 0 {
		t.Error("ValidPackets should be 0 after reset")
	}
	if s.CRCErrors != 0 {
		t.Error("CRCErrors should be 0 after reset")
	}
}

func TestStatistics_CalculateRates(t *testing.T) {
	s := NewStatistics()
	s.TotalPackets = 100
	s.CRCErrors = 5
	s.DecodeErrors = 3
	s.MalformedPackets = 2
	s.AnomalousValues = 1

	s.CalculateRates()

	if s.PacketRate <= 0 {
		t.Error("PacketRate should be positive")
	}
	if s.ErrorRate <= 0 {
		t.Error("ErrorRate should be positive")
	}
}

func TestStatistics_String(t *testing.T) {
	s := NewStatistics()
	s.TotalPackets = 100
	s.ValidPackets = 90
	s.CRCErrors = 3
	s.DecodeErrors = 2
	s.MalformedPackets = 3
	s.InvalidCounts = 1
	s.LengthMismatches = 2
	s.AnomalousValues = 2
	s.HighRPM = 1
	s.InvalidTemp = 1

	result := s.String()

	if !strings.Contains(result, "Statistics") {
		t.Error("String should contain 'Statistics'")
	}
	if !strings.Contains(result, "Total Packets") {
		t.Error("String should contain 'Total Packets'")
	}
}

// ============================================================
// Helper Types
// ============================================================

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// Float64tobits converts float64 to uint64 bits (inverse of Float64frombits)
func Float64tobits(f float64) uint64 {
	return *(*uint64)(unsafe.Pointer(&f))
}

// ============================================================
// Decoder Buffer Overflow Tests
// ============================================================

func TestDecoder_BufferOverflow_AtLength(t *testing.T) {
	d := NewDecoder()

	d.DecodeByte(StartByte)
	d.bufferIndex = MaxPacketSize

	_, err := d.DecodeByte(0x04)
	if err == nil {
		t.Error("Expected buffer overflow error at length byte")
	}
	if !strings.Contains(err.Error(), "buffer overflow at length byte") {
		t.Errorf("Expected 'buffer overflow at length byte', got '%s'", err.Error())
	}
}

func TestDecoder_BufferOverflow_AtAddress(t *testing.T) {
	d := NewDecoder()

	d.DecodeByte(StartByte)
	d.DecodeByte(0x04)
	d.bufferIndex = MaxPacketSize

	_, err := d.DecodeByte(0x01)
	if err == nil {
		t.Error("Expected buffer overflow error at address byte")
	}
	if !strings.Contains(err.Error(), "buffer overflow at address byte") {
		t.Errorf("Expected 'buffer overflow at address byte', got '%s'", err.Error())
	}
}

func TestDecoder_BufferOverflow_AtPayload(t *testing.T) {
	d := NewDecoder()

	d.DecodeByte(StartByte)
	d.DecodeByte(0x04)

	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(i))
	}

	d.bufferIndex = MaxPacketSize

	_, err := d.DecodeByte(0x01)
	if err == nil {
		t.Error("Expected buffer overflow error at payload byte")
	}
	if !strings.Contains(err.Error(), "buffer overflow: packet exceeds max size") {
		t.Errorf("Expected 'buffer overflow: packet exceeds max size', got '%s'", err.Error())
	}
}

func TestDecoder_InvalidState(t *testing.T) {
	d := NewDecoder()

	d.DecodeByte(StartByte)

	if d.state != stateLength {
		t.Fatalf("Expected stateLength after StartByte, got %d", d.state)
	}

	d.state = 999

	_, err := d.DecodeByte(0x04)
	if err == nil {
		t.Error("Expected invalid state error")
	}
	if !strings.Contains(err.Error(), "invalid state:") {
		t.Errorf("Expected 'invalid state:' error, got '%s'", err.Error())
	}
}

func TestDecoder_UnexpectedEndByte(t *testing.T) {
	d := NewDecoder()

	d.DecodeByte(StartByte)
	d.DecodeByte(0x04)

	_, err := d.DecodeByte(EndByte)
	if err == nil {
		t.Error("Expected unexpected END byte error")
	}
	if !strings.Contains(err.Error(), "unexpected END byte") {
		t.Errorf("Expected 'unexpected END byte', got '%s'", err.Error())
	}
}
