// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package fusain

import (
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// ParseCBORMessage parses a Fusain CBOR message: [msg_type, payload_map]
// Returns the message type and decoded payload map (nil for empty payloads)
func ParseCBORMessage(data []byte) (msgType uint8, payload map[int]interface{}, err error) {
	if len(data) == 0 {
		return 0, nil, fmt.Errorf("empty CBOR payload")
	}

	// Decode as array with 2 elements: [msg_type, payload]
	var msg []interface{}
	if err := cbor.Unmarshal(data, &msg); err != nil {
		return 0, nil, fmt.Errorf("failed to decode CBOR: %w", err)
	}

	if len(msg) != 2 {
		return 0, nil, fmt.Errorf("expected 2-element array, got %d elements", len(msg))
	}

	// Extract message type
	switch v := msg[0].(type) {
	case uint64:
		if v > 255 {
			return 0, nil, fmt.Errorf("message type out of range: %d", v)
		}
		msgType = uint8(v)
	default:
		return 0, nil, fmt.Errorf("expected uint for message type, got %T", msg[0])
	}

	// Extract payload map (nil for empty payloads)
	if msg[1] == nil {
		return msgType, nil, nil
	}

	// Convert map[interface{}]interface{} to map[int]interface{}
	switch v := msg[1].(type) {
	case map[interface{}]interface{}:
		payload = make(map[int]interface{})
		for key, val := range v {
			switch k := key.(type) {
			case uint64:
				payload[int(k)] = val
			case int64:
				payload[int(k)] = val
			default:
				return 0, nil, fmt.Errorf("expected integer map key, got %T", key)
			}
		}
	default:
		return 0, nil, fmt.Errorf("expected map or nil for payload, got %T", msg[1])
	}

	return msgType, payload, nil
}

// Map value extraction helpers

// GetMapUint extracts a uint64 from a CBOR map by key
func GetMapUint(m map[int]interface{}, key int) (uint64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case uint64:
		return val, true
	case int64:
		if val >= 0 {
			return uint64(val), true
		}
		return 0, false
	case float64:
		return uint64(val), true
	}
	return 0, false
}

// GetMapInt extracts an int64 from a CBOR map by key
func GetMapInt(m map[int]interface{}, key int) (int64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case int64:
		return val, true
	case uint64:
		return int64(val), true
	case float64:
		return int64(val), true
	}
	return 0, false
}

// GetMapFloat extracts a float64 from a CBOR map by key
func GetMapFloat(m map[int]interface{}, key int) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return val, true
	case int64:
		return float64(val), true
	case uint64:
		return float64(val), true
	}
	return 0, false
}

// GetMapBool extracts a bool from a CBOR map by key
func GetMapBool(m map[int]interface{}, key int) (bool, bool) {
	if m == nil {
		return false, false
	}
	v, ok := m[key]
	if !ok {
		return false, false
	}
	if val, ok := v.(bool); ok {
		return val, true
	}
	return false, false
}

// GetMapBytes extracts a []byte from a CBOR map by key
func GetMapBytes(m map[int]interface{}, key int) ([]byte, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	if val, ok := v.([]byte); ok {
		return val, true
	}
	return nil, false
}
