package runtime

import (
	"context"
	"fmt"
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
		facts, prerequisiteIDs, observationIDs, invalidatedDependencies, err = invalidatePrerequisiteObservations(facts, []runtimecontract.RuntimePrerequisiteObservation{observation})
		if err != nil {
			return RuntimeAggregate{}, err
		}
	}
	recovery, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if !consumed {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog refresh budget exhausted", common.ErrConflict)
	}
	facts.Recovery = recovery

	diagnostic := gapFact.OriginalDiagnostic.CloneOriginal()
	invalidation := runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature:       fmt.Sprintf("semantic-catalog-refresh/%d/%s/%s", refreshGeneration, feasibilityBatchID, ledger.FeasibilitySetHash),
		PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs,
		InvalidatedDependencies: invalidatedDependencies, Stale: stale,
	}
	if stale == nil {
		invalidation.RefreshDiagnostic = diagnostic
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
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningRequired
	ledger.Manifest = nil
	ledger.Attempt = 0
	ledger.StablePrefixHash = ""
	ledger.LogicalRequestID = ""
	ledger.CorrectionDirective = nil
	ledger.Document = nil
	ledger.DocumentHash = ""
	ledger.FeasibilityBatchID = ""
	ledger.Feasibilities = nil
	ledger.FeasibilitySetHash = ""
	ledger.Rejections = nil
	if err := ledger.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.SemanticPlanning = ledger
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, invalidation)
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
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if ledger.Manifest == nil || ledger.Document == nil || ledger.Document.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
			return false, fmt.Errorf("%w: semantic batch requires an accepted coverage order", common.ErrConflict)
		}
		if err := batch.ValidateAgainstManifest(*ledger.Manifest); err != nil {
			return false, err
		}
		batch, err := runtimecontract.AdvanceRuntimeSemanticPlanBatch(batch, runtimecontract.RuntimeSemanticPlanBatchRequesting)
		if err != nil {
			return false, err
		}
		for _, existing := range ledger.Batches {
			if existing.Hash == batch.Hash {
				return false, nil
			}
			if existing.Sequence == batch.Sequence {
				return false, fmt.Errorf("%w: semantic batch sequence already exists", common.ErrConflict)
			}
		}
		ledger.Batches = append(ledger.Batches, batch.Clone())
		ledger.ActiveBatchID = batch.BatchID
		ledger.LogicalRequestID = batch.RequestLogicalID
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
		if ledger.NextBatchSequence <= batch.Sequence {
			ledger.NextBatchSequence = batch.Sequence + 1
		}
		ledger.PendingCoverageIDs = append([]common.CoverageWorkItemID(nil), batch.PendingCoverageWorkItemIDs...)
		return true, nil
	})
}

func (c *RuntimeController) advanceSemanticPlanBatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, identity runtimecontract.RuntimeSemanticPlanBatch, next runtimecontract.RuntimeSemanticPlanBatchState) (RuntimeAggregate, error) {
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		for idx := range ledger.Batches {
			if ledger.Batches[idx].BatchID != identity.BatchID {
				continue
			}
			if ledger.Batches[idx].Hash == identity.Hash && ledger.Batches[idx].State == next {
				return false, nil
			}
			if ledger.Batches[idx].Hash != identity.Hash {
				return false, fmt.Errorf("%w: semantic batch identity is stale", common.ErrConflict)
			}
			advanced, err := runtimecontract.AdvanceRuntimeSemanticPlanBatch(ledger.Batches[idx], next)
			if err != nil {
				return false, err
			}
			ledger.Batches[idx] = advanced
			if next == runtimecontract.RuntimeSemanticPlanBatchAccepted {
				ledger.CompletedBatchIDs = append(ledger.CompletedBatchIDs, identity.BatchID)
			}
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
		hash := runtimecontract.StableRuntimeSemanticPlanDocumentHash(merged)
		if ledger.MergedDocumentHash == hash {
			return false, nil
		}
		if ledger.Manifest == nil || ledger.Document == nil || ledger.Document.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
			return false, fmt.Errorf("%w: semantic merge has no coverage order", common.ErrConflict)
		}
		if err := merged.ValidateAgainstManifest(*ledger.Manifest); err != nil {
			return false, err
		}
		if merged.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch || merged.BatchID != "" {
			return false, fmt.Errorf("%w: merged semantic document has invalid batch identity", common.ErrConflict)
		}
		if ledger.MergedDocumentHash != "" {
			return false, fmt.Errorf("%w: conflicting merged semantic document", common.ErrConflict)
		}
		value := merged.Clone()
		ledger.MergedDocument = &value
		ledger.MergedDocumentHash = hash
		ledger.Document = &value
		ledger.DocumentHash = hash
		ledger.FeasibilityBatchID = runtimecontract.StableRuntimeSemanticFeasibilityBatchID(*ledger.Manifest, value)
		ledger.ActiveBatchID = ""
		ledger.PendingCoverageIDs = nil
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
		if value.Status == runtimecontract.RuntimeSemanticFeasibilityStale || value.Status == runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired {
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
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		documentHash := runtimecontract.StableRuntimeSemanticPlanDocumentHash(document)
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
		if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
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
			ledger.Phase = runtimecontract.RuntimeSemanticPlanningProposalAccepted
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
		if (ledger.Phase != runtimecontract.RuntimeSemanticPlanningProposalAccepted && ledger.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted) || ledger.Manifest == nil || ledger.Document == nil {
			return false, fmt.Errorf("%w: semantic feasibility has no accepted proposal", common.ErrConflict)
		}
		ledger.Phase = runtimecontract.RuntimeSemanticPlanningFeasibilityRequired
		feasibilityDocument := ledger.Document
		if ledger.Phase == runtimecontract.RuntimeSemanticPlanningFeasibilityRequired && ledger.ActiveBatchID != "" {
			for idx := range ledger.BatchDocuments {
				if ledger.BatchDocuments[idx].BatchID == ledger.ActiveBatchID {
					value := ledger.BatchDocuments[idx].Clone()
					feasibilityDocument = &value
					break
				}
			}
		}
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
		if feasibilityDocument == nil {
			return false, fmt.Errorf("%w: semantic feasibility document is missing", common.ErrConflict)
		}
		ledger.FeasibilityBatchID = runtimecontract.StableRuntimeSemanticFeasibilityBatchID(*ledger.Manifest, *feasibilityDocument)
		return true, nil
	})
}

