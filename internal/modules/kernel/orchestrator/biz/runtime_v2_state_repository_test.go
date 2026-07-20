package orchestrator

// Owner Biz persistence tests.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeV2StateRepositoryPersistsAndLoadsThroughRunOwner(t *testing.T) {
	ctx := context.Background()
	if err := runtimeV2Bootstrap().Validate(); err != nil {
		t.Fatalf("test runtime bootstrap must validate: %v", err)
	}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	module := runmodule.New()
	if err := module.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	repository := runtimeV2StateRepository{hub: hub}
	aggregate := runtimeV2Aggregate(t, 1)
	if current, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil || current.RuntimeBootstrap == nil || !runtimecontract.SameRuntimeBootstrap(*current.RuntimeBootstrap, runtimeV2Bootstrap()) {
		t.Fatalf("persisted bootstrap mismatch current=%+v err=%v", current, err)
	}
	if err := repository.Save(ctx, aggregate, 0); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := repository.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.StateRevision != 1 || loaded.Decision.AuthoritySnapshot.ID != aggregate.Decision.AuthoritySnapshot.ID ||
		!runtimecontract.SameRuntimeDefinitionContext(loaded.Definition, aggregate.Definition) {
		t.Fatalf("loaded aggregate = %+v", loaded)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RuntimeContractVersion != runtimecontract.RuntimeContractVersionV2 || persisted.RuntimeState == nil {
		t.Fatalf("persisted runtime state = %+v", persisted)
	}
	if !runtimecontract.SameRuntimeDefinitionContext(persisted.RuntimeState.Definition, runtimeV2Definition()) {
		t.Fatalf("persisted runtime definition = %+v", persisted.RuntimeState.Definition)
	}
}

func TestRuntimeV2InitializesFromPersistedRunBootstrap(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStatePending,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := biz.InitializeRuntimeV2FromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.Facts.Focus.ID != "focus-v2" || aggregate.Definition.GoalID != "goal-v2" || aggregate.StateRevision != 1 {
		t.Fatalf("initialized aggregate = %+v", aggregate)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeContractVersion != runtimecontract.RuntimeContractVersionV2 || persisted.RuntimeState == nil || persisted.RuntimeBootstrap == nil {
		t.Fatalf("persisted run = %+v err=%v", persisted, err)
	}
}

func TestRuntimeV2StateRepositoryRejectsStaleRevision(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	aggregate := runtimeV2Aggregate(t, 2)
	record := runtimeStateRecordFromAggregate(aggregate)
	run := runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())
	run.RuntimeContractVersion = runtimecontract.RuntimeContractVersionV2
	run.RuntimeState = &record
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	module := runmodule.New()
	if err := module.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	aggregate.StateRevision = 3
	err := (runtimeV2StateRepository{hub: hub}).Save(ctx, aggregate, 1)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Save stale revision err = %v, want conflict", err)
	}
}

func TestRuntimeV2MainlinePreparesLocalPrerequisiteThroughPersistedController(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: t.TempDir(),
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	facts := runtimeV2Aggregate(t, 1).Facts
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), []runtimev2.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindValidationScratch}})
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	prepared, err := module.prepareRuntimeV2Prerequisites(ctx, initialized)
	if err != nil {
		t.Fatalf("prepareRuntimeV2Prerequisites: %v", err)
	}
	if prepared.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || prepared.Facts.Resources.ValidationScratch == nil {
		t.Fatalf("prepared aggregate = %+v", prepared)
	}
	loaded, err := biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.StateRevision != prepared.StateRevision {
		t.Fatalf("persisted prepared aggregate = %+v err=%v", loaded, err)
	}
}

