// Package tui provides the terminal user interface for Alphie.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tab index constants.
const (
	TabIndexAgents = iota
	TabIndexOutput
	TabIndexGraph
	TabIndexStats
)

// Default tab labels.
var defaultTabs = []string{"Agents", "Output", "Graph", "Stats"}

// TabBar is a navigation component for switching between views.
type TabBar struct {
	tabs   []string
	active int

	// Styles
	activeStyle   lipgloss.Style
	inactiveStyle lipgloss.Style
	barStyle      lipgloss.Style
}

// NewTabBar creates a new TabBar with default tabs.
func NewTabBar() TabBar {
	return TabBar{
		tabs:   defaultTabs,
		active: TabIndexAgents,

		activeStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Background(lipgloss.Color("236")).
			Padding(0, 2),

		inactiveStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Padding(0, 2),

		barStyle: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color("238")),
	}
}

// Update handles keyboard input for tab navigation.
func (t TabBar) Update(msg tea.Msg) (TabBar, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			t.active = (t.active + 1) % len(t.tabs)
		case "shift+tab":
			t.active = (t.active - 1 + len(t.tabs)) % len(t.tabs)
		case "1":
			t.SetActive(TabIndexAgents)
		case "2":
			t.SetActive(TabIndexOutput)
		case "3":
			t.SetActive(TabIndexGraph)
		case "4":
			t.SetActive(TabIndexStats)
		}
	}
	return t, nil
}

// View renders the tab bar.
func (t TabBar) View() string {
	var renderedTabs []string

	for i, tab := range t.tabs {
		if i == t.active {
			renderedTabs = append(renderedTabs, t.activeStyle.Render(tab))
		} else {
			renderedTabs = append(renderedTabs, t.inactiveStyle.Render(tab))
		}
	}

	return t.barStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...))
}

// SetActive sets the active tab by index.
// If the index is out of bounds, it is clamped to valid range.
func (t *TabBar) SetActive(index int) {
	if index < 0 {
		t.active = 0
	} else if index >= len(t.tabs) {
		t.active = len(t.tabs) - 1
	} else {
		t.active = index
	}
}

// Active returns the currently active tab index.
func (t TabBar) Active() int {
	return t.active
}

// Tabs returns the list of tab labels.
func (t TabBar) Tabs() []string {
	return t.tabs
}
