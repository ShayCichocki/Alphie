package tui

import (
	"testing"

	"github.com/shayc/alphie/pkg/models"
)

func TestClassifyTier(t *testing.T) {
	tests := []struct {
		input    string
		wantTier models.Tier
		wantTask string
	}{
		// Explicit prefixes
		{"!quick change button color to blue", models.TierQuick, "change button color to blue"},
		{"!quick fix typo in README", models.TierQuick, "fix typo in README"},
		{"!scout find the auth code", models.TierScout, "find the auth code"},
		{"!builder add dark mode", models.TierBuilder, "add dark mode"},
		{"!architect refactor the database layer", models.TierArchitect, "refactor the database layer"},

		// Scout keywords
		{"find where the login form is", models.TierScout, "find where the login form is"},
		{"search for API endpoints", models.TierScout, "search for API endpoints"},
		{"list all user models", models.TierScout, "list all user models"},
		{"check if tests pass", models.TierScout, "check if tests pass"},
		{"what is the config format", models.TierScout, "what is the config format"},
		{"where are errors handled", models.TierScout, "where are errors handled"},

		// Architect keywords
		{"refactor the auth system", models.TierArchitect, "refactor the auth system"},
		{"redesign the API layer", models.TierArchitect, "redesign the API layer"},
		{"migrate to new database", models.TierArchitect, "migrate to new database"},
		{"rewrite the parser", models.TierArchitect, "rewrite the parser"},

		// Default to builder
		{"add dark mode toggle", models.TierBuilder, "add dark mode toggle"},
		{"fix the login bug", models.TierBuilder, "fix the login bug"},
		{"implement user settings page", models.TierBuilder, "implement user settings page"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotTier, gotTask := ClassifyTier(tt.input)
			if gotTier != tt.wantTier {
				t.Errorf("ClassifyTier(%q) tier = %v, want %v", tt.input, gotTier, tt.wantTier)
			}
			if gotTask != tt.wantTask {
				t.Errorf("ClassifyTier(%q) task = %q, want %q", tt.input, gotTask, tt.wantTask)
			}
		})
	}
}
