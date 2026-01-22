package api

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// NotificationManager handles inter-agent communication via the .alphie directory.
type NotificationManager struct {
	alphieDir string

	mu          sync.RWMutex
	stopSignal  bool
	pauseSignal bool

	watcher *fsnotify.Watcher
	done    chan struct{}
}

// NewNotificationManager creates a new notification manager for the given repo.
func NewNotificationManager(repoPath string) (*NotificationManager, error) {
	alphieDir := filepath.Join(repoPath, ".alphie")

	// Ensure directories exist
	dirs := []string{
		alphieDir,
		filepath.Join(alphieDir, "signals"),
		filepath.Join(alphieDir, "agents"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	// Initialize decisions file if it doesn't exist
	decisionsPath := filepath.Join(alphieDir, "decisions.md")
	if _, err := os.Stat(decisionsPath); os.IsNotExist(err) {
		initial := `# Project Decisions

Shared naming conventions, patterns, and architectural decisions.
Agents read this file before each task and append new decisions after completing work.

## Naming Conventions

<!-- Add naming decisions here -->

## Patterns

<!-- Add pattern decisions here -->

## Constraints

<!-- Add constraint decisions here -->
`
		if err := os.WriteFile(decisionsPath, []byte(initial), 0644); err != nil {
			return nil, err
		}
	}

	nm := &NotificationManager{
		alphieDir: alphieDir,
		done:      make(chan struct{}),
	}

	// Start file watcher for immediate signals
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		// Continue without watcher - will use polling fallback
		return nm, nil
	}
	nm.watcher = watcher

	signalsDir := filepath.Join(alphieDir, "signals")
	if err := watcher.Add(signalsDir); err != nil {
		watcher.Close()
		return nm, nil
	}

	go nm.watchSignals()

	return nm, nil
}

// watchSignals monitors the signals directory for kill/pause files.
func (nm *NotificationManager) watchSignals() {
	for {
		select {
		case <-nm.done:
			return
		case event, ok := <-nm.watcher.Events:
			if !ok {
				return
			}
			nm.mu.Lock()
			base := filepath.Base(event.Name)
			if base == "kill" && (event.Op&fsnotify.Create != 0 || event.Op&fsnotify.Write != 0) {
				nm.stopSignal = true
			} else if base == "pause" && (event.Op&fsnotify.Create != 0 || event.Op&fsnotify.Write != 0) {
				nm.pauseSignal = true
			}
			nm.mu.Unlock()
		case <-nm.watcher.Errors:
			// Ignore errors, keep watching
		}
	}
}

// ReadDecisions returns the current contents of the decisions file.
func (nm *NotificationManager) ReadDecisions() string {
	path := filepath.Join(nm.alphieDir, "decisions.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

// AppendDecision adds a new decision to the decisions file.
func (nm *NotificationManager) AppendDecision(category, decision string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	path := filepath.Join(nm.alphieDir, "decisions.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04")
	entry := "\n- " + timestamp + ": " + decision + "\n"

	_, err = f.WriteString(entry)
	return err
}

// ShouldStop returns true if a stop signal has been received.
func (nm *NotificationManager) ShouldStop() bool {
	// Also check file directly in case watcher missed it
	killPath := filepath.Join(nm.alphieDir, "signals", "kill")
	if _, err := os.Stat(killPath); err == nil {
		nm.mu.Lock()
		nm.stopSignal = true
		nm.mu.Unlock()
	}

	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.stopSignal
}

// ShouldPause returns true if a pause signal has been received.
func (nm *NotificationManager) ShouldPause() bool {
	pausePath := filepath.Join(nm.alphieDir, "signals", "pause")
	if _, err := os.Stat(pausePath); err == nil {
		nm.mu.Lock()
		nm.pauseSignal = true
		nm.mu.Unlock()
	}

	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.pauseSignal
}

// SendKill creates a kill signal file.
func (nm *NotificationManager) SendKill() error {
	path := filepath.Join(nm.alphieDir, "signals", "kill")
	return os.WriteFile(path, []byte(time.Now().Format(time.RFC3339)), 0644)
}

// SendPause creates a pause signal file.
func (nm *NotificationManager) SendPause() error {
	path := filepath.Join(nm.alphieDir, "signals", "pause")
	return os.WriteFile(path, []byte(time.Now().Format(time.RFC3339)), 0644)
}

// ClearSignals removes all signal files and resets signal state.
func (nm *NotificationManager) ClearSignals() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	nm.stopSignal = false
	nm.pauseSignal = false

	os.Remove(filepath.Join(nm.alphieDir, "signals", "kill"))
	os.Remove(filepath.Join(nm.alphieDir, "signals", "pause"))
}

// WriteAgentMessage writes a message for a specific agent.
func (nm *NotificationManager) WriteAgentMessage(agentID, message string) error {
	path := filepath.Join(nm.alphieDir, "agents", agentID+".md")
	return os.WriteFile(path, []byte(message), 0644)
}

// ReadAgentMessage reads the message for a specific agent.
func (nm *NotificationManager) ReadAgentMessage(agentID string) string {
	path := filepath.Join(nm.alphieDir, "agents", agentID+".md")
	content, _ := os.ReadFile(path)
	return string(content)
}

// ClearAgentMessage removes the message file for an agent.
func (nm *NotificationManager) ClearAgentMessage(agentID string) error {
	path := filepath.Join(nm.alphieDir, "agents", agentID+".md")
	return os.Remove(path)
}

// AlphieDir returns the path to the .alphie directory.
func (nm *NotificationManager) AlphieDir() string {
	return nm.alphieDir
}

// Close shuts down the notification manager.
func (nm *NotificationManager) Close() {
	close(nm.done)
	if nm.watcher != nil {
		nm.watcher.Close()
	}
}
