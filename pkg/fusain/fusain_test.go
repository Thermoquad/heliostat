// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package fusain

import (
	"bytes"
	"strings"
	"testing"
	"unsafe"
)

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
// Packet Tests
// ============================================================

func TestNewPacket(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03, 0x04}
	p := NewPacket(4, 0x123456789ABCDEF0, MsgStateData, payload, 0x1234)

	if p.Length() != 4 {
		t.Errorf("Length mismatch: expected 4, got %d", p.Length())
	}
	if p.Address() != 0x123456789ABCDEF0 {
		t.Errorf("Address mismatch: expected 0x123456789ABCDEF0, got 0x%016X", p.Address())
	}
	if p.Type() != MsgStateData {
		t.Errorf("Type mismatch: expected 0x%02X, got 0x%02X", MsgStateData, p.Type())
	}
	if !bytes.Equal(p.Payload(), payload) {
		t.Errorf("Payload mismatch: expected %v, got %v", payload, p.Payload())
	}
	if p.CRC() != 0x1234 {
		t.Errorf("CRC mismatch: expected 0x1234, got 0x%04X", p.CRC())
	}
}

func TestPacket_IsBroadcast(t *testing.T) {
	p1 := NewPacket(0, AddressBroadcast, MsgStateCommand, nil, 0)
	if !p1.IsBroadcast() {
		t.Error("Packet with AddressBroadcast should return true for IsBroadcast()")
	}

	p2 := NewPacket(0, 0x123456789ABCDEF0, MsgStateCommand, nil, 0)
	if p2.IsBroadcast() {
		t.Error("Packet with non-broadcast address should return false for IsBroadcast()")
	}
}

func TestPacket_IsStateless(t *testing.T) {
	p1 := NewPacket(0, AddressStateless, MsgDiscoveryRequest, nil, 0)
	if !p1.IsStateless() {
		t.Error("Packet with AddressStateless should return true for IsStateless()")
	}

	p2 := NewPacket(0, 0x123456789ABCDEF0, MsgStateData, nil, 0)
	if p2.IsStateless() {
		t.Error("Packet with non-stateless address should return false for IsStateless()")
	}
}

func TestPacket_Timestamp(t *testing.T) {
	p := NewPacket(0, 0x123456789ABCDEF0, MsgPingRequest, nil, 0)
	ts := p.Timestamp()
	if ts.IsZero() {
		t.Error("Timestamp should not be zero")
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
	// GetRawBytes returns accumulated buffer including StartByte
	if len(raw) == 0 {
		t.Error("GetRawBytes should return accumulated bytes")
	}
}

func TestDecoder_SimplePacket(t *testing.T) {
	d := NewDecoder()

	// Build a simple packet with no payload
	// Format: START + LENGTH + ADDRESS(8) + TYPE + CRC(2) + END
	address := uint64(0x0102030405060708)
	msgType := uint8(MsgPingRequest)
	length := uint8(0)

	// Calculate CRC over: length, address bytes, type
	crcData := []byte{length}
	for i := 0; i < 8; i++ {
		crcData = append(crcData, byte(address>>(i*8)))
	}
	crcData = append(crcData, msgType)
	crc := CalculateCRC(crcData)

	// Feed bytes to decoder
	d.DecodeByte(StartByte)
	d.DecodeByte(length)

	// Address bytes (little-endian)
	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(address >> (i * 8)))
	}

	d.DecodeByte(msgType)
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
	if packet.Type() != msgType {
		t.Errorf("Type mismatch: expected 0x%02X, got 0x%02X", msgType, packet.Type())
	}
}

func TestDecoder_PacketWithPayload(t *testing.T) {
	d := NewDecoder()

	address := uint64(0x123456789ABCDEF0)
	msgType := uint8(MsgStateData)
	payload := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	length := uint8(len(payload))

	// Calculate CRC
	crcData := []byte{length}
	for i := 0; i < 8; i++ {
		crcData = append(crcData, byte(address>>(i*8)))
	}
	crcData = append(crcData, msgType)
	crcData = append(crcData, payload...)
	crc := CalculateCRC(crcData)

	// Feed bytes
	d.DecodeByte(StartByte)
	d.DecodeByte(length)

	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(address >> (i * 8)))
	}

	d.DecodeByte(msgType)

	for _, b := range payload {
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

	if !bytes.Equal(packet.Payload(), payload) {
		t.Errorf("Payload mismatch: expected %v, got %v", payload, packet.Payload())
	}
}

