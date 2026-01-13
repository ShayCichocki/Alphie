package architect

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shayc/alphie/internal/prog"
)

func setupTestDB(t *testing.T) (*prog.Client, func()) {
	t.Helper()

	// Create a temporary directory for the test database
	tmpDir, err := os.MkdirTemp("", "planner-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := prog.Open(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open database: %v", err)
	}

	if err := db.Init(); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to init database: %v", err)
	}

	client := prog.NewClient(db, "test-project")

	cleanup := func() {
		client.Close()
		os.RemoveAll(tmpDir)
	}

	return client, cleanup
}

func TestNewPlanner(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	planner := NewPlanner(client)
	if planner == nil {
		t.Fatal("expected non-nil planner")
	}
	if planner.client != client {
		t.Error("expected planner to use provided client")
	}
}

func TestPlanEmptyGaps(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	planner := NewPlanner(client)
	ctx := context.Background()

	// Test with nil gaps
	result, err := planner.Plan(ctx, nil, "test-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EpicID != "" {
		t.Errorf("expected empty EpicID, got %s", result.EpicID)
	}
	if len(result.TaskIDs) != 0 {
		t.Errorf("expected no TaskIDs, got %d", len(result.TaskIDs))
	}

	// Test with empty gaps list
	emptyReport := &GapReport{Gaps: []Gap{}}
	result, err = planner.Plan(ctx, emptyReport, "test-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EpicID != "" {
		t.Errorf("expected empty EpicID, got %s", result.EpicID)
	}
}

func TestPlanWithMissingGaps(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	planner := NewPlanner(client)
	ctx := context.Background()

	gaps := &GapReport{
		Gaps: []Gap{
			{
				FeatureID:       "feature-1",
				Status:          AuditStatusMissing,
				Description:     "Feature 1 is not implemented",
				SuggestedAction: "Implement feature 1",
			},
			{
				FeatureID:       "feature-2",
				Status:          AuditStatusMissing,
				Description:     "Feature 2 is not implemented",
				SuggestedAction: "Implement feature 2",
			},
		},
		Summary: "Two features are missing",
	}

	result, err := planner.Plan(ctx, gaps, "test-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EpicID == "" {
		t.Error("expected EpicID to be set")
	}
	if len(result.TaskIDs) != 2 {
		t.Errorf("expected 2 TaskIDs, got %d", len(result.TaskIDs))
	}

	// Verify epic was created
	epic, err := client.GetEpic(result.EpicID)
	if err != nil {
		t.Fatalf("failed to get epic: %v", err)
	}
	if epic.Title != "Implement 2 missing features" {
		t.Errorf("unexpected epic title: %s", epic.Title)
	}

	// Verify tasks were created with correct parent
	for _, taskID := range result.TaskIDs {
		task, err := client.GetItem(taskID)
		if err != nil {
			t.Fatalf("failed to get task %s: %v", taskID, err)
		}
		if task.ParentID == nil || *task.ParentID != result.EpicID {
			t.Errorf("task %s should have parent %s", taskID, result.EpicID)
		}
		if task.Priority != 1 {
			t.Errorf("missing gap task should have priority 1, got %d", task.Priority)
		}
	}
}

func TestPlanWithPartialGaps(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	planner := NewPlanner(client)
	ctx := context.Background()

	gaps := &GapReport{
		Gaps: []Gap{
			{
				FeatureID:       "feature-1",
				Status:          AuditStatusPartial,
				Description:     "Feature 1 is partially implemented",
				SuggestedAction: "Complete feature 1",
			},
		},
		Summary: "One feature is partial",
	}

	result, err := planner.Plan(ctx, gaps, "test-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EpicID == "" {
		t.Error("expected EpicID to be set")
	}
	if len(result.TaskIDs) != 1 {
		t.Errorf("expected 1 TaskID, got %d", len(result.TaskIDs))
	}

	// Verify task priority for partial gaps
	task, err := client.GetItem(result.TaskIDs[0])
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if task.Priority != 2 {
		t.Errorf("partial gap task should have priority 2, got %d", task.Priority)
	}
}

