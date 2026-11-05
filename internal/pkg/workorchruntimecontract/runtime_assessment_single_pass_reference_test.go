package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"strings"
)

// Frozen complete pre-change validators; test-only independent rejection oracles.
func referenceAssessmentAuthorityBeforeSinglePass(a RuntimeAssessmentAuthorityInput, transaction AuthorityTransaction, bindings []CoverageEvidenceBinding) error {
	if !reflect.DeepEqual(a, a.Normalized()) {
		return fmt.Errorf("runtime assessment authority input must be raw canonical")
	}
	a = a.Normalized()
	if transaction.State != ConvergenceStateNeedAssessment || a.Focus.ID == "" || a.Focus.ID != transaction.FocusID {
		return fmt.Errorf("runtime assessment authority input requires its assessment transaction focus")
	}
	if a.Purpose != RuntimeAssessmentPurposeDefinitionProof && a.Purpose != RuntimeAssessmentPurposeOwnerFactAcquisition {
		return fmt.Errorf("runtime assessment authority input has unsupported purpose %q", a.Purpose)
	}
	if a.SourceSliceID == "" || a.SourceScope == "" || a.SourceID == "" || a.SourceText == "" {
		return fmt.Errorf("runtime assessment authority input requires exact definition source")
	}
	if err := a.AuthorityInput.Validate(); err != nil || StableRuntimeEquivalentAuthorityInputID(a.AuthorityInput) != transaction.AuthorityInputID {
		return fmt.Errorf("runtime assessment authority input identity mismatch")
	}
	if StableRuntimeAuthorityFocusHash(a.Focus) != a.AuthorityInput.FocusHash ||
		StableRuntimeOperationAdmissibilityCatalogRevisionHash(a.OperationAdmissibilities) != a.AuthorityInput.CatalogRevisionHash {
		return fmt.Errorf("runtime assessment authority input preimage mismatch")
	}
	if !RuntimeCoverageEvidenceBindingProjectionsEqual(a.Focus.CoverageEvidenceBindings, bindings) {
		return fmt.Errorf("runtime assessment authority input binding preimage mismatch")
	}
	for _, observation := range a.ApplicabilityObservations {
		if err := observation.ValidateRaw(); err != nil {
			return err
		}
		if observation.Request.EvidenceRevision > transaction.EvidenceRevision {
			return fmt.Errorf("assessment contains future applicability observation")
		}
	}
	if StableRuntimeEvidenceApplicabilitySemanticHash(a.ApplicabilityObservations) != a.AuthorityInput.ApplicabilityObservationHash {
		return fmt.Errorf("assessment applicability observations do not match sealed authority identity")
	}
	return nil
}

func referenceAssessmentAdjudicationBeforeSinglePass(a RuntimeAssessmentAdjudication) error {
	if !reflect.DeepEqual(a, a.Normalized()) {
		return fmt.Errorf("runtime assessment adjudication must be raw canonical")
	}
	a = a.Normalized()
	if a.DispatchID == "" || a.AuthorityInputID == "" || a.FocusID == "" || len(a.CoverageWorkItemIDs) == 0 {
		return fmt.Errorf("runtime assessment adjudication requires dispatch, authority, focus and coverage")
	}
	if a.Disposition != AssessmentDispositionSatisfied && a.Disposition != AssessmentDispositionRepairableGap {
		return fmt.Errorf("runtime assessment adjudication has non-canonical disposition %q", a.Disposition)
	}
	if a.Disposition == AssessmentDispositionSatisfied && len(a.Proofs) == 0 {
		return fmt.Errorf("satisfied runtime assessment adjudication requires proof")
	}
	for _, proof := range a.Proofs {
		if err := ValidateRuntimeEvidencePurposeKind(proof.ApplicabilityDependencies, proof.EvidenceKind); err != nil {
			return err
		}
		if !reflect.DeepEqual(proof, proof.Normalized()) || proof.CoverageWorkItemID == "" || proof.EvidenceKind == "" || proof.EvidenceFactID == "" || proof.EvidenceFactHash == "" {
			return fmt.Errorf("runtime assessment adjudication contains malformed proof")
		}
		if err := ValidateRuntimeEvidenceDependencyPair(proof.OwnerRevisionSet, proof.ApplicabilityDependencies, ""); err != nil {
			return fmt.Errorf("runtime assessment proof owner dependencies: %w", err)
		}
	}
	seenRequirements := make(map[RuntimeOwnerFactRequirement]struct{}, len(a.OwnerFactSources))
	for _, source := range a.OwnerFactSources {
		if source.Requirement != source.Requirement.Normalized() || source.Requirement.Validate() != nil {
			return fmt.Errorf("runtime assessment owner-fact source has malformed requirement")
		}
		if _, duplicate := seenRequirements[source.Requirement]; duplicate {
			return fmt.Errorf("runtime assessment owner-fact source duplicates requirement")
		}
		seenRequirements[source.Requirement] = struct{}{}
		if source.CatalogSnapshotID != strings.TrimSpace(source.CatalogSnapshotID) || (source.CatalogSnapshotID == "") != (source.OwnerRevisionSet == nil) {
			return fmt.Errorf("runtime assessment owner-fact source catalog identity mismatch")
		}
		if source.OwnerRevisionSet != nil && len(source.OwnerRevisionSet.Entries) != 1 {
			return fmt.Errorf("runtime assessment owner-fact source must identify one exact observation")
		}
		if err := validateRuntimeOwnerRevisionSetRaw(source.OwnerRevisionSet); err != nil {
			return fmt.Errorf("runtime assessment owner-fact source dependencies: %w", err)
		}
	}
	return nil
}
