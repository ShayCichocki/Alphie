package tui

import (
	"testing"

	"github.com/ShayCichocki/alphie/pkg/models"
)

func TestClassifyTier(t *testing.T) {
	tests := []struct {
		input    string
		wantTier interface{}
		wantTask string
	}{
		// Explicit prefixes
		{"!quick change button color to blue", nil, "change button color to blue"},
		{"!quick fix typo in README", nil, "fix typo in README"},
		{"!scout find the auth code", nil, "find the auth code"},
		{"!builder add dark mode", nil, "add dark mode"},
		{"!architect refactor the database layer", nil, "refactor the database layer"},

		// Scout keywords
		{"find where the login form is", nil, "find where the login form is"},
		{"search for API endpoints", nil, "search for API endpoints"},
		{"list all user models", nil, "list all user models"},
		{"check if tests pass", nil, "check if tests pass"},
		{"what is the config format", nil, "what is the config format"},
		{"where are errors handled", nil, "where are errors handled"},

		// Architect keywords
		{"refactor the auth system", nil, "refactor the auth system"},
		{"redesign the API layer", nil, "redesign the API layer"},
		{"migrate to new database", nil, "migrate to new database"},
		{"rewrite the parser", nil, "rewrite the parser"},

		// Default to builder
		{"add dark mode toggle", nil, "add dark mode toggle"},
		{"fix the login bug", nil, "fix the login bug"},
		{"implement user settings page", nil, "implement user settings page"},
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
