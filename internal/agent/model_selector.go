// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"strings"

	"github.com/shayc/alphie/pkg/models"
)

// Model identifiers for different capability levels.
const (
	// ModelHaiku is the lightweight, fast model for simple tasks.
	ModelHaiku = "claude-3-5-haiku-20241022"
	// ModelSonnet is the balanced model for standard work.
	ModelSonnet = "claude-sonnet-4-20250514"
	// ModelOpus is the most capable model for complex tasks.
	ModelOpus = "claude-opus-4-5-20251101"
)

// Keywords that indicate a task should use haiku (simple tasks).
var haikuKeywords = []string{
	"simple",
	"boilerplate",
	"typo",
	"trivial",
	"formatting",
}

// Keywords that indicate a task should use opus (complex tasks).
var opusKeywords = []string{
	"architecture",
	"design",
	"refactor",
	"redesign",
	"complex",
}

// TierDefaultModels maps tiers to their default (primary) models.
var TierDefaultModels = map[models.Tier]string{
	models.TierScout:     ModelHaiku,
	models.TierBuilder:   ModelSonnet,
	models.TierArchitect: ModelOpus,
}

// SelectModel chooses the appropriate model for a task based on keywords
// and the agent tier. It examines the task title and description for
// keywords that indicate complexity level:
//   - Haiku keywords (simple, boilerplate, typo, trivial, formatting) -> haiku
//   - Opus keywords (architecture, design, refactor, redesign, complex) -> opus
//   - Otherwise -> tier's default model
func SelectModel(task *models.Task, tier models.Tier) string {
	if task == nil {
		return getTierDefault(tier)
	}

	// Combine title and description for keyword matching
	text := strings.ToLower(task.Title + " " + task.Description)

	// Check for haiku keywords (simple tasks)
	for _, kw := range haikuKeywords {
		if strings.Contains(text, kw) {
			return ModelHaiku
		}
	}

	// Check for opus keywords (complex tasks)
	for _, kw := range opusKeywords {
		if strings.Contains(text, kw) {
			return ModelOpus
		}
	}

	// Default to tier's primary model
	return getTierDefault(tier)
}

// getTierDefault returns the default model for a tier.
func getTierDefault(tier models.Tier) string {
	if model, ok := TierDefaultModels[tier]; ok {
		return model
	}
	// Fallback to sonnet if tier is unknown
	return ModelSonnet
}

// ContainsHaikuKeyword returns true if the text contains any haiku keyword.
// Exported for testing purposes.
func ContainsHaikuKeyword(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range haikuKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// ContainsOpusKeyword returns true if the text contains any opus keyword.
// Exported for testing purposes.
func ContainsOpusKeyword(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range opusKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
