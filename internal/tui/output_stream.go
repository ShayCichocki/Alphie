package tui

import (
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ShayCichocki/alphie/internal/agent"
)

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
