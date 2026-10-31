package runtime

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func cloneImplementationWorkCommitment(round *runtimecontract.RuntimeImplementationWorkRound) *runtimecontract.RuntimeImplementationWorkRound {
	if round == nil {
		return nil
	}
	copy := round.Clone()
	return &copy
}

// projectImplementationPurpose 消费原 accepted semantic order，原子退休该请求
// generation。effect 分支才 reserve round；terminal 分支只恢复原完整终验收敛。
func projectImplementationPurpose(a *RuntimeAggregate) (bool, error) {
	ledger := a.SemanticPlanning
	if ledger.Manifest == nil || ledger.CoverageOrderDocument == nil || ledger.CoverageOrderDocument.ImplementationPurpose == "" {
		return false, nil
	}
	if ledger.Phase != runtimecontract.RuntimeSemanticPlanningOrderAccepted || a.ActiveDispatch != nil || a.ImplementationWork.ScopePreview == nil || a.ImplementationWork.ScopePreview.Observation == nil {
		return false, fmt.Errorf("%w: implementation purpose has no exact accepted preview transaction", common.ErrConflict)
	}
	if err := validateSemanticManifestIdentity(*a, *ledger.Manifest); err != nil {
		return false, err
	}
	if err := ledger.CoverageOrderDocument.ValidateAgainstManifest(*ledger.Manifest); err != nil {
		return false, err
	}
	if err := ledger.ValidateHistoryStorage(); err != nil {
		return false, err
	}
	preview := a.ImplementationWork.ScopePreview.Clone()
	choice := runtimecontract.RuntimeImplementationWorkPurposeChoice{CoverageWorkItemID: preview.Request.CoverageWorkItemID, ScopeRequestID: preview.Request.RequestID, SemanticManifestHash: ledger.Manifest.Hash, LogicalRequestID: ledger.LogicalRequestID, DocumentHash: ledger.DocumentHash, Purpose: ledger.CoverageOrderDocument.ImplementationPurpose}
	a.ImplementationWork = a.ImplementationWork.Clone()
	if choice.Purpose == runtimecontract.RuntimeImplementationWorkPurposeEffect {
		ordinal, preceding := 1, ""
		for _, prior := range a.ImplementationWork.Rounds {
			if prior.CoverageWorkItemID == choice.CoverageWorkItemID {
				if prior.CompletedReceiptSetHash == "" {
					return false, common.ErrConflict
				}
				ordinal, preceding = prior.Ordinal+1, prior.CompletedReceiptSetHash
			}
		}
		if ordinal > preview.Request.Policy.MaxRoundsPerCWI {
			return false, fmt.Errorf("%w: implementation effect round budget exhausted", common.ErrConflict)
		}
		round := runtimecontract.RuntimeImplementationWorkRound{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, StartupInputHash: a.Definition.StartupInputHash, DefinitionSeedHash: a.Facts.DefinitionSeed.Hash, CoverageWorkItemID: choice.CoverageWorkItemID, PolicyHash: runtimecontract.StableRuntimeImplementationWorkPolicyHash(preview.Request.Policy), Ordinal: ordinal, PrecedingReceiptSetHash: preceding, ScopeRequest: preview.Request, ScopeObservation: preview.Observation}
		round.Hash = runtimecontract.StableRuntimeImplementationWorkRoundHash(round)
		a.ImplementationWork.Rounds = append(a.ImplementationWork.Rounds, round)
	} else if choice.Purpose != runtimecontract.RuntimeImplementationWorkPurposeTerminal {
		return false, common.ErrConflict
	}
	a.ImplementationWork.PurposeChoices = append(a.ImplementationWork.PurposeChoices, choice)
	a.ImplementationWork.ScopePreview = nil
	snapshot := ledger.WithoutHistory().Clone()
	history := runtimecontract.RuntimeSemanticPlanningGeneration{RetiredAtStateRevision: a.StateRevision + 1, Reason: runtimecontract.RuntimeSemanticPlanningRetirementImplementationPurpose, Snapshot: snapshot}
	if err := history.Validate(); err != nil {
		return false, err
	}
	if err := retireSemanticPlanningRequestSessions(a, ledger, "implementation_purpose_selected"); err != nil {
		return false, err
	}
	a.SemanticPlanning = runtimecontract.RuntimeSemanticPlanningLedger{SchemaVersion: ledger.SchemaVersion, Phase: runtimecontract.RuntimeSemanticPlanningRequired, DefinitionSeed: ledger.DefinitionSeed.Clone(), CatalogRefreshAttempts: ledger.CatalogRefreshAttempts, FeasibilityGaps: ledger.FeasibilityGaps, History: ledger.History, HistoryArchive: ledger.HistoryArchive}
	if appended, err := a.SemanticPlanning.AppendHistory(history); err != nil {
		return false, err
	} else {
		a.SemanticPlanning = appended
	}
	return true, nil
}

