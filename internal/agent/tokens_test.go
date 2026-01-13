package agent

import (
	"math"
	"sync"
	"testing"
)

func TestNewTokenTracker(t *testing.T) {
	tracker := NewTokenTracker("claude-sonnet-4-20250514")

	if tracker.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", tracker.Model, "claude-sonnet-4-20250514")
	}
	if tracker.Confidence != 1.0 {
		t.Errorf("Initial Confidence = %f, want 1.0", tracker.Confidence)
	}

	usage := tracker.GetUsage()
	if usage.TotalTokens != 0 {
		t.Errorf("Initial TotalTokens = %d, want 0", usage.TotalTokens)
	}
}

func TestTokenTrackerUpdate(t *testing.T) {
	tracker := NewTokenTracker("claude-sonnet-4-20250514")

	tracker.Update(MessageDeltaUsage{InputTokens: 100, OutputTokens: 50})

	usage := tracker.GetUsage()
	if usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", usage.OutputTokens)
	}
	if usage.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", usage.TotalTokens)
	}

	// Update again (cumulative)
	tracker.Update(MessageDeltaUsage{InputTokens: 200, OutputTokens: 100})

	usage = tracker.GetUsage()
	if usage.TotalTokens != 450 {
		t.Errorf("TotalTokens after second update = %d, want 450", usage.TotalTokens)
	}
}

func TestTokenTrackerUpdateSoft(t *testing.T) {
	tracker := NewTokenTracker("claude-sonnet-4-20250514")

	tracker.UpdateSoft(100, 50)

	softUsage := tracker.GetSoftUsage()
	if softUsage.InputTokens != 100 {
		t.Errorf("Soft InputTokens = %d, want 100", softUsage.InputTokens)
	}
	if softUsage.OutputTokens != 50 {
		t.Errorf("Soft OutputTokens = %d, want 50", softUsage.OutputTokens)
	}

	// Hard usage should be zero
	hardUsage := tracker.GetHardUsage()
	if hardUsage.TotalTokens != 0 {
		t.Errorf("Hard TotalTokens = %d, want 0", hardUsage.TotalTokens)
	}
}

func TestTokenTrackerGetUsageCombined(t *testing.T) {
	tracker := NewTokenTracker("claude-sonnet-4-20250514")

	tracker.Update(MessageDeltaUsage{InputTokens: 100, OutputTokens: 50})
	tracker.UpdateSoft(200, 100)

	usage := tracker.GetUsage()
	if usage.InputTokens != 300 {
		t.Errorf("Combined InputTokens = %d, want 300", usage.InputTokens)
	}
	if usage.OutputTokens != 150 {
		t.Errorf("Combined OutputTokens = %d, want 150", usage.OutputTokens)
	}
	if usage.TotalTokens != 450 {
		t.Errorf("Combined TotalTokens = %d, want 450", usage.TotalTokens)
	}
}

func TestTokenTrackerConfidence(t *testing.T) {
	tests := []struct {
		name             string
		hardInput        int64
		hardOutput       int64
		softInput        int64
		softOutput       int64
		expectedConf     float64
		tolerancePercent float64
	}{
		{
			name:         "all hard tokens",
			hardInput:    100,
			hardOutput:   50,
			expectedConf: 1.0,
		},
		{
			name:         "all soft tokens",
			softInput:    100,
			softOutput:   50,
			expectedConf: 0.0,
		},
		{
			name:             "50/50 split",
			hardInput:        50,
			hardOutput:       50,
			softInput:        50,
			softOutput:       50,
			expectedConf:     0.5,
			tolerancePercent: 0.01,
		},
		{
			name:             "75% hard",
			hardInput:        75,
			hardOutput:       75,
			softInput:        25,
			softOutput:       25,
			expectedConf:     0.75,
			tolerancePercent: 0.01,
		},
		{
			name:         "no tokens yet",
			expectedConf: 1.0, // Default is 1.0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewTokenTracker("claude-sonnet-4-20250514")

			if tt.hardInput > 0 || tt.hardOutput > 0 {
				tracker.Update(MessageDeltaUsage{InputTokens: tt.hardInput, OutputTokens: tt.hardOutput})
			}
			if tt.softInput > 0 || tt.softOutput > 0 {
				tracker.UpdateSoft(tt.softInput, tt.softOutput)
			}

			conf := tracker.GetConfidence()
			tolerance := tt.tolerancePercent
			if tolerance == 0 {
				tolerance = 0.001
			}

			if math.Abs(conf-tt.expectedConf) > tolerance {
				t.Errorf("GetConfidence() = %f, want %f (tolerance %f)", conf, tt.expectedConf, tolerance)
			}
		})
	}
}

