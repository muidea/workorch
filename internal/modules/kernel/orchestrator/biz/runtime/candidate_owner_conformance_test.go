package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
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

func TestOwnerCandidateApplicabilityViolationClosesFactAndCorrectionInOneCAS(t *testing.T) {
	for _, operation := range []struct{ kind, variant string }{
		{runtimecontract.RuntimeBlockKindPatchApply, "unified_diff"},
		{runtimecontract.RuntimeBlockKindPatchApply, "text_replace"},
		{runtimecontract.StepKindFileEdit, ""},
	} {
		t.Run(operation.kind+"/"+operation.variant, func(t *testing.T) {
			testOwnerCandidateApplicabilityClosure(t, operation.kind, operation.variant)
		})
	}
}

func testOwnerCandidateApplicabilityClosure(t *testing.T, kind, variant string) {
	ctx := context.Background()
	aggregate := runtimeOwnerConstraintAggregateForOperation(t, kind, variant)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "owner-applicability-prefix"); err != nil {
		t.Fatal(err)
	}
	firstCandidate := runtimeContentEditCandidate(kind, variant, "first")
	proposal, err := controller.AdmitCandidateProposalForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, firstCandidate)
	if err != nil || !proposal.Admission.Accepted() {
		t.Fatalf("persist first applicability proposal: admission=%+v err=%v", proposal.Admission, err)
	}
	firstRecord := runtimeOwnerApplicabilityViolationRecord(t, proposal.Aggregate, 0)
	beforeFeasibility := runtimeTestEvidenceAggregateRoundTrip(t, proposal.Aggregate)
	staleRecord := firstRecord
	staleRecord.CandidateManifestHash += "-stale"
	savesBeforeStale := repository.saves
	if _, err := controller.candidateDispatch.recordCandidateFeasibility(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, staleRecord); err == nil || repository.saves != savesBeforeStale {
		t.Fatalf("stale feasibility changed the current candidate: %v", err)
	}
	if kind == runtimecontract.RuntimeBlockKindPatchApply {
		wrongVariant := firstRecord
		observation := firstRecord.Observation.Clone()
		wrongVariant.Observation = &observation
		violation := wrongVariant.Observation.CandidateConstraintViolation
		violation.ConstraintRef = "file/text_replace/oldText/observed_target"
		violation.ParameterName = "oldText"
		violation.RequiredShape = runtimecontract.RuntimeOperationCandidateRequiredShapeTextReplaceApplicableToObservedTarget
		if variant == "text_replace" {
			violation.ConstraintRef = "patch/unified_diff/diff/observed_target"
			violation.ParameterName = "diff"
			violation.RequiredShape = runtimecontract.RuntimeOperationCandidateRequiredShapeUnifiedDiffApplicableToObservedTarget
		}
		if _, err := controller.candidateDispatch.recordCandidateFeasibility(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, wrongVariant); err == nil || repository.saves != savesBeforeStale {
			t.Fatalf("another variant's constraint changed the current candidate: %v", err)
		}
	}
	revisionBefore := proposal.Aggregate.StateRevision
	savesBefore := repository.saves
	closed, err := controller.candidateDispatch.recordCandidateFeasibility(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, firstRecord)
	if err != nil {
		t.Fatal(err)
	}
	firstAttempt := runtimeCandidateAttemptForNumber(t, closed, 0)
	if closed.StateRevision != revisionBefore+1 || repository.saves != savesBefore+1 || firstAttempt.State != runtimecontract.RuntimeCandidateAttemptStateFailed ||
		len(firstAttempt.Feasibilities) != 1 || firstAttempt.Failure == nil || firstAttempt.Failure.Code != "candidate_owner_constraint_nonconformant" ||
		firstAttempt.Correction == nil || firstAttempt.Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce ||
		firstAttempt.Correction.Witness == nil || firstAttempt.Correction.Witness.ConstraintRef != firstRecord.Observation.CandidateConstraintViolation.ConstraintRef ||
		closed.ActiveDispatch != nil || closed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("applicability fact and correction were not one atomic transition: attempt=%+v decision=%+v", firstAttempt, closed.Decision)
	}
	if len(firstAttempt.FeasibilityCatalogs) != 1 || firstAttempt.FeasibilityCatalogs[0].SnapshotID != firstRecord.AdmissibilitySnapshotID {
		t.Fatal("failed attempt did not retain the exact owner catalog preimage")
	}
	for name, mutate := range map[string]func(*runtimecontract.RuntimeCandidateAttemptRecord){
		"missing": func(attempt *runtimecontract.RuntimeCandidateAttemptRecord) { attempt.FeasibilityCatalogs = nil },
		"revision_drift": func(attempt *runtimecontract.RuntimeCandidateAttemptRecord) {
			attempt.FeasibilityCatalogs[0].ObservedRevision = "forged-revision"
		},
		"acquisition_drift": func(attempt *runtimecontract.RuntimeCandidateAttemptRecord) {
			attempt.FeasibilityCatalogs[0].AcquisitionBinding = "forged-acquisition"
		},
		"duplicate": func(attempt *runtimecontract.RuntimeCandidateAttemptRecord) {
			attempt.FeasibilityCatalogs = append(attempt.FeasibilityCatalogs, attempt.FeasibilityCatalogs[0].Clone())
		},
		"coordinated_reseal": func(attempt *runtimecontract.RuntimeCandidateAttemptRecord) {
			catalog := &attempt.FeasibilityCatalogs[0]
			catalog.ObservedRevision = "forged-revision"
			catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
			attempt.Feasibilities[0].AdmissibilitySnapshotID = catalog.SnapshotID
			attempt.Feasibilities[0].Observation.OwnerPolicy.CatalogSnapshotID = catalog.SnapshotID
			attempt.Feasibilities[0].Observation.CandidateConstraintViolation.ObservedRevision = catalog.ObservedRevision
		},
	} {
		t.Run("historical_catalog_"+name, func(t *testing.T) {
			tampered := runtimeTestEvidenceAggregateRoundTrip(t, closed)
			mutate(&tampered.CandidateAttempts[0])
			if err := tampered.Validate(); err == nil {
				t.Fatal("historical owner catalog tamper passed fresh-load validation")
			}
		})
	}
	clone := firstAttempt.Clone()
	clone.FeasibilityCatalogs[0].TargetRefs[0] = "foreign-target"
	if firstAttempt.FeasibilityCatalogs[0].TargetRefs[0] == "foreign-target" {
		t.Fatal("candidate-attempt clone aliases its historical owner catalog")
	}
	for name, mode := range map[string]failureAuthorityCASMode{
		"before_write": failureAuthorityCASFailBeforeWrite, "lost_response": failureAuthorityCASLoseResponse,
	} {
		t.Run("cas_"+name, func(t *testing.T) {
			before := runtimeTestEvidenceAggregateRoundTrip(t, beforeFeasibility)
			repository := &failureAuthorityCASRepository{aggregate: before, mode: mode}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := controller.candidateDispatch.recordCandidateFeasibility(ctx, before.Facts.WorkspaceID, before.Facts.RunID, 0, firstRecord); !errors.Is(err, errFailureAuthorityCASCut) {
				t.Fatalf("injected CAS failure: %v", err)
			}
			if mode == failureAuthorityCASFailBeforeWrite && !reflect.DeepEqual(before, repository.aggregate) {
				t.Fatal("CAS-before cut persisted a partial applicability disposition")
			}
			repository.mode = failureAuthorityCASNormal
			restarted, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := restarted.candidateDispatch.recordCandidateFeasibility(ctx, before.Facts.WorkspaceID, before.Facts.RunID, 0, firstRecord)
			if err != nil || recovered.StateRevision != closed.StateRevision || !reflect.DeepEqual(recovered.CandidateAttempts, closed.CandidateAttempts) {
				t.Fatalf("restart failed to recover the exact atomic disposition: %v", err)
			}
			saves := repository.saveAttempts
			if _, err := restarted.candidateDispatch.recordCandidateFeasibility(ctx, before.Facts.WorkspaceID, before.Facts.RunID, 0, firstRecord); err != nil || repository.saveAttempts != saves {
				t.Fatalf("duplicate disposition wrote again: %v", err)
			}
		})
	}
	reloaded := runtimeTestEvidenceAggregateRoundTrip(t, closed)
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("fresh-load applicability lineage: %v", err)
	}
	tampered := runtimeTestEvidenceAggregateRoundTrip(t, closed)
	tamperedAttempt := runtimeCandidateAttemptForNumber(t, tampered, 0)
	for idx := range tampered.CandidateAttempts {
		if tampered.CandidateAttempts[idx].Attempt != tamperedAttempt.Attempt {
			continue
		}
		tampered.CandidateAttempts[idx].Feasibilities[0].Observation.CandidateConstraintViolation.ObservedRevision = "forged-owner-revision"
		break
	}
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "sealed owner observation") {
		t.Fatalf("fresh-load applicability revision drift err=%v", err)
	}
	savesAfter := repository.saves
	duplicate, err := controller.candidateDispatch.recordCandidateFeasibility(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, firstRecord)
	if err != nil || duplicate.StateRevision != closed.StateRevision || repository.saves != savesAfter {
		t.Fatalf("applicability duplicate was not idempotent: revision=%d saves=%d err=%v", duplicate.StateRevision, repository.saves, err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, "owner-applicability-prefix"); err != nil {
		t.Fatal(err)
	}
	secondCandidate := runtimeContentEditCandidate(kind, variant, "second")
	secondProposal, err := controller.AdmitCandidateProposalForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, secondCandidate)
	if err != nil || !secondProposal.Admission.Accepted() {
		t.Fatalf("persist corrected applicability proposal: admission=%+v err=%v", secondProposal.Admission, err)
	}
	terminal, err := controller.candidateDispatch.recordCandidateFeasibility(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, runtimeOwnerApplicabilityViolationRecord(t, secondProposal.Aggregate, 1))
	if err != nil {
		t.Fatal(err)
	}
	secondAttempt := runtimeCandidateAttemptForNumber(t, terminal, 1)
	if secondAttempt.State != runtimecontract.RuntimeCandidateAttemptStateFailed || secondAttempt.Correction == nil ||
		secondAttempt.Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal || secondAttempt.Correction.TerminalCode != "candidate_retry_exhausted" ||
		terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "candidate_retry_exhausted" || terminal.ActiveDispatch != nil {
		t.Fatalf("attempt-1 applicability violation did not exhaust atomically: attempt=%+v decision=%+v", secondAttempt, terminal.Decision)
	}
	reloadedTerminal := runtimeTestEvidenceAggregateRoundTrip(t, terminal)
	if err := reloadedTerminal.Validate(); err != nil {
		t.Fatalf("terminal reload: %v", err)
	}
	if len(terminal.OperationReceipts) != 0 || len(terminal.Facts.Evidence.Accepted) != 0 {
		t.Fatal("candidate correction acquired effect/evidence credit")
	}
}

