package backend_test

import (
	"context"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// fakeBackend implements workorchport.Backend for deterministic TUI and agent tests.
type fakeBackend struct {
	mu sync.Mutex

	workspaces []projectioncontract.Workspace
	goals      []workorchmanagementcontract.Goal
	dods       []workorchmanagementcontract.DoD
	runs       []projectioncontract.Run
	decisions  []projectioncontract.Decision
	approvals  []projectioncontract.Approval
	templates  []workorchmanagementcontract.WorkspaceTemplate

	// Recorded commands for assertion.
	ResolveCalls         []workorchport.ResolveDecisionRequest
	ApproveCalls         []string
	RejectCalls          []string
	CreateWorkspaceCalls []workorchmanagementcontract.WorkspaceCreateRequest

	// Pre-configured responses.
	NextRunID       string
	NextGoalID      string
	NextDoDID       string
	NextWorkspaceID string

	eventHandlers []workorchport.EventHandler
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		workspaces: make([]projectioncontract.Workspace, 0),
		goals:      make([]workorchmanagementcontract.Goal, 0),
		dods:       make([]workorchmanagementcontract.DoD, 0),
		runs:       make([]projectioncontract.Run, 0),
		decisions:  make([]projectioncontract.Decision, 0),
		approvals:  make([]projectioncontract.Approval, 0),
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

func (f *fakeWS) Create(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error) {
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	f.b.CreateWorkspaceCalls = append(f.b.CreateWorkspaceCalls, req)
	id := common.WorkspaceID("ws-test-001")
	if req.Title != "" {
		id = common.WorkspaceID("ws-" + req.Title)
	}
	ws := &projectioncontract.Workspace{
		ID: string(id), Title: req.Title, Description: req.Description,
		DeliverableDirectory: req.DeliverableDirectory, State: "active",
	}
	f.b.workspaces = append(f.b.workspaces, *ws)
	return ws, nil
}
func (f *fakeWS) Get(ctx context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error) {
	for _, w := range f.b.workspaces {
		if w.ID == string(id) {
			return &w, nil
		}
	}
	return nil, common.ErrNotFound
}
func (f *fakeWS) List(ctx context.Context) ([]projectioncontract.Workspace, error) {
	return f.b.workspaces, nil
}
func (f *fakeWS) Snapshot(ctx context.Context, id common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error) {
	workspace, err := f.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	return &projectioncontract.WorkspaceSnapshot{
		Workspace: *workspace,
		Goals:     append([]workorchmanagementcontract.Goal(nil), f.b.goals...),
		DoDs:      append([]workorchmanagementcontract.DoD(nil), f.b.dods...),
		Runs:      append([]projectioncontract.Run(nil), f.b.runs...),
		Decisions: projectioncontract.CloneDecisions(f.b.decisions),
		Approvals: projectioncontract.CloneApprovals(f.b.approvals),
	}, nil
}

func (f *fakeWS) RuntimeView(context.Context, workorchquerycontract.WorkspaceRuntimeViewQuery) (*projectioncontract.WorkspaceRuntimeView, error) {
	return nil, nil
}

func (f *fakeWS) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*projectioncontract.Workspace, error) {
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

func (f *fakeGoal) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*workorchmanagementcontract.Goal, error) {
	g := &workorchmanagementcontract.Goal{ID: "goal-001", WorkspaceID: string(wsID), Text: text}
	f.b.goals = append(f.b.goals, *g)
	return g, nil
}
func (f *fakeGoal) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.Goal, error) {
	return f.b.goals, nil
}
func (f *fakeGoal) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*workorchmanagementcontract.Goal, error) {
	return &workorchmanagementcontract.Goal{ID: string(id), WorkspaceID: string(wsID), Text: "test"}, nil
}
func (f *fakeGoal) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*workorchmanagementcontract.Goal, error) {
	return f.Get(ctx, wsID, id)
}

// ── DoD ──

type fakeDoD struct{ b *fakeBackend }

