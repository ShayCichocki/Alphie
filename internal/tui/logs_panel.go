package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LogLevel represents the severity of a log message.
type LogLevel string

const (
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
	LogLevelDebug LogLevel = "DEBUG"
)

// PanelLogEntry represents a single log entry in the logs panel.
type PanelLogEntry struct {
	Timestamp time.Time
	Level     LogLevel
	AgentID   string // Empty means global log
	TaskID    string
	Message   string
}

// LogsPanel displays a filterable, scrollable log viewer.
type LogsPanel struct {
	logs          []PanelLogEntry
	filter        string   // "all" or agent ID
	filterOptions []string // Available filter options
	filterIndex   int
	scrollOffset  int
	autoScroll    bool
	width         int
	height        int
	focused       bool
	maxLogs       int // Maximum log entries to keep

	// Progress aggregation: one live entry per agent instead of spam
	progressEntries map[string]*PanelLogEntry

	// Styles
	titleStyle   lipgloss.Style
	borderStyle  lipgloss.Style
	filterStyle  lipgloss.Style
	infoStyle    lipgloss.Style
	warnStyle    lipgloss.Style
	errorStyle   lipgloss.Style
	debugStyle   lipgloss.Style
	timeStyle    lipgloss.Style
	agentStyle   lipgloss.Style
	messageStyle lipgloss.Style
}

// NewLogsPanel creates a new LogsPanel instance.
func NewLogsPanel() *LogsPanel {
	return &LogsPanel{
		logs:            make([]PanelLogEntry, 0),
		filter:          "all",
		filterOptions:   []string{"all"},
		filterIndex:     0,
		autoScroll:      true,
		maxLogs:         1000,
		progressEntries: make(map[string]*PanelLogEntry),

		titleStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1),

		borderStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")),

		filterStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1),

		infoStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")), // Green

		warnStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")), // Orange

		errorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")), // Red

		debugStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")), // Gray

		timeStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),

		agentStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("63")), // Blue

		messageStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),
	}
}

// AddLog adds a new log entry.
func (p *LogsPanel) AddLog(entry PanelLogEntry) {
	p.logs = append(p.logs, entry)

	// Trim old logs if exceeding max
	if len(p.logs) > p.maxLogs {
		p.logs = p.logs[len(p.logs)-p.maxLogs:]
	}

	// Update filter options if new agent
	if entry.AgentID != "" {
		p.addFilterOption(entry.AgentID)
	}

	// Auto-scroll to bottom if enabled
	if p.autoScroll {
		p.scrollToBottom()
	}
}

// UpdateProgress updates the live progress entry for an agent (instead of creating new log lines).
// This prevents progress spam in the logs.
func (p *LogsPanel) UpdateProgress(agentID string, entry PanelLogEntry) {
	p.progressEntries[agentID] = &entry

	// Update filter options if new agent
	if agentID != "" {
		p.addFilterOption(agentID)
	}
}

// ClearProgress removes the progress entry for an agent (when task completes/fails).
func (p *LogsPanel) ClearProgress(agentID string) {
	delete(p.progressEntries, agentID)
}

// addFilterOption adds an agent ID to filter options if not already present.
func (p *LogsPanel) addFilterOption(agentID string) {
	// Truncate agent ID for display
	displayID := agentID
	if len(displayID) > 8 {
		displayID = displayID[:8]
	}

	for _, opt := range p.filterOptions {
		if opt == displayID {
			return
		}
	}
	p.filterOptions = append(p.filterOptions, displayID)
}

// SetSize updates the panel dimensions.
func (p *LogsPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetFocused sets whether this panel has keyboard focus.
func (p *LogsPanel) SetFocused(focused bool) {
	p.focused = focused
}

// Update handles input messages.
func (p *LogsPanel) Update(msg tea.Msg) (*LogsPanel, tea.Cmd) {
	if !p.focused {
		return p, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.scrollOffset > 0 {
				p.scrollOffset--
				p.autoScroll = false
			}
		case "down", "j":
			filtered := p.filteredLogs()
			visibleLines := p.visibleLines()
			if p.scrollOffset < len(filtered)-visibleLines {
				p.scrollOffset++
			}
		case "f":
			// Cycle through filters
			p.filterIndex = (p.filterIndex + 1) % len(p.filterOptions)
			p.filter = p.filterOptions[p.filterIndex]
			p.scrollToBottom()
		case "g":
			// Go to top
			p.scrollOffset = 0
			p.autoScroll = false
		case "G":
			// Go to bottom and enable auto-scroll
			p.scrollToBottom()
			p.autoScroll = true
		case "a":
			// Toggle auto-scroll
			p.autoScroll = !p.autoScroll
			if p.autoScroll {
				p.scrollToBottom()
			}
		}
	}

	return p, nil
}

// visibleLines returns the number of visible log lines.
func (p *LogsPanel) visibleLines() int {
	lines := p.height - 5 // Account for title, filter, borders
	if lines < 1 {
		lines = 1
	}
	return lines
}

