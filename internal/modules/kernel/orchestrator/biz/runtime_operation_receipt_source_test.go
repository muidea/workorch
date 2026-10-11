package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeReceiptSourceGuardRepository struct {
	aggregate runtime.RuntimeAggregate
	loads     int
	saves     int
}

func (r *runtimeReceiptSourceGuardRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	r.loads++
	if r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return runtime.RuntimeAggregate{}, common.ErrNotFound
	}
	return runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(r.aggregate)), nil
}

func (r *runtimeReceiptSourceGuardRepository) Save(_ context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if r.aggregate.StateRevision != expectedRevision {
		return common.ErrConflict
	}
	if err := aggregate.Validate(); err != nil {
		return err
	}
	r.saves++
	r.aggregate = runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(aggregate))
	return nil
}

type runtimeReceiptSourceFixture struct {
	name  string
	kind  string
	owner string
}

func runtimeReceiptSourceFixtures() []runtimeReceiptSourceFixture {
	return []runtimeReceiptSourceFixture{
		{name: "file", kind: runtimecontract.StepKindFileRead, owner: runtimecontract.RuntimeModuleFile},
		{name: "git", kind: runtimecontract.StepKindGitRead, owner: runtimecontract.RuntimeModuleGit},
		{name: "shell", kind: runtimecontract.StepKindShellCommand, owner: runtimecontract.RuntimeModuleShell},
		{name: "ssh", kind: runtimecontract.StepKindSSHCommand, owner: runtimecontract.RuntimeModuleSSH},
		{name: "web", kind: runtimecontract.StepKindWebSearch, owner: runtimecontract.RuntimeModuleWeb},
		{name: "mcp", kind: runtimecontract.StepKindMCPResourceRead, owner: runtimecontract.RuntimeModuleMCP},
	}
}

func runtimeReceiptSourceTestOwner(t *testing.T, fixture runtimeReceiptSourceFixture) (*Biz, *runtimeReceiptSourceGuardRepository, string) {
	t.Helper()
	operationID := "operation-" + fixture.name
	repository := &runtimeReceiptSourceGuardRepository{aggregate: runtime.RuntimeAggregate{
		Facts: runtimecontract.RuntimeFacts{WorkspaceID: "workspace-receipt-source", RunID: "run-receipt-source"},
		ActiveDispatch: &runtime.SealedDispatch{
			ID: "dispatch-receipt-source",
			Candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
				Operation: runtimecontract.RuntimeOperation{ID: operationID, Kind: fixture.kind},
			}}}},
			OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{
				OperationID: operationID, ExecutionKey: "execution-" + fixture.name, Fence: 1,
				State: runtimecontract.OperationExecutionClaimed,
			}},
		},
	}}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	owner := newTestBiz(newRuntimeTestOwner(), nil, nil)
	owner.runtimeController = controller
	return owner, repository, operationID
}

func TestRuntimeOperationReceiptSourceIsDerivedFromEachPersistedSealedOwner(t *testing.T) {
	ctx := context.Background()
	for _, fixture := range runtimeReceiptSourceFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			owner, repository, operationID := runtimeReceiptSourceTestOwner(t, fixture)
			_, found, err := owner.runtimeController.QueryOperationReceipt(
				ctx, "workspace-receipt-source", "run-receipt-source", "dispatch-receipt-source", operationID,
				"execution-"+fixture.name, 1, fixture.owner, nil,
			)
			if err != nil || found {
				t.Fatalf("exact persisted owner source rejected: %v", err)
			}
			if repository.loads != 1 || repository.saves != 0 {
				t.Fatalf("source validation loads/saves = %d/%d, want 1/0", repository.loads, repository.saves)
			}
		})
	}
}

func TestRuntimeOperationReceiptPrivateEventRequiresExactSealedOwnerSource(t *testing.T) {
	ctx := context.Background()
	fixture := runtimeReceiptSourceFixtures()[0]
	for _, producer := range []struct {
		name     string
		source   string
		wantLoad bool
	}{
		{name: "missing"},
		{name: "noncanonical", source: " " + fixture.owner + " "},
		{name: "wrong", source: runtimecontract.RuntimeModuleShell, wantLoad: true},
	} {
		t.Run(producer.name, func(t *testing.T) {
			owner, repository, operationID := runtimeReceiptSourceTestOwner(t, fixture)
			before := repository.aggregate
			receipt := runtimecontract.RuntimeOperationReceipt{
				ExecutionKey: "execution-file", Fence: 1, OperationID: operationID,
				Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
			}
			_, err := owner.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind:  runtime.RuntimeInternalEventOperationReceiptObserved,
				Scope: runtime.RuntimeInternalEventScope{WorkspaceID: "workspace-receipt-source", RunID: "run-receipt-source"},
				Transaction: &runtime.RuntimeInternalTransactionPayload{
					DispatchID: "dispatch-receipt-source", Fence: 1,
					OperationReceiptOwnerSource: producer.source, OperationReceipt: &receipt,
				},
			})
			if err == nil {
				t.Fatal("invalid private receipt source was accepted")
			}
			if got := repository.loads; (got != 0) != producer.wantLoad {
				t.Fatalf("private receipt source loads = %d, wantLoad=%v", got, producer.wantLoad)
			}
			if repository.saves != 0 || !reflect.DeepEqual(repository.aggregate, before) {
				t.Fatalf("invalid private receipt source mutated aggregate: saves=%d", repository.saves)
			}
		})
	}
}

