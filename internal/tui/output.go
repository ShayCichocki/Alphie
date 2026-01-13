// Package tui provides the terminal user interface for Alphie.
package tui

import (
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/shayc/alphie/internal/agent"
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

// DefaultBufferSize is the default size for the ring buffer.
const DefaultBufferSize = 10000

// DefaultRateLimit is the default rate limit for updates.
const DefaultRateLimit = 16 * time.Millisecond // ~60 FPS

// RingBuffer provides efficient fixed-size line storage with O(1) operations.
// When the buffer is full, the oldest lines are automatically discarded.
type RingBuffer struct {
	data  []string
	size  int
	head  int // Write position (next write goes here)
	tail  int // Read position (oldest element)
	count int // Number of elements currently stored
}

// NewRingBuffer creates a new RingBuffer with the specified capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = DefaultBufferSize
	}
	return &RingBuffer{
		data: make([]string, capacity),
		size: capacity,
	}
}

// Append adds a line to the buffer. If the buffer is full, the oldest line is overwritten.
func (rb *RingBuffer) Append(line string) {
	rb.data[rb.head] = line
	rb.head = (rb.head + 1) % rb.size

	if rb.count < rb.size {
		rb.count++
	} else {
		// Buffer is full, move tail forward (discard oldest)
		rb.tail = (rb.tail + 1) % rb.size
	}
}

// Lines returns all lines in the buffer in order from oldest to newest.
func (rb *RingBuffer) Lines() []string {
	if rb.count == 0 {
		return nil
	}

	result := make([]string, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.tail + i) % rb.size
		result[i] = rb.data[idx]
	}
	return result
}

// Count returns the number of lines currently in the buffer.
func (rb *RingBuffer) Count() int {
	return rb.count
}

// Clear removes all lines from the buffer.
func (rb *RingBuffer) Clear() {
	rb.head = 0
	rb.tail = 0
	rb.count = 0
}

// Capacity returns the maximum number of lines the buffer can hold.
func (rb *RingBuffer) Capacity() int {
	return rb.size
}

// LiveStreamUpdateMsg is sent when the LiveStreamer has new content to display.
type LiveStreamUpdateMsg struct {
	AgentID string
}

// LiveStreamer handles real-time output streaming from an agent with
// buffer management, rate limiting, and auto-scroll functionality.
type LiveStreamer struct {
	agentID    string
	buffer     *RingBuffer
	autoScroll bool
	rateLimit  time.Duration
	lastUpdate time.Time
	mu         sync.Mutex

	// textBuffer accumulates text until a newline is received.
	textBuffer strings.Builder
}

// NewLiveStreamer creates a new LiveStreamer for the specified agent.
func NewLiveStreamer(agentID string, bufferSize int) *LiveStreamer {
	if bufferSize <= 0 {
		bufferSize = DefaultBufferSize
	}
	return &LiveStreamer{
		agentID:    agentID,
		buffer:     NewRingBuffer(bufferSize),
		autoScroll: true,
		rateLimit:  DefaultRateLimit,
	}
}

// Stream processes an API stream event and returns a tea.Cmd if an update is needed.
// It parses text from the event, applies rate limiting, and appends to the buffer.
func (ls *LiveStreamer) Stream(event agent.APIStreamEvent) tea.Cmd {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Extract text from the event based on event type
	text := ls.extractText(event)
	if text == "" {
		return nil
	}

	// Append text to the text buffer and extract complete lines
	ls.textBuffer.WriteString(text)
	buffered := ls.textBuffer.String()

	// Process complete lines (separated by newlines)
	for {
		idx := strings.Index(buffered, "\n")
		if idx == -1 {
			break
		}

		line := buffered[:idx]
		buffered = buffered[idx+1:]
		ls.buffer.Append(line)
	}

	// Keep remaining partial line in buffer
	ls.textBuffer.Reset()
	ls.textBuffer.WriteString(buffered)

	// Rate limiting: check if enough time has passed since last update
	now := time.Now()
	if now.Sub(ls.lastUpdate) < ls.rateLimit {
		return nil
	}
	ls.lastUpdate = now

	// Return update command to trigger UI refresh
	agentID := ls.agentID
	return func() tea.Msg {
		return LiveStreamUpdateMsg{AgentID: agentID}
	}
}

// extractText extracts displayable text from an API stream event.
func (ls *LiveStreamer) extractText(event agent.APIStreamEvent) string {
	switch event.Type {
	case agent.APIEventContentBlockDelta:
		if event.Delta != nil && event.Delta.Text != "" {
			return event.Delta.Text
		}
	case agent.APIEventContentBlockStart:
		if event.ContentBlock != nil && event.ContentBlock.Text != "" {
			return event.ContentBlock.Text
		}
	}
	return ""
}

// Flush flushes any remaining partial line in the text buffer to the ring buffer.
func (ls *LiveStreamer) Flush() {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.textBuffer.Len() > 0 {
		ls.buffer.Append(ls.textBuffer.String())
		ls.textBuffer.Reset()
	}
}

// Lines returns all lines currently in the buffer.
func (ls *LiveStreamer) Lines() []string {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.buffer.Lines()
}

// SetAutoScroll enables or disables auto-scrolling.
func (ls *LiveStreamer) SetAutoScroll(enabled bool) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.autoScroll = enabled
}

// IsAutoScroll returns whether auto-scroll is currently enabled.
func (ls *LiveStreamer) IsAutoScroll() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.autoScroll
}

// ToggleAutoScroll toggles the auto-scroll setting and returns the new state.
func (ls *LiveStreamer) ToggleAutoScroll() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.autoScroll = !ls.autoScroll
	return ls.autoScroll
}

// SetRateLimit sets the minimum time between UI updates.
func (ls *LiveStreamer) SetRateLimit(d time.Duration) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.rateLimit = d
}

// GetRateLimit returns the current rate limit duration.
func (ls *LiveStreamer) GetRateLimit() time.Duration {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.rateLimit
}

// AgentID returns the agent ID associated with this streamer.
func (ls *LiveStreamer) AgentID() string {
	return ls.agentID
}

// Clear clears the buffer and text buffer.
func (ls *LiveStreamer) Clear() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.buffer.Clear()
	ls.textBuffer.Reset()
}

// LineCount returns the number of lines in the buffer.
func (ls *LiveStreamer) LineCount() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.buffer.Count()
}
