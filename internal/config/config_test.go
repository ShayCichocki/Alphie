package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ShayCichocki/alphie/pkg/models"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Defaults.Tier != "builder" {
		t.Errorf("expected default tier 'builder', got %q", cfg.Defaults.Tier)
	}

	if cfg.Defaults.TokenBudget != 100000 {
		t.Errorf("expected default token budget 100000, got %d", cfg.Defaults.TokenBudget)
	}

	if cfg.TUI.RefreshRate != 100*time.Millisecond {
		t.Errorf("expected refresh rate 100ms, got %v", cfg.TUI.RefreshRate)
	}

	if cfg.Timeouts.Scout != 5*time.Minute {
		t.Errorf("expected scout timeout 5m, got %v", cfg.Timeouts.Scout)
	}

	if cfg.Timeouts.Builder != 15*time.Minute {
		t.Errorf("expected builder timeout 15m, got %v", cfg.Timeouts.Builder)
	}

	if cfg.Timeouts.Architect != 30*time.Minute {
		t.Errorf("expected architect timeout 30m, got %v", cfg.Timeouts.Architect)
	}

	if !cfg.QualityGates.Test {
		t.Error("expected quality_gates.test to be true")
	}

	if !cfg.QualityGates.Build {
		t.Error("expected quality_gates.build to be true")
	}

	if !cfg.QualityGates.Lint {
		t.Error("expected quality_gates.lint to be true")
	}

	if !cfg.QualityGates.Typecheck {
		t.Error("expected quality_gates.typecheck to be true")
	}
}

func TestLoadFromPath(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
anthropic:
  api_key: test-key
defaults:
  tier: scout
  token_budget: 50000
tui:
  refresh_rate: 200ms
timeouts:
  scout: 10m
  builder: 20m
  architect: 40m
quality_gates:
  test: false
  build: true
  lint: false
  typecheck: true
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	if cfg.Anthropic.APIKey != "test-key" {
		t.Errorf("expected api_key 'test-key', got %q", cfg.Anthropic.APIKey)
	}

	if cfg.Defaults.Tier != "scout" {
		t.Errorf("expected tier 'scout', got %q", cfg.Defaults.Tier)
	}

	if cfg.Defaults.TokenBudget != 50000 {
		t.Errorf("expected token budget 50000, got %d", cfg.Defaults.TokenBudget)
	}

	if cfg.TUI.RefreshRate != 200*time.Millisecond {
		t.Errorf("expected refresh rate 200ms, got %v", cfg.TUI.RefreshRate)
	}

	if cfg.Timeouts.Scout != 10*time.Minute {
		t.Errorf("expected scout timeout 10m, got %v", cfg.Timeouts.Scout)
	}

	if cfg.QualityGates.Test {
		t.Error("expected quality_gates.test to be false")
	}

	if !cfg.QualityGates.Build {
		t.Error("expected quality_gates.build to be true")
	}
}

func TestExpandEnv(t *testing.T) {
	// Set environment variable
	os.Setenv("TEST_VAR", "expanded-value")
	defer os.Unsetenv("TEST_VAR")

	result := expandEnv("${TEST_VAR}")
	if result != "expanded-value" {
		t.Errorf("expected 'expanded-value', got %q", result)
	}

	result = expandEnv("prefix-${TEST_VAR}-suffix")
	if result != "prefix-expanded-value-suffix" {
		t.Errorf("expected 'prefix-expanded-value-suffix', got %q", result)
	}
}

func TestGetUserConfigDir(t *testing.T) {
	// Test with XDG_CONFIG_HOME set
	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	defer os.Unsetenv("XDG_CONFIG_HOME")

	dir := getUserConfigDir()
	expected := "/custom/config/alphie"
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}

