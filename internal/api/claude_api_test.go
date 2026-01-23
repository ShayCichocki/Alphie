package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestNewClaudeAPI(t *testing.T) {
	// Create a mock client (won't make real API calls)
	client := &Client{
		model:   anthropic.ModelClaudeSonnet4_20250514,
		tracker: NewTokenTracker(),
	}

	cfg := ClaudeAPIConfig{
		Client: client,
	}

	api := NewClaudeAPI(cfg)

	if api == nil {
		t.Fatal("NewClaudeAPI returned nil")
	}
	if api.model != anthropic.ModelClaudeSonnet4_20250514 {
		t.Errorf("model = %q, want %q", api.model, anthropic.ModelClaudeSonnet4_20250514)
	}
	if api.maxIterations != 50 {
		t.Errorf("maxIterations = %d, want 50", api.maxIterations)
	}
}

func TestNewClaudeAPI_CustomIterations(t *testing.T) {
	client := &Client{
		model:   anthropic.ModelClaudeSonnet4_20250514,
		tracker: NewTokenTracker(),
	}

	cfg := ClaudeAPIConfig{
		Client:        client,
		MaxIterations: 100,
	}

	api := NewClaudeAPI(cfg)

	if api.maxIterations != 100 {
		t.Errorf("maxIterations = %d, want 100", api.maxIterations)
	}
}

func TestClaudeAPI_DoubleStart(t *testing.T) {
	client := &Client{
		model:   anthropic.ModelClaudeSonnet4_20250514,
		tracker: NewTokenTracker(),
	}

	api := NewClaudeAPI(ClaudeAPIConfig{Client: client})

	// First start - mark as started but don't actually run
	api.started = true

	err := api.Start("test", "/tmp")
	if err == nil {
		t.Fatal("Expected error on double start")
	}
	if err.Error() != "already started" {
		t.Errorf("Error = %q, want %q", err.Error(), "already started")
	}
}

func TestClaudeAPI_Kill(t *testing.T) {
	client := &Client{
		model:   anthropic.ModelClaudeSonnet4_20250514,
		tracker: NewTokenTracker(),
	}

	api := NewClaudeAPI(ClaudeAPIConfig{Client: client})

	// Kill before start should not panic
	err := api.Kill()
	if err != nil {
		t.Errorf("Kill returned error: %v", err)
	}
}

func TestClaudeAPI_PID(t *testing.T) {
	client := &Client{
		model:   anthropic.ModelClaudeSonnet4_20250514,
		tracker: NewTokenTracker(),
	}

	api := NewClaudeAPI(ClaudeAPIConfig{Client: client})

	// API mode should always return 0
	if api.PID() != 0 {
		t.Errorf("PID = %d, want 0", api.PID())
	}
}

func TestClaudeAPI_Stderr(t *testing.T) {
	client := &Client{
		model:   anthropic.ModelClaudeSonnet4_20250514,
		tracker: NewTokenTracker(),
	}

	api := NewClaudeAPI(ClaudeAPIConfig{Client: client})

	// API mode should always return empty stderr
	if api.Stderr() != "" {
		t.Errorf("Stderr = %q, want empty", api.Stderr())
	}
}

func TestClaudeAPI_Output(t *testing.T) {
	client := &Client{
		model:   anthropic.ModelClaudeSonnet4_20250514,
		tracker: NewTokenTracker(),
	}

	api := NewClaudeAPI(ClaudeAPIConfig{Client: client})

	ch := api.Output()
	if ch == nil {
		t.Error("Output channel should not be nil")
	}
}

func TestClaudeAPI_SetError(t *testing.T) {
	client := &Client{
		model:   anthropic.ModelClaudeSonnet4_20250514,
		tracker: NewTokenTracker(),
	}

	api := NewClaudeAPI(ClaudeAPIConfig{Client: client})

	// Use setError directly (doesn't require ctx)
	testErr := fmt.Errorf("test error")
	api.setError(testErr)

	if api.lastErr == nil {
		t.Fatal("lastErr should be set after setError")
	}
	if api.lastErr.Error() != "test error" {
		t.Errorf("lastErr = %q, want %q", api.lastErr.Error(), "test error")
	}
}

func TestClaudeAPI_WaitReturnsError(t *testing.T) {
	client := &Client{
		model:   anthropic.ModelClaudeSonnet4_20250514,
		tracker: NewTokenTracker(),
	}

	api := NewClaudeAPI(ClaudeAPIConfig{Client: client})

	// Simulate error being set using setError directly
	api.setError(fmt.Errorf("simulated failure"))

	// Close done channel to unblock Wait
	close(api.done)

	err := api.Wait()
	if err == nil {
		t.Fatal("Wait should return error after setError")
	}
	if err.Error() != "simulated failure" {
		t.Errorf("Wait error = %q, want %q", err.Error(), "simulated failure")
	}
}

func TestClaudeAPI_WaitReturnsNilOnSuccess(t *testing.T) {
	client := &Client{
		model:   anthropic.ModelClaudeSonnet4_20250514,
		tracker: NewTokenTracker(),
	}

	api := NewClaudeAPI(ClaudeAPIConfig{Client: client})

	// No error set, just close done
	close(api.done)

	err := api.Wait()
	if err != nil {
		t.Errorf("Wait should return nil on success, got: %v", err)
	}
}

func TestStreamEventCompat_Fields(t *testing.T) {
	event := StreamEventCompat{
		Type:       StreamEventAssistant,
		Message:    "Hello world",
		ToolAction: "read_file",
	}

	if event.Type != "assistant" {
		t.Errorf("Type = %q, want %q", event.Type, "assistant")
	}
	if event.Message != "Hello world" {
		t.Errorf("Message = %q, want %q", event.Message, "Hello world")
	}
	if event.ToolAction != "read_file" {
		t.Errorf("ToolAction = %q, want %q", event.ToolAction, "read_file")
	}
}

func TestStreamEventTypes(t *testing.T) {
	tests := []struct {
		constant string
		expected string
	}{
		{StreamEventSystem, "system"},
		{StreamEventAssistant, "assistant"},
		{StreamEventUser, "user"},
		{StreamEventResult, "result"},
		{StreamEventError, "error"},
	}

	for _, tt := range tests {
		if tt.constant != tt.expected {
			t.Errorf("Constant %q != expected %q", tt.constant, tt.expected)
		}
	}
}

func TestClaudeAPIConfig_Defaults(t *testing.T) {
	client := &Client{
		tracker: NewTokenTracker(),
	}

	cfg := ClaudeAPIConfig{
		Client: client,
		// Leave Model and MaxIterations empty
	}

	api := NewClaudeAPI(cfg)

	// Should use defaults
	// Model comes from client, so just check maxIterations default
	if api.maxIterations != 50 {
		t.Errorf("Default maxIterations = %d, want 50", api.maxIterations)
	}
}

func TestClaudeAPI_ChannelBufferSize(t *testing.T) {
	client := &Client{
		model:   anthropic.ModelClaudeSonnet4_20250514,
		tracker: NewTokenTracker(),
	}

	api := NewClaudeAPI(ClaudeAPIConfig{Client: client})

	// Verify buffer size is 100 (can send 100 without blocking)
	for i := 0; i < 100; i++ {
		select {
		case api.outputCh <- StreamEventCompat{Type: StreamEventAssistant, Message: "test"}:
			// Good - didn't block
		case <-time.After(10 * time.Millisecond):
			t.Fatalf("Channel blocked at index %d, expected buffer of 100", i)
		}
	}
}
