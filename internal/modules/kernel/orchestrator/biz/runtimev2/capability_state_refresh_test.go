package runtimev2

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestStaleCapabilityStateReceiptRefreshesPrerequisiteWithoutRepairDebt(t *testing.T) {
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindGitWrite}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindGitWrite}
	facts.Resources.CapabilityStates = []runtimecontract.RuntimeCapabilityStateSnapshot{{
		RequirementID: "capability_state:git", SnapshotID: "capability-state-git", AcquisitionKey: "capability-state/ws-v2/run-v2/capability_state:git/0",
		QueryReceiptRef: "capability-state-receipt/ws-v2/run-v2/capability_state:git/0", OwnerModuleID: runtimecontract.RuntimeModuleGit,
		CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", ObservedRevision: "repo-absent", ContentHash: "hash-before",
		Disposition:               runtimecontract.RuntimeCapabilityStateSequenceRequired,
		RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"}},
	}}
	aggregate, err := controller.Initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	init := "init"
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "git-init", Kind: runtimecontract.StepKindGitWrite, RequiredOperationRef: "repo-init", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &init}},
	}}}}})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", "git-init")
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, "git-init")
	if !ok {
		t.Fatal("claimed git execution missing")
	}
	failure := &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Code: "capability_state_stale", Signature: "capability-state-stale:capability-state-git", Message: "owner state changed"}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: "git-init", Outcome: runtimecontract.RuntimeOperationOutcomeRejected,
		ObservedEffectScope: runtimecontract.RuntimeEffectNone, Failure: failure,
		CapabilityStateStale: &runtimecontract.RuntimeCapabilityStateStaleDetail{
			StateRef:   runtimecontract.RuntimeCapabilityStateRef{OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable"},
			SnapshotID: "capability-state-git", ExpectedHash: "hash-before", ObservedRevision: "repo-initialized", ObservedHash: "hash-after", Reason: "owner state changed",
		},
	}
	receipt = attachOperationReceiptFeedbackForTest(t, claimed, receipt)
	if _, persisted, err := controller.CommitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, receipt); err != nil {
		t.Fatal(err)
	} else {
		receipt = persisted
	}
	refreshed, err := controller.RefreshCapabilityStateAfterStaleReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ActiveDispatch != nil || refreshed.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(refreshed.Facts.Resources.CapabilityStates) != 0 ||
		len(refreshed.Facts.Prerequisites.Pending) != 1 || refreshed.Facts.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindCapabilityState ||
		refreshed.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("stale refresh did not atomically return to prerequisite: %+v", refreshed)
	}
	if refreshed.Facts.Repair.RepairAttempts != 0 || len(refreshed.Facts.Repair.SignatureCounts) != 0 || len(refreshed.Facts.Evidence.Rejected) != 0 {
		t.Fatalf("stale refresh consumed ordinary repair/evidence authority: %+v", refreshed.Facts)
	}
	saves := repository.saves
	duplicate, err := controller.RefreshCapabilityStateAfterStaleReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, receipt)
	if err != nil || duplicate.StateRevision != refreshed.StateRevision || duplicate.Facts.Recovery.PrerequisiteRefreshes != 1 || repository.saves != saves {
		t.Fatalf("duplicate stale receipt was not idempotent: aggregate=%+v saves=%d want=%d err=%v", duplicate, repository.saves, saves, err)
	}
	newSnapshot := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: "capability_state:git", AcquisitionKey: "capability-state/ws-v2/run-v2/capability_state:git/1",
		QueryReceiptRef: "capability-state-receipt/capability-state/ws-v2/run-v2/capability_state:git/1", OwnerModuleID: runtimecontract.RuntimeModuleGit,
		CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", ObservedRevision: "repo-initialized", ContentHash: "hash-after",
		Disposition: runtimecontract.RuntimeCapabilityStateReady,
	}
	newSnapshot.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(newSnapshot)
	reacquired, err := controller.CommitPrerequisite(context.Background(), "ws-v2", "run-v2", PrerequisiteResult{PrerequisiteID: "capability_state:git", CapabilityState: &newSnapshot})
	if err != nil || reacquired.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("reacquired capability-state snapshot=%+v err=%v", reacquired, err)
	}
	newDispatch, err := beginAndAcceptCandidate(t, controller, reacquired, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "git-init-new", Kind: runtimecontract.StepKindGitWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		TargetRefs: []string{"target.txt"}, Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &init}},
	}}}}})
	if err != nil || newDispatch.ActiveDispatch == nil {
		t.Fatalf("new sealed dispatch after refresh=%+v err=%v", newDispatch, err)
	}
	saves = repository.saves
	late, err := controller.RefreshCapabilityStateAfterStaleReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, receipt)
	if err != nil || late.ActiveDispatch == nil || late.ActiveDispatch.ID != newDispatch.ActiveDispatch.ID || late.StateRevision != newDispatch.StateRevision || repository.saves != saves {
		t.Fatalf("late stale receipt changed newer dispatch: aggregate=%+v saves=%d want=%d err=%v", late, repository.saves, saves, err)
	}
	mismatched := receipt
	mismatched.CapabilityStateStale = &runtimecontract.RuntimeCapabilityStateStaleDetail{
		StateRef: receipt.CapabilityStateStale.StateRef, SnapshotID: receipt.CapabilityStateStale.SnapshotID,
		ExpectedHash: "different-snapshot-hash", ObservedRevision: receipt.CapabilityStateStale.ObservedRevision,
		ObservedHash: receipt.CapabilityStateStale.ObservedHash, Reason: receipt.CapabilityStateStale.Reason,
	}
	if _, err := controller.RefreshCapabilityStateAfterStaleReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, mismatched); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched stale receipt err=%v, want conflict", err)
	}
	final, err := controller.Load(context.Background(), "ws-v2", "run-v2")
	if err != nil || final.StateRevision != newDispatch.StateRevision || final.ActiveDispatch == nil || final.ActiveDispatch.ID != newDispatch.ActiveDispatch.ID || final.Facts.Recovery.PrerequisiteRefreshes != 1 || repository.saves != saves {
		t.Fatalf("mismatched stale receipt changed authority: aggregate=%+v saves=%d want=%d err=%v", final, repository.saves, saves, err)
	}
}
