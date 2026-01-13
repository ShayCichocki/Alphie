package architect

import (
	"context"
	"testing"
)

func TestNewController(t *testing.T) {
	c := NewController(10, 5.0, 3)

	if c.MaxIterations != 10 {
		t.Errorf("expected MaxIterations 10, got %d", c.MaxIterations)
	}
	if c.Budget != 5.0 {
		t.Errorf("expected Budget 5.0, got %f", c.Budget)
	}
	if c.NoConvergeAfter != 3 {
		t.Errorf("expected NoConvergeAfter 3, got %d", c.NoConvergeAfter)
	}
	if c.parser == nil {
		t.Error("expected parser to be initialized")
	}
	if c.auditor == nil {
		t.Error("expected auditor to be initialized")
	}
	if c.stopper == nil {
		t.Error("expected stopper to be initialized")
	}
}

func TestNewController_WithOptions(t *testing.T) {
	c := NewController(5, 2.0, 2,
		WithRepoPath("/test/repo"),
		WithProjectName("test-project"),
	)

	if c.RepoPath != "/test/repo" {
		t.Errorf("expected RepoPath '/test/repo', got %s", c.RepoPath)
	}
	if c.ProjectName != "test-project" {
		t.Errorf("expected ProjectName 'test-project', got %s", c.ProjectName)
	}
}

func TestController_RunContextCanceled(t *testing.T) {
	c := NewController(10, 5.0, 3,
		WithRepoPath("/nonexistent"),
		WithProjectName("test"),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := c.Run(ctx, "nonexistent.md", 1)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestIterationResult_Fields(t *testing.T) {
	result := IterationResult{
		Iteration:      1,
		GapsFound:      5,
		GapsRemaining:  3,
		EpicID:         "ep-123",
		TasksCreated:   5,
		TasksCompleted: 2,
		ProgressMade:   true,
		Cost:           0.05,
	}

	if result.Iteration != 1 {
		t.Errorf("expected Iteration 1, got %d", result.Iteration)
	}
	if result.GapsFound != 5 {
		t.Errorf("expected GapsFound 5, got %d", result.GapsFound)
	}
	if result.GapsRemaining != 3 {
		t.Errorf("expected GapsRemaining 3, got %d", result.GapsRemaining)
	}
	if result.EpicID != "ep-123" {
		t.Errorf("expected EpicID 'ep-123', got %s", result.EpicID)
	}
	if result.TasksCreated != 5 {
		t.Errorf("expected TasksCreated 5, got %d", result.TasksCreated)
	}
	if result.TasksCompleted != 2 {
		t.Errorf("expected TasksCompleted 2, got %d", result.TasksCompleted)
	}
	if !result.ProgressMade {
		t.Error("expected ProgressMade true")
	}
	if result.Cost != 0.05 {
		t.Errorf("expected Cost 0.05, got %f", result.Cost)
	}
}

func TestRunResult_Fields(t *testing.T) {
	result := RunResult{
		Iterations: []IterationResult{
			{Iteration: 1, GapsFound: 5},
			{Iteration: 2, GapsFound: 3},
		},
		StopReason:         StopReasonMaxIterations,
		TotalCost:          1.5,
		FinalCompletionPct: 75.0,
	}

	if len(result.Iterations) != 2 {
		t.Errorf("expected 2 iterations, got %d", len(result.Iterations))
	}
	if result.StopReason != StopReasonMaxIterations {
		t.Errorf("expected StopReasonMaxIterations, got %s", result.StopReason)
	}
	if result.TotalCost != 1.5 {
		t.Errorf("expected TotalCost 1.5, got %f", result.TotalCost)
	}
	if result.FinalCompletionPct != 75.0 {
		t.Errorf("expected FinalCompletionPct 75.0, got %f", result.FinalCompletionPct)
	}
}

func TestController_ZeroLimits(t *testing.T) {
	c := NewController(0, 0, 0)

	if c.MaxIterations != 0 {
		t.Errorf("expected MaxIterations 0, got %d", c.MaxIterations)
	}
	if c.Budget != 0 {
		t.Errorf("expected Budget 0, got %f", c.Budget)
	}
	if c.NoConvergeAfter != 0 {
		t.Errorf("expected NoConvergeAfter 0, got %d", c.NoConvergeAfter)
	}
}

func TestController_StopperConfig(t *testing.T) {
	c := NewController(15, 10.0, 5)

	// Verify the stopper is configured with matching values
	// by checking it stops at the expected iteration
	reason, stop := c.stopper.Check(15, 0, 50, true)
	if !stop || reason != StopReasonMaxIterations {
		t.Errorf("expected stop at iteration 15, got stop=%v, reason=%s", stop, reason)
	}

	// Reset and check budget
	c2 := NewController(100, 5.0, 10)
	reason, stop = c2.stopper.Check(1, 5.0, 50, true)
	if !stop || reason != StopReasonBudgetExceeded {
		t.Errorf("expected stop at budget 5.0, got stop=%v, reason=%s", stop, reason)
	}

	// Check convergence
	c3 := NewController(100, 100, 2)
	c3.stopper.Check(1, 0, 50, false)
	reason, stop = c3.stopper.Check(2, 0, 50, false)
	if !stop || reason != StopReasonConverged {
		t.Errorf("expected stop after 2 no-progress iterations, got stop=%v, reason=%s", stop, reason)
	}
}
