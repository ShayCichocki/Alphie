// Package config handles configuration loading and management for Alphie.
// It supports project-level .alphie.yaml and environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all configuration for Alphie.
// Simplified to only essential settings for spec-driven orchestration.
type Config struct {
	// Anthropic holds Anthropic API settings
	Anthropic AnthropicConfig `mapstructure:"anthropic"`
	// AWS holds AWS settings for Bedrock
	AWS AWSConfig `mapstructure:"aws"`
	// Execution holds execution settings
	Execution ExecutionConfig `mapstructure:"execution"`
	// Branch holds branching strategy
	Branch BranchConfig `mapstructure:"branch"`
}

// AnthropicConfig holds Anthropic API settings.
type AnthropicConfig struct {
	// APIKey is the Anthropic API key (or use ANTHROPIC_API_KEY env var)
	APIKey string `mapstructure:"api_key"`
	// Backend is "api" (default) or "bedrock"
	Backend string `mapstructure:"backend"`
}

// AWSConfig holds AWS settings for Bedrock.
type AWSConfig struct {
	// Region is the AWS region (default: us-east-1)
	Region string `mapstructure:"region"`
	// AccessKeyID from AWS_ACCESS_KEY_ID env var
	// SecretAccessKey from AWS_SECRET_ACCESS_KEY env var
}

// ExecutionConfig holds execution settings.
type ExecutionConfig struct {
	// Model is the model to use: sonnet (default), haiku, opus
	Model string `mapstructure:"model"`
	// MaxAgents is the maximum number of concurrent agents (default: 3)
	MaxAgents int `mapstructure:"max_agents"`
	// MaxRetries is the maximum retries per task (default: 3)
	MaxRetries int `mapstructure:"max_retries"`
}

// BranchConfig holds branching strategy.
type BranchConfig struct {
	// Greenfield when true, merges directly to main (no session branch)
	Greenfield bool `mapstructure:"greenfield"`
}

// Load loads configuration from .alphie.yaml in the given directory.
// Environment variables override config file values.
func Load(repoPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("anthropic.backend", "api")
	v.SetDefault("aws.region", "us-east-1")
	v.SetDefault("execution.model", "sonnet")
	v.SetDefault("execution.max_agents", 3)
	v.SetDefault("execution.max_retries", 3)
	v.SetDefault("branch.greenfield", false)

	// Look for .alphie.yaml in repo directory
	configPath := filepath.Join(repoPath, ".alphie.yaml")
	if _, err := os.Stat(configPath); err == nil {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
	}

	// Environment variables override config file
	// ANTHROPIC_API_KEY
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		v.Set("anthropic.api_key", apiKey)
	}
	// AWS_REGION
	if region := os.Getenv("AWS_REGION"); region != "" {
		v.Set("aws.region", region)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

// GetAPIKey returns the Anthropic API key from config or environment.
func (c *Config) GetAPIKey() string {
	if c.Anthropic.APIKey != "" {
		return c.Anthropic.APIKey
	}
	return os.Getenv("ANTHROPIC_API_KEY")
}

// GetAWSRegion returns the AWS region from config or environment.
func (c *Config) GetAWSRegion() string {
	if c.AWS.Region != "" {
		return c.AWS.Region
	}
	if region := os.Getenv("AWS_REGION"); region != "" {
		return region
	}
	return "us-east-1"
}

// GetModel returns the configured model, defaulting to "sonnet".
func (c *Config) GetModel() string {
	if c.Execution.Model != "" {
		return c.Execution.Model
	}
	return "sonnet"
}

// GetMaxAgents returns the configured max agents, defaulting to 3.
func (c *Config) GetMaxAgents() int {
	if c.Execution.MaxAgents > 0 {
		return c.Execution.MaxAgents
	}
	return 3
}

// GetMaxRetries returns the configured max retries, defaulting to 3.
func (c *Config) GetMaxRetries() int {
	if c.Execution.MaxRetries > 0 {
		return c.Execution.MaxRetries
	}
	return 3
}

// IsGreenfield returns whether greenfield mode is enabled.
func (c *Config) IsGreenfield() bool {
	return c.Branch.Greenfield
}
