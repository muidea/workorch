package runtimev2

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func applyAssessmentEvidenceGate(aggregate RuntimeAggregate, assessment runtimecontract.AssessmentCandidate) runtimecontract.AssessmentCandidate {
	assessment = assessment.Normalized()
	failed := make([]common.CoverageWorkItemID, 0)
	reasons := make([]string, 0)
	refs := make([]string, 0)
	for idx := range assessment.Items {
		item := &assessment.Items[idx]
		if !item.Satisfied {
			continue
		}
		itemRefs := item.EvidenceRefs
		if len(itemRefs) == 0 {
			itemRefs = assessment.EvidenceRefs
		}
		if reason := assessmentItemEvidenceGateFailure(aggregate, item.CoverageWorkItemID, itemRefs); reason != "" {
			item.Satisfied = false
			item.Reason = reason
			failed = append(failed, item.CoverageWorkItemID)
			reasons = append(reasons, fmt.Sprintf("%s: %s", item.CoverageWorkItemID, reason))
			refs = append(refs, itemRefs...)
		}
	}
	if len(failed) == 0 {
		return assessment
	}
	gap := runtimecontract.RuntimeAssessmentGap{}
	if assessment.Gap != nil {
		gap = assessment.Gap.Normalized()
	}
	gap.CoverageWorkItemIDs = append(gap.CoverageWorkItemIDs, failed...)
	gap.MissingEvidenceKinds = append(gap.MissingEvidenceKinds, runtimecontract.EvidenceKindValidation)
	gap.EvidenceRefs = append(gap.EvidenceRefs, refs...)
	if reason := strings.Join(reasons, "; "); reason != "" {
		if gap.Reason == "" {
			gap.Reason = reason
		} else {
			gap.Reason += "; " + reason
		}
	}
	gap.FailureClass = runtimecontract.FailureClassCoverageAssessmentIncomplete
	gap.FailureCode = "assessment_evidence_quality_gap"
	gap.FailureSignature = stableAssessmentEvidenceGateSignature(aggregate.Facts.Focus.ID, gap.CoverageWorkItemIDs)
	assessment.Disposition = runtimecontract.AssessmentDispositionRepairableGap
	assessment.Gap = &gap
	return assessment.Normalized()
}

func assessmentItemEvidenceGateFailure(aggregate RuntimeAggregate, coverageID common.CoverageWorkItemID, refs []string) string {
	focus := aggregate.Facts.Focus.Normalized()
	evidenceFacts := acceptedEvidenceFactsForRefs(aggregate.Facts, coverageID, refs)
	if len(evidenceFacts) == 0 {
		return "satisfied claim has no accepted evidence bound to the coverage item"
	}
	sawNonWeak := false
	sawExecutableBehavior := false
	for _, evidence := range evidenceFacts {
		if weakRuntimeEvidence(evidence) {
			continue
		}
		sawNonWeak = true
		if !assessmentFocusRequiresOperationEvidence(focus) {
			return ""
		}
		operation, found := runtimeOperationForEvidence(aggregate, evidence)
		if !found || !runtimeOperationAllowedForFocus(focus, operation) {
			continue
		}
		if focus.RequireDirectEvidence && evidence.Kind == "" {
			continue
		}
		if focus.RequireExecutableBehaviorEvidence && !runtimeEvidenceShowsExecutableBehavior(operation, evidence) {
			continue
		}
		sawExecutableBehavior = true
		// CoverageWorkItemID plus accepted evidence refs are the sealed,
		// structural claim binding. Semantic comparison between Goal/DoD prose,
		// summary, operation ID and check name belongs to the assessment
		// candidate, never to the Runtime reducer.
		return ""
	}
	if !sawNonWeak {
		return "accepted evidence is weak, zero-work, or inconclusive"
	}
	if focus.RequireExecutableBehaviorEvidence && !sawExecutableBehavior {
		return "accepted evidence does not show executed behavior for the coverage claim"
	}
	return "accepted evidence does not satisfy the focus evidence contract"
}

