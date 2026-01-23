// Package validation provides semantic validation and code review capabilities.
package validation

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/agent"
)

// SemanticValidator performs semantic validation of task implementations.
// It uses Claude to review whether the implementation matches the task intent.
type SemanticValidator struct {
	runnerFactory agent.ClaudeRunnerFactory
}

// NewSemanticValidator creates a new semantic validator.
func NewSemanticValidator(runnerFactory agent.ClaudeRunnerFactory) *SemanticValidator {
	return &SemanticValidator{
		runnerFactory: runnerFactory,
	}
}

// SemanticValidationResult contains the outcome of semantic validation.
type SemanticValidationResult struct {
	// Passed indicates whether the implementation meets the intent.
	Passed bool
	// Reasoning explains why it passed or failed.
	Reasoning string
	// Concerns lists any concerns even if overall passed.
	Concerns []string
	// Suggestions provides improvement suggestions.
	Suggestions []string
}

// Validate performs semantic validation of an implementation against task intent.
func (v *SemanticValidator) Validate(ctx context.Context, input SemanticValidationInput) (*SemanticValidationResult, error) {
	if v.runnerFactory == nil {
		return nil, fmt.Errorf("runner factory not configured")
	}

	// Create a Claude runner for this validation
	runner := v.runnerFactory.NewRunner()

	// Build the validation prompt
	prompt := v.buildValidationPrompt(input)

	// Send to Claude for analysis
	// Note: We're using the runner's internal prompt method
	// In a real implementation, we'd need to properly invoke Claude
	// For now, we'll create a simple interface

	response, err := v.invokeClaudeForValidation(ctx, runner, prompt)
	if err != nil {
		return nil, fmt.Errorf("invoke Claude for validation: %w", err)
	}

	// Parse the response
	result := v.parseValidationResponse(response)
	return result, nil
}

// SemanticValidationInput contains all information needed for semantic validation.
type SemanticValidationInput struct {
	// TaskTitle is the title of the task.
	TaskTitle string
	// TaskDescription is the full task description.
	TaskDescription string
	// TaskIntent is the verification intent (acceptance criteria).
	TaskIntent string
	// Implementation is the code that was written (diff or full content).
	Implementation string
	// ModifiedFiles lists files that were changed.
	ModifiedFiles []string
	// VerificationOutput is output from verification contracts if available.
	VerificationOutput string
}

// buildValidationPrompt constructs the prompt for semantic validation.
func (v *SemanticValidator) buildValidationPrompt(input SemanticValidationInput) string {
	var sb strings.Builder

	sb.WriteString("# Semantic Validation Task\n\n")
	sb.WriteString("You are performing semantic validation of a task implementation. ")
	sb.WriteString("Your job is to determine if the implementation correctly fulfills the task intent.\n\n")

	sb.WriteString("## Task Information\n\n")
	sb.WriteString(fmt.Sprintf("**Title**: %s\n\n", input.TaskTitle))
	sb.WriteString(fmt.Sprintf("**Description**:\n%s\n\n", input.TaskDescription))

	if input.TaskIntent != "" {
		sb.WriteString("**Acceptance Criteria**:\n")
		sb.WriteString(input.TaskIntent)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Implementation\n\n")
	if len(input.ModifiedFiles) > 0 {
		sb.WriteString("**Modified Files**:\n")
		for _, file := range input.ModifiedFiles {
			sb.WriteString(fmt.Sprintf("- %s\n", file))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**Changes**:\n```\n")
	sb.WriteString(input.Implementation)
	sb.WriteString("\n```\n\n")

	if input.VerificationOutput != "" {
		sb.WriteString("**Verification Test Results**:\n```\n")
		sb.WriteString(input.VerificationOutput)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("Analyze the implementation and determine:\n")
	sb.WriteString("1. Does it fulfill the task intent and acceptance criteria?\n")
	sb.WriteString("2. Is the approach sound and appropriate?\n")
	sb.WriteString("3. Are there any concerns or gaps?\n\n")

	sb.WriteString("Respond in the following format:\n\n")
	sb.WriteString("VERDICT: [PASS/FAIL]\n")
	sb.WriteString("REASONING: [Explain your verdict in 2-3 sentences]\n")
	sb.WriteString("CONCERNS: [List any concerns, or 'None']\n")
	sb.WriteString("SUGGESTIONS: [List any improvement suggestions, or 'None']\n\n")

	sb.WriteString("Be strict but fair. PASS only if the implementation truly meets the intent. ")
	sb.WriteString("FAIL if there are significant gaps, wrong approach, or incomplete implementation.\n")

	return sb.String()
}

// invokeClaudeForValidation sends the prompt to Claude and returns the response.
func (v *SemanticValidator) invokeClaudeForValidation(ctx context.Context, runner agent.ClaudeRunner, prompt string) (string, error) {
	// Start Claude with Sonnet model for validation
	opts := &agent.StartOptions{Model: agent.ModelSonnet}
	if err := runner.StartWithOptions(prompt, "/tmp", opts); err != nil {
		return "", fmt.Errorf("start claude for validation: %w", err)
	}

	// Collect the response
	var response strings.Builder
	for event := range runner.Output() {
		select {
		case <-ctx.Done():
			_ = runner.Kill()
			return "", ctx.Err()
		default:
		}

		switch event.Type {
		case agent.StreamEventResult:
			response.WriteString(event.Message)
		case agent.StreamEventAssistant:
			response.WriteString(event.Message)
		case agent.StreamEventError:
			return "", fmt.Errorf("claude validation error: %s", event.Error)
		}
	}

	// Wait for completion
	if err := runner.Wait(); err != nil {
		return "", fmt.Errorf("wait for claude validation: %w", err)
	}

	return response.String(), nil
}

// parseValidationResponse parses Claude's response into a structured result.
func (v *SemanticValidator) parseValidationResponse(response string) *SemanticValidationResult {
	result := &SemanticValidationResult{
		Passed:      false,
		Reasoning:   "",
		Concerns:    []string{},
		Suggestions: []string{},
	}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "VERDICT:") {
			verdict := strings.TrimSpace(strings.TrimPrefix(line, "VERDICT:"))
			result.Passed = strings.ToUpper(verdict) == "PASS"
		} else if strings.HasPrefix(line, "REASONING:") {
			result.Reasoning = strings.TrimSpace(strings.TrimPrefix(line, "REASONING:"))
		} else if strings.HasPrefix(line, "CONCERNS:") {
			concerns := strings.TrimSpace(strings.TrimPrefix(line, "CONCERNS:"))
			if concerns != "" && strings.ToLower(concerns) != "none" {
				result.Concerns = strings.Split(concerns, ";")
			}
		} else if strings.HasPrefix(line, "SUGGESTIONS:") {
			suggestions := strings.TrimSpace(strings.TrimPrefix(line, "SUGGESTIONS:"))
			if suggestions != "" && strings.ToLower(suggestions) != "none" {
				result.Suggestions = strings.Split(suggestions, ";")
			}
		}
	}

	return result
}
