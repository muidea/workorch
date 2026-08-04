package backend_test

import (
	"context"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// fakeBackend implements workorchport.Backend for deterministic TUI and agent tests.
type fakeBackend struct {
	mu sync.Mutex

	workspaces []workorchport.WorkspaceInfo
	goals      []workorchport.GoalInfo
	dods       []workorchport.DoDInfo
	runs       []workorchport.RunInfo
	decisions  []workorchport.DecisionInfo
	approvals  []workorchport.ApprovalInfo
	templates  []workorchport.TemplateInfo

	// Recorded commands for assertion.
	ActivateCalls        []workorchport.ActivateRunRequest
	ResolveCalls         []workorchport.ResolveDecisionRequest
	ApproveCalls         []string
	RejectCalls          []string
	CreateWorkspaceCalls []workorchport.CreateWorkspaceRequest

	// Pre-configured responses.
	NextRunID       string
	NextGoalID      string
	NextDoDID       string
	NextWorkspaceID string

	eventHandlers []workorchport.EventHandler
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		workspaces: make([]workorchport.WorkspaceInfo, 0),
		goals:      make([]workorchport.GoalInfo, 0),
		dods:       make([]workorchport.DoDInfo, 0),
		runs:       make([]workorchport.RunInfo, 0),
		decisions:  make([]workorchport.DecisionInfo, 0),
		approvals:  make([]workorchport.ApprovalInfo, 0),
	}
}

func (b *fakeBackend) Workspace() workorchport.WorkspaceBackend { return &fakeWS{b} }
func (b *fakeBackend) Goal() workorchport.GoalBackend           { return &fakeGoal{b} }
func (b *fakeBackend) DoD() workorchport.DoDBackend             { return &fakeDoD{b} }
func (b *fakeBackend) Run() workorchport.RunBackend             { return &fakeRun{b} }
func (b *fakeBackend) Decision() workorchport.DecisionBackend   { return &fakeDec{b} }
func (b *fakeBackend) Approval() workorchport.ApprovalBackend   { return &fakeAppr{b} }
func (b *fakeBackend) Soul() workorchport.SoulBackend           { return &fakeSoul{} }
func (b *fakeBackend) Skill() workorchport.SkillBackend         { return &fakeSkill{} }
func (b *fakeBackend) MCP() workorchport.MCPBackend             { return &fakeMCP{} }
func (b *fakeBackend) Memory() workorchport.MemoryBackend       { return &fakeMem{} }
func (b *fakeBackend) Template() workorchport.TemplateBackend   { return &fakeTmpl{b} }
func (b *fakeBackend) Config() workorchport.ConfigBackend       { return &fakeCfg{} }
func (b *fakeBackend) Events() workorchport.EventsBackend       { return &fakeEvents{b} }
func (b *fakeBackend) ProjectionEvents() workorchport.WorkspaceProjectionEventsBackend {
	return nil
}

// Emit sends a backend event to all subscribers.
func (b *fakeBackend) Emit(ctx context.Context, evt workorchport.Event) {
	b.mu.Lock()
	handlers := make([]workorchport.EventHandler, len(b.eventHandlers))
	copy(handlers, b.eventHandlers)
	b.mu.Unlock()
	for _, h := range handlers {
		h(ctx, evt)
	}
}

// ── Workspace ──

type fakeWS struct{ b *fakeBackend }

