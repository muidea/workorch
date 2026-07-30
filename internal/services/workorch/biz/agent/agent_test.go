package agent

import (
	"context"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAgentChannelLifecycle(t *testing.T) {
	a := NewWithBackend(newFakeBackend())
	if a.Events() == nil {
		t.Fatal("expected non-nil events channel")
	}

	runCreated := RunCreatedInfo{RunID: "test-run", WorkspaceID: "ws-1"}
	a.Send(Event{Type: EventRunCreated, RunCreated: &runCreated})

	select {
	case evt := <-a.Events():
		if evt.Type != EventRunCreated {
			t.Fatalf("expected EventRunCreated, got %s", evt.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	a.Close()
	a.Close()
}

func TestAgentRunCompletesFromBackendEvent(t *testing.T) {
	be := newFakeBackend()
	a := NewWithBackend(be)
	done := make(chan error, 1)
	go func() {
		done <- a.RunWithBackend(context.Background(), "ws-1", "test goal")
	}()

	select {
	case evt := <-a.Events():
		if evt.Type != EventRunCreated {
			t.Fatalf("first event = %s, want run created", evt.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for run created")
	}

	be.emit(backend.Event{
		Type:        backend.EventRunStateChanged,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     map[string]any{"state": string(common.RunStateCompleted)},
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunWithBackend error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for agent completion")
	}
	if be.runCommand.WorkspaceID != "ws-1" ||
		be.runCommand.Intent != backend.RunCommandIntentResume ||
		be.runCommand.Reason != "user_requested" ||
		be.runCommand.Source != "agent" ||
		be.runCommand.Activation.GoalID != "goal-1" ||
		be.runCommand.Activation.DoDID != "dod-1" {
		t.Fatalf("run command = %+v, want agent first-run resume command", be.runCommand)
	}
	if be.activateCalled {
		t.Fatal("agent first run used legacy Run.Activate path")
	}
}

func TestAgentEventTypes(t *testing.T) {
	types := map[EventType]string{
		EventRunCreated:   "run_created",
		EventDecision:     "decision",
		EventApproval:     "approval",
		EventRunCompleted: "run_completed",
		EventRunFailed:    "run_failed",
		EventCancelled:    "cancelled",
		EventProgress:     "progress",
	}
	for typ, expected := range types {
		if string(typ) != expected {
			t.Fatalf("EventType %v = %q, want %q", typ, string(typ), expected)
		}
	}
}

func TestDecisionApprovalInfoTypes(t *testing.T) {
	d := DecisionInfo{TicketID: "t-1", Question: "q?", Reason: "r", RunID: "run-1"}
	if d.TicketID != "t-1" || d.Question != "q?" || d.Reason != "r" || d.RunID != "run-1" {
		t.Fatal("DecisionInfo field mismatch")
	}
	a := ApprovalInfo{ApprovalID: "a-1", Reason: "reason", RunID: "run-1"}
	if a.ApprovalID != "a-1" || a.Reason != "reason" || a.RunID != "run-1" {
		t.Fatal("ApprovalInfo field mismatch")
	}
	r := RunCreatedInfo{RunID: "run-1", WorkspaceID: "ws-1"}
	if r.RunID != "run-1" || r.WorkspaceID != "ws-1" {
		t.Fatal("RunCreatedInfo field mismatch")
	}
}

func TestAgentDecisionAndApprovalResolutionRequireWorkspaceScope(t *testing.T) {
	decisionBackend := &recordingAgentDecisionBackend{}
	approvalBackend := &recordingAgentApprovalBackend{}
	a := NewWithBackend(agentResolutionBackend{
		fakeBackend: newFakeBackend(),
		decision:    decisionBackend,
		approval:    approvalBackend,
	})

	if err := a.ResolveDecision(context.Background(), "", "ticket-agent", "provide_missing_info", "answer"); err == nil {
		t.Fatal("ResolveDecision error = nil, want workspace required")
	}
	if err := a.ResolveApproval(context.Background(), "", "approval-agent", true); err == nil {
		t.Fatal("ResolveApproval error = nil, want workspace required")
	}
	if len(decisionBackend.requests) != 0 {
		t.Fatalf("decision requests = %+v, want no unscoped request", decisionBackend.requests)
	}
	if len(approvalBackend.approvals) != 0 || len(approvalBackend.rejections) != 0 {
		t.Fatalf("approval calls = %+v/%+v, want no unscoped request", approvalBackend.approvals, approvalBackend.rejections)
	}
}

func TestAgentDecisionAndApprovalResolutionUseScopedCommands(t *testing.T) {
	decisionBackend := &recordingAgentDecisionBackend{}
	approvalBackend := &recordingAgentApprovalBackend{}
	base := newFakeBackend()
	a := NewWithBackend(agentResolutionBackend{
		fakeBackend: base,
		decision:    decisionBackend,
		approval:    approvalBackend,
	})

	if err := a.ResolveDecision(context.Background(), "ws-agent", "ticket-agent", "provide_missing_info", "answer"); err != nil {
		t.Fatalf("ResolveDecision: %v", err)
	}
	if err := a.ResolveApproval(context.Background(), "ws-agent", "approval-agent", true); err != nil {
		t.Fatalf("ResolveApproval approve: %v", err)
	}
	if err := a.ResolveApproval(context.Background(), "ws-agent", "approval-reject-agent", false); err != nil {
		t.Fatalf("ResolveApproval reject: %v", err)
	}

	if len(decisionBackend.requests) != 1 {
		t.Fatalf("decision requests = %+v, want one request", decisionBackend.requests)
	}
	gotDecision := decisionBackend.requests[0]
	note, _ := gotDecision.Payload.String("note")
	if gotDecision.WorkspaceID != "ws-agent" ||
		gotDecision.TicketID != "ticket-agent" ||
		gotDecision.ResolutionType != "provide_missing_info" ||
		note != "answer" {
		t.Fatalf("decision request = %+v, want scoped resolution", gotDecision)
	}
	if len(approvalBackend.approvals) != 1 ||
		approvalBackend.approvals[0].WorkspaceID != "ws-agent" ||
		approvalBackend.approvals[0].ApprovalID != "approval-agent" {
		t.Fatalf("approval calls = %+v, want scoped approve", approvalBackend.approvals)
	}
	if len(approvalBackend.rejections) != 1 ||
		approvalBackend.rejections[0].WorkspaceID != "ws-agent" ||
		approvalBackend.rejections[0].ApprovalID != "approval-reject-agent" {
		t.Fatalf("rejection calls = %+v, want scoped reject", approvalBackend.rejections)
	}
	// A governance resolution is an owner fact. Its Runtime V2 continuation is
	// emitted by Orchestrator, so client code must not append a second resume.
	if base.runCommand.WorkspaceID != "" || base.runCommand.ExpectedRunID != "" ||
		base.runCommand.Intent != "" || base.runCommand.Reason != "" || base.runCommand.Source != "" {
		t.Fatalf("resolution unexpectedly issued a Run command: %+v", base.runCommand)
	}
}

type fakeBackend struct {
	handlers       []backend.EventHandler
	runCommand     backend.WorkspaceRunCommand
	activateCalled bool
}

func newFakeBackend() *fakeBackend { return &fakeBackend{} }

func (b *fakeBackend) Workspace() backend.WorkspaceBackend { return fakeWorkspace{} }
func (b *fakeBackend) Goal() backend.GoalBackend           { return fakeGoal{} }
func (b *fakeBackend) DoD() backend.DoDBackend             { return fakeDoD{} }
func (b *fakeBackend) Run() backend.RunBackend             { return fakeRun{parent: b} }
func (b *fakeBackend) Decision() backend.DecisionBackend   { return fakeDecision{} }
func (b *fakeBackend) Approval() backend.ApprovalBackend   { return fakeApproval{} }
func (b *fakeBackend) Soul() backend.SoulBackend           { return fakeSoul{} }
func (b *fakeBackend) Skill() backend.SkillBackend         { return fakeSkill{} }
func (b *fakeBackend) MCP() backend.MCPBackend             { return fakeMCP{} }
func (b *fakeBackend) Memory() backend.MemoryBackend       { return fakeMemory{} }
func (b *fakeBackend) Template() backend.TemplateBackend   { return fakeTemplate{} }
func (b *fakeBackend) Config() backend.ConfigBackend       { return fakeConfig{} }
func (b *fakeBackend) Events() backend.EventsBackend       { return fakeEvents{b: b} }
func (b *fakeBackend) ProjectionEvents() backend.WorkspaceProjectionEventsBackend {
	return nil
}

func (b *fakeBackend) emit(evt backend.Event) {
	for _, handler := range b.handlers {
		handler(context.Background(), evt)
	}
}

type fakeWorkspace struct{}

func (fakeWorkspace) Create(context.Context, backend.CreateWorkspaceRequest) (*backend.WorkspaceInfo, error) {
	return &backend.WorkspaceInfo{ID: "ws-1"}, nil
}
func (fakeWorkspace) Get(context.Context, common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	return &backend.WorkspaceInfo{ID: "ws-1"}, nil
}
func (fakeWorkspace) List(context.Context) ([]backend.WorkspaceInfo, error) { return nil, nil }
func (fakeWorkspace) Snapshot(context.Context, common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error) {
	return &backend.WorkspaceSnapshotInfo{Workspace: backend.WorkspaceInfo{ID: "ws-1"}}, nil
}

func (fakeWorkspace) RuntimeView(context.Context, backend.ReadWorkspaceRuntimeViewRequest) (*backend.WorkspaceRuntimeViewInfo, error) {
	return nil, nil
}

func (fakeWorkspace) UpdateLLMTarget(context.Context, common.WorkspaceID, *runtimecontract.ProviderModelTarget) (*backend.WorkspaceInfo, error) {
	return &backend.WorkspaceInfo{ID: "ws-1"}, nil
}

func (fakeWorkspace) Delete(context.Context, common.WorkspaceID, bool) error { return nil }

type fakeGoal struct{}

func (fakeGoal) Define(context.Context, common.WorkspaceID, string) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{ID: "goal-1"}, nil
}
func (fakeGoal) List(context.Context, common.WorkspaceID) ([]backend.GoalInfo, error) {
	return nil, nil
}
func (fakeGoal) Get(context.Context, common.WorkspaceID, common.GoalID) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{}, nil
}
func (fakeGoal) Update(context.Context, common.WorkspaceID, common.GoalID, string) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{}, nil
}

type fakeDoD struct{}

func (fakeDoD) Define(context.Context, common.WorkspaceID, []string) (*backend.DoDInfo, error) {
	return &backend.DoDInfo{ID: "dod-1"}, nil
}
func (fakeDoD) List(context.Context, common.WorkspaceID) ([]backend.DoDInfo, error) { return nil, nil }
func (fakeDoD) Get(context.Context, common.WorkspaceID, common.DoDID) (*backend.DoDInfo, error) {
	return &backend.DoDInfo{}, nil
}
func (fakeDoD) Update(context.Context, common.WorkspaceID, common.DoDID, []string) (*backend.DoDInfo, error) {
	return &backend.DoDInfo{}, nil
}

type fakeRun struct{ parent *fakeBackend }

func (f fakeRun) Command(_ context.Context, req backend.WorkspaceRunCommand) (*backend.RunInfo, error) {
	if f.parent != nil {
		f.parent.runCommand = req
	}
	return &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive}, nil
}
func (f fakeRun) Activate(context.Context, common.WorkspaceID, backend.ActivateRunRequest) (*backend.RunInfo, error) {
	if f.parent != nil {
		f.parent.activateCalled = true
	}
	return &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive}, nil
}
func (fakeRun) Restart(context.Context, common.WorkspaceID, backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive}, nil
}
func (fakeRun) Resume(context.Context, common.WorkspaceID, common.RunID, ...map[string]any) (*backend.RunInfo, error) {
	return &backend.RunInfo{}, nil
}
func (fakeRun) Get(context.Context, common.WorkspaceID, common.RunID) (*backend.RunInfo, error) {
	return &backend.RunInfo{}, nil
}
func (fakeRun) List(context.Context, common.WorkspaceID) ([]backend.RunInfo, error) { return nil, nil }
func (fakeRun) Status(context.Context, common.WorkspaceID, common.RunID) (*backend.RunStatusInfo, error) {
	return &backend.RunStatusInfo{}, nil
}

type agentResolutionBackend struct {
	*fakeBackend
	decision backend.DecisionBackend
	approval backend.ApprovalBackend
}

func (b agentResolutionBackend) Decision() backend.DecisionBackend { return b.decision }
func (b agentResolutionBackend) Approval() backend.ApprovalBackend { return b.approval }

type recordingAgentDecisionBackend struct {
	requests []backend.ResolveDecisionRequest
}

func (b *recordingAgentDecisionBackend) Resolve(_ context.Context, req backend.ResolveDecisionRequest) (*backend.DecisionInfo, error) {
	b.requests = append(b.requests, req)
	return &backend.DecisionInfo{ID: req.TicketID, WorkspaceID: req.WorkspaceID, State: "resolved"}, nil
}
func (b *recordingAgentDecisionBackend) BatchResolve(context.Context, common.WorkspaceID, []common.TicketID, string, workorcheventpayload.Payload) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (b *recordingAgentDecisionBackend) List(context.Context, common.WorkspaceID) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (b *recordingAgentDecisionBackend) Get(context.Context, common.WorkspaceID, common.TicketID) (*backend.DecisionInfo, error) {
	return nil, common.ErrNotFound
}

type recordingAgentApprovalCall struct {
	WorkspaceID common.WorkspaceID
	ApprovalID  common.ApprovalID
}

type recordingAgentApprovalBackend struct {
	approvals  []recordingAgentApprovalCall
	rejections []recordingAgentApprovalCall
}

func (b *recordingAgentApprovalBackend) Approve(_ context.Context, wsID common.WorkspaceID, id common.ApprovalID, _ ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	b.approvals = append(b.approvals, recordingAgentApprovalCall{WorkspaceID: wsID, ApprovalID: id})
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, State: string(common.ApprovalStateApproved)}, nil
}
func (b *recordingAgentApprovalBackend) Reject(_ context.Context, wsID common.WorkspaceID, id common.ApprovalID, _ ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	b.rejections = append(b.rejections, recordingAgentApprovalCall{WorkspaceID: wsID, ApprovalID: id})
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, State: string(common.ApprovalStateRejected)}, nil
}
func (b *recordingAgentApprovalBackend) BatchResolve(context.Context, common.WorkspaceID, []common.ApprovalID, string, ...workorcheventpayload.Payload) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (b *recordingAgentApprovalBackend) List(context.Context, common.WorkspaceID) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (b *recordingAgentApprovalBackend) Get(context.Context, common.WorkspaceID, common.ApprovalID) (*backend.ApprovalInfo, error) {
	return nil, common.ErrNotFound
}