func TestPlanWithMixedGaps(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	planner := NewPlanner(client)
	ctx := context.Background()

	gaps := &GapReport{
		Gaps: []Gap{
			{
				FeatureID:       "feature-1",
				Status:          AuditStatusMissing,
				Description:     "Feature 1 is missing",
				SuggestedAction: "Implement feature 1",
			},
			{
				FeatureID:       "feature-2",
				Status:          AuditStatusPartial,
				Description:     "Feature 2 is partial",
				SuggestedAction: "Complete feature 2",
			},
			{
				FeatureID:       "feature-3",
				Status:          AuditStatusMissing,
				Description:     "Feature 3 is missing",
				SuggestedAction: "Implement feature 3",
			},
		},
		Summary: "Mixed gaps",
	}

	result, err := planner.Plan(ctx, gaps, "test-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.TaskIDs) != 3 {
		t.Errorf("expected 3 TaskIDs, got %d", len(result.TaskIDs))
	}

	// Verify epic title
	epic, err := client.GetEpic(result.EpicID)
	if err != nil {
		t.Fatalf("failed to get epic: %v", err)
	}
	if epic.Title != "Implement 2 missing and refine 1 partial features" {
		t.Errorf("unexpected epic title: %s", epic.Title)
	}

	// Verify the partial task depends on the last missing task
	partialTaskID := result.TaskIDs[2] // Third task (partial gap)
	deps, err := client.GetDependencies(partialTaskID)
	if err != nil {
		t.Fatalf("failed to get dependencies: %v", err)
	}
	if len(deps) != 1 {
		t.Errorf("expected partial task to have 1 dependency, got %d", len(deps))
	}
}

func TestGroupGapsIntoPhases(t *testing.T) {
	planner := &Planner{}

	// Test with only missing gaps
	missingGaps := []Gap{
		{FeatureID: "f1", Status: AuditStatusMissing},
		{FeatureID: "f2", Status: AuditStatusMissing},
	}
	phases := planner.groupGapsIntoPhases(missingGaps)
	if len(phases) != 1 {
		t.Errorf("expected 1 phase for missing-only gaps, got %d", len(phases))
	}
	if phases[0].Name != "Foundation" {
		t.Errorf("expected Foundation phase, got %s", phases[0].Name)
	}
	if len(phases[0].Gaps) != 2 {
		t.Errorf("expected 2 gaps in Foundation phase, got %d", len(phases[0].Gaps))
	}

	// Test with only partial gaps
	partialGaps := []Gap{
		{FeatureID: "f1", Status: AuditStatusPartial},
	}
	phases = planner.groupGapsIntoPhases(partialGaps)
	if len(phases) != 1 {
		t.Errorf("expected 1 phase for partial-only gaps, got %d", len(phases))
	}
	if phases[0].Name != "Refinement" {
		t.Errorf("expected Refinement phase, got %s", phases[0].Name)
	}

	// Test with mixed gaps
	mixedGaps := []Gap{
		{FeatureID: "f1", Status: AuditStatusMissing},
		{FeatureID: "f2", Status: AuditStatusPartial},
	}
	phases = planner.groupGapsIntoPhases(mixedGaps)
	if len(phases) != 2 {
		t.Errorf("expected 2 phases for mixed gaps, got %d", len(phases))
	}
	if phases[0].Name != "Foundation" {
		t.Errorf("first phase should be Foundation, got %s", phases[0].Name)
	}
	if phases[1].Name != "Refinement" {
		t.Errorf("second phase should be Refinement, got %s", phases[1].Name)
	}
	if len(phases[1].DependsOnPhases) != 1 || phases[1].DependsOnPhases[0] != 0 {
		t.Error("Refinement phase should depend on Foundation phase")
	}

	// Test with empty gaps
	phases = planner.groupGapsIntoPhases([]Gap{})
	if phases != nil {
		t.Errorf("expected nil phases for empty gaps, got %v", phases)
	}
}

