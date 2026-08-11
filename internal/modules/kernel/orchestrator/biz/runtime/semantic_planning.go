package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// commitSemanticPlanning is the sole CAS boundary for the semantic sub-ledger.
// The callback is owner-local and can only change the ledger; capability facts
// and provider documents reach it through RuntimeInternalEvent after scope and
// lineage checks.
func (c *RuntimeController) commitSemanticPlanning(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, mutate func(RuntimeAggregate, *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error)) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || mutate == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Facts.WorkspaceID != workspaceID || aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, fmt.Errorf("%w: runtime semantic planning scope mismatch", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	ledger := aggregate.SemanticPlanning.Clone()
	changed, err := mutate(aggregate, &ledger)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if !changed {
		return aggregate, nil
	}
	if err := ledger.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.SemanticPlanning = ledger
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// runtimeSemanticBatchDocument is the runtime-owner-local lookup for the
// active proof document. The drive package has a similarly shaped helper for
// its own adapter layer; keeping this lookup here avoids exposing a registry or
// cross-package mutable ledger accessor.
func runtimeSemanticBatchDocument(values []runtimecontract.RuntimeSemanticPlanDocument, batchID string) (runtimecontract.RuntimeSemanticPlanDocument, bool) {
	batchID = strings.TrimSpace(batchID)
	for _, value := range values {
		if value.BatchID == batchID {
			return value.Clone(), true
		}
	}
	return runtimecontract.RuntimeSemanticPlanDocument{}, false
}

func runtimeSemanticBatch(values []runtimecontract.RuntimeSemanticPlanBatch, batchID string) (runtimecontract.RuntimeSemanticPlanBatch, bool) {
	batchID = strings.TrimSpace(batchID)
	for _, value := range values {
		if value.BatchID == batchID {
			return value.Clone(), true
		}
	}
	return runtimecontract.RuntimeSemanticPlanBatch{}, false
}

func validateSemanticManifestIdentity(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeSemanticPlanManifest) error {
	manifest = manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return err
	}
	if manifest.WorkspaceID != aggregate.Facts.WorkspaceID || manifest.RunID != aggregate.Facts.RunID ||
		manifest.DefinitionRevision != aggregate.Facts.DefinitionRevision || manifest.DefinitionSeedHash != aggregate.Facts.DefinitionSeed.Hash ||
		manifest.OperationContractRevision != aggregate.Facts.Activation.ExecutionContract.OperationContractRevision {
		return fmt.Errorf("%w: runtime semantic manifest identity is stale", common.ErrConflict)
	}
	return nil
}

// semanticCatalogInvalidationLineage binds a retired semantic generation to
// the durable prerequisite invalidation(s) that made its owner catalog stale.
// A missing record is possible only when a corrupted/legacy aggregate reaches
// the probe guard; the deterministic synthetic signature still preserves the
// exact manifest/gap identity in the generation audit record.
func semanticCatalogInvalidationLineage(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeSemanticPlanManifest, gaps []RuntimeSemanticCatalogFreshnessGap) ([]string, []string) {
	manifest = manifest.Normalized()
	ids := make([]string, 0, len(gaps))
	gapIDs := make(map[string]struct{}, len(gaps))
	for _, gap := range gaps {
		id := strings.TrimSpace(gap.PrerequisiteID)
		if id == "" {
			continue
		}
		if _, duplicate := gapIDs[id]; duplicate {
			continue
		}
		gapIDs[id] = struct{}{}
		ids = append(ids, id)
	}
	// A prerequisite invalidation ledger is intentionally append-only and may
	// contain several generations of the same stable prerequisite ID.  Only
	// records not already attached to an immutable retired generation belong to
	// this retirement.  Without this fence, every later generation would repeat
	// all historical signatures and make the lineage look causally ambiguous.
	retiredSignatures := make(map[string]struct{})
	for _, history := range aggregate.SemanticPlanning.History {
		for _, signature := range history.InvalidationSignatures {
			signature = strings.TrimSpace(signature)
			if signature != "" {
				retiredSignatures[signature] = struct{}{}
			}
		}
	}
	signatures := make([]string, 0, len(aggregate.PrerequisiteInvalidations))
	for _, invalidation := range aggregate.PrerequisiteInvalidations {
		invalidation = invalidation.Normalized()
		if _, alreadyRetired := retiredSignatures[invalidation.Signature]; alreadyRetired {
			continue
		}
		matched := false
		for _, prerequisiteID := range invalidation.PrerequisiteIDs {
			if _, found := gapIDs[prerequisiteID]; found {
				matched = true
				break
			}
		}
		if matched && invalidation.Signature != "" {
			signatures = append(signatures, invalidation.Signature)
		}
	}
	if len(signatures) == 0 {
		// Keep this deterministic and bounded; it is an audit reference, not a
		// provider-facing error string or a retry instruction.
		logicalRequestID := strings.TrimSpace(aggregate.SemanticPlanning.LogicalRequestID)
		signatures = append(signatures, "semantic-catalog-invalidated/"+manifest.Hash+"/"+logicalRequestID+"/"+strings.Join(ids, ","))
	}
	sort.Strings(signatures)
	return ids, signatures
}

// retireSemanticPlanningGeneration clears only the active semantic authority
// and appends a complete immutable snapshot to its owner-local history. It is
// intentionally the sole helper that performs this reset, so receipt-driven
// invalidation, stale refresh and the probe guard cannot diverge in what they
// preserve or discard.
func retireSemanticPlanningGeneration(aggregate *RuntimeAggregate, reason runtimecontract.RuntimeSemanticPlanningRetirementReason, gaps []RuntimeSemanticCatalogFreshnessGap) error {
	if aggregate == nil {
		return common.ErrInvalidArgument
	}
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningRequired {
		return nil
	}
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningFailed {
		return fmt.Errorf("%w: failed semantic planning generation cannot be reopened", common.ErrConflict)
	}
	if ledger.Manifest == nil {
		return fmt.Errorf("%w: semantic planning generation has no manifest", common.ErrConflict)
	}
	ids, signatures := semanticCatalogInvalidationLineage(*aggregate, *ledger.Manifest, gaps)
	if len(ids) == 0 || len(signatures) == 0 {
		return fmt.Errorf("%w: semantic planning invalidation has no catalog lineage", common.ErrConflict)
	}
	if len(ledger.History) >= runtimecontract.RuntimeSemanticPlanningHistoryLimit {
		return fmt.Errorf("%w: semantic planning generation history exhausted", common.ErrConflict)
	}
	snapshot := ledger.Clone()
	// A history entry is a complete snapshot, but nested history would duplicate
	// old generations and turn the persisted record into an unbounded tree.
	snapshot.History = nil
	history := runtimecontract.RuntimeSemanticPlanningGeneration{
		RetiredAtStateRevision: aggregate.StateRevision + 1,
		Reason:                 reason,
		InvalidationSignatures: signatures,
		PrerequisiteIDs:        ids,
		Snapshot:               snapshot,
	}
	if err := history.Validate(); err != nil {
		return err
	}
	newLedger := runtimecontract.RuntimeSemanticPlanningLedger{
		SchemaVersion:          ledger.SchemaVersion,
		Phase:                  runtimecontract.RuntimeSemanticPlanningRequired,
		DefinitionSeed:         ledger.DefinitionSeed.Clone(),
		FeasibilityGaps:        append([]runtimecontract.RuntimeSemanticFeasibilityGap(nil), ledger.FeasibilityGaps...),
		CatalogRefreshAttempts: ledger.CatalogRefreshAttempts,
		History:                append(append([]runtimecontract.RuntimeSemanticPlanningGeneration(nil), ledger.History...), history),
	}
	if err := newLedger.Validate(); err != nil {
		return err
	}
	retireSemanticPlanningRequestSessions(aggregate, ledger)
	aggregate.SemanticPlanning = newLedger
	return nil
}

func retireSemanticPlanningRequestSessions(aggregate *RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger) {
	if aggregate == nil || ledger.Manifest == nil {
		return
	}
	manifestHash := ledger.Manifest.Hash
	for idx := range aggregate.CandidateRequestSessions {
		session := &aggregate.CandidateRequestSessions[idx]
		if session.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan || session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal {
			continue
		}
		// A semantic generation owns every request session that rendered its
		// sealed manifest (order, focus batch and bounded correction sessions).
		// Sessions from a different manifest remain independent even if a
		// logical request ID was accidentally reused.
		if session.ManifestHash != manifestHash {
			continue
		}
		session.State = runtimecontract.RuntimeCandidateRequestSessionTerminal
		session.ProviderSubmitted = false
		// Keep the last typed provider-document outcome as immutable audit
		// lineage. Closing this old session must not erase an earlier incomplete
		// result that explains what the provider actually returned.
		session.LastOutcome = runtimecontract.RuntimeCandidateRequestOutcome{
			Class:     runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale,
			Code:      "semantic_catalog_invalidated",
			Signature: "semantic-catalog-invalidated/" + manifestHash + "/" + strings.TrimSpace(session.LogicalRequestID),
		}
	}
}

