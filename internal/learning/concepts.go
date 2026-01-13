package learning

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Concept represents a learning category or concept.
// Concepts group related learnings together for better organization.
type Concept struct {
	ID        string    // Unique identifier
	Name      string    // Concept name (unique)
	Project   string    // Project scope (optional)
	Summary   string    // Description/summary of the concept
	CreatedAt time.Time // When the concept was created
}

// ConceptManager provides operations for managing concepts and their
// relationships to learnings.
type ConceptManager struct {
	store *LearningStore
}

// NewConceptManager creates a new ConceptManager backed by the given store.
func NewConceptManager(store *LearningStore) *ConceptManager {
	return &ConceptManager{store: store}
}

// Create inserts a new concept into the store.
func (m *ConceptManager) Create(concept *Concept) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	_, err := m.store.db.Exec(`
		INSERT INTO concepts (id, name, description, created_at)
		VALUES (?, ?, ?, ?)
	`,
		concept.ID,
		concept.Name,
		nullString(concept.Summary),
		formatTime(concept.CreatedAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("concept with name %q already exists", concept.Name)
		}
		return fmt.Errorf("insert concept: %w", err)
	}

	return nil
}

// Get retrieves a concept by its ID.
func (m *ConceptManager) Get(id string) (*Concept, error) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	var (
		concept   Concept
		summary   sql.NullString
		createdAt string
	)

	err := m.store.db.QueryRow(`
		SELECT id, name, description, created_at
		FROM concepts WHERE id = ?
	`, id).Scan(
		&concept.ID,
		&concept.Name,
		&summary,
		&createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query concept: %w", err)
	}

	concept.Summary = summary.String
	ca, _ := parseTime(createdAt)
	concept.CreatedAt = ca

	return &concept, nil
}

// GetByName retrieves a concept by its name.
func (m *ConceptManager) GetByName(name string) (*Concept, error) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	var (
		concept   Concept
		summary   sql.NullString
		createdAt string
	)

	err := m.store.db.QueryRow(`
		SELECT id, name, description, created_at
		FROM concepts WHERE name = ?
	`, name).Scan(
		&concept.ID,
		&concept.Name,
		&summary,
		&createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query concept by name: %w", err)
	}

	concept.Summary = summary.String
	ca, _ := parseTime(createdAt)
	concept.CreatedAt = ca

	return &concept, nil
}

