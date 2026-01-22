package agent

import (
	"encoding/json"
	"strings"
)

// processStreamEvent processes a single stream event, updating the token tracker
// and capturing output.
func (e *Executor) processStreamEvent(event StreamEvent, tracker *TokenTracker, output *strings.Builder) {
	switch event.Type {
	case StreamEventAssistant:
		// Capture assistant messages as output
		if event.Message != "" {
			output.WriteString(event.Message)
			output.WriteString("\n")
		}

	case StreamEventResult:
		// Capture result messages
		if event.Message != "" {
			output.WriteString("\n--- Result ---\n")
			output.WriteString(event.Message)
			output.WriteString("\n")
		}

	case StreamEventError:
		// Capture error messages
		if event.Error != "" {
			output.WriteString("\n--- Error ---\n")
			output.WriteString(event.Error)
			output.WriteString("\n")
		}
	}

	// Try to extract token usage from raw JSON
	if event.Raw != nil {
		e.extractTokenUsage(event.Raw, tracker)
	}
}

// extractTokenUsage attempts to extract token usage information from raw JSON.
func (e *Executor) extractTokenUsage(raw json.RawMessage, tracker *TokenTracker) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}

	// Look for usage field
	usageData, ok := data["usage"].(map[string]interface{})
	if !ok {
		return
	}

	var usage MessageDeltaUsage

	if input, ok := usageData["input_tokens"].(float64); ok {
		usage.InputTokens = int64(input)
	}
	if output, ok := usageData["output_tokens"].(float64); ok {
		usage.OutputTokens = int64(output)
	}

	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		tracker.Update(usage)
	}
}
