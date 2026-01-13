package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/shayc/alphie/pkg/models"
)

// TaskSubmittedMsg is sent when the user submits a task.
type TaskSubmittedMsg struct {
	Task string
	Tier models.Tier
}

// InputField is a text input component for entering tasks.
type InputField struct {
	input textinput.Model
	width int
}

// NewInputField creates a new InputField.
func NewInputField() *InputField {
	ti := textinput.New()
	ti.Placeholder = "Type a task and press Enter..."
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 60

	return &InputField{
		input: ti,
		width: 80,
	}
}

// SetWidth sets the width of the input field.
func (f *InputField) SetWidth(width int) {
	f.width = width
	f.input.Width = width - 4 // Account for prompt and padding
}

// Update handles messages for the input field.
func (f *InputField) Update(msg tea.Msg) (*InputField, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			text := f.input.Value()
			if text != "" {
				tier, cleanText := ClassifyTier(text)
				f.input.Reset()
				return f, func() tea.Msg {
					return TaskSubmittedMsg{
						Task: cleanText,
						Tier: tier,
					}
				}
			}
		}
	}

	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

// View renders the input field.
func (f *InputField) View() string {
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(f.width - 2)

	prompt := promptStyle.Render("> ")
	return boxStyle.Render(prompt + f.input.View())
}

// Focus sets focus on the input field.
func (f *InputField) Focus() tea.Cmd {
	return f.input.Focus()
}

// Blur removes focus from the input field.
func (f *InputField) Blur() {
	f.input.Blur()
}