func TestRuntimeOperationReceiptAdapterRejectsNilEvent(t *testing.T) {
	owner, _, _ := runtimeReceiptSourceTestOwner(t, runtimeReceiptSourceFixtures()[0])
	result := magicevent.NewResult("nil-event", "blocks/file", orchestratorevents.CommandDestination)
	owner.handleRuntimeOperationReceipt(nil, result)
	if _, err := result.Get(); err == nil || !strings.Contains(err.Error(), common.ErrInvalidArgument.Error()) {
		t.Fatalf("nil receipt event error = %v, want invalid argument", err)
	}
}

func TestRuntimeOperationReceiptExactOwnerQueryReachesReceiptAuthorityForAllSixCapabilityOwners(t *testing.T) {
	ctx := context.Background()
	for _, fixture := range runtimeReceiptSourceFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			owner, repository, operationID := runtimeReceiptSourceTestOwner(t, fixture)
			event := magicevent.NewEventWithContext(
				orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt,
				fixture.owner,
				orchestratorevents.CommandDestination,
				magicevent.NewHeader(),
				ctx,
				orchestratorevents.QueryRuntimeOperationReceiptCommand{
					WorkspaceID: "workspace-receipt-source", RunID: "run-receipt-source",
					DispatchID: "dispatch-receipt-source", OperationID: operationID,
					ExecutionKey: "execution-" + fixture.name, Fence: 1,
				},
			)
			result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
			owner.handleRuntimeOperationReceipt(event, result)
			data, err := result.Get()
			response, ok := data.(orchestratorevents.QueryRuntimeOperationReceiptResult)
			if err != nil || !ok || response.Found || response.Receipt != nil {
				t.Fatalf("exact owner query response=%+v err=%v", data, err)
			}
			if repository.loads != 1 || repository.saves != 0 {
				t.Fatalf("exact owner query loads=%d saves=%d, want 1/0", repository.loads, repository.saves)
			}
		})
	}
}

func TestRuntimeOperationReceiptQueryAndCommitRespectWorkspaceRetirementReadFence(t *testing.T) {
	ctx := context.Background()
	fixture := runtimeReceiptSourceFixtures()[0]
	owner, repository, operationID := runtimeReceiptSourceTestOwner(t, fixture)
	intent := deletioncontract.WorkspaceRuntimeQuiescenceIntent{
		WorkspaceID: "workspace-receipt-source", DeletionID: "deletion-receipt-source", DeletionGeneration: 1,
		DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent/receipt-source",
		RunDrainingRevision: 1,
	}
	deletion, err := reduceWorkspaceDeletionInternalEvent(nil, workspaceDeletionInternalEvent{
		Kind: workspaceDeletionInternalEventQuiescingStarted, Intent: intent,
		GateID: orchestratorevents.StableWorkspaceRuntimeQuiescingGateID(intent),
	}, "2026-08-30T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	deletions := memoryrepo.NewOrchestratorWorkspaceDeletionRepository()
	if err := deletions.Create(ctx, deletion); err != nil {
		t.Fatal(err)
	}
	owner.workspaceDeletions = deletions

	commands := []struct {
		name  string
		topic string
		data  any
	}{
		{name: "query", topic: orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, data: orchestratorevents.QueryRuntimeOperationReceiptCommand{
			WorkspaceID: "workspace-receipt-source", RunID: "run-receipt-source", DispatchID: "dispatch-receipt-source",
			OperationID: operationID, ExecutionKey: "execution-file", Fence: 1,
		}},
		{name: "commit", topic: orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, data: orchestratorevents.CommitRuntimeOperationReceiptCommand{
			WorkspaceID: "workspace-receipt-source", RunID: "run-receipt-source", DispatchID: "dispatch-receipt-source", Fence: 1,
			Receipt: runtimecontract.RuntimeOperationReceipt{
				ExecutionKey: "execution-file", Fence: 1, OperationID: operationID,
				Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
			},
		}},
	}
	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			event := magicevent.NewEventWithContext(command.topic, fixture.owner, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, command.data)
			result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
			owner.handleRuntimeOperationReceipt(event, result)
			if _, err := result.Get(); !errors.Is(err, common.ErrNotFound) {
				t.Fatalf("retired workspace %s error=%v, want not found", command.name, err)
			}
		})
	}
	if repository.loads != 0 || repository.saves != 0 {
		t.Fatalf("retirement-fenced receipt access loads=%d saves=%d, want zero", repository.loads, repository.saves)
	}
}

