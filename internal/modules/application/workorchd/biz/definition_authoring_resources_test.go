package biz

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	fileowner "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringResourcesUseRealFileOwnerAndFailOnStaleOrUnsafeInput(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := fileowner.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "requirements.txt"), []byte("交付验收说明\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t", ResourceRefs: []string{"requirements.txt"}}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	queries := 0
	revision := uint64(1)
	driftOnSecondQuery := false
	observer := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	if err := observer.Subscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, func(ev magicevent.Event, result magicevent.Result) {
		queries++
		if driftOnSecondQuery && queries%2 == 0 {
			revision++
		}
		copy := record.Clone()
		response := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: input.WorkspaceID, WorkspaceRevision: revision, Record: &copy, DeliverableDirectory: root, CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash}
		result.Set(response, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { observer.Unsubscribe(workspaceevents.TopicWorkspaceDefinitionAuthoringQuery) })
	if err := observer.Subscribe(workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(workspaceevents.QueryAuthoringResourceScopeCommand)
		if !ok || ev.Source() != fileowner.ModuleID {
			result.Set(nil, nil)
			return
		}
		copy := record.Clone()
		view := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: input.WorkspaceID, WorkspaceRevision: revision, Record: &copy, DeliverableDirectory: root, CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash}
		scope, err := workspaceevents.AuthoringResourceScopeFor(view, command)
		if err != nil {
			result.Set(nil, nil)
			return
		}
		result.Set(scope, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { observer.Unsubscribe(workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery) })
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	facts, err := b.AcquireDefinitionAuthoringResources(ctx, input.WorkspaceID)
	if err != nil || len(facts) != 1 || facts[0].Content != "交付验收说明\n" || facts[0].WorkspaceRevision != revision || authoringResourceFactsFor(input, facts) != nil || queries != 2 {
		t.Fatalf("owner resource fact: %+v queries=%d err=%v", facts, queries, err)
	}
	firstHash := facts[0].ContentHash
	revision++
	if err := os.WriteFile(filepath.Join(root, "requirements.txt"), []byte("内容已变化\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	facts, err = b.AcquireDefinitionAuthoringResources(ctx, input.WorkspaceID)
	if err != nil || len(facts) != 1 || facts[0].ContentHash == firstHash {
		t.Fatalf("resource drift retained old hash: %+v err=%v", facts, err)
	}
	queriesBefore := queries
	revision++
	if _, err := b.AcquireDefinitionAuthoringResources(ctx, input.WorkspaceID); err != nil {
		t.Fatalf("stable resource revision rejected: %v", err)
	}
	if queries != queriesBefore+2 {
		t.Fatalf("resource read did not revalidate Workspace revision: %d", queries)
	}
	driftOnSecondQuery = true
	if _, err := b.AcquireDefinitionAuthoringResources(ctx, input.WorkspaceID); err == nil {
		t.Fatal("Workspace revision changed during resource scan without being detected")
	}
	driftOnSecondQuery = false
	input.ResourceRefs[0] = "../secret.txt"
	record.Input = input
	record.InputHash = input.InputHash()
	if _, err := b.AcquireDefinitionAuthoringResources(ctx, input.WorkspaceID); err == nil {
		t.Fatal("unsafe resource reference reached derivation")
	}
	input.ResourceRefs = []string{"requirements.txt"}
	record.Input = input
	record.InputHash = input.InputHash()
	record.TemplateSnapshotHash = "changed-template"
	if _, err := b.AcquireDefinitionAuthoringResources(ctx, input.WorkspaceID); err == nil {
		t.Fatal("template drift reached resource observation")
	}
}
