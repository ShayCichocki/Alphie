package agent

import (
	"testing"

	"github.com/shayc/alphie/pkg/models"
)

func TestNewIterationController_TierLimits(t *testing.T) {
	tests := []struct {
		name          string
		tier          models.Tier
		wantMaxIter   int
		wantThreshold int
	}{
		{
			name:          "Scout tier",
			tier:          models.TierScout,
			wantMaxIter:   3,
			wantThreshold: 5,
		},
		{
			name:          "Builder tier",
			tier:          models.TierBuilder,
			wantMaxIter:   5,
			wantThreshold: 7,
		},
		{
			name:          "Architect tier",
			tier:          models.TierArchitect,
			wantMaxIter:   7,
			wantThreshold: 8,
		},
		{
			name:          "Unknown tier",
			tier:          models.Tier("unknown"),
			wantMaxIter:   3,
			wantThreshold: 5,
		},
		{
			name:          "Empty tier",
			tier:          models.Tier(""),
			wantMaxIter:   3,
			wantThreshold: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ic := NewIterationController(tt.tier)
			if ic.GetMaxIterations() != tt.wantMaxIter {
				t.Errorf("MaxIterations = %d, want %d", ic.GetMaxIterations(), tt.wantMaxIter)
			}
			if ic.GetThreshold() != tt.wantThreshold {
				t.Errorf("Threshold = %d, want %d", ic.GetThreshold(), tt.wantThreshold)
			}
			if ic.GetIteration() != 0 {
				t.Errorf("Initial iteration = %d, want 0", ic.GetIteration())
			}
		})
	}
}

func TestIterationController_ShouldContinue_ScoreThreshold(t *testing.T) {
	tests := []struct {
		name  string
		tier  models.Tier
		score *models.RubricScore
		want  bool
	}{
		{
			name:  "Scout nil score",
			tier:  models.TierScout,
			score: nil,
			want:  true,
		},
		{
			name: "Scout below threshold",
			tier: models.TierScout,
			score: &models.RubricScore{
				Correctness: 1,
				Readability: 1,
				EdgeCases:   1,
			},
			want: true,
		},
		{
			name: "Scout at threshold",
			tier: models.TierScout,
			score: &models.RubricScore{
				Correctness: 2,
				Readability: 2,
				EdgeCases:   1,
			},
			want: false,
		},
		{
			name: "Scout above threshold",
			tier: models.TierScout,
			score: &models.RubricScore{
				Correctness: 3,
				Readability: 3,
				EdgeCases:   3,
			},
			want: false,
		},
		{
			name: "Builder at threshold (7)",
			tier: models.TierBuilder,
			score: &models.RubricScore{
				Correctness: 3,
				Readability: 2,
				EdgeCases:   2,
			},
			want: false,
		},
		{
			name: "Architect at threshold (8)",
			tier: models.TierArchitect,
			score: &models.RubricScore{
				Correctness: 3,
				Readability: 3,
				EdgeCases:   2,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ic := NewIterationController(tt.tier)
			if got := ic.ShouldContinue(tt.score); got != tt.want {
				t.Errorf("ShouldContinue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIterationController_ShouldContinue_MaxIterations(t *testing.T) {
	tests := []struct {
		name       string
		tier       models.Tier
		iterations int
		want       bool
	}{
		{
			name:       "Scout at 0 iterations",
			tier:       models.TierScout,
			iterations: 0,
			want:       true,
		},
		{
			name:       "Scout at 2 iterations",
			tier:       models.TierScout,
			iterations: 2,
			want:       true,
		},
		{
			name:       "Scout at max (3)",
			tier:       models.TierScout,
			iterations: 3,
			want:       false,
		},
		{
			name:       "Builder at 4 iterations",
			tier:       models.TierBuilder,
			iterations: 4,
			want:       true,
		},
		{
			name:       "Builder at max (5)",
			tier:       models.TierBuilder,
			iterations: 5,
			want:       false,
		},
		{
			name:       "Architect at 6 iterations",
			tier:       models.TierArchitect,
			iterations: 6,
			want:       true,
		},
		{
			name:       "Architect at max (7)",
			tier:       models.TierArchitect,
			iterations: 7,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ic := NewIterationController(tt.tier)
			// Simulate iterations
			for i := 0; i < tt.iterations; i++ {
				ic.Increment()
			}
			// Use low score so threshold doesn't stop us
			lowScore := &models.RubricScore{
				Correctness: 1,
				Readability: 1,
				EdgeCases:   1,
			}
			if got := ic.ShouldContinue(lowScore); got != tt.want {
				t.Errorf("ShouldContinue() = %v, want %v (iterations=%d)", got, tt.want, ic.GetIteration())
			}
		})
	}
}

func TestIterationController_Increment(t *testing.T) {
	ic := NewIterationController(models.TierBuilder)

	for i := 0; i < 10; i++ {
		if ic.GetIteration() != i {
			t.Errorf("Iteration = %d, want %d", ic.GetIteration(), i)
		}
		ic.Increment()
	}

	if ic.GetIteration() != 10 {
		t.Errorf("Final iteration = %d, want 10", ic.GetIteration())
	}
}

func TestIterationController_IsAtMax(t *testing.T) {
	tests := []struct {
		name       string
		tier       models.Tier
		iterations int
		want       bool
	}{
		{
			name:       "Scout not at max",
			tier:       models.TierScout,
			iterations: 2,
			want:       false,
		},
		{
			name:       "Scout at max",
			tier:       models.TierScout,
			iterations: 3,
			want:       true,
		},
		{
			name:       "Scout past max",
			tier:       models.TierScout,
			iterations: 5,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ic := NewIterationController(tt.tier)
			for i := 0; i < tt.iterations; i++ {
				ic.Increment()
			}
			if got := ic.IsAtMax(); got != tt.want {
				t.Errorf("IsAtMax() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIterationController_ShouldContinue_ScoreAndMaxCombined(t *testing.T) {
	// Test that both conditions are checked
	ic := NewIterationController(models.TierScout) // max=3, threshold=5

	// High score should stop even at iteration 0
	highScore := &models.RubricScore{
		Correctness: 3,
		Readability: 3,
		EdgeCases:   3,
	}
	if ic.ShouldContinue(highScore) {
		t.Error("Should not continue with high score at iteration 0")
	}

	// Reset and test max iterations with low score
	ic = NewIterationController(models.TierScout)
	lowScore := &models.RubricScore{
		Correctness: 1,
		Readability: 1,
		EdgeCases:   1,
	}

	for i := 0; i < 3; i++ {
		ic.Increment()
	}
	if ic.ShouldContinue(lowScore) {
		t.Error("Should not continue at max iterations even with low score")
	}
}
