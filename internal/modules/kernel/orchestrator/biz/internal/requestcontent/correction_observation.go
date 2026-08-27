package requestcontent

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ProjectRuntimeCandidateCorrectionObservation is the sole cross-owner
// projection from the persisted SM-3 failure. Audit reason and validation
// detail remain private to the Orchestrator aggregate.
func ProjectRuntimeCandidateCorrectionObservation(failure *runtimecontract.RuntimeCandidateFailure, correction *runtimecontract.RuntimeCandidateCorrectionDirective) (*runtimecontract.RuntimeCandidateCorrectionObservation, error) {
	if failure == nil && correction == nil {
		return nil, nil
	}
	if failure == nil || correction == nil {
		return nil, fmt.Errorf("project runtime candidate correction observation requires paired failure and correction")
	}
	sealedCorrection := correction.Normalized()
	if err := sealedCorrection.Validate(); err != nil {
		return nil, fmt.Errorf("project runtime candidate correction observation: %w", err)
	}
	if sealedCorrection.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || sealedCorrection.Witness == nil {
		return nil, fmt.Errorf("project runtime candidate correction observation requires a correctable-once directive")
	}
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: failure.Class, Code: failure.Code, Signature: failure.Signature,
		TargetRefs: append([]string(nil), failure.TargetRefs...), Diagnostic: failure.Diagnostic.Clone(),
		SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(
			sealedCorrection.ManifestHash, sealedCorrection.Objectives, sealedCorrection.Witness,
		),
	}
	if observation.Diagnostic == nil || observation.Diagnostic.Normalized().ManifestHash != sealedCorrection.ManifestHash ||
		!runtimeCandidateCorrectionObjectivesEqual(observation.Diagnostic.Normalized().Objectives, sealedCorrection.Objectives) {
		return nil, fmt.Errorf("project runtime candidate correction observation has stale failure/directive lineage")
	}
	if err := observation.Validate(); err != nil {
		return nil, fmt.Errorf("project runtime candidate correction observation: %w", err)
	}
	value := observation.Clone()
	return &value, nil
}

func runtimeCandidateCorrectionObjectivesEqual(left, right []runtimecontract.RuntimeCandidateCorrectionObjective) bool {
	left = (&runtimecontract.RuntimeCandidateFailureDiagnostic{Objectives: append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), left...)}).Normalized().Objectives
	right = (&runtimecontract.RuntimeCandidateFailureDiagnostic{Objectives: append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), right...)}).Normalized().Objectives
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

// ProjectRuntimeSemanticCorrectionObservation is the sole cross-owner
// projection from the producer-authored audit diagnostic. The returned DTO
// cannot represent message/details/raw/validation/governance fields.
func ProjectRuntimeSemanticCorrectionObservation(diagnostic *runtimecontract.RuntimeOriginalDiagnostic) (*runtimecontract.RuntimeSemanticCorrectionObservation, error) {
	if diagnostic == nil {
		return nil, nil
	}
	if err := diagnostic.Validate(); err != nil {
		return nil, err
	}
	observation := runtimecontract.RuntimeSemanticCorrectionObservation{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionObservationSchemaCurrent,
		OwnerModuleID: diagnostic.OwnerModuleID, Class: diagnostic.OriginalClass, Code: diagnostic.OriginalCode,
		Signature: diagnostic.Signature, OperationID: diagnostic.OperationID,
		TargetRefs: append([]string(nil), diagnostic.TargetRefs...), ExpectedRevision: diagnostic.ExpectedRevision,
		ObservedRevision:   diagnostic.ObservedRevision,
		FieldViolations:    runtimecontract.CloneRuntimeFieldViolations(diagnostic.FieldViolations),
		SemanticViolations: append([]runtimecontract.RuntimeSemanticContractViolation(nil), diagnostic.SemanticViolations...),
	}
	if diagnostic.TargetObservation != nil {
		value := diagnostic.TargetObservation.Clone()
		observation.TargetObservation = &value
	}
	if err := observation.Validate(); err != nil {
		return nil, fmt.Errorf("project runtime semantic correction observation: %w", err)
	}
	value := observation.Clone()
	return &value, nil
}
