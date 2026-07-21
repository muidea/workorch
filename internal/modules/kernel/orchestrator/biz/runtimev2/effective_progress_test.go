package runtimev2

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
		OperationID: "validate-1", Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusAccepted,
		Evidence: runtimecontract.RuntimeOperationEvidence{Quality: common.EvidenceQualityValid, Validation: &runtimecontract.RuntimeValidationObservation{
			ExecutedChecks: []string{"check-1"}, PassedChecks: []string{"check-1"},
		}},
	}
	attachRuntimeV2TestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.CommitEvidence(context.Background(), evidence)
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
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeV2TestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	ledger := committed.Aggregate.Facts.Repair
	if ledger.RepairAttempts != 2 || ledger.ValidationOnlyRetries != 0 || ledger.ActiveGap != nil || ledger.LastCycleID != "" || ledger.LastProgressRevision == "" {
		t.Fatalf("durable state progress ledger = %+v", ledger)
	}
}
