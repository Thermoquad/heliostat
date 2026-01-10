// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package fusain

import "fmt"

// AnomalyType represents different types of packet anomalies
type AnomalyType int

const (
	AnomalyInvalidCount AnomalyType = iota
	AnomalyLengthMismatch
	AnomalyHighRPM
	AnomalyInvalidTemp
	AnomalyInvalidPWM
	AnomalyInvalidValue
	AnomalyCRCError
	AnomalyDecodeError
)

// ValidationError represents a packet validation failure
type ValidationError struct {
	Type    AnomalyType
	Message string
	Details map[string]interface{}
}

// Error implements the error interface
func (v *ValidationError) Error() string {
	return v.Message
}

// ValidatePacket validates packet structure and detects anomalies
// Returns a slice of validation errors (empty if packet is valid)
func ValidatePacket(p *Packet) []ValidationError {
	errors := []ValidationError{}

	switch p.msgType {
	case MsgStateData:
		errors = append(errors, validateStateData(p)...)
	case MsgMotorData:
		errors = append(errors, validateMotorData(p)...)
	case MsgTempData:
		errors = append(errors, validateTemperatureData(p)...)
	case MsgGlowCommand:
		errors = append(errors, validateGlowCommand(p)...)
	case MsgDeviceAnnounce:
		errors = append(errors, validateDeviceAnnounce(p)...)
	}

	return errors
}

// validateStateData validates STATE_DATA packet
func validateStateData(p *Packet) []ValidationError {
	errors := []ValidationError{}

	if len(p.payload) < 16 {
		return []ValidationError{{
			Type:    AnomalyLengthMismatch,
			Message: "STATE_DATA payload too short (expected 16 bytes)",
			Details: map[string]interface{}{"length": len(p.payload), "expected": 16},
		}}
	}

	// Extract state value
	state := uint32(p.payload[8]) | uint32(p.payload[9])<<8 | uint32(p.payload[10])<<16 | uint32(p.payload[11])<<24
	if state > uint32(SysStateEstop) {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidValue,
			Message: fmt.Sprintf("Invalid state value=%d (max %d)", state, uint32(SysStateEstop)),
			Details: map[string]interface{}{"state": state, "max": uint32(SysStateEstop)},
		})
	}

	// Extract error code
	code := int32(uint32(p.payload[4]) | uint32(p.payload[5])<<8 | uint32(p.payload[6])<<16 | uint32(p.payload[7])<<24)
	if code < 0 || code > int32(ErrorCommandedStop) {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidValue,
			Message: fmt.Sprintf("Invalid error code=%d (valid 0-%d)", code, int32(ErrorCommandedStop)),
			Details: map[string]interface{}{"code": code, "max": int32(ErrorCommandedStop)},
		})
	}

	return errors
}

// validateMotorData validates MOTOR_DATA packet
func validateMotorData(p *Packet) []ValidationError {
	errors := []ValidationError{}

	if len(p.payload) < 32 {
		return []ValidationError{{
			Type:    AnomalyLengthMismatch,
			Message: "MOTOR_DATA payload too short (expected 32 bytes)",
			Details: map[string]interface{}{"length": len(p.payload), "expected": 32},
		}}
	}

	rpm := int32(uint32(p.payload[8]) | uint32(p.payload[9])<<8 |
		uint32(p.payload[10])<<16 | uint32(p.payload[11])<<24)
	target := int32(uint32(p.payload[12]) | uint32(p.payload[13])<<8 |
		uint32(p.payload[14])<<16 | uint32(p.payload[15])<<24)
	pwm := int32(uint32(p.payload[24]) | uint32(p.payload[25])<<8 |
		uint32(p.payload[26])<<16 | uint32(p.payload[27])<<24)
	pwmMax := int32(uint32(p.payload[28]) | uint32(p.payload[29])<<8 |
		uint32(p.payload[30])<<16 | uint32(p.payload[31])<<24)

	if rpm > 6000 || target > 6000 {
		errors = append(errors, ValidationError{
			Type:    AnomalyHighRPM,
			Message: fmt.Sprintf("High RPM (rpm=%d, target=%d, max 6000)", rpm, target),
			Details: map[string]interface{}{"rpm": rpm, "target_rpm": target, "max": 6000},
		})
	}

	if pwmMax > 0 && pwm > pwmMax {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidPWM,
			Message: fmt.Sprintf("PWM > pwmMax (%d > %d)", pwm, pwmMax),
			Details: map[string]interface{}{"pwm": pwm, "pwm_max": pwmMax},
		})
	}

	return errors
}

