package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type Agent struct {
	be     backend.Backend
	events chan Event

	mu               sync.Mutex
	closeOnce        sync.Once
	notifiedDecision string
	notifiedApproval string
	lastResult       RunCompletedInfo
}

type Event struct {
	Type    EventType
	Payload any
	Error   error
}

type EventType string

const (
	EventRunCreated   EventType = "run_created"
	EventDecision     EventType = "decision"
	EventApproval     EventType = "approval"
	EventRunCompleted EventType = "run_completed"
	EventRunFailed    EventType = "run_failed"
	EventCancelled    EventType = "cancelled"
	EventProgress     EventType = "progress"
)

type DecisionInfo struct {
	TicketID string
	Question string
	Reason   string
	RunID    string
}

type ApprovalInfo struct {
	ApprovalID string
	Reason     string
	Action     string
	Context    map[string]any
	RunID      string
}

type RunCreatedInfo struct {
	RunID       string
	WorkspaceID string
}

type RunCompletedInfo struct {
	RunID        string
	Summary      string
	DoDSatisfied bool
	StepCount    int
	Outputs      []StepOutput
}

type StepOutput struct {
	Kind    string
	Summary string
}

func NewWithBackend(be backend.Backend) *Agent {
	return &Agent{be: be, events: make(chan Event, 64)}
}

func (a *Agent) Events() <-chan Event { return a.events }

func (a *Agent) Send(evt Event) {
	select {
	case a.events <- evt:
	default:
	}
}

func (a *Agent) Close() {
	a.closeOnce.Do(func() {})
}

func (a *Agent) RunWithBackend(ctx context.Context, workspaceID string, goalText string, llmProviderID string) error {
	defer close(a.events)
	if a.be == nil {
		return fmt.Errorf("agent backend is required")
	}
	if workspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	runDone := make(chan error, 1)
	var runID common.RunID
	var runIDMu sync.RWMutex

	eventCtx, cancelEvents := context.WithCancel(ctx)
	defer cancelEvents()
	unsubscribe, err := a.be.Events().Subscribe(eventCtx, func(_ context.Context, evt backend.Event) {
		runIDMu.RLock()
		currentRunID := runID
		runIDMu.RUnlock()
		if evt.WorkspaceID != common.WorkspaceID(workspaceID) {
			return
		}
		if currentRunID != "" && evt.RunID != "" && evt.RunID != currentRunID {
			return
		}
		a.handleBackendEvent(evt, runDone)
	})
	if err != nil {
		return fmt.Errorf("subscribe events: %w", err)
	}
	defer unsubscribe()

	goal, err := a.be.Goal().Define(ctx, common.WorkspaceID(workspaceID), goalText)
	if err != nil {
		return fmt.Errorf("define goal: %w", err)
	}
	dodItems := backend.DefaultDoDItemsForWorkspace(ctx, a.be, common.WorkspaceID(workspaceID))
	dod, err := a.be.DoD().Define(ctx, common.WorkspaceID(workspaceID), dodItems)
	if err != nil {
		return fmt.Errorf("define dod: %w", err)
	}
	run, err := a.be.Run().Activate(ctx, common.WorkspaceID(workspaceID), backend.ActivateRunRequest{
		GoalID:        goal.ID,
		DoDID:         dod.ID,
		LLMProviderID: llmProviderID,
	})
	if err != nil {
		return fmt.Errorf("activate execution: %w", err)
	}

	runIDMu.Lock()
	runID = run.ID
	runIDMu.Unlock()
	a.Send(Event{Type: EventRunCreated, Payload: RunCreatedInfo{RunID: string(run.ID), WorkspaceID: workspaceID}})

	select {
	case <-ctx.Done():
		a.Send(Event{Type: EventCancelled, Error: ctx.Err()})
		return ctx.Err()
	case err := <-runDone:
		return err
	}
}

