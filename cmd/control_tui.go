// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2025 Kaz Walker, Thermoquad

package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Thermoquad/heliostat/pkg/fusain"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

//////////////////////////////////////////////////////////////
// Constants
//////////////////////////////////////////////////////////////

const (
	discoveryTimeoutSeconds = 3 // Discovery ends N seconds after last device seen
	pingIntervalSeconds     = 5 // Send ping requests every N seconds
	maxRPM                  = 6000
	minRPM                  = 0
)

// Focus states
const (
	focusDeviceList = iota
	focusRPMInput
	focusButton
)

//////////////////////////////////////////////////////////////
// Types
//////////////////////////////////////////////////////////////

// device represents a discovered Helios heater
type device struct {
	address   uint64
	state     uint64
	stateName string
	lastSeen  time.Time
}

// Implement list.Item interface
func (d device) Title() string       { return fmt.Sprintf("Heater %016X", d.address) }
func (d device) Description() string { return d.stateName }
func (d device) FilterValue() string { return fmt.Sprintf("%X", d.address) }

// controlModel is the Bubble Tea model for the control TUI
type controlModel struct {
	// Connection manager (for sending commands and reconnection)
	connMgr  *connectionManager
	connInfo string

	// Device tracking
	devices    []device
	deviceList list.Model

	// Discovery state
	discoveryDone    bool
	lastDeviceSeen   time.Time
	discoveryDevices map[uint64]*device // Track devices during discovery

	// Monitoring (reused from tui.go patterns)
	stats         *fusain.Statistics
	errorLog      []errorLogEntry
	maxLogEntries int
	lastTelemetry map[uint64]*telemetryData // Telemetry per device address

	// Control
	rpmInput     textinput.Model
	focusedField int

	// UI state
	width          int
	height         int
	synchronized   bool
	quitting       bool
	connectionLost bool

	// Ping state
	lastPingTime    time.Time
	routerUptime    uint64 // Router uptime from stateless address ping responses
	hasRouterUptime bool
}

//////////////////////////////////////////////////////////////
// Messages
//////////////////////////////////////////////////////////////

type controlTickMsg time.Time

type controlDataMsg struct {
	packet           *fusain.Packet
	decodeErr        error
	validationErrors []fusain.ValidationError
}

type controlSyncMsg struct {
	invalidBytes int
}

type controlBatchMsg struct {
	messages []controlDataMsg
	syncMsg  *controlSyncMsg
}

type discoveryCompleteMsg struct{}

type connectionLostMsg struct{}

type reconnectedMsg struct {
	connInfo string
}

//////////////////////////////////////////////////////////////
// Model Initialization
//////////////////////////////////////////////////////////////

func initialControlModel(connMgr *connectionManager, connInfo string) controlModel {
	// Initialize text input for RPM
	ti := textinput.New()
	ti.Placeholder = "1500"
	ti.CharLimit = 5
	ti.Width = 10

	// Initialize device list with empty items
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetHeight(2)
	deviceList := list.New([]list.Item{}, delegate, 30, 10)
	deviceList.Title = "Devices"
	deviceList.SetShowStatusBar(false)
	deviceList.SetShowHelp(false)
	deviceList.SetFilteringEnabled(false)

	return controlModel{
		connMgr:          connMgr,
		connInfo:         connInfo,
		devices:          make([]device, 0),
		deviceList:       deviceList,
		discoveryDone:    false,
		discoveryDevices: make(map[uint64]*device),
		stats:            fusain.NewStatistics(),
		errorLog:         make([]errorLogEntry, 0),
		maxLogEntries:    100,
		lastTelemetry:    make(map[uint64]*telemetryData),
		rpmInput:         ti,
		focusedField:     focusDeviceList,
		width:            80,
		height:           24,
		synchronized:     false,
	}
}

//////////////////////////////////////////////////////////////
// Bubble Tea Interface
//////////////////////////////////////////////////////////////

func (m controlModel) Init() tea.Cmd {
	return controlTickCmd()
}

func controlTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return controlTickMsg(t)
	})
}

func (m controlModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.MouseMsg:
		return m.handleMouseMsg(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateListSize()

	case controlTickMsg:
		m.stats.CalculateRates()
		// Check discovery timeout
		if !m.discoveryDone && !m.lastDeviceSeen.IsZero() {
			if time.Since(m.lastDeviceSeen) > time.Duration(discoveryTimeoutSeconds)*time.Second {
				m.finishDiscovery()
			}
		}
		// Send periodic ping requests (after discovery)
		// - To each device: gets device uptime (works in UART mode)
		// - To stateless: keeps router subscription alive, gets router uptime
		if m.discoveryDone && time.Since(m.lastPingTime) >= time.Duration(pingIntervalSeconds)*time.Second {
			m.lastPingTime = time.Now()
			for _, dev := range m.devices {
				m.sendPingRequest(dev.address)
			}
			m.sendPingRequest(fusain.AddressStateless)
		}
		return m, controlTickCmd()

	case controlSyncMsg:
		m.synchronized = true
		if msg.invalidBytes > 0 {
			m.addLogEntry(fmt.Sprintf("Synchronized after skipping %d invalid bytes", msg.invalidBytes), false)
		} else {
			m.addLogEntry("Synchronized", false)
		}

	case controlBatchMsg:
		if msg.syncMsg != nil {
			m.synchronized = true
			if msg.syncMsg.invalidBytes > 0 {
				m.addLogEntry(fmt.Sprintf("Synchronized after skipping %d invalid bytes", msg.syncMsg.invalidBytes), false)
			} else {
				m.addLogEntry("Synchronized", false)
			}
		}
		for _, data := range msg.messages {
			m.processControlData(data)
		}

	case discoveryCompleteMsg:
		m.finishDiscovery()

	case connectionLostMsg:
		m.connectionLost = true
		m.addLogEntry("Connection lost - reconnecting...", true)

	case reconnectedMsg:
		m.connectionLost = false
		m.connInfo = msg.connInfo
		// Reset discovery state for new discovery cycle
		m.resetDiscovery()
		m.addLogEntry("Reconnected - starting discovery", false)
	}

	// Update child components
	var cmd tea.Cmd
	if m.focusedField == focusRPMInput {
		m.rpmInput, cmd = m.rpmInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.focusedField == focusDeviceList {
		m.deviceList, cmd = m.deviceList.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *controlModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "tab":
		return m.cycleFocus(1), nil

	case "shift+tab":
		return m.cycleFocus(-1), nil

	case "enter":
		if m.discoveryDone {
			return m.handleEnter()
		}

	case "up", "k":
		if m.focusedField == focusDeviceList {
			m.deviceList, _ = m.deviceList.Update(msg)
		}

	case "down", "j":
		if m.focusedField == focusDeviceList {
			m.deviceList, _ = m.deviceList.Update(msg)
		}
	}

	// Pass through to focused component
	if m.focusedField == focusRPMInput {
		var cmd tea.Cmd
		m.rpmInput, cmd = m.rpmInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *controlModel) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	// Handle mouse clicks based on position
	// Device list is in left panel (columns 0-30)
	// Control panel is on right
	// Button detection is approximate based on layout

	// For now, pass mouse events to the list
	m.deviceList, _ = m.deviceList.Update(msg)

	return m, nil
}

func (m *controlModel) cycleFocus(delta int) *controlModel {
	if !m.discoveryDone {
		return m
	}

	// Determine max focus based on state
	maxFocus := focusButton
	selected := m.getSelectedDevice()
	if selected == nil {
		m.focusedField = focusDeviceList
		return m
	}

	// Cycle through focus states
	m.focusedField = (m.focusedField + delta + maxFocus + 1) % (maxFocus + 1)

	// Skip RPM input if not in IDLE state
	if m.focusedField == focusRPMInput && selected.state != uint64(fusain.SysStateIdle) {
		m.focusedField = (m.focusedField + delta + maxFocus + 1) % (maxFocus + 1)
	}

	// Update focus state
	if m.focusedField == focusRPMInput {
		m.rpmInput.Focus()
	} else {
		m.rpmInput.Blur()
	}

	return m
}

func (m *controlModel) handleEnter() (tea.Model, tea.Cmd) {
	// Don't allow control commands while connection is lost
	if m.connectionLost {
		m.addLogEntry("Cannot send command: connection lost", true)
		return m, nil
	}

	selected := m.getSelectedDevice()
	if selected == nil {
		return m, nil
	}

	// In IDLE state: Start Fan button or RPM input triggers fan command
	if selected.state == uint64(fusain.SysStateIdle) {
		if m.focusedField == focusButton || m.focusedField == focusRPMInput {
			return m.sendFanCommand()
		}
	}

	// In FAN/BLOWING state: Return to Idle button
	if selected.state == uint64(fusain.SysStateBlowing) {
		if m.focusedField == focusButton {
			return m.sendIdleCommand()
		}
	}

	return m, nil
}

func (m controlModel) View() string {
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

	focusedBoxStyle := boxStyle.
		BorderForeground(lipgloss.Color("12"))

	buttonStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("12")).
		Padding(0, 2)

	focusedButtonStyle := buttonStyle.
		Background(lipgloss.Color("10"))

	// Header
	helpText := "q=quit"
	if m.discoveryDone {
		helpText = "q=quit Tab=switch"
	}
	s.WriteString(titleStyle.Render("HELIOSTAT CONTROL"))
	s.WriteString(" ")
	connStatus := m.connInfo
	if m.connectionLost {
		connStatus = warningStyle.Render("RECONNECTING...")
	}
	s.WriteString(headerStyle.Render(fmt.Sprintf("| %s | %s", connStatus, helpText)))
	s.WriteString("\n")

	// Router uptime (below header)
	if m.hasRouterUptime {
		s.WriteString(fmt.Sprintf(" %s %s",
			statsLabelStyle.Render("Router Uptime:"),
			statsValueStyle.Render(formatUptime(m.routerUptime))))
	}
	s.WriteString("\n\n")

	if !m.discoveryDone {
		// Discovery mode view
		s.WriteString(m.renderDiscoveryView(statsLabelStyle, statsValueStyle, warningStyle, boxStyle))
	} else {
		// Normal control view
		s.WriteString(m.renderControlView(statsLabelStyle, statsValueStyle, errorStyle, warningStyle, headerStyle, boxStyle, focusedBoxStyle, buttonStyle, focusedButtonStyle))
	}

	return s.String()
}