func TestTokenTrackerGetCost(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		inputTokens  int64
		outputTokens int64
		expectedCost float64
		tolerance    float64
	}{
		{
			name:         "sonnet 1M tokens",
			model:        "claude-sonnet-4-20250514",
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			expectedCost: 3.00 + 15.00, // $3/1M input + $15/1M output
			tolerance:    0.01,
		},
		{
			name:         "opus 1M tokens",
			model:        "claude-opus-4-5-20251101",
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			expectedCost: 15.00 + 75.00, // $15/1M input + $75/1M output
			tolerance:    0.01,
		},
		{
			name:         "haiku 1M tokens",
			model:        "claude-3-5-haiku-20241022",
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			expectedCost: 0.80 + 4.00, // $0.80/1M input + $4/1M output
			tolerance:    0.01,
		},
		{
			name:         "sonnet small usage",
			model:        "claude-sonnet-4-20250514",
			inputTokens:  10_000,
			outputTokens: 5_000,
			expectedCost: 0.03 + 0.075, // $0.03 input + $0.075 output
			tolerance:    0.001,
		},
		{
			name:         "unknown model returns zero",
			model:        "unknown-model",
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			expectedCost: 0.0,
			tolerance:    0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewTokenTracker(tt.model)
			tracker.Update(MessageDeltaUsage{InputTokens: tt.inputTokens, OutputTokens: tt.outputTokens})

			cost := tracker.GetCost()
			if math.Abs(cost-tt.expectedCost) > tt.tolerance {
				t.Errorf("GetCost() = %f, want %f (tolerance %f)", cost, tt.expectedCost, tt.tolerance)
			}
		})
	}
}

func TestTokenTrackerSetPricing(t *testing.T) {
	tracker := NewTokenTracker("unknown-model")

	// Custom pricing
	tracker.SetPricing(ModelPricing{
		InputPerMillion:  5.00,
		OutputPerMillion: 10.00,
	})

	tracker.Update(MessageDeltaUsage{InputTokens: 1_000_000, OutputTokens: 1_000_000})

	cost := tracker.GetCost()
	expectedCost := 5.00 + 10.00

	if math.Abs(cost-expectedCost) > 0.01 {
		t.Errorf("GetCost() with custom pricing = %f, want %f", cost, expectedCost)
	}
}

func TestTokenTrackerCostWithSoftTokens(t *testing.T) {
	tracker := NewTokenTracker("claude-sonnet-4-20250514")

	tracker.Update(MessageDeltaUsage{InputTokens: 500_000, OutputTokens: 500_000})
	tracker.UpdateSoft(500_000, 500_000)

	cost := tracker.GetCost()
	// Total: 1M input + 1M output
	expectedCost := 3.00 + 15.00

	if math.Abs(cost-expectedCost) > 0.01 {
		t.Errorf("GetCost() with soft tokens = %f, want %f", cost, expectedCost)
	}
}

func TestTokenTrackerConcurrency(t *testing.T) {
	tracker := NewTokenTracker("claude-sonnet-4-20250514")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.Update(MessageDeltaUsage{InputTokens: 10, OutputTokens: 5})
			_ = tracker.GetUsage()
			_ = tracker.GetConfidence()
			_ = tracker.GetCost()
		}()
	}
	wg.Wait()

	usage := tracker.GetUsage()
	if usage.InputTokens != 1000 {
		t.Errorf("After concurrent updates, InputTokens = %d, want 1000", usage.InputTokens)
	}
}

