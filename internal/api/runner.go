package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// Runner provides simple text-in/text-out Claude API calls.
// This is useful for decomposition, review, and other non-tool tasks.
type Runner struct {
	client *Client
}

// NewRunner creates a new API runner.
func NewRunner(client *Client) *Runner {
	return &Runner{client: client}
}

// Run executes a prompt and returns the text response.
// No tools are provided - this is for simple text completion tasks.
func (r *Runner) Run(ctx context.Context, prompt string) (string, error) {
	resp, err := r.client.sdk().Messages.New(ctx, anthropic.MessageNewParams{
		Model:     r.client.Model(),
		MaxTokens: 8192,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}

	r.client.Tracker().Add(resp.Usage.InputTokens, resp.Usage.OutputTokens)

	var result strings.Builder
	for _, block := range resp.Content {
		if variant, ok := block.AsAny().(anthropic.TextBlock); ok {
			result.WriteString(variant.Text)
		}
	}

	return result.String(), nil
}

// RunWithSystem executes a prompt with a system message.
func (r *Runner) RunWithSystem(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := r.client.sdk().Messages.New(ctx, anthropic.MessageNewParams{
		Model:     r.client.Model(),
		MaxTokens: 8192,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}

	r.client.Tracker().Add(resp.Usage.InputTokens, resp.Usage.OutputTokens)

	var result strings.Builder
	for _, block := range resp.Content {
		if variant, ok := block.AsAny().(anthropic.TextBlock); ok {
			result.WriteString(variant.Text)
		}
	}

	return result.String(), nil
}

// RunJSON executes a prompt and parses the JSON response into the provided target.
func (r *Runner) RunJSON(ctx context.Context, prompt string, target interface{}) error {
	response, err := r.Run(ctx, prompt)
	if err != nil {
		return err
	}

	// Find JSON in the response
	jsonStart := strings.Index(response, "{")
	if jsonStart == -1 {
		jsonStart = strings.Index(response, "[")
	}
	jsonEnd := strings.LastIndex(response, "}")
	if jsonEnd == -1 {
		jsonEnd = strings.LastIndex(response, "]")
	}

	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return fmt.Errorf("no valid JSON found in response: %s", truncate(response, 200))
	}

	jsonStr := response[jsonStart : jsonEnd+1]
	if err := json.Unmarshal([]byte(jsonStr), target); err != nil {
		return fmt.Errorf("parse JSON: %w (response: %s)", err, truncate(jsonStr, 200))
	}

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Decompose breaks a user request into tasks using the API directly.
// This is an API-native replacement for the subprocess-based Decomposer.
func (r *Runner) Decompose(ctx context.Context, request string, decompositionPrompt string) ([]map[string]interface{}, error) {
	prompt := fmt.Sprintf(decompositionPrompt, request)

	var tasks []map[string]interface{}
	if err := r.RunJSON(ctx, prompt, &tasks); err != nil {
		return nil, fmt.Errorf("decompose: %w", err)
	}

	if len(tasks) == 0 {
		return nil, fmt.Errorf("empty task list returned")
	}

	return tasks, nil
}

// Review performs a code review using the API directly.
func (r *Runner) Review(ctx context.Context, diff, taskDescription string) (approved bool, concerns []string, rawOutput string, err error) {
	prompt := fmt.Sprintf(`You are a code reviewer performing a second review of high-risk changes.

TASK DESCRIPTION:
%s

DIFF TO REVIEW:
%s

Please review this diff carefully and provide your assessment.

Your response MUST include:
1. A clear APPROVED or NOT APPROVED verdict on the first line
2. A list of concerns, if any (prefix each with "CONCERN:")
3. Any recommendations for improvement

Focus on:
- Security vulnerabilities
- Breaking changes
- Data integrity issues
- Missing error handling
- Potential performance problems

If you approve, state "APPROVED" on the first line.
If you have concerns that block approval, state "NOT APPROVED" on the first line.`, taskDescription, diff)

	rawOutput, err = r.Run(ctx, prompt)
	if err != nil {
		return false, nil, "", err
	}

	// Parse response
	lines := strings.Split(rawOutput, "\n")

	// Check first non-empty line for approval
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upperLine := strings.ToUpper(line)
		if strings.Contains(upperLine, "APPROVED") && !strings.Contains(upperLine, "NOT APPROVED") {
			approved = true
		}
		break
	}

	// Extract concerns
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "CONCERN:") {
			concern := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "CONCERN:"), "concern:"))
			if concern != "" {
				concerns = append(concerns, concern)
			}
		}
	}

	return approved, concerns, rawOutput, nil
}

// Merge resolves merge conflicts using the API directly.
func (r *Runner) Merge(ctx context.Context, branch1, branch2, diff1, diff2 string, conflictFiles []string) (mergedFiles map[string]string, reasoning string, err error) {
	systemPrompt := `You are a merge conflict resolver. Understand the INTENT of each change, not just the text.

When resolving conflicts:
1. Analyze what each branch is trying to accomplish
2. Preserve the intent of both changes when possible
3. If changes are truly incompatible, favor the change that maintains correctness
4. Ensure the merged result compiles and maintains logical consistency
5. Explain your reasoning before providing the merged code`

	userPrompt := fmt.Sprintf(`Resolve the following merge conflict.

Branch 1 (%s) changes:
%s

Branch 2 (%s) changes:
%s

Conflict files: %s

Return ONLY a JSON object with this exact structure (no other text):
{
  "merged_files": {
    "path/to/file1.go": "full merged file content...",
    "path/to/file2.go": "full merged file content..."
  },
  "reasoning": "Brief explanation of how conflicts were resolved"
}`, branch1, diff1, branch2, diff2, strings.Join(conflictFiles, ", "))

	response, err := r.RunWithSystem(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, "", err
	}

	// Parse JSON response
	var result struct {
		MergedFiles map[string]string `json:"merged_files"`
		Reasoning   string            `json:"reasoning"`
	}

	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, "", fmt.Errorf("no valid JSON found in response")
	}

	if err := json.Unmarshal([]byte(response[jsonStart:jsonEnd+1]), &result); err != nil {
		return nil, "", fmt.Errorf("parse merge response: %w", err)
	}

	return result.MergedFiles, result.Reasoning, nil
}
