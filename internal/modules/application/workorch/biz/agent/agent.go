package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
)

type Agent struct {
	session          *appservice.RuntimeSession
	be               backend.Backend
	mu               sync.Mutex
	events           chan Event
	closeOnce        sync.Once
	notifiedDecision string
	notifiedApproval string
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

func New() *Agent {
	return &Agent{events: make(chan Event, 64)}
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
	a.closeOnce.Do(func() {
		if a.session != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			a.session.Close(ctx)
		}
	})
}

// Run executes the agent loop using the embedded runtime session (legacy path).
func (a *Agent) Run(ctx context.Context, workspaceID string, goalText string, llmProviderID string) error {
	defer close(a.events)
	session, err := appservice.NewRuntimeSessionFromInitiator()
	if err != nil {
		return fmt.Errorf("create runtime session: %w", err)
	}
	a.session = session
	return a.execute(ctx, workspaceID, goalText, llmProviderID)
}

// RunWithBackend executes the agent loop using the shared Backend.
// Progress is driven by EventHub subscriptions instead of fixed-interval polling.
func (a *Agent) RunWithBackend(ctx context.Context, workspaceID string, goalText string, llmProviderID string) error {
	defer close(a.events)

	// Subscribe to backend events for real-time progress.
	eventCtx, unsub := context.WithCancel(ctx)
	defer unsub()

	unsubscribe, err := a.be.Events().Subscribe(eventCtx, func(_ context.Context, evt backend.Event) {
		if evt.WorkspaceID != common.WorkspaceID(workspaceID) && evt.RunID == "" {
			return
		}
		switch evt.Type {
		case backend.EventDecisionCreated:
			if ticketID, ok := evt.Payload["ticketId"].(string); ok {
				if ticketID != a.notifiedDecision {
					a.notifiedDecision = ticketID
					question, _ := evt.Payload["question"].(string)
					reason, _ := evt.Payload["reason"].(string)
					a.Send(Event{Type: EventDecision, Payload: DecisionInfo{
						TicketID: ticketID, Question: question, Reason: reason, RunID: workspaceID,
					}})
				}
			}
		case backend.EventApprovalCreated:
			if approvalID, ok := evt.Payload["approvalId"].(string); ok {
				if approvalID != a.notifiedApproval {
					a.notifiedApproval = approvalID
					reason, _ := evt.Payload["reason"].(string)
					a.Send(Event{Type: EventApproval, Payload: ApprovalInfo{
						ApprovalID: approvalID, Reason: reason, RunID: workspaceID,
					}})
				}
			}
		case backend.EventRunStateChanged:
			if state, ok := evt.Payload["state"].(string); ok {
				if state == "completed" {
					a.Send(Event{Type: EventRunCompleted})
				} else if state == "failed" {
					msg, _ := evt.Payload["failureReason"].(string)
					a.Send(Event{Type: EventRunFailed, Error: fmt.Errorf("%s", msg)})
				}
			}
		}
	})
	if err == nil {
		defer unsubscribe()
	}

	return a.execute(ctx, workspaceID, goalText, llmProviderID)
}