//////////////////////////////////////////////////////////////
// View Helpers
//////////////////////////////////////////////////////////////

func (m controlModel) renderDiscoveryView(statsLabelStyle, statsValueStyle, warningStyle, boxStyle lipgloss.Style) string {
	var s strings.Builder

	s.WriteString(warningStyle.Render("Discovering devices..."))
	s.WriteString("\n")
	s.WriteString(fmt.Sprintf("Found: %d heater(s)\n\n", len(m.discoveryDevices)))

	// Event log during discovery
	s.WriteString(m.renderEventLog(statsLabelStyle, warningStyle, boxStyle))

	return s.String()
}

func (m controlModel) renderControlView(statsLabelStyle, statsValueStyle, errorStyle, warningStyle, headerStyle, boxStyle, focusedBoxStyle, buttonStyle, focusedButtonStyle lipgloss.Style) string {
	var s strings.Builder

	// Layout: left panel (devices) | right panel (control)
	leftWidth := 30
	rightWidth := m.width - leftWidth - 6

	// Device list panel
	listStyle := boxStyle.Width(leftWidth)
	if m.focusedField == focusDeviceList {
		listStyle = focusedBoxStyle.Width(leftWidth)
	}
	devicePanel := listStyle.Render(m.deviceList.View())

	// Control panel
	controlContent := m.renderControlPanel(statsLabelStyle, statsValueStyle, headerStyle, buttonStyle, focusedButtonStyle)
	controlStyle := boxStyle.Width(rightWidth)
	controlPanel := controlStyle.Render(controlContent)

	// Join panels horizontally
	s.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, devicePanel, " ", controlPanel))
	s.WriteString("\n\n")

	// Statistics bar
	s.WriteString(m.renderStatisticsBar(statsLabelStyle, statsValueStyle, errorStyle, boxStyle))
	s.WriteString("\n\n")

	// Telemetry for selected device
	selected := m.getSelectedDevice()
	if selected != nil {
		s.WriteString(m.renderTelemetry(selected.address, statsLabelStyle, statsValueStyle, boxStyle))
		s.WriteString("\n\n")
	}

	// Event log
	s.WriteString(m.renderEventLog(statsLabelStyle, warningStyle, boxStyle))

	return s.String()
}

