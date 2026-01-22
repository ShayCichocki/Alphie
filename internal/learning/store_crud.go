// Package learning provides learning and context management capabilities.
package learning

import (
	"database/sql"
	"fmt"
	"time"
)

// Create inserts a new learning into the store.
func (s *LearningStore) Create(learning *Learning) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ttlSeconds := int64(learning.TTL.Seconds())
	var lastTriggered *string
	if !learning.LastTriggered.IsZero() {
		lt := formatTime(learning.LastTriggered)
		lastTriggered = &lt
	}

	_, err := s.db.Exec(`
		INSERT INTO learnings (
			id, condition, action, outcome, commit_hash, log_snippet_id,
			scope, ttl_seconds, last_triggered, trigger_count, outcome_type, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		learning.ID,
		learning.Condition,
		learning.Action,
		learning.Outcome,
		nullString(learning.CommitHash),
		nullString(learning.LogSnippetID),
		learning.Scope,
		ttlSeconds,
		lastTriggered,
		learning.TriggerCount,
		learning.OutcomeType,
		formatTime(learning.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert learning: %w", err)
	}

	return nil
}

// Get retrieves a learning by its ID.
func (s *LearningStore) Get(id string) (*Learning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var (
		learning      Learning
		ttlSeconds    int64
		lastTriggered sql.NullString
		commitHash    sql.NullString
		logSnippetID  sql.NullString
		createdAt     string
	)

	err := s.db.QueryRow(`
		SELECT id, condition, action, outcome, commit_hash, log_snippet_id,
			   scope, ttl_seconds, last_triggered, trigger_count, outcome_type, created_at
		FROM learnings WHERE id = ?
	`, id).Scan(
		&learning.ID,
		&learning.Condition,
		&learning.Action,
		&learning.Outcome,
		&commitHash,
		&logSnippetID,
		&learning.Scope,
		&ttlSeconds,
		&lastTriggered,
		&learning.TriggerCount,
		&learning.OutcomeType,
		&createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query learning: %w", err)
	}

	learning.TTL = time.Duration(ttlSeconds) * time.Second
	learning.CommitHash = commitHash.String
	learning.LogSnippetID = logSnippetID.String

	if lastTriggered.Valid {
		lt, _ := parseTime(lastTriggered.String)
		learning.LastTriggered = lt
	}

	ca, _ := parseTime(createdAt)
	learning.CreatedAt = ca

	return &learning, nil
}

// Update updates an existing learning in the store.
func (s *LearningStore) Update(learning *Learning) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ttlSeconds := int64(learning.TTL.Seconds())
	var lastTriggered *string
	if !learning.LastTriggered.IsZero() {
		lt := formatTime(learning.LastTriggered)
		lastTriggered = &lt
	}

	result, err := s.db.Exec(`
		UPDATE learnings SET
			condition = ?,
			action = ?,
			outcome = ?,
			commit_hash = ?,
			log_snippet_id = ?,
			scope = ?,
			ttl_seconds = ?,
			last_triggered = ?,
			trigger_count = ?,
			outcome_type = ?
		WHERE id = ?
	`,
		learning.Condition,
		learning.Action,
		learning.Outcome,
		nullString(learning.CommitHash),
		nullString(learning.LogSnippetID),
		learning.Scope,
		ttlSeconds,
		lastTriggered,
		learning.TriggerCount,
		learning.OutcomeType,
		learning.ID,
	)
	if err != nil {
		return fmt.Errorf("update learning: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("learning not found: %s", learning.ID)
	}

	return nil
}

// Delete removes a learning from the store.
func (s *LearningStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM learnings WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete learning: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("learning not found: %s", id)
	}

	return nil
}

// IncrementTriggerCount increments the trigger count and updates the last triggered time.
func (s *LearningStore) IncrementTriggerCount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		UPDATE learnings SET
			trigger_count = trigger_count + 1,
			last_triggered = ?
		WHERE id = ?
	`, formatTime(time.Now()), id)
	if err != nil {
		return fmt.Errorf("increment trigger count: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("learning not found: %s", id)
	}

	return nil
}