func (a *Agent) execute(ctx context.Context, workspaceID string, goalText string, llmProviderID string) error {
	// Use backend if available; fall back to session.
	if a.be != nil {
		return a.executeWithBackend(ctx, workspaceID, goalText, llmProviderID)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	session := a.session
	if session == nil {
		var err error
		session, err = appservice.NewRuntimeSessionFromInitiator()
		if err != nil {
			return fmt.Errorf("create runtime session: %w", err)
		}
		a.session = session
	}

	goal, err := appservice.DefineGoalWithSession(ctx, session.Borrow(), workspaceID, goalText)
	if err != nil {
		return fmt.Errorf("define goal: %w", err)
	}

	dod, err := appservice.DefineDoDWithSession(ctx, session.Borrow(), workspaceID, []string{
		"目标已完成，输出包含明确的执行结果",
	})
	if err != nil {
		return fmt.Errorf("define dod: %w", err)
	}

	cmd := appservice.ActivateWorkspaceExecutionCommand{
		GoalID:        goal.ID,
		DoDID:         dod.ID,
		LLMProviderID: llmProviderID,
	}
	activation, err := appservice.ActivateWorkspaceExecutionWithSession(ctx, session.Borrow(), workspaceID, cmd)
	if err != nil {
		return fmt.Errorf("activate execution: %w", err)
	}

	a.Send(Event{Type: EventRunCreated, Payload: RunCreatedInfo{
		RunID:       activation.Run.ID,
		WorkspaceID: workspaceID,
	}})

	return a.pollRun(ctx, activation.Run.ID)
}

func (a *Agent) executeWithBackend(ctx context.Context, workspaceID string, goalText string, llmProviderID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	wsID := common.WorkspaceID(workspaceID)

	goal, err := a.be.Goal().Define(ctx, wsID, goalText)
	if err != nil {
		return fmt.Errorf("define goal: %w", err)
	}

	dod, err := a.be.DoD().Define(ctx, wsID, []string{"目标已完成，输出包含明确的执行结果"})
	if err != nil {
		return fmt.Errorf("define dod: %w", err)
	}

	run, err := a.be.Run().Activate(ctx, wsID, backend.ActivateRunRequest{
		GoalID:        goal.ID,
		DoDID:         dod.ID,
		LLMProviderID: llmProviderID,
	})
	if err != nil {
		return fmt.Errorf("activate execution: %w", err)
	}

	a.Send(Event{Type: EventRunCreated, Payload: RunCreatedInfo{
		RunID:       string(run.ID),
		WorkspaceID: workspaceID,
	}})

	// EventHub subscriptions drive progress with backend.
	return a.pollRunWithBackend(ctx, string(run.ID))
}

// pollRunWithBackend uses the backend's Status() API exclusively — no direct repository access.
func (a *Agent) pollRunWithBackend(ctx context.Context, runID string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.Send(Event{Type: EventCancelled, Error: ctx.Err()})
			return ctx.Err()
		case <-ticker.C:
			status, err := a.be.Run().Status(ctx, common.RunID(runID))
			if err != nil {
				return err
			}
			switch status.State {
			case common.RunStateCompleted:
				a.Send(Event{Type: EventRunCompleted})
				return nil
			case common.RunStateFailed:
				a.Send(Event{Type: EventRunFailed, Error: fmt.Errorf("%s", status.FailureReason)})
				return fmt.Errorf("run failed: %s", status.FailureReason)
			case common.RunStateBlockedOnDecision:
				ticketID := string(status.BlockingTicketID)
				if ticketID != a.notifiedDecision {
					a.notifiedDecision = ticketID
					dec, err := a.be.Decision().Get(ctx, common.TicketID(ticketID))
					if err == nil {
						a.Send(Event{Type: EventDecision, Payload: DecisionInfo{
							TicketID: ticketID, Question: dec.Question, Reason: dec.Reason, RunID: runID,
						}})
					}
				}
			case common.RunStateBlockedOnApproval:
				approvalID := string(status.BlockingApprovalID)
				if approvalID != a.notifiedApproval {
					a.notifiedApproval = approvalID
					appr, err := a.be.Approval().Get(ctx, common.ApprovalID(approvalID))
					if err == nil {
						a.Send(Event{Type: EventApproval, Payload: ApprovalInfo{
							ApprovalID: approvalID, Reason: appr.Reason, RunID: runID,
						}})
					}
				}
			default:
				a.Send(Event{Type: EventProgress})
			}
		}
	}
}

func (a *Agent) pollRun(ctx context.Context, runID string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.Send(Event{Type: EventCancelled, Error: ctx.Err()})
			return ctx.Err()

		case <-ticker.C:
			// Legacy session-based polling.
			session := a.session.Borrow()
			exports, err := session.Exports()
			if err != nil {
				return err
			}
			run, err := exports.Repositories.Runs.Get(ctx, common.RunID(runID))
			if err != nil {
				return err
			}
			switch run.State {
			case common.RunStateCompleted:
				a.Send(Event{Type: EventRunCompleted, Payload: a.buildCompletedInfo(ctx, runID)})
				return nil
			case common.RunStateFailed:
				msg := run.FailureReason
				if msg == "" {
					msg = "unknown failure"
				}
				a.Send(Event{Type: EventRunFailed, Error: fmt.Errorf("%s", msg)})
				return fmt.Errorf("run failed: %s", msg)
			case common.RunStateBlockedOnDecision:
				ticketID := string(run.BlockingTicketID)
				if ticketID != a.notifiedDecision {
					a.notifiedDecision = ticketID
					ticket, err := exports.Repositories.Tickets.Get(ctx, run.BlockingTicketID)
					if err == nil {
						a.Send(Event{Type: EventDecision, Payload: DecisionInfo{
							TicketID: ticketID, Question: ticket.Question, Reason: ticket.Reason, RunID: runID,
						}})
					}
				}
			case common.RunStateBlockedOnApproval:
				approvalID := string(run.BlockingApprovalID)
				if approvalID != a.notifiedApproval {
					a.notifiedApproval = approvalID
					approval, err := exports.Repositories.Approvals.Get(ctx, run.BlockingApprovalID)
					if err == nil {
						a.Send(Event{Type: EventApproval, Payload: ApprovalInfo{
							ApprovalID: approvalID, Reason: approval.Reason, RunID: runID,
						}})
					}
				}
			default:
				a.Send(Event{Type: EventProgress})
			}
		}
	}
}

