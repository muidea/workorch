package runtimev2

import (
	"fmt"
	"strings"
	"unicode"

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
	claim := runtimeCoverageClaimText(aggregate.Facts.Coverage, coverageID)
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
		if focus.RequireClaimEvidence && !runtimeEvidenceAlignsWithClaim(claim, operation, evidence) {
			continue
		}
		return ""
	}
	if !sawNonWeak {
		return "accepted evidence is weak, zero-work, or inconclusive"
	}
	if focus.RequireExecutableBehaviorEvidence && !sawExecutableBehavior {
		return "accepted evidence does not show executed behavior for the coverage claim"
	}
	if focus.RequireClaimEvidence {
		return "accepted evidence is not aligned with the concrete coverage claim"
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

func runtimeEvidenceAlignsWithClaim(claim string, operation runtimecontract.RuntimeOperation, evidence runtimecontract.EvidenceFact) bool {
	claim = strings.TrimSpace(claim)
	if claim == "" {
		return true
	}
	signals := []string{evidence.Summary, operation.ID, operation.Kind}
	signals = append(signals, operation.TargetRefs...)
	signals = append(signals, evidence.EvidenceRefs...)
	if evidence.Validation != nil {
		observation := evidence.Validation.Normalized()
		signals = append(signals, observation.PassedChecks...)
		signals = append(signals, observation.OutputPreview)
	}
	signalText := strings.Join(signals, " ")
	if !numericClaimEvidenceAligned(claim, signalText) {
		return false
	}
	claimTokens := alignmentTokens(claim)
	significant := make([]string, 0, len(claimTokens))
	seen := map[string]struct{}{}
	for _, token := range claimTokens {
		if len([]rune(token)) < 3 || alignmentStopToken(token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		significant = append(significant, token)
	}
	if claimAlignmentMayBeSkipped(significant, claim) {
		return true
	}
	signalTokens := stringSet(alignmentTokens(signalText))
	matched := 0
	for _, token := range significant {
		if _, ok := signalTokens[token]; ok {
			matched++
		}
	}
	required := (len(significant)*2 + 2) / 3
	if required < 2 {
		required = 2
	}
	if required > len(significant) {
		required = len(significant)
	}
	return matched >= required
}

func claimAlignmentMayBeSkipped(tokens []string, claim string) bool {
	if len(numericTokens(claim)) > 0 {
		return false
	}
	if len(tokens) == 0 {
		return true
	}
	return len(tokens) == 1 && len([]rune(tokens[0])) <= 8
}

func runtimeCoverageClaimText(values []runtimecontract.RuntimeCoverageItemFact, id common.CoverageWorkItemID) string {
	for _, value := range values {
		if value.ID == id {
			return strings.TrimSpace(value.TargetText)
		}
	}
	return ""
}

func alignmentTokens(value string) []string {
	value = splitAlignmentIdentifier(value)
	return strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func splitAlignmentIdentifier(value string) string {
	runes := []rune(value)
	if len(runes) < 2 {
		return value
	}
	var builder strings.Builder
	for idx, current := range runes {
		if idx > 0 {
			previous := runes[idx-1]
			var next rune
			if idx+1 < len(runes) {
				next = runes[idx+1]
			}
			letterDigitBoundary := unicode.IsLetter(previous) != unicode.IsLetter(current) &&
				(unicode.IsDigit(previous) || unicode.IsDigit(current))
			camelBoundary := unicode.IsLower(previous) && unicode.IsUpper(current)
			acronymBoundary := unicode.IsUpper(previous) && unicode.IsUpper(current) && next != 0 && unicode.IsLower(next)
			if letterDigitBoundary || camelBoundary || acronymBoundary {
				builder.WriteRune(' ')
			}
		}
		builder.WriteRune(current)
	}
	return builder.String()
}

func numericClaimEvidenceAligned(claim, signal string) bool {
	numbers := numericTokens(claim)
	if len(numbers) == 0 {
		return true
	}
	signalNumbers := stringSet(numericTokens(signal))
	for _, number := range numbers {
		if _, ok := signalNumbers[number]; ok {
			return true
		}
	}
	return false
}

func numericTokens(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return !unicode.IsDigit(r) })
}

func alignmentStopToken(value string) bool {
	switch value {
	case "test", "tests", "testing", "pass", "passed", "run", "runtime", "validation", "validate", "validated", "operation", "completed", "evidence":
		return true
	default:
		return false
	}
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
