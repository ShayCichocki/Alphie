package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SessionStatus represents the status of a session.
type SessionStatus string

const (
	SessionActive    SessionStatus = "active"
	SessionCompleted SessionStatus = "completed"
	SessionFailed    SessionStatus = "failed"
	SessionCanceled  SessionStatus = "canceled"
)

// AgentStatus represents the status of an agent.
type AgentStatus string

const (
	AgentPending         AgentStatus = "pending"
	AgentRunning         AgentStatus = "running"
	AgentPaused          AgentStatus = "paused"
	AgentWaitingApproval AgentStatus = "waiting_approval"
	AgentDone            AgentStatus = "done"
	AgentFailed          AgentStatus = "failed"
)

// TaskStatus represents the status of a task.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskBlocked    TaskStatus = "blocked"
	TaskDone       TaskStatus = "done"
	TaskCanceled   TaskStatus = "canceled"
)

// Session represents an Alphie orchestration session.
type Session struct {
	ID          string        `json:"id"`
	RootTask    string        `json:"root_task"`
	Tier        string        `json:"tier"`
	TokenBudget int           `json:"token_budget"`
	TokensUsed  int           `json:"tokens_used"`
	StartedAt   time.Time     `json:"started_at"`
	Status      SessionStatus `json:"status"`
}

// Agent represents a Claude Code agent working on a task.
type Agent struct {
	ID           string      `json:"id"`
	TaskID       string      `json:"task_id"`
	Status       AgentStatus `json:"status"`
	WorktreePath string      `json:"worktree_path"`
	PID          int         `json:"pid"`
	StartedAt    *time.Time  `json:"started_at"`
	TokensUsed   int         `json:"tokens_used"`
	Cost         float64     `json:"cost"`
	RalphIter    int         `json:"ralph_iter"`
	RalphScore   int         `json:"ralph_score"`
}