func TestDecoder_ByteStuffing(t *testing.T) {
	d := NewDecoder()

	// Test with StartByte in payload (must be escaped)
	address := uint64(0x0102030405060708)
	msgType := uint8(MsgStateData)
	// Payload contains StartByte which needs escaping
	payload := []byte{StartByte, 0x02, 0x03, 0x04}
	length := uint8(len(payload))

	// Calculate CRC (over unescaped data)
	crcData := []byte{length}
	for i := 0; i < 8; i++ {
		crcData = append(crcData, byte(address>>(i*8)))
	}
	crcData = append(crcData, msgType)
	crcData = append(crcData, payload...)
	crc := CalculateCRC(crcData)

	// Feed bytes
	d.DecodeByte(StartByte)
	d.DecodeByte(length)

	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(address >> (i * 8)))
	}

	d.DecodeByte(msgType)

	// Send payload with escaping
	for _, b := range payload {
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

	if !bytes.Equal(packet.Payload(), payload) {
		t.Errorf("Payload mismatch after byte unstuffing: expected %v, got %v", payload, packet.Payload())
	}
}

func TestDecoder_CRCMismatch(t *testing.T) {
	d := NewDecoder()

	address := uint64(0x0102030405060708)
	msgType := uint8(MsgPingRequest)
	length := uint8(0)

	// Intentionally wrong CRC
	wrongCRC := uint16(0xBEEF)

	d.DecodeByte(StartByte)
	d.DecodeByte(length)

	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(address >> (i * 8)))
	}

	d.DecodeByte(msgType)
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
	msgType := uint8(MsgPingRequest)
	length := uint8(0)

	crcData := []byte{length}
	for i := 0; i < 8; i++ {
		crcData = append(crcData, byte(address>>(i*8)))
	}
	crcData = append(crcData, msgType)
	crc := CalculateCRC(crcData)

	d.DecodeByte(length)
	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(address >> (i * 8)))
	}
	d.DecodeByte(msgType)
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
	// Valid STATE_DATA: 16 bytes payload
	// timestamp(4) + error(4) + state(4) + mode(4)
	payload := make([]byte, 16)
	// Set valid state value (0 = INITIALIZING)
	payload[8] = 0x00
	// Set valid error code (0 = ERROR_NONE)
	payload[4] = 0x00

	p := NewPacket(16, 0x123456789ABCDEF0, MsgStateData, payload, 0)
	errors := ValidatePacket(p)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d: %v", len(errors), errors)
	}
}

func TestValidatePacket_StateData_InvalidState(t *testing.T) {
	payload := make([]byte, 16)
	// Set invalid state value (255 > SYS_STATE_ESTOP)
	payload[8] = 0xFF
	payload[9] = 0xFF
	payload[10] = 0xFF
	payload[11] = 0xFF

	p := NewPacket(16, 0x123456789ABCDEF0, MsgStateData, payload, 0)
	errors := ValidatePacket(p)
	if len(errors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(errors))
		return
	}
	if errors[0].Type != AnomalyInvalidValue {
		t.Errorf("Expected AnomalyInvalidValue, got %d", errors[0].Type)
	}
}

func TestValidatePacket_StateData_TooShort(t *testing.T) {
	payload := make([]byte, 8) // Too short (needs 16)
	p := NewPacket(8, 0x123456789ABCDEF0, MsgStateData, payload, 0)
	errors := ValidatePacket(p)
	if len(errors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(errors))
		return
	}
	if errors[0].Type != AnomalyLengthMismatch {
		t.Errorf("Expected AnomalyLengthMismatch, got %d", errors[0].Type)
	}
}

func TestValidatePacket_MotorData_Valid(t *testing.T) {
	// Valid MOTOR_DATA: 32 bytes
	payload := make([]byte, 32)
	// RPM = 3000 (valid, < 6000)
	payload[8] = 0xB8
	payload[9] = 0x0B
	// Target = 3000
	payload[12] = 0xB8
	payload[13] = 0x0B
	// PWM = 1000
	payload[24] = 0xE8
	payload[25] = 0x03
	// PWM Max = 2000
	payload[28] = 0xD0
	payload[29] = 0x07

	p := NewPacket(32, 0x123456789ABCDEF0, MsgMotorData, payload, 0)
	errors := ValidatePacket(p)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d: %v", len(errors), errors)
	}
}

