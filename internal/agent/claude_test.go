package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNewClaudeProcess(t *testing.T) {
	ctx := context.Background()
	proc := NewClaudeProcess(ctx)

	if proc == nil {
		t.Fatal("NewClaudeProcess returned nil")
	}
	if proc.outputCh == nil {
		t.Error("outputCh should not be nil")
	}
	if proc.done == nil {
		t.Error("done channel should not be nil")
	}
	if proc.ctx == nil {
		t.Error("ctx should not be nil")
	}
	if proc.cancel == nil {
		t.Error("cancel should not be nil")
	}
}

func TestClaudeProcess_StartBeforeStarted(t *testing.T) {
	ctx := context.Background()
	proc := NewClaudeProcess(ctx)

	if proc.started {
		t.Error("Process should not be started initially")
	}
}

func TestClaudeProcess_WaitWithoutStart(t *testing.T) {
	ctx := context.Background()
	proc := NewClaudeProcess(ctx)

	err := proc.Wait()
	if err == nil {
		t.Error("Wait should return error when process not started")
	}
	if err.Error() != "process not started" {
		t.Errorf("Error = %q, want %q", err.Error(), "process not started")
	}
}

func TestClaudeProcess_KillWithoutStart(t *testing.T) {
	ctx := context.Background()
	proc := NewClaudeProcess(ctx)

	// Should not panic or error
	err := proc.Kill()
	if err != nil {
		t.Errorf("Kill without start should not error, got: %v", err)
	}
}

func TestClaudeProcess_PIDWithoutStart(t *testing.T) {
	ctx := context.Background()
	proc := NewClaudeProcess(ctx)

	pid := proc.PID()
	if pid != 0 {
		t.Errorf("PID without start should be 0, got %d", pid)
	}
}

func TestClaudeProcess_StderrWithoutStart(t *testing.T) {
	ctx := context.Background()
	proc := NewClaudeProcess(ctx)

	stderr := proc.Stderr()
	if stderr != "" {
		t.Errorf("Stderr without start should be empty, got %q", stderr)
	}
}

func TestClaudeProcess_Output(t *testing.T) {
	ctx := context.Background()
	proc := NewClaudeProcess(ctx)

	ch := proc.Output()
	if ch == nil {
		t.Error("Output should return a channel")
	}
}

func TestClaudeProcess_StartTwice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	proc := NewClaudeProcess(ctx)

	// First start will fail because 'claude' CLI is not available in test
	// But we're testing the double-start protection
	_ = proc.Start("test", "")

	// Set started manually to test double-start protection
	proc.mu.Lock()
	proc.started = true
	proc.mu.Unlock()

	err := proc.Start("test2", "")
	if err == nil {
		t.Error("Second Start should return error")
	}
	if err.Error() != "process already started" {
		t.Errorf("Error = %q, want %q", err.Error(), "process already started")
	}
}

func TestStreamEventType_Constants(t *testing.T) {
	tests := []struct {
		eventType StreamEventType
		expected  string
	}{
		{StreamEventSystem, "system"},
		{StreamEventAssistant, "assistant"},
		{StreamEventUser, "user"},
		{StreamEventResult, "result"},
		{StreamEventError, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.eventType) != tt.expected {
				t.Errorf("StreamEventType = %q, want %q", tt.eventType, tt.expected)
			}
		})
	}
}

func TestStreamEvent_Fields(t *testing.T) {
	raw := json.RawMessage(`{"type":"assistant","message":"hello"}`)

	event := StreamEvent{
		Type:    StreamEventAssistant,
		Message: "hello",
		Error:   "",
		Raw:     raw,
	}

	if event.Type != StreamEventAssistant {
		t.Errorf("Type = %q, want %q", event.Type, StreamEventAssistant)
	}
	if event.Message != "hello" {
		t.Errorf("Message = %q, want %q", event.Message, "hello")
	}
	if event.Error != "" {
		t.Errorf("Error should be empty, got %q", event.Error)
	}
}

func TestParseStreamEvent_Assistant(t *testing.T) {
	data := []byte(`{"type":"assistant","message":"Working on task"}`)

	event, err := parseStreamEvent(data)
	if err != nil {
		t.Fatalf("parseStreamEvent failed: %v", err)
	}

	if event.Type != StreamEventAssistant {
		t.Errorf("Type = %q, want %q", event.Type, StreamEventAssistant)
	}
	if event.Message != "Working on task" {
		t.Errorf("Message = %q, want %q", event.Message, "Working on task")
	}
}

func TestParseStreamEvent_AssistantWithContent(t *testing.T) {
	data := []byte(`{"type":"assistant","content":"Working on task"}`)

	event, err := parseStreamEvent(data)
	if err != nil {
		t.Fatalf("parseStreamEvent failed: %v", err)
	}

	if event.Message != "Working on task" {
		t.Errorf("Message = %q, want %q", event.Message, "Working on task")
	}
}

