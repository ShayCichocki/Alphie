package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/shayc/alphie/internal/agent"
	"github.com/shayc/alphie/internal/architect"
	"github.com/spf13/cobra"
)

var auditJSON bool

var auditCmd = &cobra.Command{
	Use:   "audit <arch.md>",
	Short: "Audit codebase against architecture specification",
	Long: `Audit the current codebase against an architecture document.

This command parses an architecture specification (markdown file) and
compares it against the actual codebase to identify implementation gaps.

The audit process:
  1. Parses the architecture document to extract features/requirements
  2. Analyzes the codebase to determine implementation status of each feature
  3. Reports gaps (MISSING or PARTIAL implementations)

Output formats:
  - Human-readable (default): Formatted text report
  - JSON (--json flag): Machine-readable structured output

Examples:
  alphie audit docs/architecture.md           # Human-readable report
  alphie audit docs/architecture.md --json    # JSON output
  alphie audit spec.md | jq '.gaps'           # Filter JSON for gaps only`,
	Args: cobra.ExactArgs(1),
	RunE: runAudit,
}

func init() {
	auditCmd.Flags().BoolVar(&auditJSON, "json", false, "Output in JSON format")
}

func runAudit(cmd *cobra.Command, args []string) error {
	docPath := args[0]

	// Verify architecture document exists
	if _, err := os.Stat(docPath); os.IsNotExist(err) {
		return fmt.Errorf("architecture document not found: %s", docPath)
	}

	// Get current working directory as repo path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Create a context for the operation
	ctx := context.Background()

	// Create Claude process for parsing
	parserClaude := agent.NewClaudeProcess(ctx)

	// Parse the architecture document
	if !auditJSON {
		fmt.Println("Parsing architecture document...")
	}

	parser := architect.NewParser()
	spec, err := parser.Parse(ctx, docPath, parserClaude)
	if err != nil {
		return fmt.Errorf("parse architecture document: %w", err)
	}

	if !auditJSON {
		fmt.Printf("Found %d features/requirements\n", len(spec.Features))
	}

	// Create Claude process for auditing
	auditorClaude := agent.NewClaudeProcess(ctx)

	// Run the audit
	if !auditJSON {
		fmt.Println("Auditing codebase against specification...")
	}

	auditor := architect.NewAuditor()
	report, err := auditor.Audit(ctx, spec, repoPath, auditorClaude)
	if err != nil {
		return fmt.Errorf("audit codebase: %w", err)
	}

	// Output the report
	if auditJSON {
		return outputAuditJSON(report)
	}
	return outputAuditHumanReadable(report)
}

// outputAuditJSON outputs the gap report as JSON.
func outputAuditJSON(report *architect.GapReport) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// outputAuditHumanReadable outputs the gap report in human-readable format.
func outputAuditHumanReadable(report *architect.GapReport) error {
	fmt.Println()
	fmt.Println("=== Architecture Audit Report ===")
	fmt.Println()

	// Summary
	if report.Summary != "" {
		fmt.Println("Summary:")
		fmt.Printf("  %s\n", report.Summary)
		fmt.Println()
	}

	// Feature status overview
	completeCount := 0
	partialCount := 0
	missingCount := 0

	for _, fs := range report.Features {
		switch fs.Status {
		case architect.AuditStatusComplete:
			completeCount++
		case architect.AuditStatusPartial:
			partialCount++
		case architect.AuditStatusMissing:
			missingCount++
		}
	}

	fmt.Printf("Feature Status: %d complete, %d partial, %d missing\n",
		completeCount, partialCount, missingCount)
	fmt.Println()

	// Detailed feature status
	fmt.Println("--- Feature Details ---")
	for _, fs := range report.Features {
		statusIcon := auditStatusIcon(fs.Status)
		fmt.Printf("\n%s [%s] %s\n", statusIcon, fs.Feature.ID, fs.Feature.Name)

		if fs.Evidence != "" {
			fmt.Printf("   Evidence: %s\n", truncateAuditStr(fs.Evidence, 100))
		}
		if fs.Reasoning != "" {
			fmt.Printf("   Reasoning: %s\n", truncateAuditStr(fs.Reasoning, 150))
		}
	}

	// Gaps section
	if len(report.Gaps) > 0 {
		fmt.Println()
		fmt.Println("--- Gaps Requiring Action ---")
		for _, gap := range report.Gaps {
			statusIcon := auditStatusIcon(gap.Status)
			fmt.Printf("\n%s [%s] %s\n", statusIcon, gap.FeatureID, gap.Status)
			fmt.Printf("   Issue: %s\n", gap.Description)
			if gap.SuggestedAction != "" {
				fmt.Printf("   Suggested: %s\n", gap.SuggestedAction)
			}
		}
	} else {
		fmt.Println()
		fmt.Println("No gaps found - all features appear to be implemented!")
	}

	fmt.Println()
	return nil
}

// auditStatusIcon returns an icon representing the audit status.
func auditStatusIcon(status architect.AuditStatus) string {
	switch status {
	case architect.AuditStatusComplete:
		return "[OK]"
	case architect.AuditStatusPartial:
		return "[PARTIAL]"
	case architect.AuditStatusMissing:
		return "[MISSING]"
	default:
		return "[?]"
	}
}

// truncateAuditStr shortens a string to maxLen characters, adding ellipsis if needed.
func truncateAuditStr(s string, maxLen int) string {
	// Remove newlines for cleaner display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)

	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
