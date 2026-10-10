package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDefinitionAuthoringCreationFileCutpointReplayPreservesLaterDraft(t *testing.T) {
	for _, stage := range []string{"bindings", "index"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			template := &workspace.WorkspaceTemplate{ID: "template", Name: "模板", DefaultSoulID: "soul-default", SoulBindings: map[string]string{"implementation": "soul-worker"}}
			if err := owners.repositories.WorkspaceTemplates.Create(ctx, template); err != nil {
				owners.close()
				t.Fatal(err)
			}
			id := workspace.DefinitionAuthoringWorkspaceID("cutpoint-" + stage)
			cmd := workspaceevents.CreateDefinitionAuthoringCommand{RequestID: "cutpoint-" + stage, WorkspaceID: id, Title: "任务", DeliverableDirectory: root + "/project", InternalDirectory: root + "/internal", Input: workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, TemplateID: template.ID, GoalText: "原始目标"}}
			obstacle := filepath.Join(root, "internal", "workspaces", string(id), "config")
			if stage == "index" {
				obstacle = filepath.Join(root, "system", "workspace_index.json")
			}
			if stage == "index" {
				if err := os.MkdirAll(obstacle, 0o755); err != nil {
					owners.close()
					t.Fatal(err)
				}
			} else {
				if err := os.MkdirAll(filepath.Dir(obstacle), 0o755); err != nil {
					owners.close()
					t.Fatal(err)
				}
				if err := os.WriteFile(obstacle, []byte("测试创建障碍"), 0o600); err != nil {
					owners.close()
					t.Fatal(err)
				}
			}
			if _, err := owners.workspace.createDefinitionAuthoring(ctx, cmd); err == nil {
				owners.close()
				t.Fatal("partial create reported success")
			}
			owners.close()
			if err := os.Remove(obstacle); err != nil {
				t.Fatal(err)
			}
			owners = openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			if _, err := owners.workspace.createDefinitionAuthoring(ctx, cmd); err != nil {
				t.Fatal(err)
			}
			current, err := owners.repositories.Workspaces.Get(ctx, id)
			if err != nil || current.DefaultSoulID != "soul-default" || current.SoulBindings["implementation"] != "soul-worker" || current.DefinitionAuthoringCreation.PendingPersistence != nil {
				t.Fatalf("recovery missing binding: %+v %v", current, err)
			}
			items, err := owners.repositories.Workspaces.List(ctx)
			if err != nil || len(items) != 1 {
				t.Fatalf("recovery missing index: %+v %v", items, err)
			}
			next := current.DefinitionAuthoring.Input
			next.Revision, next.GoalText = 2, "后续目标"
			save := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-after-create", ExpectedInputHash: current.DefinitionAuthoring.InputHash, Input: next, ExpectedWorkspaceRevision: current.Revision}
			if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err != nil {
				t.Fatal(err)
			}
			if _, err := owners.workspace.createDefinitionAuthoring(ctx, cmd); err != nil {
				t.Fatal(err)
			}
			current, err = owners.repositories.Workspaces.Get(ctx, id)
			if err != nil || current.DefinitionAuthoring.Input.GoalText != "后续目标" || current.DefinitionAuthoring.Input.Revision != 2 {
				t.Fatalf("initial replay rewrote later draft: %+v %v", current, err)
			}
			// 退役／删除标记不能被迟到创建重放绕过；坏标记同样 fail closed。
			current.Deletion = &workspace.WorkspaceDeletionRecord{}
			if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			if _, err := owners.workspace.createDefinitionAuthoring(ctx, cmd); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("late create bypassed deletion guard: %v", err)
			}
		})
	}
}

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
