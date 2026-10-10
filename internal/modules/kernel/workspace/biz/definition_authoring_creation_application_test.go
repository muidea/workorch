package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	appbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestDefinitionAuthoringApplicationCreationRecoversFileOwner(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &workspace.WorkspaceTemplate{ID: "template", Name: "模板"}); err != nil {
		owners.close()
		t.Fatal(err)
	}
	application := &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	if err := owners.workspace.Start(); err != nil {
		owners.close()
		t.Fatal(err)
	}
	request := workorchmanagementcontract.DefinitionAuthoringCreate{RequestID: "durable-create", Title: "任务", TemplateID: "template", Input: workorchmanagementcontract.DefinitionAuthoringInput{GoalText: "用户目标", ResourceRefs: []string{"input.txt"}}}
	view, err := application.CreateDefinitionAuthoringView(ctx, "file", root, request)
	if err != nil || view.State != "draft" || view.Input == nil || view.Input.GoalText != "用户目标" {
		owners.close()
		t.Fatalf("application create: %+v %v", view, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application = &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	replay, err := application.CreateDefinitionAuthoringView(ctx, "file", root, request)
	if err != nil || replay.WorkspaceID != view.WorkspaceID || replay.WorkspaceRevision != view.WorkspaceRevision || replay.InputHash != view.InputHash {
		t.Fatalf("lost create response did not recover: %+v %v", replay, err)
	}
	save := workorchmanagementcontract.DefinitionAuthoringSave{RequestID: "save-edited", TemplateID: request.TemplateID, Input: *view.Input, ExpectedWorkspaceRevision: view.WorkspaceRevision, ExpectedInputHash: view.InputHash}
	save.Input.GoalText = "修订目标"
	updated, err := application.SaveDefinitionAuthoringView(ctx, common.WorkspaceID(view.WorkspaceID), save)
	if err != nil || updated.InputRevision != 2 || updated.InputHash == view.InputHash {
		t.Fatalf("application edit: %+v %v", updated, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application = &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	savedReplay, err := application.SaveDefinitionAuthoringView(ctx, common.WorkspaceID(view.WorkspaceID), save)
	if err != nil || savedReplay.InputHash != updated.InputHash || savedReplay.WorkspaceRevision != updated.WorkspaceRevision || savedReplay.InputRevision != 2 {
		t.Fatalf("lost revision-save response did not recover after reopen: %+v %v", savedReplay, err)
	}
	conflictingSave := save
	conflictingSave.Input.GoalText = "过期请求试图覆盖新目标"
	if _, err := application.SaveDefinitionAuthoringView(ctx, common.WorkspaceID(view.WorkspaceID), conflictingSave); err == nil {
		t.Fatal("stale different payload overwrote current revision")
	}
	replay, err = application.CreateDefinitionAuthoringView(ctx, "file", root, request)
	if err != nil || replay.InputHash != updated.InputHash {
		t.Fatalf("initial create replay reset edited record: %+v %v", replay, err)
	}
	items, err := owners.repositories.Workspaces.List(ctx)
	if err != nil || len(items) != 1 {
		t.Fatal("application created duplicate workspace")
	}
	newSave := workorchmanagementcontract.DefinitionAuthoringSave{RequestID: "save-successor", TemplateID: request.TemplateID, Input: *updated.Input, ExpectedWorkspaceRevision: updated.WorkspaceRevision, ExpectedInputHash: updated.InputHash}
	newSave.Input.GoalText = "后续目标"
	successor, err := application.SaveDefinitionAuthoringView(ctx, common.WorkspaceID(view.WorkspaceID), newSave)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.SaveDefinitionAuthoringView(ctx, common.WorkspaceID(view.WorkspaceID), save); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("历史保存回执冒充当前保存成功: %v", err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, common.WorkspaceID(view.WorkspaceID))
	if err != nil || current.Revision != successor.WorkspaceRevision || current.DefinitionAuthoring.InputHash != successor.InputHash || current.ActiveRunID != "" || current.StartupInput != nil {
		t.Fatalf("历史保存重放改变当前记录: %+v %v", current, err)
	}
}

func TestDefinitionAuthoringApplicationTemplateDirectoryReplayAfterReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	for _, id := range []string{"a", "b"} {
		if err := owners.repositories.WorkspaceTemplates.Create(ctx, &workspace.WorkspaceTemplate{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application := &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	create := workorchmanagementcontract.DefinitionAuthoringCreate{RequestID: "template-directory", Title: "任务", TemplateID: "a", Input: workorchmanagementcontract.DefinitionAuthoringInput{GoalText: "目标"}}
	view, err := application.CreateDefinitionAuthoringView(ctx, "file", root, create)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "delivery")
	save := workorchmanagementcontract.DefinitionAuthoringSave{RequestID: "switch", TemplateID: "b", DeliverableDirectory: &directory, Input: *view.Input, ExpectedWorkspaceRevision: view.WorkspaceRevision, ExpectedInputHash: view.InputHash}
	saved, err := application.SaveDefinitionAuthoringView(ctx, common.WorkspaceID(view.WorkspaceID), save)
	if err != nil {
		t.Fatal(err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application = &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	replay, err := application.SaveDefinitionAuthoringView(ctx, common.WorkspaceID(view.WorkspaceID), save)
	if err != nil || replay.WorkspaceRevision != saved.WorkspaceRevision || replay.InputHash != saved.InputHash || replay.TemplateID != "b" {
		t.Fatalf("模板目录原请求未精确恢复: %+v %v", replay, err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, common.WorkspaceID(view.WorkspaceID))
	if err != nil || current.DeliverableDirectory != directory || len(current.DefinitionAuthoring.SaveReceipts) != 1 {
		t.Fatal("恢复额外写入或丢失目录")
	}
	nextDirectory := filepath.Join(t.TempDir(), "next")
	successor := workorchmanagementcontract.DefinitionAuthoringSave{RequestID: "directory-successor", TemplateID: "b", DeliverableDirectory: &nextDirectory, Input: *saved.Input, ExpectedWorkspaceRevision: saved.WorkspaceRevision, ExpectedInputHash: saved.InputHash}
	next, err := application.SaveDefinitionAuthoringView(ctx, common.WorkspaceID(view.WorkspaceID), successor)
	if err != nil || next.InputHash == saved.InputHash {
		t.Fatalf("仅目录修订未形成新身份: %+v %v", next, err)
	}
	if _, err := application.SaveDefinitionAuthoringView(ctx, common.WorkspaceID(view.WorkspaceID), save); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("旧目录回执被当作当前成功: %v", err)
	}
	current, err = owners.repositories.Workspaces.Get(ctx, common.WorkspaceID(view.WorkspaceID))
	if err != nil || current.Revision != next.WorkspaceRevision || current.DeliverableDirectory != nextDirectory || len(current.DefinitionAuthoring.SaveReceipts) != 2 || current.ActiveRunID != "" {
		t.Fatal("历史请求改变后续目录或回执")
	}
}
