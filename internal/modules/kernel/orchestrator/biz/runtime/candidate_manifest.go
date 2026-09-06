package runtime

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// BuildRuntimeCandidateContractManifestForAggregate is the single SM-3
// projection from persisted SM-2 authority into the pre-seal candidate
// manifest. Assessment purpose is not a default: the immutable Definition
// source and the exact accepted semantic binding generation must both prove
// it before any provider request can be opened.
func BuildRuntimeCandidateContractManifestForAggregate(aggregate RuntimeAggregate) (runtimecontract.RuntimeCandidateContractManifest, error) {
	if err := rejectCandidateBeforeRepairSurfaceConfirmation(aggregate); err != nil {
		return runtimecontract.RuntimeCandidateContractManifest{}, err
	}
	ownerFacts := runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
		runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
	)
	if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
		focus := aggregate.Decision.Focus.Normalized()
		if _, err := runtimecontract.RuntimeAssessmentSourceProjection(aggregate.Facts, focus.ID, focus.CoverageWorkItemIDs); err != nil {
			return runtimecontract.RuntimeCandidateContractManifest{}, fmt.Errorf("runtime assessment candidate Definition preimage: %w", err)
		}
		preimage, err := runtimecontract.ResolveRuntimeAssessmentPurposePreimage(
			aggregate.SemanticPlanning,
			focus.CoverageEvidenceBindings,
		)
		if err != nil {
			return runtimecontract.RuntimeCandidateContractManifest{}, fmt.Errorf("runtime assessment candidate purpose preimage: %w", err)
		}
		ownerFacts.AssessmentPurpose = preimage.Purpose
		ownerFacts.AssessmentPurposeManifestHash = preimage.SemanticManifestHash
	}
	return runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, ownerFacts)
}

// ValidateRuntimeCandidateContractManifestForAggregate is the aggregate-aware
// exact replay fence used before and after provider transport. Downstream
// owners use only the intrinsic sealed-decision validator.
func ValidateRuntimeCandidateContractManifestForAggregate(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest) error {
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForSealedDecision(manifest, aggregate.Decision); err != nil {
		return err
	}
	expected, err := BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if err != nil {
		return err
	}
	if manifest.Hash != expected.Hash {
		return fmt.Errorf("runtime candidate manifest does not match persisted aggregate authority")
	}
	return nil
}
