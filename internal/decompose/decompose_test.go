package decompose

import (
	"strings"
	"testing"

	"github.com/ShayCichocki/alphie/pkg/models"
)

func TestNew(t *testing.T) {
	decomposer := New(nil)
	if decomposer == nil {
		t.Fatal("New returned nil")
	}
}

func TestParseResponse_Valid(t *testing.T) {
	response := `[
		{
			"title": "Task 1",
			"description": "Description 1",
			"depends_on": [],
			"acceptance_criteria": "Criteria 1"
		},
		{
			"title": "Task 2",
			"description": "Description 2",
			"depends_on": ["Task 1"],
			"acceptance_criteria": "Criteria 2"
		}
	]`

	tasks, err := ParseResponse(response)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(tasks))
	}

	if tasks[0].Title != "Task 1" {
		t.Errorf("Task 0 title = %q, want %q", tasks[0].Title, "Task 1")
	}
	if len(tasks[0].DependsOn) != 0 {
		t.Errorf("Task 0 should have no dependencies, got %d", len(tasks[0].DependsOn))
	}

	if len(tasks[1].DependsOn) != 1 {
		t.Errorf("Task 1 should have 1 dependency, got %d", len(tasks[1].DependsOn))
	}
	if tasks[1].DependsOn[0] != tasks[0].ID {
		t.Errorf("Task 1 should depend on Task 0's ID")
	}
}

func TestParseResponse_WithExtraText(t *testing.T) {
	response := `Here are the tasks:
[
	{
		"title": "Single Task",
		"description": "Description",
		"depends_on": [],
		"acceptance_criteria": "Done"
	}
]
End of response.`

	tasks, err := ParseResponse(response)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	if len(tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(tasks))
	}
}

