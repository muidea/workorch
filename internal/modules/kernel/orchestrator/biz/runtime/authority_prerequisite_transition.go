package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *authorityComponent) invalidateSemanticCatalogForFocusScope(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, gap RuntimeSemanticCatalogScopeGap) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	signature := runtimeSemanticCatalogScopeGapSignature(gap)
	if prerequisiteInvalidationExists(aggregate, signature) {
		return aggregate, nil
	}
	current, found := NextRuntimeSemanticCatalogScopeGap(aggregate)
	if !found || current.PrerequisiteID != strings.TrimSpace(gap.PrerequisiteID) ||
		current.ObservationID != strings.TrimSpace(gap.ObservationID) || current.OwnerModuleID != strings.TrimSpace(gap.OwnerModuleID) ||
		current.CatalogSnapshotID != strings.TrimSpace(gap.CatalogSnapshotID) || current.FocusID != strings.TrimSpace(gap.FocusID) ||
		!sameRuntimeTargetRefs(current.RequiredTargetRefs, gap.RequiredTargetRefs) {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog focus-scope gap does not match current authority", common.ErrConflict)
	}
	if aggregate.ActiveDispatch != nil || runtimeAggregateHasCurrentCandidateWork(aggregate) {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog focus-scope invalidation requires no current candidate or dispatch", common.ErrConflict)
	}
	var observation runtimecontract.RuntimePrerequisiteObservation
	for _, value := range aggregate.Facts.Resources.PrerequisiteObservations {
		if value.ObservationID == current.ObservationID {
			observation = value
			break
		}
	}
	facts, prerequisiteIDs, observationIDs, _, err := invalidatePrerequisiteObservations(aggregate.Facts, []runtimecontract.RuntimePrerequisiteObservation{observation})
	if err != nil {
		return RuntimeAggregate{}, err
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
		OriginalClass: "semantic_catalog_focus_scope", OriginalCode: "semantic_catalog_focus_scope_mismatch",
		OriginalMessage: "owner catalog target scope does not cover the current semantic focus",
		Signature:       signature, TargetRefs: append([]string(nil), current.RequiredTargetRefs...),
		ExpectedRevision: current.CatalogSnapshotID, ObservedRevision: current.FocusID,
	}
	if err := diagnostic.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs,
		// The owner dependency remains current. Only the bounded catalog request
		// scope changes for the new focus, so recording that dependency as stale
		// would incorrectly reject the owner's fresh same-revision response.
		RefreshDiagnostic: &diagnostic,
	})
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
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

// SemanticManifestCatalogFreshnessGaps compares every owner catalog in the
// active semantic manifest with the current typed admissibility and
// prerequisite-observation facts. A catalog is current only when both facts
// carry the same snapshot identity; a matching prerequisite ID alone is not a
// freshness proof after a durable receipt.

func (c *authorityComponent) invalidatePrerequisitesForReadOnlyStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stale runtimecontract.RuntimePrerequisiteStale) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	stale = stale.Normalized()
	if err := stale.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
	if prerequisiteInvalidationExists(aggregate, signature) {
		return aggregate, nil
	}
	if aggregate.ActiveDispatch != nil || runtimeAggregateHasCurrentCandidateWork(aggregate) {
		return RuntimeAggregate{}, fmt.Errorf("%w: read-only prerequisite stale requires no current candidate or dispatch", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	facts, prerequisiteIDs, observationIDs, invalidatedDependencies, err := invalidatePrerequisiteObservationsForStale(aggregate.Facts, stale)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	ledger, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	facts.Recovery = ledger
	staleCopy := stale
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs,
		InvalidatedDependencies: invalidatedDependencies, Stale: &staleCopy,
	})
	if !consumed {
		facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted",
			Signature: signature, Message: "prerequisite refresh budget exhausted: " + stale.Reason,
		}
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil, aggregate.CoverageProofLineage)
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

// runtimeAggregateHasCurrentCandidateWork distinguishes active SM-3 work from
// immutable lineage that belongs to a prior authority snapshot.  Resource
// receipt invalidation intentionally keeps the accepted candidate/session that
// authorized the durable effect; a later SM-2 stale fact may proceed only when
// it cannot invalidate a current provider request or pending candidate.
