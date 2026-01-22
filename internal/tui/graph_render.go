package tui

import (
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// Rendering methods for GraphView.

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
