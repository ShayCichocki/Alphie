// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/verification"
)

// ClaudePromptRunner implements verification.PromptRunner using ClaudeRunner.
type ClaudePromptRunner struct {
	// factory creates ClaudeRunner instances.
	// If nil, falls back to creating ClaudeProcess (legacy).
	factory ClaudeRunnerFactory
}

// NewClaudePromptRunner creates a new prompt runner that uses Claude.
func NewClaudePromptRunner() *ClaudePromptRunner {
	return &ClaudePromptRunner{}
}

// NewClaudePromptRunnerWithFactory creates a prompt runner with a specific factory.
func NewClaudePromptRunnerWithFactory(factory ClaudeRunnerFactory) *ClaudePromptRunner {
	return &ClaudePromptRunner{factory: factory}
}

// RunPrompt runs a prompt using Claude and returns the response.
func (r *ClaudePromptRunner) RunPrompt(ctx context.Context, prompt string, workDir string) (string, error) {
	// Create a new Claude runner via factory (required)
	if r.factory == nil {
		return "", fmt.Errorf("ClaudePromptRunner: factory is required - use NewClaudePromptRunnerWithFactory")
	}
	claude := r.factory.NewRunner()

	// Start with Sonnet for verification generation (fast and capable)
	opts := &StartOptions{Model: "claude-sonnet-4-20250514"}
	if err := claude.StartWithOptions(prompt, workDir, opts); err != nil {
		return "", fmt.Errorf("start claude process: %w", err)
	}

	// Collect the response
	var response strings.Builder
	for event := range claude.Output() {
		select {
		case <-ctx.Done():
			_ = claude.Kill()
			return "", ctx.Err()
		default:
		}

		switch event.Type {
		case StreamEventResult:
			response.WriteString(event.Message)
		case StreamEventAssistant:
			response.WriteString(event.Message)
		case StreamEventError:
			return "", fmt.Errorf("claude error: %s", event.Error)
		}
	}

	// Wait for runner to complete
	if err := claude.Wait(); err != nil {
		return "", fmt.Errorf("wait for claude: %w", err)
	}

	return response.String(), nil
}

// Ensure ClaudePromptRunner implements PromptRunner
var _ verification.PromptRunner = (*ClaudePromptRunner)(nil)
