package workspace

import (
	"context"
	"errors"
	"path/filepath"
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
			manager := NewGoalDoDManager(workspaceRepo, goalRepo, dodRepo)
			biz := &Biz{manager: manager, goals: goalRepo, dods: dodRepo}
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
