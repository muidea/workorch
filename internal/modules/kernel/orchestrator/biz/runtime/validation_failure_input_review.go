package runtime

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Input consistency is independent of implementation attribution. A complete
// native failure may require reviewing its original input even when independent
// reproduction/behavior could not produce a repair target. This grants no route.
type runtimeFailureInputReviewTrigger struct {
	coverageID  common.CoverageWorkItemID
	failure     runtimecontract.FailureFact
	convergence runtimecontract.RuntimeValidationFailureRepairConvergence
}

func matchRuntimeFailureInputReviewTrigger(a RuntimeAggregate, fact runtimecontract.EvidenceFact, op runtimecontract.RuntimeOperation) (runtimeFailureInputReviewTrigger, bool, error) {
	existing, matched, err := matchRuntimeValidationFailureRepairTrigger(a, fact, op)
	if err != nil || matched {
		return runtimeFailureInputReviewTrigger{coverageID: existing.coverageID, failure: existing.failure, convergence: existing.convergence}, matched, err
	}
	var empty runtimeFailureInputReviewTrigger
	f := a.Facts.Normalized()
	if fact.Kind != runtimecontract.EvidenceKindValidation || f.Failure == nil || f.Failure.ValidationDiagnostic == nil || f.Failure.OriginalDiagnostic == nil || len(f.Focus.CoverageWorkItemIDs) != 1 || f.Resources.FailureTarget != nil {
		return empty, false, nil
	}
	switch f.Failure.ValidationDiagnostic.Unavailability {
	case runtimecontract.ValidationDiagnosticReproductionUnavailable, runtimecontract.ValidationDiagnosticBehaviorUnobserved,
		runtimecontract.ValidationDiagnosticExecutionUnavailable, runtimecontract.ValidationDiagnosticDeadlineExceeded:
	default:
		return empty, false, nil
	}
	coverageID := f.Focus.CoverageWorkItemIDs[0]
	binding, found := coverageEvidenceBindingForCoverage(f.Focus, coverageID)
	descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(op.Kind)
	original := f.Failure.OriginalDiagnostic
	if !found || !known || descriptor.OwnerModuleID != original.OwnerModuleID || original.OperationID != fact.OperationID || op.ID != fact.OperationID || fact.FailureSignature != f.Failure.Signature || !contains(op.BindingRefs, binding.ID) || !contains(fact.BindingRefs, binding.ID) {
		return empty, false, nil
	}
	declaration, found, err := runtimeDefinitionFocusForCoverage(f.DefinitionFocuses, coverageID)
	if err != nil || !found || declaration.ValidationFailureRepairConvergence == nil {
		return empty, false, err
	}
	return runtimeFailureInputReviewTrigger{coverageID: coverageID, failure: f.Failure.Clone(), convergence: declaration.ValidationFailureRepairConvergence.Clone()}, true, nil
}

