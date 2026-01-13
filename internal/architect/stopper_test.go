package architect

import "testing"

func TestStopChecker_Complete(t *testing.T) {
	checker := NewStopChecker(DefaultStopConfig())

	reason, stop := checker.Check(1, 0.5, 100.0, true)
	if !stop {
		t.Fatal("expected stop on 100% completion")
	}
	if reason != StopReasonComplete {
		t.Fatalf("expected StopReasonComplete, got %s", reason)
	}
}

func TestStopChecker_MaxIterations(t *testing.T) {
	config := StopConfig{
		MaxIterations:   3,
		BudgetLimit:     0,
		NoProgressLimit: 0,
	}
	checker := NewStopChecker(config)

	// First two iterations should continue
	reason, stop := checker.Check(1, 0.5, 50.0, true)
	if stop {
		t.Fatal("should not stop on iteration 1")
	}
	if reason != StopReasonNone {
		t.Fatalf("expected StopReasonNone, got %s", reason)
	}

	reason, stop = checker.Check(2, 1.0, 60.0, true)
	if stop {
		t.Fatal("should not stop on iteration 2")
	}

	// Third iteration should stop
	reason, stop = checker.Check(3, 1.5, 70.0, true)
	if !stop {
		t.Fatal("expected stop on iteration 3 (max)")
	}
	if reason != StopReasonMaxIterations {
		t.Fatalf("expected StopReasonMaxIterations, got %s", reason)
	}
}

func TestStopChecker_BudgetExceeded(t *testing.T) {
	config := StopConfig{
		MaxIterations:   100,
		BudgetLimit:     2.0,
		NoProgressLimit: 0,
	}
	checker := NewStopChecker(config)

	// Under budget should continue
	reason, stop := checker.Check(1, 1.0, 50.0, true)
	if stop {
		t.Fatal("should not stop when under budget")
	}

	// At or over budget should stop
	reason, stop = checker.Check(2, 2.0, 60.0, true)
	if !stop {
		t.Fatal("expected stop when budget reached")
	}
	if reason != StopReasonBudgetExceeded {
		t.Fatalf("expected StopReasonBudgetExceeded, got %s", reason)
	}
}

func TestStopChecker_Converged(t *testing.T) {
	config := StopConfig{
		MaxIterations:   100,
		BudgetLimit:     100.0,
		NoProgressLimit: 3,
	}
	checker := NewStopChecker(config)

	// Progress made - should not converge
	checker.Check(1, 0.5, 50.0, true)
	if checker.NoProgressCount() != 0 {
		t.Fatalf("expected no progress count 0, got %d", checker.NoProgressCount())
	}

	// No progress for 1 iteration
	checker.Check(2, 1.0, 50.0, false)
	if checker.NoProgressCount() != 1 {
		t.Fatalf("expected no progress count 1, got %d", checker.NoProgressCount())
	}

	// No progress for 2 iterations
	checker.Check(3, 1.5, 50.0, false)
	if checker.NoProgressCount() != 2 {
		t.Fatalf("expected no progress count 2, got %d", checker.NoProgressCount())
	}

	// No progress for 3 iterations - should converge
	reason, stop := checker.Check(4, 2.0, 50.0, false)
	if !stop {
		t.Fatal("expected stop after 3 iterations without progress")
	}
	if reason != StopReasonConverged {
		t.Fatalf("expected StopReasonConverged, got %s", reason)
	}
}

func TestStopChecker_ProgressResetsCount(t *testing.T) {
	config := StopConfig{
		MaxIterations:   100,
		BudgetLimit:     100.0,
		NoProgressLimit: 3,
	}
	checker := NewStopChecker(config)

	// No progress for 2 iterations
	checker.Check(1, 0.5, 50.0, false)
	checker.Check(2, 1.0, 50.0, false)
	if checker.NoProgressCount() != 2 {
		t.Fatalf("expected no progress count 2, got %d", checker.NoProgressCount())
	}

	// Progress made - should reset
	checker.Check(3, 1.5, 60.0, true)
	if checker.NoProgressCount() != 0 {
		t.Fatalf("expected no progress count reset to 0, got %d", checker.NoProgressCount())
	}

	// No progress again - should start from 0
	checker.Check(4, 2.0, 60.0, false)
	if checker.NoProgressCount() != 1 {
		t.Fatalf("expected no progress count 1, got %d", checker.NoProgressCount())
	}
}

func TestStopChecker_CompleteTakesPriority(t *testing.T) {
	config := StopConfig{
		MaxIterations:   1,   // Would trigger max iterations
		BudgetLimit:     0.1, // Would trigger budget exceeded
		NoProgressLimit: 1,   // Would trigger convergence
	}
	checker := NewStopChecker(config)

	// All conditions met, but complete should take priority
	reason, stop := checker.Check(1, 1.0, 100.0, false)
	if !stop {
		t.Fatal("expected stop")
	}
	if reason != StopReasonComplete {
		t.Fatalf("expected StopReasonComplete to take priority, got %s", reason)
	}
}

func TestStopChecker_NoLimits(t *testing.T) {
	config := StopConfig{
		MaxIterations:   0,
		BudgetLimit:     0,
		NoProgressLimit: 0,
	}
	checker := NewStopChecker(config)

	// With no limits set, only completion should stop
	reason, stop := checker.Check(1000, 10000.0, 99.0, false)
	if stop {
		t.Fatal("should not stop with no limits set and < 100% complete")
	}
	if reason != StopReasonNone {
		t.Fatalf("expected StopReasonNone, got %s", reason)
	}

	// But 100% complete should still stop
	reason, stop = checker.Check(1001, 10001.0, 100.0, false)
	if !stop {
		t.Fatal("expected stop on 100% completion even with no limits")
	}
	if reason != StopReasonComplete {
		t.Fatalf("expected StopReasonComplete, got %s", reason)
	}
}

func TestStopChecker_IterationsCompleted(t *testing.T) {
	checker := NewStopChecker(DefaultStopConfig())

	checker.Check(1, 0.5, 50.0, true)
	if checker.IterationsCompleted() != 1 {
		t.Fatalf("expected 1, got %d", checker.IterationsCompleted())
	}

	checker.Check(5, 2.5, 70.0, true)
	if checker.IterationsCompleted() != 5 {
		t.Fatalf("expected 5, got %d", checker.IterationsCompleted())
	}
}

func TestStopChecker_Reset(t *testing.T) {
	checker := NewStopChecker(DefaultStopConfig())

	// Accumulate some state
	checker.Check(5, 2.5, 70.0, false)
	checker.Check(6, 3.0, 70.0, false)

	if checker.NoProgressCount() == 0 {
		t.Fatal("expected non-zero no progress count before reset")
	}
	if checker.IterationsCompleted() == 0 {
		t.Fatal("expected non-zero iterations before reset")
	}

	// Reset
	checker.Reset()

	if checker.NoProgressCount() != 0 {
		t.Fatalf("expected 0 no progress count after reset, got %d", checker.NoProgressCount())
	}
	if checker.IterationsCompleted() != 0 {
		t.Fatalf("expected 0 iterations after reset, got %d", checker.IterationsCompleted())
	}
}

func TestDefaultStopConfig(t *testing.T) {
	config := DefaultStopConfig()

	if config.MaxIterations <= 0 {
		t.Fatal("expected positive MaxIterations in default config")
	}
	if config.BudgetLimit <= 0 {
		t.Fatal("expected positive BudgetLimit in default config")
	}
	if config.NoProgressLimit <= 0 {
		t.Fatal("expected positive NoProgressLimit in default config")
	}
}
