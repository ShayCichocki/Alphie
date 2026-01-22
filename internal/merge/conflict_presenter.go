// Package merge provides conflict analysis and presentation.
package merge

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ShayCichocki/alphie/internal/git"
)

// ConflictPresenter analyzes merge conflicts and creates structured presentations for user resolution.
type ConflictPresenter struct {
	repoPath string
	git      git.Runner
}

// NewConflictPresenter creates a new ConflictPresenter.
func NewConflictPresenter(repoPath string, gitRunner git.Runner) *ConflictPresenter {
	return &ConflictPresenter{
		repoPath: repoPath,
		git:      gitRunner,
	}
}

// AnalyzeConflict analyzes a conflicting file and creates a presentation.
func (cp *ConflictPresenter) AnalyzeConflict(
	ctx context.Context,
	filePath string,
	sessionBranch, agentBranch string,
	taskID, agentID string,
	attemptNumber int,
) (*ConflictPresentation, error) {
	// Get merge base
	mergeBase, err := cp.git.MergeBase(sessionBranch, agentBranch)
	if err != nil {
		return nil, fmt.Errorf("get merge base: %w", err)
	}

	// Get file content from all three versions
	baseContent, err := cp.getFileContent(mergeBase, filePath)
	if err != nil {
		// File might not exist in base (new file in both branches)
		baseContent = ""
	}

	sessionContent, err := cp.getFileContent(sessionBranch, filePath)
	if err != nil {
		sessionContent = ""
	}

	agentContent, err := cp.getFileContent(agentBranch, filePath)
	if err != nil {
		agentContent = ""
	}

	// Parse conflict regions from the working tree (which has conflict markers)
	workingContent, err := cp.readWorkingFile(filePath)
	if err != nil {
		// If we can't read the working file, create a simple presentation
		// This can happen if the file was deleted in one branch
		return &ConflictPresentation{
			BaseContent:     baseContent,
			SessionContent:  sessionContent,
			AgentContent:    agentContent,
			ConflictRegions: []ConflictRegion{},
			FilePath:        filePath,
			TaskID:          taskID,
			AgentID:         agentID,
			SessionBranch:   sessionBranch,
			AgentBranch:     agentBranch,
			AttemptNumber:   attemptNumber,
		}, nil
	}

	// Parse conflict markers to identify regions
	regions := cp.parseConflictMarkers(workingContent)

	return &ConflictPresentation{
		BaseContent:     baseContent,
		SessionContent:  sessionContent,
		AgentContent:    agentContent,
		ConflictRegions: regions,
		FilePath:        filePath,
		TaskID:          taskID,
		AgentID:         agentID,
		SessionBranch:   sessionBranch,
		AgentBranch:     agentBranch,
		AttemptNumber:   attemptNumber,
	}, nil
}

// AnalyzeMultipleConflicts analyzes multiple conflicting files.
func (cp *ConflictPresenter) AnalyzeMultipleConflicts(
	ctx context.Context,
	filePaths []string,
	sessionBranch, agentBranch string,
	taskID, agentID string,
	attemptNumber int,
) ([]ConflictPresentation, error) {
	presentations := make([]ConflictPresentation, 0, len(filePaths))

	for _, filePath := range filePaths {
		presentation, err := cp.AnalyzeConflict(ctx, filePath, sessionBranch, agentBranch, taskID, agentID, attemptNumber)
		if err != nil {
			return nil, fmt.Errorf("analyze conflict for %s: %w", filePath, err)
		}
		presentations = append(presentations, *presentation)
	}

	return presentations, nil
}

// getFileContent retrieves file content from a specific git ref.
func (cp *ConflictPresenter) getFileContent(ref, filePath string) (string, error) {
	// Use git show to get file content at specific ref
	output, err := cp.git.ShowFile(ref, filePath)
	if err != nil {
		return "", err
	}
	return output, nil
}