func (m controlModel) renderControlPanel(statsLabelStyle, statsValueStyle, headerStyle, buttonStyle, focusedButtonStyle lipgloss.Style) string {
	var s strings.Builder

	selected := m.getSelectedDevice()
	if selected == nil {
		s.WriteString(headerStyle.Render("No device selected"))
		return s.String()
	}

	// Selected device info
	s.WriteString(fmt.Sprintf("%s Heater %016X\n", statsLabelStyle.Render("Selected:"), selected.address))
	s.WriteString(fmt.Sprintf("%s %s\n\n", statsLabelStyle.Render("State:"), statsValueStyle.Render(selected.stateName)))

	// Control based on state
	if selected.state == uint64(fusain.SysStateIdle) {
		// IDLE state: show fan RPM input and Start Fan button
		s.WriteString(statsLabelStyle.Render("Fan RPM: "))
		if m.focusedField == focusRPMInput {
			s.WriteString(m.rpmInput.View())
		} else {
			// Show as plain text when not focused
			val := m.rpmInput.Value()
			if val == "" {
				val = m.rpmInput.Placeholder
			}
			s.WriteString(fmt.Sprintf("[%s]", val))
		}
		s.WriteString("\n\n")

		// Start Fan button
		btnText := "[ Start Fan ]"
		if m.focusedField == focusButton {
			s.WriteString(focusedButtonStyle.Render(btnText))
		} else {
			s.WriteString(buttonStyle.Render(btnText))
		}
	} else if selected.state == uint64(fusain.SysStateBlowing) {
		// FAN/BLOWING state: show current RPM and Return to Idle button
		telem := m.lastTelemetry[selected.address]
		if telem != nil && len(telem.motorRPM) > 0 {
			s.WriteString(fmt.Sprintf("Current: %s / %d target\n\n",
				statsValueStyle.Render(fmt.Sprintf("%d RPM", telem.motorRPM[0])),
				telem.motorTarget[0]))
		}

		// Return to Idle button
		btnText := "[ Return to Idle ]"
		if m.focusedField == focusButton {
			s.WriteString(focusedButtonStyle.Render(btnText))
		} else {
			s.WriteString(buttonStyle.Render(btnText))
		}
	} else {
		// Other states: just show state name
		s.WriteString(headerStyle.Render(fmt.Sprintf("State: %s (no controls available)", selected.stateName)))
	}

	return s.String()
}

