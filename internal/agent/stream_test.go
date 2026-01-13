package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAPIEventTypeConstants(t *testing.T) {
	tests := []struct {
		eventType APIEventType
		expected  string
	}{
		{APIEventMessageStart, "message_start"},
		{APIEventContentBlockStart, "content_block_start"},
		{APIEventContentBlockDelta, "content_block_delta"},
		{APIEventContentBlockStop, "content_block_stop"},
		{APIEventMessageDelta, "message_delta"},
		{APIEventMessageStop, "message_stop"},
		{APIEventPing, "ping"},
	}

	for _, tt := range tests {
		if string(tt.eventType) != tt.expected {
			t.Errorf("APIEventType = %q, want %q", tt.eventType, tt.expected)
		}
	}
}

func TestContentBlockTypeConstants(t *testing.T) {
	tests := []struct {
		blockType ContentBlockType
		expected  string
	}{
		{ContentBlockText, "text"},
		{ContentBlockToolUse, "tool_use"},
		{ContentBlockThinking, "thinking"},
		{ContentBlockServerToolUse, "server_tool_use"},
	}

	for _, tt := range tests {
		if string(tt.blockType) != tt.expected {
			t.Errorf("ContentBlockType = %q, want %q", tt.blockType, tt.expected)
		}
	}
}

func TestParseAPIStreamMessageStart(t *testing.T) {
	input := `{"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":100,"output_tokens":10}}}`

	reader := strings.NewReader(input)
	events := ParseAPIStream(reader)

	var receivedEvents []APIStreamEvent
	for event := range events {
		receivedEvents = append(receivedEvents, event)
	}

	if len(receivedEvents) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(receivedEvents))
	}

	event := receivedEvents[0]
	if event.Type != APIEventMessageStart {
		t.Errorf("Type = %q, want %q", event.Type, APIEventMessageStart)
	}
	if event.Message == nil {
		t.Fatal("Message is nil")
	}
	if event.Message.ID != "msg_123" {
		t.Errorf("Message.ID = %q, want %q", event.Message.ID, "msg_123")
	}
	if event.Message.Role != "assistant" {
		t.Errorf("Message.Role = %q, want %q", event.Message.Role, "assistant")
	}
}

func TestParseAPIStreamContentBlockStart(t *testing.T) {
	input := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`

	reader := strings.NewReader(input)
	events := ParseAPIStream(reader)

	var receivedEvents []APIStreamEvent
	for event := range events {
		receivedEvents = append(receivedEvents, event)
	}

	if len(receivedEvents) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(receivedEvents))
	}

	event := receivedEvents[0]
	if event.Type != APIEventContentBlockStart {
		t.Errorf("Type = %q, want %q", event.Type, APIEventContentBlockStart)
	}
	if event.Index != 0 {
		t.Errorf("Index = %d, want 0", event.Index)
	}
	if event.ContentBlock == nil {
		t.Fatal("ContentBlock is nil")
	}
	if event.ContentBlock.Type != ContentBlockText {
		t.Errorf("ContentBlock.Type = %q, want %q", event.ContentBlock.Type, ContentBlockText)
	}
}

func TestParseAPIStreamContentBlockDelta(t *testing.T) {
	input := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello world"}}`

	reader := strings.NewReader(input)
	events := ParseAPIStream(reader)

	var receivedEvents []APIStreamEvent
	for event := range events {
		receivedEvents = append(receivedEvents, event)
	}

	if len(receivedEvents) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(receivedEvents))
	}

	event := receivedEvents[0]
	if event.Type != APIEventContentBlockDelta {
		t.Errorf("Type = %q, want %q", event.Type, APIEventContentBlockDelta)
	}
	if event.Delta == nil {
		t.Fatal("Delta is nil")
	}
	if event.Delta.Text != "Hello world" {
		t.Errorf("Delta.Text = %q, want %q", event.Delta.Text, "Hello world")
	}
}

