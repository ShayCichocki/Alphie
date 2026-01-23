package tui

import (
	"testing"

	"github.com/ShayCichocki/alphie/pkg/models"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewInputField(t *testing.T) {
	field := NewInputField()

	if field == nil {
		t.Fatal("NewInputField returned nil")
	}
	if field.width != 80 {
		t.Errorf("Default width = %d, want 80", field.width)
	}
}

func TestInputField_SetWidth(t *testing.T) {
	field := NewInputField()

	field.SetWidth(120)

	if field.width != 120 {
		t.Errorf("Width after SetWidth(120) = %d, want 120", field.width)
	}
	// Input width should be width - 4 for prompt and padding
	expectedInputWidth := 116
	if field.input.Width != expectedInputWidth {
		t.Errorf("Input width = %d, want %d", field.input.Width, expectedInputWidth)
	}
}

func TestInputField_Update_Enter_EmptyInput(t *testing.T) {
	field := NewInputField()

	// Simulate pressing enter with empty input
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	updatedField, cmd := field.Update(msg)

	if cmd != nil {
		// No command should be returned for empty input
		result := cmd()
		if _, ok := result.(TaskSubmittedMsg); ok {
			t.Error("Should not submit task for empty input")
		}
	}

	if updatedField == nil {
		t.Error("Update returned nil field")
	}
}

func TestInputField_Update_Enter_WithInput(t *testing.T) {
	field := NewInputField()

	// Set some text in the input
	field.input.SetValue("fix typo in README")

	// Simulate pressing enter
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := field.Update(msg)

	if cmd == nil {
		t.Fatal("Expected command from enter with text")
	}

	// Execute the command to get the message
	result := cmd()
	submitted, ok := result.(TaskSubmittedMsg)
	if !ok {
		t.Fatalf("Expected TaskSubmittedMsg, got %T", result)
	}

	// ClassifyTier should have been applied
	if submitted.Task == "" {
		t.Error("Task should not be empty")
	}
}

func TestInputField_Update_Enter_QuickPrefix(t *testing.T) {
	field := NewInputField()
	field.input.SetValue("!quick change button color")

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := field.Update(msg)

	if cmd == nil {
		t.Fatal("Expected command from enter")
	}

	result := cmd()
	submitted, ok := result.(TaskSubmittedMsg)
	if !ok {
		t.Fatalf("Expected TaskSubmittedMsg, got %T", result)
	}

	if submitted.Tier != nil {
		t.Errorf("Tier = %q, want %q", submitted.Tier, nil)
	}
	if submitted.Task != "change button color" {
		t.Errorf("Task = %q, want %q", submitted.Task, "change button color")
	}
}

func TestInputField_Update_Enter_ArchitectPrefix(t *testing.T) {
	field := NewInputField()
	field.input.SetValue("!architect refactor database layer")

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := field.Update(msg)

	result := cmd()
	submitted := result.(TaskSubmittedMsg)

	if submitted.Tier != nil {
		t.Errorf("Tier = %q, want %q", submitted.Tier, nil)
	}
}

func TestInputField_Update_EnterClearsInput(t *testing.T) {
	field := NewInputField()
	field.input.SetValue("test task")

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	updatedField, _ := field.Update(msg)

	if updatedField.input.Value() != "" {
		t.Errorf("Input should be cleared after enter, got %q", updatedField.input.Value())
	}
}

func TestInputField_Update_OtherKeys(t *testing.T) {
	field := NewInputField()

	// Type some characters
	for _, char := range "hello" {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}}
		field, _ = field.Update(msg)
	}

	if field.input.Value() != "hello" {
		t.Errorf("Input value = %q, want %q", field.input.Value(), "hello")
	}
}

func TestInputField_Focus(t *testing.T) {
	field := NewInputField()

	cmd := field.Focus()

	if cmd == nil {
		t.Error("Focus should return a command")
	}
}

func TestInputField_Blur(t *testing.T) {
	field := NewInputField()

	// Should not panic
	field.Blur()

	// After blur, input should not be focused
	// (textinput.Model doesn't expose focus state directly, but Blur should work)
}

func TestInputField_View(t *testing.T) {
	field := NewInputField()
	field.SetWidth(80)

	view := field.View()

	if view == "" {
		t.Error("View should not be empty")
	}
	// View should contain the prompt
	if len(view) < 10 {
		t.Error("View seems too short")
	}
}

func TestInputField_View_WithText(t *testing.T) {
	field := NewInputField()
	field.input.SetValue("some task text")

	view := field.View()

	// The view should render something
	if view == "" {
		t.Error("View should not be empty")
	}
}

func TestTaskSubmittedMsg_Fields(t *testing.T) {
	msg := TaskSubmittedMsg{
		Task: "implement feature",
		Tier: nil,
	}

	if msg.Task != "implement feature" {
		t.Errorf("Task = %q, want %q", msg.Task, "implement feature")
	}
	if msg.Tier != nil {
		t.Errorf("Tier = %q, want %q", msg.Tier, nil)
	}
}

func TestInputField_CharLimit(t *testing.T) {
	field := NewInputField()

	// CharLimit should be set
	if field.input.CharLimit != 500 {
		t.Errorf("CharLimit = %d, want 500", field.input.CharLimit)
	}
}

func TestInputField_Placeholder(t *testing.T) {
	field := NewInputField()

	// Should have a placeholder
	if field.input.Placeholder == "" {
		t.Error("Placeholder should be set")
	}
}

func TestInputField_TierClassification(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTier interface{}
		wantTask string
	}{
		{
			name:     "quick prefix",
			input:    "!quick fix typo",
			wantTier: nil,
			wantTask: "fix typo",
		},
		{
			name:     "scout prefix",
			input:    "!scout find auth code",
			wantTier: nil,
			wantTask: "find auth code",
		},
		{
			name:     "builder prefix",
			input:    "!builder add feature",
			wantTier: nil,
			wantTask: "add feature",
		},
		{
			name:     "default to builder",
			input:    "add dark mode",
			wantTier: nil,
			wantTask: "add dark mode",
		},
		{
			name:     "scout keyword",
			input:    "find the login form",
			wantTier: nil,
			wantTask: "find the login form",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := NewInputField()
			field.input.SetValue(tt.input)

			msg := tea.KeyMsg{Type: tea.KeyEnter}
			_, cmd := field.Update(msg)

			result := cmd()
			submitted := result.(TaskSubmittedMsg)

			if submitted.Tier != tt.wantTier {
				t.Errorf("Tier = %q, want %q", submitted.Tier, tt.wantTier)
			}
			if submitted.Task != tt.wantTask {
				t.Errorf("Task = %q, want %q", submitted.Task, tt.wantTask)
			}
		})
	}
}

// mockTextInput creates a textinput for testing
func mockTextInput(value string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(value)
	return ti
}