func (m controlModel) renderStatisticsBar(statsLabelStyle, statsValueStyle, errorStyle, boxStyle lipgloss.Style) string {
	m.stats.CalculateRates()
	var validPercent, errorPercent float64
	if m.stats.TotalPackets > 0 {
		validPercent = float64(m.stats.ValidPackets) * 100.0 / float64(m.stats.TotalPackets)
		totalErrors := m.stats.CRCErrors + m.stats.DecodeErrors + m.stats.MalformedPackets + m.stats.AnomalousValues
		errorPercent = float64(totalErrors) * 100.0 / float64(m.stats.TotalPackets)
	}

	content := fmt.Sprintf("%s %s  %s %s  %s %s  %s %s",
		statsLabelStyle.Render("Total:"), statsValueStyle.Render(fmt.Sprintf("%d", m.stats.TotalPackets)),
		statsLabelStyle.Render("Valid:"), statsValueStyle.Render(fmt.Sprintf("%.1f%%", validPercent)),
		statsLabelStyle.Render("Errors:"), func() string {
			if errorPercent > 0 {
				return errorStyle.Render(fmt.Sprintf("%.1f%%", errorPercent))
			}
			return statsValueStyle.Render("0.0%")
		}(),
		statsLabelStyle.Render("Rate:"), statsValueStyle.Render(fmt.Sprintf("%.1f pkt/s", m.stats.PacketRate)),
	)

	return boxStyle.Width(m.width - 4).Render(content)
}

func (m controlModel) renderTelemetry(address uint64, statsLabelStyle, statsValueStyle, boxStyle lipgloss.Style) string {
	telem := m.lastTelemetry[address]

	var content strings.Builder
	content.WriteString(statsLabelStyle.Render("TELEMETRY"))
	content.WriteString(" | ")

	if telem == nil {
		content.WriteString("No telemetry data")
		return boxStyle.Width(m.width - 4).Render(content.String())
	}

	// State
	content.WriteString(fmt.Sprintf("%s %s  ", statsLabelStyle.Render("State:"), statsValueStyle.Render(telem.stateName)))

	// Motor
	if len(telem.motorRPM) > 0 {
		content.WriteString(fmt.Sprintf("%s %s  ",
			statsLabelStyle.Render("Motor:"),
			statsValueStyle.Render(fmt.Sprintf("%d RPM", telem.motorRPM[0]))))
	}

	// Temperature
	if len(telem.temperatures) > 0 {
		content.WriteString(fmt.Sprintf("%s %s  ",
			statsLabelStyle.Render("Temp:"),
			statsValueStyle.Render(fmt.Sprintf("%.1fC", telem.temperatures[0]))))
	}

	// Device uptime (from device-addressed ping response)
	if telem.hasUptime {
		content.WriteString(fmt.Sprintf("%s %s",
			statsLabelStyle.Render("Uptime:"),
			statsValueStyle.Render(formatUptime(telem.uptime))))
	}

	return boxStyle.Width(m.width - 4).Render(content.String())
}

func (m controlModel) renderEventLog(statsLabelStyle, warningStyle, boxStyle lipgloss.Style) string {
	var s strings.Builder
	s.WriteString(statsLabelStyle.Render("EVENTS"))
	s.WriteString("\n")

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyleLocal := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)

	// Calculate available height for log
	logHeight := 8
	if len(m.errorLog) < logHeight {
		logHeight = len(m.errorLog)
	}

	startIdx := len(m.errorLog) - logHeight
	if startIdx < 0 {
		startIdx = 0
	}

	if len(m.errorLog) == 0 {
		s.WriteString(headerStyle.Render("  (no events yet)"))
	} else {
		for i := startIdx; i < len(m.errorLog); i++ {
			entry := m.errorLog[i]
			timestamp := entry.timestamp.Format("15:04:05.000")
			icon := "i"
			style := warningStyle
			if entry.isError {
				icon = "x"
				style = errorStyleLocal
			}
			s.WriteString(fmt.Sprintf("%s %s %s\n",
				headerStyle.Render(timestamp),
				style.Render(icon),
				entry.message))
		}
	}

	return boxStyle.Width(m.width - 4).Render(s.String())
}

//////////////////////////////////////////////////////////////
// Data Processing
//////////////////////////////////////////////////////////////

