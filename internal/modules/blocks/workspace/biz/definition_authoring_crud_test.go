package workspace

import (
	"context"
	"errors"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestAuthoringDraftExcludesFormalDefinitionCRUD(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	ctx := context.Background()
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-authoring-crud", Revision: 1, GoalText: "目标", TemplateID: "t"}
	w := &workspace.Workspace{ID: input.WorkspaceID, TemplateID: "t", DefinitionAuthoring: &workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshotHash: "snapshot"}}
	if err := owners.repositories.Workspaces.Create(ctx, w); err != nil {
		t.Fatal(err)
	}
	m := owners.workspace.manager
	for name, operation := range map[string]func() error{
		"defineGoal": func() error {
			return m.DefineGoal(ctx, workspaceevents.DefineGoalCommand{WorkspaceID: w.ID, GoalID: "g", Text: "覆盖目标"})
		},
		"updateGoal": func() error {
			return m.UpdateGoal(ctx, workspaceevents.UpdateGoalCommand{WorkspaceID: w.ID, GoalID: "g", Text: "覆盖目标"})
		},
		"deleteGoal": func() error {
			return m.DeleteGoal(ctx, workspaceevents.DeleteGoalCommand{WorkspaceID: w.ID, GoalID: "g"})
		},
		"defineDoD": func() error {
			return m.DefineDoD(ctx, workspaceevents.DefineDoDCommand{WorkspaceID: w.ID, DoDID: "d", Items: []string{"条件"}})
		},
		"updateDoD": func() error {
			return m.UpdateDoD(ctx, workspaceevents.UpdateDoDCommand{WorkspaceID: w.ID, DoDID: "d", Items: []string{"条件"}})
		},
		"deleteDoD": func() error { return m.DeleteDoD(ctx, workspaceevents.DeleteDoDCommand{WorkspaceID: w.ID, DoDID: "d"}) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("want authoring conflict, got %v", err)
			}
		})
	}
}

func TestFormalDefinitionsExcludeNewAuthoringDraft(t *testing.T) {
	fixture, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	w, err := fixture.owner.workspaces.Get(fixture.ctx, fixture.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	cmd := workspaceevents.SaveDefinitionAuthoringCommand{Input: workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: w.ID, Revision: 1, TemplateID: w.TemplateID, GoalText: "另一个目标"}, ExpectedWorkspaceRevision: w.Revision}
	if _, err := fixture.owner.submitDefinitionSpecificationInternalEvent(fixture.ctx, definitionSpecificationInternalEvent{WorkspaceID: w.ID, Authoring: &cmd}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("formal definitions accepted new draft: %v", err)
	}
}
