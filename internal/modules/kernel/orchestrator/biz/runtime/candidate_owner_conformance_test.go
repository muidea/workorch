package runtime

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerCandidateConformanceTransitionIsAtomicRestartSafeAndBounded(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeOwnerConstraintAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := controller.BeginCandidateAttemptForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "owner-conformance-prefix"); err != nil {
		t.Fatal(err)
	}
	firstCandidate := runtimeUnifiedDiffCandidate("first malformed diff")
	firstAdmission, err := controller.AdmitCandidateProposalForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, firstCandidate)
	if err != nil || !firstAdmission.Admission.Accepted() {
		t.Fatalf("persist first candidate proposal: failure=%+v correction=%+v err=%v", firstAdmission.Admission.Failure, firstAdmission.Admission.Correction, err)
	}
	firstRecord := runtimeOwnerConstraintNonconformanceRecord(t, firstAdmission.Aggregate, 0)
	revisionBeforeFirst := firstAdmission.Aggregate.StateRevision
	savesBeforeFirst := repository.saves
	firstClosed, err := controller.candidateDispatch.recordCandidateConformance(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, firstRecord)
	if err != nil {
		t.Fatal(err)
	}
	firstAttempt := runtimeCandidateAttemptForNumber(t, firstClosed, 0)
	if firstClosed.StateRevision != revisionBeforeFirst+1 || repository.saves != savesBeforeFirst+1 ||
		firstAttempt.State != runtimecontract.RuntimeCandidateAttemptStateFailed || len(firstAttempt.Conformances) != 1 ||
		firstAttempt.Failure == nil || firstAttempt.Failure.Code != "candidate_owner_constraint_nonconformant" ||
		firstAttempt.Correction == nil || firstAttempt.Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce ||
		firstAttempt.Correction.Witness == nil || firstAttempt.Correction.Witness.ConstraintRef != firstRecord.Observation.Violation.ConstraintRef ||
		firstClosed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || firstClosed.ActiveDispatch != nil {
		t.Fatalf("attempt 0 conformance closure is not one atomic bounded retry: aggregate=%+v attempt=%+v", firstClosed.Decision, firstAttempt)
	}

	// Response loss before the caller observes the CAS must not create a second
	// write, and a newly constructed controller must see the same persisted fact.
	savesAfterFirst := repository.saves
	duplicate, err := controller.candidateDispatch.recordCandidateConformance(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, firstRecord)
	if err != nil || duplicate.StateRevision != firstClosed.StateRevision || repository.saves != savesAfterFirst {
		t.Fatalf("same-controller duplicate conformance: revision=%d saves=%d err=%v", duplicate.StateRevision, repository.saves, err)
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.candidateDispatch.recordCandidateConformance(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, firstRecord)
	if err != nil || replayed.StateRevision != firstClosed.StateRevision || repository.saves != savesAfterFirst {
		t.Fatalf("restart duplicate conformance: revision=%d saves=%d err=%v", replayed.StateRevision, repository.saves, err)
	}

	if _, err := restarted.BeginCandidateAttemptForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, "owner-conformance-prefix"); err != nil {
		t.Fatal(err)
	}
	secondCandidate := runtimeUnifiedDiffCandidate("second malformed diff")
	secondAdmission, err := restarted.AdmitCandidateProposalForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, secondCandidate)
	if err != nil || !secondAdmission.Admission.Accepted() {
		t.Fatalf("persist corrected candidate proposal: failure=%+v correction=%+v err=%v", secondAdmission.Admission.Failure, secondAdmission.Admission.Correction, err)
	}
	secondRecord := runtimeOwnerConstraintNonconformanceRecord(t, secondAdmission.Aggregate, 1)
	revisionBeforeSecond := secondAdmission.Aggregate.StateRevision
	terminal, err := restarted.candidateDispatch.recordCandidateConformance(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, secondRecord)
	if err != nil {
		t.Fatal(err)
	}
	secondAttempt := runtimeCandidateAttemptForNumber(t, terminal, 1)
	if terminal.StateRevision != revisionBeforeSecond+1 || secondAttempt.State != runtimecontract.RuntimeCandidateAttemptStateFailed ||
		secondAttempt.Correction == nil || secondAttempt.Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal ||
		secondAttempt.Correction.TerminalCode != "candidate_retry_exhausted" ||
		terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "candidate_retry_exhausted" ||
		terminal.ActiveDispatch != nil {
		t.Fatalf("attempt 1 did not atomically exhaust without dispatch: aggregate=%+v attempt=%+v", terminal.Decision, secondAttempt)
	}

	// Even after SM-2 becomes terminal, exact re-delivery is identified by the
	// persisted logical request, not confused with a future authority generation.
	savesAfterTerminal := repository.saves
	terminalReplay, err := restarted.candidateDispatch.recordCandidateConformance(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, secondRecord)
	if err != nil || terminalReplay.StateRevision != terminal.StateRevision || repository.saves != savesAfterTerminal {
		t.Fatalf("terminal duplicate conformance: revision=%d saves=%d err=%v", terminalReplay.StateRevision, repository.saves, err)
	}
}

