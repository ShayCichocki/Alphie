// Package architect provides tools for analyzing and auditing codebases against specifications.
package architect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shayc/alphie/internal/agent"
)

// AuditStatus represents the implementation status of a feature.
type AuditStatus string

const (
	// AuditStatusComplete indicates the feature is fully implemented.
	AuditStatusComplete AuditStatus = "COMPLETE"
	// AuditStatusPartial indicates the feature is partially implemented.
	AuditStatusPartial AuditStatus = "PARTIAL"
	// AuditStatusMissing indicates the feature is not implemented.
	AuditStatusMissing AuditStatus = "MISSING"
)

// Feature represents a feature from the architecture specification.
type Feature struct {
	// ID is the unique identifier for this feature.
	ID string `json:"id"`
	// Name is the short name of the feature.
	Name string `json:"name"`
	// Description provides detailed information about the feature.
	Description string `json:"description"`
	// Criteria defines what constitutes full implementation.
	Criteria string `json:"criteria,omitempty"`
}

// FeatureStatus represents the status of a single feature after audit.
type FeatureStatus struct {
	// Feature is the feature being assessed.
	Feature Feature `json:"feature"`
	// Status is the implementation status.
	Status AuditStatus `json:"status"`
	// Evidence contains file references and code snippets supporting the assessment.
	Evidence string `json:"evidence"`
	// Reasoning explains the rationale for the status determination.
	Reasoning string `json:"reasoning"`
}

// Gap represents a feature that needs work.
type Gap struct {
	// FeatureID is the ID of the feature with the gap.
	FeatureID string `json:"feature_id"`
	// Status is the current implementation status (PARTIAL or MISSING).
	Status AuditStatus `json:"status"`
	// Description describes what is missing or incomplete.
	Description string `json:"description"`
	// SuggestedAction provides guidance on how to address the gap.
	SuggestedAction string `json:"suggested_action"`
}

// GapReport contains the full audit results.
type GapReport struct {
	// Features lists the status of each audited feature.
	Features []FeatureStatus `json:"features"`
	// Gaps lists features that need work.
	Gaps []Gap `json:"gaps"`
	// Summary provides an overall assessment.
	Summary string `json:"summary"`
}

// ArchSpec represents an architecture specification.
type ArchSpec struct {
	// Name is the name of the specification.
	Name string `json:"name"`
	// Features lists all features in the specification.
	Features []Feature `json:"features"`
}

// Auditor compares architecture specifications against actual code.
type Auditor struct {
	// maxFilesToScan limits the number of files sent to Claude for context.
	maxFilesToScan int
}

// NewAuditor creates a new Auditor instance.
func NewAuditor() *Auditor {
	return &Auditor{
		maxFilesToScan: 50,
	}
}

// Audit compares parsed features against the codebase and returns a gap report.
// It uses Claude to analyze each feature's implementation status.
func (a *Auditor) Audit(ctx context.Context, spec *ArchSpec, repoPath string, claude *agent.ClaudeProcess) (*GapReport, error) {
	if spec == nil || len(spec.Features) == 0 {
		return &GapReport{
			Features: []FeatureStatus{},
			Gaps:     []Gap{},
			Summary:  "No features to audit",
		}, nil
	}

	// Gather context from the codebase
	codeContext, err := a.gatherCodeContext(repoPath)
	if err != nil {
		return nil, fmt.Errorf("gather code context: %w", err)
	}

	// Build the audit prompt
	prompt := a.buildAuditPrompt(spec, codeContext)

	// Start Claude process
	if err := claude.Start(prompt, repoPath); err != nil {
		return nil, fmt.Errorf("start claude process: %w", err)
	}

	// Collect output
	var outputBuilder strings.Builder
	for event := range claude.Output() {
		switch event.Type {
		case agent.StreamEventAssistant, agent.StreamEventResult:
			if event.Message != "" {
				outputBuilder.WriteString(event.Message)
			}
		case agent.StreamEventError:
			if event.Error != "" {
				return nil, fmt.Errorf("claude error: %s", event.Error)
			}
		}
	}

	// Wait for process completion
	if err := claude.Wait(); err != nil {
		return nil, fmt.Errorf("claude process failed: %w", err)
	}

	// Parse the response
	report, err := a.parseAuditResponse(outputBuilder.String(), spec.Features)
	if err != nil {
		return nil, fmt.Errorf("parse audit response: %w", err)
	}

	return report, nil
}

// gatherCodeContext scans the repository and gathers relevant file information.
func (a *Auditor) gatherCodeContext(repoPath string) (string, error) {
	var sb strings.Builder
	sb.WriteString("## Repository Structure\n\n")

	fileCount := 0
	err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories and common non-code directories
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files and non-code files
		if strings.HasPrefix(name, ".") {
			return nil
		}

		// Only include code files
		ext := filepath.Ext(name)
		if !isCodeFile(ext) {
			return nil
		}

		if fileCount >= a.maxFilesToScan {
			return filepath.SkipAll
		}

		relPath, _ := filepath.Rel(repoPath, path)
		sb.WriteString(fmt.Sprintf("- %s\n", relPath))
		fileCount++

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return "", err
	}

	return sb.String(), nil
}

