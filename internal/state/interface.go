// Package state provides SQLite-based state management for Alphie.
package state

import "io"

// SessionStore handles session-related persistence operations.
type SessionStore interface {
	CreateSession(s *Session) error
	GetSession(id string) (*Session, error)
	UpdateSession(s *Session) error
	GetActiveSession() (*Session, error)
}

// AgentStore handles agent-related persistence operations.
type AgentStore interface {
	CreateAgent(a *Agent) error
	GetAgent(id string) (*Agent, error)
	UpdateAgent(a *Agent) error
	ListAgentsByTask(taskID string) ([]Agent, error)
}

// TaskStore handles task-related persistence operations.
type TaskStore interface {
	CreateTask(t *Task) error
	GetTask(id string) (*Task, error)
	UpdateTask(t *Task) error
	ListTasksByParent(parentID string) ([]Task, error)
}

// Migrator handles database schema migrations.
// Separating this allows clients to depend only on migration functionality.
type Migrator interface {
	// Migrate applies all pending schema migrations.
	Migrate() error
}

// StateStore defines the interface for state persistence.
// This interface allows the orchestrator to work with any state backend
// without depending on the concrete SQLite implementation.
// It composes focused sub-interfaces for better modularity.
type StateStore interface {
	io.Closer
	Migrator
	SessionStore
	AgentStore
	TaskStore
}

// Compile-time verification that DB implements all interfaces.
var (
	_ StateStore   = (*DB)(nil)
	_ Migrator     = (*DB)(nil)
	_ SessionStore = (*DB)(nil)
	_ AgentStore   = (*DB)(nil)
	_ TaskStore    = (*DB)(nil)
)
