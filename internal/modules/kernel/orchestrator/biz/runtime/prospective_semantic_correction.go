package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// DeriveRuntimeProspectiveSatisfactionCorrection projects the exact typed
// proposal constraint implied by one already-persisted File+required-owner
// composite. It is a pure SM-2/SM-3 evaluator: the caller still owns whether
// the observation is terminal or may consume the sole semantic correction.
func DeriveRuntimeProspectiveSatisfactionCorrection(
	ledger runtimecontract.RuntimeSemanticPlanningLedger,
	fact runtimecontract.RuntimeSemanticProofFeasibility,
) (runtimecontract.RuntimeOriginalDiagnostic, bool, error) {
	ledger = ledger.Clone()
	fact = fact.Clone()
	if ledger.Manifest == nil || ledger.ActiveBatchID == "" || ledger.FeasibilityBatchID == "" {
		return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: prospective satisfaction correction has no active feasibility batch", common.ErrConflict)
	}
	document, found := runtimeSemanticBatchDocument(ledger.BatchDocuments, ledger.ActiveBatchID)
	if !found || document.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch {
		return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: prospective satisfaction correction has no active proof document", common.ErrConflict)
	}
	prospective := fact.OwnerFactProspectiveSatisfactionObservation
	if prospective == nil || prospective.Satisfaction == nil {
		return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: prospective satisfaction correction requires a persisted satisfaction fact", common.ErrConflict)
	}
	satisfaction := prospective.Satisfaction.Clone()
	if satisfaction.Outcome != runtimecontract.RuntimeOwnerFactProspectiveWouldNotSatisfy || strings.TrimSpace(satisfaction.Reason) == "" {
		return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: prospective satisfaction correction requires would_not_satisfy", common.ErrConflict)
	}

	proposalIndex := -1
	var proposal runtimecontract.RuntimeSemanticProofProposal
	for idx := range document.Proposals {
		candidate := document.Proposals[idx].Clone()
		if runtimecontract.StableRuntimeSemanticProofProposalHash(candidate) != fact.ProposalHash {
			continue
		}
		if proposalIndex >= 0 {
			return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: prospective satisfaction correction proposal identity is ambiguous", common.ErrConflict)
		}
		proposalIndex = idx
		proposal = candidate
	}
	if proposalIndex < 0 {
		return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: prospective satisfaction correction proposal is outside the active batch", common.ErrConflict)
	}
	if err := fact.ValidateCompositeForProposal(*ledger.Manifest, proposal); err != nil {
		return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: prospective satisfaction composite does not match its proposal: %v", common.ErrConflict, err)
	}
	proposal = proposal.Normalized()
	failedTarget := strings.TrimSpace(proposal.Target.ProspectiveTargetRef)
	if proposal.Target.Policy != runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget || failedTarget == "" {
		return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: prospective satisfaction correction requires an exact prospective proposal target", common.ErrConflict)
	}

	var obligation *runtimecontract.RuntimeSemanticProofObligation
	for idx := range ledger.Manifest.ProofObligations {
		candidate := ledger.Manifest.ProofObligations[idx].Clone()
		if candidate.CoverageWorkItemID == proposal.CoverageWorkItemID {
			obligation = &candidate
			break
		}
	}
	if obligation == nil || obligation.OwnerFactAcquisition == nil ||
		obligation.OwnerFactAcquisition.AcquisitionTargetPolicy != runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget {
		return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: prospective satisfaction correction has no sealed prospective obligation", common.ErrConflict)
	}
	acquisition := obligation.OwnerFactAcquisition.Clone()
	var acquisitionCatalog *runtimecontract.RuntimeOperationAdmissibility
	for idx := range ledger.Manifest.OwnerCatalogs {
		catalog := ledger.Manifest.OwnerCatalogs[idx].Normalized()
		if catalog.OwnerModuleID != acquisition.AcquisitionOwnerModuleID {
			continue
		}
		if catalog.SnapshotID != acquisition.OwnerCatalogSnapshotID || catalog.ObservedRevision != acquisition.OwnerCatalogObservedRevision {
			return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: prospective satisfaction correction acquisition catalog identity is stale", common.ErrConflict)
		}
		acquisitionCatalog = &catalog
		break
	}
	if acquisitionCatalog == nil {
		return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: prospective satisfaction correction has no sealed acquisition owner catalog", common.ErrConflict)
	}

	path := fmt.Sprintf("$.proposals[%d].target.prospectiveTargetRef", proposalIndex)
	fieldViolation := runtimecontract.RuntimeFieldViolation{
		Path: path, Actual: failedTarget, Forbidden: []string{failedTarget},
	}
	semanticViolation := runtimecontract.RuntimeSemanticContractViolation{
		Kind: runtimecontract.RuntimeSemanticContractViolationProposalConstraint,
		Path: path, CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalIndex: proposalIndex,
		Actual: failedTarget,
	}
	correctable := false
	candidates := append([]string(nil), obligation.DeclarationTargetSelectorCandidateRefs...)
	if len(candidates) == 0 {
		// An open declaration selector still provides a bounded field delta: the
		// next document must keep the sealed selector/constraints while replacing
		// this exact failed target. It does not authorize Orchestrator to choose a
		// replacement target.
		semanticViolation.Rule = runtimecontract.RuntimeSemanticConstraintRelational
		semanticViolation.Required = []string{"a different non-empty target satisfying the sealed selector and prospective constraints"}
		fieldViolation.Required = append([]string(nil), semanticViolation.Required...)
		correctable = true
	} else {
		failedTargetSealed := false
		remaining := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate == failedTarget {
				failedTargetSealed = true
				continue
			}
			if runtimeProspectiveCorrectionContains(acquisitionCatalog.TargetRefs, candidate) {
				// A target already present in the manifest-current acquisition-owner
				// catalog cannot be proposed as a prospective creation.
				continue
			}
			candidateDocument := document.Clone()
			candidateDocument.Proposals[proposalIndex].Target.ProspectiveTargetRef = candidate
			// Reuse the canonical manifest proposal fence for the sealed selector,
			// prospective constraints, variant and owner catalog. A correction
			// candidate gets no weaker interpretation than an initial proposal.
			if err := candidateDocument.ValidateAgainstManifest(*ledger.Manifest); err != nil {
				continue
			}
			remaining = append(remaining, candidate)
		}
		if !failedTargetSealed {
			return runtimecontract.RuntimeOriginalDiagnostic{}, false, fmt.Errorf("%w: failed prospective target is outside the sealed selector candidates", common.ErrConflict)
		}
		semanticViolation.Rule = runtimecontract.RuntimeSemanticConstraintAllowedValue
		semanticViolation.Allowed = append([]string(nil), remaining...)
		fieldViolation.Allowed = append([]string(nil), remaining...)
		// A fixed/singleton declaration has no remaining candidate. Persisting
		// the diagnostic is useful for terminal audit, but no provider retry can
		// produce a different admitted target.
		correctable = len(remaining) > 0
	}
	if err := semanticViolation.ValidateAgainstManifest(*ledger.Manifest); err != nil {
		return runtimecontract.RuntimeOriginalDiagnostic{}, false, err
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
		OriginalClass: "semantic_contract", OriginalCode: "prospective_owner_fact_unsatisfied",
		OriginalMessage: satisfaction.Reason,
		Signature:       "semantic-feasibility/prospective-owner-fact/" + strings.TrimPrefix(runtimecontract.StableRuntimeOwnerFactProspectiveSatisfactionHash(satisfaction), runtimecontract.RuntimeOwnerFactProspectiveSatisfactionHashPrefix),
		TargetRefs:      append([]string(nil), fact.TargetRefs...),
		FieldViolations: []runtimecontract.RuntimeFieldViolation{fieldViolation},
		SemanticViolations: []runtimecontract.RuntimeSemanticContractViolation{
			semanticViolation,
		},
	}
	return diagnostic, correctable, diagnostic.Validate()
}

func runtimeProspectiveCorrectionContains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
