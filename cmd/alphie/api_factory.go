package main

import (
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/api"
)

// createRunnerFactory creates an APIRunnerFactory for Claude execution.
// This always uses direct Anthropic API calls.
func createRunnerFactory() (agent.ClaudeRunnerFactory, error) {
	return createRunnerFactoryWithModel(anthropic.ModelClaudeSonnet4_20250514)
}

// createRunnerFactoryWithModel creates a factory with a specific model.
func createRunnerFactoryWithModel(model anthropic.Model) (agent.ClaudeRunnerFactory, error) {
	apiClient, err := api.NewClient(api.ClientConfig{
		Model: model,
	})
	if err != nil {
		return nil, fmt.Errorf("create API client: %w", err)
	}

	cwd, _ := os.Getwd()
	notifs, err := api.NewNotificationManager(cwd)
	if err != nil {
		notifs = nil // Notifications are optional
	}

	return &agent.APIRunnerFactory{
		Client:        apiClient,
		Notifications: notifs,
	}, nil
}
