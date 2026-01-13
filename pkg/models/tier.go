package models

// Tier represents the agent tier for task execution.
type Tier string

const (
	// TierQuick is for simple, single-agent tasks with no decomposition.
	TierQuick Tier = "quick"
	// TierScout is for lightweight tasks like exploration and research.
	TierScout Tier = "scout"
	// TierBuilder is for standard implementation tasks.
	TierBuilder Tier = "builder"
	// TierArchitect is for complex design and architecture tasks.
	TierArchitect Tier = "architect"
)

// Valid returns true if the tier is a known value.
func (t Tier) Valid() bool {
	switch t {
	case TierQuick, TierScout, TierBuilder, TierArchitect:
		return true
	default:
		return false
	}
}
