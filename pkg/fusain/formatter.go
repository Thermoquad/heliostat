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
	msgType := FormatMessageType(p.Type())

	result := fmt.Sprintf("[%s] %s (0x%02X) addr=%016X len=%d\n", timestamp, msgType, p.Type(), p.address, p.length)

	payloadMap := p.PayloadMap()
	if payloadMap != nil || p.Type() == MsgPingRequest || p.Type() == MsgDiscoveryRequest {
		result += FormatPayloadMap(p.Type(), payloadMap)
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

// FormatPayloadMap formats the CBOR payload map based on message type
func FormatPayloadMap(msgType uint8, m map[int]interface{}) string {
	switch msgType {
	case MsgPingRequest, MsgDiscoveryRequest:
		return "  (no payload)\n"

	case MsgPingResponse:
		// 0 => uptime-ms
		uptime, _ := GetMapUint(m, 0)
		return fmt.Sprintf("  Uptime: %s\n", formatDuration(uptime))

	case MsgStateCommand:
		// 0 => mode, 1 => argument (optional)
		mode, _ := GetMapUint(m, 0)
		arg, hasArg := GetMapInt(m, 1)
		modeStr := formatMode(uint32(mode))
		if hasArg {
			return fmt.Sprintf("  Mode: %s (%d), Argument: %d\n", modeStr, mode, arg)
		}
		return fmt.Sprintf("  Mode: %s (%d)\n", modeStr, mode)

	case MsgStateData:
		// 0 => error (bool), 1 => code, 2 => state, 3 => timestamp
		errorFlag, _ := GetMapBool(m, 0)
		code, _ := GetMapInt(m, 1)
		state, _ := GetMapUint(m, 2)
		timestamp, _ := GetMapUint(m, 3)
		stateName := formatState(uint32(state))
		errorStr := "No"
		if errorFlag {
			errorStr = "Yes"
		}
		return fmt.Sprintf("  State: %s (%d), Error: %s, Code: %s (%d), Time: %d ms\n",
			stateName, state, errorStr, formatErrorCode(int32(code)), code, timestamp)

	case MsgMotorCommand:
		// 0 => motor, 1 => rpm
		motor, _ := GetMapUint(m, 0)
		rpm, _ := GetMapInt(m, 1)
		return fmt.Sprintf("  Motor: %d, Target RPM: %d\n", motor, rpm)

	case MsgPumpCommand:
		// 0 => pump, 1 => rate-ms
		pump, _ := GetMapUint(m, 0)
		rate, _ := GetMapInt(m, 1)
		return fmt.Sprintf("  Pump: %d, Rate: %d ms\n", pump, rate)

	case MsgGlowCommand:
		// 0 => glow, 1 => duration
		glow, _ := GetMapUint(m, 0)
		duration, _ := GetMapInt(m, 1)
		return fmt.Sprintf("  Glow: %d, Duration: %d ms\n", glow, duration)

	case MsgTempCommand:
		// 0 => thermometer, 1 => type, 2 => motor-index (opt), 3 => target-temp (opt)
		therm, _ := GetMapUint(m, 0)
		cmdType, _ := GetMapUint(m, 1)
		motorIdx, hasMotor := GetMapInt(m, 2)
		target, hasTarget := GetMapFloat(m, 3)
		cmdStr := formatTempCommandType(uint32(cmdType))
		result := fmt.Sprintf("  Thermometer: %d, Type: %s (%d)", therm, cmdStr, cmdType)
		if hasMotor {
			result += fmt.Sprintf(", Motor: %d", motorIdx)
		}
		if hasTarget {
			result += fmt.Sprintf(", Target: %.1f°C", target)
		}
		return result + "\n"

	case MsgTelemetryConfig:
		// 0 => enabled (bool), 1 => interval-ms
		enabled, _ := GetMapBool(m, 0)
		interval, _ := GetMapUint(m, 1)
		enabledStr := "Disabled"
		if enabled {
			enabledStr = "Enabled"
		}
		modeStr := "Broadcast"
		if interval == 0 {
			modeStr = "Polling"
		}
		return fmt.Sprintf("  Telemetry: %s, Interval: %d ms, Mode: %s\n", enabledStr, interval, modeStr)

	case MsgTimeoutConfig:
		// 0 => enabled (bool), 1 => timeout-ms
		enabled, _ := GetMapBool(m, 0)
		timeout, _ := GetMapUint(m, 1)
		enabledStr := "Disabled"
		if enabled {
			enabledStr = "Enabled"
		}
		return fmt.Sprintf("  Timeout: %s, Interval: %d ms\n", enabledStr, timeout)

	case MsgSendTelemetry:
		// 0 => telemetry-type, 1 => index (optional)
		telType, _ := GetMapUint(m, 0)
		idx, hasIdx := GetMapUint(m, 1)
		typeStr := formatTelemetryType(uint32(telType))
		idxStr := "ALL"
		if hasIdx && idx != 0xFFFFFFFF {
			idxStr = fmt.Sprintf("%d", idx)
		}
		return fmt.Sprintf("  Telemetry Type: %s (%d), Index: %s\n", typeStr, telType, idxStr)

	case MsgMotorData:
		// 0 => motor, 1 => timestamp, 2 => rpm, 3 => target
		// 4 => max-rpm (opt), 5 => min-rpm (opt), 6 => pwm (opt), 7 => pwm-max (opt)
		motor, _ := GetMapUint(m, 0)
		timestamp, _ := GetMapUint(m, 1)
		rpm, _ := GetMapInt(m, 2)
		target, _ := GetMapInt(m, 3)
		maxRPM, hasMax := GetMapInt(m, 4)
		minRPM, hasMin := GetMapInt(m, 5)
		pwm, hasPWM := GetMapUint(m, 6)
		pwmMax, hasPWMMax := GetMapUint(m, 7)

		result := fmt.Sprintf("  Motor %d: RPM=%d (target=%d)", motor, rpm, target)
		if hasMin && hasMax {
			result += fmt.Sprintf(", Range=[%d-%d]", minRPM, maxRPM)
		}
		if hasPWM && hasPWMMax {
			result += fmt.Sprintf(", PWM=%d/%d µs", pwm, pwmMax)
		}
		result += fmt.Sprintf(", Time=%d ms\n", timestamp)
		return result

	case MsgPumpData:
		// 0 => pump, 1 => timestamp, 2 => type (event), 3 => rate (opt)
		pump, _ := GetMapUint(m, 0)
		timestamp, _ := GetMapUint(m, 1)
		eventType, _ := GetMapUint(m, 2)
		rate, hasRate := GetMapInt(m, 3)
		eventStr := formatPumpEvent(uint32(eventType))
		if hasRate {
			return fmt.Sprintf("  Pump %d: Event=%s (%d), Rate=%d ms, Time=%d ms\n",
				pump, eventStr, eventType, rate, timestamp)
		}
		return fmt.Sprintf("  Pump %d: Event=%s (%d), Time=%d ms\n",
			pump, eventStr, eventType, timestamp)

	case MsgGlowData:
		// 0 => glow, 1 => timestamp, 2 => lit (bool)
		glow, _ := GetMapUint(m, 0)
		timestamp, _ := GetMapUint(m, 1)
		lit, _ := GetMapBool(m, 2)
		litStr := "Off"
		if lit {
			litStr = "On"
		}
		return fmt.Sprintf("  Glow %d: Status=%s, Time=%d ms\n", glow, litStr, timestamp)

	case MsgTempData:
		// 0 => thermometer, 1 => timestamp, 2 => reading (float)
		// 3 => temperature-rpm-control (opt, bool), 4 => watched-motor (opt), 5 => target-temperature (opt)
		therm, _ := GetMapUint(m, 0)
		timestamp, _ := GetMapUint(m, 1)
		temp, _ := GetMapFloat(m, 2)
		rpmCtrl, hasRpmCtrl := GetMapBool(m, 3)
		watchedMotor, hasMotor := GetMapInt(m, 4)
		targetTemp, hasTarget := GetMapFloat(m, 5)

		result := fmt.Sprintf("  Thermometer %d: %.1f°C", therm, temp)
		if hasTarget {
			result += fmt.Sprintf(" (target=%.1f°C)", targetTemp)
		}
		if hasRpmCtrl {
			rpmStr := "Off"
			if rpmCtrl {
				rpmStr = "On"
			}
			result += fmt.Sprintf(", RPM_Ctrl=%s", rpmStr)
		}
		if hasMotor {
			result += fmt.Sprintf(", Motor=%d", watchedMotor)
		}
		result += fmt.Sprintf(", Time=%d ms\n", timestamp)
		return result

	case MsgDeviceAnnounce:
		// 0 => motor-count, 1 => thermometer-count, 2 => pump-count, 3 => glow-count
		motorCount, _ := GetMapUint(m, 0)
		tempCount, _ := GetMapUint(m, 1)
		pumpCount, _ := GetMapUint(m, 2)
		glowCount, _ := GetMapUint(m, 3)
		return fmt.Sprintf("  Motors: %d, Temperatures: %d, Pumps: %d, Glow Plugs: %d\n",
			motorCount, tempCount, pumpCount, glowCount)

	case MsgErrorInvalidCmd:
		// 0 => error-code
		code, _ := GetMapInt(m, 0)
		codeStr := "Unknown"
		switch code {
		case 1:
			codeStr = "Invalid parameter value"
		case 2:
			codeStr = "Invalid device index"
		}
		return fmt.Sprintf("  Error Code: %d (%s)\n", code, codeStr)

	case MsgErrorStateReject:
		// 0 => error-code (state that rejected)
		state, _ := GetMapUint(m, 0)
		stateName := formatState(uint32(state))
		return fmt.Sprintf("  Rejected by state: %s (%d)\n", stateName, state)

	case MsgDataSubscription, MsgDataUnsubscribe:
		// 0 => appliance-address
		addr, _ := GetMapUint(m, 0)
		return fmt.Sprintf("  Appliance Address: 0x%016X\n", addr)

	case MsgMotorConfig:
		// 0 => motor, 1 => pwm-period (opt), 2-4 => PID (opt), 5-6 => RPM limits (opt), 7 => min-pwm (opt)
		motor, _ := GetMapUint(m, 0)
		result := fmt.Sprintf("  Motor %d:", motor)

		if pwm, ok := GetMapUint(m, 1); ok {
			result += fmt.Sprintf(" PWM=%d ns", pwm)
		}
		if kp, ok := GetMapFloat(m, 2); ok {
			ki, _ := GetMapFloat(m, 3)
			kd, _ := GetMapFloat(m, 4)
			result += fmt.Sprintf(", PID=[%.2f,%.2f,%.2f]", kp, ki, kd)
		}
		if maxRPM, ok := GetMapInt(m, 5); ok {
			minRPM, _ := GetMapInt(m, 6)
			result += fmt.Sprintf(", RPM=[%d-%d]", minRPM, maxRPM)
		}
		if minPWM, ok := GetMapUint(m, 7); ok {
			result += fmt.Sprintf(", MinPWM=%d ns", minPWM)
		}
		return result + "\n"

	case MsgPumpConfig:
		// 0 => pump, 1 => pulse-ms (opt), 2 => recovery-ms (opt)
		pump, _ := GetMapUint(m, 0)
		result := fmt.Sprintf("  Pump %d:", pump)
		if pulse, ok := GetMapUint(m, 1); ok {
			result += fmt.Sprintf(" Pulse=%d ms", pulse)
		}
		if recovery, ok := GetMapUint(m, 2); ok {
			result += fmt.Sprintf(", Recovery=%d ms", recovery)
		}
		return result + "\n"

	case MsgTempConfig:
		// 0 => thermometer, 1-3 => PID (opt)
		therm, _ := GetMapUint(m, 0)
		result := fmt.Sprintf("  Thermometer %d:", therm)
		if kp, ok := GetMapFloat(m, 1); ok {
			ki, _ := GetMapFloat(m, 2)
			kd, _ := GetMapFloat(m, 3)
			result += fmt.Sprintf(" PID=[%.2f,%.2f,%.2f]", kp, ki, kd)
		}
		return result + "\n"

	case MsgGlowConfig:
		// 0 => glow, 1 => max-duration (opt)
		glow, _ := GetMapUint(m, 0)
		result := fmt.Sprintf("  Glow %d:", glow)
		if maxDur, ok := GetMapUint(m, 1); ok {
			result += fmt.Sprintf(" MaxDuration=%d ms", maxDur)
		}
		return result + "\n"
	}

	// Default: show map contents
	if m == nil {
		return "  (nil payload)\n"
	}
	result := "  Payload: {"
	for k, v := range m {
		result += fmt.Sprintf("%d: %v, ", k, v)
	}
	return result + "}\n"
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
