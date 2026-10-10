package workspace

import (
	"context"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestDefinitionAuthoringFileReopenAndQuery(t *testing.T) {
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	ctx := context.Background()
	w := &workspace.Workspace{ID: "ws-authoring", TemplateID: "authoring-template"}
	if err := owners.repositories.Workspaces.Create(ctx, w); err != nil {
		owners.close()
		t.Fatal(err)
	}
	if err := owners.repositories.WorkspaceTemplates.Create(ctx, &workspace.WorkspaceTemplate{ID: w.TemplateID, Name: "模板"}); err != nil {
		owners.close()
		t.Fatal(err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil {
		owners.close()
		t.Fatal(err)
	}
	command := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-test", Input: workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: w.ID, Revision: 1, GoalText: "目标", TemplateID: w.TemplateID, AcceptanceRequirements: []string{"用户条件"}}, ExpectedWorkspaceRevision: stored.Revision}
	_, err = owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: w.ID, Authoring: &command})
	owners.close()
	if err != nil {
		t.Fatal(err)
	}
	owners = openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	query := workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: w.ID}
	result, err := owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if result.Record == nil || result.Record.InputHash != command.Input.InputHash() || result.WorkspaceRevision <= stored.Revision {
		t.Fatalf("record not recovered: %+v", result)
	}
	result.Record.Input.AcceptanceRequirements[0] = "外部修改"
	again, err := owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil || again.Record.Input.AcceptanceRequirements[0] != "用户条件" {
		t.Fatalf("query aliases owner: %v", err)
	}
	if _, err := owners.workspace.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: w.ID, Authoring: &command}); err != nil {
		t.Fatal(err)
	}
	last, err := owners.workspace.queryDefinitionAuthoring(ctx, query)
	if err != nil || last.WorkspaceRevision != again.WorkspaceRevision {
		t.Fatalf("replay wrote again: %v", err)
	}
	goals, err := owners.repositories.Goals.ListByWorkspaceID(ctx, w.ID)
	if err != nil || len(goals) != 0 {
		t.Fatal("draft created formal goal")
	}
	dods, err := owners.repositories.DoDs.ListByWorkspaceID(ctx, w.ID)
	if err != nil || len(dods) != 0 {
		t.Fatal("draft created placeholder DoD")
	}
}