// isCodeFile returns true if the extension indicates a code file.
func isCodeFile(ext string) bool {
	codeExts := map[string]bool{
		".go":   true,
		".js":   true,
		".ts":   true,
		".py":   true,
		".java": true,
		".rs":   true,
		".c":    true,
		".cpp":  true,
		".h":    true,
		".hpp":  true,
		".rb":   true,
		".php":  true,
		".cs":   true,
		".kt":   true,
		".swift": true,
		".scala": true,
	}
	return codeExts[ext]
}

// buildAuditPrompt constructs the prompt for Claude to audit features.
func (a *Auditor) buildAuditPrompt(spec *ArchSpec, codeContext string) string {
	var sb strings.Builder

	sb.WriteString("You are auditing a codebase against an architecture specification.\n\n")
	sb.WriteString("Your task is to analyze each feature and determine its implementation status.\n\n")

	sb.WriteString("## Specification: ")
	sb.WriteString(spec.Name)
	sb.WriteString("\n\n")

	sb.WriteString("## Features to Audit\n\n")
	for i, f := range spec.Features {
		sb.WriteString(fmt.Sprintf("### Feature %d: %s (ID: %s)\n", i+1, f.Name, f.ID))
		sb.WriteString(fmt.Sprintf("Description: %s\n", f.Description))
		if f.Criteria != "" {
			sb.WriteString(fmt.Sprintf("Criteria: %s\n", f.Criteria))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(codeContext)
	sb.WriteString("\n")

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("For each feature, examine the codebase and determine:\n")
	sb.WriteString("- Status: COMPLETE (fully implemented), PARTIAL (partially implemented), or MISSING (not implemented)\n")
	sb.WriteString("- Evidence: File references and code snippets supporting your assessment\n")
	sb.WriteString("- Reasoning: Why you reached this conclusion\n\n")

	sb.WriteString("Respond with valid JSON in this exact format:\n")
	sb.WriteString("```json\n")
	sb.WriteString(`{
  "features": [
    {
      "feature_id": "string",
      "status": "COMPLETE|PARTIAL|MISSING",
      "evidence": "string",
      "reasoning": "string"
    }
  ],
  "gaps": [
    {
      "feature_id": "string",
      "status": "PARTIAL|MISSING",
      "description": "string",
      "suggested_action": "string"
    }
  ],
  "summary": "string"
}
`)
	sb.WriteString("```\n")

	return sb.String()
}

// parseAuditResponse parses Claude's JSON response into a GapReport.
func (a *Auditor) parseAuditResponse(response string, features []Feature) (*GapReport, error) {
	// Extract JSON from response (it may be wrapped in markdown code blocks)
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	var rawReport struct {
		Features []struct {
			FeatureID string `json:"feature_id"`
			Status    string `json:"status"`
			Evidence  string `json:"evidence"`
			Reasoning string `json:"reasoning"`
		} `json:"features"`
		Gaps []struct {
			FeatureID       string `json:"feature_id"`
			Status          string `json:"status"`
			Description     string `json:"description"`
			SuggestedAction string `json:"suggested_action"`
		} `json:"gaps"`
		Summary string `json:"summary"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &rawReport); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	// Build feature map for lookup
	featureMap := make(map[string]Feature)
	for _, f := range features {
		featureMap[f.ID] = f
	}

	// Convert to GapReport
	report := &GapReport{
		Features: make([]FeatureStatus, 0, len(rawReport.Features)),
		Gaps:     make([]Gap, 0, len(rawReport.Gaps)),
		Summary:  rawReport.Summary,
	}

	for _, rf := range rawReport.Features {
		feature, ok := featureMap[rf.FeatureID]
		if !ok {
			continue // Skip unknown features
		}

		status := parseAuditStatus(rf.Status)
		report.Features = append(report.Features, FeatureStatus{
			Feature:   feature,
			Status:    status,
			Evidence:  rf.Evidence,
			Reasoning: rf.Reasoning,
		})
	}

	for _, rg := range rawReport.Gaps {
		status := parseAuditStatus(rg.Status)
		report.Gaps = append(report.Gaps, Gap{
			FeatureID:       rg.FeatureID,
			Status:          status,
			Description:     rg.Description,
			SuggestedAction: rg.SuggestedAction,
		})
	}

	return report, nil
}

// extractJSON extracts JSON content from a response that may include markdown.
func extractJSON(response string) string {
	// Try to find JSON in code blocks first
	start := strings.Index(response, "```json")
	if start != -1 {
		start += 7 // Skip "```json"
		end := strings.Index(response[start:], "```")
		if end != -1 {
			return strings.TrimSpace(response[start : start+end])
		}
	}

	// Try plain code block
	start = strings.Index(response, "```")
	if start != -1 {
		start += 3
		end := strings.Index(response[start:], "```")
		if end != -1 {
			return strings.TrimSpace(response[start : start+end])
		}
	}

	// Try to find raw JSON (starts with {)
	start = strings.Index(response, "{")
	if start != -1 {
		// Find matching closing brace
		depth := 0
		for i := start; i < len(response); i++ {
			switch response[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return response[start : i+1]
				}
			}
		}
	}

	return ""
}

// parseAuditStatus converts a string to AuditStatus.
func parseAuditStatus(s string) AuditStatus {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "COMPLETE":
		return AuditStatusComplete
	case "PARTIAL":
		return AuditStatusPartial
	case "MISSING":
		return AuditStatusMissing
	default:
		return AuditStatusMissing
	}
}
