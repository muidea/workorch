package runtimev2

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// C12/T27: an owner receipt is already durable before the evidence CAS. A
// fresh controller therefore commits the same binding-derived evidence exactly
// once and must not execute the durable operation again.
func TestCoverageEvidenceBindingReceiptCrashRecoveryCreditsExactlyOnce(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts = runtimeV2TestFactsWithCurrentFocusBindings(t, facts, controllerDefinition())
	aggregate, err := controller.Initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	bindingRef := aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{
			ID: "write-c12-recovery", Kind: runtimecontract.StepKindFileWrite, BindingRefs: []string{bindingRef},
			TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("binding recovery"),
		},
	}}}}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, candidate)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := controller.claimOperationExecution(ctx, facts.WorkspaceID, facts.RunID, "write-c12-recovery")
	if err != nil {
		t.Fatal(err)
	}
	execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, "write-c12-recovery")
	if !found {
		t.Fatal("sealed operation execution is missing")
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		Changed: true, TargetRefs: []string{"target.txt"}, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner committed before evidence CAS"},
	}
	if _, _, err := controller.CommitOperationReceipt(ctx, facts.WorkspaceID, facts.RunID, sealed.ActiveDispatch.ID, execution.Fence, receipt); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	noReplay := &failIfExecutedOperationExecutor{}
	completed, err := restarted.ExecuteActiveDispatch(ctx, facts.WorkspaceID, facts.RunID, noReplay)
	if err != nil {
		t.Fatal(err)
	}
	if noReplay.called {
		t.Fatal("crash recovery replayed owner side effect after persisted receipt")
	}
	assertSingleBindingCredit(t, completed, bindingRef, facts.Focus.CoverageWorkItemIDs[0])

	// A lost response after the evidence CAS is represented by the original
	// event identity being replayed. It must be a no-op, not a second credit.
	evidence := completed.Facts.Evidence.Accepted[0]
	duplicate, err := restarted.CommitEvidence(ctx, ExecutionEvidence{
		EventID: string(evidence.ID), WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		DispatchID: evidence.DispatchID, AuthoritySnapshotID: evidence.AuthoritySnapshotID, OperationID: evidence.OperationID,
		Kind: evidence.Kind, Status: ExecutionEvidenceStatusAccepted, SourceRef: evidence.SourceRef,
	})
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.EvidenceSequence != completed.EvidenceSequence {
		t.Fatalf("evidence response-loss replay = %+v err=%v", duplicate, err)
	}
}

// C12/T28: binding IDs and recipe names are not process-global. Two active
// Runs may deliberately use the same names without sharing a receipt, credit,
// execution key, or terminal decision.
func TestCoverageEvidenceBindingIsolatedAcrossConcurrentWorkspaces(t *testing.T) {
	ctx := context.Background()
	repository := &scopedControllerRepository{aggregates: make(map[string]RuntimeAggregate)}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	type scenario struct {
		workspaceID common.WorkspaceID
		runID       common.RunID
		target      string
		facts       runtimecontract.RuntimeFacts
	}
	scenarios := []scenario{
		{workspaceID: "ws-c12-a", runID: "run-c12-a", target: "a.txt"},
		{workspaceID: "ws-c12-b", runID: "run-c12-b", target: "b.txt"},
	}
	for index := range scenarios {
		facts := controllerFacts()
		facts.WorkspaceID, facts.RunID = scenarios[index].workspaceID, scenarios[index].runID
		facts.Focus.ID = "focus-c12-shared"
		facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-c12-shared"}
		facts.Focus.TargetRefs = []string{scenarios[index].target}
		facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-c12-shared", State: runtimecontract.RuntimeCoverageItemStateOpen}}
		scenarios[index].facts = runtimeV2TestFactsWithCurrentFocusBindings(t, facts, controllerDefinition())
	}
	type outcome struct {
		scenario  scenario
		aggregate RuntimeAggregate
		err       error
	}
	results := make(chan outcome, len(scenarios))
	var group sync.WaitGroup
	for _, current := range scenarios {
		current := current
		group.Add(1)
		go func() {
			defer group.Done()
			aggregate, err := controller.Initialize(ctx, current.facts, controllerDefinition(), nil)
			if err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			bindingRef := aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID
			candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
				Operation: runtimecontract.RuntimeOperation{ID: "write-c12-shared", Kind: runtimecontract.StepKindFileWrite, BindingRefs: []string{bindingRef}, TargetRefs: []string{current.target}, Parameters: runtimeV2FileWriteParameters(current.target)},
			}}}}
			if _, err := controller.BeginCandidateAttemptForTest(ctx, current.workspaceID, current.runID, 0, "c12-isolation-prefix"); err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			if err := RecordFixtureCandidateFeasibilities(ctx, controller, current.workspaceID, current.runID, 0, candidate); err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			if _, err := controller.AcceptCandidateAttempt(ctx, current.workspaceID, current.runID, 0, candidate); err != nil {
				results <- outcome{scenario: current, err: err}
				return
			}
			completed, err := controller.ExecuteActiveDispatch(ctx, current.workspaceID, current.runID, &acceptingOperationExecutor{owner: controller})
			results <- outcome{scenario: current, aggregate: completed, err: err}
		}()
	}
	group.Wait()
	close(results)

	completed := make(map[common.WorkspaceID]RuntimeAggregate, len(scenarios))
	for result := range results {
		if result.err != nil {
			t.Fatalf("workspace %s C12 execution: %v", result.scenario.workspaceID, result.err)
		}
		bindingRef := result.aggregate.Facts.Evidence.Accepted[0].BindingRefs[0]
		assertSingleBindingCredit(t, result.aggregate, bindingRef, "cwi-c12-shared")
		if len(result.aggregate.OperationReceipts) != 1 || !strings.Contains(result.aggregate.OperationReceipts[0].ExecutionKey, string(result.scenario.workspaceID)) ||
			!sameRuntimeTargetRefs(result.aggregate.OperationReceipts[0].TargetRefs, []string{result.scenario.target}) {
			t.Fatalf("workspace %s receipt leaked scope: %+v", result.scenario.workspaceID, result.aggregate.OperationReceipts)
		}
		completed[result.scenario.workspaceID] = result.aggregate
	}
	first, second := completed[scenarios[0].workspaceID], completed[scenarios[1].workspaceID]
	if first.OperationReceipts[0].ExecutionKey == second.OperationReceipts[0].ExecutionKey ||
		first.Facts.Evidence.Accepted[0].ID == second.Facts.Evidence.Accepted[0].ID {
		t.Fatalf("workspace-scoped receipt/evidence identities collided: first=%+v second=%+v", first, second)
	}
}