func (f *fakeWS) Create(ctx context.Context, req workorchport.CreateWorkspaceRequest) (*workorchport.WorkspaceInfo, error) {
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	f.b.CreateWorkspaceCalls = append(f.b.CreateWorkspaceCalls, req)
	id := common.WorkspaceID("ws-test-001")
	if req.Title != "" {
		id = common.WorkspaceID("ws-" + req.Title)
	}
	ws := &workorchport.WorkspaceInfo{
		ID: id, Title: req.Title, Description: req.Description,
		DeliverableDirectory: req.DeliverableDirectory, State: "active",
	}
	f.b.workspaces = append(f.b.workspaces, *ws)
	return ws, nil
}
func (f *fakeWS) Get(ctx context.Context, id common.WorkspaceID) (*workorchport.WorkspaceInfo, error) {
	for _, w := range f.b.workspaces {
		if w.ID == id {
			return &w, nil
		}
	}
	return nil, common.ErrNotFound
}
func (f *fakeWS) List(ctx context.Context) ([]workorchport.WorkspaceInfo, error) {
	return f.b.workspaces, nil
}
func (f *fakeWS) Snapshot(ctx context.Context, id common.WorkspaceID) (*workorchport.WorkspaceSnapshotInfo, error) {
	workspace, err := f.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	return &workorchport.WorkspaceSnapshotInfo{
		Workspace: *workspace,
		Goals:     append([]workorchport.GoalInfo(nil), f.b.goals...),
		DoDs:      append([]workorchport.DoDInfo(nil), f.b.dods...),
		Runs:      append([]workorchport.RunInfo(nil), f.b.runs...),
		Decisions: append([]workorchport.DecisionInfo(nil), f.b.decisions...),
		Approvals: append([]workorchport.ApprovalInfo(nil), f.b.approvals...),
	}, nil
}

func (f *fakeWS) RuntimeView(context.Context, workorchport.ReadWorkspaceRuntimeViewRequest) (*workorchport.WorkspaceRuntimeViewInfo, error) {
	return nil, nil
}

func (f *fakeWS) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*workorchport.WorkspaceInfo, error) {
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

func (f *fakeGoal) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*workorchport.GoalInfo, error) {
	g := &workorchport.GoalInfo{ID: "goal-001", WorkspaceID: wsID, Text: text}
	f.b.goals = append(f.b.goals, *g)
	return g, nil
}
func (f *fakeGoal) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchport.GoalInfo, error) {
	return f.b.goals, nil
}
func (f *fakeGoal) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*workorchport.GoalInfo, error) {
	return &workorchport.GoalInfo{ID: id, WorkspaceID: wsID, Text: "test"}, nil
}
func (f *fakeGoal) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*workorchport.GoalInfo, error) {
	return f.Get(ctx, wsID, id)
}

// ── DoD ──

type fakeDoD struct{ b *fakeBackend }

func (f *fakeDoD) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*workorchport.DoDInfo, error) {
	d := &workorchport.DoDInfo{ID: "dod-001", WorkspaceID: wsID, Items: items, State: "ready"}
	f.b.dods = append(f.b.dods, *d)
	return d, nil
}
func (f *fakeDoD) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchport.DoDInfo, error) {
	return f.b.dods, nil
}
func (f *fakeDoD) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*workorchport.DoDInfo, error) {
	return &workorchport.DoDInfo{ID: id, WorkspaceID: wsID}, nil
}
func (f *fakeDoD) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*workorchport.DoDInfo, error) {
	return f.Get(ctx, wsID, id)
}

// ── Run ──

type fakeRun struct{ b *fakeBackend }

func (f *fakeRun) Command(ctx context.Context, req workorchport.WorkspaceRunCommand) (*workorchport.RunInfo, error) {
	switch req.Intent {
	case "", workorchport.RunCommandIntentResume:
		return f.Activate(ctx, req.WorkspaceID, workorchport.ActivateRunRequest{})
	case workorchport.RunCommandIntentRestart:
		return f.Restart(ctx, req.WorkspaceID, workorchport.ActivateRunRequest{})
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (f *fakeRun) Activate(ctx context.Context, wsID common.WorkspaceID, req workorchport.ActivateRunRequest) (*workorchport.RunInfo, error) {
	f.b.mu.Lock()
	f.b.ActivateCalls = append(f.b.ActivateCalls, req)
	f.b.mu.Unlock()
	r := &workorchport.RunInfo{ID: "run-001", WorkspaceID: wsID, State: "running"}
	f.b.runs = append(f.b.runs, *r)
	return r, nil
}
func (f *fakeRun) Restart(ctx context.Context, wsID common.WorkspaceID, req workorchport.ActivateRunRequest) (*workorchport.RunInfo, error) {
	return f.Activate(ctx, wsID, req)
}
func (f *fakeRun) Resume(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*workorchport.RunInfo, error) {
	return &workorchport.RunInfo{ID: id, WorkspaceID: wsID, State: "running"}, nil
}
func (f *fakeRun) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*workorchport.RunInfo, error) {
	return &workorchport.RunInfo{ID: id, WorkspaceID: wsID, State: "running"}, nil
}
func (f *fakeRun) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchport.RunInfo, error) {
	return f.b.runs, nil
}
func (f *fakeRun) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*workorchport.RunStatusInfo, error) {
	return &workorchport.RunStatusInfo{ID: id, WorkspaceID: wsID, State: "running"}, nil
}

