package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
)

type Agent struct {
	session          *appservice.RuntimeSession
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
	return &Agent{
		events: make(chan Event, 64),
	}
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

func (a *Agent) Run(ctx context.Context, workspaceID string, goalText string, llmProviderID string) error {
	defer close(a.events)

	session, err := appservice.NewRuntimeSessionFromInitiator()
	if err != nil {
		return fmt.Errorf("create runtime session: %w", err)
	}
	a.session = session

	if err := ctx.Err(); err != nil {
		return err
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

func (a *Agent) pollRun(ctx context.Context, runID string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.Send(Event{Type: EventCancelled, Error: ctx.Err()})
			return ctx.Err()

		case <-ticker.C:
			a.Send(Event{Type: EventProgress})
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
							TicketID: ticketID,
							Question: ticket.Question,
							Reason:   ticket.Reason,
							RunID:    runID,
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
							ApprovalID: approvalID,
							Reason:     approval.Reason,
							RunID:      runID,
						}})
					}
				}
			}
		}
	}
}

func (a *Agent) ResolveDecision(ctx context.Context, ticketID string, resolutionType string, note string) error {
	session := a.session.Borrow()
	_, err := appservice.ResolveDecisionWithSession(ctx, session, ticketID, appservice.ResolveDecisionCommand{
		ResolutionType: resolutionType,
		ResolutionPayload: map[string]any{"note": note},
	})
	return err
}

func (a *Agent) ResolveApproval(ctx context.Context, approvalID string, approved bool) error {
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
	session := a.session.Borrow()
	a.notifiedDecision = ""
	a.notifiedApproval = ""
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

	// Retry result fetch up to 3 times (results may not be persisted when run completes).
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

	// Fall back to non-LLM step outputs.
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
