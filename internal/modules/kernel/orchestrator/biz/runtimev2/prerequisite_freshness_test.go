package runtimev2

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPrerequisiteStaleInvalidatesProposalAndReacquiresWithoutCandidateDebt(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()
	facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"}, ObservedRevision: "tree-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: inventory.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	initialized, err := controller.Initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialFreshnessHash := initialized.Decision.AuthoritySnapshot.PrerequisiteFreshnessHash
	if initialFreshnessHash == "" {
		t.Fatal("fresh observation did not contribute to authority transaction")
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		TargetRefs: []string{"target.txt"}, Parameters: runtimeV2FileWriteParameters("state"),
	}}}}}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.RecordCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2", Reason: "workspace changed",
	}
	invalidated, err := controller.InvalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || invalidated.ActiveDispatch != nil ||
		len(invalidated.Facts.Prerequisites.Pending) != 1 || len(invalidated.Facts.Prerequisites.Satisfied) != 0 ||
		len(invalidated.Facts.Resources.OperationAdmissibilities) != 0 || len(invalidated.Facts.Resources.PrerequisiteObservations) != 0 ||
		invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 || invalidated.Facts.Recovery.CapabilityStateRefreshes != 1 {
		t.Fatalf("stale did not atomically return to prerequisite acquisition: %+v", invalidated)
	}
	if len(invalidated.CandidateAttempts) != 1 || invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated ||
		invalidated.CandidateAttempts[0].Failure != nil || invalidated.CandidateAttempts[0].Proposal != nil ||
		len(invalidated.Facts.Evidence.Accepted) != 0 || len(invalidated.Facts.Evidence.Rejected) != 0 || invalidated.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("stale consumed candidate, evidence, or repair authority: %+v", invalidated)
	}
	saves := repository.saves
	duplicate, err := controller.InvalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil || duplicate.StateRevision != invalidated.StateRevision || repository.saves != saves || duplicate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("duplicate stale was not idempotent: aggregate=%+v saves=%d err=%v", duplicate, repository.saves, err)
	}
	if _, err := controller.CommitPrerequisite(ctx, "ws-v2", "run-v2", PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &inventory, Observation: &observation,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale prerequisite result commit = %v, want conflict", err)
	}
	newInventory := inventory
	newInventory.ObservedRevision = "tree-v2"
	newInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(newInventory)
	newObservation := observation
	newObservation.ContentHash = newInventory.SnapshotID
	newObservation.Dependencies[0].Revision = "tree-v2"
	newObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(newObservation)
	reacquired, err := controller.CommitPrerequisite(ctx, "ws-v2", "run-v2", PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &newInventory, Observation: &newObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reacquired.Decision.Action != runtimecontract.RuntimeActionProduceStateChange ||
		reacquired.Decision.AuthoritySnapshot.PrerequisiteFreshnessHash == initialFreshnessHash {
		t.Fatalf("fresh acquisition did not seal a new authority transaction: %+v", reacquired.Decision)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "fresh-prefix"); err != nil {
		t.Fatalf("new authority did not reopen candidate attempt 0: %v", err)
	}
}

func TestPrerequisiteStaleInvalidatesPendingCandidateRequestWithoutCandidateDebt(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	prerequisite, observation := addFreshFileAdmissibilityObservation(t, &aggregate)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-1", "run-1", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2", Reason: "workspace changed",
	}
	invalidated, err := controller.InvalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-1", "run-1", 0, stale)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite ||
		len(invalidated.CandidateAttempts) != 1 || invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated ||
		invalidated.CandidateAttempts[0].Proposal != nil || invalidated.CandidateAttempts[0].Failure != nil ||
		invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 || len(invalidated.Facts.Evidence.Accepted) != 0 ||
		len(invalidated.Facts.Evidence.Rejected) != 0 || invalidated.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("pending candidate stale consumed candidate/repair/evidence authority: %+v", invalidated)
	}
}

