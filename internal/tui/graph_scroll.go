package tui

// Scrolling and navigation methods for GraphView.

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
