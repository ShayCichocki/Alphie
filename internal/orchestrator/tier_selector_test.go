package orchestrator

import (
	"testing"

	"github.com/shayc/alphie/pkg/models"
)

func TestSelectTier_ScoutKeywords(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        models.Tier
	}{
		{"docs keyword", "Update the docs for the API", models.TierScout},
		{"readme keyword", "Fix typos in README", models.TierScout},
		{"documentation keyword", "Add documentation for the new feature", models.TierScout},
		{"typo keyword", "Fix typo in error message", models.TierScout},
		{"formatting keyword", "Apply code formatting to utils package", models.TierScout},
		{"comment keyword", "Add comment to explain the algorithm", models.TierScout},
		{"single-file keyword", "Make a single-file change to config", models.TierScout},
		{"uppercase docs", "Update DOCS for API", models.TierScout},
		{"mixed case typo", "Fix Typo in the codebase", models.TierScout},
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
		{"refactoring", "Refactor the parser module", models.TierBuilder},
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
	detector := NewProtectedAreaDetector()
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
		{"scout", "Fix typo in README", models.TierScout},
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