// The evidence CAS opens the existing semantic transaction before any repair
// target acquisition. This is a new review of affected inputs, not a replay of
// the prior report or a reclassification of the failed execution.
func openRuntimeFailureInputReview(a *RuntimeAggregate, fact runtimecontract.EvidenceFact, op runtimecontract.RuntimeOperation) (bool, error) {
	if a == nil || fact.Kind != runtimecontract.EvidenceKindValidation || a.Facts.Failure == nil || a.ActiveDispatch != nil || (a.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed && !runtimeFailureInputReviewedCurrentBinding(*a)) {
		return false, nil
	}
	trigger, matched, err := matchRuntimeFailureInputReviewTrigger(*a, fact, op)
	if err != nil || !matched {
		return false, err
	}
	binding, ok := coverageEvidenceBindingForCoverage(a.Facts.Focus, trigger.coverageID)
	if !ok {
		return false, common.ErrConflict
	}
	// Consume the exact committed binding's owner subject. Historical catalogs
	// for a different binding are not declarations for this execution.
	var ref runtimecontract.RuntimeCWIBindingRef
	for _, row := range a.Facts.Coverage {
		if row.ID == trigger.coverageID && row.CurrentBindingRef != nil {
			ref = *row.CurrentBindingRef
		}
	}
	var input runtimecontract.RuntimeValidationProofRequirementBinding
	matches := 0
	proof, proved := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
	var originalManifest *runtimecontract.RuntimeSemanticPlanManifest
	for _, l := range runtimeFailureInputReviewLedgers(a.SemanticPlanning) {
		if l.Manifest == nil || l.Manifest.Hash != ref.SemanticManifestHash {
			continue
		}
		if originalManifest != nil {
			if !reflect.DeepEqual(*originalManifest, *l.Manifest) {
				return false, common.ErrConflict
			}
			continue // Same committed manifest retained at another lifecycle cut.
		}
		originalManifest = l.Manifest
		for _, catalog := range l.Manifest.OwnerCatalogs {
			for _, surface := range catalog.ValidationSurfaces {
				for _, b := range surface.ProofRequirementBindings {
					if proved && b.CoverageWorkItemID == trigger.coverageID && b.RequirementHash == proof.ValidationProofRequirementHash && b.InputReview != nil {
						input = b.Clone()
						matches++
					}
				}
			}
		}
	}
	if matches == 0 {
		return false, nil
	}
	if matches != 1 {
		return false, fmt.Errorf("failure input review requires unique exact original subject")
	}
	// Structural/build diagnostics without native failed checks retain their
	// existing typed repair route. They are not missing native assertions.
	if fact.Validation == nil || len(fact.Validation.FailedChecks) == 0 {
		return false, nil
	}
	if a.Facts.Repair.Exhausted || a.Facts.Repair.RepairAttempts > trigger.convergence.MaxAttempts {
		return true, markRuntimeValidationFailureRepairExhausted(&a.Facts, trigger.failure, trigger.convergence)
	}
	fingerprint := runtimecontract.RuntimeValidationFailureFingerprint(fact, binding)
	if fingerprint == "" {
		return true, markRuntimeFailureInputReviewUnavailable(a, "validation_failure_assertions_unavailable", "native failure has no complete exact assertion identity")
	}
	for _, g := range a.SemanticPlanning.AllHistory() {
		s := g.Snapshot.FailureInputReview
		if g.Reason == runtimecontract.RuntimeSemanticPlanningRetirementFailureInputReviewed && s != nil && s.SubjectHash == input.InputReview.Hash && s.FailureFingerprint == fingerprint && s.Feedback.CoverageWorkItemID == trigger.coverageID {
			return false, nil
		}
	}
	dispatch, _, found := operationReceiptDispatch(*a, fact.DispatchID)
	execution, executed := operationExecutionRecord(dispatch.OperationExecutions, op.ID)
	receipt, received := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, execution.ExecutionKey, execution.Fence)
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	if !found || !executed || !received || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.OwnerReceiptHash != receiptHash {
		return false, common.ErrConflict
	}
	if err := validateRuntimeReceiptEvidenceRejectedLineage(*a, dispatch, op, execution, receipt, receiptHash); err != nil {
		return false, err
	}
	if receipt.Evidence.OwnerRevisionSet == nil {
		return true, markRuntimeFailureInputReviewUnavailable(a, "validation_failure_input_snapshot_unavailable", "failed validation has no original owner input reads")
	}
	if err := validateRuntimeReviewedInputReads(input, *receipt.Evidence.OwnerRevisionSet, trigger.failure.OriginalDiagnostic.OwnerModuleID+"/"+string(a.Facts.WorkspaceID)); err != nil {
		return true, markRuntimeFailureInputReviewUnavailable(a, "validation_failure_input_snapshot_unavailable", err.Error())
	}
	diagnostic, err := runtimecontract.ProjectRuntimeSemanticExecutionDiagnostic(receipt.Feedback, trigger.failure.OriginalDiagnostic.OwnerModuleID)
	if err != nil {
		return false, err
	}
	feedback := runtimecontract.RuntimeSemanticExecutionFeedback{
		WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, DefinitionRevision: a.Definition.Revision, StartupInputHash: a.Definition.StartupInputHash,
		ManifestHash: ref.SemanticManifestHash, CoverageWorkItemID: trigger.coverageID, FailedFocusID: "",
		FailureSignature: fact.FailureSignature, OwnerModuleID: trigger.failure.OriginalDiagnostic.OwnerModuleID, OperationKind: op.Kind,
		EvidenceID: fact.ID, EvidenceRevision: fact.Revision, DispatchID: fact.DispatchID, OperationID: fact.OperationID,
		ReceiptRef: execution.OwnerReceiptRef, ReceiptHash: receiptHash, Validation: runtimecontract.ProjectRuntimeValidationExecutionFeedback(receipt.Evidence.Validation), Diagnostic: diagnostic,
	}
	declaration, _, err := runtimeDefinitionFocusForCoverage(a.Facts.DefinitionFocuses, trigger.coverageID)
	if err != nil {
		return false, err
	}
	previous, err := runtimeFailurePreviousInputReview(*a, ref, input.InputReview.Hash, trigger.coverageID)
	if err != nil {
		return true, markRuntimeFailureInputReviewUnavailable(a, "validation_failure_input_snapshot_unavailable", err.Error())
	}
	feedback.PreviousInputReview = previous
	feedback.FailedFocusID = declaration.ID
	feedback.Hash = runtimecontract.StableRuntimeSemanticExecutionFeedbackHash(feedback)
	source := &runtimecontract.RuntimeValidationFailureInputReview{BindingRef: ref, SubjectHash: input.InputReview.Hash, FailureFingerprint: fingerprint, EvidenceHash: runtimecontract.StableRuntimeEvidenceFactHash(fact), Feedback: feedback}
	if err := source.Validate(); err != nil {
		// Complete raw output with unavailable applicable assertions is a determined
		// diagnostic gap. Never spend repair attempts guessing from hashes.
		var capacity *runtimecontract.RuntimeExecutionFeedbackCapacityError
		if errors.As(err, &capacity) {
			return true, markRuntimeFailureInputReviewUnavailable(a, "validation_failure_feedback_capacity_exceeded", err.Error())
		}
		return true, markRuntimeFailureInputReviewUnavailable(a, "validation_failure_assertions_unavailable", err.Error())
	}
	ledger := a.SemanticPlanning
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningRequired {
		a.SemanticPlanning = runtimeFailureInputReviewRequiredLedger(ledger, source)
		return true, nil
	}
	generation := runtimecontract.RuntimeSemanticPlanningGeneration{Reason: runtimecontract.RuntimeSemanticPlanningRetirementFailureInputReview, RetiredAtStateRevision: a.StateRevision + 1, Snapshot: ledger.WithoutHistory().CloneRaw()}
	a.SemanticPlanning = runtimeFailureInputReviewRequiredLedger(ledger, source)
	next, err := a.SemanticPlanning.AppendHistory(generation)
	if err != nil {
		return false, err
	}
	a.SemanticPlanning = next
	return true, nil
}

