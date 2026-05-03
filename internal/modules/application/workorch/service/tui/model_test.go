package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/common"
)

func asModel(m tea.Model) AppModel { switch v := m.(type) { case AppModel: return v; case *AppModel: return *v }; panic("not AppModel") }

func TestModelInit(t *testing.T) {
	m := NewAppModel(newFake(t))
	if m.mode != modeStatusDashboard {
		t.Fatal("expected dashboard mode")
	}
}

func TestModelQuit(t *testing.T) {
	m := NewAppModel(newFake(t))
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if asModel(next).mode != modeQuit {
		t.Fatal("expected quit mode")
	}
}

func TestModelWorkspaceList(t *testing.T) {
	m := NewAppModel(newFake(t))
	msg := WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-a", Title: "A", State: "active"},
		{ID: "ws-b", Title: "B", State: "running"},
	}}
	next, _ := m.Update(msg)
	if len(asModel(next).workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(asModel(next).workspaces))
	}
}

func TestModelError(t *testing.T) {
	m := NewAppModel(newFake(t))
	next, _ := m.Update(ErrorMsg{Error: context.DeadlineExceeded})
	if asModel(next).lastError == nil {
		t.Fatal("expected error")
	}
}

func TestModelView(t *testing.T) {
	m := NewAppModel(newFake(t))
	next, _ := m.Update(WorkspaceListMsg{Workspaces: []backend.WorkspaceInfo{
		{ID: "ws-1", Title: "Project", State: "active"},
	}})
	m = asModel(next)
	m.width = 80
	m.height = 24
	if m.View() == "" {
		t.Fatal("expected non-empty view")
	}
}

// ── fake backend ──

type fakeBE struct{}

func newFake(t *testing.T) *fakeBE { t.Helper(); return &fakeBE{} }
func (b *fakeBE) Workspace() backend.WorkspaceBackend { return &fakeWS{} }
func (b *fakeBE) Goal() backend.GoalBackend            { return &fakeG{} }
func (b *fakeBE) DoD() backend.DoDBackend              { return &fakeD{} }
func (b *fakeBE) Run() backend.RunBackend              { return &fakeR{} }
func (b *fakeBE) Decision() backend.DecisionBackend     { return &fakeDC{} }
func (b *fakeBE) Approval() backend.ApprovalBackend     { return &fakeA{} }
func (b *fakeBE) Soul() backend.SoulBackend             { return &fakeS{} }
func (b *fakeBE) Skill() backend.SkillBackend           { return &fakeSK{} }
func (b *fakeBE) MCP() backend.MCPBackend               { return &fakeM{} }
func (b *fakeBE) Memory() backend.MemoryBackend         { return &fakeMM{} }
func (b *fakeBE) Template() backend.TemplateBackend     { return &fakeT{} }
func (b *fakeBE) Config() backend.ConfigBackend         { return &fakeC{} }
func (b *fakeBE) Events() backend.EventsBackend         { return &fakeE{} }

type fakeWS struct{}
func (f *fakeWS) Create(ctx context.Context, req backend.CreateWorkspaceRequest) (*backend.WorkspaceInfo, error) {
	return &backend.WorkspaceInfo{ID: "ws-new", Title: req.Title, State: "active"}, nil
}
func (f *fakeWS) Get(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	return &backend.WorkspaceInfo{ID: id}, nil
}
func (f *fakeWS) List(ctx context.Context) ([]backend.WorkspaceInfo, error) { return nil, nil }
func (f *fakeWS) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error { return nil }

type fakeG struct{}
func (f *fakeG) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{ID: "g-1", Text: text}, nil
}
func (f *fakeG) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.GoalInfo, error) { return nil, nil }
func (f *fakeG) Get(ctx context.Context, id common.GoalID) (*backend.GoalInfo, error) {
	return &backend.GoalInfo{ID: id}, nil
}
func (f *fakeG) Update(ctx context.Context, id common.GoalID, text string) (*backend.GoalInfo, error) {
	return f.Get(ctx, id)
}

type fakeD struct{}
func (f *fakeD) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*backend.DoDInfo, error) {
	return &backend.DoDInfo{ID: "d-1", Items: items}, nil
}
func (f *fakeD) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.DoDInfo, error) { return nil, nil }
func (f *fakeD) Get(ctx context.Context, id common.DoDID) (*backend.DoDInfo, error) { return &backend.DoDInfo{ID: id}, nil }
func (f *fakeD) Update(ctx context.Context, id common.DoDID, items []string) (*backend.DoDInfo, error) {
	return f.Get(ctx, id)
}

type fakeR struct{}
func (f *fakeR) Activate(ctx context.Context, wsID common.WorkspaceID, req backend.ActivateRunRequest) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: "run-1", State: "running"}, nil
}
func (f *fakeR) Resume(ctx context.Context, id common.RunID) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: id}, nil
}
func (f *fakeR) Get(ctx context.Context, id common.RunID) (*backend.RunInfo, error) {
	return &backend.RunInfo{ID: id}, nil
}
func (f *fakeR) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.RunInfo, error) { return nil, nil }
func (f *fakeR) Status(ctx context.Context, id common.RunID) (*backend.RunStatusInfo, error) {
	return &backend.RunStatusInfo{ID: id, State: "running"}, nil
}