func TestValidatePacket_MotorData_HighRPM(t *testing.T) {
	payload := make([]byte, 32)
	// RPM = 7000 (invalid, > 6000)
	payload[8] = 0x58
	payload[9] = 0x1B

	p := NewPacket(32, 0x123456789ABCDEF0, MsgMotorData, payload, 0)
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
	payload := make([]byte, 32)
	// PWM = 2500
	payload[24] = 0xC4
	payload[25] = 0x09
	// PWM Max = 2000
	payload[28] = 0xD0
	payload[29] = 0x07

	p := NewPacket(32, 0x123456789ABCDEF0, MsgMotorData, payload, 0)
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
	payload := make([]byte, 40)
	// Temperature = 25.0°C (valid)
	tempBits := Float64tobits(25.0)
	for i := 0; i < 8; i++ {
		payload[8+i] = byte(tempBits >> (i * 8))
	}
	// Target temp = 100.0°C (valid)
	targetBits := Float64tobits(100.0)
	for i := 0; i < 8; i++ {
		payload[24+i] = byte(targetBits >> (i * 8))
	}

	p := NewPacket(40, 0x123456789ABCDEF0, MsgTempData, payload, 0)
	errors := ValidatePacket(p)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d: %v", len(errors), errors)
	}
}

func TestValidatePacket_TempData_InvalidTemp(t *testing.T) {
	payload := make([]byte, 40)
	// Temperature = -100.0°C (invalid, < -50)
	tempBits := Float64tobits(-100.0)
	for i := 0; i < 8; i++ {
		payload[8+i] = byte(tempBits >> (i * 8))
	}

	p := NewPacket(40, 0x123456789ABCDEF0, MsgTempData, payload, 0)
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
	payload := make([]byte, 8)
	// Duration = 60000 ms (valid, < 300000)
	payload[4] = 0x60
	payload[5] = 0xEA

	p := NewPacket(8, 0x123456789ABCDEF0, MsgGlowCommand, payload, 0)
	errors := ValidatePacket(p)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d: %v", len(errors), errors)
	}
}

func TestValidatePacket_GlowCommand_InvalidDuration(t *testing.T) {
	payload := make([]byte, 8)
	// Duration = 400000 ms (invalid, > 300000)
	payload[4] = 0x80
	payload[5] = 0x1A
	payload[6] = 0x06

	p := NewPacket(8, 0x123456789ABCDEF0, MsgGlowCommand, payload, 0)
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
	payload := make([]byte, 8)
	// motor_count = 2, temp_count = 3, pump_count = 1, glow_count = 1
	payload[0] = 2
	payload[2] = 3
	payload[4] = 1
	payload[6] = 1

	p := NewPacket(8, 0x123456789ABCDEF0, MsgDeviceAnnounce, payload, 0)
	errors := ValidatePacket(p)
	if len(errors) != 0 {
		t.Errorf("Expected no validation errors, got %d: %v", len(errors), errors)
	}
}

func TestValidatePacket_DeviceAnnounce_InvalidCount(t *testing.T) {
	payload := make([]byte, 8)
	// motor_count = 20 (invalid, > 10)
	payload[0] = 20

	p := NewPacket(8, 0x123456789ABCDEF0, MsgDeviceAnnounce, payload, 0)
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
	payload := make([]byte, 8)
	// All zeros (end-of-discovery)

	p := NewPacket(8, AddressStateless, MsgDeviceAnnounce, payload, 0)
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

func TestFormatPayload_PingRequest(t *testing.T) {
	result := FormatPayload(MsgPingRequest, nil)
	if result != "  (no payload)\n" {
		t.Errorf("Expected '  (no payload)\\n', got '%s'", result)
	}
}

func TestFormatPayload_PingResponse(t *testing.T) {
	// Uptime = 3600000 ms (1 hour) = 0x36EE80 in little endian
	payload := []byte{0x80, 0xEE, 0x36, 0x00}
	result := FormatPayload(MsgPingResponse, payload)
	if result != "  Uptime: 1 hour\n" {
		t.Errorf("Expected uptime formatting, got '%s'", result)
	}
}

func TestFormatPayload_UnknownType(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03, 0x04}
	result := FormatPayload(0x99, payload)
	if !strings.Contains(result, "Payload:") {
		t.Error("Unknown type should produce hex dump with 'Payload:'")
	}
}

func TestFormatPayload_StateData(t *testing.T) {
	// STATE_DATA: error(4) + code(4) + state(4) + timestamp(4)
	payload := make([]byte, 16)
	payload[0] = 1     // error flag = 1
	payload[4] = 1     // code = OVERHEAT
	payload[8] = 5     // state = HEATING
	payload[12] = 0x10 // timestamp = 16 ms
	result := FormatPayload(MsgStateData, payload)
	if !strings.Contains(result, "HEATING") {
		t.Error("Should contain state name 'HEATING'")
	}
	if !strings.Contains(result, "OVERHEAT") {
		t.Error("Should contain error code 'OVERHEAT'")
	}
}

