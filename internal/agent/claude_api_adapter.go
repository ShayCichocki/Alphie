package agent

import (
	"github.com/ShayCichocki/alphie/internal/api"
)

// ClaudeAPIAdapter adapts the api.ClaudeAPI to the ClaudeRunner interface.
type ClaudeAPIAdapter struct {
	api      *api.ClaudeAPI
	outputCh chan StreamEvent
}

// NewClaudeAPIAdapter creates a new adapter for the given ClaudeAPI.
func NewClaudeAPIAdapter(claudeAPI *api.ClaudeAPI) *ClaudeAPIAdapter {
	return &ClaudeAPIAdapter{
		api:      claudeAPI,
		outputCh: make(chan StreamEvent, 100),
	}
}

// Start launches Claude via the API.
func (a *ClaudeAPIAdapter) Start(prompt, workDir string) error {
	if err := a.api.Start(prompt, workDir); err != nil {
		return err
	}
	// Start a goroutine to convert events
	go a.convertEvents()
	return nil
}

// StartWithOptions launches Claude with options via the API.
func (a *ClaudeAPIAdapter) StartWithOptions(prompt, workDir string, opts *StartOptions) error {
	var apiOpts *api.StartOptionsAPI
	if opts != nil {
		apiOpts = &api.StartOptionsAPI{
			Model:       opts.Model,
			Temperature: opts.Temperature,
		}
	}
	if err := a.api.StartWithOptions(prompt, workDir, apiOpts); err != nil {
		return err
	}
	// Start a goroutine to convert events
	go a.convertEvents()
	return nil
}

// convertEvents converts api.StreamEventCompat to agent.StreamEvent.
func (a *ClaudeAPIAdapter) convertEvents() {
	defer close(a.outputCh)
	for apiEvent := range a.api.Output() {
		event := StreamEvent{
			Type:       convertEventType(apiEvent.Type),
			Message:    apiEvent.Message,
			Error:      apiEvent.Error,
			ToolAction: apiEvent.ToolAction,
			Raw:        apiEvent.Raw,
		}
		a.outputCh <- event
	}
}

// convertEventType maps API event types to agent event types.
func convertEventType(apiType string) StreamEventType {
	switch apiType {
	case api.StreamEventSystem:
		return StreamEventSystem
	case api.StreamEventAssistant:
		return StreamEventAssistant
	case api.StreamEventUser:
		return StreamEventUser
	case api.StreamEventResult:
		return StreamEventResult
	case api.StreamEventError:
		return StreamEventError
	default:
		return StreamEventType(apiType)
	}
}

// Output returns the event channel.
func (a *ClaudeAPIAdapter) Output() <-chan StreamEvent {
	return a.outputCh
}

// Wait waits for execution to complete.
func (a *ClaudeAPIAdapter) Wait() error {
	return a.api.Wait()
}

// Kill terminates execution.
func (a *ClaudeAPIAdapter) Kill() error {
	return a.api.Kill()
}

// Stderr returns any captured stderr output.
// For API mode, this is always empty.
func (a *ClaudeAPIAdapter) Stderr() string {
	return a.api.Stderr()
}

// PID returns the process ID.
// For API mode, returns 0.
func (a *ClaudeAPIAdapter) PID() int {
	return a.api.PID()
}

// Client returns the underlying API client for token tracking.
func (a *ClaudeAPIAdapter) Client() *api.Client {
	return a.api.Client()
}

// Verify ClaudeAPIAdapter implements ClaudeRunner at compile time.
var _ ClaudeRunner = (*ClaudeAPIAdapter)(nil)

// APIRunnerFactory creates ClaudeRunner instances using the API backend.
type APIRunnerFactory struct {
	Client        *api.Client
	Notifications *api.NotificationManager
}

// NewRunner creates a new API-based ClaudeRunner.
func (f *APIRunnerFactory) NewRunner() ClaudeRunner {
	claudeAPI := api.NewClaudeAPI(api.ClaudeAPIConfig{
		Client:        f.Client,
		Notifications: f.Notifications,
	})
	return NewClaudeAPIAdapter(claudeAPI)
}

