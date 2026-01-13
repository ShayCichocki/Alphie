// Package learning provides learning and context management capabilities.
package learning

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Learning represents a captured learning from a development session.
// It follows the WHEN-DO-RESULT pattern for encoding actionable knowledge.
type Learning struct {
	ID            string        // Unique identifier
	Condition     string        // WHEN: The triggering condition
	Action        string        // DO: The action to take
	Outcome       string        // RESULT: The expected outcome
	CommitHash    string        // Associated commit (optional)
	LogSnippetID  string        // Reference to log snippet (optional)
	Scope         string        // repo, module, or global
	TTL           time.Duration // Time-to-live (0 = permanent)
	LastTriggered time.Time     // Last time this learning was triggered
	TriggerCount  int           // Number of times triggered
	OutcomeType   string        // success, failure, neutral
	CreatedAt     time.Time     // When the learning was created
}

// LearningStore provides SQLite-backed storage for learnings.
type LearningStore struct {
	db     *sql.DB
	dbPath string
	mu     sync.RWMutex
}

// GlobalDBPath returns the path to the global Alphie learnings database.
func GlobalDBPath() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "alphie", "alphie.db")
}

// ProjectDBPath returns the path to the project-local learnings database.
func ProjectDBPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".alphie", "learnings.db")
}

// NewLearningStore creates a new LearningStore with the given database path.
// It creates the parent directories if they don't exist.
func NewLearningStore(dbPath string) (*LearningStore, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for concurrent reads
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := conn.Exec("PRAGMA foreign_keys=ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	store := &LearningStore{
		db:     conn,
		dbPath: dbPath,
	}

	return store, nil
}

// Close closes the database connection.
func (s *LearningStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// Path returns the path to the database file.
func (s *LearningStore) Path() string {
	return s.dbPath
}

// Migrate creates the necessary tables and indexes if they don't exist.
func (s *LearningStore) Migrate() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create schema version table
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS learning_schema_version (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	// Get current version
	var currentVersion int
	row := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM learning_schema_version")
	if err := row.Scan(&currentVersion); err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	// Apply migrations
	migrations := []struct {
		version int
		sql     string
	}{
		{1, migrationV1Learnings},
		{2, migrationV2Concepts},
	}

	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}

		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration v%d: %w", m.version, err)
		}

		if _, err := tx.Exec("INSERT INTO learning_schema_version (version) VALUES (?)", m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration v%d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration v%d: %w", m.version, err)
		}
	}

	return nil
}

// Migration SQL statements
const migrationV1Learnings = `
CREATE TABLE IF NOT EXISTS learnings (
	id TEXT PRIMARY KEY,
	condition TEXT NOT NULL,
	action TEXT NOT NULL,
	outcome TEXT NOT NULL,
	commit_hash TEXT,
	log_snippet_id TEXT,
	scope TEXT NOT NULL DEFAULT 'repo',
	ttl_seconds INTEGER NOT NULL DEFAULT 0,
	last_triggered DATETIME,
	trigger_count INTEGER NOT NULL DEFAULT 0,
	outcome_type TEXT NOT NULL DEFAULT 'neutral',
	created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_learnings_scope ON learnings(scope);
CREATE INDEX IF NOT EXISTS idx_learnings_outcome_type ON learnings(outcome_type);
CREATE INDEX IF NOT EXISTS idx_learnings_created_at ON learnings(created_at);

-- Full-text search on condition, action, outcome
CREATE VIRTUAL TABLE IF NOT EXISTS learnings_fts USING fts5(
	condition,
	action,
	outcome,
	content='learnings',
	content_rowid='rowid'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS learnings_ai AFTER INSERT ON learnings BEGIN
	INSERT INTO learnings_fts(rowid, condition, action, outcome)
	VALUES (NEW.rowid, NEW.condition, NEW.action, NEW.outcome);
END;

CREATE TRIGGER IF NOT EXISTS learnings_ad AFTER DELETE ON learnings BEGIN
	INSERT INTO learnings_fts(learnings_fts, rowid, condition, action, outcome)
	VALUES ('delete', OLD.rowid, OLD.condition, OLD.action, OLD.outcome);
END;

CREATE TRIGGER IF NOT EXISTS learnings_au AFTER UPDATE ON learnings BEGIN
	INSERT INTO learnings_fts(learnings_fts, rowid, condition, action, outcome)
	VALUES ('delete', OLD.rowid, OLD.condition, OLD.action, OLD.outcome);
	INSERT INTO learnings_fts(rowid, condition, action, outcome)
	VALUES (NEW.rowid, NEW.condition, NEW.action, NEW.outcome);
END;
`

const migrationV2Concepts = `
CREATE TABLE IF NOT EXISTS concepts (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT,
	created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS learning_concepts (
	learning_id TEXT NOT NULL,
	concept_id TEXT NOT NULL,
	PRIMARY KEY (learning_id, concept_id),
	FOREIGN KEY (learning_id) REFERENCES learnings(id) ON DELETE CASCADE,
	FOREIGN KEY (concept_id) REFERENCES concepts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_learning_concepts_concept ON learning_concepts(concept_id);
`

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

// Helper functions

// formatTime formats a time.Time for SQLite storage.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// parseTime parses a time string from SQLite.
func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// nullString converts a string to sql.NullString, treating empty as null.
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
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