func TestRuntimeV2MainlineRequestsAndSealsTypedCandidate(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeV2ReceiptTestOwner(t, hub)

	var requestedDefinition runtimecontract.RuntimeDefinitionContext
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		requestedDefinition = command.DefinitionContext
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
				Parameters: runtimeV2FileEditParameters("target.txt"),
			}}}}},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: "/tmp/workspace"}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.ExecuteFileEditOperationCommand)
		receipt := orchestratorevents.NewRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusAccepted, command.TargetRefs, runtimecontract.RuntimeOperationEvidence{}, nil, nil)
		committed, err := orchestratorevents.CommitPersistedRuntimeOperationReceipt(event.Context(), hub, runtimecontract.RuntimeModuleFile, command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
		if err != nil {
			t.Errorf("commit owner receipt: %v", err)
			return
		}
		result.Set(orchestratorevents.ExecuteFileEditOperationResult{
			Receipt: committed,
			EventID: "event-" + command.OperationID, OperationID: command.OperationID,
			EvidenceKind: command.EvidenceKind, Status: runtimecontract.RuntimeOperationExecutionStatusAccepted,
			SourceRef: "workorch://runtime-operations/" + command.DispatchID + "/" + command.OperationID,
		}, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation) })

	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	sealed, err := module.requestAndSealRuntimeV2Candidate(ctx, initialized, "stable-prefix-v2", 0)
	if err != nil {
		t.Fatalf("requestAndSealRuntimeV2Candidate: %v", err)
	}
	if sealed.ActiveDispatch == nil || sealed.ActiveDispatch.Transaction != initialized.Decision.AuthorityTransaction() {
		t.Fatalf("sealed aggregate = %+v", sealed)
	}
	if !runtimecontract.SameRuntimeDefinitionContext(requestedDefinition, runtimeV2Definition()) {
		t.Fatalf("candidate request definition = %+v", requestedDefinition)
	}
	completed, err := module.executeRuntimeV2ActiveDispatch(ctx, sealed)
	if err != nil {
		t.Fatalf("executeRuntimeV2ActiveDispatch: %v", err)
	}
	if completed.ActiveDispatch != nil || completed.Decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("completed aggregate = %+v", completed)
	}
	loaded, err := biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.ActiveDispatch != nil || loaded.Decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("persisted completed aggregate = %+v err=%v", loaded, err)
	}
}

func TestRuntimeV2OperationExecutorRoutesTypedCommandToCapabilityOwner(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: "/tmp/workspace", InternalDirectory: "/tmp/workspace-internal",
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	var captured orchestratorevents.ExecuteFileEditOperationCommand
	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation, func(event magicevent.Event, result magicevent.Result) {
		captured = event.Data().(orchestratorevents.ExecuteFileEditOperationCommand)
		result.Set(orchestratorevents.ExecuteFileEditOperationResult{
			EventID: "event-write-1", OperationID: "write-1", EvidenceKind: runtimecontract.EvidenceKindStateChange,
			Status: runtimecontract.RuntimeOperationExecutionStatusAccepted, SourceRef: "workorch://runtime-operations/dispatch-1/write-1",
		}, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation) })

	response, err := (eventRuntimeV2OperationExecutor{hub: hub}).ExecuteRuntimeOperation(ctx, runtimev2.RuntimeOperationExecutionRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: "dispatch-1",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-1"},
		Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
		},
		EvidenceKind:     runtimecontract.EvidenceKindStateChange,
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{ID: "policy-1", AppliesTo: runtimecontract.StepKindFileEdit}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || captured.WorkspaceDeliverableDirectory != "/tmp/workspace" || captured.OperationID != "write-1" || captured.Data.Path != "target.txt" ||
		len(captured.EvidencePolicies) != 1 || captured.EvidencePolicies[0].ID != "policy-1" {
		t.Fatalf("response=%+v command=%+v", response, captured)
	}
}

func TestRuntimeV2MainlineRetriesSameCandidateContractOnce(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	var attempts []orchestratorevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		attempts = append(attempts, command)
		if command.CandidateAttempt == 0 {
			failure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassSchema, Reason: "invalid schema"}
			result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Failure: &failure}, nil)
			return
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
				TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
			}}}}},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	sealed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, "stable-prefix-v2")
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ActiveDispatch == nil || len(attempts) != 2 || attempts[0].CandidateAttempt != 0 || attempts[1].CandidateAttempt != 1 ||
		attempts[0].Transaction != attempts[1].Transaction || attempts[0].StablePrefixHash != attempts[1].StablePrefixHash {
		t.Fatalf("attempts=%+v sealed=%+v", attempts, sealed)
	}
	if len(sealed.CandidateAttempts) != 2 || sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed || sealed.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
		t.Fatalf("persisted attempts = %+v", sealed.CandidateAttempts)
	}
}

