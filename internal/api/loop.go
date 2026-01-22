package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// AgentLoop manages the API call and tool execution cycle.
type AgentLoop struct {
	client        *Client
	executor      *ToolExecutor
	notifications *NotificationManager
	onStream      func(StreamEvent)
	maxIterations int
}

// StreamEvent represents an event during agent execution for streaming to UI.
type StreamEvent struct {
	Type    string // "text", "tool_use", "tool_result", "thinking", "done", "error"
	Content string
	Tool    string
	Input   json.RawMessage
}

// LoopResult contains the results of an agent loop execution.
type LoopResult struct {
	Output     string
	TokensIn   int64
	TokensOut  int64
	ToolCalls  int
	Iterations int
	Stopped    bool // True if stopped by signal
}

// AgentLoopConfig contains configuration for the agent loop.
type AgentLoopConfig struct {
	Client        *Client
	WorkDir       string
	Notifications *NotificationManager
	MaxIterations int // Max API calls before stopping (0 = unlimited)
}

// NewAgentLoop creates a new agent loop with the given configuration.
func NewAgentLoop(cfg AgentLoopConfig) *AgentLoop {
	maxIter := cfg.MaxIterations
	if maxIter == 0 {
		maxIter = 50 // Default max iterations
	}

	return &AgentLoop{
		client:        cfg.Client,
		executor:      NewToolExecutor(cfg.WorkDir),
		notifications: cfg.Notifications,
		maxIterations: maxIter,
	}
}

// SetStreamHandler sets a callback for streaming events during execution.
func (l *AgentLoop) SetStreamHandler(fn func(StreamEvent)) {
	l.onStream = fn
}

// emit sends a stream event if a handler is configured.
func (l *AgentLoop) emit(event StreamEvent) {
	if l.onStream != nil {
		l.onStream(event)
	}
}

// Run executes the agent loop with the given prompts.
func (l *AgentLoop) Run(ctx context.Context, systemPrompt, userPrompt string) (*LoopResult, error) {
	result := &LoopResult{}

	// 1. Check notifications and inject into context
	if l.notifications != nil {
		decisions := l.notifications.ReadDecisions()
		if decisions != "" {
			systemPrompt = fmt.Sprintf("%s\n\n## Project Decisions\n%s", systemPrompt, decisions)
		}

		// Check for stop signal
		if l.notifications.ShouldStop() {
			result.Stopped = true
			return result, fmt.Errorf("stop signal received before start")
		}
	}

	// Build initial messages
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
	}

	// Agent loop
	for result.Iterations < l.maxIterations {
		result.Iterations++

		// Check for signals each iteration
		if l.notifications != nil && l.notifications.ShouldStop() {
			result.Stopped = true
			return result, fmt.Errorf("stop signal received")
		}

		// Make API call
		resp, err := l.client.sdk().Messages.New(ctx, anthropic.MessageNewParams{
			Model:     l.client.Model(),
			MaxTokens: 8192,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: messages,
			Tools:    ToolDefinitions(),
		})
		if err != nil {
			l.emit(StreamEvent{Type: "error", Content: err.Error()})
			return result, fmt.Errorf("API call failed: %w", err)
		}

		// Track tokens
		result.TokensIn += resp.Usage.InputTokens
		result.TokensOut += resp.Usage.OutputTokens
		l.client.Tracker().Add(resp.Usage.InputTokens, resp.Usage.OutputTokens)

		// Process response content
		var assistantBlocks []anthropic.ContentBlockParamUnion
		var toolResultBlocks []anthropic.ContentBlockParamUnion
		var textOutput string

		for _, block := range resp.Content {
			switch variant := block.AsAny().(type) {
			case anthropic.TextBlock:
				textOutput += variant.Text
				l.emit(StreamEvent{Type: "text", Content: variant.Text})
				assistantBlocks = append(assistantBlocks, anthropic.NewTextBlock(variant.Text))

			case anthropic.ToolUseBlock:
				result.ToolCalls++

				l.emit(StreamEvent{
					Type:  "tool_use",
					Tool:  variant.Name,
					Input: variant.Input,
				})
				assistantBlocks = append(assistantBlocks,
					anthropic.NewToolUseBlock(variant.ID, variant.Input, variant.Name))

				// Execute tool
				toolResult := l.executor.Execute(ctx, variant.Name, variant.Input)
				l.emit(StreamEvent{
					Type:    "tool_result",
					Tool:    variant.Name,
					Content: truncateForDisplay(toolResult.Content),
				})

				toolResultBlocks = append(toolResultBlocks,
					anthropic.NewToolResultBlock(variant.ID, toolResult.Content, toolResult.IsError))
			}
		}

		// Check if done (no more tool use)
		if resp.StopReason == anthropic.StopReasonEndTurn {
			result.Output = textOutput
			l.emit(StreamEvent{Type: "done"})
			return result, nil
		}

		// Continue conversation with tool results
		messages = append(messages, anthropic.NewAssistantMessage(assistantBlocks...))
		if len(toolResultBlocks) > 0 {
			messages = append(messages, anthropic.NewUserMessage(toolResultBlocks...))
		}
	}

	return result, fmt.Errorf("max iterations (%d) reached", l.maxIterations)
}

