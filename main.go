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
		d.packet = &Packet{Length: b, Payload: make([]byte, 0, b)}
		d.buffer[d.bufferIndex] = b
		d.bufferIndex++
		d.state = STATE_TYPE
		return nil, nil

	case STATE_TYPE:
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
			return fmt.Sprintf("  Uptime: %d ms (%.2f sec)\n", uptime, float64(uptime)/1000.0)
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
		if len(payload) >= 4 {
			state := payload[0]
			errorCode := payload[1]
			motorCount := payload[2]
			tempCount := payload[3]

			result := fmt.Sprintf("  State: 0x%02X, Error: 0x%02X, Motors: %d, Temps: %d\n", state, errorCode, motorCount, tempCount)

			offset := 4
			for i := 0; i < int(motorCount) && offset+7 <= len(payload); i++ {
				rpm := uint32(payload[offset]) | uint32(payload[offset+1])<<8 | uint32(payload[offset+2])<<16 | uint32(payload[offset+3])<<24
				targetRPM := uint32(payload[offset+4]) | uint32(payload[offset+5])<<8 | uint32(payload[offset+6])<<16 | uint32(payload[offset+7])<<24
				pwm := payload[offset+8]
				result += fmt.Sprintf("    Motor %d: RPM=%d, Target=%d, PWM=%d%%\n", i, rpm, targetRPM, pwm)
				offset += 9
			}

			for i := 0; i < int(tempCount) && offset+3 <= len(payload); i++ {
				// Temperature is float32 (4 bytes, little-endian)
				tempBits := uint32(payload[offset]) | uint32(payload[offset+1])<<8 | uint32(payload[offset+2])<<16 | uint32(payload[offset+3])<<24
				temp := float32frombits(tempBits)
				result += fmt.Sprintf("    Temp %d: %.1f°C\n", i, temp)
				offset += 4
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

// float32frombits converts a uint32 to float32
func float32frombits(b uint32) float32 {
	return *(*float32)(unsafe.Pointer(&b))
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
