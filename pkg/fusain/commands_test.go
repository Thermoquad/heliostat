// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package fusain

import "testing"

func TestNewStateCommand(t *testing.T) {
	tests := []struct {
		name     string
		address  uint64
		mode     uint8
		argument *int64
		wantMode uint64
		wantArg  bool
	}{
		{
			name:     "idle mode no argument",
			address:  0x1234567890ABCDEF,
			mode:     uint8(ModeIdle),
			argument: nil,
			wantMode: 0,
			wantArg:  false,
		},
		{
			name:     "fan mode with rpm argument",
			address:  0x1234567890ABCDEF,
			mode:     uint8(ModeFan),
			argument: ptr(int64(2500)),
			wantMode: 1,
			wantArg:  true,
		},
		{
			name:     "heat mode with pump rate argument",
			address:  0x1234567890ABCDEF,
			mode:     uint8(ModeHeat),
			argument: ptr(int64(100)),
			wantMode: 2,
			wantArg:  true,
		},
		{
			name:     "emergency mode",
			address:  0x1234567890ABCDEF,
			mode:     uint8(ModeEmergency),
			argument: nil,
			wantMode: 255,
			wantArg:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewStateCommand(tt.address, tt.mode, tt.argument)

			if p.Address() != tt.address {
				t.Errorf("Address() = 0x%X, want 0x%X", p.Address(), tt.address)
			}
			if p.Type() != MsgStateCommand {
				t.Errorf("Type() = 0x%02X, want 0x%02X", p.Type(), MsgStateCommand)
			}

			payload := p.PayloadMap()
			if payload == nil {
				t.Fatal("PayloadMap() returned nil")
			}

			mode, ok := GetMapUint(payload, 0)
			if !ok {
				t.Error("payload missing mode (key 0)")
			}
			if mode != tt.wantMode {
				t.Errorf("mode = %d, want %d", mode, tt.wantMode)
			}

			_, hasArg := payload[1]
			if hasArg != tt.wantArg {
				t.Errorf("has argument = %v, want %v", hasArg, tt.wantArg)
			}
		})
	}
}

func TestNewStateCommand_RoundTrip(t *testing.T) {
	arg := int64(2500)
	p := NewStateCommand(0x1234567890ABCDEF, uint8(ModeFan), &arg)

	encoded, err := EncodePacket(p.Address(), p.Type(), p.PayloadMap())
	if err != nil {
		t.Fatalf("EncodePacket failed: %v", err)
	}

	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatalf("DecodePacket failed: %v", err)
	}

	if decoded.Type() != MsgStateCommand {
		t.Errorf("decoded Type() = 0x%02X, want 0x%02X", decoded.Type(), MsgStateCommand)
	}

	payload := decoded.PayloadMap()
	mode, _ := GetMapUint(payload, 0)
	if mode != 1 {
		t.Errorf("decoded mode = %d, want 1", mode)
	}

	argument, ok := GetMapInt(payload, 1)
	if !ok {
		t.Error("decoded payload missing argument (key 1)")
	}
	if argument != 2500 {
		t.Errorf("decoded argument = %d, want 2500", argument)
	}
}

func TestNewPingRequest(t *testing.T) {
	p := NewPingRequest(0xAABBCCDDEEFF0011)

	if p.Address() != 0xAABBCCDDEEFF0011 {
		t.Errorf("Address() = 0x%X, want 0xAABBCCDDEEFF0011", p.Address())
	}
	if p.Type() != MsgPingRequest {
		t.Errorf("Type() = 0x%02X, want 0x%02X", p.Type(), MsgPingRequest)
	}

	payload := p.PayloadMap()
	if payload != nil && len(payload) > 0 {
		t.Errorf("PayloadMap() should be nil or empty, got %v", payload)
	}
}

func TestNewPingRequest_RoundTrip(t *testing.T) {
	p := NewPingRequest(0x1234567890ABCDEF)

	encoded, err := EncodePacket(p.Address(), p.Type(), p.PayloadMap())
	if err != nil {
		t.Fatalf("EncodePacket failed: %v", err)
	}

	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatalf("DecodePacket failed: %v", err)
	}

	if decoded.Type() != MsgPingRequest {
		t.Errorf("decoded Type() = 0x%02X, want 0x%02X", decoded.Type(), MsgPingRequest)
	}
}

