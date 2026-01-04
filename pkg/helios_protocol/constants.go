// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package helios_protocol

// Protocol Framing Bytes
const (
	START_BYTE = 0x7E
	END_BYTE   = 0x7F
	ESC_BYTE   = 0x7D
	ESC_XOR    = 0x20
)

// Packet Size Limits
const (
	MAX_PACKET_SIZE  = 128
	MAX_PAYLOAD_SIZE = 122
)

// CRC-16-CCITT Configuration
const (
	CRC_POLYNOMIAL = 0x1021
	CRC_INITIAL    = 0xFFFF
)

// Message Types - Commands (Master → ICU)
const (
	MSG_SET_MODE           = 0x10
	MSG_SET_PUMP_RATE      = 0x11
	MSG_SET_TARGET_RPM     = 0x12
	MSG_PING_REQUEST       = 0x13
	MSG_SET_TIMEOUT_CONFIG = 0x14
	MSG_EMERGENCY_STOP     = 0x15
	MSG_TELEMETRY_CONFIG   = 0x16
)

// Message Types - Data (ICU → Master)
const (
	MSG_STATE_DATA       = 0x20
	MSG_MOTOR_DATA       = 0x21
	MSG_TEMPERATURE_DATA = 0x22
	MSG_PUMP_DATA        = 0x23
	MSG_GLOW_DATA        = 0x24
	MSG_TELEMETRY_BUNDLE = 0x25
	MSG_PING_RESPONSE    = 0x26
)

// Message Types - Errors
const (
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