// semanticPlanningCatalogsCurrent is the common guard used before a sealed
// semantic generation may be reused. It returns false for any missing,
// retired, or snapshot-mismatched owner observation.
func semanticPlanningCatalogsCurrent(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeSemanticPlanManifest) ([]RuntimeSemanticCatalogFreshnessGap, bool) {
	gaps := SemanticManifestCatalogFreshnessGaps(aggregate, manifest)
	return gaps, len(gaps) == 0
}

// semanticCatalogPrerequisitePending is the receipt-to-generation fence used
// by commitPrerequisite. It intentionally derives retirement from the current
// pending prerequisite, not from an unbounded scan of historical invalidation
// records: a prior generation may legitimately contain the same stable
// prerequisite ID after a fresh catalog was reacquired.
func semanticCatalogPrerequisitePending(aggregate RuntimeAggregate, prerequisiteID string) bool {
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed || aggregate.SemanticPlanning.Manifest == nil {
		return false
	}
	prerequisiteID = strings.TrimSpace(prerequisiteID)
	if prerequisiteID == "" {
		return false
	}
	var catalog runtimecontract.RuntimeOperationAdmissibility
	found := false
	for _, value := range aggregate.SemanticPlanning.Manifest.OwnerCatalogs {
		value = value.Normalized()
		if value.PrerequisiteID == prerequisiteID {
			catalog, found = value, true
			break
		}
	}
	if !found {
		return false
	}
	for _, pending := range aggregate.Facts.Prerequisites.Pending {
		if pending.ID == prerequisiteID && pending.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility &&
			pending.OwnerModuleID == catalog.OwnerModuleID {
			return true
		}
	}
	return false
}

// resetSemanticPlanningForInvalidatedCatalog retires the active generation
// when an owner catalog is no longer current. The caller remains responsible
// for the surrounding SM-2 reducer/CAS transition.
func resetSemanticPlanningForInvalidatedCatalog(aggregate *RuntimeAggregate, reason runtimecontract.RuntimeSemanticPlanningRetirementReason) (bool, error) {
	if aggregate == nil || aggregate.SemanticPlanning.Manifest == nil {
		return false, nil
	}
	if aggregate.ActiveDispatch != nil {
		// A sealed dispatch may finish its immutable operation set. Its receipt
		// invalidation is applied to the next authority only after dispatch
		// completion; never tear down the ledger needed to admit that receipt.
		return false, nil
	}
	gaps, current := semanticPlanningCatalogsCurrent(*aggregate, *aggregate.SemanticPlanning.Manifest)
	if current {
		return false, nil
	}
	if err := retireSemanticPlanningGeneration(aggregate, reason, gaps); err != nil {
		return false, err
	}
	return true, nil
}

// requireSemanticContract is the SM-2 entry fence. Initialization already
// persists this phase; duplicate delivery is therefore a read-only no-op.
func (c *RuntimeController) requireSemanticContract(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, feasibilityBatchID string, refreshGeneration int) (RuntimeAggregate, error) {
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
	if feasibilityBatchID == "" || ledger.Phase != runtimecontract.RuntimeSemanticPlanningFeasibilityRequired ||
		ledger.Manifest == nil || ledger.Document == nil || ledger.FeasibilityBatchID != feasibilityBatchID ||
		ledger.CatalogRefreshAttempts != refreshGeneration {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog refresh has no matching feasibility batch", common.ErrConflict)
	}

	gapFact, found := refreshableRuntimeSemanticFeasibility(ledger.Feasibilities)
	if !found || gapFact.OriginalDiagnostic == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic feasibility batch has no refreshable owner fact", common.ErrConflict)
	}
	catalog, found := runtimeSemanticCatalogForOwner(*ledger.Manifest, gapFact.OwnerModuleID)
	if !found || catalog.SnapshotID != gapFact.CatalogSnapshotID {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic feasibility catalog identity is stale", common.ErrConflict)
	}
	observation, found := semanticCatalogPrerequisiteObservation(aggregate.Facts, catalog, gapFact.CatalogObservationID)
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog prerequisite observation is not current", common.ErrConflict)
	}

	facts := aggregate.Facts
	var prerequisiteIDs, observationIDs []string
	var invalidatedDependencies []runtimecontract.RuntimeFreshnessDependency
	var stale *runtimecontract.RuntimePrerequisiteStale
	if gapFact.Status == runtimecontract.RuntimeSemanticFeasibilityStale {
		value, staleErr := runtimeSemanticCatalogStale(observation, *gapFact.OriginalDiagnostic)
		if staleErr != nil {
			return RuntimeAggregate{}, staleErr
		}
		facts, prerequisiteIDs, observationIDs, invalidatedDependencies, err = invalidatePrerequisiteObservationsForStale(facts, value)
		if err != nil {
			return RuntimeAggregate{}, err
		}
		stale = &value
	} else {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog refresh requires a stale owner revision", common.ErrConflict)
	}
	recovery, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if !consumed {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog refresh budget exhausted", common.ErrConflict)
	}
	facts.Recovery = recovery

	invalidation := runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature:       fmt.Sprintf("semantic-catalog-refresh/%d/%s/%s", refreshGeneration, feasibilityBatchID, ledger.FeasibilitySetHash),
		PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs,
		InvalidatedDependencies: invalidatedDependencies, Stale: stale,
	}
	if err := invalidation.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}

	gap := runtimecontract.RuntimeSemanticFeasibilityGap{
		RefreshGeneration: refreshGeneration,
		ManifestHash:      ledger.Manifest.Hash, DocumentHash: ledger.DocumentHash,
		FeasibilityBatchID: ledger.FeasibilityBatchID, FeasibilitySetHash: ledger.FeasibilitySetHash,
		Feasibilities: append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), ledger.Feasibilities...),
		Rejections:    append([]runtimecontract.RuntimeOriginalDiagnostic(nil), ledger.Rejections...),
	}
	ledger.FeasibilityGaps = append(ledger.FeasibilityGaps, gap)
	ledger.CatalogRefreshAttempts++
	// Keep the stale generation as an immutable history snapshot, then clear
	// every active manifest/order/batch/fact field in one owner-local reset.
	// The invalidation record is installed before comparison so the history can
	// point to the exact durable stale event.
	aggregate.SemanticPlanning = ledger
	aggregate.Facts = facts
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, invalidation)
	retired, resetErr := resetSemanticPlanningForInvalidatedCatalog(&aggregate, runtimecontract.RuntimeSemanticPlanningRetirementCatalogRefresh)
	if resetErr != nil {
		return RuntimeAggregate{}, resetErr
	}
	if !retired {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog refresh cannot retire an active dispatch", common.ErrConflict)
	}
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