func TestParseAPIStreamContentBlockStop(t *testing.T) {
	input := `{"type":"content_block_stop","index":0}`

	reader := strings.NewReader(input)
	events := ParseAPIStream(reader)

	var receivedEvents []APIStreamEvent
	for event := range events {
		receivedEvents = append(receivedEvents, event)
	}

	if len(receivedEvents) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(receivedEvents))
	}

	event := receivedEvents[0]
	if event.Type != APIEventContentBlockStop {
		t.Errorf("Type = %q, want %q", event.Type, APIEventContentBlockStop)
	}
	if event.Index != 0 {
		t.Errorf("Index = %d, want 0", event.Index)
	}
}

func TestParseAPIStreamMessageDelta(t *testing.T) {
	input := `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`

	reader := strings.NewReader(input)
	events := ParseAPIStream(reader)

	var receivedEvents []APIStreamEvent
	for event := range events {
		receivedEvents = append(receivedEvents, event)
	}

	if len(receivedEvents) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(receivedEvents))
	}

	event := receivedEvents[0]
	if event.Type != APIEventMessageDelta {
		t.Errorf("Type = %q, want %q", event.Type, APIEventMessageDelta)
	}
	if event.Delta == nil {
		t.Fatal("Delta is nil")
	}
	if event.Delta.StopReason != "end_turn" {
		t.Errorf("Delta.StopReason = %q, want %q", event.Delta.StopReason, "end_turn")
	}
}

func TestParseAPIStreamMultipleEvents(t *testing.T) {
	input := `{"type":"message_start","message":{"id":"msg_123"}}
{"type":"content_block_start","index":0,"content_block":{"type":"text"}}
{"type":"content_block_delta","index":0,"delta":{"text":"Hello"}}
{"type":"content_block_stop","index":0}
{"type":"message_stop"}`

	reader := strings.NewReader(input)
	events := ParseAPIStream(reader)

	var receivedEvents []APIStreamEvent
	for event := range events {
		receivedEvents = append(receivedEvents, event)
	}

	if len(receivedEvents) != 5 {
		t.Fatalf("Expected 5 events, got %d", len(receivedEvents))
	}

	expectedTypes := []APIEventType{
		APIEventMessageStart,
		APIEventContentBlockStart,
		APIEventContentBlockDelta,
		APIEventContentBlockStop,
		APIEventMessageStop,
	}

	for i, expected := range expectedTypes {
		if receivedEvents[i].Type != expected {
			t.Errorf("Event[%d].Type = %q, want %q", i, receivedEvents[i].Type, expected)
		}
	}
}

func TestParseAPIStreamSkipsEmptyLines(t *testing.T) {
	input := `{"type":"message_start","message":{}}

{"type":"message_stop"}
`

	reader := strings.NewReader(input)
	events := ParseAPIStream(reader)

	var receivedEvents []APIStreamEvent
	for event := range events {
		receivedEvents = append(receivedEvents, event)
	}

	if len(receivedEvents) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(receivedEvents))
	}
}

func TestParseAPIStreamSkipsMalformedJSON(t *testing.T) {
	input := `{"type":"message_start","message":{}}
invalid json line
{"type":"message_stop"}`

	reader := strings.NewReader(input)
	events := ParseAPIStream(reader)

	var receivedEvents []APIStreamEvent
	for event := range events {
		receivedEvents = append(receivedEvents, event)
	}

	// Should have 2 events (malformed line skipped)
	if len(receivedEvents) != 2 {
		t.Fatalf("Expected 2 events (malformed skipped), got %d", len(receivedEvents))
	}
}

