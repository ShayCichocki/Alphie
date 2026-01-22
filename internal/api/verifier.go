package api

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// Verifier implements 3-tier verification for agent output.
type Verifier struct {
	client  *Client
	workDir string
}

// VerificationResult contains the result of a verification check.
type VerificationResult struct {
	Passed   bool
	Tier     int    // 0=build, 1=diff, 2=judge
	TierName string // "build", "architecture", "judge"
	Feedback string
}

// NewVerifier creates a new verifier for the given working directory.
func NewVerifier(client *Client, workDir string) *Verifier {
	return &Verifier{client: client, workDir: workDir}
}

// Verify runs the full 3-tier verification pipeline.
func (v *Verifier) Verify(ctx context.Context, archDocs string) (*VerificationResult, error) {
	// Tier 0: Lint/Build/Test
	if result := v.verifyBuild(ctx); !result.Passed {
		return result, nil
	}

	// Tier 1: Diff against architecture docs
	if archDocs != "" {
		if result, err := v.verifyArchitecture(ctx, archDocs); err != nil {
			return nil, err
		} else if !result.Passed {
			return result, nil
		}
	}

	// Tier 2: LLM Judge
	return v.verifyWithJudge(ctx)
}

// VerifyBuildOnly runs only the build verification tier.
func (v *Verifier) VerifyBuildOnly(ctx context.Context) *VerificationResult {
	return v.verifyBuild(ctx)
}

func (v *Verifier) verifyBuild(ctx context.Context) *VerificationResult {
	commands := []struct {
		name string
		cmd  string
	}{
		{"go vet", "go vet ./..."},
		{"go build", "go build ./..."},
		{"go test", "go test -short ./..."},
	}

	for _, c := range commands {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		cmd := exec.CommandContext(ctx, "bash", "-c", c.cmd)
		cmd.Dir = v.workDir
		output, err := cmd.CombinedOutput()
		cancel()

		if err != nil {
			return &VerificationResult{
				Passed:   false,
				Tier:     0,
				TierName: "build",
				Feedback: fmt.Sprintf("%s failed:\n%s", c.name, string(output)),
			}
		}
	}

	return &VerificationResult{Passed: true, Tier: 0, TierName: "build"}
}

func (v *Verifier) verifyArchitecture(ctx context.Context, archDocs string) (*VerificationResult, error) {
	// Get the diff of recent changes
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD~1", "--", ".")
	cmd.Dir = v.workDir
	diff, err := cmd.Output()
	if err != nil {
		// No previous commit or other error - skip this tier
		return &VerificationResult{Passed: true, Tier: 1, TierName: "architecture"}, nil
	}

	if len(diff) == 0 {
		return &VerificationResult{Passed: true, Tier: 1, TierName: "architecture"}, nil
	}

	// Use LLM to compare diff against architecture docs
	prompt := fmt.Sprintf(`You are reviewing code changes against architecture documentation.

## Architecture Documentation
%s

## Code Diff
%s

Check if the changes violate any architectural patterns or constraints documented above.

Respond with EXACTLY one of:
- PASS: Changes are consistent with architecture
- FAIL: [specific violations found]

Be strict but fair. Only flag actual violations, not style preferences.`, archDocs, string(diff))

	resp, err := v.client.sdk().Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_20250514,
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("architecture check failed: %w", err)
	}

	v.client.Tracker().Add(resp.Usage.InputTokens, resp.Usage.OutputTokens)

	response := extractText(resp)

	if strings.HasPrefix(response, "PASS") {
		return &VerificationResult{Passed: true, Tier: 1, TierName: "architecture"}, nil
	}

	return &VerificationResult{
		Passed:   false,
		Tier:     1,
		TierName: "architecture",
		Feedback: response,
	}, nil
}

