package runtime

import (
	"context"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerFactAcquisitionReceiptDoesNotRebuildConsumedProspectiveSelection(t *testing.T) {
	_, repository, bootstrapFacts, definition := prepareTerminalValidationOwnerFactAcquisitionAssessment(t, true)
	aggregate := *repository.aggregate

	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("post-acquisition state = %q, want %q", aggregate.Decision.State, runtimecontract.ConvergenceStateNeedAssessment)
	}
	if !prospectiveOwnerFactAcquisitionReceiptCommitted(aggregate.Facts, aggregate.CoverageProofLineage) {
		t.Fatal("accepted acquisition receipt was not recognized as the durable create-only selection boundary")
	}
	assertNoOwnerFactAcquisitionTargetSelection(t, aggregate.Facts, aggregate.CoverageProofLineage)

	// Exercise the receipt-time transition directly: resource invalidation may
	// requeue the old selector, but the exact accepted acquisition consumes it.
	withInvalidatedSelection := aggregate.Facts.Normalized()
	withInvalidatedSelection.Prerequisites.Pending = append(
		[]runtimecontract.RuntimePrerequisite(nil),
		withInvalidatedSelection.Prerequisites.Pending...,
	)
	selection := ownerFactAcquisitionTargetSelectionPrerequisite(t, withInvalidatedSelection)
	withInvalidatedSelection.Prerequisites.Pending = append(withInvalidatedSelection.Prerequisites.Pending, selection)
	retired, changed := retireConsumedProspectiveTargetSelection(withInvalidatedSelection, aggregate.CoverageProofLineage)
	if !changed {
		t.Fatal("matching invalidated target selection was not retired")
	}
	if prerequisiteKindPresent(retired.Prerequisites.Pending, runtimecontract.PrerequisiteKindTargetSelection) {
		t.Fatalf("consumed target selection remained pending: %+v", retired.Prerequisites.Pending)
	}
	if !prerequisiteIDPresent(retired.Prerequisites.Retired, selection.ID) {
		t.Fatalf("consumed target selection lifecycle was not preserved in retired: %+v", retired.Prerequisites.Retired)
	}
	if err := retired.Validate(); err != nil {
		t.Fatalf("retired selection facts are invalid: %v", err)
	}

	// A freshly constructed controller represents process restart. Replaying
	// initialization against the durable aggregate must neither save nor
	// recreate the already-consumed selector.
	revision, saves := repository.aggregate.StateRevision, repository.saves
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.initialize(context.Background(), bootstrapFacts, definition, nil)
	if err != nil {
		t.Fatalf("restart initialize: %v", err)
	}
	if recovered.StateRevision != revision || repository.saves != saves {
		t.Fatalf("restart changed durable aggregate: revision=%d->%d saves=%d->%d", revision, recovered.StateRevision, saves, repository.saves)
	}
	assertNoOwnerFactAcquisitionTargetSelection(t, recovered.Facts, recovered.CoverageProofLineage)

	// The original owner result is fenced by its persisted evidence identity;
	// duplicate delivery cannot regress prerequisite ordering or persist again.
	evidence := ownerFactAcquisitionReplayEvidence(t, recovered)
	duplicate, err := restarted.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatalf("duplicate acquisition receipt: %v", err)
	}
	if duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != revision || repository.saves != saves {
		t.Fatalf("duplicate receipt changed aggregate: disposition=%q revision=%d saves=%d", duplicate.Disposition, duplicate.Aggregate.StateRevision, repository.saves)
	}
	assertNoOwnerFactAcquisitionTargetSelection(t, duplicate.Aggregate.Facts, duplicate.Aggregate.CoverageProofLineage)
}

func TestOwnerFactAcquisitionSelectionSuppressionRejectsStaleSourceLineage(t *testing.T) {
	_, repository, _, _ := prepareTerminalValidationOwnerFactAcquisitionAssessment(t, true)
	aggregate := *repository.aggregate
	if len(aggregate.CoverageProofLineage) == 0 {
		t.Fatal("owner-fact acquisition fixture has no source proof lineage")
	}

	stale := append([]runtimecontract.RuntimeCoverageProofLineage(nil), aggregate.CoverageProofLineage...)
	stale[0] = stale[0].Clone()
	if stale[0].Kind != runtimecontract.RuntimeCoverageProofLineageKindSource {
		t.Fatalf("first normalized owner-fact lineage is not a source row: %+v", stale[0])
	}
	stale[0].SourceBindingHash = "stale-owner-fact-acquisition-binding"
	stale[0].ID = runtimecontract.StableRuntimeCoverageProofLineageSourceID(
		stale[0].SourceReceiptHash,
		stale[0].SourceEvidenceID,
		stale[0].SourceBindingHash,
		stale[0].SourceProofIdentity.Hash,
	)
	if err := stale[0].Validate(); err != nil {
		t.Fatalf("stale source lineage fixture is not independently valid: %v", err)
	}
	if prospectiveOwnerFactAcquisitionReceiptCommitted(aggregate.Facts, stale) {
		t.Fatal("stale source binding lineage suppressed target grounding")
	}
	requirements := runtimeTargetGroundingRequirements(aggregate.Facts, stale)
	if !requirementKindPresent(requirements, runtimecontract.PrerequisiteKindTargetSelection) {
		t.Fatalf("stale source lineage did not fail closed to target selection: %+v", requirements)
	}
}

