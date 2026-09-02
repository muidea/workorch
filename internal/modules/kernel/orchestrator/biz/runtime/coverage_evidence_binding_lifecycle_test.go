package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// C12/T30: an exact-target binding is a bounded authorization set, not a
// requirement that every individual operation and receipt repeat the complete
// set. This exercises the production semantic compiler, SM-3 manifest/
// candidate compiler and sealed dispatch before SM-2 derives credit from the
// two singleton owner receipts.
func TestCoverageEvidenceBindingMultiTargetExactOperationsCreditAndRecoverExactlyOnce(t *testing.T) {
	ctx := context.Background()
	targets := []string{"go.mod", "main.go", "semantic_test.go"}
	controller, repository, aggregate := sealMultiTargetExactBindingForTest(t, targets)
	binding := aggregate.Facts.Focus.CoverageEvidenceBindings[0]
	if !sameRuntimeTargetRefs(binding.TargetRefs, targets) || len(binding.ProspectiveTargetRefs) != 0 {
		t.Fatalf("semantic compiler did not seal the exact target set: %+v", binding)
	}

	candidate := multiTargetExactCandidate(binding.ID, "main.go", "semantic_test.go")
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, candidate)
	if err != nil {
		t.Fatal(err)
	}
	mainOperationID := runtimeTestActiveOperationIDForTarget(t, sealed, "main.go")
	semanticOperationID := runtimeTestActiveOperationIDForTarget(t, sealed, "semantic_test.go")
	if mainOperationID == semanticOperationID {
		t.Fatalf("distinct singleton targets shared operation identity %q", mainOperationID)
	}
	for _, operation := range candidateRuntimeOperations(sealed.ActiveDispatch.Candidate) {
		if len(operation.TargetRefs) != 1 || len(operation.BindingRefs) != 1 || operation.BindingRefs[0] != binding.ID || operation.TargetRefs[0] == "go.mod" {
			t.Fatalf("SM-3 did not seal a binding-authorized proper subset operation: %+v", operation)
		}
	}

	// The first owner effect and receipt are durable, then the process is lost
	// before SM-3 can project the receipt into SM-2 evidence.
	claimed, err := controller.claimOperationExecution(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, mainOperationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, mainOperationID)
	if !found {
		t.Fatal("claimed exact-target execution is missing")
	}
	firstReceipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		Changed: true, TargetRefs: []string{"main.go"},
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "main.go owner effect committed"},
	}
	if _, _, err := controller.commitOperationReceipt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, firstReceipt); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingOperationExecutor{owner: restarted}
	completed, err := restarted.executeActiveDispatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, executor)
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.requests) != 1 || executor.requests[0].Operation.ID != semanticOperationID ||
		!sameRuntimeTargetRefs(executor.requests[0].Operation.TargetRefs, []string{"semantic_test.go"}) {
		t.Fatalf("fresh owner replayed the first effect or lost the remaining singleton: %+v", executor.requests)
	}
	if completed.ActiveDispatch != nil || len(completed.CompletedDispatches) != 1 ||
		completed.Decision.State != runtimecontract.ConvergenceStateNeedAssessment || completed.Decision.Terminal != nil {
		t.Fatalf("multi-target exact dispatch did not converge: %+v", completed)
	}
	if len(completed.OperationReceipts) != 2 || len(completed.Facts.Evidence.Accepted) != 2 || completed.EvidenceSequence != 2 {
		t.Fatalf("multi-target exact receipt/evidence ledger = receipts=%d accepted=%d sequence=%d", len(completed.OperationReceipts), len(completed.Facts.Evidence.Accepted), completed.EvidenceSequence)
	}
	for _, evidence := range completed.Facts.Evidence.Accepted {
		if len(evidence.BindingRefs) != 1 || evidence.BindingRefs[0] != binding.ID ||
			len(evidence.CoverageWorkItemIDs) != 1 || evidence.CoverageWorkItemIDs[0] != binding.CoverageWorkItemID ||
			len(evidence.TargetRefs) != 1 {
			t.Fatalf("singleton receipt did not receive exact binding-derived credit: %+v", evidence)
		}
	}
	if len(completed.Facts.Coverage) != 1 || completed.Facts.Coverage[0].State == runtimecontract.RuntimeCoverageItemStateSatisfied {
		t.Fatalf("receipt credit bypassed the separately owned assessment gate: %+v", completed.Facts.Coverage)
	}

	// A second fresh process cannot replay either effect, and replaying the
	// already-committed evidence identity cannot add credit or advance CAS.
	stateRevision := completed.StateRevision
	saves := repository.saves
	secondRestart, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	noReplay := &failIfExecutedOperationExecutor{}
	if _, err := secondRestart.executeActiveDispatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, noReplay); err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("completed dispatch reload err = %v, want conflict", err)
	}
	if noReplay.called || repository.aggregate.StateRevision != stateRevision || repository.saves != saves {
		t.Fatalf("completed dispatch reload replayed effect or state: called=%t revision=%d saves=%d", noReplay.called, repository.aggregate.StateRevision, repository.saves)
	}
	firstEvidence := completed.Facts.Evidence.Accepted[0]
	duplicate, err := secondRestart.commitEvidence(ctx, ExecutionEvidence{
		EventID: string(firstEvidence.ID), WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DispatchID: firstEvidence.DispatchID, AuthoritySnapshotID: firstEvidence.AuthoritySnapshotID, OperationID: firstEvidence.OperationID,
		Kind: firstEvidence.Kind, Status: ExecutionEvidenceStatusAccepted, SourceRef: firstEvidence.SourceRef,
	})
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != stateRevision ||
		duplicate.Aggregate.EvidenceSequence != 2 || repository.saves != saves {
		t.Fatalf("multi-target exact evidence replay = %+v err=%v saves=%d", duplicate, err, repository.saves)
	}
}

