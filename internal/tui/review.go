// Package tui provides the terminal user interface for Alphie.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// ReviewRequestMsg is sent when an agent's task completes and requires human review.
type ReviewRequestMsg struct {
	AgentID         string
	Diff            string
	TaskDescription string
}

// ReviewResponseMsg is sent after the human approves or rejects the review.
type ReviewResponseMsg struct {
	AgentID  string
	Approved bool
	Reason   string
}

// ReviewResult holds the outcome of a human review.
type ReviewResult struct {
	Approved bool
	Reason   string
}


// ReviewGate displays a diff for human review and prompts for approval.
type ReviewGate struct {
	// tier determines if review is required (Architect tier).
	tier models.Tier
	// width is the viewport width.
	width int
	// height is the viewport height.
	height int
	// active indicates if a review is currently in progress.
	active bool
	// agentID is the ID of the agent being reviewed.
	agentID string
	// taskID is the ID of the task being reviewed.
	taskID string
	// diff is the diff content to display.
	diff string
	// taskDescription is the description of the completed task.
	taskDescription string
	// baseCommit is the git commit hash for the base of the diff.
	baseCommit string
	// scrollOffset is the current scroll position.
	scrollOffset int
	// diffLines is the parsed diff split into lines.
	diffLines []string

	// Styles for diff rendering.
	addStyle    lipgloss.Style
	removeStyle lipgloss.Style
	contextStyle lipgloss.Style
	headerStyle  lipgloss.Style
	promptStyle  lipgloss.Style
	titleStyle   lipgloss.Style
}

// NewReviewGate creates a new ReviewGate for the given tier.
func NewReviewGate(tier models.Tier) *ReviewGate {
	return &ReviewGate{
		tier:      tier,
		width:     80,
		height:    24,
		diffLines: make([]string, 0),

		addStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")), // Green
		removeStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")), // Red
		contextStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")), // Gray
		headerStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")). // Blue
			Bold(true),
		promptStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")). // Yellow
			Bold(true),
		titleStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Background(lipgloss.Color("236")).
			Padding(0, 2),
	}
}

// ShowReview activates the review gate with the given diff and task info.
// Returns a command that triggers the review UI.
func (r *ReviewGate) ShowReview(agentID string, diff string, taskDescription string) tea.Cmd {
	return func() tea.Msg {
		return ReviewRequestMsg{
			AgentID:         agentID,
			Diff:            diff,
			TaskDescription: taskDescription,
		}
	}
}

// Update handles input for the review gate.
func (r *ReviewGate) Update(msg tea.Msg) (*ReviewGate, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !r.active {
			return r, nil
		}

		switch msg.String() {
		case "y", "Y":
			r.active = false
			agentID := r.agentID
			r.reset()
			return r, func() tea.Msg {
				return ReviewResponseMsg{
					AgentID:  agentID,
					Approved: true,
					Reason:   "",
				}
			}

		case "n", "N":
			r.active = false
			agentID := r.agentID
			r.reset()
			return r, func() tea.Msg {
				return ReviewResponseMsg{
					AgentID:  agentID,
					Approved: false,
					Reason:   "rejected by human reviewer",
				}
			}

		case "up", "k":
			r.scrollUp()
		case "down", "j":
			r.scrollDown()
		case "pgup", "b":
			r.scrollPageUp()
		case "pgdown", "f", " ":
			r.scrollPageDown()
		case "home", "g":
			r.scrollOffset = 0
		case "end", "G":
			r.scrollToBottom()
		}

	case tea.WindowSizeMsg:
		r.width = msg.Width
		r.height = msg.Height

	case ReviewRequestMsg:
		r.agentID = msg.AgentID
		r.diff = msg.Diff
		r.taskDescription = msg.TaskDescription
		r.diffLines = strings.Split(msg.Diff, "\n")
		r.scrollOffset = 0
		r.active = true
	}

	return r, nil
}