func runtimeOwnerApplicabilityViolationRecord(t *testing.T, aggregate RuntimeAggregate, attempt int) runtimecontract.RuntimeCandidateFeasibilityRecord {
	t.Helper()
	attemptRecord := runtimeCandidateAttemptForNumber(t, aggregate, attempt)
	manifest, err := persistedCandidateAttemptManifest(aggregate, attemptRecord)
	if err != nil {
		t.Fatal(err)
	}
	operations := candidateRuntimeOperations(*attemptRecord.Proposal)
	if len(operations) != 1 {
		t.Fatalf("applicability fixture operations=%+v inventories=%+v", operations, aggregate.Facts.Resources.OperationAdmissibilities)
	}
	operation := operations[0].Normalized()
	inventory, found := runtimeOperationAdmissibilityForKind(aggregate.Facts.Resources.OperationAdmissibilities, runtimecontract.RuntimeModuleFile, operation.Kind)
	if !found {
		t.Fatalf("applicability fixture has no File inventory for %q", operation.Kind)
	}
	var selected runtimecontract.RuntimeOperationCandidateConstraint
	for _, authorization := range manifest.Operations {
		if authorization.Kind != operation.Kind {
			continue
		}
		_, constraints, err := runtimecontract.RuntimeOperationCandidateConstraintsForOperation(operation, authorization)
		if err != nil {
			t.Fatal(err)
		}
		for _, constraint := range constraints {
			if constraint.Kind == runtimecontract.RuntimeOperationCandidateConstraintTargetApplicability {
				selected = constraint
			}
		}
	}
	if selected.Ref == "" {
		t.Fatal("fixture has no sealed target-applicability constraint")
	}
	observation := runtimecontract.RuntimeOwnerFeasibilityObservation{
		Outcome:     runtimecontract.RuntimeOwnerFeasibilityCandidateConstraintViolated,
		OwnerPolicy: runtimecontract.RuntimeOwnerPolicyIdentity{OwnerModuleID: runtimecontract.RuntimeModuleFile, PolicyRevision: "file-owner-feasibility/v1", CatalogSnapshotID: inventory.SnapshotID},
		CandidateConstraintViolation: &runtimecontract.RuntimeOperationCandidateConstraintViolation{
			ConstraintRef: selected.Ref, ParameterName: selected.ParameterName,
			RequiredShape: selected.RequiredShape,
			ActualShape:   runtimecontract.RuntimeOperationCandidateActualShapeObservedTargetContextMismatch,
			TargetRef:     operation.TargetRefs[0], ObservedRevision: inventory.ObservedRevision,
		},
	}
	return runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
		CandidateManifestHash: manifest.Hash, Observation: &observation,
	}
}