// invalidateSemanticCatalogGeneration is the private SM-2 handoff used by
// semantic feasibility probing when the active manifest no longer has a
// current owner observation. It is intentionally separate from the generic
// stale-refresh event: missing/mismatched current facts must reopen a fresh
// semantic generation, but must not invent a capability-owner prerequisite
// command or consume the bounded stale-refresh budget.
func (c *RuntimeController) invalidateSemanticCatalogGeneration(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifestHash, logicalRequestID string) (RuntimeAggregate, error) {
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
func (c *RuntimeController) compileSemanticPlanBatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, batch runtimecontract.RuntimeSemanticPlanBatch) (RuntimeAggregate, error) {
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(aggregate RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if ledger.Manifest == nil || ledger.CoverageOrderDocument == nil || ledger.Document == nil || ledger.Document.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder || ledger.ActiveBatchID != "" {
			return false, fmt.Errorf("%w: semantic batch requires an accepted coverage order", common.ErrConflict)
		}
		if ledger.NextBatchSequence != len(ledger.Batches) || batch.Sequence != ledger.NextBatchSequence {
			return false, fmt.Errorf("%w: semantic batch sequence is not the next contiguous sequence", common.ErrConflict)
		}
		batch = batch.Normalized()
		if batch.State != runtimecontract.RuntimeSemanticPlanBatchPending && batch.State != runtimecontract.RuntimeSemanticPlanBatchRequesting {
			return false, fmt.Errorf("%w: semantic batch compile requires pending/requesting state", common.ErrConflict)
		}
		if err := batch.ValidateAgainstManifest(*ledger.Manifest); err != nil {
			return false, err
		}
		maxItems := aggregate.Facts.ConvergencePolicy.MaxCandidateContextItems
		if maxItems <= 0 {
			maxItems = len(ledger.Manifest.CoverageItems)
		}
		expected, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
			*ledger.Manifest, *ledger.CoverageOrderDocument, batch.Sequence, batch.RequestLogicalID,
			aggregate.Facts.Focus.CoverageWorkItemIDs, ledger.PendingCoverageIDs, maxItems,
		)
		if err != nil {
			return false, err
		}
		if !runtimecontract.SameRuntimeSemanticPlanBatchImmutableIdentity(batch, expected) {
			return false, fmt.Errorf("%w: semantic batch identity does not match current focus and pending lineage", common.ErrConflict)
		}
		requesting, err := runtimecontract.AdvanceRuntimeSemanticPlanBatch(expected, runtimecontract.RuntimeSemanticPlanBatchRequesting)
		if err != nil {
			return false, err
		}
		for idx := range ledger.Batches {
			existing := ledger.Batches[idx]
			if existing.Sequence == batch.Sequence || existing.BatchID == batch.BatchID {
				if !runtimecontract.SameRuntimeSemanticPlanBatchImmutableIdentity(existing, requesting) {
					return false, fmt.Errorf("%w: semantic batch sequence or identity already exists with different contents", common.ErrConflict)
				}
				if existing.State == runtimecontract.RuntimeSemanticPlanBatchFailed {
					return false, fmt.Errorf("%w: semantic batch was already failed", common.ErrConflict)
				}
				if existing.State == runtimecontract.RuntimeSemanticPlanBatchPending {
					ledger.Batches[idx] = requesting
					ledger.ActiveBatchID = requesting.BatchID
					ledger.LogicalRequestID = requesting.RequestLogicalID
					ledger.Phase = runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen
					return true, nil
				}
				// The compile event was replayed after the private transition (or
				// after a later batch state). Immutable identity is sufficient for a
				// read-only replay; lifecycle state remains owner-authoritative.
				return false, nil
			}
		}
		ledger.Batches = append(ledger.Batches, requesting)
		ledger.ActiveBatchID = requesting.BatchID
		ledger.LogicalRequestID = requesting.RequestLogicalID
		// Correction lineage is scoped to one provider document. A proof-batch
		// request is a new sealed document contract and must never inherit an
		// order-document attempt or directive.
		ledger.Attempt = 0
		ledger.CorrectionDirective = nil
		ledger.Rejections = nil
		// Feasibility fields are scoped to the active batch. Completed facts
		// remain durably archived in BatchFeasibilityResults; the next batch
		// starts with an empty working set.
		ledger.FeasibilityBatchID = ""
		ledger.Feasibilities = nil
		ledger.FeasibilitySetHash = ""
		ledger.Phase = runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen
		ledger.NextBatchSequence = requesting.Sequence + 1
		ledger.PendingCoverageIDs = append([]common.CoverageWorkItemID(nil), requesting.PendingCoverageWorkItemIDs...)
		return true, nil
	})
}

func (c *RuntimeController) advanceSemanticPlanBatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, identity runtimecontract.RuntimeSemanticPlanBatch, next runtimecontract.RuntimeSemanticPlanBatchState) (RuntimeAggregate, error) {
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		for idx := range ledger.Batches {
			if ledger.Batches[idx].BatchID != identity.BatchID {
				continue
			}
			persisted := ledger.Batches[idx].Clone()
			identity = identity.Normalized()
			if !runtimecontract.SameRuntimeSemanticPlanBatchImmutableIdentity(persisted, identity) {
				return false, fmt.Errorf("%w: semantic batch immutable identity is stale", common.ErrConflict)
			}
			if persisted.State == next {
				return false, nil
			}
			if persisted.Hash != identity.Hash || persisted.State != identity.State {
				return false, fmt.Errorf("%w: semantic batch identity is stale", common.ErrConflict)
			}
			advanced, err := runtimecontract.AdvanceRuntimeSemanticPlanBatch(persisted, next)
			if err != nil {
				return false, err
			}
			ledger.Batches[idx] = advanced
			return true, nil
		}
		return false, fmt.Errorf("%w: semantic batch is missing", common.ErrConflict)
	})
}

func (c *RuntimeController) observeSemanticPlanBatchFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, result runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult) (RuntimeAggregate, error) {
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		for _, existing := range ledger.BatchFeasibilityResults {
			if existing.BatchID != result.BatchID {
				continue
			}
			if existing.Hash == result.Hash {
				return false, nil
			}
			return false, fmt.Errorf("%w: conflicting semantic batch feasibility result", common.ErrConflict)
		}
		var batch *runtimecontract.RuntimeSemanticPlanBatch
		for idx := range ledger.Batches {
			if ledger.Batches[idx].BatchID == result.BatchID {
				value := ledger.Batches[idx]
				batch = &value
				break
			}
		}
		if batch == nil || batch.Hash != result.BatchHash {
			return false, fmt.Errorf("%w: semantic batch feasibility result is stale", common.ErrConflict)
		}
		if batch.BatchID != ledger.ActiveBatchID || batch.State != runtimecontract.RuntimeSemanticPlanBatchAccepted {
			return false, fmt.Errorf("%w: semantic batch feasibility result is not for the accepted active batch", common.ErrConflict)
		}
		var document *runtimecontract.RuntimeSemanticPlanDocument
		for idx := range ledger.BatchDocuments {
			if ledger.BatchDocuments[idx].BatchID == result.BatchID {
				value := ledger.BatchDocuments[idx].Clone()
				document = &value
				break
			}
		}
		if document == nil {
			return false, fmt.Errorf("%w: semantic batch feasibility result has no document", common.ErrConflict)
		}
		if err := result.Validate(*batch, *document, *ledger.Manifest); err != nil {
			return false, err
		}
		if result.Hash != runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(result) {
			return false, fmt.Errorf("%w: semantic batch feasibility result hash mismatch", common.ErrConflict)
		}
		ledger.BatchFeasibilityResults = append(ledger.BatchFeasibilityResults, result.Normalized())
		return true, nil
	})
}

func (c *RuntimeController) observeSemanticPlanBatchDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, document runtimecontract.RuntimeSemanticPlanDocument) (RuntimeAggregate, error) {
	document = document.Normalized()
	if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch && len(document.CoverageOrder) != 0 {
		document.CoverageOrder = nil
		document = document.Normalized()
	}
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if ledger.Manifest == nil || ledger.Document == nil || document.BatchID == "" {
			return false, fmt.Errorf("%w: semantic batch document identity is incomplete", common.ErrConflict)
		}
		if err := document.ValidateAgainstManifest(*ledger.Manifest); err != nil {
			return false, err
		}
		if ledger.ActiveBatchID != document.BatchID {
			return false, fmt.Errorf("%w: semantic batch document is not active", common.ErrConflict)
		}
		for _, existing := range ledger.BatchDocuments {
			if existing.BatchID != document.BatchID {
				continue
			}
			if runtimecontract.StableRuntimeSemanticPlanDocumentHash(existing) == runtimecontract.StableRuntimeSemanticPlanDocumentHash(document) {
				return false, nil
			}
			return false, fmt.Errorf("%w: conflicting semantic batch document", common.ErrConflict)
		}
		ledger.BatchDocuments = append(ledger.BatchDocuments, document.Clone())
		for idx := range ledger.Batches {
			if ledger.Batches[idx].BatchID == document.BatchID {
				if ledger.Batches[idx].State != runtimecontract.RuntimeSemanticPlanBatchRequesting {
					return false, fmt.Errorf("%w: semantic batch document is not expected", common.ErrConflict)
				}
				advanced, advanceErr := runtimecontract.AdvanceRuntimeSemanticPlanBatch(ledger.Batches[idx], runtimecontract.RuntimeSemanticPlanBatchDocumentComplete)
				if advanceErr != nil {
					return false, advanceErr
				}
				advanced.DocumentHash = runtimecontract.StableRuntimeSemanticPlanDocumentHash(document)
				advanced.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(advanced)
				ledger.Batches[idx] = advanced
				break
			}
		}
		ledger.Phase = runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted
		return true, nil
	})
}

