// Package proof contains stateless evidence and binding predicates.  It does
// not credit a CWI, mutate a ledger, or decide whether an assessment reopens
// a focus; those decisions remain in the Orchestrator owner.
package proof

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func CoverageWorkItemIncluded(values []common.CoverageWorkItemID, expected common.CoverageWorkItemID) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func EvidenceReferencesBinding(evidence runtimecontract.EvidenceFact, bindingRef string) bool {
	bindingRef = strings.TrimSpace(bindingRef)
	if bindingRef == "" {
		return false
	}
	for _, value := range evidence.BindingRefs {
		if strings.TrimSpace(value) == bindingRef {
			return true
		}
	}
	return false
}

func WeakEvidence(evidence runtimecontract.EvidenceFact) bool {
	quality := common.EvidenceQuality(strings.TrimSpace(string(evidence.Quality)))
	return evidence.ZeroWork || quality == common.EvidenceQualityInvalid || quality == common.EvidenceQualityInconclusive || quality == common.EvidenceQualitySkipped
}

func OperationAllowedForFocus(focus runtimecontract.CoverageFocus, operation runtimecontract.RuntimeOperation) bool {
	kind := runtimecontract.CanonicalRuntimeBlockKind(operation.Kind)
	if stringIncluded(focus.RejectedStepKinds, kind) {
		return false
	}
	return len(focus.AcceptedStepKinds) == 0 || stringIncluded(focus.AcceptedStepKinds, kind)
}

// ValidationEvidenceShowsExecutableBehavior checks only the sealed validation
// proof declaration and owner receipt fields. It intentionally has no fallback
// based on arbitrary passed checks when no proof recipe was sealed.
func ValidationEvidenceShowsExecutableBehavior(binding runtimecontract.CoverageEvidenceBinding, evidence runtimecontract.EvidenceFact) bool {
	if evidence.Validation == nil {
		return false
	}
	proof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
	if !found {
		return false
	}
	return runtimecontract.RuntimeValidationObservationSatisfiesCoverageProof(proof, *evidence.Validation)
}

func SameProofLineageResolution(
	lineage []runtimecontract.RuntimeCoverageProofLineage,
	sourceLineageID string,
	currentBinding runtimecontract.CoverageEvidenceBinding,
	currentIdentity runtimecontract.CoverageEvidenceProofIdentity,
) bool {
	for _, resolution := range lineage {
		resolution = resolution.Normalized()
		if resolution.Kind != runtimecontract.RuntimeCoverageProofLineageKindResolution ||
			resolution.SourceLineageID != strings.TrimSpace(sourceLineageID) ||
			resolution.CurrentBindingRef != currentBinding.ID || resolution.CurrentBindingHash != currentBinding.Hash ||
			resolution.Disposition != runtimecontract.RuntimeCoverageProofLineageDispositionEquivalent ||
			!runtimecontract.SameCoverageEvidenceProofIdentity(resolution.CurrentProofIdentity, currentIdentity) {
			continue
		}
		return true
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
