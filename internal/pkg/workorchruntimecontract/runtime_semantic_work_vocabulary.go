package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	"slices"
)

// RuntimeSemanticRequestContractError denotes a deterministic compiler defect or
// an empty intersection of sealed request vocabularies. It is never evidence of
// a missing user requirement and must not create a Definition clarification.
type RuntimeSemanticRequestContractError struct {
	CoverageRef string
	Reason      string
}

func (e *RuntimeSemanticRequestContractError) Error() string {
	return fmt.Sprintf("semantic_request_contract_conflict: %s: %s", e.CoverageRef, e.Reason)
}

type runtimeSemanticWorkProposalBranch struct {
	Operation          RuntimeOperationVariantRef
	Policy             RuntimeOperationTargetPolicy
	SelectorRefs       []string
	TargetRefs         []string
	ExcludedTargetRefs []string
}

// Scope selectors belong to the Profile/resource observation. Proposal selectors
// belong to the exact capability operation fact. Only their target sets meet;
// their opaque selector identities must never be compared or substituted.
func compileRuntimeSemanticWorkProposalBranches(m RuntimeSemanticPlanManifest, obligation RuntimeSemanticProofObligation) ([]runtimeSemanticWorkProposalBranch, error) {
	w := obligation.ImplementationWork
	if w == nil {
		return nil, nil
	}
	policy := w.Scope.Request.Policy
	var catalog RuntimeOperationAdmissibility
	for _, c := range m.OwnerCatalogs {
		if c.OwnerModuleID == policy.OwnerModuleID {
			catalog = c
			break
		}
	}
	var branches []runtimeSemanticWorkProposalBranch
	for _, operation := range policy.AllowedOperations {
		if !slices.Contains(catalog.OperationKinds, operation.Kind) {
			continue
		}
		for _, targetPolicy := range policy.AllowedTargetPolicies {
			allowed, selectors, constraints, _, err := RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, operation.Kind, operation.VariantID, targetPolicy)
			if err != nil || !allowed || !slices.Contains(m.AllowedTargetPolicies, targetPolicy) {
				continue
			}
			b := runtimeSemanticWorkProposalBranch{Operation: operation, Policy: targetPolicy, SelectorRefs: selectors}
			if targetPolicy == RuntimeOperationTargetPolicyProspectiveOwnerTarget {
				b.ExcludedTargetRefs = normalizeStrings(append(append([]string(nil), w.ExcludedTargetRefs...), catalog.TargetRefs...))
				if len(selectors) == 0 || len(policy.ProspectiveTargetAllOfConstraintRefs) == 0 || !allStringsContained(policy.ProspectiveTargetAllOfConstraintRefs, constraints) {
					continue
				}
				for _, target := range policy.TargetSelectorCandidateRefs {
					if !slices.Contains(b.ExcludedTargetRefs, target) {
						b.TargetRefs = append(b.TargetRefs, target)
					}
				}
				if len(policy.TargetSelectorCandidateRefs) != 0 && len(b.TargetRefs) == 0 {
					continue
				}
			} else {
				for _, target := range w.Scope.Selection.TargetRefs {
					if slices.Contains(catalog.TargetRefs, target) && !slices.Contains(w.ExcludedTargetRefs, target) && (len(policy.TargetSelectorCandidateRefs) == 0 || slices.Contains(policy.TargetSelectorCandidateRefs, target)) {
						b.TargetRefs = append(b.TargetRefs, target)
					}
				}
				if len(b.TargetRefs) == 0 {
					continue
				}
			}
			branches = append(branches, b)
		}
	}
	if len(branches) == 0 {
		return nil, &RuntimeSemanticRequestContractError{CoverageRef: string(obligation.CoverageWorkItemID), Reason: "work policy, observed scope and exact owner operation vocabulary have no representable proposal"}
	}
	return branches, nil
}

// ValidateRuntimeSemanticRequestContract is a pure pre-provider gate. It does
// not choose an operation, assert feasibility or grant evidence. Historical
// manifests remain readable; only a new physical request must pass this gate.
func ValidateRuntimeSemanticRequestContract(m RuntimeSemanticPlanManifest, step RuntimeSemanticPlanStep, coverageIDs []common.CoverageWorkItemID) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if step != RuntimeSemanticPlanStepProofBatch {
		return nil
	}
	for _, obligation := range m.ProofObligations {
		if !slices.Contains(coverageIDs, obligation.CoverageWorkItemID) || obligation.ImplementationWork == nil {
			continue
		}
		if _, err := compileRuntimeSemanticWorkProposalBranches(m, obligation); err != nil {
			return err
		}
	}
	return nil
}
