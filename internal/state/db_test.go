package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// tempDBPath returns a path to a temp database file.
func tempDBPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}

// setupTestDB creates a new temporary database for testing.
func setupTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(tempDBPath(t))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	return db
}

func TestOpen(t *testing.T) {
	path := tempDBPath(t)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Check path is set correctly
	if db.Path() != path {
		t.Errorf("Path() = %q, want %q", db.Path(), path)
	}

	// Check file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("database file does not exist at %s", path)
	}
}

func TestOpen_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")
	path := filepath.Join(nested, "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(nested); os.IsNotExist(err) {
		t.Errorf("parent directories not created: %s", nested)
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	// Try to open a database at a path that can't be created
	// (on Linux, we can't create files under /proc)
	_, err := Open("/proc/nonexistent/test.db")
	if err == nil {
		t.Error("expected error opening db at invalid path")
	}
}

func TestClose(t *testing.T) {
	db, err := Open(tempDBPath(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Subsequent operations should fail
	_, err = db.Query("SELECT 1")
	if err == nil {
		t.Error("expected error after close, got nil")
	}
}

func TestMigrate(t *testing.T) {
	db, err := Open(tempDBPath(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// First migration
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Check tables exist
	tables := []string{"schema_version", "sessions", "agents", "tasks"}
	for _, table := range tables {
		var count int
		row := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table)
		if err := row.Scan(&count); err != nil {
			t.Errorf("failed to check table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s does not exist", table)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db, err := Open(tempDBPath(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Run migration multiple times
	for i := 0; i < 3; i++ {
		if err := db.Migrate(); err != nil {
			t.Fatalf("Migrate (iteration %d) failed: %v", i, err)
		}
	}

	// Check schema version
	var version int
	row := db.QueryRow("SELECT MAX(version) FROM schema_version")
	if err := row.Scan(&version); err != nil {
		t.Fatalf("failed to get schema version: %v", err)
	}
	if version != 3 {
		t.Errorf("schema version = %d, want 3", version)
	}
}

func TestMigrate_SchemaVersionTracking(t *testing.T) {
	db, err := Open(tempDBPath(t))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Check that all migration versions are recorded
	rows, err := db.Query("SELECT version FROM schema_version ORDER BY version")
	if err != nil {
		t.Fatalf("failed to query schema_version: %v", err)
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("failed to scan version: %v", err)
		}
		versions = append(versions, v)
	}

	expected := []int{1, 2, 3}
	if len(versions) != len(expected) {
		t.Errorf("versions = %v, want %v", versions, expected)
	}
	for i, v := range expected {
		if i >= len(versions) || versions[i] != v {
			t.Errorf("version[%d] = %d, want %d", i, versions[i], v)
		}
	}
}

func TestExec(t *testing.T) {
	db := setupTestDB(t)

	result, err := db.Exec("INSERT INTO sessions (id, root_task, tier, token_budget, tokens_used, started_at, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"test-1", "task-1", "premium", 1000, 0, "2024-01-01T00:00:00Z", "active")
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected failed: %v", err)
	}
	if affected != 1 {
		t.Errorf("RowsAffected = %d, want 1", affected)
	}
}

func TestQuery(t *testing.T) {
	db := setupTestDB(t)

	// Insert test data
	_, err := db.Exec("INSERT INTO sessions (id, root_task, tier, token_budget, tokens_used, started_at, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"test-1", "task-1", "premium", 1000, 0, "2024-01-01T00:00:00Z", "active")
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	rows, err := db.Query("SELECT id, root_task FROM sessions WHERE id = ?", "test-1")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	var id, rootTask string
	if !rows.Next() {
		t.Fatal("expected row, got none")
	}
	if err := rows.Scan(&id, &rootTask); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if id != "test-1" || rootTask != "task-1" {
		t.Errorf("got (%s, %s), want (test-1, task-1)", id, rootTask)
	}
}

func TestQueryRow(t *testing.T) {
	db := setupTestDB(t)

	// Insert test data
	_, err := db.Exec("INSERT INTO sessions (id, root_task, tier, token_budget, tokens_used, started_at, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"test-1", "task-1", "premium", 1000, 0, "2024-01-01T00:00:00Z", "active")
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	var count int
	row := db.QueryRow("SELECT COUNT(*) FROM sessions")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("QueryRow.Scan failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestTransaction_Success(t *testing.T) {
	db := setupTestDB(t)

	err := db.Transaction(func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO sessions (id, root_task, tier, token_budget, tokens_used, started_at, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"tx-1", "task-1", "premium", 1000, 0, "2024-01-01T00:00:00Z", "active")
		return err
	})
	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}

	// Verify data was committed
	var count int
	row := db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = ?", "tx-1")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("failed to verify: %v", err)
	}
	if count != 1 {
		t.Error("transaction was not committed")
	}
}

func TestTransaction_Rollback(t *testing.T) {
	db := setupTestDB(t)

	err := db.Transaction(func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO sessions (id, root_task, tier, token_budget, tokens_used, started_at, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"tx-fail", "task-1", "premium", 1000, 0, "2024-01-01T00:00:00Z", "active")
		if err != nil {
			return err
		}
		return fmt.Errorf("simulated error")
	})
	if err == nil {
		t.Error("expected error from Transaction")
	}

	// Verify data was rolled back
	var count int
	row := db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = ?", "tx-fail")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("failed to verify: %v", err)
	}
	if count != 0 {
		t.Error("transaction was not rolled back")
	}
}

func TestGlobalDBPath(t *testing.T) {
	// Save and restore env
	original := os.Getenv("XDG_DATA_HOME")
	defer os.Setenv("XDG_DATA_HOME", original)

	// Test with XDG_DATA_HOME set
	os.Setenv("XDG_DATA_HOME", "/custom/data")
	path := GlobalDBPath()
	expected := "/custom/data/alphie/alphie.db"
	if path != expected {
		t.Errorf("GlobalDBPath() = %q, want %q", path, expected)
	}

	// Test without XDG_DATA_HOME
	os.Unsetenv("XDG_DATA_HOME")
	path = GlobalDBPath()
	home, _ := os.UserHomeDir()
	expected = filepath.Join(home, ".local", "share", "alphie", "alphie.db")
	if path != expected {
		t.Errorf("GlobalDBPath() = %q, want %q", path, expected)
	}
}

func TestProjectDBPath(t *testing.T) {
	path := ProjectDBPath("/my/project")
	expected := "/my/project/.alphie/state.db"
	if path != expected {
		t.Errorf("ProjectDBPath() = %q, want %q", path, expected)
	}
}

func TestFormatAndParseTime(t *testing.T) {
	// Test round-trip
	now := time.Now()
	formatted := formatTime(now)
	parsed, err := parseTime(formatted)
	if err != nil {
		t.Fatalf("parseTime failed: %v", err)
	}

	// Times should be equal when truncated to second precision in UTC
	if !now.UTC().Truncate(time.Second).Equal(parsed.Truncate(time.Second)) {
		t.Errorf("time round-trip failed: got %v, want %v", parsed, now.UTC())
	}
}

func TestParseNullableTime(t *testing.T) {
	// Test valid time
	validTime := sql.NullString{String: "2024-01-01T12:00:00Z", Valid: true}
	result := parseNullableTime(validTime)
	if result == nil {
		t.Error("expected non-nil time for valid input")
	}

	// Test null/invalid time
	nullTime := sql.NullString{Valid: false}
	result = parseNullableTime(nullTime)
	if result != nil {
		t.Error("expected nil time for invalid input")
	}

	// Test invalid format
	badFormat := sql.NullString{String: "not a time", Valid: true}
	result = parseNullableTime(badFormat)
	if result != nil {
		t.Error("expected nil time for invalid format")
	}
}
