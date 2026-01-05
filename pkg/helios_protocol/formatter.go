// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package helios_protocol

import (
	"fmt"
	"strings"
	"unsafe"
)

// FormatPacket formats a packet into a human-readable string
func FormatPacket(p *Packet) string {
	timestamp := p.timestamp.Format("15:04:05.000")
	msgType := FormatMessageType(p.msgType)

	result := fmt.Sprintf("[%s] %s (0x%02X) len=%d\n", timestamp, msgType, p.msgType, p.length)

	if len(p.payload) > 0 {
		result += FormatPayload(p.msgType, p.payload)
	}

	return result
}

// FormatMessageType returns the human-readable name for a message type
func FormatMessageType(msgType uint8) string {
	switch msgType {
	// Commands
	case MSG_STATE_COMMAND:
		return "STATE_COMMAND"
	case MSG_MOTOR_COMMAND:
		return "MOTOR_COMMAND"
	case MSG_PUMP_COMMAND:
		return "PUMP_COMMAND"
	case MSG_GLOW_COMMAND:
		return "GLOW_COMMAND"
	case MSG_TEMP_COMMAND:
		return "TEMP_COMMAND"
	case MSG_TELEMETRY_CONFIG:
		return "TELEMETRY_CONFIG"
	case MSG_PING_REQUEST:
		return "PING_REQUEST"

	// Data
	case MSG_STATE_DATA:
		return "STATE_DATA"
	case MSG_MOTOR_DATA:
		return "MOTOR_DATA"
	case MSG_TEMPERATURE_DATA:
		return "TEMPERATURE_DATA"
	case MSG_PUMP_DATA:
		return "PUMP_DATA"
	case MSG_GLOW_DATA:
		return "GLOW_DATA"
	case MSG_TELEMETRY_BUNDLE:
		return "TELEMETRY_BUNDLE"
	case MSG_PING_RESPONSE:
		return "PING_RESPONSE"

	// Errors
	case MSG_ERROR_INVALID_COMMAND:
		return "ERROR_INVALID_COMMAND"
	case MSG_ERROR_INVALID_CRC:
		return "ERROR_INVALID_CRC"
	case MSG_ERROR_INVALID_LENGTH:
		return "ERROR_INVALID_LENGTH"
	case MSG_ERROR_TIMEOUT:
		return "ERROR_TIMEOUT"

	default:
		return "UNKNOWN"
	}
}