func (m *controlModel) processControlData(msg controlDataMsg) {
	if msg.decodeErr != nil {
		if m.synchronized {
			m.stats.Update(nil, msg.decodeErr, nil)
			m.addLogEntry(fmt.Sprintf("DECODE ERROR: %v", msg.decodeErr), true)
		}
		return
	}

	if msg.packet == nil {
		return
	}

	m.stats.Update(msg.packet, nil, msg.validationErrors)

	// Process packet based on type
	msgType := msg.packet.Type()
	address := msg.packet.Address()

	switch msgType {
	case fusain.MsgDeviceAnnounce:
		m.handleDeviceAnnounce(msg.packet)

	case fusain.MsgStateData:
		m.handleStateData(msg.packet, address)
		// Also parse telemetry
		m.parseTelemetryForDevice(msg.packet, address)

	case fusain.MsgMotorData, fusain.MsgTempData:
		m.parseTelemetryForDevice(msg.packet, address)

	case fusain.MsgPingResponse:
		// If from stateless address (router), update router uptime only
		// If from specific device, update that device's uptime
		if address == fusain.AddressStateless {
			// Router uptime - store separately, don't update device uptimes
			payloadMap := msg.packet.PayloadMap()
			uptime, ok := fusain.GetMapUint(payloadMap, 0)
			if ok {
				m.routerUptime = uptime
				m.hasRouterUptime = true
			}
		} else {
			// Device-specific uptime
			m.parseTelemetryForDevice(msg.packet, address)
		}

	default:
		// Other packet types - just log if there are validation errors
		if len(msg.validationErrors) > 0 {
			for _, err := range msg.validationErrors {
				m.addLogEntry(fmt.Sprintf("%s: %s", fusain.FormatMessageType(msgType), err.Message), true)
			}
		}
	}
}

func (m *controlModel) handleDeviceAnnounce(packet *fusain.Packet) {
	address := packet.Address()

	// Check for end-of-discovery marker:
	// Per Fusain spec: ADDRESS = stateless (0xFFFFFFFFFFFFFFFF), all counts = 0
	if address == fusain.AddressStateless {
		if !m.discoveryDone {
			m.finishDiscovery()
		}
		return
	}

	// Add/update device in discovery map
	if _, exists := m.discoveryDevices[address]; !exists {
		m.discoveryDevices[address] = &device{
			address:   address,
			state:     uint64(fusain.SysStateIdle),
			stateName: "IDLE",
			lastSeen:  time.Now(),
		}
		m.addLogEntry(fmt.Sprintf("Device discovered: %016X", address), false)
	}
	m.lastDeviceSeen = time.Now()
}

func (m *controlModel) handleStateData(packet *fusain.Packet, address uint64) {
	payloadMap := packet.PayloadMap()
	stateNames := []string{"INITIALIZING", "IDLE", "BLOWING", "PREHEAT", "PREHEAT_STAGE_2", "HEATING", "COOLING", "ERROR", "E_STOP"}

	state, hasState := fusain.GetMapUint(payloadMap, 2)
	if !hasState {
		return
	}

	stateName := "UNKNOWN"
	if int(state) < len(stateNames) {
		stateName = stateNames[state]
	}

	// Update device state
	if m.discoveryDone {
		for i := range m.devices {
			if m.devices[i].address == address {
				oldState := m.devices[i].stateName
				m.devices[i].state = state
				m.devices[i].stateName = stateName
				m.devices[i].lastSeen = time.Now()

				// Log state change
				if oldState != stateName {
					m.addLogEntry(fmt.Sprintf("Device %016X: %s -> %s", address, oldState, stateName), false)
				}

				// Update list
				m.updateDeviceList()
				break
			}
		}
	} else {
		// During discovery, update the discovery device
		if dev, exists := m.discoveryDevices[address]; exists {
			dev.state = state
			dev.stateName = stateName
			dev.lastSeen = time.Now()
		}
		m.lastDeviceSeen = time.Now()
	}
}

