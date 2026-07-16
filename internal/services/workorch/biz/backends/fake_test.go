package backend_test

import (
	"context"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	backend "github.com/muidea/workorch/internal/services/workorch/biz/backends"
)

// fakeBackend implements backend.Backend for deterministic TUI and agent tests.
type fakeBackend struct {
	mu sync.Mutex

	workspaces []backend.WorkspaceInfo
	goals      []backend.GoalInfo
	dods       []backend.DoDInfo
	runs       []backend.RunInfo
	decisions  []backend.DecisionInfo
	approvals  []backend.ApprovalInfo
	templates  []backend.TemplateInfo

	// Recorded commands for assertion.
	ActivateCalls        []backend.ActivateRunRequest
	ResolveCalls         []backend.ResolveDecisionRequest
	ApproveCalls         []string
	RejectCalls          []string
	CreateWorkspaceCalls []backend.CreateWorkspaceRequest

	// Pre-configured responses.
	NextRunID       string
	NextGoalID      string
	NextDoDID       string
	NextWorkspaceID string

	eventHandlers []backend.EventHandler
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		workspaces: make([]backend.WorkspaceInfo, 0),
		goals:      make([]backend.GoalInfo, 0),
		dods:       make([]backend.DoDInfo, 0),
		runs:       make([]backend.RunInfo, 0),
		decisions:  make([]backend.DecisionInfo, 0),
		approvals:  make([]backend.ApprovalInfo, 0),
	}
}

func (b *fakeBackend) Workspace() backend.WorkspaceBackend { return &fakeWS{b} }
func (b *fakeBackend) Goal() backend.GoalBackend           { return &fakeGoal{b} }
func (b *fakeBackend) DoD() backend.DoDBackend             { return &fakeDoD{b} }
func (b *fakeBackend) Run() backend.RunBackend             { return &fakeRun{b} }
func (b *fakeBackend) Decision() backend.DecisionBackend   { return &fakeDec{b} }
func (b *fakeBackend) Approval() backend.ApprovalBackend   { return &fakeAppr{b} }
func (b *fakeBackend) Soul() backend.SoulBackend           { return &fakeSoul{} }
func (b *fakeBackend) Skill() backend.SkillBackend         { return &fakeSkill{} }
func (b *fakeBackend) MCP() backend.MCPBackend             { return &fakeMCP{} }
func (b *fakeBackend) Memory() backend.MemoryBackend       { return &fakeMem{} }
func (b *fakeBackend) Template() backend.TemplateBackend   { return &fakeTmpl{b} }
func (b *fakeBackend) Config() backend.ConfigBackend       { return &fakeCfg{} }
func (b *fakeBackend) Events() backend.EventsBackend       { return &fakeEvents{b} }
func (b *fakeBackend) ProjectionEvents() backend.WorkspaceProjectionEventsBackend {
	return nil
}

// Emit sends a backend event to all subscribers.
func (b *fakeBackend) Emit(ctx context.Context, evt backend.Event) {
	b.mu.Lock()
	handlers := make([]backend.EventHandler, len(b.eventHandlers))
	copy(handlers, b.eventHandlers)
	b.mu.Unlock()
	for _, h := range handlers {
		h(ctx, evt)
	}
}

// ── Workspace ──

type fakeWS struct{ b *fakeBackend }

func (f *fakeWS) Create(ctx context.Context, req backend.CreateWorkspaceRequest) (*backend.WorkspaceInfo, error) {
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	f.b.CreateWorkspaceCalls = append(f.b.CreateWorkspaceCalls, req)
	id := common.WorkspaceID("ws-test-001")
	if req.Title != "" {
		id = common.WorkspaceID("ws-" + req.Title)
	}
	ws := &backend.WorkspaceInfo{
		ID: id, Title: req.Title, Description: req.Description,
		DeliverableDirectory: req.DeliverableDirectory, State: "active",
	}
	f.b.workspaces = append(f.b.workspaces, *ws)
	return ws, nil
}
func (f *fakeWS) Get(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	for _, w := range f.b.workspaces {
		if w.ID == id {
			return &w, nil
		}
	}
	return nil, common.ErrNotFound
}
func (f *fakeWS) List(ctx context.Context) ([]backend.WorkspaceInfo, error) {
	return f.b.workspaces, nil
}
func (f *fakeWS) Snapshot(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error) {
	workspace, err := f.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	return &backend.WorkspaceSnapshotInfo{
		Workspace: *workspace,
		Goals:     append([]backend.GoalInfo(nil), f.b.goals...),
		DoDs:      append([]backend.DoDInfo(nil), f.b.dods...),
		Runs:      append([]backend.RunInfo(nil), f.b.runs...),
		Decisions: append([]backend.DecisionInfo(nil), f.b.decisions...),
		Approvals: append([]backend.ApprovalInfo(nil), f.b.approvals...),
	}, nil
}

