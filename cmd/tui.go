// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/Thermoquad/heliostat/pkg/helios_protocol"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
	portName       string
	baudRate       int
	statsInterval  int
	showAll        bool
	stats          *helios_protocol.Statistics
	errorLog       []errorLogEntry
	maxLogEntries  int
	synchronized   bool
	invalidBytes   int
	width          int
	height         int
	quitting       bool
	lastTelemetry  *telemetryData
}

// Messages
type tickMsg time.Time
type serialDataMsg struct {
	packet           *helios_protocol.Packet
	decodeErr        error
	validationErrors []helios_protocol.ValidationError
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
		stats:         helios_protocol.NewStatistics(),
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
				msgType := helios_protocol.FormatMessageType(msg.packet.Type())
				for _, err := range msg.validationErrors {
					m.addLogEntry(fmt.Sprintf("%s: %s", msgType, err.Message), true)
				}
			} else if m.showAll {
				// Valid packet (only if --show-all)
				msgType := helios_protocol.FormatMessageType(msg.packet.Type())
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
func (m *model) parseTelemetry(packet *helios_protocol.Packet) {
	payload := packet.Payload()
	stateNames := []string{"INITIALIZING", "IDLE", "BLOWING", "PREHEAT", "PREHEAT_STAGE_2", "HEATING", "COOLING", "ERROR", "E_STOP"}

	switch packet.Type() {
	case helios_protocol.MSG_TELEMETRY_BUNDLE:
		if len(payload) < 7 {
			return
		}

		state := uint32(payload[0]) | uint32(payload[1])<<8 |
			uint32(payload[2])<<16 | uint32(payload[3])<<24
		errorCode := payload[4]
		motorCount := payload[5]
		tempCount := payload[6]

		stateName := "UNKNOWN"
		if int(state) < len(stateNames) {
			stateName = stateNames[state]
		}

		// Extract motor data
		motorRPM := []int32{}
		motorTarget := []int32{}
		offset := 7
		for i := 0; i < int(motorCount) && offset+15 < len(payload); i++ {
			rpm := int32(uint32(payload[offset]) | uint32(payload[offset+1])<<8 |
				uint32(payload[offset+2])<<16 | uint32(payload[offset+3])<<24)
			target := int32(uint32(payload[offset+4]) | uint32(payload[offset+5])<<8 |
				uint32(payload[offset+6])<<16 | uint32(payload[offset+7])<<24)
			motorRPM = append(motorRPM, rpm)
			motorTarget = append(motorTarget, target)
			offset += 16
		}

		// Extract temperature data
		temperatures := []float64{}
		for i := 0; i < int(tempCount) && offset+7 < len(payload); i++ {
			tempBits := uint64(payload[offset]) | uint64(payload[offset+1])<<8 |
				uint64(payload[offset+2])<<16 | uint64(payload[offset+3])<<24 |
				uint64(payload[offset+4])<<32 | uint64(payload[offset+5])<<40 |
				uint64(payload[offset+6])<<48 | uint64(payload[offset+7])<<56
			temp := helios_protocol.Float64frombits(tempBits)
			temperatures = append(temperatures, temp)
			offset += 8
		}

		m.lastTelemetry = &telemetryData{
			timestamp:    time.Now(),
			state:        state,
			stateName:    stateName,
			errorCode:    errorCode,
			motorCount:   motorCount,
			tempCount:    tempCount,
			motorRPM:     motorRPM,
			motorTarget:  motorTarget,
			temperatures: temperatures,
			hasUptime:    false,
		}

	case helios_protocol.MSG_STATE_DATA:
		if len(payload) < 16 {
			return
		}

		state := uint32(payload[0]) | uint32(payload[1])<<8 |
			uint32(payload[2])<<16 | uint32(payload[3])<<24
		errorCode := payload[4]
		uptime := uint64(payload[8]) | uint64(payload[9])<<8 |
			uint64(payload[10])<<16 | uint64(payload[11])<<24 |
			uint64(payload[12])<<32 | uint64(payload[13])<<40 |
			uint64(payload[14])<<48 | uint64(payload[15])<<56

		stateName := "UNKNOWN"
		if int(state) < len(stateNames) {
			stateName = stateNames[state]
		}

		// Update or create telemetry with uptime
		if m.lastTelemetry != nil {
			m.lastTelemetry.uptime = uptime
			m.lastTelemetry.hasUptime = true
		} else {
			m.lastTelemetry = &telemetryData{
				timestamp: time.Now(),
				state:     state,
				stateName: stateName,
				errorCode: errorCode,
				uptime:    uptime,
				hasUptime: true,
			}
		}
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
