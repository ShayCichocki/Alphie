package tui

import (
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/pkg/models"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// GraphView displays an ASCII visualization of task dependencies.
type GraphView struct {
	tasks    []*models.Task
	selected string
	width    int
	height   int

	// Scrolling state
	scrollOffset int
	visibleRows  int

	// Collapse/expand state: maps parent task ID to collapsed state
	collapsed map[string]bool

	// Cached rendered lines for scrolling
	renderedLines []renderedLine

	// Styles
	headerStyle   lipgloss.Style
	nodeStyle     lipgloss.Style
	selectedStyle lipgloss.Style
	arrowStyle    lipgloss.Style
	statusDone    lipgloss.Style
	statusRunning lipgloss.Style
	statusWaiting lipgloss.Style
	statusBlocked lipgloss.Style
	statusPending lipgloss.Style
	collapseStyle lipgloss.Style
}

// renderedLine represents a single line in the graph with its associated task.
type renderedLine struct {
	taskID   string
	text     string
	depth    int
	isParent bool
}

// NewGraphView creates a new GraphView instance.
func NewGraphView() *GraphView {
	return &GraphView{
		tasks:        make([]*models.Task, 0),
		selected:     "",
		collapsed:    make(map[string]bool),
		visibleRows:  20,
		scrollOffset: 0,

		headerStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("7")).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("240")),

		nodeStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),

		selectedStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("15")).
			Bold(true),

		arrowStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),

		statusDone: lipgloss.NewStyle().
			Foreground(lipgloss.Color("28")), // Dark green

		statusRunning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")), // Green

		statusWaiting: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")), // Orange

		statusBlocked: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")), // Red

		statusPending: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")), // Gray

		collapseStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),
	}
}

// Update handles input messages.
func (g *GraphView) Update(msg tea.Msg) (*GraphView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			g.selectPrevious()
			g.ensureSelectedVisible()
		case "down", "j":
			g.selectNext()
			g.ensureSelectedVisible()
		case "enter":
			if task := g.SelectedTask(); task != nil {
				return g, g.showTaskDetails(task)
			}
		case " ", "space":
			// Toggle collapse/expand for selected task
			g.toggleCollapse()
		case "c":
			// Collapse all
			g.collapseAll()
		case "e":
			// Expand all
			g.expandAll()
		case "pgup", "ctrl+u":
			g.scrollUp(g.visibleRows / 2)
		case "pgdown", "ctrl+d":
			g.scrollDown(g.visibleRows / 2)
		case "home", "g":
			g.scrollToTop()
		case "end", "G":
			g.scrollToBottom()
		}

	case tea.WindowSizeMsg:
		g.width = msg.Width
		g.height = msg.Height
		// Reserve space for header, footer, and padding
		g.visibleRows = msg.Height - 8
		if g.visibleRows < 5 {
			g.visibleRows = 5
		}
	}

	return g, nil
}

// View renders the dependency graph.
func (g *GraphView) View() string {
	if len(g.tasks) == 0 {
		return g.nodeStyle.Render("No tasks to display")
	}

	var b strings.Builder

	// Header with task count
	totalTasks := len(g.tasks)
	header := fmt.Sprintf("Task Dependency Graph (%d tasks)", totalTasks)
	b.WriteString(g.headerStyle.Render(header))
	b.WriteString("\n\n")

	// Build rendered lines
	g.buildRenderedLines()

	// Calculate visible range with bounds checking
	totalLines := len(g.renderedLines)
	if totalLines == 0 {
		b.WriteString(g.nodeStyle.Render("No visible tasks"))
		return b.String()
	}

	// Ensure scroll offset is valid
	if g.scrollOffset < 0 {
		g.scrollOffset = 0
	}
	maxOffset := totalLines - g.visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if g.scrollOffset > maxOffset {
		g.scrollOffset = maxOffset
	}

	// Render visible lines
	endIdx := g.scrollOffset + g.visibleRows
	if endIdx > totalLines {
		endIdx = totalLines
	}

	for i := g.scrollOffset; i < endIdx; i++ {
		line := g.renderedLines[i]

		// Apply selection styling
		if line.taskID == g.selected {
			b.WriteString(g.selectedStyle.Render(line.text))
		} else {
			b.WriteString(g.nodeStyle.Render(line.text))
		}
		b.WriteString("\n")
	}

	// Add scroll indicators if needed
	if totalLines > g.visibleRows {
		scrollInfo := g.renderScrollInfo(totalLines)
		b.WriteString("\n")
		b.WriteString(scrollInfo)
	}

	// Footer with key hints
	b.WriteString("\n")
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("[enter] details  [j/k] navigate  [space] collapse/expand  [c/e] collapse/expand all")
	b.WriteString(footer)

	return b.String()
}

// SetTasks updates the list of tasks to display.
func (g *GraphView) SetTasks(tasks []*models.Task) {
	g.tasks = tasks
	// If no selection and we have tasks, select the first one
	if g.selected == "" && len(tasks) > 0 {
		g.selected = tasks[0].ID
	}
	// Verify selection still exists
	found := false
	for _, task := range tasks {
		if task.ID == g.selected {
			found = true
			break
		}
	}
	if !found && len(tasks) > 0 {
		g.selected = tasks[0].ID
	}
	// Rebuild rendered lines
	g.buildRenderedLines()
	g.ensureSelectedVisible()
}

// SelectTask sets the currently selected task by ID.
func (g *GraphView) SelectTask(id string) {
	for _, task := range g.tasks {
		if task.ID == id {
			g.selected = id
			g.ensureSelectedVisible()
			return
		}
	}
}

// SelectedTask returns the currently selected task.
func (g *GraphView) SelectedTask() *models.Task {
	for _, task := range g.tasks {
		if task.ID == g.selected {
			return task
		}
	}
	return nil
}

// TaskDetailsMsg is sent when task details are requested.
type TaskDetailsMsg struct {
	Task *models.Task
}

// showTaskDetails sends a message to show task details.
func (g *GraphView) showTaskDetails(task *models.Task) tea.Cmd {
	return func() tea.Msg {
		return TaskDetailsMsg{Task: task}
	}
}
