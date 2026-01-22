package prog

import (
	"encoding/json"
	"fmt"
	"time"
)

// CreateLearning inserts a new learning and its concept associations.
// Creates concepts that don't exist yet.
func (db *DB) CreateLearning(l *Learning) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize files to JSON
	filesJSON := "[]"
	if len(l.Files) > 0 {
		b, err := json.Marshal(l.Files)
		if err != nil {
			return fmt.Errorf("failed to marshal files: %w", err)
		}
		filesJSON = string(b)
	}

	// Insert learning
	_, err = tx.Exec(`
		INSERT INTO learnings (id, project, created_at, updated_at, task_id, summary, detail, files, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, l.ID, l.Project, l.CreatedAt, l.UpdatedAt, l.TaskID, l.Summary, l.Detail, filesJSON, l.Status)
	if err != nil {
		return fmt.Errorf("failed to insert learning: %w", err)
	}

	// Ensure concepts exist and create associations
	for _, conceptName := range l.Concepts {
		// Check if concept exists
		var conceptID string
		err = tx.QueryRow(`SELECT id FROM concepts WHERE name = ? AND project = ?`, conceptName, l.Project).Scan(&conceptID)
		if err != nil {
			// Concept doesn't exist, create it
			conceptID = GenerateConceptID()
			_, err = tx.Exec(`
				INSERT INTO concepts (id, name, project, last_updated)
				VALUES (?, ?, ?, ?)
			`, conceptID, conceptName, l.Project, l.UpdatedAt)
			if err != nil {
				return fmt.Errorf("failed to create concept %q: %w", conceptName, err)
			}
		} else {
			// Update last_updated
			_, err = tx.Exec(`UPDATE concepts SET last_updated = ? WHERE id = ?`, l.UpdatedAt, conceptID)
			if err != nil {
				return fmt.Errorf("failed to update concept %q: %w", conceptName, err)
			}
		}

		// Create association
		_, err = tx.Exec(`
			INSERT INTO learning_concepts (learning_id, concept_id)
			VALUES (?, ?)
		`, l.ID, conceptID)
		if err != nil {
			return fmt.Errorf("failed to create concept association: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetLearning retrieves a learning by ID.
func (db *DB) GetLearning(id string) (*Learning, error) {
	var l Learning
	var filesJSON string
	var taskID *string

	err := db.QueryRow(`
		SELECT id, project, created_at, updated_at, task_id, summary, detail, files, status
		FROM learnings WHERE id = ?
	`, id).Scan(&l.ID, &l.Project, &l.CreatedAt, &l.UpdatedAt, &taskID, &l.Summary, &l.Detail, &filesJSON, &l.Status)
	if err != nil {
		return nil, fmt.Errorf("learning not found: %s", id)
	}
	l.TaskID = taskID

	// Parse files JSON
	if filesJSON != "" && filesJSON != "[]" {
		if err := json.Unmarshal([]byte(filesJSON), &l.Files); err != nil {
			return nil, fmt.Errorf("failed to unmarshal files: %w", err)
		}
	}

	// Get associated concepts
	rows, err := db.Query(`
		SELECT c.name FROM learning_concepts lc
		JOIN concepts c ON c.id = lc.concept_id
		WHERE lc.learning_id = ?
	`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get concepts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var concept string
		if err := rows.Scan(&concept); err != nil {
			return nil, fmt.Errorf("failed to scan concept: %w", err)
		}
		l.Concepts = append(l.Concepts, concept)
	}

	return &l, nil
}

// GetCurrentTaskID returns the ID of the first in-progress task for a project.
// Returns nil if no task is in progress.
func (db *DB) GetCurrentTaskID(project string) (*string, error) {
	var taskID string
	err := db.QueryRow(`
		SELECT id FROM items
		WHERE status = 'in_progress' AND project = ?
		ORDER BY updated_at DESC
		LIMIT 1
	`, project).Scan(&taskID)
	if err != nil {
		return nil, nil // No task in progress, not an error
	}
	return &taskID, nil
}

// UpdateLearningSummary updates a learning's summary.
func (db *DB) UpdateLearningSummary(id, summary string) error {
	result, err := db.Exec(`
		UPDATE learnings SET summary = ?, updated_at = ?
		WHERE id = ?
	`, summary, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update learning: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("learning not found: %s", id)
	}
	return nil
}

// UpdateLearningStatus updates a learning's status (active, stale, archived).
func (db *DB) UpdateLearningStatus(id string, status LearningStatus) error {
	result, err := db.Exec(`
		UPDATE learnings SET status = ?, updated_at = ?
		WHERE id = ?
	`, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update learning status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("learning not found: %s", id)
	}
	return nil
}

// UpdateLearningDetail updates a learning's detail.
func (db *DB) UpdateLearningDetail(id, detail string) error {
	result, err := db.Exec(`
		UPDATE learnings SET detail = ?, updated_at = ?
		WHERE id = ?
	`, detail, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update learning detail: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("learning not found: %s", id)
	}
	return nil
}

// DeleteLearning removes a learning and its concept associations.
func (db *DB) DeleteLearning(id string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete concept associations
	_, err = tx.Exec(`DELETE FROM learning_concepts WHERE learning_id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete concept associations: %w", err)
	}

	// Delete learning
	result, err := tx.Exec(`DELETE FROM learnings WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete learning: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("learning not found: %s", id)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}