func TestMCPOwnerCatalogStaleRestartDoesNotReuseFailureTarget(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:mcp", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{"server-a"}, ObservedRevision: "mcp-catalog-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleMCP, RequiredCapabilities: []string{runtimecontract.StepKindMCPToolCall}}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", Revision: "mcp-catalog-v1"}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	targetObservation := &runtimecontract.RuntimeFailureTargetObservation{OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "mcp-catalog-v1", TargetRefs: []string{"implementation.go"}}
	facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Signature: "mcp-failure", Message: "observed failure", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation}
	facts.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: "mcp-failure", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	initialized, err := controller.Initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.AuthoritySnapshot.PrerequisiteFreshnessHash == "" {
		t.Fatal("MCP owner-catalog observation was not sealed into authority")
	}
	if _, err := controller.BeginCandidateAttempt(ctx, "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID, Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", ExpectedRevision: "mcp-catalog-v1", ObservedRevision: "mcp-catalog-v2", Reason: "mcp catalog changed"}
	invalidated, err := controller.InvalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.StateRevision != invalidated.StateRevision || loaded.Facts.Resources.FailureTarget != nil || loaded.Facts.Failure == nil || len(loaded.Facts.Failure.TargetRefs) != 0 ||
		len(loaded.Facts.Prerequisites.Pending) != 1 || loaded.Facts.Prerequisites.Pending[0].ID != prerequisite.ID || loaded.Facts.RepairDirective != nil ||
		len(invalidated.CandidateAttempts) != 1 || invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated {
		t.Fatalf("restart retained stale MCP failure authority: loaded=%+v invalidated=%+v err=%v", loaded, invalidated, err)
	}
}

func TestActiveDispatchPrerequisiteStaleInvalidatesWithoutRejectedEvidence(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	prerequisite, observation := addFreshFileAdmissibilityObservation(t, &aggregate)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	sealed := sealCandidateForTest(t, repository, stateChangeCandidate())
	execution := &repository.aggregate.ActiveDispatch.OperationExecutions[0]
	execution.Fence = 1
	execution.State = runtimecontract.OperationExecutionOwnerCommitted
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2", Reason: "workspace changed",
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Failure:           &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Code: "prerequisite_stale", Signature: "stale-file-tree", Message: "workspace changed"},
		PrerequisiteStale: &stale,
	}
	receipt = receipt.Normalized()
	execution.OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	repository.aggregate.OperationReceipts = append(repository.aggregate.OperationReceipts, receipt)
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	invalidated, err := controller.InvalidateActiveDispatchForPrerequisiteStale(ctx, "ws-1", "run-1", sealed.ActiveDispatch.ID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if invalidated.ActiveDispatch != nil || len(invalidated.CompletedDispatches) != 1 ||
		invalidated.CompletedDispatches[0].State != DispatchStateInvalidated ||
		invalidated.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite ||
		len(invalidated.Facts.Evidence.Accepted) != 0 || len(invalidated.Facts.Evidence.Rejected) != 0 ||
		invalidated.Facts.Repair.RepairAttempts != 0 || invalidated.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("generic stale did not return to acquisition cleanly: %+v", invalidated)
	}
	if len(invalidated.Facts.Prerequisites.Pending) != 1 || len(invalidated.Facts.Resources.OperationAdmissibilities) != 0 {
		t.Fatalf("stale did not clear fresh prerequisite facts: %+v", invalidated.Facts)
	}
	stateRevision := invalidated.StateRevision
	duplicate, err := controller.InvalidateActiveDispatchForPrerequisiteStale(ctx, "ws-1", "run-1", sealed.ActiveDispatch.ID, receipt)
	if err != nil || duplicate.StateRevision != stateRevision || duplicate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("duplicate active stale changed authority: aggregate=%+v err=%v", duplicate, err)
	}
}