func TestRuntimeOperationReceiptQueryRejectsEmptyWrongAndCrossOwnerSourceBeforeReceiptAuthority(t *testing.T) {
	ctx := context.Background()
	fixtures := runtimeReceiptSourceFixtures()
	for index, fixture := range fixtures {
		for _, producer := range []struct {
			name         string
			source       string
			wantMismatch bool
			wantLoad     bool
		}{
			{name: "empty"},
			{name: "whitespace", source: " "},
			{name: "noncanonical", source: " " + fixture.owner + " "},
			{name: "wrong", source: "kernel/governance", wantMismatch: true, wantLoad: true},
			{name: "cross-owner", source: fixtures[(index+1)%len(fixtures)].owner, wantMismatch: true, wantLoad: true},
		} {
			t.Run(fixture.name+"/"+producer.name, func(t *testing.T) {
				owner, repository, operationID := runtimeReceiptSourceTestOwner(t, fixture)
				before := repository.aggregate
				event := magicevent.NewEventWithContext(
					orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt,
					producer.source,
					orchestratorevents.CommandDestination,
					magicevent.NewHeader(),
					ctx,
					orchestratorevents.QueryRuntimeOperationReceiptCommand{
						WorkspaceID: "workspace-receipt-source", RunID: "run-receipt-source",
						DispatchID: "dispatch-receipt-source", OperationID: operationID,
						ExecutionKey: "execution-" + fixture.name, Fence: 1,
					},
				)
				result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
				owner.handleRuntimeOperationReceipt(event, result)
				if _, err := result.Get(); err == nil {
					t.Fatal("forged receipt query was accepted")
				} else if producer.wantMismatch && !strings.Contains(err.Error(), "does not own sealed operation") {
					t.Fatalf("query rejected outside sealed owner-source gate: %v", err)
				}
				if got := repository.loads; (got != 0) != producer.wantLoad {
					t.Fatalf("forged query aggregate loads = %d, wantLoad=%v", got, producer.wantLoad)
				}
				if repository.saves != 0 {
					t.Fatalf("forged query CAS count = %d, want zero", repository.saves)
				}
				if !reflect.DeepEqual(repository.aggregate, before) {
					t.Fatal("forged query changed persisted aggregate")
				}
			})
		}
	}
}

func TestRuntimeOperationReceiptCommitRejectsEveryOutcomeFromEmptyWrongAndCrossOwnerSourceBeforeCAS(t *testing.T) {
	ctx := context.Background()
	fixtures := runtimeReceiptSourceFixtures()
	for index, fixture := range fixtures {
		for _, producer := range []struct {
			name         string
			source       string
			wantMismatch bool
			wantLoad     bool
		}{
			{name: "empty"},
			{name: "whitespace", source: " "},
			{name: "noncanonical", source: " " + fixture.owner + " "},
			{name: "wrong", source: "kernel/governance", wantMismatch: true, wantLoad: true},
			{name: "cross-owner", source: fixtures[(index+1)%len(fixtures)].owner, wantMismatch: true, wantLoad: true},
		} {
			for _, outcome := range []runtimecontract.RuntimeOperationOutcome{
				runtimecontract.RuntimeOperationOutcomeAccepted,
				runtimecontract.RuntimeOperationOutcomeNoOp,
				runtimecontract.RuntimeOperationOutcomeRejected,
			} {
				t.Run(fixture.name+"/"+producer.name+"/"+string(outcome), func(t *testing.T) {
					owner, repository, operationID := runtimeReceiptSourceTestOwner(t, fixture)
					before := repository.aggregate
					event := magicevent.NewEventWithContext(
						orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt,
						producer.source,
						orchestratorevents.CommandDestination,
						magicevent.NewHeader(),
						ctx,
						orchestratorevents.CommitRuntimeOperationReceiptCommand{
							WorkspaceID: "workspace-receipt-source", RunID: "run-receipt-source", DispatchID: "dispatch-receipt-source", Fence: 1,
							Receipt: runtimecontract.RuntimeOperationReceipt{
								ExecutionKey: "execution-" + fixture.name, Fence: 1, OperationID: operationID,
								Outcome: outcome, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
							},
						},
					)
					result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
					owner.handleRuntimeOperationReceipt(event, result)
					if _, err := result.Get(); err == nil {
						t.Fatal("forged receipt commit was accepted")
					} else if producer.wantMismatch && !strings.Contains(err.Error(), "does not own sealed operation") {
						t.Fatalf("commit rejected outside sealed owner-source gate: %v", err)
					}
					if got := repository.loads; (got != 0) != producer.wantLoad {
						t.Fatalf("forged commit aggregate loads = %d, wantLoad=%v", got, producer.wantLoad)
					}
					if repository.saves != 0 {
						t.Fatalf("forged %s receipt CAS count = %d, want zero", outcome, repository.saves)
					}
					if !reflect.DeepEqual(repository.aggregate, before) {
						t.Fatalf("forged %s receipt changed persisted aggregate", outcome)
					}
				})
			}
		}
	}
}