type fakeDecision struct{}

func (fakeDecision) Resolve(context.Context, backend.ResolveDecisionRequest) (*backend.DecisionInfo, error) {
	return &backend.DecisionInfo{}, nil
}
func (fakeDecision) BatchResolve(context.Context, common.WorkspaceID, []common.TicketID, string, workorcheventpayload.Payload) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (fakeDecision) List(context.Context, common.WorkspaceID) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (fakeDecision) Get(context.Context, common.WorkspaceID, common.TicketID) (*backend.DecisionInfo, error) {
	return &backend.DecisionInfo{}, nil
}

type fakeApproval struct{}

func (fakeApproval) Approve(context.Context, common.WorkspaceID, common.ApprovalID, ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{}, nil
}
func (fakeApproval) Reject(context.Context, common.WorkspaceID, common.ApprovalID, ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{}, nil
}
func (fakeApproval) BatchResolve(context.Context, common.WorkspaceID, []common.ApprovalID, string, ...workorcheventpayload.Payload) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (fakeApproval) List(context.Context, common.WorkspaceID) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (fakeApproval) Get(context.Context, common.WorkspaceID, common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{}, nil
}

type fakeSoul struct{}
type fakeSkill struct{}
type fakeMCP struct{}
type fakeMemory struct{}
type fakeTemplate struct{}
type fakeConfig struct{}

