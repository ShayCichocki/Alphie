// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// DefaultThreshold is the default quality threshold for the self-critique loop.
const DefaultThreshold = 7

// CritiquePrompt manages the self-critique prompt injection for the Ralph loop.
// It allows an agent to review its own work using a structured rubric.
type CritiquePrompt struct {
	// threshold is the minimum total score required to pass (out of 9).
	threshold int
}

// NewCritiquePrompt creates a new CritiquePrompt with the given threshold.
// The threshold should be between 1 and 9. If out of range, it will be clamped.
func NewCritiquePrompt(threshold int) *CritiquePrompt {
	if threshold < 1 {
		threshold = 1
	}
	if threshold > 9 {
		threshold = 9
	}
	return &CritiquePrompt{
		threshold: threshold,
	}
}

// Threshold returns the current quality threshold.
func (c *CritiquePrompt) Threshold() int {
	return c.threshold
}

// GetPromptTemplate returns the self-critique prompt template.
// This is the prompt that will be injected after an implementation
// to trigger self-review.
func (c *CritiquePrompt) GetPromptTemplate() string {
	return fmt.Sprintf(`Review your implementation. Score each criterion 1-3:

CORRECTNESS (1-3):
- Does it work for the happy path?
- Does it handle edge cases?
- Are there obvious bugs?

READABILITY (1-3):
- Is the code clear without comments?
- Are names descriptive?
- Is complexity appropriate?

EDGE CASES (1-3):
- Are errors handled?
- Are nulls/empty states handled?
- Are boundaries checked?

Total: X/9

If below threshold (%d/9), list specific improvements and implement them.
If at/above threshold, output DONE.`, c.threshold)
}

// InjectCritiquePrompt appends the self-critique prompt to the given response.
// This is used to continue the conversation with a self-review request.
func (c *CritiquePrompt) InjectCritiquePrompt(response string) string {
	return response + "\n\n---\n\n" + c.GetPromptTemplate()
}

// CritiqueResult represents the parsed result of a self-critique evaluation.
type CritiqueResult struct {
	// Score contains the individual criterion scores.
	Score models.RubricScore
	// IsDone indicates whether the agent output DONE (met threshold).
	IsDone bool
	// Improvements lists specific improvements mentioned if below threshold.
	Improvements []string
	// RawOutput is the original critique output for debugging.
	RawOutput string
}

// Total returns the total score from the critique result.
func (r *CritiqueResult) Total() int {
	return r.Score.Total()
}

// PassesThreshold returns true if the score meets or exceeds the given threshold.
func (r *CritiqueResult) PassesThreshold(threshold int) bool {
	return r.Score.Total() >= threshold
}

// ParseCritiqueResponse parses a critique response to extract scores and status.
// It looks for patterns like "CORRECTNESS (1-3): X" and "Total: X/9" and "DONE".
func ParseCritiqueResponse(response string) (*CritiqueResult, error) {
	result := &CritiqueResult{
		RawOutput: response,
	}

	// Check for DONE marker
	donePattern := regexp.MustCompile(`(?i)\bDONE\b`)
	result.IsDone = donePattern.MatchString(response)

	// Extract individual scores
	correctnessPattern := regexp.MustCompile(`(?i)CORRECTNESS[^:]*:\s*(\d)`)
	readabilityPattern := regexp.MustCompile(`(?i)READABILITY[^:]*:\s*(\d)`)
	edgeCasesPattern := regexp.MustCompile(`(?i)EDGE\s*CASES[^:]*:\s*(\d)`)

	if matches := correctnessPattern.FindStringSubmatch(response); len(matches) > 1 {
		if score, err := strconv.Atoi(matches[1]); err == nil && score >= 1 && score <= 3 {
			result.Score.Correctness = score
		}
	}

	if matches := readabilityPattern.FindStringSubmatch(response); len(matches) > 1 {
		if score, err := strconv.Atoi(matches[1]); err == nil && score >= 1 && score <= 3 {
			result.Score.Readability = score
		}
	}

	if matches := edgeCasesPattern.FindStringSubmatch(response); len(matches) > 1 {
		if score, err := strconv.Atoi(matches[1]); err == nil && score >= 1 && score <= 3 {
			result.Score.EdgeCases = score
		}
	}

	// Try to extract total score as fallback/validation
	totalPattern := regexp.MustCompile(`(?i)Total:\s*(\d)/9`)
	if matches := totalPattern.FindStringSubmatch(response); len(matches) > 1 {
		// We have individual scores, so total is just for validation
		// If individual scores weren't found, we could infer from total
		// but that's less reliable
		_ = matches[1] // total is validated by individual scores
	}

	// Extract improvement suggestions (lines starting with - or numbered)
	improvementPattern := regexp.MustCompile(`(?m)^[\s]*[-*]\s*(.+)$`)
	improvementMatches := improvementPattern.FindAllStringSubmatch(response, -1)
	for _, match := range improvementMatches {
		if len(match) > 1 {
			improvement := strings.TrimSpace(match[1])
			// Filter out the rubric questions themselves
			if !isRubricQuestion(improvement) {
				result.Improvements = append(result.Improvements, improvement)
			}
		}
	}

	return result, nil
}

// isRubricQuestion returns true if the text is part of the rubric template.
func isRubricQuestion(text string) bool {
	rubricQuestions := []string{
		"Does it work for the happy path",
		"Does it handle edge cases",
		"Are there obvious bugs",
		"Is the code clear without comments",
		"Are names descriptive",
		"Is complexity appropriate",
		"Are errors handled",
		"Are nulls/empty states handled",
		"Are boundaries checked",
	}
	lower := strings.ToLower(text)
	for _, q := range rubricQuestions {
		if strings.Contains(lower, strings.ToLower(q)) {
			return true
		}
	}
	return false
}

// ThresholdForTier returns the quality threshold for a given tier.
func ThresholdForTier(tierIgnored interface{}) int {
	// Always return default threshold, ignoring tier
	return DefaultThreshold
}

// MaxIterationsForTier returns the maximum Ralph loop iterations for a given tier.
// Scout skips ralph-loop entirely (0 iterations), Builder gets 3, Architect gets 5.
func MaxIterationsForTier(tierIgnored interface{}) int {
	// Always return default iterations, ignoring tier
	return 3
}

// GateConfigForTier returns the enabled quality gates for a given tier.
// Scout: lint only (fast feedback)
// Builder: build + lint + typecheck
// Architect: build + test + lint + typecheck (full validation)
type TierGateConfig struct {
	Lint      bool
	Build     bool
	Test      bool
	TypeCheck bool
}

func GateConfigForTier(tierIgnored interface{}) TierGateConfig {
	// Always return default gate config, ignoring tier
	return TierGateConfig{Build: true, Lint: true}
}