type fakeDC struct{}
func (f *fakeDC) Resolve(ctx context.Context, req backend.ResolveDecisionRequest) (*backend.DecisionInfo, error) {
	return &backend.DecisionInfo{ID: req.TicketID, State: "resolved"}, nil
}
func (f *fakeDC) BatchResolve(ctx context.Context, ids []common.TicketID, rt string, p map[string]any) ([]backend.DecisionInfo, error) {
	return nil, nil
}
func (f *fakeDC) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.DecisionInfo, error) { return nil, nil }
func (f *fakeDC) Get(ctx context.Context, id common.TicketID) (*backend.DecisionInfo, error) {
	return &backend.DecisionInfo{ID: id, Question: "?", State: "open"}, nil
}

type fakeA struct{}
func (f *fakeA) Approve(ctx context.Context, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{ID: id, State: "approved"}, nil
}
func (f *fakeA) Reject(ctx context.Context, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{ID: id, State: "rejected"}, nil
}
func (f *fakeA) BatchResolve(ctx context.Context, ids []common.ApprovalID, action string) ([]backend.ApprovalInfo, error) {
	return nil, nil
}
func (f *fakeA) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.ApprovalInfo, error) { return nil, nil }
func (f *fakeA) Get(ctx context.Context, id common.ApprovalID) (*backend.ApprovalInfo, error) {
	return &backend.ApprovalInfo{ID: id, Reason: "reason", State: "pending"}, nil
}

type fakeS struct{}
func (f *fakeS) Create(ctx context.Context, s *backend.SoulInfo) (*backend.SoulInfo, error) { return s, nil }
func (f *fakeS) Get(ctx context.Context, id common.SoulID) (*backend.SoulInfo, error) {
	return &backend.SoulInfo{ID: id}, nil
}
func (f *fakeS) List(ctx context.Context) ([]backend.SoulInfo, error) { return nil, nil }
func (f *fakeS) Update(ctx context.Context, id common.SoulID, s *backend.SoulInfo) (*backend.SoulInfo, error) { return s, nil }
func (f *fakeS) Delete(ctx context.Context, id common.SoulID) error { return nil }

type fakeSK struct{}
func (f *fakeSK) Create(ctx context.Context, s *backend.SkillInfo) (*backend.SkillInfo, error) { return s, nil }
func (f *fakeSK) Get(ctx context.Context, id common.SkillID) (*backend.SkillInfo, error) {
	return &backend.SkillInfo{ID: id}, nil
}
func (f *fakeSK) List(ctx context.Context) ([]backend.SkillInfo, error) { return nil, nil }

type fakeM struct{}
func (f *fakeM) CreateServer(ctx context.Context, s *backend.MCPServerInfo) (*backend.MCPServerInfo, error) { return s, nil }
func (f *fakeM) GetServer(ctx context.Context, id common.MCPServerID) (*backend.MCPServerInfo, error) {
	return &backend.MCPServerInfo{ID: id}, nil
}
func (f *fakeM) ListServers(ctx context.Context) ([]backend.MCPServerInfo, error) { return nil, nil }
func (f *fakeM) UpdateServer(ctx context.Context, id common.MCPServerID, s *backend.MCPServerInfo) (*backend.MCPServerInfo, error) {
	return s, nil
}
func (f *fakeM) ListCapabilities(ctx context.Context) ([]backend.MCPCapabilityInfo, error) { return nil, nil }

type fakeMM struct{}
func (f *fakeMM) Create(ctx context.Context, wsID common.WorkspaceID, m *backend.MemoryItemInfo) (*backend.MemoryItemInfo, error) {
	return m, nil
}
func (f *fakeMM) List(ctx context.Context, wsID common.WorkspaceID) ([]backend.MemoryItemInfo, error) { return nil, nil }

type fakeT struct{}
func (f *fakeT) Create(ctx context.Context, t *backend.TemplateInfo) (*backend.TemplateInfo, error) { return t, nil }
func (f *fakeT) Get(ctx context.Context, id string) (*backend.TemplateInfo, error) {
	return &backend.TemplateInfo{ID: id}, nil
}
func (f *fakeT) List(ctx context.Context) ([]backend.TemplateInfo, error) { return nil, nil }
func (f *fakeT) Update(ctx context.Context, id string, t *backend.TemplateInfo) (*backend.TemplateInfo, error) { return t, nil }
func (f *fakeT) Delete(ctx context.Context, id string) error { return nil }

type fakeC struct{}
func (f *fakeC) Get(ctx context.Context) (*backend.ServiceConfig, error) { return &backend.ServiceConfig{}, nil }

type fakeE struct{}
func (f *fakeE) Subscribe(ctx context.Context, handler backend.EventHandler) (func(), error) {
	return func() {}, nil
}
