package workspace

import (
	"context"
	"testing"

	appbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
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
	defer owners.close()
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	application = &appbiz.Biz{Base: basebiz.New(appbiz.ModuleID, owners.hub, nil)}
	replay, err := application.CreateDefinitionAuthoringView(ctx, "file", root, request)
	if err != nil || replay.WorkspaceID != view.WorkspaceID || replay.WorkspaceRevision != view.WorkspaceRevision || replay.InputHash != view.InputHash {
		t.Fatalf("lost create response did not recover: %+v %v", replay, err)
	}
	save := workorchmanagementcontract.DefinitionAuthoringSave{Input: *view.Input, ExpectedWorkspaceRevision: view.WorkspaceRevision, ExpectedInputHash: view.InputHash}
	save.Input.GoalText = "修订目标"
	updated, err := application.SaveDefinitionAuthoringView(ctx, common.WorkspaceID(view.WorkspaceID), save)
	if err != nil || updated.InputRevision != 2 || updated.InputHash == view.InputHash {
		t.Fatalf("application edit: %+v %v", updated, err)
	}
	replay, err = application.CreateDefinitionAuthoringView(ctx, "file", root, request)
	if err != nil || replay.InputHash != updated.InputHash {
		t.Fatalf("initial create replay reset edited record: %+v %v", replay, err)
	}
	items, err := owners.repositories.Workspaces.List(ctx)
	if err != nil || len(items) != 1 {
		t.Fatal("application created duplicate workspace")
	}
}
