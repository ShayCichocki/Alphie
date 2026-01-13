package main

import (
	"testing"

	"github.com/shayc/alphie/internal/config"
	"github.com/shayc/alphie/pkg/models"
)

func TestModelForTier(t *testing.T) {
	tests := []struct {
		name     string
		tier     models.Tier
		expected string
	}{
		{
			name:     "quick tier uses haiku",
			tier:     models.TierQuick,
			expected: "claude-haiku-3-5-20241022",
		},
		{
			name:     "scout tier uses haiku",
			tier:     models.TierScout,
			expected: "claude-haiku-3-5-20241022",
		},
		{
			name:     "builder tier uses sonnet",
			tier:     models.TierBuilder,
			expected: "claude-sonnet-4-20250514",
		},
		{
			name:     "architect tier uses opus",
			tier:     models.TierArchitect,
			expected: "claude-opus-4-20250514",
		},
		{
			name:     "unknown tier defaults to sonnet",
			tier:     models.Tier("unknown"),
			expected: "claude-sonnet-4-20250514",
		},
		{
			name:     "empty tier defaults to sonnet",
			tier:     models.Tier(""),
			expected: "claude-sonnet-4-20250514",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := modelForTier(tt.tier)
			if result != tt.expected {
				t.Errorf("modelForTier(%q) = %q, want %q", tt.tier, result, tt.expected)
			}
		})
	}
}

func TestMaxAgentsFromTierConfigs_NilConfigs(t *testing.T) {
	tests := []struct {
		name     string
		tier     models.Tier
		expected int
	}{
		{"quick tier", models.TierQuick, 1},
		{"scout tier", models.TierScout, 2},
		{"builder tier", models.TierBuilder, 3},
		{"architect tier", models.TierArchitect, 5},
		{"unknown tier", models.Tier("unknown"), 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maxAgentsFromTierConfigs(tt.tier, nil)
			if result != tt.expected {
				t.Errorf("maxAgentsFromTierConfigs(%q, nil) = %d, want %d", tt.tier, result, tt.expected)
			}
		})
	}
}

func TestMaxAgentsFromTierConfigs_WithConfigs(t *testing.T) {
	// Create tier configs with custom values
	tierConfigs := &config.TierConfigs{
		Scout: &config.TierConfig{
			MaxAgents: 4,
		},
		Builder: &config.TierConfig{
			MaxAgents: 6,
		},
		Architect: &config.TierConfig{
			MaxAgents: 10,
		},
	}

	tests := []struct {
		name     string
		tier     models.Tier
		expected int
	}{
		{"scout uses config value", models.TierScout, 4},
		{"builder uses config value", models.TierBuilder, 6},
		{"architect uses config value", models.TierArchitect, 10},
		// Quick tier is not in config, so it falls back to builder config value
		// since TierConfigs.Get() returns Builder for unknown tiers
		{"quick uses builder fallback", models.TierQuick, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maxAgentsFromTierConfigs(tt.tier, tierConfigs)
			if result != tt.expected {
				t.Errorf("maxAgentsFromTierConfigs(%q, configs) = %d, want %d", tt.tier, result, tt.expected)
			}
		})
	}
}

func TestMaxAgentsFromTierConfigs_ZeroValue(t *testing.T) {
	// Config with zero MaxAgents should fall back to defaults
	tierConfigs := &config.TierConfigs{
		Builder: &config.TierConfig{
			MaxAgents: 0,
		},
	}

	result := maxAgentsFromTierConfigs(models.TierBuilder, tierConfigs)
	if result != 3 {
		t.Errorf("maxAgentsFromTierConfigs with zero MaxAgents = %d, want 3 (default)", result)
	}
}

func TestTierValidation(t *testing.T) {
	tests := []struct {
		name  string
		tier  models.Tier
		valid bool
	}{
		{"quick is valid", models.TierQuick, true},
		{"scout is valid", models.TierScout, true},
		{"builder is valid", models.TierBuilder, true},
		{"architect is valid", models.TierArchitect, true},
		{"unknown is invalid", models.Tier("unknown"), false},
		{"empty is invalid", models.Tier(""), false},
		{"typo is invalid", models.Tier("biulder"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.tier.Valid()
			if result != tt.valid {
				t.Errorf("Tier(%q).Valid() = %v, want %v", tt.tier, result, tt.valid)
			}
		})
	}
}

func TestModelForTier_Consistency(t *testing.T) {
	// Ensure haiku models are used for lightweight tiers
	haikuTiers := []models.Tier{models.TierQuick, models.TierScout}
	for _, tier := range haikuTiers {
		model := modelForTier(tier)
		if model != "claude-haiku-3-5-20241022" {
			t.Errorf("Tier %q should use haiku model, got %q", tier, model)
		}
	}

	// Ensure more capable models for heavier tiers
	if modelForTier(models.TierBuilder) == modelForTier(models.TierQuick) {
		t.Error("Builder tier should use a different model than Quick tier")
	}

	if modelForTier(models.TierArchitect) == modelForTier(models.TierBuilder) {
		t.Error("Architect tier should use a different model than Builder tier")
	}
}

func TestMaxAgentsFromTierConfigs_Scaling(t *testing.T) {
	// Verify that agent counts increase with tier complexity
	nilConfigs := (*config.TierConfigs)(nil)

	quickAgents := maxAgentsFromTierConfigs(models.TierQuick, nilConfigs)
	scoutAgents := maxAgentsFromTierConfigs(models.TierScout, nilConfigs)
	builderAgents := maxAgentsFromTierConfigs(models.TierBuilder, nilConfigs)
	architectAgents := maxAgentsFromTierConfigs(models.TierArchitect, nilConfigs)

	if quickAgents >= scoutAgents {
		t.Errorf("Quick (%d) should have fewer agents than Scout (%d)", quickAgents, scoutAgents)
	}
	if scoutAgents >= builderAgents {
		t.Errorf("Scout (%d) should have fewer agents than Builder (%d)", scoutAgents, builderAgents)
	}
	if builderAgents >= architectAgents {
		t.Errorf("Builder (%d) should have fewer agents than Architect (%d)", builderAgents, architectAgents)
	}
}