// Task represents a unit of work.
type Task struct {
	ID          string     `json:"id"`
	ParentID    string     `json:"parent_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	DependsOn   []string   `json:"depends_on"`
	AssignedTo  string     `json:"assigned_to"`
	Tier        string     `json:"tier"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// Session CRUD operations

// CreateSession creates a new session.
func (db *DB) CreateSession(s *Session) error {
	_, err := db.Exec(`
		INSERT INTO sessions (id, root_task, tier, token_budget, tokens_used, started_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.RootTask, s.Tier, s.TokenBudget, s.TokensUsed, formatTime(s.StartedAt), string(s.Status))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetSession retrieves a session by ID.
func (db *DB) GetSession(id string) (*Session, error) {
	row := db.QueryRow(`
		SELECT id, root_task, tier, token_budget, tokens_used, started_at, status
		FROM sessions WHERE id = ?
	`, id)

	var s Session
	var startedAt string
	err := row.Scan(&s.ID, &s.RootTask, &s.Tier, &s.TokenBudget, &s.TokensUsed, &startedAt, &s.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	s.StartedAt, _ = parseTime(startedAt)
	return &s, nil
}

// UpdateSession updates a session.
func (db *DB) UpdateSession(s *Session) error {
	_, err := db.Exec(`
		UPDATE sessions SET root_task = ?, tier = ?, token_budget = ?, tokens_used = ?, status = ?
		WHERE id = ?
	`, s.RootTask, s.Tier, s.TokenBudget, s.TokensUsed, string(s.Status), s.ID)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}
	return nil
}

// DeleteSession deletes a session by ID.
func (db *DB) DeleteSession(id string) error {
	_, err := db.Exec("DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// ListSessions lists all sessions, optionally filtered by status.
func (db *DB) ListSessions(status *SessionStatus) ([]Session, error) {
	var rows *sql.Rows
	var err error

	if status != nil {
		rows, err = db.Query(`
			SELECT id, root_task, tier, token_budget, tokens_used, started_at, status
			FROM sessions WHERE status = ? ORDER BY started_at DESC
		`, string(*status))
	} else {
		rows, err = db.Query(`
			SELECT id, root_task, tier, token_budget, tokens_used, started_at, status
			FROM sessions ORDER BY started_at DESC
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		var startedAt string
		if err := rows.Scan(&s.ID, &s.RootTask, &s.Tier, &s.TokenBudget, &s.TokensUsed, &startedAt, &s.Status); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		s.StartedAt, _ = parseTime(startedAt)
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// GetActiveSession returns the current active session, if any.
func (db *DB) GetActiveSession() (*Session, error) {
	status := SessionActive
	sessions, err := db.ListSessions(&status)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return &sessions[0], nil
}

// Agent CRUD operations

// CreateAgent creates a new agent.
func (db *DB) CreateAgent(a *Agent) error {
	var startedAt *string
	if a.StartedAt != nil {
		s := formatTime(*a.StartedAt)
		startedAt = &s
	}

	_, err := db.Exec(`
		INSERT INTO agents (id, task_id, status, worktree_path, pid, started_at, tokens_used, cost, ralph_iter, ralph_score)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.TaskID, string(a.Status), a.WorktreePath, a.PID, startedAt, a.TokensUsed, a.Cost, a.RalphIter, a.RalphScore)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	return nil
}

// GetAgent retrieves an agent by ID.
func (db *DB) GetAgent(id string) (*Agent, error) {
	row := db.QueryRow(`
		SELECT id, task_id, status, worktree_path, pid, started_at, tokens_used, cost, ralph_iter, ralph_score
		FROM agents WHERE id = ?
	`, id)

	var a Agent
	var startedAt sql.NullString
	var worktreePath sql.NullString
	var pid sql.NullInt64
	err := row.Scan(&a.ID, &a.TaskID, &a.Status, &worktreePath, &pid, &startedAt, &a.TokensUsed, &a.Cost, &a.RalphIter, &a.RalphScore)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}

	if worktreePath.Valid {
		a.WorktreePath = worktreePath.String
	}
	if pid.Valid {
		a.PID = int(pid.Int64)
	}
	a.StartedAt = parseNullableTime(startedAt)
	return &a, nil
}

// UpdateAgent updates an agent.
func (db *DB) UpdateAgent(a *Agent) error {
	var startedAt *string
	if a.StartedAt != nil {
		s := formatTime(*a.StartedAt)
		startedAt = &s
	}

	_, err := db.Exec(`
		UPDATE agents SET task_id = ?, status = ?, worktree_path = ?, pid = ?, started_at = ?,
			tokens_used = ?, cost = ?, ralph_iter = ?, ralph_score = ?
		WHERE id = ?
	`, a.TaskID, string(a.Status), a.WorktreePath, a.PID, startedAt, a.TokensUsed, a.Cost, a.RalphIter, a.RalphScore, a.ID)
	if err != nil {
		return fmt.Errorf("update agent: %w", err)
	}
	return nil
}

// DeleteAgent deletes an agent by ID.
func (db *DB) DeleteAgent(id string) error {
	_, err := db.Exec("DELETE FROM agents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	return nil
}

// ListAgents lists all agents, optionally filtered by status.
func (db *DB) ListAgents(status *AgentStatus) ([]Agent, error) {
	var rows *sql.Rows
	var err error

	if status != nil {
		rows, err = db.Query(`
			SELECT id, task_id, status, worktree_path, pid, started_at, tokens_used, cost, ralph_iter, ralph_score
			FROM agents WHERE status = ?
		`, string(*status))
	} else {
		rows, err = db.Query(`
			SELECT id, task_id, status, worktree_path, pid, started_at, tokens_used, cost, ralph_iter, ralph_score
			FROM agents
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		var startedAt sql.NullString
		var worktreePath sql.NullString
		var pid sql.NullInt64
		if err := rows.Scan(&a.ID, &a.TaskID, &a.Status, &worktreePath, &pid, &startedAt, &a.TokensUsed, &a.Cost, &a.RalphIter, &a.RalphScore); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		if worktreePath.Valid {
			a.WorktreePath = worktreePath.String
		}
		if pid.Valid {
			a.PID = int(pid.Int64)
		}
		a.StartedAt = parseNullableTime(startedAt)
		agents = append(agents, a)
	}
	return agents, nil
}

// ListAgentsByTask lists all agents for a task.
func (db *DB) ListAgentsByTask(taskID string) ([]Agent, error) {
	rows, err := db.Query(`
		SELECT id, task_id, status, worktree_path, pid, started_at, tokens_used, cost, ralph_iter, ralph_score
		FROM agents WHERE task_id = ?
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list agents by task: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		var startedAt sql.NullString
		var worktreePath sql.NullString
		var pid sql.NullInt64
		if err := rows.Scan(&a.ID, &a.TaskID, &a.Status, &worktreePath, &pid, &startedAt, &a.TokensUsed, &a.Cost, &a.RalphIter, &a.RalphScore); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		if worktreePath.Valid {
			a.WorktreePath = worktreePath.String
		}
		if pid.Valid {
			a.PID = int(pid.Int64)
		}
		a.StartedAt = parseNullableTime(startedAt)
		agents = append(agents, a)
	}
	return agents, nil
}

// Task CRUD operations

// CreateTask creates a new task.
func (db *DB) CreateTask(t *Task) error {
	dependsOn, _ := json.Marshal(t.DependsOn)

	_, err := db.Exec(`
		INSERT INTO tasks (id, parent_id, title, description, status, depends_on, assigned_to, tier, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.ParentID, t.Title, t.Description, string(t.Status), string(dependsOn), t.AssignedTo, t.Tier, formatTime(t.CreatedAt), nil)
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}

// GetTask retrieves a task by ID.
func (db *DB) GetTask(id string) (*Task, error) {
	row := db.QueryRow(`
		SELECT id, parent_id, title, description, status, depends_on, assigned_to, tier, created_at, completed_at
		FROM tasks WHERE id = ?
	`, id)

	var t Task
	var createdAt string
	var completedAt sql.NullString
	var parentID, description, dependsOn, assignedTo, tier sql.NullString
	err := row.Scan(&t.ID, &parentID, &t.Title, &description, &t.Status, &dependsOn, &assignedTo, &tier, &createdAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	if parentID.Valid {
		t.ParentID = parentID.String
	}
	if description.Valid {
		t.Description = description.String
	}
	if dependsOn.Valid {
		json.Unmarshal([]byte(dependsOn.String), &t.DependsOn)
	}
	if assignedTo.Valid {
		t.AssignedTo = assignedTo.String
	}
	if tier.Valid {
		t.Tier = tier.String
	}
	t.CreatedAt, _ = parseTime(createdAt)
	t.CompletedAt = parseNullableTime(completedAt)
	return &t, nil
}

// UpdateTask updates a task.
func (db *DB) UpdateTask(t *Task) error {
	dependsOn, _ := json.Marshal(t.DependsOn)
	var completedAt *string
	if t.CompletedAt != nil {
		s := formatTime(*t.CompletedAt)
		completedAt = &s
	}

	_, err := db.Exec(`
		UPDATE tasks SET parent_id = ?, title = ?, description = ?, status = ?, depends_on = ?,
			assigned_to = ?, tier = ?, completed_at = ?
		WHERE id = ?
	`, t.ParentID, t.Title, t.Description, string(t.Status), string(dependsOn), t.AssignedTo, t.Tier, completedAt, t.ID)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

// DeleteTask deletes a task by ID.
func (db *DB) DeleteTask(id string) error {
	_, err := db.Exec("DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

// ListTasks lists all tasks, optionally filtered by status.
func (db *DB) ListTasks(status *TaskStatus) ([]Task, error) {
	var rows *sql.Rows
	var err error

	if status != nil {
		rows, err = db.Query(`
			SELECT id, parent_id, title, description, status, depends_on, assigned_to, tier, created_at, completed_at
			FROM tasks WHERE status = ? ORDER BY created_at
		`, string(*status))
	} else {
		rows, err = db.Query(`
			SELECT id, parent_id, title, description, status, depends_on, assigned_to, tier, created_at, completed_at
			FROM tasks ORDER BY created_at
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows)
}

// ListTasksByParent lists all tasks with a given parent.
func (db *DB) ListTasksByParent(parentID string) ([]Task, error) {
	rows, err := db.Query(`
		SELECT id, parent_id, title, description, status, depends_on, assigned_to, tier, created_at, completed_at
		FROM tasks WHERE parent_id = ? ORDER BY created_at
	`, parentID)
	if err != nil {
		return nil, fmt.Errorf("list tasks by parent: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows)
}

// ListReadyTasks lists tasks that are pending and have no unmet dependencies.
func (db *DB) ListReadyTasks() ([]Task, error) {
	// Get all pending tasks
	status := TaskPending
	tasks, err := db.ListTasks(&status)
	if err != nil {
		return nil, err
	}

	// Filter to those with no unmet dependencies
	var ready []Task
	for _, t := range tasks {
		if len(t.DependsOn) == 0 {
			ready = append(ready, t)
			continue
		}

		// Check if all dependencies are done
		allDone := true
		for _, depID := range t.DependsOn {
			dep, err := db.GetTask(depID)
			if err != nil {
				return nil, err
			}
			if dep == nil || dep.Status != TaskDone {
				allDone = false
				break
			}
		}
		if allDone {
			ready = append(ready, t)
		}
	}

	return ready, nil
}

// scanTasks scans task rows into a slice.
func scanTasks(rows *sql.Rows) ([]Task, error) {
	var tasks []Task
	for rows.Next() {
		var t Task
		var createdAt string
		var completedAt sql.NullString
		var parentID, description, dependsOn, assignedTo, tier sql.NullString
		if err := rows.Scan(&t.ID, &parentID, &t.Title, &description, &t.Status, &dependsOn, &assignedTo, &tier, &createdAt, &completedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		if parentID.Valid {
			t.ParentID = parentID.String
		}
		if description.Valid {
			t.Description = description.String
		}
		if dependsOn.Valid {
			json.Unmarshal([]byte(dependsOn.String), &t.DependsOn)
		}
		if assignedTo.Valid {
			t.AssignedTo = assignedTo.String
		}
		if tier.Valid {
			t.Tier = tier.String
		}
		t.CreatedAt, _ = parseTime(createdAt)
		t.CompletedAt = parseNullableTime(completedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}
