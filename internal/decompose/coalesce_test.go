package decompose

import (
	"testing"
	"time"

	"github.com/ShayCichocki/alphie/pkg/models"
)

func TestCoalesceSetupTasks_NoSetup(t *testing.T) {
	tasks := []*models.Task{
		{ID: "1", Title: "Feature A", TaskType: models.TaskTypeFeature},
		{ID: "2", Title: "Feature B", TaskType: models.TaskTypeFeature},
	}

	result := CoalesceSetupTasks(tasks)

	if len(result) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(result))
	}
}

func TestCoalesceSetupTasks_SingleSetup(t *testing.T) {
	tasks := []*models.Task{
		{ID: "1", Title: "Setup", TaskType: models.TaskTypeSetup, FileBoundaries: []string{"package.json"}},
		{ID: "2", Title: "Feature", TaskType: models.TaskTypeFeature},
	}

	result := CoalesceSetupTasks(tasks)

	if len(result) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(result))
	}
}

func TestCoalesceSetupTasks_MergesOverlappingSetup(t *testing.T) {
	now := time.Now()
	tasks := []*models.Task{
		{
			ID:             "1",
			Title:          "Setup React",
			TaskType:       models.TaskTypeSetup,
			FileBoundaries: []string{"package.json", "tsconfig.json"},
			Description:    "Set up React frontend",
			CreatedAt:      now,
		},
		{
			ID:             "2",
			Title:          "Setup Express",
			TaskType:       models.TaskTypeSetup,
			FileBoundaries: []string{"package.json", "server/index.ts"},
			Description:    "Set up Express backend",
			CreatedAt:      now,
		},
		{
			ID:        "3",
			Title:     "Implement API",
			TaskType:  models.TaskTypeFeature,
			DependsOn: []string{"2"},
		},
	}

	result := CoalesceSetupTasks(tasks)

	// Should have 2 tasks: 1 merged SETUP + 1 FEATURE
	if len(result) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(result))
	}

	// Find the SETUP task
	var setupTask *models.Task
	for _, task := range result {
		if task.TaskType == models.TaskTypeSetup {
			setupTask = task
			break
		}
	}

	if setupTask == nil {
		t.Fatal("no SETUP task found")
	}

	// Should have merged title
	if setupTask.Title != "Project Setup: Setup React + Setup Express" {
		t.Errorf("unexpected title: %s", setupTask.Title)
	}

	// Should have combined file boundaries
	if len(setupTask.FileBoundaries) != 3 {
		t.Errorf("expected 3 file boundaries, got %d", len(setupTask.FileBoundaries))
	}

	// The feature task should now depend on the merged SETUP task
	var featureTask *models.Task
	for _, task := range result {
		if task.TaskType == models.TaskTypeFeature {
			featureTask = task
			break
		}
	}

	if featureTask == nil {
		t.Fatal("no FEATURE task found")
	}

	// Dependency should be updated to the merged task's ID (which is "1" - first task's ID)
	if len(featureTask.DependsOn) != 1 || featureTask.DependsOn[0] != "1" {
		t.Errorf("expected dependency on '1', got %v", featureTask.DependsOn)
	}
}

func TestCoalesceSetupTasks_NoOverlap(t *testing.T) {
	// Tasks with completely different critical files should NOT be merged
	tasks := []*models.Task{
		{
			ID:             "1",
			Title:          "Setup Go Backend",
			TaskType:       models.TaskTypeSetup,
			FileBoundaries: []string{"go.mod", "cmd/main.go"},
		},
		{
			ID:             "2",
			Title:          "Setup Rust CLI",
			TaskType:       models.TaskTypeSetup,
			FileBoundaries: []string{"Cargo.toml", "src/main.rs"},
		},
	}

	result := CoalesceSetupTasks(tasks)

	// Both tasks should be preserved (different ecosystems, no overlap)
	if len(result) != 2 {
		t.Errorf("expected 2 tasks (no overlap), got %d", len(result))
	}
}

func TestCoalesceSetupTasks_MonorepoPackageJson(t *testing.T) {
	// Note: HasCriticalFileOverlap compares by BASENAME, so client/package.json
	// and server/package.json ARE considered overlapping (both are "package.json").
	// This is conservative but safe for greenfield projects.
	tasks := []*models.Task{
		{
			ID:             "1",
			Title:          "Setup Client",
			TaskType:       models.TaskTypeSetup,
			FileBoundaries: []string{"client/package.json"},
		},
		{
			ID:             "2",
			Title:          "Setup Server",
			TaskType:       models.TaskTypeSetup,
			FileBoundaries: []string{"server/package.json"},
		},
	}

	result := CoalesceSetupTasks(tasks)

	// Should merge because basename "package.json" overlaps
	setupCount := 0
	for _, task := range result {
		if task.TaskType == models.TaskTypeSetup {
			setupCount++
		}
	}

	if setupCount != 1 {
		t.Errorf("expected 1 merged SETUP task (basename overlap), got %d", setupCount)
	}
}

