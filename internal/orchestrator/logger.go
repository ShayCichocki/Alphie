// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// pkgLogger is the package-level debug logger used by orchestrator components.
var pkgLogger *DebugLogger
var pkgLoggerMu sync.RWMutex

// setPackageLogger sets the package-level logger.
func setPackageLogger(l *DebugLogger) {
	pkgLoggerMu.Lock()
	defer pkgLoggerMu.Unlock()
	pkgLogger = l
}

// debugLog writes a message using the package-level logger.
// This is used by internal components (graph, scheduler, etc.) that don't
// have direct access to the orchestrator's logger.
func debugLog(format string, args ...interface{}) {
	pkgLoggerMu.RLock()
	l := pkgLogger
	pkgLoggerMu.RUnlock()

	if l != nil {
		l.Log(format, args...)
	}
}

// DebugLogger provides debug logging for orchestrator operations.
// It wraps file-based logging with thread-safe access.
type DebugLogger struct {
	mu   sync.Mutex
	file *os.File
}

// NewDebugLogger creates a logger writing to the specified path.
// If the path is empty, returns a no-op logger.
// Creates parent directories if they don't exist.
func NewDebugLogger(logPath string) (*DebugLogger, error) {
	if logPath == "" {
		return &DebugLogger{}, nil
	}

	// Ensure parent directory exists
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	logger := &DebugLogger{file: f}

	// Write header
	logger.Log("=== Orchestrator Debug Log Started at %s ===", time.Now().Format(time.RFC3339))

	return logger, nil
}

// NewDebugLoggerForRepo creates a debug logger in the repo's .alphie/logs directory.
// Returns a no-op logger if the directory cannot be created.
func NewDebugLoggerForRepo(repoPath string) *DebugLogger {
	logPath := filepath.Join(repoPath, ".alphie", "logs", "orchestrator-debug.log")
	logger, err := NewDebugLogger(logPath)
	if err != nil {
		// Return no-op logger on error
		return &DebugLogger{}
	}
	return logger
}

// NopLogger returns a no-op logger for testing or when logging is disabled.
func NopLogger() *DebugLogger {
	return &DebugLogger{}
}

// Log writes a timestamped message to the debug log.
// If the logger is nil or has no file, this is a no-op.
func (l *DebugLogger) Log(format string, args ...interface{}) {
	if l == nil || l.file == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05.000")
	fmt.Fprintf(l.file, "[%s] %s\n", timestamp, msg)
	l.file.Sync()
}

// Close closes the log file.
// Safe to call on nil logger or logger without file.
func (l *DebugLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.file.Close()
}