func TestNewTelemetryConfig(t *testing.T) {
	tests := []struct {
		name       string
		enabled    bool
		intervalMs uint32
	}{
		{"enabled with interval", true, 100},
		{"disabled", false, 0},
		{"polling mode", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewTelemetryConfig(0x1234567890ABCDEF, tt.enabled, tt.intervalMs)

			if p.Type() != MsgTelemetryConfig {
				t.Errorf("Type() = 0x%02X, want 0x%02X", p.Type(), MsgTelemetryConfig)
			}

			payload := p.PayloadMap()
			if payload == nil {
				t.Fatal("PayloadMap() returned nil")
			}

			enabled, ok := GetMapBool(payload, 0)
			if !ok {
				t.Error("payload missing enabled (key 0)")
			}
			if enabled != tt.enabled {
				t.Errorf("enabled = %v, want %v", enabled, tt.enabled)
			}

			interval, ok := GetMapUint(payload, 1)
			if !ok {
				t.Error("payload missing interval_ms (key 1)")
			}
			if interval != uint64(tt.intervalMs) {
				t.Errorf("interval_ms = %d, want %d", interval, tt.intervalMs)
			}
		})
	}
}

func TestNewTelemetryConfig_RoundTrip(t *testing.T) {
	p := NewTelemetryConfig(0x1234567890ABCDEF, true, 100)

	encoded, err := EncodePacket(p.Address(), p.Type(), p.PayloadMap())
	if err != nil {
		t.Fatalf("EncodePacket failed: %v", err)
	}

	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatalf("DecodePacket failed: %v", err)
	}

	if decoded.Type() != MsgTelemetryConfig {
		t.Errorf("decoded Type() = 0x%02X, want 0x%02X", decoded.Type(), MsgTelemetryConfig)
	}

	payload := decoded.PayloadMap()
	enabled, ok := GetMapBool(payload, 0)
	if !ok {
		t.Error("decoded payload missing enabled (key 0)")
	}
	if enabled != true {
		t.Errorf("decoded enabled = %v, want true", enabled)
	}

	interval, ok := GetMapUint(payload, 1)
	if !ok {
		t.Error("decoded payload missing interval_ms (key 1)")
	}
	if interval != 100 {
		t.Errorf("decoded interval_ms = %d, want 100", interval)
	}
}

func TestNewMotorCommand(t *testing.T) {
	p := NewMotorCommand(0x1234567890ABCDEF, 0, 2500)

	if p.Type() != MsgMotorCommand {
		t.Errorf("Type() = 0x%02X, want 0x%02X", p.Type(), MsgMotorCommand)
	}

	payload := p.PayloadMap()
	if payload == nil {
		t.Fatal("PayloadMap() returned nil")
	}

	motor, ok := GetMapInt(payload, 0)
	if !ok {
		t.Error("payload missing motor (key 0)")
	}
	if motor != 0 {
		t.Errorf("motor = %d, want 0", motor)
	}

	rpm, ok := GetMapInt(payload, 1)
	if !ok {
		t.Error("payload missing rpm (key 1)")
	}
	if rpm != 2500 {
		t.Errorf("rpm = %d, want 2500", rpm)
	}
}

func TestNewMotorCommand_RoundTrip(t *testing.T) {
	p := NewMotorCommand(0x1234567890ABCDEF, 1, 3000)

	encoded, err := EncodePacket(p.Address(), p.Type(), p.PayloadMap())
	if err != nil {
		t.Fatalf("EncodePacket failed: %v", err)
	}

	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatalf("DecodePacket failed: %v", err)
	}

	if decoded.Type() != MsgMotorCommand {
		t.Errorf("decoded Type() = 0x%02X, want 0x%02X", decoded.Type(), MsgMotorCommand)
	}

	payload := decoded.PayloadMap()
	motor, _ := GetMapInt(payload, 0)
	rpm, _ := GetMapInt(payload, 1)
	if motor != 1 || rpm != 3000 {
		t.Errorf("decoded motor=%d rpm=%d, want motor=1 rpm=3000", motor, rpm)
	}
}