func TestAcceptedReceiptResourceChangeInvalidatesRemainingDispatch(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	_, _ = addFreshFileAdmissibilityObservation(t, &aggregate)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		{Operation: patchApplyTestOperation("write-1", "main.go")},
		{Operation: func() runtimecontract.RuntimeOperation {
			operation := patchApplyTestOperation("write-2", "other.go")
			operation.DependsOn = []string{"write-1"}
			return operation
		}()},
	}}}
	sealed := sealCandidateForTest(t, repository, candidate)
	evidence := ExecutionEvidence{
		EventID: "event-freshness-resource-change", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-1",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeV2TestOwnerReceipt(t, &repository.aggregate, &evidence)
	receipt := evidence.Receipt.Clone()
	receipt.ResourceChanges = []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", BeforeRevision: "tree-v1", AfterRevision: "tree-v2",
	}}
	receipt = receipt.Normalized()
	hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	for idx := range repository.aggregate.OperationReceipts {
		if repository.aggregate.OperationReceipts[idx].ExecutionKey == receipt.ExecutionKey && repository.aggregate.OperationReceipts[idx].Fence == receipt.Fence {
			repository.aggregate.OperationReceipts[idx] = receipt
		}
	}
	for idx := range repository.aggregate.ActiveDispatch.OperationExecutions {
		if repository.aggregate.ActiveDispatch.OperationExecutions[idx].OperationID == receipt.OperationID {
			repository.aggregate.ActiveDispatch.OperationExecutions[idx].OwnerReceiptHash = hash
		}
	}
	evidence.Receipt = &receipt
	evidence.OwnerReceiptHash = hash
	committed, err := coordinator.CommitEvidence(ctx, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !committed.DispatchCompleted || committed.Aggregate.ActiveDispatch != nil || len(committed.Aggregate.CompletedDispatches) != 1 ||
		committed.Aggregate.CompletedDispatches[0].State != DispatchStateInvalidated ||
		len(committed.Aggregate.Facts.Evidence.Accepted) != 1 || len(committed.Aggregate.Facts.Evidence.Rejected) != 0 ||
		committed.Aggregate.Facts.Repair.RepairAttempts != 0 || committed.Aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 ||
		committed.Aggregate.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 1 ||
		committed.Aggregate.Decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite || len(committed.Aggregate.Facts.FreshnessReacquireCoverage) != 1 ||
		committed.Aggregate.Facts.FreshnessReacquireCoverage[0] != "cwi-1" {
		t.Fatalf("accepted resource change did not atomically invalidate remaining dispatch: %+v", committed.Aggregate)
	}
	if len(committed.Aggregate.Facts.Prerequisites.Pending) != 1 || len(committed.Aggregate.Facts.Resources.OperationAdmissibilities) != 0 ||
		len(committed.Aggregate.PrerequisiteInvalidations) != 1 {
		t.Fatalf("resource change did not invalidate intersecting observation: %+v", committed.Aggregate)
	}
	if changes := committed.Aggregate.PrerequisiteInvalidations[0].ResourceChanges; len(changes) != 1 ||
		changes[0].Domain != runtimecontract.RuntimeResourceWorkspaceTree || changes[0].AfterRevision != "tree-v2" {
		t.Fatalf("invalidation did not retain owner resource-change source: %#v", changes)
	}
	duplicate, err := coordinator.CommitEvidence(ctx, evidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 ||
		duplicate.Aggregate.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 1 {
		t.Fatalf("duplicate resource-change evidence changed refresh budget: aggregate=%+v err=%v", duplicate.Aggregate, err)
	}
}

func TestAcceptedReceiptResourceChangeExhaustionSealsTerminalFact(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	_, _ = addFreshFileAdmissibilityObservation(t, &aggregate)
	aggregate.Facts.RecoveryPolicy.MaxPrerequisiteResourceChangeRefreshes = 1
	aggregate.Facts.Recovery.PrerequisiteResourceChangeRefreshes = 1

	changed, err := applyReceiptResourceChangeInvalidation(&aggregate, "dispatch-1", "write-1", runtimecontract.RuntimeOperationReceipt{
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", BeforeRevision: "tree-v1", AfterRevision: "tree-v2",
		}},
	})
	if err != nil || !changed {
		t.Fatalf("resource-change invalidation = changed:%v err:%v", changed, err)
	}
	if aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 || aggregate.Facts.Recovery.PrerequisiteResourceChangeRefreshes != 1 ||
		aggregate.Facts.Failure == nil || aggregate.Facts.Failure.Class != runtimecontract.FailureClassPrerequisiteRefreshExhausted ||
		aggregate.Facts.Failure.Code != "prerequisite_receipt_refresh_exhausted" {
		t.Fatalf("resource-change exhaustion did not preserve independent terminal fact: %+v", aggregate.Facts)
	}
	_, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
	if err != nil || decision.Action != runtimecontract.RuntimeActionFail {
		t.Fatalf("resource-change exhaustion decision = %+v err=%v", decision, err)
	}
}

