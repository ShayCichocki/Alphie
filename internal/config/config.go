// Package config handles configuration loading and management for Alphie.
// It supports XDG config paths, project-level overrides, and environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// Config holds all configuration for Alphie.
type Config struct {
	Anthropic    AnthropicConfig    `mapstructure:"anthropic"`
	Defaults     DefaultsConfig     `mapstructure:"defaults"`
	TUI          TUIConfig          `mapstructure:"tui"`
	Timeouts     TimeoutsConfig     `mapstructure:"timeouts"`
	QualityGates QualityGatesConfig `mapstructure:"quality_gates"`
}

// AnthropicConfig holds Anthropic API settings.
type AnthropicConfig struct {
	APIKey string `mapstructure:"api_key"`
}

// DefaultsConfig holds default values for Alphie sessions.
type DefaultsConfig struct {
	Tier        string `mapstructure:"tier"`
	TokenBudget int    `mapstructure:"token_budget"`
}

// TUIConfig holds TUI display settings.
type TUIConfig struct {
	RefreshRate time.Duration `mapstructure:"refresh_rate"`
}

// TimeoutsConfig holds timeout settings per tier.
type TimeoutsConfig struct {
	Scout     time.Duration `mapstructure:"scout"`
	Builder   time.Duration `mapstructure:"builder"`
	Architect time.Duration `mapstructure:"architect"`
}

// QualityGatesConfig holds quality gate toggles.
type QualityGatesConfig struct {
	Test      bool `mapstructure:"test"`
	Build     bool `mapstructure:"build"`
	Lint      bool `mapstructure:"lint"`
	Typecheck bool `mapstructure:"typecheck"`
}

// TierConfig holds configuration for a single tier loaded from YAML.
type TierConfig struct {
	// Tier is the tier name (scout, builder, architect).
	Tier string `mapstructure:"tier"`
	// MaxAgents is the maximum number of concurrent agents.
	MaxAgents int `mapstructure:"max_agents"`
	// PrimaryModel is the primary LLM model to use.
	PrimaryModel string `mapstructure:"primary_model"`
	// QualityThreshold is the rubric score threshold (out of 9).
	QualityThreshold int `mapstructure:"quality_threshold"`
	// MaxRalphIterations is the maximum ralph-loop iterations.
	MaxRalphIterations int `mapstructure:"max_ralph_iterations"`
	// QuestionsAllowed is the number of questions the agent can ask (-1 for unlimited).
	QuestionsAllowed interface{} `mapstructure:"questions_allowed"`
	// Timeout is the per-task timeout duration.
	Timeout time.Duration `mapstructure:"timeout"`
	// OverrideGates contains override gate configuration.
	OverrideGates *OverrideGatesConfig `mapstructure:"override_gates"`
	// Models contains model selection configuration.
	Models *ModelsConfig `mapstructure:"models"`
	// Review contains review settings.
	Review *ReviewConfig `mapstructure:"review"`
}

// OverrideGatesConfig holds override gate settings for Scout tier.
type OverrideGatesConfig struct {
	// BlockedAfterNAttempts allows questions after N failed attempts.
	BlockedAfterNAttempts int `mapstructure:"blocked_after_n_attempts"`
	// ProtectedAreaDetected allows questions when protected areas are detected.
	ProtectedAreaDetected bool `mapstructure:"protected_area_detected"`
}

// ModelsConfig holds model selection settings.
type ModelsConfig struct {
	// Default is the default model to use.
	Default string `mapstructure:"default"`
	// Fallback is the fallback model when the default fails.
	Fallback string `mapstructure:"fallback"`
}

// ReviewConfig holds review settings for a tier.
type ReviewConfig struct {
	// HumanReviewRequired indicates if human review is required.
	HumanReviewRequired bool `mapstructure:"human_review_required"`
	// SampledSecondReviewer enables probabilistic second review.
	SampledSecondReviewer bool `mapstructure:"sampled_second_reviewer"`
	// SampleConditions lists conditions that trigger second review.
	SampleConditions []string `mapstructure:"sample_conditions"`
}

// GetQuestionsAllowedInt returns the questions allowed as an integer.
// Returns -1 for "unlimited", the numeric value otherwise.
func (tc *TierConfig) GetQuestionsAllowedInt() int {
	switch v := tc.QuestionsAllowed.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		if v == "unlimited" {
			return -1
		}
		return 0
	default:
		return 0
	}
}

// TierConfigs holds all tier configurations.
type TierConfigs struct {
	Scout     *TierConfig
	Builder   *TierConfig
	Architect *TierConfig
}

