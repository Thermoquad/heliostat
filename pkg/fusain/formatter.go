// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package fusain

import (
	"fmt"
	"strings"
	"unsafe"
)

// FormatPacket formats a packet into a human-readable string
func FormatPacket(p *Packet) string {
	timestamp := p.timestamp.Format("15:04:05.000")
	msgType := FormatMessageType(p.msgType)

	result := fmt.Sprintf("[%s] %s (0x%02X) addr=%016X len=%d\n", timestamp, msgType, p.msgType, p.address, p.length)

	if len(p.payload) > 0 {
		result += FormatPayload(p.msgType, p.payload)
	}

	return result
}

// FormatMessageType returns the human-readable name for a message type
func FormatMessageType(msgType uint8) string {
	switch msgType {
	// Configuration Commands (0x10-0x1F)
	case MsgMotorConfig:
		return "MOTOR_CONFIG"
	case MsgPumpConfig:
		return "PUMP_CONFIG"
	case MsgTempConfig:
		return "TEMP_CONFIG"
	case MsgGlowConfig:
		return "GLOW_CONFIG"
	case MsgDataSubscription:
		return "DATA_SUBSCRIPTION"
	case MsgDataUnsubscribe:
		return "DATA_UNSUBSCRIBE"
	case MsgTelemetryConfig:
		return "TELEMETRY_CONFIG"
	case MsgTimeoutConfig:
		return "TIMEOUT_CONFIG"
	case MsgDiscoveryRequest:
		return "DISCOVERY_REQUEST"

	// Control Commands (0x20-0x2F)
	case MsgStateCommand:
		return "STATE_COMMAND"
	case MsgMotorCommand:
		return "MOTOR_COMMAND"
	case MsgPumpCommand:
		return "PUMP_COMMAND"
	case MsgGlowCommand:
		return "GLOW_COMMAND"
	case MsgTempCommand:
		return "TEMP_COMMAND"
	case MsgSendTelemetry:
		return "SEND_TELEMETRY"
	case MsgPingRequest:
		return "PING_REQUEST"

	// Telemetry Data (0x30-0x3F)
	case MsgStateData:
		return "STATE_DATA"
	case MsgMotorData:
		return "MOTOR_DATA"
	case MsgPumpData:
		return "PUMP_DATA"
	case MsgGlowData:
		return "GLOW_DATA"
	case MsgTempData:
		return "TEMP_DATA"
	case MsgDeviceAnnounce:
		return "DEVICE_ANNOUNCE"
	case MsgPingResponse:
		return "PING_RESPONSE"

	// Errors (0xE0-0xEF)
	case MsgErrorInvalidCmd:
		return "ERROR_INVALID_CMD"
	case MsgErrorStateReject:
		return "ERROR_STATE_REJECT"

	default:
		return "UNKNOWN"
	}
}

