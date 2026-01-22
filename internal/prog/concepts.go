package prog

import (
	"fmt"
	"strings"
	"time"
)

// ListConcepts returns all concepts for a project, sorted by learning count (most used first).
func (db *DB) ListConcepts(project string, sortByRecent bool) ([]Concept, error) {
	orderBy := "count DESC, c.name"
	if sortByRecent {
		orderBy = "c.last_updated DESC, c.name"
	}

	rows, err := db.Query(`
		SELECT c.id, c.name, c.project, c.summary, c.last_updated,
			(SELECT COUNT(*) FROM learning_concepts lc WHERE lc.concept_id = c.id) as count
		FROM concepts c
		WHERE c.project = ?
		ORDER BY `+orderBy, project)
	if err != nil {
		return nil, fmt.Errorf("failed to list concepts: %w", err)
	}
	defer rows.Close()

	var concepts []Concept
	for rows.Next() {
		var c Concept
		var summary *string
		if err := rows.Scan(&c.ID, &c.Name, &c.Project, &summary, &c.LastUpdated, &c.LearningCount); err != nil {
			return nil, fmt.Errorf("failed to scan concept: %w", err)
		}
		if summary != nil {
			c.Summary = *summary
		}
		concepts = append(concepts, c)
	}

	return concepts, nil
}

// EnsureConcept creates a concept if it doesn't exist.
func (db *DB) EnsureConcept(name, project string) error {
	_, err := db.Exec(`
		INSERT INTO concepts (id, name, project, last_updated)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (name, project) DO NOTHING
	`, GenerateConceptID(), name, project, time.Now())
	return err
}

// SetConceptSummary updates a concept's summary.
func (db *DB) SetConceptSummary(name, project, summary string) error {
	result, err := db.Exec(`
		UPDATE concepts SET summary = ?, last_updated = ?
		WHERE name = ? AND project = ?
	`, summary, time.Now(), name, project)
	if err != nil {
		return fmt.Errorf("failed to update concept: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("concept not found: %s", name)
	}
	return nil
}

// RenameConcept changes a concept's name.
func (db *DB) RenameConcept(oldName, newName, project string) error {
	result, err := db.Exec(`
		UPDATE concepts SET name = ?, last_updated = ?
		WHERE name = ? AND project = ?
	`, newName, time.Now(), oldName, project)
	if err != nil {
		return fmt.Errorf("failed to rename concept: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("concept not found: %s", oldName)
	}
	return nil
}

// ConceptStats holds statistics for a concept.
type ConceptStats struct {
	Name          string
	LearningCount int
	OldestAge     *time.Duration // nil if no learnings
}

// ListConceptsWithStats returns all concepts with learning count and oldest learning age.
func (db *DB) ListConceptsWithStats(project string) ([]ConceptStats, error) {
	rows, err := db.Query(`
		SELECT c.name,
			COUNT(l.id) as count,
			MIN(l.created_at) as oldest
		FROM concepts c
		LEFT JOIN learning_concepts lc ON lc.concept_id = c.id
		LEFT JOIN learnings l ON l.id = lc.learning_id AND l.status = 'active'
		WHERE c.project = ?
		GROUP BY c.id
		ORDER BY count DESC, c.name
	`, project)
	if err != nil {
		return nil, fmt.Errorf("failed to list concept stats: %w", err)
	}
	defer rows.Close()

	var stats []ConceptStats
	now := time.Now()
	for rows.Next() {
		var s ConceptStats
		var oldestStr *string
		if err := rows.Scan(&s.Name, &s.LearningCount, &oldestStr); err != nil {
			return nil, fmt.Errorf("failed to scan concept stats: %w", err)
		}
		if oldestStr != nil && *oldestStr != "" {
			// Parse the timestamp string - Go's default format with monotonic clock suffix
			// Format: "2006-01-02 15:04:05.999999999 -0700 MST m=+0.000000000"
			str := *oldestStr
			// Strip monotonic clock suffix if present
			if idx := strings.Index(str, " m="); idx > 0 {
				str = str[:idx]
			}
			oldest, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", str)
			if err != nil {
				oldest, err = time.Parse(time.RFC3339Nano, str)
			}
			if err == nil {
				age := now.Sub(oldest)
				s.OldestAge = &age
			}
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// GetRelatedConcepts returns concepts that match keywords in a task's title/description.
// Matches are case-insensitive and ranked by learning count.
func (db *DB) GetRelatedConcepts(taskID string) ([]Concept, error) {
	// Get task details
	item, err := db.GetItem(taskID)
	if err != nil {
		return nil, err
	}

	// Get all concepts for this project
	concepts, err := db.ListConcepts(item.Project, false)
	if err != nil {
		return nil, err
	}

	if len(concepts) == 0 {
		return nil, nil
	}

	// Build search text from title and description
	searchText := strings.ToLower(item.Title + " " + item.Description)

	// Filter concepts whose name appears in the search text
	// Only include concepts that have at least one learning
	var related []Concept
	for _, c := range concepts {
		if c.LearningCount > 0 && strings.Contains(searchText, strings.ToLower(c.Name)) {
			related = append(related, c)
		}
	}

	return related, nil
}