func TestFormatPayload_MotorData(t *testing.T) {
	payload := make([]byte, 32)
	payload[0] = 1   // motor index
	payload[8] = 100 // rpm = 100
	result := FormatPayload(MsgMotorData, payload)
	if !strings.Contains(result, "Motor 1") {
		t.Error("Should contain 'Motor 1'")
	}
}

func TestFormatPayload_StateCommand(t *testing.T) {
	payload := make([]byte, 8)
	payload[0] = 2 // uint32(ModeHeat)
	result := FormatPayload(MsgStateCommand, payload)
	if !strings.Contains(result, "HEAT") {
		t.Error("Should contain mode 'HEAT'")
	}
}

func TestFormatPayload_AllModes(t *testing.T) {
	modes := []struct {
		mode     uint32
		expected string
	}{
		{uint32(ModeIdle), "IDLE"},
		{uint32(ModeFan), "FAN"},
		{uint32(ModeHeat), "HEAT"},
		{uint32(ModeEmergency), "EMERGENCY"},
		{99, "UNKNOWN"},
	}

	for _, m := range modes {
		payload := make([]byte, 8)
		payload[0] = byte(m.mode)
		result := FormatPayload(MsgStateCommand, payload)
		if !strings.Contains(result, m.expected) {
			t.Errorf("Mode %d should format as '%s', got '%s'", m.mode, m.expected, result)
		}
	}
}

func TestFormatPayload_AllStates(t *testing.T) {
	states := []struct {
		state    uint32
		expected string
	}{
		{0, "INITIALIZING"},
		{1, "IDLE"},
		{2, "BLOWING"},
		{3, "PREHEAT"},
		{4, "PREHEAT_STAGE_2"},
		{5, "HEATING"},
		{6, "COOLING"},
		{7, "ERROR"},
		{8, "E_STOP"},
		{99, "UNKNOWN"},
	}

	for _, s := range states {
		payload := make([]byte, 16)
		payload[8] = byte(s.state)
		result := FormatPayload(MsgStateData, payload)
		if !strings.Contains(result, s.expected) {
			t.Errorf("State %d should format as '%s', got '%s'", s.state, s.expected, result)
		}
	}
}

func TestFormatPayload_AllErrorCodes(t *testing.T) {
	codes := []struct {
		code     int32
		expected string
	}{
		{0, "NONE"},
		{1, "OVERHEAT"},
		{2, "SENSOR_FAULT"},
		{3, "IGNITION_FAIL"},
		{4, "FLAME_OUT"},
		{5, "MOTOR_STALL"},
		{6, "PUMP_FAULT"},
		{7, "COMMANDED_ESTOP"},
		{99, "UNKNOWN"},
		{-1, "UNKNOWN"},
	}

	for _, c := range codes {
		payload := make([]byte, 16)
		payload[4] = byte(c.code)
		if c.code < 0 {
			payload[4] = 0xFF
			payload[5] = 0xFF
			payload[6] = 0xFF
			payload[7] = 0xFF
		}
		result := FormatPayload(MsgStateData, payload)
		if !strings.Contains(result, c.expected) {
			t.Errorf("Error code %d should format as '%s', got '%s'", c.code, c.expected, result)
		}
	}
}

func TestFormatPayload_TempCommand(t *testing.T) {
	cmdTypes := []struct {
		cmdType  uint32
		expected string
	}{
		{uint32(TempCmdWatchMotor), "WATCH_MOTOR"},
		{uint32(TempCmdUnwatchMotor), "UNWATCH_MOTOR"},
		{uint32(TempCmdEnableRpmControl), "ENABLE_RPM_CONTROL"},
		{uint32(TempCmdDisableRpmControl), "DISABLE_RPM_CONTROL"},
		{uint32(TempCmdSetTargetTemp), "SET_TARGET_TEMP"},
		{99, "UNKNOWN"},
	}

	for _, ct := range cmdTypes {
		payload := make([]byte, 20)
		payload[4] = byte(ct.cmdType)
		result := FormatPayload(MsgTempCommand, payload)
		if !strings.Contains(result, ct.expected) {
			t.Errorf("Temp command type %d should format as '%s', got '%s'", ct.cmdType, ct.expected, result)
		}
	}
}

