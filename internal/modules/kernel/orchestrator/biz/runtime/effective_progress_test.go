package runtime

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAcceptedValidationEvidencePreservesRepairLedgerAndUsesAssessmentRevisionFreshness(t *testing.T) {
	aggregate := validationAggregate(t)
	aggregate.Facts.Focus.RequireAssessment = true
	aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID, SliceID: aggregate.Facts.Focus.SliceID,
		RepairAttempts: 1, ValidationOnlyRetries: 1, LastCycleID: "repair-cycle-1",
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
	}
	priorEvidenceRevision := aggregate.Facts.EvidenceRevision
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
	// Assessment facts are an immutable audit ledger. Freshness is a reducer
	// comparison against the later accepted receipt revision, never a deletion
	// side effect in the evidence commit transaction. The typed global-ledger
	// tests separately prove that persisted assessments have dispatch lineage.
	auditFacts := committed.Aggregate.Facts
	auditFacts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-old", Revision: priorEvidenceRevision, FocusID: auditFacts.Focus.ID,
		Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Gap:         &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
	}}
	if _, current := assessmentForCurrentFocus(auditFacts); current || len(auditFacts.Assessments) != 1 {
		t.Fatalf("accepted validation did not supersede the immutable prior assessment by revision: %+v", auditFacts.Assessments)
	}
}

func TestAcceptedDurableStateChangeReopensValidationWindowWithoutResettingMandatoryAssessmentRepairDebt(t *testing.T) {
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
	if ledger.RepairAttempts != 2 || ledger.ValidationOnlyRetries != 0 || ledger.ActiveGap == nil || ledger.LastCycleID != "repair-cycle-2" || ledger.LastProgressRevision != "" ||
		committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("durable state change reset progress before validation/assessment: ledger=%+v state=%s terminal=%+v failure=%+v", ledger, committed.Aggregate.Decision.State, committed.Aggregate.Decision.Terminal, committed.Aggregate.Facts.Failure)
	}
}

func TestChangedStateChangeWithoutExactCoverageCreditDoesNotResetRepairProgressWindow(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID, SliceID: aggregate.Facts.Focus.SliceID,
		RepairAttempts: 2, ValidationOnlyRetries: 1, LastCycleID: "repair-cycle-no-credit",
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{Reason: "repair remains open"},
	}
	fact := runtimecontract.EvidenceFact{
		ID: "accepted-diagnostic-state-change", Kind: runtimecontract.EvidenceKindStateChange,
		BindingRefs: []string{aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID},
		TargetRefs:  append([]string(nil), aggregate.Facts.Focus.TargetRefs...),
		// CoverageWorkItemIDs is intentionally empty: the receipt was observed,
		// but did not match the sealed proof strongly enough to receive credit.
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		Changed: true, TargetRefs: append([]string(nil), aggregate.Facts.Focus.TargetRefs...),
	}
	evidence := ExecutionEvidence{Status: ExecutionEvidenceStatusAccepted, Receipt: &receipt}
	if runtimeEvidenceIsEffectiveProgress(aggregate.Facts, fact, evidence, aggregate.CoverageProofLineage) {
		t.Fatal("changed diagnostic receipt without exact CWI credit was treated as effective progress")
	}
	before := aggregate.Facts.Repair
	applyAcceptedEvidenceProgress(&aggregate, executionEvidenceAdmission{Evidence: evidence}, fact, false)
	if aggregate.Facts.Repair.LastProgressRevision != before.LastProgressRevision ||
		aggregate.Facts.Repair.ValidationOnlyRetries != before.ValidationOnlyRetries ||
		aggregate.Facts.Repair.ActiveGap == nil || aggregate.Facts.Repair.LastCycleID != before.LastCycleID {
		t.Fatalf("empty-credit state-change reset repair ledger: before=%+v after=%+v", before, aggregate.Facts.Repair)
	}
}

func TestAcceptedDurableStateChangeRequiringAssessmentDoesNotResetRepairProgressWindow(t *testing.T) {
	aggregate := stateChangeAggregateWithPreparation(t, func(initial *RuntimeAggregate) {
		// Isolate the post-state-change assessment boundary before SM-2 seals
		// the provider/owner preimage. The shared fixture normally also requires
		// validation, which correctly precedes mandatory assessment.
		initial.Facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
		initial.Facts.Focus.RequireAssessment = true
	})
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
	return stateChangeAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Focus.TargetRefs = append([]string(nil), targetRefs...)
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
		addCandidateAdmissionTargetPolicyFixture(
			&aggregate.Facts,
			runtimecontract.RuntimeModuleFile,
			runtimecontract.RuntimeBlockKindPatchApply,
			"file_write",
			runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			targetRefs,
		)
	})
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
