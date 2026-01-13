package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shayc/alphie/pkg/models"
)

// TasksPanel displays a scrollable list of tasks with status indicators.
type TasksPanel struct {
	tasks        []*models.Task
	selected     int
	scrollOffset int
	width        int
	height       int
	focused      bool

	// Styles
	titleStyle    lipgloss.Style
	borderStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	normalStyle   lipgloss.Style
	pendingStyle  lipgloss.Style
	runningStyle  lipgloss.Style
	doneStyle     lipgloss.Style
	failedStyle   lipgloss.Style
	blockedStyle  lipgloss.Style
}

// NewTasksPanel creates a new TasksPanel instance.
func NewTasksPanel() *TasksPanel {
	return &TasksPanel{
		tasks:    make([]*models.Task, 0),
		selected: 0,

		titleStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1),

		borderStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")),

		selectedStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("15")).
			Bold(true),

		normalStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),

		pendingStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")), // Gray

		runningStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")), // Green

		doneStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("28")), // Dark green

		failedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")), // Red

		blockedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")), // Orange
	}
}

// SetTasks updates the list of tasks.
func (p *TasksPanel) SetTasks(tasks []*models.Task) {
	p.tasks = tasks
	// Ensure selected index is valid
	if p.selected >= len(tasks) {
		p.selected = len(tasks) - 1
	}
	if p.selected < 0 {
		p.selected = 0
	}
}

// SetSize updates the panel dimensions.
func (p *TasksPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetFocused sets whether this panel has keyboard focus.
func (p *TasksPanel) SetFocused(focused bool) {
	p.focused = focused
}

// Update handles input messages.
func (p *TasksPanel) Update(msg tea.Msg) (*TasksPanel, tea.Cmd) {
	if !p.focused {
		return p, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.selected > 0 {
				p.selected--
				p.ensureVisible()
			}
		case "down", "j":
			if p.selected < len(p.tasks)-1 {
				p.selected++
				p.ensureVisible()
			}
		}
	}

	return p, nil
}

// ensureVisible adjusts scroll offset to keep selected item visible.
func (p *TasksPanel) ensureVisible() {
	// Account for title and borders
	visibleRows := p.height - 4
	if visibleRows < 1 {
		visibleRows = 1
	}

	if p.selected < p.scrollOffset {
		p.scrollOffset = p.selected
	} else if p.selected >= p.scrollOffset+visibleRows {
		p.scrollOffset = p.selected - visibleRows + 1
	}
}

// View renders the tasks panel.
func (p *TasksPanel) View() string {
	var b strings.Builder

	// Title
	title := "Tasks"
	if p.focused {
		title = "[Tasks]"
	}
	b.WriteString(p.titleStyle.Render(title))
	b.WriteString("\n")

	// Calculate visible area
	contentHeight := p.height - 4 // Account for title, borders, padding
	if contentHeight < 1 {
		contentHeight = 1
	}

	if len(p.tasks) == 0 {
		b.WriteString(p.normalStyle.Render("  No tasks"))
	} else {
		// Render visible tasks
		endIdx := p.scrollOffset + contentHeight
		if endIdx > len(p.tasks) {
			endIdx = len(p.tasks)
		}

		for i := p.scrollOffset; i < endIdx; i++ {
			task := p.tasks[i]
			line := p.renderTaskLine(task, i == p.selected)
			b.WriteString(line)
			b.WriteString("\n")
		}

		// Show scroll indicators if needed
		if p.scrollOffset > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  ... more above"))
			b.WriteString("\n")
		}
		if endIdx < len(p.tasks) {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  ... more below"))
		}
	}

	// Apply border and size constraints
	content := b.String()
	borderColor := lipgloss.Color("240")
	if p.focused {
		borderColor = lipgloss.Color("63") // Blue when focused
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(p.width - 2). // Account for border
		Height(p.height - 2).
		Render(content)
}

// renderTaskLine renders a single task line.
func (p *TasksPanel) renderTaskLine(task *models.Task, selected bool) string {
	icon := p.statusIcon(task.Status)

	// Truncate title to fit
	maxTitleLen := p.width - 8 // Account for icon, padding, border
	if maxTitleLen < 10 {
		maxTitleLen = 10
	}
	title := task.Title
	if len(title) > maxTitleLen {
		title = title[:maxTitleLen-3] + "..."
	}

	line := fmt.Sprintf(" %s %s", icon, title)

	if selected {
		return p.selectedStyle.Render(line)
	}
	return p.normalStyle.Render(line)
}

// statusIcon returns the appropriate icon for a task status.
func (p *TasksPanel) statusIcon(status models.TaskStatus) string {
	switch status {
	case models.TaskStatusPending:
		return p.pendingStyle.Render(iconPending)
	case models.TaskStatusInProgress:
		return p.runningStyle.Render(iconRunning)
	case models.TaskStatusDone:
		return p.doneStyle.Render(iconDone)
	case models.TaskStatusFailed:
		return p.failedStyle.Render(iconFailed)
	case models.TaskStatusBlocked:
		return p.blockedStyle.Render(iconWaiting)
	default:
		return p.pendingStyle.Render(iconPending)
	}
}

// SelectedTask returns the currently selected task, or nil if none.
func (p *TasksPanel) SelectedTask() *models.Task {
	if len(p.tasks) == 0 || p.selected >= len(p.tasks) {
		return nil
	}
	return p.tasks[p.selected]
}

// TaskCount returns the total number of tasks.
func (p *TasksPanel) TaskCount() int {
	return len(p.tasks)
}

// CompletedCount returns the number of completed tasks.
func (p *TasksPanel) CompletedCount() int {
	count := 0
	for _, task := range p.tasks {
		if task.Status == models.TaskStatusDone {
			count++
		}
	}
	return count
}