func (m *controlModel) parseTelemetryForDevice(packet *fusain.Packet, address uint64) {
	payloadMap := packet.PayloadMap()
	stateNames := []string{"INITIALIZING", "IDLE", "BLOWING", "PREHEAT", "PREHEAT_STAGE_2", "HEATING", "COOLING", "ERROR", "E_STOP"}

	// Ensure telemetry entry exists
	if m.lastTelemetry[address] == nil {
		m.lastTelemetry[address] = &telemetryData{timestamp: time.Now()}
	}
	telem := m.lastTelemetry[address]

	switch packet.Type() {
	case fusain.MsgStateData:
		state, hasState := fusain.GetMapUint(payloadMap, 2)
		if hasState {
			telem.state = state
			if int(state) < len(stateNames) {
				telem.stateName = stateNames[state]
			}
		}
		errorCode, _ := fusain.GetMapInt(payloadMap, 1)
		telem.errorCode = errorCode
		telem.timestamp = time.Now()

	case fusain.MsgPingResponse:
		uptime, ok := fusain.GetMapUint(payloadMap, 0)
		if ok {
			telem.uptime = uptime
			telem.hasUptime = true
		}

	case fusain.MsgMotorData:
		motorIdx, ok := fusain.GetMapInt(payloadMap, 0)
		if !ok || motorIdx < 0 {
			return
		}
		rpm, _ := fusain.GetMapInt(payloadMap, 2)
		target, _ := fusain.GetMapInt(payloadMap, 3)

		for len(telem.motorRPM) <= int(motorIdx) {
			telem.motorRPM = append(telem.motorRPM, 0)
		}
		for len(telem.motorTarget) <= int(motorIdx) {
			telem.motorTarget = append(telem.motorTarget, 0)
		}
		telem.motorRPM[motorIdx] = rpm
		telem.motorTarget[motorIdx] = target

	case fusain.MsgTempData:
		tempIdx, ok := fusain.GetMapInt(payloadMap, 0)
		if !ok || tempIdx < 0 {
			return
		}
		reading, _ := fusain.GetMapFloat(payloadMap, 2)

		for len(telem.temperatures) <= int(tempIdx) {
			telem.temperatures = append(telem.temperatures, 0)
		}
		telem.temperatures[tempIdx] = reading
	}
}

//////////////////////////////////////////////////////////////
// Commands
//////////////////////////////////////////////////////////////

func (m *controlModel) sendFanCommand() (tea.Model, tea.Cmd) {
	selected := m.getSelectedDevice()
	if selected == nil {
		return m, nil
	}

	// Parse RPM from input
	rpmStr := m.rpmInput.Value()
	if rpmStr == "" {
		rpmStr = m.rpmInput.Placeholder
	}

	rpm, err := strconv.ParseInt(rpmStr, 10, 64)
	if err != nil {
		m.addLogEntry(fmt.Sprintf("Invalid RPM value: %s", rpmStr), true)
		return m, nil
	}

	if rpm < minRPM || rpm > maxRPM {
		m.addLogEntry(fmt.Sprintf("RPM must be between %d and %d", minRPM, maxRPM), true)
		return m, nil
	}

	// Send fan command
	packet := fusain.NewStateCommand(selected.address, uint8(fusain.ModeFan), &rpm)
	wireBytes := fusain.MustEncodePacket(packet)
	conn := m.connMgr.getConn()
	if conn == nil {
		m.addLogEntry("Cannot send command: connection lost", true)
		return m, nil
	}
	_, err = conn.Write(wireBytes)
	if err != nil {
		m.addLogEntry(fmt.Sprintf("Failed to send command: %v", err), true)
		return m, nil
	}

	m.addLogEntry(fmt.Sprintf("Sent FAN command (RPM=%d) to %016X", rpm, selected.address), false)
	return m, nil
}

