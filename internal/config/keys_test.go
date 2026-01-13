package config

import (
	"os"
	"testing"
)

func TestGetAPIKey(t *testing.T) {
	// Clear any existing env var
	originalKey := os.Getenv("ANTHROPIC_API_KEY")
	defer os.Setenv("ANTHROPIC_API_KEY", originalKey)

	t.Run("from environment variable", func(t *testing.T) {
		os.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

		cfg := &Config{}
		key, err := GetAPIKey(cfg)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if key != "sk-ant-test-key" {
			t.Errorf("expected 'sk-ant-test-key', got %q", key)
		}

		os.Unsetenv("ANTHROPIC_API_KEY")
	})

	t.Run("from config", func(t *testing.T) {
		os.Unsetenv("ANTHROPIC_API_KEY")

		cfg := &Config{
			Anthropic: AnthropicConfig{
				APIKey: "sk-ant-config-key",
			},
		}
		key, err := GetAPIKey(cfg)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if key != "sk-ant-config-key" {
			t.Errorf("expected 'sk-ant-config-key', got %q", key)
		}
	})

	t.Run("no key configured", func(t *testing.T) {
		os.Unsetenv("ANTHROPIC_API_KEY")

		cfg := &Config{}
		_, err := GetAPIKey(cfg)
		if err != ErrNoAPIKey {
			t.Errorf("expected ErrNoAPIKey, got %v", err)
		}
	})
}

func TestValidateAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key", "sk-ant-abcdefghijklmnopqrstuvwxyz", false},
		{"empty key", "", true},
		{"wrong prefix", "sk-openai-12345678901234567890", true},
		{"too short", "sk-ant-abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAPIKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAPIKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"valid key", "sk-ant-abcdefghijklmnopqrstuvwxyz", "sk-ant-...wxyz"},
		{"empty key", "", "(not set)"},
		{"short key", "short", "***"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskAPIKey(tt.key)
			if result != tt.expected {
				t.Errorf("MaskAPIKey() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetAPIKeySource(t *testing.T) {
	// Clear any existing env var
	originalKey := os.Getenv("ANTHROPIC_API_KEY")
	defer os.Setenv("ANTHROPIC_API_KEY", originalKey)

	t.Run("from environment", func(t *testing.T) {
		os.Setenv("ANTHROPIC_API_KEY", "test-key")
		defer os.Unsetenv("ANTHROPIC_API_KEY")

		source := GetAPIKeySource(&Config{})
		if source != KeySourceEnv {
			t.Errorf("expected KeySourceEnv, got %v", source)
		}
	})

	t.Run("from config", func(t *testing.T) {
		os.Unsetenv("ANTHROPIC_API_KEY")

		cfg := &Config{
			Anthropic: AnthropicConfig{
				APIKey: "sk-ant-config-key",
			},
		}
		source := GetAPIKeySource(cfg)
		if source != KeySourceConfig {
			t.Errorf("expected KeySourceConfig, got %v", source)
		}
	})

	t.Run("no key", func(t *testing.T) {
		os.Unsetenv("ANTHROPIC_API_KEY")

		source := GetAPIKeySource(&Config{})
		if source != KeySourceNone {
			t.Errorf("expected KeySourceNone, got %v", source)
		}
	})
}