func (fakeSoul) Create(context.Context, *backend.SoulInfo) (*backend.SoulInfo, error) {
	return &backend.SoulInfo{}, nil
}
func (fakeSoul) Get(context.Context, common.SoulID) (*backend.SoulInfo, error) {
	return &backend.SoulInfo{}, nil
}
func (fakeSoul) List(context.Context) ([]backend.SoulInfo, error) { return nil, nil }
func (fakeSoul) Update(context.Context, common.SoulID, *backend.SoulInfo) (*backend.SoulInfo, error) {
	return &backend.SoulInfo{}, nil
}
func (fakeSoul) Delete(context.Context, common.SoulID) error { return nil }

func (fakeSkill) Create(context.Context, *backend.SkillInfo) (*backend.SkillInfo, error) {
	return &backend.SkillInfo{}, nil
}
func (fakeSkill) Get(context.Context, common.SkillID) (*backend.SkillInfo, error) {
	return &backend.SkillInfo{}, nil
}
func (fakeSkill) List(context.Context) ([]backend.SkillInfo, error) { return nil, nil }

func (fakeMCP) CreateServer(context.Context, *backend.MCPServerInfo) (*backend.MCPServerInfo, error) {
	return &backend.MCPServerInfo{}, nil
}
func (fakeMCP) GetServer(context.Context, common.MCPServerID) (*backend.MCPServerInfo, error) {
	return &backend.MCPServerInfo{}, nil
}
func (fakeMCP) ListServers(context.Context) ([]backend.MCPServerInfo, error) { return nil, nil }
func (fakeMCP) UpdateServer(context.Context, common.MCPServerID, *backend.MCPServerInfo) (*backend.MCPServerInfo, error) {
	return &backend.MCPServerInfo{}, nil
}
func (fakeMCP) ListCapabilities(context.Context) ([]backend.MCPCapabilityInfo, error) {
	return nil, nil
}

