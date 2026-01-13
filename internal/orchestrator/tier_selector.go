// Package orchestrator provides task decomposition and coordination.
package orchestrator

import (
	"strings"

	"github.com/shayc/alphie/pkg/models"
)

// scoutKeywords are words that indicate lightweight tasks suitable for Scout tier.
var scoutKeywords = []string{
	"docs",
	"readme",
	"documentation",
	"typo",
	"formatting",
	"comment",
	"single-file",
}

// architectKeywords are words that indicate complex tasks requiring Architect tier.
var architectKeywords = []string{
	"migration",
	"auth",
	"authentication",
	"security",
	"infra",
	"infrastructure",
	"schema",
	"database",
}

// TierSelector selects the appropriate tier based on task signals.
type TierSelector struct {
	scoutKeywords     []string
	architectKeywords []string
	protectedDetector *ProtectedAreaDetector
}

// NewTierSelector creates a new TierSelector with default keywords and
// an optional ProtectedAreaDetector for detecting protected areas.
func NewTierSelector(detector *ProtectedAreaDetector) *TierSelector {
	return &TierSelector{
		scoutKeywords:     append([]string{}, scoutKeywords...),
		architectKeywords: append([]string{}, architectKeywords...),
		protectedDetector: detector,
	}
}

// SelectTier analyzes a task description and returns the appropriate tier.
// It checks for:
//  1. Scout keywords (docs, typo, formatting, etc.) -> Scout tier
//  2. Architect keywords (migration, auth, infra, etc.) -> Architect tier
//  3. Protected area references in the description -> Architect tier
//  4. Default -> Builder tier
func (s *TierSelector) SelectTier(taskDescription string) models.Tier {
	lowerDesc := strings.ToLower(taskDescription)

	// Check for architect keywords first (higher priority than scout).
	for _, keyword := range s.architectKeywords {
		if strings.Contains(lowerDesc, strings.ToLower(keyword)) {
			return models.TierArchitect
		}
	}

	// Check for protected areas in the task description.
	// This looks for file paths or patterns that match protected areas.
	if s.protectedDetector != nil && s.containsProtectedReference(taskDescription) {
		return models.TierArchitect
	}

	// Check for scout keywords.
	for _, keyword := range s.scoutKeywords {
		if strings.Contains(lowerDesc, strings.ToLower(keyword)) {
			return models.TierScout
		}
	}

	// Default to Builder tier.
	return models.TierBuilder
}

// containsProtectedReference checks if the task description mentions
// any paths or patterns that would be flagged by the protected area detector.
func (s *TierSelector) containsProtectedReference(taskDescription string) bool {
	if s.protectedDetector == nil {
		return false
	}

	// Extract potential file paths from the description.
	// We look for common path patterns and check each one.
	words := strings.Fields(taskDescription)
	for _, word := range words {
		// Clean up trailing punctuation only (preserve leading dots for dotfiles).
		cleaned := strings.TrimRight(word, ",;:\"'`()[]{}!")

		// Check if this looks like a path (contains / or \).
		if strings.Contains(cleaned, "/") || strings.Contains(cleaned, "\\") {
			if s.protectedDetector.IsProtected(cleaned) {
				return true
			}
		}

		// Also check bare words that might be directory/file names.
		if s.protectedDetector.IsProtected(cleaned) {
			return true
		}
	}

	return false
}

// SelectTier is a convenience function that creates a TierSelector with
// a ProtectedAreaDetector and returns the selected tier for the given task.
func SelectTier(taskDescription string) models.Tier {
	detector := NewProtectedAreaDetector()
	selector := NewTierSelector(detector)
	return selector.SelectTier(taskDescription)
}
