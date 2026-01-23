// Package config provides API key management utilities.
package config

import (
	"errors"
	"os"
	"strings"
)

// ErrNoAPIKey is returned when no API key is configured.
var ErrNoAPIKey = errors.New("no Anthropic API key configured")

// GetAPIKey returns the Anthropic API key from the configuration.
// It checks in order: environment variable, config file.
func GetAPIKey(cfg *Config) (string, error) {
	// First check environment variable directly
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key, nil
	}

	// Then check config
	if cfg != nil && cfg.Anthropic.APIKey != "" {
		// Expand any remaining env var references
		key := os.ExpandEnv(cfg.Anthropic.APIKey)
		if key != "" && !strings.HasPrefix(key, "${") {
			return key, nil
		}
	}

	return "", ErrNoAPIKey
}

// ValidateAPIKey performs basic validation on an API key.
// It checks format but does not verify the key with Anthropic's API.
func ValidateAPIKey(key string) error {
	if key == "" {
		return ErrNoAPIKey
	}

	// Anthropic API keys start with "sk-ant-"
	if !strings.HasPrefix(key, "sk-ant-") {
		return errors.New("invalid API key format: expected 'sk-ant-' prefix")
	}

	// Keys should be reasonably long
	if len(key) < 20 {
		return errors.New("invalid API key format: key too short")
	}

	return nil
}

// MaskAPIKey returns a masked version of the API key for display.
// Shows the first 7 characters (sk-ant-) and last 4 characters.
func MaskAPIKey(key string) string {
	if key == "" {
		return "(not set)"
	}

	if len(key) <= 15 {
		return "***"
	}

	return key[:7] + "..." + key[len(key)-4:]
}

// KeySource represents where an API key was loaded from.
type KeySource string

const (
	KeySourceEnv    KeySource = "environment"
	KeySourceConfig KeySource = "config_file"
	KeySourceNone   KeySource = "none"
)

// GetAPIKeySource returns where the API key was sourced from.
func GetAPIKeySource(cfg *Config) KeySource {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return KeySourceEnv
	}

	if cfg != nil && cfg.Anthropic.APIKey != "" {
		key := os.ExpandEnv(cfg.Anthropic.APIKey)
		if key != "" && !strings.HasPrefix(key, "${") {
			return KeySourceConfig
		}
	}

	return KeySourceNone
}
