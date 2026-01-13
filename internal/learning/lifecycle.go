// Package learning provides learning and context management capabilities.
package learning

import (
	"fmt"
	"time"
)

// DefaultTTL is the default time-to-live for learnings (90 days).
const DefaultTTL = 90 * 24 * time.Hour

// LifecycleStats contains statistics about the health of stored learnings.
type LifecycleStats struct {
	Total         int            // Total number of learnings
	Active        int            // Learnings triggered within TTL
	Stale         int            // Learnings past their TTL
	ByOutcomeType map[string]int // Distribution by outcome type
}

// LifecycleManager manages the lifecycle of learnings including
// TTL tracking, trigger counting, and cleanup of stale learnings.
type LifecycleManager struct {
	store      *LearningStore
	defaultTTL time.Duration
	now        func() time.Time // For testing
}

// NewLifecycleManager creates a new LifecycleManager with the given store
// and default TTL. If defaultTTL is 0, DefaultTTL (90 days) is used.
func NewLifecycleManager(store *LearningStore, defaultTTL time.Duration) *LifecycleManager {
	if defaultTTL == 0 {
		defaultTTL = DefaultTTL
	}
	return &LifecycleManager{
		store:      store,
		defaultTTL: defaultTTL,
		now:        time.Now,
	}
}

// RecordTrigger records that a learning was triggered.
// It updates LastTriggered to the current time and increments TriggerCount.
func (lm *LifecycleManager) RecordTrigger(learningID string) error {
	learning, err := lm.store.Get(learningID)
	if err != nil {
		return fmt.Errorf("get learning: %w", err)
	}
	if learning == nil {
		return fmt.Errorf("learning not found: %s", learningID)
	}

	learning.LastTriggered = lm.now()
	learning.TriggerCount++

	if err := lm.store.Update(learning); err != nil {
		return fmt.Errorf("update learning: %w", err)
	}

	return nil
}

// CleanupStale finds and deletes learnings that have exceeded their TTL.
// A learning is considered stale if:
// - It has a TTL > 0 and LastTriggered + TTL < now, OR
// - It has TTL == 0 (uses defaultTTL) and LastTriggered + defaultTTL < now
// Learnings that have never been triggered use CreatedAt instead.
// Returns the number of learnings deleted.
func (lm *LifecycleManager) CleanupStale() (int, error) {
	lm.store.mu.Lock()
	defer lm.store.mu.Unlock()

	now := lm.now()
	defaultTTLSeconds := int64(lm.defaultTTL.Seconds())

	// Find stale learnings
	// A learning is stale if:
	// - TTL > 0: last_triggered (or created_at) + TTL < now
	// - TTL == 0: last_triggered (or created_at) + defaultTTL < now
	result, err := lm.store.db.Exec(`
		DELETE FROM learnings
		WHERE (
			-- Has custom TTL and is expired
			(ttl_seconds > 0 AND
				datetime(COALESCE(last_triggered, created_at), '+' || ttl_seconds || ' seconds') < ?)
			OR
			-- Uses default TTL and is expired
			(ttl_seconds = 0 AND
				datetime(COALESCE(last_triggered, created_at), '+' || ? || ' seconds') < ?)
		)
	`, formatTime(now), defaultTTLSeconds, formatTime(now))
	if err != nil {
		return 0, fmt.Errorf("delete stale learnings: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get rows affected: %w", err)
	}

	return int(count), nil
}

// GetHealthStats returns statistics about the health of stored learnings.
func (lm *LifecycleManager) GetHealthStats() (*LifecycleStats, error) {
	lm.store.mu.RLock()
	defer lm.store.mu.RUnlock()

	stats := &LifecycleStats{
		ByOutcomeType: make(map[string]int),
	}

	now := lm.now()
	defaultTTLSeconds := int64(lm.defaultTTL.Seconds())

	// Get total count
	err := lm.store.db.QueryRow("SELECT COUNT(*) FROM learnings").Scan(&stats.Total)
	if err != nil {
		return nil, fmt.Errorf("count total: %w", err)
	}

	// Get stale count
	err = lm.store.db.QueryRow(`
		SELECT COUNT(*) FROM learnings
		WHERE (
			-- Has custom TTL and is expired
			(ttl_seconds > 0 AND
				datetime(COALESCE(last_triggered, created_at), '+' || ttl_seconds || ' seconds') < ?)
			OR
			-- Uses default TTL and is expired
			(ttl_seconds = 0 AND
				datetime(COALESCE(last_triggered, created_at), '+' || ? || ' seconds') < ?)
		)
	`, formatTime(now), defaultTTLSeconds, formatTime(now)).Scan(&stats.Stale)
	if err != nil {
		return nil, fmt.Errorf("count stale: %w", err)
	}

	stats.Active = stats.Total - stats.Stale

	// Get outcome type distribution
	rows, err := lm.store.db.Query(`
		SELECT outcome_type, COUNT(*)
		FROM learnings
		GROUP BY outcome_type
	`)
	if err != nil {
		return nil, fmt.Errorf("query outcome types: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var outcomeType string
		var count int
		if err := rows.Scan(&outcomeType, &count); err != nil {
			return nil, fmt.Errorf("scan outcome type: %w", err)
		}
		stats.ByOutcomeType[outcomeType] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outcome types: %w", err)
	}

	return stats, nil
}

// DefaultTTLDuration returns the default TTL configured for this manager.
func (lm *LifecycleManager) DefaultTTLDuration() time.Duration {
	return lm.defaultTTL
}
