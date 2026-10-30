package workorchruntimecontract

import (
	"fmt"
	"slices"
)

func implementationWorkingOperationManifest(operation RuntimeCandidateOperationManifest, binding RuntimeImplementationWorkingBinding) (RuntimeCandidateOperationManifest, error) {
	var target *RuntimeOperationTargetAuthorization
	for _, proposal := range binding.Proposals {
		if proposal.OperationKind != operation.Kind {
			continue
		}
		if target != nil && target.Policy != proposal.Target.Policy {
			return RuntimeCandidateOperationManifest{}, fmt.Errorf("implementation work cannot mix target policies for one operation kind")
		}
		if target == nil {
			copy := cloneRuntimeOperationTargetAuthorizationRaw(proposal.Target)
			target = &copy
		} else {
			target.ExactTargetRefs = append(target.ExactTargetRefs, proposal.Target.ExactTargetRefs...)
			target.OwnerScopeTargetRefs = append(target.OwnerScopeTargetRefs, proposal.Target.OwnerScopeTargetRefs...)
			target.ProspectiveTargetRefs = append(target.ProspectiveTargetRefs, proposal.Target.ProspectiveTargetRefs...)
		}
	}
	if target == nil || operation.OwnerModuleID != binding.OwnerCatalog.OwnerModuleID {
		return RuntimeCandidateOperationManifest{}, fmt.Errorf("implementation work operation has no original semantic working proposal")
	}
	target.ExcludedTargetRefs = slices.Clone(binding.ExcludedTargetRefs)
	operation.InputRevision = binding.InputRevision.Clone()
	operation.Target = target.Normalized()
	operation.TargetParameterBindings = runtimeCandidateTargetParameterBindings(operation.Kind, operation.Target, []RuntimeOperationAdmissibility{binding.OwnerCatalog}, binding.OwnerCatalog.OwnerModuleID)
	operation.PrerequisiteSnapshotRefs = []string{binding.CatalogObservation.ObservationID}
	operation.AdmissibilitySnapshotRefs = []string{binding.OwnerCatalog.SnapshotID}
	operation.RequireTargetRefs = true
	operation.AcceptedEffectScopes = []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}
	operation.RequiredEffectScopes = []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}
	for index := range operation.BindingAuthorizations {
		operation.BindingAuthorizations[index].AllowedBindingRefs = nil
	}
	operation.ValidationRecipes = nil
	operation.RequiredOperationFields = runtimeCandidateRequiredOperationFields(operation)
	return operation.Normalized(), nil
}
