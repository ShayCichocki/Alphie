// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// StreamEventType represents the type of stream event from Claude Code.
type StreamEventType string

const (
	// StreamEventSystem indicates a system message.
	StreamEventSystem StreamEventType = "system"
	// StreamEventAssistant indicates an assistant message.
	StreamEventAssistant StreamEventType = "assistant"
	// StreamEventUser indicates a user message.
	StreamEventUser StreamEventType = "user"
	// StreamEventResult indicates a final result.
	StreamEventResult StreamEventType = "result"
	// StreamEventError indicates an error.
	StreamEventError StreamEventType = "error"
)

// StreamEvent represents a parsed event from Claude Code's stream-json output.
type StreamEvent struct {
	// Type is the event type.
	Type StreamEventType `json:"type"`
	// Message contains the event content when applicable.
	Message string `json:"message,omitempty"`
	// Error contains error details when Type is StreamEventError.
	Error string `json:"error,omitempty"`
	// ToolAction describes the current tool being used (e.g., "Reading auth.go").
	ToolAction string `json:"tool_action,omitempty"`
	// Raw contains the original JSON for debugging.
	Raw json.RawMessage `json:"-"`
}

// ClaudeProcess manages a Claude Code subprocess.
//
// Deprecated: Use ClaudeAPIAdapter with api.ClaudeAPI for direct API calls.
// Set ALPHIE_USE_API=1 to enable API mode. ClaudeProcess is kept for backward
// compatibility but will be removed in a future release.
type ClaudeProcess struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser

	ctx       context.Context
	cancel    context.CancelFunc
	outputCh  chan StreamEvent
	stderrBuf []byte
	once      sync.Once
	mu        sync.Mutex
	started   bool
	done      chan struct{}
}

// NewClaudeProcess creates a new ClaudeProcess with the given context.
// The context is used for timeout cancellation.
func NewClaudeProcess(ctx context.Context) *ClaudeProcess {
	ctx, cancel := context.WithCancel(ctx)
	return &ClaudeProcess{
		ctx:      ctx,
		cancel:   cancel,
		outputCh: make(chan StreamEvent, 100),
		done:     make(chan struct{}),
	}
}

// StartOptions contains optional parameters for starting a Claude process.
type StartOptions struct {
	// Model is the Claude model to use (e.g., "claude-sonnet-4-20250514").
	// If empty, uses the CLI's default model.
	Model string
}

// Start launches the Claude Code subprocess with the given prompt and worktree path.
// The subprocess is started with --output-format stream-json and --print --verbose flags.
func (p *ClaudeProcess) Start(prompt, worktreePath string) error {
	return p.StartWithOptions(prompt, worktreePath, nil)
}

// StartWithOptions launches the Claude Code subprocess with additional options.
func (p *ClaudeProcess) StartWithOptions(prompt, worktreePath string, opts *StartOptions) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return fmt.Errorf("process already started")
	}

	// Build the command with required flags
	// Use --allowedTools to allow common operations without prompting.
	// Project's .claude/settings.json can still deny specific patterns.
	args := []string{
		"--output-format", "stream-json",
		"--print",
		"--verbose",
		"--allowedTools", "Read,Write,Edit,Bash,Glob,Grep,WebFetch",
	}

	// Add model if specified
	if opts != nil && opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	// Add prompt last
	args = append(args, "-p", prompt)

	p.cmd = exec.CommandContext(p.ctx, "claude", args...)

	// Set working directory if specified
	if worktreePath != "" {
		p.cmd.Dir = worktreePath
	}

	var err error
	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	p.stderr, err = p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	p.started = true

	// Start goroutines to read output
	go p.readOutput()
	go p.readStderr()

	return nil
}