func (c *RuntimeController) mergeSemanticPlanBatchDocuments(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, merged runtimecontract.RuntimeSemanticPlanDocument) (RuntimeAggregate, error) {
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		// A response-loss replay arrives after the durable merge CAS cleared the
		// active batch. Match the persisted immutable hash before consulting the
		// working-state guards; duplicate private events are a read-only no-op and
		// must not be misclassified as "no coverage order".
		incomingHash := runtimecontract.StableRuntimeSemanticPlanDocumentHash(merged)
		if ledger.MergedDocumentHash == incomingHash && ledger.MergedDocument != nil &&
			runtimecontract.StableRuntimeSemanticPlanDocumentHash(*ledger.MergedDocument) == incomingHash {
			return false, nil
		}
		if ledger.Manifest == nil || ledger.CoverageOrderDocument == nil || ledger.Document == nil || ledger.Document.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder || ledger.ActiveBatchID == "" {
			return false, fmt.Errorf("%w: semantic merge has no coverage order", common.ErrConflict)
		}
		var batch *runtimecontract.RuntimeSemanticPlanBatch
		for idx := range ledger.Batches {
			if ledger.Batches[idx].BatchID == ledger.ActiveBatchID {
				value := ledger.Batches[idx].Clone()
				batch = &value
				break
			}
		}
		if batch == nil || batch.State != runtimecontract.RuntimeSemanticPlanBatchAccepted {
			return false, fmt.Errorf("%w: semantic merge has no accepted active batch", common.ErrConflict)
		}
		batchDocument, found := runtimeSemanticBatchDocument(ledger.BatchDocuments, batch.BatchID)
		if !found {
			return false, fmt.Errorf("%w: semantic merge has no active batch document", common.ErrConflict)
		}
		expected, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(*ledger.Manifest, *batch, batchDocument)
		if err != nil {
			return false, err
		}
		hash := runtimecontract.StableRuntimeSemanticPlanDocumentHash(expected)
		if hash != runtimecontract.StableRuntimeSemanticPlanDocumentHash(merged) {
			return false, fmt.Errorf("%w: semantic merge expands or changes the active focus batch", common.ErrConflict)
		}
		if ledger.MergedDocumentHash == hash {
			return false, nil
		}
		if ledger.MergedDocumentHash != "" {
			return false, fmt.Errorf("%w: conflicting merged semantic document", common.ErrConflict)
		}
		value := expected.Clone()
		ledger.MergedDocument = &value
		ledger.MergedDocumentHash = hash
		ledger.Document = &value
		ledger.DocumentHash = hash
		ledger.FeasibilityBatchID = runtimecontract.StableRuntimeSemanticFeasibilityBatchID(*ledger.Manifest, value)
		if !containsString(ledger.CompletedBatchIDs, batch.BatchID) {
			ledger.CompletedBatchIDs = append(ledger.CompletedBatchIDs, batch.BatchID)
		}
		ledger.ActiveBatchID = ""
		// Pending coverage belongs to the immutable global order lineage, not
		// to this focus's binding set.  Keep it so the next focus can open its
		// own bounded proof batch after this one settles.
		ledger.Phase = runtimecontract.RuntimeSemanticPlanningFeasibilityRequired
		return true, nil
	})
}

func (c *RuntimeController) mergeSemanticPlanBatchFeasibilities(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, values []runtimecontract.RuntimeSemanticProofFeasibility) (RuntimeAggregate, error) {
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if ledger.MergedDocument == nil || ledger.Manifest == nil {
			return false, fmt.Errorf("%w: semantic feasibility merge has no merged document", common.ErrConflict)
		}
		if err := runtimecontract.ValidateRuntimeSemanticFeasibilitySet(*ledger.Manifest, *ledger.MergedDocument, values, false); err != nil {
			return false, err
		}
		hash := runtimecontract.StableRuntimeSemanticFeasibilitySetHash(values)
		if ledger.FeasibilitySetHash == hash {
			return false, nil
		}
		if ledger.FeasibilitySetHash != "" {
			if ledger.ActiveBatchID != "" || len(ledger.BatchFeasibilityResults) != len(ledger.Batches) {
				return false, fmt.Errorf("%w: conflicting merged semantic feasibility", common.ErrConflict)
			}
			for _, batch := range ledger.Batches {
				if batch.State != runtimecontract.RuntimeSemanticPlanBatchAccepted {
					return false, fmt.Errorf("%w: merged semantic feasibility has incomplete batch", common.ErrConflict)
				}
			}
		}
		ledger.Feasibilities = append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), values...)
		ledger.FeasibilitySetHash = hash
		ledger.FeasibilityBatchID = runtimecontract.StableRuntimeSemanticFeasibilityBatchID(*ledger.Manifest, *ledger.MergedDocument)
		return true, nil
	})
}

func refreshableRuntimeSemanticFeasibility(values []runtimecontract.RuntimeSemanticProofFeasibility) (runtimecontract.RuntimeSemanticProofFeasibility, bool) {
	for _, value := range values {
		if value.Status == runtimecontract.RuntimeSemanticFeasibilityStale {
			return value.Clone(), true
		}
	}
	return runtimecontract.RuntimeSemanticProofFeasibility{}, false
}

func runtimeSemanticCatalogForOwner(manifest runtimecontract.RuntimeSemanticPlanManifest, ownerModuleID string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	for _, catalog := range manifest.Normalized().OwnerCatalogs {
		if catalog.OwnerModuleID == ownerModuleID {
			return catalog, true
		}
	}
	return runtimecontract.RuntimeOperationAdmissibility{}, false
}

func semanticCatalogPrerequisiteObservation(facts runtimecontract.RuntimeFacts, catalog runtimecontract.RuntimeOperationAdmissibility, observationID string) (runtimecontract.RuntimePrerequisiteObservation, bool) {
	observationID = strings.TrimSpace(observationID)
	for _, observation := range facts.Normalized().Resources.PrerequisiteObservations {
		if observation.PrerequisiteID == catalog.PrerequisiteID && observation.ObservationID == observationID &&
			observation.OwnerModuleID == catalog.OwnerModuleID && observation.ContentHash == catalog.SnapshotID {
			return observation, true
		}
	}
	return runtimecontract.RuntimePrerequisiteObservation{}, false
}

func runtimeSemanticCatalogStale(observation runtimecontract.RuntimePrerequisiteObservation, diagnostic runtimecontract.RuntimeOriginalDiagnostic) (runtimecontract.RuntimePrerequisiteStale, error) {
	diagnosticValue := diagnostic.Normalized()
	if diagnosticValue == nil || diagnosticValue.ExpectedRevision == "" || diagnosticValue.ObservedRevision == "" {
		return runtimecontract.RuntimePrerequisiteStale{}, fmt.Errorf("%w: semantic stale diagnostic requires expected and observed revisions", common.ErrConflict)
	}
	for _, dependency := range observation.Normalized().Dependencies {
		if dependency.OwnerModuleID != diagnosticValue.OwnerModuleID || dependency.Revision != diagnosticValue.ExpectedRevision {
			continue
		}
		stale := runtimecontract.RuntimePrerequisiteStale{
			PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID,
			Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef,
			ExpectedRevision: dependency.Revision, ObservedRevision: diagnosticValue.ObservedRevision,
			Reason: diagnosticValue.OriginalMessage,
		}
		return stale, stale.Validate()
	}
	return runtimecontract.RuntimePrerequisiteStale{}, fmt.Errorf("%w: semantic stale diagnostic does not match catalog dependency", common.ErrConflict)
}

// startSemanticPlanRequest is the only SM-3 transition that opens attempt 0.
func (c *RuntimeController) startSemanticPlanRequest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifest runtimecontract.RuntimeSemanticPlanManifest, stablePrefixHash, logicalRequestID string) (RuntimeAggregate, error) {
	manifest = manifest.Normalized()
	stablePrefixHash = strings.TrimSpace(stablePrefixHash)
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	if stablePrefixHash == "" || logicalRequestID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(aggregate RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if (ledger.Phase == runtimecontract.RuntimeSemanticPlanningRequestOpen || ledger.Phase == runtimecontract.RuntimeSemanticPlanningOrderRequestOpen) && ledger.Manifest != nil && ledger.Manifest.Hash == manifest.Hash && ledger.StablePrefixHash == stablePrefixHash && ledger.LogicalRequestID == logicalRequestID && ledger.Attempt == 0 {
			return false, nil
		}
		if ledger.Phase != runtimecontract.RuntimeSemanticPlanningRequired {
			return false, fmt.Errorf("%w: semantic plan request cannot start from phase %q", common.ErrConflict, ledger.Phase)
		}
		if err := validateSemanticManifestIdentity(aggregate, manifest); err != nil {
			return false, err
		}
		ledger.Phase = runtimecontract.RuntimeSemanticPlanningRequestOpen
		ledger.Manifest = &manifest
		ledger.Attempt = 0
		ledger.StablePrefixHash = stablePrefixHash
		ledger.LogicalRequestID = logicalRequestID
		return true, nil
	})
}