// C12/T29: a producer's satisfied claim cannot make missing binding proof a
// governance problem. A receipt that was explicitly sealed to close coverage
// but cannot satisfy that binding is a determined SM-2 no-progress terminal:
// it keeps the CWI open and prevents an unbounded candidate/validation loop.
func TestCoverageEvidenceBindingMissingProofCannotBeClaimedOrGovernedAway(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := validationQualityFacts(t)
	facts.Focus.RequireAssessment = true
	facts.Focus.RequiredCheckRefs = []string{"required-check"}
	facts = runtimeV2TestFactsWithCurrentFocusBindings(t, facts, controllerDefinition())
	aggregate, err := controller.Initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	bindingRef := aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID
	candidate := managedValidationCandidate("validate-c12-missing-proof")
	candidate.Validation.Operations[0].Operation.BindingRefs = []string{bindingRef}
	candidate.Validation.Operations[0].Operation.CoverageWorkItemIDs = nil
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, candidate)
	if err != nil {
		t.Fatal(err)
	}
	evidence := ExecutionEvidence{
		EventID: "evidence-c12-missing-proof", WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "validate-c12-missing-proof",
		Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusAccepted,
		Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{
			RecipeID: runtimeV2ManagedValidationEvidencePolicy().ID, ExecutedChecks: []string{"required-check"}, PassedChecks: nil,
		}},
	}
	attachRuntimeV2TestOwnerReceipt(t, repository.aggregate, &evidence)
	committed, err := controller.CommitEvidence(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(committed.Aggregate.Facts.Evidence.Accepted) != 1 || len(committed.Aggregate.Facts.Evidence.Accepted[0].CoverageWorkItemIDs) != 0 ||
		committed.Aggregate.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateOpen {
		t.Fatalf("missing validation proof received CWI credit: %+v", committed.Aggregate)
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || committed.Aggregate.Decision.Terminal == nil ||
		committed.Aggregate.Decision.Terminal.Code != coverageBindingMismatchFailureCode ||
		len(committed.Aggregate.GovernanceRequests) != 0 ||
		committed.Aggregate.Facts.Recovery.ProviderTransportAttempts != 0 || committed.Aggregate.Facts.Recovery.OperationRecoveryAttempts != 0 {
		t.Fatalf("missing proof did not reach a bounded SM-2 terminal: %+v", committed.Aggregate)
	}
	if got := committed.Aggregate.Facts.Evidence.Accepted[0].EvidenceReason; !strings.Contains(got, "coverage_binding_mismatch") {
		t.Fatalf("original binding diagnostic was not retained: %q", got)
	}
	stateRevision := committed.Aggregate.StateRevision
	if _, err := controller.BeginCandidateAttemptForTest(ctx, facts.WorkspaceID, facts.RunID, 0, "assessment-c12-missing-proof"); err == nil {
		t.Fatal("terminal binding mismatch accepted a new candidate attempt")
	}
	if repository.aggregate.StateRevision != stateRevision || len(repository.aggregate.GovernanceRequests) != 0 {
		t.Fatalf("terminal binding mismatch mutated after rejected retry: %+v", repository.aggregate)
	}
}

func assertSingleBindingCredit(t *testing.T, aggregate RuntimeAggregate, bindingRef string, coverageID common.CoverageWorkItemID) {
	t.Helper()
	if len(aggregate.OperationReceipts) != 1 || len(aggregate.Facts.Evidence.Accepted) != 1 || aggregate.EvidenceSequence != 1 {
		t.Fatalf("C12 receipt/evidence was not exactly once: %+v", aggregate)
	}
	evidence := aggregate.Facts.Evidence.Accepted[0]
	if len(evidence.BindingRefs) != 1 || evidence.BindingRefs[0] != bindingRef || len(evidence.CoverageWorkItemIDs) != 1 || evidence.CoverageWorkItemIDs[0] != coverageID {
		t.Fatalf("C12 derived credit does not match sealed binding: %+v", evidence)
	}
	if len(aggregate.Facts.Coverage) != 1 || aggregate.Facts.Coverage[0].ID != coverageID || aggregate.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateSatisfied {
		t.Fatalf("C12 coverage is not binding-derived and satisfied: %+v", aggregate.Facts.Coverage)
	}
}