func TestParseAPIStreamToolUseContentBlock(t *testing.T) {
	input := `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_123","name":"read_file"}}`

	reader := strings.NewReader(input)
	events := ParseAPIStream(reader)

	var receivedEvents []APIStreamEvent
	for event := range events {
		receivedEvents = append(receivedEvents, event)
	}

	if len(receivedEvents) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(receivedEvents))
	}

	event := receivedEvents[0]
	if event.ContentBlock == nil {
		t.Fatal("ContentBlock is nil")
	}
	if event.ContentBlock.Type != ContentBlockToolUse {
		t.Errorf("ContentBlock.Type = %q, want %q", event.ContentBlock.Type, ContentBlockToolUse)
	}
	if event.ContentBlock.ID != "tool_123" {
		t.Errorf("ContentBlock.ID = %q, want %q", event.ContentBlock.ID, "tool_123")
	}
	if event.ContentBlock.Name != "read_file" {
		t.Errorf("ContentBlock.Name = %q, want %q", event.ContentBlock.Name, "read_file")
	}
}

func TestToolInputBuffer(t *testing.T) {
	buf := NewToolInputBuffer()

	if buf.String() != "" {
		t.Errorf("Initial buffer content = %q, want empty", buf.String())
	}

	buf.Append(`{"path":`)
	buf.Append(`"/tmp/file.txt"}`)

	if buf.String() != `{"path":"/tmp/file.txt"}` {
		t.Errorf("Buffer content = %q, want %q", buf.String(), `{"path":"/tmp/file.txt"}`)
	}
}

func TestToolInputBufferReset(t *testing.T) {
	buf := NewToolInputBuffer()
	buf.Append("some content")
	buf.Reset()

	if buf.String() != "" {
		t.Errorf("Buffer after Reset = %q, want empty", buf.String())
	}
}

func TestToolInputBufferTryParse(t *testing.T) {
	buf := NewToolInputBuffer()
	buf.Append(`{"path":"/tmp/file.txt","line":42}`)

	var target struct {
		Path string `json:"path"`
		Line int    `json:"line"`
	}

	if !buf.TryParse(&target) {
		t.Error("TryParse() returned false for valid JSON")
	}
	if target.Path != "/tmp/file.txt" {
		t.Errorf("Parsed path = %q, want %q", target.Path, "/tmp/file.txt")
	}
	if target.Line != 42 {
		t.Errorf("Parsed line = %d, want 42", target.Line)
	}
}

func TestToolInputBufferTryParseIncomplete(t *testing.T) {
	buf := NewToolInputBuffer()
	buf.Append(`{"path":"/tmp/file`)

	var target struct {
		Path string `json:"path"`
	}

	if buf.TryParse(&target) {
		t.Error("TryParse() returned true for incomplete JSON")
	}
}

func TestToolInputBufferTryParseEmpty(t *testing.T) {
	buf := NewToolInputBuffer()

	var target struct{}

	if buf.TryParse(&target) {
		t.Error("TryParse() returned true for empty buffer")
	}
}

func TestStreamProcessorToolUseBuffering(t *testing.T) {
	processor := NewStreamProcessor()

	// Start tool use content block
	startEvent := APIStreamEvent{
		Type:  APIEventContentBlockStart,
		Index: 0,
		ContentBlock: &ContentBlock{
			Type: ContentBlockToolUse,
			ID:   "tool_123",
			Name: "read_file",
		},
	}
	processor.ProcessEvent(startEvent)

	// Send partial JSON deltas
	delta1 := APIStreamEvent{
		Type:  APIEventContentBlockDelta,
		Index: 0,
		Delta: &Delta{
			PartialJSON: `{"path":`,
		},
	}
	processor.ProcessEvent(delta1)

	delta2 := APIStreamEvent{
		Type:  APIEventContentBlockDelta,
		Index: 0,
		Delta: &Delta{
			PartialJSON: `"/tmp/file.txt"}`,
		},
	}
	processor.ProcessEvent(delta2)

	// Stop content block
	stopEvent := APIStreamEvent{
		Type:  APIEventContentBlockStop,
		Index: 0,
	}
	processor.ProcessEvent(stopEvent)

	// Retrieve buffered input
	buf := processor.GetToolInput(0)
	if buf == nil {
		t.Fatal("GetToolInput() returned nil")
	}

	var input struct {
		Path string `json:"path"`
	}
	if !buf.TryParse(&input) {
		t.Fatal("Failed to parse buffered tool input")
	}
	if input.Path != "/tmp/file.txt" {
		t.Errorf("Parsed path = %q, want %q", input.Path, "/tmp/file.txt")
	}
}

