package biz

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceowner "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func TestCreationRecoveryResponseRetainsIdentityAcrossDirectoryFailure(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	storage := filepath.Join(root, "data")
	directory := filepath.Join(root, "project")
	if err := os.WriteFile(directory, []byte("用户既有文件，创建不得覆盖"), 0o600); err != nil {
		t.Fatal(err)
	}
	repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
	if err := repositories.WorkspaceTemplates.Create(ctx, &workspace.WorkspaceTemplate{ID: "template", Name: "模板"}); err != nil {
		t.Fatal(err)
	}
	request := management.DefinitionAuthoringCreate{RequestID: "creation-directory-recovery", Title: "任务", TemplateID: "template", DeliverableDirectory: directory, Input: management.DefinitionAuthoringInput{GoalText: "原始目标"}}
	open := func() (*Biz, func()) {
		hub := magicevent.NewHub(32)
		owners := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
		owner, err := workspaceowner.New(hub, nil, workspaceowner.Dependencies{OwnerRepositories: owners.WorkspaceOwner, EventHub: hub})
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		app := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
		return app, func() {
			if err := owner.Teardown(ctx); err != nil {
				t.Error(err)
			}
			hub.Terminate(ctx)
		}
	}
	app, closeOwners := open()
	_, err := app.CreateDefinitionAuthoringView(ctx, "file", storage, request)
	var failure *management.DefinitionAuthoringCreationError
	if !errors.As(err, &failure) || failure.Recovery.Persistence != "persisted" || failure.Recovery.Phase != "directory_preparation" || failure.Recovery.RequestID != request.RequestID || !failure.Recovery.RetrySameRequest {
		closeOwners()
		t.Fatalf("missing recoverable identity: %v", err)
	}
	id := workspace.DefinitionAuthoringWorkspaceID(request.RequestID)
	if failure.Recovery.WorkspaceID != string(id) {
		closeOwners()
		t.Fatal("wrong recovery workspace")
	}
	stored, err := repositories.Workspaces.Get(ctx, id)
	if err != nil || stored.DefinitionAuthoring == nil {
		closeOwners()
		t.Fatalf("directory failure lost draft: %+v %v", stored, err)
	}
	if bytes, err := os.ReadFile(directory); err != nil || string(bytes) != "用户既有文件，创建不得覆盖" {
		closeOwners()
		t.Fatal("creation modified existing file")
	}
	closeOwners()
	// 只移开测试生成的故障文件，保全其内容；创建流程本身不清理用户目录。
	if err := os.Rename(directory, directory+".preserved"); err != nil {
		t.Fatal(err)
	}
	app, closeOwners = open()
	defer closeOwners()
	view, err := app.CreateDefinitionAuthoringView(ctx, "file", storage, request)
	if err != nil || view.WorkspaceID != string(id) || view.WorkspaceRevision != stored.Revision || view.Input.GoalText != request.Input.GoalText {
		t.Fatalf("replay changed identity: %+v %v", view, err)
	}
	items, err := repositories.Workspaces.List(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("replay duplicated workspace: %+v %v", items, err)
	}
	runs, err := repositories.Runs.List(ctx)
	if err != nil || len(runs) != 0 {
		t.Fatal("recovery implicitly created Run")
	}
}