// List returns all concepts ordered by name.
func (m *ConceptManager) List() ([]*Concept, error) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	rows, err := m.store.db.Query(`
		SELECT id, name, description, created_at
		FROM concepts
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list concepts: %w", err)
	}
	defer rows.Close()

	return scanConcepts(rows)
}

// Delete removes a concept from the store.
// Related learning_concepts entries are automatically deleted via CASCADE.
func (m *ConceptManager) Delete(id string) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	result, err := m.store.db.Exec("DELETE FROM concepts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete concept: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("concept not found: %s", id)
	}

	return nil
}

// AddLearningToConcept creates a relationship between a learning and a concept.
func (m *ConceptManager) AddLearningToConcept(learningID, conceptID string) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	_, err := m.store.db.Exec(`
		INSERT OR IGNORE INTO learning_concepts (learning_id, concept_id)
		VALUES (?, ?)
	`, learningID, conceptID)
	if err != nil {
		return fmt.Errorf("add learning to concept: %w", err)
	}

	return nil
}

// RemoveLearningFromConcept removes a relationship between a learning and a concept.
func (m *ConceptManager) RemoveLearningFromConcept(learningID, conceptID string) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	result, err := m.store.db.Exec(`
		DELETE FROM learning_concepts
		WHERE learning_id = ? AND concept_id = ?
	`, learningID, conceptID)
	if err != nil {
		return fmt.Errorf("remove learning from concept: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("learning-concept relationship not found")
	}

	return nil
}

// GetLearningsByConcept returns all learnings associated with a concept.
func (m *ConceptManager) GetLearningsByConcept(conceptID string) ([]*Learning, error) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	rows, err := m.store.db.Query(`
		SELECT l.id, l.condition, l.action, l.outcome, l.commit_hash, l.log_snippet_id,
			   l.scope, l.ttl_seconds, l.last_triggered, l.trigger_count, l.outcome_type, l.created_at
		FROM learnings l
		INNER JOIN learning_concepts lc ON l.id = lc.learning_id
		WHERE lc.concept_id = ?
		ORDER BY l.created_at DESC
	`, conceptID)
	if err != nil {
		return nil, fmt.Errorf("get learnings by concept: %w", err)
	}
	defer rows.Close()

	return scanLearnings(rows)
}

// GetConceptsForLearning returns all concepts associated with a learning.
func (m *ConceptManager) GetConceptsForLearning(learningID string) ([]*Concept, error) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	rows, err := m.store.db.Query(`
		SELECT c.id, c.name, c.description, c.created_at
		FROM concepts c
		INNER JOIN learning_concepts lc ON c.id = lc.concept_id
		WHERE lc.learning_id = ?
		ORDER BY c.name
	`, learningID)
	if err != nil {
		return nil, fmt.Errorf("get concepts for learning: %w", err)
	}
	defer rows.Close()

	return scanConcepts(rows)
}

// SuggestConcepts suggests relevant concepts based on content keywords.
// It performs a simple keyword match against concept names and summaries.
func (m *ConceptManager) SuggestConcepts(content string) ([]*Concept, error) {
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}

	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	// Extract words from content (simple tokenization)
	words := extractKeywords(content)
	if len(words) == 0 {
		return nil, nil
	}

	// Build query with LIKE clauses for each keyword
	var conditions []string
	var args []interface{}
	for _, word := range words {
		conditions = append(conditions, "(LOWER(name) LIKE ? OR LOWER(description) LIKE ?)")
		pattern := "%" + strings.ToLower(word) + "%"
		args = append(args, pattern, pattern)
	}

	query := fmt.Sprintf(`
		SELECT id, name, description, created_at
		FROM concepts
		WHERE %s
		ORDER BY name
	`, strings.Join(conditions, " OR "))

	rows, err := m.store.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("suggest concepts: %w", err)
	}
	defer rows.Close()

	return scanConcepts(rows)
}

// extractKeywords extracts significant words from content.
// It filters out short words and common stop words.
func extractKeywords(content string) []string {
	// Common stop words to filter out
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "being": true, "have": true, "has": true,
		"had": true, "do": true, "does": true, "did": true, "will": true,
		"would": true, "could": true, "should": true, "may": true, "might": true,
		"must": true, "shall": true, "can": true, "this": true, "that": true,
		"these": true, "those": true, "it": true, "its": true, "of": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"with": true, "by": true, "from": true, "as": true, "into": true,
		"through": true, "during": true, "before": true, "after": true,
		"above": true, "below": true, "between": true, "under": true,
		"when": true, "result": true, // Filter CAO markers too
	}

	// Split on non-alphanumeric characters
	words := strings.FieldsFunc(content, func(c rune) bool {
		return !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'))
	})

	var keywords []string
	seen := make(map[string]bool)

	for _, word := range words {
		lower := strings.ToLower(word)
		// Skip short words, stop words, and duplicates
		if len(lower) < 3 || stopWords[lower] || seen[lower] {
			continue
		}
		seen[lower] = true
		keywords = append(keywords, word)
	}

	return keywords
}

// scanConcepts scans rows into a slice of Concept pointers.
func scanConcepts(rows *sql.Rows) ([]*Concept, error) {
	var concepts []*Concept

	for rows.Next() {
		var (
			concept   Concept
			summary   sql.NullString
			createdAt string
		)

		err := rows.Scan(
			&concept.ID,
			&concept.Name,
			&summary,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan concept: %w", err)
		}

		concept.Summary = summary.String
		ca, _ := parseTime(createdAt)
		concept.CreatedAt = ca

		concepts = append(concepts, &concept)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate concepts: %w", err)
	}

	return concepts, nil
}