func TestFormatPayload_SendTelemetry(t *testing.T) {
	telTypes := []struct {
		telType  uint32
		expected string
	}{
		{uint32(TelemetryTypeState), "STATE"},
		{uint32(TelemetryTypeMotor), "MOTOR"},
		{uint32(TelemetryTypeTemp), "TEMPERATURE"},
		{uint32(TelemetryTypePump), "PUMP"},
		{uint32(TelemetryTypeGlow), "GLOW"},
		{99, "UNKNOWN"},
	}

	for _, tt := range telTypes {
		payload := make([]byte, 8)
		payload[0] = byte(tt.telType)
		result := FormatPayload(MsgSendTelemetry, payload)
		if !strings.Contains(result, tt.expected) {
			t.Errorf("Telemetry type %d should format as '%s', got '%s'", tt.telType, tt.expected, result)
		}
	}
}

func TestFormatPayload_SendTelemetry_AllIndex(t *testing.T) {
	payload := make([]byte, 8)
	payload[4] = 0xFF
	payload[5] = 0xFF
	payload[6] = 0xFF
	payload[7] = 0xFF
	result := FormatPayload(MsgSendTelemetry, payload)
	if !strings.Contains(result, "ALL") {
		t.Errorf("Index 0xFFFFFFFF should format as 'ALL', got '%s'", result)
	}
}

func TestFormatPayload_PumpData(t *testing.T) {
	eventTypes := []struct {
		eventType uint32
		expected  string
	}{
		{uint32(PumpEventInitializing), "INITIALIZING"},
		{uint32(PumpEventReady), "READY"},
		{uint32(PumpEventError), "ERROR"},
		{uint32(PumpEventCycleStart), "CYCLE_START"},
		{uint32(PumpEventPulseEnd), "PULSE_END"},
		{uint32(PumpEventCycleEnd), "CYCLE_END"},
		{99, "UNKNOWN"},
	}

	for _, et := range eventTypes {
		payload := make([]byte, 16)
		payload[8] = byte(et.eventType)
		result := FormatPayload(MsgPumpData, payload)
		if !strings.Contains(result, et.expected) {
			t.Errorf("Pump event type %d should format as '%s', got '%s'", et.eventType, et.expected, result)
		}
	}
}

func TestFormatPayload_GlowData(t *testing.T) {
	payload := make([]byte, 12)
	payload[8] = 1 // lit = On
	result := FormatPayload(MsgGlowData, payload)
	if !strings.Contains(result, "On") {
		t.Error("Should contain 'On' for lit glow plug")
	}

	payload[8] = 0 // lit = Off
	result = FormatPayload(MsgGlowData, payload)
	if !strings.Contains(result, "Off") {
		t.Error("Should contain 'Off' for unlit glow plug")
	}
}

func TestFormatPayload_TempData(t *testing.T) {
	payload := make([]byte, 40)
	// Set temperature reading (bytes 8-15)
	tempBits := Float64tobits(25.5)
	for i := 0; i < 8; i++ {
		payload[8+i] = byte(tempBits >> (i * 8))
	}
	payload[16] = 1 // rpm_ctrl = On
	result := FormatPayload(MsgTempData, payload)
	if !strings.Contains(result, "25.5") {
		t.Error("Should contain temperature value")
	}
	if !strings.Contains(result, "RPM_Ctrl=On") {
		t.Error("Should contain 'RPM_Ctrl=On'")
	}
}

func TestFormatPayload_DeviceAnnounce(t *testing.T) {
	payload := make([]byte, 8)
	payload[0] = 2 // motors
	payload[2] = 3 // temps
	payload[4] = 1 // pumps
	payload[6] = 1 // glow
	result := FormatPayload(MsgDeviceAnnounce, payload)
	if !strings.Contains(result, "Motors: 2") {
		t.Error("Should contain 'Motors: 2'")
	}
}

func TestFormatPayload_ErrorInvalidCmd(t *testing.T) {
	payload := make([]byte, 4)
	payload[0] = 1 // Invalid parameter value
	result := FormatPayload(MsgErrorInvalidCmd, payload)
	if !strings.Contains(result, "Invalid parameter value") {
		t.Error("Should contain error message")
	}

	payload[0] = 2 // Invalid device index
	result = FormatPayload(MsgErrorInvalidCmd, payload)
	if !strings.Contains(result, "Invalid device index") {
		t.Error("Should contain error message")
	}
}

func TestFormatPayload_ErrorStateReject(t *testing.T) {
	payload := make([]byte, 4)
	payload[0] = 5 // HEATING state
	result := FormatPayload(MsgErrorStateReject, payload)
	if !strings.Contains(result, "HEATING") {
		t.Error("Should contain state name")
	}
}

func TestFormatPayload_DataSubscription(t *testing.T) {
	payload := make([]byte, 8)
	payload[0] = 0x01
	payload[1] = 0x02
	result := FormatPayload(MsgDataSubscription, payload)
	if !strings.Contains(result, "Address") {
		t.Error("Should contain 'Address'")
	}
}