func (f *fakeWS) RuntimeView(context.Context, backend.ReadWorkspaceRuntimeViewRequest) (*backend.WorkspaceRuntimeViewInfo, error) {
	return nil, nil
}

func (f *fakeWS) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*backend.WorkspaceInfo, error) {
	item, err := f.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	item.DefaultLLMTarget = target
	return item, nil
}

func (f *fakeWS) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error { return nil }

// ── Goal ──

type fakeGoal struct{ b *fakeBackend }

func (f *fakeGoal) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*backend.GoalInfo, error) {
	g := &backend.GoalInfo{ID: "goal-001", WorkspaceID: wsID, Text: text}
	f.b.goals = append(f.b.goals, *g)
	return g, nil
}
func (f *fakeGoal) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.GoalInfo, error) {
	return f.b.goals, nil
}
func (f *fakeGoal) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{ID: id, WorkspaceID: wsID, Text: "test"}, nil
}
func (f *fakeGoal) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*backend.GoalInfo, error) {
	return f.Get(ctx, wsID, id)
}

// ── DoD ──

type fakeDoD struct{ b *fakeBackend }

func (f *fakeDoD) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*backend.DoDInfo, error) {
	d := &backend.DoDInfo{ID: "dod-001", WorkspaceID: wsID, Items: items, State: "ready"}
	f.b.dods = append(f.b.dods, *d)
	return d, nil
}
func (f *fakeDoD) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.DoDInfo, error) {
	return f.b.dods, nil
}
func (f *fakeDoD) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*backend.DoDInfo, error) {
	return &backend.DoDInfo{ID: id, WorkspaceID: wsID}, nil
}
func (f *fakeDoD) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*backend.DoDInfo, error) {
	return f.Get(ctx, wsID, id)
}

// ── Run ──

type fakeRun struct{ b *fakeBackend }

func (f *fakeRun) Command(ctx context.Context, req backend.WorkspaceRunCommand) (*backend.RunInfo, error) {
	switch req.Intent {
	case "", backend.RunCommandIntentResume:
		if req.ExpectedRunID == "" {
			return f.Activate(ctx, req.WorkspaceID, req.Activation)
		}
		return f.Resume(ctx, req.WorkspaceID, req.ExpectedRunID)
	case backend.RunCommandIntentRestart:
		return f.Restart(ctx, req.WorkspaceID, req.Activation)
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (f *fakeRun) Activate(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	f.b.mu.Lock()
	f.b.ActivateCalls = append(f.b.ActivateCalls, req)
	f.b.mu.Unlock()
	r := &backend.RunInfo{ID: "run-001", WorkspaceID: wsID, State: "running"}
	f.b.runs = append(f.b.runs, *r)
	return r, nil
}
func (f *fakeRun) Restart(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return f.Activate(ctx, wsID, req)
}
func (f *fakeRun) Resume(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: id, WorkspaceID: wsID, State: "running"}, nil
}
func (f *fakeRun) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: id, WorkspaceID: wsID, State: "running"}, nil
}
func (f *fakeRun) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.RunInfo, error) {
	return f.b.runs, nil
}
func (f *fakeRun) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*backend.RunStatusInfo, error) {
	return &backend.RunStatusInfo{ID: id, WorkspaceID: wsID, State: "running"}, nil
}

// ── Decision ──

type fakeDec struct{ b *fakeBackend }

func (f *fakeDec) Resolve(ctx context.Context, req backend.ResolveDecisionRequest) (*backend.DecisionInfo, error) {
	f.b.mu.Lock()
	f.b.ResolveCalls = append(f.b.ResolveCalls, req)
	f.b.mu.Unlock()
	return &backend.DecisionInfo{ID: req.TicketID, State: "resolved"}, nil
}
func (f *fakeDec) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ticketIDs []common.TicketID, rt string, p workorcheventpayload.Payload) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (f *fakeDec) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.DecisionInfo, error) {
	return f.b.decisions, nil
}
func (f *fakeDec) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*backend.DecisionInfo, error) {
	return &backend.DecisionInfo{ID: id, WorkspaceID: wsID, Question: "test?", State: "open"}, nil
}

// ── Approval ──

type fakeAppr struct{ b *fakeBackend }

func (f *fakeAppr) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	f.b.mu.Lock()
	f.b.ApproveCalls = append(f.b.ApproveCalls, string(id))
	f.b.mu.Unlock()
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, State: "approved"}, nil
}
func (f *fakeAppr) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*backend.ApprovalInfo, error) {
	f.b.mu.Lock()
	f.b.RejectCalls = append(f.b.RejectCalls, string(id))
	f.b.mu.Unlock()
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, State: "rejected"}, nil
}
func (f *fakeAppr) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (f *fakeAppr) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.ApprovalInfo, error) {
	return f.b.approvals, nil
}
func (f *fakeAppr) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{ID: id, WorkspaceID: wsID, Reason: "test reason", State: "pending"}, nil
}

