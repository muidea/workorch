package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

func ownerFactExhaustedDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeDecision {
	decision := terminalDecision(base, runtimecontract.ConvergenceStateFailed, "owner_fact_convergence_exhausted", "required capability-owner fact remained unavailable after the bounded semantic replan budget")
	decision.Terminal.OriginalDiagnostic = missingOwnerFactDiagnostic(facts)
	return decision
}

// Only SM-2 seals this audit cause in its existing internal reduction event.
// No text/code drives policy; the missing-fact/budget decision is unchanged.
func missingOwnerFactDiagnostic(facts runtimecontract.RuntimeFacts) *runtimecontract.RuntimeOriginalDiagnostic {
	for _, requirement := range runtimecontract.RuntimeMissingOwnerFactRequirements(facts.Focus.RequiredOwnerFacts, facts.Resources.OperationAdmissibilities) {
		if requirement.ValidationProofRequirementHash == "" {
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
			}
		}
		if len(matched) == 1 {
			return matched[0].CloneOriginal()
		}
	}
	return nil
}