func TestGenerateEpicTitle(t *testing.T) {
	planner := &Planner{}

	tests := []struct {
		name     string
		gaps     *GapReport
		expected string
	}{
		{
			name: "only missing",
			gaps: &GapReport{
				Gaps: []Gap{
					{Status: AuditStatusMissing},
					{Status: AuditStatusMissing},
				},
			},
			expected: "Implement 2 missing features",
		},
		{
			name: "only partial",
			gaps: &GapReport{
				Gaps: []Gap{
					{Status: AuditStatusPartial},
				},
			},
			expected: "Refine 1 partial features",
		},
		{
			name: "mixed",
			gaps: &GapReport{
				Gaps: []Gap{
					{Status: AuditStatusMissing},
					{Status: AuditStatusPartial},
					{Status: AuditStatusPartial},
				},
			},
			expected: "Implement 1 missing and refine 2 partial features",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			title := planner.generateEpicTitle(tc.gaps)
			if title != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, title)
			}
		})
	}
}

func TestGenerateTaskTitle(t *testing.T) {
	planner := &Planner{}

	tests := []struct {
		gap      Gap
		expected string
	}{
		{
			gap:      Gap{FeatureID: "feature-1", Status: AuditStatusMissing},
			expected: "Implement feature-1",
		},
		{
			gap:      Gap{FeatureID: "feature-2", Status: AuditStatusPartial},
			expected: "Complete feature-2",
		},
	}

	for _, tc := range tests {
		title := planner.generateTaskTitle(tc.gap)
		if title != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, title)
		}
	}
}

func TestGenerateTaskDescription(t *testing.T) {
	planner := &Planner{}

	gap := Gap{
		FeatureID:       "feature-1",
		Status:          AuditStatusMissing,
		Description:     "Feature is not implemented",
		SuggestedAction: "Add the implementation",
	}

	desc := planner.generateTaskDescription(gap)

	if !containsString(desc, "MISSING") {
		t.Error("description should contain status")
	}
	if !containsString(desc, "Feature is not implemented") {
		t.Error("description should contain gap description")
	}
	if !containsString(desc, "Add the implementation") {
		t.Error("description should contain suggested action")
	}
}

func TestGapPriority(t *testing.T) {
	planner := &Planner{}

	missingGap := Gap{Status: AuditStatusMissing}
	if planner.gapPriority(missingGap) != 1 {
		t.Error("missing gap should have priority 1")
	}

	partialGap := Gap{Status: AuditStatusPartial}
	if planner.gapPriority(partialGap) != 2 {
		t.Error("partial gap should have priority 2")
	}
}

func TestPlanResultStruct(t *testing.T) {
	result := PlanResult{
		EpicID:  "ep-123456",
		TaskIDs: []string{"ts-111111", "ts-222222"},
	}

	if result.EpicID != "ep-123456" {
		t.Errorf("unexpected EpicID: %s", result.EpicID)
	}
	if len(result.TaskIDs) != 2 {
		t.Errorf("expected 2 TaskIDs, got %d", len(result.TaskIDs))
	}
}

func TestPhaseStruct(t *testing.T) {
	phase := Phase{
		Name: "Test Phase",
		Gaps: []Gap{
			{FeatureID: "f1"},
		},
		DependsOnPhases: []int{0, 1},
	}

	if phase.Name != "Test Phase" {
		t.Errorf("unexpected phase name: %s", phase.Name)
	}
	if len(phase.Gaps) != 1 {
		t.Errorf("expected 1 gap, got %d", len(phase.Gaps))
	}
	if len(phase.DependsOnPhases) != 2 {
		t.Errorf("expected 2 dependencies, got %d", len(phase.DependsOnPhases))
	}
}

// Helper function
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