// FormatPayload formats the payload based on message type
func FormatPayload(msgType uint8, payload []byte) string {
	switch msgType {
	case MsgPingRequest, MsgDiscoveryRequest:
		return "  (no payload)\n"

	case MsgPingResponse:
		if len(payload) >= 4 {
			uptime := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			return fmt.Sprintf("  Uptime: %s\n", formatDuration(uint64(uptime)))
		}

	case MsgStateCommand:
		if len(payload) >= 8 {
			mode := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			arg := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24
			modeStr := formatMode(mode)
			return fmt.Sprintf("  Mode: %s (%d), Argument: %d\n", modeStr, mode, arg)
		}

	case MsgStateData:
		if len(payload) >= 16 {
			errorFlag := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			code := int32(uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24)
			state := uint32(payload[8]) | uint32(payload[9])<<8 | uint32(payload[10])<<16 | uint32(payload[11])<<24
			timestamp := uint32(payload[12]) | uint32(payload[13])<<8 | uint32(payload[14])<<16 | uint32(payload[15])<<24
			stateName := formatState(state)
			errorStr := "No"
			if errorFlag != 0 {
				errorStr = "Yes"
			}
			return fmt.Sprintf("  State: %s (%d), Error: %s, Code: %s (%d), Time: %d ms\n",
				stateName, state, errorStr, formatErrorCode(code), code, timestamp)
		}

	case MsgMotorCommand:
		if len(payload) >= 8 {
			motor := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			rpm := int32(uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24)
			return fmt.Sprintf("  Motor: %d, Target RPM: %d\n", motor, rpm)
		}

	case MsgPumpCommand:
		if len(payload) >= 8 {
			pump := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			rate := int32(uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24)
			return fmt.Sprintf("  Pump: %d, Rate: %d ms\n", pump, rate)
		}

	case MsgGlowCommand:
		if len(payload) >= 8 {
			glow := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			duration := int32(uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24)
			return fmt.Sprintf("  Glow: %d, Duration: %d ms\n", glow, duration)
		}

	case MsgTempCommand:
		if len(payload) >= 20 {
			therm := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			cmdType := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24
			motorIdx := int32(uint32(payload[8]) | uint32(payload[9])<<8 | uint32(payload[10])<<16 | uint32(payload[11])<<24)
			targetBits := uint64(payload[12]) | uint64(payload[13])<<8 | uint64(payload[14])<<16 | uint64(payload[15])<<24 |
				uint64(payload[16])<<32 | uint64(payload[17])<<40 | uint64(payload[18])<<48 | uint64(payload[19])<<56
			target := Float64frombits(targetBits)
			cmdStr := formatTempCommandType(cmdType)
			return fmt.Sprintf("  Thermometer: %d, Type: %s (%d), Motor: %d, Target: %.1f°C\n",
				therm, cmdStr, cmdType, motorIdx, target)
		}

	case MsgTelemetryConfig:
		if len(payload) >= 8 {
			enabled := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			interval := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24
			enabledStr := "Disabled"
			if enabled != 0 {
				enabledStr = "Enabled"
			}
			modeStr := "Broadcast"
			if interval == 0 {
				modeStr = "Polling"
			}
			return fmt.Sprintf("  Telemetry: %s, Interval: %d ms, Mode: %s\n", enabledStr, interval, modeStr)
		}

	case MsgTimeoutConfig:
		if len(payload) >= 8 {
			enabled := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			timeout := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24
			enabledStr := "Disabled"
			if enabled != 0 {
				enabledStr = "Enabled"
			}
			return fmt.Sprintf("  Timeout: %s, Interval: %d ms\n", enabledStr, timeout)
		}

	case MsgSendTelemetry:
		if len(payload) >= 8 {
			telType := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			idx := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24
			typeStr := formatTelemetryType(telType)
			idxStr := fmt.Sprintf("%d", idx)
			if idx == 0xFFFFFFFF {
				idxStr = "ALL"
			}
			return fmt.Sprintf("  Telemetry Type: %s (%d), Index: %s\n", typeStr, telType, idxStr)
		}

	case MsgMotorData:
		if len(payload) >= 32 {
			motor := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			timestamp := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24
			rpm := int32(uint32(payload[8]) | uint32(payload[9])<<8 | uint32(payload[10])<<16 | uint32(payload[11])<<24)
			target := int32(uint32(payload[12]) | uint32(payload[13])<<8 | uint32(payload[14])<<16 | uint32(payload[15])<<24)
			maxRPM := int32(uint32(payload[16]) | uint32(payload[17])<<8 | uint32(payload[18])<<16 | uint32(payload[19])<<24)
			minRPM := int32(uint32(payload[20]) | uint32(payload[21])<<8 | uint32(payload[22])<<16 | uint32(payload[23])<<24)
			pwm := int32(uint32(payload[24]) | uint32(payload[25])<<8 | uint32(payload[26])<<16 | uint32(payload[27])<<24)
			pwmMax := int32(uint32(payload[28]) | uint32(payload[29])<<8 | uint32(payload[30])<<16 | uint32(payload[31])<<24)
			return fmt.Sprintf("  Motor %d: RPM=%d (target=%d), Range=[%d-%d], PWM=%d/%d µs, Time=%d ms\n",
				motor, rpm, target, minRPM, maxRPM, pwm, pwmMax, timestamp)
		}

	case MsgPumpData:
		if len(payload) >= 16 {
			pump := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			timestamp := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24
			eventType := uint32(payload[8]) | uint32(payload[9])<<8 | uint32(payload[10])<<16 | uint32(payload[11])<<24
			rate := int32(uint32(payload[12]) | uint32(payload[13])<<8 | uint32(payload[14])<<16 | uint32(payload[15])<<24)
			eventStr := formatPumpEvent(eventType)
			return fmt.Sprintf("  Pump %d: Event=%s (%d), Rate=%d ms, Time=%d ms\n",
				pump, eventStr, eventType, rate, timestamp)
		}

	case MsgGlowData:
		if len(payload) >= 12 {
			glow := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			timestamp := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24
			lit := uint32(payload[8]) | uint32(payload[9])<<8 | uint32(payload[10])<<16 | uint32(payload[11])<<24
			litStr := "Off"
			if lit != 0 {
				litStr = "On"
			}
			return fmt.Sprintf("  Glow %d: Status=%s, Time=%d ms\n", glow, litStr, timestamp)
		}

	case MsgTempData:
		if len(payload) >= 32 {
			therm := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			timestamp := uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24
			tempBits := uint64(payload[8]) | uint64(payload[9])<<8 | uint64(payload[10])<<16 | uint64(payload[11])<<24 |
				uint64(payload[12])<<32 | uint64(payload[13])<<40 | uint64(payload[14])<<48 | uint64(payload[15])<<56
			temp := Float64frombits(tempBits)
			rpmCtrl := uint32(payload[16]) | uint32(payload[17])<<8 | uint32(payload[18])<<16 | uint32(payload[19])<<24
			watchedMotor := int32(uint32(payload[20]) | uint32(payload[21])<<8 | uint32(payload[22])<<16 | uint32(payload[23])<<24)
			targetBits := uint64(payload[24]) | uint64(payload[25])<<8 | uint64(payload[26])<<16 | uint64(payload[27])<<24 |
				uint64(payload[28])<<32 | uint64(payload[29])<<40 | uint64(payload[30])<<48 | uint64(payload[31])<<56
			targetTemp := Float64frombits(targetBits)
			rpmStr := "Off"
			if rpmCtrl != 0 {
				rpmStr = "On"
			}
			return fmt.Sprintf("  Thermometer %d: %.1f°C (target=%.1f°C), RPM_Ctrl=%s, Motor=%d, Time=%d ms\n",
				therm, temp, targetTemp, rpmStr, watchedMotor, timestamp)
		}

	case MsgDeviceAnnounce:
		if len(payload) >= 4 {
			// Per spec: counts are u8 (1 byte each), followed by 4 bytes padding
			motorCount := payload[0]
			tempCount := payload[1]
			pumpCount := payload[2]
			glowCount := payload[3]
			return fmt.Sprintf("  Motors: %d, Temperatures: %d, Pumps: %d, Glow Plugs: %d\n",
				motorCount, tempCount, pumpCount, glowCount)
		}

	case MsgErrorInvalidCmd:
		if len(payload) >= 4 {
			code := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			codeStr := "Unknown"
			switch code {
			case 1:
				codeStr = "Invalid parameter value"
			case 2:
				codeStr = "Invalid device index"
			}
			return fmt.Sprintf("  Error Code: %d (%s)\n", code, codeStr)
		}

	case MsgErrorStateReject:
		if len(payload) >= 4 {
			state := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			stateName := formatState(state)
			return fmt.Sprintf("  Rejected by state: %s (%d)\n", stateName, state)
		}

	case MsgDataSubscription, MsgDataUnsubscribe:
		if len(payload) >= 8 {
			addr := uint64(payload[0]) | uint64(payload[1])<<8 | uint64(payload[2])<<16 | uint64(payload[3])<<24 |
				uint64(payload[4])<<32 | uint64(payload[5])<<40 | uint64(payload[6])<<48 | uint64(payload[7])<<56
			return fmt.Sprintf("  Appliance Address: 0x%016X\n", addr)
		}

	case MsgMotorConfig:
		if len(payload) >= 44 {
			motor := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			pwmPeriod := int32(uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24)
			kpBits := uint64(payload[8]) | uint64(payload[9])<<8 | uint64(payload[10])<<16 | uint64(payload[11])<<24 |
				uint64(payload[12])<<32 | uint64(payload[13])<<40 | uint64(payload[14])<<48 | uint64(payload[15])<<56
			kp := Float64frombits(kpBits)
			kiBits := uint64(payload[16]) | uint64(payload[17])<<8 | uint64(payload[18])<<16 | uint64(payload[19])<<24 |
				uint64(payload[20])<<32 | uint64(payload[21])<<40 | uint64(payload[22])<<48 | uint64(payload[23])<<56
			ki := Float64frombits(kiBits)
			kdBits := uint64(payload[24]) | uint64(payload[25])<<8 | uint64(payload[26])<<16 | uint64(payload[27])<<24 |
				uint64(payload[28])<<32 | uint64(payload[29])<<40 | uint64(payload[30])<<48 | uint64(payload[31])<<56
			kd := Float64frombits(kdBits)
			maxRPM := int32(uint32(payload[32]) | uint32(payload[33])<<8 | uint32(payload[34])<<16 | uint32(payload[35])<<24)
			minRPM := int32(uint32(payload[36]) | uint32(payload[37])<<8 | uint32(payload[38])<<16 | uint32(payload[39])<<24)
			minPWM := int32(uint32(payload[40]) | uint32(payload[41])<<8 | uint32(payload[42])<<16 | uint32(payload[43])<<24)
			return fmt.Sprintf("  Motor %d: PWM=%d µs, PID=[%.2f,%.2f,%.2f], RPM=[%d-%d], MinPWM=%d µs\n",
				motor, pwmPeriod, kp, ki, kd, minRPM, maxRPM, minPWM)
		}

	case MsgPumpConfig:
		if len(payload) >= 12 {
			pump := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			pulse := int32(uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24)
			recovery := int32(uint32(payload[8]) | uint32(payload[9])<<8 | uint32(payload[10])<<16 | uint32(payload[11])<<24)
			return fmt.Sprintf("  Pump %d: Pulse=%d ms, Recovery=%d ms\n", pump, pulse, recovery)
		}

	case MsgTempConfig:
		if len(payload) >= 36 {
			therm := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			kpBits := uint64(payload[4]) | uint64(payload[5])<<8 | uint64(payload[6])<<16 | uint64(payload[7])<<24 |
				uint64(payload[8])<<32 | uint64(payload[9])<<40 | uint64(payload[10])<<48 | uint64(payload[11])<<56
			kp := Float64frombits(kpBits)
			kiBits := uint64(payload[12]) | uint64(payload[13])<<8 | uint64(payload[14])<<16 | uint64(payload[15])<<24 |
				uint64(payload[16])<<32 | uint64(payload[17])<<40 | uint64(payload[18])<<48 | uint64(payload[19])<<56
			ki := Float64frombits(kiBits)
			kdBits := uint64(payload[20]) | uint64(payload[21])<<8 | uint64(payload[22])<<16 | uint64(payload[23])<<24 |
				uint64(payload[24])<<32 | uint64(payload[25])<<40 | uint64(payload[26])<<48 | uint64(payload[27])<<56
			kd := Float64frombits(kdBits)
			sampleCount := int32(uint32(payload[28]) | uint32(payload[29])<<8 | uint32(payload[30])<<16 | uint32(payload[31])<<24)
			readRate := int32(uint32(payload[32]) | uint32(payload[33])<<8 | uint32(payload[34])<<16 | uint32(payload[35])<<24)
			return fmt.Sprintf("  Thermometer %d: PID=[%.2f,%.2f,%.2f], Samples=%d, ReadRate=%d ms\n",
				therm, kp, ki, kd, sampleCount, readRate)
		}

	case MsgGlowConfig:
		if len(payload) >= 8 {
			glow := int32(uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24)
			maxDur := int32(uint32(payload[4]) | uint32(payload[5])<<8 | uint32(payload[6])<<16 | uint32(payload[7])<<24)
			return fmt.Sprintf("  Glow %d: MaxDuration=%d ms\n", glow, maxDur)
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

// formatState returns a human-readable state name
func formatState(state uint32) string {
	names := []string{"INITIALIZING", "IDLE", "BLOWING", "PREHEAT", "PREHEAT_STAGE_2", "HEATING", "COOLING", "ERROR", "E_STOP"}
	if int(state) < len(names) {
		return names[state]
	}
	return "UNKNOWN"
}

// formatMode returns a human-readable mode name
func formatMode(mode uint32) string {
	switch mode {
	case uint32(ModeIdle):
		return "IDLE"
	case uint32(ModeFan):
		return "FAN"
	case uint32(ModeHeat):
		return "HEAT"
	case uint32(ModeEmergency):
		return "EMERGENCY"
	default:
		return "UNKNOWN"
	}
}

// formatErrorCode returns a human-readable error code name
func formatErrorCode(code int32) string {
	names := []string{"NONE", "OVERHEAT", "SENSOR_FAULT", "IGNITION_FAIL", "FLAME_OUT", "MOTOR_STALL", "PUMP_FAULT", "COMMANDED_ESTOP"}
	if int(code) < len(names) && code >= 0 {
		return names[code]
	}
	return "UNKNOWN"
}

// formatTempCommandType returns a human-readable temperature command type
func formatTempCommandType(cmdType uint32) string {
	switch cmdType {
	case uint32(TempCmdWatchMotor):
		return "WATCH_MOTOR"
	case uint32(TempCmdUnwatchMotor):
		return "UNWATCH_MOTOR"
	case uint32(TempCmdEnableRpmControl):
		return "ENABLE_RPM_CONTROL"
	case uint32(TempCmdDisableRpmControl):
		return "DISABLE_RPM_CONTROL"
	case uint32(TempCmdSetTargetTemp):
		return "SET_TARGET_TEMP"
	default:
		return "UNKNOWN"
	}
}

// formatTelemetryType returns a human-readable telemetry type
func formatTelemetryType(telType uint32) string {
	switch telType {
	case uint32(TelemetryTypeState):
		return "STATE"
	case uint32(TelemetryTypeMotor):
		return "MOTOR"
	case uint32(TelemetryTypeTemp):
		return "TEMPERATURE"
	case uint32(TelemetryTypePump):
		return "PUMP"
	case uint32(TelemetryTypeGlow):
		return "GLOW"
	default:
		return "UNKNOWN"
	}
}

// formatPumpEvent returns a human-readable pump event type
func formatPumpEvent(eventType uint32) string {
	switch eventType {
	case uint32(PumpEventInitializing):
		return "INITIALIZING"
	case uint32(PumpEventReady):
		return "READY"
	case uint32(PumpEventError):
		return "ERROR"
	case uint32(PumpEventCycleStart):
		return "CYCLE_START"
	case uint32(PumpEventPulseEnd):
		return "PULSE_END"
	case uint32(PumpEventCycleEnd):
		return "CYCLE_END"
	default:
		return "UNKNOWN"
	}
}

// formatDuration converts milliseconds to human-readable duration
func formatDuration(ms uint64) string {
	seconds := ms / 1000
	if seconds == 0 {
		return fmt.Sprintf("%d ms", ms)
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
	// Note: len(parts) >= 1 is guaranteed since seconds >= 1 when we reach here
	if len(parts) == 1 {
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
