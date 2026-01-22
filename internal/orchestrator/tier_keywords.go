// Package orchestrator provides task decomposition and coordination.
package orchestrator

import (
	"strings"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// TierKeywords is the single source of truth for tier classification keywords.
// These keywords are used by both the orchestrator (tier_selector.go) and
// the TUI (tier_classifier.go) to ensure consistent tier selection.
type TierKeywords struct {
	// Quick keywords indicate simple tasks that don't need decomposition.
	// These are single-agent, fast-execution tasks.
	Quick []string

	// Scout keywords indicate exploration, research, and lightweight tasks.
	// Scout agents search, explore, and gather information.
	Scout []string

	// Builder keywords are not matched - Builder is the default tier.
	// Any task that doesn't match Quick, Scout, or Architect goes to Builder.

	// Architect keywords indicate complex tasks requiring careful design.
	// These tasks touch protected areas, require migrations, or involve security.
	Architect []string
}

// DefaultTierKeywords returns the authoritative keyword mappings.
// These match the documentation in README.md.
var DefaultTierKeywords = TierKeywords{
	// Quick: Simple fixes that don't need multi-agent orchestration
	Quick: []string{
		"typo",
		"rename",
		"fix typo",
		"formatting",
		"comment",
	},

	// Scout: Exploration and research tasks
	Scout: []string{
		"find",
		"search",
		"list",
		"check",
		"where",
		"what",
		"show",
		"count",
		"look",
		"scan",
		"locate",
		"which",
		"docs",
		"readme",
		"documentation",
	},

	// Architect: Complex tasks requiring careful design
	Architect: []string{
		"refactor",
		"redesign",
		"architect",
		"migrate",
		"migration",
		"rewrite",
		"overhaul",
		"restructure",
		"reorganize",
		"rearchitect",
		"auth",
		"authentication",
		"security",
		"infra",
		"infrastructure",
		"schema",
		"database",
	},
}

// TierSelection represents a tier selection with confidence information.
type TierSelection struct {
	// Tier is the selected tier.
	Tier models.Tier
	// Confidence is how confident the selection is (0.0-1.0).
	// Low confidence suggests the task might be misclassified.
	Confidence float64
	// Reason explains why this tier was selected.
	Reason string
	// MatchedKeyword is the keyword that triggered this selection (if any).
	MatchedKeyword string
}

// ClassifyWithConfidence selects a tier based on keywords with confidence scoring.
// It returns the tier, confidence score, and matching details.
func ClassifyWithConfidence(taskText string) TierSelection {
	lower := strings.ToLower(taskText)

	// Architect takes highest priority
	for _, kw := range DefaultTierKeywords.Architect {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return TierSelection{
				Tier:           models.TierArchitect,
				Confidence:     0.85,
				Reason:         "matched architect keyword",
				MatchedKeyword: kw,
			}
		}
	}

	// Quick keywords for simple tasks
	for _, kw := range DefaultTierKeywords.Quick {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return TierSelection{
				Tier:           models.TierQuick,
				Confidence:     0.80,
				Reason:         "matched quick keyword",
				MatchedKeyword: kw,
			}
		}
	}

	// Scout keywords for exploration
	for _, kw := range DefaultTierKeywords.Scout {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return TierSelection{
				Tier:           models.TierScout,
				Confidence:     0.75,
				Reason:         "matched scout keyword",
				MatchedKeyword: kw,
			}
		}
	}

	// Default to Builder with lower confidence
	return TierSelection{
		Tier:       models.TierBuilder,
		Confidence: 0.60,
		Reason:     "no keyword match, defaulting to builder",
	}
}

// ClassifyTier returns just the tier for a task description.
// This is a convenience wrapper around ClassifyWithConfidence.
func ClassifyTier(taskText string) models.Tier {
	return ClassifyWithConfidence(taskText).Tier
}

// IsQuickKeyword returns true if the text contains a Quick tier keyword.
func IsQuickKeyword(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range DefaultTierKeywords.Quick {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// IsScoutKeyword returns true if the text contains a Scout tier keyword.
func IsScoutKeyword(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range DefaultTierKeywords.Scout {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// IsArchitectKeyword returns true if the text contains an Architect tier keyword.
func IsArchitectKeyword(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range DefaultTierKeywords.Architect {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}
