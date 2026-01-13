package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompareToBaseline_NewFailures(t *testing.T) {
	baseline := &Baseline{
		FailingTests: []string{"TestA", "TestB"},
		LintErrors:   []string{"error1"},
		TypeErrors:   []string{},
		CapturedAt:   time.Now(),
	}

	current := &GateResults{
		FailingTests: []string{"TestA", "TestB", "TestC"}, // TestC is new
		LintErrors:   []string{"error1"},
		TypeErrors:   []string{},
	}

	comparison := CompareToBaseline(current, baseline)

	if !comparison.IsRegression {
		t.Error("Should be a regression with new failure")
	}

	if len(comparison.NewFailures) != 1 {
		t.Errorf("Expected 1 new failure, got %d", len(comparison.NewFailures))
	}

	found := false
	for _, f := range comparison.NewFailures {
		if f == "TestC" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected TestC in new failures")
	}
}

func TestCompareToBaseline_Improved(t *testing.T) {
	baseline := &Baseline{
		FailingTests: []string{"TestA", "TestB", "TestC"},
		LintErrors:   []string{},
		TypeErrors:   []string{},
		CapturedAt:   time.Now(),
	}

	current := &GateResults{
		FailingTests: []string{"TestA"}, // TestB and TestC fixed
		LintErrors:   []string{},
		TypeErrors:   []string{},
	}

	comparison := CompareToBaseline(current, baseline)

	if comparison.IsRegression {
		t.Error("Should not be a regression when tests improved")
	}

	if len(comparison.Improved) != 2 {
		t.Errorf("Expected 2 improvements, got %d", len(comparison.Improved))
	}

	// Check that TestB and TestC are in improved
	improved := make(map[string]bool)
	for _, f := range comparison.Improved {
		improved[f] = true
	}
	if !improved["TestB"] || !improved["TestC"] {
		t.Error("Expected TestB and TestC in improvements")
	}
}

func TestCompareToBaseline_Unchanged(t *testing.T) {
	baseline := &Baseline{
		FailingTests: []string{"TestA"},
		LintErrors:   []string{"lint1"},
		TypeErrors:   []string{},
		CapturedAt:   time.Now(),
	}

	current := &GateResults{
		FailingTests: []string{"TestA"},
		LintErrors:   []string{"lint1"},
		TypeErrors:   []string{},
	}

	comparison := CompareToBaseline(current, baseline)

	if comparison.IsRegression {
		t.Error("Should not be a regression when unchanged")
	}

	if len(comparison.NewFailures) != 0 {
		t.Errorf("Expected 0 new failures, got %d", len(comparison.NewFailures))
	}

	if len(comparison.Improved) != 0 {
		t.Errorf("Expected 0 improvements, got %d", len(comparison.Improved))
	}
}

func TestCompareToBaseline_WorseLints(t *testing.T) {
	baseline := &Baseline{
		FailingTests: []string{},
		LintErrors:   []string{"lint1"},
		TypeErrors:   []string{},
		CapturedAt:   time.Now(),
	}

	current := &GateResults{
		FailingTests: []string{},
		LintErrors:   []string{"lint1", "lint2", "lint3"}, // 2 new lint errors
		TypeErrors:   []string{},
	}

	comparison := CompareToBaseline(current, baseline)

	if !comparison.IsRegression {
		t.Error("Should be a regression with worse lints")
	}

	if comparison.WorseLints != 2 {
		t.Errorf("Expected WorseLints = 2, got %d", comparison.WorseLints)
	}
}

func TestCompareToBaseline_ImprovedLints(t *testing.T) {
	baseline := &Baseline{
		FailingTests: []string{},
		LintErrors:   []string{"lint1", "lint2", "lint3"},
		TypeErrors:   []string{},
		CapturedAt:   time.Now(),
	}

	current := &GateResults{
		FailingTests: []string{},
		LintErrors:   []string{"lint1"}, // Fixed 2 lint errors
		TypeErrors:   []string{},
	}

	comparison := CompareToBaseline(current, baseline)

	if comparison.IsRegression {
		t.Error("Should not be a regression when lints improved")
	}

	if comparison.WorseLints != -2 {
		t.Errorf("Expected WorseLints = -2, got %d", comparison.WorseLints)
	}
}

