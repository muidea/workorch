package run

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestServiceCreateRunPersistsOwnerStateAndPublishesTypedNotification(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-create-run")
	runID := common.RunID("run-create-run")
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	observer := magicevent.NewSimpleObserverWithMatchID("/run/consumers/create-test", "/run/consumers/create-test", hub)
	received := make(chan runevents.RunStateChangedData, 1)
	observer.Subscribe(runevents.TopicRunCreated, func(event magicevent.Event, result magicevent.Result) {
		data, ok := event.Data().(runevents.RunStateChangedData)
		if ok {
			received <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunCreated) })

	service := newServiceWithPorts(runRepo, testWorkspaceRepositoryReader{WorkspaceRepository: workspaceRepo}, eventRunStateNotifier{hub: hub})
	command := runcontracts.CreateRunCommand{
		RunID: runID, WorkspaceID: workspaceID, GoalID: "goal-create-run", DoDID: "dod-create-run",
		DefinitionRevision: "definition-create-run", InitialStepKind: "llm/workspace_execution", Priority: 3,
	}
	if err := service.CreateRun(ctx, command); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	stored, err := runRepo.GetByWorkspaceID(ctx, workspaceID, runID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != common.RunStatePending || stored.DefinitionRevision != command.DefinitionRevision || stored.Priority != command.Priority {
		t.Fatalf("stored run=%+v", stored)
	}
	select {
	case data := <-received:
		if data.RunID != runID || data.WorkspaceID != workspaceID || data.State != common.RunStatePending {
			t.Fatalf("notification=%+v", data)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for RunCreated notification")
	}
}

func TestServiceEnsureRunIsIdempotentAndRejectsSemanticMismatch(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-ensure-run")
	runID := common.RunID("run-ensure-run")
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	service := newServiceWithPorts(runRepo, testWorkspaceRepositoryReader{WorkspaceRepository: workspaceRepo}, nil)
	command := runcontracts.CreateRunCommand{RunID: runID, WorkspaceID: workspaceID, GoalID: "goal-ensure", DoDID: "dod-ensure", DefinitionRevision: "definition-ensure"}
	if err := service.EnsureRun(ctx, command); err != nil {
		t.Fatalf("first EnsureRun: %v", err)
	}
	if err := service.EnsureRun(ctx, command); err != nil {
		t.Fatalf("second EnsureRun: %v", err)
	}
	command.Priority = 1
	if err := service.EnsureRun(ctx, command); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched EnsureRun err=%v, want conflict", err)
	}
}

func TestServiceCreateRunValidatesScopeAndWorkspace(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	service := newServiceWithPorts(memory.NewRunRepository(), testWorkspaceRepositoryReader{WorkspaceRepository: workspaceRepo}, nil)
	if err := service.CreateRun(ctx, runcontracts.CreateRunCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty CreateRun err=%v", err)
	}
	command := runcontracts.CreateRunCommand{RunID: "run-missing-workspace", WorkspaceID: "ws-missing"}
	if err := service.CreateRun(ctx, command); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing workspace err=%v", err)
	}
	scoped := common.ContextWithRuntimeWorkspaceID(ctx, "ws-other")
	if err := service.CreateRun(scoped, command); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("scope mismatch err=%v", err)
	}
}

func TestCreateRunPersistsPredecessorLineage(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-lineage-create")
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	service := newServiceWithPorts(runRepo, testWorkspaceRepositoryReader{WorkspaceRepository: workspaceRepo}, nil)
	if err := service.CreateRun(ctx, runcontracts.CreateRunCommand{
		RunID: "run-successor", WorkspaceID: workspaceID, GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "definition-1",
		Lineage: runmodel.RunLineage{PredecessorRunID: "run-pred", PredecessorResultID: "result-pred"},
	}); err != nil {
		t.Fatal(err)
	}
	item, err := runRepo.GetByWorkspaceID(ctx, workspaceID, "run-successor")
	if err != nil {
		t.Fatal(err)
	}
	if item.Lineage.PredecessorRunID != "run-pred" || item.Lineage.PredecessorResultID != "result-pred" || item.Lineage.SuccessorRunID != "" || item.Lineage.LineageState != "active" {
		t.Fatalf("lineage = %+v", item.Lineage)
	}
}

func TestLinkRunSuccessorCASIdempotentAndConflict(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-lineage-link")
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	service := newServiceWithPorts(runRepo, testWorkspaceRepositoryReader{WorkspaceRepository: workspaceRepo}, nil)
	if err := service.CreateRun(ctx, runcontracts.CreateRunCommand{RunID: "run-pred", WorkspaceID: workspaceID, GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "definition-1"}); err != nil {
		t.Fatal(err)
	}
	if err := service.CreateRun(ctx, runcontracts.CreateRunCommand{
		RunID: "run-succ", WorkspaceID: workspaceID, GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "definition-1",
		Lineage: runmodel.RunLineage{PredecessorRunID: "run-pred", PredecessorResultID: "result-pred"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.LinkRunSuccessor(ctx, runcontracts.LinkRunSuccessorCommand{
		WorkspaceID: workspaceID, PredecessorRunID: "run-pred", SuccessorRunID: "run-succ",
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.LinkRunSuccessor(ctx, runcontracts.LinkRunSuccessorCommand{
		WorkspaceID: workspaceID, PredecessorRunID: "run-pred", SuccessorRunID: "run-succ",
	}); err != nil {
		t.Fatalf("idempotent link: %v", err)
	}
	if err := service.CreateRun(ctx, runcontracts.CreateRunCommand{
		RunID: "run-other", WorkspaceID: workspaceID, GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "definition-1",
		Lineage: runmodel.RunLineage{PredecessorRunID: "run-pred"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.LinkRunSuccessor(ctx, runcontracts.LinkRunSuccessorCommand{
		WorkspaceID: workspaceID, PredecessorRunID: "run-pred", SuccessorRunID: "run-other",
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	pred, err := runRepo.GetByWorkspaceID(ctx, workspaceID, "run-pred")
	if err != nil {
		t.Fatal(err)
	}
	if pred.Lineage.SuccessorRunID != "run-succ" {
		t.Fatalf("predecessor successor = %q", pred.Lineage.SuccessorRunID)
	}
}
