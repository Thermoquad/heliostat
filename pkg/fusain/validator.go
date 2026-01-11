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

	// Check for CBOR parse errors first
	if err := p.ParseError(); err != nil {
		return []ValidationError{{
			Type:    AnomalyDecodeError,
			Message: fmt.Sprintf("CBOR parse error: %v", err),
			Details: map[string]interface{}{"error": err.Error()},
		}}
	}

	msgType := p.Type()
	payloadMap := p.PayloadMap()

	switch msgType {
	case MsgStateData:
		errors = append(errors, validateStateData(payloadMap)...)
	case MsgMotorData:
		errors = append(errors, validateMotorData(payloadMap)...)
	case MsgTempData:
		errors = append(errors, validateTemperatureData(payloadMap)...)
	case MsgGlowCommand:
		errors = append(errors, validateGlowCommand(payloadMap)...)
	case MsgDeviceAnnounce:
		errors = append(errors, validateDeviceAnnounce(payloadMap, p.IsStateless())...)
	}

	return errors
}

// validateStateData validates STATE_DATA payload
// CBOR keys: 0=error(bool), 1=code, 2=state, 3=timestamp
func validateStateData(m map[int]interface{}) []ValidationError {
	errors := []ValidationError{}

	if m == nil {
		return []ValidationError{{
			Type:    AnomalyLengthMismatch,
			Message: "STATE_DATA missing payload",
			Details: map[string]interface{}{},
		}}
	}

	// Validate state value (key 2)
	state, ok := GetMapUint(m, 2)
	if ok && state > uint64(SysStateEstop) {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidValue,
			Message: fmt.Sprintf("Invalid state value=%d (max %d)", state, uint64(SysStateEstop)),
			Details: map[string]interface{}{"state": state, "max": uint64(SysStateEstop)},
		})
	}

	// Validate error code (key 1)
	code, ok := GetMapInt(m, 1)
	if ok && (code < 0 || code > int64(ErrorCommandedStop)) {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidValue,
			Message: fmt.Sprintf("Invalid error code=%d (valid 0-%d)", code, int64(ErrorCommandedStop)),
			Details: map[string]interface{}{"code": code, "max": int64(ErrorCommandedStop)},
		})
	}

	return errors
}

// validateMotorData validates MOTOR_DATA payload
// CBOR keys: 0=motor, 1=timestamp, 2=rpm, 3=target, 4=max-rpm, 5=min-rpm, 6=pwm, 7=pwm-max
func validateMotorData(m map[int]interface{}) []ValidationError {
	errors := []ValidationError{}

	if m == nil {
		return []ValidationError{{
			Type:    AnomalyLengthMismatch,
			Message: "MOTOR_DATA missing payload",
			Details: map[string]interface{}{},
		}}
	}

	rpm, _ := GetMapInt(m, 2)
	target, _ := GetMapInt(m, 3)
	pwm, hasPWM := GetMapUint(m, 6)
	pwmMax, hasPWMMax := GetMapUint(m, 7)

	if rpm > 6000 || target > 6000 {
		errors = append(errors, ValidationError{
			Type:    AnomalyHighRPM,
			Message: fmt.Sprintf("High RPM (rpm=%d, target=%d, max 6000)", rpm, target),
			Details: map[string]interface{}{"rpm": rpm, "target_rpm": target, "max": 6000},
		})
	}

	if hasPWM && hasPWMMax && pwmMax > 0 && pwm > pwmMax {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidPWM,
			Message: fmt.Sprintf("PWM > pwmMax (%d > %d)", pwm, pwmMax),
			Details: map[string]interface{}{"pwm": pwm, "pwm_max": pwmMax},
		})
	}

	return errors
}

// validateTemperatureData validates TEMP_DATA payload
// CBOR keys: 0=thermometer, 1=timestamp, 2=reading, 3=temperature-rpm-control, 4=watched-motor, 5=target-temperature
func validateTemperatureData(m map[int]interface{}) []ValidationError {
	errors := []ValidationError{}

	if m == nil {
		return []ValidationError{{
			Type:    AnomalyLengthMismatch,
			Message: "TEMP_DATA missing payload",
			Details: map[string]interface{}{},
		}}
	}

	// Current temperature (key 2)
	temp, hasTemp := GetMapFloat(m, 2)
	if hasTemp && (temp < -50.0 || temp > 1000.0) {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidTemp,
			Message: fmt.Sprintf("Temperature out of range (%.1f°C, valid: -50 to 1000°C)", temp),
			Details: map[string]interface{}{"value": temp, "min": -50.0, "max": 1000.0},
		})
	}

	// Target temperature (key 5, optional)
	targetTemp, hasTarget := GetMapFloat(m, 5)
	if hasTarget && (targetTemp < -50.0 || targetTemp > 1000.0) {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidTemp,
			Message: fmt.Sprintf("Target temperature out of range (%.1f°C, valid: -50 to 1000°C)", targetTemp),
			Details: map[string]interface{}{"value": targetTemp, "min": -50.0, "max": 1000.0},
		})
	}

	return errors
}

// validateGlowCommand validates GLOW_COMMAND payload
// CBOR keys: 0=glow, 1=duration
func validateGlowCommand(m map[int]interface{}) []ValidationError {
	errors := []ValidationError{}

	if m == nil {
		return []ValidationError{{
			Type:    AnomalyLengthMismatch,
			Message: "GLOW_COMMAND missing payload",
			Details: map[string]interface{}{},
		}}
	}

	// Duration (key 1)
	duration, ok := GetMapInt(m, 1)
	if ok && (duration < 0 || duration > 300000) {
		errors = append(errors, ValidationError{
			Type:    AnomalyInvalidValue,
			Message: fmt.Sprintf("Invalid glow duration (%d ms, valid: 0-300000)", duration),
			Details: map[string]interface{}{"duration": duration, "min": 0, "max": 300000},
		})
	}

	return errors
}

// validateDeviceAnnounce validates DEVICE_ANNOUNCE payload
// CBOR keys: 0=motor-count, 1=thermometer-count, 2=pump-count, 3=glow-count
func validateDeviceAnnounce(m map[int]interface{}, isStateless bool) []ValidationError {
	errors := []ValidationError{}

	// End-of-discovery marker uses stateless address with all zeros
	if isStateless {
		return errors
	}

	if m == nil {
		return []ValidationError{{
			Type:    AnomalyLengthMismatch,
			Message: "DEVICE_ANNOUNCE missing payload",
			Details: map[string]interface{}{},
		}}
	}

	motorCount, _ := GetMapUint(m, 0)
	tempCount, _ := GetMapUint(m, 1)
	pumpCount, _ := GetMapUint(m, 2)
	glowCount, _ := GetMapUint(m, 3)

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