// reduceImplementationWork is called only inside the sole SM-2 reducer. It
// projects a bounded state-change output without rewriting terminal Focus proof.
func reduceImplementationWork(a RuntimeAggregate) (runtimecontract.RuntimeDecision, bool, error) {
	round, err := CurrentRuntimeImplementationWorkReservation(a)
	if err != nil || round == nil || round.WorkingBinding == nil {
		return runtimecontract.RuntimeDecision{}, false, err
	}
	if a.Facts.Failure != nil || a.Facts.RepairDirective != nil || a.Termination != nil {
		return runtimecontract.RuntimeDecision{}, false, nil
	}
	if runtimecontract.RuntimeImplementationWorkPrerequisitesPending(a.Facts, round.WorkingBinding) {
		return runtimecontract.RuntimeDecision{}, false, nil
	}
	binding := round.WorkingBinding
	required := runtimecontract.RequiredOutputContract{ImplementationWorkBindingHash: binding.Hash,
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}}
	for index, proposal := range binding.Proposals {
		required.AcceptedKinds = append(required.AcceptedKinds, proposal.OperationKind)
		required.AcceptedOperationVariants = append(required.AcceptedOperationVariants, runtimecontract.RuntimeOperationVariantRef{Kind: proposal.OperationKind, VariantID: proposal.OperationVariantID})
		for targetIndex, target := range runtimecontract.RuntimeSemanticProposalTargetRefs(proposal) {
			required.TargetRefs = append(required.TargetRefs, target)
			required.RequiredOperationSequence = append(required.RequiredOperationSequence, runtimecontract.RuntimeRequiredOperationVariant{RequirementID: fmt.Sprintf("implementation-work/%s/%d/%d", round.ScopeRequest.RequestID, index, targetIndex), Kind: proposal.OperationKind, VariantID: proposal.OperationVariantID, TargetRef: target})
		}
	}
	return runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange, Focus: a.Facts.Focus, RequiredOutput: required.Normalized()}, true, nil
}

