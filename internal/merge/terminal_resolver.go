// Package merge provides terminal-based interactive conflict resolution.
package merge

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// TerminalResolver implements HumanMergeResolver using simple terminal I/O.
// This is a basic implementation that can be enhanced with a full TUI later.
type TerminalResolver struct {
	reader *bufio.Reader
}

// NewTerminalResolver creates a new terminal-based conflict resolver.
func NewTerminalResolver() *TerminalResolver {
	return &TerminalResolver{
		reader: bufio.NewReader(os.Stdin),
	}
}

// PresentConflict presents a single conflict to the user and waits for resolution.
func (tr *TerminalResolver) PresentConflict(ctx context.Context, conflict ConflictPresentation) (Resolution, error) {
	return tr.PresentMultipleConflicts(ctx, []ConflictPresentation{conflict})
}

// PresentMultipleConflicts presents multiple conflicting files and waits for resolution.
func (tr *TerminalResolver) PresentMultipleConflicts(ctx context.Context, conflicts []ConflictPresentation) (Resolution, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return Resolution{}, ctx.Err()
	default:
	}

	// Clear screen and show header
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("MERGE CONFLICT - Human Resolution Required")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	// Show conflict summary
	fmt.Println(FormatConflictSummary(conflicts))
	fmt.Println()

	// Show detailed diffs for each file
	if len(conflicts) <= 3 {
		// Show full diffs for small number of files
		for i, conflict := range conflicts {
			fmt.Printf("\n--- File %d/%d ---\n", i+1, len(conflicts))
			fmt.Println(FormatConflictDiff(conflict))
		}
	} else {
		// Just show file list for many conflicts
		fmt.Println("Multiple files in conflict. Use file editor to review.")
		for i, conflict := range conflicts {
			fmt.Printf("  %d. %s (%d regions)\n", i+1, conflict.FilePath, len(conflict.ConflictRegions))
		}
	}

	// Present resolution options
	fmt.Println()
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("Resolution Options:")
	fmt.Println("  1. Accept Session - Keep current session branch version (discard agent changes)")
	fmt.Println("  2. Accept Agent   - Take agent's changes (discard session version)")
	fmt.Println("  3. Manual Merge   - Resolve conflicts manually in editor (NOT YET IMPLEMENTED)")
	fmt.Println("  4. Skip Agent     - Skip this agent's work and mark task as blocked")
	fmt.Println("  5. Abort Session  - Stop entire orchestration")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println()

	// Read user choice
	for {
		fmt.Print("Enter your choice (1-5): ")
		line, err := tr.reader.ReadString('\n')
		if err != nil {
			return Resolution{}, fmt.Errorf("failed to read input: %w", err)
		}

		line = strings.TrimSpace(line)
		choice, err := strconv.Atoi(line)
		if err != nil || choice < 1 || choice > 5 {
			fmt.Println("Invalid choice. Please enter a number between 1 and 5.")
			continue
		}

		// Confirm the choice
		var strategy ResolutionStrategy
		var strategyName string
		switch choice {
		case 1:
			strategy = AcceptSession
			strategyName = "Accept Session (keep current version)"
		case 2:
			strategy = AcceptAgent
			strategyName = "Accept Agent (take agent's changes)"
		case 3:
			strategy = ManualMerge
			strategyName = "Manual Merge"
			fmt.Println("Manual merge is not yet implemented. Please choose another option.")
			continue
		case 4:
			strategy = SkipAgent
			strategyName = "Skip Agent (mark task as blocked)"
		case 5:
			strategy = AbortSession
			strategyName = "Abort Session (stop orchestration)"
		}

		// Confirm
		fmt.Printf("\nYou chose: %s\n", strategyName)
		if choice == 5 {
			fmt.Println("WARNING: This will stop the entire session!")
		}
		fmt.Print("Confirm? (y/n): ")
		confirmLine, err := tr.reader.ReadString('\n')
		if err != nil {
			return Resolution{}, fmt.Errorf("failed to read confirmation: %w", err)
		}

		confirmLine = strings.TrimSpace(strings.ToLower(confirmLine))
		if confirmLine == "y" || confirmLine == "yes" {
			fmt.Println("\nApplying resolution...")
			return Resolution{
				Strategy:      strategy,
				SelectedFiles: make(map[string]string),
			}, nil
		}

		// User declined, ask again
		fmt.Println("Choice cancelled. Please select again.")
	}
}