func TestRuntimeV2MainlineRecordsInfeasibleCandidateBeforeDispatchSeal(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
		failure := &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "operation-admissibility/file/edit/operation_target_not_regular", Message: "target is a directory", TargetRefs: []string{"."}}
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: command.Operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
	})
	t.Cleanup(func() {
		probeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	})

	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		candidate := runtimecontract.RuntimeCandidate{
			StateChange: &runtimecontract.StateChangeCandidate{
				Operations: []runtimecontract.StateChangingOperation{{
					Operation: runtimecontract.RuntimeOperation{
						ID: "directory-edit", Kind: runtimecontract.StepKindFileEdit,
						CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"."},
						Parameters: runtimeV2FileEditParameters("."),
					},
				}},
			},
		}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: candidate}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	facts := runtimeV2Aggregate(t, 1).Facts
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"."}, ObservedRevision: "test-revision",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{
		runtimeV2OperationAdmissibilityPrerequisite(inventory),
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{
		runtimeV2FileAdmissibilityObservation(inventory),
	}
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	failed, err := module.requestAndSealRuntimeV2Candidate(ctx, initialized, "stable-admissibility-prefix", 0)
	if err != nil {
		t.Fatal(err)
	}
	if failed.ActiveDispatch != nil || len(failed.CandidateAttempts) != 1 || failed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed || failed.CandidateAttempts[0].Failure == nil || failed.CandidateAttempts[0].Failure.Class != runtimecontract.RuntimeCandidateFailureClassInfeasible || failed.CandidateAttempts[0].Failure.Code != "operation_target_not_regular" {
		t.Fatalf("infeasible candidate was sealed or not persisted as an admission failure: %+v", failed)
	}
	if len(failed.OperationReceipts) != 0 || len(failed.Facts.Evidence.Accepted) != 0 || len(failed.Facts.Evidence.Rejected) != 0 {
		t.Fatalf("infeasible candidate created execution state: %+v", failed)
	}
}

func TestRuntimeV2CandidateAdmissionSkipsOwnerProbeForAssessment(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(1)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := newTestBiz(NewRuntimeOrchestrator(nil, nil, nil, nil), hub, nil)
	candidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "assessment-only"}}
	failure, stale, err := module.probeRuntimeV2CandidateFeasibility(ctx, runtimev2.RuntimeAggregate{}, t.TempDir(), candidate, 0)
	if err != nil || failure != nil || stale != nil {
		t.Fatalf("assessment candidate should not enter owner operation probe: failure=%+v stale=%+v err=%v", failure, stale, err)
	}
}

func TestRuntimeV2MainlineRetriesCorrectedInfeasibleCandidateBeforeSeal(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	probes := 0
	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		probes++
		command := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
		response := orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: command.Operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: probes != 1}
		if !response.Feasible {
			response.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "operation-admissibility/file/edit/operation_target_not_regular", Message: "target is a directory", TargetRefs: []string{"."}}
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() {
		probeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	})

	var requests []orchestratorevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		requests = append(requests, command)
		target := "target.txt"
		if command.CandidateAttempt == 0 {
			target = "."
		} else if command.PreviousFailure == nil || command.PreviousFailure.Class != runtimecontract.RuntimeCandidateFailureClassInfeasible || command.PreviousFailure.Code != "operation_target_not_regular" {
			t.Fatalf("retry did not receive persisted infeasibility: %+v", command.PreviousFailure)
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "edit-" + target, Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: runtimeV2FileEditParameters(target),
		}}}}}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: candidate}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	facts := runtimeV2Aggregate(t, 1).Facts
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{".", "target.txt"}, ObservedRevision: "test-revision"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{
		runtimeV2OperationAdmissibilityPrerequisite(inventory),
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{
		runtimeV2FileAdmissibilityObservation(inventory),
	}
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, "stable-admissibility-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ActiveDispatch == nil || probes != 2 || len(requests) != 2 || len(sealed.CandidateAttempts) != 2 ||
		sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed || sealed.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
		t.Fatalf("corrected candidate was not sealed after one infeasible retry: probes=%d requests=%+v aggregate=%+v", probes, requests, sealed)
	}
	if len(sealed.OperationReceipts) != 0 || len(sealed.Facts.Evidence.Accepted) != 0 || len(sealed.Facts.Evidence.Rejected) != 0 {
		t.Fatalf("candidate admission created execution state: %+v", sealed)
	}
}

