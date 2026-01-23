package tui

import (
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/pkg/models"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TasksPanel displays a scrollable list of tasks with status indicators.
// Tasks are grouped under their parent epics with expand/collapse support.
type TasksPanel struct {
	tasks        []*models.Task
	selected     int
	scrollOffset int
	width        int
	height       int
	focused      bool
	collapsed    map[string]bool // Map of epic ID -> collapsed state

	// Rendered lines for navigation
	visibleItems []visibleItem

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
	sectionStyle  lipgloss.Style
	epicStyle     lipgloss.Style
	childStyle    lipgloss.Style
}

// visibleItem represents a rendered line that can be selected.
type visibleItem struct {
	taskID   string
	isEpic   bool
	parentID string // For subtasks
}

// NewTasksPanel creates a new TasksPanel instance.
func NewTasksPanel() *TasksPanel {
	return &TasksPanel{
		tasks:        make([]*models.Task, 0),
		selected:     0,
		collapsed:    make(map[string]bool),
		visibleItems: make([]visibleItem, 0),

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

		sectionStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true),

		epicStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("75")), // Light blue for epics

		childStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")), // Dimmer for child indicator
	}
}

// SetTasks updates the list of tasks.
func (p *TasksPanel) SetTasks(tasks []*models.Task) {
	p.tasks = tasks
	// Rebuild visible items after task update
	p.buildVisibleItems()
	// Ensure selected index is valid
	if p.selected >= len(p.visibleItems) {
		p.selected = len(p.visibleItems) - 1
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

// buildVisibleItems creates the list of selectable items based on current collapse state.
func (p *TasksPanel) buildVisibleItems() {
	p.visibleItems = make([]visibleItem, 0)

	// Group tasks by parent
	epics := make(map[string][]*models.Task)   // parentID -> children
	rootTasks := make([]*models.Task, 0)       // Tasks without parent
	epicTasks := make(map[string]*models.Task) // epicID -> epic task

	for _, task := range p.tasks {
		if task.ParentID == "" {
			// Check if this task has children (is an epic)
			hasChildren := false
			for _, other := range p.tasks {
				if other.ParentID == task.ID {
					hasChildren = true
					break
				}
			}
			if hasChildren {
				epicTasks[task.ID] = task
			} else {
				rootTasks = append(rootTasks, task)
			}
		} else {
			epics[task.ParentID] = append(epics[task.ParentID], task)
		}
	}

	// Add epics and their children
	for epicID, children := range epics {
		// Add the epic itself
		p.visibleItems = append(p.visibleItems, visibleItem{
			taskID: epicID,
			isEpic: true,
		})

		// Add children if not collapsed
		if !p.collapsed[epicID] {
			for _, child := range children {
				p.visibleItems = append(p.visibleItems, visibleItem{
					taskID:   child.ID,
					isEpic:   false,
					parentID: epicID,
				})
			}
		}
	}

	// Add standalone root tasks (tasks without parent and without children)
	for _, task := range rootTasks {
		p.visibleItems = append(p.visibleItems, visibleItem{
			taskID: task.ID,
			isEpic: false,
		})
	}
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
			if p.selected < len(p.visibleItems)-1 {
				p.selected++
				p.ensureVisible()
			}
		case "enter":
			// Toggle collapse for epics
			if p.selected >= 0 && p.selected < len(p.visibleItems) {
				item := p.visibleItems[p.selected]
				if item.isEpic {
					p.collapsed[item.taskID] = !p.collapsed[item.taskID]
					p.buildVisibleItems()
				}
			}
		case "r":
			// Retry failed task
			task := p.SelectedTask()
			if task != nil && task.Status == models.TaskStatusFailed {
				return p, func() tea.Msg {
					return TaskRetryMsg{
						TaskID:    task.ID,
						TaskTitle: task.Title,
						Tier:      nil, // TODO: tier removed - always use default
					}
				}
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

// View renders the tasks panel with hierarchical task display.
func (p *TasksPanel) View() string {
	var b strings.Builder

	// Title
	title := "Tasks"
	if p.focused {
		title = "[Tasks]"
	}
	b.WriteString(p.titleStyle.Render(title))
	b.WriteString("\n")

	if len(p.tasks) == 0 {
		b.WriteString(p.normalStyle.Render("  No tasks"))
	} else {
		// Build task index for lookup
		taskIndex := make(map[string]*models.Task)
		for _, task := range p.tasks {
			taskIndex[task.ID] = task
		}

		// Count active and completed
		activeCount := 0
		completedCount := 0
		for _, task := range p.tasks {
			if task.Status == models.TaskStatusDone || task.Status == models.TaskStatusFailed {
				completedCount++
			} else {
				activeCount++
			}
		}

		// Section header
		b.WriteString(p.sectionStyle.Render(fmt.Sprintf(" Tasks (%d active, %d done)", activeCount, completedCount)))
		b.WriteString("\n")

		// Render visible items
		for i, item := range p.visibleItems {
			task := taskIndex[item.taskID]
			isSelected := i == p.selected

			var line string
			if item.isEpic {
				line = p.renderEpicLine(task, item.taskID, isSelected)
			} else if item.parentID != "" {
				line = p.renderChildLine(task, isSelected)
			} else {
				line = p.renderTaskLine(task, isSelected)
			}

			b.WriteString(line)
			if i < len(p.visibleItems)-1 {
				b.WriteString("\n")
			}
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

// renderEpicLine renders an epic (parent task) line with collapse indicator.
func (p *TasksPanel) renderEpicLine(task *models.Task, epicID string, selected bool) string {
	// Collapse indicator
	collapseIcon := "▼"
	if p.collapsed[epicID] {
		collapseIcon = "▶"
	}

	// Status icon - use pending if task is nil
	var icon string
	if task != nil {
		icon = p.statusIcon(task.Status)
	} else {
		icon = p.runningStyle.Render(iconRunning)
	}

	// Count children
	childCount := 0
	doneCount := 0
	for _, t := range p.tasks {
		if t.ParentID == epicID {
			childCount++
			if t.Status == models.TaskStatusDone || t.Status == models.TaskStatusFailed {
				doneCount++
			}
		}
	}

	// Truncate title
	maxTitleLen := p.width - 18 // Account for icons, counters, padding
	if maxTitleLen < 10 {
		maxTitleLen = 10
	}
	title := "(epic)"
	if task != nil {
		title = task.Title
	}
	if len(title) > maxTitleLen {
		title = title[:maxTitleLen-3] + "..."
	}

	// Build line with child count
	countStr := fmt.Sprintf("[%d/%d]", doneCount, childCount)
	line := fmt.Sprintf(" %s %s %s %s", collapseIcon, icon, p.epicStyle.Render(title), p.childStyle.Render(countStr))

	if selected {
		return p.selectedStyle.Render(line)
	}
	return p.normalStyle.Render(line)
}

// renderChildLine renders a child task with indentation.
func (p *TasksPanel) renderChildLine(task *models.Task, selected bool) string {
	if task == nil {
		return ""
	}

	icon := p.statusIcon(task.Status)

	// Add agent ID if assigned
	agentSuffix := ""
	if task.AssignedTo != "" {
		agentShort := task.AssignedTo
		if len(agentShort) > 8 {
			agentShort = agentShort[:8]
		}
		agentSuffix = fmt.Sprintf(" [%s]", agentShort)
	}

	// Truncate title
	maxTitleLen := p.width - 12 - len(agentSuffix) // Account for indent, icon, padding, agent
	if maxTitleLen < 10 {
		maxTitleLen = 10
	}
	title := task.Title
	if len(title) > maxTitleLen {
		title = title[:maxTitleLen-3] + "..."
	}

	line := fmt.Sprintf("   └─ %s %s%s", icon, title, p.childStyle.Render(agentSuffix))

	// Add error preview for failed tasks
	if task.Status == models.TaskStatusFailed && task.Error != "" {
		errPreview := task.Error
		maxErrLen := p.width - 14
		if maxErrLen < 20 {
			maxErrLen = 20
		}
		if len(errPreview) > maxErrLen {
			errPreview = errPreview[:maxErrLen-3] + "..."
		}
		line += "\n       " + p.failedStyle.Render(errPreview)
	}

	if selected {
		return p.selectedStyle.Render(line)
	}
	return p.normalStyle.Render(line)
}

// renderTaskLine renders a standalone task line.
func (p *TasksPanel) renderTaskLine(task *models.Task, selected bool) string {
	if task == nil {
		return ""
	}

	icon := p.statusIcon(task.Status)

	// Add agent ID if assigned
	agentSuffix := ""
	if task.AssignedTo != "" {
		agentShort := task.AssignedTo
		if len(agentShort) > 8 {
			agentShort = agentShort[:8]
		}
		agentSuffix = fmt.Sprintf(" [%s]", agentShort)
	}

	// Truncate title to fit
	maxTitleLen := p.width - 8 - len(agentSuffix) // Account for icon, padding, border, agent
	if maxTitleLen < 10 {
		maxTitleLen = 10
	}
	title := task.Title
	if len(title) > maxTitleLen {
		title = title[:maxTitleLen-3] + "..."
	}

	line := fmt.Sprintf(" %s %s%s", icon, title, p.childStyle.Render(agentSuffix))

	// Add error preview for failed tasks
	if task.Status == models.TaskStatusFailed && task.Error != "" {
		errPreview := task.Error
		maxErrLen := p.width - 10
		if maxErrLen < 20 {
			maxErrLen = 20
		}
		if len(errPreview) > maxErrLen {
			errPreview = errPreview[:maxErrLen-3] + "..."
		}
		line += "\n     " + p.failedStyle.Render(errPreview)
	}

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
	if len(p.visibleItems) == 0 || p.selected >= len(p.visibleItems) || p.selected < 0 {
		return nil
	}

	item := p.visibleItems[p.selected]
	for _, task := range p.tasks {
		if task.ID == item.taskID {
			return task
		}
	}
	return nil
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