// readOutput reads and parses JSON events from stdout.
func (p *ClaudeProcess) readOutput() {
	defer close(p.outputCh)
	defer close(p.done)

	scanner := bufio.NewScanner(p.stdout)
	// Increase buffer size for large JSON objects
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		event, err := parseStreamEvent(line)
		if err != nil {
			// Send parse error as an event
			p.outputCh <- StreamEvent{
				Type:  StreamEventError,
				Error: fmt.Sprintf("parse error: %v", err),
				Raw:   line,
			}
			continue
		}

		select {
		case p.outputCh <- event:
		case <-p.ctx.Done():
			return
		}
	}

	if err := scanner.Err(); err != nil && p.ctx.Err() == nil {
		p.outputCh <- StreamEvent{
			Type:  StreamEventError,
			Error: fmt.Sprintf("read error: %v", err),
		}
	}
}

// readStderr reads stderr output incrementally and emits it as error events.
// This allows us to capture stderr output during startup hangs.
func (p *ClaudeProcess) readStderr() {
	scanner := bufio.NewScanner(p.stderr)
	// Use a reasonable buffer for stderr lines
	buf := make([]byte, 16*1024)
	scanner.Buffer(buf, 256*1024)

	var allStderr []byte
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Append to buffer for later retrieval
		p.mu.Lock()
		allStderr = append(allStderr, line...)
		allStderr = append(allStderr, '\n')
		p.stderrBuf = allStderr
		p.mu.Unlock()

		// Emit stderr as an error event so it's captured immediately
		// This helps diagnose startup hangs
		select {
		case p.outputCh <- StreamEvent{
			Type:  StreamEventError,
			Error: fmt.Sprintf("[stderr] %s", string(line)),
		}:
		case <-p.ctx.Done():
			return
		default:
			// Channel full, skip emitting but still capture in buffer
		}
	}

	// Capture any final stderr that didn't end with newline
	if err := scanner.Err(); err != nil && p.ctx.Err() == nil {
		p.mu.Lock()
		errMsg := fmt.Sprintf("[stderr read error: %v]", err)
		allStderr = append(allStderr, []byte(errMsg)...)
		p.stderrBuf = allStderr
		p.mu.Unlock()
	}
}

// parseStreamEvent parses a JSON line into a StreamEvent.
func parseStreamEvent(data []byte) (StreamEvent, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return StreamEvent{}, fmt.Errorf("unmarshal json: %w", err)
	}

	event := StreamEvent{
		Raw: data,
	}

	// Extract type
	if t, ok := raw["type"].(string); ok {
		event.Type = StreamEventType(t)
	}

	// Extract message based on type
	switch event.Type {
	case StreamEventSystem, StreamEventAssistant, StreamEventUser:
		if msg, ok := raw["message"].(string); ok {
			event.Message = msg
		} else if content, ok := raw["content"].(string); ok {
			event.Message = content
		}
		// Check for tool use in assistant messages
		if event.Type == StreamEventAssistant {
			event.ToolAction = extractToolAction(raw)
		}
	case StreamEventResult:
		if result, ok := raw["result"].(string); ok {
			event.Message = result
		} else if content, ok := raw["content"].(string); ok {
			event.Message = content
		}
	case StreamEventError:
		if errMsg, ok := raw["error"].(string); ok {
			event.Error = errMsg
		} else if msg, ok := raw["message"].(string); ok {
			event.Error = msg
		}
	}

	return event, nil
}

// extractToolAction extracts a human-readable tool action from an event.
// Returns empty string if no tool use is detected.
func extractToolAction(raw map[string]interface{}) string {
	// Claude Code emits tool_use in various formats. Check common patterns.

	// Pattern 1: message.content is an array with tool_use objects
	if msg, ok := raw["message"].(map[string]interface{}); ok {
		if content, ok := msg["content"].([]interface{}); ok {
			for _, item := range content {
				if block, ok := item.(map[string]interface{}); ok {
					if blockType, _ := block["type"].(string); blockType == "tool_use" {
						return formatToolAction(block)
					}
				}
			}
		}
	}

	// Pattern 2: content is an array at top level
	if content, ok := raw["content"].([]interface{}); ok {
		for _, item := range content {
			if block, ok := item.(map[string]interface{}); ok {
				if blockType, _ := block["type"].(string); blockType == "tool_use" {
					return formatToolAction(block)
				}
			}
		}
	}

	// Pattern 3: direct tool_use field
	if toolUse, ok := raw["tool_use"].(map[string]interface{}); ok {
		return formatToolAction(toolUse)
	}

	return ""
}

