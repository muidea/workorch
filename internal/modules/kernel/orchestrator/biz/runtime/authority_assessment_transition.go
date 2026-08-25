package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/proof"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *authorityComponent) commitAssessment(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment || aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.Candidate.Assessment == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime dispatch does not carry assessment", common.ErrConflict)
	}
	assessment := aggregate.ActiveDispatch.Candidate.Assessment.Normalized()
	claimedDisposition, err := runtimecontract.ValidateAssessmentDisposition(assessment.Disposition, assessment.Items, assessment.Gap)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	// The candidate disposition is a producer observation. Keep it intact for
	// audit/input validation, but never use it to select coverage, repair or a
	// Runtime decision below.
	assessment.Disposition = claimedDisposition
	// The entire assessment remains a scoped producer observation. It must not
	// be rewritten or rejected because its claimed evidence, item coverage, or
	// disposition disagrees with persisted facts; SM-2 establishes canonical
	// coverage from owner receipts below.
	assessment.CoverageWorkItemIDs = runtimeAssessmentCoverageIDs(assessment.CoverageWorkItemIDs)
	missingOwnerFacts := runtimecontract.RuntimeMissingOwnerFactRequirements(
		aggregate.Facts.Focus.RequiredOwnerFacts,
		aggregate.Facts.Resources.OperationAdmissibilities,
	)
	ownerFactAcquisition, hasOwnerFactAcquisition, err := currentRuntimeSemanticOwnerFactAcquisition(aggregate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	ownerFactAcquisitionSettled := hasOwnerFactAcquisition && len(missingOwnerFacts) == 0 && canonicalAssessmentProofCompleteWithoutOwnerFacts(aggregate)
	ownerFactOnlyGap := len(missingOwnerFacts) != 0 && canonicalAssessmentProofCompleteWithoutOwnerFacts(aggregate)
	canonicalCoverage := applyCanonicalAssessmentCoverage(aggregate, aggregate.ActiveDispatch.ID)
	canonicalGap := canonicalAssessmentCoverageGap(canonicalCoverage, aggregate.Facts.Focus, nil)
	if ownerFactOnlyGap {
		canonicalGap = canonicalAssessmentCoverageGap(canonicalCoverage, aggregate.Facts.Focus, missingOwnerFacts)
	}
	if ownerFactAcquisitionSettled {
		// This assessment closes only the manifest-sealed acquisition
		// transaction. Preserve its evidence lineage, but keep the Definition
		// CWI open: a fresh semantic generation must still bind and prove the
		// immutable terminal evidence contract.
		canonicalCoverage, err = retainRuntimeOwnerFactAcquisitionCoverageOpen(canonicalCoverage, aggregate.Facts.Focus)
		if err != nil {
			return RuntimeAggregate{}, err
		}
	}
	canonicalDisposition := runtimecontract.AssessmentDispositionSatisfied
	if canonicalGap != nil {
		canonicalDisposition = runtimecontract.AssessmentDispositionRepairableGap
	}
	aggregate.Facts.Coverage = canonicalCoverage
	// Coverage credit and its canonical CWI convergence state are one SM-2
	// transaction. Current-schema projection permits exactly one CWI per focus,
	// so an accepted assessment cannot leave a satisfied CWI paired with an
	// unresolved sibling's focus or repair state.
	settledCoverage, settleErr := settleRuntimeCWIsForCurrentFocus(aggregate.Facts)
	if settleErr != nil {
		return RuntimeAggregate{}, settleErr
	}
	aggregate.Facts = settledCoverage
	expectedRevision := aggregate.StateRevision
	fact := runtimecontract.RuntimeAssessmentFact{
		ID: aggregate.ActiveDispatch.ID, FocusID: assessment.FocusID, SliceID: firstNonEmpty(assessment.SliceID, aggregate.Facts.Focus.SliceID),
		Scope: aggregate.Facts.Focus.AssessmentScope, RequirementID: aggregate.Facts.Focus.AssessmentID,
		RequirementText: aggregate.Facts.Focus.AssessmentText, CoverageWorkItemIDs: assessment.CoverageWorkItemIDs,
		ClaimedDisposition: assessment.Disposition, Disposition: canonicalDisposition, Items: append([]runtimecontract.RuntimeAssessmentItemFact(nil), assessment.Items...),
		Summary: assessment.Summary, EvidenceRefs: assessment.EvidenceRefs, ArtifactRefs: assessment.ArtifactRefs,
	}
	if assessment.Gap != nil {
		gap := assessment.Gap.Normalized()
		fact.Gap = &gap
	}
	aggregate.Facts.Assessments = append(aggregate.Facts.Assessments, fact)
	if canonicalDisposition == runtimecontract.AssessmentDispositionRepairableGap && !ownerFactOnlyGap {
		priorGap := aggregate.Facts.Repair.ActiveGap
		// The producer's gap stays intact in AssessmentFact for audit. SM-2's
		// ledger receives only an owner-derived coverage/evidence classification:
		// no claimed disposition, missing kind, target, reason or free text can
		// alter its budget, closure or next action.
		signature := runtimecontract.StableAssessmentFailureSignature(aggregate.Facts.Focus.ID, runtimecontract.AssessmentDispositionRepairableGap, canonicalGap)
		canonicalGap.FailureSignature = signature
		cycleID := runtimecontract.StableRepairCycleID(string(aggregate.Facts.RunID), aggregate.Facts.Focus.ID, signature, fact.ID)
		ledger, _, cycleErr := beginRepairCycle(aggregate.Facts.Repair, aggregate.Facts.Focus.ID, aggregate.Facts.Focus.SliceID, cycleID, signature, aggregate.Facts.ConvergencePolicy)
		if cycleErr != nil {
			return RuntimeAggregate{}, cycleErr
		}
		ledger.ActiveGap = canonicalGap
		// A repairable assessment only resets the same-focus progress windows
		// when its persisted neutral gap set strictly narrows. The comparison
		// itself is a pure contract helper; SM-2 alone decides to apply it.
		// Repeated or expanded gaps retain their signatures/shapes and must not
		// create a fresh retry window merely because an assessment was accepted.
		if priorGap != nil && runtimecontract.RuntimeAssessmentGapStrictlyNarrows(*priorGap, *canonicalGap) {
			ledger = noteRepairGapProgress(ledger, *canonicalGap, aggregate.Facts.EvidenceRevision)
		} else {
			// An accepted operation is not harmless merely because its receipt
			// succeeded. Once owner-derived coverage reports the same (or a wider)
			// gap, retain every concrete current-focus operation shape that led
			// here. Candidate admission consumes this ledger on restart as well.
			ledger = recordAcceptedNoProgressShapes(aggregate, ledger, signature)
		}
		aggregate.Facts.Repair = ledger
	}
	if canonicalDisposition == runtimecontract.AssessmentDispositionSatisfied {
		if hasStateChangeNoOpWithoutProvenance(aggregate.Facts) {
			// A satisfied assessment can establish that the observed current state
			// meets this focus, but it must not erase the distinct no-op provenance
			// fact before the reducer consumes it. Retaining the exact rejected
			// operation shape makes the completion branch restart-safe and prevents
			// a later raw missing-state-change fallback from manufacturing a delta.
			aggregate.Facts.Repair = noteRepairProgressPreservingNoOpProvenance(aggregate.Facts.Repair, aggregate.Facts.EvidenceRevision)
		} else {
			aggregate.Facts.Repair = noteRepairProgress(aggregate.Facts.Repair, aggregate.Facts.EvidenceRevision)
		}
	}
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	dispatch := *aggregate.ActiveDispatch
	dispatch.State = DispatchStateCommitted
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
	aggregate.ActiveDispatch = nil
	previousFocusID := aggregate.Facts.Focus.ID
	replanned := false
	if ownerFactAcquisitionSettled {
		replanned, err = settleRuntimeOwnerFactAcquisition(&aggregate, fact.ID, ownerFactAcquisition)
		if err != nil {
			return RuntimeAggregate{}, err
		}
	} else if canonicalDisposition == runtimecontract.AssessmentDispositionRepairableGap && ownerFactOnlyGap {
		replanned, err = replanRuntimeCWIForUnmetOwnerFacts(&aggregate, fact.ID)
		if err != nil {
			return RuntimeAggregate{}, err
		}
	}
	if !replanned {
		advanced, advanceErr := advanceRuntimeCWIFocus(aggregate.Facts, aggregate.CoverageProofLineage)
		if advanceErr != nil {
			return RuntimeAggregate{}, advanceErr
		}
		aggregate.Facts = advanced
		if aggregate.Facts.Focus.ID != previousFocusID {
			// Reaching an unbound placeholder is a local SM-2 transition, not a
			// continuation chosen by the assessment. Retire only the just-sealed
			// batch working state so SM-4 can request the next scoped proof batch.
			if err := prepareSemanticPlanningForAdvancedFocus(&aggregate); err != nil {
				return RuntimeAggregate{}, err
			}
		}
	}
	facts, nextDecision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	// Validation-only accounting belongs to the canonical SM-2 result, never
	// to an assessment candidate's claimed missing evidence kind. Increment
	// after the budget fence selected the action so the next same-focus gap
	// observes the persisted retry that was actually authorized.
	if canonicalDisposition == runtimecontract.AssessmentDispositionRepairableGap && nextDecision.State == runtimecontract.ConvergenceStateNeedValidation {
		facts.Repair = markValidationOnlyRetry(facts.Repair)
	}
	aggregate.Facts = facts
	aggregate.Decision = nextDecision
	aggregate.StateRevision++
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func recordAcceptedNoProgressShapes(aggregate RuntimeAggregate, ledger runtimecontract.RuntimeRepairLedger, signature string) runtimecontract.RuntimeRepairLedger {
	for _, evidence := range aggregate.Facts.Evidence.Accepted {
		if !runtimeEvidenceAppliesToCurrentCoverageWithLineage(aggregate.Facts, evidence, aggregate.CoverageProofLineage) {
			continue
		}
		if evidence.Kind != runtimecontract.EvidenceKindStateChange {
			// A repair closure deliberately requires a fresh validation after its
			// durable effect. Keeping a prior validation shape in the generic
			// anti-replay ledger would reject that mandatory follow-up before SM-3
			// can seal it. State-change shapes remain replay-protected here; the
			// directive's evidence barrier governs validation freshness instead.
			continue
		}
		operation, found := runtimeOperationForEvidence(aggregate, evidence)
		if !found {
			continue
		}
		ledger = recordNoProgressOperationShape(ledger, runtimecontract.RuntimeRejectedOperationShape{
			Operation:        operation,
			EvidenceID:       evidence.ID,
			FailureClass:     runtimecontract.FailureClassCoverageAssessmentIncomplete,
			FailureCode:      "accepted_operation_same_gap",
			FailureSignature: signature,
			FailureMessage:   "accepted operation did not reduce the persisted assessment gap",
		})
	}
	return ledger
}

func runtimeAssessmentCoverageIDs(ids []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	seen := map[common.CoverageWorkItemID]struct{}{}
	ret := make([]common.CoverageWorkItemID, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ret = append(ret, id)
	}
	return ret
}

// applyCanonicalAssessmentCoverage closes coverage only from persisted,
// owner-confirmed evidence already bound to the current focus/item. Assessment
// candidate disposition, item satisfaction, missing kinds, targets and prose
// are deliberately absent from this classifier: they remain in the committed
// RuntimeAssessmentFact as producer observation/audit lineage.
func applyCanonicalAssessmentCoverage(aggregate RuntimeAggregate, assessmentID string) []runtimecontract.RuntimeCoverageItemFact {
	coverage := append([]runtimecontract.RuntimeCoverageItemFact(nil), aggregate.Facts.Coverage...)
	focus := aggregate.Facts.Focus.Normalized()
	for idx := range coverage {
		item := &coverage[idx]
		if item.SliceID != "" && focus.SliceID != "" && item.SliceID != focus.SliceID {
			continue
		}
		if !coverageWorkItemIDIncluded(focus.CoverageWorkItemIDs, item.ID) || item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			continue
		}
		refs, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, item.ID)
		item.LastAssessmentID = assessmentID
		item.EvidenceRefs = append(item.EvidenceRefs, refs...)
		if satisfied {
			item.State = runtimecontract.RuntimeCoverageItemStateSatisfied
		} else {
			item.State = runtimecontract.RuntimeCoverageItemStateUnsatisfied
		}
	}
	return runtimecontract.NormalizeRuntimeCoverageFacts(coverage)
}

