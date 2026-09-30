package workspace

import (
	"context"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestDefinitionAuthoringRevisionChangesTemplateAndDirectoryAtomically(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	for _, id := range []string{"old", "new"} {
		if err := owners.repositories.WorkspaceTemplates.Create(ctx, &workspace.WorkspaceTemplate{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	id := workspace.DefinitionAuthoringWorkspaceID("switch-template")
	cmd := workspaceevents.CreateDefinitionAuthoringCommand{RequestID: "switch-template", WorkspaceID: id, Title: "任务", DeliverableDirectory: "/old", InternalDirectory: "/internal", Input: workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: id, Revision: 1, TemplateID: "old", GoalText: "用户目标"}}
	if _, err := owners.workspace.createDefinitionAuthoring(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	before, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	directory := "/new"
	input := cmd.Input.Clone()
	input.Revision++
	input.TemplateID = "new"
	save := workspaceevents.SaveDefinitionAuthoringCommand{Input: input, ExpectedWorkspaceRevision: before.Revision, DeliverableDirectory: &directory}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err != nil {
		t.Fatal(err)
	}
	after, err := owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || after.TemplateID != "new" || after.DeliverableDirectory != directory || after.DefinitionAuthoring.Input.Revision != 2 || after.DefinitionAuthoring.Input.GoalText != "用户目标" || after.DefinitionAuthoring.TemplateSnapshot.ID != "new" || after.DefinitionAuthoring.InputHash == before.DefinitionAuthoring.InputHash {
		t.Fatalf("revision did not atomically update inputs: %+v %v", after, err)
	}
	revision := after.Revision
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err != nil {
		t.Fatalf("lost response replay failed: %v", err)
	}
	if _, err := owners.workspace.createDefinitionAuthoring(ctx, cmd); err != nil {
		t.Fatalf("initial create retry after edits failed: %v", err)
	}
	after, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || after.Revision != revision || after.DefinitionAuthoring.InputHash != input.InputHash() {
		t.Fatal("initial creation replay reapplied old inputs")
	}
	input.Revision++
	input.TemplateID = "old"
	save.Input = input
	save.ExpectedWorkspaceRevision = revision
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err != nil {
		t.Fatalf("ABA revision failed: %v", err)
	}
	after, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || after.DefinitionAuthoring.InputHash == before.DefinitionAuthoring.InputHash || after.DefinitionAuthoring.Input.Revision != 3 {
		t.Fatal("ABA revision reused initial identity")
	}
	revision = after.Revision
	input.Revision++
	input.TemplateID = "missing"
	save.Input = input
	save.ExpectedWorkspaceRevision = revision
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: id, Authoring: &save}); err == nil {
		t.Fatal("missing template accepted")
	}
	after, err = owners.repositories.Workspaces.Get(ctx, id)
	if err != nil || after.Revision != revision || after.TemplateID != "old" {
		t.Fatal("failed revision partially changed configuration")
	}
}