func TestFileTargetChangeInvalidatesShellValidationObservation(t *testing.T) {
	fileInventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"main.go"}, ObservedRevision: "tree-v1",
	}
	fileInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileInventory)
	shellInventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:shell", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.StepKindShellCommand}, ObservedRevision: "shell-policy-v1",
	}
	shellInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(shellInventory)
	filePrerequisite := runtimecontract.RuntimePrerequisite{ID: fileInventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}}
	shellPrerequisite := runtimecontract.RuntimePrerequisite{ID: shellInventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand}}
	fileObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: filePrerequisite.ID, Kind: filePrerequisite.Kind, OwnerModuleID: filePrerequisite.OwnerModuleID, ContentHash: fileInventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v1"}},
	}
	fileObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(fileObservation)
	shellObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: shellPrerequisite.ID, Kind: shellPrerequisite.Kind, OwnerModuleID: shellPrerequisite.OwnerModuleID, ContentHash: shellInventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{
			{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", Revision: "shell-policy-v1"},
			{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "main.go", Revision: "missing"},
		},
	}
	shellObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(shellObservation)
	facts := runtimecontract.RuntimeFacts{
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{filePrerequisite, shellPrerequisite}},
		Resources: runtimecontract.RuntimeResourceFacts{
			OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{fileInventory, shellInventory},
			PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{fileObservation, shellObservation},
		},
	}
	updated, prerequisites, observations, _, err := invalidatePrerequisiteObservationsForChanges(facts, []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "main.go", AfterRevision: "main-v1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(prerequisites) != 1 || prerequisites[0] != shellPrerequisite.ID || len(observations) != 1 || observations[0] != shellObservation.ObservationID {
		t.Fatalf("File target change affected wrong prerequisites/observations: %v / %v", prerequisites, observations)
	}
	if len(updated.Prerequisites.Pending) != 1 || updated.Prerequisites.Pending[0].ID != shellPrerequisite.ID ||
		len(updated.Prerequisites.Satisfied) != 1 || updated.Prerequisites.Satisfied[0].ID != filePrerequisite.ID ||
		len(updated.Resources.OperationAdmissibilities) != 1 || updated.Resources.OperationAdmissibilities[0].PrerequisiteID != filePrerequisite.ID {
		t.Fatalf("File target change did not precisely invalidate Shell observation: %+v", updated)
	}
}

func TestOwnerCatalogChangeInvalidatesBoundFailureTargetObservation(t *testing.T) {
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:mcp", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{"server-a"}, ObservedRevision: "mcp-server-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, RequiredCapabilities: []string{runtimecontract.StepKindMCPToolCall},
	}
	prerequisiteObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", Revision: "mcp-server-v1",
		}},
	}
	prerequisiteObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(prerequisiteObservation)
	targetObservation := &runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "mcp-server-v1", TargetRefs: []string{"implementation.go"},
	}
	failure := &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassBuildFailure, Signature: "mcp-failure", Message: "observed failure", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation,
	}
	facts := runtimecontract.RuntimeFacts{
		Failure: failure,
		RepairDirective: &runtimecontract.RuntimeRepairDirective{
			ID: "repair", FocusID: "focus", Trigger: runtimecontract.RuntimeRepairTriggerRejectedValidation,
			RequiredAction: runtimecontract.RuntimeActionProduceStateChange, CurrentFailure: failure, TargetRefs: []string{"implementation.go"},
		},
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{prerequisite}},
		Resources: runtimecontract.RuntimeResourceFacts{
			FailureTarget:            &runtimecontract.FailureTarget{Signature: "mcp-failure", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation},
			OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{inventory},
			PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{prerequisiteObservation},
		},
	}
	updated, prerequisiteIDs, observationIDs, _, err := invalidatePrerequisiteObservationsForChanges(facts, []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", BeforeRevision: "mcp-server-v1", AfterRevision: "mcp-server-v2",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(prerequisiteIDs) != 1 || prerequisiteIDs[0] != prerequisite.ID || len(observationIDs) != 1 || observationIDs[0] != prerequisiteObservation.ObservationID ||
		updated.Resources.FailureTarget != nil || updated.Failure == nil || len(updated.Failure.TargetRefs) != 0 || updated.Failure.TargetObservation != nil || updated.RepairDirective != nil {
		t.Fatalf("stale owner observation was retained: facts=%+v prerequisites=%v observations=%v", updated, prerequisiteIDs, observationIDs)
	}
	if len(updated.Prerequisites.Pending) != 1 || updated.Prerequisites.Pending[0].ID != prerequisite.ID || len(updated.Resources.OperationAdmissibilities) != 0 {
		t.Fatalf("owner catalog prerequisite was not reacquired: %+v", updated)
	}
	duplicate, duplicatePrerequisites, duplicateObservations, _, err := invalidatePrerequisiteObservationsForChanges(updated, []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", BeforeRevision: "mcp-server-v1", AfterRevision: "mcp-server-v2",
	}})
	if err != nil || len(duplicatePrerequisites) != 0 || len(duplicateObservations) != 0 || len(duplicate.Prerequisites.Pending) != 1 ||
		duplicate.Resources.FailureTarget != nil || duplicate.RepairDirective != nil {
		t.Fatalf("duplicate owner catalog invalidation was not idempotent: facts=%+v prerequisites=%v observations=%v err=%v", duplicate, duplicatePrerequisites, duplicateObservations, err)
	}
}

