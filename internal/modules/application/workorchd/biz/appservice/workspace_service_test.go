package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCreateWorkspaceWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := CreateWorkspaceWithSession(context.Background(), nil, "workspace 1", "", "workspaces/workspace 1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestCreateWorkspaceWithBackendReturnsCreatedWorkspace(t *testing.T) {
	item, err := CreateWorkspaceWithBackend(context.Background(), "file", t.TempDir(), "appservice created workspace", "appservice description", "workspaces/appservice created workspace")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if item.ID == "" || item.Title != "appservice created workspace" || item.Description != "appservice description" || item.WorkingDirectory != "workspaces/appservice created workspace" {
		t.Fatalf("workspace = %+v, want created workspace", item)
	}
	if !strings.HasPrefix(item.ID, "ws-") {
		t.Fatalf("workspace id = %q, want generated ws- id", item.ID)
	}
	if item.State != "active" {
		t.Fatalf("state = %q, want active", item.State)
	}
}

func TestCreateWorkspaceWithTemplateBindsWorkspaceSouls(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	for _, id := range []string{"soul-template-default", "soul-template-planning", "soul-template-execution"} {
		if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
			ID:     id,
			Name:   id,
			SoulMD: "test soul",
		}); err != nil {
			t.Fatalf("CreateSoulWithBackend %s: %v", id, err)
		}
	}
	if _, err := CreateWorkspaceTemplateWithBackend(ctx, "file", dataRoot, WorkspaceTemplate{
		ID:            "template-code-agent",
		Name:          "code agent",
		DefaultSoulID: "soul-template-default",
		SoulBindings: map[string]string{
			"planning":  "soul-template-planning",
			"execution": "soul-template-execution",
		},
		DoDItems: []string{"tests pass"},
	}); err != nil {
		t.Fatalf("CreateWorkspaceTemplateWithBackend: %v", err)
	}

	item, err := CreateWorkspaceWithTemplateWithBackend(ctx, "file", dataRoot, CreateWorkspaceCommand{
		Title:            "software workspace",
		WorkingDirectory: "workspaces/software",
		TemplateID:       "template-code-agent",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceWithTemplateWithBackend: %v", err)
	}

	if item.DefaultSoulID != "soul-template-default" {
		t.Fatalf("defaultSoulId = %q, want soul-template-default", item.DefaultSoulID)
	}
	for _, stage := range []string{"planning", "execution"} {
		if item.SoulBindings[stage] == "" {
			t.Fatalf("soul binding %q missing in %+v", stage, item.SoulBindings)
		}
	}

	readBack, err := ReadWorkspaceWithBackend(ctx, "file", dataRoot, item.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if readBack.DefaultSoulID != item.DefaultSoulID || readBack.SoulBindings["execution"] != item.SoulBindings["execution"] {
		t.Fatalf("read workspace soul bindings = %+v, want %+v", readBack, item)
	}
}

func TestReadWorkspaceWithBackendReturnsPersistedWorkspace(t *testing.T) {
	dataRoot := t.TempDir()
	created, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "appservice read workspace", "read description", "workspaces/appservice read workspace")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	item, err := ReadWorkspaceWithBackend(context.Background(), "file", dataRoot, created.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if item.ID != created.ID || item.Title != "appservice read workspace" || item.Description != "read description" || item.WorkingDirectory != "workspaces/appservice read workspace" {
		t.Fatalf("workspace = %+v, want persisted workspace", item)
	}
}

func TestReadAndListWorkspacesProjectActiveRunState(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	created, err := CreateWorkspaceWithBackend(ctx, "file", dataRoot, "projected workspace", "", "workspaces/projected")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	run, err := CreateRunWithBackend(ctx, "file", dataRoot, created.ID, "run-project-workspace", "", "")
	if err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	session, err := NewRuntimeSessionWithBackend("file", dataRoot)
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	workspaceItem, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(created.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.State = common.WorkspaceStateRunning
	workspaceItem.ActiveRunID = common.RunID(run.ID)
	if err := exports.Repositories.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace: %v", err)
	}
	runItem, err := exports.Repositories.Runs.Get(ctx, common.RunID(run.ID))
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	runItem.State = common.RunStateBlockedOnApproval
	runItem.BlockingApprovalID = common.ApprovalID("approval-project-workspace")
	if err := exports.Repositories.Runs.Update(ctx, runItem); err != nil {
		t.Fatalf("update run: %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session: %v", err)
	}

	readBack, err := ReadWorkspaceWithBackend(ctx, "file", dataRoot, created.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceWithBackend: %v", err)
	}
	if readBack.State != string(common.WorkspaceStateBlocked) || readBack.ActiveRunID != run.ID {
		t.Fatalf("read workspace = %+v, want blocked projection with active run", readBack)
	}
	items, err := ListWorkspacesWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("ListWorkspacesWithBackend: %v", err)
	}
	if len(items) != 1 || items[0].State != string(common.WorkspaceStateBlocked) || items[0].ActiveRunID != run.ID {
		t.Fatalf("listed workspaces = %+v, want blocked projection with active run", items)
	}
}

