package biz

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceowner "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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
	var modelCalls atomic.Int32
	open := func() (*Biz, func()) {
		hub := magicevent.NewHub(32)
		trap := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
		if err := trap.Subscribe(llmevents.TopicLLMCommandCompleteProviderDocument, func(_ magicevent.Event, result magicevent.Result) {
			modelCalls.Add(1)
			result.Set(nil, def.NewError(def.Unexpected, "STEP01 must not invoke a model"))
		}); err != nil {
			t.Fatal(err)
		}
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
			if err := trap.Unsubscribe(llmevents.TopicLLMCommandCompleteProviderDocument); err != nil {
				t.Error(err)
			}
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
	observed, err := app.ReadDefinitionAuthoringView(ctx, id)
	if err != nil || observed.Creation == nil || observed.Creation.DirectoryPrepared || observed.Creation.Replay == nil || observed.State != "draft" {
		closeOwners()
		t.Fatalf("GET fabricated creation completion or lost replay source: %+v %v", observed, err)
	}
	replay := *observed.Creation.Replay // Closed-page recovery uses owner facts, not browser storage.
	closeOwners()
	// 只移开测试生成的故障文件，保全其内容；创建流程本身不清理用户目录。
	if err := os.Rename(directory, directory+".preserved"); err != nil {
		t.Fatal(err)
	}
	app, closeOwners = open()
	defer closeOwners()
	view, err := app.CreateDefinitionAuthoringView(ctx, "file", storage, replay)
	if err != nil || view.WorkspaceID != string(id) || view.WorkspaceRevision != stored.Revision+1 || view.Input.GoalText != request.Input.GoalText || view.Creation == nil || !view.Creation.DirectoryPrepared {
		t.Fatalf("replay did not complete the original creation: %+v %v cause=%v", view, err, errors.Unwrap(err))
	}
	items, err := repositories.Workspaces.List(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("replay duplicated workspace: %+v %v", items, err)
	}
	runs, err := repositories.Runs.List(ctx)
	if err != nil || len(runs) != 0 {
		t.Fatal("recovery implicitly created Run")
	}
	read, err := app.ReadDefinitionAuthoringView(ctx, id)
	if err != nil || read.Creation == nil || !read.Creation.DirectoryPrepared || read.Creation.RequestID != request.RequestID || read.InputHash != view.InputHash {
		t.Fatalf("independent read lost completed original creation: %+v %v", read, err)
	}
	replay.Input.GoalText = "conflicting new content"
	if _, err := app.CreateDefinitionAuthoringView(ctx, "file", storage, replay); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same identity overwrote original: %v", err)
	}
	if modelCalls.Load() != 0 {
		t.Fatalf("creation/read/replay invoked model: %d", modelCalls.Load())
	}
	goals, goalErr := repositories.Goals.ListByWorkspaceID(ctx, id)
	dods, dodErr := repositories.DoDs.ListByWorkspaceID(ctx, id)
	if goalErr != nil || dodErr != nil || len(goals) != 0 || len(dods) != 0 {
		t.Fatal("draft created a formal contract")
	}
}
