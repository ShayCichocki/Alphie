package tui

import (
	"strings"

	"github.com/ShayCichocki/alphie/internal/orchestrator"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// ClassifyTier determines the appropriate tier for a task based on its text.
// It checks for explicit prefixes (!quick, !scout, !builder, !architect) first,
// then falls back to keyword matching using the shared tier keywords.
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

	// Use shared tier keywords for classification
	tier := orchestrator.ClassifyTier(text)
	return tier, text
}
