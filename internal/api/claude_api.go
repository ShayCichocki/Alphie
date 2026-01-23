package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// ClaudeAPI provides a subprocess-compatible interface for the Anthropic API.
// This allows gradual migration from ClaudeProcess to direct API calls.
type ClaudeAPI struct {
	client   *Client
	executor *ToolExecutor
	notifs   *NotificationManager

	ctx       context.Context
	cancel    context.CancelFunc
	outputCh  chan StreamEventCompat
	done      chan struct{}
	mu        sync.Mutex
	started   bool
	stderrBuf []byte
	lastErr   error // Stores the last error for Wait() to return

	// Config
	model         anthropic.Model
	maxIterations int
	temperature   *float64
}

// StreamEventCompat is compatible with the agent.StreamEvent type.
type StreamEventCompat struct {
	Type       string          `json:"type"`
	Message    string          `json:"message,omitempty"`
	Error      string          `json:"error,omitempty"`
	ToolAction string          `json:"tool_action,omitempty"`
	Raw        json.RawMessage `json:"-"`
}

// StreamEventTypeCompat mirrors agent.StreamEventType constants.
const (
	StreamEventSystem    = "system"
	StreamEventAssistant = "assistant"
	StreamEventUser      = "user"
	StreamEventResult    = "result"
	StreamEventError     = "error"
)

// ClaudeAPIConfig contains configuration for ClaudeAPI.
type ClaudeAPIConfig struct {
	Client        *Client
	Notifications *NotificationManager
	Model         anthropic.Model
	MaxIterations int
	Temperature   *float64 // Optional temperature (0.0-1.0)
}

// NewClaudeAPI creates a new API-based Claude runner.
func NewClaudeAPI(cfg ClaudeAPIConfig) *ClaudeAPI {
	maxIter := cfg.MaxIterations
	if maxIter == 0 {
		maxIter = 50
	}

	// Use the client's model which has already been translated for Bedrock
	model := cfg.Client.Model()

	return &ClaudeAPI{
		client:        cfg.Client,
		notifs:        cfg.Notifications,
		model:         model,
		maxIterations: maxIter,
		temperature:   cfg.Temperature,
		outputCh:      make(chan StreamEventCompat, 100),
		done:          make(chan struct{}),
	}
}

// Start launches the API-based execution with the given prompt.
// This is compatible with ClaudeProcess.Start().
func (c *ClaudeAPI) Start(prompt, workDir string) error {
	return c.StartWithOptions(prompt, workDir, nil)
}

// StartOptions mirrors agent.StartOptions.
type StartOptionsAPI struct {
	Model       string
	Temperature *float64
}

// StartWithOptions launches with additional options.
func (c *ClaudeAPI) StartWithOptions(prompt, workDir string, opts *StartOptionsAPI) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return fmt.Errorf("already started")
	}
	c.started = true
	c.mu.Unlock()

	// Create context
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// Create tool executor for this workdir
	c.executor = NewToolExecutor(workDir)

	// Override model if specified
	model := c.model
	if opts != nil && opts.Model != "" {
		// Translate the model for Bedrock if needed
		model = c.client.TranslateModel(anthropic.Model(opts.Model))
	}

	// Override temperature if specified
	if opts != nil && opts.Temperature != nil {
		c.temperature = opts.Temperature
	}

	// Start the agent loop in a goroutine
	go c.runLoop(prompt, model)

	return nil
}

