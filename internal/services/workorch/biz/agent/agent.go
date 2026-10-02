package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
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
	Type         EventType
	RunCreated   *RunCreatedInfo
	Decision     *DecisionInfo
	Approval     *ApprovalInfo
	RunCompleted *RunCompletedInfo
	Error        error
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
	TicketID    string
	Question    string
	Reason      string
	WorkspaceID string
	RunID       string
}

type ApprovalInfo struct {
	ApprovalID  string
	Reason      string
	Action      string
	Context     workorcheventpayload.Payload
	WorkspaceID string
	RunID       string
}

type RunCreatedInfo struct {
	RunID       string
	WorkspaceID string
}

type RunCompletedInfo struct {
	RunID        string
	Summary      string
	Sources      []string
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

func (a *Agent) RunWithBackend(ctx context.Context, workspaceID string, goalText string) error {
	defer close(a.events)
	return fmt.Errorf("%w: Goal-first agent 执行已退休，请通过 Web 完成规格创作、审查与确认，再显式提交工作空间执行意图", common.ErrConflict)
}

func (a *Agent) handleBackendEvent(ctx context.Context, evt backend.Event, runDone chan<- error) {
	switch evt.Type {
	case backend.EventDecisionCreated:
		ticketID := firstString(evt.Payload, "ticketId", "ID", "id", backend.PayloadKeyBlockingTicketID)
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
		info := DecisionInfo{
			TicketID:    ticketID,
			Question:    firstString(evt.Payload, "question", "Question"),
			Reason:      firstString(evt.Payload, "reason", "Reason", backend.PayloadKeyReason),
			WorkspaceID: string(evt.WorkspaceID),
			RunID:       string(evt.RunID),
		}
		a.Send(Event{Type: EventDecision, Decision: &info})
	case backend.EventResultAccepted:
		a.mu.Lock()
		a.lastResult = RunCompletedInfo{
			RunID:   string(evt.RunID),
			Summary: firstString(evt.Payload, "summary", "Summary", backend.PayloadKeySummary),
			Sources: a.webSourceLabels(ctx, evt.WorkspaceID, evt.RunID),
		}
		a.mu.Unlock()
	case backend.EventRunStateChanged:
		state := firstString(evt.Payload, "state", "State", backend.PayloadKeyState)
		switch common.RunState(state) {
		case common.RunStateCompleted:
			info := a.completedInfo(evt.RunID)
			a.Send(Event{Type: EventRunCompleted, RunCompleted: &info})
			select {
			case runDone <- nil:
			default:
			}
		case common.RunStateFailed:
			err := fmt.Errorf("%s", firstString(evt.Payload, "failureReason", "FailureReason", backend.PayloadKeyFailureReason))
			a.Send(Event{Type: EventRunFailed, Error: err})
			select {
			case runDone <- err:
			default:
			}
		}
	}
}

func (a *Agent) ResolveDecision(ctx context.Context, workspaceID string, ticketID string, resolutionType string, note string) error {
	if a.be == nil {
		return fmt.Errorf("agent backend is required")
	}
	wsID := common.WorkspaceID(strings.TrimSpace(workspaceID))
	if wsID == "" {
		return fmt.Errorf("workspace is required")
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
	_, err := a.be.Decision().Resolve(ctx, backend.ResolveDecisionRequest{
		WorkspaceID:    wsID,
		TicketID:       common.TicketID(ticketID),
		ResolutionType: resolutionType,
		Payload:        workorcheventpayload.New(workorcheventpayload.StringField("note", note)),
	})
	return err
}

func (a *Agent) ResolveApproval(ctx context.Context, workspaceID string, approvalID string, approved bool) error {
	if a.be == nil {
		return fmt.Errorf("agent backend is required")
	}
	wsID := common.WorkspaceID(strings.TrimSpace(workspaceID))
	if wsID == "" {
		return fmt.Errorf("workspace is required")
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
	if approved {
		_, err := a.be.Approval().Approve(ctx, wsID, common.ApprovalID(approvalID), workorcheventpayload.Payload{})
		return err
	}
	_, err := a.be.Approval().Reject(ctx, wsID, common.ApprovalID(approvalID), workorcheventpayload.Payload{})
	return err
}

func (a *Agent) ClearNotified() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notifiedDecision = ""
	a.notifiedApproval = ""
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

func (a *Agent) webSourceLabels(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) []string {
	if a == nil || a.be == nil || workspaceID == "" {
		return nil
	}
	snapshotBackend, ok := a.be.Workspace().(interface {
		Snapshot(context.Context, common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error)
	})
	if !ok {
		return nil
	}
	snapshot, err := snapshotBackend.Snapshot(ctx, workspaceID)
	if err != nil || snapshot == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0)
	add := func(kind string, title string, source string, url string, evidenceRef string) {
		url = strings.TrimSpace(url)
		if url == "" {
			return
		}
		label := firstNonEmpty(strings.TrimSpace(title), strings.TrimSpace(source), url)
		if label != url {
			label += " - " + url
		}
		if evidenceRef != "" {
			label += " (" + evidenceRef + ")"
		}
		label = kind + ": " + label
		if _, ok := seen[label]; ok {
			return
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	_ = runID
	for _, trace := range snapshot.WebSources {
		for _, source := range trace.FetchedSources {
			add("fetch", source.Title, source.Source, firstNonEmpty(source.FinalURL, source.URL), source.EvidenceRef)
		}
		for _, source := range trace.Sources {
			add("search", source.Title, source.Source, source.URL, source.EvidenceRef)
		}
	}
	if len(out) > 8 {
		return out[:8]
	}
	return out
}

func CreateWorkspaceWithBackend(ctx context.Context, be backend.Backend, title string, description string, workingDir string) (string, error) {
	ws, err := be.Workspace().Create(ctx, workorchmanagementcontract.WorkspaceCreateRequest{
		Title: title, Description: description, DeliverableDirectory: workingDir,
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
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