// observeSemanticFeasibility persists one canonical owner's factual result.
// Conflicting duplicates are rejected instead of replacing owner history.
func (c *RuntimeController) observeSemanticFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, batchID string, observation runtimecontract.RuntimeSemanticProofFeasibility) (RuntimeAggregate, error) {
	batchID = strings.TrimSpace(batchID)
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if ledger.Phase != runtimecontract.RuntimeSemanticPlanningFeasibilityRequired || ledger.Manifest == nil || ledger.Document == nil || ledger.FeasibilityBatchID != batchID {
			return false, fmt.Errorf("%w: semantic feasibility batch is stale", common.ErrConflict)
		}
		feasibilityDocument := ledger.Document
		if ledger.ActiveBatchID != "" {
			for idx := range ledger.BatchDocuments {
				if ledger.BatchDocuments[idx].BatchID == ledger.ActiveBatchID {
					value := ledger.BatchDocuments[idx].Clone()
					feasibilityDocument = &value
					break
				}
			}
		}
		if feasibilityDocument == nil {
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
		for _, existing := range ledger.Feasibilities {
			if existing.CoverageWorkItemID != observation.CoverageWorkItemID {
				continue
			}
			if runtimecontract.StableRuntimeSemanticFeasibilitySetHash([]runtimecontract.RuntimeSemanticProofFeasibility{existing}) == runtimecontract.StableRuntimeSemanticFeasibilitySetHash([]runtimecontract.RuntimeSemanticProofFeasibility{observation}) {
				return false, nil
			}
			return false, fmt.Errorf("%w: conflicting semantic feasibility observation", common.ErrConflict)
		}
		ledger.Feasibilities = append(ledger.Feasibilities, observation.Clone())
		ledger.FeasibilitySetHash = runtimecontract.StableRuntimeSemanticFeasibilitySetHash(ledger.Feasibilities)
		if len(ledger.Feasibilities) == len(feasibilityDocument.Proposals) && ledger.Phase == runtimecontract.RuntimeSemanticPlanningFeasibilityRequired {
			// Only the proof-batch path owns the additional batch-complete
			// phase.  The legacy full-document semantic path remains in
			// feasibility_required until the existing atomic seal event; it has
			// no batch lifecycle to advance.
			if feasibilityDocument.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch && ledger.ActiveBatchID != "" {
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
						ledger.CompletedBatchIDs = append(ledger.CompletedBatchIDs, advanced.BatchID)
						break
					}
				}
			}
		}
		return true, nil
	})
}

// requireSemanticPlanCorrection records the complete attempt-0 rejection and
// opens exactly one correction identity under the same manifest/prefix.
func (c *RuntimeController) requireSemanticPlanCorrection(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stablePrefixHash, logicalRequestID string, directive runtimecontract.RuntimeSemanticCorrectionDirective, rejection runtimecontract.RuntimeOriginalDiagnostic) (RuntimeAggregate, error) {
	stablePrefixHash = strings.TrimSpace(stablePrefixHash)
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	directive = directive.Clone()
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
		if err := directive.ValidateAgainst(*ledger.Manifest, rejection); err != nil {
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
	if aggregate.ActiveDispatch != nil || len(aggregate.CompletedDispatches) != 0 || len(aggregate.Facts.Evidence.Accepted) != 0 || len(aggregate.Facts.Evidence.Rejected) != 0 {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic contract cannot seal after operation or evidence activity", common.ErrConflict)
	}
	focuses, reconciliation, err := runtimecontract.CompileRuntimeSemanticBindings(*ledger.Manifest, *ledger.Document, ledger.Feasibilities)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	lineage := []string{ledger.Manifest.Hash, ledger.DocumentHash, ledger.FeasibilityBatchID, ledger.FeasibilitySetHash, reconciliation.BindingSetHash}
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningSealed
	ledger.SealedBindingSetHash = reconciliation.BindingSetHash
	ledger.SealedLineageRefs = lineage
	if err := ledger.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.SemanticPlanning = ledger
	aggregate.Facts.Focus = focuses[0]
	aggregate.Facts.RemainingFocuses = append([]runtimecontract.CoverageFocus(nil), focuses[1:]...)
	aggregate.Facts.CompletedFocuses = nil
	aggregate.Facts.ReconciliationContract = reconciliation
	aggregate.Facts.RepairDirective = nil
	aggregate.Facts.Feasibility = nil
	aggregate.Facts.Failure = nil
	aggregate.Facts = aggregate.Facts.Normalized()
	aggregate.Facts, aggregate.Decision, err = ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
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

// failSemanticContract is the only semantic terminal transition. It preserves
// the producer-authored diagnostic and never creates governance work.
func (c *RuntimeController) failSemanticContract(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, failure runtimecontract.RuntimeOriginalDiagnostic) (RuntimeAggregate, error) {
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
		Class: runtimecontract.FailureClassNoProgressExhausted, Code: "semantic_contract_failed",
		Signature: failure.Signature, Message: failure.OriginalMessage,
	}
	aggregate.Facts, aggregate.Decision, err = ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
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
