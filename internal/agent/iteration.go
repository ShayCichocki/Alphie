package agent

import (
	"sync"

	"github.com/shayc/alphie/internal/config"
	"github.com/shayc/alphie/pkg/models"
)

// IterationController tracks ralph-loop iterations per agent and enforces
// tier-based thresholds and hidden max iteration limits.
type IterationController struct {
	// currentIter tracks the current iteration number.
	currentIter int
	// maxIter is the hidden maximum iterations to prevent infinite loops.
	maxIter int
	// threshold is the score threshold required to pass (out of 9).
	threshold int
	// tier is the agent tier for this controller.
	tier models.Tier
}

// tierConfigInternal holds the configuration for each tier.
type tierConfigInternal struct {
	threshold int // Score threshold out of 9
	maxIter   int // Hidden max iterations
}

// tierConfigs maps each tier to its configuration.
// This can be updated by SetTierConfigs to use loaded YAML configurations.
var tierConfigs = map[models.Tier]tierConfigInternal{
	models.TierScout:     {threshold: 5, maxIter: 3},
	models.TierBuilder:   {threshold: 7, maxIter: 5},
	models.TierArchitect: {threshold: 8, maxIter: 7},
}

// tierConfigsMu protects tierConfigs from concurrent access.
var tierConfigsMu sync.RWMutex

// defaultConfig is used for unknown tiers.
var defaultConfig = tierConfigInternal{threshold: 5, maxIter: 3}

// SetTierConfigs updates the tier configurations from loaded config.
// This should be called at startup after loading configs/*.yaml.
func SetTierConfigs(configs *config.TierConfigs) {
	if configs == nil {
		return
	}

	tierConfigsMu.Lock()
	defer tierConfigsMu.Unlock()

	if configs.Scout != nil {
		tierConfigs[models.TierScout] = tierConfigInternal{
			threshold: configs.Scout.QualityThreshold,
			maxIter:   configs.Scout.MaxRalphIterations,
		}
	}
	if configs.Builder != nil {
		tierConfigs[models.TierBuilder] = tierConfigInternal{
			threshold: configs.Builder.QualityThreshold,
			maxIter:   configs.Builder.MaxRalphIterations,
		}
	}
	if configs.Architect != nil {
		tierConfigs[models.TierArchitect] = tierConfigInternal{
			threshold: configs.Architect.QualityThreshold,
			maxIter:   configs.Architect.MaxRalphIterations,
		}
	}
}

// GetTierConfig returns the internal tier configuration for a given tier.
// This is useful for testing and inspection.
func GetTierConfig(tier models.Tier) (threshold, maxIter int) {
	tierConfigsMu.RLock()
	defer tierConfigsMu.RUnlock()

	cfg, ok := tierConfigs[tier]
	if !ok {
		cfg = defaultConfig
	}
	return cfg.threshold, cfg.maxIter
}

// NewIterationController creates a new iteration controller for the given tier.
func NewIterationController(tier models.Tier) *IterationController {
	tierConfigsMu.RLock()
	cfg, ok := tierConfigs[tier]
	if !ok {
		cfg = defaultConfig
	}
	tierConfigsMu.RUnlock()

	return &IterationController{
		currentIter: 0,
		maxIter:     cfg.maxIter,
		threshold:   cfg.threshold,
		tier:        tier,
	}
}

// ShouldContinue returns true if the loop should continue iterating.
// It returns false when either:
// 1. The score meets or exceeds the tier threshold
// 2. The max iterations have been reached
func (ic *IterationController) ShouldContinue(score *models.RubricScore) bool {
	// If at max iterations, stop
	if ic.currentIter >= ic.maxIter {
		return false
	}

	// If no score provided, continue
	if score == nil {
		return true
	}

	// If score meets threshold, stop
	if score.Total() >= ic.threshold {
		return false
	}

	return true
}

// Increment increases the current iteration count by one.
func (ic *IterationController) Increment() {
	ic.currentIter++
}

// GetIteration returns the current iteration number.
func (ic *IterationController) GetIteration() int {
	return ic.currentIter
}

// IsAtMax returns true if the current iteration has reached the maximum.
func (ic *IterationController) IsAtMax() bool {
	return ic.currentIter >= ic.maxIter
}

// GetThreshold returns the score threshold for this tier.
func (ic *IterationController) GetThreshold() int {
	return ic.threshold
}

// GetMaxIterations returns the maximum iterations for this tier.
func (ic *IterationController) GetMaxIterations() int {
	return ic.maxIter
}