func (f *fakeDoD) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*workorchmanagementcontract.DoD, error) {
	d := &workorchmanagementcontract.DoD{ID: "dod-001", WorkspaceID: string(wsID), Items: items, State: "ready"}
	f.b.dods = append(f.b.dods, *d)
	return d, nil
}
func (f *fakeDoD) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.DoD, error) {
	return f.b.dods, nil
}
func (f *fakeDoD) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*workorchmanagementcontract.DoD, error) {
	return &workorchmanagementcontract.DoD{ID: string(id), WorkspaceID: string(wsID)}, nil
}
func (f *fakeDoD) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*workorchmanagementcontract.DoD, error) {
	return f.Get(ctx, wsID, id)
}

// ── Run ──

type fakeRun struct{ b *fakeBackend }

func (f *fakeRun) Command(ctx context.Context, req workorchport.WorkspaceRunCommand) (*workorchport.RunCommandResult, error) {
	f.b.mu.Lock()
	r := &projectioncontract.Run{ID: "run-001", WorkspaceID: string(req.WorkspaceID), State: "running"}
	f.b.runs = append(f.b.runs, *r)
	f.b.mu.Unlock()
	return &workorchport.RunCommandResult{WorkspaceID: req.WorkspaceID, RunID: common.RunID(r.ID), RequestID: "request-fake", Kind: runtimecontract.RuntimeExecutionIntentKind(req.Intent), Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted}, nil
}
func (f *fakeRun) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.Run, error) {
	return &projectioncontract.Run{ID: string(id), WorkspaceID: string(wsID), State: "running"}, nil
}
func (f *fakeRun) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Run, error) {
	return f.b.runs, nil
}
func (f *fakeRun) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.RunStatus, error) {
	return &projectioncontract.RunStatus{ID: string(id), WorkspaceID: string(wsID), State: "running"}, nil
}

// ── Decision ──

type fakeDec struct{ b *fakeBackend }

func (f *fakeDec) Resolve(ctx context.Context, req workorchport.ResolveDecisionRequest) (*projectioncontract.Decision, error) {
	f.b.mu.Lock()
	f.b.ResolveCalls = append(f.b.ResolveCalls, req)
	f.b.mu.Unlock()
	return &projectioncontract.Decision{ID: string(req.TicketID), State: "resolved"}, nil
}
func (f *fakeDec) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ticketIDs []common.TicketID, rt string, p workorcheventpayload.Payload) ([]projectioncontract.Decision, error) {
	return nil, nil
}
func (f *fakeDec) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Decision, error) {
	return f.b.decisions, nil
}
func (f *fakeDec) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*projectioncontract.Decision, error) {
	return &projectioncontract.Decision{ID: string(id), WorkspaceID: string(wsID), Question: "test?", State: "open"}, nil
}

// ── Approval ──

type fakeAppr struct{ b *fakeBackend }

func (f *fakeAppr) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	f.b.mu.Lock()
	f.b.ApproveCalls = append(f.b.ApproveCalls, string(id))
	f.b.mu.Unlock()
	return &projectioncontract.Approval{ID: string(id), WorkspaceID: string(wsID), State: "approved"}, nil
}
func (f *fakeAppr) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*projectioncontract.Approval, error) {
	f.b.mu.Lock()
	f.b.RejectCalls = append(f.b.RejectCalls, string(id))
	f.b.mu.Unlock()
	return &projectioncontract.Approval{ID: string(id), WorkspaceID: string(wsID), State: "rejected"}, nil
}
func (f *fakeAppr) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]projectioncontract.Approval, error) {
	return nil, nil
}
func (f *fakeAppr) List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Approval, error) {
	return f.b.approvals, nil
}
func (f *fakeAppr) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*projectioncontract.Approval, error) {
	return &projectioncontract.Approval{ID: string(id), WorkspaceID: string(wsID), Reason: "test reason", State: "pending"}, nil
}

// ── Stubs ──

type fakeSoul struct{}

