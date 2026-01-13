package models

import "testing"

func TestTier_Valid(t *testing.T) {
	tests := []struct {
		name string
		tier Tier
		want bool
	}{
		{"scout is valid", TierScout, true},
		{"builder is valid", TierBuilder, true},
		{"architect is valid", TierArchitect, true},
		{"empty string is invalid", Tier(""), false},
		{"unknown tier is invalid", Tier("unknown"), false},
		{"typo tier is invalid", Tier("bulider"), false},
		{"uppercase is invalid", Tier("SCOUT"), false},
		{"mixed case is invalid", Tier("Scout"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tier.Valid(); got != tt.want {
				t.Errorf("Tier(%q).Valid() = %v, want %v", tt.tier, got, tt.want)
			}
		})
	}
}

func TestTier_StringValues(t *testing.T) {
	// Verify the string values are as expected
	tests := []struct {
		tier Tier
		want string
	}{
		{TierScout, "scout"},
		{TierBuilder, "builder"},
		{TierArchitect, "architect"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := string(tt.tier); got != tt.want {
				t.Errorf("string(Tier) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTier_DefaultValue(t *testing.T) {
	tier := Tier("")

	if tier.Valid() {
		t.Error("Empty Tier should not be valid")
	}
	if string(tier) != "" {
		t.Errorf("Empty Tier string should be empty, got %q", string(tier))
	}
}

func TestTier_AllTiersAreDistinct(t *testing.T) {
	tiers := []Tier{
		TierScout,
		TierBuilder,
		TierArchitect,
	}

	seen := make(map[Tier]bool)
	for _, tier := range tiers {
		if seen[tier] {
			t.Errorf("Duplicate Tier: %q", tier)
		}
		seen[tier] = true
	}

	if len(seen) != 3 {
		t.Errorf("Expected 3 distinct Tier values, got %d", len(seen))
	}
}

func TestTier_ValidTierCount(t *testing.T) {
	validTiers := []Tier{TierScout, TierBuilder, TierArchitect}
	validCount := 0

	for _, tier := range validTiers {
		if tier.Valid() {
			validCount++
		}
	}

	if validCount != 3 {
		t.Errorf("Expected all 3 tiers to be valid, got %d", validCount)
	}
}

func TestTier_InvalidTiers(t *testing.T) {
	invalidTiers := []Tier{
		Tier(""),
		Tier("invalid"),
		Tier("SCOUT"),
		Tier("Builder"),
		Tier("ARCHITECT"),
		Tier("scout "),
		Tier(" builder"),
		Tier("tier1"),
	}

	for _, tier := range invalidTiers {
		if tier.Valid() {
			t.Errorf("Tier(%q) should not be valid", tier)
		}
	}
}

func TestTier_UsedInTask(t *testing.T) {
	// Test that Tier can be properly used in Task struct
	task := Task{
		ID:   "test-task",
		Tier: TierBuilder,
	}

	if task.Tier != TierBuilder {
		t.Errorf("Task.Tier = %q, want %q", task.Tier, TierBuilder)
	}
	if !task.Tier.Valid() {
		t.Error("Task.Tier should be valid")
	}
}

func TestTier_UsedInSession(t *testing.T) {
	// Test that Tier can be properly used in Session struct
	session := Session{
		ID:   "test-session",
		Tier: TierArchitect,
	}

	if session.Tier != TierArchitect {
		t.Errorf("Session.Tier = %q, want %q", session.Tier, TierArchitect)
	}
	if !session.Tier.Valid() {
		t.Error("Session.Tier should be valid")
	}
}

func TestTier_Comparison(t *testing.T) {
	// Test that tiers can be compared
	if TierScout == TierBuilder {
		t.Error("TierScout should not equal TierBuilder")
	}
	if TierBuilder == TierArchitect {
		t.Error("TierBuilder should not equal TierArchitect")
	}
	if TierScout == TierArchitect {
		t.Error("TierScout should not equal TierArchitect")
	}

	// Same tier should be equal
	scout1 := TierScout
	scout2 := TierScout
	if scout1 != scout2 {
		t.Error("Same tier values should be equal")
	}
}