func canonicalAssessmentEvidenceForCoverage(aggregate RuntimeAggregate, coverageID common.CoverageWorkItemID) ([]string, bool) {
	refs, satisfied := canonicalAssessmentEvidenceWithoutOwnerFactsForCoverage(aggregate, coverageID)
	if !satisfied {
		return refs, false
	}
	if len(runtimecontract.RuntimeMissingOwnerFactRequirements(aggregate.Facts.Focus.RequiredOwnerFacts, aggregate.Facts.Resources.OperationAdmissibilities)) != 0 {
		return refs, false
	}
	return refs, true
}

// canonicalAssessmentEvidenceWithoutOwnerFactsForCoverage evaluates the
// sealed proof transaction independently from profile-declared owner
// postconditions. Keeping these predicates separate lets SM-2 distinguish a
// genuine catalog postcondition gap from an ordinary missing/weak proof: only
// the former may retire the semantic generation and reopen the CWI.
func canonicalAssessmentEvidenceWithoutOwnerFactsForCoverage(aggregate RuntimeAggregate, coverageID common.CoverageWorkItemID) ([]string, bool) {
	focus := aggregate.Facts.Focus.Normalized()
	binding, bound := coverageEvidenceBindingForCoverage(focus, coverageID)
	// Current-schema bootstrap and state validation require a binding for every
	// CWI that may receive credit. An unbound focus can retain its action-level
	// requirements for diagnostic/repair presentation, but cannot close coverage.
	needed := stringSet(focus.RequiredEvidenceKinds)
	if bound {
		needed = stringSet(binding.EvidenceKinds())
	}
	seenKinds := make(map[string]struct{}, len(needed))
	refs := make([]string, 0)
	for _, evidence := range aggregate.Facts.Evidence.Accepted {
		if weakRuntimeEvidence(evidence) ||
			!runtimeEvidenceAppliesToCoverageWorkItemWithLineage(aggregate.Facts, evidence, coverageID, aggregate.CoverageProofLineage) {
			continue
		}
		if !runtimeEvidenceMatchesCoverageBindingWithLineage(aggregate.Facts, focus, coverageID, evidence, aggregate.CoverageProofLineage) {
			continue
		}
		if bound || assessmentFocusRequiresOperationEvidence(focus) {
			operation, found := runtimeOperationForEvidence(aggregate, evidence)
			if !found || !runtimeOperationAllowedForFocus(focus, operation) {
				continue
			}
			if focus.RequireDirectEvidence && strings.TrimSpace(evidence.Kind) == "" {
				continue
			}
		}
		refs = append(refs, string(evidence.ID))
		seenKinds[strings.TrimSpace(evidence.Kind)] = struct{}{}
	}
	if len(refs) == 0 {
		return nil, false
	}
	for kind := range needed {
		if _, ok := seenKinds[kind]; !ok {
			return nil, false
		}
	}
	// A binding can deliberately require a durable state-change receipt and a
	// validation receipt. The former proves the effect while the latter proves
	// executable behavior. Evaluate their complete proof set together; judging
	// every individual receipt as an executable behavior proof would reject the
	// state receipt by construction and make a valid conjunction impossible.
	if canonicalAssessmentEvidenceQualityFailure(aggregate, coverageID, refs) != "" {
		return nil, false
	}
	return refs, true
}