func (m *controlModel) sendIdleCommand() (tea.Model, tea.Cmd) {
	selected := m.getSelectedDevice()
	if selected == nil {
		return m, nil
	}

	// Send idle command
	packet := fusain.NewStateCommand(selected.address, uint8(fusain.ModeIdle), nil)
	wireBytes := fusain.MustEncodePacket(packet)
	conn := m.connMgr.getConn()
	if conn == nil {
		m.addLogEntry("Cannot send command: connection lost", true)
		return m, nil
	}
	_, err := conn.Write(wireBytes)
	if err != nil {
		m.addLogEntry(fmt.Sprintf("Failed to send command: %v", err), true)
		return m, nil
	}

	m.addLogEntry(fmt.Sprintf("Sent IDLE command to %016X", selected.address), false)
	return m, nil
}

//////////////////////////////////////////////////////////////
// Helpers
//////////////////////////////////////////////////////////////

func (m *controlModel) addLogEntry(message string, isError bool) {
	entry := errorLogEntry{
		timestamp: time.Now(),
		message:   message,
		isError:   isError,
	}
	m.errorLog = append(m.errorLog, entry)

	if len(m.errorLog) > m.maxLogEntries {
		m.errorLog = m.errorLog[len(m.errorLog)-m.maxLogEntries:]
	}
}

func (m *controlModel) getSelectedDevice() *device {
	if len(m.devices) == 0 {
		return nil
	}

	idx := m.deviceList.Index()
	if idx < 0 || idx >= len(m.devices) {
		return nil
	}

	return &m.devices[idx]
}

func (m *controlModel) finishDiscovery() {
	if m.discoveryDone {
		return
	}

	m.discoveryDone = true

	// Convert discovery map to device slice
	m.devices = make([]device, 0, len(m.discoveryDevices))
	for _, dev := range m.discoveryDevices {
		m.devices = append(m.devices, *dev)
	}

	// Update the list model
	m.updateDeviceList()

	m.addLogEntry(fmt.Sprintf("Discovery complete: %d heater(s)", len(m.devices)), false)

	// Send telemetry subscription to each discovered device
	for _, dev := range m.devices {
		m.sendTelemetrySubscription(dev.address)
	}

	// Focus device list if we have devices
	if len(m.devices) > 0 {
		m.focusedField = focusDeviceList
	}
}

func (m *controlModel) resetDiscovery() {
	m.discoveryDone = false
	m.discoveryDevices = make(map[uint64]*device)
	m.devices = make([]device, 0)
	m.lastDeviceSeen = time.Time{}
	m.lastTelemetry = make(map[uint64]*telemetryData)
	m.synchronized = false
	m.updateDeviceList()
}

func (m *controlModel) sendTelemetrySubscription(address uint64) {
	// Send DATA_SUBSCRIPTION to router (stateless address) to subscribe to this appliance
	packet := fusain.NewDataSubscription(fusain.AddressStateless, address)
	wireBytes := fusain.MustEncodePacket(packet)
	conn := m.connMgr.getConn()
	if conn == nil {
		m.addLogEntry(fmt.Sprintf("Failed to subscribe to %016X: connection lost", address), true)
		return
	}
	_, err := conn.Write(wireBytes)
	if err != nil {
		m.addLogEntry(fmt.Sprintf("Failed to subscribe to %016X: %v", address, err), true)
		return
	}
	m.addLogEntry(fmt.Sprintf("Subscribed to telemetry: %016X", address), false)
}

func (m *controlModel) sendPingRequest(address uint64) {
	// Send PING_REQUEST to device to get uptime
	packet := fusain.NewPingRequest(address)
	wireBytes := fusain.MustEncodePacket(packet)
	conn := m.connMgr.getConn()
	if conn == nil {
		return // Silently fail - connection lost is handled elsewhere
	}
	_, err := conn.Write(wireBytes)
	if err != nil {
		return // Silently fail - next tick will retry
	}
}

func (m *controlModel) updateDeviceList() {
	items := make([]list.Item, len(m.devices))
	for i, d := range m.devices {
		items[i] = d
	}
	m.deviceList.SetItems(items)
}

func (m *controlModel) updateListSize() {
	// Adjust list size based on terminal size
	listHeight := m.height / 3
	if listHeight < 5 {
		listHeight = 5
	}
	m.deviceList.SetSize(28, listHeight)
}
