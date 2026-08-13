package runtime

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ensureReconciliationRequest seals the SM-4 input into the Orchestrator
// aggregate. It is intentionally idempotent: a duplicate request returns the
// persisted ledger without another CAS or owner query.
func (c *RuntimeController) ensureReconciliationRequest(ctx context.Context, request runtimecontract.RuntimeCWIReconciliationRequest) (RuntimeAggregate, bool, error) {
	if c == nil || c.repository == nil {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	request = request.Clone()
	if err := request.Validate(); err != nil {
		return RuntimeAggregate{}, false, err
	}
	aggregate, err := c.repository.Load(ctx, request.WorkspaceID, request.RunID)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	if aggregate.Facts.ReconciliationContract.SchemaVersion == "" {
		return RuntimeAggregate{}, false, fmt.Errorf("%w: runtime aggregate has no activation-sealed reconciliation contract", common.ErrConflict)
	}
	if err := request.ValidateAgainstContract(aggregate.Facts.ReconciliationContract); err != nil {
		return RuntimeAggregate{}, false, err
	}
	requestHash := runtimecontract.StableRuntimeCWIReconciliationRequestHash(request)
	expectedRevision := aggregate.StateRevision
	if aggregate.Reconciliation.Request != nil {
		current := aggregate.Reconciliation.Request.Normalized()
		if err := current.ValidateAgainstContract(aggregate.Facts.ReconciliationContract); err != nil {
			// An active row may only be retired by the Runtime owner. Do this
			// before request idempotency checks so an old binding set can never
			// suppress the current generation.
			if _, supersedeErr := supersedeActiveReconciliationForCurrentContract(&aggregate); supersedeErr != nil {
				return RuntimeAggregate{}, false, supersedeErr
			}
		}
	}
	for _, history := range aggregate.Reconciliation.History {
		history = history.Normalized()
		if history.Request.RequestID == request.RequestID && history.RequestHash == requestHash {
			// Historical generations are audit only. They may acknowledge a
			// duplicate while a current exact request is active, but must never
			// be resurrected when the active row was cleared or superseded.
			if aggregate.Reconciliation.Request != nil {
				current := aggregate.Reconciliation.Request.Normalized()
				if current.ValidateAgainstContract(aggregate.Facts.ReconciliationContract) == nil {
					return aggregate, false, nil
				}
			}
			return RuntimeAggregate{}, false, fmt.Errorf("%w: reconciliation request was superseded and cannot be reactivated", common.ErrConflict)
		}
	}
	ledger := aggregate.Reconciliation.Clone()
	if ledger.Request != nil {
		current := ledger.Request.Normalized()
		if current.RequestID == request.RequestID && ledger.RequestHash == requestHash {
			return aggregate, false, nil
		}
	}
	highestGeneration := uint64(0)
	for _, history := range ledger.History {
		if history.Request.Generation > highestGeneration {
			highestGeneration = history.Request.Generation
		}
	}
	if ledger.Request != nil && ledger.Request.Generation > highestGeneration {
		highestGeneration = ledger.Request.Generation
	}
	if request.Generation <= highestGeneration {
		return RuntimeAggregate{}, false, fmt.Errorf("%w: reconciliation request generation is not newer than current generation", common.ErrConflict)
	}
	if ledger.Request != nil {
		if len(ledger.History) >= runtimecontract.RuntimeCWIReconciliationHistoryLimit {
			// Reject before replacing the active generation.  The immutable
			// history is an audit bound, not a rolling cache: discarding an old
			// request would make a later duplicate/restart ambiguous.
			return RuntimeAggregate{}, false, fmt.Errorf("%w: runtime reconciliation history exceeds bounded limit %d", common.ErrConflict, runtimecontract.RuntimeCWIReconciliationHistoryLimit)
		}
		previous := runtimecontract.RuntimeCWIReconciliationGeneration{
			Superseded:  true,
			Request:     *ledger.Request,
			RequestHash: ledger.RequestHash,
			Batch:       ledger.Batch,
			Results:     append([]runtimecontract.RuntimeCWIReconciliationResult(nil), ledger.Results...),
			Reopened:    append([]runtimecontract.RuntimeCWIReconciliationReopenedLineage(nil), ledger.Reopened...),
		}
		ledger.History = append(ledger.History, previous)
	}
	ledger.Request = &request
	ledger.RequestHash = requestHash
	ledger.Batch = nil
	ledger.Results = nil
	ledger.Reopened = nil
	aggregate.Reconciliation = ledger.Normalized()
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, false, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, false, err
	}
	return aggregate, true, nil
}

