package runtime

import (
	"fmt"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"strings"
)

// Complete functions frozen before the history-source closure. They intentionally
// retain their original catalog selection/normalization and admission boundaries.
func historySourceReferenceRuntimeValidationProofBindingForOperation(dispatch runtimecontract.RuntimeDispatchRecord, operation runtimecontract.RuntimeOperation, catalog *runtimecontract.RuntimeOperationAdmissibility) (runtimecontract.RuntimeValidationProofRequirementBinding, error) {
	if runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) != runtimecontract.RuntimeBlockKindManagedValidationSession {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, nil
	}
	if catalog == nil {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, fmt.Errorf("managed-validation execution requires persisted owner catalog")
	}
	if len(operation.BindingRefs) != 1 || strings.TrimSpace(operation.BindingRefs[0]) == "" || operation.BindingRefs[0] != strings.TrimSpace(operation.BindingRefs[0]) {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, fmt.Errorf("managed-validation execution requires exactly one persisted CWI binding")
	}
	operation = operation.Normalized()
	coverageBinding, found := runtimecontract.CoverageEvidenceBindingForRef(dispatch.CoverageBindings, operation.BindingRefs[0])
	if !found {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, fmt.Errorf("managed-validation execution binding %q is absent from sealed dispatch", operation.BindingRefs[0])
	}
	proof, found := runtimecontract.CoverageEvidenceProofForKind(coverageBinding, runtimecontract.EvidenceKindValidation)
	if !found || proof.ValidationRecipeID == "" || proof.ValidationProofRequirementHash == "" || len(proof.ValidationCheckAttestations) == 0 {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, fmt.Errorf("managed-validation execution binding %q has no exact native validation proof", coverageBinding.ID)
	}
	wanted := runtimecontract.RuntimeValidationProofRequirementBinding{
		RequirementHash:    proof.ValidationProofRequirementHash,
		DefinitionRevision: coverageBinding.DefinitionRevision,
		CoverageWorkItemID: coverageBinding.CoverageWorkItemID,
		DeclarationRef:     coverageBinding.DeclarationRef,
		ValidationRecipeID: proof.ValidationRecipeID,
		RequiredCheckRefs:  append([]string(nil), proof.RequiredCheckRefs...),
		CheckAttestations:  runtimecontract.CloneRuntimeValidationCheckAttestationBindings(proof.ValidationCheckAttestations),
	}.Clone()
	if err := wanted.Validate(); err != nil {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, fmt.Errorf("managed-validation persisted CWI proof: %w", err)
	}

	if err := catalog.Validate(); err != nil {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, fmt.Errorf("managed-validation persisted owner catalog: %w", err)
	}
	ownerCatalog := catalog.Normalized()
	surface, found := runtimecontract.RuntimeValidationSurfaceForID(ownerCatalog.ValidationSurfaces, wanted.ValidationRecipeID)
	if !found || surface.Readiness != runtimecontract.RuntimeValidationSurfaceReady || surface.ProofApplicability != runtimecontract.RuntimeValidationProofDirect {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, fmt.Errorf("managed-validation recipe %q has no ready exact owner surface", wanted.ValidationRecipeID)
	}
	var selected *runtimecontract.RuntimeValidationProofRequirementBinding
	for _, raw := range surface.ProofRequirementBindings {
		binding := raw.Clone()
		if binding.RequirementHash != wanted.RequirementHash || binding.CoverageWorkItemID != wanted.CoverageWorkItemID {
			continue
		}
		if selected != nil {
			return runtimecontract.RuntimeValidationProofRequirementBinding{}, fmt.Errorf("managed-validation owner surface duplicates exact CWI proof binding")
		}
		value := binding
		selected = &value
	}
	if selected == nil || !sameRuntimeValidationExecutionProofBinding(*selected, wanted) {
		return runtimecontract.RuntimeValidationProofRequirementBinding{}, fmt.Errorf("managed-validation persisted CWI proof does not match exact owner surface binding")
	}
	return selected.Clone(), nil
}