func (v *Verifier) verifyWithJudge(ctx context.Context) (*VerificationResult, error) {
	// Get the diff for the judge to review
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD~1", "--", ".")
	cmd.Dir = v.workDir
	diff, _ := cmd.Output()

	if len(diff) == 0 {
		return &VerificationResult{Passed: true, Tier: 2, TierName: "judge"}, nil
	}

	// Truncate very large diffs
	diffStr := string(diff)
	if len(diffStr) > 50000 {
		diffStr = diffStr[:50000] + "\n... (diff truncated)"
	}

	judgePrompt := fmt.Sprintf(`You are a Senior Staff Engineer and Principal Architect conducting a rigorous code review.

Your job is to be HYPER-CRITICAL. You are the last line of defense before code ships.
Find issues. Don't rubber-stamp changes.

## Review Criteria (ALL must pass)

1. **Correctness**: Does this actually solve the stated problem? Are there logic errors?
2. **Edge Cases**: What happens with nil, empty, zero, negative, very large values?
3. **Error Handling**: Are errors checked? Are they handled appropriately? Good error messages?
4. **Security**: SQL injection? Command injection? Path traversal? Data exposure?
5. **Concurrency**: Race conditions? Deadlocks? Proper mutex usage?
6. **Performance**: O(n²) loops? Unnecessary allocations? Missing indexes?
7. **Maintainability**: Is this code readable? Will someone understand it in 6 months?
8. **Testing**: Is this testable? Are edge cases covered?

## Response Format

Respond with EXACTLY one of:
- APPROVED: [1-2 sentence summary of why it's acceptable]
- REJECTED: [Numbered list of specific issues that MUST be fixed]

If you find ANY issue that could cause bugs, security problems, or significant maintenance burden, REJECT.

## Diff to Review

%s`, diffStr)

	resp, err := v.client.sdk().Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_20250514,
		MaxTokens: 2048,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(judgePrompt)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("judge review failed: %w", err)
	}

	v.client.Tracker().Add(resp.Usage.InputTokens, resp.Usage.OutputTokens)

	response := extractText(resp)

	if strings.HasPrefix(response, "APPROVED") {
		return &VerificationResult{
			Passed:   true,
			Tier:     2,
			TierName: "judge",
			Feedback: response,
		}, nil
	}

	return &VerificationResult{
		Passed:   false,
		Tier:     2,
		TierName: "judge",
		Feedback: response,
	}, nil
}

func extractText(resp *anthropic.Message) string {
	var result string
	for _, block := range resp.Content {
		if variant, ok := block.AsAny().(anthropic.TextBlock); ok {
			result += variant.Text
		}
	}
	return strings.TrimSpace(result)
}

// CritiqueResult contains the result of a critique iteration.
type CritiqueResult struct {
	Score    int    // 0-100
	Issues   string // Issues found
	Done     bool   // True if LGTM
	Feedback string // Full critique text
}

// Critique runs a single critique iteration on the current code state.
// This is used for ralph-loop style iterative improvement.
func (v *Verifier) Critique(ctx context.Context, taskDescription, previousOutput string) (*CritiqueResult, error) {
	// Get current state
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD", "--", ".")
	cmd.Dir = v.workDir
	diff, _ := cmd.Output()

	diffStr := string(diff)
	if len(diffStr) > 30000 {
		diffStr = diffStr[:30000] + "\n... (truncated)"
	}

	critiquePrompt := fmt.Sprintf(`You are a Senior Staff Engineer reviewing work on this task:

## Task
%s

## Previous Agent Output
%s

## Current Changes
%s

## Your Job

Review the implementation critically. Look for:
1. Does it actually accomplish the task?
2. Are there bugs or edge cases missed?
3. Is the code clean and maintainable?
4. Any security or performance issues?

## Response Format

Score: [0-100]
Issues:
- [Issue 1]
- [Issue 2]
Status: [NEEDS_WORK | LGTM]

If LGTM, the score should be 90+. Be honest but fair.`, taskDescription, previousOutput, diffStr)

	resp, err := v.client.sdk().Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_20250514,
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(critiquePrompt)),
		},
	})
	if err != nil {
		return nil, err
	}

	v.client.Tracker().Add(resp.Usage.InputTokens, resp.Usage.OutputTokens)

	text := extractText(resp)

	result := &CritiqueResult{
		Feedback: text,
		Done:     strings.Contains(text, "LGTM"),
	}

	// Parse score
	if idx := strings.Index(text, "Score:"); idx >= 0 {
		fmt.Sscanf(text[idx:], "Score: %d", &result.Score)
	}

	// Extract issues section
	if idx := strings.Index(text, "Issues:"); idx >= 0 {
		endIdx := strings.Index(text[idx:], "Status:")
		if endIdx > 0 {
			result.Issues = strings.TrimSpace(text[idx+7 : idx+endIdx])
		} else {
			result.Issues = strings.TrimSpace(text[idx+7:])
		}
	}

	return result, nil
}