func TestRuntimeOperationReceiptExactFileOwnerReachesQueryAndCommitForEveryTerminalOutcome(t *testing.T) {
	for _, outcome := range []runtimecontract.RuntimeOperationOutcome{
		runtimecontract.RuntimeOperationOutcomeAccepted,
		runtimecontract.RuntimeOperationOutcomeNoOp,
		runtimecontract.RuntimeOperationOutcomeRejected,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			testExactFileOwnerReceiptHandlerTransition(t, outcome)
		})
	}
}

func testExactFileOwnerReceiptHandlerTransition(t *testing.T, outcome runtimecontract.RuntimeOperationOutcome) {
	t.Helper()
	ctx := context.Background()
	target := "receipt-source-target.txt"
	aggregate := runtimeAggregate(t, 1)
	aggregate.Facts = runtimeCandidateSealExactFileEditFacts(t, target)
	aggregate.Facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: target, Content: "before", ContentHash: "receipt-source-target-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	repository := &runtimeReceiptSourceGuardRepository{aggregate: aggregate}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, aggregate)
	if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "receipt-source-handler"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "receipt-source-file-edit", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: runtimeFileEditParameters(target),
	}}}}}
	proposal, err := admitRuntimeCandidateProposalForTest(ctx, controller, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil || !proposal.Admission.Accepted() {
		t.Fatalf("admit receipt source candidate = %+v, err=%v", proposal, err)
	}
	recordRuntimeFileEditFeasibility(t, ctx, controller)
	sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("seal receipt source candidate = %+v, err=%v", sealed, err)
	}
	owner := newTestBiz(newRuntimeTestOwner(), nil, nil)
	owner.runtimeController = controller
	beforeExecutionSaves := repository.saves
	ownerCalled := false
	_, executeErr := runtimeTestControllerExecuteDispatch(controller, ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID,
		runtimeOperationExecutorFunc(func(callCtx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
			ownerCalled = true
			queryEvent := magicevent.NewEventWithContext(
				orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt,
				runtimecontract.RuntimeModuleFile,
				orchestratorevents.CommandDestination,
				magicevent.NewHeader(),
				callCtx,
				orchestratorevents.QueryRuntimeOperationReceiptCommand{
					WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID,
					OperationID: request.Operation.ID, ExecutionKey: request.Execution.ExecutionKey, Fence: request.Execution.Fence,
				},
			)
			queryResult := magicevent.NewResult(queryEvent.ID(), queryEvent.Source(), queryEvent.Destination())
			owner.handleRuntimeOperationReceipt(queryEvent, queryResult)
			data, queryErr := queryResult.Get()
			response, ok := data.(orchestratorevents.QueryRuntimeOperationReceiptResult)
			if queryErr != nil || !ok || response.Found || response.Receipt != nil {
				return runtime.RuntimeOperationExecutionResult{}, errors.New("exact owner did not reach authorized found=false receipt query")
			}

			receipt := validFileReceiptForSourceTest(t, request, outcome)
			beforeCommitLoads := repository.loads
			beforeCommitSaves := repository.saves
			commitEvent := magicevent.NewEventWithContext(
				orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt,
				runtimecontract.RuntimeModuleFile,
				orchestratorevents.CommandDestination,
				magicevent.NewHeader(),
				callCtx,
				orchestratorevents.CommitRuntimeOperationReceiptCommand{
					WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID,
					Fence: request.Execution.Fence, Receipt: receipt,
				},
			)
			commitResult := magicevent.NewResult(commitEvent.ID(), commitEvent.Source(), commitEvent.Destination())
			owner.handleRuntimeOperationReceipt(commitEvent, commitResult)
			data, commitErr := commitResult.Get()
			committed, ok := data.(orchestratorevents.CommitRuntimeOperationReceiptResult)
			if commitErr != nil || !ok || committed.StateRevision == 0 || repository.loads != beforeCommitLoads+1 || repository.saves != beforeCommitSaves+1 {
				return runtime.RuntimeOperationExecutionResult{}, errors.New("exact owner did not reach receipt SM-3 CAS")
			}
			readSource := runtimeOperationExecutionContext(request, "source-query-directory", "")
			for _, mismatch := range []bool{false, true} {
				value := runtimecontract.CloneRuntimeOperationExecutionContext(readSource)
				if mismatch {
					value.Transaction.DefinitionRevision = "other"
				}
				event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, runtimecontract.RuntimeModuleFile, orchestratorevents.CommandDestination, magicevent.NewHeader(), callCtx, orchestratorevents.QueryRuntimeOperationReceiptCommand{
					WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID,
					OperationID: request.Operation.ID, ExecutionKey: request.Execution.ExecutionKey, Fence: request.Execution.Fence, ReadSource: &value,
				})
				result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
				loads, saves := repository.loads, repository.saves
				owner.handleRuntimeOperationReceipt(event, result)
				data, err := result.Get()
				if mismatch {
					if err == nil || repository.saves != saves {
						return runtime.RuntimeOperationExecutionResult{}, errors.New("read source mismatch accepted")
					}
				} else {
					response, ok := data.(orchestratorevents.QueryRuntimeOperationReceiptResult)
					if err != nil || !ok || !response.Found || response.Receipt == nil || !runtimecontract.SameRuntimeOperationReceipt(*response.Receipt, committed.Receipt) || repository.loads != loads+1 || repository.saves != saves {
						return runtime.RuntimeOperationExecutionResult{}, errors.New("exact read source did not return original receipt in one read")
					}
				}
			}
			// Exercise the normal response-loss path as well: Runtime must recover
			// the just-persisted receipt rather than execute a second owner call.
			return runtime.RuntimeOperationExecutionResult{}, errors.New("simulated owner response loss after receipt CAS")
		}))
	if executeErr != nil {
		t.Fatalf("exact %s receipt handler transition: %v", outcome, executeErr)
	}
	if !ownerCalled || repository.saves <= beforeExecutionSaves {
		t.Fatalf("exact %s receipt ownerCalled=%v saves=%d before=%d", outcome, ownerCalled, repository.saves, beforeExecutionSaves)
	}
	if len(repository.aggregate.OperationReceipts) != 1 || repository.aggregate.OperationReceipts[0].Outcome != outcome {
		t.Fatalf("persisted exact %s receipt = %+v", outcome, repository.aggregate.OperationReceipts)
	}
}

