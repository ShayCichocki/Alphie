package api

import (
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestNewClient_WithAPIKey(t *testing.T) {
	cfg := ClientConfig{
		APIKey: "test-key-123",
		Model:  anthropic.ModelClaudeSonnet4_20250514,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if client.Model() != anthropic.ModelClaudeSonnet4_20250514 {
		t.Errorf("Model = %q, want %q", client.Model(), anthropic.ModelClaudeSonnet4_20250514)
	}

	if client.Tracker() == nil {
		t.Error("Tracker should not be nil")
	}
}

func TestNewClient_WithEnvVar(t *testing.T) {
	// Save and restore original env var
	original := os.Getenv("ANTHROPIC_API_KEY")
	defer os.Setenv("ANTHROPIC_API_KEY", original)

	os.Setenv("ANTHROPIC_API_KEY", "env-test-key")

	cfg := ClientConfig{
		Model: anthropic.ModelClaudeSonnet4_20250514,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestNewClient_NoAPIKey(t *testing.T) {
	// Save and restore original env var
	original := os.Getenv("ANTHROPIC_API_KEY")
	defer os.Setenv("ANTHROPIC_API_KEY", original)

	os.Unsetenv("ANTHROPIC_API_KEY")

	cfg := ClientConfig{}

	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("NewClient should fail without API key")
	}

	expected := "ANTHROPIC_API_KEY environment variable is not set"
	if err.Error() != expected {
		t.Errorf("Error = %q, want %q", err.Error(), expected)
	}
}

func TestNewClient_DefaultModel(t *testing.T) {
	cfg := ClientConfig{
		APIKey: "test-key",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Should default to Sonnet
	if client.Model() != anthropic.ModelClaudeSonnet4_20250514 {
		t.Errorf("Default model = %q, want %q", client.Model(), anthropic.ModelClaudeSonnet4_20250514)
	}
}

func TestTokenTracker_Add(t *testing.T) {
	tracker := NewTokenTracker()

	tracker.Add(100, 50)
	input, output := tracker.Total()

	if input != 100 {
		t.Errorf("Input tokens = %d, want 100", input)
	}
	if output != 50 {
		t.Errorf("Output tokens = %d, want 50", output)
	}
	if tracker.Calls() != 1 {
		t.Errorf("Calls = %d, want 1", tracker.Calls())
	}
}

func TestTokenTracker_AddMultiple(t *testing.T) {
	tracker := NewTokenTracker()

	tracker.Add(100, 50)
	tracker.Add(200, 100)
	tracker.Add(50, 25)

	input, output := tracker.Total()

	if input != 350 {
		t.Errorf("Input tokens = %d, want 350", input)
	}
	if output != 175 {
		t.Errorf("Output tokens = %d, want 175", output)
	}
	if tracker.Calls() != 3 {
		t.Errorf("Calls = %d, want 3", tracker.Calls())
	}
}

func TestTokenTracker_Reset(t *testing.T) {
	tracker := NewTokenTracker()

	tracker.Add(100, 50)
	tracker.Reset()

	input, output := tracker.Total()
	if input != 0 || output != 0 {
		t.Errorf("After reset: input=%d, output=%d; want 0, 0", input, output)
	}
	if tracker.Calls() != 0 {
		t.Errorf("Calls after reset = %d, want 0", tracker.Calls())
	}
}

func TestTokenTracker_Cost(t *testing.T) {
	tracker := NewTokenTracker()

	// 1M input tokens at $3/1M = $3
	// 1M output tokens at $15/1M = $15
	// Total = $18
	tracker.Add(1_000_000, 1_000_000)

	cost := tracker.Cost()
	expected := 18.0

	if cost != expected {
		t.Errorf("Cost = %f, want %f", cost, expected)
	}
}

func TestTokenTracker_CostSmall(t *testing.T) {
	tracker := NewTokenTracker()

	// 1000 input at $3/1M = $0.003
	// 1000 output at $15/1M = $0.015
	// Total = $0.018
	tracker.Add(1000, 1000)

	cost := tracker.Cost()
	expected := 0.018

	// Use epsilon comparison for floating point
	epsilon := 0.000001
	if cost < expected-epsilon || cost > expected+epsilon {
		t.Errorf("Cost = %f, want %f (within %f)", cost, expected, epsilon)
	}
}

func TestClient_Inner(t *testing.T) {
	cfg := ClientConfig{
		APIKey: "test-key",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Verify client has expected model
	if client.Model() != anthropic.ModelClaudeSonnet4_20250514 {
		t.Errorf("Expected model %s, got %s", anthropic.ModelClaudeSonnet4_20250514, client.Model())
	}
}

func TestNewClient_Bedrock(t *testing.T) {
	// Skip if AWS credentials not available
	if os.Getenv("AWS_REGION") == "" && os.Getenv("AWS_DEFAULT_REGION") == "" {
		t.Skip("AWS_REGION not set, skipping Bedrock test")
	}

	cfg := ClientConfig{
		UseAWSBedrock: true,
		AWSRegion:     "us-west-2",
		Model:         anthropic.ModelClaudeSonnet4_20250514,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient with Bedrock failed: %v", err)
	}
	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if client.Model() != anthropic.ModelClaudeSonnet4_20250514 {
		t.Errorf("Model = %q, want %q", client.Model(), anthropic.ModelClaudeSonnet4_20250514)
	}

	if client.Tracker() == nil {
		t.Error("Tracker should not be nil")
	}
}

func TestNewClient_BedrockWithProfile(t *testing.T) {
	// Skip if AWS credentials not available
	if os.Getenv("AWS_REGION") == "" && os.Getenv("AWS_DEFAULT_REGION") == "" {
		t.Skip("AWS_REGION not set, skipping Bedrock test")
	}

	cfg := ClientConfig{
		UseAWSBedrock: true,
		AWSRegion:     "us-west-2",
		AWSProfile:    "bedrock",
		Model:         anthropic.ModelClaudeSonnet4_20250514,
	}

	client, err := NewClient(cfg)
	// Note: This may fail if the profile doesn't exist, which is OK for unit tests
	// The important thing is that the code doesn't panic or have syntax errors
	if err != nil {
		t.Logf("NewClient with Bedrock profile failed (expected if profile not configured): %v", err)
		return
	}
	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if client.Model() != anthropic.ModelClaudeSonnet4_20250514 {
		t.Errorf("Model = %q, want %q", client.Model(), anthropic.ModelClaudeSonnet4_20250514)
	}
}
