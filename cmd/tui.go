// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/Thermoquad/heliostat/pkg/fusain"
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
	state        uint64
	stateName    string
	errorCode    int64
	motorCount   uint8
	tempCount    uint8
	motorRPM     []int64
	motorTarget  []int64
	temperatures []float64
	uptime       uint64 // milliseconds
	hasUptime    bool
}

// TUI model
type model struct {
	connInfo      string
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
type batchDataMsg struct {
	messages []serialDataMsg
	syncMsg  *syncMsg
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

func initialModel(connInfo string, statsInterval int, showAll bool) model {
	return model{
		connInfo:      connInfo,
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
	return tickCmd()
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
		m.processSerialData(msg)

	case batchDataMsg:
		// Handle sync message first
		if msg.syncMsg != nil {
			m.synchronized = true
			m.invalidBytes = msg.syncMsg.invalidBytes
			if msg.syncMsg.invalidBytes > 0 {
				m.addLogEntry(fmt.Sprintf("Synchronized after skipping %d invalid bytes", msg.syncMsg.invalidBytes), false)
			} else {
				m.addLogEntry("Synchronized", false)
			}
		}
		// Process all batched messages
		for _, data := range msg.messages {
			m.processSerialData(data)
		}
	}

	return m, nil
}

func (m *model) processSerialData(msg serialDataMsg) {
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
		} else if msg.packet.Type() == fusain.MsgPingResponse {
			// Ping responses update telemetry silently (no log entry)
			// The uptime will appear in the "Latest Telemetry" box
		} else if m.showAll {
			// Valid packet (only if --show-all)
			msgType := fusain.FormatMessageType(msg.packet.Type())
			m.addLogEntry(fmt.Sprintf("%s (valid)", msgType), false)
		}
	}
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

// parseTelemetry extracts telemetry data from packets using CBOR payload maps
func (m *model) parseTelemetry(packet *fusain.Packet) {
	payloadMap := packet.PayloadMap()
	stateNames := []string{"INITIALIZING", "IDLE", "BLOWING", "PREHEAT", "PREHEAT_STAGE_2", "HEATING", "COOLING", "ERROR", "E_STOP"}

	switch packet.Type() {
	case fusain.MsgStateData:
		// CBOR keys: 0=error(bool), 1=code, 2=state, 3=timestamp
		state, hasState := fusain.GetMapUint(payloadMap, 2)
		if !hasState {
			return
		}

		errorCode, _ := fusain.GetMapInt(payloadMap, 1)

		stateName := "UNKNOWN"
		if int(state) < len(stateNames) {
			stateName = stateNames[state]
		}

		// Update existing telemetry or create new one, preserving other fields
		if m.lastTelemetry == nil {
			m.lastTelemetry = &telemetryData{timestamp: time.Now()}
		}
		m.lastTelemetry.state = state
		m.lastTelemetry.stateName = stateName
		m.lastTelemetry.errorCode = errorCode
		m.lastTelemetry.timestamp = time.Now()

	case fusain.MsgPingResponse:
		// CBOR keys: 0=uptime
		uptime, ok := fusain.GetMapUint(payloadMap, 0)
		if !ok {
			return
		}

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

	case fusain.MsgMotorData:
		// CBOR keys: 0=motor, 1=timestamp, 2=rpm, 3=target, 4=max-rpm, 5=min-rpm, 6=pwm, 7=pwm-max
		motorIdx, ok := fusain.GetMapInt(payloadMap, 0)
		if !ok || motorIdx < 0 {
			return
		}

		rpm, _ := fusain.GetMapInt(payloadMap, 2)
		target, _ := fusain.GetMapInt(payloadMap, 3)

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

	case fusain.MsgTempData:
		// CBOR keys: 0=thermometer, 1=timestamp, 2=reading, 3=temperature-rpm-control, 4=watched-motor, 5=target-temperature
		tempIdx, ok := fusain.GetMapInt(payloadMap, 0)
		if !ok || tempIdx < 0 {
			return
		}

		reading, _ := fusain.GetMapFloat(payloadMap, 2)

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

	var s strings.Builder

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
	s.WriteString(titleStyle.Render("HELIOSTAT - ERROR DETECTION"))
	s.WriteString("\n")
	s.WriteString(headerStyle.Render(fmt.Sprintf("%s | Mode: %s | Press 'q' to quit",
		m.connInfo, func() string {
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

	// Telemetry section - always show fixed height layout
	s.WriteString(statsLabelStyle.Render("Latest Telemetry:"))
	s.WriteString("\n")

	telemetryContent := strings.Builder{}

	// State and error - always show (use defaults if no data)
	stateName := "---            "
	errorCode := int64(0)
	if m.lastTelemetry != nil {
		stateName = fmt.Sprintf("%-15s", m.lastTelemetry.stateName)
		errorCode = m.lastTelemetry.errorCode
	}
	telemetryContent.WriteString(fmt.Sprintf("%s %s   %s 0x%02X\n",
		statsLabelStyle.Render("State:"), statsValueStyle.Render(stateName),
		statsLabelStyle.Render("Error:"), errorCode,
	))

	// Uptime - always show line
	uptimeStr := "---                                     "
	if m.lastTelemetry != nil && m.lastTelemetry.hasUptime {
		uptimeStr = fmt.Sprintf("%-40s", formatUptime(m.lastTelemetry.uptime))
	}
	telemetryContent.WriteString(fmt.Sprintf("%s %s\n",
		statsLabelStyle.Render("Uptime:"), statsValueStyle.Render(uptimeStr),
	))

	// Motor 0 - always show
	motorRPM := int64(0)
	motorTarget := int64(0)
	if m.lastTelemetry != nil && len(m.lastTelemetry.motorRPM) > 0 {
		motorRPM = m.lastTelemetry.motorRPM[0]
		if len(m.lastTelemetry.motorTarget) > 0 {
			motorTarget = m.lastTelemetry.motorTarget[0]
		}
	}
	telemetryContent.WriteString(fmt.Sprintf("%s %s (target: %5d)\n",
		statsLabelStyle.Render("Motor 0:"),
		statsValueStyle.Render(fmt.Sprintf("%5d RPM", motorRPM)),
		motorTarget,
	))

	// Temp 0 - always show
	temp := 0.0
	if m.lastTelemetry != nil && len(m.lastTelemetry.temperatures) > 0 {
		temp = m.lastTelemetry.temperatures[0]
	}
	telemetryContent.WriteString(fmt.Sprintf("%s %s\n",
		statsLabelStyle.Render("Temp 0: "),
		statsValueStyle.Render(fmt.Sprintf("%7.1f°C", temp)),
	))

	s.WriteString(boxStyle.Render(telemetryContent.String()))
	s.WriteString("\n\n")

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