func TestCoalesceSetupTasks_TransitiveOverlap(t *testing.T) {
	// A overlaps with B, B overlaps with C -> all three should merge
	tasks := []*models.Task{
		{
			ID:             "1",
			Title:          "Setup A",
			TaskType:       models.TaskTypeSetup,
			FileBoundaries: []string{"package.json"},
		},
		{
			ID:             "2",
			Title:          "Setup B",
			TaskType:       models.TaskTypeSetup,
			FileBoundaries: []string{"package.json", "tsconfig.json"},
		},
		{
			ID:             "3",
			Title:          "Setup C",
			TaskType:       models.TaskTypeSetup,
			FileBoundaries: []string{"tsconfig.json"},
		},
	}

	result := CoalesceSetupTasks(tasks)

	// All three should merge into one
	setupCount := 0
	for _, task := range result {
		if task.TaskType == models.TaskTypeSetup {
			setupCount++
		}
	}

	if setupCount != 1 {
		t.Errorf("expected 1 merged SETUP task, got %d", setupCount)
	}
}

func TestCoalesceSetupTasks_PreservesNonSetupDependencies(t *testing.T) {
	tasks := []*models.Task{
		{
			ID:        "1",
			Title:     "Setup A",
			TaskType:  models.TaskTypeSetup,
			DependsOn: []string{"external"}, // depends on non-merged task
		},
		{
			ID:             "2",
			Title:          "Setup B",
			TaskType:       models.TaskTypeSetup,
			FileBoundaries: []string{"package.json"},
		},
		{
			ID:       "external",
			Title:    "External Setup",
			TaskType: models.TaskTypeFeature, // Not a SETUP task
		},
	}

	result := CoalesceSetupTasks(tasks)

	// Find the first SETUP task (they don't overlap so won't merge)
	var setupA *models.Task
	for _, task := range result {
		if task.Title == "Setup A" {
			setupA = task
			break
		}
	}

	if setupA == nil {
		t.Fatal("Setup A not found")
	}

	// Should still depend on "external"
	if len(setupA.DependsOn) != 1 || setupA.DependsOn[0] != "external" {
		t.Errorf("expected dependency on 'external', got %v", setupA.DependsOn)
	}
}

func TestGroupByOverlappingCritical(t *testing.T) {
	tasks := []*models.Task{
		{ID: "1", FileBoundaries: []string{"package.json"}},
		{ID: "2", FileBoundaries: []string{"go.mod"}},
		{ID: "3", FileBoundaries: []string{"package.json", "tsconfig.json"}},
		{ID: "4", FileBoundaries: []string{"Cargo.toml"}},
	}

	groups := groupByOverlappingCritical(tasks)

	// Should have 3 groups:
	// - [1, 3] (share package.json)
	// - [2] (go.mod alone)
	// - [4] (Cargo.toml alone)
	if len(groups) != 3 {
		t.Errorf("expected 3 groups, got %d", len(groups))
	}

	// Find the merged group
	var mergedGroup []*models.Task
	for _, group := range groups {
		if len(group) > 1 {
			mergedGroup = group
			break
		}
	}

	if mergedGroup == nil {
		t.Fatal("expected one group with multiple tasks")
	}

	if len(mergedGroup) != 2 {
		t.Errorf("expected merged group of 2, got %d", len(mergedGroup))
	}
}

func TestUpdateDependencies(t *testing.T) {
	mapping := map[string]string{
		"old1": "new1",
		"old2": "new1", // Both map to same new ID
	}

	deps := []string{"old1", "old2", "unchanged"}
	result := updateDependencies(deps, mapping)

	// Should have 2 unique deps: "new1" and "unchanged"
	if len(result) != 2 {
		t.Errorf("expected 2 deps, got %d: %v", len(result), result)
	}

	foundNew := false
	foundUnchanged := false
	for _, dep := range result {
		if dep == "new1" {
			foundNew = true
		}
		if dep == "unchanged" {
			foundUnchanged = true
		}
	}

	if !foundNew || !foundUnchanged {
		t.Errorf("missing expected dependencies: %v", result)
	}
}
