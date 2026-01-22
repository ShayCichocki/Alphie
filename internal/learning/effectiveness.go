// Package learning provides learning effectiveness tracking.
package learning

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// TaskOutcome represents the result of a task execution with learnings applied.
type TaskOutcome struct {
	TaskID             string
	SessionID          string
	Outcome            string   // "success", "failure", "blocked"
	VerificationPassed bool     // Whether verification passed
	LearningsUsed      []string // IDs of learnings that were retrieved
	CreatedAt          time.Time
}

// EffectivenessTracker tracks task outcomes and updates learning effectiveness.
type EffectivenessTracker struct {
	store *LearningStore
}

// NewEffectivenessTracker creates a new effectiveness tracker.
func NewEffectivenessTracker(store *LearningStore) *EffectivenessTracker {
	return &EffectivenessTracker{
		store: store,
	}
}

// RecordOutcome records a task outcome and updates effectiveness for associated learnings.
func (et *EffectivenessTracker) RecordOutcome(outcome TaskOutcome) error {
	et.store.mu.Lock()
	defer et.store.mu.Unlock()

	// Start transaction
	tx, err := et.store.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Record the task outcome
	learningsJSON, _ := json.Marshal(outcome.LearningsUsed)
	verificationInt := 0
	if outcome.VerificationPassed {
		verificationInt = 1
	}

	_, err = tx.Exec(`
		INSERT OR REPLACE INTO task_outcomes (task_id, session_id, outcome, verification_passed, learnings_used, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, outcome.TaskID, outcome.SessionID, outcome.Outcome, verificationInt, string(learningsJSON), formatTime(outcome.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert task outcome: %w", err)
	}

	// Determine if this was a success or failure
	isSuccess := outcome.Outcome == "success" && outcome.VerificationPassed

	// Update effectiveness for each learning used
	for _, learningID := range outcome.LearningsUsed {
		if isSuccess {
			// Increment success count
			_, err = tx.Exec(`
				UPDATE learnings
				SET success_count = success_count + 1,
				    effectiveness = CAST(success_count + 1 AS REAL) / (success_count + failure_count + 1)
				WHERE id = ?
			`, learningID)
		} else {
			// Increment failure count
			_, err = tx.Exec(`
				UPDATE learnings
				SET failure_count = failure_count + 1,
				    effectiveness = CAST(success_count AS REAL) / (success_count + failure_count + 1)
				WHERE id = ?
			`, learningID)
		}

		if err != nil {
			return fmt.Errorf("update learning effectiveness: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// GetLearningEffectiveness retrieves effectiveness stats for a learning.
func (et *EffectivenessTracker) GetLearningEffectiveness(learningID string) (*EffectivenessStats, error) {
	et.store.mu.RLock()
	defer et.store.mu.RUnlock()

	var stats EffectivenessStats
	err := et.store.db.QueryRow(`
		SELECT success_count, failure_count, effectiveness
		FROM learnings
		WHERE id = ?
	`, learningID).Scan(&stats.SuccessCount, &stats.FailureCount, &stats.Effectiveness)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("learning not found: %s", learningID)
		}
		return nil, fmt.Errorf("query effectiveness: %w", err)
	}

	stats.TotalUses = stats.SuccessCount + stats.FailureCount
	return &stats, nil
}

// GetTopLearnings returns the most effective learnings.
func (et *EffectivenessTracker) GetTopLearnings(limit int) ([]Learning, error) {
	et.store.mu.RLock()
	defer et.store.mu.RUnlock()

	// Get learnings with at least 5 uses, sorted by effectiveness
	rows, err := et.store.db.Query(`
		SELECT id, condition, action, outcome, commit_hash, log_snippet_id, scope,
		       ttl_seconds, last_triggered, trigger_count, outcome_type, created_at,
		       success_count, failure_count, effectiveness
		FROM learnings
		WHERE (success_count + failure_count) >= 5
		ORDER BY effectiveness DESC, (success_count + failure_count) DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query top learnings: %w", err)
	}
	defer rows.Close()

	learnings := []Learning{}
	for rows.Next() {
		var l Learning
		var commitHash, logSnippetID, lastTriggered sql.NullString
		var createdAt string
		var ttlSeconds int64

		err := rows.Scan(
			&l.ID, &l.Condition, &l.Action, &l.Outcome,
			&commitHash, &logSnippetID, &l.Scope,
			&ttlSeconds, &lastTriggered, &l.TriggerCount, &l.OutcomeType, &createdAt,
			&l.SuccessCount, &l.FailureCount, &l.Effectiveness,
		)
		if err != nil {
			return nil, fmt.Errorf("scan learning: %w", err)
		}

		if commitHash.Valid {
			l.CommitHash = commitHash.String
		}
		if logSnippetID.Valid {
			l.LogSnippetID = logSnippetID.String
		}
		l.TTL = time.Duration(ttlSeconds) * time.Second
		l.CreatedAt, _ = parseTime(createdAt)
		if lastTriggered.Valid {
			l.LastTriggered, _ = parseTime(lastTriggered.String)
		}

		learnings = append(learnings, l)
	}

	return learnings, rows.Err()
}