func (c *ClaudeAPI) runLoop(prompt string, model anthropic.Model) {
	defer close(c.outputCh)
	defer close(c.done)

	// Build system prompt
	systemPrompt := "You are an AI assistant helping with software development tasks."

	// Inject decisions if notifications are configured
	if c.notifs != nil {
		decisions := c.notifs.ReadDecisions()
		if decisions != "" {
			systemPrompt = fmt.Sprintf("%s\n\n## Project Decisions\n%s", systemPrompt, decisions)
		}
	}

	// Build initial messages
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
	}

	iterations := 0
	for iterations < c.maxIterations {
		iterations++

		// Check for stop signal
		if c.notifs != nil && c.notifs.ShouldStop() {
			c.emitError("stop signal received")
			return
		}

		// Check context
		select {
		case <-c.ctx.Done():
			c.setError(c.ctx.Err())
			return
		default:
		}

		// Make API call
		params := anthropic.MessageNewParams{
			Model:     model,
			MaxTokens: 8192,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: messages,
			Tools:    ToolDefinitions(),
		}

		// Add temperature if specified
		if c.temperature != nil {
			params.Temperature = anthropic.Float(*c.temperature)
		}

		resp, err := c.client.sdk().Messages.New(c.ctx, params)
		if err != nil {
			// Debug: Write full error to file
			if debugFile, ferr := os.OpenFile("/tmp/alphie-api-error.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); ferr == nil {
				fmt.Fprintf(debugFile, "[%s] API Error:\n", time.Now().Format(time.RFC3339))
				fmt.Fprintf(debugFile, "  Error: %v\n", err)
				fmt.Fprintf(debugFile, "  Type: %T\n", err)
				fmt.Fprintf(debugFile, "  Model: %s\n", model)
				fmt.Fprintf(debugFile, "\n")
				debugFile.Close()
			}
			c.emitError(fmt.Sprintf("API error: %v", err))
			return
		}

		// Track tokens
		c.client.Tracker().Add(resp.Usage.InputTokens, resp.Usage.OutputTokens)

		// Emit usage as raw JSON for compatibility with token extraction
		usageJSON, _ := json.Marshal(map[string]interface{}{
			"usage": map[string]interface{}{
				"input_tokens":  resp.Usage.InputTokens,
				"output_tokens": resp.Usage.OutputTokens,
			},
		})

		// Process response content
		var assistantBlocks []anthropic.ContentBlockParamUnion
		var toolResultBlocks []anthropic.ContentBlockParamUnion

		for _, block := range resp.Content {
			switch variant := block.AsAny().(type) {
			case anthropic.TextBlock:
				c.emit(StreamEventCompat{
					Type:    StreamEventAssistant,
					Message: variant.Text,
					Raw:     usageJSON,
				})
				assistantBlocks = append(assistantBlocks, anthropic.NewTextBlock(variant.Text))

			case anthropic.ToolUseBlock:
				// Emit tool use
				toolAction := FormatToolAction(variant.Name, variant.Input)
				c.emit(StreamEventCompat{
					Type:       StreamEventAssistant,
					ToolAction: toolAction,
					Raw:        usageJSON,
				})

				// Execute tool
				toolResult := c.executor.Execute(c.ctx, variant.Name, variant.Input)

				assistantBlocks = append(assistantBlocks,
					anthropic.NewToolUseBlock(variant.ID, variant.Input, variant.Name))

				toolResultBlocks = append(toolResultBlocks,
					anthropic.NewToolResultBlock(variant.ID, toolResult.Content, toolResult.IsError))

				// Emit tool result if error
				if toolResult.IsError {
					c.emit(StreamEventCompat{
						Type:  StreamEventError,
						Error: toolResult.Content,
					})
				}
			}
		}

		// Check if done
		if resp.StopReason == anthropic.StopReasonEndTurn {
			// Emit final result
			var finalText string
			for _, block := range resp.Content {
				if variant, ok := block.AsAny().(anthropic.TextBlock); ok {
					finalText += variant.Text
				}
			}
			c.emit(StreamEventCompat{
				Type:    StreamEventResult,
				Message: finalText,
				Raw:     usageJSON,
			})
			return
		}

		// Continue conversation
		messages = append(messages, anthropic.NewAssistantMessage(assistantBlocks...))
		if len(toolResultBlocks) > 0 {
			messages = append(messages, anthropic.NewUserMessage(toolResultBlocks...))
		}
	}

	c.emitError(fmt.Sprintf("max iterations (%d) reached", c.maxIterations))
}

func (c *ClaudeAPI) emit(event StreamEventCompat) {
	select {
	case c.outputCh <- event:
	case <-c.ctx.Done():
	}
}

func (c *ClaudeAPI) setError(err error) {
	c.mu.Lock()
	c.lastErr = err
	c.mu.Unlock()
}

func (c *ClaudeAPI) emitError(msg string) {
	c.mu.Lock()
	c.lastErr = fmt.Errorf("%s", msg)
	c.mu.Unlock()
	c.emit(StreamEventCompat{
		Type:  StreamEventError,
		Error: msg,
	})
}

// Output returns a channel that receives stream events.
// Compatible with ClaudeProcess.Output().
func (c *ClaudeAPI) Output() <-chan StreamEventCompat {
	return c.outputCh
}

// Wait waits for the process to complete.
// Compatible with ClaudeProcess.Wait().
func (c *ClaudeAPI) Wait() error {
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

// Kill terminates the process.
// Compatible with ClaudeProcess.Kill().
func (c *ClaudeAPI) Kill() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// Stderr returns any captured stderr output.
// For API mode, this is always empty.
func (c *ClaudeAPI) Stderr() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.stderrBuf)
}

// PID returns the process ID. For API mode, returns 0.
func (c *ClaudeAPI) PID() int {
	return 0
}

// Client returns the underlying API client for token tracking.
func (c *ClaudeAPI) Client() *Client {
	return c.client
}
