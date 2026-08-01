package runtime

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// canonicalAssessmentEvidenceQualityFailure evaluates only persisted,
// owner-confirmed evidence for one coverage item. It deliberately receives no
// assessment candidate: producer disposition, item satisfaction, references,
// prose and gap data remain audit observations and cannot be rewritten into
// canonical repair authority.
func canonicalAssessmentEvidenceQualityFailure(aggregate RuntimeAggregate, coverageID common.CoverageWorkItemID, refs []string) string {
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
		if !runtimeEvidenceMatchesCoverageBinding(focus, coverageID, evidence) {
			continue
		}
		if focus.RequireExecutableBehaviorEvidence && !runtimeEvidenceShowsExecutableBehavior(focus, coverageID, operation, evidence) {
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

func runtimeEvidenceMatchesCoverageBinding(focus runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID, evidence runtimecontract.EvidenceFact) bool {
	focus = focus.Normalized()
	if len(focus.CoverageEvidenceBindings) == 0 {
		return true // only unpersisted, isolated contract fixtures may omit bindings.
	}
	for _, ref := range evidence.BindingRefs {
		binding, found := runtimecontract.CoverageEvidenceBindingForRef(focus.CoverageEvidenceBindings, ref)
		if found && binding.CoverageWorkItemID == coverageID {
			_, proofFound := runtimecontract.CoverageEvidenceProofForKind(binding, evidence.Kind)
			if proofFound {
				return true
			}
		}
	}
	return false
}

func runtimeEvidenceShowsExecutableBehavior(focus runtimecontract.CoverageFocus, coverageID common.CoverageWorkItemID, operation runtimecontract.RuntimeOperation, evidence runtimecontract.EvidenceFact) bool {
	_ = operation // binding, not operation-name heuristics, defines behavior proof.
	focus = focus.Normalized()
	for _, ref := range evidence.BindingRefs {
		binding, found := runtimecontract.CoverageEvidenceBindingForRef(focus.CoverageEvidenceBindings, ref)
		if !found || binding.CoverageWorkItemID != coverageID || evidence.Validation == nil {
			continue
		}
		proof, proofFound := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
		if !proofFound {
			continue
		}
		observation := evidence.Validation.Normalized()
		if observation.RecipeID != proof.ValidationRecipeID {
			continue
		}
		for _, required := range proof.RequiredCheckRefs {
			if !stringIncluded(observation.PassedChecks, required) {
				return false
			}
		}
		return len(proof.RequiredCheckRefs) != 0
	}
	// No non-empty PassedChecks fallback: a behavior proof without a declared
	// binding recipe/check set is unavailable rather than implicitly satisfied.
	return false
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
