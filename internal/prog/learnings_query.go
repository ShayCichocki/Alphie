package prog

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GetLearningsByConcepts returns learnings that have any of the specified concepts.
// Only returns active learnings by default. Results are sorted by created_at desc.
func (db *DB) GetLearningsByConcepts(project string, conceptNames []string, includeStale bool) ([]Learning, error) {
	if len(conceptNames) == 0 {
		return nil, nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(conceptNames))
	args := make([]interface{}, 0, len(conceptNames)+2)
	args = append(args, project)
	for i, name := range conceptNames {
		placeholders[i] = "?"
		args = append(args, name)
	}

	statusFilter := "AND l.status = 'active'"
	if includeStale {
		statusFilter = "AND l.status IN ('active', 'stale')"
	}

	query := `
		SELECT DISTINCT l.id, l.project, l.created_at, l.updated_at, l.task_id,
			l.summary, l.detail, l.files, l.status
		FROM learnings l
		JOIN learning_concepts lc ON lc.learning_id = l.id
		JOIN concepts c ON c.id = lc.concept_id
		WHERE l.project = ? AND c.name IN (` + strings.Join(placeholders, ",") + `)
		` + statusFilter + `
		ORDER BY l.created_at DESC
	`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query learnings: %w", err)
	}
	defer rows.Close()

	return db.scanLearnings(rows)
}

// SearchLearnings performs full-text search on learnings.
// Returns learnings matching the query, sorted by relevance.
func (db *DB) SearchLearnings(project string, query string, includeStale bool) ([]Learning, error) {
	statusFilter := "AND l.status = 'active'"
	if includeStale {
		statusFilter = "AND l.status IN ('active', 'stale')"
	}

	sqlQuery := `
		SELECT l.id, l.project, l.created_at, l.updated_at, l.task_id,
			l.summary, l.detail, l.files, l.status
		FROM learnings l
		JOIN learnings_fts fts ON l.rowid = fts.rowid
		WHERE learnings_fts MATCH ? AND l.project = ?
		` + statusFilter + `
		ORDER BY rank
	`

	rows, err := db.Query(sqlQuery, query, project)
	if err != nil {
		return nil, fmt.Errorf("failed to search learnings: %w", err)
	}
	defer rows.Close()

	return db.scanLearnings(rows)
}

// GetAllLearnings returns all learnings for a project, sorted by created_at desc.
// Only returns active learnings by default.
func (db *DB) GetAllLearnings(project string, includeStale bool) ([]Learning, error) {
	statusFilter := "AND l.status = 'active'"
	if includeStale {
		statusFilter = "AND l.status IN ('active', 'stale')"
	}

	query := `
		SELECT l.id, l.project, l.created_at, l.updated_at, l.task_id,
			l.summary, l.detail, l.files, l.status
		FROM learnings l
		WHERE l.project = ?
		` + statusFilter + `
		ORDER BY l.created_at DESC
	`

	rows, err := db.Query(query, project)
	if err != nil {
		return nil, fmt.Errorf("failed to query learnings: %w", err)
	}
	defer rows.Close()

	return db.scanLearnings(rows)
}

// scanLearnings is a helper to scan learning rows and fetch their concepts.
func (db *DB) scanLearnings(rows Rows) ([]Learning, error) {
	var learnings []Learning
	for rows.Next() {
		var l Learning
		var filesJSON string
		var taskID *string
		if err := rows.Scan(&l.ID, &l.Project, &l.CreatedAt, &l.UpdatedAt, &taskID,
			&l.Summary, &l.Detail, &filesJSON, &l.Status); err != nil {
			return nil, fmt.Errorf("failed to scan learning: %w", err)
		}
		l.TaskID = taskID

		// Parse files JSON
		if filesJSON != "" && filesJSON != "[]" {
			if err := json.Unmarshal([]byte(filesJSON), &l.Files); err != nil {
				return nil, fmt.Errorf("failed to unmarshal files: %w", err)
			}
		}

		// Get associated concepts
		conceptRows, err := db.Query(`
			SELECT c.name FROM learning_concepts lc
			JOIN concepts c ON c.id = lc.concept_id
			WHERE lc.learning_id = ?
		`, l.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get concepts: %w", err)
		}
		for conceptRows.Next() {
			var concept string
			if err := conceptRows.Scan(&concept); err != nil {
				conceptRows.Close()
				return nil, fmt.Errorf("failed to scan concept: %w", err)
			}
			l.Concepts = append(l.Concepts, concept)
		}
		conceptRows.Close()

		learnings = append(learnings, l)
	}

	return learnings, nil
}

// Rows interface allows mocking for scanLearnings.
type Rows interface {
	Next() bool
	Scan(dest ...interface{}) error
}