func TestRuntimeV2MainlineExhaustsTwoInfeasibleCandidatesWithoutDispatch(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: command.Operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "operation-admissibility/file/edit/operation_target_not_regular", Message: "target is a directory", TargetRefs: []string{"."}}}, nil)
	})
	t.Cleanup(func() {
		probeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	})

	requests := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requests++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		target := "."
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "directory-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: runtimeV2FileEditParameters(target),
		}}}}}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Candidate: candidate}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	facts := runtimeV2Aggregate(t, 1).Facts
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"."}, ObservedRevision: "test-revision"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, "stable-admissibility-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || terminal.ActiveDispatch != nil || terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.Facts.Failure == nil || terminal.Facts.Failure.Class != runtimecontract.FailureClassCandidateExhausted || terminal.Facts.Failure.Code != "candidate_retry_exhausted" {
		t.Fatalf("two infeasible candidates did not terminalize as bounded exhaustion: requests=%d aggregate=%+v", requests, terminal)
	}
	if len(terminal.OperationReceipts) != 0 || len(terminal.Facts.Evidence.Accepted) != 0 || len(terminal.Facts.Evidence.Rejected) != 0 {
		t.Fatalf("infeasible candidates created execution state: %+v", terminal)
	}
}

func TestRuntimeV2MainlineRestartsFromPersistedCandidateProposalWithoutProviderReplay(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		providerCalls++
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	_, err := biz.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "persisted-proposal-prefix"
	pending, err := biz.BeginRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, prefix)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "persisted-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	persisted, err := biz.RecordRuntimeV2CandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.StateRevision <= pending.StateRevision || len(persisted.CandidateAttempts) != 1 || persisted.CandidateAttempts[0].Proposal == nil {
		t.Fatalf("candidate proposal was not persisted: pending=%+v persisted=%+v", pending, persisted)
	}
	restarted, err := biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	sealed, err := module.requestAndSealRuntimeV2Candidate(ctx, restarted, prefix, 0)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || sealed.ActiveDispatch == nil || sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || sealed.CandidateAttempts[0].Proposal != nil {
		t.Fatalf("restart replayed provider or did not seal persisted proposal: calls=%d aggregate=%+v", providerCalls, sealed)
	}
}

func TestRuntimeV2MainlineRestartSkipsPersistedFeasibilityProbe(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	providerCalls, probeCalls := 0, 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, func(_ magicevent.Event, _ magicevent.Result) { probeCalls++ })
	t.Cleanup(func() {
		probeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	})

	facts := runtimeV2Aggregate(t, 1).Facts
	inventory := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "test-revision"}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	if _, err := biz.InitializeRuntimeV2(ctx, facts, runtimeV2Definition(), nil); err != nil {
		t.Fatal(err)
	}
	const prefix = "persisted-feasibility-prefix"
	if _, err := biz.BeginRuntimeV2CandidateAttempt(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "persisted-feasible-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileEditParameters("target.txt"),
	}}}}}
	if _, err := biz.RecordRuntimeV2CandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := biz.RecordRuntimeV2CandidateFeasibility(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{OperationID: "persisted-feasible-edit", AdmissibilitySnapshotID: inventory.SnapshotID, Feasible: true}); err != nil {
		t.Fatal(err)
	}
	restarted, err := biz.LoadRuntimeV2(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeV2Candidate(ctx, restarted, prefix, 0)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || probeCalls != 0 || sealed.ActiveDispatch == nil || sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || len(sealed.CandidateAttempts[0].Feasibilities) != 0 {
		t.Fatalf("restart replayed provider/probe or retained unsealed feasibility state: provider=%d probes=%d aggregate=%+v", providerCalls, probeCalls, sealed)
	}
}