func TestNewPumpCommand(t *testing.T) {
	p := NewPumpCommand(0x1234567890ABCDEF, 0, 150)

	if p.Type() != MsgPumpCommand {
		t.Errorf("Type() = 0x%02X, want 0x%02X", p.Type(), MsgPumpCommand)
	}

	payload := p.PayloadMap()
	if payload == nil {
		t.Fatal("PayloadMap() returned nil")
	}

	pump, ok := GetMapInt(payload, 0)
	if !ok {
		t.Error("payload missing pump (key 0)")
	}
	if pump != 0 {
		t.Errorf("pump = %d, want 0", pump)
	}

	rateMs, ok := GetMapInt(payload, 1)
	if !ok {
		t.Error("payload missing rate_ms (key 1)")
	}
	if rateMs != 150 {
		t.Errorf("rate_ms = %d, want 150", rateMs)
	}
}

func TestNewPumpCommand_Stop(t *testing.T) {
	p := NewPumpCommand(0x1234567890ABCDEF, 0, 0)

	payload := p.PayloadMap()
	rateMs, _ := GetMapInt(payload, 1)
	if rateMs != 0 {
		t.Errorf("rate_ms = %d, want 0 (stop)", rateMs)
	}
}

func TestNewPumpCommand_RoundTrip(t *testing.T) {
	p := NewPumpCommand(0x1234567890ABCDEF, 1, 200)

	encoded, err := EncodePacket(p.Address(), p.Type(), p.PayloadMap())
	if err != nil {
		t.Fatalf("EncodePacket failed: %v", err)
	}

	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatalf("DecodePacket failed: %v", err)
	}

	if decoded.Type() != MsgPumpCommand {
		t.Errorf("decoded Type() = 0x%02X, want 0x%02X", decoded.Type(), MsgPumpCommand)
	}

	payload := decoded.PayloadMap()
	pump, ok := GetMapInt(payload, 0)
	if !ok {
		t.Error("decoded payload missing pump (key 0)")
	}
	if pump != 1 {
		t.Errorf("decoded pump = %d, want 1", pump)
	}

	rateMs, ok := GetMapInt(payload, 1)
	if !ok {
		t.Error("decoded payload missing rate_ms (key 1)")
	}
	if rateMs != 200 {
		t.Errorf("decoded rate_ms = %d, want 200", rateMs)
	}
}

func TestNewGlowCommand(t *testing.T) {
	p := NewGlowCommand(0x1234567890ABCDEF, 0, 5000)

	if p.Type() != MsgGlowCommand {
		t.Errorf("Type() = 0x%02X, want 0x%02X", p.Type(), MsgGlowCommand)
	}

	payload := p.PayloadMap()
	if payload == nil {
		t.Fatal("PayloadMap() returned nil")
	}

	glow, ok := GetMapInt(payload, 0)
	if !ok {
		t.Error("payload missing glow (key 0)")
	}
	if glow != 0 {
		t.Errorf("glow = %d, want 0", glow)
	}

	duration, ok := GetMapInt(payload, 1)
	if !ok {
		t.Error("payload missing duration (key 1)")
	}
	if duration != 5000 {
		t.Errorf("duration = %d, want 5000", duration)
	}
}

func TestNewGlowCommand_Extinguish(t *testing.T) {
	p := NewGlowCommand(0x1234567890ABCDEF, 0, 0)

	payload := p.PayloadMap()
	duration, _ := GetMapInt(payload, 1)
	if duration != 0 {
		t.Errorf("duration = %d, want 0 (extinguish)", duration)
	}
}

func TestNewGlowCommand_RoundTrip(t *testing.T) {
	p := NewGlowCommand(0x1234567890ABCDEF, 0, 10000)

	encoded, err := EncodePacket(p.Address(), p.Type(), p.PayloadMap())
	if err != nil {
		t.Fatalf("EncodePacket failed: %v", err)
	}

	decoded, err := DecodePacket(encoded)
	if err != nil {
		t.Fatalf("DecodePacket failed: %v", err)
	}

	if decoded.Type() != MsgGlowCommand {
		t.Errorf("decoded Type() = 0x%02X, want 0x%02X", decoded.Type(), MsgGlowCommand)
	}

	payload := decoded.PayloadMap()
	glow, _ := GetMapInt(payload, 0)
	duration, _ := GetMapInt(payload, 1)
	if glow != 0 || duration != 10000 {
		t.Errorf("decoded glow=%d duration=%d, want glow=0 duration=10000", glow, duration)
	}
}

// ptr is a helper to create pointer to value
func ptr[T any](v T) *T {
	return &v
}
