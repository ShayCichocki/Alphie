// Package learning provides learning and context management capabilities.
package learning

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
	// Effectiveness tracking
	SuccessCount  int     // Number of successful task completions using this learning
	FailureCount  int     // Number of failed task completions using this learning
	Effectiveness float64 // Calculated effectiveness (success_count / total_uses)
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
