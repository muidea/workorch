package orchestrator

import (
	"context"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDriveRuntimeV2CompletesTypedPathAndIsRestartIdempotent(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
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

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspacemodel.Workspace{
			ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir(),
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
		response, err := orchestratorevents.ResolveRuntimeCapabilities(command, runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileEdit)
		if err != nil {
			t.Errorf("resolve capabilities: %v", err)
			return
		}
		result.Set(response, nil)
	})
	operationCount := 0
	fileObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation, func(event magicevent.Event, result magicevent.Result) {
		operationCount++
		command := event.Data().(orchestratorevents.ExecuteFileEditOperationCommand)
		result.Set(orchestratorevents.ExecuteFileEditOperationResult{
			EventID: "event-" + command.OperationID, OperationID: command.OperationID,
			EvidenceKind: command.EvidenceKind, Status: runtimecontract.RuntimeOperationExecutionStatusAccepted,
			SourceRef: "workorch://runtime-operations/" + command.DispatchID + "/" + command.OperationID,
			Evidence:  runtimecontract.RuntimeOperationEvidence{Summary: "状态变更已完成"},
		}, nil)
	})
	t.Cleanup(func() {
		fileObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteFileEditOperation)
		fileObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	})

	candidateCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
				ID: "write-1", Kind: runtimecontract.StepKindFileEdit,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
				Parameters: runtimeV2FileEditParameters("target.txt"),
			}}}}},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	module := newRuntimeV2DriveTestModule(t, hub)
	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)

	command := orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"}
	completed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal || completed.Action != runtimecontract.RuntimeActionComplete || completed.Terminal == nil {
		t.Fatalf("completed drive result = %+v", completed)
	}
	if candidateCount != 1 || operationCount != 1 {
		t.Fatalf("candidateCount=%d operationCount=%d, want 1/1", candidateCount, operationCount)
	}

	replayed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.StateRevision != completed.StateRevision || replayed.AuthoritySnapshotID != completed.AuthoritySnapshotID || candidateCount != 1 || operationCount != 1 {
		t.Fatalf("replayed=%+v completed=%+v counts=%d/%d", replayed, completed, candidateCount, operationCount)
	}

	driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2)
	restarted := newRuntimeV2DriveTestModule(t, hub)
	driveObserver = magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, restarted.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	recovered, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.StateRevision != completed.StateRevision || recovered.AuthoritySnapshotID != completed.AuthoritySnapshotID || candidateCount != 1 || operationCount != 1 {
		t.Fatalf("recovered=%+v completed=%+v counts=%d/%d", recovered, completed, candidateCount, operationCount)
	}
}

func TestDriveRuntimeV2ReturnsPersistedGovernedWait(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeV2Bootstrap()
	bootstrap.Definition.CapabilityContract = append(bootstrap.Definition.CapabilityContract, runtimecontract.RuntimeBlockKindDecisionRequest)
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
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

	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionmodule.Dependencies{
		Tickets: memory.NewDecisionTicketRepository(), Resolutions: memory.NewDecisionResolutionRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := decisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = decisionOwner.Stop(ctx) })

	module := newRuntimeV2DriveTestModule(t, hub)
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceDependency, Signature: "external-approval", Message: "需要外部裁决",
	}
	if _, err := module.bizPtr.InitializeRuntimeV2(ctx, facts, bootstrap.Definition, nil); err != nil {
		t.Fatal(err)
	}

	requestCount := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requestCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate: runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
				Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决",
			}}},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })
	command := orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"}
	waiting, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Disposition != orchestratorevents.RuntimeV2DriveDispositionGovernedWait || waiting.Action != runtimecontract.RuntimeActionRequestGovernance || waiting.BlockingTicketID == "" || requestCount != 1 {
		t.Fatalf("waiting=%+v requestCount=%d", waiting, requestCount)
	}
	replayed, err := DriveRuntimeV2(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.StateRevision != waiting.StateRevision || replayed.BlockingTicketID != waiting.BlockingTicketID || requestCount != 1 {
		t.Fatalf("replayed=%+v waiting=%+v requestCount=%d", replayed, waiting, requestCount)
	}
}