// scrollToBottom scrolls to the bottom of the log.
func (p *LogsPanel) scrollToBottom() {
	filtered := p.filteredLogs()
	visibleLines := p.visibleLines()
	p.scrollOffset = len(filtered) - visibleLines
	if p.scrollOffset < 0 {
		p.scrollOffset = 0
	}
}

// filteredLogs returns logs filtered by current filter setting.
func (p *LogsPanel) filteredLogs() []PanelLogEntry {
	if p.filter == "all" {
		return p.logs
	}

	filtered := make([]PanelLogEntry, 0)
	for _, log := range p.logs {
		// Match truncated agent ID
		agentID := log.AgentID
		if len(agentID) > 8 {
			agentID = agentID[:8]
		}
		if agentID == p.filter {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

// View renders the logs panel.
func (p *LogsPanel) View() string {
	var b strings.Builder

	// Title with filter indicator
	title := "Logs"
	if p.focused {
		title = "[Logs]"
	}
	b.WriteString(p.titleStyle.Render(title))

	// Filter indicator
	filterText := fmt.Sprintf(" [%s]", p.filter)
	if p.autoScroll {
		filterText += " (auto)"
	}
	b.WriteString(p.filterStyle.Render(filterText))
	b.WriteString("\n")

	// Get filtered logs
	filtered := p.filteredLogs()

	// Reserve lines for progress entries at bottom
	progressLines := len(p.progressEntries)
	visibleLines := p.visibleLines() - progressLines
	if visibleLines < 1 {
		visibleLines = 1
	}

	if len(filtered) == 0 && progressLines == 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true).
			Render("  No logs"))
	} else {
		// Calculate visible range for regular logs
		endIdx := p.scrollOffset + visibleLines
		if endIdx > len(filtered) {
			endIdx = len(filtered)
		}
		startIdx := p.scrollOffset
		if startIdx < 0 {
			startIdx = 0
		}

		// Render visible logs
		for i := startIdx; i < endIdx; i++ {
			line := p.renderLogLine(filtered[i])
			b.WriteString(line)
			b.WriteString("\n")
		}

		// Scroll position indicator (only if there are logs beyond visible)
		if len(filtered) > visibleLines {
			scrollPct := float64(p.scrollOffset) / float64(len(filtered)-visibleLines) * 100
			scrollIndicator := fmt.Sprintf(" [%d/%d %.0f%%]", endIdx, len(filtered), scrollPct)
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Render(scrollIndicator))
			b.WriteString("\n")
		}

		// Render live progress entries at bottom (one line per agent, always visible)
		if progressLines > 0 {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Render("─── live ───"))
			b.WriteString("\n")
			for _, entry := range p.progressEntries {
				line := p.renderLogLine(*entry)
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
	}

	// Apply border and size constraints
	content := b.String()
	borderColor := lipgloss.Color("240")
	if p.focused {
		borderColor = lipgloss.Color("63") // Blue when focused
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(p.width - 2).
		Height(p.height - 2).
		Render(content)
}

// renderLogLine renders a single log entry.
func (p *LogsPanel) renderLogLine(entry PanelLogEntry) string {
	var parts []string

	// Timestamp
	timeStr := entry.Timestamp.Format("15:04:05")
	parts = append(parts, p.timeStyle.Render(timeStr))

	// Level indicator
	levelStyle := p.infoStyle
	levelIcon := "I"
	switch entry.Level {
	case LogLevelWarn:
		levelStyle = p.warnStyle
		levelIcon = "W"
	case LogLevelError:
		levelStyle = p.errorStyle
		levelIcon = "E"
	case LogLevelDebug:
		levelStyle = p.debugStyle
		levelIcon = "D"
	}
	parts = append(parts, levelStyle.Render(levelIcon))

	// Agent ID (if present and not filtered)
	if entry.AgentID != "" && p.filter == "all" {
		agentID := entry.AgentID
		if len(agentID) > 6 {
			agentID = agentID[:6]
		}
		parts = append(parts, p.agentStyle.Render("["+agentID+"]"))
	}

	// Message (truncated to fit)
	maxMsgLen := p.width - 25
	if maxMsgLen < 20 {
		maxMsgLen = 20
	}
	msg := entry.Message
	if len(msg) > maxMsgLen {
		msg = msg[:maxMsgLen-3] + "..."
	}
	parts = append(parts, p.messageStyle.Render(msg))

	return strings.Join(parts, " ")
}

// LogCount returns the total number of logs.
func (p *LogsPanel) LogCount() int {
	return len(p.logs)
}

// FilteredCount returns the number of logs matching current filter.
func (p *LogsPanel) FilteredCount() int {
	return len(p.filteredLogs())
}

// CurrentFilter returns the current filter value.
func (p *LogsPanel) CurrentFilter() string {
	return p.filter
}