func runtimeFailureInputReviewRequiredLedger(l runtimecontract.RuntimeSemanticPlanningLedger, source *runtimecontract.RuntimeValidationFailureInputReview) runtimecontract.RuntimeSemanticPlanningLedger {
	return runtimecontract.RuntimeSemanticPlanningLedger{SchemaVersion: l.SchemaVersion, Phase: runtimecontract.RuntimeSemanticPlanningRequired,
		DefinitionSeed: l.DefinitionSeed.Clone(), CatalogRefreshAttempts: l.CatalogRefreshAttempts, FeasibilityGaps: l.FeasibilityGaps,
		FailureInputReview: source.Clone(), History: l.History, HistoryArchive: l.HistoryArchive}
}

func runtimeFailureInputReviewLedgers(l runtimecontract.RuntimeSemanticPlanningLedger) []runtimecontract.RuntimeSemanticPlanningLedger {
	out := []runtimecontract.RuntimeSemanticPlanningLedger{l.WithoutHistory()}
	for _, g := range l.AllHistory() {
		out = append(out, g.Snapshot)
	}
	return out
}

// Consuming a committed review retains the current sealed operation binding;
// it does not rerun its semantic proof or manufacture a new binding/seal.
func runtimeFailureInputReviewedCurrentBinding(a RuntimeAggregate) bool {
	if a.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired {
		return false
	}
	var history []runtimecontract.RuntimeSemanticPlanningGeneration
	for _, g := range a.SemanticPlanning.AllHistory() {
		history = append(history, g)
	}
	for _, g := range slices.Backward(history) {
		s := g.Snapshot.FailureInputReview
		if g.Reason != runtimecontract.RuntimeSemanticPlanningRetirementFailureInputReviewed || s == nil {
			continue
		}
		for _, row := range a.Facts.Coverage {
			if row.ID == s.Feedback.CoverageWorkItemID && row.CurrentBindingRef != nil && runtimecontract.SameRuntimeCWIBindingRef(*row.CurrentBindingRef, s.BindingRef) {
				return true
			}
		}
	}
	return false
}

