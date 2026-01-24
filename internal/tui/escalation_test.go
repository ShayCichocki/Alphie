package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestEscalationMsg tests that escalation messages update the state correctly.
func TestEscalationMsg(t *testing.T) {
	app := NewImplementApp()

	// Send escalation message
	msg := EscalationMsg{
		TaskID:    "task-123",
		TaskTitle: "Fix authentication bug",
		Reason:    "Tests failed after 3 attempts",
		Attempts:  3,
		LogFile:   "/tmp/task-123.log",
	}

	updatedModel, _ := app.Update(msg)
	updatedApp := updatedModel.(*ImplementApp)

	// Verify state was updated
	state := updatedApp.view.state
	if !state.IsEscalating {
		t.Error("Expected IsEscalating to be true")
	}
	if state.EscalationTaskID != "task-123" {
		t.Errorf("Expected EscalationTaskID 'task-123', got: %s", state.EscalationTaskID)
	}
	if state.EscalationTask != "Fix authentication bug" {
		t.Errorf("Expected EscalationTask 'Fix authentication bug', got: %s", state.EscalationTask)
	}
	if state.EscalationReason != "Tests failed after 3 attempts" {
		t.Errorf("Expected EscalationReason 'Tests failed after 3 attempts', got: %s", state.EscalationReason)
	}
	if state.EscalationAttempts != 3 {
		t.Errorf("Expected EscalationAttempts 3, got: %d", state.EscalationAttempts)
	}
	if state.EscalationLogFile != "/tmp/task-123.log" {
		t.Errorf("Expected EscalationLogFile '/tmp/task-123.log', got: %s", state.EscalationLogFile)
	}

	// Verify log entry was created
	if len(updatedApp.logs) == 0 {
		t.Error("Expected escalation log entry to be created")
	} else {
		lastLog := updatedApp.logs[len(updatedApp.logs)-1]
		if lastLog.Phase != "ESCALATION" {
			t.Errorf("Expected log phase 'ESCALATION', got: %s", lastLog.Phase)
		}
	}
}

// TestEscalationKeypress tests that escalation keypresses trigger the handler.
func TestEscalationKeypress(t *testing.T) {
	app := NewImplementApp()

	// Track handler calls
	handlerCalled := false
	var handlerAction string

	app.SetEscalationHandler(func(action string) error {
		handlerCalled = true
		handlerAction = action
		return nil
	})

	// Set escalating state
	state := app.view.state
	state.IsEscalating = true
	state.EscalationTask = "Test task"
	app.view.SetState(state)

	// Test retry key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	updatedModel, _ := app.Update(keyMsg)
	updatedApp := updatedModel.(*ImplementApp)

	if !handlerCalled {
		t.Error("Expected handler to be called")
	}
	if handlerAction != "retry" {
		t.Errorf("Expected action 'retry', got: %s", handlerAction)
	}

	// Verify escalation state was cleared
	updatedState := updatedApp.view.state
	if updatedState.IsEscalating {
		t.Error("Expected IsEscalating to be false after response")
	}
}

// TestEscalationKeypressWithoutHandler tests that missing handler doesn't crash.
func TestEscalationKeypressWithoutHandler(t *testing.T) {
	app := NewImplementApp()

	// No handler set - should not crash

	// Set escalating state
	state := app.view.state
	state.IsEscalating = true
	app.view.SetState(state)

	// Press key - should not crash
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	_, _ = app.Update(keyMsg)

	// If we got here without panic, test passes
}

// TestEscalationViewRendering tests that escalation prompt is rendered.
func TestEscalationViewRendering(t *testing.T) {
	view := NewImplementView()

	// Set escalating state
	state := ImplementState{
		IsEscalating:       true,
		EscalationTask:     "Fix bug in auth",
		EscalationReason:   "Compilation failed",
		EscalationAttempts: 2,
		EscalationLogFile:  "/logs/task.log",
	}
	view.SetState(state)

	// Render view
	rendered := view.View()

	// Check for expected content
	expectedStrings := []string{
		"TASK ESCALATION REQUIRED",
		"Fix bug in auth",
		"Compilation failed",
		"Retry",
		"Skip",
		"Abort",
		"Manual Fix",
	}

	for _, expected := range expectedStrings {
		if !contains(rendered, expected) {
			t.Errorf("Expected rendered view to contain '%s'", expected)
		}
	}
}

// TestAllEscalationActions tests all 4 escalation actions.
func TestAllEscalationActions(t *testing.T) {
	actions := []string{"retry", "skip", "abort", "manual"}
	keys := []rune{'r', 's', 'a', 'm'}

	for i, expectedAction := range actions {
		app := NewImplementApp()

		var receivedAction string
		app.SetEscalationHandler(func(action string) error {
			receivedAction = action
			return nil
		})

		// Set escalating state
		state := app.view.state
		state.IsEscalating = true
		app.view.SetState(state)

		// Press key
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{keys[i]}}
		_, _ = app.Update(keyMsg)

		if receivedAction != expectedAction {
			t.Errorf("Key '%c': expected action '%s', got: %s", keys[i], expectedAction, receivedAction)
		}
	}
}

// contains checks if a string contains a substring (case-sensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || contains(s[1:], substr)))
}
