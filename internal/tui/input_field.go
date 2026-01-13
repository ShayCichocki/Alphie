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

	// History support
	history []string // stored commands
	histIdx int      // current position (-1 = typing new input)
	draft   string   // saves current input when browsing history
}

// NewInputField creates a new InputField.
func NewInputField() *InputField {
	ti := textinput.New()
	ti.Placeholder = "Type a task and press Enter..."
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 60

	return &InputField{
		input:   ti,
		width:   80,
		history: make([]string, 0),
		histIdx: -1, // -1 = typing new input
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
				// Add to history
				f.history = append(f.history, text)
				f.histIdx = -1
				f.draft = ""

				tier, cleanText := ClassifyTier(text)
				f.input.Reset()
				return f, func() tea.Msg {
					return TaskSubmittedMsg{
						Task: cleanText,
						Tier: tier,
					}
				}
			}

		case "up":
			// Navigate to previous history entry
			if len(f.history) > 0 {
				if f.histIdx == -1 {
					// Save current input as draft before navigating
					f.draft = f.input.Value()
					f.histIdx = len(f.history) - 1
				} else if f.histIdx > 0 {
					f.histIdx--
				}
				f.input.SetValue(f.history[f.histIdx])
				f.input.CursorEnd()
			}
			return f, nil

		case "down":
			// Navigate to next history entry or back to draft
			if f.histIdx >= 0 {
				if f.histIdx < len(f.history)-1 {
					f.histIdx++
					f.input.SetValue(f.history[f.histIdx])
				} else {
					// Return to draft
					f.histIdx = -1
					f.input.SetValue(f.draft)
				}
				f.input.CursorEnd()
			}
			return f, nil
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
