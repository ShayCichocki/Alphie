package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/api"
	"github.com/ShayCichocki/alphie/internal/config"
)

// ProcessRunnerFactory creates subprocess-based ClaudeProcess runners.
type ProcessRunnerFactory struct{}

// NewRunner creates a new ClaudeProcess instance.
func (f *ProcessRunnerFactory) NewRunner() agent.ClaudeRunner {
	return agent.NewClaudeProcess(context.Background())
}

// createRunnerFactory creates a ClaudeRunnerFactory for Claude execution.
// If useCLI is true, uses subprocess (claude CLI). Otherwise uses API.
func createRunnerFactory(useCLI bool) (agent.ClaudeRunnerFactory, error) {
	if useCLI {
		return &ProcessRunnerFactory{}, nil
	}
	return createRunnerFactoryWithModel(anthropic.ModelClaudeSonnet4_20250514)
}

// createRunnerFactoryWithModel creates an API factory with a specific model.
func createRunnerFactoryWithModel(model anthropic.Model) (agent.ClaudeRunnerFactory, error) {
	// Get current working directory for config
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	// Load config to determine backend
	cfg, err := config.Load(cwd)
	if err != nil {
		// Use defaults if config not found
		cfg = &config.Config{}
	}

	clientCfg := api.ClientConfig{
		Model: model,
	}

	// Determine backend
	backend := strings.ToLower(cfg.Anthropic.Backend)
	if backend == "bedrock" {
		clientCfg.UseAWSBedrock = true
		clientCfg.AWSRegion = cfg.GetAWSRegion()
	} else {
		clientCfg.APIKey = cfg.GetAPIKey()
	}

	apiClient, err := api.NewClient(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("create API client: %w", err)
	}

	notifs, err := api.NewNotificationManager(cwd)
	if err != nil {
		notifs = nil // Notifications are optional
	}

	return &agent.APIRunnerFactory{
		Client:        apiClient,
		Notifications: notifs,
	}, nil
}
