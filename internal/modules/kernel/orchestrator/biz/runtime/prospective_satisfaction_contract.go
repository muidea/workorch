package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ValidateRuntimeSemanticProposalBeforeOwnerProbe replays the canonical
// manifest/document admission fence at the final Orchestrator boundary before
// any capability owner is called. Normal provider mismatches are handled by
// the existing document/schema correction transaction; this guard prevents a
// corrupted or stale persisted proposal from escaping that SM-3 authority.
func ValidateRuntimeSemanticProposalBeforeOwnerProbe(
	ledger runtimecontract.RuntimeSemanticPlanningLedger,
	proposal runtimecontract.RuntimeSemanticProofProposal,
) error {
	ledger = ledger.Clone()
	proposal = proposal.Clone()
	if ledger.Manifest == nil || ledger.ActiveBatchID == "" || ledger.FeasibilityBatchID == "" {
		return fmt.Errorf("%w: semantic owner probe has no active manifest and feasibility batch", common.ErrConflict)
	}
	document, found := runtimeSemanticBatchDocument(ledger.BatchDocuments, ledger.ActiveBatchID)
	if !found || document.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch {
		return fmt.Errorf("%w: semantic owner probe has no active proof document", common.ErrConflict)
	}
	if err := document.ValidateAgainstManifest(*ledger.Manifest); err != nil {
		return fmt.Errorf("%w: semantic owner probe document is outside its manifest: %v", common.ErrConflict, err)
	}
	want := runtimecontract.StableRuntimeSemanticProofProposalHash(proposal)
	matches := 0
	for _, persisted := range document.Proposals {
		if runtimecontract.StableRuntimeSemanticProofProposalHash(persisted) == want {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("%w: semantic owner probe proposal is not the unique persisted proposal", common.ErrConflict)
	}
	return nil
}

// RuntimeProspectiveSatisfactionContractViolationDiagnostic maps one already
// persisted capability-owner would_not_satisfy fact to the Orchestrator's terminal
// semantic-contract diagnostic. A proposal reaches this point only after the
// same owner-authored all-of target constraint was sealed in the manifest and
// validated before any owner probe, so another target retry cannot resolve the
// contradiction without changing owner authority.
func RuntimeProspectiveSatisfactionContractViolationDiagnostic(
	fact runtimecontract.RuntimeSemanticProofFeasibility,
) (runtimecontract.RuntimeOriginalDiagnostic, error) {
	fact = fact.Clone()
	prospective := fact.OwnerFactProspectiveSatisfactionObservation
	if prospective == nil || prospective.Satisfaction == nil {
		return runtimecontract.RuntimeOriginalDiagnostic{}, fmt.Errorf("%w: prospective satisfaction contract violation requires a persisted satisfaction fact", common.ErrConflict)
	}
	satisfaction := prospective.Satisfaction.Clone()
	if satisfaction.Outcome != runtimecontract.RuntimeOwnerFactProspectiveWouldNotSatisfy || strings.TrimSpace(satisfaction.Reason) == "" {
		return runtimecontract.RuntimeOriginalDiagnostic{}, fmt.Errorf("%w: prospective satisfaction contract violation requires would_not_satisfy", common.ErrConflict)
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion:   runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID:   runtimecontract.RuntimeModuleOrchestrator,
		OriginalClass:   "semantic_contract",
		OriginalCode:    "prospective_satisfaction_contract_violated",
		OriginalMessage: satisfaction.Reason,
		Signature: "semantic-feasibility/prospective-satisfaction-contract/" +
			strings.TrimPrefix(
				runtimecontract.StableRuntimeOwnerFactProspectiveSatisfactionHash(satisfaction),
				runtimecontract.RuntimeOwnerFactProspectiveSatisfactionHashPrefix,
			),
		TargetRefs: append([]string(nil), fact.TargetRefs...),
	}
	return diagnostic, diagnostic.Validate()
}