func TestDriveRuntimeV2CandidateFailureStopsAfterSameContractRetry(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	bootstrap := runtimeV2Bootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, State: common.RunStateActive,
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

	module := newRuntimeV2DriveTestModule(t, hub)
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	if _, err := module.bizPtr.InitializeRuntimeV2(ctx, facts, bootstrap.Definition, nil); err != nil {
		t.Fatal(err)
	}

	requestCount := 0
	var requests []orchestratorevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requestCount++
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		requests = append(requests, command)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Failure: &runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassProvider, Reason: "provider unavailable",
			},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })

	driveObserver := magicevent.NewSimpleObserver(ModuleID, hub)
	driveObserver.Subscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2, module.handleRuntimeV2Drive)
	t.Cleanup(func() { driveObserver.Unsubscribe(orchestratorevents.TopicOrchestratorDriveRuntimeV2) })

	completed, err := DriveRuntimeV2(ctx, hub, orchestratorevents.DriveRuntimeV2Command{WorkspaceID: "ws-v2", RunID: "run-v2"})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 || completed.Disposition != orchestratorevents.RuntimeV2DriveDispositionTerminal ||
		completed.Action != runtimecontract.RuntimeActionFail || completed.Terminal == nil || completed.Terminal.Code != "candidate_retry_exhausted" {
		t.Fatalf("completed=%+v requestCount=%d, want bounded terminal after two requests", completed, requestCount)
	}
	if len(requests) != 2 || requests[0].PreviousFailure != nil || requests[1].PreviousFailure == nil ||
		requests[1].PreviousFailure.Class != runtimecontract.RuntimeCandidateFailureClassProvider || requests[1].PreviousFailure.Reason != "provider unavailable" {
		t.Fatalf("candidate retry did not carry the persisted first failure: %+v", requests)
	}
}

func newRuntimeV2DriveTestModule(t *testing.T, hub magicevent.Hub) *Module {
	t.Helper()
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	return &Module{bizPtr: biz, rawEventHub: hub}
}

func TestRuntimeV2EnsureCandidateFeasibleBlocksProviderWithoutKinds(t *testing.T) {
	aggregate := runtimev2.RuntimeAggregate{
		Decision: runtimecontract.RuntimeDecision{
			Action: runtimecontract.RuntimeActionProduceValidation,
			RequiredOutput: runtimecontract.RequiredOutputContract{
				Kind:                runtimecontract.RequiredOutputKindValidation,
				AcceptedKinds:       []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			},
		},
		Facts: runtimecontract.RuntimeFacts{
			Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.StepKindShellCommand}},
		},
	}
	if err := runtimeV2EnsureCandidateFeasible(aggregate); err == nil || !strings.Contains(err.Error(), "no feasible operation kinds") {
		t.Fatalf("expected infeasible validation, got %v", err)
	}
	aggregate.Facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	if err := runtimeV2EnsureCandidateFeasible(aggregate); err != nil {
		t.Fatalf("managed validation should be feasible: %v", err)
	}
}

func TestRuntimeV2DecisionPersistsFeasibilityWithoutCallingProvider(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: "definition-v2", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:        runtimecontract.CoverageFocus{ID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.StepKindShellCommand}},
	}
	decision, err := runtimev2.ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	repository := &runtimeV2DriveRepository{aggregate: runtimev2.RuntimeAggregate{StateRevision: 1, Definition: runtimeV2Definition(), Facts: facts.Normalized(), Decision: decision}}
	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(repository); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	providerCalls := 0
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	module := &Module{bizPtr: biz, rawEventHub: hub}
	next, err := module.driveRuntimeV2Decision(context.Background(), repository.aggregate, "unused-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || next.Facts.Feasibility == nil || next.Decision.Action != runtimecontract.RuntimeActionFail || next.Decision.Terminal == nil || next.Decision.Terminal.Code != "capability_unavailable" {
		t.Fatalf("feasibility transition = %+v providerCalls=%d", next, providerCalls)
	}
}

type runtimeV2DriveRepository struct {
	aggregate runtimev2.RuntimeAggregate
}

func (r *runtimeV2DriveRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	if r == nil || r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return runtimev2.RuntimeAggregate{}, common.ErrNotFound
	}
	return r.aggregate, nil
}

func (r *runtimeV2DriveRepository) Save(_ context.Context, aggregate runtimev2.RuntimeAggregate, expectedRevision uint64) error {
	if r == nil || r.aggregate.StateRevision != expectedRevision {
		return common.ErrConflict
	}
	r.aggregate = aggregate
	return nil
}