// GetBottomLearnings returns the least effective learnings.
func (et *EffectivenessTracker) GetBottomLearnings(limit int) ([]Learning, error) {
	et.store.mu.RLock()
	defer et.store.mu.RUnlock()

	// Get learnings with at least 10 uses, sorted by lowest effectiveness
	rows, err := et.store.db.Query(`
		SELECT id, condition, action, outcome, commit_hash, log_snippet_id, scope,
		       ttl_seconds, last_triggered, trigger_count, outcome_type, created_at,
		       success_count, failure_count, effectiveness
		FROM learnings
		WHERE (success_count + failure_count) >= 10
		ORDER BY effectiveness ASC, (success_count + failure_count) DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query bottom learnings: %w", err)
	}
	defer rows.Close()

	learnings := []Learning{}
	for rows.Next() {
		var l Learning
		var commitHash, logSnippetID, lastTriggered sql.NullString
		var createdAt string
		var ttlSeconds int64

		err := rows.Scan(
			&l.ID, &l.Condition, &l.Action, &l.Outcome,
			&commitHash, &logSnippetID, &l.Scope,
			&ttlSeconds, &lastTriggered, &l.TriggerCount, &l.OutcomeType, &createdAt,
			&l.SuccessCount, &l.FailureCount, &l.Effectiveness,
		)
		if err != nil {
			return nil, fmt.Errorf("scan learning: %w", err)
		}

		if commitHash.Valid {
			l.CommitHash = commitHash.String
		}
		if logSnippetID.Valid {
			l.LogSnippetID = logSnippetID.String
		}
		l.TTL = time.Duration(ttlSeconds) * time.Second
		l.CreatedAt, _ = parseTime(createdAt)
		if lastTriggered.Valid {
			l.LastTriggered, _ = parseTime(lastTriggered.String)
		}

		learnings = append(learnings, l)
	}

	return learnings, rows.Err()
}

// GetLearningsForRetirement returns learnings that should be retired.
// Criteria:
// - Effectiveness < 0.3 after 10+ uses, OR
// - Effectiveness < 0.2 after 20+ uses (auto-retire)
func (et *EffectivenessTracker) GetLearningsForRetirement() ([]Learning, error) {
	et.store.mu.RLock()
	defer et.store.mu.RUnlock()

	rows, err := et.store.db.Query(`
		SELECT id, condition, action, outcome, commit_hash, log_snippet_id, scope,
		       ttl_seconds, last_triggered, trigger_count, outcome_type, created_at,
		       success_count, failure_count, effectiveness
		FROM learnings
		WHERE (
			(effectiveness < 0.3 AND (success_count + failure_count) >= 10) OR
			(effectiveness < 0.2 AND (success_count + failure_count) >= 20)
		)
		ORDER BY effectiveness ASC, (success_count + failure_count) DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query learnings for retirement: %w", err)
	}
	defer rows.Close()

	learnings := []Learning{}
	for rows.Next() {
		var l Learning
		var commitHash, logSnippetID, lastTriggered sql.NullString
		var createdAt string
		var ttlSeconds int64

		err := rows.Scan(
			&l.ID, &l.Condition, &l.Action, &l.Outcome,
			&commitHash, &logSnippetID, &l.Scope,
			&ttlSeconds, &lastTriggered, &l.TriggerCount, &l.OutcomeType, &createdAt,
			&l.SuccessCount, &l.FailureCount, &l.Effectiveness,
		)
		if err != nil {
			return nil, fmt.Errorf("scan learning: %w", err)
		}

		if commitHash.Valid {
			l.CommitHash = commitHash.String
		}
		if logSnippetID.Valid {
			l.LogSnippetID = logSnippetID.String
		}
		l.TTL = time.Duration(ttlSeconds) * time.Second
		l.CreatedAt, _ = parseTime(createdAt)
		if lastTriggered.Valid {
			l.LastTriggered, _ = parseTime(lastTriggered.String)
		}

		learnings = append(learnings, l)
	}

	return learnings, rows.Err()
}

// EffectivenessStats contains effectiveness statistics for a learning.
type EffectivenessStats struct {
	SuccessCount  int
	FailureCount  int
	TotalUses     int
	Effectiveness float64
}

// String returns a human-readable representation.
func (s *EffectivenessStats) String() string {
	return fmt.Sprintf("%.1f%% effective (%d/%d successes)",
		s.Effectiveness*100, s.SuccessCount, s.TotalUses)
}
