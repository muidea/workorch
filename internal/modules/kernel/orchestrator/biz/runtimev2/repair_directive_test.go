package runtimev2

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRepairDirectiveSurvivesPrerequisiteReacquisition(t *testing.T) {
	// Mirrors the production failure chain: a validation candidate is infeasible,
	// SM-2 selects a state-change repair, and a later rejected repair invalidates
	// its target snapshot. The prerequisite interlude must not re-open validation.
	facts := validationFacts()
	facts.Focus.TargetRefs = []string{"lsp_manager.go"}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassCandidateInfeasible,
		Code:  "validation_target_missing", Signature: "missing-test-target",
		Message: "validation target is not executable",
	}

	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange || facts.RepairDirective == nil {
		t.Fatalf("initial repair = decision=%+v directive=%+v", decision, facts.RepairDirective)
	}

	facts = PreflightRuntimeFacts(facts, []RuntimeRequirement{{
		Kind: runtimecontract.PrerequisiteKindTargetSnapshot, SubjectRef: "lsp_manager.go", Reason: "repair requires a current target snapshot",
	}})
	facts, decision, err = ReduceRuntimeStateWithDirective(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionAcquirePrerequisite {
		t.Fatalf("prerequisite decision = %+v", decision)
	}
	if facts.RepairDirective == nil || facts.RepairDirective.RequiredAction != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("prerequisite acquisition lost state-change repair directive: %+v", facts.RepairDirective)
	}

	prerequisiteID := facts.Prerequisites.Pending[0].ID
	facts, err = CommitPrerequisiteResult(facts, PrerequisiteResult{
		PrerequisiteID: prerequisiteID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{
			TargetRef: "lsp_manager.go", Content: "package main\n", ContentHash: "snapshot-2", ContentState: runtimecontract.SnapshotContentStateComplete,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	facts, decision, err = ReduceRuntimeStateWithDirective(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("post-prerequisite repair action = %s, want produce_state_change", decision.Action)
	}
	if facts.RepairDirective == nil || facts.RepairDirective.RequiredAction != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("post-prerequisite directive = %+v", facts.RepairDirective)
	}
}

func TestRepairDirectiveFlipsOnlyAfterAcceptedStateChange(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-cwi-1-produce_state_change-0", FocusID: facts.Focus.ID,
		Trigger:                    runtimecontract.RuntimeRepairTriggerCandidateInfeasible,
		RequiredAction:             runtimecontract.RuntimeActionProduceStateChange,
		RequiredEffectScopes:       []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds:      []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds:      []string{runtimecontract.RuntimeBlockKindPatchApply},
		CoverageWorkItemIDs:        facts.Focus.CoverageWorkItemIDs,
		SupersedesEvidenceRevision: "evidence-v2-00000000000000000002",
	}
	// A rejected state-change does not complete or flip the repair continuation.
	facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-patch", Kind: runtimecontract.EvidenceKindStateChange,
		Revision: "evidence-v2-00000000000000000003", FailureClass: runtimecontract.FailureClassEffectMismatch,
		FailureMessage: "hunk context mismatch", CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts.EvidenceRevision = "evidence-v2-00000000000000000003"
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("rejected repair action = %s", decision.Action)
	}

	facts.Evidence.Accepted = append(facts.Evidence.Accepted, runtimecontract.EvidenceFact{
		ID: "accepted-repair", Kind: runtimecontract.EvidenceKindStateChange,
		Revision: "evidence-v2-00000000000000000004", CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	})
	facts.EvidenceRevision = "evidence-v2-00000000000000000004"
	facts, decision, err = ReduceRuntimeStateWithDirective(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceValidation || facts.RepairDirective == nil || facts.RepairDirective.RequiredAction != runtimecontract.RuntimeActionProduceValidation {
		t.Fatalf("accepted repair did not advance to validation: decision=%+v directive=%+v", decision, facts.RepairDirective)
	}
}

func TestNormalStateChangeDoesNotCreateRepairDirective(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange || facts.RepairDirective != nil {
		t.Fatalf("normal state change = decision=%+v directive=%+v", decision, facts.RepairDirective)
	}
}

func TestRejectedValidationSupersedesPersistedValidationDirectiveAndFeasibility(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Activation.ExecutionContract.ValidationPolicies = []runtimecontract.RuntimeValidationPolicy{{
		ID:                                  "validation-repair",
		Scope:                               "focus",
		AppliesToEvidenceKinds:              []string{runtimecontract.EvidenceKindValidation},
		ValidationFailureImpliesStateChange: true,
	}}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-state-change", Kind: runtimecontract.EvidenceKindStateChange,
		Revision: "evidence-v2-00000000000000000002", CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-validation", Kind: runtimecontract.EvidenceKindValidation,
		Revision: "evidence-v2-00000000000000000004", FailureClass: runtimecontract.FailureClassExecutionFailure,
		FailureCode: "shell_exit_nonzero", FailureMessage: "validation exited non-zero",
		CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts.EvidenceRevision = "evidence-v2-00000000000000000004"
	facts.Feasibility = &runtimecontract.RuntimeFeasibilityFact{
		FocusID: facts.Focus.ID, Action: runtimecontract.RuntimeActionProduceValidation,
		RequiredOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		Reason:                 "previous validation action has no feasible operation",
	}
	facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-cwi-1-produce_validation-1", FocusID: facts.Focus.ID,
		Trigger:                    runtimecontract.RuntimeRepairTriggerMissingEvidence,
		RequiredAction:             runtimecontract.RuntimeActionProduceValidation,
		RequiredEffectScopes:       []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
		RequiredEvidenceKinds:      []string{runtimecontract.EvidenceKindValidation},
		AllowedOperationKinds:      []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		CoverageWorkItemIDs:        facts.Focus.CoverageWorkItemIDs,
		SupersedesEvidenceRevision: "evidence-v2-00000000000000000002",
	}

	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange || decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("decision = %+v, want produce_state_change without governance", decision)
	}
	if facts.RepairDirective == nil || facts.RepairDirective.RequiredAction != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("directive = %+v, want superseding state-change repair", facts.RepairDirective)
	}
	if facts.RepairDirective.Trigger != runtimecontract.RuntimeRepairTriggerRejectedValidation {
		t.Fatalf("directive trigger = %s, want rejected_validation", facts.RepairDirective.Trigger)
	}
}
