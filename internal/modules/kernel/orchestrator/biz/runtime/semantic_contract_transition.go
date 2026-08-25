package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *planningTransactionComponent) requireSemanticContract(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, feasibilityBatchID string, refreshGeneration int) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || refreshGeneration < 0 {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	feasibilityBatchID = strings.TrimSpace(feasibilityBatchID)
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Facts.WorkspaceID != workspaceID || aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime semantic planning scope mismatch", common.ErrConflict)
	}
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningRequired {
		return replayRequiredSemanticContract(aggregate, ledger, feasibilityBatchID, refreshGeneration)
	}
	if !semanticCatalogRefreshMatches(ledger, feasibilityBatchID, refreshGeneration) {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog refresh has no matching feasibility batch", common.ErrConflict)
	}
	facts, invalidation, err := prepareSemanticCatalogRefresh(aggregate.Facts, ledger, feasibilityBatchID, refreshGeneration)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	ledger.FeasibilityGaps = append(ledger.FeasibilityGaps, semanticFeasibilityGap(ledger, refreshGeneration))
	ledger.CatalogRefreshAttempts++
	// Keep the stale generation as an immutable history snapshot, then clear
	// every active manifest/order/batch/fact field in one owner-local reset.
	// The invalidation record is installed before comparison so the history can
	// point to the exact durable stale event.
	aggregate.SemanticPlanning = ledger
	aggregate.Facts = facts
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, invalidation)
	if err := applySemanticCatalogRefresh(&aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func replayRequiredSemanticContract(aggregate RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger, feasibilityBatchID string, refreshGeneration int) (RuntimeAggregate, error) {
	if feasibilityBatchID == "" {
		return aggregate, nil
	}
	if refreshGeneration < len(ledger.FeasibilityGaps) {
		gap := ledger.FeasibilityGaps[refreshGeneration]
		if gap.RefreshGeneration == refreshGeneration && gap.FeasibilityBatchID == feasibilityBatchID {
			return aggregate, nil
		}
	}
	return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog refresh event is stale", common.ErrConflict)
}

func semanticCatalogRefreshMatches(ledger runtimecontract.RuntimeSemanticPlanningLedger, feasibilityBatchID string, refreshGeneration int) bool {
	return feasibilityBatchID != "" && ledger.Phase == runtimecontract.RuntimeSemanticPlanningFeasibilityRequired &&
		ledger.Manifest != nil && ledger.Document != nil && ledger.FeasibilityBatchID == feasibilityBatchID &&
		ledger.CatalogRefreshAttempts == refreshGeneration
}

func prepareSemanticCatalogRefresh(facts runtimecontract.RuntimeFacts, ledger runtimecontract.RuntimeSemanticPlanningLedger, feasibilityBatchID string, refreshGeneration int) (runtimecontract.RuntimeFacts, runtimecontract.RuntimePrerequisiteInvalidationRecord, error) {
	gapFact, found := refreshableRuntimeSemanticFeasibility(ledger.Feasibilities)
	if !found {
		return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, fmt.Errorf("%w: semantic feasibility batch has no refreshable owner fact", common.ErrConflict)
	}
	var stale runtimecontract.RuntimePrerequisiteStale
	if prospective := gapFact.OwnerFactProspectiveSatisfactionObservation; prospective != nil && prospective.Stale != nil {
		catalog, catalogFound := runtimeSemanticDependencyCatalogForOwner(*ledger.Manifest, prospective.Stale.OwnerModuleID)
		if !catalogFound || catalog.SnapshotID == "" || catalog.ObservedRevision != prospective.Stale.ExpectedRevision {
			return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, fmt.Errorf("%w: prospective satisfaction dependency catalog identity is stale", common.ErrConflict)
		}
		observation, observationFound := semanticCatalogPrerequisiteObservation(facts, catalog, prospective.Stale.ObservationID)
		if !observationFound || observation.PrerequisiteID != prospective.Stale.PrerequisiteID {
			return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, fmt.Errorf("%w: prospective satisfaction dependency observation is not current", common.ErrConflict)
		}
		stale = prospective.Stale.Normalized()
		if err := stale.Validate(); err != nil {
			return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, err
		}
	} else {
		if gapFact.OriginalDiagnostic == nil || gapFact.Status != runtimecontract.RuntimeSemanticFeasibilityStale {
			return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, fmt.Errorf("%w: semantic catalog refresh requires a stale owner revision", common.ErrConflict)
		}
		catalog, catalogFound := runtimeSemanticCatalogForOwner(*ledger.Manifest, gapFact.OwnerModuleID)
		if !catalogFound || catalog.SnapshotID != gapFact.CatalogSnapshotID {
			return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, fmt.Errorf("%w: semantic feasibility catalog identity is stale", common.ErrConflict)
		}
		observation, observationFound := semanticCatalogPrerequisiteObservation(facts, catalog, gapFact.CatalogObservationID)
		if !observationFound {
			return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, fmt.Errorf("%w: semantic catalog prerequisite observation is not current", common.ErrConflict)
		}
		var staleErr error
		stale, staleErr = runtimeSemanticCatalogStale(observation, *gapFact.OriginalDiagnostic)
		if staleErr != nil {
			return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, staleErr
		}
	}
	var err error
	facts, prerequisiteIDs, observationIDs, dependencies, err := invalidatePrerequisiteObservationsForStale(facts, stale)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, err
	}
	recovery, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, err
	}
	if !consumed {
		return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, fmt.Errorf("%w: semantic catalog refresh budget exhausted", common.ErrConflict)
	}
	facts.Recovery = recovery
	invalidation := runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature:       fmt.Sprintf("semantic-catalog-refresh/%d/%s/%s", refreshGeneration, feasibilityBatchID, ledger.FeasibilitySetHash),
		PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs, InvalidatedDependencies: dependencies, Stale: &stale,
	}
	if err := invalidation.Validate(); err != nil {
		return runtimecontract.RuntimeFacts{}, runtimecontract.RuntimePrerequisiteInvalidationRecord{}, err
	}
	return facts, invalidation, nil
}