func canonicalAssessmentProofCompleteWithoutOwnerFacts(aggregate RuntimeAggregate) bool {
	focus := aggregate.Facts.Focus.Normalized()
	if len(focus.CoverageWorkItemIDs) == 0 {
		return false
	}
	for _, coverageID := range focus.CoverageWorkItemIDs {
		if _, satisfied := canonicalAssessmentEvidenceWithoutOwnerFactsForCoverage(aggregate, coverageID); !satisfied {
			return false
		}
	}
	return true
}

func coverageEvidenceBindingForCoverage(focus runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID) (runtimecontract.CoverageEvidenceBinding, bool) {
	return proof.CoverageBindingForCoverage(focus, coverageID)
}

func canonicalAssessmentCoverageGap(coverage []runtimecontract.RuntimeCoverageItemFact, focus runtimecontract.CoverageFocus, missingOwnerFacts []runtimecontract.RuntimeOwnerFactRequirement) *runtimecontract.RuntimeAssessmentGap {
	open := openCoverageIDs(coverage, focus.CoverageWorkItemIDs)
	if len(open) == 0 {
		return nil
	}
	gap := &runtimecontract.RuntimeAssessmentGap{
		CoverageWorkItemIDs: open,
		FailureClass:        runtimecontract.FailureClassCoverageAssessmentIncomplete,
		FailureCode:         "assessment_coverage_open",
	}
	if len(missingOwnerFacts) != 0 {
		gap.FailureCode = "assessment_owner_fact_open"
		gap.Reason = "required capability-owner fact is not present in the current catalog"
	}
	return gap
}