func settleRuntimeFailureInputReview(a RuntimeAggregate, l runtimecontract.RuntimeSemanticPlanningLedger, conclusion string) (RuntimeAggregate, bool, error) {
	s := l.FailureInputReview
	if s == nil {
		return a, false, nil
	}
	fact, op, receipt, err := runtimeFailureInputReviewSource(a, *s)
	if err != nil {
		return a, false, err
	}
	// Work on detached state: unsupported maintenance must leave the original
	// binding/failure untouched for the existing explicit conflict route.
	a = a.CloneRaw()
	// A factual clarification may have replaced the mutable failure. Restore
	// only the exact original rejected receipt's failure at this new SM-2 boundary.
	originalFailure := failureFactFromOperationReceiptFeedback(receipt)
	if originalFailure == nil {
		return a, false, common.ErrConflict
	}
	if err := replaceRuntimeFailureAuthority(&a.Facts, *originalFailure, runtimeFailureAuthorityRetire, nil); err != nil {
		return a, false, err
	}
	if conclusion == runtimecontract.RuntimeValidationInputConflict {
		carriedRepair := a.Facts.Repair.Normalized()
		if err := retireRuntimeValidationRepairBinding(&a, s.Feedback.CoverageWorkItemID, s.BindingRef, string(fact.ID), "validation_failure_input_conflict"); err != nil {
			return a, false, err
		}
		carriedRepair.FocusID, carriedRepair.SliceID = a.Facts.Focus.ID, a.Facts.Focus.SliceID
		a.Facts.Repair = carriedRepair
		a.Facts.Failure = nil
		a.Facts.RepairDirective = nil
		a.SemanticPlanning = l
		return retireRuntimeInputReviewForRevision(a)
	}
	if conclusion != runtimecontract.RuntimeValidationInputConsistent {
		return a, false, nil
	}
	if err := retireSemanticPlanningRequestSessions(&a, l, "validation_failure_input_reviewed"); err != nil {
		return a, false, err
	}
	a.SemanticPlanning = runtimeFailureInputReviewRequiredLedger(l, nil)
	generation := runtimecontract.RuntimeSemanticPlanningGeneration{Reason: runtimecontract.RuntimeSemanticPlanningRetirementFailureInputReviewed, RetiredAtStateRevision: a.StateRevision + 1, Snapshot: l.WithoutHistory().CloneRaw()}
	a.SemanticPlanning, err = a.SemanticPlanning.AppendHistory(generation)
	if err != nil {
		return a, false, err
	}
	// A consistent input review cannot invent missing implementation behavior.
	if unavailable, err := markValidationFailureDiagnosticUnavailable(&a, fact); err != nil {
		return a, false, err
	} else if unavailable {
		a.Facts, a.Decision, err = reduceRuntimeAggregateWithDirective(a, a.Facts, nil)
		return a, true, err
	}
	// The unchanged original repair route still owns target acquisition, fence,
	// attempt limits and effect authority. A review does not reserve a new budget.
	opened, err := openRuntimeValidationFailureRepairGeneration(&a, fact, op)
	if err != nil {
		return a, false, err
	}
	if !opened {
		// Review does not supply a missing resource-owner repair surface.
		if _, err := markRuntimeValidationFailureRepairSurfaceUnavailable(&a, fact, op); err != nil {
			return a, false, err
		}
	}
	a.Facts, a.Decision, err = reduceRuntimeAggregateWithDirective(a, a.Facts, nil)
	return a, true, err
}

