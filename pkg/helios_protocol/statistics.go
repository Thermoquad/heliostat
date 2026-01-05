// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package helios_protocol

import (
	"fmt"
	"time"
)

// Statistics tracks packet statistics and error rates
type Statistics struct {
	StartTime      time.Time
	LastUpdateTime time.Time

	// Counters
	TotalPackets     uint64
	ValidPackets     uint64
	CRCErrors        uint64
	DecodeErrors     uint64
	MalformedPackets uint64
	InvalidCounts    uint64
	LengthMismatches uint64
	AnomalousValues  uint64
	HighRPM          uint64
	InvalidTemp      uint64
	InvalidPWM       uint64

	// Rates (calculated)
	PacketRate float64 // packets/sec
	ErrorRate  float64 // errors/sec
}

// NewStatistics creates a new statistics tracker
func NewStatistics() *Statistics {
	now := time.Now()
	return &Statistics{
		StartTime:      now,
		LastUpdateTime: now,
	}
}

// Update updates statistics based on a packet and its errors
func (s *Statistics) Update(packet *Packet, decodeErr error, validationErrors []ValidationError) {
	s.TotalPackets++

	// Handle decode errors
	if decodeErr != nil {
		// Check if it's a CRC error (special case - only count as CRC error)
		if len(decodeErr.Error()) > 0 && decodeErr.Error()[:12] == "CRC mismatch" {
			s.CRCErrors++
		} else {
			// Other decode errors (framing, overflow, etc.)
			s.DecodeErrors++
		}
		return // Don't process packet further if decode failed
	}

	// Handle validation errors
	if len(validationErrors) > 0 {
		for _, err := range validationErrors {
			switch err.Type {
			case ANOMALY_INVALID_COUNT:
				s.InvalidCounts++
				s.MalformedPackets++
			case ANOMALY_LENGTH_MISMATCH:
				s.LengthMismatches++
				s.MalformedPackets++
			case ANOMALY_HIGH_RPM:
				s.HighRPM++
				s.AnomalousValues++
			case ANOMALY_INVALID_TEMP:
				s.InvalidTemp++
				s.AnomalousValues++
			case ANOMALY_INVALID_PWM:
				s.InvalidPWM++
				s.AnomalousValues++
			}
		}
	} else {
		// No errors - packet is valid
		s.ValidPackets++
	}

	// Update timestamp for rate calculation
	s.LastUpdateTime = time.Now()
}

// CalculateRates calculates packet and error rates
func (s *Statistics) CalculateRates() {
	elapsed := time.Since(s.StartTime).Seconds()
	if elapsed > 0 {
		s.PacketRate = float64(s.TotalPackets) / elapsed
		errorCount := s.CRCErrors + s.DecodeErrors + s.MalformedPackets + s.AnomalousValues
		s.ErrorRate = float64(errorCount) / elapsed
	}
}

// String returns a formatted statistics summary
func (s *Statistics) String() string {
	s.CalculateRates()

	// Calculate percentages
	var validPercent, crcErrorPercent, decodeErrorPercent, malformedPercent, anomalousPercent float64
	if s.TotalPackets > 0 {
		validPercent = float64(s.ValidPackets) * 100.0 / float64(s.TotalPackets)
		crcErrorPercent = float64(s.CRCErrors) * 100.0 / float64(s.TotalPackets)
		decodeErrorPercent = float64(s.DecodeErrors) * 100.0 / float64(s.TotalPackets)
		malformedPercent = float64(s.MalformedPackets) * 100.0 / float64(s.TotalPackets)
		anomalousPercent = float64(s.AnomalousValues) * 100.0 / float64(s.TotalPackets)
	}

	elapsed := time.Since(s.StartTime)

	result := fmt.Sprintf("=== Statistics (%.0f seconds) ===\n", elapsed.Seconds())
	result += fmt.Sprintf("Total Packets:   %8d\n", s.TotalPackets)
	result += fmt.Sprintf("Valid Packets:   %8d (%.1f%%)\n", s.ValidPackets, validPercent)

	if s.CRCErrors > 0 {
		result += fmt.Sprintf("CRC Errors:      %8d (%.1f%%)\n", s.CRCErrors, crcErrorPercent)
	}
	if s.DecodeErrors > 0 {
		result += fmt.Sprintf("Decode Errors:   %8d (%.1f%%)\n", s.DecodeErrors, decodeErrorPercent)
	}
	if s.MalformedPackets > 0 {
		result += fmt.Sprintf("Malformed Pkts:  %8d (%.1f%%)\n", s.MalformedPackets, malformedPercent)
		if s.InvalidCounts > 0 {
			result += fmt.Sprintf("  Invalid Counts:   %5d\n", s.InvalidCounts)
		}
		if s.LengthMismatches > 0 {
			result += fmt.Sprintf("  Length Mismatch:  %5d\n", s.LengthMismatches)
		}
	}
	if s.AnomalousValues > 0 {
		result += fmt.Sprintf("Anomalous Values:%8d (%.1f%%)\n", s.AnomalousValues, anomalousPercent)
		if s.HighRPM > 0 {
			result += fmt.Sprintf("  High RPM (>6000): %5d\n", s.HighRPM)
		}
		if s.InvalidTemp > 0 {
			result += fmt.Sprintf("  Invalid Temp:     %5d\n", s.InvalidTemp)
		}
		if s.InvalidPWM > 0 {
			result += fmt.Sprintf("  Invalid PWM:      %5d\n", s.InvalidPWM)
		}
	}

	result += fmt.Sprintf("Packet Rate:     %8.1f pkts/sec\n", s.PacketRate)
	result += fmt.Sprintf("Error Rate:      %8.1f errors/sec\n", s.ErrorRate)
	result += "================================\n"

	return result
}

// Reset resets all statistics counters
func (s *Statistics) Reset() {
	now := time.Now()
	s.StartTime = now
	s.LastUpdateTime = now
	s.TotalPackets = 0
	s.ValidPackets = 0
	s.CRCErrors = 0
	s.DecodeErrors = 0
	s.MalformedPackets = 0
	s.InvalidCounts = 0
	s.LengthMismatches = 0
	s.AnomalousValues = 0
	s.HighRPM = 0
	s.InvalidTemp = 0
	s.InvalidPWM = 0
	s.PacketRate = 0
	s.ErrorRate = 0
}
