package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config [key] [value]",
	Short: "Manage configuration",
	Long: `View or modify Alphie configuration.

Without arguments, displays current configuration.
With one argument (key), displays the value for that key.
With two arguments (key value), sets the configuration value.

Configuration is stored at ~/.config/alphie/config.yaml
Project-specific overrides can be placed in .alphie.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		switch len(args) {
		case 0:
			displayAllConfig(cfg)
		case 1:
			displayConfigKey(cfg, args[0])
		default:
			setConfigKey(cfg, args[0], args[1])
		}
	},
}

// displayAllConfig prints all configuration values.
func displayAllConfig(cfg *config.Config) {
	// Mask API key if set
	apiKeyDisplay := "(not set)"
	if cfg.Anthropic.APIKey != "" {
		apiKeyDisplay = "****"
	}

	fmt.Printf("anthropic.api_key: %s\n", apiKeyDisplay)
	fmt.Printf("defaults.tier: %s\n", cfg.Defaults.Tier)
	fmt.Printf("defaults.token_budget: %d\n", cfg.Defaults.TokenBudget)
	fmt.Printf("tui.refresh_rate: %s\n", cfg.TUI.RefreshRate)
	fmt.Printf("timeouts.scout: %s\n", cfg.Timeouts.Scout)
	fmt.Printf("timeouts.builder: %s\n", cfg.Timeouts.Builder)
	fmt.Printf("timeouts.architect: %s\n", cfg.Timeouts.Architect)
	fmt.Printf("quality_gates.test: %t\n", cfg.QualityGates.Test)
	fmt.Printf("quality_gates.build: %t\n", cfg.QualityGates.Build)
	fmt.Printf("quality_gates.lint: %t\n", cfg.QualityGates.Lint)
	fmt.Printf("quality_gates.typecheck: %t\n", cfg.QualityGates.Typecheck)
}

// displayConfigKey prints a single configuration value.
func displayConfigKey(cfg *config.Config, key string) {
	value, err := getConfigValue(cfg, key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(value)
}

// setConfigKey sets a configuration value and saves the config.
func setConfigKey(cfg *config.Config, key, value string) {
	if err := setConfigValue(cfg, key, value); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Set %s = %s\n", key, value)
}

// getConfigValue retrieves a configuration value by dot-notation key.
func getConfigValue(cfg *config.Config, key string) (string, error) {
	switch strings.ToLower(key) {
	case "anthropic.api_key":
		if cfg.Anthropic.APIKey == "" {
			return "(not set)", nil
		}
		return "****", nil
	case "defaults.tier":
		return cfg.Defaults.Tier, nil
	case "defaults.token_budget":
		return strconv.Itoa(cfg.Defaults.TokenBudget), nil
	case "tui.refresh_rate":
		return cfg.TUI.RefreshRate.String(), nil
	case "timeouts.scout":
		return cfg.Timeouts.Scout.String(), nil
	case "timeouts.builder":
		return cfg.Timeouts.Builder.String(), nil
	case "timeouts.architect":
		return cfg.Timeouts.Architect.String(), nil
	case "quality_gates.test":
		return strconv.FormatBool(cfg.QualityGates.Test), nil
	case "quality_gates.build":
		return strconv.FormatBool(cfg.QualityGates.Build), nil
	case "quality_gates.lint":
		return strconv.FormatBool(cfg.QualityGates.Lint), nil
	case "quality_gates.typecheck":
		return strconv.FormatBool(cfg.QualityGates.Typecheck), nil
	default:
		return "", fmt.Errorf("unknown configuration key: %s", key)
	}
}

// setConfigValue sets a configuration value by dot-notation key.
func setConfigValue(cfg *config.Config, key, value string) error {
	switch strings.ToLower(key) {
	case "anthropic.api_key":
		cfg.Anthropic.APIKey = value
	case "defaults.tier":
		cfg.Defaults.Tier = value
	case "defaults.token_budget":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid value for token_budget: %w", err)
		}
		cfg.Defaults.TokenBudget = n
	case "tui.refresh_rate":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration for refresh_rate: %w", err)
		}
		cfg.TUI.RefreshRate = d
	case "timeouts.scout":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration for timeouts.scout: %w", err)
		}
		cfg.Timeouts.Scout = d
	case "timeouts.builder":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration for timeouts.builder: %w", err)
		}
		cfg.Timeouts.Builder = d
	case "timeouts.architect":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration for timeouts.architect: %w", err)
		}
		cfg.Timeouts.Architect = d
	case "quality_gates.test":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean for quality_gates.test: %w", err)
		}
		cfg.QualityGates.Test = b
	case "quality_gates.build":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean for quality_gates.build: %w", err)
		}
		cfg.QualityGates.Build = b
	case "quality_gates.lint":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean for quality_gates.lint: %w", err)
		}
		cfg.QualityGates.Lint = b
	case "quality_gates.typecheck":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean for quality_gates.typecheck: %w", err)
		}
		cfg.QualityGates.Typecheck = b
	default:
		return fmt.Errorf("unknown configuration key: %s", key)
	}
	return nil
}