func TestOwnerCandidateConformantFactSurvivesRestartAndAllowsDispatchSeal(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeOwnerConstraintAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "owner-conformant-prefix"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimeUnifiedDiffCandidate("--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n")
	proposal, err := controller.AdmitCandidateProposalForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil || !proposal.Admission.Accepted() {
		t.Fatalf("persist conformant proposal: failure=%+v err=%v", proposal.Admission.Failure, err)
	}
	record := runtimeOwnerConstraintConformanceRecord(t, proposal.Aggregate, 0, runtimecontract.RuntimeOperationCandidateConformant)
	persisted, err := controller.candidateDispatch.recordCandidateConformance(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, record)
	if err != nil {
		t.Fatal(err)
	}
	attempt := runtimeCandidateAttemptForNumber(t, persisted, 0)
	if attempt.State != runtimecontract.RuntimeCandidateAttemptStatePending || len(attempt.Conformances) != 1 || attempt.Failure != nil || attempt.Correction != nil || persisted.ActiveDispatch != nil {
		t.Fatalf("conformant owner fact changed candidate state: %+v", attempt)
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	saves := repository.saves
	if duplicate, err := restarted.candidateDispatch.recordCandidateConformance(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, record); err != nil || duplicate.StateRevision != persisted.StateRevision || repository.saves != saves {
		t.Fatalf("restart conformant duplicate: revision=%d saves=%d err=%v", duplicate.StateRevision, repository.saves, err)
	}
	if err := RecordFixtureCandidateFeasibilities(ctx, restarted, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate); err != nil {
		t.Fatal(err)
	}
	sealed, err := restarted.AcceptCandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil || sealed.ActiveDispatch == nil {
		t.Fatalf("seal candidate with persisted conformance: dispatch=%+v err=%v", sealed.ActiveDispatch, err)
	}
	accepted := runtimeCandidateAttemptForNumber(t, sealed, 0)
	if accepted.State != runtimecontract.RuntimeCandidateAttemptStateAccepted || len(accepted.Conformances) != 0 {
		t.Fatalf("accepted attempt retained pre-seal conformance facts: %+v", accepted)
	}
}

func runtimeOwnerConstraintAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	return stateChangeAggregateWithPreparation(t, func(initial *RuntimeAggregate) {
		initial.Facts.Focus.AcceptedOperationVariants = []runtimecontract.RuntimeOperationVariantRef{{
			Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "unified_diff",
		}}
		catalog := runtimecontract.RuntimeOperationAdmissibility{
			PrerequisiteID: "test-owner-conformance-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
				Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "unified_diff",
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
			}},
			TargetRefs: []string{"main.go"}, ObservedRevision: "test-owner-conformance-file-v1",
		}
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
		initial.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	})
}

func runtimeUnifiedDiffCandidate(diff string) runtimecontract.RuntimeCandidate {
	patchType := "unified_diff"
	baseHash := "test-semantic-snapshot/main.go"
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}, {Name: "baseHash", String: &baseHash}},
	}}}}}
}

func runtimeOwnerConstraintNonconformanceRecord(t *testing.T, aggregate RuntimeAggregate, attempt int) runtimecontract.RuntimeCandidateConformanceRecord {
	return runtimeOwnerConstraintConformanceRecord(t, aggregate, attempt, runtimecontract.RuntimeOperationCandidateNonconformant)
}

func runtimeOwnerConstraintConformanceRecord(t *testing.T, aggregate RuntimeAggregate, attempt int, outcome runtimecontract.RuntimeOperationCandidateConformanceOutcome) runtimecontract.RuntimeCandidateConformanceRecord {
	t.Helper()
	attemptRecord := runtimeCandidateAttemptForNumber(t, aggregate, attempt)
	if attemptRecord.Proposal == nil {
		t.Fatalf("candidate attempt %d has no persisted proposal", attempt)
	}
	manifest, err := persistedCandidateAttemptManifest(aggregate, attemptRecord)
	if err != nil {
		t.Fatal(err)
	}
	operations := candidateRuntimeOperations(*attemptRecord.Proposal)
	if len(operations) != 1 {
		t.Fatalf("candidate attempt %d operations=%+v", attempt, operations)
	}
	operation := operations[0].Normalized()
	authorization, found := runtimeCandidateOperationAuthorization(manifest, operation.Kind)
	if !found {
		t.Fatalf("candidate operation %q has no manifest authorization", operation.Kind)
	}
	variantID, constraints, err := runtimecontract.RuntimeOperationCandidateConstraintsForOperation(operation, authorization)
	if err != nil || len(constraints) == 0 {
		t.Fatalf("candidate operation constraints=%+v err=%v", constraints, err)
	}
	constraint := constraints[0]
	for _, candidateConstraint := range constraints {
		if candidateConstraint.RequiredShape == runtimecontract.RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile {
			constraint = candidateConstraint
			break
		}
	}
	observation := runtimecontract.RuntimeOperationCandidateConformanceObservation{
		OperationID: operation.ID, OperationKind: operation.Kind, VariantID: variantID,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, CandidateManifestHash: manifest.Hash,
		Outcome: outcome,
	}
	if outcome == runtimecontract.RuntimeOperationCandidateNonconformant {
		observation.Violation = &runtimecontract.RuntimeOperationCandidateConformanceViolation{
			ConstraintRef: constraint.Ref, ParameterName: constraint.ParameterName,
			RequiredShape: constraint.RequiredShape, ActualShape: runtimecontract.RuntimeOperationCandidateActualShapeInvalidSyntax,
		}
	}
	observation = observation.Normalized()
	return runtimecontract.RuntimeCandidateConformanceRecord{
		OperationID: operation.ID, LogicalRequestID: attemptRecord.LogicalRequestID,
		CandidateManifestHash: manifest.Hash, Observation: observation,
	}
}

