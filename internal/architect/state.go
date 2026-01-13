// Package architect provides architecture document parsing and analysis.
package architect

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// Session represents an implementation session with crash recovery state.
type Session struct {
	ID             string
	ArchDoc        string
	Iteration      int
	TotalCost      float64
	Status         string
	CheckpointPath string
	StartedAt      time.Time
	UpdatedAt      time.Time
}

// StateStore manages implementation session state for crash recovery.
type StateStore struct {
	db *sql.DB
}

// NewStateStore creates a new StateStore with the given database path.
func NewStateStore(dbPath string) (*StateStore, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Create table if not exists
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS implement_sessions (
			id TEXT PRIMARY KEY,
			arch_doc TEXT,
			iteration INT,
			total_cost REAL,
			status TEXT,
			checkpoint_path TEXT,
			started_at DATETIME,
			updated_at DATETIME
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &StateStore{db: db}, nil
}

// CreateSession creates a new implementation session.
func (s *StateStore) CreateSession(archDoc string) (*Session, error) {
	now := time.Now()
	session := &Session{
		ID:        uuid.New().String(),
		ArchDoc:   archDoc,
		Iteration: 0,
		TotalCost: 0,
		Status:    "started",
		StartedAt: now,
		UpdatedAt: now,
	}

	_, err := s.db.Exec(`
		INSERT INTO implement_sessions (id, arch_doc, iteration, total_cost, status, checkpoint_path, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.ArchDoc, session.Iteration, session.TotalCost, session.Status, session.CheckpointPath, session.StartedAt, session.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}

	return session, nil
}

// UpdateSession updates an existing session.
func (s *StateStore) UpdateSession(session *Session) error {
	session.UpdatedAt = time.Now()

	result, err := s.db.Exec(`
		UPDATE implement_sessions
		SET arch_doc = ?, iteration = ?, total_cost = ?, status = ?, checkpoint_path = ?, updated_at = ?
		WHERE id = ?
	`, session.ArchDoc, session.Iteration, session.TotalCost, session.Status, session.CheckpointPath, session.UpdatedAt, session.ID)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("session not found: %s", session.ID)
	}

	return nil
}

// GetSession retrieves a session by ID.
func (s *StateStore) GetSession(id string) (*Session, error) {
	row := s.db.QueryRow(`
		SELECT id, arch_doc, iteration, total_cost, status, checkpoint_path, started_at, updated_at
		FROM implement_sessions
		WHERE id = ?
	`, id)

	var session Session
	var checkpointPath sql.NullString
	err := row.Scan(
		&session.ID,
		&session.ArchDoc,
		&session.Iteration,
		&session.TotalCost,
		&session.Status,
		&checkpointPath,
		&session.StartedAt,
		&session.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}

	if checkpointPath.Valid {
		session.CheckpointPath = checkpointPath.String
	}

	return &session, nil
}

// DeleteSession removes a session by ID.
func (s *StateStore) DeleteSession(id string) error {
	result, err := s.db.Exec(`DELETE FROM implement_sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// Close closes the database connection.
func (s *StateStore) Close() error {
	return s.db.Close()
}