func TestParseStreamEvent_Result(t *testing.T) {
	data := []byte(`{"type":"result","result":"Task completed"}`)

	event, err := parseStreamEvent(data)
	if err != nil {
		t.Fatalf("parseStreamEvent failed: %v", err)
	}

	if event.Type != StreamEventResult {
		t.Errorf("Type = %q, want %q", event.Type, StreamEventResult)
	}
	if event.Message != "Task completed" {
		t.Errorf("Message = %q, want %q", event.Message, "Task completed")
	}
}

func TestParseStreamEvent_Error(t *testing.T) {
	data := []byte(`{"type":"error","error":"Something went wrong"}`)

	event, err := parseStreamEvent(data)
	if err != nil {
		t.Fatalf("parseStreamEvent failed: %v", err)
	}

	if event.Type != StreamEventError {
		t.Errorf("Type = %q, want %q", event.Type, StreamEventError)
	}
	if event.Error != "Something went wrong" {
		t.Errorf("Error = %q, want %q", event.Error, "Something went wrong")
	}
}

func TestParseStreamEvent_ErrorWithMessage(t *testing.T) {
	data := []byte(`{"type":"error","message":"Error message"}`)

	event, err := parseStreamEvent(data)
	if err != nil {
		t.Fatalf("parseStreamEvent failed: %v", err)
	}

	if event.Error != "Error message" {
		t.Errorf("Error = %q, want %q", event.Error, "Error message")
	}
}

func TestParseStreamEvent_System(t *testing.T) {
	data := []byte(`{"type":"system","message":"System message"}`)

	event, err := parseStreamEvent(data)
	if err != nil {
		t.Fatalf("parseStreamEvent failed: %v", err)
	}

	if event.Type != StreamEventSystem {
		t.Errorf("Type = %q, want %q", event.Type, StreamEventSystem)
	}
}

func TestParseStreamEvent_User(t *testing.T) {
	data := []byte(`{"type":"user","message":"User message"}`)

	event, err := parseStreamEvent(data)
	if err != nil {
		t.Fatalf("parseStreamEvent failed: %v", err)
	}

	if event.Type != StreamEventUser {
		t.Errorf("Type = %q, want %q", event.Type, StreamEventUser)
	}
}

func TestParseStreamEvent_InvalidJSON(t *testing.T) {
	data := []byte(`not valid json`)

	_, err := parseStreamEvent(data)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestParseStreamEvent_EmptyJSON(t *testing.T) {
	data := []byte(`{}`)

	event, err := parseStreamEvent(data)
	if err != nil {
		t.Fatalf("parseStreamEvent failed: %v", err)
	}

	if event.Type != "" {
		t.Errorf("Type should be empty, got %q", event.Type)
	}
}

func TestParseStreamEvent_PreservesRaw(t *testing.T) {
	data := []byte(`{"type":"assistant","message":"test"}`)

	event, err := parseStreamEvent(data)
	if err != nil {
		t.Fatalf("parseStreamEvent failed: %v", err)
	}

	if event.Raw == nil {
		t.Error("Raw should be preserved")
	}
	if string(event.Raw) != string(data) {
		t.Errorf("Raw = %q, want %q", string(event.Raw), string(data))
	}
}

func TestParseStreamEvent_WithUsage(t *testing.T) {
	data := []byte(`{"type":"assistant","message":"test","usage":{"input_tokens":100,"output_tokens":50}}`)

	event, err := parseStreamEvent(data)
	if err != nil {
		t.Fatalf("parseStreamEvent failed: %v", err)
	}

	if event.Type != StreamEventAssistant {
		t.Errorf("Type = %q, want %q", event.Type, StreamEventAssistant)
	}
	// Raw should contain the full JSON including usage
	if event.Raw == nil {
		t.Error("Raw should contain usage data")
	}
}

func TestClaudeProcess_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	proc := NewClaudeProcess(ctx)

	// Cancel immediately
	cancel()

	// Context should be done
	select {
	case <-proc.ctx.Done():
		// Expected
	default:
		t.Error("Process context should be cancelled")
	}
}

func TestClaudeProcess_KillCancelsContext(t *testing.T) {
	ctx := context.Background()
	proc := NewClaudeProcess(ctx)

	proc.Kill()

	// After kill, context should be cancelled
	select {
	case <-proc.ctx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Kill should cancel context")
	}
}

func TestClaudeProcess_MultiplekillsAreSafe(t *testing.T) {
	ctx := context.Background()
	proc := NewClaudeProcess(ctx)

	// Multiple kills should not panic
	for i := 0; i < 5; i++ {
		err := proc.Kill()
		if err != nil {
			t.Errorf("Kill %d failed: %v", i, err)
		}
	}
}

func TestStreamEvent_JSONTags(t *testing.T) {
	event := StreamEvent{
		Type:    StreamEventAssistant,
		Message: "test",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Check JSON field names
	if _, ok := parsed["type"]; !ok {
		t.Error("JSON should have 'type' field")
	}
	if _, ok := parsed["message"]; !ok {
		t.Error("JSON should have 'message' field")
	}
}