func TestLoadTierConfigs(t *testing.T) {
	// Create temporary tier config files
	tmpDir := t.TempDir()

	// Create scout.yaml
	scoutContent := `
tier: scout
max_agents: 2
primary_model: haiku
quality_threshold: 5
max_ralph_iterations: 3
questions_allowed: 0
timeout: 5m
override_gates:
  blocked_after_n_attempts: 5
  protected_area_detected: true
models:
  default: haiku
  fallback: null
`
	if err := os.WriteFile(filepath.Join(tmpDir, "scout.yaml"), []byte(scoutContent), 0644); err != nil {
		t.Fatalf("failed to write scout.yaml: %v", err)
	}

	// Create builder.yaml
	builderContent := `
tier: builder
max_agents: 3
primary_model: sonnet
quality_threshold: 7
max_ralph_iterations: 5
questions_allowed: 2
timeout: 15m
models:
  default: sonnet
  fallback: haiku
review:
  sampled_second_reviewer: true
  sample_conditions:
    - protected_area
    - large_diff
`
	if err := os.WriteFile(filepath.Join(tmpDir, "builder.yaml"), []byte(builderContent), 0644); err != nil {
		t.Fatalf("failed to write builder.yaml: %v", err)
	}

	// Create architect.yaml
	architectContent := `
tier: architect
max_agents: 5
primary_model: opus
quality_threshold: 8
max_ralph_iterations: 7
questions_allowed: unlimited
timeout: 30m
models:
  default: opus
  fallback: sonnet
review:
  human_review_required: true
  sampled_second_reviewer: true
  sample_conditions:
    - protected_area
    - large_diff
`
	if err := os.WriteFile(filepath.Join(tmpDir, "architect.yaml"), []byte(architectContent), 0644); err != nil {
		t.Fatalf("failed to write architect.yaml: %v", err)
	}

	// Load tier configs
	tierCfg, err := LoadTierConfigs(tmpDir)
	if err != nil {
		t.Fatalf("LoadTierConfigs failed: %v", err)
	}

	// Verify scout config
	if tierCfg.Scout == nil {
		t.Fatal("expected scout config to be non-nil")
	}
	if tierCfg.Scout.Tier != "scout" {
		t.Errorf("expected scout tier 'scout', got %q", tierCfg.Scout.Tier)
	}
	if tierCfg.Scout.MaxAgents != 2 {
		t.Errorf("expected scout max_agents 2, got %d", tierCfg.Scout.MaxAgents)
	}
	if tierCfg.Scout.QualityThreshold != 5 {
		t.Errorf("expected scout quality_threshold 5, got %d", tierCfg.Scout.QualityThreshold)
	}
	if tierCfg.Scout.MaxRalphIterations != 3 {
		t.Errorf("expected scout max_ralph_iterations 3, got %d", tierCfg.Scout.MaxRalphIterations)
	}
	if tierCfg.Scout.GetQuestionsAllowedInt() != 0 {
		t.Errorf("expected scout questions_allowed 0, got %d", tierCfg.Scout.GetQuestionsAllowedInt())
	}
	if tierCfg.Scout.OverrideGates == nil {
		t.Error("expected scout override_gates to be non-nil")
	} else {
		if tierCfg.Scout.OverrideGates.BlockedAfterNAttempts != 5 {
			t.Errorf("expected blocked_after_n_attempts 5, got %d", tierCfg.Scout.OverrideGates.BlockedAfterNAttempts)
		}
		if !tierCfg.Scout.OverrideGates.ProtectedAreaDetected {
			t.Error("expected protected_area_detected to be true")
		}
	}

	// Verify builder config
	if tierCfg.Builder == nil {
		t.Fatal("expected builder config to be non-nil")
	}
	if tierCfg.Builder.MaxAgents != 3 {
		t.Errorf("expected builder max_agents 3, got %d", tierCfg.Builder.MaxAgents)
	}
	if tierCfg.Builder.QualityThreshold != 7 {
		t.Errorf("expected builder quality_threshold 7, got %d", tierCfg.Builder.QualityThreshold)
	}
	if tierCfg.Builder.GetQuestionsAllowedInt() != 2 {
		t.Errorf("expected builder questions_allowed 2, got %d", tierCfg.Builder.GetQuestionsAllowedInt())
	}
	if tierCfg.Builder.Review == nil {
		t.Error("expected builder review to be non-nil")
	} else {
		if !tierCfg.Builder.Review.SampledSecondReviewer {
			t.Error("expected sampled_second_reviewer to be true")
		}
	}

	// Verify architect config
	if tierCfg.Architect == nil {
		t.Fatal("expected architect config to be non-nil")
	}
	if tierCfg.Architect.MaxAgents != 5 {
		t.Errorf("expected architect max_agents 5, got %d", tierCfg.Architect.MaxAgents)
	}
	if tierCfg.Architect.QualityThreshold != 8 {
		t.Errorf("expected architect quality_threshold 8, got %d", tierCfg.Architect.QualityThreshold)
	}
	if tierCfg.Architect.GetQuestionsAllowedInt() != -1 {
		t.Errorf("expected architect questions_allowed -1 (unlimited), got %d", tierCfg.Architect.GetQuestionsAllowedInt())
	}
	if tierCfg.Architect.Review == nil {
		t.Error("expected architect review to be non-nil")
	} else {
		if !tierCfg.Architect.Review.HumanReviewRequired {
			t.Error("expected human_review_required to be true")
		}
	}
}

func TestDefaultTierConfigs(t *testing.T) {
	tierCfg := DefaultTierConfigs()

	if tierCfg.Scout == nil || tierCfg.Builder == nil || tierCfg.Architect == nil {
		t.Fatal("expected all tier configs to be non-nil")
	}

	// Verify scout defaults
	if tierCfg.Scout.MaxAgents != 2 {
		t.Errorf("expected default scout max_agents 2, got %d", tierCfg.Scout.MaxAgents)
	}
	if tierCfg.Scout.QualityThreshold != 5 {
		t.Errorf("expected default scout quality_threshold 5, got %d", tierCfg.Scout.QualityThreshold)
	}

	// Verify builder defaults
	if tierCfg.Builder.MaxAgents != 3 {
		t.Errorf("expected default builder max_agents 3, got %d", tierCfg.Builder.MaxAgents)
	}
	if tierCfg.Builder.QualityThreshold != 7 {
		t.Errorf("expected default builder quality_threshold 7, got %d", tierCfg.Builder.QualityThreshold)
	}

	// Verify architect defaults
	if tierCfg.Architect.MaxAgents != 5 {
		t.Errorf("expected default architect max_agents 5, got %d", tierCfg.Architect.MaxAgents)
	}
	if tierCfg.Architect.QualityThreshold != 8 {
		t.Errorf("expected default architect quality_threshold 8, got %d", tierCfg.Architect.QualityThreshold)
	}
}

func TestTierConfigsGet(t *testing.T) {
	tierCfg := DefaultTierConfigs()

	tests := []struct {
		tier     string
		expected *TierConfig
	}{
		{"scout", tierCfg.Scout},
		{"builder", tierCfg.Builder},
		{"architect", tierCfg.Architect},
		{"unknown", tierCfg.Builder}, // Defaults to builder
	}

	for _, tc := range tests {
		got := tierCfg.Get(models.Tier(tc.tier))
		if got != tc.expected {
			t.Errorf("Get(%q) = %v, want %v", tc.tier, got, tc.expected)
		}
	}
}