func TestStreamProcessorNonToolUseContentBlock(t *testing.T) {
	processor := NewStreamProcessor()

	// Start text content block (not tool_use)
	startEvent := APIStreamEvent{
		Type:  APIEventContentBlockStart,
		Index: 0,
		ContentBlock: &ContentBlock{
			Type: ContentBlockText,
		},
	}
	processor.ProcessEvent(startEvent)

	// Should not create a buffer for text blocks
	buf := processor.GetToolInput(0)
	if buf != nil {
		t.Error("GetToolInput() should return nil for text content blocks")
	}
}

func TestStreamProcessorClearToolInput(t *testing.T) {
	processor := NewStreamProcessor()

	startEvent := APIStreamEvent{
		Type:  APIEventContentBlockStart,
		Index: 0,
		ContentBlock: &ContentBlock{
			Type: ContentBlockToolUse,
		},
	}
	processor.ProcessEvent(startEvent)

	processor.ClearToolInput(0)

	buf := processor.GetToolInput(0)
	if buf != nil {
		t.Error("GetToolInput() should return nil after ClearToolInput()")
	}
}

func TestStreamProcessorMultipleToolBlocks(t *testing.T) {
	processor := NewStreamProcessor()

	// First tool use block
	processor.ProcessEvent(APIStreamEvent{
		Type:  APIEventContentBlockStart,
		Index: 0,
		ContentBlock: &ContentBlock{
			Type: ContentBlockToolUse,
		},
	})
	processor.ProcessEvent(APIStreamEvent{
		Type:  APIEventContentBlockDelta,
		Index: 0,
		Delta: &Delta{PartialJSON: `{"a":1}`},
	})

	// Second tool use block
	processor.ProcessEvent(APIStreamEvent{
		Type:  APIEventContentBlockStart,
		Index: 1,
		ContentBlock: &ContentBlock{
			Type: ContentBlockToolUse,
		},
	})
	processor.ProcessEvent(APIStreamEvent{
		Type:  APIEventContentBlockDelta,
		Index: 1,
		Delta: &Delta{PartialJSON: `{"b":2}`},
	})

	// Verify both buffers exist
	buf0 := processor.GetToolInput(0)
	buf1 := processor.GetToolInput(1)

	if buf0 == nil || buf1 == nil {
		t.Fatal("Expected both tool input buffers to exist")
	}

	if buf0.String() != `{"a":1}` {
		t.Errorf("Buffer 0 = %q, want %q", buf0.String(), `{"a":1}`)
	}
	if buf1.String() != `{"b":2}` {
		t.Errorf("Buffer 1 = %q, want %q", buf1.String(), `{"b":2}`)
	}
}

func TestContentBlockInput(t *testing.T) {
	cb := ContentBlock{
		Type:  ContentBlockToolUse,
		ID:    "tool_123",
		Name:  "read_file",
		Input: json.RawMessage(`{"path":"/tmp/test.txt"}`),
	}

	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(cb.Input, &input); err != nil {
		t.Fatalf("Failed to unmarshal Input: %v", err)
	}
	if input.Path != "/tmp/test.txt" {
		t.Errorf("Input.Path = %q, want %q", input.Path, "/tmp/test.txt")
	}
}

func TestUsageStruct(t *testing.T) {
	usage := Usage{
		InputTokens:  100,
		OutputTokens: 50,
	}

	if usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", usage.OutputTokens)
	}
}