// FormatPayload formats the payload based on message type
func FormatPayload(msgType uint8, payload []byte) string {
	switch msgType {
	case MSG_PING_REQUEST:
		return "  (no payload)\n"

	case MSG_PING_RESPONSE:
		if len(payload) >= 8 {
			uptime := uint64(payload[0]) | uint64(payload[1])<<8 | uint64(payload[2])<<16 | uint64(payload[3])<<24 |
				uint64(payload[4])<<32 | uint64(payload[5])<<40 | uint64(payload[6])<<48 | uint64(payload[7])<<56
			return fmt.Sprintf("  Uptime: %s\n", formatDuration(uptime))
		}

	case MSG_STATE_COMMAND:
		if len(payload) >= 5 {
			mode := payload[0]
			param := uint32(payload[1]) | uint32(payload[2])<<8 | uint32(payload[3])<<16 | uint32(payload[4])<<24
			modeName := []string{"IDLE", "FAN", "HEAT", "EMERGENCY"}
			modeStr := "UNKNOWN"
			if mode < 3 {
				modeStr = modeName[mode]
			} else if mode == 0xFF {
				modeStr = "EMERGENCY"
			}
			return fmt.Sprintf("  Mode: %s (0x%02X), Parameter: %d\n", modeStr, mode, param)
		}

	case MSG_STATE_DATA:
		if len(payload) >= 2 {
			state := payload[0]
			errorCode := payload[1]
			stateNames := []string{"INITIALIZING", "IDLE", "BLOWING", "PREHEAT", "PREHEAT_STAGE_2", "HEATING", "COOLING", "ERROR", "E_STOP"}
			stateName := "UNKNOWN"
			if int(state) < len(stateNames) {
				stateName = stateNames[state]
			}
			return fmt.Sprintf("  State: %s (0x%02X), Error: 0x%02X\n", stateName, state, errorCode)
		}

	case MSG_TELEMETRY_BUNDLE:
		if len(payload) >= 7 {
			state := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			errorCode := payload[4]
			motorCount := payload[5]
			tempCount := payload[6]

			stateNames := []string{"INITIALIZING", "IDLE", "BLOWING", "PREHEAT", "PREHEAT_STAGE_2", "HEATING", "COOLING", "ERROR", "E_STOP"}
			stateName := "UNKNOWN"
			if int(state) < len(stateNames) {
				stateName = stateNames[state]
			}

			result := fmt.Sprintf("  State: %s (0x%02X), Error: 0x%02X, Motors: %d, Temps: %d\n", stateName, state, errorCode, motorCount, tempCount)

			offset := 7

			// Parse motor data (16 bytes each: rpm, target, pwm_duty, pwm_period)
			for i := 0; i < int(motorCount) && offset+15 <= len(payload); i++ {
				rpm := int32(uint32(payload[offset]) | uint32(payload[offset+1])<<8 | uint32(payload[offset+2])<<16 | uint32(payload[offset+3])<<24)
				targetRPM := int32(uint32(payload[offset+4]) | uint32(payload[offset+5])<<8 | uint32(payload[offset+6])<<16 | uint32(payload[offset+7])<<24)
				pwmDuty := int32(uint32(payload[offset+8]) | uint32(payload[offset+9])<<8 | uint32(payload[offset+10])<<16 | uint32(payload[offset+11])<<24)
				pwmPeriod := int32(uint32(payload[offset+12]) | uint32(payload[offset+13])<<8 | uint32(payload[offset+14])<<16 | uint32(payload[offset+15])<<24)

				// Calculate PWM percentage
				var pwmPercent float64
				if pwmPeriod > 0 {
					pwmPercent = (float64(pwmDuty) / float64(pwmPeriod)) * 100.0
				}

				result += fmt.Sprintf("    Motor %d: RPM=%d (target=%d), Power=%.1f%% (%d/%d ns)\n",
					i, rpm, targetRPM, pwmPercent, pwmDuty, pwmPeriod)
				offset += 16
			}

			// Parse temperature data (8 bytes each: f64)
			for i := 0; i < int(tempCount) && offset+7 <= len(payload); i++ {
				tempBits := uint64(payload[offset]) | uint64(payload[offset+1])<<8 | uint64(payload[offset+2])<<16 | uint64(payload[offset+3])<<24 |
					uint64(payload[offset+4])<<32 | uint64(payload[offset+5])<<40 | uint64(payload[offset+6])<<48 | uint64(payload[offset+7])<<56
				temp := Float64frombits(tempBits)
				result += fmt.Sprintf("    Temp %d: %.1f°C\n", i, temp)
				offset += 8
			}

			return result
		}

	case MSG_PUMP_COMMAND:
		if len(payload) >= 4 {
			rate := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			return fmt.Sprintf("  Rate: %d ms\n", rate)
		}

	case MSG_GLOW_COMMAND:
		if len(payload) >= 8 {
			glow := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			duration := int32(uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24)
			return fmt.Sprintf("  Glow: %d, Duration: %d ms\n", glow, duration)
		}

	case MSG_MOTOR_COMMAND:
		if len(payload) >= 4 {
			rpm := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			return fmt.Sprintf("  Target RPM: %d\n", rpm)
		}

	case MSG_TELEMETRY_CONFIG:
		if len(payload) >= 12 {
			enabled := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			interval := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24
			mode := uint32(payload[8]) | uint32(payload[9])<<8 | uint32(payload[10])<<16 | uint32(payload[11])<<24

			enabledStr := "Disabled"
			if enabled != 0 {
				enabledStr = "Enabled"
			}

			modeStr := "Bundled"
			if mode != 0 {
				modeStr = "Individual"
			}

			return fmt.Sprintf("  Telemetry: %s, Interval: %d ms, Mode: %s\n", enabledStr, interval, modeStr)
		}

	case MSG_MOTOR_DATA:
		if len(payload) >= 32 {
			motor := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			timestamp := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24
			rpm := int32(uint32(payload[8]) | uint32(payload[9])<<8 | uint32(payload[10])<<16 | uint32(payload[11])<<24)
			target := int32(uint32(payload[12]) | uint32(payload[13])<<8 | uint32(payload[14])<<16 | uint32(payload[15])<<24)
			maxRPM := int32(uint32(payload[16]) | uint32(payload[17])<<8 | uint32(payload[18])<<16 | uint32(payload[19])<<24)
			minRPM := int32(uint32(payload[20]) | uint32(payload[21])<<8 | uint32(payload[22])<<16 | uint32(payload[23])<<24)
			pwm := int32(uint32(payload[24]) | uint32(payload[25])<<8 | uint32(payload[26])<<16 | uint32(payload[27])<<24)
			pwmMax := int32(uint32(payload[28]) | uint32(payload[29])<<8 | uint32(payload[30])<<16 | uint32(payload[31])<<24)

			return fmt.Sprintf("  Motor %d: RPM=%d (target=%d), Range=[%d-%d], PWM=%dns/%dns, Time=%d µs\n",
				motor, rpm, target, minRPM, maxRPM, pwm, pwmMax, timestamp)
		}

	case MSG_TEMPERATURE_DATA:
		if len(payload) >= 32 {
			thermometer := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			timestamp := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24

			// Temperature is f64 (8 bytes, little-endian)
			tempBits := uint64(payload[8]) | uint64(payload[9])<<8 | uint64(payload[10])<<16 | uint64(payload[11])<<24 |
				uint64(payload[12])<<32 | uint64(payload[13])<<40 | uint64(payload[14])<<48 | uint64(payload[15])<<56
			temp := Float64frombits(tempBits)

			pidEnabled := payload[16]
			rpmCtrlEnabled := payload[17]
			watchedMotor := int32(uint32(payload[18]) | uint32(payload[19])<<8 | uint32(payload[20])<<16 | uint32(payload[21])<<24)

			// Target temp is f64 (8 bytes)
			targetBits := uint64(payload[22]) | uint64(payload[23])<<8 | uint64(payload[24])<<16 | uint64(payload[25])<<24 |
				uint64(payload[26])<<32 | uint64(payload[27])<<40 | uint64(payload[28])<<48 | uint64(payload[29])<<56
			targetTemp := Float64frombits(targetBits)

			pidStr := "Off"
			if pidEnabled != 0 {
				pidStr = "On"
			}

			rpmStr := "Off"
			if rpmCtrlEnabled != 0 {
				rpmStr = "On"
			}

			return fmt.Sprintf("  Thermometer %d: %.1f°C (target=%.1f°C), PID=%s, RPM_Ctrl=%s, Motor=%d, Time=%d µs\n",
				thermometer, temp, targetTemp, pidStr, rpmStr, watchedMotor, timestamp)
		}

	case MSG_ERROR_INVALID_CRC:
		if len(payload) >= 4 {
			received := uint16(payload[0]) | uint16(payload[1])<<8
			calculated := uint16(payload[2]) | uint16(payload[3])<<8
			return fmt.Sprintf("  Received CRC: 0x%04X, Calculated CRC: 0x%04X\n", received, calculated)
		}

	case MSG_ERROR_INVALID_COMMAND:
		if len(payload) >= 1 {
			return fmt.Sprintf("  Invalid Command: 0x%02X\n", payload[0])
		}

	case MSG_ERROR_INVALID_LENGTH:
		if len(payload) >= 2 {
			return fmt.Sprintf("  Received Length: %d, Expected: %d\n", payload[0], payload[1])
		}
	}

	// Default: hex dump
	result := "  Payload: "
	for i, b := range payload {
		if i > 0 && i%16 == 0 {
			result += "\n           "
		}
		result += fmt.Sprintf("%02X ", b)
	}
	return result + "\n"
}