// Source validation checks identities and committed receipt/fact integrity;
// it does not replay the original review's business judgment on Load/recovery.
func runtimeFailureInputReviewSource(a RuntimeAggregate, s runtimecontract.RuntimeValidationFailureInputReview) (runtimecontract.EvidenceFact, runtimecontract.RuntimeOperation, runtimecontract.RuntimeOperationReceipt, error) {
	var emptyFact runtimecontract.EvidenceFact
	var emptyOp runtimecontract.RuntimeOperation
	var emptyReceipt runtimecontract.RuntimeOperationReceipt
	if err := s.Validate(); err != nil {
		return emptyFact, emptyOp, emptyReceipt, err
	}
	f := s.Feedback
	if f.WorkspaceID != a.Facts.WorkspaceID || f.RunID != a.Facts.RunID || f.DefinitionRevision != a.Definition.Revision || f.StartupInputHash != a.Definition.StartupInputHash {
		return emptyFact, emptyOp, emptyReceipt, common.ErrConflict
	}
	fact, found := runtimeRejectedEvidenceFactForEvent(a.Facts.Evidence.Rejected, string(f.EvidenceID))
	dispatch, _, dispatched := operationReceiptDispatch(a, f.DispatchID)
	op, operated := candidateOperationByID(dispatch.Candidate, f.OperationID)
	execution, executed := operationExecutionRecord(dispatch.OperationExecutions, f.OperationID)
	receipt, received := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found || !dispatched || !operated || !executed || !received || fact.DispatchID != dispatch.ID || fact.OperationID != op.ID ||
		runtimecontract.StableRuntimeEvidenceFactHash(fact) != s.EvidenceHash || fact.FailureSignature != f.FailureSignature ||
		execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.EvidenceFactHash != s.EvidenceHash || execution.EvidenceFactID != fact.ID ||
		execution.OwnerReceiptRef != f.ReceiptRef || execution.OwnerReceiptHash != f.ReceiptHash || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != f.ReceiptHash || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected {
		return emptyFact, emptyOp, emptyReceipt, fmt.Errorf("%w: failure input review original source missing or changed", common.ErrConflict)
	}
	if fact.Kind != runtimecontract.EvidenceKindValidation || fact.Revision != f.EvidenceRevision || !slices.Contains(op.BindingRefs, s.BindingRef.BindingID) {
		return emptyFact, emptyOp, emptyReceipt, common.ErrConflict
	}
	binding, bound := runtimecontract.CoverageEvidenceBindingForRef(dispatch.CoverageBindings, s.BindingRef.BindingID)
	if !bound || binding.Hash != s.BindingRef.BindingHash || binding.FocusID != s.BindingRef.FocusID || binding.CoverageWorkItemID != f.CoverageWorkItemID || runtimecontract.RuntimeValidationFailureFingerprint(fact, binding) != s.FailureFingerprint {
		return emptyFact, emptyOp, emptyReceipt, common.ErrConflict
	}
	diagnostic, err := runtimecontract.ProjectRuntimeSemanticExecutionDiagnostic(receipt.Feedback, f.OwnerModuleID)
	if err != nil || !reflect.DeepEqual(f.Validation, runtimecontract.ProjectRuntimeValidationExecutionFeedback(receipt.Evidence.Validation)) || !reflect.DeepEqual(f.Diagnostic, diagnostic) {
		return emptyFact, emptyOp, emptyReceipt, common.ErrConflict
	}
	previous, err := runtimeFailurePreviousInputReview(a, s.BindingRef, s.SubjectHash, f.CoverageWorkItemID)
	if err != nil || !reflect.DeepEqual(previous, f.PreviousInputReview) {
		return emptyFact, emptyOp, emptyReceipt, fmt.Errorf("failure input review prior report missing or changed")
	}
	subjectFound := 0
	var originalManifest *runtimecontract.RuntimeSemanticPlanManifest
	for _, ledger := range runtimeFailureInputReviewLedgers(a.SemanticPlanning) {
		if ledger.Manifest == nil || ledger.Manifest.Hash != s.BindingRef.SemanticManifestHash {
			continue
		}
		if originalManifest != nil {
			if !reflect.DeepEqual(*originalManifest, *ledger.Manifest) {
				return emptyFact, emptyOp, emptyReceipt, common.ErrConflict
			}
			continue
		}
		originalManifest = ledger.Manifest
		for _, catalog := range ledger.Manifest.OwnerCatalogs {
			for _, surface := range catalog.ValidationSurfaces {
				for _, input := range surface.ProofRequirementBindings {
					if input.CoverageWorkItemID == f.CoverageWorkItemID && input.InputReview != nil && input.InputReview.Hash == s.SubjectHash {
						subjectFound++
					}
				}
			}
		}
	}
	if subjectFound != 1 {
		return emptyFact, emptyOp, emptyReceipt, fmt.Errorf("failure input review lost unique original subject: matches=%d manifest=%s phase=%s", subjectFound, s.BindingRef.SemanticManifestHash, a.SemanticPlanning.Phase)
	}
	return fact, op, receipt, nil
}

