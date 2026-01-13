// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// APIEventType represents the type of streaming event from the Anthropic API.
type APIEventType string

const (
	APIEventMessageStart      APIEventType = "message_start"
	APIEventContentBlockStart APIEventType = "content_block_start"
	APIEventContentBlockDelta APIEventType = "content_block_delta"
	APIEventContentBlockStop  APIEventType = "content_block_stop"
	APIEventMessageDelta      APIEventType = "message_delta"
	APIEventMessageStop       APIEventType = "message_stop"
	APIEventPing              APIEventType = "ping"
)

// ContentBlockType represents the type of content block.
type ContentBlockType string

const (
	ContentBlockText          ContentBlockType = "text"
	ContentBlockToolUse       ContentBlockType = "tool_use"
	ContentBlockThinking      ContentBlockType = "thinking"
	ContentBlockServerToolUse ContentBlockType = "server_tool_use"
)

// APIStreamEvent represents a parsed streaming event from the Anthropic API.
type APIStreamEvent struct {
	Type         APIEventType  `json:"type"`
	Index        int           `json:"index,omitempty"`
	Message      *Message      `json:"message,omitempty"`
	Delta        *Delta        `json:"delta,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`
}

// Message represents the message structure in streaming events.
type Message struct {
	ID           string         `json:"id,omitempty"`
	Type         string         `json:"type,omitempty"`
	Role         string         `json:"role,omitempty"`
	Content      []ContentBlock `json:"content,omitempty"`
	Model        string         `json:"model,omitempty"`
	StopReason   string         `json:"stop_reason,omitempty"`
	StopSequence string         `json:"stop_sequence,omitempty"`
	Usage        *Usage         `json:"usage,omitempty"`
}

// Usage represents token usage information.
type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// Delta represents incremental updates in streaming events.
type Delta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

// ContentBlock represents a content block in the message.
type ContentBlock struct {
	Type  ContentBlockType `json:"type"`
	ID    string           `json:"id,omitempty"`
	Name  string           `json:"name,omitempty"`
	Text  string           `json:"text,omitempty"`
	Input json.RawMessage  `json:"input,omitempty"`
}

// ToolInputBuffer buffers partial JSON for tool input streaming.
type ToolInputBuffer struct {
	buffer strings.Builder
}

// NewToolInputBuffer creates a new ToolInputBuffer.
func NewToolInputBuffer() *ToolInputBuffer {
	return &ToolInputBuffer{}
}

// Append adds partial JSON to the buffer.
func (b *ToolInputBuffer) Append(partialJSON string) {
	b.buffer.WriteString(partialJSON)
}

// String returns the current buffered content.
func (b *ToolInputBuffer) String() string {
	return b.buffer.String()
}

// Reset clears the buffer.
func (b *ToolInputBuffer) Reset() {
	b.buffer.Reset()
}

// TryParse attempts to parse the buffered JSON into the target.
// Returns true if parsing succeeded, false if more data is needed.
func (b *ToolInputBuffer) TryParse(target interface{}) bool {
	content := b.buffer.String()
	if content == "" {
		return false
	}
	return json.Unmarshal([]byte(content), target) == nil
}

// ParseAPIStream parses NDJSON (newline-delimited JSON) from an io.Reader
// and returns a channel of APIStreamEvents for Anthropic API streaming.
func ParseAPIStream(r io.Reader) <-chan APIStreamEvent {
	events := make(chan APIStreamEvent)

	go func() {
		defer close(events)

		scanner := bufio.NewScanner(r)
		// Set a larger buffer for potentially long JSON lines
		const maxScanTokenSize = 1024 * 1024 // 1MB
		buf := make([]byte, maxScanTokenSize)
		scanner.Buffer(buf, maxScanTokenSize)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var event APIStreamEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				// Skip malformed JSON lines
				continue
			}

			events <- event
		}
	}()

	return events
}

// StreamProcessor handles streaming events and maintains state.
type StreamProcessor struct {
	toolInputBuffers map[int]*ToolInputBuffer
}

// NewStreamProcessor creates a new StreamProcessor.
func NewStreamProcessor() *StreamProcessor {
	return &StreamProcessor{
		toolInputBuffers: make(map[int]*ToolInputBuffer),
	}
}

// ProcessEvent processes a single API stream event and updates internal state.
// For tool_use content blocks, it buffers partial JSON from deltas.
func (p *StreamProcessor) ProcessEvent(event APIStreamEvent) {
	switch event.Type {
	case APIEventContentBlockStart:
		if event.ContentBlock != nil && event.ContentBlock.Type == ContentBlockToolUse {
			p.toolInputBuffers[event.Index] = NewToolInputBuffer()
		}
	case APIEventContentBlockDelta:
		if event.Delta != nil && event.Delta.PartialJSON != "" {
			if buf, ok := p.toolInputBuffers[event.Index]; ok {
				buf.Append(event.Delta.PartialJSON)
			}
		}
	case APIEventContentBlockStop:
		// Buffer remains available for retrieval until explicitly cleared
	}
}

// GetToolInput returns the buffered tool input for a given index.
func (p *StreamProcessor) GetToolInput(index int) *ToolInputBuffer {
	return p.toolInputBuffers[index]
}

// ClearToolInput removes the tool input buffer for a given index.
func (p *StreamProcessor) ClearToolInput(index int) {
	delete(p.toolInputBuffers, index)
}
