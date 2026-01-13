package architect

import (
	"testing"
)

func TestAuditStatusConstants(t *testing.T) {
	tests := []struct {
		status   AuditStatus
		expected string
	}{
		{AuditStatusComplete, "COMPLETE"},
		{AuditStatusPartial, "PARTIAL"},
		{AuditStatusMissing, "MISSING"},
	}

	for _, tc := range tests {
		if string(tc.status) != tc.expected {
			t.Errorf("expected %s, got %s", tc.expected, tc.status)
		}
	}
}

func TestNewAuditor(t *testing.T) {
	auditor := NewAuditor()
	if auditor == nil {
		t.Fatal("expected non-nil auditor")
	}
	if auditor.maxFilesToScan != 50 {
		t.Errorf("expected maxFilesToScan=50, got %d", auditor.maxFilesToScan)
	}
}

func TestParseAuditStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected AuditStatus
	}{
		{"COMPLETE", AuditStatusComplete},
		{"complete", AuditStatusComplete},
		{"Complete", AuditStatusComplete},
		{" COMPLETE ", AuditStatusComplete},
		{"PARTIAL", AuditStatusPartial},
		{"partial", AuditStatusPartial},
		{"MISSING", AuditStatusMissing},
		{"missing", AuditStatusMissing},
		{"unknown", AuditStatusMissing},
		{"", AuditStatusMissing},
	}

	for _, tc := range tests {
		result := parseAuditStatus(tc.input)
		if result != tc.expected {
			t.Errorf("parseAuditStatus(%q): expected %s, got %s", tc.input, tc.expected, result)
		}
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "json code block",
			input:    "Here is the result:\n```json\n{\"key\": \"value\"}\n```\nDone!",
			expected: `{"key": "value"}`,
		},
		{
			name:     "plain code block",
			input:    "Here is the result:\n```\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "raw json",
			input:    "The answer is {\"key\": \"value\"} as expected.",
			expected: `{"key": "value"}`,
		},
		{
			name:     "nested json",
			input:    `{"outer": {"inner": "value"}}`,
			expected: `{"outer": {"inner": "value"}}`,
		},
		{
			name:     "no json",
			input:    "No JSON here",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractJSON(tc.input)
			if result != tc.expected {
				t.Errorf("extractJSON(%q): expected %q, got %q", tc.input, tc.expected, result)
			}
		})
	}
}

func TestIsCodeFile(t *testing.T) {
	tests := []struct {
		ext      string
		expected bool
	}{
		{".go", true},
		{".js", true},
		{".ts", true},
		{".py", true},
		{".java", true},
		{".rs", true},
		{".txt", false},
		{".md", false},
		{".json", false},
		{".yaml", false},
		{"", false},
	}

	for _, tc := range tests {
		result := isCodeFile(tc.ext)
		if result != tc.expected {
			t.Errorf("isCodeFile(%q): expected %v, got %v", tc.ext, tc.expected, result)
		}
	}
}

func TestBuildAuditPrompt(t *testing.T) {
	auditor := NewAuditor()
	spec := &ArchSpec{
		Name: "Test Spec",
		Features: []Feature{
			{
				ID:          "f1",
				Name:        "Feature One",
				Description: "First feature description",
				Criteria:    "Must do X",
			},
			{
				ID:          "f2",
				Name:        "Feature Two",
				Description: "Second feature description",
			},
		},
	}

	codeContext := "## Repository Structure\n\n- main.go\n- pkg/util.go\n"

	prompt := auditor.buildAuditPrompt(spec, codeContext)

	// Check that prompt contains essential elements
	checks := []string{
		"Test Spec",
		"Feature One",
		"f1",
		"Feature Two",
		"f2",
		"First feature description",
		"Must do X",
		"COMPLETE",
		"PARTIAL",
		"MISSING",
		"main.go",
		"pkg/util.go",
	}

	for _, check := range checks {
		if !contains(prompt, check) {
			t.Errorf("prompt should contain %q", check)
		}
	}
}

