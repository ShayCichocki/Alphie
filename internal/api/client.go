// Package api provides direct Anthropic API integration for Alphie agents.
package api

import (
	"fmt"
	"os"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
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
}

// NewClient creates a new Anthropic API client.
func NewClient(cfg ClientConfig) (*Client, error) {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set")
	}

	inner := anthropic.NewClient(option.WithAPIKey(apiKey))

	model := cfg.Model
	if model == "" {
		model = anthropic.ModelClaudeSonnet4_20250514
	}

	return &Client{
		inner:   inner,
		model:   model,
		tracker: NewTokenTracker(),
	}, nil
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