func (a *Agent) handleBackendEvent(evt backend.Event, runDone chan<- error) {
	switch evt.Type {
	case backend.EventStepDispatched, backend.EventStepCompleted:
		a.Send(Event{Type: EventProgress, Payload: evt.Payload})
	case backend.EventDecisionCreated:
		ticketID := firstString(evt.Payload, "ticketId", "ID", "id", events.PayloadKeyBlockingTicketID)
		if ticketID == "" {
			ticketID = string(evt.RunID)
		}
		a.mu.Lock()
		if ticketID == a.notifiedDecision {
			a.mu.Unlock()
			return
		}
		a.notifiedDecision = ticketID
		a.mu.Unlock()
		a.Send(Event{Type: EventDecision, Payload: DecisionInfo{
			TicketID: ticketID,
			Question: firstString(evt.Payload, "question", "Question"),
			Reason:   firstString(evt.Payload, "reason", "Reason", events.PayloadKeyReason),
			RunID:    string(evt.RunID),
		}})
	case backend.EventApprovalCreated:
		approvalID := firstString(evt.Payload, "approvalId", "ID", "id", events.PayloadKeyBlockingApprovalID)
		if approvalID == "" {
			approvalID = string(evt.RunID)
		}
		a.mu.Lock()
		if approvalID == a.notifiedApproval {
			a.mu.Unlock()
			return
		}
		a.notifiedApproval = approvalID
		a.mu.Unlock()
		a.Send(Event{Type: EventApproval, Payload: ApprovalInfo{
			ApprovalID: approvalID,
			Reason:     firstString(evt.Payload, "reason", "Reason", events.PayloadKeyReason),
			Action:     firstString(evt.Payload, "action", "Action", events.PayloadKeyAction),
			Context:    mapFromPayload(evt.Payload, "context", "Context", events.PayloadKeyContext),
			RunID:      string(evt.RunID),
		}})
	case backend.EventResultAccepted:
		a.mu.Lock()
		a.lastResult = RunCompletedInfo{
			RunID:   string(evt.RunID),
			Summary: firstString(evt.Payload, "summary", "Summary", events.PayloadKeySummary),
		}
		a.mu.Unlock()
	case backend.EventRunStateChanged:
		state := firstString(evt.Payload, "state", "State", events.PayloadKeyState)
		switch common.RunState(state) {
		case common.RunStateCompleted:
			info := a.completedInfo(evt.RunID)
			a.Send(Event{Type: EventRunCompleted, Payload: info})
			select {
			case runDone <- nil:
			default:
			}
		case common.RunStateFailed:
			err := fmt.Errorf("%s", firstString(evt.Payload, "failureReason", "FailureReason", events.PayloadKeyFailureReason))
			a.Send(Event{Type: EventRunFailed, Error: err})
			select {
			case runDone <- err:
			default:
			}
		}
	}
}

func (a *Agent) ResolveDecision(ctx context.Context, ticketID string, resolutionType string, note string) error {
	if a.be == nil {
		return fmt.Errorf("agent backend is required")
	}
	_, err := a.be.Decision().Resolve(ctx, backend.ResolveDecisionRequest{
		TicketID:       common.TicketID(ticketID),
		ResolutionType: resolutionType,
		Payload:        map[string]any{"note": note},
	})
	return err
}

func (a *Agent) ResolveApproval(ctx context.Context, approvalID string, approved bool) error {
	if a.be == nil {
		return fmt.Errorf("agent backend is required")
	}
	if approved {
		_, err := a.be.Approval().Approve(ctx, common.ApprovalID(approvalID))
		return err
	}
	_, err := a.be.Approval().Reject(ctx, common.ApprovalID(approvalID))
	return err
}

func (a *Agent) ClearNotified() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notifiedDecision = ""
	a.notifiedApproval = ""
}

func (a *Agent) ResumeRun(ctx context.Context, runID string) error {
	a.ClearNotified()
	if a.be == nil {
		return fmt.Errorf("agent backend is required")
	}
	_, err := a.be.Run().Resume(ctx, common.RunID(runID))
	return err
}

func (a *Agent) completedInfo(runID common.RunID) RunCompletedInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	info := a.lastResult
	if info.RunID == "" {
		info.RunID = string(runID)
	}
	return info
}

func CreateWorkspaceWithBackend(ctx context.Context, be backend.Backend, title string, description string, workingDir string) (string, error) {
	ws, err := be.Workspace().Create(ctx, backend.CreateWorkspaceRequest{
		Title: title, Description: description, WorkingDirectory: workingDir,
	})
	if err != nil {
		return "", err
	}
	return string(ws.ID), nil
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func mapFromPayload(payload map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		value, ok := payload[key].(map[string]any)
		if !ok {
			continue
		}
		out := make(map[string]any, len(value))
		for k, v := range value {
			out[k] = v
		}
		return out
	}
	return nil
}
