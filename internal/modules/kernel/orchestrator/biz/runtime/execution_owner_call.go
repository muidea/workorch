package runtime

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (x *activeDispatchExecution) operationExecutionCallHandoff(operation runtimecontract.RuntimeOperation, prepared preparedActiveDispatchOperation) (*RuntimeOperationExecutionCallHandoff, error) {
	validationScopeTargetRefs, err := runtimeValidationScopeTargetRefs(x.dispatch, operation)
	if err != nil {
		return nil, err
	}
	ownerCatalog := runtimeOwnerCatalogForOperation(
		x.dispatch.OperationAdmissibilities,
		x.dispatch.OperationAdmissibilityObservations,
		operation,
		prepared.execution.AdmissibilitySnapshotID,
		prepared.execution.AcquisitionBinding,
	)
	validationProofBinding, err := runtimeValidationProofBindingForOperation(x.dispatch, operation, ownerCatalog)
	if err != nil {
		return nil, err
	}
	handoff := &RuntimeOperationExecutionCallHandoff{Request: RuntimeOperationExecutionRequest{
		WorkspaceID: x.workspaceID, RunID: x.runID, DispatchID: x.dispatch.ID,
		Transaction: x.dispatch.Transaction, Operation: operation, Execution: prepared.execution,
		GovernanceGrant:                    prepared.governanceGrant,
		EvidenceKind:                       evidenceKindForDecision(x.aggregate.Decision),
		HistoricalEffectProofContractRef:   runtimeHistoricalEffectProofContractForDispatch(x.dispatch, operation),
		TargetRole:                         x.dispatch.FocusTargetRole,
		EvidencePolicies:                   x.aggregate.Facts.Activation.EvidencePolicies,
		ValidationRecipes:                  runtimecontract.NormalizeRuntimeValidationRecipes(x.dispatch.ValidationRecipes),
		ValidationScopeTargetRefs:          validationScopeTargetRefs,
		ValidationProofBinding:             validationProofBinding,
		CapabilityStates:                   append([]runtimecontract.RuntimeCapabilityStateSnapshot(nil), x.aggregate.Facts.Resources.CapabilityStates...),
		WorkspaceTreeObservation:           runtimeWorkspaceManifestObservation(x.aggregate),
		CompletedRequiredOperationReceipts: completedRequiredOperationReceiptBindings(*x.aggregate.ActiveDispatch, x.aggregate.OperationReceipts, x.completed),
		OwnerCatalog:                       ownerCatalog,
		OwnerCatalogObservation:            runtimeOwnerCatalogObservationForOperation(x.dispatch.OperationAdmissibilityObservations, ownerCatalog),
	}}
	if x.recoveryAuthorization != nil {
		handoff.RecoveryAuthorizationID = x.recoveryAuthorization.AuthorizationID
		handoff.RecoveryClaimID = x.recoveryAuthorization.ClaimID
	}
	if err := handoff.Validate(); err != nil {
		return nil, err
	}
	return handoff, nil
}

// Detached value projection of the purpose fields only. The enclosing
// aggregate/dispatch gate independently validates the complete authority.
func runtimeHistoricalEffectProofContractForDispatch(dispatch SealedDispatch, operation runtimecontract.RuntimeOperation) string {
	return runtimecontract.RuntimeHistoricalEffectProofContractForDispatch(runtimeHistoricalEffectPurposeDispatch(dispatch), operation)
}

func runtimeHistoricalEffectPurposeDispatch(dispatch SealedDispatch) runtimecontract.RuntimeDispatchRecord {
	return runtimecontract.RuntimeDispatchRecord{
		Transaction: dispatch.Transaction, CoverageBindings: dispatch.CoverageBindings,
		RepairCommitment: dispatch.RepairCommitment, RepairCommitmentHash: dispatch.RepairCommitmentHash,
		ImplementationWorkCommitment: dispatch.ImplementationWorkCommitment,
	}
}

