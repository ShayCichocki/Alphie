// Package learning provides learning and context management capabilities.
package learning

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Search performs a full-text search on condition, action, and outcome fields.
func (s *LearningStore) Search(query string) ([]*Learning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT l.id, l.condition, l.action, l.outcome, l.commit_hash, l.log_snippet_id,
			   l.scope, l.ttl_seconds, l.last_triggered, l.trigger_count, l.outcome_type, l.created_at
		FROM learnings l
		JOIN learnings_fts fts ON l.rowid = fts.rowid
		WHERE learnings_fts MATCH ?
		ORDER BY rank
	`, query)
	if err != nil {
		return nil, fmt.Errorf("search learnings: %w", err)
	}
	defer rows.Close()

	return scanLearnings(rows)
}

// SearchByScope performs a full-text search filtered by scope(s).
// Pass multiple scopes to include learnings from any of those scopes.
func (s *LearningStore) SearchByScope(query string, scopes []string) ([]*Learning, error) {
	if len(scopes) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build placeholder string for IN clause
	placeholders := make([]string, len(scopes))
	args := make([]interface{}, len(scopes)+1)
	args[0] = query
	for i, scope := range scopes {
		placeholders[i] = "?"
		args[i+1] = scope
	}

	sqlQuery := fmt.Sprintf(`
		SELECT l.id, l.condition, l.action, l.outcome, l.commit_hash, l.log_snippet_id,
			   l.scope, l.ttl_seconds, l.last_triggered, l.trigger_count, l.outcome_type, l.created_at
		FROM learnings l
		JOIN learnings_fts fts ON l.rowid = fts.rowid
		WHERE learnings_fts MATCH ? AND l.scope IN (%s)
		ORDER BY rank
	`, strings.Join(placeholders, ", "))

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search learnings by scope: %w", err)
	}
	defer rows.Close()

	return scanLearnings(rows)
}

// List returns the most recent learnings up to the specified limit.
func (s *LearningStore) List(limit int) ([]*Learning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, condition, action, outcome, commit_hash, log_snippet_id,
			   scope, ttl_seconds, last_triggered, trigger_count, outcome_type, created_at
		FROM learnings
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list learnings: %w", err)
	}
	defer rows.Close()

	return scanLearnings(rows)
}

// ListByScope returns the most recent learnings filtered by scope(s) up to the specified limit.
// Pass multiple scopes to include learnings from any of those scopes.
func (s *LearningStore) ListByScope(scopes []string, limit int) ([]*Learning, error) {
	if len(scopes) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build placeholder string for IN clause
	placeholders := make([]string, len(scopes))
	args := make([]interface{}, len(scopes)+1)
	for i, scope := range scopes {
		placeholders[i] = "?"
		args[i] = scope
	}
	args[len(scopes)] = limit

	query := fmt.Sprintf(`
		SELECT id, condition, action, outcome, commit_hash, log_snippet_id,
			   scope, ttl_seconds, last_triggered, trigger_count, outcome_type, created_at
		FROM learnings
		WHERE scope IN (%s)
		ORDER BY created_at DESC
		LIMIT ?
	`, strings.Join(placeholders, ", "))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list learnings by scope: %w", err)
	}
	defer rows.Close()

	return scanLearnings(rows)
}

// SearchByCondition searches learnings by matching the condition field.
func (s *LearningStore) SearchByCondition(pattern string) ([]*Learning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, condition, action, outcome, commit_hash, log_snippet_id,
			   scope, ttl_seconds, last_triggered, trigger_count, outcome_type, created_at
		FROM learnings
		WHERE condition LIKE ?
		ORDER BY created_at DESC
	`, "%"+pattern+"%")
	if err != nil {
		return nil, fmt.Errorf("search by condition: %w", err)
	}
	defer rows.Close()

	return scanLearnings(rows)
}

// SearchByConditionAndScope searches learnings by matching the condition field filtered by scope(s).
func (s *LearningStore) SearchByConditionAndScope(pattern string, scopes []string) ([]*Learning, error) {
	if len(scopes) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build placeholder string for IN clause
	placeholders := make([]string, len(scopes))
	args := make([]interface{}, len(scopes)+1)
	args[0] = "%" + pattern + "%"
	for i, scope := range scopes {
		placeholders[i] = "?"
		args[i+1] = scope
	}

	query := fmt.Sprintf(`
		SELECT id, condition, action, outcome, commit_hash, log_snippet_id,
			   scope, ttl_seconds, last_triggered, trigger_count, outcome_type, created_at
		FROM learnings
		WHERE condition LIKE ? AND scope IN (%s)
		ORDER BY created_at DESC
	`, strings.Join(placeholders, ", "))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("search by condition and scope: %w", err)
	}
	defer rows.Close()

	return scanLearnings(rows)
}

// SearchByPath searches learnings by file path prefix in the condition field.
func (s *LearningStore) SearchByPath(pathPrefix string) ([]*Learning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, condition, action, outcome, commit_hash, log_snippet_id,
			   scope, ttl_seconds, last_triggered, trigger_count, outcome_type, created_at
		FROM learnings
		WHERE condition LIKE ?
		ORDER BY created_at DESC
	`, "%"+pathPrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("search by path: %w", err)
	}
	defer rows.Close()

	return scanLearnings(rows)
}

// SearchByPathAndScope searches learnings by file path prefix filtered by scope(s).
func (s *LearningStore) SearchByPathAndScope(pathPrefix string, scopes []string) ([]*Learning, error) {
	if len(scopes) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build placeholder string for IN clause
	placeholders := make([]string, len(scopes))
	args := make([]interface{}, len(scopes)+1)
	args[0] = "%" + pathPrefix + "%"
	for i, scope := range scopes {
		placeholders[i] = "?"
		args[i+1] = scope
	}

	query := fmt.Sprintf(`
		SELECT id, condition, action, outcome, commit_hash, log_snippet_id,
			   scope, ttl_seconds, last_triggered, trigger_count, outcome_type, created_at
		FROM learnings
		WHERE condition LIKE ? AND scope IN (%s)
		ORDER BY created_at DESC
	`, strings.Join(placeholders, ", "))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("search by path and scope: %w", err)
	}
	defer rows.Close()

	return scanLearnings(rows)
}

// scanLearnings scans rows into a slice of Learning pointers.
func scanLearnings(rows *sql.Rows) ([]*Learning, error) {
	var learnings []*Learning

	for rows.Next() {
		var (
			learning      Learning
			ttlSeconds    int64
			lastTriggered sql.NullString
			commitHash    sql.NullString
			logSnippetID  sql.NullString
			createdAt     string
		)

		err := rows.Scan(
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
		if err != nil {
			return nil, fmt.Errorf("scan learning: %w", err)
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

		learnings = append(learnings, &learning)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate learnings: %w", err)
	}

	return learnings, nil
}