func runtimeOwnerConstraintAggregate(t *testing.T) RuntimeAggregate {
	return runtimeOwnerConstraintAggregateForOperation(t, runtimecontract.RuntimeBlockKindPatchApply, "unified_diff")
}

func runtimeOwnerConstraintAggregateForOperation(t *testing.T, kind, variant string) RuntimeAggregate {
	t.Helper()
	return stateChangeAggregateWithPreparation(t, func(initial *RuntimeAggregate) {
		// Seal the same patch/apply capability that this fixture proposes. The
		// semantic manifest is compiled from immutable capability facts before
		// candidate admission; changing only the Focus variant would leave the
		// owner catalog correctly sealed for file/write and make the patch
		// candidate an unrelated operation.
		initial.Facts.Capabilities.StateChangeKinds = []string{kind}
		initial.Definition.CapabilityContract = append(initial.Definition.CapabilityContract, kind)
		initial.Facts.Focus.AcceptedStepKinds = []string{kind}
		initial.Facts.Focus.AcceptedOperationVariants = nil
		if variant != "" {
			initial.Facts.Focus.AcceptedOperationVariants = []runtimecontract.RuntimeOperationVariantRef{{Kind: kind, VariantID: variant}}
		}
		catalog := runtimecontract.RuntimeOperationAdmissibility{
			PrerequisiteID: "test-owner-conformance-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKinds: []string{kind},
			OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
				Kind: kind, VariantID: variant,
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
			}},
			TargetRefs: []string{"main.go"}, ObservedRevision: "test-owner-conformance-file-v1",
		}
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
		initial.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	})
}

func runtimeContentEditCandidate(kind, variant, text string) runtimecontract.RuntimeCandidate {
	if variant == "unified_diff" {
		return runtimeUnifiedDiffCandidate("--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+" + text + "\n")
	}
	baseHash, replacement := "test-semantic-snapshot/main.go", "replacement"
	parameters := []runtimecontract.OperationParameter{{Name: "oldText", String: &text}, {Name: "newText", String: &replacement}, {Name: "baseHash", String: &baseHash}}
	if variant != "" {
		parameters = append(parameters, runtimecontract.OperationParameter{Name: "patchType", String: &variant})
	}
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "edit-1", Kind: kind, TargetRefs: []string{"main.go"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Parameters: parameters,
	}}}}}
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
		observation.Violation = &runtimecontract.RuntimeOperationCandidateConstraintViolation{
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