// RunWithTools executes the agent loop with a custom tool set.
func (l *AgentLoop) RunWithTools(ctx context.Context, systemPrompt, userPrompt string, tools []anthropic.ToolUnionParam) (*LoopResult, error) {
	result := &LoopResult{}

	// Inject decisions
	if l.notifications != nil {
		decisions := l.notifications.ReadDecisions()
		if decisions != "" {
			systemPrompt = fmt.Sprintf("%s\n\n## Project Decisions\n%s", systemPrompt, decisions)
		}
	}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
	}

	for result.Iterations < l.maxIterations {
		result.Iterations++

		if l.notifications != nil && l.notifications.ShouldStop() {
			result.Stopped = true
			return result, fmt.Errorf("stop signal received")
		}

		resp, err := l.client.sdk().Messages.New(ctx, anthropic.MessageNewParams{
			Model:     l.client.Model(),
			MaxTokens: 8192,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: messages,
			Tools:    tools,
		})
		if err != nil {
			return result, fmt.Errorf("API call failed: %w", err)
		}

		result.TokensIn += resp.Usage.InputTokens
		result.TokensOut += resp.Usage.OutputTokens
		l.client.Tracker().Add(resp.Usage.InputTokens, resp.Usage.OutputTokens)

		var assistantBlocks []anthropic.ContentBlockParamUnion
		var toolResultBlocks []anthropic.ContentBlockParamUnion
		var textOutput string

		for _, block := range resp.Content {
			switch variant := block.AsAny().(type) {
			case anthropic.TextBlock:
				textOutput += variant.Text
				l.emit(StreamEvent{Type: "text", Content: variant.Text})
				assistantBlocks = append(assistantBlocks, anthropic.NewTextBlock(variant.Text))

			case anthropic.ToolUseBlock:
				result.ToolCalls++

				l.emit(StreamEvent{Type: "tool_use", Tool: variant.Name, Input: variant.Input})
				assistantBlocks = append(assistantBlocks,
					anthropic.NewToolUseBlock(variant.ID, variant.Input, variant.Name))

				toolResult := l.executor.Execute(ctx, variant.Name, variant.Input)
				l.emit(StreamEvent{Type: "tool_result", Tool: variant.Name, Content: truncateForDisplay(toolResult.Content)})

				toolResultBlocks = append(toolResultBlocks,
					anthropic.NewToolResultBlock(variant.ID, toolResult.Content, toolResult.IsError))
			}
		}

		if resp.StopReason == anthropic.StopReasonEndTurn {
			result.Output = textOutput
			l.emit(StreamEvent{Type: "done"})
			return result, nil
		}

		messages = append(messages, anthropic.NewAssistantMessage(assistantBlocks...))
		if len(toolResultBlocks) > 0 {
			messages = append(messages, anthropic.NewUserMessage(toolResultBlocks...))
		}
	}

	return result, fmt.Errorf("max iterations (%d) reached", l.maxIterations)
}

// SimpleCall makes a single API call without tool execution (for simple prompts).
func (l *AgentLoop) SimpleCall(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := l.client.sdk().Messages.New(ctx, anthropic.MessageNewParams{
		Model:     l.client.Model(),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
	})
	if err != nil {
		return "", err
	}

	l.client.Tracker().Add(resp.Usage.InputTokens, resp.Usage.OutputTokens)

	var result string
	for _, block := range resp.Content {
		if variant, ok := block.AsAny().(anthropic.TextBlock); ok {
			result += variant.Text
		}
	}

	return result, nil
}

func truncateForDisplay(s string) string {
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}