func TestParseResponse_NoJSONArray(t *testing.T) {
	response := "No JSON here"

	_, err := ParseResponse(response)
	if err == nil {
		t.Error("Expected error for response without JSON array")
	}
	if !strings.Contains(err.Error(), "no valid JSON array found") {
		t.Errorf("Error = %q, should contain 'no valid JSON array found'", err.Error())
	}
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	response := "[{invalid json}]"

	_, err := ParseResponse(response)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestParseResponse_EmptyArray(t *testing.T) {
	response := "[]"

	_, err := ParseResponse(response)
	if err == nil {
		t.Error("Expected error for empty task list")
	}
	if !strings.Contains(err.Error(), "empty task list") {
		t.Errorf("Error = %q, should contain 'empty task list'", err.Error())
	}
}

func TestParseResponse_UnknownDependency(t *testing.T) {
	response := `[
		{
			"title": "Task 1",
			"description": "Description",
			"depends_on": ["Nonexistent Task"],
			"acceptance_criteria": "Done"
		}
	]`

	_, err := ParseResponse(response)
	if err == nil {
		t.Error("Expected error for unknown dependency")
	}
	if !strings.Contains(err.Error(), "unknown dependency") {
		t.Errorf("Error = %q, should contain 'unknown dependency'", err.Error())
	}
}

func TestParseResponse_TaskFields(t *testing.T) {
	response := `[
		{
			"title": "Task Title",
			"description": "Task Description",
			"depends_on": [],
			"acceptance_criteria": "Task Criteria"
		}
	]`

	tasks, err := ParseResponse(response)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	task := tasks[0]

	if task.ID == "" {
		t.Error("Task should have an ID")
	}
	if task.Title != "Task Title" {
		t.Errorf("Title = %q, want %q", task.Title, "Task Title")
	}
	if task.Description != "Task Description" {
		t.Errorf("Description = %q, want %q", task.Description, "Task Description")
	}
	if task.AcceptanceCriteria != "Task Criteria" {
		t.Errorf("AcceptanceCriteria = %q, want %q", task.AcceptanceCriteria, "Task Criteria")
	}
	if task.Status != models.TaskStatusPending {
		t.Errorf("Status = %q, want %q", task.Status, models.TaskStatusPending)
	}
	if task.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestValidateNoCycles_NoCycles(t *testing.T) {
	tasks := []*models.Task{
		{ID: "1", Title: "Task 1", DependsOn: []string{}},
		{ID: "2", Title: "Task 2", DependsOn: []string{"1"}},
		{ID: "3", Title: "Task 3", DependsOn: []string{"1", "2"}},
	}

	err := ValidateNoCycles(tasks)
	if err != nil {
		t.Errorf("Expected no error for valid DAG, got: %v", err)
	}
}

func TestValidateNoCycles_DirectCycle(t *testing.T) {
	tasks := []*models.Task{
		{ID: "1", Title: "Task 1", DependsOn: []string{"2"}},
		{ID: "2", Title: "Task 2", DependsOn: []string{"1"}},
	}

	err := ValidateNoCycles(tasks)
	if err == nil {
		t.Error("Expected error for direct cycle")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("Error = %q, should contain 'circular dependency'", err.Error())
	}
}

func TestValidateNoCycles_IndirectCycle(t *testing.T) {
	tasks := []*models.Task{
		{ID: "1", Title: "Task 1", DependsOn: []string{"3"}},
		{ID: "2", Title: "Task 2", DependsOn: []string{"1"}},
		{ID: "3", Title: "Task 3", DependsOn: []string{"2"}},
	}

	err := ValidateNoCycles(tasks)
	if err == nil {
		t.Error("Expected error for indirect cycle")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("Error = %q, should contain 'circular dependency'", err.Error())
	}
}

func TestValidateNoCycles_SelfCycle(t *testing.T) {
	tasks := []*models.Task{
		{ID: "1", Title: "Task 1", DependsOn: []string{"1"}},
	}

	err := ValidateNoCycles(tasks)
	if err == nil {
		t.Error("Expected error for self-referencing cycle")
	}
}

func TestValidateNoCycles_EmptyList(t *testing.T) {
	tasks := []*models.Task{}

	err := ValidateNoCycles(tasks)
	if err != nil {
		t.Errorf("Expected no error for empty task list, got: %v", err)
	}
}

func TestValidateNoCycles_SingleTask(t *testing.T) {
	tasks := []*models.Task{
		{ID: "1", Title: "Task 1", DependsOn: []string{}},
	}

	err := ValidateNoCycles(tasks)
	if err != nil {
		t.Errorf("Expected no error for single task, got: %v", err)
	}
}

func TestValidateNoCycles_DiamondDependency(t *testing.T) {
	tasks := []*models.Task{
		{ID: "A", Title: "Task A", DependsOn: []string{}},
		{ID: "B", Title: "Task B", DependsOn: []string{"A"}},
		{ID: "C", Title: "Task C", DependsOn: []string{"A"}},
		{ID: "D", Title: "Task D", DependsOn: []string{"B", "C"}},
	}

	err := ValidateNoCycles(tasks)
	if err != nil {
		t.Errorf("Expected no error for diamond dependency, got: %v", err)
	}
}

func TestValidateNoCycles_MissingDependency(t *testing.T) {
	tasks := []*models.Task{
		{ID: "1", Title: "Task 1", DependsOn: []string{"nonexistent"}},
	}

	err := ValidateNoCycles(tasks)
	if err != nil {
		t.Errorf("ValidateNoCycles should not fail on missing deps, got: %v", err)
	}
}

func TestParseResponse_MultipleDependencies(t *testing.T) {
	response := `[
		{
			"title": "Setup",
			"description": "Initial setup",
			"depends_on": [],
			"acceptance_criteria": "Done"
		},
		{
			"title": "Config",
			"description": "Configuration",
			"depends_on": [],
			"acceptance_criteria": "Done"
		},
		{
			"title": "Build",
			"description": "Build project",
			"depends_on": ["Setup", "Config"],
			"acceptance_criteria": "Done"
		}
	]`

	tasks, err := ParseResponse(response)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	if len(tasks) != 3 {
		t.Fatalf("Expected 3 tasks, got %d", len(tasks))
	}

	buildTask := tasks[2]
	if len(buildTask.DependsOn) != 2 {
		t.Errorf("Build task should have 2 dependencies, got %d", len(buildTask.DependsOn))
	}

	setupID := tasks[0].ID
	configID := tasks[1].ID

	foundSetup := false
	foundConfig := false
	for _, dep := range buildTask.DependsOn {
		if dep == setupID {
			foundSetup = true
		}
		if dep == configID {
			foundConfig = true
		}
	}

	if !foundSetup {
		t.Error("Build should depend on Setup")
	}
	if !foundConfig {
		t.Error("Build should depend on Config")
	}
}

func TestDecomposedTask_Struct(t *testing.T) {
	dt := decomposedTask{
		Title:              "Test Task",
		Description:        "Test Description",
		DependsOn:          []string{"Dep1", "Dep2"},
		AcceptanceCriteria: "Test Criteria",
	}

	if dt.Title != "Test Task" {
		t.Errorf("Title = %q, want %q", dt.Title, "Test Task")
	}
	if len(dt.DependsOn) != 2 {
		t.Errorf("DependsOn len = %d, want 2", len(dt.DependsOn))
	}
}

func TestDecompositionPrompt(t *testing.T) {
	if !strings.Contains(decompositionPrompt, "Break this user request") {
		t.Error("Prompt should contain decomposition instruction")
	}
	if !strings.Contains(decompositionPrompt, "JSON array") {
		t.Error("Prompt should mention JSON array format")
	}
	if !strings.Contains(decompositionPrompt, "depends_on") {
		t.Error("Prompt should mention depends_on field")
	}
	if !strings.Contains(decompositionPrompt, "acceptance_criteria") {
		t.Error("Prompt should mention acceptance_criteria field")
	}
}
