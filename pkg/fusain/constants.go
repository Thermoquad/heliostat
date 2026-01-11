// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

// Package fusain provides a reference Go implementation of the Fusain serial protocol.
//
// Fusain is a binary protocol for communication between controllers and appliances
// in the Thermoquad ecosystem. This package provides packet encoding/decoding,
// CRC validation, and payload formatting.
//
// See the Fusain specification at origin/documentation/source/specifications/fusain/
package fusain

// Protocol framing bytes
const (
	StartByte = 0x7E
	EndByte   = 0x7F
	EscByte   = 0x7D
	EscXor    = 0x20
)

// Packet size limits
const (
	MaxPacketSize  = 128 // 14 overhead + 114 payload
	MaxPayloadSize = 114
	AddressSize    = 8
)

// CRC-16-CCITT configuration
const (
	crcPolynomial = 0x1021
	crcInitial    = 0xFFFF
)

// Special addresses
const (
	AddressBroadcast = 0x0000000000000000 // All devices
	AddressStateless = 0xFFFFFFFFFFFFFFFF // Routers, subscriptions
)

// Message types - Configuration Commands (Controller → Appliance) 0x10-0x1F
const (
	MsgMotorConfig      = 0x10
	MsgPumpConfig       = 0x11
	MsgTempConfig       = 0x12
	MsgGlowConfig       = 0x13
	MsgDataSubscription = 0x14
	MsgDataUnsubscribe  = 0x15
	MsgTelemetryConfig  = 0x16
	MsgTimeoutConfig    = 0x17
	MsgDiscoveryRequest = 0x1F
)

// Message types - Control Commands (Controller → Appliance) 0x20-0x2F
const (
	MsgStateCommand  = 0x20
	MsgMotorCommand  = 0x21
	MsgPumpCommand   = 0x22
	MsgGlowCommand   = 0x23
	MsgTempCommand   = 0x24
	MsgSendTelemetry = 0x25
	MsgPingRequest   = 0x2F
)

// Message types - Telemetry Data (Appliance → Controller) 0x30-0x3F
const (
	MsgStateData      = 0x30
	MsgMotorData      = 0x31
	MsgPumpData       = 0x32
	MsgGlowData       = 0x33
	MsgTempData       = 0x34
	MsgDeviceAnnounce = 0x35
	MsgPingResponse   = 0x3F
)

// Message types - Errors (Bidirectional) 0xE0-0xEF
const (
	MsgErrorInvalidCmd  = 0xE0
	MsgErrorStateReject = 0xE1
)

// Decoder states (internal)
// No separate TYPE state - type is embedded in CBOR payload
const (
	stateIdle = iota
	stateLength
	stateAddress
	statePayload
	stateCRC1
	stateCRC2
)

// SysState represents system states from STATE_DATA payload
type SysState int

// System state values
const (
	SysStateInitializing SysState = iota
	SysStateIdle
	SysStateBlowing
	SysStatePreheat
	SysStatePreheatStage2
	SysStateHeating
	SysStateCooling
	SysStateError
	SysStateEstop
)

// ErrorCode represents error codes from STATE_DATA payload
type ErrorCode int

// Error code values
const (
	ErrorNone          ErrorCode = 0x00
	ErrorOverheat      ErrorCode = 0x01
	ErrorSensorFault   ErrorCode = 0x02
	ErrorIgnitionFail  ErrorCode = 0x03
	ErrorFlameOut      ErrorCode = 0x04
	ErrorMotorStall    ErrorCode = 0x05
	ErrorPumpFault     ErrorCode = 0x06
	ErrorCommandedStop ErrorCode = 0x07
)

// Mode represents operating modes for STATE_COMMAND
type Mode int

// Operating mode values
const (
	ModeIdle      Mode = 0x00
	ModeFan       Mode = 0x01
	ModeHeat      Mode = 0x02
	ModeEmergency Mode = 0x03
)

// TempCmdType represents temperature command types
type TempCmdType int

// Temperature command type values
const (
	TempCmdWatchMotor        TempCmdType = 0x00
	TempCmdUnwatchMotor      TempCmdType = 0x01
	TempCmdEnableRpmControl  TempCmdType = 0x02
	TempCmdDisableRpmControl TempCmdType = 0x03
	TempCmdSetTargetTemp     TempCmdType = 0x04
)

// TelemetryType represents telemetry types for SEND_TELEMETRY
type TelemetryType int

// Telemetry type values
const (
	TelemetryTypeState TelemetryType = 0x00
	TelemetryTypeMotor TelemetryType = 0x01
	TelemetryTypeTemp  TelemetryType = 0x02
	TelemetryTypePump  TelemetryType = 0x03
	TelemetryTypeGlow  TelemetryType = 0x04
)

// PumpEvent represents pump event types
type PumpEvent int

// Pump event values
const (
	PumpEventInitializing PumpEvent = 0x00
	PumpEventReady        PumpEvent = 0x01
	PumpEventError        PumpEvent = 0x02
	PumpEventCycleStart   PumpEvent = 0x03
	PumpEventPulseEnd     PumpEvent = 0x04
	PumpEventCycleEnd     PumpEvent = 0x05
)
