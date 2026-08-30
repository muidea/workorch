package workspace

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspacePrivilegedIngressRejectsForgedSourcesWithoutOwnerMutation(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	workspaces := memoryrepo.NewWorkspaceRepository()
	templates := memoryrepo.NewWorkspaceTemplateRepository()
	owner, err := New(hub, nil, Dependencies{
		Workspaces: workspaces, DeletionMaterialCleaner: workspaces,
		Goals: memoryrepo.NewGoalRepository(), DoDs: memoryrepo.NewDoDRepository(), Templates: templates,
		EventHub: hub,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "workspace-source", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}

	t.Run("system defaults", func(t *testing.T) {
		event := magicevent.NewEventWithContext(
			workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults,
			"application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
			workspaceevents.EnsureWorkspaceTemplateDefaultsCommand{},
		)
		result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
		owner.handleSystemDefaultsCommand(event, result)
		if !errors.Is(result.Error(), common.ErrInvalidArgument) {
			t.Fatalf("forged defaults source error = %v, want invalid argument", result.Error())
		}
		items, listErr := templates.List(ctx)
		if listErr != nil || len(items) != 0 {
			t.Fatalf("forged defaults source seeded templates: len=%d err=%v", len(items), listErr)
		}
	})

	t.Run("definition admission", func(t *testing.T) {
		command := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: testAdmissionCandidate()}
		command.Candidate.WorkspaceID = "workspace-source"
		command.Candidate = command.Candidate.WithHash()
		event := magicevent.NewEventWithContext(
			workspaceevents.TopicWorkspaceDefinitionAdmissionIntent,
			"kernel/orchestrator", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command,
		)
		result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
		owner.handleDefinitionAdmissionCommand(event, result)
		if !errors.Is(result.Error(), common.ErrInvalidArgument) {
			t.Fatalf("forged definition source error = %v, want invalid argument", result.Error())
		}
		stored, readErr := workspaces.Get(ctx, "workspace-source")
		if readErr != nil || len(stored.DefinitionAdmissionCandidates) != 0 || stored.StartupInput != nil || stored.ActivationAdmission != nil {
			t.Fatalf("forged definition source mutated Workspace: value=%+v err=%v", stored, readErr)
		}
	})

	t.Run("execution intent", func(t *testing.T) {
		event := magicevent.NewEventWithContext(
			workspaceevents.TopicWorkspaceCommandExecutionIntent,
			"kernel/orchestrator", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
			workspaceevents.WorkspaceExecutionIntentCommand{
				WorkspaceID: "workspace-source", RequestID: "resume-source",
				Kind: runtimecontract.RuntimeExecutionIntentResume,
			},
		)
		result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
		owner.handleExecutionIntentCommand(event, result)
		if !errors.Is(result.Error(), common.ErrInvalidArgument) {
			t.Fatalf("forged execution source error = %v, want invalid argument", result.Error())
		}
		stored, readErr := workspaces.Get(ctx, "workspace-source")
		if readErr != nil || stored.ActiveRunID != "" || stored.Revision != 1 {
			t.Fatalf("forged execution source mutated Workspace: value=%+v err=%v", stored, readErr)
		}
	})
}

func TestWorkspaceTemplateDefaultsIngressAcceptsOnlySystemOwner(t *testing.T) {
	ctx := context.Background()
	templates := memoryrepo.NewWorkspaceTemplateRepository()
	owner := &Biz{templates: templates}
	event := magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults,
		workspaceTemplateDefaultsCommandSource, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceevents.EnsureWorkspaceTemplateDefaultsCommand{},
	)
	result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	owner.handleSystemDefaultsCommand(event, result)
	if result.Error() != nil {
		t.Fatalf("system defaults error = %v", result.Error())
	}
	items, err := templates.List(ctx)
	if err != nil || len(items) == 0 {
		t.Fatalf("system defaults templates len=%d err=%v", len(items), err)
	}
}
