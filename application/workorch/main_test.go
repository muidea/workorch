package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	cliconfig "github.com/muidea/workorch/internal/services/workorch/biz/config"
	"github.com/muidea/workorch/internal/services/workorch/biz/session"
)

func TestResolveDefaultWorkspacePrefersCurrentDirectoryBinding(t *testing.T) {
	cfgPath := writeCLIConfig(t, "ws-last")
	be := &startupFakeBackend{workspaces: []backend.WorkspaceInfo{
		{ID: "ws-current", Title: "current", DeliverableDirectory: "/project"},
		{ID: "ws-last", Title: "last", DeliverableDirectory: "/other"},
	}}

	got := session.ResolveDefaultWorkspace(context.Background(), be, "/project", cfgPath)

	if got.ID != "ws-current" {
		t.Fatalf("workspace = %s, want ws-current", got.ID)
	}
	if be.created != 0 {
		t.Fatalf("created = %d, want 0", be.created)
	}
}

func TestResolveDefaultWorkspaceUsesLastActiveWhenCurrentDirectoryUnbound(t *testing.T) {
	cfgPath := writeCLIConfig(t, "ws-last")
	be := &startupFakeBackend{workspaces: []backend.WorkspaceInfo{
		{ID: "ws-last", Title: "last", DeliverableDirectory: "/other"},
	}}

	got := session.ResolveDefaultWorkspace(context.Background(), be, "/project", cfgPath)

	if got.ID != "ws-last" {
		t.Fatalf("workspace = %s, want ws-last", got.ID)
	}
	if be.created != 0 {
		t.Fatalf("created = %d, want 0", be.created)
	}
}

func TestResolveDefaultWorkspaceDoesNotCreateWhenNoReusableWorkspace(t *testing.T) {
	cfgPath := writeCLIConfig(t, "")
	workingDir := t.TempDir()
	be := &startupFakeBackend{}

	got := session.ResolveDefaultWorkspace(context.Background(), be, workingDir, cfgPath)

	if got.ID != "" {
		t.Fatalf("workspace = %s, want empty", got.ID)
	}
	if be.created != 0 {
		t.Fatalf("created = %d, want 0", be.created)
	}
}

func TestResolveDefaultWorkspaceDoesNotCreateForCodeDirectory(t *testing.T) {
	cfgPath := writeCLIConfig(t, "")
	workingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	be := &startupFakeBackend{}

	got := session.ResolveDefaultWorkspace(context.Background(), be, workingDir, cfgPath)

	if got.ID != "" {
		t.Fatalf("workspace = %s, want empty", got.ID)
	}
	if be.created != 0 {
		t.Fatalf("created = %d, want 0", be.created)
	}
}

func writeCLIConfig(t *testing.T, lastActive string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := cliconfig.Default()
	cfg.Defaults.LastActiveWorkspaceID = lastActive
	if err := cliconfig.Save(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return path
}

type startupFakeBackend struct {
	workspaces []backend.WorkspaceInfo
	created    int
	lastCreate backend.CreateWorkspaceRequest
}

func (b *startupFakeBackend) Workspace() backend.WorkspaceBackend {
	return &startupFakeWorkspace{parent: b}
}
func (b *startupFakeBackend) Goal() backend.GoalBackend                                  { return nil }
func (b *startupFakeBackend) DoD() backend.DoDBackend                                    { return nil }
func (b *startupFakeBackend) Run() backend.RunBackend                                    { return nil }
func (b *startupFakeBackend) Decision() backend.DecisionBackend                          { return nil }
func (b *startupFakeBackend) Approval() backend.ApprovalBackend                          { return nil }
func (b *startupFakeBackend) Soul() backend.SoulBackend                                  { return nil }
func (b *startupFakeBackend) Skill() backend.SkillBackend                                { return nil }
func (b *startupFakeBackend) MCP() backend.MCPBackend                                    { return nil }
func (b *startupFakeBackend) Memory() backend.MemoryBackend                              { return nil }
func (b *startupFakeBackend) Template() backend.TemplateBackend                          { return nil }
func (b *startupFakeBackend) Config() backend.ConfigBackend                              { return nil }
func (b *startupFakeBackend) Events() backend.EventsBackend                              { return nil }
func (b *startupFakeBackend) ProjectionEvents() backend.WorkspaceProjectionEventsBackend { return nil }

type startupFakeWorkspace struct{ parent *startupFakeBackend }

func (b *startupFakeWorkspace) Create(ctx context.Context, req backend.CreateWorkspaceRequest) (*backend.WorkspaceInfo, error) {
	b.parent.created++
	b.parent.lastCreate = req
	ws := backend.WorkspaceInfo{ID: "ws-created", Title: req.Title, DeliverableDirectory: req.DeliverableDirectory}
	b.parent.workspaces = append(b.parent.workspaces, ws)
	return &ws, nil
}
func (b *startupFakeWorkspace) Get(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceInfo, error) {
	for _, ws := range b.parent.workspaces {
		if ws.ID == id {
			return &ws, nil
		}
	}
	return nil, common.ErrNotFound
}
func (b *startupFakeWorkspace) List(ctx context.Context) ([]backend.WorkspaceInfo, error) {
	return b.parent.workspaces, nil
}
func (b *startupFakeWorkspace) Snapshot(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error) {
	workspace, err := b.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &backend.WorkspaceSnapshotInfo{Workspace: *workspace}, nil
}

func (b *startupFakeWorkspace) RuntimeView(context.Context, backend.ReadWorkspaceRuntimeViewRequest) (*backend.WorkspaceRuntimeViewInfo, error) {
	return nil, nil
}

func (b *startupFakeWorkspace) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return nil
}
