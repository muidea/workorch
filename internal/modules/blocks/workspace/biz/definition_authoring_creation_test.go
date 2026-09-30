package workspace

import (
	"context"
	"errors"
	"sync"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDefinitionAuthoringCreationFileReplayAndConflict(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	template := &workspace.WorkspaceTemplate{ID: "template", Name: "模板"}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, template); err != nil {
		owners.close()
		t.Fatal(err)
	}
	id := workspace.DefinitionAuthoringWorkspaceID("create-request")
	cmd := workspaceevents.CreateDefinitionAuthoringCommand{RequestID: "create-request", WorkspaceID: id, Title: "任务", DeliverableDirectory: root + "/project", InternalDirectory: root + "/internal", Input: workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, TemplateID: template.ID, GoalText: "用户目标", AcceptanceRequirements: []string{"保留要求"}}}
	first, err := owners.workspace.createDefinitionAuthoring(ctx, cmd)
	if err != nil {
		owners.close()
		t.Fatal(err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || stored.DefinitionAuthoring == nil || stored.DefinitionAuthoringCreation == nil || stored.StartupInput != nil || stored.ActiveRunID != "" {
		owners.close()
		t.Fatalf("initial draft missing or promoted: %+v %v", stored, err)
	}
	initialRevision := stored.Revision
	stored.DefinitionAuthoringCreation.RequestID = "external-change"
	again, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || again.DefinitionAuthoringCreation.RequestID != cmd.RequestID {
		owners.close()
		t.Fatal("creation identity aliases repository")
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	// 模板不存在也可重放已经持久的创建回执。
	if err := owners.repositories.WorkspaceTemplates.Delete(ctx, template.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			replay, err := owners.workspace.createDefinitionAuthoring(ctx, cmd)
			if err != nil || replay != first {
				t.Errorf("creation replay differs: %+v %v", replay, err)
			}
		})
	}
	wg.Wait()
	cmd.Input.GoalText = "不同目标"
	if _, err := owners.workspace.createDefinitionAuthoring(ctx, cmd); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same request changed payload: %v", err)
	}
	last, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || last.Revision != initialRevision || last.DefinitionAuthoring.Input.GoalText != "用户目标" {
		t.Fatalf("replay rewrote draft: %+v %v", last, err)
	}
	items, err := owners.repositories.Workspaces.List(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("creation duplicated workspace: %d %v", len(items), err)
	}
	goals, err := owners.repositories.Goals.ListByWorkspaceID(ctx, id)
	if err != nil || len(goals) != 0 {
		t.Fatal("creation wrote formal Goal")
	}
	dods, err := owners.repositories.DoDs.ListByWorkspaceID(ctx, id)
	if err != nil || len(dods) != 0 {
		t.Fatal("creation wrote placeholder DoD")
	}
}