// readWorkingFile reads file content from the working tree.
func (cp *ConflictPresenter) readWorkingFile(filePath string) (string, error) {
	fullPath := filepath.Join(cp.repoPath, filePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// parseConflictMarkers parses git conflict markers to identify conflict regions.
// Git conflict markers look like:
// <<<<<<< HEAD
// session content
// =======
// agent content
// >>>>>>> branch-name
func (cp *ConflictPresenter) parseConflictMarkers(content string) []ConflictRegion {
	regions := []ConflictRegion{}
	scanner := bufio.NewScanner(strings.NewReader(content))

	lineNum := 0
	inConflict := false
	var currentRegion ConflictRegion
	var sessionLines, agentLines, contextLines []string
	beforeConflictLines := []string{} // Track lines before conflict for context

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if strings.HasPrefix(line, "<<<<<<<") {
			// Start of conflict region
			inConflict = true
			currentRegion = ConflictRegion{
				StartLine: lineNum,
			}
			// Capture context (last 3 lines before conflict)
			contextStart := len(beforeConflictLines) - 3
			if contextStart < 0 {
				contextStart = 0
			}
			contextLines = beforeConflictLines[contextStart:]
			sessionLines = []string{}
			agentLines = []string{}
			continue
		}

		if strings.HasPrefix(line, "=======") && inConflict {
			// Switch from session to agent content
			continue
		}

		if strings.HasPrefix(line, ">>>>>>>") && inConflict {
			// End of conflict region
			currentRegion.EndLine = lineNum
			currentRegion.SessionContent = strings.Join(sessionLines, "\n")
			currentRegion.AgentContent = strings.Join(agentLines, "\n")
			currentRegion.Context = strings.Join(contextLines, "\n")
			regions = append(regions, currentRegion)

			inConflict = false
			sessionLines = []string{}
			agentLines = []string{}
			contextLines = []string{}
			beforeConflictLines = []string{}
			continue
		}

		if inConflict {
			// Inside conflict region
			if len(agentLines) > 0 || strings.HasPrefix(line, "=======") {
				// After the ======= marker, collecting agent content
				agentLines = append(agentLines, line)
			} else {
				// Before the ======= marker, collecting session content
				sessionLines = append(sessionLines, line)
			}
		} else {
			// Track lines before conflict for context
			beforeConflictLines = append(beforeConflictLines, line)
			// Keep only last 10 lines for memory efficiency
			if len(beforeConflictLines) > 10 {
				beforeConflictLines = beforeConflictLines[1:]
			}
		}
	}

	return regions
}

// FormatConflictSummary creates a human-readable summary of conflicts.
func FormatConflictSummary(presentations []ConflictPresentation) string {
	if len(presentations) == 0 {
		return "No conflicts to display"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Merge Conflicts Summary (Attempt #%d)\n", presentations[0].AttemptNumber))
	sb.WriteString(fmt.Sprintf("Task: %s | Agent: %s\n", presentations[0].TaskID, presentations[0].AgentID))
	sb.WriteString(fmt.Sprintf("Session: %s | Agent Branch: %s\n\n", presentations[0].SessionBranch, presentations[0].AgentBranch))

	sb.WriteString(fmt.Sprintf("Conflicting Files: %d\n", len(presentations)))
	for i, p := range presentations {
		sb.WriteString(fmt.Sprintf("  %d. %s (%d conflict regions)\n", i+1, p.FilePath, len(p.ConflictRegions)))
	}

	return sb.String()
}

// FormatConflictDiff creates a unified diff-style view of a conflict.
func FormatConflictDiff(presentation ConflictPresentation) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("=== Conflict in %s ===\n\n", presentation.FilePath))

	if len(presentation.ConflictRegions) == 0 {
		// No specific regions (e.g., file deleted in one branch)
		sb.WriteString("Base Version:\n")
		sb.WriteString(formatContent(presentation.BaseContent))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("Session (%s):\n", presentation.SessionBranch))
		sb.WriteString(formatContent(presentation.SessionContent))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("Agent (%s):\n", presentation.AgentBranch))
		sb.WriteString(formatContent(presentation.AgentContent))
	} else {
		// Show each conflict region
		for i, region := range presentation.ConflictRegions {
			sb.WriteString(fmt.Sprintf("Conflict Region %d (lines %d-%d):\n", i+1, region.StartLine, region.EndLine))
			if region.Context != "" {
				sb.WriteString("Context:\n")
				sb.WriteString(formatContent(region.Context))
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("<<<<<< Session (%s)\n", presentation.SessionBranch))
			sb.WriteString(formatContent(region.SessionContent))
			sb.WriteString("======\n")
			sb.WriteString(formatContent(region.AgentContent))
			sb.WriteString(fmt.Sprintf(">>>>>> Agent (%s)\n\n", presentation.AgentBranch))
		}
	}

	return sb.String()
}

// formatContent formats content with line numbers for display.
func formatContent(content string) string {
	if content == "" {
		return "  (empty)\n"
	}

	lines := strings.Split(content, "\n")
	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString(fmt.Sprintf("  %3d | %s\n", i+1, line))
	}
	return sb.String()
}
