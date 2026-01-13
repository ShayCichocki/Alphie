//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shayc/alphie/internal/agent"
)

// TestBaselineCaptureGateComparison tests the full flow of:
// 1. Capturing a baseline
// 2. Running quality gates
// 3. Comparing results to detect regressions
func TestBaselineCaptureGateComparison(t *testing.T) {
	// Create a mock baseline (simulating captured state)
	baseline := &agent.Baseline{
		FailingTests: []string{"TestOldBroken/case1", "TestOldBroken/case2"},
		LintErrors:   []string{"file.go:10: unused variable"},
		TypeErrors:   []string{},
		CapturedAt:   time.Now().Add(-1 * time.Hour),
	}

	// Simulate current gate results - no new failures
	currentGood := &agent.GateResults{
		FailingTests: []string{"TestOldBroken/case1", "TestOldBroken/case2"},
		LintErrors:   []string{"file.go:10: unused variable"},
		TypeErrors:   []string{},
	}

	// Compare - should show no regression
	comparison := agent.CompareToBaseline(currentGood, baseline)
	if comparison.IsRegression {
		t.Errorf("Expected no regression, but got IsRegression=true")
	}
	if len(comparison.NewFailures) != 0 {
		t.Errorf("Expected 0 new failures, got %d", len(comparison.NewFailures))
	}
	if len(comparison.Improved) != 0 {
		t.Errorf("Expected 0 improvements, got %d", len(comparison.Improved))
	}
}

// TestBaselineDetectsNewFailures tests that new test failures are detected as regressions.
func TestBaselineDetectsNewFailures(t *testing.T) {
	baseline := &agent.Baseline{
		FailingTests: []string{"TestOldBroken"},
		LintErrors:   []string{},
		TypeErrors:   []string{},
		CapturedAt:   time.Now().Add(-1 * time.Hour),
	}

	// Current results have a NEW failure
	current := &agent.GateResults{
		FailingTests: []string{"TestOldBroken", "TestNewBroken"}, // Added new failure
		LintErrors:   []string{},
		TypeErrors:   []string{},
	}

	comparison := agent.CompareToBaseline(current, baseline)
	if !comparison.IsRegression {
		t.Errorf("Expected regression due to new failure, but IsRegression=false")
	}
	if len(comparison.NewFailures) != 1 {
		t.Errorf("Expected 1 new failure, got %d", len(comparison.NewFailures))
	}
	if len(comparison.NewFailures) > 0 && comparison.NewFailures[0] != "TestNewBroken" {
		t.Errorf("Expected new failure to be TestNewBroken, got %s", comparison.NewFailures[0])
	}
}

// TestBaselineDetectsImprovements tests that fixing previously broken tests is detected.
func TestBaselineDetectsImprovements(t *testing.T) {
	baseline := &agent.Baseline{
		FailingTests: []string{"TestOldBroken1", "TestOldBroken2"},
		LintErrors:   []string{},
		TypeErrors:   []string{},
		CapturedAt:   time.Now().Add(-1 * time.Hour),
	}

	// Current results - fixed one of the old failures
	current := &agent.GateResults{
		FailingTests: []string{"TestOldBroken1"}, // Fixed TestOldBroken2
		LintErrors:   []string{},
		TypeErrors:   []string{},
	}

	comparison := agent.CompareToBaseline(current, baseline)
	if comparison.IsRegression {
		t.Errorf("Expected no regression, but IsRegression=true")
	}
	if len(comparison.Improved) != 1 {
		t.Errorf("Expected 1 improvement, got %d", len(comparison.Improved))
	}
	if len(comparison.Improved) > 0 && comparison.Improved[0] != "TestOldBroken2" {
		t.Errorf("Expected improvement to be TestOldBroken2, got %s", comparison.Improved[0])
	}
}

// TestBaselineDetectsWorseLints tests that additional lint errors are detected.
func TestBaselineDetectsWorseLints(t *testing.T) {
	baseline := &agent.Baseline{
		FailingTests: []string{},
		LintErrors:   []string{"error1"},
		TypeErrors:   []string{},
		CapturedAt:   time.Now().Add(-1 * time.Hour),
	}

	// Current results have MORE lint errors
	current := &agent.GateResults{
		FailingTests: []string{},
		LintErrors:   []string{"error1", "error2", "error3"}, // 2 new errors
		TypeErrors:   []string{},
	}

	comparison := agent.CompareToBaseline(current, baseline)
	if !comparison.IsRegression {
		t.Errorf("Expected regression due to worse lints, but IsRegression=false")
	}
	if comparison.WorseLints != 2 {
		t.Errorf("Expected WorseLints=2, got %d", comparison.WorseLints)
	}
}

// TestBaselineSaveLoad tests saving and loading baseline from disk.
func TestBaselineSaveLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "alphie-baseline-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	baselinePath := filepath.Join(tmpDir, ".alphie", "baseline.json")

	// Create baseline
	original := &agent.Baseline{
		FailingTests: []string{"TestA", "TestB"},
		LintErrors:   []string{"lint1", "lint2"},
		TypeErrors:   []string{"type1"},
		CapturedAt:   time.Now().Truncate(time.Second), // Truncate for comparison
	}

	// Save
	if err := original.Save(baselinePath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		t.Fatal("Baseline file was not created")
	}

	// Load
	loaded, err := agent.LoadBaseline(baselinePath)
	if err != nil {
		t.Fatalf("LoadBaseline() error = %v", err)
	}

	// Verify contents
	if len(loaded.FailingTests) != 2 {
		t.Errorf("FailingTests length = %d, want 2", len(loaded.FailingTests))
	}
	if len(loaded.LintErrors) != 2 {
		t.Errorf("LintErrors length = %d, want 2", len(loaded.LintErrors))
	}
	if len(loaded.TypeErrors) != 1 {
		t.Errorf("TypeErrors length = %d, want 1", len(loaded.TypeErrors))
	}

	// Verify timestamp (with some tolerance for timezone differences)
	timeDiff := loaded.CapturedAt.Sub(original.CapturedAt)
	if timeDiff < -time.Second || timeDiff > time.Second {
		t.Errorf("CapturedAt difference = %v, want ~0", timeDiff)
	}
}

