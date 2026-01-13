package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// TaskCounts holds the count of tasks in each status.
type TaskCounts struct {
	Done    int
	Failed  int
	Running int
}

// Footer renders the status bar and keyboard hints.
type Footer struct {
	message      string
	success      bool
	sessionDone  bool
	focusedPanel int
	width        int
	taskCounts   TaskCounts

	// Styles
	successStyle   lipgloss.Style
	errorStyle     lipgloss.Style
	hintStyle      lipgloss.Style
	separatorStyle lipgloss.Style
}

// NewFooter creates a new Footer instance.
func NewFooter() *Footer {
	return &Footer{
		focusedPanel: 1, // Agents panel by default

		successStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("28")).
			Bold(true),

		errorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true),

		hintStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),

		separatorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("236")),
	}
}

// SetMessage sets the status message.
func (f *Footer) SetMessage(message string, success bool) {
	f.message = message
	f.success = success
}

// SetSessionDone marks the session as complete.
func (f *Footer) SetSessionDone(done bool, success bool, message string) {
	f.sessionDone = done
	f.success = success
	f.message = message
}

// SetFocusedPanel sets which panel is currently focused.
func (f *Footer) SetFocusedPanel(panel int) {
	f.focusedPanel = panel
}

// SetWidth sets the footer width.
func (f *Footer) SetWidth(width int) {
	f.width = width
}

// SetTaskCounts updates the task counts for display.
func (f *Footer) SetTaskCounts(counts TaskCounts) {
	f.taskCounts = counts
}

// View renders the footer.
func (f *Footer) View() string {
	var left string
	var right string

	// Left side: task counts and status message
	total := f.taskCounts.Done + f.taskCounts.Failed + f.taskCounts.Running
	if total > 0 {
		counts := fmt.Sprintf("✓%d", f.taskCounts.Done)
		if f.taskCounts.Failed > 0 {
			counts += f.errorStyle.Render(fmt.Sprintf(" ✗%d", f.taskCounts.Failed))
		}
		if f.taskCounts.Running > 0 {
			counts += fmt.Sprintf(" ⏳%d", f.taskCounts.Running)
		}
		left = counts
	}

	if f.sessionDone {
		if f.success {
			left = f.successStyle.Render("✓ " + f.message)
		} else {
			left = f.errorStyle.Render("✗ " + f.message)
		}
	} else if f.message != "" && left == "" {
		left = f.hintStyle.Render(f.message)
	}

	// Right side: keyboard hints based on focused panel
	right = f.keyboardHints()

	// Combine with spacing
	sep := f.separatorStyle.Render(" │ ")

	if left != "" && right != "" {
		return left + sep + right
	} else if left != "" {
		return left
	}
	return right
}

// keyboardHints returns context-sensitive keyboard hints.
func (f *Footer) keyboardHints() string {
	if f.sessionDone {
		return f.hintStyle.Render("Press q to exit")
	}

	// Base hints
	hints := "←/→ panels"

	// Panel-specific hints
	switch f.focusedPanel {
	case 0: // Tasks panel
		hints += " │ ↑/↓ scroll │ r retry"
	case 1: // Agents panel
		hints += " │ ↑/↓/←/→ nav"
	case 2: // Logs panel
		hints += " │ ↑/↓ scroll │ f filter │ a auto-scroll"
	}

	hints += " │ q quit"

	return f.hintStyle.Render(hints)
}

// PanelName returns the name of the given panel index.
func PanelName(panel int) string {
	switch panel {
	case 0:
		return "Tasks"
	case 1:
		return "Agents"
	case 2:
		return "Logs"
	default:
		return fmt.Sprintf("Panel %d", panel)
	}
}
