// Package orchestrator provides task decomposition and coordination.
package orchestrator

import (
	"strings"

	"github.com/ShayCichocki/alphie/internal/protect"
)

// TierSelector selects the appropriate tier based on task signals.
type TierSelector struct {
	protectedDetector *protect.Detector
}

// NewTierSelector creates a new TierSelector with an optional
// ProtectedAreaDetector for detecting protected areas.
func NewTierSelector(detector *protect.Detector) *TierSelector {
	return &TierSelector{
		protectedDetector: detector,
	}
}

// SelectTier analyzes a task description and returns the appropriate tier.
// It uses the shared tier keywords from tier_keywords.go and checks for:
//  1. Architect keywords (migration, auth, infra, etc.) -> Architect tier
//  2. Protected area references in the description -> Architect tier
//  3. Quick keywords (typo, rename) -> Quick tier
//  4. Scout keywords (find, search, docs) -> Scout tier
//  5. Default -> Builder tier
func (s *TierSelector) SelectTier(taskDescription string) interface{} {
	// Check for architect keywords first (highest priority)
	if IsArchitectKeyword(taskDescription) {
		return nil
	}

	// Check for protected areas in the task description.
	if s.protectedDetector != nil && s.containsProtectedReference(taskDescription) {
		return nil
	}

	// Use the shared classification for remaining tiers
	return ClassifyTier(taskDescription)
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
//
// Priority order (using shared keywords from tier_keywords.go):
// 1. Architect keywords → TierArchitect
// 2. Protected area references → TierArchitect
// 3. Quick keywords → TierQuick
// 4. Scout keywords → TierScout
// 5. Default → TierBuilder
func SelectTier(taskDescription string) interface{} {
	detector := protect.New()
	selector := NewTierSelector(detector)
	return selector.SelectTier(taskDescription)
}