func validFileReceiptForSourceTest(t *testing.T, request runtime.RuntimeOperationExecutionRequest, outcome runtimecontract.RuntimeOperationOutcome) runtimecontract.RuntimeOperationReceipt {
	t.Helper()
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: request.Execution.ExecutionKey, Fence: request.Execution.Fence, OperationID: request.Operation.ID,
		Outcome: outcome, TargetRefs: append([]string(nil), request.Operation.TargetRefs...),
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner source receipt " + string(outcome)},
	}
	switch outcome {
	case runtimecontract.RuntimeOperationOutcomeAccepted:
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectWorkspaceDurable
		receipt.Changed = true
	case runtimecontract.RuntimeOperationOutcomeNoOp:
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectNone
	case runtimecontract.RuntimeOperationOutcomeRejected:
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
		failure := runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "receipt_source_rejected",
			Signature: "receipt-source-rejected", Message: "owner rejected the sealed operation", TargetRefs: append([]string(nil), request.Operation.TargetRefs...),
		}
		feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
			WorkspaceID: request.WorkspaceID, RunID: request.RunID,
			AuthoritySnapshotID: request.Transaction.AuthoritySnapshotID, DispatchID: request.DispatchID,
		}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(outcome), runtimecontract.RuntimeModuleFile, failure)
		if err != nil {
			t.Fatal(err)
		}
		feedback.OriginalDiagnostic.OperationID = request.Operation.ID
		receipt.Feedback = &feedback
	default:
		t.Fatalf("unsupported receipt source test outcome %q", outcome)
	}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("valid File receipt %s: %v", outcome, err)
	}
	return receipt
}