// sealImplementationWorkingBinding is part of SemanticContractSealed's private
// SM-2/SM-3 transaction. It deliberately does not compile CoverageEvidenceBinding,
// mutate the original CWI declaration, or confer any terminal credit.
func (c *planningTransactionComponent) sealImplementationWorkingBinding(ctx context.Context, a RuntimeAggregate, round runtimecontract.RuntimeImplementationWorkRound, ledger runtimecontract.RuntimeSemanticPlanningLedger) (RuntimeAggregate, error) {
	if round.WorkingBinding != nil && ledger.Phase == runtimecontract.RuntimeSemanticPlanningSealed {
		if ledger.SealedBindingSetHash != round.WorkingBinding.Hash {
			return RuntimeAggregate{}, common.ErrConflict
		}
		return a, nil
	}
	if ledger.Phase != runtimecontract.RuntimeSemanticPlanningFeasibilityRequired || ledger.Manifest == nil || ledger.Document == nil || a.ActiveDispatch != nil || round.ScopeObservation == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: implementation working binding is not seal-ready", common.ErrConflict)
	}
	if err := validateSemanticManifestIdentity(a, *ledger.Manifest); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := runtimecontract.ValidateRuntimeSemanticFeasibilitySet(*ledger.Manifest, *ledger.Document, ledger.Feasibilities, true); err != nil {
		return RuntimeAggregate{}, err
	}
	batch, found := runtimeSemanticBatch(ledger.Batches, ledger.Document.BatchID)
	if !found || batch.State != runtimecontract.RuntimeSemanticPlanBatchAccepted || batch.DocumentHash != ledger.DocumentHash {
		return RuntimeAggregate{}, fmt.Errorf("%w: implementation work requires original accepted proof batch", common.ErrConflict)
	}
	if len(ledger.Manifest.ProofObligations) != 1 || ledger.Manifest.ProofObligations[0].ImplementationWork == nil {
		return RuntimeAggregate{}, common.ErrConflict
	}
	work := ledger.Manifest.ProofObligations[0].ImplementationWork
	binding := runtimecontract.RuntimeImplementationWorkingBinding{
		InputRevision:        round.ScopeRequest.InputRevision.Clone(),
		SemanticManifestHash: ledger.Manifest.Hash, LogicalRequestID: batch.RequestLogicalID,
		BindingGeneration: uint64(round.Ordinal), Proposals: ledger.Document.Proposals, Feasibilities: ledger.Feasibilities,
		MutationInputCatalogs: work.MutationInputCatalogs, ExcludedTargetRefs: work.ExcludedTargetRefs,
	}
	binding.SemanticGenerationID = runtimecontract.StableRuntimeSemanticGenerationID(binding.SemanticManifestHash, binding.LogicalRequestID)
	for _, catalog := range ledger.Manifest.OwnerCatalogs {
		if catalog.OwnerModuleID == round.ScopeRequest.Policy.OwnerModuleID {
			binding.OwnerCatalog = catalog.Clone()
		}
	}
	observations := 0
	for _, observation := range a.Facts.Resources.PrerequisiteObservations {
		if observation.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility && observation.OwnerModuleID == binding.OwnerCatalog.OwnerModuleID && observation.PrerequisiteID == binding.OwnerCatalog.PrerequisiteID && observation.ContentHash == binding.OwnerCatalog.SnapshotID {
			binding.CatalogObservation = observation
			observations++
		}
	}
	if observations != 1 {
		return RuntimeAggregate{}, fmt.Errorf("%w: implementation work needs unique original catalog observation", common.ErrConflict)
	}
	binding.Hash = runtimecontract.StableRuntimeImplementationWorkingBindingHash(binding)
	round.WorkingBinding = &binding
	round.Hash = runtimecontract.StableRuntimeImplementationWorkRoundHash(round)
	if err := round.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	expected := a.StateRevision
	a.ImplementationWork = a.ImplementationWork.Clone()
	found = false
	for index := range a.ImplementationWork.Rounds {
		if a.ImplementationWork.Rounds[index].ScopeRequest.RequestID == round.ScopeRequest.RequestID {
			a.ImplementationWork.Rounds[index] = round.Clone()
			found = true
		}
	}
	if !found {
		return RuntimeAggregate{}, common.ErrConflict
	}
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningSealed
	ledger.SealedBindingSetHash = binding.Hash
	ledger.SealedLineageRefs = []string{ledger.Manifest.Hash, runtimecontract.StableRuntimeSemanticPlanCoverageOrderHash(*ledger.CoverageOrderDocument), ledger.DocumentHash, ledger.FeasibilityBatchID, ledger.FeasibilitySetHash, binding.Hash}
	a.SemanticPlanning = ledger
	// The sole reducer will project a state-change output from this private
	// working commitment; terminal DefinitionFocus and reconciliation stay intact.
	var err error
	a.Facts, a.Decision, err = reduceRuntimeAggregateWithDirective(a, a.Facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	a.StateRevision++
	if err := c.transitions.Commit(ctx, a, expected); err != nil {
		return RuntimeAggregate{}, err
	}
	return a, nil
}