func markRuntimeFailureInputReviewUnavailable(a *RuntimeAggregate, code, message string) error {
	failure := a.Facts.Failure.Clone()
	failure.Class = runtimecontract.FailureClassNoProgressExhausted
	failure.Code, failure.Message = code, message
	return replaceRuntimeFailureAuthority(&a.Facts, failure, runtimeFailureAuthorityRetire, nil)
}

// Consume the original accepted batch body, without replaying its semantic
// judgment. A retained copy of the same formal batch is the same source.
func runtimeFailurePreviousInputReview(a RuntimeAggregate, ref runtimecontract.RuntimeCWIBindingRef, subject string, cwi common.CoverageWorkItemID) (*runtimecontract.RuntimeValidationInputReviewFeedback, error) {
	var result *runtimecontract.RuntimeValidationInputReviewFeedback
	for _, l := range runtimeFailureInputReviewLedgers(a.SemanticPlanning) {
		if l.Manifest == nil || l.Manifest.Hash != ref.SemanticManifestHash {
			continue
		}
		for _, d := range l.BatchDocuments {
			batch, found := runtimeSemanticBatch(l.Batches, d.BatchID)
			if !found || !runtimecontract.RuntimeSemanticAcceptedBatchAddressesBinding(batch, d, cwi, ref) {
				continue
			}
			for _, p := range d.Proposals {
				if p.CoverageWorkItemID != cwi || p.ValidationInputReview == nil || p.ValidationInputReview.SubjectHash != subject {
					continue
				}
				b, ok := runtimecontract.RuntimeSemanticValidationInputReviewBinding(p, l.Manifest.OwnerCatalogs)
				if !ok {
					return nil, common.ErrConflict
				}
				source := l
				source.LogicalRequestID = batch.RequestLogicalID
				f := runtimecontract.RuntimeValidationInputReviewFeedback{ReviewRef: runtimeInputReviewRef(a, source, d), ProtocolRef: b.InputReview.ProtocolRef, CoverageWorkItemID: cwi, RequirementHash: b.RequirementHash, ReportHash: runtimecontract.StableRuntimeValidationInputReviewHash(*p.ValidationInputReview), Report: p.ValidationInputReview.Clone()}
				if err := f.Validate(); err != nil {
					return nil, err
				}
				if result != nil && !reflect.DeepEqual(*result, f) {
					return nil, fmt.Errorf("failure input review has ambiguous prior formal report")
				}
				result = &f
			}
		}
	}
	if result == nil {
		return nil, fmt.Errorf("failure input review lost original accepted report")
	}
	return result, nil
}