func (f *fakeSoul) Create(ctx context.Context, s *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error) {
	return &workorchmanagementcontract.Soul{ID: s.ID, Name: s.Name}, nil
}
func (f *fakeSoul) Get(ctx context.Context, id common.SoulID) (*workorchmanagementcontract.Soul, error) {
	return &workorchmanagementcontract.Soul{ID: string(id)}, nil
}
func (f *fakeSoul) List(ctx context.Context) ([]workorchmanagementcontract.Soul, error) {
	return nil, nil
}
func (f *fakeSoul) Update(ctx context.Context, id common.SoulID, s *workorchmanagementcontract.SoulMutation) (*workorchmanagementcontract.Soul, error) {
	return &workorchmanagementcontract.Soul{ID: string(id), Name: s.Name}, nil
}
func (f *fakeSoul) Delete(ctx context.Context, id common.SoulID) error { return nil }

type fakeSkill struct{}

func (f *fakeSkill) Create(ctx context.Context, s *workorchmanagementcontract.SkillMutation) (*workorchmanagementcontract.Skill, error) {
	return &workorchmanagementcontract.Skill{ID: s.ID, Name: s.Name}, nil
}
func (f *fakeSkill) Get(ctx context.Context, id common.SkillID) (*workorchmanagementcontract.Skill, error) {
	return &workorchmanagementcontract.Skill{ID: string(id)}, nil
}
func (f *fakeSkill) List(ctx context.Context) ([]workorchmanagementcontract.Skill, error) {
	return nil, nil
}

type fakeMCP struct{}

func (f *fakeMCP) CreateServer(ctx context.Context, s *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error) {
	return &workorchmanagementcontract.MCPServer{ID: s.ID, Name: s.Name}, nil
}
func (f *fakeMCP) GetServer(ctx context.Context, id common.MCPServerID) (*workorchmanagementcontract.MCPServer, error) {
	return &workorchmanagementcontract.MCPServer{ID: string(id)}, nil
}
func (f *fakeMCP) ListServers(ctx context.Context) ([]workorchmanagementcontract.MCPServer, error) {
	return nil, nil
}
func (f *fakeMCP) UpdateServer(ctx context.Context, id common.MCPServerID, s *workorchmanagementcontract.MCPServerMutation) (*workorchmanagementcontract.MCPServer, error) {
	return &workorchmanagementcontract.MCPServer{ID: string(id), Name: s.Name}, nil
}
func (f *fakeMCP) ListCapabilities(ctx context.Context) ([]workorchmanagementcontract.MCPCapability, error) {
	return nil, nil
}

type fakeMem struct{}

func (f *fakeMem) Create(ctx context.Context, wsID common.WorkspaceID, m *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	return &workorchmanagementcontract.MemoryItem{ID: m.MemoryID, WorkspaceID: string(wsID), Summary: m.Summary}, nil
}
func (f *fakeMem) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*workorchmanagementcontract.MemoryItem, error) {
	return &workorchmanagementcontract.MemoryItem{ID: string(id), WorkspaceID: string(wsID)}, nil
}
func (f *fakeMem) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *workorchmanagementcontract.MemoryMutation) (*workorchmanagementcontract.MemoryItem, error) {
	return &workorchmanagementcontract.MemoryItem{ID: string(id), WorkspaceID: string(wsID), Summary: m.Summary}, nil
}
func (f *fakeMem) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	return nil
}
func (f *fakeMem) List(ctx context.Context, wsID common.WorkspaceID) ([]workorchmanagementcontract.MemoryItem, error) {
	return nil, nil
}
func (f *fakeMem) Search(ctx context.Context, req workorchquerycontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error) {
	return nil, nil
}

type fakeTmpl struct{ b *fakeBackend }

func (f *fakeTmpl) Create(ctx context.Context, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	f.b.templates = append(f.b.templates, *t)
	return t, nil
}
func (f *fakeTmpl) Get(ctx context.Context, id string) (*workorchmanagementcontract.WorkspaceTemplate, error) {
	for _, item := range f.b.templates {
		if item.ID == id {
			return &item, nil
		}
	}
	return &workorchmanagementcontract.WorkspaceTemplate{ID: id}, nil
}
func (f *fakeTmpl) List(ctx context.Context) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	return f.b.templates, nil
}
func (f *fakeTmpl) Update(ctx context.Context, id string, t *workorchmanagementcontract.WorkspaceTemplate) (*workorchmanagementcontract.WorkspaceTemplate, error) {
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