// evaluateSemanticPlan persists one complete accepted provider document. It
// cannot create feasibility, binding, dispatch, coverage or terminal facts.
func (c *RuntimeController) evaluateSemanticPlan(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, document runtimecontract.RuntimeSemanticPlanDocument) (RuntimeAggregate, error) {
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	document = document.Normalized()
	// A proof response is focus-local and must not carry a second global order
	// authority. Canonicalize the provider document before hashing/persisting so
	// the immutable batch document and the Orchestrator merge have one identity.
	if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch && len(document.CoverageOrder) != 0 {
		document.CoverageOrder = nil
		document = document.Normalized()
	}
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		documentHash := runtimecontract.StableRuntimeSemanticPlanDocumentHash(document)
		if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch && document.BatchID == "" {
			return false, fmt.Errorf("%w: semantic proof document requires active batch identity", common.ErrConflict)
		}
		if ledger.LogicalRequestID == logicalRequestID {
			if document.BatchID == "" && ledger.DocumentHash == documentHash {
				return false, nil
			}
			for _, existing := range ledger.BatchDocuments {
				if existing.BatchID == document.BatchID && runtimecontract.StableRuntimeSemanticPlanDocumentHash(existing) == documentHash {
					return false, nil
				}
			}
		}
		if (ledger.Phase != runtimecontract.RuntimeSemanticPlanningOrderRequestOpen && ledger.Phase != runtimecontract.RuntimeSemanticPlanningRequestOpen && ledger.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen && ledger.Phase != runtimecontract.RuntimeSemanticPlanningCorrectionRequired) || ledger.Manifest == nil || ledger.LogicalRequestID != logicalRequestID {
			return false, fmt.Errorf("%w: semantic plan document has no matching request", common.ErrConflict)
		}
		if err := document.ValidateAgainstManifest(*ledger.Manifest); err != nil {
			return false, err
		}
		// Every proof document is focus-local. A proof document without the
		// Orchestrator-issued batch identity would be a second full-manifest
		// authority path and could admit later CWI outside the active focus.
		switch ledger.Phase {
		case runtimecontract.RuntimeSemanticPlanningOrderRequestOpen, runtimecontract.RuntimeSemanticPlanningRequestOpen:
			if document.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder || document.BatchID != "" {
				return false, fmt.Errorf("%w: initial semantic request requires coverage-order document", common.ErrConflict)
			}
		case runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen:
			if document.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch || document.BatchID == "" || document.BatchID != ledger.ActiveBatchID {
				return false, fmt.Errorf("%w: proof request document is not bound to active batch", common.ErrConflict)
			}
		case runtimecontract.RuntimeSemanticPlanningCorrectionRequired:
			if ledger.ActiveBatchID == "" {
				if document.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder || document.BatchID != "" {
					return false, fmt.Errorf("%w: coverage-order correction has invalid document identity", common.ErrConflict)
				}
			} else if document.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch || document.BatchID != ledger.ActiveBatchID {
				return false, fmt.Errorf("%w: proof-batch correction is outside active batch", common.ErrConflict)
			}
		default:
			return false, fmt.Errorf("%w: semantic plan document cannot be evaluated from phase %q", common.ErrConflict, ledger.Phase)
		}
		if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
			// The global provider order is immutable lineage.  It remains separate
			// from the current-focus proof document so later proof batches cannot
			// broaden their authority by restating it.
			order := document.Clone()
			ledger.CoverageOrderDocument = &order
			ledger.PendingCoverageIDs = append([]common.CoverageWorkItemID(nil), order.CoverageOrder...)
			ledger.Phase = runtimecontract.RuntimeSemanticPlanningOrderAccepted
		} else if document.BatchID != "" {
			if ledger.ActiveBatchID != document.BatchID {
				return false, fmt.Errorf("%w: semantic batch document is not the active batch", common.ErrConflict)
			}
			for _, existing := range ledger.BatchDocuments {
				if existing.BatchID != document.BatchID {
					continue
				}
				if runtimecontract.StableRuntimeSemanticPlanDocumentHash(existing) == documentHash {
					return false, nil
				}
				return false, fmt.Errorf("%w: conflicting semantic batch document", common.ErrConflict)
			}
			for idx := range ledger.Batches {
				if ledger.Batches[idx].BatchID == document.BatchID {
					advanced, advanceErr := runtimecontract.AdvanceRuntimeSemanticPlanBatch(ledger.Batches[idx], runtimecontract.RuntimeSemanticPlanBatchDocumentComplete)
					if advanceErr != nil {
						return false, advanceErr
					}
					advanced.DocumentHash = documentHash
					advanced.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(advanced)
					ledger.Batches[idx] = advanced
					break
				}
			}
			ledger.BatchDocuments = append(ledger.BatchDocuments, document.Clone())
			ledger.Phase = runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted
			return true, nil
		} else {
			// The legacy full-document proposal path has no bounded focus and is
			// intentionally unreachable. The batch branch above is the only proof
			// admission path.
			return false, fmt.Errorf("%w: semantic proof document has no active batch", common.ErrConflict)
		}
		ledger.Document = &document
		ledger.DocumentHash = documentHash
		return true, nil
	})
}

// requireSemanticFeasibility is the SM-2 handoff from an accepted proposal to
// a bounded owner-probe batch.
func (c *RuntimeController) requireSemanticFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if ledger.Phase == runtimecontract.RuntimeSemanticPlanningFeasibilityRequired {
			return false, nil
		}
		if ledger.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted || ledger.Manifest == nil || ledger.Document == nil || ledger.ActiveBatchID == "" {
			return false, fmt.Errorf("%w: semantic feasibility requires an accepted focus proof batch", common.ErrConflict)
		}
		batch, found := runtimeSemanticBatch(ledger.Batches, ledger.ActiveBatchID)
		if !found || batch.State != runtimecontract.RuntimeSemanticPlanBatchDocumentComplete {
			return false, fmt.Errorf("%w: semantic feasibility has no document-complete active batch", common.ErrConflict)
		}
		feasibilityDocument, found := runtimeSemanticBatchDocument(ledger.BatchDocuments, ledger.ActiveBatchID)
		if !found || feasibilityDocument.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch {
			return false, fmt.Errorf("%w: semantic feasibility has no active proof document", common.ErrConflict)
		}
		ledger.Phase = runtimecontract.RuntimeSemanticPlanningFeasibilityRequired
		for idx := range ledger.Batches {
			if ledger.Batches[idx].BatchID == ledger.ActiveBatchID {
				advanced, advanceErr := runtimecontract.AdvanceRuntimeSemanticPlanBatch(ledger.Batches[idx], runtimecontract.RuntimeSemanticPlanBatchFeasibilityPending)
				if advanceErr != nil {
					return false, advanceErr
				}
				ledger.Batches[idx] = advanced
				break
			}
		}
		ledger.FeasibilityBatchID = runtimecontract.StableRuntimeSemanticFeasibilityBatchID(*ledger.Manifest, feasibilityDocument)
		return true, nil
	})
}