func TestFormatPayload_TelemetryConfig(t *testing.T) {
	payload := make([]byte, 8)
	payload[0] = 1    // enabled
	payload[4] = 0x64 // interval = 100ms
	result := FormatPayload(MsgTelemetryConfig, payload)
	if !strings.Contains(result, "Enabled") {
		t.Error("Should contain 'Enabled'")
	}

	payload[4] = 0 // interval = 0 (polling mode)
	result = FormatPayload(MsgTelemetryConfig, payload)
	if !strings.Contains(result, "Polling") {
		t.Error("Should contain 'Polling'")
	}
}

func TestFormatPayload_TimeoutConfig(t *testing.T) {
	payload := make([]byte, 8)
	payload[0] = 0 // disabled
	result := FormatPayload(MsgTimeoutConfig, payload)
	if !strings.Contains(result, "Disabled") {
		t.Error("Should contain 'Disabled'")
	}
}

func TestFormatPayload_MotorConfig(t *testing.T) {
	payload := make([]byte, 56)
	payload[0] = 1 // motor index
	result := FormatPayload(MsgMotorConfig, payload)
	if !strings.Contains(result, "Motor 1") {
		t.Error("Should contain 'Motor 1'")
	}
}

func TestFormatPayload_PumpConfig(t *testing.T) {
	payload := make([]byte, 12)
	payload[0] = 1 // pump index
	result := FormatPayload(MsgPumpConfig, payload)
	if !strings.Contains(result, "Pump 1") {
		t.Error("Should contain 'Pump 1'")
	}
}

func TestFormatPayload_TempConfig(t *testing.T) {
	payload := make([]byte, 36)
	payload[0] = 1 // therm index
	result := FormatPayload(MsgTempConfig, payload)
	if !strings.Contains(result, "Thermometer 1") {
		t.Error("Should contain 'Thermometer 1'")
	}
}

func TestFormatPayload_GlowConfig(t *testing.T) {
	payload := make([]byte, 8)
	payload[0] = 1 // glow index
	result := FormatPayload(MsgGlowConfig, payload)
	if !strings.Contains(result, "Glow 1") {
		t.Error("Should contain 'Glow 1'")
	}
}

func TestFormatPayload_MotorCommand(t *testing.T) {
	payload := make([]byte, 8)
	payload[0] = 1
	payload[4] = 0xE8
	payload[5] = 0x03 // rpm = 1000
	result := FormatPayload(MsgMotorCommand, payload)
	if !strings.Contains(result, "Motor: 1") {
		t.Error("Should contain 'Motor: 1'")
	}
}

func TestFormatPayload_PumpCommand(t *testing.T) {
	payload := make([]byte, 8)
	payload[0] = 1
	result := FormatPayload(MsgPumpCommand, payload)
	if !strings.Contains(result, "Pump: 1") {
		t.Error("Should contain 'Pump: 1'")
	}
}

func TestFormatPayload_GlowCommand(t *testing.T) {
	payload := make([]byte, 8)
	payload[0] = 1
	result := FormatPayload(MsgGlowCommand, payload)
	if !strings.Contains(result, "Glow: 1") {
		t.Error("Should contain 'Glow: 1'")
	}
}

func TestFormatDuration_EdgeCases(t *testing.T) {
	// Note: PING_RESPONSE uptime is u32 (max ~49 days), so we test within that range
	tests := []struct {
		ms       uint32
		expected string
	}{
		{500, "500 ms"},                             // less than 1 second
		{1000, "1 second"},                          // exactly 1 second
		{2000, "2 seconds"},                         // plural seconds
		{60000, "1 minute"},                         // exactly 1 minute
		{120000, "2 minutes"},                       // plural minutes
		{3600000, "1 hour"},                         // exactly 1 hour
		{7200000, "2 hours"},                        // plural hours
		{86400000, "1 day"},                         // exactly 1 day
		{172800000, "2 days"},                       // plural days
		{90000000, "1 day and 1 hour"},              // day + hour (exact)
		{3661000, "1 hour, 1 minute, and 1 second"}, // hour + minute + second
	}

	for _, tt := range tests {
		// Test via FormatPayload using PING_RESPONSE
		payload := make([]byte, 4)
		payload[0] = byte(tt.ms)
		payload[1] = byte(tt.ms >> 8)
		payload[2] = byte(tt.ms >> 16)
		payload[3] = byte(tt.ms >> 24)
		result := FormatPayload(MsgPingResponse, payload)
		if !strings.Contains(result, tt.expected) {
			t.Errorf("Duration %d ms should format as '%s', got '%s'", tt.ms, tt.expected, result)
		}
	}
}