// commitReconciliationBatch validates a complete owner-observation set and
// applies the only legal SM-2 reconciliation transition in one CAS. An
// observation can preserve existing credit or reopen it; it can never create
// new credit on a run that has not produced a matching sealed receipt.
func (c *RuntimeController) commitReconciliationBatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, batch runtimecontract.RuntimeCWIReconciliationBatch) (RuntimeAggregate, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	batch = batch.Clone()
	if aggregate.Reconciliation.Request == nil {
		if reconciliationHistoryContainsExactBatch(aggregate.Reconciliation.History, batch) {
			// A reopen supersedes and clears the active generation in the same
			// SM-2 CAS. A delayed replay of that exact owner batch is audit-only;
			// it must not recreate a request, reopen again, or fail the drive.
			return aggregate, false, nil
		}
		return RuntimeAggregate{}, false, fmt.Errorf("%w: reconciliation request is not sealed", common.ErrConflict)
	}
	if aggregate.ActiveDispatch != nil {
		return RuntimeAggregate{}, false, fmt.Errorf("%w: reconciliation cannot mutate CWI while a sealed dispatch is active", common.ErrConflict)
	}
	request := aggregate.Reconciliation.Request.Clone()
	if err := request.ValidateAgainstContract(aggregate.Facts.ReconciliationContract); err != nil {
		return RuntimeAggregate{}, false, fmt.Errorf("%w: reconciliation batch targets a superseded request: %v", common.ErrConflict, err)
	}
	if reconciliationHistoryContainsExactBatch(aggregate.Reconciliation.History, batch) {
		return aggregate, false, nil
	}
	if err := batch.ValidateForRequest(request); err != nil {
		return RuntimeAggregate{}, false, err
	}
	if aggregate.Reconciliation.Batch != nil {
		current := aggregate.Reconciliation.Batch.Normalized()
		if current.Hash == batch.Hash {
			return aggregate, false, nil
		}
		if current.Generation >= batch.Generation {
			return RuntimeAggregate{}, false, fmt.Errorf("%w: stale reconciliation batch generation", common.ErrConflict)
		}
	}
	expectedRevision := aggregate.StateRevision
	results, reopened, triggers, reconcileErr := reconcileCoverage(aggregate.Facts, request, batch)
	if reconcileErr != nil {
		return RuntimeAggregate{}, false, reconcileErr
	}
	actualReopened, reopenErr := applyRuntimeCWIReopen(&aggregate, triggers)
	if reopenErr != nil {
		return RuntimeAggregate{}, false, reopenErr
	}
	if len(actualReopened) != len(reopened) {
		return RuntimeAggregate{}, false, fmt.Errorf("%w: reconciliation reopen lineage does not match canonical CWI transition", common.ErrConflict)
	}
	for _, lineage := range reopened {
		found := false
		for _, coverageID := range actualReopened {
			if coverageID == lineage.CoverageWorkItemID {
				found = true
				break
			}
		}
		if !found {
			return RuntimeAggregate{}, false, fmt.Errorf("%w: reconciliation reopen %q was not applied", common.ErrConflict, lineage.CoverageWorkItemID)
		}
	}
	aggregate.Reconciliation.Batch = &batch
	aggregate.Reconciliation.Results = results
	aggregate.Reconciliation.Reopened = append(aggregate.Reconciliation.Reopened, reopened...)
	aggregate.Reconciliation = aggregate.Reconciliation.Normalized()
	if len(reopened) != 0 {
		if _, supersedeErr := supersedeActiveReconciliationForCurrentContract(&aggregate); supersedeErr != nil {
			return RuntimeAggregate{}, false, supersedeErr
		}
		if retireErr := retireSemanticPlanningForCurrentReopenedCWIs(&aggregate); retireErr != nil {
			return RuntimeAggregate{}, false, retireErr
		}
	}
	nextFacts, nextDecision, reduceErr := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if reduceErr != nil {
		return RuntimeAggregate{}, false, reduceErr
	}
	aggregate.Facts = nextFacts
	aggregate.Decision = nextDecision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, false, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, false, err
	}
	return aggregate, true, nil
}

func reconciliationHistoryContainsExactBatch(history []runtimecontract.RuntimeCWIReconciliationGeneration, batch runtimecontract.RuntimeCWIReconciliationBatch) bool {
	batch = batch.Normalized()
	if batch.Hash == "" {
		return false
	}
	for _, generation := range history {
		generation = generation.Normalized()
		if generation.Batch == nil || generation.Batch.Hash != batch.Hash {
			continue
		}
		if batch.ValidateForRequest(generation.Request) == nil {
			return true
		}
	}
	return false
}

