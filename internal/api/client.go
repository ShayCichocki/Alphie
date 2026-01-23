// Package api provides direct Anthropic API integration for Alphie agents.
package api

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/aws/aws-sdk-go-v2/config"
)

// Client wraps the Anthropic SDK client with token tracking.
type Client struct {
	inner   anthropic.Client
	model   anthropic.Model
	tracker *TokenTracker
}

// ClientConfig contains configuration for creating a new Client.
type ClientConfig struct {
	// Model is the Claude model to use (e.g., anthropic.ModelClaudeSonnet4_20250514).
	Model anthropic.Model
	// APIKey is the Anthropic API key. If empty, uses ANTHROPIC_API_KEY env var.
	APIKey string
	// UseAWSBedrock indicates whether to use AWS Bedrock instead of direct API.
	UseAWSBedrock bool
	// AWSRegion is the AWS region for Bedrock (e.g., "us-west-2").
	AWSRegion string
	// AWSProfile is the optional AWS profile name to use.
	AWSProfile string
}

// NewClient creates a new Anthropic API client.
func NewClient(cfg ClientConfig) (*Client, error) {
	var opts []option.RequestOption

	if cfg.UseAWSBedrock {
		// AWS Bedrock path
		ctx := context.Background()

		var loadOpts []func(*config.LoadOptions) error
		if cfg.AWSRegion != "" {
			loadOpts = append(loadOpts, config.WithRegion(cfg.AWSRegion))
		}
		if cfg.AWSProfile != "" {
			loadOpts = append(loadOpts, config.WithSharedConfigProfile(cfg.AWSProfile))
		}

		opts = append(opts, bedrock.WithLoadDefaultConfig(ctx, loadOpts...))
	} else {
		// Traditional API key path
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set")
		}
		opts = append(opts, option.WithAPIKey(apiKey))
	}

	inner := anthropic.NewClient(opts...)

	model := cfg.Model
	if model == "" {
		model = anthropic.ModelClaudeSonnet4_20250514
	}

	// Translate model name for Bedrock
	if cfg.UseAWSBedrock {
		model = translateModelForBedrock(model)
	}

	return &Client{
		inner:   inner,
		model:   model,
		tracker: NewTokenTracker(),
	}, nil
}

// translateModelForBedrock converts standard Anthropic model names to Bedrock inference profile format.
// Bedrock uses cross-region inference profiles: us.anthropic.{model}-v1:0
func translateModelForBedrock(model anthropic.Model) anthropic.Model {
	// Map common model names to Bedrock inference profiles (with us. prefix for cross-region)
	bedrockModels := map[anthropic.Model]string{
		anthropic.ModelClaudeSonnet4_20250514:    "us.anthropic.claude-sonnet-4-20250514-v1:0",
		anthropic.ModelClaudeSonnet4_5_20250929:  "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
		anthropic.ModelClaudeHaiku4_5_20251001:   "us.anthropic.claude-haiku-4-5-20251001-v1:0",
		anthropic.ModelClaudeOpus4_1_20250805:    "us.anthropic.claude-opus-4-1-20250805-v1:0",
		anthropic.ModelClaudeOpus4_5_20251101:    "us.anthropic.claude-opus-4-5-20251101-v1:0",
		anthropic.ModelClaude3_7Sonnet20250219:   "us.anthropic.claude-3-7-sonnet-20250219-v1:0",
		anthropic.ModelClaude3_5Haiku20241022:    "us.anthropic.claude-3-5-haiku-20241022-v1:0",
	}

	if bedrockModel, ok := bedrockModels[model]; ok {
		return anthropic.Model(bedrockModel)
	}

	// If not in map, return as-is (might already be Bedrock format or a custom model)
	return model
}

// sdk returns the underlying Anthropic client for internal API access.
// This is package-private to prevent implementation leakage.
func (c *Client) sdk() *anthropic.Client {
	return &c.inner
}

// Model returns the configured model name.
func (c *Client) Model() anthropic.Model {
	return c.model
}

// Tracker returns the token tracker for this client.
func (c *Client) Tracker() *TokenTracker {
	return c.tracker
}

// TranslateModel translates a model name for Bedrock if needed.
// This is used when model names are provided dynamically (e.g., via StartOptions).
func (c *Client) TranslateModel(model anthropic.Model) anthropic.Model {
	// Only translate if this client is using Bedrock
	// Check if our configured model starts with "us.anthropic" (Bedrock format)
	if strings.HasPrefix(string(c.model), "us.anthropic") {
		return translateModelForBedrock(model)
	}
	return model
}

// TokenTracker tracks token usage across API calls.
type TokenTracker struct {
	mu        sync.Mutex
	inputTok  int64
	outputTok int64
	calls     int
}

// NewTokenTracker creates a new token tracker.
func NewTokenTracker() *TokenTracker {
	return &TokenTracker{}
}

// Add records token usage from an API call.
func (t *TokenTracker) Add(input, output int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inputTok += input
	t.outputTok += output
	t.calls++
}

// Total returns the total input and output tokens tracked.
func (t *TokenTracker) Total() (input, output int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inputTok, t.outputTok
}

// Calls returns the number of API calls made.
func (t *TokenTracker) Calls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls
}

// Reset clears all tracked token usage.
func (t *TokenTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inputTok = 0
	t.outputTok = 0
	t.calls = 0
}

// Cost estimates the cost in USD based on current Claude pricing.
// This uses approximate pricing and should be updated as pricing changes.
func (t *TokenTracker) Cost() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Sonnet pricing: $3/1M input, $15/1M output (approximate)
	inputCost := float64(t.inputTok) / 1_000_000 * 3.0
	outputCost := float64(t.outputTok) / 1_000_000 * 15.0
	return inputCost + outputCost
}