func (fakeMemory) Create(context.Context, common.WorkspaceID, *backend.MemoryItemInfo) (*backend.MemoryItemInfo, error) {
	return &backend.MemoryItemInfo{}, nil
}
func (fakeMemory) Get(context.Context, common.WorkspaceID, common.MemoryID) (*backend.MemoryItemInfo, error) {
	return &backend.MemoryItemInfo{}, nil
}
func (fakeMemory) Update(context.Context, common.WorkspaceID, common.MemoryID, *backend.MemoryItemInfo) (*backend.MemoryItemInfo, error) {
	return &backend.MemoryItemInfo{}, nil
}
func (fakeMemory) Delete(context.Context, common.WorkspaceID, common.MemoryID) error {
	return nil
}
func (fakeMemory) List(context.Context, common.WorkspaceID) ([]backend.MemoryItemInfo, error) {
	return nil, nil
}
func (fakeMemory) Search(context.Context, common.WorkspaceID, backend.MemorySearchRequest) ([]backend.MemoryItemInfo, error) {
	return nil, nil
}

func (fakeTemplate) Create(context.Context, *backend.TemplateInfo) (*backend.TemplateInfo, error) {
	return &backend.TemplateInfo{}, nil
}
func (fakeTemplate) Get(context.Context, string) (*backend.TemplateInfo, error) {
	return &backend.TemplateInfo{}, nil
}
func (fakeTemplate) List(context.Context) ([]backend.TemplateInfo, error) { return nil, nil }
func (fakeTemplate) Update(context.Context, string, *backend.TemplateInfo) (*backend.TemplateInfo, error) {
	return &backend.TemplateInfo{}, nil
}
func (fakeTemplate) Delete(context.Context, string) error { return nil }

func (fakeConfig) Get(context.Context) (*backend.ServiceConfig, error) {
	return &backend.ServiceConfig{}, nil
}

func (fakeConfig) ListChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	return nil, nil
}

func (fakeConfig) RefreshChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	return nil, nil
}

type fakeEvents struct{ b *fakeBackend }

func (f fakeEvents) Subscribe(ctx context.Context, handler backend.EventHandler) (func(), error) {
	f.b.handlers = append(f.b.handlers, handler)
	return func() {}, nil
}