// Get returns the tier config for the given tier.
func (tc *TierConfigs) Get(tier models.Tier) *TierConfig {
	switch tier {
	case models.TierScout:
		return tc.Scout
	case models.TierBuilder:
		return tc.Builder
	case models.TierArchitect:
		return tc.Architect
	default:
		return tc.Builder // Default to builder
	}
}

// Load loads configuration from XDG paths, project overrides, and environment variables.
// Precedence (highest to lowest):
// 1. Environment variables (ANTHROPIC_API_KEY)
// 2. Project config (.alphie.yaml in current directory or parent)
// 3. User config (~/.config/alphie/config.yaml)
// 4. Built-in defaults
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Load user config from XDG path
	userConfigDir := getUserConfigDir()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(userConfigDir)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading user config: %w", err)
		}
	}

	// Load project config if present
	projectConfig := findProjectConfig()
	if projectConfig != "" {
		projectViper := viper.New()
		projectViper.SetConfigFile(projectConfig)
		if err := projectViper.ReadInConfig(); err == nil {
			// Merge project config (takes precedence)
			if err := v.MergeConfigMap(projectViper.AllSettings()); err != nil {
				return nil, fmt.Errorf("merging project config: %w", err)
			}
		}
	}

	// Environment variable overrides
	v.AutomaticEnv()
	v.SetEnvPrefix("")

	// Map specific environment variables
	v.BindEnv("anthropic.api_key", "ANTHROPIC_API_KEY")

	// Expand environment variable references in api_key
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Expand ${VAR} references
	cfg.Anthropic.APIKey = expandEnv(cfg.Anthropic.APIKey)

	return cfg, nil
}

// LoadFromPath loads configuration from a specific path (for testing).
func LoadFromPath(path string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config from %s: %w", path, err)
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	cfg.Anthropic.APIKey = expandEnv(cfg.Anthropic.APIKey)

	return cfg, nil
}

// Save writes the current configuration to the user config file.
func Save(cfg *Config) error {
	userConfigDir := getUserConfigDir()
	if err := os.MkdirAll(userConfigDir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	configPath := filepath.Join(userConfigDir, "config.yaml")

	v := viper.New()
	v.SetConfigFile(configPath)

	v.Set("anthropic.api_key", cfg.Anthropic.APIKey)
	v.Set("defaults.tier", cfg.Defaults.Tier)
	v.Set("defaults.token_budget", cfg.Defaults.TokenBudget)
	v.Set("tui.refresh_rate", cfg.TUI.RefreshRate.String())
	v.Set("timeouts.scout", cfg.Timeouts.Scout.String())
	v.Set("timeouts.builder", cfg.Timeouts.Builder.String())
	v.Set("timeouts.architect", cfg.Timeouts.Architect.String())
	v.Set("quality_gates.test", cfg.QualityGates.Test)
	v.Set("quality_gates.build", cfg.QualityGates.Build)
	v.Set("quality_gates.lint", cfg.QualityGates.Lint)
	v.Set("quality_gates.typecheck", cfg.QualityGates.Typecheck)

	return v.WriteConfig()
}

// GetUserConfigPath returns the path to the user config file.
func GetUserConfigPath() string {
	return filepath.Join(getUserConfigDir(), "config.yaml")
}

// GetProjectConfigPath returns the path to the project config file if it exists.
func GetProjectConfigPath() string {
	return findProjectConfig()
}

// setDefaults configures default values.
func setDefaults(v *viper.Viper) {
	// Anthropic defaults
	v.SetDefault("anthropic.api_key", "")

	// Session defaults
	v.SetDefault("defaults.tier", "builder")
	v.SetDefault("defaults.token_budget", 100000)

	// TUI defaults
	v.SetDefault("tui.refresh_rate", "100ms")

	// Timeout defaults
	v.SetDefault("timeouts.scout", "5m")
	v.SetDefault("timeouts.builder", "15m")
	v.SetDefault("timeouts.architect", "30m")

	// Quality gate defaults
	v.SetDefault("quality_gates.test", true)
	v.SetDefault("quality_gates.build", true)
	v.SetDefault("quality_gates.lint", true)
	v.SetDefault("quality_gates.typecheck", true)
}

// getUserConfigDir returns the XDG config directory for Alphie.
func getUserConfigDir() string {
	// Check XDG_CONFIG_HOME first
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "alphie")
	}

	// Fall back to ~/.config/alphie
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "alphie")
	}
	return filepath.Join(home, ".config", "alphie")
}