// C12/T31: membership in the same exact binding set does not let an owner
// receipt drift from the singleton operation that was actually sealed. The
// durable owner fact is retained, but SM-2 must reject coverage credit and
// close the deterministic contract defect without replaying the effect.
func TestCoverageEvidenceBindingMultiTargetExactRejectsReceiptOperationTargetDrift(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate := sealMultiTargetExactBindingForTest(t, []string{"go.mod", "main.go", "semantic_test.go"})
	binding := aggregate.Facts.Focus.CoverageEvidenceBindings[0]
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, multiTargetExactCandidate(binding.ID, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationIDForTarget(t, sealed, "main.go")
	claimed, err := controller.claimOperationExecution(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, operationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
	if !found {
		t.Fatal("claimed drift fixture execution is missing")
	}
	driftedReceipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		Changed: true, TargetRefs: []string{"semantic_test.go"},
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Summary: "owner reported the wrong exact target", EvidenceReason: "owner observed a durable effect at its returned target",
		},
	}
	if _, _, err := controller.commitOperationReceipt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, driftedReceipt); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	noReplay := &failIfExecutedOperationExecutor{}
	failed, err := restarted.executeActiveDispatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, noReplay)
	if err != nil {
		t.Fatal(err)
	}
	if noReplay.called {
		t.Fatal("receipt target drift replayed an already durable owner effect")
	}
	if failed.Decision.State != runtimecontract.ConvergenceStateFailed || failed.Decision.Terminal == nil ||
		failed.Decision.Terminal.Code != "runtime_contract_defect" || failed.Facts.Failure == nil ||
		failed.Facts.Failure.Code != coverageBindingMismatchFailureCode {
		t.Fatalf("receipt/operation target drift did not fail closed: %+v", failed)
	}
	if len(failed.Facts.Evidence.Accepted) != 1 || len(failed.Facts.Evidence.Accepted[0].CoverageWorkItemIDs) != 0 ||
		failed.Facts.Evidence.Accepted[0].EvidenceReason != driftedReceipt.Evidence.EvidenceReason ||
		len(failed.Facts.Coverage) != 1 || failed.Facts.Coverage[0].State == runtimecontract.RuntimeCoverageItemStateSatisfied {
		t.Fatalf("receipt/operation target drift received coverage credit: %+v", failed)
	}
}

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
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, ctx, controller, facts, controllerDefinition(), nil)
	facts = aggregate.Facts
	bindingRef := aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{
			ID: "write-c12-recovery", Kind: runtimecontract.StepKindFileWrite, BindingRefs: []string{bindingRef},
			TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("binding recovery"),
		},
	}}}}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, candidate)
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	claimed, err := controller.claimOperationExecution(ctx, facts.WorkspaceID, facts.RunID, operationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
	if !found {
		t.Fatal("sealed operation execution is missing")
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		Changed: true, TargetRefs: []string{"target.txt"}, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner committed before evidence CAS"},
	}
	if _, _, err := controller.commitOperationReceipt(ctx, facts.WorkspaceID, facts.RunID, sealed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, receipt); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	noReplay := &failIfExecutedOperationExecutor{}
	completed, err := restarted.executeActiveDispatch(ctx, facts.WorkspaceID, facts.RunID, noReplay)
	if err != nil {
		t.Fatal(err)
	}
	if noReplay.called {
		t.Fatal("crash recovery replayed owner side effect after persisted receipt")
	}
	completed, err = runtimeTestCommitSatisfiedCoverageAssessment(ctx, restarted, completed)
	if err != nil {
		t.Fatal(err)
	}
	assertSingleBindingCredit(t, completed, bindingRef, facts.Focus.CoverageWorkItemIDs[0])

	// A lost response after the evidence CAS is represented by the original
	// event identity being replayed. It must be a no-op, not a second credit.
	evidence := completed.Facts.Evidence.Accepted[0]
	duplicate, err := restarted.commitEvidence(ctx, ExecutionEvidence{
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
		aggregate   RuntimeAggregate
	}
	scenarios := []scenario{
		{workspaceID: "ws-c12-a", runID: "run-c12-a", target: "a.txt"},
		{workspaceID: "ws-c12-b", runID: "run-c12-b", target: "b.txt"},
	}
	for index := range scenarios {
		facts := controllerFacts()
		facts.WorkspaceID, facts.RunID = scenarios[index].workspaceID, scenarios[index].runID
		seed := facts.DefinitionSeed.Clone()
		seed.CoverageItems[0].ID = "cwi-c12-shared"
		seed.Slices[0].CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-c12-shared"}
		seed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(seed)
		facts.DefinitionSeed = seed
		facts.Slices = append([]runtimecontract.RuntimeAcceptanceSlice(nil), seed.Slices...)
		facts.Focus.ID = "focus-c12-shared"
		facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-c12-shared"}
		facts.Focus.TargetRefs = []string{scenarios[index].target}
		facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{{
			ID: "cwi-c12-shared", SliceID: seed.CoverageItems[0].SliceID,
			SourceScope: seed.CoverageItems[0].SourceScope, SourceID: seed.CoverageItems[0].SourceID,
			TargetText: seed.CoverageItems[0].TargetText, State: runtimecontract.RuntimeCoverageItemStateOpen,
		}}
		scenarios[index].aggregate = runtimeTestInitializeAndSealSemanticContract(t, ctx, controller, facts, controllerDefinition(), nil)
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
			aggregate := current.aggregate
			bindingRef := aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID
			candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
				Operation: runtimecontract.RuntimeOperation{ID: "write-c12-shared", Kind: runtimecontract.StepKindFileWrite, BindingRefs: []string{bindingRef}, TargetRefs: []string{current.target}, Parameters: runtimeFileWriteParameters(current.target)},
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
			completed, err := controller.executeActiveDispatch(ctx, current.workspaceID, current.runID, &acceptingOperationExecutor{owner: controller})
			if err == nil {
				completed, err = runtimeTestCommitSatisfiedCoverageAssessment(ctx, controller, completed)
			}
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
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	aggregate := runtimeTestInitializeAndSealSemanticContract(t, ctx, controller, facts, definition, nil)
	facts = aggregate.Facts
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
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: runtimeTestActiveOperationID(t, sealed, 0),
		Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusAccepted,
		Evidence: runtimecontract.RuntimeOperationEvidence{EvidenceReason: "validation owner returned no proof for the required check", Validation: &runtimecontract.RuntimeValidationObservation{
			RecipeID: runtimeManagedValidationEvidencePolicy().ID, ExecutedChecks: []string{"required-check"}, PassedChecks: nil,
		}},
	}
	attachRuntimeTestOwnerReceipt(t, repository.aggregate, &evidence)
	committed, err := controller.commitEvidence(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(committed.Aggregate.Facts.Evidence.Accepted) != 1 || len(committed.Aggregate.Facts.Evidence.Accepted[0].CoverageWorkItemIDs) != 0 ||
		committed.Aggregate.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateOpen {
		t.Fatalf("missing validation proof received CWI credit: %+v", committed.Aggregate)
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || committed.Aggregate.Decision.Terminal == nil ||
		committed.Aggregate.Decision.Terminal.Code != "runtime_contract_defect" ||
		len(committed.Aggregate.GovernanceRequests) != 0 ||
		committed.Aggregate.Facts.Recovery.ProviderTransportAttempts != 0 || committed.Aggregate.Facts.Recovery.OperationRecoveryAttempts != 0 {
		t.Fatalf("missing proof did not reach a bounded SM-2 terminal: %+v", committed.Aggregate)
	}
	if committed.Aggregate.Facts.Failure == nil || committed.Aggregate.Facts.Failure.Class != runtimecontract.FailureClassRuntimeContractMisalignment ||
		committed.Aggregate.Facts.Failure.OriginalDiagnostic == nil ||
		committed.Aggregate.Facts.Failure.OriginalDiagnostic.OriginalCode != coverageBindingMismatchFailureCode {
		t.Fatalf("binding mismatch was not retained as a runtime contract diagnostic: %+v", committed.Aggregate.Facts.Failure)
	}
	if got := committed.Aggregate.Facts.Evidence.Accepted[0].EvidenceReason; got != evidence.Receipt.Evidence.EvidenceReason {
		t.Fatalf("owner receipt reason drifted during binding mismatch adjudication: fact=%q receipt=%q", got, evidence.Receipt.Evidence.EvidenceReason)
	}
	stateRevision := committed.Aggregate.StateRevision
	if _, err := controller.BeginCandidateAttemptForTest(ctx, facts.WorkspaceID, facts.RunID, 0, "assessment-c12-missing-proof"); err == nil {
		t.Fatal("terminal binding mismatch accepted a new candidate attempt")
	}
	if repository.aggregate.StateRevision != stateRevision || len(repository.aggregate.GovernanceRequests) != 0 {
		t.Fatalf("terminal binding mismatch mutated after rejected retry: %+v", repository.aggregate)
	}
}

// sealMultiTargetExactBindingForTest deliberately enters through the same
// semantic manifest/document/feasibility/internal-event chain as production.
// It must not install a hand-written binding because this regression protects
// the exact authorization semantics shared by semantic seal and dispatch.
func sealMultiTargetExactBindingForTest(t *testing.T, targets []string) (*RuntimeController, *controllerRepository, RuntimeAggregate) {
	return sealMultiTargetExactBindingProofsForTest(t, targets, true, false, "", "file-multi-target-revision-1")
}

func sealMultiTargetExactValidationBindingForTest(t *testing.T, targets []string, targetRole string) (*RuntimeController, *controllerRepository, RuntimeAggregate) {
	return sealMultiTargetExactBindingProofsForTest(t, targets, true, true, targetRole, "tree-v1")
}

func sealMultiTargetExactValidationOnlyBindingForTest(t *testing.T, targets []string, targetRole string) (*RuntimeController, *controllerRepository, RuntimeAggregate) {
	return sealMultiTargetExactBindingProofsForTest(t, targets, false, true, targetRole, "tree-v1")
}

func sealMultiTargetExactBindingProofsForTest(t *testing.T, targets []string, includeStateChange, includeValidation bool, targetRole, observedRevision string) (*RuntimeController, *controllerRepository, RuntimeAggregate) {
	t.Helper()
	if !includeStateChange && !includeValidation {
		t.Fatal("multi-target exact binding fixture requires at least one proof kind")
	}
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.TargetRefs = append([]string(nil), targets...)
	facts.Focus.TargetRole = targetRole
	facts.Focus.RequiredEvidenceKinds = nil
	if includeStateChange {
		facts.Focus.RequiredEvidenceKinds = append(facts.Focus.RequiredEvidenceKinds, runtimecontract.EvidenceKindStateChange)
	}
	if includeValidation {
		facts.Focus.RequiredEvidenceKinds = append(facts.Focus.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation)
		facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
		facts = runtimeFactsWithManagedValidationRecipe(facts)
		facts.Resources.Manifest = &runtimecontract.WorkspaceManifest{Revision: observedRevision, TargetRefs: append([]string(nil), targets...)}
	}
	facts.DefinitionFocuses = nil
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "semantic-multi-target-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: append([]string(nil), targets...),
		ObservedRevision: observedRevision,
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	ownerCatalogs := []runtimecontract.RuntimeOperationAdmissibility{catalog}
	for _, ownerCatalog := range facts.Resources.OperationAdmissibilities {
		if ownerCatalog.Normalized().OwnerModuleID != runtimecontract.RuntimeModuleFile {
			ownerCatalogs = append(ownerCatalogs, ownerCatalog)
		}
	}
	facts.Resources.OperationAdmissibilities = ownerCatalogs
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	for _, ownerCatalog := range ownerCatalogs {
		ownerCatalog = ownerCatalog.Normalized()
		if ownerCatalog.OwnerModuleID == runtimecontract.RuntimeModuleFile {
			continue
		}
		ownerObservation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: ownerCatalog.PrerequisiteID,
			Kind:           runtimecontract.PrerequisiteKindOperationAdmissibility,
			OwnerModuleID:  ownerCatalog.OwnerModuleID,
			ContentHash:    ownerCatalog.SnapshotID,
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: ownerCatalog.OwnerModuleID,
				SubjectRef: ownerCatalog.PrerequisiteID, Revision: ownerCatalog.ObservedRevision,
			}},
		}
		ownerObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(ownerObservation)
		facts.Resources.PrerequisiteObservations = append(facts.Resources.PrerequisiteObservations, ownerObservation)
	}
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, prerequisite)
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	definition := controllerDefinition()
	if includeValidation {
		definition.CapabilityContract = append(definition.CapabilityContract, runtimecontract.RuntimeBlockKindManagedValidationSession)
	}
	aggregate, err := controller.initialize(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		aggregate.Definition,
		aggregate.Facts.DefinitionSeed,
		aggregate.Facts.DefinitionFocuses,
		aggregate.Facts.Activation.ExecutionContract,
		aggregate.Facts.Resources.OperationAdmissibilities,
		runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts),
	)
	if err != nil {
		t.Fatal(err)
	}
	order := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{manifest.CoverageItems[0].ID},
	}
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	declarationRef := "semantic/multi-target-exact/" + string(manifest.CoverageItems[0].ID)
	stateChangeProposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: manifest.CoverageItems[0].ID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKind: runtimecontract.StepKindFileWrite,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: append([]string(nil), targets...),
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        declarationRef,
	}
	type proposalFixture struct {
		proposal             runtimecontract.RuntimeSemanticProofProposal
		catalog              runtimecontract.RuntimeOperationAdmissibility
		catalogObservationID string
		requiredCheckRefs    []string
	}
	proposalFixtures := make([]proposalFixture, 0, 2)
	if includeStateChange {
		proposalFixtures = append(proposalFixtures, proposalFixture{proposal: stateChangeProposal, catalog: catalog, catalogObservationID: observation.ObservationID})
	}
	if includeValidation {
		var shellCatalog runtimecontract.RuntimeOperationAdmissibility
		var surface runtimecontract.RuntimeValidationSurface
		for _, ownerCatalog := range aggregate.Facts.Resources.OperationAdmissibilities {
			if ownerCatalog.Normalized().OwnerModuleID != runtimecontract.RuntimeModuleShell {
				continue
			}
			shellCatalog = ownerCatalog.Normalized()
			for _, candidateSurface := range shellCatalog.ValidationSurfaces {
				candidateSurface = candidateSurface.Normalized()
				if candidateSurface.Readiness == runtimecontract.RuntimeValidationSurfaceReady &&
					candidateSurface.ProofApplicability == runtimecontract.RuntimeValidationProofDirect {
					surface = candidateSurface
					break
				}
			}
		}
		if shellCatalog.SnapshotID == "" || surface.ID == "" {
			t.Fatal("multi-target validation semantic fixture has no ready Shell surface")
		}
		shellObservation, found := CurrentSemanticCatalogObservation(aggregate, shellCatalog)
		if !found {
			t.Fatal("multi-target validation semantic fixture has no current Shell catalog observation")
		}
		validationProposal := runtimecontract.RuntimeSemanticProofProposal{
			CoverageWorkItemID: manifest.CoverageItems[0].ID, OwnerModuleID: shellCatalog.OwnerModuleID,
			OperationKind: surface.OperationKind,
			Target: runtimecontract.RuntimeOperationTargetAuthorization{
				Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: append([]string(nil), targets...),
			},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{surface.EffectScope},
			ValidationRecipeID:    surface.ID,
			DeclarationRef:        declarationRef,
		}
		proposalFixtures = append(proposalFixtures, proposalFixture{
			proposal: validationProposal, catalog: shellCatalog,
			catalogObservationID: shellObservation.ObservationID,
			requiredCheckRefs:    append([]string(nil), surface.CheckRefs...),
		})
	}
	apply := func(event RuntimeInternalEvent) {
		t.Helper()
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, applyErr := controller.ApplyInternalEvent(ctx, event)
		if applyErr != nil {
			t.Fatalf("apply %s: %v (phase=%q activeBatch=%q feasibilityBatch=%q proposals=%d feasibilities=%d)",
				event.Kind, applyErr, aggregate.SemanticPlanning.Phase, aggregate.SemanticPlanning.ActiveBatchID,
				aggregate.SemanticPlanning.FeasibilityBatchID, func() int {
					if aggregate.SemanticPlanning.Document == nil {
						return 0
					}
					return len(aggregate.SemanticPlanning.Document.Proposals)
				}(), len(aggregate.SemanticPlanning.Feasibilities))
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{
		SemanticManifest: &manifest, StablePrefixHash: "semantic-multi-target-prefix", LogicalRequestID: "semantic-multi-target-request",
	}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{
		SemanticDocument: &order, LogicalRequestID: "semantic-multi-target-request",
	}})
	batch, err := semanticplan.CompileFocusBatch(
		manifest, order, 0, "semantic-multi-target-proof-request", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	proof := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:  batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
		Proposals: func() []runtimecontract.RuntimeSemanticProofProposal {
			ret := make([]runtimecontract.RuntimeSemanticProofProposal, 0, len(proposalFixtures))
			for _, fixture := range proposalFixtures {
				ret = append(ret, fixture.proposal)
			}
			return ret
		}(),
	}
	proof = proof.Normalized()
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{
		SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID,
	}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	for _, fixture := range proposalFixtures {
		candidateProposal := fixture.proposal.Normalized()
		feasibility := runtimecontract.RuntimeSemanticProofFeasibility{
			SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
			ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
			CoverageWorkItemID: candidateProposal.CoverageWorkItemID,
			ProposalHash:       runtimecontract.StableRuntimeSemanticProofProposalHash(candidateProposal),
			OwnerModuleID:      candidateProposal.OwnerModuleID, CatalogSnapshotID: fixture.catalog.SnapshotID,
			CatalogObservationID: fixture.catalogObservationID, Status: runtimecontract.RuntimeSemanticFeasibilityFeasible,
			ObservedRevision: fixture.catalog.ObservedRevision, TargetRefs: append([]string(nil), targets...),
			ValidationRecipeID: candidateProposal.ValidationRecipeID,
			RequiredCheckRefs:  append([]string(nil), fixture.requiredCheckRefs...),
		}
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{
			SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility,
		}})
	}
	accepted := aggregate.SemanticPlanning.Batches[0]
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, proof)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{
		SemanticPlanMergedDocument: &merged,
	}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
	if len(aggregate.Facts.Focus.CoverageEvidenceBindings) != 1 {
		t.Fatalf("semantic contract did not seal one exact binding: %+v", aggregate.Facts.Focus)
	}
	for _, pending := range append([]runtimecontract.RuntimePrerequisite(nil), aggregate.Facts.Prerequisites.Pending...) {
		if pending.Kind != runtimecontract.PrerequisiteKindTargetSnapshot {
			t.Fatalf("multi-target exact seal requested an unexpected prerequisite: %+v", pending)
		}
		content := "current content for " + pending.SubjectRef
		snapshot := runtimecontract.TargetSnapshot{
			TargetRef: pending.SubjectRef, Content: content, ContentHash: "snapshot/" + pending.SubjectRef,
			ContentState: runtimecontract.SnapshotContentStateComplete,
		}
		aggregate, err = controller.commitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{
			PrerequisiteID: pending.ID, TargetSnapshot: &snapshot,
		})
		if err != nil {
			t.Fatalf("commit exact target snapshot %q: %v", pending.SubjectRef, err)
		}
	}
	wantState := runtimecontract.ConvergenceStateNeedStateChange
	if !includeStateChange {
		wantState = runtimecontract.ConvergenceStateNeedValidation
	}
	if aggregate.Decision.State != wantState || len(aggregate.Facts.Prerequisites.Pending) != 0 {
		t.Fatalf("multi-target exact prerequisite closure = decision=%+v pending=%+v", aggregate.Decision, aggregate.Facts.Prerequisites.Pending)
	}
	return controller, repository, aggregate
}