// TestQualityGatesRunning tests running quality gates on a Go project.
func TestQualityGatesRunning(t *testing.T) {
	// Use the current project directory for testing
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Navigate up to project root (from internal/integration)
	projectRoot := filepath.Join(cwd, "..", "..")

	// Verify go.mod exists
	if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); os.IsNotExist(err) {
		t.Skip("Not in a Go project, skipping quality gates test")
	}

	gates := agent.NewQualityGates(projectRoot)
	gates.EnableBuild(true)
	gates.SetTimeout(2 * time.Minute)

	results, err := gates.RunGates()
	if err != nil {
		t.Fatalf("RunGates() error = %v", err)
	}

	// Should have at least the build result
	if len(results) < 1 {
		t.Errorf("Expected at least 1 gate result, got %d", len(results))
	}

	// Check that we got a build result
	var buildResult *agent.GateOutput
	for _, r := range results {
		if r.Gate == "build" {
			buildResult = r
			break
		}
	}

	if buildResult == nil {
		t.Error("Expected build gate result")
	} else {
		// Build should pass or skip (not error)
		if buildResult.Result == agent.GateError {
			t.Errorf("Build gate errored: %s", buildResult.Output)
		}
	}
}

// TestBaselineNilHandling tests handling of nil baseline/current values.
func TestBaselineNilHandling(t *testing.T) {
	// Nil baseline with non-nil current
	current := &agent.GateResults{
		FailingTests: []string{"TestFail"},
	}
	comparison := agent.CompareToBaseline(current, nil)
	if !comparison.IsRegression {
		t.Error("Expected regression when baseline is nil but current has failures")
	}

	// Nil current with non-nil baseline
	baseline := &agent.Baseline{
		FailingTests: []string{"TestFail"},
	}
	comparison = agent.CompareToBaseline(nil, baseline)
	if comparison.IsRegression {
		t.Error("Expected no regression when current is nil")
	}

	// Both nil
	comparison = agent.CompareToBaseline(nil, nil)
	if comparison.IsRegression {
		t.Error("Expected no regression when both are nil")
	}
}

// TestGateResultString tests the string representation of gate results.
func TestGateResultString(t *testing.T) {
	tests := []struct {
		result agent.GateResult
		want   string
	}{
		{agent.GatePass, "pass"},
		{agent.GateFail, "fail"},
		{agent.GateSkip, "skip"},
		{agent.GateError, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.result.String(); got != tt.want {
				t.Errorf("String() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestBaselineWithTypeErrors tests handling of type/compilation errors.
func TestBaselineWithTypeErrors(t *testing.T) {
	baseline := &agent.Baseline{
		FailingTests: []string{},
		LintErrors:   []string{},
		TypeErrors:   []string{"undefined: foo"},
		CapturedAt:   time.Now(),
	}

	// Current has same type error - no regression
	current1 := &agent.GateResults{
		FailingTests: []string{},
		LintErrors:   []string{},
		TypeErrors:   []string{"undefined: foo"},
	}
	comparison := agent.CompareToBaseline(current1, baseline)
	if comparison.IsRegression {
		t.Error("Same type errors should not be a regression")
	}

	// Current has NEW type error - regression
	current2 := &agent.GateResults{
		FailingTests: []string{},
		LintErrors:   []string{},
		TypeErrors:   []string{"undefined: foo", "undefined: bar"},
	}
	comparison = agent.CompareToBaseline(current2, baseline)
	if !comparison.IsRegression {
		t.Error("New type error should be a regression")
	}
	if len(comparison.NewFailures) != 1 {
		t.Errorf("Expected 1 new failure, got %d", len(comparison.NewFailures))
	}
}

// TestBaselineComplexScenario tests a complex scenario with mixed improvements and regressions.
func TestBaselineComplexScenario(t *testing.T) {
	baseline := &agent.Baseline{
		FailingTests: []string{"TestA", "TestB", "TestC"},
		LintErrors:   []string{"lint1", "lint2"},
		TypeErrors:   []string{"type1"},
		CapturedAt:   time.Now(),
	}

	// Current: Fixed TestA, but broke TestD
	// Improved lints (fewer), but added type error
	current := &agent.GateResults{
		FailingTests: []string{"TestB", "TestC", "TestD"}, // Fixed A, broke D
		LintErrors:   []string{"lint1"},                   // Fixed lint2
		TypeErrors:   []string{"type1", "type2"},          // Added type2
	}

	comparison := agent.CompareToBaseline(current, baseline)

	// Should be a regression due to new failures
	if !comparison.IsRegression {
		t.Error("Expected regression due to new failures")
	}

	// Should have 2 new failures (TestD and type2)
	if len(comparison.NewFailures) != 2 {
		t.Errorf("Expected 2 new failures, got %d", len(comparison.NewFailures))
	}

	// Should have 2 improvements (TestA and lint2)
	if len(comparison.Improved) != 2 {
		t.Errorf("Expected 2 improvements, got %d", len(comparison.Improved))
	}

	// Lint count improved (fewer errors)
	if comparison.WorseLints != -1 {
		t.Errorf("Expected WorseLints=-1, got %d", comparison.WorseLints)
	}
}
