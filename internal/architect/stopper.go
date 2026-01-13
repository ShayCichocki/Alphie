// Package architect provides components for the architect iteration loop.
package architect

// StopReason indicates why the architect loop should stop.
type StopReason string

const (
	// StopReasonNone indicates no stop condition has been met.
	StopReasonNone StopReason = ""
	// StopReasonMaxIterations indicates the maximum iteration count was reached.
	StopReasonMaxIterations StopReason = "max_iterations"
	// StopReasonBudgetExceeded indicates the cost budget was exceeded.
	StopReasonBudgetExceeded StopReason = "budget_exceeded"
	// StopReasonConverged indicates no progress for N iterations (convergence).
	StopReasonConverged StopReason = "converged"
	// StopReasonComplete indicates 100% completion was achieved.
	StopReasonComplete StopReason = "complete"
)

// StopConfig holds configuration for stop condition evaluation.
type StopConfig struct {
	// MaxIterations is the maximum number of iterations before stopping.
	// A value of 0 means no limit.
	MaxIterations int
	// BudgetLimit is the maximum cost allowed (in dollars).
	// A value of 0 means no limit.
	BudgetLimit float64
	// NoProgressLimit is the number of consecutive iterations without progress
	// before considering the loop converged. A value of 0 means no convergence check.
	NoProgressLimit int
}

// DefaultStopConfig returns a StopConfig with sensible defaults.
func DefaultStopConfig() StopConfig {
	return StopConfig{
		MaxIterations:   10,
		BudgetLimit:     5.0,
		NoProgressLimit: 3,
	}
}

// StopChecker evaluates stop conditions for the architect iteration loop.
type StopChecker struct {
	config              StopConfig
	noProgressCount     int
	lastCompletionPct   float64
	iterationsCompleted int
}

// NewStopChecker creates a new StopChecker with the given configuration.
func NewStopChecker(config StopConfig) *StopChecker {
	return &StopChecker{
		config:            config,
		noProgressCount:   0,
		lastCompletionPct: 0,
	}
}

// Check evaluates all stop conditions and returns whether to stop and why.
// Parameters:
//   - iteration: the current iteration number (1-based)
//   - cost: the cumulative cost so far
//   - completePct: the current completion percentage (0-100)
//   - progressMade: whether progress was made in this iteration
//
// Returns:
//   - StopReason: the reason for stopping, or StopReasonNone if continuing
//   - bool: true if the loop should stop, false otherwise
func (s *StopChecker) Check(iteration int, cost float64, completePct float64, progressMade bool) (StopReason, bool) {
	s.iterationsCompleted = iteration

	// Check for 100% completion first (most desirable outcome)
	if completePct >= 100.0 {
		return StopReasonComplete, true
	}

	// Check max iterations
	if s.config.MaxIterations > 0 && iteration >= s.config.MaxIterations {
		return StopReasonMaxIterations, true
	}

	// Check budget exceeded
	if s.config.BudgetLimit > 0 && cost >= s.config.BudgetLimit {
		return StopReasonBudgetExceeded, true
	}

	// Track progress for convergence detection
	if progressMade {
		s.noProgressCount = 0
	} else {
		s.noProgressCount++
	}
	s.lastCompletionPct = completePct

	// Check convergence (no progress for N iterations)
	if s.config.NoProgressLimit > 0 && s.noProgressCount >= s.config.NoProgressLimit {
		return StopReasonConverged, true
	}

	return StopReasonNone, false
}

// NoProgressCount returns the current count of iterations without progress.
func (s *StopChecker) NoProgressCount() int {
	return s.noProgressCount
}

// IterationsCompleted returns the number of iterations completed so far.
func (s *StopChecker) IterationsCompleted() int {
	return s.iterationsCompleted
}

// Reset resets the internal state of the StopChecker.
func (s *StopChecker) Reset() {
	s.noProgressCount = 0
	s.lastCompletionPct = 0
	s.iterationsCompleted = 0
}
