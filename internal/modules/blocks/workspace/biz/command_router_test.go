package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryRepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestWorkspaceCommandTopicsHaveOneSemanticFamily(t *testing.T) {
	expected := map[string]workspaceCommandFamily{
		workspaceevents.TopicWorkspaceCommandCreate:             workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandGet:                workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandList:               workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandUpdate:             workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandUpdateLLMSelection: workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandDelete:             workspaceCommandFamilyDefinition,
		workspaceevents.TopicWorkspaceCommandDefineGoal:         workspaceCommandFamilyGoal,
		workspaceevents.TopicWorkspaceCommandUpdateGoal:         workspaceCommandFamilyGoal,
		workspaceevents.TopicWorkspaceCommandDeleteGoal:         workspaceCommandFamilyGoal,
		workspaceevents.TopicWorkspaceCommandGetGoal:            workspaceCommandFamilyGoal,
		workspaceevents.TopicWorkspaceCommandListGoalsByWS:      workspaceCommandFamilyGoal,
		workspaceevents.TopicWorkspaceCommandDefineDoD:          workspaceCommandFamilyDoD,
		workspaceevents.TopicWorkspaceCommandUpdateDoD:          workspaceCommandFamilyDoD,
		workspaceevents.TopicWorkspaceCommandDeleteDoD:          workspaceCommandFamilyDoD,
		workspaceevents.TopicWorkspaceCommandGetDoD:             workspaceCommandFamilyDoD,
		workspaceevents.TopicWorkspaceCommandListDoDsByWS:       workspaceCommandFamilyDoD,
		workspaceevents.TopicWorkspaceTemplateCommandCreate:     workspaceCommandFamilyTemplate,
		workspaceevents.TopicWorkspaceTemplateCommandGet:        workspaceCommandFamilyTemplate,
		workspaceevents.TopicWorkspaceTemplateCommandList:       workspaceCommandFamilyTemplate,
		workspaceevents.TopicWorkspaceTemplateCommandUpdate:     workspaceCommandFamilyTemplate,
		workspaceevents.TopicWorkspaceTemplateCommandDelete:     workspaceCommandFamilyTemplate,
	}
	topics := workspaceCommandTopics()
	if len(topics) != len(expected) {
		t.Fatalf("unexpected command topic count: got %d want %d", len(topics), len(expected))
	}
	seen := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if _, duplicate := seen[topic]; duplicate {
			t.Fatalf("duplicate command topic %q", topic)
		}
		seen[topic] = struct{}{}
		family, ok := workspaceCommandFamilyForTopic(topic)
		if !ok {
			t.Fatalf("command topic %q has no family", topic)
		}
		if family != expected[topic] {
			t.Fatalf("command topic %q family = %d, want %d", topic, family, expected[topic])
		}
	}
	if _, ok := workspaceCommandFamilyForTopic("/workspace/command/unknown"); ok {
		t.Fatal("unknown command topic must fail closed")
	}
}