// ── Decision ──

type fakeDec struct{ b *fakeBackend }

func (f *fakeDec) Resolve(ctx context.Context, req workorchport.ResolveDecisionRequest) (*workorchport.DecisionInfo, error) {
	f.b.mu.Lock()
	f.b.ResolveCalls = append(f.b.ResolveCalls, req)
	f.b.mu.Unlock()
	return &workorchport.DecisionInfo{ID: req.TicketID, State: "resolved"}, nil
}
func (f *fakeDec) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ticketIDs []common.TicketID, rt string, p workorcheventpayload.Payload) ([]workorchport.DecisionInfo, error) {
	return nil, nil
}
func (f *fakeDec) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchport.DecisionInfo, error) {
	return f.b.decisions, nil
}
func (f *fakeDec) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*workorchport.DecisionInfo, error) {
	return &workorchport.DecisionInfo{ID: id, WorkspaceID: wsID, Question: "test?", State: "open"}, nil
}

// ── Approval ──

type fakeAppr struct{ b *fakeBackend }

func (f *fakeAppr) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*workorchport.ApprovalInfo, error) {
	f.b.mu.Lock()
	f.b.ApproveCalls = append(f.b.ApproveCalls, string(id))
	f.b.mu.Unlock()
	return &workorchport.ApprovalInfo{ID: id, WorkspaceID: wsID, State: "approved"}, nil
}
func (f *fakeAppr) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*workorchport.ApprovalInfo, error) {
	f.b.mu.Lock()
	f.b.RejectCalls = append(f.b.RejectCalls, string(id))
	f.b.mu.Unlock()
	return &workorchport.ApprovalInfo{ID: id, WorkspaceID: wsID, State: "rejected"}, nil
}
func (f *fakeAppr) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]workorchport.ApprovalInfo, error) {
	return nil, nil
}
func (f *fakeAppr) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchport.ApprovalInfo, error) {
	return f.b.approvals, nil
}
func (f *fakeAppr) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*workorchport.ApprovalInfo, error) {
	return &workorchport.ApprovalInfo{ID: id, WorkspaceID: wsID, Reason: "test reason", State: "pending"}, nil
}

// ── Stubs ──

type fakeSoul struct{}

func (f *fakeSoul) Create(ctx context.Context, s *workorchport.SoulInfo) (*workorchport.SoulInfo, error) {
	return s, nil
}
func (f *fakeSoul) Get(ctx context.Context, id common.SoulID) (*workorchport.SoulInfo, error) {
	return &workorchport.SoulInfo{ID: id}, nil
}
func (f *fakeSoul) List(ctx context.Context) ([]workorchport.SoulInfo, error) { return nil, nil }
func (f *fakeSoul) Update(ctx context.Context, id common.SoulID, s *workorchport.SoulInfo) (*workorchport.SoulInfo, error) {
	return s, nil
}
func (f *fakeSoul) Delete(ctx context.Context, id common.SoulID) error { return nil }

type fakeSkill struct{}