// findProjectConfig searches for .alphie.yaml in the current directory and parents.
func findProjectConfig() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		configPath := filepath.Join(cwd, ".alphie.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}

		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}

	return ""
}

// expandEnv expands ${VAR} references in a string.
func expandEnv(s string) string {
	return os.ExpandEnv(s)
}

// Default returns a Config with default values.
func Default() *Config {
	return &Config{
		Anthropic: AnthropicConfig{
			APIKey: "",
		},
		Defaults: DefaultsConfig{
			Tier:        "builder",
			TokenBudget: 100000,
		},
		TUI: TUIConfig{
			RefreshRate: 100 * time.Millisecond,
		},
		Timeouts: TimeoutsConfig{
			Scout:     5 * time.Minute,
			Builder:   15 * time.Minute,
			Architect: 30 * time.Minute,
		},
		QualityGates: QualityGatesConfig{
			Test:      true,
			Build:     true,
			Lint:      true,
			Typecheck: true,
		},
	}
}

// LoadTierConfigs loads tier configurations from the configs/ directory.
// It looks for scout.yaml, builder.yaml, and architect.yaml.
// The configsDir parameter specifies the directory containing the YAML files.
// If configsDir is empty, it defaults to "configs" relative to the current directory.
func LoadTierConfigs(configsDir string) (*TierConfigs, error) {
	if configsDir == "" {
		configsDir = "configs"
	}

	tiers := &TierConfigs{}

	// Load scout config
	scoutPath := filepath.Join(configsDir, "scout.yaml")
	scoutCfg, err := loadTierConfig(scoutPath)
	if err != nil {
		return nil, fmt.Errorf("load scout config: %w", err)
	}
	tiers.Scout = scoutCfg

	// Load builder config
	builderPath := filepath.Join(configsDir, "builder.yaml")
	builderCfg, err := loadTierConfig(builderPath)
	if err != nil {
		return nil, fmt.Errorf("load builder config: %w", err)
	}
	tiers.Builder = builderCfg

	// Load architect config
	architectPath := filepath.Join(configsDir, "architect.yaml")
	architectCfg, err := loadTierConfig(architectPath)
	if err != nil {
		return nil, fmt.Errorf("load architect config: %w", err)
	}
	tiers.Architect = architectCfg

	return tiers, nil
}

// loadTierConfig loads a single tier configuration from a YAML file.
func loadTierConfig(path string) (*TierConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	cfg := &TierConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling %s: %w", path, err)
	}

	return cfg, nil
}

// DefaultTierConfigs returns hardcoded default tier configurations.
// This is used as a fallback when YAML files are not available.
func DefaultTierConfigs() *TierConfigs {
	return &TierConfigs{
		Scout: &TierConfig{
			Tier:               "scout",
			MaxAgents:          2,
			PrimaryModel:       "haiku",
			QualityThreshold:   5,
			MaxRalphIterations: 3,
			QuestionsAllowed:   0,
			Timeout:            5 * time.Minute,
			OverrideGates: &OverrideGatesConfig{
				BlockedAfterNAttempts: 5,
				ProtectedAreaDetected: true,
			},
			Models: &ModelsConfig{
				Default:  "haiku",
				Fallback: "",
			},
		},
		Builder: &TierConfig{
			Tier:               "builder",
			MaxAgents:          3,
			PrimaryModel:       "sonnet",
			QualityThreshold:   7,
			MaxRalphIterations: 5,
			QuestionsAllowed:   2,
			Timeout:            15 * time.Minute,
			Models: &ModelsConfig{
				Default:  "sonnet",
				Fallback: "haiku",
			},
			Review: &ReviewConfig{
				SampledSecondReviewer: true,
				SampleConditions:      []string{"protected_area", "large_diff", "weak_tests", "cross_cutting"},
			},
		},
		Architect: &TierConfig{
			Tier:               "architect",
			MaxAgents:          5,
			PrimaryModel:       "opus",
			QualityThreshold:   8,
			MaxRalphIterations: 7,
			QuestionsAllowed:   "unlimited",
			Timeout:            30 * time.Minute,
			Models: &ModelsConfig{
				Default:  "opus",
				Fallback: "sonnet",
			},
			Review: &ReviewConfig{
				HumanReviewRequired:   true,
				SampledSecondReviewer: true,
				SampleConditions:      []string{"protected_area", "large_diff", "weak_tests", "cross_cutting"},
			},
		},
	}
}