// runtimeValidationProofBindingForOperation selects the immutable Shell proof
// mapping needed by one managed-validation execution. The persisted semantic
// binding and the exact owner-catalog binding must agree byte-for-byte on all
// owner-neutral identities before the latter may cross EventHub. Native check
// and protocol values remain opaque here.
func runtimeValidationProofBindingForOperation(dispatch SealedDispatch, operation runtimecontract.RuntimeOperation, catalog *runtimecontract.RuntimeOperationAdmissibility) (runtimecontract.RuntimeValidationProofRequirementBinding, error) {
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

func sameRuntimeValidationExecutionProofBinding(left, right runtimecontract.RuntimeValidationProofRequirementBinding) bool {
	left = left.Clone()
	right = right.Clone()
	return left.RequirementHash == right.RequirementHash &&
		left.DefinitionRevision == right.DefinitionRevision &&
		left.CoverageWorkItemID == right.CoverageWorkItemID &&
		left.DeclarationRef == right.DeclarationRef &&
		left.ValidationRecipeID == right.ValidationRecipeID &&
		sameRuntimeStrings(left.RequiredCheckRefs, right.RequiredCheckRefs) &&
		runtimecontract.SameRuntimeValidationCheckAttestationBindings(left.CheckAttestations, right.CheckAttestations)
}

func runtimeOwnerCatalogObservationForOperation(values []runtimecontract.RuntimePrerequisiteObservation, catalog *runtimecontract.RuntimeOperationAdmissibility) *runtimecontract.RuntimePrerequisiteObservation {
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

func runtimeOwnerCatalogForOperation(values []runtimecontract.RuntimeOperationAdmissibility, observations []runtimecontract.RuntimePrerequisiteObservation, operation runtimecontract.RuntimeOperation, requiredSnapshotID, requiredAcquisitionBinding string) *runtimecontract.RuntimeOperationAdmissibility {
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
		if runtimeOwnerCatalogObservationForOperation(observations, &catalog) == nil {
			continue
		}
		value := catalog
		return &value
	}
	return nil
}

func runtimeCatalogContainsOperationKind(catalog runtimecontract.RuntimeOperationAdmissibility, kind string) bool {
	for _, available := range catalog.OperationKinds {
		if runtimecontract.CanonicalRuntimeBlockKind(available) == kind {
			return true
		}
	}
	return false
}

func runtimeWorkspaceManifestObservation(aggregate RuntimeAggregate) *runtimecontract.RuntimePrerequisiteObservation {
	if aggregate.Facts.Resources.Manifest == nil {
		return nil
	}
	revision := strings.TrimSpace(aggregate.Facts.Resources.Manifest.Revision)
	var matched *runtimecontract.RuntimePrerequisiteObservation
	for _, prerequisite := range aggregate.Facts.Prerequisites.Satisfied {
		if prerequisite.Kind != runtimecontract.PrerequisiteKindManifest || strings.TrimSpace(prerequisite.OwnerModuleID) == "" {
			continue
		}
		observation, found := runtimecontract.RuntimePrerequisiteObservationForID(aggregate.Facts.Resources.PrerequisiteObservations, prerequisite.ID)
		observation = observation.Normalized()
		if !found || observation.Validate() != nil || observation.Kind != runtimecontract.PrerequisiteKindManifest ||
			observation.OwnerModuleID != strings.TrimSpace(prerequisite.OwnerModuleID) || observation.ContentHash != revision {
			continue
		}
		if matched != nil && matched.OwnerModuleID != observation.OwnerModuleID {
			return nil
		}
		value := observation
		value.Dependencies = append([]runtimecontract.RuntimeFreshnessDependency(nil), observation.Dependencies...)
		matched = &value
	}
	return matched
}

func validateOwnerReceiptIdentity(operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord, result *RuntimeOperationExecutionResult) error {
	result.Receipt = result.Receipt.Normalized()
	if err := result.Receipt.Validate(); err != nil {
		return fmt.Errorf("owner receipt: %w", err)
	}
	if result.Receipt.ExecutionKey != execution.ExecutionKey || result.Receipt.OperationID != operation.ID {
		return fmt.Errorf("owner receipt does not match claimed execution")
	}
	return nil
}
