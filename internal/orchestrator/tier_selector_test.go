package orchestrator

import (
	"testing"

	"github.com/ShayCichocki/alphie/internal/protect"
	"github.com/ShayCichocki/alphie/pkg/models"
)

func TestSelectTier_QuickKeywords(t *testing.T) {
	// Quick keywords are simple tasks that don't need decomposition
	tests := []struct {
		name        string
		description string
		want        models.Tier
	}{
		{"typo keyword", "Fix typo in error message", models.TierQuick},
		{"rename keyword", "Rename the function to be clearer", models.TierQuick},
		{"formatting keyword", "Apply code formatting to utils package", models.TierQuick},
		{"comment keyword", "Add comment to explain the algorithm", models.TierQuick},
		{"mixed case typo", "Fix Typo in the codebase", models.TierQuick},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := NewTierSelector(nil)
			got := selector.SelectTier(tt.description)
			if got != tt.want {
				t.Errorf("SelectTier(%q) = %v, want %v", tt.description, got, tt.want)
			}
		})
	}
}

func TestSelectTier_ScoutKeywords(t *testing.T) {
	// Scout keywords are for exploration and research tasks
	tests := []struct {
		name        string
		description string
		want        models.Tier
	}{
		{"find keyword", "Find the login handler", models.TierScout},
		{"search keyword", "Search for API endpoints", models.TierScout},
		{"list keyword", "List all user models", models.TierScout},
		{"check keyword", "Check if tests pass", models.TierScout},
		{"docs keyword", "Update the docs for the API", models.TierScout},
		{"documentation keyword", "Add documentation for the new feature", models.TierScout},
		{"readme keyword", "Check the README for instructions", models.TierScout},
		{"what keyword", "What is the config format", models.TierScout},
		{"where keyword", "Where are errors handled", models.TierScout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := NewTierSelector(nil)
			got := selector.SelectTier(tt.description)
			if got != tt.want {
				t.Errorf("SelectTier(%q) = %v, want %v", tt.description, got, tt.want)
			}
		})
	}
}

func TestSelectTier_ArchitectKeywords(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        models.Tier
	}{
		{"migration keyword", "Add database migration for users table", models.TierArchitect},
		{"auth keyword", "Implement auth flow for OAuth", models.TierArchitect},
		{"authentication keyword", "Fix authentication bug in login", models.TierArchitect},
		{"security keyword", "Add security headers to responses", models.TierArchitect},
		{"infra keyword", "Update infra configuration", models.TierArchitect},
		{"infrastructure keyword", "Refactor infrastructure layer", models.TierArchitect},
		{"schema keyword", "Update database schema", models.TierArchitect},
		{"database keyword", "Optimize database queries", models.TierArchitect},
		{"uppercase migration", "Add MIGRATION script", models.TierArchitect},
		{"mixed case auth", "Fix Auth service", models.TierArchitect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := NewTierSelector(nil)
			got := selector.SelectTier(tt.description)
			if got != tt.want {
				t.Errorf("SelectTier(%q) = %v, want %v", tt.description, got, tt.want)
			}
		})
	}
}

func TestSelectTier_ArchitectTakesPrecedence(t *testing.T) {
	// When both scout and architect keywords are present, architect should win.
	tests := []struct {
		name        string
		description string
		want        models.Tier
	}{
		{"auth in docs", "Update auth documentation", models.TierArchitect},
		{"migration typo", "Fix typo in migration script", models.TierArchitect},
		{"security comment", "Add comment about security implications", models.TierArchitect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := NewTierSelector(nil)
			got := selector.SelectTier(tt.description)
			if got != tt.want {
				t.Errorf("SelectTier(%q) = %v, want %v", tt.description, got, tt.want)
			}
		})
	}
}

func TestSelectTier_DefaultToBuilder(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        models.Tier
	}{
		{"generic task", "Add new feature to handle user input", models.TierBuilder},
		{"tests", "Add unit tests for the service", models.TierBuilder},
		{"bug fix", "Fix bug in file upload handler", models.TierBuilder},
		{"empty string", "", models.TierBuilder},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := NewTierSelector(nil)
			got := selector.SelectTier(tt.description)
			if got != tt.want {
				t.Errorf("SelectTier(%q) = %v, want %v", tt.description, got, tt.want)
			}
		})
	}
}

func TestSelectTier_WithProtectedAreaDetector(t *testing.T) {
	detector := protect.New()
	selector := NewTierSelector(detector)

	tests := []struct {
		name        string
		description string
		want        models.Tier
	}{
		{"protected path reference", "Update the internal/auth/handler.go file", models.TierArchitect},
		{"migrations path", "Modify migrations/001_create_users.sql", models.TierArchitect},
		{"terraform reference", "Update terraform/main.tf", models.TierArchitect},
		{"secrets reference", "Check secrets/config.yaml", models.TierArchitect},
		{"env file", "Update .env file", models.TierArchitect},
		{"sql file reference", "Fix issue in schema.sql", models.TierArchitect},
		{"pem file reference", "Rotate server.pem certificate", models.TierArchitect},
		{"key file reference", "Update private.key", models.TierArchitect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selector.SelectTier(tt.description)
			if got != tt.want {
				t.Errorf("SelectTier(%q) = %v, want %v", tt.description, got, tt.want)
			}
		})
	}
}

func TestSelectTier_ConvenienceFunction(t *testing.T) {
	// Test the package-level convenience function.
	tests := []struct {
		name        string
		description string
		want        models.Tier
	}{
		{"quick", "Fix typo in the code", models.TierQuick},
		{"scout", "Find all API endpoints", models.TierScout},
		{"architect", "Add database migration", models.TierArchitect},
		{"builder", "Implement new feature", models.TierBuilder},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectTier(tt.description)
			if got != tt.want {
				t.Errorf("SelectTier(%q) = %v, want %v", tt.description, got, tt.want)
			}
		})
	}
}

func TestTierSelector_NilDetector(t *testing.T) {
	// Ensure it works correctly with nil detector.
	selector := NewTierSelector(nil)

	// This should not panic and should return Builder (default).
	got := selector.SelectTier("Update internal/auth/handler.go")
	// Without detector, "auth" keyword should trigger Architect.
	if got != models.TierArchitect {
		t.Errorf("SelectTier() = %v, want %v (auth keyword should trigger Architect)", got, models.TierArchitect)
	}
}