// observeSemanticFeasibility persists one canonical owner's factual result.
// Conflicting duplicates are rejected instead of replacing owner history.
func (c *RuntimeController) observeSemanticFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, batchID string, observation runtimecontract.RuntimeSemanticProofFeasibility) (RuntimeAggregate, error) {
	batchID = strings.TrimSpace(batchID)
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if ledger.Manifest == nil || ledger.Document == nil || ledger.FeasibilityBatchID != batchID {
			return false, fmt.Errorf("%w: semantic feasibility batch is stale", common.ErrConflict)
		}
		// The final owner fact advances a proof batch to its completed phase in
		// the same CAS. A duplicate delivery can therefore arrive after that
		// transition (or after seal); compare the persisted fact before applying
		// the phase gate so exact replays are read-only and conflicting facts are
		// still rejected.
		incomingHash := runtimecontract.StableRuntimeSemanticFeasibilitySetHash([]runtimecontract.RuntimeSemanticProofFeasibility{observation})
		for _, existing := range ledger.Feasibilities {
			if existing.CoverageWorkItemID != observation.CoverageWorkItemID {
				continue
			}
			existingHash := runtimecontract.StableRuntimeSemanticFeasibilitySetHash([]runtimecontract.RuntimeSemanticProofFeasibility{existing})
			if existingHash == incomingHash && (ledger.Phase == runtimecontract.RuntimeSemanticPlanningFeasibilityRequired || ledger.Phase == runtimecontract.RuntimeSemanticPlanningProofBatchFeasibilityComplete || ledger.Phase == runtimecontract.RuntimeSemanticPlanningSealed) {
				return false, nil
			}
			return false, fmt.Errorf("%w: conflicting semantic feasibility observation", common.ErrConflict)
		}
		if ledger.ActiveBatchID == "" {
			return false, fmt.Errorf("%w: semantic feasibility batch is stale", common.ErrConflict)
		}
		if ledger.Phase != runtimecontract.RuntimeSemanticPlanningFeasibilityRequired {
			return false, fmt.Errorf("%w: semantic feasibility batch is stale", common.ErrConflict)
		}
		feasibilityDocument, found := runtimeSemanticBatchDocument(ledger.BatchDocuments, ledger.ActiveBatchID)
		if !found || feasibilityDocument.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch {
			return false, fmt.Errorf("%w: semantic feasibility document is missing", common.ErrConflict)
		}
		var proposal *runtimecontract.RuntimeSemanticProofProposal
		for idx := range feasibilityDocument.Proposals {
			if feasibilityDocument.Proposals[idx].CoverageWorkItemID == observation.CoverageWorkItemID {
				value := feasibilityDocument.Proposals[idx]
				proposal = &value
				break
			}
		}
		if proposal == nil {
			return false, fmt.Errorf("%w: semantic feasibility references unknown coverage", common.ErrConflict)
		}
		if err := observation.ValidateForProposal(*ledger.Manifest, *proposal); err != nil {
			return false, err
		}
		ledger.Feasibilities = append(ledger.Feasibilities, observation.Clone())
		ledger.FeasibilitySetHash = runtimecontract.StableRuntimeSemanticFeasibilitySetHash(ledger.Feasibilities)
		// Only an all-feasible owner set may advance the batch to the
		// completed/accepted state.  Stale, prerequisite-required, and
		// infeasible facts remain in the feasibility-required phase so the
		// SM-4 driver can route them through the bounded refresh/terminal
		// path before any batch merge or binding authority is created.
		allFeasible := len(ledger.Feasibilities) == len(feasibilityDocument.Proposals)
		if allFeasible {
			for _, fact := range ledger.Feasibilities {
				if fact.Status != runtimecontract.RuntimeSemanticFeasibilityFeasible {
					allFeasible = false
					break
				}
			}
		}
		if allFeasible && ledger.Phase == runtimecontract.RuntimeSemanticPlanningFeasibilityRequired {
			ledger.Phase = runtimecontract.RuntimeSemanticPlanningProofBatchFeasibilityComplete
			for idx := range ledger.Batches {
				if ledger.Batches[idx].BatchID == ledger.ActiveBatchID {
					advanced, advanceErr := runtimecontract.AdvanceRuntimeSemanticPlanBatch(ledger.Batches[idx], runtimecontract.RuntimeSemanticPlanBatchFeasibilityComplete)
					if advanceErr != nil {
						return false, advanceErr
					}
					advanced, advanceErr = runtimecontract.AdvanceRuntimeSemanticPlanBatch(advanced, runtimecontract.RuntimeSemanticPlanBatchAccepted)
					if advanceErr != nil {
						return false, advanceErr
					}
					ledger.Batches[idx] = advanced
					break
				}
			}
		}
		return true, nil
	})
}

// requireSemanticPlanCorrection records the complete attempt-0 rejection and
// opens exactly one correction identity under the same manifest/prefix.
func (c *RuntimeController) requireSemanticPlanCorrection(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stablePrefixHash, logicalRequestID string, expected runtimecontract.RuntimeExpectedDocumentContract, rejection runtimecontract.RuntimeOriginalDiagnostic) (RuntimeAggregate, error) {
	stablePrefixHash = strings.TrimSpace(stablePrefixHash)
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	if stablePrefixHash == "" || logicalRequestID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if err := rejection.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if ledger.Manifest == nil {
			return false, fmt.Errorf("%w: semantic correction has no sealed manifest", common.ErrConflict)
		}
		expected = expected.Clone()
		expected.CorrectionAttempt = 1
		expected.LogicalRequestID = ""
		expected.Hash = ""
		directive, deriveErr := runtimecontract.DeriveRuntimeSemanticCorrectionDirective(*ledger.Manifest, expected, stablePrefixHash, rejection)
		if deriveErr != nil {
			return false, deriveErr
		}
		if err := directive.ValidateAgainst(*ledger.Manifest, rejection); err != nil {
			return false, err
		}
		if err := validateSemanticPlanCorrectionScope(*ledger, directive); err != nil {
			return false, err
		}
		if directive.StablePrefixHash != stablePrefixHash {
			return false, fmt.Errorf("%w: semantic correction stable prefix is stale", common.ErrConflict)
		}
		if ledger.Phase == runtimecontract.RuntimeSemanticPlanningCorrectionRequired && ledger.LogicalRequestID == logicalRequestID && ledger.CorrectionDirective != nil && ledger.CorrectionDirective.Hash == directive.Hash {
			return false, nil
		}
		if (ledger.Phase != runtimecontract.RuntimeSemanticPlanningRequestOpen && ledger.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen) || ledger.Attempt != 0 || ledger.Manifest == nil || ledger.StablePrefixHash != stablePrefixHash {
			return false, fmt.Errorf("%w: semantic plan correction has no matching attempt 0", common.ErrConflict)
		}
		ledger.Phase = runtimecontract.RuntimeSemanticPlanningCorrectionRequired
		ledger.Attempt = 1
		ledger.LogicalRequestID = logicalRequestID
		value := directive.Clone()
		ledger.CorrectionDirective = &value
		ledger.Rejections = append(ledger.Rejections, *rejection.CloneOriginal())
		return true, nil
	})
}

func validateSemanticPlanCorrectionScope(ledger runtimecontract.RuntimeSemanticPlanningLedger, directive runtimecontract.RuntimeSemanticCorrectionDirective) error {
	if directive.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch {
		return nil
	}
	if len(directive.Violations) == 0 {
		return fmt.Errorf("%w: proof-batch correction requires bounded violations", common.ErrConflict)
	}
	for _, batch := range ledger.Batches {
		if batch.BatchID != ledger.ActiveBatchID {
			continue
		}
		batchCoverage := make(map[common.CoverageWorkItemID]struct{}, len(batch.CoverageWorkItemIDs))
		for _, id := range batch.CoverageWorkItemIDs {
			batchCoverage[id] = struct{}{}
		}
		for _, violation := range directive.Violations {
			switch violation.Kind {
			case runtimecontract.RuntimeSemanticContractViolationProofBatchDocument:
				required := violation.RequiredCoverageWorkItemIDs
				if len(required) != len(batch.CoverageWorkItemIDs) {
					return fmt.Errorf("%w: proof-batch correction coverage differs from active batch", common.ErrConflict)
				}
				for idx := range required {
					if required[idx] != batch.CoverageWorkItemIDs[idx] {
						return fmt.Errorf("%w: proof-batch correction coverage differs from active batch", common.ErrConflict)
					}
				}
			case runtimecontract.RuntimeSemanticContractViolationProposalConstraint:
				if _, found := batchCoverage[violation.CoverageWorkItemID]; !found {
					return fmt.Errorf("%w: proposal correction coverage is outside active batch", common.ErrConflict)
				}
			default:
				return fmt.Errorf("%w: proof-batch correction has unsupported violation", common.ErrConflict)
			}
		}
		return nil
	}
	return fmt.Errorf("%w: proof-batch correction has no active batch", common.ErrConflict)
}