func TestParseAuditResponse(t *testing.T) {
	auditor := NewAuditor()
	features := []Feature{
		{ID: "f1", Name: "Feature One", Description: "Desc 1"},
		{ID: "f2", Name: "Feature Two", Description: "Desc 2"},
	}

	response := `Here is my analysis:
` + "```json" + `
{
  "features": [
    {
      "feature_id": "f1",
      "status": "COMPLETE",
      "evidence": "Found in main.go",
      "reasoning": "Fully implemented"
    },
    {
      "feature_id": "f2",
      "status": "PARTIAL",
      "evidence": "Partial in util.go",
      "reasoning": "Missing tests"
    }
  ],
  "gaps": [
    {
      "feature_id": "f2",
      "status": "PARTIAL",
      "description": "Missing test coverage",
      "suggested_action": "Add unit tests"
    }
  ],
  "summary": "One complete, one partial"
}
` + "```"

	report, err := auditor.parseAuditResponse(response, features)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(report.Features) != 2 {
		t.Errorf("expected 2 features, got %d", len(report.Features))
	}

	if len(report.Gaps) != 1 {
		t.Errorf("expected 1 gap, got %d", len(report.Gaps))
	}

	if report.Summary != "One complete, one partial" {
		t.Errorf("unexpected summary: %s", report.Summary)
	}

	// Check first feature
	if report.Features[0].Status != AuditStatusComplete {
		t.Errorf("expected first feature status COMPLETE, got %s", report.Features[0].Status)
	}
	if report.Features[0].Feature.ID != "f1" {
		t.Errorf("expected first feature ID f1, got %s", report.Features[0].Feature.ID)
	}

	// Check second feature
	if report.Features[1].Status != AuditStatusPartial {
		t.Errorf("expected second feature status PARTIAL, got %s", report.Features[1].Status)
	}

	// Check gap
	if report.Gaps[0].FeatureID != "f2" {
		t.Errorf("expected gap feature ID f2, got %s", report.Gaps[0].FeatureID)
	}
	if report.Gaps[0].SuggestedAction != "Add unit tests" {
		t.Errorf("unexpected suggested action: %s", report.Gaps[0].SuggestedAction)
	}
}

func TestParseAuditResponseNoJSON(t *testing.T) {
	auditor := NewAuditor()
	features := []Feature{}

	_, err := auditor.parseAuditResponse("No JSON here", features)
	if err == nil {
		t.Error("expected error for response without JSON")
	}
}

func TestParseAuditResponseInvalidJSON(t *testing.T) {
	auditor := NewAuditor()
	features := []Feature{}

	_, err := auditor.parseAuditResponse("{invalid json}", features)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestAuditEmptySpec(t *testing.T) {
	auditor := NewAuditor()

	// Test with nil spec
	report, err := auditor.Audit(nil, nil, "/tmp", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Summary != "No features to audit" {
		t.Errorf("unexpected summary: %s", report.Summary)
	}

	// Test with empty features
	emptySpec := &ArchSpec{Name: "Empty", Features: []Feature{}}
	report, err = auditor.Audit(nil, emptySpec, "/tmp", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Summary != "No features to audit" {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}

func TestFeatureStatusStruct(t *testing.T) {
	feature := Feature{
		ID:          "test-id",
		Name:        "Test Feature",
		Description: "A test feature",
		Criteria:    "Must work",
	}

	status := FeatureStatus{
		Feature:   feature,
		Status:    AuditStatusComplete,
		Evidence:  "Found in file.go",
		Reasoning: "Fully implemented",
	}

	if status.Feature.ID != "test-id" {
		t.Errorf("expected feature ID test-id, got %s", status.Feature.ID)
	}
	if status.Status != AuditStatusComplete {
		t.Errorf("expected status COMPLETE, got %s", status.Status)
	}
}

func TestGapStruct(t *testing.T) {
	gap := Gap{
		FeatureID:       "gap-feature",
		Status:          AuditStatusMissing,
		Description:     "Not implemented",
		SuggestedAction: "Implement the feature",
	}

	if gap.FeatureID != "gap-feature" {
		t.Errorf("expected feature ID gap-feature, got %s", gap.FeatureID)
	}
	if gap.Status != AuditStatusMissing {
		t.Errorf("expected status MISSING, got %s", gap.Status)
	}
}

func TestGapReportStruct(t *testing.T) {
	report := GapReport{
		Features: []FeatureStatus{
			{Status: AuditStatusComplete},
			{Status: AuditStatusPartial},
		},
		Gaps: []Gap{
			{FeatureID: "f2", Status: AuditStatusPartial},
		},
		Summary: "Mixed results",
	}

	if len(report.Features) != 2 {
		t.Errorf("expected 2 features, got %d", len(report.Features))
	}
	if len(report.Gaps) != 1 {
		t.Errorf("expected 1 gap, got %d", len(report.Gaps))
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
