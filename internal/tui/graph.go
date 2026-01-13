package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shayc/alphie/pkg/models"
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

// buildRenderedLines creates the cached rendered lines for the graph.
func (g *GraphView) buildRenderedLines() {
	g.renderedLines = make([]renderedLine, 0, len(g.tasks))

	// Build task index for quick lookup
	taskIndex := make(map[string]*models.Task)
	for _, task := range g.tasks {
		taskIndex[task.ID] = task
	}

	// Group tasks by parent (epic hierarchy)
	epics := make(map[string][]*models.Task)
	var rootTasks []*models.Task

	for _, task := range g.tasks {
		if task.ParentID == "" {
			rootTasks = append(rootTasks, task)
		} else {
			epics[task.ParentID] = append(epics[task.ParentID], task)
		}
	}

	// Render root tasks and their children
	for _, task := range rootTasks {
		g.buildTaskLines(task, taskIndex, epics, 0)
	}

	// Render orphaned children (tasks whose parent is not in the list)
	for parentID, children := range epics {
		if _, exists := taskIndex[parentID]; !exists {
			// Parent not in our task list, show as orphaned epic header
			epicLine := g.arrowStyle.Render(fmt.Sprintf("  [Epic: %s]", truncate(parentID, 12)))
			g.renderedLines = append(g.renderedLines, renderedLine{
				taskID:   "",
				text:     epicLine,
				depth:    0,
				isParent: true,
			})
			for _, child := range children {
				g.buildTaskLines(child, taskIndex, epics, 1)
			}
		}
	}
}

// buildTaskLines builds rendered lines for a task and its children.
func (g *GraphView) buildTaskLines(task *models.Task, taskIndex map[string]*models.Task, epics map[string][]*models.Task, depth int) {
	children, hasChildren := epics[task.ID]

	// Build the task line
	indent := strings.Repeat("  ", depth)
	prefix := ""

	if depth > 0 {
		prefix = g.arrowStyle.Render("|-- ")
	}

	// Collapse indicator for parent tasks
	collapseIndicator := ""
	if hasChildren {
		if g.collapsed[task.ID] {
			collapseIndicator = g.collapseStyle.Render("[+] ")
		} else {
			collapseIndicator = g.collapseStyle.Render("[-] ")
		}
	} else {
		collapseIndicator = "    "
	}

	// Status icon
	icon := g.statusIcon(task.Status)

	// Task line
	taskLine := fmt.Sprintf("%s%s%s%s %s", indent, prefix, collapseIndicator, icon, truncate(task.Title, 35))

	// Add dependency info
	if len(task.DependsOn) > 0 {
		depInfo := g.renderDependencies(task.DependsOn, taskIndex)
		taskLine += " " + g.arrowStyle.Render(depInfo)
	}

	// Add child count if collapsed
	if hasChildren && g.collapsed[task.ID] {
		childCount := g.countDescendants(task.ID, epics)
		taskLine += g.collapseStyle.Render(fmt.Sprintf(" (%d hidden)", childCount))
	}

	g.renderedLines = append(g.renderedLines, renderedLine{
		taskID:   task.ID,
		text:     taskLine,
		depth:    depth,
		isParent: hasChildren,
	})

	// Render children if not collapsed
	if hasChildren && !g.collapsed[task.ID] {
		for _, child := range children {
			g.buildTaskLines(child, taskIndex, epics, depth+1)
		}
	}
}

// countDescendants counts all descendants of a task.
func (g *GraphView) countDescendants(taskID string, epics map[string][]*models.Task) int {
	count := 0
	if children, ok := epics[taskID]; ok {
		count += len(children)
		for _, child := range children {
			count += g.countDescendants(child.ID, epics)
		}
	}
	return count
}

// renderScrollInfo renders scroll position information.
func (g *GraphView) renderScrollInfo(totalLines int) string {
	startLine := g.scrollOffset + 1
	endLine := g.scrollOffset + g.visibleRows
	if endLine > totalLines {
		endLine = totalLines
	}

	percent := 0
	if totalLines > g.visibleRows {
		percent = (g.scrollOffset * 100) / (totalLines - g.visibleRows)
	}

	indicators := ""
	if g.scrollOffset > 0 {
		indicators += "[up]"
	}
	if g.scrollOffset+g.visibleRows < totalLines {
		if indicators != "" {
			indicators += " "
		}
		indicators += "[down]"
	}

	return g.arrowStyle.Render(fmt.Sprintf("Lines %d-%d of %d (%d%%) %s", startLine, endLine, totalLines, percent, indicators))
}

// renderDependencies creates a string showing blocked-by relationships.
func (g *GraphView) renderDependencies(deps []string, taskIndex map[string]*models.Task) string {
	if len(deps) == 0 {
		return ""
	}

	var depIcons []string
	for _, depID := range deps {
		if depTask, exists := taskIndex[depID]; exists {
			icon := g.statusIconRaw(depTask.Status)
			depIcons = append(depIcons, fmt.Sprintf("%s%s", icon, truncate(depID, 8)))
		} else {
			depIcons = append(depIcons, fmt.Sprintf("[?]%s", truncate(depID, 8)))
		}
	}

	return "<-- " + strings.Join(depIcons, ", ")
}

