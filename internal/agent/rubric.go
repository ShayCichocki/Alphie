package agent

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/shayc/alphie/pkg/models"
)

// Common errors for rubric parsing.
var (
	// ErrMalformedResponse indicates the critique response could not be parsed.
	ErrMalformedResponse = errors.New("malformed critique response")
	// ErrScoreOutOfRange indicates a score was outside the valid 1-3 range.
	ErrScoreOutOfRange = errors.New("score out of range (must be 1-3)")
	// ErrMissingScore indicates a required score category was not found.
	ErrMissingScore = errors.New("missing required score")
)

// Regular expressions for parsing critique responses.
var (
	// totalPattern matches "Total: X/9" format.
	totalPattern = regexp.MustCompile(`(?i)Total:\s*(\d+)/9`)
	// correctnessPattern matches "CORRECTNESS: X" or "Correctness: X/3".
	correctnessPattern = regexp.MustCompile(`(?i)CORRECTNESS[:\s]+(\d+)(?:/3)?`)
	// readabilityPattern matches "READABILITY: X" or "Readability: X/3".
	readabilityPattern = regexp.MustCompile(`(?i)READABILITY[:\s]+(\d+)(?:/3)?`)
	// edgeCasesPattern matches "EDGE CASES: X", "EDGE_CASES: X", or "EdgeCases: X/3".
	edgeCasesPattern = regexp.MustCompile(`(?i)EDGE[\s_]?CASES[:\s]+(\d+)(?:/3)?`)
)

// ParseScore extracts a RubricScore from a critique response string.
// It looks for:
//   - Individual scores: CORRECTNESS, READABILITY, EDGE CASES (each 1-3)
//   - Total score: "Total: X/9"
//
// Returns an error if:
//   - The response format is malformed
//   - Any score is outside the valid 1-3 range
//   - Required scores are missing
func ParseScore(response string) (*models.RubricScore, error) {
	if strings.TrimSpace(response) == "" {
		return nil, ErrMalformedResponse
	}

	score := &models.RubricScore{}
	var foundCount int

	// Try to extract correctness score
	if matches := correctnessPattern.FindStringSubmatch(response); len(matches) >= 2 {
		val, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, ErrMalformedResponse
		}
		if val < 1 || val > 3 {
			return nil, ErrScoreOutOfRange
		}
		score.Correctness = val
		foundCount++
	}

	// Try to extract readability score
	if matches := readabilityPattern.FindStringSubmatch(response); len(matches) >= 2 {
		val, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, ErrMalformedResponse
		}
		if val < 1 || val > 3 {
			return nil, ErrScoreOutOfRange
		}
		score.Readability = val
		foundCount++
	}

	// Try to extract edge cases score
	if matches := edgeCasesPattern.FindStringSubmatch(response); len(matches) >= 2 {
		val, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, ErrMalformedResponse
		}
		if val < 1 || val > 3 {
			return nil, ErrScoreOutOfRange
		}
		score.EdgeCases = val
		foundCount++
	}

	// If we found all individual scores, return
	if foundCount == 3 {
		return score, nil
	}

	// Try to extract from total if individual scores are missing
	if matches := totalPattern.FindStringSubmatch(response); len(matches) >= 2 {
		total, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, ErrMalformedResponse
		}
		if total < 3 || total > 9 {
			return nil, ErrScoreOutOfRange
		}
		// If we have partial individual scores, validate against total
		if foundCount > 0 {
			currentTotal := score.Correctness + score.Readability + score.EdgeCases
			if currentTotal != total {
				// Scores don't match total, this is inconsistent
				return nil, ErrMalformedResponse
			}
		}
		// If we have all individual scores and they match total, we're done
		if foundCount == 3 {
			return score, nil
		}
	}

	// If we didn't find all individual scores, it's a missing score error
	if foundCount < 3 {
		return nil, ErrMissingScore
	}

	return score, nil
}

// MeetsThreshold returns true if the given RubricScore's total meets or exceeds
// the specified threshold.
func MeetsThreshold(score *models.RubricScore, threshold int) bool {
	if score == nil {
		return false
	}
	return score.Total() >= threshold
}