func multiTargetExactCandidate(bindingRef string, targets ...string) runtimecontract.RuntimeCandidate {
	operations := make([]runtimecontract.StateChangingOperation, 0, len(targets))
	for index, target := range targets {
		operations = append(operations, runtimecontract.StateChangingOperation{Operation: runtimecontract.RuntimeOperation{
			ID: "write-exact-" + target, Kind: runtimecontract.StepKindFileWrite,
			BindingRefs: []string{bindingRef}, TargetRefs: []string{target},
			Parameters: runtimeFileWriteParameters("exact target content " + string(rune('a'+index))),
		}})
	}
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: operations}}
}

func assertSingleBindingCredit(t *testing.T, aggregate RuntimeAggregate, bindingRef string, coverageID common.CoverageWorkItemID) {
	t.Helper()
	if len(aggregate.OperationReceipts) != 1 || len(aggregate.Facts.Evidence.Accepted) != 1 ||
		len(aggregate.Facts.Assessments) != 1 || aggregate.EvidenceSequence != 2 {
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

func runtimeTestCommitSatisfiedCoverageAssessment(ctx context.Context, controller *RuntimeController, aggregate RuntimeAggregate) (RuntimeAggregate, error) {
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		return RuntimeAggregate{}, fmt.Errorf("coverage assessment fixture state = %q, want %q", aggregate.Decision.State, runtimecontract.ConvergenceStateNeedAssessment)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "coverage-assessment/"+aggregate.Facts.Focus.ID); err != nil {
		return RuntimeAggregate{}, err
	}
	candidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: aggregate.Facts.Focus.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...),
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "sealed coverage evidence satisfies the active focus",
	}}
	if _, err := controller.AcceptCandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate); err != nil {
		return RuntimeAggregate{}, err
	}
	return controller.commitAssessment(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
}