func TestFormatDuration_Years(t *testing.T) {
	// Test year handling directly (exceeds u32 range used by PING_RESPONSE)
	const (
		msPerSecond = 1000
		msPerMinute = 60 * msPerSecond
		msPerHour   = 60 * msPerMinute
		msPerDay    = 24 * msPerHour
		msPerYear   = 365 * msPerDay
	)

	tests := []struct {
		ms       uint64
		expected string
	}{
		{msPerYear, "1 year"},                                           // exactly 1 year
		{2 * msPerYear, "2 years"},                                      // plural years
		{msPerYear + msPerDay, "1 year and 1 day"},                      // year + day
		{2*msPerYear + 2*msPerDay, "2 years and 2 days"},                // years + days
		{msPerYear + msPerDay + msPerHour, "1 year, 1 day, and 1 hour"}, // year + day + hour
	}

	for _, tt := range tests {
		result := formatDuration(tt.ms)
		if result != tt.expected {
			t.Errorf("formatDuration(%d) = '%s', expected '%s'", tt.ms, result, tt.expected)
		}
	}
}

func TestFormatPacket(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03, 0x04}
	p := NewPacket(4, 0x123456789ABCDEF0, MsgStateData, payload, 0x1234)
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
	p := NewPacket(0, 0x123456789ABCDEF0, MsgPingRequest, nil, 0)

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
	p := NewPacket(0, 0x123456789ABCDEF0, MsgMotorData, nil, 0)
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

	// Check that key sections appear in output
	if !strings.Contains(result, "Statistics") {
		t.Error("String should contain 'Statistics'")
	}
	if !strings.Contains(result, "Total Packets") {
		t.Error("String should contain 'Total Packets'")
	}
	if !strings.Contains(result, "Valid Packets") {
		t.Error("String should contain 'Valid Packets'")
	}
	if !strings.Contains(result, "CRC Errors") {
		t.Error("String should contain 'CRC Errors'")
	}
	if !strings.Contains(result, "Decode Errors") {
		t.Error("String should contain 'Decode Errors'")
	}
	if !strings.Contains(result, "Malformed") {
		t.Error("String should contain 'Malformed'")
	}
	if !strings.Contains(result, "Invalid Counts") {
		t.Error("String should contain 'Invalid Counts'")
	}
	if !strings.Contains(result, "Anomalous") {
		t.Error("String should contain 'Anomalous'")
	}
	if !strings.Contains(result, "High RPM") {
		t.Error("String should contain 'High RPM'")
	}
}

func TestStatistics_String_NoErrors(t *testing.T) {
	s := NewStatistics()
	s.TotalPackets = 50
	s.ValidPackets = 50

	result := s.String()

	// Should have basic sections but not error details
	if !strings.Contains(result, "Total Packets") {
		t.Error("String should contain 'Total Packets'")
	}
	// Should not have CRC Errors section if count is 0
	if strings.Contains(result, "CRC Errors") {
		t.Error("String should not contain 'CRC Errors' when count is 0")
	}
}

func TestStatistics_String_ZeroPackets(t *testing.T) {
	s := NewStatistics()
	result := s.String()

	if !strings.Contains(result, "Total Packets:") {
		t.Error("String should contain 'Total Packets:' even with 0 packets")
	}
}

func TestStatistics_Update_AllValidationErrors(t *testing.T) {
	// Test AnomalyInvalidValue branch
	s := NewStatistics()
	p := NewPacket(0, 0x123456789ABCDEF0, MsgStateData, nil, 0)
	validationErrors := []ValidationError{
		{Type: AnomalyInvalidValue, Message: "Invalid value"},
	}

	s.Update(p, nil, validationErrors)

	if s.AnomalousValues != 1 {
		t.Errorf("AnomalousValues should be 1, got %d", s.AnomalousValues)
	}

	// Test AnomalyInvalidCount
	s2 := NewStatistics()
	validationErrors2 := []ValidationError{
		{Type: AnomalyInvalidCount, Message: "Invalid count"},
	}
	s2.Update(p, nil, validationErrors2)
	if s2.InvalidCounts != 1 {
		t.Errorf("InvalidCounts should be 1, got %d", s2.InvalidCounts)
	}
	if s2.MalformedPackets != 1 {
		t.Errorf("MalformedPackets should be 1, got %d", s2.MalformedPackets)
	}

	// Test AnomalyLengthMismatch
	s3 := NewStatistics()
	validationErrors3 := []ValidationError{
		{Type: AnomalyLengthMismatch, Message: "Length mismatch"},
	}
	s3.Update(p, nil, validationErrors3)
	if s3.LengthMismatches != 1 {
		t.Errorf("LengthMismatches should be 1, got %d", s3.LengthMismatches)
	}

	// Test AnomalyInvalidTemp
	s4 := NewStatistics()
	validationErrors4 := []ValidationError{
		{Type: AnomalyInvalidTemp, Message: "Invalid temp"},
	}
	s4.Update(p, nil, validationErrors4)
	if s4.InvalidTemp != 1 {
		t.Errorf("InvalidTemp should be 1, got %d", s4.InvalidTemp)
	}

	// Test AnomalyInvalidPWM
	s5 := NewStatistics()
	validationErrors5 := []ValidationError{
		{Type: AnomalyInvalidPWM, Message: "Invalid PWM"},
	}
	s5.Update(p, nil, validationErrors5)
	if s5.InvalidPWM != 1 {
		t.Errorf("InvalidPWM should be 1, got %d", s5.InvalidPWM)
	}
}

