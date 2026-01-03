// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad
//
// Heliostat - Helios Serial Protocol Analyzer
//
// A CLI tool for monitoring and decoding Helios serial protocol packets
// in human-readable format.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"unsafe"

	"go.bug.st/serial"
)

const (
	// Protocol Constants
	START_BYTE = 0x7E
	END_BYTE   = 0x7F
	ESC_BYTE   = 0x7D
	ESC_XOR    = 0x20

	MAX_PACKET_SIZE  = 64
	MAX_PAYLOAD_SIZE = 58

	// CRC-16-CCITT
	CRC_POLYNOMIAL = 0x1021
	CRC_INITIAL    = 0xFFFF
)

// Message Types
const (
	// Commands (Master → ICU)
	MSG_SET_MODE           = 0x10
	MSG_SET_PUMP_RATE      = 0x11
	MSG_SET_TARGET_RPM     = 0x12
	MSG_PING_REQUEST       = 0x13
	MSG_SET_TIMEOUT_CONFIG = 0x14
	MSG_EMERGENCY_STOP     = 0x15
	MSG_TELEMETRY_CONFIG   = 0x16

	// Data (ICU → Master)
	MSG_STATE_DATA       = 0x20
	MSG_MOTOR_DATA       = 0x21
	MSG_TEMPERATURE_DATA = 0x22
	MSG_PUMP_DATA        = 0x23
	MSG_GLOW_DATA        = 0x24
	MSG_TELEMETRY_BUNDLE = 0x25
	MSG_PING_RESPONSE    = 0x26

	// Errors
	MSG_ERROR_INVALID_COMMAND = 0xE0
	MSG_ERROR_INVALID_CRC     = 0xE1
	MSG_ERROR_INVALID_LENGTH  = 0xE2
	MSG_ERROR_TIMEOUT         = 0xE3
)

// Decoder States
const (
	STATE_IDLE = iota
	STATE_LENGTH
	STATE_TYPE
	STATE_PAYLOAD
	STATE_CRC1
	STATE_CRC2
	STATE_END
)

type Packet struct {
	Length    uint8
	Type      uint8
	Payload   []byte
	CRC       uint16
	Timestamp time.Time
}

type Decoder struct {
	state       int
	buffer      []byte
	bufferIndex int
	escapeNext  bool
	packet      *Packet
	rawBuffer   []byte // Accumulate raw bytes including framing
}

func NewDecoder() *Decoder {
	return &Decoder{
		state:     STATE_IDLE,
		buffer:    make([]byte, MAX_PACKET_SIZE),
		rawBuffer: make([]byte, 0, MAX_PACKET_SIZE*2),
	}
}

func (d *Decoder) Reset() {
	d.state = STATE_IDLE
	d.bufferIndex = 0
	d.escapeNext = false
	d.packet = nil
	d.rawBuffer = d.rawBuffer[:0]
}

