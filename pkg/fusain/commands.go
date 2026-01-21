// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package fusain

// Command builder functions create Packet structs ready for encoding.
// These are convenience wrappers around NewPacketWithPayload that ensure
// correct payload key usage per the Fusain protocol specification.

// NewStateCommand creates a STATE_COMMAND packet (0x20).
// Mode values: ModeIdle (0), ModeFan (1), ModeHeat (2), ModeEmergency (255).
// The argument is optional and mode-specific:
//   - FAN mode: target RPM
//   - HEAT mode: pump rate in milliseconds
//   - IDLE/EMERGENCY: ignored (pass nil)
func NewStateCommand(address uint64, mode uint8, argument *int64) *Packet {
	payload := map[int]interface{}{
		0: uint64(mode),
	}
	if argument != nil {
		payload[1] = *argument
	}
	return NewPacketWithPayload(address, MsgStateCommand, payload)
}

// NewPingRequest creates a PING_REQUEST packet (0x2F).
// Appliances respond with PING_RESPONSE containing uptime.
func NewPingRequest(address uint64) *Packet {
	return NewPacketWithPayload(address, MsgPingRequest, nil)
}

// NewTelemetryConfig creates a TELEMETRY_CONFIG packet (0x16).
// When enabled is true and intervalMs > 0, the appliance sends periodic telemetry.
// When intervalMs is 0, polling mode is used (use SEND_TELEMETRY to request data).
func NewTelemetryConfig(address uint64, enabled bool, intervalMs uint32) *Packet {
	payload := map[int]interface{}{
		0: enabled,
		1: uint64(intervalMs),
	}
	return NewPacketWithPayload(address, MsgTelemetryConfig, payload)
}

// NewMotorCommand creates a MOTOR_COMMAND packet (0x21).
// Sets the target RPM for the specified motor.
// Use rpm=0 to stop the motor.
func NewMotorCommand(address uint64, motor uint8, rpm int32) *Packet {
	payload := map[int]interface{}{
		0: int64(motor),
		1: int64(rpm),
	}
	return NewPacketWithPayload(address, MsgMotorCommand, payload)
}

// NewPumpCommand creates a PUMP_COMMAND packet (0x22).
// Sets the pulse interval for the specified fuel pump.
// Use rateMs=0 to stop the pump.
func NewPumpCommand(address uint64, pump uint8, rateMs int32) *Packet {
	payload := map[int]interface{}{
		0: int64(pump),
		1: int64(rateMs),
	}
	return NewPacketWithPayload(address, MsgPumpCommand, payload)
}

// NewGlowCommand creates a GLOW_COMMAND packet (0x23).
// Controls the glow plug for ignition.
// Use durationMs=0 to extinguish the glow plug.
func NewGlowCommand(address uint64, glow uint8, durationMs int32) *Packet {
	payload := map[int]interface{}{
		0: int64(glow),
		1: int64(durationMs),
	}
	return NewPacketWithPayload(address, MsgGlowCommand, payload)
}

// NewDiscoveryRequest creates a DISCOVERY_REQUEST packet (0x1F).
// Routers respond with DEVICE_ANNOUNCE for each known device, followed by
// an end-of-discovery marker (DEVICE_ANNOUNCE with all zeros).
func NewDiscoveryRequest(address uint64) *Packet {
	return NewPacketWithPayload(address, MsgDiscoveryRequest, nil)
}

// NewDataSubscription creates a DATA_SUBSCRIPTION packet (0x14).
// Tells a router to forward telemetry from the specified appliance to this controller.
// The packet address should be the router's address (or broadcast/stateless).
// The applianceAddress is the device whose telemetry you want to receive.
func NewDataSubscription(routerAddress uint64, applianceAddress uint64) *Packet {
	payload := map[int]interface{}{
		0: applianceAddress,
	}
	return NewPacketWithPayload(routerAddress, MsgDataSubscription, payload)
}