// sealSemanticContract is the SM-2 all-or-nothing authority transition. It
// compiles binding/focus/reconciliation values from the persisted document and
// complete owner batch inside the same CAS as the semantic ledger seal. No
// caller may submit a binding hash or focus as external authority.
func (c *RuntimeController) sealSemanticContract(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
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
	focuses, _, err := runtimecontract.CompileRuntimeSemanticBindings(*ledger.Manifest, *ledger.Document, ledger.Feasibilities)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	reconciliation, err := semanticReconciliationContractAfterFocusBatch(aggregate.Facts, focuses)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	pendingFocuses, err := semanticPendingFocuses(aggregate.Facts, ledger.PendingCoverageIDs, focuses)
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
	aggregate.Facts.Focus = focuses[0]
	aggregate.Facts.RemainingFocuses = append(append([]runtimecontract.CoverageFocus(nil), focuses[1:]...), pendingFocuses...)
	aggregate.Facts.Capabilities = runtimeCapabilitiesWithSemanticProofs(aggregate.Facts.Capabilities, focuses)
	aggregate.Facts.ReconciliationContract = reconciliation
	aggregate.Facts.RepairDirective = nil
	aggregate.Facts.Feasibility = nil
	aggregate.Facts.Failure = nil
	aggregate.Facts = aggregate.Facts.Normalized()
	if err := resolveCoverageProofLineageForFocus(&aggregate, aggregate.Facts.Focus); err != nil {
		return RuntimeAggregate{}, err
	}
	previousFocusID := aggregate.Facts.Focus.ID
	aggregate.Facts = advanceSatisfiedCoverageFocus(aggregate.Facts, aggregate.CoverageProofLineage)
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
func semanticPendingFocuses(facts runtimecontract.RuntimeFacts, pending []common.CoverageWorkItemID, sealed []runtimecontract.CoverageFocus) ([]runtimecontract.CoverageFocus, error) {
	sealedCoverage := make(map[common.CoverageWorkItemID]struct{})
	for _, focus := range sealed {
		for _, id := range focus.CoverageWorkItemIDs {
			sealedCoverage[id] = struct{}{}
		}
	}
	sources := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus)
	for _, focus := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		for _, id := range focus.CoverageWorkItemIDs {
			if _, found := sources[id]; !found {
				sources[id] = focus.Normalized()
			}
		}
	}
	ret := make([]runtimecontract.CoverageFocus, 0, len(pending))
	for _, id := range pending {
		if _, alreadySealed := sealedCoverage[id]; alreadySealed {
			continue
		}
		source, found := sources[id]
		if !found {
			return nil, fmt.Errorf("runtime semantic pending coverage %q has no bootstrap focus", id)
		}
		placeholder := source.Normalized()
		placeholder.ID = "semantic-pending/" + source.ID + "/" + string(id)
		placeholder.CoverageWorkItemIDs = []common.CoverageWorkItemID{id}
		placeholder.CoverageEvidenceBindings = nil
		placeholder = placeholder.Normalized()
		ret = append(ret, placeholder)
	}
	return ret, nil
}

// semanticReconciliationContractAfterFocusBatch retains the immutable proof
// identities of already completed focuses while adding exactly the current
// proof-batch bindings. A batch is never allowed to restate or replace a
// completed CWI binding; duplicate coverage is rejected by the contract
// compiler. The returned value is audit/reconciliation identity only. SM-3
// admission still reads the current focus's exact binding set.
func semanticReconciliationContractAfterFocusBatch(facts runtimecontract.RuntimeFacts, focuses []runtimecontract.CoverageFocus) (runtimecontract.RuntimeCWIReconciliationContract, error) {
	all := make([]runtimecontract.CoverageFocus, 0, len(facts.CompletedFocuses)+len(focuses))
	for _, focus := range facts.CompletedFocuses {
		if len(focus.CoverageEvidenceBindings) != 0 {
			all = append(all, focus)
		}
	}
	all = append(all, focuses...)
	return runtimecontract.CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, all)
}

// prepareSemanticPlanningForAdvancedFocus turns a completed current-focus
// semantic seal back into the already accepted global-order phase when the
// next focus has not yet received a binding.  It is called only inside the
// same Orchestrator CAS that advanced SM-2 focus; it does not issue provider
// I/O, mutate a capability owner, or create another state machine.
func prepareSemanticPlanningForAdvancedFocus(aggregate *RuntimeAggregate) error {
	if aggregate == nil || len(aggregate.Facts.Focus.CoverageEvidenceBindings) != 0 {
		return nil
	}
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
		return nil
	}
	if ledger.Manifest == nil {
		return fmt.Errorf("%w: sealed semantic generation has no manifest", common.ErrConflict)
	}
	// A durable receipt may have retired one of the owner observations that
	// produced this sealed manifest. Re-acquisition can complete before focus
	// advancement, so comparing only the focus ID is insufficient: the old
	// order/batch must never be reopened against a new catalog generation.
	_, current := semanticPlanningCatalogsCurrent(*aggregate, *ledger.Manifest)
	if !current && aggregate.ActiveDispatch != nil {
		return fmt.Errorf("%w: semantic catalog is stale while an active dispatch still owns the sealed generation", common.ErrConflict)
	}
	retired, err := resetSemanticPlanningForInvalidatedCatalog(aggregate, runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated)
	if err != nil {
		return err
	}
	if !retired && !current {
		return fmt.Errorf("%w: semantic catalog is stale but its generation was not retired", common.ErrConflict)
	}
	if retired {
		return nil
	}
	ledger = aggregate.SemanticPlanning.Clone()
	if ledger.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
		return nil
	}
	if ledger.Manifest == nil || ledger.CoverageOrderDocument == nil || len(ledger.PendingCoverageIDs) == 0 {
		return fmt.Errorf("runtime semantic next focus has no retained coverage-order lineage")
	}
	currentPending := false
	for _, id := range aggregate.Facts.Focus.CoverageWorkItemIDs {
		for _, pending := range ledger.PendingCoverageIDs {
			if id == pending {
				currentPending = true
				break
			}
		}
	}
	if !currentPending {
		return fmt.Errorf("runtime semantic next focus is outside retained pending lineage")
	}
	order := ledger.CoverageOrderDocument.Clone()
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningOrderAccepted
	ledger.Attempt = 0
	ledger.CorrectionDirective = nil
	ledger.Document = &order
	ledger.DocumentHash = runtimecontract.StableRuntimeSemanticPlanDocumentHash(order)
	ledger.ActiveBatchID = ""
	ledger.MergedDocument = nil
	ledger.MergedDocumentHash = ""
	ledger.FeasibilityBatchID = ""
	ledger.Feasibilities = nil
	ledger.FeasibilitySetHash = ""
	ledger.SealedBindingSetHash = ""
	ledger.SealedLineageRefs = nil
	ledger.Rejections = nil
	ledger.Failure = nil
	aggregate.SemanticPlanning = ledger
	// The persisted contract remains the immutable audit/reconciliation record
	// for already sealed CWI bindings. It grants no authority to this unbound
	// placeholder: SM-3 admission remains fenced by its empty local binding set
	// and the non-sealed semantic ledger phase above.
	return nil
}

// runtimeCapabilitiesWithSemanticProofs projects only already-feasible,
// sealed proof kinds into the reducer capability view. This remains an SM-2
// internal derivation: capability owners supplied the catalogs and feasibility
// facts, while the orchestrator merely records which concrete proof operations
// may satisfy each convergence state.
func runtimeCapabilitiesWithSemanticProofs(capabilities runtimecontract.RuntimeCapabilities, focuses []runtimecontract.CoverageFocus) runtimecontract.RuntimeCapabilities {
	for _, focus := range focuses {
		for _, binding := range focus.CoverageEvidenceBindings {
			for _, proof := range binding.Proofs {
				switch proof.EvidenceKind {
				case runtimecontract.EvidenceKindStateChange:
					capabilities.StateChangeKinds = append(capabilities.StateChangeKinds, proof.AllowedOperationKinds...)
				case runtimecontract.EvidenceKindValidation:
					capabilities.ValidationKinds = append(capabilities.ValidationKinds, proof.AllowedOperationKinds...)
				}
			}
		}
	}
	return capabilities
}

