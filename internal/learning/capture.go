// Package learning provides learning capture capabilities from agent failures.
package learning

import (
	"regexp"
	"strings"
)

// SuggestedLearning represents a potential learning extracted from failure output.
// It needs user confirmation before being stored.
type SuggestedLearning struct {
	// CAO contains the extracted WHEN-DO-RESULT triple.
	CAO *CAOTriple
	// Confidence indicates how confident we are in this extraction (0-1).
	Confidence float64
	// Source describes where this learning was extracted from.
	Source string
	// RawContext contains the original error/output context.
	RawContext string
}

// FailureAnalyzer analyzes agent output to extract potential learnings.
type FailureAnalyzer struct {
	// patterns are compiled regex patterns for common failure types
	patterns []*failurePattern
}

// failurePattern represents a pattern for extracting learnings from errors.
type failurePattern struct {
	name       string
	pattern    *regexp.Regexp
	extractor  func(match []string, context string) *SuggestedLearning
	confidence float64
}

// NewFailureAnalyzer creates a new FailureAnalyzer with default patterns.
func NewFailureAnalyzer() *FailureAnalyzer {
	fa := &FailureAnalyzer{}
	fa.initPatterns()
	return fa
}

// initPatterns initializes the failure patterns for extraction.
func (fa *FailureAnalyzer) initPatterns() {
	fa.patterns = []*failurePattern{
		// Go compilation errors
		{
			name:       "go_undefined",
			pattern:    regexp.MustCompile(`undefined:\s+(\w+)`),
			confidence: 0.7,
			extractor: func(match []string, context string) *SuggestedLearning {
				if len(match) < 2 {
					return nil
				}
				return &SuggestedLearning{
					CAO: &CAOTriple{
						Condition: "Go compilation fails with 'undefined: " + match[1] + "'",
						Action:    "Check imports and ensure " + match[1] + " is defined or imported correctly",
						Outcome:   "Compilation succeeds",
					},
					Confidence: 0.7,
					Source:     "go_undefined",
					RawContext: context,
				}
			},
		},
		// Go type errors
		{
			name:       "go_type_mismatch",
			pattern:    regexp.MustCompile(`cannot use (.+?) \(type (.+?)\) as type (.+?) in`),
			confidence: 0.6,
			extractor: func(match []string, context string) *SuggestedLearning {
				if len(match) < 4 {
					return nil
				}
				return &SuggestedLearning{
					CAO: &CAOTriple{
						Condition: "Go type error: cannot use type " + match[2] + " as " + match[3],
						Action:    "Convert the value or use the correct type",
						Outcome:   "Type check passes",
					},
					Confidence: 0.6,
					Source:     "go_type_mismatch",
					RawContext: context,
				}
			},
		},
		// Go import errors
		{
			name:       "go_import_cycle",
			pattern:    regexp.MustCompile(`import cycle not allowed`),
			confidence: 0.8,
			extractor: func(match []string, context string) *SuggestedLearning {
				return &SuggestedLearning{
					CAO: &CAOTriple{
						Condition: "Go import cycle detected",
						Action:    "Restructure packages to break the import cycle, possibly by introducing an interface package",
						Outcome:   "Imports resolve without cycles",
					},
					Confidence: 0.8,
					Source:     "go_import_cycle",
					RawContext: context,
				}
			},
		},
		// Test failures
		{
			name:       "go_test_fail",
			pattern:    regexp.MustCompile(`--- FAIL: (\w+)`),
			confidence: 0.5,
			extractor: func(match []string, context string) *SuggestedLearning {
				if len(match) < 2 {
					return nil
				}
				return &SuggestedLearning{
					CAO: &CAOTriple{
						Condition: "Test " + match[1] + " fails",
						Action:    "Review test expectations and implementation",
						Outcome:   "Test passes",
					},
					Confidence: 0.5,
					Source:     "go_test_fail",
					RawContext: context,
				}
			},
		},
		// Permission errors
		{
			name:       "permission_denied",
			pattern:    regexp.MustCompile(`permission denied`),
			confidence: 0.7,
			extractor: func(match []string, context string) *SuggestedLearning {
				return &SuggestedLearning{
					CAO: &CAOTriple{
						Condition: "Operation fails with permission denied",
						Action:    "Check file permissions or run with appropriate privileges",
						Outcome:   "Operation succeeds",
					},
					Confidence: 0.7,
					Source:     "permission_denied",
					RawContext: context,
				}
			},
		},
		// File not found
		{
			name:       "file_not_found",
			pattern:    regexp.MustCompile(`no such file or directory:\s*(.+)`),
			confidence: 0.7,
			extractor: func(match []string, context string) *SuggestedLearning {
				if len(match) < 2 {
					return nil
				}
				path := strings.TrimSpace(match[1])
				return &SuggestedLearning{
					CAO: &CAOTriple{
						Condition: "File or directory not found: " + path,
						Action:    "Verify the path exists or create the required file/directory",
						Outcome:   "File access succeeds",
					},
					Confidence: 0.7,
					Source:     "file_not_found",
					RawContext: context,
				}
			},
		},
		// Git conflicts
		{
			name:       "git_conflict",
			pattern:    regexp.MustCompile(`CONFLICT \(content\):\s*Merge conflict in (.+)`),
			confidence: 0.8,
			extractor: func(match []string, context string) *SuggestedLearning {
				if len(match) < 2 {
					return nil
				}
				return &SuggestedLearning{
					CAO: &CAOTriple{
						Condition: "Git merge conflict in " + strings.TrimSpace(match[1]),
						Action:    "Resolve conflicts manually by editing the file and choosing correct changes",
						Outcome:   "Merge completes successfully",
					},
					Confidence: 0.8,
					Source:     "git_conflict",
					RawContext: context,
				}
			},
		},
		// Timeout errors
		{
			name:       "timeout",
			pattern:    regexp.MustCompile(`(context deadline exceeded|timeout|timed out)`),
			confidence: 0.6,
			extractor: func(match []string, context string) *SuggestedLearning {
				return &SuggestedLearning{
					CAO: &CAOTriple{
						Condition: "Operation times out",
						Action:    "Increase timeout duration or optimize the slow operation",
						Outcome:   "Operation completes within time limit",
					},
					Confidence: 0.6,
					Source:     "timeout",
					RawContext: context,
				}
			},
		},
	}
}

