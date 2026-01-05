// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package helios_protocol

import "fmt"

// AnomalyType represents different types of packet anomalies
type AnomalyType int

const (
	ANOMALY_INVALID_COUNT AnomalyType = iota
	ANOMALY_LENGTH_MISMATCH
	ANOMALY_HIGH_RPM
	ANOMALY_INVALID_TEMP
	ANOMALY_INVALID_PWM
	ANOMALY_INVALID_VALUE
	ANOMALY_CRC_ERROR
	ANOMALY_DECODE_ERROR
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
	case MSG_TELEMETRY_BUNDLE:
		errors = append(errors, validateTelemetryBundle(p)...)
	case MSG_MOTOR_DATA:
		errors = append(errors, validateMotorData(p)...)
	case MSG_TEMPERATURE_DATA:
		errors = append(errors, validateTemperatureData(p)...)
	case MSG_GLOW_COMMAND:
		errors = append(errors, validateGlowCommand(p)...)
	}

	return errors
}

// validateTelemetryBundle validates telemetry bundle packet
func validateTelemetryBundle(p *Packet) []ValidationError {
	errors := []ValidationError{}

	if len(p.payload) < 7 {
		return []ValidationError{{
			Type:    ANOMALY_LENGTH_MISMATCH,
			Message: "Telemetry bundle payload too short (minimum 7 bytes)",
			Details: map[string]interface{}{"length": len(p.payload), "minimum": 7},
		}}
	}

	// Extract counts
	motorCount := p.payload[5]
	tempCount := p.payload[6]

	// Validate counts (max 10 each as per Slate validation)
	if motorCount > 10 {
		errors = append(errors, ValidationError{
			Type:    ANOMALY_INVALID_COUNT,
			Message: fmt.Sprintf("Invalid motor_count=%d (max 10)", motorCount),
			Details: map[string]interface{}{"motor_count": motorCount, "max": 10},
		})
	}

	if tempCount > 10 {
		errors = append(errors, ValidationError{
			Type:    ANOMALY_INVALID_COUNT,
			Message: fmt.Sprintf("Invalid temp_count=%d (max 10)", tempCount),
			Details: map[string]interface{}{"temp_count": tempCount, "max": 10},
		})
	}

	// Validate payload length matches expected size
	expectedSize := 7 + (int(motorCount) * 16) + (int(tempCount) * 8)
	if len(p.payload) != expectedSize {
		errors = append(errors, ValidationError{
			Type: ANOMALY_LENGTH_MISMATCH,
			Message: fmt.Sprintf("Payload length mismatch: received=%d, expected=%d (motors=%d, temps=%d)",
				len(p.payload), expectedSize, motorCount, tempCount),
			Details: map[string]interface{}{
				"received":    len(p.payload),
				"expected":    expectedSize,
				"motor_count": motorCount,
				"temp_count":  tempCount,
			},
		})
	}

	// Only validate motor/temp values if counts and length are valid
	if motorCount <= 10 && tempCount <= 10 && len(p.payload) == expectedSize {
		offset := 7

		// Validate motor data
		for i := 0; i < int(motorCount) && offset+15 <= len(p.payload); i++ {
			rpm := int32(uint32(p.payload[offset]) | uint32(p.payload[offset+1])<<8 |
				uint32(p.payload[offset+2])<<16 | uint32(p.payload[offset+3])<<24)
			targetRPM := int32(uint32(p.payload[offset+4]) | uint32(p.payload[offset+5])<<8 |
				uint32(p.payload[offset+6])<<16 | uint32(p.payload[offset+7])<<24)
			pwmDuty := int32(uint32(p.payload[offset+8]) | uint32(p.payload[offset+9])<<8 |
				uint32(p.payload[offset+10])<<16 | uint32(p.payload[offset+11])<<24)
			pwmPeriod := int32(uint32(p.payload[offset+12]) | uint32(p.payload[offset+13])<<8 |
				uint32(p.payload[offset+14])<<16 | uint32(p.payload[offset+15])<<24)

			// Check RPM values
			if rpm > 6000 || targetRPM > 6000 {
				errors = append(errors, ValidationError{
					Type:    ANOMALY_HIGH_RPM,
					Message: fmt.Sprintf("Motor %d: High RPM detected (rpm=%d, target=%d, max 6000)", i, rpm, targetRPM),
					Details: map[string]interface{}{"motor": i, "rpm": rpm, "target_rpm": targetRPM, "max": 6000},
				})
			}

			// Check PWM validity
			if pwmPeriod > 0 && pwmDuty > pwmPeriod {
				errors = append(errors, ValidationError{
					Type:    ANOMALY_INVALID_PWM,
					Message: fmt.Sprintf("Motor %d: PWM duty > period (%d > %d)", i, pwmDuty, pwmPeriod),
					Details: map[string]interface{}{"motor": i, "pwm_duty": pwmDuty, "pwm_period": pwmPeriod},
				})
			}

			offset += 16
		}

		// Validate temperature data
		for i := 0; i < int(tempCount) && offset+7 <= len(p.payload); i++ {
			tempBits := uint64(p.payload[offset]) | uint64(p.payload[offset+1])<<8 |
				uint64(p.payload[offset+2])<<16 | uint64(p.payload[offset+3])<<24 |
				uint64(p.payload[offset+4])<<32 | uint64(p.payload[offset+5])<<40 |
				uint64(p.payload[offset+6])<<48 | uint64(p.payload[offset+7])<<56
			temp := Float64frombits(tempBits)

			// Check temperature range (-50°C to 1000°C)
			if temp < -50.0 || temp > 1000.0 {
				errors = append(errors, ValidationError{
					Type:    ANOMALY_INVALID_TEMP,
					Message: fmt.Sprintf("Temp %d: Out of range (%.1f°C, valid: -50 to 1000°C)", i, temp),
					Details: map[string]interface{}{"temp": i, "value": temp, "min": -50.0, "max": 1000.0},
				})
			}

			offset += 8
		}
	}

	return errors
}

