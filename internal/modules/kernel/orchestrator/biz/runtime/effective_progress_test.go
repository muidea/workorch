package runtime

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAcceptedValidationEvidenceInvalidatesAssessmentWithoutResettingRepairLedger(t *testing.T) {
	aggregate := validationAggregate(t)
	aggregate.Facts.Focus.RequireAssessment = true
	aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID, SliceID: aggregate.Facts.Focus.SliceID,
		RepairAttempts: 1, ValidationOnlyRetries: 1, LastCycleID: "repair-cycle-1",
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
	}
	aggregate.Facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-old", FocusID: aggregate.Facts.Focus.ID, Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Gap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
	}}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	evidence := ExecutionEvidence{
		EventID: "event-validation-valid", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusAccepted,
		Evidence: runtimecontract.RuntimeOperationEvidence{Quality: common.EvidenceQualityValid, Validation: &runtimecontract.RuntimeValidationObservation{
			ExecutedChecks: []string{"check-1"}, PassedChecks: []string{"check-1"},
		}},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	ledger := committed.Aggregate.Facts.Repair
	if ledger.ValidationOnlyRetries != 1 || ledger.RepairAttempts != 1 || ledger.ActiveGap == nil || ledger.LastCycleID != "repair-cycle-1" {
		t.Fatalf("accepted validation reset repair ledger: %+v", ledger)
	}
	if len(committed.Aggregate.Facts.Assessments) != 0 {
		t.Fatalf("accepted validation did not invalidate stale assessment: %+v", committed.Aggregate.Facts.Assessments)
	}
}

func TestAcceptedDurableStateChangeResetsRepairProgressWindow(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID, SliceID: aggregate.Facts.Focus.SliceID,
		RepairAttempts: 2, ValidationOnlyRetries: 1, LastCycleID: "repair-cycle-2",
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{Reason: "repair"},
	}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, stateChangeCandidate())
	evidence := ExecutionEvidence{
		EventID: "event-state-progress", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	ledger := committed.Aggregate.Facts.Repair
	if ledger.RepairAttempts != 2 || ledger.ValidationOnlyRetries != 0 || ledger.ActiveGap != nil || ledger.LastCycleID != "" || ledger.LastProgressRevision == "" {
		t.Fatalf("durable state progress ledger = %+v", ledger)
	}
}

func TestAcceptedDurableStateChangeRequiringAssessmentDoesNotResetRepairProgressWindow(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequireAssessment = true
	// Isolate the post-state-change assessment boundary. The shared fixture
	// normally also requires validation, in which case SM-2 correctly requests
	// validation before assessment and this test would assert the wrong action.
	aggregate.Facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	runtimeTestEnsureCurrentFocusBindings(t, &aggregate)
	aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID, SliceID: aggregate.Facts.Focus.SliceID,
		RepairAttempts: 2, ValidationOnlyRetries: 1, LastCycleID: "repair-cycle-2",
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{Reason: "repair"},
	}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, stateChangeCandidate())
	evidence := ExecutionEvidence{
		EventID: "event-state-await-assessment", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	ledger := committed.Aggregate.Facts.Repair
	if ledger.ValidationOnlyRetries != 1 || ledger.ActiveGap == nil || ledger.LastProgressRevision != "" || committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("assessment-required state change reset repair before assessment: ledger=%+v decision=%+v", ledger, committed.Aggregate.Decision)
	}
}

func effectiveProgressOwnerScopeAggregate(t *testing.T, targetRefs []string) RuntimeAggregate {
	t.Helper()
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.TargetRefs = nil
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "deliverable"
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection:effective-progress-owner-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs:  append([]string(nil), targetRefs...), CandidateTargetRefs: append([]string(nil), targetRefs...),
		CandidateCount: len(targetRefs), ObservedRevision: "tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection
	runtimeTestSealCurrentFocusBindingTargetSurface(&aggregate.Facts, runtimeTestBindingTargetSurface{
		SelectorRef: selection.SelectorRef, SelectorOwnerRef: selection.OwnerModuleID, SelectorSubjectRef: selection.TargetRole,
	})
	bindCurrentFocusTargetSelection(&aggregate.Facts)
	addCandidateAdmissionTargetPolicyFixture(
		&aggregate.Facts,
		runtimecontract.RuntimeModuleFile,
		runtimecontract.RuntimeBlockKindPatchApply,
		"file_write",
		runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
		targetRefs,
	)
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	return aggregate
}

func TestAcceptedDurableStateChangeOutsideFailureTargetDoesNotResetRepairProgressWindow(t *testing.T) {
	aggregate := effectiveProgressOwnerScopeAggregate(t, []string{"implementation.go", "validation_entry.go"})
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Signature: "failed-validation",
		Message: "validation still fails", TargetRefs: []string{"implementation.go"},
	}
	aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: "failed-validation", TargetRefs: []string{"implementation.go"}}
	aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID, SliceID: aggregate.Facts.Focus.SliceID,
		RepairAttempts: 2, ValidationOnlyRetries: 1, LastCycleID: "repair-cycle-2",
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{Reason: "repair", TargetRefs: []string{"implementation.go"}},
	}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	// The sealed operation changes a different bounded workspace target. The
	// receipt is durable and accepted, but it cannot reset this repair cycle.
	candidate := stateChangeCandidate()
	candidate.StateChange.Operations[0].Operation.TargetRefs = []string{"validation_entry.go"}
	candidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters("validation_entry.go")
	sealed := sealCandidateForTest(t, repository, candidate)
	evidence := ExecutionEvidence{
		EventID: "event-state-unrelated", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	ledger := committed.Aggregate.Facts.Repair
	if ledger.ValidationOnlyRetries != 1 || ledger.ActiveGap == nil || ledger.LastCycleID != "repair-cycle-2" || ledger.LastProgressRevision != "" {
		t.Fatalf("unrelated durable state change reset repair ledger: %+v", ledger)
	}
}

func TestAcceptedDurableStateChangeOutsideSelectedTargetDoesNotResetRepairProgressWindow(t *testing.T) {
	aggregate := effectiveProgressOwnerScopeAggregate(t, []string{"implementation.go", "validation-entry.go"})
	aggregate.Facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-selected-implementation", FocusID: aggregate.Facts.Focus.ID,
		Trigger: runtimecontract.RuntimeRepairTriggerAssessmentGap, RequiredState: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
		CoverageWorkItemIDs:   append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...),
		TargetRefs:            []string{"implementation.go"},
	}
	aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID, SliceID: aggregate.Facts.Focus.SliceID,
		RepairAttempts: 2, ValidationOnlyRetries: 1, LastCycleID: "repair-cycle-2",
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{Reason: "repair", TargetRefs: []string{"implementation.go"}},
	}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	candidate := stateChangeCandidate()
	candidate.StateChange.Operations[0].Operation.TargetRefs = []string{"validation-entry.go"}
	candidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters("validation-entry.go")
	sealed := sealCandidateForTest(t, repository, candidate)
	evidence := ExecutionEvidence{
		EventID: "event-state-outside-selected-target", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	ledger := committed.Aggregate.Facts.Repair
	if ledger.ValidationOnlyRetries != 1 || ledger.ActiveGap == nil || ledger.LastProgressRevision != "" {
		t.Fatalf("outside selected target reset repair progress: %+v", ledger)
	}
}