// View renders the review gate UI.
func (r *ReviewGate) View() string {
	if !r.active {
		return ""
	}

	var sb strings.Builder

	// Title bar
	title := r.titleStyle.Render(" Human Review Required ")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	// Task description
	sb.WriteString(r.headerStyle.Render("Task: "))
	sb.WriteString(r.taskDescription)
	sb.WriteString("\n")
	sb.WriteString(r.headerStyle.Render("Agent: "))
	sb.WriteString(r.agentID)
	sb.WriteString("\n\n")

	// Diff header
	sb.WriteString(r.headerStyle.Render("Changes:"))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("-", min(r.width, 80)))
	sb.WriteString("\n")

	// Calculate visible diff area
	diffAreaHeight := r.height - 12 // Reserve space for header, footer, prompt
	if diffAreaHeight < 5 {
		diffAreaHeight = 5
	}

	// Render visible diff lines with syntax highlighting
	totalLines := len(r.diffLines)
	if r.scrollOffset > totalLines-diffAreaHeight {
		r.scrollOffset = max(0, totalLines-diffAreaHeight)
	}

	start := r.scrollOffset
	end := min(start+diffAreaHeight, totalLines)

	for i := start; i < end; i++ {
		line := r.diffLines[i]
		styledLine := r.styleDiffLine(line)
		sb.WriteString(styledLine)
		sb.WriteString("\n")
	}

	// Scroll indicator
	if totalLines > diffAreaHeight {
		percent := 0
		maxOffset := totalLines - diffAreaHeight
		if maxOffset > 0 {
			percent = (r.scrollOffset * 100) / maxOffset
		}
		indicator := fmt.Sprintf("--- %d%% (%d/%d lines) ---", percent, r.scrollOffset+diffAreaHeight, totalLines)
		sb.WriteString(r.contextStyle.Render(indicator))
		sb.WriteString("\n")
	}

	sb.WriteString(strings.Repeat("-", min(r.width, 80)))
	sb.WriteString("\n\n")

	// Approval prompt
	prompt := r.promptStyle.Render("Approve these changes? [Y]es / [N]o")
	sb.WriteString(prompt)
	sb.WriteString("\n")
	sb.WriteString(r.contextStyle.Render("(Use j/k or arrows to scroll, Y to approve, N to reject)"))

	return sb.String()
}

// styleDiffLine applies syntax highlighting to a diff line.
func (r *ReviewGate) styleDiffLine(line string) string {
	if len(line) == 0 {
		return line
	}

	switch line[0] {
	case '+':
		// Don't highlight +++ header lines as additions
		if strings.HasPrefix(line, "+++") {
			return r.headerStyle.Render(line)
		}
		return r.addStyle.Render(line)
	case '-':
		// Don't highlight --- header lines as removals
		if strings.HasPrefix(line, "---") {
			return r.headerStyle.Render(line)
		}
		return r.removeStyle.Render(line)
	case '@':
		// Hunk headers
		return r.headerStyle.Render(line)
	case 'd':
		// diff --git header
		if strings.HasPrefix(line, "diff --git") {
			return r.headerStyle.Render(line)
		}
		return r.contextStyle.Render(line)
	case 'i':
		// index line
		if strings.HasPrefix(line, "index ") {
			return r.headerStyle.Render(line)
		}
		return r.contextStyle.Render(line)
	default:
		return r.contextStyle.Render(line)
	}
}

// IsActive returns true if a review is in progress.
func (r *ReviewGate) IsActive() bool {
	return r.active
}

// RequiresReview returns true if the tier requires human review.
func (r *ReviewGate) RequiresReview() bool {
	return r.tier == models.TierArchitect
}

// SetSize updates the viewport dimensions.
func (r *ReviewGate) SetSize(width, height int) {
	r.width = width
	r.height = height
}

// scrollUp moves the viewport up by one line.
func (r *ReviewGate) scrollUp() {
	if r.scrollOffset > 0 {
		r.scrollOffset--
	}
}

// scrollDown moves the viewport down by one line.
func (r *ReviewGate) scrollDown() {
	diffAreaHeight := r.height - 12
	if diffAreaHeight < 5 {
		diffAreaHeight = 5
	}
	maxOffset := max(0, len(r.diffLines)-diffAreaHeight)
	if r.scrollOffset < maxOffset {
		r.scrollOffset++
	}
}

// scrollPageUp moves the viewport up by one page.
func (r *ReviewGate) scrollPageUp() {
	pageSize := r.height - 12
	if pageSize < 5 {
		pageSize = 5
	}
	r.scrollOffset -= pageSize
	if r.scrollOffset < 0 {
		r.scrollOffset = 0
	}
}

// scrollPageDown moves the viewport down by one page.
func (r *ReviewGate) scrollPageDown() {
	pageSize := r.height - 12
	if pageSize < 5 {
		pageSize = 5
	}
	diffAreaHeight := pageSize
	maxOffset := max(0, len(r.diffLines)-diffAreaHeight)
	r.scrollOffset += pageSize
	if r.scrollOffset > maxOffset {
		r.scrollOffset = maxOffset
	}
}

// scrollToBottom moves the viewport to the end of the diff.
func (r *ReviewGate) scrollToBottom() {
	diffAreaHeight := r.height - 12
	if diffAreaHeight < 5 {
		diffAreaHeight = 5
	}
	r.scrollOffset = max(0, len(r.diffLines)-diffAreaHeight)
}

// reset clears the review state.
func (r *ReviewGate) reset() {
	r.agentID = ""
	r.taskID = ""
	r.diff = ""
	r.taskDescription = ""
	r.baseCommit = ""
	r.diffLines = make([]string, 0)
	r.scrollOffset = 0
}

// GetTaskID returns the ID of the task being reviewed.
func (r *ReviewGate) GetTaskID() string {
	return r.taskID
}