func TestWorkspaceCommandResultsAreNarrowPerTopic(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-narrow-result")
	workspace := &workspaceevents.Workspace{ID: workspaceID, Revision: 2}
	goal := &workspacemodel.Goal{ID: "goal-narrow-result", WorkspaceID: workspaceID}
	dod := &workspacemodel.DoD{ID: "dod-narrow-result", WorkspaceID: workspaceID}
	template := &workspaceevents.WorkspaceTemplate{ID: "template-narrow-result"}
	tests := []struct {
		topic   string
		command any
		data    workspaceCommandResponse
		want    any
	}{
		{workspaceevents.TopicWorkspaceCommandGet, workspaceevents.GetWorkspaceCommand{WorkspaceID: workspaceID}, workspaceCommandResponse{Workspace: workspace}, workspaceevents.WorkspaceResult{}},
		{workspaceevents.TopicWorkspaceCommandList, workspaceevents.ListWorkspacesCommand{}, workspaceCommandResponse{Workspaces: []*workspaceevents.Workspace{workspace}}, workspaceevents.WorkspaceListResult{}},
		{workspaceevents.TopicWorkspaceCommandUpdateLLMSelection, workspaceevents.UpdateWorkspaceLLMSelectionCommand{WorkspaceID: workspaceID, ExpectedRevision: 1}, workspaceCommandResponse{Workspace: workspace, Changed: true}, workspaceevents.WorkspaceLLMSelectionResult{}},
		{workspaceevents.TopicWorkspaceCommandGetGoal, workspaceevents.GetGoalCommand{WorkspaceID: workspaceID, GoalID: goal.ID}, workspaceCommandResponse{Goal: goal}, workspaceevents.GoalResult{}},
		{workspaceevents.TopicWorkspaceCommandListGoalsByWS, workspaceevents.ListGoalsByWorkspaceCommand{WorkspaceID: workspaceID}, workspaceCommandResponse{Goals: []*workspacemodel.Goal{goal}}, workspaceevents.GoalListResult{}},
		{workspaceevents.TopicWorkspaceCommandDeleteGoal, workspaceevents.DeleteGoalCommand{WorkspaceID: workspaceID, GoalID: goal.ID}, workspaceCommandResponse{}, workspaceevents.DeleteGoalResult{}},
		{workspaceevents.TopicWorkspaceCommandGetDoD, workspaceevents.GetDoDCommand{WorkspaceID: workspaceID, DoDID: dod.ID}, workspaceCommandResponse{DoD: dod}, workspaceevents.DoDResult{}},
		{workspaceevents.TopicWorkspaceCommandListDoDsByWS, workspaceevents.ListDoDsByWorkspaceCommand{WorkspaceID: workspaceID}, workspaceCommandResponse{DoDs: []*workspacemodel.DoD{dod}}, workspaceevents.DoDListResult{}},
		{workspaceevents.TopicWorkspaceCommandDeleteDoD, workspaceevents.DeleteDoDCommand{WorkspaceID: workspaceID, DoDID: dod.ID}, workspaceCommandResponse{}, workspaceevents.DeleteDoDResult{}},
		{workspaceevents.TopicWorkspaceTemplateCommandGet, workspaceevents.GetWorkspaceTemplateCommand{TemplateID: template.ID}, workspaceCommandResponse{Template: template}, workspaceevents.WorkspaceTemplateResult{}},
		{workspaceevents.TopicWorkspaceTemplateCommandList, workspaceevents.ListWorkspaceTemplatesCommand{}, workspaceCommandResponse{Templates: []*workspaceevents.WorkspaceTemplate{template}}, workspaceevents.WorkspaceTemplateListResult{}},
		{workspaceevents.TopicWorkspaceTemplateCommandDelete, workspaceevents.DeleteWorkspaceTemplateCommand{TemplateID: template.ID}, workspaceCommandResponse{}, workspaceevents.DeleteWorkspaceTemplateResult{}},
	}
	for _, test := range tests {
		t.Run(test.topic, func(t *testing.T) {
			event := magicevent.NewEventWithContext(test.topic, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), context.Background(), test.command)
			result := magicevent.NewResult(test.topic, workspaceevents.CommandDestination, "test")
			setWorkspaceMagicResult(result, event, test.data, nil)
			if result.Error() != nil {
				t.Fatalf("result error = %v", result.Error())
			}
			value, err := result.Get()
			if err != nil {
				t.Fatalf("get result: %v", err)
			}
			if reflect.TypeOf(value) != reflect.TypeOf(test.want) {
				t.Fatalf("result type = %T, want %T", value, test.want)
			}
		})
	}

	result := magicevent.NewResult(workspaceevents.TopicWorkspaceCommandGet, workspaceevents.CommandDestination, "test")
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), context.Background(), workspaceevents.GetWorkspaceCommand{WorkspaceID: workspaceID})
	setWorkspaceMagicResult(result, event, workspaceCommandResponse{}, nil)
	if result.Error() == nil {
		t.Fatal("missing item result must fail closed")
	}
}

