package learning

import (
	"testing"
	"time"
)

func TestNewLifecycleManager(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	lm := NewLifecycleManager(store, 30*24*time.Hour)
	if lm == nil {
		t.Fatal("NewLifecycleManager() returned nil")
	}

	if lm.DefaultTTLDuration() != 30*24*time.Hour {
		t.Errorf("DefaultTTLDuration() = %v, want %v", lm.DefaultTTLDuration(), 30*24*time.Hour)
	}
}

func TestNewLifecycleManager_DefaultTTL(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	lm := NewLifecycleManager(store, 0) // Use 0 for default
	if lm == nil {
		t.Fatal("NewLifecycleManager() returned nil")
	}

	if lm.DefaultTTLDuration() != DefaultTTL {
		t.Errorf("DefaultTTLDuration() = %v, want %v", lm.DefaultTTLDuration(), DefaultTTL)
	}
}

func TestLifecycleManager_RecordTrigger(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learning := &Learning{
		ID:           "test-1",
		Condition:    "test condition",
		Action:       "test action",
		Outcome:      "test outcome",
		Scope:        "repo",
		OutcomeType:  "success",
		TriggerCount: 0,
		CreatedAt:    now,
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	lm := NewLifecycleManager(store, 30*24*time.Hour)

	// Record trigger
	if err := lm.RecordTrigger("test-1"); err != nil {
		t.Fatalf("RecordTrigger() error = %v, want nil", err)
	}

	// Verify trigger was recorded
	got, err := store.Get("test-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.TriggerCount != 1 {
		t.Errorf("TriggerCount = %v, want 1", got.TriggerCount)
	}

	if got.LastTriggered.IsZero() {
		t.Error("LastTriggered should not be zero after trigger")
	}
}

func TestLifecycleManager_RecordTrigger_Increment(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learning := &Learning{
		ID:           "test-1",
		Condition:    "test condition",
		Action:       "test action",
		Outcome:      "test outcome",
		Scope:        "repo",
		OutcomeType:  "success",
		TriggerCount: 5,
		CreatedAt:    now,
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	lm := NewLifecycleManager(store, 30*24*time.Hour)

	// Record trigger
	if err := lm.RecordTrigger("test-1"); err != nil {
		t.Fatalf("RecordTrigger() error = %v", err)
	}

	// Verify increment
	got, err := store.Get("test-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.TriggerCount != 6 {
		t.Errorf("TriggerCount = %v, want 6", got.TriggerCount)
	}
}

func TestLifecycleManager_RecordTrigger_NotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	lm := NewLifecycleManager(store, 30*24*time.Hour)

	err := lm.RecordTrigger("nonexistent")
	if err == nil {
		t.Error("RecordTrigger() error = nil, want error for nonexistent ID")
	}
}

func TestLifecycleManager_CleanupStale_CustomTTL(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()

	// Create learnings with different TTLs
	learnings := []*Learning{
		{
			ID:            "stale-custom-ttl",
			Condition:     "test condition 1",
			Action:        "test action 1",
			Outcome:       "test outcome 1",
			Scope:         "repo",
			OutcomeType:   "success",
			TTL:           1 * time.Hour, // 1 hour TTL
			LastTriggered: now.Add(-2 * time.Hour), // 2 hours ago - stale
			TriggerCount:  1,
			CreatedAt:     now.Add(-24 * time.Hour),
		},
		{
			ID:            "fresh-custom-ttl",
			Condition:     "test condition 2",
			Action:        "test action 2",
			Outcome:       "test outcome 2",
			Scope:         "repo",
			OutcomeType:   "success",
			TTL:           24 * time.Hour, // 24 hour TTL
			LastTriggered: now.Add(-1 * time.Hour), // 1 hour ago - fresh
			TriggerCount:  1,
			CreatedAt:     now.Add(-24 * time.Hour),
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	lm := NewLifecycleManager(store, DefaultTTL)

	// Cleanup stale learnings
	deleted, err := lm.CleanupStale()
	if err != nil {
		t.Fatalf("CleanupStale() error = %v, want nil", err)
	}

	if deleted != 1 {
		t.Errorf("CleanupStale() deleted = %v, want 1", deleted)
	}

	// Verify stale was deleted
	stale, _ := store.Get("stale-custom-ttl")
	if stale != nil {
		t.Error("Stale learning should have been deleted")
	}

	// Verify fresh was kept
	fresh, _ := store.Get("fresh-custom-ttl")
	if fresh == nil {
		t.Error("Fresh learning should NOT have been deleted")
	}
}

func TestLifecycleManager_CleanupStale_DefaultTTL(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	shortTTL := 1 * time.Hour // Use short TTL for testing

	// Create learning with TTL=0 (uses default)
	learning := &Learning{
		ID:            "stale-default-ttl",
		Condition:     "test condition",
		Action:        "test action",
		Outcome:       "test outcome",
		Scope:         "repo",
		OutcomeType:   "success",
		TTL:           0, // Uses default TTL
		LastTriggered: now.Add(-2 * time.Hour), // 2 hours ago
		TriggerCount:  1,
		CreatedAt:     now.Add(-24 * time.Hour),
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	lm := NewLifecycleManager(store, shortTTL) // 1 hour default TTL

	// Cleanup stale learnings
	deleted, err := lm.CleanupStale()
	if err != nil {
		t.Fatalf("CleanupStale() error = %v, want nil", err)
	}

	if deleted != 1 {
		t.Errorf("CleanupStale() deleted = %v, want 1", deleted)
	}

	// Verify stale was deleted
	stale, _ := store.Get("stale-default-ttl")
	if stale != nil {
		t.Error("Stale learning with default TTL should have been deleted")
	}
}

func TestLifecycleManager_CleanupStale_UsesCreatedAtForNeverTriggered(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	shortTTL := 1 * time.Hour

	// Create learning that was never triggered
	learning := &Learning{
		ID:           "never-triggered",
		Condition:    "test condition",
		Action:       "test action",
		Outcome:      "test outcome",
		Scope:        "repo",
		OutcomeType:  "success",
		TTL:          0, // Uses default TTL
		TriggerCount: 0, // Never triggered
		CreatedAt:    now.Add(-2 * time.Hour), // 2 hours ago
		// LastTriggered is zero
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	lm := NewLifecycleManager(store, shortTTL)

	deleted, err := lm.CleanupStale()
	if err != nil {
		t.Fatalf("CleanupStale() error = %v", err)
	}

	if deleted != 1 {
		t.Errorf("CleanupStale() deleted = %v, want 1 (never triggered, past TTL)", deleted)
	}
}

func TestLifecycleManager_CleanupStale_NoStale(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()

	// Create fresh learning
	learning := &Learning{
		ID:            "fresh",
		Condition:     "test condition",
		Action:        "test action",
		Outcome:       "test outcome",
		Scope:         "repo",
		OutcomeType:   "success",
		TTL:           24 * time.Hour,
		LastTriggered: now.Add(-1 * time.Hour),
		TriggerCount:  1,
		CreatedAt:     now.Add(-1 * time.Hour),
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	lm := NewLifecycleManager(store, DefaultTTL)

	deleted, err := lm.CleanupStale()
	if err != nil {
		t.Fatalf("CleanupStale() error = %v, want nil", err)
	}

	if deleted != 0 {
		t.Errorf("CleanupStale() deleted = %v, want 0", deleted)
	}
}

func TestLifecycleManager_GetHealthStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	shortTTL := 1 * time.Hour

	learnings := []*Learning{
		{
			ID:            "active-1",
			Condition:     "test 1",
			Action:        "action 1",
			Outcome:       "outcome 1",
			Scope:         "repo",
			OutcomeType:   "success",
			TTL:           24 * time.Hour,
			LastTriggered: now.Add(-30 * time.Minute),
			TriggerCount:  1,
			CreatedAt:     now.Add(-1 * time.Hour),
		},
		{
			ID:            "active-2",
			Condition:     "test 2",
			Action:        "action 2",
			Outcome:       "outcome 2",
			Scope:         "repo",
			OutcomeType:   "failure",
			TTL:           24 * time.Hour,
			LastTriggered: now.Add(-30 * time.Minute),
			TriggerCount:  1,
			CreatedAt:     now.Add(-1 * time.Hour),
		},
		{
			ID:            "stale-1",
			Condition:     "test 3",
			Action:        "action 3",
			Outcome:       "outcome 3",
			Scope:         "repo",
			OutcomeType:   "neutral",
			TTL:           0, // Uses default (short TTL)
			LastTriggered: now.Add(-2 * time.Hour),
			TriggerCount:  1,
			CreatedAt:     now.Add(-3 * time.Hour),
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	lm := NewLifecycleManager(store, shortTTL)

	stats, err := lm.GetHealthStats()
	if err != nil {
		t.Fatalf("GetHealthStats() error = %v, want nil", err)
	}

	if stats.Total != 3 {
		t.Errorf("Total = %v, want 3", stats.Total)
	}

	if stats.Stale != 1 {
		t.Errorf("Stale = %v, want 1", stats.Stale)
	}

	if stats.Active != 2 {
		t.Errorf("Active = %v, want 2", stats.Active)
	}

	// Check outcome type distribution
	if stats.ByOutcomeType["success"] != 1 {
		t.Errorf("ByOutcomeType[success] = %v, want 1", stats.ByOutcomeType["success"])
	}
	if stats.ByOutcomeType["failure"] != 1 {
		t.Errorf("ByOutcomeType[failure] = %v, want 1", stats.ByOutcomeType["failure"])
	}
	if stats.ByOutcomeType["neutral"] != 1 {
		t.Errorf("ByOutcomeType[neutral] = %v, want 1", stats.ByOutcomeType["neutral"])
	}
}

func TestLifecycleManager_GetHealthStats_Empty(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	lm := NewLifecycleManager(store, DefaultTTL)

	stats, err := lm.GetHealthStats()
	if err != nil {
		t.Fatalf("GetHealthStats() error = %v, want nil", err)
	}

	if stats.Total != 0 {
		t.Errorf("Total = %v, want 0", stats.Total)
	}
	if stats.Stale != 0 {
		t.Errorf("Stale = %v, want 0", stats.Stale)
	}
	if stats.Active != 0 {
		t.Errorf("Active = %v, want 0", stats.Active)
	}
}
