package agent

import (
	"testing"

	"github.com/ShayCichocki/alphie/pkg/models"
)

func TestSelectModel(t *testing.T) {
	tests := []struct {
		name     string
		task     *models.Task
		tier     models.Tier
		expected string
	}{
		// Nil task falls back to tier default
		{
			name:     "nil task with scout tier",
			task:     nil,
			tier:     models.TierScout,
			expected: ModelHaiku,
		},
		{
			name:     "nil task with builder tier",
			task:     nil,
			tier:     models.TierBuilder,
			expected: ModelSonnet,
		},
		{
			name:     "nil task with architect tier",
			task:     nil,
			tier:     models.TierArchitect,
			expected: ModelOpus,
		},

		// Haiku keywords
		{
			name: "simple keyword in title",
			task: &models.Task{
				Title:       "Simple fix for button color",
				Description: "Change the button color to blue",
			},
			tier:     models.TierBuilder,
			expected: ModelHaiku,
		},
		{
			name: "boilerplate keyword in title",
			task: &models.Task{
				Title:       "Add boilerplate for new service",
				Description: "Create the basic structure",
			},
			tier:     models.TierBuilder,
			expected: ModelHaiku,
		},
		{
			name: "typo keyword in description",
			task: &models.Task{
				Title:       "Fix naming issue",
				Description: "There is a typo in the variable name",
			},
			tier:     models.TierBuilder,
			expected: ModelHaiku,
		},
		{
			name: "trivial keyword",
			task: &models.Task{
				Title:       "Trivial change to README",
				Description: "Update version number",
			},
			tier:     models.TierArchitect,
			expected: ModelHaiku,
		},
		{
			name: "formatting keyword",
			task: &models.Task{
				Title:       "Fix formatting issues",
				Description: "Run gofmt on all files",
			},
			tier:     models.TierBuilder,
			expected: ModelHaiku,
		},

		// Opus keywords
		{
			name: "architecture keyword in title",
			task: &models.Task{
				Title:       "Define architecture for new system",
				Description: "Create the overall design",
			},
			tier:     models.TierScout,
			expected: ModelOpus,
		},
		{
			name: "design keyword in title",
			task: &models.Task{
				Title:       "Design the API structure",
				Description: "Plan endpoints and data models",
			},
			tier:     models.TierBuilder,
			expected: ModelOpus,
		},
		{
			name: "refactor keyword in description",
			task: &models.Task{
				Title:       "Improve code quality",
				Description: "Refactor the authentication module",
			},
			tier:     models.TierBuilder,
			expected: ModelOpus,
		},
		{
			name: "redesign keyword",
			task: &models.Task{
				Title:       "Redesign the user dashboard",
				Description: "Complete overhaul of UI",
			},
			tier:     models.TierBuilder,
			expected: ModelOpus,
		},
		{
			name: "complex keyword",
			task: &models.Task{
				Title:       "Implement complex feature",
				Description: "Multi-step process with many edge cases",
			},
			tier:     models.TierScout,
			expected: ModelOpus,
		},

		// Default to tier's model when no keywords match
		{
			name: "no keywords - scout tier",
			task: &models.Task{
				Title:       "Update configuration file",
				Description: "Change the timeout value",
			},
			tier:     models.TierScout,
			expected: ModelHaiku,
		},
		{
			name: "no keywords - builder tier",
			task: &models.Task{
				Title:       "Add new endpoint",
				Description: "Create a GET endpoint for users",
			},
			tier:     models.TierBuilder,
			expected: ModelSonnet,
		},
		{
			name: "no keywords - architect tier",
			task: &models.Task{
				Title:       "Review pull request",
				Description: "Check for issues in the code",
			},
			tier:     models.TierArchitect,
			expected: ModelOpus,
		},

		// Case insensitivity
		{
			name: "SIMPLE keyword uppercase",
			task: &models.Task{
				Title:       "SIMPLE change",
				Description: "Very easy task",
			},
			tier:     models.TierBuilder,
			expected: ModelHaiku,
		},
		{
			name: "Architecture keyword mixed case",
			task: &models.Task{
				Title:       "ARCHITECTURE review",
				Description: "Review the system Architecture",
			},
			tier:     models.TierScout,
			expected: ModelOpus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SelectModel(tt.task, tt.tier)
			if result != tt.expected {
				t.Errorf("SelectModel() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetTierDefault(t *testing.T) {
	tests := []struct {
		name     string
		tier     models.Tier
		expected string
	}{
		{
			name:     "scout tier",
			tier:     models.TierScout,
			expected: ModelHaiku,
		},
		{
			name:     "builder tier",
			tier:     models.TierBuilder,
			expected: ModelSonnet,
		},
		{
			name:     "architect tier",
			tier:     models.TierArchitect,
			expected: ModelOpus,
		},
		{
			name:     "unknown tier",
			tier:     models.Tier("unknown"),
			expected: ModelSonnet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTierDefault(tt.tier)
			if result != tt.expected {
				t.Errorf("getTierDefault() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestContainsHaikuKeyword(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"simple task", true},
		{"add boilerplate code", true},
		{"fix typo in name", true},
		{"trivial change", true},
		{"formatting fix", true},
		{"SIMPLE TASK", true}, // case insensitive
		{"implement feature", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := ContainsHaikuKeyword(tt.text)
			if result != tt.expected {
				t.Errorf("ContainsHaikuKeyword(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestContainsOpusKeyword(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"system architecture", true},
		{"api design", true},
		{"refactor module", true},
		{"redesign ui", true},
		{"complex task", true},
		{"ARCHITECTURE REVIEW", true}, // case insensitive
		{"simple fix", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := ContainsOpusKeyword(tt.text)
			if result != tt.expected {
				t.Errorf("ContainsOpusKeyword(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestHaikuKeywordsTakePrecedence(t *testing.T) {
	// When both haiku and opus keywords are present, haiku should take precedence
	// (checked first in SelectModel)
	task := &models.Task{
		Title:       "Simple architecture fix",
		Description: "Trivial design change",
	}

	result := SelectModel(task, models.TierBuilder)
	if result != ModelHaiku {
		t.Errorf("Expected haiku to take precedence when both keyword types present, got %v", result)
	}
}
