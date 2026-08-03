package runtime

import (
	"context"
	"fmt"
	"strings"

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
	if aggregate.Reconciliation.Request != nil {
		current := aggregate.Reconciliation.Request.Normalized()
		if current.RequestID == request.RequestID && aggregate.Reconciliation.RequestHash == requestHash {
			return aggregate, false, nil
		}
		for _, history := range aggregate.Reconciliation.History {
			history = history.Normalized()
			if history.Request.RequestID == request.RequestID && history.RequestHash == requestHash {
				// A duplicate resume may arrive after a later generation has
				// already been sealed.  The historical generation is immutable;
				// reusing it must not rewind the current ledger or trigger a second
				// owner query.
				return aggregate, false, nil
			}
		}
		if request.Generation <= current.Generation {
			return RuntimeAggregate{}, false, fmt.Errorf("%w: reconciliation request generation is not newer than current generation", common.ErrConflict)
		}
	}
	expectedRevision := aggregate.StateRevision
	ledger := aggregate.Reconciliation.Clone()
	if ledger.Request != nil {
		previous := runtimecontract.RuntimeCWIReconciliationGeneration{
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
	if aggregate.Reconciliation.Request == nil {
		return RuntimeAggregate{}, false, fmt.Errorf("%w: reconciliation request is not sealed", common.ErrConflict)
	}
	request := aggregate.Reconciliation.Request.Clone()
	batch = batch.Clone()
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
	results, reopened := reconcileCoverage(aggregate.Facts, request, batch)
	aggregate.Facts.Coverage = runtimecontract.NormalizeRuntimeCoverageFacts(aggregate.Facts.Coverage)
	for _, lineage := range reopened {
		for index := range aggregate.Facts.Coverage {
			if aggregate.Facts.Coverage[index].ID != lineage.CoverageWorkItemID {
				continue
			}
			aggregate.Facts.Coverage[index].State = runtimecontract.RuntimeCoverageItemStateOpen
			aggregate.Facts.Coverage[index].ReopenedByEvidenceRefs = appendUniqueRuntimeString(aggregate.Facts.Coverage[index].ReopenedByEvidenceRefs, lineage.ObservationRef)
		}
	}
	aggregate.Facts.Coverage = runtimecontract.NormalizeRuntimeCoverageFacts(aggregate.Facts.Coverage)
	aggregate.Reconciliation.Batch = &batch
	aggregate.Reconciliation.Results = results
	aggregate.Reconciliation.Reopened = append(aggregate.Reconciliation.Reopened, reopened...)
	aggregate.Reconciliation = aggregate.Reconciliation.Normalized()
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, false, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, false, err
	}
	return aggregate, true, nil
}

func reconcileCoverage(
	facts runtimecontract.RuntimeFacts,
	request runtimecontract.RuntimeCWIReconciliationRequest,
	batch runtimecontract.RuntimeCWIReconciliationBatch,
) ([]runtimecontract.RuntimeCWIReconciliationResult, []runtimecontract.RuntimeCWIReconciliationReopenedLineage) {
	facts = facts.Normalized()
	byCoverage := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItemFact, len(facts.Coverage))
	for _, item := range facts.Coverage {
		byCoverage[item.ID] = item
	}
	results := make([]runtimecontract.RuntimeCWIReconciliationResult, 0, len(request.Bindings))
	var reopened []runtimecontract.RuntimeCWIReconciliationReopenedLineage
	for _, binding := range request.Bindings {
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
		item := byCoverage[binding.CoverageWorkItemID]
		disposition := runtimecontract.RuntimeCWIReconciliationDispositionPrerequisite
		if request.Scope == runtimecontract.RuntimeCWIReconciliationScopeCurrentRun && allSatisfied && item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
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
			reopened = append(reopened, runtimecontract.RuntimeCWIReconciliationReopenedLineage{
				RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID,
				Generation: request.Generation, BatchHash: batch.Hash, CoverageWorkItemID: binding.CoverageWorkItemID,
				BindingID: binding.BindingID, ObservationRef: observationRef,
				PreviousEvidenceRefs: append([]string(nil), item.EvidenceRefs...),
				Reason:               reconciliationReopenReason(request.Scope, observations),
			})
		}
	}
	return results, reopened
}

func reconciliationReopenReason(scope runtimecontract.RuntimeCWIReconciliationScope, observations []runtimecontract.RuntimeCWIReconciliationObservation) string {
	if scope == runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull {
		return "workspace_full reconciliation requires a new current-run proof"
	}
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

func appendUniqueRuntimeString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
