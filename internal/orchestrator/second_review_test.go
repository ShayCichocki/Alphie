package orchestrator

import (
	"strings"
	"testing"

	"github.com/ShayCichocki/alphie/internal/protect"
	"github.com/ShayCichocki/alphie/pkg/models"
)

func TestNewSecondReviewer(t *testing.T) {
	protected := protect.New()
	reviewer := NewSecondReviewer(protected, nil)

	if reviewer == nil {
		t.Fatal("expected non-nil reviewer")
	}

	if reviewer.protected != protected {
		t.Error("expected protected detector to be set")
	}

	if reviewer.claude != nil {
		t.Error("expected claude to be nil")
	}
}

func TestReviewTrigger_ProtectedAreas(t *testing.T) {
	protected := protect.New()
	reviewer := NewSecondReviewer(protected, nil)

	task := &models.Task{
		ID:          "test-task",
		Title:       "Test Task",
		Description: "Update authentication logic",
	}

	// Files in protected areas should trigger review
	changedFiles := []string{
		"internal/auth/handler.go",
		"internal/api/routes.go",
	}

	trigger := reviewer.ShouldSecondReview("small diff", changedFiles, task)

	if !trigger.Triggered {
		t.Error("expected review to be triggered for protected areas")
	}

	foundProtectedReason := false
	for _, reason := range trigger.Reasons {
		if strings.Contains(reason, "protected areas") {
			foundProtectedReason = true
			break
		}
	}

	if !foundProtectedReason {
		t.Error("expected protected areas reason")
	}
}

func TestReviewTrigger_LargeDiff(t *testing.T) {
	reviewer := NewSecondReviewer(nil, nil)

	task := &models.Task{
		ID:          "test-task",
		Title:       "Test Task",
		Description: "Large refactoring",
	}

	// Create a large diff (>200 lines)
	var largeDiff strings.Builder
	for i := 0; i < 250; i++ {
		largeDiff.WriteString("+ line " + string(rune('a'+i%26)) + "\n")
	}

	changedFiles := []string{"internal/app/service.go"}

	trigger := reviewer.ShouldSecondReview(largeDiff.String(), changedFiles, task)

	if !trigger.Triggered {
		t.Error("expected review to be triggered for large diff")
	}

	foundLargeDiffReason := false
	for _, reason := range trigger.Reasons {
		if strings.Contains(reason, "large diff") {
			foundLargeDiffReason = true
			break
		}
	}

	if !foundLargeDiffReason {
		t.Error("expected large diff reason")
	}
}

func TestReviewTrigger_WeakTests(t *testing.T) {
	reviewer := NewSecondReviewer(nil, nil)

	task := &models.Task{
		ID:          "test-task",
		Title:       "Test Task",
		Description: "Add new feature",
	}

	// Source files without corresponding test files
	changedFiles := []string{
		"internal/app/service.go",
		"internal/app/handler.go",
	}

	trigger := reviewer.ShouldSecondReview("small diff", changedFiles, task)

	if !trigger.Triggered {
		t.Error("expected review to be triggered for weak tests")
	}

	foundWeakTestsReason := false
	for _, reason := range trigger.Reasons {
		if strings.Contains(reason, "weak or absent tests") {
			foundWeakTestsReason = true
			break
		}
	}

	if !foundWeakTestsReason {
		t.Error("expected weak tests reason")
	}
}

func TestReviewTrigger_WithTests(t *testing.T) {
	reviewer := NewSecondReviewer(nil, nil)

	task := &models.Task{
		ID:          "test-task",
		Title:       "Test Task",
		Description: "Add new feature with tests",
	}

	// Source files with corresponding test files
	changedFiles := []string{
		"internal/app/service.go",
		"internal/app/service_test.go",
	}

	trigger := reviewer.ShouldSecondReview("small diff", changedFiles, task)

	// Should not trigger for weak tests since test file is present
	for _, reason := range trigger.Reasons {
		if strings.Contains(reason, "weak or absent tests") {
			t.Error("should not trigger weak tests reason when test files are present")
		}
	}
}

func TestReviewTrigger_CrossCutting(t *testing.T) {
	reviewer := NewSecondReviewer(nil, nil)

	task := &models.Task{
		ID:          "test-task",
		Title:       "Test Task",
		Description: "Cross-cutting changes",
	}

	// Changes spanning more than 3 packages
	changedFiles := []string{
		"internal/app/service.go",
		"internal/app/service_test.go",
		"internal/api/handler.go",
		"internal/api/handler_test.go",
		"internal/db/repository.go",
		"internal/db/repository_test.go",
		"internal/util/helpers.go",
		"internal/util/helpers_test.go",
	}

	trigger := reviewer.ShouldSecondReview("small diff", changedFiles, task)

	if !trigger.Triggered {
		t.Error("expected review to be triggered for cross-cutting changes")
	}

	foundCrossCuttingReason := false
	for _, reason := range trigger.Reasons {
		if strings.Contains(reason, "cross-cutting") {
			foundCrossCuttingReason = true
			break
		}
	}

	if !foundCrossCuttingReason {
		t.Error("expected cross-cutting reason")
	}
}

func TestReviewTrigger_NoTrigger(t *testing.T) {
	reviewer := NewSecondReviewer(nil, nil)

	task := &models.Task{
		ID:          "test-task",
		Title:       "Test Task",
		Description: "Simple change",
	}

	// Small change with tests in few packages
	changedFiles := []string{
		"internal/app/service.go",
		"internal/app/service_test.go",
	}

	trigger := reviewer.ShouldSecondReview("small diff", changedFiles, task)

	if trigger.Triggered {
		t.Errorf("expected no trigger, but got reasons: %v", trigger.Reasons)
	}
}