func historySourceReferenceRuntimeOwnerCatalogObservationForOperation(values []runtimecontract.RuntimePrerequisiteObservation, catalog *runtimecontract.RuntimeOperationAdmissibility) *runtimecontract.RuntimePrerequisiteObservation {
	if catalog == nil {
		return nil
	}
	catalogValue := catalog.Normalized()
	for _, raw := range values {
		observation := raw.Normalized()
		if observation.PrerequisiteID != catalogValue.PrerequisiteID || observation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility ||
			observation.OwnerModuleID != catalogValue.OwnerModuleID || observation.ContentHash != catalogValue.SnapshotID || observation.Validate() != nil {
			continue
		}
		observation.Dependencies = append([]runtimecontract.RuntimeFreshnessDependency(nil), observation.Dependencies...)
		return &observation
	}
	return nil
}

func historySourceReferenceRuntimeOwnerCatalogForOperation(values []runtimecontract.RuntimeOperationAdmissibility, observations []runtimecontract.RuntimePrerequisiteObservation, operation runtimecontract.RuntimeOperation, requiredSnapshotID, requiredAcquisitionBinding string) *runtimecontract.RuntimeOperationAdmissibility {
	owner := runtimecontract.RuntimeModuleForOperationKind(operation.Kind)
	canonicalKind := runtimecontract.CanonicalRuntimeBlockKind(operation.Kind)
	requiredSnapshotID = strings.TrimSpace(requiredSnapshotID)
	requiredAcquisitionBinding = strings.TrimSpace(requiredAcquisitionBinding)
	// SM-3 execution authority comes only from the catalog identity sealed into
	// the execution record.  Looking up an arbitrary current catalog by owner and
	// operation kind would silently replace missing authority after restart.
	if requiredSnapshotID == "" {
		return nil
	}
	for _, catalog := range values {
		catalog = catalog.Normalized()
		if catalog.OwnerModuleID != owner || catalog.SnapshotID != requiredSnapshotID || catalog.AcquisitionBinding != requiredAcquisitionBinding ||
			!runtimeCatalogContainsOperationKind(catalog, canonicalKind) || catalog.Validate() != nil {
			continue
		}
		if historySourceReferenceRuntimeOwnerCatalogObservationForOperation(observations, &catalog) == nil {
			continue
		}
		value := catalog
		return &value
	}
	return nil
}

func historySourceReferenceRuntimeValidationScopeTargetRefs(dispatch runtimecontract.RuntimeDispatchRecord, operation runtimecontract.RuntimeOperation) ([]string, error) {
	if runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) != runtimecontract.RuntimeBlockKindManagedValidationSession {
		return nil, nil
	}
	if _, err := runtimeValidationTargetAuthorization(dispatch, operation); err != nil {
		return nil, err
	}
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, operation.ID)
	if !found || execution.OperationHash != runtimecontract.StableRuntimeOperationHash(operation) || execution.AdmissibilitySnapshotID == "" {
		return nil, fmt.Errorf("sealed managed-validation operation %q has no exact owner catalog authority", operation.ID)
	}
	catalog := historySourceReferenceRuntimeOwnerCatalogForOperation(
		dispatch.OperationAdmissibilities,
		dispatch.OperationAdmissibilityObservations,
		operation,
		execution.AdmissibilitySnapshotID,
		execution.AcquisitionBinding,
	)
	if catalog == nil {
		return nil, fmt.Errorf("sealed managed-validation operation %q has no complete owner catalog preimage", operation.ID)
	}
	scope := append([]string(nil), catalog.Normalized().TargetRefs...)
	if !runtimeValidationTargetsWithinScope(operation.TargetRefs, scope) {
		return nil, fmt.Errorf("sealed managed-validation operation %q target is outside owner diagnostic scope", operation.ID)
	}
	return scope, nil
}