// failSemanticContract is the only semantic terminal transition. It preserves
// the producer-authored diagnostic and never creates governance work.
func (c *RuntimeController) failSemanticContract(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, terminalCode string, failure runtimecontract.RuntimeOriginalDiagnostic) (RuntimeAggregate, error) {
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
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningFailed
	ledger.Failure = failure.CloneOriginal()
	if err := ledger.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.SemanticPlanning = ledger
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class:              semanticFailureClassForFailureCode(terminalCode, failure),
		Code:               "semantic_contract_failed",
		Signature:          failure.Signature,
		Message:            failure.OriginalMessage,
		OriginalDiagnostic: failure.CloneOriginal(),
	}
	aggregate.Facts, aggregate.Decision, err = ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || aggregate.Decision.Terminal == nil || aggregate.Decision.Terminal.Code != "semantic_contract_failed" {
		return RuntimeAggregate{}, fmt.Errorf("runtime semantic failure did not seal a terminal decision")
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

// semanticFailureClass preserves the historical code-only helper for tests and
// owner-local callers.  Production terminalization uses the full diagnostic so
// provider/owner classes are not erased when a semantic contract is sealed.
func semanticFailureClass(code string) runtimecontract.FailureClass {
	return semanticFailureClassForDiagnostic(runtimecontract.RuntimeOriginalDiagnostic{OriginalCode: code})
}

// semanticFailureClassForFailureCode lets the Orchestrator retain a bounded
// terminal outcome selected by its own persisted budget (for example refresh
// exhaustion) while preserving the complete producer diagnostic unchanged.
// The outer code is never copied into OriginalDiagnostic; it only supplies the
// stable class for these explicit owner-local terminal transitions.
func semanticFailureClassForFailureCode(code string, diagnostic runtimecontract.RuntimeOriginalDiagnostic) runtimecontract.FailureClass {
	code = strings.TrimSpace(code)
	switch code {
	case "semantic_catalog_refresh_exhausted":
		return runtimecontract.FailureClassPrerequisiteRefreshExhausted
	case "semantic_plan_retry_exhausted", "candidate_retry_exhausted":
		return runtimecontract.FailureClassCandidateExhausted
	case "semantic_document_recovery_exhausted", "candidate_document_recovery_exhausted":
		return runtimecontract.FailureClassCandidateDocumentRecoveryExhausted
	case "semantic_provider_submission_unknown", "provider_submission_unknown":
		return runtimecontract.FailureClassProviderSubmissionUnknown
	case "semantic_provider_body_idle_timeout", "semantic_provider_transport_exhausted",
		"provider_body_idle_timeout", "provider_transport_exhausted":
		return runtimecontract.FailureClassProviderTransient
	case "semantic_provider_authentication", "provider_authentication", "authentication_failed", "invalid_api_key":
		return runtimecontract.FailureClassProviderAuthentication
	case "semantic_provider_configuration", "provider_configuration", "provider_not_configured":
		return runtimecontract.FailureClassProviderConfiguration
	case "semantic_provider_contract_error", "semantic_provider_failure", "semantic_provider_outcome_missing",
		"semantic_provider_outcome_invalid", "semantic_provider_document_missing", "provider_protocol_failure",
		"provider_document_content_invalid":
		return runtimecontract.FailureClassProviderProtocol
	case "validation_recipe_exhausted", "validation_recipe_unavailable", "validation_recipe_not_sealed",
		"mcp_semantic_proof_unavailable", "mcp_recipe_not_admissible", "mcp_selector_not_admissible",
		"file_semantic_proof_unavailable", "git_semantic_proof_unavailable", "web_semantic_proof_unavailable",
		"ssh_semantic_proof_unavailable", "semantic_target_unavailable", "semantic_target_invalid",
		"semantic_target_not_regular", "prospective_target_exists", "prospective_target_unobservable",
		"semantic_owner_feasibility_infeasible":
		return runtimecontract.FailureClassCandidateInfeasible
	case "semantic_provider_ceiling_missing", "semantic_catalog_stale", "mcp_catalog_unavailable",
		"mcp_capability_catalog_unavailable", "web_catalog_unavailable", "git_repository_state_unavailable",
		"shell_policy_unavailable", "workspace_manifest_unavailable", "semantic_owner_catalog_missing":
		return runtimecontract.FailureClassCapabilityUnavailable
	case "runtime_contract_defect", "semantic_context_integrity", "semantic_manifest_invalid", "semantic_request_context_invalid",
		"semantic_expected_document_invalid", "semantic_request_identity_invalid", "semantic_request_session_mismatch",
		"semantic_request_terminal", "semantic_expected_document_missing", "semantic_correction_lineage_invalid",
		"semantic_feasibility_context_invalid", "semantic_batch_identity_missing", "semantic_batch_document_missing",
		"semantic_owner_probe_invalid":
		return runtimecontract.FailureClassRuntimeContractMisalignment
	}
	return semanticFailureClassForDiagnostic(diagnostic)
}

// semanticFailureClassForDiagnostic is the exhaustive SM-2 mapping for a
// semantic-planning terminal.  The semantic ledger has its own terminal code
// (semantic_contract_failed), but its stable failure class must retain the
// factual producer category.  In particular, a zero-attempt owner gap is not
// a no-progress exhaustion: that class is only produced by the reducer after
// a persisted repair/retry budget has actually been consumed.
func semanticFailureClassForDiagnostic(diagnostic runtimecontract.RuntimeOriginalDiagnostic) runtimecontract.FailureClass {
	code := strings.TrimSpace(diagnostic.OriginalCode)
	producerClass := strings.TrimSpace(diagnostic.OriginalClass)

	// LLM feedback carries the candidate/provider class in OriginalClass.  Keep
	// those classes even when the Orchestrator wraps the diagnostic in a
	// semantic terminal.
	switch runtimecontract.RuntimeCandidateFailureClass(producerClass) {
	case runtimecontract.RuntimeCandidateFailureClassProviderTransient:
		return runtimecontract.FailureClassProviderTransient
	case runtimecontract.RuntimeCandidateFailureClassProviderAuthentication:
		return runtimecontract.FailureClassProviderAuthentication
	case runtimecontract.RuntimeCandidateFailureClassProviderConfiguration:
		return runtimecontract.FailureClassProviderConfiguration
	case runtimecontract.RuntimeCandidateFailureClassProvider, runtimecontract.RuntimeCandidateFailureClassProviderProtocol:
		return runtimecontract.FailureClassProviderProtocol
	}

	// These are terminal outcomes of bounded semantic/provider sessions.  They
	// must remain distinct from runtime contract defects and no-progress.
	switch code {
	case "semantic_catalog_refresh_exhausted":
		return runtimecontract.FailureClassPrerequisiteRefreshExhausted
	case "semantic_plan_retry_exhausted", "candidate_retry_exhausted":
		return runtimecontract.FailureClassCandidateExhausted
	case "semantic_document_recovery_exhausted", "candidate_document_recovery_exhausted":
		return runtimecontract.FailureClassCandidateDocumentRecoveryExhausted
	case "semantic_provider_submission_unknown", "provider_submission_unknown":
		return runtimecontract.FailureClassProviderSubmissionUnknown
	case "semantic_provider_body_idle_timeout", "semantic_provider_transport_exhausted",
		"provider_body_idle_timeout", "provider_transport_exhausted":
		return runtimecontract.FailureClassProviderTransient
	case "semantic_provider_authentication", "provider_authentication", "authentication_failed", "invalid_api_key":
		return runtimecontract.FailureClassProviderAuthentication
	case "semantic_provider_configuration", "provider_configuration", "provider_not_configured":
		return runtimecontract.FailureClassProviderConfiguration
	case "semantic_provider_contract_error", "semantic_provider_failure", "semantic_provider_outcome_missing",
		"semantic_provider_outcome_invalid", "semantic_provider_document_missing", "provider_protocol_failure",
		"provider_document_content_invalid":
		return runtimecontract.FailureClassProviderProtocol
	}

	// Owner semantic feasibility statuses are represented by a small, closed
	// code vocabulary.  Capability/catalog gaps retain capability-unavailable;
	// an owner that cannot independently prove the requested recipe/target is a
	// candidate-infeasible fact.  Unknown owner codes fail closed as a runtime
	// contract defect rather than being guessed from free text.
	switch code {
	case "semantic_provider_ceiling_missing", "semantic_catalog_stale", "mcp_catalog_unavailable",
		"mcp_capability_catalog_unavailable", "web_catalog_unavailable", "git_repository_state_unavailable",
		"shell_policy_unavailable", "workspace_manifest_unavailable", "semantic_owner_catalog_missing",
		"semantic_owner_prerequisite_required", "semantic_owner_feasibility_stale":
		return runtimecontract.FailureClassCapabilityUnavailable
	case "validation_recipe_exhausted", "validation_recipe_unavailable", "validation_recipe_not_sealed",
		"mcp_semantic_proof_unavailable", "mcp_recipe_not_admissible", "mcp_selector_not_admissible",
		"file_semantic_proof_unavailable", "git_semantic_proof_unavailable", "web_semantic_proof_unavailable",
		"ssh_semantic_proof_unavailable", "semantic_target_unavailable", "semantic_target_invalid",
		"semantic_target_not_regular", "prospective_target_exists", "prospective_target_unobservable",
		"semantic_owner_feasibility_infeasible":
		return runtimecontract.FailureClassCandidateInfeasible
	case "mcp_recipe_invalid", "semantic_owner_probe_invalid":
		return runtimecontract.FailureClassRuntimeContractMisalignment
	}

	// A typed owner feasibility diagnostic with the generic semantic class is
	// still an infeasibility only when the owner used the known semantic status
	// marker.  Otherwise preserve fail-closed contract-defect semantics.
	if producerClass == "semantic_feasibility" && code == "semantic_proof_infeasible" {
		return runtimecontract.FailureClassCandidateInfeasible
	}
	return runtimecontract.FailureClassRuntimeContractMisalignment
}