func TestParseReviewResponse_Approved(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		approved bool
	}{
		{
			name:     "APPROVED on first line",
			output:   "APPROVED\n\nNo concerns.",
			approved: true,
		},
		{
			name:     "Approved with extra text",
			output:   "APPROVED - looks good\n\nNo issues found.",
			approved: true,
		},
		{
			name:     "NOT APPROVED",
			output:   "NOT APPROVED\n\nCONCERN: Missing error handling",
			approved: false,
		},
		{
			name:     "not approved lowercase",
			output:   "not approved\n\nCONCERN: Security issue",
			approved: false,
		},
		{
			name:     "Empty output",
			output:   "",
			approved: false,
		},
		{
			name:     "Whitespace only",
			output:   "   \n\n   ",
			approved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseReviewResponse(tt.output)
			if result.Approved != tt.approved {
				t.Errorf("expected approved=%v, got %v", tt.approved, result.Approved)
			}
		})
	}
}

func TestParseReviewResponse_Concerns(t *testing.T) {
	output := `NOT APPROVED

CONCERN: Missing error handling in the main function
CONCERN: No input validation
CONCERN: Potential SQL injection

Please address these issues.`

	result := parseReviewResponse(output)

	if result.Approved {
		t.Error("expected not approved")
	}

	if len(result.Concerns) != 3 {
		t.Errorf("expected 3 concerns, got %d", len(result.Concerns))
	}

	expectedConcerns := []string{
		"Missing error handling in the main function",
		"No input validation",
		"Potential SQL injection",
	}

	for i, expected := range expectedConcerns {
		if i >= len(result.Concerns) {
			t.Errorf("missing concern %d: %s", i, expected)
			continue
		}
		if result.Concerns[i] != expected {
			t.Errorf("concern %d: expected %q, got %q", i, expected, result.Concerns[i])
		}
	}

	if result.ReviewerOutput != output {
		t.Error("expected raw output to be preserved")
	}
}

func TestIsSourceFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main.go", true},
		{"service.ts", true},
		{"app.py", true},
		{"Main.java", true},
		{"README.md", false},
		{"config.yaml", false},
		{"Dockerfile", false},
		{".gitignore", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isSourceFile(tt.path); got != tt.expected {
				t.Errorf("isSourceFile(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"service_test.go", true},
		{"app.test.ts", true},
		{"test_utils.py", false}, // Starts with test_, but doesn't match the patterns
		{"UserTest.java", true},
		{"service_spec.rb", true},
		{"main.go", false},
		{"handler.ts", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isTestFile(tt.path); got != tt.expected {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestGetTestBaseName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"service_test.go", "service"},
		{"internal/app/handler_test.go", "handler"},
		{"app.test.ts", "app"},
		{"UserTest.java", "User"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := getTestBaseName(tt.path); got != tt.expected {
				t.Errorf("getTestBaseName(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestGetSourceBaseName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"service.go", "service"},
		{"internal/app/handler.go", "handler"},
		{"app.ts", "app"},
		{"User.java", "User"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := getSourceBaseName(tt.path); got != tt.expected {
				t.Errorf("getSourceBaseName(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestExtractPackage(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"internal/app/service.go", "internal/app"},
		{"service.go", ""},
		{"cmd/alphie/main.go", "cmd/alphie"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := extractPackage(tt.path); got != tt.expected {
				t.Errorf("extractPackage(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestBuildReviewPrompt(t *testing.T) {
	diff := "+ new line\n- old line"
	taskDesc := "Implement user authentication"

	prompt := buildReviewPrompt(diff, taskDesc)

	if !strings.Contains(prompt, "TASK DESCRIPTION:") {
		t.Error("prompt should contain task description header")
	}

	if !strings.Contains(prompt, taskDesc) {
		t.Error("prompt should contain task description")
	}

	if !strings.Contains(prompt, "DIFF TO REVIEW:") {
		t.Error("prompt should contain diff header")
	}

	if !strings.Contains(prompt, diff) {
		t.Error("prompt should contain diff")
	}

	if !strings.Contains(prompt, "APPROVED") {
		t.Error("prompt should mention APPROVED verdict")
	}

	if !strings.Contains(prompt, "CONCERN:") {
		t.Error("prompt should mention CONCERN prefix")
	}
}

func TestReviewTrigger_MultipleTriggers(t *testing.T) {
	protected := protect.New()
	reviewer := NewSecondReviewer(protected, nil)

	task := &models.Task{
		ID:          "test-task",
		Title:       "Test Task",
		Description: "Major refactoring",
	}

	// Create a large diff
	var largeDiff strings.Builder
	for i := 0; i < 250; i++ {
		largeDiff.WriteString("+ line\n")
	}

	// Files in protected areas spanning multiple packages without tests
	changedFiles := []string{
		"internal/auth/handler.go",
		"internal/security/validator.go",
		"internal/api/routes.go",
		"internal/db/queries.go",
	}

	trigger := reviewer.ShouldSecondReview(largeDiff.String(), changedFiles, task)

	if !trigger.Triggered {
		t.Error("expected review to be triggered")
	}

	// Should have multiple reasons
	if len(trigger.Reasons) < 3 {
		t.Errorf("expected at least 3 reasons, got %d: %v", len(trigger.Reasons), trigger.Reasons)
	}
}