// validateTemperatureData validates TEMP_DATA packet
func validateTemperatureData(p *Packet) []ValidationError {
	errors := []ValidationError{}

	if len(p.payload) < 32 {
		return []ValidationError{{
			Type:    AnomalyLengthMismatch,
			Message: "TEMP_DATA payload too short (expected at least 32 bytes)",
			Details: map[string]interface{}{"length": len(p.payload), "minimum": 32},
		}}
	}

	// Current temperature
	tempBits := uint64(p.payload[8]) | uint64(p.payload[9])<<8 |
		uint64(p.payload[10])<<16 | uint64(p.payload[11])<<24 |
		uint64(p.payload[12])<<32 | uint64(p.payload[13])<<40 |
		uint64(p.payload[14])<<48 | uint64(p.payload[15])<<56
	temp := Float64frombits(tempBits)

	// Target temperature (if available)
	var targetTemp float64
	if len(p.payload) >= 32 {
		targetBits := uint64(p.payload[24]) | uint64(p.payload[25])<<8 |
			uint64(p.payload[26])<<16 | uint64(p.payload[27])<<24 |
			uint64(p.payload[28])<<32 | uint64(p.payload[29])<<40 |
			uint64(p.payload[30])<<48 | uint64(p.payload[31])<<56
		targetTemp = Float64frombits(targetBits)
	}

	if temp < -50.0 || temp > 1000.0 {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidTemp,
			Message: fmt.Sprintf("Temperature out of range (%.1f°C, valid: -50 to 1000°C)", temp),
			Details: map[string]interface{}{"value": temp, "min": -50.0, "max": 1000.0},
		})
	}

	if len(p.payload) >= 32 && (targetTemp < -50.0 || targetTemp > 1000.0) {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidTemp,
			Message: fmt.Sprintf("Target temperature out of range (%.1f°C, valid: -50 to 1000°C)", targetTemp),
			Details: map[string]interface{}{"value": targetTemp, "min": -50.0, "max": 1000.0},
		})
	}

	return errors
}

// validateGlowCommand validates GLOW_COMMAND packet
func validateGlowCommand(p *Packet) []ValidationError {
	errors := []ValidationError{}

	if len(p.payload) != 8 {
		return []ValidationError{{
			Type:    AnomalyLengthMismatch,
			Message: "GLOW_COMMAND payload length mismatch (expected 8 bytes)",
			Details: map[string]interface{}{"length": len(p.payload), "expected": 8},
		}}
	}

	// Extract duration (bytes 4-7, little-endian int32)
	duration := int32(uint32(p.payload[4]) | uint32(p.payload[5])<<8 |
		uint32(p.payload[6])<<16 | uint32(p.payload[7])<<24)

	// Validate duration (0-300000 ms)
	if duration < 0 || duration > 300000 {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidValue,
			Message: fmt.Sprintf("Invalid glow duration (%d ms, valid: 0-300000)", duration),
			Details: map[string]interface{}{"duration": duration, "min": 0, "max": 300000},
		})
	}

	return errors
}

// validateDeviceAnnounce validates DEVICE_ANNOUNCE packet
func validateDeviceAnnounce(p *Packet) []ValidationError {
	errors := []ValidationError{}

	// Per spec: 4 bytes for counts (u8 each) + 4 bytes padding = 8 bytes
	// But we only need the first 4 bytes for validation
	if len(p.payload) < 4 {
		return []ValidationError{{
			Type:    AnomalyLengthMismatch,
			Message: "DEVICE_ANNOUNCE payload too short (expected at least 4 bytes)",
			Details: map[string]interface{}{"length": len(p.payload), "minimum": 4},
		}}
	}

	// End-of-discovery marker uses stateless address with all zeros
	if p.IsStateless() {
		// This is an end-of-discovery marker - all counts should be 0
		return errors
	}

	// Per spec: counts are u8 (1 byte each)
	motorCount := p.payload[0]
	tempCount := p.payload[1]
	pumpCount := p.payload[2]
	glowCount := p.payload[3]

	if motorCount > 10 {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidCount,
			Message: fmt.Sprintf("Invalid motor_count=%d (max 10)", motorCount),
			Details: map[string]interface{}{"motor_count": motorCount, "max": 10},
		})
	}

	if tempCount > 10 {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidCount,
			Message: fmt.Sprintf("Invalid temp_count=%d (max 10)", tempCount),
			Details: map[string]interface{}{"temp_count": tempCount, "max": 10},
		})
	}

	if pumpCount > 10 {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidCount,
			Message: fmt.Sprintf("Invalid pump_count=%d (max 10)", pumpCount),
			Details: map[string]interface{}{"pump_count": pumpCount, "max": 10},
		})
	}

	if glowCount > 10 {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidCount,
			Message: fmt.Sprintf("Invalid glow_count=%d (max 10)", glowCount),
			Details: map[string]interface{}{"glow_count": glowCount, "max": 10},
		})
	}

	return errors
}
