package tui

import (
	"strings"

	"github.com/shayc/alphie/pkg/models"
)

var (
	scoutKeywords = []string{
		"find", "search", "list", "check", "where", "what",
		"show", "count", "look", "scan", "locate", "which",
	}
	architectKeywords = []string{
		"refactor", "redesign", "architect", "migrate", "rewrite",
		"overhaul", "restructure", "reorganize", "rearchitect",
	}
)

// ClassifyTier determines the appropriate tier for a task based on its text.
// It checks for explicit prefixes (!quick, !scout, !builder, !architect) first,
// then falls back to keyword matching.
// Returns the tier and the task text with any prefix stripped.
func ClassifyTier(taskText string) (models.Tier, string) {
	text := strings.TrimSpace(taskText)

	// Check for explicit prefix overrides
	if strings.HasPrefix(text, "!quick ") {
		return models.TierQuick, strings.TrimPrefix(text, "!quick ")
	}
	if strings.HasPrefix(text, "!scout ") {
		return models.TierScout, strings.TrimPrefix(text, "!scout ")
	}
	if strings.HasPrefix(text, "!builder ") {
		return models.TierBuilder, strings.TrimPrefix(text, "!builder ")
	}
	if strings.HasPrefix(text, "!architect ") {
		return models.TierArchitect, strings.TrimPrefix(text, "!architect ")
	}

	lower := strings.ToLower(text)

	// Check for architect keywords first (more specific)
	for _, kw := range architectKeywords {
		if strings.Contains(lower, kw) {
			return models.TierArchitect, text
		}
	}

	// Check for scout keywords
	for _, kw := range scoutKeywords {
		if strings.Contains(lower, kw) {
			return models.TierScout, text
		}
	}

	// Default to builder
	return models.TierBuilder, text
}