func (f *fakeSkill) Create(ctx context.Context, s *workorchport.SkillInfo) (*workorchport.SkillInfo, error) {
	return s, nil
}
func (f *fakeSkill) Get(ctx context.Context, id common.SkillID) (*workorchport.SkillInfo, error) {
	return &workorchport.SkillInfo{ID: id}, nil
}
func (f *fakeSkill) List(ctx context.Context) ([]workorchport.SkillInfo, error) { return nil, nil }

type fakeMCP struct{}

func (f *fakeMCP) CreateServer(ctx context.Context, s *workorchport.MCPServerInfo) (*workorchport.MCPServerInfo, error) {
	return s, nil
}
func (f *fakeMCP) GetServer(ctx context.Context, id common.MCPServerID) (*workorchport.MCPServerInfo, error) {
	return &workorchport.MCPServerInfo{ID: id}, nil
}
func (f *fakeMCP) ListServers(ctx context.Context) ([]workorchport.MCPServerInfo, error) {
	return nil, nil
}
func (f *fakeMCP) UpdateServer(ctx context.Context, id common.MCPServerID, s *workorchport.MCPServerInfo) (*workorchport.MCPServerInfo, error) {
	return s, nil
}
func (f *fakeMCP) ListCapabilities(ctx context.Context) ([]workorchport.MCPCapabilityInfo, error) {
	return nil, nil
}

type fakeMem struct{}

func (f *fakeMem) Create(ctx context.Context, wsID common.WorkspaceID, m *workorchport.MemoryItemInfo) (*workorchport.MemoryItemInfo, error) {
	return m, nil
}
func (f *fakeMem) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*workorchport.MemoryItemInfo, error) {
	return &workorchport.MemoryItemInfo{ID: id, WorkspaceID: wsID}, nil
}
func (f *fakeMem) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *workorchport.MemoryItemInfo) (*workorchport.MemoryItemInfo, error) {
	m.ID = id
	m.WorkspaceID = wsID
	return m, nil
}
func (f *fakeMem) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	return nil
}
func (f *fakeMem) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchport.MemoryItemInfo, error) {
	return nil, nil
}
func (f *fakeMem) Search(ctx context.Context, wsID common.WorkspaceID, req workorchport.MemorySearchRequest) ([]workorchport.MemoryItemInfo, error) {
	return nil, nil
}

type fakeTmpl struct{ b *fakeBackend }

func (f *fakeTmpl) Create(ctx context.Context, t *workorchport.TemplateInfo) (*workorchport.TemplateInfo, error) {
	f.b.templates = append(f.b.templates, *t)
	return t, nil
}
func (f *fakeTmpl) Get(ctx context.Context, id string) (*workorchport.TemplateInfo, error) {
	for _, item := range f.b.templates {
		if item.ID == id {
			return &item, nil
		}
	}
	return &workorchport.TemplateInfo{ID: id}, nil
}
func (f *fakeTmpl) List(ctx context.Context) ([]workorchport.TemplateInfo, error) {
	return f.b.templates, nil
}
func (f *fakeTmpl) Update(ctx context.Context, id string, t *workorchport.TemplateInfo) (*workorchport.TemplateInfo, error) {
	t.ID = id
	return t, nil
}
func (f *fakeTmpl) Delete(ctx context.Context, id string) error { return nil }

type fakeCfg struct{}

func (f *fakeCfg) Get(ctx context.Context) (*workorchport.ServiceConfig, error) {
	return &workorchport.ServiceConfig{}, nil
}

func (f *fakeCfg) ListChatModelTargets(context.Context) ([]workorchport.ChatModelTargetOption, error) {
	return nil, nil
}

func (f *fakeCfg) RefreshChatModelTargets(context.Context) ([]workorchport.ChatModelTargetOption, error) {
	return nil, nil
}

type fakeEvents struct{ b *fakeBackend }

func (f *fakeEvents) Subscribe(ctx context.Context, handler workorchport.EventHandler) (func(), error) {
	f.b.mu.Lock()
	f.b.eventHandlers = append(f.b.eventHandlers, handler)
	f.b.mu.Unlock()
	return func() {}, nil
}
