package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Header renders the Alphie logo and title bar.
type Header struct {
	width int
}

// NewHeader creates a new Header.
func NewHeader() *Header {
	return &Header{
		width: 80,
	}
}

// SetWidth sets the header width.
func (h *Header) SetWidth(width int) {
	h.width = width
}

// View renders the header.
func (h *Header) View() string {
	// Gradient colors for the logo
	colors := []string{"#FF6B6B", "#FF8E53", "#FFC857", "#4ECDC4", "#45B7D1", "#96E6A1"}

	logo := []string{
		"  █████╗ ██╗     ██████╗ ██╗  ██╗██╗███████╗",
		" ██╔══██╗██║     ██╔══██╗██║  ██║██║██╔════╝",
		" ███████║██║     ██████╔╝███████║██║█████╗  ",
		" ██╔══██║██║     ██╔═══╝ ██╔══██║██║██╔══╝  ",
		" ██║  ██║███████╗██║     ██║  ██║██║███████╗",
		" ╚═╝  ╚═╝╚══════╝╚═╝     ╚═╝  ╚═╝╚═╝╚══════╝",
	}

	// Apply gradient to each line
	var styledLines []string
	for i, line := range logo {
		color := colors[i%len(colors)]
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
		styledLines = append(styledLines, style.Render(line))
	}

	// Join lines
	logoBlock := lipgloss.JoinVertical(lipgloss.Left, styledLines...)

	// Subtitle
	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Italic(true).
		Render("Agent Orchestrator & Learning Engine")

	// Center the logo and subtitle
	logoStyle := lipgloss.NewStyle().
		Width(h.width).
		Align(lipgloss.Center).
		MarginTop(3).
		PaddingBottom(1)

	return logoStyle.Render(lipgloss.JoinVertical(lipgloss.Center, logoBlock, subtitle))
}

// Height returns the header height in lines.
func (h *Header) Height() int {
	return 12 // 3 margin + 6 logo lines + 1 subtitle + 1 padding + 1 newline
}
