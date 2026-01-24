package validation

import (
	"context"
	"testing"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
)

// TestValidatorAdapter tests the adapter integration between agent and validation packages.
func TestValidatorAdapter(t *testing.T) {
	// Create a mock validator that always passes
	mockValidator := &mockValidator{
		shouldPass: true,
		summary:    "All layers passed",
	}

	adapter := NewValidatorAdapter(mockValidator)

	// Create validation input
	input := agent.TaskValidationInput{
		RepoPath:             "/tmp/test-repo",
		TaskTitle:            "Test task",
		TaskDescription:      "Test description",
		VerificationContract: nil,
		Implementation:       "// test code",
		ModifiedFiles:        []string{"test.go"},
		AcceptanceCriteria:   []string{"Must compile", "Must pass tests"},
	}

	// Run validation
	result, err := adapter.Validate(context.Background(), input)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Verify result
	if !result.AllPassed {
		t.Errorf("Expected validation to pass, got: %v", result.FailureReason)
	}

	if result.Summary != "All layers passed" {
		t.Errorf("Expected summary 'All layers passed', got: %s", result.Summary)
	}
}

// TestValidatorAdapterFailure tests adapter behavior when validation fails.
func TestValidatorAdapterFailure(t *testing.T) {
	// Create a mock validator that always fails
	mockValidator := &mockValidator{
		shouldPass:    false,
		summary:       "Build failed",
		failureReason: "Compilation errors",
	}

	adapter := NewValidatorAdapter(mockValidator)

	input := agent.TaskValidationInput{
		RepoPath:    "/tmp/test-repo",
		TaskTitle:   "Failing task",
		TaskDescription: "Will fail validation",
		Implementation: "invalid code",
	}

	result, err := adapter.Validate(context.Background(), input)
	if err != nil {
		t.Fatalf("Validation returned error: %v", err)
	}

	// Verify failure result
	if result.AllPassed {
		t.Error("Expected validation to fail, but it passed")
	}

	if result.FailureReason != "Compilation errors" {
		t.Errorf("Expected failure reason 'Compilation errors', got: %s", result.FailureReason)
	}

	if result.Summary != "Build failed" {
		t.Errorf("Expected summary 'Build failed', got: %s", result.Summary)
	}
}

// mockValidator is a test double for the Validator.
type mockValidator struct {
	shouldPass    bool
	summary       string
	failureReason string
}

// Validate implements a mock validation that returns predetermined results.
func (m *mockValidator) Validate(ctx context.Context, input ValidationInput) (*ValidationResult, error) {
	return &ValidationResult{
		AllPassed: m.shouldPass,
		Layers: ValidationLayers{
			Layer1: &LayerResult{Name: "Layer 1", Passed: m.shouldPass, Duration: 100 * time.Millisecond},
			Layer2: &LayerResult{Name: "Layer 2", Passed: m.shouldPass, Duration: 200 * time.Millisecond},
			Layer3: &LayerResult{Name: "Layer 3", Passed: m.shouldPass, Duration: 150 * time.Millisecond},
			Layer4: &LayerResult{Name: "Layer 4", Passed: m.shouldPass, Duration: 180 * time.Millisecond},
		},
		Summary:       m.summary,
		FailureReason: m.failureReason,
		Duration:      630 * time.Millisecond,
	}, nil
}
