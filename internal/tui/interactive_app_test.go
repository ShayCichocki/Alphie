package tui

import (
	"testing"
	"time"

	"github.com/ShayCichocki/alphie/pkg/models"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewInteractiveApp(t *testing.T) {
	app := NewInteractiveApp()

	if app == nil {
		t.Fatal("NewInteractiveApp returned nil")
	}
	if app.panelApp == nil {
		t.Error("panelApp should not be nil")
	}
	if app.inputField == nil {
		t.Error("inputField should not be nil")
	}
}

func TestInteractiveApp_SetTaskSubmitHandler(t *testing.T) {
	app := NewInteractiveApp()

	var receivedTask string
	var receivedTier interface{}

	handler := func(task string, tier interface{}) {
		receivedTask = task
		receivedTier = tier
	}

	app.SetTaskSubmitHandler(handler)

	if app.onTaskSubmit == nil {
		t.Error("onTaskSubmit handler should be set")
	}

	// Trigger the handler
	app.onTaskSubmit("test task", nil)

	if receivedTask != "test task" {
		t.Errorf("Handler received task = %q, want %q", receivedTask, "test task")
	}
	if receivedTier != nil {
		t.Errorf("Handler received tier = %q, want %q", receivedTier, nil)
	}
}

func TestInteractiveApp_Init(t *testing.T) {
	app := NewInteractiveApp()

	cmd := app.Init()

	// Init should return a focus command for the input field
	if cmd == nil {
		t.Error("Init should return a command to focus the input")
	}
}

func TestInteractiveApp_Update_CtrlC(t *testing.T) {
	app := NewInteractiveApp()

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	model, cmd := app.Update(msg)

	updatedApp := model.(*InteractiveApp)
	if !updatedApp.quitting {
		t.Error("quitting should be true after Ctrl+C")
	}

	// Should return quit command
	if cmd == nil {
		t.Error("Expected quit command")
	}
}

func TestInteractiveApp_Update_WindowSize(t *testing.T) {
	app := NewInteractiveApp()

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	model, _ := app.Update(msg)

	updatedApp := model.(*InteractiveApp)
	if updatedApp.width != 120 {
		t.Errorf("width = %d, want 120", updatedApp.width)
	}
	if updatedApp.height != 40 {
		t.Errorf("height = %d, want 40", updatedApp.height)
	}
}

func TestInteractiveApp_Update_TaskSubmitted(t *testing.T) {
	app := NewInteractiveApp()

	var handlerCalled bool
	app.SetTaskSubmitHandler(func(task string, tier interface{}) {
		handlerCalled = true
	})

	msg := TaskSubmittedMsg{Task: "test task", Tier: nil}
	_, _ = app.Update(msg)

	if !handlerCalled {
		t.Error("Task submit handler should have been called")
	}
}

func TestInteractiveApp_Update_TaskSubmitted_NoHandler(t *testing.T) {
	app := NewInteractiveApp()
	// Don't set a handler

	msg := TaskSubmittedMsg{Task: "test task", Tier: nil}

	// Should not panic
	_, _ = app.Update(msg)
}

func TestInteractiveApp_Update_AgentUpdateMsg(t *testing.T) {
	app := NewInteractiveApp()

	agent := &models.Agent{
		ID:     "agent-1",
		Status: models.AgentStatusRunning,
	}
	msg := AgentUpdateMsg{Agent: agent}

	// Should forward to panel app
	_, _ = app.Update(msg)

	// No crash is a success here
}

func TestInteractiveApp_Update_TaskUpdateMsg(t *testing.T) {
	app := NewInteractiveApp()

	task := &models.Task{
		ID:     "task-1",
		Title:  "Test task",
		Status: models.TaskStatusInProgress,
	}
	msg := TaskUpdateMsg{Task: task}

	_, _ = app.Update(msg)
}

func TestInteractiveApp_Update_OrchestratorEventMsg(t *testing.T) {
	app := NewInteractiveApp()

	msg := OrchestratorEventMsg{
		Type:      "task_started",
		TaskID:    "task-1",
		TaskTitle: "Test task",
		AgentID:   "agent-1",
		Message:   "Task started",
		Timestamp: time.Now(),
	}

	_, _ = app.Update(msg)
}

func TestInteractiveApp_Update_SessionDoneMsg(t *testing.T) {
	app := NewInteractiveApp()

	msg := SessionDoneMsg{
		Success: true,
		Message: "All tasks completed",
	}

	_, _ = app.Update(msg)
}

func TestInteractiveApp_Update_DebugLogMsg(t *testing.T) {
	app := NewInteractiveApp()

	msg := DebugLogMsg{
		Message: "Debug message",
	}

	_, _ = app.Update(msg)
}

func TestInteractiveApp_View_NotQuitting(t *testing.T) {
	app := NewInteractiveApp()
	app.width = 80
	app.height = 24
	app.updateSizes()

	view := app.View()

	if view == "" {
		t.Error("View should not be empty")
	}
	// Should NOT contain goodbye message
	if view == "Goodbye!\n" {
		t.Error("Should not show goodbye when not quitting")
	}
}

func TestInteractiveApp_View_Quitting(t *testing.T) {
	app := NewInteractiveApp()
	app.quitting = true

	view := app.View()

	if view != "Goodbye!\n" {
		t.Errorf("View when quitting = %q, want %q", view, "Goodbye!\n")
	}
}

func TestInteractiveApp_updateSizes(t *testing.T) {
	app := NewInteractiveApp()
	app.width = 100
	app.height = 30

	app.updateSizes()

	// Panel app should have adjusted height (minus input field and spacing)
	expectedPanelHeight := 30 - 3 - 1 // height - inputHeight - spacing
	if app.panelApp.height != expectedPanelHeight {
		t.Errorf("panelApp.height = %d, want %d", app.panelApp.height, expectedPanelHeight)
	}

	// Input field should have the full width
	if app.inputField.width != 100 {
		t.Errorf("inputField.width = %d, want 100", app.inputField.width)
	}
}

func TestInteractiveApp_GetPanelApp(t *testing.T) {
	app := NewInteractiveApp()

	panelApp := app.GetPanelApp()

	if panelApp == nil {
		t.Error("GetPanelApp should not return nil")
	}
	if panelApp != app.panelApp {
		t.Error("GetPanelApp should return the same panelApp instance")
	}
}

func TestNewInteractiveProgram(t *testing.T) {
	program, app := NewInteractiveProgram()

	if program == nil {
		t.Error("Program should not be nil")
	}
	if app == nil {
		t.Error("App should not be nil")
	}
}

func TestInteractiveApp_Send(t *testing.T) {
	app := NewInteractiveApp()

	// Send is a no-op in the app itself (handled by tea.Program)
	// Just verify it doesn't panic
	app.Send(DebugLogMsg{Message: "test"})
}

// Message type tests

func TestAgentUpdateMsg(t *testing.T) {
	agent := &models.Agent{
		ID:     "test-agent",
		Status: models.AgentStatusRunning,
	}
	msg := AgentUpdateMsg{Agent: agent}

	if msg.Agent.ID != "test-agent" {
		t.Errorf("Agent.ID = %q, want %q", msg.Agent.ID, "test-agent")
	}
}

func TestTaskUpdateMsg(t *testing.T) {
	task := &models.Task{
		ID:    "test-task",
		Title: "Test",
	}
	msg := TaskUpdateMsg{Task: task}

	if msg.Task.ID != "test-task" {
		t.Errorf("Task.ID = %q, want %q", msg.Task.ID, "test-task")
	}
}

func TestOrchestratorEventMsg_AllFields(t *testing.T) {
	now := time.Now()
	msg := OrchestratorEventMsg{
		Type:       "task_completed",
		TaskID:     "task-123",
		TaskTitle:  "Fix bug",
		AgentID:    "agent-456",
		Message:    "Task completed successfully",
		Error:      "",
		Timestamp:  now,
		TokensUsed: 1000,
		Cost:       0.05,
		Duration:   5 * time.Second,
	}

	if msg.Type != "task_completed" {
		t.Errorf("Type = %q, want %q", msg.Type, "task_completed")
	}
	if msg.TokensUsed != 1000 {
		t.Errorf("TokensUsed = %d, want 1000", msg.TokensUsed)
	}
	if msg.Cost != 0.05 {
		t.Errorf("Cost = %f, want 0.05", msg.Cost)
	}
	if msg.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", msg.Duration)
	}
}

func TestSessionDoneMsg_Success(t *testing.T) {
	msg := SessionDoneMsg{
		Success: true,
		Message: "Completed",
	}

	if !msg.Success {
		t.Error("Success should be true")
	}
}

func TestSessionDoneMsg_Failure(t *testing.T) {
	msg := SessionDoneMsg{
		Success: false,
		Message: "Failed with error",
	}

	if msg.Success {
		t.Error("Success should be false")
	}
}

func TestDebugLogMsg(t *testing.T) {
	msg := DebugLogMsg{
		Message: "Debug info",
	}

	if msg.Message != "Debug info" {
		t.Errorf("Message = %q, want %q", msg.Message, "Debug info")
	}
}
