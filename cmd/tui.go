// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Thermoquad/heliostat/pkg/fusain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// float64frombits converts uint64 bits to float64
func float64frombits(b uint64) float64 {
	return math.Float64frombits(b)
}

// Error log entry
type errorLogEntry struct {
	timestamp time.Time
	message   string
	isError   bool // true for errors, false for warnings
}

// Telemetry data
type telemetryData struct {
	timestamp    time.Time
	state        uint32
	stateName    string
	errorCode    uint8
	motorCount   uint8
	tempCount    uint8
	motorRPM     []int32
	motorTarget  []int32
	temperatures []float64
	uptime       uint64 // milliseconds
	hasUptime    bool
}

// TUI model
type model struct {
	portName      string
	baudRate      int
	statsInterval int
	showAll       bool
	stats         *fusain.Statistics
	errorLog      []errorLogEntry
	maxLogEntries int
	synchronized  bool
	invalidBytes  int
	width         int
	height        int
	quitting      bool
	lastTelemetry *telemetryData
}

// Messages
type tickMsg time.Time
type serialDataMsg struct {
	packet           *fusain.Packet
	decodeErr        error
	validationErrors []fusain.ValidationError
}
type syncMsg struct {
	invalidBytes int
}

// formatUptime formats uptime in milliseconds to human-friendly string
func formatUptime(ms uint64) string {
	if ms == 0 {
		return "0 seconds"
	}

	seconds := ms / 1000
	minutes := seconds / 60
	hours := minutes / 60
	days := hours / 24
	months := days / 30
	years := months / 12

	seconds %= 60
	minutes %= 60
	hours %= 24
	days %= 30
	months %= 12

	parts := []string{}
	if years > 0 {
		if years == 1 {
			parts = append(parts, "1 year")
		} else {
			parts = append(parts, fmt.Sprintf("%d years", years))
		}
	}
	if months > 0 {
		if months == 1 {
			parts = append(parts, "1 month")
		} else {
			parts = append(parts, fmt.Sprintf("%d months", months))
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
	if seconds > 0 || len(parts) == 0 {
		if seconds == 1 {
			parts = append(parts, "1 second")
		} else {
			parts = append(parts, fmt.Sprintf("%d seconds", seconds))
		}
	}

	// Join with commas and "and" for last item
	if len(parts) == 1 {
		return parts[0]
	}
	if len(parts) == 2 {
		return parts[0] + " and " + parts[1]
	}
	last := parts[len(parts)-1]
	rest := strings.Join(parts[:len(parts)-1], ", ")
	return rest + ", and " + last
}

func initialModel(portName string, baudRate, statsInterval int, showAll bool) model {
	return model{
		portName:      portName,
		baudRate:      baudRate,
		statsInterval: statsInterval,
		showAll:       showAll,
		stats:         fusain.NewStatistics(),
		errorLog:      make([]errorLogEntry, 0),
		maxLogEntries: 100,
		synchronized:  false,
		invalidBytes:  0,
		width:         80,
		height:        24,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		tea.EnterAltScreen,
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		// Update statistics rates
		m.stats.CalculateRates()
		return m, tickCmd()

	case syncMsg:
		m.synchronized = true
		m.invalidBytes = msg.invalidBytes
		if msg.invalidBytes > 0 {
			m.addLogEntry(fmt.Sprintf("Synchronized after skipping %d invalid bytes", msg.invalidBytes), false)
		} else {
			m.addLogEntry("Synchronized", false)
		}

	case serialDataMsg:
		if msg.decodeErr != nil {
			if m.synchronized {
				m.stats.Update(nil, msg.decodeErr, nil)
				m.addLogEntry(fmt.Sprintf("DECODE ERROR: %v", msg.decodeErr), true)
			}
		} else if msg.packet != nil {
			m.stats.Update(msg.packet, nil, msg.validationErrors)

			// Parse telemetry data
			m.parseTelemetry(msg.packet)

			if len(msg.validationErrors) > 0 {
				// Validation errors
				msgType := fusain.FormatMessageType(msg.packet.Type())
				for _, err := range msg.validationErrors {
					m.addLogEntry(fmt.Sprintf("%s: %s", msgType, err.Message), true)
				}
			} else if msg.packet.Type() == fusain.MSG_PING_RESPONSE {
				// Ping responses update telemetry silently (no log entry)
				// The uptime will appear in the "Latest Telemetry" box
			} else if m.showAll {
				// Valid packet (only if --show-all)
				msgType := fusain.FormatMessageType(msg.packet.Type())
				m.addLogEntry(fmt.Sprintf("%s (valid)", msgType), false)
			}
		}
	}

	return m, nil
}

func (m *model) addLogEntry(message string, isError bool) {
	entry := errorLogEntry{
		timestamp: time.Now(),
		message:   message,
		isError:   isError,
	}
	m.errorLog = append(m.errorLog, entry)

	// Keep only last N entries
	if len(m.errorLog) > m.maxLogEntries {
		m.errorLog = m.errorLog[len(m.errorLog)-m.maxLogEntries:]
	}
}

// parseTelemetry extracts telemetry data from packets
func (m *model) parseTelemetry(packet *fusain.Packet) {
	payload := packet.Payload()
	stateNames := []string{"INITIALIZING", "IDLE", "BLOWING", "PREHEAT", "PREHEAT_STAGE_2", "HEATING", "COOLING", "ERROR", "E_STOP"}

	switch packet.Type() {
	case fusain.MSG_STATE_DATA:
		if len(payload) < 16 {
			return
		}

		// STATE_DATA payload per spec: error(4) + code(4) + state(4) + timestamp(4)
		// code is the error code (bytes 4-7)
		errorCode := uint8(payload[4]) // Just first byte for error code display
		state := uint32(payload[8]) | uint32(payload[9])<<8 |
			uint32(payload[10])<<16 | uint32(payload[11])<<24

		stateName := "UNKNOWN"
		if int(state) < len(stateNames) {
			stateName = stateNames[state]
		}

		// Preserve uptime if we already have it from a PING_RESPONSE
		uptime := uint64(0)
		hasUptime := false
		if m.lastTelemetry != nil && m.lastTelemetry.hasUptime {
			uptime = m.lastTelemetry.uptime
			hasUptime = true
		}

		m.lastTelemetry = &telemetryData{
			timestamp: time.Now(),
			state:     state,
			stateName: stateName,
			errorCode: errorCode,
			uptime:    uptime,
			hasUptime: hasUptime,
		}

	case fusain.MSG_PING_RESPONSE:
		if len(payload) < 4 {
			return
		}

		// Extract uptime from ping response (4 bytes, little-endian per spec)
		uptime := uint64(payload[0]) | uint64(payload[1])<<8 |
			uint64(payload[2])<<16 | uint64(payload[3])<<24

		// Update or create telemetry with uptime
		if m.lastTelemetry != nil {
			m.lastTelemetry.uptime = uptime
			m.lastTelemetry.hasUptime = true
		} else {
			m.lastTelemetry = &telemetryData{
				timestamp: time.Now(),
				uptime:    uptime,
				hasUptime: true,
			}
		}

	case fusain.MSG_MOTOR_DATA:
		if len(payload) < 32 {
			return
		}

		// MOTOR_DATA: motor(4) + timestamp(4) + rpm(4) + target(4) + max_rpm(4) + min_rpm(4) + pwm(4) + pwm_max(4)
		motorIdx := int32(payload[0]) | int32(payload[1])<<8 |
			int32(payload[2])<<16 | int32(payload[3])<<24
		rpm := int32(payload[8]) | int32(payload[9])<<8 |
			int32(payload[10])<<16 | int32(payload[11])<<24
		target := int32(payload[12]) | int32(payload[13])<<8 |
			int32(payload[14])<<16 | int32(payload[15])<<24

		// Ensure we have storage for this motor
		if m.lastTelemetry == nil {
			m.lastTelemetry = &telemetryData{timestamp: time.Now()}
		}

		// Expand slices if needed
		for len(m.lastTelemetry.motorRPM) <= int(motorIdx) {
			m.lastTelemetry.motorRPM = append(m.lastTelemetry.motorRPM, 0)
		}
		for len(m.lastTelemetry.motorTarget) <= int(motorIdx) {
			m.lastTelemetry.motorTarget = append(m.lastTelemetry.motorTarget, 0)
		}

		m.lastTelemetry.motorRPM[motorIdx] = rpm
		m.lastTelemetry.motorTarget[motorIdx] = target

	case fusain.MSG_TEMP_DATA:
		if len(payload) < 32 {
			return
		}

		// TEMP_DATA: temperature(4) + timestamp(4) + reading(8) + rpm_control(4) + watched_motor(4) + target_temp(8)
		tempIdx := int32(payload[0]) | int32(payload[1])<<8 |
			int32(payload[2])<<16 | int32(payload[3])<<24

		// Extract float64 reading (bytes 8-15, little-endian)
		readingBits := uint64(payload[8]) | uint64(payload[9])<<8 |
			uint64(payload[10])<<16 | uint64(payload[11])<<24 |
			uint64(payload[12])<<32 | uint64(payload[13])<<40 |
			uint64(payload[14])<<48 | uint64(payload[15])<<56
		reading := float64frombits(readingBits)

		// Ensure we have storage for this temperature
		if m.lastTelemetry == nil {
			m.lastTelemetry = &telemetryData{timestamp: time.Now()}
		}

		// Expand slice if needed
		for len(m.lastTelemetry.temperatures) <= int(tempIdx) {
			m.lastTelemetry.temperatures = append(m.lastTelemetry.temperatures, 0)
		}

		m.lastTelemetry.temperatures[tempIdx] = reading
	}
}

func (m model) View() string {
	if m.quitting {
		return "Shutting down...\n"
	}

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		Background(lipgloss.Color("235")).
		Padding(0, 1)

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	statsLabelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Bold(true)

	statsValueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10"))

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true)

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	// Header
	var s strings.Builder
	s.WriteString(titleStyle.Render("HELIOSTAT - ERROR DETECTION"))
	s.WriteString("\n")
	s.WriteString(headerStyle.Render(fmt.Sprintf("Port: %s @ %d baud | Mode: %s | Press 'q' to quit",
		m.portName, m.baudRate, func() string {
			if m.showAll {
				return "All packets"
			}
			return "Errors only"
		}())))
	s.WriteString("\n\n")

	// Sync status
	if !m.synchronized {
		s.WriteString(warningStyle.Render("⏳ Waiting for synchronization..."))
		s.WriteString("\n\n")
	} else {
		s.WriteString(statsValueStyle.Render("✓ Synchronized"))
		if m.invalidBytes > 0 {
			s.WriteString(headerStyle.Render(fmt.Sprintf(" (skipped %d invalid bytes)", m.invalidBytes)))
		}
		s.WriteString("\n\n")
	}

	// Statistics
	m.stats.CalculateRates()
	var validPercent, errorPercent float64
	if m.stats.TotalPackets > 0 {
		validPercent = float64(m.stats.ValidPackets) * 100.0 / float64(m.stats.TotalPackets)
		totalErrors := m.stats.CRCErrors + m.stats.DecodeErrors + m.stats.MalformedPackets + m.stats.AnomalousValues
		errorPercent = float64(totalErrors) * 100.0 / float64(m.stats.TotalPackets)
	}

	statsContent := strings.Builder{}
	statsContent.WriteString(fmt.Sprintf("%s %s   %s %s   %s %s\n",
		statsLabelStyle.Render("Total:"), statsValueStyle.Render(fmt.Sprintf("%d", m.stats.TotalPackets)),
		statsLabelStyle.Render("Valid:"), statsValueStyle.Render(fmt.Sprintf("%d (%.1f%%)", m.stats.ValidPackets, validPercent)),
		statsLabelStyle.Render("Errors:"), errorStyle.Render(fmt.Sprintf("%d (%.1f%%)", m.stats.CRCErrors+m.stats.DecodeErrors+m.stats.MalformedPackets+m.stats.AnomalousValues, errorPercent)),
	))

	if m.stats.CRCErrors > 0 || m.stats.DecodeErrors > 0 {
		statsContent.WriteString(fmt.Sprintf("%s %s   %s %s\n",
			statsLabelStyle.Render("CRC Errors:"), errorStyle.Render(fmt.Sprintf("%d", m.stats.CRCErrors)),
			statsLabelStyle.Render("Decode Errors:"), errorStyle.Render(fmt.Sprintf("%d", m.stats.DecodeErrors)),
		))
	}

	if m.stats.MalformedPackets > 0 {
		statsContent.WriteString(fmt.Sprintf("%s %s",
			statsLabelStyle.Render("Malformed:"), errorStyle.Render(fmt.Sprintf("%d", m.stats.MalformedPackets)),
		))
		if m.stats.InvalidCounts > 0 || m.stats.LengthMismatches > 0 {
			statsContent.WriteString(fmt.Sprintf(" (%s: %d, %s: %d)",
				headerStyle.Render("invalid counts"), m.stats.InvalidCounts,
				headerStyle.Render("length mismatches"), m.stats.LengthMismatches,
			))
		}
		statsContent.WriteString("\n")
	}

	if m.stats.AnomalousValues > 0 {
		statsContent.WriteString(fmt.Sprintf("%s %s",
			statsLabelStyle.Render("Anomalous:"), warningStyle.Render(fmt.Sprintf("%d", m.stats.AnomalousValues)),
		))
		if m.stats.HighRPM > 0 || m.stats.InvalidTemp > 0 || m.stats.InvalidPWM > 0 {
			statsContent.WriteString(fmt.Sprintf(" (%s: %d, %s: %d, %s: %d)",
				headerStyle.Render("high RPM"), m.stats.HighRPM,
				headerStyle.Render("invalid temp"), m.stats.InvalidTemp,
				headerStyle.Render("invalid PWM"), m.stats.InvalidPWM,
			))
		}
		statsContent.WriteString("\n")
	}

	statsContent.WriteString(fmt.Sprintf("%s %s   %s %s",
		statsLabelStyle.Render("Packet Rate:"), statsValueStyle.Render(fmt.Sprintf("%.1f pkts/s", m.stats.PacketRate)),
		statsLabelStyle.Render("Error Rate:"), func() string {
			if m.stats.ErrorRate > 0 {
				return errorStyle.Render(fmt.Sprintf("%.1f err/s", m.stats.ErrorRate))
			}
			return statsValueStyle.Render(fmt.Sprintf("%.1f err/s", m.stats.ErrorRate))
		}(),
	))

	s.WriteString(boxStyle.Render(statsContent.String()))
	s.WriteString("\n\n")

	// Telemetry section (only shown if telemetry received)
	if m.lastTelemetry != nil {
		s.WriteString(statsLabelStyle.Render("Latest Telemetry:"))
		s.WriteString("\n")

		telemetryContent := strings.Builder{}

		// State and error
		telemetryContent.WriteString(fmt.Sprintf("%s %s   %s 0x%02X\n",
			statsLabelStyle.Render("State:"), statsValueStyle.Render(m.lastTelemetry.stateName),
			statsLabelStyle.Render("Error:"), m.lastTelemetry.errorCode,
		))

		// Uptime if available
		if m.lastTelemetry.hasUptime {
			uptimeStr := formatUptime(m.lastTelemetry.uptime)
			telemetryContent.WriteString(fmt.Sprintf("%s %s\n",
				statsLabelStyle.Render("Uptime:"), statsValueStyle.Render(uptimeStr),
			))
		}

		// Motors
		if len(m.lastTelemetry.motorRPM) > 0 {
			for i, rpm := range m.lastTelemetry.motorRPM {
				target := int32(0)
				if i < len(m.lastTelemetry.motorTarget) {
					target = m.lastTelemetry.motorTarget[i]
				}
				telemetryContent.WriteString(fmt.Sprintf("%s %s (target: %d)\n",
					statsLabelStyle.Render(fmt.Sprintf("Motor %d:", i)),
					statsValueStyle.Render(fmt.Sprintf("%d RPM", rpm)),
					target,
				))
			}
		}

		// Temperatures
		if len(m.lastTelemetry.temperatures) > 0 {
			for i, temp := range m.lastTelemetry.temperatures {
				telemetryContent.WriteString(fmt.Sprintf("%s %s\n",
					statsLabelStyle.Render(fmt.Sprintf("Temp %d:", i)),
					statsValueStyle.Render(fmt.Sprintf("%.1f°C", temp)),
				))
			}
		}

		s.WriteString(boxStyle.Render(telemetryContent.String()))
		s.WriteString("\n\n")
	}

	// Error log
	s.WriteString(statsLabelStyle.Render("Recent Events:"))
	s.WriteString("\n")

	// Calculate how many log entries we can show
	logHeight := m.height - 15 // Reserve space for header and stats
	if logHeight < 5 {
		logHeight = 5
	}

	logContent := strings.Builder{}
	startIdx := len(m.errorLog) - logHeight
	if startIdx < 0 {
		startIdx = 0
	}

	if len(m.errorLog) == 0 {
		logContent.WriteString(headerStyle.Render("  (no events yet)"))
	} else {
		for i := startIdx; i < len(m.errorLog); i++ {
			entry := m.errorLog[i]
			timestamp := entry.timestamp.Format("01/02/06 15:04:05.000")
			if entry.isError {
				logContent.WriteString(fmt.Sprintf("%s %s\n",
					headerStyle.Render(timestamp),
					errorStyle.Render("✗ "+entry.message),
				))
			} else {
				logContent.WriteString(fmt.Sprintf("%s %s\n",
					headerStyle.Render(timestamp),
					warningStyle.Render("ℹ "+entry.message),
				))
			}
		}
	}

	s.WriteString(boxStyle.Width(m.width - 4).Render(logContent.String()))

	return s.String()
}