// formatToolAction formats a tool_use block into a human-readable string.
func formatToolAction(block map[string]interface{}) string {
	name, _ := block["name"].(string)
	if name == "" {
		return ""
	}

	input, _ := block["input"].(map[string]interface{})

	switch name {
	case "Read":
		if path, ok := input["file_path"].(string); ok {
			return "Reading " + truncateFilename(path)
		}
		return "Reading file"
	case "Edit":
		if path, ok := input["file_path"].(string); ok {
			return "Editing " + truncateFilename(path)
		}
		return "Editing file"
	case "Write":
		if path, ok := input["file_path"].(string); ok {
			return "Writing " + truncateFilename(path)
		}
		return "Writing file"
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			return "Running " + truncateCommand(cmd)
		}
		return "Running command"
	case "Glob":
		if pattern, ok := input["pattern"].(string); ok {
			return "Searching " + pattern
		}
		return "Searching files"
	case "Grep":
		if pattern, ok := input["pattern"].(string); ok {
			return "Grep " + truncatePattern(pattern)
		}
		return "Searching code"
	case "WebFetch":
		return "Fetching URL"
	case "Task":
		return "Running subagent"
	default:
		return name
	}
}

// truncateFilename extracts just the filename from a path and truncates if needed.
func truncateFilename(path string) string {
	// Extract just the filename
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			path = path[i+1:]
			break
		}
	}
	if len(path) > 20 {
		return path[:17] + "..."
	}
	return path
}

// truncateCommand truncates a command for display.
func truncateCommand(cmd string) string {
	// Take first word or truncate
	for i, c := range cmd {
		if c == ' ' || c == '\n' {
			cmd = cmd[:i]
			break
		}
	}
	if len(cmd) > 20 {
		return cmd[:17] + "..."
	}
	return cmd
}

// truncatePattern truncates a search pattern for display.
func truncatePattern(pattern string) string {
	if len(pattern) > 15 {
		return pattern[:12] + "..."
	}
	return pattern
}

// Output returns a channel that receives stream events from the process.
// The channel is closed when the process exits or is killed.
func (p *ClaudeProcess) Output() <-chan StreamEvent {
	return p.outputCh
}

// Wait waits for the process to exit and returns any error.
func (p *ClaudeProcess) Wait() error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return fmt.Errorf("process not started")
	}
	p.mu.Unlock()

	// Wait for output reading to complete
	<-p.done

	err := p.cmd.Wait()
	if err != nil {
		// Include stderr in the error if available
		p.mu.Lock()
		stderr := string(p.stderrBuf)
		p.mu.Unlock()

		// Build a detailed error message
		errMsg := fmt.Sprintf("process exited with error: %v", err)

		// Check if it was killed by context cancellation
		if p.ctx.Err() != nil {
			errMsg += fmt.Sprintf(" (context: %v)", p.ctx.Err())
		}

		// Add stderr if present
		if stderr != "" {
			errMsg += fmt.Sprintf("; stderr: %s", stderr)
		}

		return fmt.Errorf("%s", errMsg)
	}
	return nil
}

// Kill terminates the process immediately.
func (p *ClaudeProcess) Kill() error {
	p.once.Do(func() {
		p.cancel()
	})

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started || p.cmd.Process == nil {
		return nil
	}

	return p.cmd.Process.Kill()
}

// Stderr returns any stderr output captured from the process.
func (p *ClaudeProcess) Stderr() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return string(p.stderrBuf)
}

// PID returns the process ID of the subprocess, or 0 if not started.
func (p *ClaudeProcess) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}