func (a *Agent) ResolveDecision(ctx context.Context, ticketID string, resolutionType string, note string) error {
	if a.be != nil {
		_, err := a.be.Decision().Resolve(ctx, backend.ResolveDecisionRequest{
			TicketID: common.TicketID(ticketID), ResolutionType: resolutionType,
			Payload: map[string]any{"note": note},
		})
		return err
	}
	session := a.session.Borrow()
	_, err := appservice.ResolveDecisionWithSession(ctx, session, ticketID, appservice.ResolveDecisionCommand{
		ResolutionType: resolutionType, ResolutionPayload: map[string]any{"note": note},
	})
	return err
}

func (a *Agent) ResolveApproval(ctx context.Context, approvalID string, approved bool) error {
	if a.be != nil {
		if approved {
			_, err := a.be.Approval().Approve(ctx, common.ApprovalID(approvalID))
			return err
		}
		_, err := a.be.Approval().Reject(ctx, common.ApprovalID(approvalID))
		return err
	}
	session := a.session.Borrow()
	if approved {
		_, err := appservice.ApproveApprovalWithSession(ctx, session, approvalID)
		return err
	}
	_, err := appservice.RejectApprovalWithSession(ctx, session, approvalID)
	return err
}

func (a *Agent) ClearNotified() {
	a.notifiedDecision = ""
	a.notifiedApproval = ""
}

func (a *Agent) ResumeRun(ctx context.Context, runID string) error {
	a.notifiedDecision = ""
	a.notifiedApproval = ""
	if a.be != nil {
		_, err := a.be.Run().Resume(ctx, common.RunID(runID))
		return err
	}
	session := a.session.Borrow()
	_, err := appservice.ResumeRunWithSession(ctx, session, runID)
	return err
}

func (a *Agent) buildCompletedInfo(ctx context.Context, runID string) RunCompletedInfo {
	info := RunCompletedInfo{RunID: runID}
	session := a.session.Borrow()
	exports, err := session.Exports()
	if err != nil {
		return info
	}

	steps, _ := exports.Repositories.Steps.ListByRun(ctx, common.RunID(runID))
	info.StepCount = len(steps)

	for _, s := range steps {
		if s.State == "completed" && s.Output != nil && !strings.HasPrefix(s.Kind, "llm/") {
			output := StepOutput{Kind: s.Kind}
			output.Summary = extractString(s.Output, "summary", "exitSummary", "stdout")
			if output.Summary == "" {
				output.Summary = extractString(s.Output, "output", "result")
			}
			if output.Summary != "" {
				info.Outputs = append(info.Outputs, output)
			}
		}
	}

	var results []*resultmodel.Result
	for attempt := 0; attempt < 3; attempt++ {
		results, _ = exports.Repositories.Results.ListByRun(ctx, common.RunID(runID))
		if len(results) > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	for _, r := range results {
		if r.State == "accepted" {
			if r.SummaryDetail.GoalOutcome != "" {
				info.Summary = r.SummaryDetail.GoalOutcome
			} else if r.SummaryDetail.ImplementationSummary != "" {
				info.Summary = r.SummaryDetail.ImplementationSummary
			} else if r.SummaryDetail.VerificationSummary != "" {
				info.Summary = r.SummaryDetail.VerificationSummary
			} else if r.Summary != "" {
				info.Summary = r.Summary
			}
			if r.DoDAssessment.Satisfied {
				info.DoDSatisfied = true
			}
		}
	}
	if info.Summary == "" {
		for _, s := range steps {
			if s.State == "completed" && s.Output != nil && !strings.HasPrefix(s.Kind, "llm/") {
				if v := extractString(s.Output, "summary", "stdout", "exitSummary"); v != "" {
					info.Summary = v
					break
				}
			}
		}
	}
	return info
}

func extractString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func CreateWorkspace(ctx context.Context, title string, description string) (string, error) {
	session, err := appservice.NewRuntimeSessionFromInitiator()
	if err != nil {
		return "", fmt.Errorf("create runtime session: %w", err)
	}
	defer session.Close(ctx)
	ws, err := appservice.CreateWorkspaceWithSession(ctx, session.Borrow(), title, description, "")
	if err != nil {
		return "", err
	}
	return ws.ID, nil
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