func (d *Decoder) DecodeByte(b byte) (*Packet, error) {
	// Always accumulate raw bytes for verification
	d.rawBuffer = append(d.rawBuffer, b)

	// Handle byte stuffing
	if b == ESC_BYTE && !d.escapeNext {
		d.escapeNext = true
		return nil, nil
	}

	originalB := b
	if d.escapeNext {
		b ^= ESC_XOR
		d.escapeNext = false
	}

	// Handle framing bytes
	if originalB == START_BYTE && !d.escapeNext {
		d.Reset()
		d.rawBuffer = append(d.rawBuffer[:0], originalB)
		d.state = STATE_LENGTH
		return nil, nil
	}

	if originalB == END_BYTE && !d.escapeNext {
		if d.state == STATE_CRC2 {
			// Packet complete - validate CRC
			packet := d.packet
			calculatedCRC := calculateCRC(d.buffer[:d.bufferIndex])

			if packet.CRC != calculatedCRC {
				err := fmt.Errorf("CRC mismatch: expected 0x%04X, got 0x%04X", calculatedCRC, packet.CRC)
				d.Reset()
				return nil, err
			}

			packet.Timestamp = time.Now()

			d.Reset()
			return packet, nil
		}
		d.Reset()
		return nil, fmt.Errorf("unexpected END byte in state %d", d.state)
	}

	// State machine
	switch d.state {
	case STATE_IDLE:
		// Waiting for START byte
		return nil, nil

	case STATE_LENGTH:
		if b > MAX_PAYLOAD_SIZE {
			d.Reset()
			return nil, fmt.Errorf("invalid length: %d", b)
		}
		// Protocol v1.3: Check for buffer overflow
		if d.bufferIndex >= MAX_PACKET_SIZE {
			d.Reset()
			return nil, fmt.Errorf("buffer overflow at length byte")
		}
		d.packet = &Packet{Length: b, Payload: make([]byte, 0, b)}
		d.buffer[d.bufferIndex] = b
		d.bufferIndex++
		d.state = STATE_TYPE
		return nil, nil

	case STATE_TYPE:
		// Protocol v1.3: Check for buffer overflow
		if d.bufferIndex >= MAX_PACKET_SIZE {
			d.Reset()
			return nil, fmt.Errorf("buffer overflow at type byte")
		}
		d.packet.Type = b
		d.buffer[d.bufferIndex] = b
		d.bufferIndex++
		if d.packet.Length == 0 {
			d.state = STATE_CRC1
		} else {
			d.state = STATE_PAYLOAD
		}
		return nil, nil

	case STATE_PAYLOAD:
		// Protocol v1.3: Check for buffer overflow before accepting byte
		if d.bufferIndex >= MAX_PACKET_SIZE {
			d.Reset()
			return nil, fmt.Errorf("buffer overflow: packet exceeds max size")
		}
		d.packet.Payload = append(d.packet.Payload, b)
		d.buffer[d.bufferIndex] = b
		d.bufferIndex++
		if len(d.packet.Payload) >= int(d.packet.Length) {
			d.state = STATE_CRC1
		}
		return nil, nil

	case STATE_CRC1:
		d.packet.CRC = uint16(b) << 8
		d.state = STATE_CRC2
		return nil, nil

	case STATE_CRC2:
		d.packet.CRC |= uint16(b)
		// Wait for END byte
		return nil, nil

	default:
		d.Reset()
		return nil, fmt.Errorf("invalid state: %d", d.state)
	}
}

