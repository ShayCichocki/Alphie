package learning

import (
	"strings"
	"testing"
)

func TestFailureAnalyzer_AnalyzeFailure_GoUndefined(t *testing.T) {
	fa := NewFailureAnalyzer()
	output := `# github.com/ShayCichocki/alphie/internal/agent
internal/agent/executor.go:45:2: undefined: SomeFunction`
	errorMsg := ""

	suggestions := fa.AnalyzeFailure(output, errorMsg)

	if len(suggestions) == 0 {
		t.Fatal("expected at least one suggestion")
	}

	found := false
	for _, s := range suggestions {
		if s.Source == "go_undefined" {
			found = true
			if !strings.Contains(s.CAO.Condition, "SomeFunction") {
				t.Errorf("expected condition to contain 'SomeFunction', got: %s", s.CAO.Condition)
			}
			if s.Confidence < 0.5 {
				t.Errorf("expected confidence >= 0.5, got: %f", s.Confidence)
			}
		}
	}
	if !found {
		t.Error("expected to find go_undefined suggestion")
	}
}

func TestFailureAnalyzer_AnalyzeFailure_TypeMismatch(t *testing.T) {
	fa := NewFailureAnalyzer()
	output := `cannot use x (type int) as type string in argument`
	errorMsg := ""

	suggestions := fa.AnalyzeFailure(output, errorMsg)

	found := false
	for _, s := range suggestions {
		if s.Source == "go_type_mismatch" {
			found = true
			if !strings.Contains(s.CAO.Condition, "int") || !strings.Contains(s.CAO.Condition, "string") {
				t.Errorf("expected condition to contain types, got: %s", s.CAO.Condition)
			}
		}
	}
	if !found {
		t.Error("expected to find go_type_mismatch suggestion")
	}
}

func TestFailureAnalyzer_AnalyzeFailure_ImportCycle(t *testing.T) {
	fa := NewFailureAnalyzer()
	output := `import cycle not allowed
	package github.com/foo/bar
		imports github.com/foo/baz
		imports github.com/foo/bar`
	errorMsg := ""

	suggestions := fa.AnalyzeFailure(output, errorMsg)

	found := false
	for _, s := range suggestions {
		if s.Source == "go_import_cycle" {
			found = true
			if !strings.Contains(s.CAO.Action, "interface") {
				t.Errorf("expected action to mention interface pattern, got: %s", s.CAO.Action)
			}
		}
	}
	if !found {
		t.Error("expected to find go_import_cycle suggestion")
	}
}

func TestFailureAnalyzer_AnalyzeFailure_TestFail(t *testing.T) {
	fa := NewFailureAnalyzer()
	output := `--- FAIL: TestSomething
    test.go:15: expected true, got false`
	errorMsg := ""

	suggestions := fa.AnalyzeFailure(output, errorMsg)

	found := false
	for _, s := range suggestions {
		if s.Source == "go_test_fail" {
			found = true
			if !strings.Contains(s.CAO.Condition, "TestSomething") {
				t.Errorf("expected condition to contain test name, got: %s", s.CAO.Condition)
			}
		}
	}
	if !found {
		t.Error("expected to find go_test_fail suggestion")
	}
}

func TestFailureAnalyzer_AnalyzeFailure_PermissionDenied(t *testing.T) {
	fa := NewFailureAnalyzer()
	output := ``
	errorMsg := `open /etc/passwd: permission denied`

	suggestions := fa.AnalyzeFailure(output, errorMsg)

	found := false
	for _, s := range suggestions {
		if s.Source == "permission_denied" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find permission_denied suggestion")
	}
}

func TestFailureAnalyzer_AnalyzeFailure_FileNotFound(t *testing.T) {
	fa := NewFailureAnalyzer()
	output := `open config.yaml: no such file or directory: config.yaml`
	errorMsg := ""

	suggestions := fa.AnalyzeFailure(output, errorMsg)

	found := false
	for _, s := range suggestions {
		if s.Source == "file_not_found" {
			found = true
			if !strings.Contains(s.CAO.Condition, "config.yaml") {
				t.Errorf("expected condition to contain filename, got: %s", s.CAO.Condition)
			}
		}
	}
	if !found {
		t.Error("expected to find file_not_found suggestion")
	}
}

