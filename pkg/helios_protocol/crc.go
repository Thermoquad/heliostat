// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package helios_protocol

// CalculateCRC computes CRC-16-CCITT checksum for the given data
func CalculateCRC(data []byte) uint16 {
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
