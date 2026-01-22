// Package learning provides learning and context management capabilities.
package learning

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
		return err
	}

	// Get current version
	var currentVersion int
	row := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM learning_schema_version")
	if err := row.Scan(&currentVersion); err != nil {
		return err
	}

	// Apply migrations
	migrations := []struct {
		version int
		sql     string
	}{
		{1, migrationV1Learnings},
		{2, migrationV2Concepts},
		{3, migrationV3Effectiveness},
	}

	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		tx, err := s.db.Begin()
		if err != nil {
			return err
		}

		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return err
		}

		if _, err := tx.Exec("INSERT INTO learning_schema_version (version) VALUES (?)", m.version); err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
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

const migrationV3Effectiveness = `
-- Add effectiveness tracking fields
ALTER TABLE learnings ADD COLUMN success_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE learnings ADD COLUMN failure_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE learnings ADD COLUMN effectiveness REAL NOT NULL DEFAULT 1.0;

-- Create index for effectiveness queries
CREATE INDEX IF NOT EXISTS idx_learnings_effectiveness ON learnings(effectiveness DESC);

-- Create task outcomes table for tracking learning effectiveness
CREATE TABLE IF NOT EXISTS task_outcomes (
	task_id TEXT PRIMARY KEY,
	session_id TEXT,
	outcome TEXT NOT NULL,  -- 'success', 'failure', 'blocked'
	verification_passed INTEGER,  -- 1 for pass, 0 for fail, NULL if not run
	learnings_used TEXT,  -- JSON array of learning IDs
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_task_outcomes_session ON task_outcomes(session_id);
CREATE INDEX IF NOT EXISTS idx_task_outcomes_outcome ON task_outcomes(outcome);
CREATE INDEX IF NOT EXISTS idx_task_outcomes_created_at ON task_outcomes(created_at);
`