func TestWorkspaceDefinitionCommandsEnforceScopeAcrossBackends(t *testing.T) {
	for _, backend := range []struct {
		name string
		new  func(*testing.T) (repository.GoalRepository, repository.DoDRepository)
	}{
		{
			name: "memory",
			new: func(*testing.T) (repository.GoalRepository, repository.DoDRepository) {
				return memoryRepo.NewGoalRepository(), memoryRepo.NewDoDRepository()
			},
		},
		{
			name: "file",
			new: func(t *testing.T) (repository.GoalRepository, repository.DoDRepository) {
				dir := t.TempDir()
				return fileRepo.NewGoalRepository(filepath.Join(dir, "goals.json")), fileRepo.NewDoDRepository(filepath.Join(dir, "dods.json"))
			},
		},
	} {
		t.Run(backend.name, func(t *testing.T) {
			ctx := context.Background()
			workspaceRepo := memoryRepo.NewWorkspaceRepository()
			goalRepo, dodRepo := backend.new(t)
			manager := newGoalDoDManager(workspaceRepo, goalRepo, dodRepo)
			biz := &Biz{manager: manager, workspaces: workspaceRepo, goals: goalRepo, dods: dodRepo}
			for _, workspaceID := range []common.WorkspaceID{"ws-scope-a", "ws-scope-b"} {
				if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: string(workspaceID), DeliverableDirectory: "/tmp/" + string(workspaceID), State: common.WorkspaceStateActive}); err != nil {
					t.Fatalf("seed workspace %s: %v", workspaceID, err)
				}
			}
			workspaceA := common.WorkspaceID("ws-scope-a")
			workspaceB := common.WorkspaceID("ws-scope-b")
			if err := manager.DefineGoal(ctx, workspaceevents.DefineGoalCommand{WorkspaceID: workspaceA, GoalID: "goal-scope", Text: "workspace A goal"}); err != nil {
				t.Fatalf("define goal: %v", err)
			}
			if err := manager.DefineDoD(ctx, workspaceevents.DefineDoDCommand{WorkspaceID: workspaceA, DoDID: "dod-scope", Items: []string{"workspace A done"}}); err != nil {
				t.Fatalf("define DoD: %v", err)
			}

			commands := []struct {
				topic string
				data  any
			}{
				{workspaceevents.TopicWorkspaceCommandGetGoal, workspaceevents.GetGoalCommand{WorkspaceID: workspaceB, GoalID: "goal-scope"}},
				{workspaceevents.TopicWorkspaceCommandUpdateGoal, workspaceevents.UpdateGoalCommand{WorkspaceID: workspaceB, GoalID: "goal-scope", Text: "cross-scope update"}},
				{workspaceevents.TopicWorkspaceCommandDeleteGoal, workspaceevents.DeleteGoalCommand{WorkspaceID: workspaceB, GoalID: "goal-scope"}},
				{workspaceevents.TopicWorkspaceCommandGetDoD, workspaceevents.GetDoDCommand{WorkspaceID: workspaceB, DoDID: "dod-scope"}},
				{workspaceevents.TopicWorkspaceCommandUpdateDoD, workspaceevents.UpdateDoDCommand{WorkspaceID: workspaceB, DoDID: "dod-scope", Items: []string{"cross-scope update"}}},
				{workspaceevents.TopicWorkspaceCommandDeleteDoD, workspaceevents.DeleteDoDCommand{WorkspaceID: workspaceB, DoDID: "dod-scope"}},
			}
			for _, command := range commands {
				event := magicevent.NewEventWithContext(command.topic, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command.data)
				if _, err := biz.executeCommand(event); !errors.Is(err, common.ErrNotFound) {
					t.Fatalf("command %s err = %v, want scoped not found", command.topic, err)
				}
			}
			unscoped := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetGoal, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.GetGoalCommand{GoalID: "goal-scope"})
			if _, err := biz.executeCommand(unscoped); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("unscoped get err = %v, want invalid argument", err)
			}
			mismatched := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGetGoal, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), common.ContextWithRuntimeWorkspaceID(ctx, workspaceA), workspaceevents.GetGoalCommand{WorkspaceID: workspaceB, GoalID: "goal-scope"})
			if _, err := biz.executeCommand(mismatched); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("context/command scope mismatch err = %v, want conflict", err)
			}

			goalResult, err := biz.executeCommand(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDefineGoal, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.DefineGoalCommand{WorkspaceID: workspaceB, GoalID: "goal-scope", Text: "workspace B goal"}))
			if err != nil || goalResult.Goal == nil || goalResult.Goal.WorkspaceID != workspaceB {
				t.Fatalf("define duplicate local goal in workspace B result=%+v err=%v", goalResult, err)
			}
			dodResult, err := biz.executeCommand(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandDefineDoD, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.DefineDoDCommand{WorkspaceID: workspaceB, DoDID: "dod-scope", Items: []string{"workspace B done"}}))
			if err != nil || dodResult.DoD == nil || dodResult.DoD.WorkspaceID != workspaceB {
				t.Fatalf("define duplicate local DoD in workspace B result=%+v err=%v", dodResult, err)
			}

			scopedA := common.ContextWithRuntimeWorkspaceID(ctx, workspaceA)
			goal, err := goalRepo.Get(scopedA, "goal-scope")
			if err != nil || goal.Text != "workspace A goal" {
				t.Fatalf("goal after cross-scope commands = %+v err=%v", goal, err)
			}
			dod, err := dodRepo.Get(scopedA, "dod-scope")
			if err != nil || len(dod.Items) != 1 || dod.Items[0] != "workspace A done" {
				t.Fatalf("DoD after cross-scope commands = %+v err=%v", dod, err)
			}
		})
	}
}