// ── Stubs ──

type fakeSoul struct{}

func (f *fakeSoul) Create(ctx context.Context, s *backend.SoulInfo) (*backend.SoulInfo, error) {
	return s, nil
}
func (f *fakeSoul) Get(ctx context.Context, id common.SoulID) (*backend.SoulInfo, error) {
	return &backend.SoulInfo{ID: id}, nil
}
func (f *fakeSoul) List(ctx context.Context) ([]backend.SoulInfo, error) { return nil, nil }
func (f *fakeSoul) Update(ctx context.Context, id common.SoulID, s *backend.SoulInfo) (*backend.SoulInfo, error) {
	return s, nil
}
func (f *fakeSoul) Delete(ctx context.Context, id common.SoulID) error { return nil }

type fakeSkill struct{}

func (f *fakeSkill) Create(ctx context.Context, s *backend.SkillInfo) (*backend.SkillInfo, error) {
	return s, nil
}
func (f *fakeSkill) Get(ctx context.Context, id common.SkillID) (*backend.SkillInfo, error) {
	return &backend.SkillInfo{ID: id}, nil
}
func (f *fakeSkill) List(ctx context.Context) ([]backend.SkillInfo, error) { return nil, nil }

type fakeMCP struct{}

func (f *fakeMCP) CreateServer(ctx context.Context, s *backend.MCPServerInfo) (*backend.MCPServerInfo, error) {
	return s, nil
}
func (f *fakeMCP) GetServer(ctx context.Context, id common.MCPServerID) (*backend.MCPServerInfo, error) {
	return &backend.MCPServerInfo{ID: id}, nil
}
func (f *fakeMCP) ListServers(ctx context.Context) ([]backend.MCPServerInfo, error) { return nil, nil }
func (f *fakeMCP) UpdateServer(ctx context.Context, id common.MCPServerID, s *backend.MCPServerInfo) (*backend.MCPServerInfo, error) {
	return s, nil
}
func (f *fakeMCP) ListCapabilities(ctx context.Context) ([]backend.MCPCapabilityInfo, error) {
	return nil, nil
}

type fakeMem struct{}

func (f *fakeMem) Create(ctx context.Context, wsID common.WorkspaceID, m *backend.MemoryItemInfo) (*backend.MemoryItemInfo, error) {
	return m, nil
}
func (f *fakeMem) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*backend.MemoryItemInfo, error) {
	return &backend.MemoryItemInfo{ID: id, WorkspaceID: wsID}, nil
}
func (f *fakeMem) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *backend.MemoryItemInfo) (*backend.MemoryItemInfo, error) {
	m.ID = id
	m.WorkspaceID = wsID
	return m, nil
}
func (f *fakeMem) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	return nil
}
func (f *fakeMem) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.MemoryItemInfo, error) {
	return nil, nil
}
func (f *fakeMem) Search(ctx context.Context, wsID common.WorkspaceID, req backend.MemorySearchRequest) ([]backend.MemoryItemInfo, error) {
	return nil, nil
}

type fakeTmpl struct{ b *fakeBackend }

func (f *fakeTmpl) Create(ctx context.Context, t *backend.TemplateInfo) (*backend.TemplateInfo, error) {
	f.b.templates = append(f.b.templates, *t)
	return t, nil
}
func (f *fakeTmpl) Get(ctx context.Context, id string) (*backend.TemplateInfo, error) {
	for _, item := range f.b.templates {
		if item.ID == id {
			return &item, nil
		}
	}
	return &backend.TemplateInfo{ID: id}, nil
}
func (f *fakeTmpl) List(ctx context.Context) ([]backend.TemplateInfo, error) {
	return f.b.templates, nil
}
func (f *fakeTmpl) Update(ctx context.Context, id string, t *backend.TemplateInfo) (*backend.TemplateInfo, error) {
	t.ID = id
	return t, nil
}
func (f *fakeTmpl) Delete(ctx context.Context, id string) error { return nil }

type fakeCfg struct{}

func (f *fakeCfg) Get(ctx context.Context) (*backend.ServiceConfig, error) {
	return &backend.ServiceConfig{}, nil
}

func (f *fakeCfg) ListChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	return nil, nil
}

func (f *fakeCfg) RefreshChatModelTargets(context.Context) ([]backend.ChatModelTargetOption, error) {
	return nil, nil
}

type fakeEvents struct{ b *fakeBackend }

func (f *fakeEvents) Subscribe(ctx context.Context, handler backend.EventHandler) (func(), error) {
	f.b.mu.Lock()
	f.b.eventHandlers = append(f.b.eventHandlers, handler)
	f.b.mu.Unlock()
	return func() {}, nil
}