func TestAggregateTracker(t *testing.T) {
	agg := NewAggregateTracker()

	t1 := NewTokenTracker("claude-sonnet-4-20250514")
	t1.Update(MessageDeltaUsage{InputTokens: 100, OutputTokens: 50})

	t2 := NewTokenTracker("claude-sonnet-4-20250514")
	t2.Update(MessageDeltaUsage{InputTokens: 200, OutputTokens: 100})

	agg.Add("agent-1", t1)
	agg.Add("agent-2", t2)

	usage := agg.GetUsage()
	if usage.InputTokens != 300 {
		t.Errorf("Aggregate InputTokens = %d, want 300", usage.InputTokens)
	}
	if usage.OutputTokens != 150 {
		t.Errorf("Aggregate OutputTokens = %d, want 150", usage.OutputTokens)
	}
}

func TestAggregateTrackerGetCost(t *testing.T) {
	agg := NewAggregateTracker()

	t1 := NewTokenTracker("claude-sonnet-4-20250514")
	t1.Update(MessageDeltaUsage{InputTokens: 1_000_000, OutputTokens: 1_000_000})

	t2 := NewTokenTracker("claude-opus-4-5-20251101")
	t2.Update(MessageDeltaUsage{InputTokens: 1_000_000, OutputTokens: 1_000_000})

	agg.Add("agent-1", t1)
	agg.Add("agent-2", t2)

	cost := agg.GetCost()
	// Sonnet: $3 + $15 = $18
	// Opus: $15 + $75 = $90
	// Total: $108
	expectedCost := 18.0 + 90.0

	if math.Abs(cost-expectedCost) > 0.01 {
		t.Errorf("Aggregate GetCost() = %f, want %f", cost, expectedCost)
	}
}

func TestAggregateTrackerGetConfidence(t *testing.T) {
	agg := NewAggregateTracker()

	t1 := NewTokenTracker("claude-sonnet-4-20250514")
	t1.Update(MessageDeltaUsage{InputTokens: 100, OutputTokens: 100}) // 200 hard tokens

	t2 := NewTokenTracker("claude-sonnet-4-20250514")
	t2.UpdateSoft(100, 100) // 200 soft tokens

	agg.Add("agent-1", t1)
	agg.Add("agent-2", t2)

	conf := agg.GetConfidence()
	// Weighted average: (1.0 * 200 + 0.0 * 200) / 400 = 0.5
	if math.Abs(conf-0.5) > 0.01 {
		t.Errorf("Aggregate GetConfidence() = %f, want 0.5", conf)
	}
}

func TestAggregateTrackerEmpty(t *testing.T) {
	agg := NewAggregateTracker()

	usage := agg.GetUsage()
	if usage.TotalTokens != 0 {
		t.Errorf("Empty aggregate TotalTokens = %d, want 0", usage.TotalTokens)
	}

	cost := agg.GetCost()
	if cost != 0.0 {
		t.Errorf("Empty aggregate cost = %f, want 0", cost)
	}

	conf := agg.GetConfidence()
	if conf != 1.0 {
		t.Errorf("Empty aggregate confidence = %f, want 1.0", conf)
	}
}

func TestAggregateTrackerRemove(t *testing.T) {
	agg := NewAggregateTracker()

	t1 := NewTokenTracker("claude-sonnet-4-20250514")
	t1.Update(MessageDeltaUsage{InputTokens: 100, OutputTokens: 50})

	agg.Add("agent-1", t1)
	agg.Remove("agent-1")

	if agg.Count() != 0 {
		t.Errorf("Count() after Remove() = %d, want 0", agg.Count())
	}
}

func TestAggregateTrackerGet(t *testing.T) {
	agg := NewAggregateTracker()

	t1 := NewTokenTracker("claude-sonnet-4-20250514")
	agg.Add("agent-1", t1)

	retrieved := agg.Get("agent-1")
	if retrieved != t1 {
		t.Error("Get() did not return the expected tracker")
	}

	notFound := agg.Get("nonexistent")
	if notFound != nil {
		t.Error("Get() for nonexistent agent should return nil")
	}
}

func TestAggregateTrackerCount(t *testing.T) {
	agg := NewAggregateTracker()

	if agg.Count() != 0 {
		t.Errorf("Initial Count() = %d, want 0", agg.Count())
	}

	agg.Add("agent-1", NewTokenTracker("model"))
	agg.Add("agent-2", NewTokenTracker("model"))

	if agg.Count() != 2 {
		t.Errorf("Count() after adding 2 = %d, want 2", agg.Count())
	}
}
