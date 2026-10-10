package events

import (
	"errors"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceManagementResultsValidateExactCommandIdentity(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-result-contract")
	workspace := &Workspace{ID: workspaceID}
	for name, command := range map[string]WorkspaceResultCommand{
		"create": CreateWorkspaceCommand{WorkspaceID: workspaceID},
		"get":    GetWorkspaceCommand{WorkspaceID: workspaceID},
		"update": UpdateWorkspaceCommand{WorkspaceID: workspaceID},
	} {
		t.Run(name, func(t *testing.T) {
			if err := (WorkspaceResult{Workspace: workspace}).ValidateFor(command); err != nil {
				t.Fatalf("validate matching result: %v", err)
			}
			if err := (WorkspaceResult{Workspace: &Workspace{ID: "ws-foreign"}}).ValidateFor(command); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("wrong workspace identity error = %v, want conflict", err)
			}
		})
	}

	goalCommand := GetGoalCommand{WorkspaceID: workspaceID, GoalID: "goal-result-contract"}
	if err := (GoalResult{Goal: &workspacemodel.Goal{ID: goalCommand.GoalID, WorkspaceID: workspaceID}}).ValidateFor(goalCommand); err != nil {
		t.Fatalf("validate matching goal result: %v", err)
	}
	for name, result := range map[string]GoalResult{
		"wrong identity": {Goal: &workspacemodel.Goal{ID: "goal-foreign", WorkspaceID: workspaceID}},
		"wrong scope":    {Goal: &workspacemodel.Goal{ID: goalCommand.GoalID, WorkspaceID: "ws-foreign"}},
	} {
		t.Run("goal "+name, func(t *testing.T) {
			if err := result.ValidateFor(goalCommand); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("error = %v, want conflict", err)
			}
		})
	}

	doDCommand := GetDoDCommand{WorkspaceID: workspaceID, DoDID: "dod-result-contract"}
	if err := (DoDResult{DoD: &workspacemodel.DoD{ID: doDCommand.DoDID, WorkspaceID: workspaceID}}).ValidateFor(doDCommand); err != nil {
		t.Fatalf("validate matching DoD result: %v", err)
	}
	for name, result := range map[string]DoDResult{
		"wrong identity": {DoD: &workspacemodel.DoD{ID: "dod-foreign", WorkspaceID: workspaceID}},
		"wrong scope":    {DoD: &workspacemodel.DoD{ID: doDCommand.DoDID, WorkspaceID: "ws-foreign"}},
	} {
		t.Run("dod "+name, func(t *testing.T) {
			if err := result.ValidateFor(doDCommand); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("error = %v, want conflict", err)
			}
		})
	}

	templateCommand := GetWorkspaceTemplateCommand{TemplateID: "template-result-contract"}
	if err := (WorkspaceTemplateResult{Template: &WorkspaceTemplate{ID: templateCommand.TemplateID}}).ValidateFor(templateCommand); err != nil {
		t.Fatalf("validate matching template result: %v", err)
	}
	if err := (WorkspaceTemplateResult{Template: &WorkspaceTemplate{ID: "template-foreign"}}).ValidateFor(templateCommand); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong template identity error = %v, want conflict", err)
	}

	if err := (DeleteGoalResult{WorkspaceID: workspaceID, GoalID: goalCommand.GoalID}).ValidateFor(DeleteGoalCommand{WorkspaceID: workspaceID, GoalID: goalCommand.GoalID}); err != nil {
		t.Fatalf("validate matching goal deletion: %v", err)
	}
	if err := (DeleteDoDResult{WorkspaceID: "ws-foreign", DoDID: doDCommand.DoDID}).ValidateFor(DeleteDoDCommand{WorkspaceID: workspaceID, DoDID: doDCommand.DoDID}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong DoD deletion scope error = %v, want conflict", err)
	}
	if err := (DeleteWorkspaceTemplateResult{TemplateID: "template-foreign"}).ValidateFor(DeleteWorkspaceTemplateCommand{TemplateID: templateCommand.TemplateID}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong template deletion identity error = %v, want conflict", err)
	}

	var typedNil *GetWorkspaceCommand
	if err := (WorkspaceResult{Workspace: workspace}).ValidateFor(typedNil); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("typed-nil command error = %v, want invalid argument", err)
	}
}

func TestWorkspaceManagementListResultsRejectForeignOrDuplicateOwnerIdentity(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-result-list")
	if err := (WorkspaceListResult{Workspaces: []*Workspace{{ID: workspaceID}, {ID: workspaceID}}}).ValidateFor(ListWorkspacesCommand{}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate workspace identity error = %v, want conflict", err)
	}
	goalCommand := ListGoalsByWorkspaceCommand{WorkspaceID: workspaceID}
	if err := (GoalListResult{Goals: []*workspacemodel.Goal{{ID: "goal-a", WorkspaceID: workspaceID}, {ID: "goal-b", WorkspaceID: "ws-foreign"}}}).ValidateFor(goalCommand); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("foreign goal scope error = %v, want conflict", err)
	}
	if err := (GoalListResult{Goals: []*workspacemodel.Goal{{ID: "goal-a", WorkspaceID: workspaceID}, {ID: "goal-a", WorkspaceID: workspaceID}}}).ValidateFor(goalCommand); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate goal identity error = %v, want conflict", err)
	}
	doDCommand := ListDoDsByWorkspaceCommand{WorkspaceID: workspaceID}
	if err := (DoDListResult{DoDs: []*workspacemodel.DoD{{ID: "dod-a", WorkspaceID: "ws-foreign"}}}).ValidateFor(doDCommand); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("foreign DoD scope error = %v, want conflict", err)
	}
	if err := (WorkspaceTemplateListResult{Templates: []*WorkspaceTemplate{{ID: "template-a"}, {ID: "template-a"}}}).ValidateFor(ListWorkspaceTemplatesCommand{}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate template identity error = %v, want conflict", err)
	}
}

func TestWorkspaceLLMSelectionResultValidatesExactCommandState(t *testing.T) {
	command := UpdateWorkspaceLLMSelectionCommand{WorkspaceID: "ws-llm-result", ExpectedRevision: 7}
	if err := (WorkspaceLLMSelectionResult{Workspace: &Workspace{ID: command.WorkspaceID, Revision: 7}}).ValidateFor(command); err != nil {
		t.Fatalf("validate unchanged result: %v", err)
	}
	if err := (WorkspaceLLMSelectionResult{Workspace: &Workspace{ID: command.WorkspaceID, Revision: 8}, Changed: true}).ValidateFor(command); err != nil {
		t.Fatalf("validate changed result: %v", err)
	}
	for name, result := range map[string]WorkspaceLLMSelectionResult{
		"wrong identity":  {Workspace: &Workspace{ID: "ws-foreign", Revision: 7}},
		"wrong revision":  {Workspace: &Workspace{ID: command.WorkspaceID, Revision: 8}},
		"wrong selection": {Workspace: &Workspace{ID: command.WorkspaceID, Revision: 7, DefaultLLMSelection: &runtimecontract.ProviderInferenceSelection{}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := result.ValidateFor(command); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("error = %v, want conflict", err)
			}
		})
	}
	if err := (EnsureWorkspaceTemplateDefaultsResult{}).ValidateFor(EnsureWorkspaceTemplateDefaultsCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("false ensure result error = %v, want invalid argument", err)
	}
}