func TestWorkspaceTreeChangeInvalidatesOwnerTargetSelectionOnce(t *testing.T) {
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "target_selection:blocks/file:deliverable:file/unique_workspace_entry:::0", Kind: runtimecontract.PrerequisiteKindTargetSelection,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "deliverable", TargetSelectorRef: "file/unique_workspace_entry",
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: prerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "deliverable", SelectorRef: "file/unique_workspace_entry",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"implementation.go"}, CandidateTargetRefs: []string{"implementation.go"}, CandidateCount: 1, ObservedRevision: "tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v1"}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts := runtimecontract.RuntimeFacts{
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{prerequisite}},
		Resources:     runtimecontract.RuntimeResourceFacts{TargetSelection: &selection, PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{observation}},
	}
	changes := []runtimecontract.RuntimeResourceChange{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", BeforeRevision: "tree-v1", AfterRevision: "tree-v2"}}
	updated, prerequisiteIDs, observationIDs, _, err := invalidatePrerequisiteObservationsForChanges(facts, changes)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Resources.TargetSelection != nil || len(updated.Prerequisites.Satisfied) != 0 || len(updated.Prerequisites.Pending) != 1 || updated.Prerequisites.Pending[0].ID != prerequisite.ID || len(prerequisiteIDs) != 1 || prerequisiteIDs[0] != prerequisite.ID || len(observationIDs) != 1 || observationIDs[0] != observation.ObservationID {
		t.Fatalf("target selection was not precisely invalidated: updated=%+v prerequisites=%v observations=%v", updated, prerequisiteIDs, observationIDs)
	}
	duplicate, duplicatePrerequisites, duplicateObservations, _, err := invalidatePrerequisiteObservationsForChanges(updated, changes)
	if err != nil || duplicate.Resources.TargetSelection != nil || len(duplicatePrerequisites) != 0 || len(duplicateObservations) != 0 || len(duplicate.Prerequisites.Pending) != 1 {
		t.Fatalf("duplicate target selection invalidation was not idempotent: facts=%+v prerequisites=%v observations=%v err=%v", duplicate, duplicatePrerequisites, duplicateObservations, err)
	}
}

func addFreshFileAdmissibilityObservation(t *testing.T, aggregate *RuntimeAggregate) (runtimecontract.RuntimePrerequisite, runtimecontract.RuntimePrerequisiteObservation) {
	t.Helper()
	aggregate.Facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	aggregate.Facts.RecoveryPolicy.MaxPrerequisiteResourceChangeRefreshes = 2
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"main.go", "other.go"}, ObservedRevision: "tree-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: inventory.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	aggregate.Facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	aggregate.Facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	return prerequisite, observation
}
