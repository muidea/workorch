package workspace

import (
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestDefinitionAuthoringReducerSavesWithoutAdmission(t *testing.T) {
	w := &workspace.Workspace{ID: "ws", Revision: 1, TemplateID: "t"}
	template := &workspace.WorkspaceTemplate{ID: "t"}
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, TemplateID: "t", GoalText: "目标", AcceptanceRequirements: []string{"条件"}}
	cmd := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-test", Input: input, ExpectedWorkspaceRevision: 1}
	o := definitionSpecificationObservation{workspace: w, template: template}
	first, err := reduceDefinitionAuthoring(o, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if first.Next == nil || first.Next.DefinitionAuthoring == nil || w.DefinitionAuthoring != nil {
		t.Fatal("missing detached record")
	}
	if first.Next.StartupInput != nil || first.Next.ActivationAdmission != nil || first.Next.ActiveRunID != "" {
		t.Fatal("draft promoted authority")
	}
	first.Authoring.Record.Input.AcceptanceRequirements[0] = "外部改写"
	if first.Next.DefinitionAuthoring.Input.AcceptanceRequirements[0] != "条件" {
		t.Fatal("result aliases persisted record")
	}
	o.workspace = first.Next
	o.workspace.Revision++
	replay, err := reduceDefinitionAuthoring(o, cmd)
	if err != nil || replay.Next != nil {
		t.Fatalf("duplicate must not write: %v", err)
	}
	cmd.Input.GoalText = "新目标"
	if _, err := reduceDefinitionAuthoring(o, cmd); err == nil {
		t.Fatal("same revision changed input accepted")
	}
	cmd.Input.Revision++
	if _, err := reduceDefinitionAuthoring(o, cmd); err == nil {
		t.Fatal("stale CAS accepted")
	}
	cmd.ExpectedWorkspaceRevision = o.workspace.Revision
	cmd.ExpectedInputHash = o.workspace.DefinitionAuthoring.InputHash
	cmd.RequestID = "new-save"
	if _, err := reduceDefinitionAuthoring(o, cmd); err != nil {
		t.Fatal(err)
	}
	cmd.Input = input
	template.Description = "模板已变"
	if _, err := reduceDefinitionAuthoring(o, cmd); err == nil {
		t.Fatal("template drift silently replayed")
	}
}

func TestDefinitionAuthoringReducerRejectsExistingAuthority(t *testing.T) {
	cmd := workspaceevents.SaveDefinitionAuthoringCommand{RequestID: "save-test", Input: workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, TemplateID: "t", GoalText: "目标"}, ExpectedWorkspaceRevision: 1}
	for _, mutate := range []func(*workspace.Workspace){
		func(w *workspace.Workspace) { w.StartupInput = &workspace.WorkspaceStartupInputContract{} },
		func(w *workspace.Workspace) { w.ActivationAdmission = &workspace.WorkspaceActivationAdmission{} },
		func(w *workspace.Workspace) { w.ActiveRunID = "run" },
		func(w *workspace.Workspace) {
			w.DefinitionSpecificationLedger = &workspace.DefinitionSpecificationRequestLedger{}
		},
	} {
		w := &workspace.Workspace{ID: "ws", Revision: 1, TemplateID: "t"}
		mutate(w)
		if _, err := reduceDefinitionAuthoring(definitionSpecificationObservation{workspace: w, template: &workspace.WorkspaceTemplate{ID: "t"}}, cmd); err == nil {
			t.Fatal("existing authority accepted new creation draft")
		}
	}
}