func calculateCRC(data []byte) uint16 {
	crc := uint16(CRC_INITIAL)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ CRC_POLYNOMIAL
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func formatPacket(p *Packet) string {
	timestamp := p.Timestamp.Format("15:04:05.000")
	msgType := formatMessageType(p.Type)

	result := fmt.Sprintf("[%s] %s (0x%02X) len=%d\n", timestamp, msgType, p.Type, p.Length)

	if len(p.Payload) > 0 {
		result += formatPayload(p.Type, p.Payload)
	}

	return result
}

func formatMessageType(msgType uint8) string {
	switch msgType {
	// Commands
	case MSG_SET_MODE:
		return "SET_MODE"
	case MSG_SET_PUMP_RATE:
		return "SET_PUMP_RATE"
	case MSG_SET_TARGET_RPM:
		return "SET_TARGET_RPM"
	case MSG_PING_REQUEST:
		return "PING_REQUEST"
	case MSG_SET_TIMEOUT_CONFIG:
		return "SET_TIMEOUT_CONFIG"
	case MSG_EMERGENCY_STOP:
		return "EMERGENCY_STOP"
	case MSG_TELEMETRY_CONFIG:
		return "TELEMETRY_CONFIG"

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

func formatPayload(msgType uint8, payload []byte) string {
	switch msgType {
	case MSG_PING_REQUEST:
		return "  (no payload)\n"

	case MSG_PING_RESPONSE:
		if len(payload) >= 8 {
			uptime := uint64(payload[0]) | uint64(payload[1])<<8 | uint64(payload[2])<<16 | uint64(payload[3])<<24 |
				uint64(payload[4])<<32 | uint64(payload[5])<<40 | uint64(payload[6])<<48 | uint64(payload[7])<<56
			return fmt.Sprintf("  Uptime: %s\n", formatDuration(uptime))
		}

	case MSG_SET_MODE:
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

			// Parse motor data (12 bytes each: rpm, target, pwm_duty)
			for i := 0; i < int(motorCount) && offset+11 <= len(payload); i++ {
				rpm := int32(uint32(payload[offset]) | uint32(payload[offset+1])<<8 | uint32(payload[offset+2])<<16 | uint32(payload[offset+3])<<24)
				targetRPM := int32(uint32(payload[offset+4]) | uint32(payload[offset+5])<<8 | uint32(payload[offset+6])<<16 | uint32(payload[offset+7])<<24)
				pwmDuty := int32(uint32(payload[offset+8]) | uint32(payload[offset+9])<<8 | uint32(payload[offset+10])<<16 | uint32(payload[offset+11])<<24)
				result += fmt.Sprintf("    Motor %d: RPM=%d (target=%d), PWM=%d ns\n", i, rpm, targetRPM, pwmDuty)
				offset += 12
			}

			// Parse temperature data (8 bytes each: f64)
			for i := 0; i < int(tempCount) && offset+7 <= len(payload); i++ {
				tempBits := uint64(payload[offset]) | uint64(payload[offset+1])<<8 | uint64(payload[offset+2])<<16 | uint64(payload[offset+3])<<24 |
					uint64(payload[offset+4])<<32 | uint64(payload[offset+5])<<40 | uint64(payload[offset+6])<<48 | uint64(payload[offset+7])<<56
				temp := float64frombits(tempBits)
				result += fmt.Sprintf("    Temp %d: %.1f°C\n", i, temp)
				offset += 8
			}

			return result
		}

	case MSG_SET_PUMP_RATE:
		if len(payload) >= 4 {
			rate := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
			return fmt.Sprintf("  Rate: %d ms\n", rate)
		}

	case MSG_SET_TARGET_RPM:
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
			temp := float64frombits(tempBits)

			pidEnabled := payload[16]
			rpmCtrlEnabled := payload[17]
			watchedMotor := int32(uint32(payload[18]) | uint32(payload[19])<<8 | uint32(payload[20])<<16 | uint32(payload[21])<<24)

			// Target temp is f64 (8 bytes)
			targetBits := uint64(payload[22]) | uint64(payload[23])<<8 | uint64(payload[24])<<16 | uint64(payload[25])<<24 |
				uint64(payload[26])<<32 | uint64(payload[27])<<40 | uint64(payload[28])<<48 | uint64(payload[29])<<56
			targetTemp := float64frombits(targetBits)

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

// float32frombits converts a uint32 to float32
func float32frombits(b uint32) float32 {
	return *(*float32)(unsafe.Pointer(&b))
}

// float64frombits converts a uint64 to float64
func float64frombits(b uint64) float64 {
	return *(*float64)(unsafe.Pointer(&b))
}

func main() {
	portName := flag.String("port", "", "Serial port device (e.g., /dev/ttyACM0)")
	baudRate := flag.Int("baud", 115200, "Baud rate")
	flag.Parse()

	if *portName == "" {
		fmt.Fprintf(os.Stderr, "Usage: heliostat -port <device> [-baud <rate>]\n")
		fmt.Fprintf(os.Stderr, "Example: heliostat -port /dev/ttyACM0\n")
		os.Exit(1)
	}

	// Open serial port for UART probe
	mode := &serial.Mode{
		BaudRate: *baudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(*portName, mode)
	if err != nil {
		log.Fatalf("Failed to open serial port %s: %v", *portName, err)
	}
	defer port.Close()

	fmt.Printf("Heliostat - Helios Serial Protocol Analyzer\n")
	fmt.Printf("Port: %s @ %d baud\n", *portName, *baudRate)
	fmt.Printf("Press Ctrl+C to exit\n\n")

	decoder := NewDecoder()
	buf := make([]byte, 128)

	for {
		n, err := port.Read(buf)
		if err != nil {
			log.Printf("Read error: %v", err)
			continue
		}

		for i := 0; i < n; i++ {
			packet, err := decoder.DecodeByte(buf[i])
			if err != nil {
				fmt.Printf("[ERROR] %v\n", err)
				continue
			}
			if packet != nil {
				fmt.Print(formatPacket(packet))
			}
		}
	}
}