func ownerFactAcquisitionTargetSelectionPrerequisite(t *testing.T, facts runtimecontract.RuntimeFacts) runtimecontract.RuntimePrerequisite {
	t.Helper()
	focus := facts.Focus.Normalized()
	if len(focus.ProspectiveTargetRefs) != 1 {
		t.Fatalf("owner-fact acquisition focus prospective targets = %v", focus.ProspectiveTargetRefs)
	}
	for _, requirement := range runtimeTargetGroundingRequirements(facts) {
		if requirement.Kind != runtimecontract.PrerequisiteKindTargetSelection {
			continue
		}
		return runtimePrerequisiteFromRequirement(requirement)
	}
	t.Fatal("owner-fact acquisition target selection requirement is missing without durable lineage")
	return runtimecontract.RuntimePrerequisite{}
}

func ownerFactAcquisitionReplayEvidence(t *testing.T, aggregate RuntimeAggregate) ExecutionEvidence {
	t.Helper()
	for _, fact := range aggregate.Facts.Evidence.Accepted {
		if fact.Kind != runtimecontract.EvidenceKindStateChange || fact.FocusID != aggregate.Facts.Focus.ID {
			continue
		}
		evidence := ExecutionEvidence{
			EventID: string(fact.ID), WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			DispatchID: fact.DispatchID, AuthoritySnapshotID: fact.AuthoritySnapshotID,
			OperationID: fact.OperationID, Kind: fact.Kind, Status: ExecutionEvidenceStatusAccepted,
			SourceRef: fact.SourceRef,
		}
		for _, raw := range aggregate.OperationReceipts {
			receipt := raw.Normalized()
			if receipt.OperationID != fact.OperationID {
				continue
			}
			evidence.Receipt = &receipt
			evidence.ObservedEffectScope = receipt.ObservedEffectScope
			evidence.OwnerReceiptRef = fact.SourceRef
			evidence.OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
			return evidence
		}
		t.Fatalf("owner-fact acquisition receipt for operation %q is missing", fact.OperationID)
	}
	t.Fatal("owner-fact acquisition accepted evidence is missing")
	return ExecutionEvidence{}
}

func assertNoOwnerFactAcquisitionTargetSelection(t *testing.T, facts runtimecontract.RuntimeFacts, lineage []runtimecontract.RuntimeCoverageProofLineage) {
	t.Helper()
	if facts.Resources.TargetSelection != nil {
		t.Fatalf("consumed target selection remains current: %+v", facts.Resources.TargetSelection)
	}
	if prerequisiteKindPresent(facts.Prerequisites.Pending, runtimecontract.PrerequisiteKindTargetSelection) ||
		prerequisiteKindPresent(facts.Prerequisites.Satisfied, runtimecontract.PrerequisiteKindTargetSelection) {
		t.Fatalf("consumed target selection remains active: pending=%+v satisfied=%+v", facts.Prerequisites.Pending, facts.Prerequisites.Satisfied)
	}
	if requirementKindPresent(runtimeTargetGroundingRequirements(facts, lineage), runtimecontract.PrerequisiteKindTargetSelection) {
		t.Fatalf("reducer rebuilt consumed target selection: %+v", runtimeTargetGroundingRequirements(facts, lineage))
	}
}

func prerequisiteKindPresent(prerequisites []runtimecontract.RuntimePrerequisite, kind runtimecontract.PrerequisiteKind) bool {
	for _, prerequisite := range prerequisites {
		if prerequisite.Kind == kind {
			return true
		}
	}
	return false
}

func requirementKindPresent(requirements []runtimecontract.RuntimeRequirement, kind runtimecontract.PrerequisiteKind) bool {
	for _, requirement := range requirements {
		if requirement.Kind == kind {
			return true
		}
	}
	return false
}

func prerequisiteIDPresent(prerequisites []runtimecontract.RuntimePrerequisite, id string) bool {
	for _, prerequisite := range prerequisites {
		if prerequisite.ID == id {
			return true
		}
	}
	return false
}