func runtimeCandidateAttemptForNumber(t *testing.T, aggregate RuntimeAggregate, attempt int) runtimecontract.RuntimeCandidateAttemptRecord {
	t.Helper()
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt == attempt {
			return record
		}
	}
	t.Fatalf("candidate attempt %d is missing", attempt)
	return runtimecontract.RuntimeCandidateAttemptRecord{}
}

func TestOwnerCandidateConformanceRejectionSealsOneManifestBoundCorrection(t *testing.T) {
	manifest := runtimeOwnerConstraintManifest(t)
	patchType := "unified_diff"
	diff := "not-a-unified-diff"
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{
			ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"main.go"},
			Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}},
		},
	}}}}
	frontier := candidateCorrectionFrontier{candidate: &candidate}
	failure := runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract,
		Code:  "candidate_owner_constraint_nonconformant", Signature: "candidate-owner-constraint-nonconformant",
		Reason: "bounded owner candidate conformance rejection",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			OperationID: "patch-1", Path: "$.operations[0].parameters[name=diff]",
			Actual:        string(runtimecontract.RuntimeOperationCandidateActualShapeInvalidSyntax),
			ConstraintRef: "patch/unified_diff/diff/single_file",
			RequiredShape: string(runtimecontract.RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile),
			ManifestHash:  manifest.Hash,
		},
	}
	sealed, correction, err := sealCandidateRejectionWithFrontier(&failure, manifest, frontier)
	if err != nil {
		t.Fatal(err)
	}
	if sealed == nil || correction == nil || correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || correction.Witness == nil {
		t.Fatalf("sealed owner correction = failure=%+v correction=%+v", sealed, correction)
	}
	witness := correction.Witness.Normalized()
	if witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessOwnerConstraint ||
		witness.OperationKind != runtimecontract.RuntimeBlockKindPatchApply ||
		witness.ConstraintRef != "patch/unified_diff/diff/single_file" ||
		witness.RejectedValue != string(runtimecontract.RuntimeOperationCandidateActualShapeInvalidSyntax) ||
		witness.AlternativeValue != string(runtimecontract.RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile) ||
		witness.RejectedShapeHash != "" || witness.AlternativeShapeHash != "" {
		t.Fatalf("owner candidate correction witness = %+v", witness)
	}
	if err := runtimecontract.ValidateRuntimeCandidateFailureCorrectionPair(sealed, correction); err != nil {
		t.Fatalf("failure/correction pair: %v", err)
	}

	tampered := failure
	tampered.Diagnostic = failure.Diagnostic.Clone()
	tampered.Diagnostic.ConstraintRef = "patch/unified_diff/diff/not-sealed"
	_, rejected, err := sealCandidateRejectionWithFrontier(&tampered, manifest, frontier)
	if err != nil {
		t.Fatal(err)
	}
	if rejected == nil || rejected.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal || rejected.TerminalCode != "candidate_authority_exhausted" {
		t.Fatalf("tampered owner constraint obtained retry authority: %+v", rejected)
	}
}

func runtimeOwnerConstraintManifest(t *testing.T) runtimecontract.RuntimeCandidateContractManifest {
	t.Helper()
	contract, found := runtimecontract.RuntimeOperationContractForKind(runtimecontract.RuntimeBlockKindPatchApply)
	if !found {
		t.Fatal("patch/apply contract is missing")
	}
	manifest := runtimecontract.RuntimeCandidateContractManifest{
		Revision:                  runtimecontract.RuntimeCandidateContractManifestRevision,
		State:                     runtimecontract.ConvergenceStateNeedStateChange,
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		Operations: []runtimecontract.RuntimeCandidateOperationManifest{{
			Kind: contract.Kind, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			Parameters: contract.Parameters, Variants: contract.Variants,
			OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		}},
	}.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	return manifest
}