func TestFailureAnalyzer_AnalyzeFailure_GitConflict(t *testing.T) {
	fa := NewFailureAnalyzer()
	output := `CONFLICT (content): Merge conflict in main.go
Auto-merging utils.go`
	errorMsg := ""

	suggestions := fa.AnalyzeFailure(output, errorMsg)

	found := false
	for _, s := range suggestions {
		if s.Source == "git_conflict" {
			found = true
			if !strings.Contains(s.CAO.Condition, "main.go") {
				t.Errorf("expected condition to contain filename, got: %s", s.CAO.Condition)
			}
		}
	}
	if !found {
		t.Error("expected to find git_conflict suggestion")
	}
}

func TestFailureAnalyzer_AnalyzeFailure_Timeout(t *testing.T) {
	fa := NewFailureAnalyzer()
	output := ``
	errorMsg := `context deadline exceeded`

	suggestions := fa.AnalyzeFailure(output, errorMsg)

	found := false
	for _, s := range suggestions {
		if s.Source == "timeout" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find timeout suggestion")
	}
}

func TestFailureAnalyzer_AnalyzeFailure_NoMatch(t *testing.T) {
	fa := NewFailureAnalyzer()
	output := `Everything completed successfully!`
	errorMsg := ""

	suggestions := fa.AnalyzeFailure(output, errorMsg)

	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions for success output, got: %d", len(suggestions))
	}
}

func TestFormatForConfirmation(t *testing.T) {
	sl := &SuggestedLearning{
		CAO: &CAOTriple{
			Condition: "Test fails",
			Action:    "Fix the test",
			Outcome:   "Test passes",
		},
		Source:     "test_source",
		Confidence: 0.8,
	}

	formatted := FormatForConfirmation(sl)

	if !strings.Contains(formatted, "WHEN: Test fails") {
		t.Error("expected formatted output to contain WHEN clause")
	}
	if !strings.Contains(formatted, "DO: Fix the test") {
		t.Error("expected formatted output to contain DO clause")
	}
	if !strings.Contains(formatted, "RESULT: Test passes") {
		t.Error("expected formatted output to contain RESULT clause")
	}
	if !strings.Contains(formatted, "test_source") {
		t.Error("expected formatted output to contain source")
	}
}

func TestFormatForConfirmation_Nil(t *testing.T) {
	result := FormatForConfirmation(nil)
	if result != "" {
		t.Errorf("expected empty string for nil input, got: %s", result)
	}

	result = FormatForConfirmation(&SuggestedLearning{})
	if result != "" {
		t.Errorf("expected empty string for nil CAO, got: %s", result)
	}
}

func TestCapturer_CaptureFromFailure(t *testing.T) {
	c := NewCapturer(nil) // No system, just analyze

	output := `--- FAIL: TestFoo
    test.go:10: expected bar, got baz`
	errorMsg := "test failed"

	result, err := c.CaptureFromFailure(output, errorMsg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Suggestions) == 0 {
		t.Error("expected suggestions from failure")
	}

	// No system means no existing learnings
	if len(result.ExistingLearnings) != 0 {
		t.Errorf("expected no existing learnings without system, got: %d", len(result.ExistingLearnings))
	}
}

func TestCapturer_ConfirmAndStore_NoSystem(t *testing.T) {
	c := NewCapturer(nil)

	sl := &SuggestedLearning{
		CAO: &CAOTriple{
			Condition: "Test condition",
			Action:    "Test action",
			Outcome:   "Test outcome",
		},
	}

	learning, err := c.ConfirmAndStore(sl, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if learning != nil {
		t.Error("expected nil learning when no system")
	}
}

func TestCapturer_ConfirmAndStore_NilSuggestion(t *testing.T) {
	c := NewCapturer(nil)

	learning, err := c.ConfirmAndStore(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if learning != nil {
		t.Error("expected nil learning for nil suggestion")
	}
}
