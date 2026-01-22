package tui

// PanelDimensions holds calculated dimensions for each panel in the layout.
type PanelDimensions struct {
	// TasksWidth is the width of the tasks panel (left).
	TasksWidth int
	// AgentsWidth is the width of the agents panel (center).
	AgentsWidth int
	// LogsWidth is the width of the logs panel (right).
	LogsWidth int
	// ContentHeight is the height available for panel content (excluding footer).
	ContentHeight int
}

// LayoutManager calculates panel dimensions based on terminal size.
type LayoutManager struct {
	// totalWidth is the terminal width.
	totalWidth int
	// totalHeight is the terminal height.
	totalHeight int
	// headerHeight is the height reserved for the header.
	headerHeight int
	// footerHeight is the height reserved for the footer (default 1).
	footerHeight int
}

// NewLayoutManager creates a new LayoutManager with the given terminal dimensions.
func NewLayoutManager(width, height int) *LayoutManager {
	return &LayoutManager{
		totalWidth:   width,
		totalHeight:  height,
		headerHeight: 12, // margin (3) + logo (6) + subtitle (1) + padding (1) + newline (1)
		footerHeight: 1,
	}
}

// SetSize updates the terminal dimensions.
func (l *LayoutManager) SetSize(width, height int) {
	l.totalWidth = width
	l.totalHeight = height
}

// Calculate returns the panel dimensions based on current terminal size.
// Layout ratios: Tasks 20%, Agents 50%, Logs 30%
func (l *LayoutManager) Calculate() PanelDimensions {
	// Minimum widths to ensure usability
	const (
		minTasksWidth  = 20
		minAgentsWidth = 30
		minLogsWidth   = 20
	)

	// Calculate proportional widths
	tasksWidth := int(float64(l.totalWidth) * 0.20)
	agentsWidth := int(float64(l.totalWidth) * 0.50)
	logsWidth := l.totalWidth - tasksWidth - agentsWidth // Remainder goes to logs

	// Ensure minimum widths
	if tasksWidth < minTasksWidth {
		tasksWidth = minTasksWidth
	}
	if agentsWidth < minAgentsWidth {
		agentsWidth = minAgentsWidth
	}
	if logsWidth < minLogsWidth {
		logsWidth = minLogsWidth
	}

	// If total exceeds available width, scale down proportionally
	total := tasksWidth + agentsWidth + logsWidth
	if total > l.totalWidth {
		scale := float64(l.totalWidth) / float64(total)
		tasksWidth = int(float64(tasksWidth) * scale)
		agentsWidth = int(float64(agentsWidth) * scale)
		logsWidth = l.totalWidth - tasksWidth - agentsWidth
	}

	// Calculate content height (excluding header and footer)
	contentHeight := l.totalHeight - l.headerHeight - l.footerHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	return PanelDimensions{
		TasksWidth:    tasksWidth,
		AgentsWidth:   agentsWidth,
		LogsWidth:     logsWidth,
		ContentHeight: contentHeight,
	}
}

// TotalWidth returns the current terminal width.
func (l *LayoutManager) TotalWidth() int {
	return l.totalWidth
}

// TotalHeight returns the current terminal height.
func (l *LayoutManager) TotalHeight() int {
	return l.totalHeight
}

// FooterHeight returns the height reserved for the footer.
func (l *LayoutManager) FooterHeight() int {
	return l.footerHeight
}

// HeaderHeight returns the height reserved for the header.
func (l *LayoutManager) HeaderHeight() int {
	return l.headerHeight
}

// SetHeaderHeight sets the header height (use 0 to disable header).
func (l *LayoutManager) SetHeaderHeight(height int) {
	l.headerHeight = height
}

// CalculateMainTab returns dimensions for Tab 1 (Tasks + Agents only, no logs).
// Layout: Tasks 40%, Agents 60%
func (l *LayoutManager) CalculateMainTab(tabBarHeight int) PanelDimensions {
	const minTasksWidth = 30

	// Calculate proportional widths (40% Tasks, 60% Agents)
	tasksWidth := l.totalWidth * 40 / 100
	if tasksWidth < minTasksWidth {
		tasksWidth = minTasksWidth
	}
	agentsWidth := l.totalWidth - tasksWidth

	// Content height excluding header, footer, and tab bar
	contentHeight := l.totalHeight - l.headerHeight - l.footerHeight - tabBarHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	return PanelDimensions{
		TasksWidth:    tasksWidth,
		AgentsWidth:   agentsWidth,
		LogsWidth:     0, // Not used in Tab 1
		ContentHeight: contentHeight,
	}
}

// CalculateLogsTab returns dimensions for Tab 2 (full-screen logs).
func (l *LayoutManager) CalculateLogsTab(tabBarHeight int) PanelDimensions {
	// Content height excluding header, footer, and tab bar
	contentHeight := l.totalHeight - l.headerHeight - l.footerHeight - tabBarHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	return PanelDimensions{
		TasksWidth:    0,
		AgentsWidth:   0,
		LogsWidth:     l.totalWidth,
		ContentHeight: contentHeight,
	}
}