func assessmentFocusRequiresOperationEvidence(focus runtimecontract.CoverageFocus) bool {
	return focus.RequireDirectEvidence || focus.RequireClaimEvidence || focus.RequireExecutableBehaviorEvidence ||
		len(focus.AcceptedStepKinds) > 0 || len(focus.RejectedStepKinds) > 0
}

func acceptedEvidenceFactsForRefs(facts runtimecontract.RuntimeFacts, coverageID common.CoverageWorkItemID, refs []string) []runtimecontract.EvidenceFact {
	wanted := stringSet(refs)
	ret := make([]runtimecontract.EvidenceFact, 0)
	for _, evidence := range facts.Evidence.Accepted {
		if !coverageWorkItemIDIncluded(evidence.CoverageWorkItemIDs, coverageID) {
			continue
		}
		matched := false
		for _, ref := range append([]string{string(evidence.ID), evidence.SourceRef}, evidence.EvidenceRefs...) {
			if _, ok := wanted[strings.TrimSpace(ref)]; ok {
				matched = true
				break
			}
		}
		if matched {
			ret = append(ret, evidence)
		}
	}
	return ret
}

func runtimeOperationForEvidence(aggregate RuntimeAggregate, evidence runtimecontract.EvidenceFact) (runtimecontract.RuntimeOperation, bool) {
	for _, dispatch := range aggregate.CompletedDispatches {
		if evidence.DispatchID != "" && dispatch.ID != evidence.DispatchID {
			continue
		}
		for _, operation := range candidateRuntimeOperations(dispatch.Candidate) {
			operation = operation.Normalized()
			if operation.ID == evidence.OperationID {
				return operation, true
			}
		}
	}
	if aggregate.ActiveDispatch != nil {
		for _, operation := range candidateRuntimeOperations(aggregate.ActiveDispatch.Candidate) {
			operation = operation.Normalized()
			if operation.ID == evidence.OperationID {
				return operation, true
			}
		}
	}
	return runtimecontract.RuntimeOperation{}, false
}

func runtimeOperationAllowedForFocus(focus runtimecontract.CoverageFocus, operation runtimecontract.RuntimeOperation) bool {
	kind := runtimecontract.CanonicalRuntimeBlockKind(operation.Kind)
	if stringIncluded(focus.RejectedStepKinds, kind) {
		return false
	}
	return len(focus.AcceptedStepKinds) == 0 || stringIncluded(focus.AcceptedStepKinds, kind)
}

func weakRuntimeEvidence(evidence runtimecontract.EvidenceFact) bool {
	quality := common.EvidenceQuality(strings.TrimSpace(string(evidence.Quality)))
	return evidence.ZeroWork || quality == common.EvidenceQualityInvalid || quality == common.EvidenceQualityInconclusive || quality == common.EvidenceQualitySkipped
}

func runtimeEvidenceShowsExecutableBehavior(operation runtimecontract.RuntimeOperation, evidence runtimecontract.EvidenceFact) bool {
	kind := runtimecontract.CanonicalRuntimeBlockKind(operation.Kind)
	if kind != runtimecontract.StepKindShellCommand && kind != runtimecontract.RuntimeBlockKindManagedValidationSession {
		return true
	}
	if evidence.Validation == nil {
		return false
	}
	observation := evidence.Validation.Normalized()
	return len(observation.PassedChecks) > 0
}

func coverageWorkItemIDIncluded(values []common.CoverageWorkItemID, expected common.CoverageWorkItemID) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func stringIncluded(values []string, expected string) bool {
	for _, value := range values {
		if runtimecontract.CanonicalRuntimeBlockKind(value) == expected {
			return true
		}
	}
	return false
}

func stableAssessmentEvidenceGateSignature(focusID string, ids []common.CoverageWorkItemID) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, string(id))
	}
	return "assessment-evidence-quality/" + strings.TrimSpace(focusID) + "/" + strings.Join(values, ",")
}