func runtimeV2FileEditParameters(path string) []runtimecontract.OperationParameter {
	oldText := "old"
	newText := "new"
	return []runtimecontract.OperationParameter{
		{Name: "path", String: &path},
		{Name: "oldText", String: &oldText},
		{Name: "newText", String: &newText},
	}
}

func TestRuntimeV2MainlineStopsAfterSecondCandidateFailure(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeV2RunForTest(common.RunStateActive, runtimeV2Definition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	requests := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requests++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		failure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassProvider, Reason: "provider unavailable"}
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, Failure: &failure}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntimeV2(ctx, runtimeV2Aggregate(t, 1).Facts, runtimeV2Definition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, "stable-prefix-v2")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || terminal.Decision.Action != runtimecontract.RuntimeActionFail || terminal.ActiveDispatch != nil {
		t.Fatalf("requests=%d terminal=%+v", requests, terminal)
	}
}

func TestNextRuntimeV2CandidateAttemptResumesPersistedPendingOrFailedState(t *testing.T) {
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-1", FocusID: "focus-v2", Action: runtimecontract.RuntimeActionProduceStateChange, RequiredOutputHash: "output-v2", DefinitionRevision: "definition-v2", EvidenceRevision: "evidence-v2"}
	base := runtimev2.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-v2", RunID: "run-v2"}, Decision: runtimecontract.RuntimeDecision{AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "snapshot-1", FocusID: "focus-v2", Action: runtimecontract.RuntimeActionProduceStateChange, RequiredOutputHash: "output-v2", DefinitionRevision: "definition-v2", EvidenceRevision: "evidence-v2"}}}
	contractID := runtimecontract.StableAuthorityContractID(base.Facts.WorkspaceID, base.Facts.RunID, transaction)
	tests := []struct {
		name     string
		attempts []runtimecontract.RuntimeCandidateAttemptRecord
		want     int
		ok       bool
	}{
		{name: "new", want: 0, ok: true},
		{name: "pending zero", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStatePending}}, want: 0, ok: true},
		{name: "failed zero", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateFailed}}, want: 1, ok: true},
		{name: "pending one", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateFailed}, {AuthorityContractID: contractID, Transaction: transaction, Attempt: 1, State: runtimecontract.RuntimeCandidateAttemptStatePending}}, want: 1, ok: true},
		{name: "accepted", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateAccepted}}, ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			aggregate := base
			aggregate.CandidateAttempts = tc.attempts
			got, ok := nextRuntimeV2CandidateAttempt(aggregate)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("next attempt = %d/%t, want %d/%t", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestNextRuntimeV2CandidateAttemptRejectsResealRunaway(t *testing.T) {
	aggregate := runtimeV2Aggregate(t, 100)
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction)
	for idx := 0; idx < 3; idx++ {
		resealed := transaction
		resealed.AuthoritySnapshotID = fmt.Sprintf("resealed-snapshot-%d", idx)
		resealed.Attempt = idx
		aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
			AuthorityContractID: contractID, Transaction: resealed, Attempt: 0, StablePrefixHash: "stable-prefix-v2",
			State:   runtimecontract.RuntimeCandidateAttemptStateFailed,
			Failure: &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Reason: "invalid candidate"},
		})
	}
	if attempt, ok := nextRuntimeV2CandidateAttempt(aggregate); ok {
		t.Fatalf("reseal runaway offered candidate attempt %d", attempt)
	}
	if !runtimeV2CandidateContractExhausted(aggregate) {
		t.Fatal("reseal runaway was not recognized as exhausted")
	}
}