func openCoverageIDs(coverage []runtimecontract.RuntimeCoverageItemFact, ids []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	state := map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItemState{}
	for _, item := range coverage {
		state[item.ID] = item.State
	}
	ret := make([]common.CoverageWorkItemID, 0, len(ids))
	for _, id := range ids {
		if state[id] != runtimecontract.RuntimeCoverageItemStateSatisfied {
			ret = append(ret, id)
		}
	}
	return ret
}

func sameRuntimeDefinition(left runtimecontract.RuntimeFacts, right runtimecontract.RuntimeFacts) bool {
	left = left.Normalized()
	right = right.Normalized()
	// Focus is the persisted SM-2 cursor and legitimately changes as coverage
	// advances. Restart identity is limited to immutable run definition and
	// activation fields; comparing the current cursor with bootstrap focus 0
	// would reject every multi-focus run after its first atomic advance.
	return left.WorkspaceID == right.WorkspaceID &&
		left.RunID == right.RunID &&
		left.DefinitionRevision == right.DefinitionRevision &&
		runtimecontract.SameRuntimeActivationContext(left.Activation, right.Activation) &&
		runtimecontract.SameRuntimeExecutionBudgetPolicy(left.ExecutionBudgetPolicy, right.ExecutionBudgetPolicy)
}

func sealedDispatchesToRuntimeRecords(values []SealedDispatch) []runtimecontract.RuntimeDispatchRecord {
	ret := make([]runtimecontract.RuntimeDispatchRecord, 0, len(values))
	for _, value := range values {
		ret = append(ret, sealedDispatchToRuntimeRecord(value))
	}
	return ret
}

func sealedDispatchToRuntimeRecord(value SealedDispatch) runtimecontract.RuntimeDispatchRecord {
	return runtimecontract.RuntimeDispatchRecord{
		ID: value.ID, WorkspaceID: value.WorkspaceID, RunID: value.RunID,
		Transaction: value.Transaction, CandidateHash: value.CandidateHash, Candidate: value.Candidate,
		ValidationRecipes: runtimecontract.NormalizeRuntimeValidationRecipes(value.ValidationRecipes),
		State:             runtimecontract.RuntimeDispatchState(value.State), CompletedOperationIDs: append([]string(nil), value.CompletedOperationIDs...),
		OperationExecutions: runtimecontract.NormalizeRuntimeOperationExecutionRecords(value.OperationExecutions),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