// AnalyzeFailure analyzes agent failure output and extracts suggested learnings.
// It returns a slice of SuggestedLearning that need user confirmation.
func (fa *FailureAnalyzer) AnalyzeFailure(output string, errorMsg string) []*SuggestedLearning {
	var suggestions []*SuggestedLearning

	// Combine output and error for analysis
	combined := output + "\n" + errorMsg

	// Try each pattern
	for _, p := range fa.patterns {
		matches := p.pattern.FindStringSubmatch(combined)
		if matches != nil {
			if suggestion := p.extractor(matches, combined); suggestion != nil {
				suggestions = append(suggestions, suggestion)
			}
		}
	}

	return suggestions
}

// FormatForConfirmation formats a suggested learning for user confirmation.
func FormatForConfirmation(sl *SuggestedLearning) string {
	if sl == nil || sl.CAO == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Suggested Learning (")
	sb.WriteString(sl.Source)
	sb.WriteString("):\n")
	sb.WriteString("  WHEN: ")
	sb.WriteString(sl.CAO.Condition)
	sb.WriteString("\n  DO: ")
	sb.WriteString(sl.CAO.Action)
	sb.WriteString("\n  RESULT: ")
	sb.WriteString(sl.CAO.Outcome)
	sb.WriteString("\n")

	return sb.String()
}

// CaptureResult represents the result of capturing learnings from a failure.
type CaptureResult struct {
	// Suggestions are the extracted learning suggestions.
	Suggestions []*SuggestedLearning
	// ExistingLearnings are learnings that already exist for this error pattern.
	ExistingLearnings []*Learning
}

// Capturer combines failure analysis with the learning system to suggest
// and store learnings from failures.
type Capturer struct {
	analyzer *FailureAnalyzer
	system   *LearningSystem
}

// NewCapturer creates a new Capturer with the given learning system.
func NewCapturer(system *LearningSystem) *Capturer {
	return &Capturer{
		analyzer: NewFailureAnalyzer(),
		system:   system,
	}
}

// CaptureFromFailure analyzes a failure and returns suggestions for new learnings.
// It also checks for existing learnings that might be relevant.
func (c *Capturer) CaptureFromFailure(output string, errorMsg string) (*CaptureResult, error) {
	result := &CaptureResult{}

	// Get suggestions from failure analysis
	result.Suggestions = c.analyzer.AnalyzeFailure(output, errorMsg)

	// Check for existing learnings if we have a system
	if c.system != nil && errorMsg != "" {
		existing, err := c.system.OnFailure(errorMsg)
		if err != nil {
			return nil, err
		}
		result.ExistingLearnings = existing
	}

	return result, nil
}

// ConfirmAndStore stores a confirmed learning suggestion.
// This should only be called after user confirmation.
func (c *Capturer) ConfirmAndStore(sl *SuggestedLearning, conceptNames []string) (*Learning, error) {
	if c.system == nil {
		return nil, nil
	}

	if sl == nil || sl.CAO == nil {
		return nil, nil
	}

	// Set outcome type to failure since this came from a failure
	learning, err := c.system.AddLearning(sl.CAO, conceptNames)
	if err != nil {
		return nil, err
	}

	// Update outcome type to failure
	learning.OutcomeType = "failure"
	if err := c.system.Store().Update(learning); err != nil {
		return nil, err
	}

	return learning, nil
}