// formatDuration converts milliseconds to human-readable duration
func formatDuration(ms uint64) string {
	seconds := ms / 1000
	if seconds == 0 {
		return "0 seconds"
	}

	const (
		secondsPerMinute = 60
		secondsPerHour   = 60 * secondsPerMinute
		secondsPerDay    = 24 * secondsPerHour
		secondsPerYear   = 365 * secondsPerDay
	)

	years := seconds / secondsPerYear
	seconds %= secondsPerYear

	days := seconds / secondsPerDay
	seconds %= secondsPerDay

	hours := seconds / secondsPerHour
	seconds %= secondsPerHour

	minutes := seconds / secondsPerMinute
	seconds %= secondsPerMinute

	parts := []string{}

	if years > 0 {
		if years == 1 {
			parts = append(parts, "1 year")
		} else {
			parts = append(parts, fmt.Sprintf("%d years", years))
		}
	}

	if days > 0 {
		if days == 1 {
			parts = append(parts, "1 day")
		} else {
			parts = append(parts, fmt.Sprintf("%d days", days))
		}
	}

	if hours > 0 {
		if hours == 1 {
			parts = append(parts, "1 hour")
		} else {
			parts = append(parts, fmt.Sprintf("%d hours", hours))
		}
	}

	if minutes > 0 {
		if minutes == 1 {
			parts = append(parts, "1 minute")
		} else {
			parts = append(parts, fmt.Sprintf("%d minutes", minutes))
		}
	}

	if seconds > 0 {
		if seconds == 1 {
			parts = append(parts, "1 second")
		} else {
			parts = append(parts, fmt.Sprintf("%d seconds", seconds))
		}
	}

	// Join parts with commas and "and"
	if len(parts) == 0 {
		return "0 seconds"
	} else if len(parts) == 1 {
		return parts[0]
	} else if len(parts) == 2 {
		return parts[0] + " and " + parts[1]
	} else {
		last := parts[len(parts)-1]
		rest := parts[:len(parts)-1]
		return strings.Join(rest, ", ") + ", and " + last
	}
}

// Float64frombits converts a uint64 to float64
func Float64frombits(b uint64) float64 {
	return *(*float64)(unsafe.Pointer(&b))
}