func TestStatistics_String_WithInvalidPWM(t *testing.T) {
	s := NewStatistics()
	s.TotalPackets = 10
	s.AnomalousValues = 1
	s.InvalidPWM = 1

	result := s.String()

	if !strings.Contains(result, "Invalid PWM") {
		t.Error("String should contain 'Invalid PWM'")
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

// These tests verify defensive buffer overflow checks by directly
// manipulating decoder internal state. These branches cannot be
// triggered through normal API usage but are important safety guards.

func TestDecoder_BufferOverflow_AtLength(t *testing.T) {
	d := NewDecoder()

	// Start a packet normally
	d.DecodeByte(StartByte)

	// Simulate buffer overflow condition by setting bufferIndex to max
	d.bufferIndex = MaxPacketSize

	// Next byte (length) should trigger overflow error
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

	// Start a packet and get to ADDRESS state
	d.DecodeByte(StartByte)
	d.DecodeByte(0x04) // Valid length

	// Simulate buffer overflow condition
	d.bufferIndex = MaxPacketSize

	// Next byte (address) should trigger overflow error
	_, err := d.DecodeByte(0x01)
	if err == nil {
		t.Error("Expected buffer overflow error at address byte")
	}
	if !strings.Contains(err.Error(), "buffer overflow at address byte") {
		t.Errorf("Expected 'buffer overflow at address byte', got '%s'", err.Error())
	}
}

func TestDecoder_BufferOverflow_AtType(t *testing.T) {
	d := NewDecoder()

	// Start a packet and get to TYPE state
	d.DecodeByte(StartByte)
	d.DecodeByte(0x04) // Valid length

	// Feed all 8 address bytes
	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(i))
	}

	// Now in STATE_TYPE - simulate buffer overflow
	d.bufferIndex = MaxPacketSize

	// Next byte (type) should trigger overflow error
	_, err := d.DecodeByte(MsgPingRequest)
	if err == nil {
		t.Error("Expected buffer overflow error at type byte")
	}
	if !strings.Contains(err.Error(), "buffer overflow at type byte") {
		t.Errorf("Expected 'buffer overflow at type byte', got '%s'", err.Error())
	}
}

func TestDecoder_BufferOverflow_AtPayload(t *testing.T) {
	d := NewDecoder()

	// Start a packet and get to PAYLOAD state
	d.DecodeByte(StartByte)
	d.DecodeByte(0x04) // Length = 4 bytes payload

	// Feed all 8 address bytes
	for i := 0; i < 8; i++ {
		d.DecodeByte(byte(i))
	}

	d.DecodeByte(MsgStateData) // Type

	// Now in STATE_PAYLOAD - simulate buffer overflow
	d.bufferIndex = MaxPacketSize

	// Next byte (payload) should trigger overflow error
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

	// Start a packet to get into LENGTH state
	d.DecodeByte(StartByte)

	// Verify we're in LENGTH state after StartByte
	if d.state != stateLength {
		t.Fatalf("Expected stateLength after StartByte, got %d", d.state)
	}

	// Manually set an invalid state
	d.state = 999

	// Next byte (not START or END) should trigger invalid state error
	// Note: Reset() is called before error is generated, so state shows as 0
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

	// Start a packet
	d.DecodeByte(StartByte)
	d.DecodeByte(0x04) // Length

	// Send END byte while in ADDRESS state (unexpected)
	_, err := d.DecodeByte(EndByte)
	if err == nil {
		t.Error("Expected unexpected END byte error")
	}
	if !strings.Contains(err.Error(), "unexpected END byte") {
		t.Errorf("Expected 'unexpected END byte', got '%s'", err.Error())
	}
}