// validateMotorData validates individual motor data packet
func validateMotorData(p *Packet) []ValidationError {
	errors := []ValidationError{}

	if len(p.payload) < 32 {
		return []ValidationError{{
			Type:    ANOMALY_LENGTH_MISMATCH,
			Message: "Motor data payload too short (expected 32 bytes)",
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
			Type:    ANOMALY_HIGH_RPM,
			Message: fmt.Sprintf("High RPM (rpm=%d, target=%d, max 6000)", rpm, target),
			Details: map[string]interface{}{"rpm": rpm, "target_rpm": target, "max": 6000},
		})
	}

	if pwmMax > 0 && pwm > pwmMax {
		errors = append(errors, ValidationError{
			Type:    ANOMALY_INVALID_PWM,
			Message: fmt.Sprintf("PWM > pwmMax (%d > %d)", pwm, pwmMax),
			Details: map[string]interface{}{"pwm": pwm, "pwm_max": pwmMax},
		})
	}

	return errors
}

// validateTemperatureData validates temperature data packet
func validateTemperatureData(p *Packet) []ValidationError {
	errors := []ValidationError{}

	if len(p.payload) < 32 {
		return []ValidationError{{
			Type:    ANOMALY_LENGTH_MISMATCH,
			Message: "Temperature data payload too short (expected 32 bytes)",
			Details: map[string]interface{}{"length": len(p.payload), "expected": 32},
		}}
	}

	// Current temperature
	tempBits := uint64(p.payload[8]) | uint64(p.payload[9])<<8 |
		uint64(p.payload[10])<<16 | uint64(p.payload[11])<<24 |
		uint64(p.payload[12])<<32 | uint64(p.payload[13])<<40 |
		uint64(p.payload[14])<<48 | uint64(p.payload[15])<<56
	temp := Float64frombits(tempBits)

	// Target temperature
	targetBits := uint64(p.payload[22]) | uint64(p.payload[23])<<8 |
		uint64(p.payload[24])<<16 | uint64(p.payload[25])<<24 |
		uint64(p.payload[26])<<32 | uint64(p.payload[27])<<40 |
		uint64(p.payload[28])<<48 | uint64(p.payload[29])<<56
	targetTemp := Float64frombits(targetBits)

	if temp < -50.0 || temp > 1000.0 {
		errors = append(errors, ValidationError{
			Type:    ANOMALY_INVALID_TEMP,
			Message: fmt.Sprintf("Temperature out of range (%.1f°C, valid: -50 to 1000°C)", temp),
			Details: map[string]interface{}{"value": temp, "min": -50.0, "max": 1000.0},
		})
	}

	if targetTemp < -50.0 || targetTemp > 1000.0 {
		errors = append(errors, ValidationError{
			Type:    ANOMALY_INVALID_TEMP,
			Message: fmt.Sprintf("Target temperature out of range (%.1f°C, valid: -50 to 1000°C)", targetTemp),
			Details: map[string]interface{}{"value": targetTemp, "min": -50.0, "max": 1000.0},
		})
	}

	return errors
}

// validateGlowCommand validates glow command packet
func validateGlowCommand(p *Packet) []ValidationError {
	errors := []ValidationError{}

	if len(p.payload) != 8 {
		return []ValidationError{{
			Type:    ANOMALY_LENGTH_MISMATCH,
			Message: "Glow command payload length mismatch (expected 8 bytes)",
			Details: map[string]interface{}{"length": len(p.payload), "expected": 8},
		}}
	}

	// Extract duration (bytes 4-7, little-endian int32)
	duration := int32(uint32(p.payload[4]) | uint32(p.payload[5])<<8 |
		uint32(p.payload[6])<<16 | uint32(p.payload[7])<<24)

	// Validate duration (0-300000 ms)
	if duration < 0 || duration > 300000 {
		errors = append(errors, ValidationError{
			Type:    ANOMALY_INVALID_VALUE,
			Message: fmt.Sprintf("Invalid glow duration (%d ms, valid: 0-300000)", duration),
			Details: map[string]interface{}{"duration": duration, "min": 0, "max": 300000},
		})
	}

	return errors
}