func reconcileCoverage(
	facts runtimecontract.RuntimeFacts,
	request runtimecontract.RuntimeCWIReconciliationRequest,
	batch runtimecontract.RuntimeCWIReconciliationBatch,
) ([]runtimecontract.RuntimeCWIReconciliationResult, []runtimecontract.RuntimeCWIReconciliationReopenedLineage, []runtimeCWIReopenTrigger, error) {
	facts = facts.Normalized()
	if err := runtimecontract.ValidateRuntimeCWILedger(facts); err != nil {
		return nil, nil, nil, err
	}
	byCoverage := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItemFact, len(facts.Coverage))
	for _, item := range facts.Coverage {
		byCoverage[item.ID] = item
	}
	results := make([]runtimecontract.RuntimeCWIReconciliationResult, 0, len(request.Bindings))
	var reopened []runtimecontract.RuntimeCWIReconciliationReopenedLineage
	triggers := make([]runtimeCWIReopenTrigger, 0)
	for _, binding := range request.Bindings {
		item, found := byCoverage[binding.CoverageWorkItemID]
		if !found || item.CurrentBindingRef == nil || item.CurrentBindingRef.BindingID != binding.BindingID || item.CurrentBindingRef.BindingHash != binding.BindingHash {
			return nil, nil, nil, fmt.Errorf("%w: reconciliation binding %q is no longer current for CWI %q", common.ErrConflict, binding.BindingID, binding.CoverageWorkItemID)
		}
		observations := make([]runtimecontract.RuntimeCWIReconciliationObservation, 0, len(binding.OwnerModuleIDs))
		for _, observation := range batch.Observations {
			if observation.CoverageWorkItemID == binding.CoverageWorkItemID && observation.BindingID == binding.BindingID {
				observations = append(observations, observation)
			}
		}
		allSatisfied := len(observations) == len(binding.OwnerModuleIDs)
		for _, observation := range observations {
			if observation.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
				allSatisfied = false
			}
		}
		disposition := runtimecontract.RuntimeCWIReconciliationDispositionPrerequisite
		// Scope selects which CWI bindings are observed; it is not a credit
		// invalidation policy. A satisfied row belongs to this Runtime aggregate
		// and remains satisfied when every exact owner observation still holds,
		// including a workspace_full generation. Fresh restart predecessor credit
		// is rejected when the new aggregate is formed, not repaired here.
		if allSatisfied && item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			disposition = runtimecontract.RuntimeCWIReconciliationDispositionRetain
		} else if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			disposition = runtimecontract.RuntimeCWIReconciliationDispositionReopen
		}
		observationRef := runtimecontract.StableRuntimeCWIReconciliationObservationSetRef(observations)
		results = append(results, runtimecontract.RuntimeCWIReconciliationResult{
			RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID,
			Generation: request.Generation, BatchHash: batch.Hash, CoverageItemID: binding.CoverageWorkItemID,
			BindingID: binding.BindingID, Disposition: disposition, ObservationRef: observationRef,
		})
		if disposition == runtimecontract.RuntimeCWIReconciliationDispositionReopen {
			lineage := runtimecontract.RuntimeCWIReconciliationReopenedLineage{
				RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID,
				Generation: request.Generation, BatchHash: batch.Hash, CoverageWorkItemID: binding.CoverageWorkItemID,
				BindingID: binding.BindingID, ObservationRef: observationRef,
				PreviousEvidenceRefs: append([]string(nil), item.EvidenceRefs...),
				Reason:               reconciliationReopenReason(observations),
			}
			reopened = append(reopened, lineage)
			triggers = append(triggers, runtimeCWIReopenTrigger{
				CoverageWorkItemID: binding.CoverageWorkItemID,
				TriggerRef:         lineage.ObservationRef,
				Reason:             lineage.Reason,
			})
		}
	}
	return results, reopened, triggers, nil
}

func reconciliationReopenReason(observations []runtimecontract.RuntimeCWIReconciliationObservation) string {
	for _, observation := range observations {
		if observation.Status == runtimecontract.RuntimeCWIReconciliationObservationStale {
			return "current-run owner observation is stale"
		}
		if observation.Status == runtimecontract.RuntimeCWIReconciliationObservationUnverifiable {
			return "current-run owner observation is unverifiable"
		}
	}
	return "current-run owner observation no longer satisfies the sealed proof"
}