func runtimeV2Aggregate(t *testing.T, revision uint64) runtimev2.RuntimeAggregate {
	t.Helper()
	policy := runtimeV2TestAcceptanceSlicePolicy()
	facts := runtimecontract.RuntimeFacts{
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{
				OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
				AcceptanceSlicePolicy:     &policy,
			},
		},
		WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: "definition-v2", EvidenceRevision: "evidence-v2-00000000000000000000",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-v2", SliceID: "slice-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"target.txt"},
		},
		Coverage:          []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-v2", SliceID: "slice-v2", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities:      runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{runtimecontract.StepKindFileEdit}},
		ConvergencePolicy: runtimeV2TestConvergencePolicy(),
		RecoveryPolicy:    runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
	decision, err := runtimev2.ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	return runtimev2.RuntimeAggregate{StateRevision: revision, Definition: runtimeV2Definition(), Facts: facts, Decision: decision}
}

func runtimeV2FileAdmissibilityObservation(inventory runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisiteObservation {
	inventory = inventory.Normalized()
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: inventory.PrerequisiteID,
		Kind:           runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID:  inventory.OwnerModuleID,
		ContentHash:    inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain:        runtimecontract.RuntimeResourceWorkspaceTree,
			OwnerModuleID: inventory.OwnerModuleID,
			SubjectRef:    "workspace_deliverable",
			Revision:      inventory.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func runtimeV2OperationAdmissibilityPrerequisite(inventory runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisite {
	inventory = inventory.Normalized()
	return runtimecontract.RuntimePrerequisite{
		ID:                   inventory.PrerequisiteID,
		Kind:                 runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID:        inventory.OwnerModuleID,
		RequiredCapabilities: append([]string(nil), inventory.OperationKinds...),
	}
}

func runtimeV2Definition() runtimecontract.RuntimeDefinitionContext {
	return runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-v2", GoalID: "goal-v2", GoalDefinition: "完成目标",
		DoDID: "dod-v2", DoDDefinition: "产生可验证交付物", CapabilityContract: []string{runtimecontract.StepKindFileEdit},
	}
}

func runtimeV2Bootstrap() runtimecontract.RuntimeBootstrap {
	policy := runtimeV2TestAcceptanceSlicePolicy()
	return runtimecontract.RuntimeBootstrap{
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		Definition:    runtimeV2Definition(),
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{
				OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
				AcceptanceSlicePolicy:     &policy,
			},
		},
		Focuses: []runtimecontract.CoverageFocus{{
			ID: "focus-v2", SliceID: "slice-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"target.txt"},
		}},
		Slices:        []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-v2", Title: "Test", Scope: "test", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed}},
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi-v2", SliceID: "slice-v2"}},
		Requirements: []runtimecontract.RuntimeRequirement{{
			Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit},
		}},
		ConvergencePolicy: runtimeV2TestConvergencePolicy(),
		RecoveryPolicy:    runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
}

func runtimeV2TestAcceptanceSlicePolicy() runtimecontract.RuntimeAcceptanceSlicePolicy {
	return runtimecontract.RuntimeAcceptanceSlicePolicy{
		SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2,
		MaxWorkItems:  4, MaxFocuses: 16, MaxCandidateContextItems: 4,
		Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{ID: "test", Title: "Test", Scope: "test", Priority: 1, MaxWorkItems: 4, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true}}},
	}
}

func runtimeV2TestConvergencePolicy() runtimecontract.RuntimeConvergencePolicy {
	policy := runtimecontract.DefaultRuntimeConvergencePolicy()
	policy.MaxAcceptanceSliceWorkItems = 4
	policy.MaxFocuses = 16
	policy.MaxCandidateContextItems = 4
	return policy
}

func runtimeV2RunForTest(state common.RunState, definition runtimecontract.RuntimeDefinitionContext) *runmodel.Run {
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition = definition
	return &runmodel.Run{
		ID:                 "run-v2",
		WorkspaceID:        "ws-v2",
		GoalID:             definition.GoalID,
		DoDID:              definition.DoDID,
		DefinitionRevision: definition.Revision,
		RuntimeBootstrap:   &bootstrap,
		State:              state,
	}
}
