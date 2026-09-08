package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

func ownerFactExhaustedDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeDecision {
	decision := terminalDecision(base, runtimecontract.ConvergenceStateFailed, "owner_fact_convergence_exhausted", "required capability-owner fact is unavailable under the declared bounded acquisition contract")
	decision.Terminal.OriginalDiagnostic = missingOwnerFactDiagnostic(facts)
	return decision
}

// Only SM-2 seals this audit cause in its existing internal reduction event.
// Diagnostic text and codes do not drive policy; typed owner facts and the
// existing bounded acquisition contract determine the disposition.
func missingOwnerFactDiagnostic(facts runtimecontract.RuntimeFacts) *runtimecontract.RuntimeOriginalDiagnostic {
	unavailable := missingOwnerFactIsUnavailable(facts)
	for _, requirement := range runtimecontract.RuntimeMissingOwnerFactRequirements(currentFocusOwnerFactRequirements(facts), facts.Resources.OperationAdmissibilities) {
		if requirement.ValidationProofRequirementHash == "" {
			continue
		}
		if unavailable && !runtimecontract.RuntimeOwnerFactHasUnavailableValidationProof(requirement, facts.Resources.OperationAdmissibilities) {
			continue
		}
		var matched []*runtimecontract.RuntimeOriginalDiagnostic
		for _, catalog := range facts.Resources.OperationAdmissibilities {
			if catalog.OwnerModuleID != requirement.OwnerModuleID {
				continue
			}
			for _, surface := range catalog.ValidationSurfaces {
				if surface.OwnerModuleID != requirement.OwnerModuleID || surface.OperationKind != requirement.OperationKind {
					continue
				}
				for _, preparation := range surface.ProofRequirementPreparations {
					if preparation.RequirementHash != requirement.ValidationProofRequirementHash || preparation.DefinitionRevision != facts.DefinitionRevision ||
						preparation.OriginalDiagnostic == nil || preparation.Validate() != nil ||
						preparation.OriginalDiagnostic.ObservedRevision != catalog.ObservedRevision {
						continue
					}
					matched = append(matched, preparation.OriginalDiagnostic)
				}
				for _, unavailable := range surface.ProofRequirementUnavailabilities {
					if unavailable.Requirement.Hash != requirement.ValidationProofRequirementHash || unavailable.Requirement.DefinitionRevision != facts.DefinitionRevision ||
						unavailable.Validate() != nil || unavailable.ValidationRecipeID != surface.ID || unavailable.OriginalDiagnostic.OwnerModuleID != catalog.OwnerModuleID ||
						unavailable.OriginalDiagnostic.ObservedRevision != catalog.ObservedRevision {
						continue
					}
					matched = append(matched, unavailable.OriginalDiagnostic)
				}
			}
		}
		if len(matched) == 1 {
			return matched[0].CloneOriginal()
		}
	}
	return nil
}

func missingOwnerFactIsUnavailable(facts runtimecontract.RuntimeFacts) bool {
	for _, requirement := range runtimecontract.RuntimeMissingOwnerFactRequirements(currentFocusOwnerFactRequirements(facts), facts.Resources.OperationAdmissibilities) {
		if runtimecontract.RuntimeOwnerFactHasUnavailableValidationProof(requirement, facts.Resources.OperationAdmissibilities) {
			return true
		}
	}
	return false
}

// A binding-free queue cursor deliberately carries no owner policy. Resolve
// its immutable declaration by exact CWI identity, without copying it into the
// cursor or rebuilding a binding. Bound acquisition/repair focuses keep their
// generation-specific obligation until their own receipt settles it.
func currentFocusOwnerFactRequirements(facts runtimecontract.RuntimeFacts) []runtimecontract.RuntimeOwnerFactRequirement {
	if len(facts.Focus.CoverageEvidenceBindings) != 0 || len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return facts.Focus.RequiredOwnerFacts
	}
	declaration, found, err := runtimeDefinitionFocusForCoverage(facts.DefinitionFocuses, facts.Focus.CoverageWorkItemIDs[0])
	if err != nil || !found {
		return nil
	}
	return declaration.RequiredOwnerFacts
}
