// Package tui provides the terminal user interface for Alphie.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// OutputLineMsg is sent when a new line is appended to an agent's output.
type OutputLineMsg struct {
	AgentID string
	Line    string
}

// OutputView displays a scrollable view of an agent's output.
type OutputView struct {
	// agentID is the ID of the agent whose output is being displayed.
	agentID string
	// lines contains all output lines from the agent.
	lines []string
	// scrollOffset is the current scroll position (0 = top).
	scrollOffset int
	// width is the viewport width in characters.
	width int
	// height is the viewport height in lines.
	height int
	// autoScroll enables automatic scrolling to bottom on new content.
	autoScroll bool
	// streamer provides live output streaming support.
	streamer *LiveStreamer
}

// NewOutputView creates a new OutputView instance.
func NewOutputView() *OutputView {
	return &OutputView{
		lines:        make([]string, 0),
		scrollOffset: 0,
		width:        80,
		height:       20,
		autoScroll:   true,
	}
}

// Update handles input messages and returns the updated view and any commands.
func (o *OutputView) Update(msg tea.Msg) (*OutputView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			o.ScrollUp()
			// User scrolled up: disable auto-scroll
			o.autoScroll = false
			if o.streamer != nil {
				o.streamer.SetAutoScroll(false)
			}
		case "down", "j":
			o.ScrollDown()
		case "pgup", "b":
			o.ScrollPageUp()
			// User scrolled up: disable auto-scroll
			o.autoScroll = false
			if o.streamer != nil {
				o.streamer.SetAutoScroll(false)
			}
		case "pgdown", " ":
			o.ScrollPageDown()
		case "f":
			// 'f' key: toggle follow mode (re-enable auto-scroll and scroll to bottom)
			o.autoScroll = true
			if o.streamer != nil {
				o.streamer.SetAutoScroll(true)
			}
			o.scrollToBottom()
		case "home", "g":
			o.scrollOffset = 0
			// User scrolled to top: disable auto-scroll
			o.autoScroll = false
			if o.streamer != nil {
				o.streamer.SetAutoScroll(false)
			}
		case "end", "G":
			o.scrollToBottom()
			// User scrolled to bottom: re-enable auto-scroll
			o.autoScroll = true
			if o.streamer != nil {
				o.streamer.SetAutoScroll(true)
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Agent switching is handled by parent; return key for delegation
			return o, nil
		}

	case tea.WindowSizeMsg:
		o.width = msg.Width
		o.height = msg.Height - 4 // Reserve space for header/footer

	case OutputLineMsg:
		if msg.AgentID == o.agentID {
			o.AppendLine(msg.Line)
		}

	case LiveStreamUpdateMsg:
		// Update lines from the streamer if active
		if o.streamer != nil && msg.AgentID == o.agentID {
			o.lines = o.streamer.Lines()
			if o.autoScroll {
				o.scrollToBottom()
			}
		}
	}

	return o, nil
}

// View renders the output view as a string.
func (o *OutputView) View() string {
	if o.agentID == "" {
		return "No agent selected. Press 1-9 to select an agent."
	}

	if len(o.lines) == 0 {
		return "Waiting for output from agent " + o.agentID[:min(8, len(o.agentID))] + "..."
	}

	// Wrap lines to viewport width
	wrappedLines := o.wrapLines()

	// Calculate visible range
	totalLines := len(wrappedLines)
	if o.scrollOffset > totalLines-o.height {
		o.scrollOffset = max(0, totalLines-o.height)
	}

	start := o.scrollOffset
	end := min(start+o.height, totalLines)

	// Build visible content
	var sb strings.Builder
	for i := start; i < end; i++ {
		sb.WriteString(wrappedLines[i])
		if i < end-1 {
			sb.WriteString("\n")
		}
	}

	// Add scroll indicator
	scrollInfo := o.scrollIndicator(totalLines)
	if scrollInfo != "" {
		sb.WriteString("\n")
		sb.WriteString(scrollInfo)
	}

	return sb.String()
}

// SetAgent switches to displaying output from a different agent.
func (o *OutputView) SetAgent(agentID string) {
	if o.agentID != agentID {
		o.agentID = agentID
		o.lines = make([]string, 0)
		o.scrollOffset = 0
	}
}

// GetAgentID returns the current agent ID.
func (o *OutputView) GetAgentID() string {
	return o.agentID
}