// statusIcon returns the styled status icon for a task.
func (g *GraphView) statusIcon(status models.TaskStatus) string {
	switch status {
	case models.TaskStatusDone:
		return g.statusDone.Render(iconDone)
	case models.TaskStatusInProgress:
		return g.statusRunning.Render(iconRunning)
	case models.TaskStatusBlocked:
		return g.statusWaiting.Render(iconWaiting)
	case models.TaskStatusFailed:
		return g.statusBlocked.Render(iconFailed)
	case models.TaskStatusPending:
		return g.statusPending.Render(iconPending)
	default:
		return g.statusPending.Render(iconPending)
	}
}

// statusIconRaw returns the raw status icon for a task (for dependency display).
func (g *GraphView) statusIconRaw(status models.TaskStatus) string {
	switch status {
	case models.TaskStatusDone:
		return iconDone
	case models.TaskStatusInProgress:
		return iconRunning
	case models.TaskStatusBlocked:
		return iconWaiting
	case models.TaskStatusFailed:
		return iconFailed
	case models.TaskStatusPending:
		return iconPending
	default:
		return iconPending
	}
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

// selectPrevious moves selection to the previous visible task.
func (g *GraphView) selectPrevious() {
	if len(g.renderedLines) == 0 {
		return
	}

	// Find current position in rendered lines
	currentIdx := -1
	for i, line := range g.renderedLines {
		if line.taskID == g.selected {
			currentIdx = i
			break
		}
	}

	// Find previous selectable line (skip non-task lines)
	for i := currentIdx - 1; i >= 0; i-- {
		if g.renderedLines[i].taskID != "" {
			g.selected = g.renderedLines[i].taskID
			return
		}
	}
}

// selectNext moves selection to the next visible task.
func (g *GraphView) selectNext() {
	if len(g.renderedLines) == 0 {
		return
	}

	// Find current position in rendered lines
	currentIdx := -1
	for i, line := range g.renderedLines {
		if line.taskID == g.selected {
			currentIdx = i
			break
		}
	}

	// Find next selectable line (skip non-task lines)
	for i := currentIdx + 1; i < len(g.renderedLines); i++ {
		if g.renderedLines[i].taskID != "" {
			g.selected = g.renderedLines[i].taskID
			return
		}
	}
}

// ensureSelectedVisible scrolls to make the selected task visible.
func (g *GraphView) ensureSelectedVisible() {
	if len(g.renderedLines) == 0 {
		return
	}

	// Find selected line index
	selectedIdx := -1
	for i, line := range g.renderedLines {
		if line.taskID == g.selected {
			selectedIdx = i
			break
		}
	}

	if selectedIdx < 0 {
		return
	}

	// Adjust scroll offset to make selected visible
	if selectedIdx < g.scrollOffset {
		g.scrollOffset = selectedIdx
	} else if selectedIdx >= g.scrollOffset+g.visibleRows {
		g.scrollOffset = selectedIdx - g.visibleRows + 1
	}
}

// toggleCollapse toggles the collapse state of the selected task.
func (g *GraphView) toggleCollapse() {
	if g.selected == "" {
		return
	}

	// Check if selected task has children
	for _, task := range g.tasks {
		if task.ParentID == g.selected {
			// Has children, toggle collapse
			g.collapsed[g.selected] = !g.collapsed[g.selected]
			g.buildRenderedLines()
			return
		}
	}
}

// collapseAll collapses all parent tasks.
func (g *GraphView) collapseAll() {
	// Find all tasks that have children
	hasChildren := make(map[string]bool)
	for _, task := range g.tasks {
		if task.ParentID != "" {
			hasChildren[task.ParentID] = true
		}
	}

	// Collapse all parents
	for parentID := range hasChildren {
		g.collapsed[parentID] = true
	}
	g.buildRenderedLines()
	g.ensureSelectedVisible()
}

// expandAll expands all collapsed tasks.
func (g *GraphView) expandAll() {
	g.collapsed = make(map[string]bool)
	g.buildRenderedLines()
	g.ensureSelectedVisible()
}

// scrollUp scrolls up by n lines.
func (g *GraphView) scrollUp(n int) {
	g.scrollOffset -= n
	if g.scrollOffset < 0 {
		g.scrollOffset = 0
	}
}

// scrollDown scrolls down by n lines.
func (g *GraphView) scrollDown(n int) {
	maxOffset := len(g.renderedLines) - g.visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	g.scrollOffset += n
	if g.scrollOffset > maxOffset {
		g.scrollOffset = maxOffset
	}
}

// scrollToTop scrolls to the top.
func (g *GraphView) scrollToTop() {
	g.scrollOffset = 0
	// Select first selectable item
	for _, line := range g.renderedLines {
		if line.taskID != "" {
			g.selected = line.taskID
			break
		}
	}
}

// scrollToBottom scrolls to the bottom.
func (g *GraphView) scrollToBottom() {
	g.scrollOffset = len(g.renderedLines) - g.visibleRows
	if g.scrollOffset < 0 {
		g.scrollOffset = 0
	}
	// Select last selectable item
	for i := len(g.renderedLines) - 1; i >= 0; i-- {
		if g.renderedLines[i].taskID != "" {
			g.selected = g.renderedLines[i].taskID
			break
		}
	}
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