func semanticFeasibilityGap(ledger runtimecontract.RuntimeSemanticPlanningLedger, refreshGeneration int) runtimecontract.RuntimeSemanticFeasibilityGap {
	return runtimecontract.RuntimeSemanticFeasibilityGap{
		RefreshGeneration: refreshGeneration, ManifestHash: ledger.Manifest.Hash, DocumentHash: ledger.DocumentHash,
		FeasibilityBatchID: ledger.FeasibilityBatchID, FeasibilitySetHash: ledger.FeasibilitySetHash,
		Feasibilities: append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), ledger.Feasibilities...),
		Rejections:    append([]runtimecontract.RuntimeOriginalDiagnostic(nil), ledger.Rejections...),
	}
}

func applySemanticCatalogRefresh(aggregate *RuntimeAggregate) error {
	retired, err := resetSemanticPlanningForInvalidatedCatalog(aggregate, runtimecontract.RuntimeSemanticPlanningRetirementCatalogRefresh)
	if err != nil {
		return err
	}
	if !retired {
		return fmt.Errorf("%w: semantic catalog refresh cannot retire an active dispatch", common.ErrConflict)
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil, aggregate.CoverageProofLineage)
	if err != nil {
		return err
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	return nil
}

// invalidateSemanticCatalogGeneration is the private SM-2 handoff used by
// semantic feasibility probing when the active manifest no longer has a
// current owner observation. It is intentionally separate from the generic
// stale-refresh event: missing/mismatched current facts must reopen a fresh
// semantic generation, but must not invent a capability-owner prerequisite
// command or consume the bounded stale-refresh budget.

func (c *planningTransactionComponent) invalidateSemanticCatalogGeneration(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifestHash, logicalRequestID string) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(manifestHash) == "" || strings.TrimSpace(logicalRequestID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Facts.WorkspaceID != workspaceID || aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog invalidation scope mismatch", common.ErrConflict)
	}
	manifestHash = strings.TrimSpace(manifestHash)
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	if aggregate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningRequired {
		// The reset is the idempotency fence. A duplicate probe after the CAS
		// already opened the fresh generation must match one exact retired
		// generation identity. A resource rollback can legally recreate the
		// same manifest hash under a new logical request, so hash-only matching
		// would let a delayed old event acknowledge or retire the wrong
		// generation.
		for _, history := range aggregate.SemanticPlanning.History {
			if history.Snapshot.Manifest != nil && history.Snapshot.Manifest.Hash == manifestHash && history.Snapshot.LogicalRequestID == logicalRequestID {
				return aggregate, nil
			}
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog invalidation does not match a retired generation", common.ErrConflict)
	}
	if aggregate.SemanticPlanning.Manifest == nil || aggregate.SemanticPlanning.Manifest.Hash != manifestHash || aggregate.SemanticPlanning.LogicalRequestID != logicalRequestID {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog invalidation manifest is not current", common.ErrConflict)
	}
	if aggregate.ActiveDispatch != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog invalidation cannot retire an active dispatch", common.ErrConflict)
	}
	gaps, current := semanticPlanningCatalogsCurrent(aggregate, *aggregate.SemanticPlanning.Manifest)
	if current {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog invalidation has no stale owner observation", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	if err := retireSemanticPlanningGeneration(&aggregate, runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated, gaps); err != nil {
		return RuntimeAggregate{}, err
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// compileSemanticPlanBatch persists a batch compiled from an already accepted
// provider-authored order. It is intentionally an Orchestrator-private CAS
// transition; callers cannot mutate the semantic ledger directly.

func (c *planningTransactionComponent) sealSemanticContract(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Facts.WorkspaceID != workspaceID || aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime semantic seal scope mismatch", common.ErrConflict)
	}
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Manifest != nil {
		gaps, current := semanticPlanningCatalogsCurrent(aggregate, *ledger.Manifest)
		if current {
			for _, feasibility := range ledger.Feasibilities {
				gaps = append(gaps, SemanticProspectiveFeasibilityFreshnessGaps(aggregate, *ledger.Manifest, feasibility)...)
			}
			current = len(gaps) == 0
		}
		if current && ledger.Document != nil {
			for _, feasibility := range ledger.Feasibilities {
				if feasibility.OwnerFactProspectiveSatisfactionObservation == nil {
					continue
				}
				proposal, found := runtimecontract.RuntimeSemanticProofProposalForHash(*ledger.Document, feasibility.ProposalHash)
				if !found {
					return RuntimeAggregate{}, fmt.Errorf("%w: prospective semantic feasibility proposal is absent at seal", common.ErrConflict)
				}
				batchID := feasibility.OwnerFactProspectiveSatisfactionObservation.FeasibilityBatchID
				if err := validateRuntimeSemanticProspectiveObservationRequest(aggregate, *ledger.Manifest, proposal, batchID, feasibility); err != nil {
					return RuntimeAggregate{}, err
				}
			}
		}
		if !current {
			if aggregate.ActiveDispatch != nil {
				return RuntimeAggregate{}, fmt.Errorf("%w: stale semantic contract cannot retire an active dispatch", common.ErrConflict)
			}
			expectedRevision := aggregate.StateRevision
			if err := retireSemanticPlanningGeneration(&aggregate, runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated, gaps); err != nil {
				return RuntimeAggregate{}, err
			}
			facts, decision, reduceErr := ReduceRuntimeStateWithDirective(aggregate.Facts, nil, aggregate.CoverageProofLineage)
			if reduceErr != nil {
				return RuntimeAggregate{}, reduceErr
			}
			aggregate.Facts, aggregate.Decision = facts, decision
			aggregate.StateRevision++
			if err := aggregate.Validate(); err != nil {
				return RuntimeAggregate{}, err
			}
			if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
				return RuntimeAggregate{}, err
			}
			return aggregate, nil
		}
	}
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningSealed {
		if aggregate.Facts.ReconciliationContract.BindingSetHash != ledger.SealedBindingSetHash {
			return RuntimeAggregate{}, fmt.Errorf("%w: sealed semantic ledger and reconciliation contract differ", common.ErrConflict)
		}
		return aggregate, nil
	}
	if ledger.Phase != runtimecontract.RuntimeSemanticPlanningFeasibilityRequired || ledger.Manifest == nil || ledger.Document == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic contract is not seal-ready", common.ErrConflict)
	}
	if aggregate.ActiveDispatch != nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic contract cannot seal while a dispatch is active", common.ErrConflict)
	}
	focuses, _, err := runtimecontract.CompileRuntimeSemanticBindings(*ledger.Manifest, ledger.LogicalRequestID, *ledger.Document, ledger.Feasibilities)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	pendingFocuses, err := semanticPendingFocuses(aggregate.Facts, ledger.PendingCoverageIDs, focuses)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	sealedFacts, err := sealRuntimeCWIBindings(aggregate.Facts, ledger.Manifest.Hash, focuses, pendingFocuses)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	reconciliation, err := canonicalRuntimeCWIReconciliationContract(sealedFacts)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	coverageOrderHash := runtimecontract.StableRuntimeSemanticPlanCoverageOrderHash(*ledger.CoverageOrderDocument)
	lineage := []string{ledger.Manifest.Hash, coverageOrderHash, ledger.DocumentHash, ledger.FeasibilityBatchID, ledger.FeasibilitySetHash, reconciliation.BindingSetHash}
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningSealed
	ledger.SealedBindingSetHash = reconciliation.BindingSetHash
	ledger.SealedLineageRefs = lineage
	if err := ledger.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.SemanticPlanning = ledger
	aggregate.Facts = sealedFacts
	aggregate.Facts.Capabilities = runtimeCapabilitiesWithSemanticProofs(aggregate.Facts.Capabilities, focuses)
	aggregate.Facts.ReconciliationContract = reconciliation
	if _, err := supersedeActiveReconciliationForCurrentContract(&aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts.RepairDirective = nil
	aggregate.Facts.Feasibility = nil
	aggregate.Facts.Failure = nil
	aggregate.Facts = aggregate.Facts.Normalized()
	if err := resolveCoverageProofLineageForFocus(&aggregate, aggregate.Facts.Focus); err != nil {
		return RuntimeAggregate{}, err
	}
	previousFocusID := aggregate.Facts.Focus.ID
	advanced, advanceErr := advanceRuntimeCWIFocus(aggregate.Facts, aggregate.CoverageProofLineage)
	if advanceErr != nil {
		return RuntimeAggregate{}, advanceErr
	}
	aggregate.Facts = advanced
	if aggregate.Facts.Focus.ID != previousFocusID {
		if err := prepareSemanticPlanningForAdvancedFocus(&aggregate); err != nil {
			return RuntimeAggregate{}, err
		}
	}
	aggregate.Facts, aggregate.Decision, err = ReduceRuntimeStateWithDirective(aggregate.Facts, nil, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// semanticPendingFocuses keeps unsealed CWI as inert, deterministic focus
// placeholders.  They remain visible in the Run aggregate but have no proof
// binding and therefore cannot pass SM-3.  A later Orchestrator-only semantic
// transaction replaces each placeholder with its matching sealed focus.

func (c *planningTransactionComponent) failSemanticContract(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, terminalCode string, failure runtimecontract.RuntimeOriginalDiagnostic) (RuntimeAggregate, error) {
	if err := failure.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Facts.WorkspaceID != workspaceID || aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime semantic failure scope mismatch", common.ErrConflict)
	}
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningFailed && ledger.Failure != nil && ledger.Failure.Signature == failure.Signature && ledger.Failure.OriginalCode == failure.OriginalCode && ledger.Failure.OriginalMessage == failure.OriginalMessage {
		return aggregate, nil
	}
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningSealed || ledger.Phase == runtimecontract.RuntimeSemanticPlanningFailed {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic contract cannot fail from phase %q", common.ErrConflict, ledger.Phase)
	}
	expectedRevision := aggregate.StateRevision
	aggregate, err = applySemanticContractFailureToAggregate(aggregate, terminalCode, failure)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// applySemanticContractFailureToAggregate is the pure SM-2 semantic-terminal
// transform. The provider-terminal coordinator composes it with the SM-3
// request close before one owner-local CAS; ordinary semantic failures reuse
// the same transform through failSemanticContract.
func applySemanticContractFailureToAggregate(aggregate RuntimeAggregate, terminalCode string, failure runtimecontract.RuntimeOriginalDiagnostic) (RuntimeAggregate, error) {
	if err := failure.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningSealed || ledger.Phase == runtimecontract.RuntimeSemanticPlanningFailed {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic contract cannot fail from phase %q", common.ErrConflict, ledger.Phase)
	}
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningFailed
	ledger.Failure = failure.CloneOriginal()
	if err := ledger.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.SemanticPlanning = ledger
	semanticFailure := semanticContractFailureFact(terminalCode, failure)
	aggregate.Facts.Failure = &semanticFailure
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	if aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || aggregate.Decision.Terminal == nil || aggregate.Decision.Terminal.Code != "semantic_contract_failed" {
		return RuntimeAggregate{}, fmt.Errorf("runtime semantic failure did not seal a terminal decision")
	}
	return aggregate, nil
}

func semanticContractFailureFact(terminalCode string, failure runtimecontract.RuntimeOriginalDiagnostic) runtimecontract.FailureFact {
	return runtimecontract.FailureFact{
		Class:              semanticFailureClassForFailureCode(terminalCode, failure),
		Code:               "semantic_contract_failed",
		Signature:          failure.Signature,
		Message:            failure.OriginalMessage,
		OriginalDiagnostic: failure.CloneOriginal(),
	}
}

// semanticFailureClass preserves the historical code-only helper for tests and
// owner-local callers.  Production terminalization uses the full diagnostic so
// provider/owner classes are not erased when a semantic contract is sealed.