func TestCompareToBaseline_NilCurrent(t *testing.T) {
	baseline := &Baseline{
		FailingTests: []string{"TestA"},
		CapturedAt:   time.Now(),
	}

	comparison := CompareToBaseline(nil, baseline)

	if comparison.IsRegression {
		t.Error("Nil current should not be a regression")
	}
}

func TestCompareToBaseline_NilBaseline(t *testing.T) {
	current := &GateResults{
		FailingTests: []string{"TestA"},
	}

	comparison := CompareToBaseline(current, nil)

	if !comparison.IsRegression {
		t.Error("Should be a regression when no baseline exists")
	}
}

func TestCompareToBaseline_BothNil(t *testing.T) {
	comparison := CompareToBaseline(nil, nil)

	if comparison.IsRegression {
		t.Error("Both nil should not be a regression")
	}
}

func TestCompareToBaseline_TypeErrors(t *testing.T) {
	baseline := &Baseline{
		FailingTests: []string{},
		LintErrors:   []string{},
		TypeErrors:   []string{"type error 1"},
		CapturedAt:   time.Now(),
	}

	current := &GateResults{
		FailingTests: []string{},
		LintErrors:   []string{},
		TypeErrors:   []string{"type error 1", "type error 2"}, // New type error
	}

	comparison := CompareToBaseline(current, baseline)

	if !comparison.IsRegression {
		t.Error("Should be a regression with new type error")
	}

	found := false
	for _, f := range comparison.NewFailures {
		if f == "type error 2" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'type error 2' in new failures")
	}
}

func TestBaseline_SaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "baseline-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	baseline := &Baseline{
		FailingTests: []string{"TestA", "TestB"},
		LintErrors:   []string{"lint1", "lint2"},
		TypeErrors:   []string{"type1"},
		CapturedAt:   time.Now().Truncate(time.Second), // Truncate for comparison
	}

	path := filepath.Join(tmpDir, "subdir", "baseline.json")

	// Save
	if err := baseline.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load
	loaded, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline() error = %v", err)
	}

	// Compare
	if len(loaded.FailingTests) != 2 {
		t.Errorf("Expected 2 failing tests, got %d", len(loaded.FailingTests))
	}
	if len(loaded.LintErrors) != 2 {
		t.Errorf("Expected 2 lint errors, got %d", len(loaded.LintErrors))
	}
	if len(loaded.TypeErrors) != 1 {
		t.Errorf("Expected 1 type error, got %d", len(loaded.TypeErrors))
	}
	if !loaded.CapturedAt.Equal(baseline.CapturedAt) {
		t.Errorf("CapturedAt mismatch: %v vs %v", loaded.CapturedAt, baseline.CapturedAt)
	}
}

func TestLoadBaseline_NonExistent(t *testing.T) {
	_, err := LoadBaseline("/nonexistent/path/baseline.json")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestLoadBaseline_InvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "baseline-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "invalid.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	_, err = LoadBaseline(path)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestGateResults_Fields(t *testing.T) {
	results := &GateResults{
		FailingTests: []string{"Test1", "Test2"},
		LintErrors:   []string{"lint1"},
		TypeErrors:   []string{"type1", "type2", "type3"},
	}

	if len(results.FailingTests) != 2 {
		t.Errorf("Expected 2 failing tests, got %d", len(results.FailingTests))
	}
	if len(results.LintErrors) != 1 {
		t.Errorf("Expected 1 lint error, got %d", len(results.LintErrors))
	}
	if len(results.TypeErrors) != 3 {
		t.Errorf("Expected 3 type errors, got %d", len(results.TypeErrors))
	}
}

func TestComparison_Fields(t *testing.T) {
	comparison := &Comparison{
		NewFailures:  []string{"fail1", "fail2"},
		WorseLints:   3,
		Improved:     []string{"improved1"},
		IsRegression: true,
	}

	if len(comparison.NewFailures) != 2 {
		t.Errorf("Expected 2 new failures, got %d", len(comparison.NewFailures))
	}
	if comparison.WorseLints != 3 {
		t.Errorf("Expected WorseLints = 3, got %d", comparison.WorseLints)
	}
	if len(comparison.Improved) != 1 {
		t.Errorf("Expected 1 improvement, got %d", len(comparison.Improved))
	}
	if !comparison.IsRegression {
		t.Error("Expected IsRegression = true")
	}
}