// AppendLine adds a new line to the output.
func (o *OutputView) AppendLine(line string) {
	o.lines = append(o.lines, line)

	// Auto-scroll to bottom if autoScroll is enabled
	if o.autoScroll {
		o.scrollToBottom()
	}
}

// ScrollUp moves the viewport up by one line.
func (o *OutputView) ScrollUp() {
	if o.scrollOffset > 0 {
		o.scrollOffset--
	}
}

// ScrollDown moves the viewport down by one line.
func (o *OutputView) ScrollDown() {
	wrappedLines := o.wrapLines()
	maxOffset := max(0, len(wrappedLines)-o.height)
	if o.scrollOffset < maxOffset {
		o.scrollOffset++
	}
}

// ScrollPageUp moves the viewport up by one page.
func (o *OutputView) ScrollPageUp() {
	o.scrollOffset -= o.height
	if o.scrollOffset < 0 {
		o.scrollOffset = 0
	}
}

// ScrollPageDown moves the viewport down by one page.
func (o *OutputView) ScrollPageDown() {
	wrappedLines := o.wrapLines()
	maxOffset := max(0, len(wrappedLines)-o.height)
	o.scrollOffset += o.height
	if o.scrollOffset > maxOffset {
		o.scrollOffset = maxOffset
	}
}

// SetSize updates the viewport dimensions.
func (o *OutputView) SetSize(width, height int) {
	o.width = width
	o.height = height
}

// scrollToBottom moves the viewport to show the last lines.
func (o *OutputView) scrollToBottom() {
	wrappedLines := o.wrapLines()
	o.scrollOffset = max(0, len(wrappedLines)-o.height)
}

// wrapLines wraps all lines to fit the viewport width.
func (o *OutputView) wrapLines() []string {
	if o.width <= 0 {
		return o.lines
	}

	var wrapped []string
	for _, line := range o.lines {
		if len(line) <= o.width {
			wrapped = append(wrapped, line)
			continue
		}

		// Wrap long lines
		for len(line) > o.width {
			// Try to break at a space for cleaner wrapping
			breakPoint := o.width
			for i := o.width - 1; i > o.width/2; i-- {
				if line[i] == ' ' {
					breakPoint = i + 1
					break
				}
			}
			wrapped = append(wrapped, line[:breakPoint])
			line = line[breakPoint:]
		}
		if len(line) > 0 {
			wrapped = append(wrapped, line)
		}
	}

	return wrapped
}

// scrollIndicator returns a string showing scroll position and follow mode status.
func (o *OutputView) scrollIndicator(totalLines int) string {
	if totalLines <= o.height {
		return ""
	}

	percent := 0
	maxOffset := totalLines - o.height
	if maxOffset > 0 {
		percent = (o.scrollOffset * 100) / maxOffset
	}

	followIndicator := ""
	if o.autoScroll {
		followIndicator = " [FOLLOW]"
	} else {
		followIndicator = " [PAUSED - press 'f' to follow]"
	}

	return strings.Repeat(" ", max(0, o.width/2-15)) +
		"--- " + string(rune('0'+percent/100%10)) +
		string(rune('0'+percent/10%10)) +
		string(rune('0'+percent%10)) + "% ---" + followIndicator
}

// ClearLines removes all output lines.
func (o *OutputView) ClearLines() {
	o.lines = make([]string, 0)
	o.scrollOffset = 0
	o.autoScroll = true
}

// SetStreamer attaches a LiveStreamer to this output view.
func (o *OutputView) SetStreamer(streamer *LiveStreamer) {
	o.streamer = streamer
	if streamer != nil {
		o.autoScroll = streamer.IsAutoScroll()
		o.lines = streamer.Lines()
		if o.autoScroll {
			o.scrollToBottom()
		}
	}
}

// GetStreamer returns the attached LiveStreamer, if any.
func (o *OutputView) GetStreamer() *LiveStreamer {
	return o.streamer
}

// IsAutoScroll returns whether auto-scroll is enabled.
func (o *OutputView) IsAutoScroll() bool {
	return o.autoScroll
}

// SetAutoScroll enables or disables auto-scroll.
func (o *OutputView) SetAutoScroll(enabled bool) {
	o.autoScroll = enabled
	if o.streamer != nil {
		o.streamer.SetAutoScroll(enabled)
	}
}

// LineCount returns the number of raw (unwrapped) lines.
func (o *OutputView) LineCount() int {
	return len(o.lines)
}