func TestCreateWorkspaceWithBackendDefaultsWorkingDirectory(t *testing.T) {
	dataRoot := t.TempDir()
	item, err := CreateWorkspaceWithBackend(context.Background(), "file", dataRoot, "appservice default directory workspace", "", "")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	want := filepath.Join(dataRoot, "workspaces", item.ID, "working")
	if item.WorkingDirectory != want {
		t.Fatalf("workingDirectory = %q, want %q", item.WorkingDirectory, want)
	}
	for _, dir := range []string{
		filepath.Join(dataRoot, "workspaces", item.ID, "working"),
		filepath.Join(dataRoot, "workspaces", item.ID, "artifacts"),
		filepath.Join(dataRoot, "workspaces", item.ID, "runs"),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("workspace directory %q = %v/%v, want existing directory", dir, info, err)
		}
	}
}

func TestCreateWorkspaceWithBackendCreatesExplicitWorkingDirectory(t *testing.T) {
	workingDirectory := filepath.Join(t.TempDir(), "custom-working")
	item, err := CreateWorkspaceWithBackend(context.Background(), "file", t.TempDir(), "appservice explicit directory workspace", "", workingDirectory)
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if item.WorkingDirectory != workingDirectory {
		t.Fatalf("workingDirectory = %q, want %q", item.WorkingDirectory, workingDirectory)
	}
	if info, err := os.Stat(workingDirectory); err != nil || !info.IsDir() {
		t.Fatalf("explicit working directory = %v/%v, want existing directory", info, err)
	}
}

func TestReadWorkspaceWithBackendReturnsNotFoundWhenMissing(t *testing.T) {
	if _, err := ReadWorkspaceWithBackend(context.Background(), "file", t.TempDir(), "ws-missing"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestDeleteWorkspaceWithBackendDeletesOnlyWorkspaceByDefault(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	created, err := CreateWorkspaceWithBackend(ctx, "file", dataRoot, "delete workspace", "", "")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	if _, err := DefineGoalWithBackend(ctx, "file", dataRoot, created.ID, "keep related goal"); err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}

	if err := DeleteWorkspaceWithBackend(ctx, "file", dataRoot, created.ID, DeleteWorkspaceCommand{}); err != nil {
		t.Fatalf("DeleteWorkspaceWithBackend: %v", err)
	}
	if _, err := ReadWorkspaceWithBackend(ctx, "file", dataRoot, created.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ReadWorkspaceWithBackend err = %v, want not found", err)
	}
	if info, err := os.Stat(created.WorkingDirectory); err != nil || !info.IsDir() {
		t.Fatalf("working directory = %v/%v, want preserved directory", info, err)
	}
}

func TestDeleteWorkspaceWithBackendPurgesWorkspaceData(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	created, err := CreateWorkspaceWithBackend(ctx, "file", dataRoot, "purge workspace", "", "")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, created.ID, "purge related goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, created.ID, []string{"purge related dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateRunWithBackend(ctx, "file", dataRoot, created.ID, "run-purge-workspace", goal.ID, dod.ID); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}

	if err := DeleteWorkspaceWithBackend(ctx, "file", dataRoot, created.ID, DeleteWorkspaceCommand{Purge: true}); err != nil {
		t.Fatalf("DeleteWorkspaceWithBackend purge: %v", err)
	}
	if _, err := ReadWorkspaceWithBackend(ctx, "file", dataRoot, created.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ReadWorkspaceWithBackend err = %v, want not found", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "workspaces", created.ID)); !os.IsNotExist(err) {
		t.Fatalf("workspace data root stat err = %v, want not exist", err)
	}
}

func TestListWorkspacesWithBackendReturnsSortedWorkspaces(t *testing.T) {
	dataRoot := t.TempDir()
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-b", "workspace b", "", "workspaces/workspace b"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-b: %v", err)
	}
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-a", "workspace a", "", "workspaces/workspace a"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend ws-a: %v", err)
	}

	items, err := ListWorkspacesWithBackend(context.Background(), "file", dataRoot)
	if err != nil {
		t.Fatalf("ListWorkspacesWithBackend: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != "ws-a" || items[1].ID != "ws-b" {
		t.Fatalf("workspace ids = %q/%q, want ws-a/ws-b", items[0].ID, items[1].ID)
	}
}
