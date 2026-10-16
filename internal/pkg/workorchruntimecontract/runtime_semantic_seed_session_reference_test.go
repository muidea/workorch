package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Complete old ledger/generation call order; seed gate independently re-runs.
func referenceSemanticGenerationBeforeSeedSession(raw, g RuntimeSemanticPlanningGeneration, session *runtimeSemanticValidationSession) error {
	if len(raw.InvalidationSignatures) != len(g.InvalidationSignatures) || len(raw.PrerequisiteIDs) != len(g.PrerequisiteIDs) ||
		len(raw.ReopenedCoverageIDs) != len(g.ReopenedCoverageIDs) || len(raw.ReopenTriggerRefs) != len(g.ReopenTriggerRefs) ||
		len(raw.ChangedCoverageIDs) != len(g.ChangedCoverageIDs) || len(raw.ClosureChangeTriggerRefs) != len(g.ClosureChangeTriggerRefs) {
		return fmt.Errorf("runtime semantic planning generation contains duplicate or empty lineage values")
	}
	if g.RetiredAtStateRevision == 0 || g.Reason == "" {
		return fmt.Errorf("runtime semantic planning generation requires retirement identity and reason")
	}
	switch g.Reason {
	case RuntimeSemanticPlanningRetirementImplementationPurpose:
		if len(g.InvalidationSignatures) != 0 || len(g.PrerequisiteIDs) != 0 || len(g.ReopenedCoverageIDs) != 0 || len(g.ReopenTriggerRefs) != 0 || len(g.ChangedCoverageIDs) != 0 || len(g.ClosureChangeTriggerRefs) != 0 || g.Snapshot.Phase != RuntimeSemanticPlanningOrderAccepted || g.Snapshot.CoverageOrderDocument == nil || g.Snapshot.CoverageOrderDocument.ImplementationPurpose == "" {
			return fmt.Errorf("implementation purpose retirement requires exact original accepted order only")
		}
	case RuntimeSemanticPlanningRetirementInputReviewInformation:
		if g.InputReviewResolutionID == "" || len(g.InvalidationSignatures) != 0 || len(g.PrerequisiteIDs) != 0 || len(g.ReopenedCoverageIDs) != 0 || len(g.ReopenTriggerRefs) != 0 || len(g.ChangedCoverageIDs) != 0 || len(g.ClosureChangeTriggerRefs) != 0 {
			return fmt.Errorf("runtime input review retirement requires only exact information resolution lineage")
		}
	case RuntimeSemanticPlanningRetirementResourceInvalidated, RuntimeSemanticPlanningRetirementCatalogRefresh:
		if len(g.InvalidationSignatures) == 0 || len(g.PrerequisiteIDs) == 0 || len(g.ReopenedCoverageIDs) != 0 || len(g.ReopenTriggerRefs) != 0 || len(g.ChangedCoverageIDs) != 0 || len(g.ClosureChangeTriggerRefs) != 0 {
			return fmt.Errorf("runtime semantic catalog retirement requires only catalog lineage")
		}
	case RuntimeSemanticPlanningRetirementCWIReopened, RuntimeSemanticPlanningRetirementBindingContradiction:
		if len(g.InvalidationSignatures) != 0 || len(g.PrerequisiteIDs) != 0 || len(g.ReopenedCoverageIDs) == 0 || len(g.ReopenTriggerRefs) == 0 || len(g.ChangedCoverageIDs) != 0 || len(g.ClosureChangeTriggerRefs) != 0 {
			return fmt.Errorf("runtime semantic CWI reopen retirement requires only CWI lineage")
		}
	case RuntimeSemanticPlanningRetirementEligibleClosureChanged:
		if len(g.InvalidationSignatures) != 0 || len(g.PrerequisiteIDs) != 0 || len(g.ReopenedCoverageIDs) != 0 || len(g.ReopenTriggerRefs) != 0 || len(g.ChangedCoverageIDs) == 0 || len(g.ClosureChangeTriggerRefs) == 0 {
			return fmt.Errorf("runtime semantic eligible-closure retirement requires only closure-change lineage")
		}
	default:
		return fmt.Errorf("runtime semantic planning generation has unsupported retirement reason %q", g.Reason)
	}
	if g.Reason != RuntimeSemanticPlanningRetirementInputReviewInformation && g.InputReviewResolutionID != "" {
		return fmt.Errorf("runtime non-review retirement cannot carry an information resolution")
	}
	if g.Snapshot.History != nil {
		return fmt.Errorf("runtime semantic planning generation snapshot must not contain nested history")
	}
	if g.Snapshot.Phase == RuntimeSemanticPlanningRequired || g.Snapshot.Phase == RuntimeSemanticPlanningFailed {
		return fmt.Errorf("runtime semantic planning generation snapshot is not an active semantic generation")
	}
	if g.Snapshot.Manifest == nil {
		return fmt.Errorf("runtime semantic planning generation snapshot requires a manifest")
	}
	if err := referenceSemanticLedgerBeforeSeedSession(raw.Snapshot, g.Snapshot, session); err != nil {
		return fmt.Errorf("runtime semantic planning generation snapshot: %w", err)
	}
	manifestCatalogs := append([]RuntimeOperationAdmissibility(nil), g.Snapshot.Manifest.OwnerCatalogs...)
	manifestCatalogs = append(manifestCatalogs, g.Snapshot.Manifest.OwnerFactDependencyCatalogs...)
	manifestPrerequisites := make(map[string]struct{}, len(manifestCatalogs))
	for _, catalog := range manifestCatalogs {
		manifestPrerequisites[catalog.PrerequisiteID] = struct{}{}
	}
	seenPrerequisites := make(map[string]struct{}, len(g.PrerequisiteIDs))
	for _, prerequisiteID := range g.PrerequisiteIDs {
		if _, duplicate := seenPrerequisites[prerequisiteID]; duplicate {
			return fmt.Errorf("runtime semantic planning generation duplicates prerequisite %q", prerequisiteID)
		}
		seenPrerequisites[prerequisiteID] = struct{}{}
		if _, found := manifestPrerequisites[prerequisiteID]; !found {
			return fmt.Errorf("runtime semantic planning generation prerequisite %q is outside snapshot manifest", prerequisiteID)
		}
	}
	if g.Reason == RuntimeSemanticPlanningRetirementCWIReopened || g.Reason == RuntimeSemanticPlanningRetirementBindingContradiction || g.Reason == RuntimeSemanticPlanningRetirementEligibleClosureChanged {
		seedCoverage := make(map[common.CoverageWorkItemID]struct{}, len(g.Snapshot.DefinitionSeed.CoverageItems))
		for _, item := range g.Snapshot.DefinitionSeed.CoverageItems {
			seedCoverage[item.ID] = struct{}{}
		}
		for _, coverageID := range g.ReopenedCoverageIDs {
			if _, found := seedCoverage[coverageID]; !found {
				return fmt.Errorf("runtime semantic CWI reopen %q is outside snapshot definition seed", coverageID)
			}
		}
		for _, coverageID := range g.ChangedCoverageIDs {
			if _, found := seedCoverage[coverageID]; !found {
				return fmt.Errorf("runtime semantic closure change %q is outside snapshot definition seed", coverageID)
			}
		}
	}
	return nil
}

func referenceSemanticLedgerBeforeSeedSession(raw, l RuntimeSemanticPlanningLedger, session *runtimeSemanticValidationSession) error {
	rawHistory := append([]RuntimeSemanticPlanningGeneration(nil), raw.History...)
	if l.SchemaVersion != RuntimeSemanticPlanningLedgerSchemaCurrent {
		return fmt.Errorf("runtime semantic planning ledger requires current schema")
	}
	// Ledger collections are identity-bearing history, not presentation lists.
	// Normalization may remove duplicate/empty values for read models, but a
	// persisted authority ledger must reject that input rather than silently
	// changing its lineage while validating it.
	if len(raw.PendingCoverageIDs) != len(l.PendingCoverageIDs) || len(raw.CompletedBatchIDs) != len(l.CompletedBatchIDs) ||
		len(raw.Batches) != len(l.Batches) || len(raw.BatchDocuments) != len(l.BatchDocuments) || len(raw.BatchFeasibilityResults) != len(l.BatchFeasibilityResults) ||
		len(raw.History) != len(l.History) {
		return fmt.Errorf("runtime semantic planning ledger contains duplicate or empty identity history")
	}
	if len(l.History) > RuntimeSemanticPlanningHistoryLimit {
		return fmt.Errorf("runtime semantic planning history exceeds bounded limit %d", RuntimeSemanticPlanningHistoryLimit)
	}
	type generationIdentity struct {
		manifestHash     string
		logicalRequestID string
	}
	seenHistory := make(map[generationIdentity]struct{}, len(l.History))
	seenInvalidationSignatures := make(map[string]struct{})
	lastRetiredRevision := uint64(0)
	for idx := range l.History {
		if idx >= len(rawHistory) || len(rawHistory[idx].InvalidationSignatures) != len(l.History[idx].InvalidationSignatures) || len(rawHistory[idx].PrerequisiteIDs) != len(l.History[idx].PrerequisiteIDs) ||
			len(rawHistory[idx].ReopenedCoverageIDs) != len(l.History[idx].ReopenedCoverageIDs) || len(rawHistory[idx].ReopenTriggerRefs) != len(l.History[idx].ReopenTriggerRefs) ||
			len(rawHistory[idx].ChangedCoverageIDs) != len(l.History[idx].ChangedCoverageIDs) || len(rawHistory[idx].ClosureChangeTriggerRefs) != len(l.History[idx].ClosureChangeTriggerRefs) {
			return fmt.Errorf("runtime semantic planning history %d contains duplicate or empty lineage values", idx)
		}
		history := l.History[idx]
		if err := referenceSemanticGenerationBeforeSeedSession(rawHistory[idx], history, session); err != nil {
			return fmt.Errorf("runtime semantic planning history %d: %w", idx, err)
		}
		if history.Snapshot.DefinitionSeed.Hash != l.DefinitionSeed.Hash {
			return fmt.Errorf("runtime semantic planning history %d definition seed mismatch", idx)
		}
		if lastRetiredRevision != 0 && history.RetiredAtStateRevision <= lastRetiredRevision {
			return fmt.Errorf("runtime semantic planning history is not append ordered")
		}
		lastRetiredRevision = history.RetiredAtStateRevision
		for _, signature := range history.InvalidationSignatures {
			if _, duplicate := seenInvalidationSignatures[signature]; duplicate {
				return fmt.Errorf("runtime semantic planning history duplicates invalidation signature %q", signature)
			}
			seenInvalidationSignatures[signature] = struct{}{}
		}
		// A document, batch, or phase change under the same sealed catalog and
		// logical request is still one generation and must not be appended as a
		// second history entry.  A later request with the same catalog hash is
		// allowed only when it has a distinct logical identity (resource rollback
		// recovery); it cannot reuse the old snapshot's request/session lineage.
		identity := generationIdentity{manifestHash: history.Snapshot.Manifest.Hash, logicalRequestID: history.Snapshot.LogicalRequestID}
		if _, duplicate := seenHistory[identity]; duplicate {
			return fmt.Errorf("runtime semantic planning history duplicates generation %q", identity)
		}
		seenHistory[identity] = struct{}{}
	}
	for idx := range raw.Batches {
		rawBatch, normalizedBatch := raw.Batches[idx], l.Batches[idx]
		if len(rawBatch.CoverageWorkItemIDs) != len(normalizedBatch.CoverageWorkItemIDs) ||
			len(rawBatch.PendingCoverageWorkItemIDs) != len(normalizedBatch.PendingCoverageWorkItemIDs) ||
			len(rawBatch.DependencyBarrierRefs) != len(normalizedBatch.DependencyBarrierRefs) {
			return fmt.Errorf("runtime semantic planning batch %d contains duplicate or empty identity values", idx)
		}
	}
	if err := l.DefinitionSeed.Validate(); err != nil {
		return err
	}
	var validatedManifest *runtimeValidatedSemanticManifest
	if raw.Manifest != nil {
		validated, err := validateRuntimeSemanticPlanManifestInSession(*raw.Manifest, session)
		if err != nil {
			return err
		}
		validatedManifest = &validated
		l.Manifest = &validated.manifest
	}
	if l.CoverageOrderDocument != nil {
		if l.Manifest == nil {
			return fmt.Errorf("runtime semantic coverage order has no manifest")
		}
		order := l.CoverageOrderDocument.Clone()
		if order.PlanStep != RuntimeSemanticPlanStepCoverageOrder || order.BatchID != "" ||
			order.ManifestHash != l.Manifest.Hash || order.DefinitionSeedHash != l.Manifest.DefinitionSeedHash {
			return fmt.Errorf("runtime semantic coverage order lineage does not match manifest")
		}
		if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(order, *validatedManifest); err != nil {
			return err
		}
	}
	if len(l.PendingCoverageIDs) != 0 {
		if l.CoverageOrderDocument == nil {
			return fmt.Errorf("runtime semantic pending coverage has no immutable coverage order")
		}
		ordered := make(map[common.CoverageWorkItemID]struct{}, len(l.CoverageOrderDocument.CoverageOrder))
		for _, id := range l.CoverageOrderDocument.CoverageOrder {
			ordered[id] = struct{}{}
		}
		for _, id := range l.PendingCoverageIDs {
			if _, found := ordered[id]; !found {
				return fmt.Errorf("runtime semantic pending coverage %q is outside immutable coverage order", id)
			}
		}
	}
	if l.NextBatchSequence < 0 || l.NextBatchSequence != len(l.Batches) || (l.ActiveBatchID != "" && len(l.Batches) == 0) {
		return fmt.Errorf("runtime semantic planning batch cursor is invalid")
	}
	if len(l.Batches) != 0 && l.CoverageOrderDocument == nil {
		return fmt.Errorf("runtime semantic planning batches require immutable coverage order")
	}
	batchIDs := make(map[string]struct{}, len(l.Batches))
	batchSequences := make(map[int]struct{}, len(l.Batches))
	maxBatchSequence := -1
	for idx := range l.Batches {
		batch := l.Batches[idx]
		if batch.BatchID == "" {
			return fmt.Errorf("runtime semantic planning batch %d has empty identity", idx)
		}
		if _, duplicate := batchIDs[batch.BatchID]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates batch %q", batch.BatchID)
		}
		if _, duplicate := batchSequences[batch.Sequence]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates batch sequence %d", batch.Sequence)
		}
		if batch.Sequence != idx {
			return fmt.Errorf("runtime semantic planning batch sequence %d is not contiguous at index %d", batch.Sequence, idx)
		}
		batchIDs[batch.BatchID] = struct{}{}
		batchSequences[batch.Sequence] = struct{}{}
		if batch.Sequence > maxBatchSequence {
			maxBatchSequence = batch.Sequence
		}
		if l.Manifest != nil {
			if err := validateRuntimeSemanticPlanBatchAgainstValidatedManifest(batch, *validatedManifest); err != nil {
				return err
			}
		}
		if l.CoverageOrderDocument != nil && batch.CoverageOrderHash != StableRuntimeSemanticPlanCoverageOrderHash(*l.CoverageOrderDocument) {
			return fmt.Errorf("runtime semantic batch %q is not bound to the immutable coverage order", batch.BatchID)
		}
	}
	if maxBatchSequence >= 0 && l.NextBatchSequence != maxBatchSequence+1 {
		return fmt.Errorf("runtime semantic planning next batch sequence is not contiguous with history")
	}
	if l.CoverageOrderDocument != nil {
		orderIDs := append([]common.CoverageWorkItemID(nil), l.CoverageOrderDocument.CoverageOrder...)
		pendingBefore := orderIDs
		items := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(l.Manifest.CoverageItems))
		for _, item := range l.Manifest.CoverageItems {
			items[item.ID] = item
		}
		for _, batch := range l.Batches {
			selected := batch.CoverageWorkItemIDs
			selectedSet := make(map[common.CoverageWorkItemID]struct{}, len(selected))
			for _, id := range selected {
				if _, duplicate := selectedSet[id]; duplicate {
					return fmt.Errorf("runtime semantic batch %q duplicates selected coverage %q", batch.BatchID, id)
				}
				selectedSet[id] = struct{}{}
			}
			expectedPending := subtractOrderedRuntimeCoverageIDs(pendingBefore, selectedSet)
			if len(expectedPending)+len(selected) != len(pendingBefore) ||
				!sameOrderedRuntimeCoverageIDs(batch.PendingCoverageWorkItemIDs, expectedPending) {
				return fmt.Errorf("runtime semantic batch %q does not form an exact selected/pending partition", batch.BatchID)
			}
			expectedBarriers := runtimeSemanticPendingDependencyBarriers(items, expectedPending, selectedSet)
			if !sameRuntimeStringSet(batch.DependencyBarrierRefs, expectedBarriers) {
				return fmt.Errorf("runtime semantic batch %q dependency barriers do not match pending lineage", batch.BatchID)
			}
			pendingBefore = expectedPending
		}
		if !sameOrderedRuntimeCoverageIDs(l.PendingCoverageIDs, pendingBefore) {
			return fmt.Errorf("runtime semantic pending coverage does not match the latest batch lineage")
		}
	}
	batchDocuments := make(map[string]RuntimeSemanticPlanDocument, len(l.BatchDocuments))
	for _, document := range l.BatchDocuments {
		if _, duplicate := batchDocuments[document.BatchID]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates batch document %q", document.BatchID)
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, document.BatchID)
		if !found {
			return fmt.Errorf("runtime semantic batch document has no matching batch")
		}
		if document.PlanStep != RuntimeSemanticPlanStepProofBatch || document.BatchSequence != batch.Sequence ||
			!sameOrderedRuntimeCoverageIDs(document.BatchCoverageWorkItemIDs, batch.CoverageWorkItemIDs) ||
			batch.DocumentHash != StableRuntimeSemanticPlanDocumentHash(document) {
			return fmt.Errorf("runtime semantic batch document identity is not bound to batch")
		}
		if l.Manifest == nil {
			return fmt.Errorf("runtime semantic batch document has no manifest")
		}
		if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, *validatedManifest); err != nil {
			return err
		}
		correctionBaseline := (l.Phase == RuntimeSemanticPlanningCorrectionRequired || l.Phase == RuntimeSemanticPlanningFailed) && l.Attempt == 1 &&
			document.BatchID == l.ActiveBatchID && batch.State == RuntimeSemanticPlanBatchRequesting
		if batch.State == RuntimeSemanticPlanBatchPending || (batch.State == RuntimeSemanticPlanBatchRequesting && !correctionBaseline) || batch.State == RuntimeSemanticPlanBatchFailed {
			return fmt.Errorf("runtime semantic batch document %q is not in a document-bearing batch state", document.BatchID)
		}
		batchDocuments[document.BatchID] = document.Clone()
	}
	if l.ActiveBatchID != "" {
		if _, found := batchIDs[l.ActiveBatchID]; !found {
			return fmt.Errorf("runtime semantic planning active batch is missing")
		}
		if len(l.Batches) == 0 || l.Batches[len(l.Batches)-1].BatchID != l.ActiveBatchID {
			return fmt.Errorf("runtime semantic planning active batch is not the latest batch")
		}
	}
	seenBatchResults := make(map[string]struct{}, len(l.BatchFeasibilityResults))
	for _, result := range l.BatchFeasibilityResults {
		if l.Manifest == nil {
			return fmt.Errorf("runtime semantic planning batch feasibility has no manifest")
		}
		if _, duplicate := seenBatchResults[result.BatchID]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates batch feasibility %q", result.BatchID)
		}
		seenBatchResults[result.BatchID] = struct{}{}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, result.BatchID)
		if !found {
			return fmt.Errorf("runtime semantic planning batch feasibility has no matching batch")
		}
		document, found := batchDocuments[result.BatchID]
		if !found {
			return fmt.Errorf("runtime semantic planning batch feasibility has no matching document")
		}
		if err := validateRuntimeSemanticPlanBatchFeasibilityResultWithValidatedManifest(result, batch, document, *validatedManifest); err != nil {
			return err
		}
	}
	completed := make(map[string]struct{}, len(l.CompletedBatchIDs))
	for idx, id := range l.CompletedBatchIDs {
		if _, duplicate := completed[id]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates completed batch %q", id)
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, id)
		if !found || batch.State != RuntimeSemanticPlanBatchAccepted {
			return fmt.Errorf("runtime semantic completed batch %q is not an accepted batch", id)
		}
		if idx >= len(l.Batches) || l.Batches[idx].BatchID != id {
			return fmt.Errorf("runtime semantic completed batches are not a contiguous prefix")
		}
		if id == l.ActiveBatchID {
			return fmt.Errorf("runtime semantic completed batch %q is still active", id)
		}
		completed[id] = struct{}{}
	}
	if l.CoverageOrderDocument != nil {
		orderHash := StableRuntimeSemanticPlanCoverageOrderHash(*l.CoverageOrderDocument)
		for _, batch := range l.Batches {
			if batch.CoverageOrderHash != orderHash {
				return fmt.Errorf("runtime semantic batch %q has stale coverage-order identity", batch.BatchID)
			}
		}
	}
	orderIDs := runtimeSemanticPlanningCoverageOrder(l.CoverageOrderDocument)
	if l.CoverageOrderDocument != nil {
		if err := validateRuntimeSemanticPendingCoverage(l.PendingCoverageIDs, orderIDs); err != nil {
			return err
		}
		completedCoverage := make(map[common.CoverageWorkItemID]struct{})
		for id := range completed {
			batch, _ := runtimeSemanticPlanningBatch(l.Batches, id)
			for _, coverageID := range batch.CoverageWorkItemIDs {
				completedCoverage[coverageID] = struct{}{}
			}
		}
		for _, coverageID := range l.PendingCoverageIDs {
			if _, done := completedCoverage[coverageID]; done {
				return fmt.Errorf("runtime semantic pending coverage %q belongs to a completed batch", coverageID)
			}
		}
	}
	if l.MergedDocument != nil {
		if l.MergedDocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.MergedDocument) {
			return fmt.Errorf("runtime semantic merged document hash does not match contents")
		}
		if l.MergedDocument.PlanStep != RuntimeSemanticPlanStepProofBatch || l.MergedDocument.BatchID == "" {
			return fmt.Errorf("runtime semantic merged document must be a focus proof batch")
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, l.MergedDocument.BatchID)
		if !found || batch.State != RuntimeSemanticPlanBatchAccepted || batch.DocumentHash != l.MergedDocumentHash {
			return fmt.Errorf("runtime semantic merged document is not bound to an accepted batch")
		}
		if document, found := batchDocuments[l.MergedDocument.BatchID]; !found || StableRuntimeSemanticPlanDocumentHash(document) != l.MergedDocumentHash {
			return fmt.Errorf("runtime semantic merged document has no matching batch document")
		}
	}
	for idx := range l.Rejections {
		if err := l.Rejections[idx].Validate(); err != nil {
			return fmt.Errorf("runtime semantic planning rejection %d: %w", idx, err)
		}
	}
	if l.CorrectionDirective != nil {
		if l.Attempt != 1 || len(l.Rejections) != 1 || l.Manifest == nil {
			return fmt.Errorf("runtime semantic correction directive has no exact attempt-0 lineage")
		}
		if err := l.CorrectionDirective.validateAgainstPersistedViolations(*l.Manifest, l.Rejections[0].SemanticViolations); err != nil {
			return err
		}
		if l.CorrectionDirective.StablePrefixHash != l.StablePrefixHash {
			return fmt.Errorf("runtime semantic correction directive stable prefix is stale")
		}
	} else if l.Attempt == 1 {
		return fmt.Errorf("runtime semantic attempt 1 requires a correction directive")
	}
	if l.CatalogRefreshAttempts < 0 || l.CatalogRefreshAttempts != len(l.FeasibilityGaps) {
		return fmt.Errorf("runtime semantic catalog refresh attempts do not match gap lineage")
	}
	for idx := range l.FeasibilityGaps {
		if err := l.FeasibilityGaps[idx].Validate(); err != nil {
			return fmt.Errorf("runtime semantic feasibility gap %d: %w", idx, err)
		}
		if l.FeasibilityGaps[idx].RefreshGeneration != idx {
			return fmt.Errorf("runtime semantic feasibility gap generation %d is not contiguous", l.FeasibilityGaps[idx].RefreshGeneration)
		}
	}
	switch l.Phase {
	case RuntimeSemanticPlanningRequired:
		if l.Manifest != nil || l.Attempt != 0 || l.StablePrefixHash != "" || l.LogicalRequestID != "" || l.CorrectionDirective != nil || l.CoverageOrderDocument != nil || l.Document != nil || l.DocumentHash != "" || len(l.Batches) != 0 || l.ActiveBatchID != "" || l.NextBatchSequence != 0 || len(l.PendingCoverageIDs) != 0 || len(l.CompletedBatchIDs) != 0 || len(l.BatchFeasibilityResults) != 0 || len(l.BatchDocuments) != 0 || l.MergedDocument != nil || l.MergedDocumentHash != "" || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 || len(l.Rejections) != 0 || l.Failure != nil {
			return fmt.Errorf("required runtime semantic planning ledger has unexpected downstream facts")
		}
	case RuntimeSemanticPlanningOrderRequestOpen, RuntimeSemanticPlanningRequestOpen, RuntimeSemanticPlanningCorrectionRequired:
		batchCorrectionDocument := l.Phase == RuntimeSemanticPlanningCorrectionRequired && l.ActiveBatchID != "" && l.Document != nil && l.Document.PlanStep == RuntimeSemanticPlanStepCoverageOrder && l.DocumentHash == StableRuntimeSemanticPlanDocumentHash(*l.Document)
		if l.Manifest == nil || l.Attempt < 0 || l.Attempt > 1 || l.StablePrefixHash == "" || l.LogicalRequestID == "" || (!batchCorrectionDocument && (l.Document != nil || l.DocumentHash != "")) || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 || l.Failure != nil {
			return fmt.Errorf("open runtime semantic planning ledger has invalid shape")
		}
		if (l.Attempt == 0 && l.CorrectionDirective != nil) || (l.Attempt == 1 && l.CorrectionDirective == nil) || (l.Phase == RuntimeSemanticPlanningCorrectionRequired && l.Attempt != 1) {
			return fmt.Errorf("runtime semantic planning correction lineage is invalid")
		}
		if ((l.Phase == RuntimeSemanticPlanningOrderRequestOpen || l.Phase == RuntimeSemanticPlanningRequestOpen) && len(l.Rejections) != 0) || (l.Phase == RuntimeSemanticPlanningCorrectionRequired && len(l.Rejections) != 1) {
			return fmt.Errorf("runtime semantic planning rejection lineage is invalid")
		}
		if l.Phase != RuntimeSemanticPlanningCorrectionRequired {
			if l.CoverageOrderDocument != nil || len(l.Batches) != 0 || l.ActiveBatchID != "" || l.NextBatchSequence != 0 || len(l.PendingCoverageIDs) != 0 || len(l.CompletedBatchIDs) != 0 || len(l.BatchDocuments) != 0 || len(l.BatchFeasibilityResults) != 0 || l.MergedDocument != nil || l.MergedDocumentHash != "" {
				return fmt.Errorf("open runtime semantic planning ledger has unexpected batch lineage")
			}
		} else if l.ActiveBatchID != "" {
			batch, found := runtimeSemanticPlanningBatch(l.Batches, l.ActiveBatchID)
			if !found || batch.State != RuntimeSemanticPlanBatchRequesting || l.CoverageOrderDocument == nil || l.Document == nil || l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder {
				return fmt.Errorf("proof-batch correction ledger has invalid active batch lineage")
			}
			if err := validateRuntimeSemanticPendingForBatch(l, batch); err != nil {
				return err
			}
		} else if l.CoverageOrderDocument != nil || len(l.Batches) != 0 || l.NextBatchSequence != 0 || len(l.PendingCoverageIDs) != 0 || len(l.CompletedBatchIDs) != 0 || len(l.BatchDocuments) != 0 || len(l.BatchFeasibilityResults) != 0 || l.MergedDocument != nil || l.MergedDocumentHash != "" {
			return fmt.Errorf("coverage-order correction ledger has unexpected batch lineage")
		}
	case RuntimeSemanticPlanningOrderAccepted:
		if err := validateRuntimeSemanticPlanningAcceptedBase(l, validatedManifest); err != nil {
			return err
		}
		if l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.CoverageOrderDocument == nil ||
			StableRuntimeSemanticPlanDocumentHash(*l.CoverageOrderDocument) != StableRuntimeSemanticPlanDocumentHash(*l.Document) ||
			l.ActiveBatchID != "" || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" ||
			l.MergedDocument != nil || l.MergedDocumentHash != "" {
			return fmt.Errorf("order-accepted semantic ledger has invalid order/batch state")
		}
		if err := validateRuntimeSemanticPendingAfterCompleted(l); err != nil {
			return err
		}
	case RuntimeSemanticPlanningProofBatchRequestOpen:
		if err := validateRuntimeSemanticPlanningAcceptedBase(l, validatedManifest); err != nil {
			return err
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, l.ActiveBatchID)
		if !found || batch.State != RuntimeSemanticPlanBatchRequesting || l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.CoverageOrderDocument == nil {
			return fmt.Errorf("proof-batch request ledger requires requesting active batch and order document")
		}
		if err := validateRuntimeSemanticPendingForBatch(l, batch); err != nil {
			return err
		}
		if l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" || l.MergedDocument != nil || l.MergedDocumentHash != "" {
			return fmt.Errorf("proof-batch request ledger has downstream feasibility facts")
		}
	case RuntimeSemanticPlanningProofBatchDocumentAccepted:
		if err := validateRuntimeSemanticPlanningAcceptedBase(l, validatedManifest); err != nil {
			return err
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, l.ActiveBatchID)
		batchDocument, documentFound := batchDocuments[l.ActiveBatchID]
		if !found || batch.State != RuntimeSemanticPlanBatchDocumentComplete || !documentFound ||
			batchDocument.PlanStep != RuntimeSemanticPlanStepProofBatch || l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.CoverageOrderDocument == nil {
			return fmt.Errorf("proof-batch document phase requires complete active batch document")
		}
		if err := validateRuntimeSemanticPendingForBatch(l, batch); err != nil {
			return err
		}
		if l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" || l.MergedDocument != nil || l.MergedDocumentHash != "" {
			return fmt.Errorf("proof-batch document phase has downstream feasibility facts")
		}
	case RuntimeSemanticPlanningFeasibilityRequired:
		if err := validateRuntimeSemanticPlanningAcceptedBase(l, validatedManifest); err != nil {
			return err
		}
		if l.ActiveBatchID != "" {
			batch, found := runtimeSemanticPlanningBatch(l.Batches, l.ActiveBatchID)
			batchDocument, documentFound := batchDocuments[l.ActiveBatchID]
			if !found || batch.State != RuntimeSemanticPlanBatchFeasibilityPending || !documentFound ||
				batchDocument.PlanStep != RuntimeSemanticPlanStepProofBatch || l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.CoverageOrderDocument == nil {
				return fmt.Errorf("semantic feasibility phase requires pending active proof batch")
			}
			if err := validateRuntimeSemanticPendingForBatch(l, batch); err != nil {
				return err
			}
			if l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, batchDocument) {
				return fmt.Errorf("runtime semantic feasibility batch identity does not match active document")
			}
			if len(l.Feasibilities) > len(batchDocument.Proposals) || l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
				return fmt.Errorf("runtime semantic feasibility set hash does not match observations")
			}
			if err := validateRuntimeSemanticFeasibilitySetWithValidatedManifest(*validatedManifest, batchDocument, l.Feasibilities, false); err != nil {
				return err
			}
		} else {
			// The active batch is cleared only by the private merge event. This
			// shape is the short-lived seal-ready state; it must retain the exact
			// merged focus document rather than falling back to a full-manifest
			// document with no batch identity.
			if l.MergedDocument == nil || l.Document.PlanStep != RuntimeSemanticPlanStepProofBatch || l.Document.BatchID == "" ||
				l.DocumentHash != l.MergedDocumentHash || StableRuntimeSemanticPlanDocumentHash(*l.Document) != l.MergedDocumentHash {
				return fmt.Errorf("semantic feasibility phase without active batch requires merged focus document")
			}
			batch, found := runtimeSemanticPlanningBatch(l.Batches, l.Document.BatchID)
			if !found || batch.State != RuntimeSemanticPlanBatchAccepted || l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, *l.Document) {
				return fmt.Errorf("semantic merged focus document is not bound to accepted batch")
			}
			if len(l.Feasibilities) != len(l.Document.Proposals) || l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
				return fmt.Errorf("merged semantic focus requires complete owner facts")
			}
			if err := validateRuntimeSemanticFeasibilitySetWithValidatedManifest(*validatedManifest, *l.Document, l.Feasibilities, true); err != nil {
				return err
			}
		}
	case RuntimeSemanticPlanningProofBatchFeasibilityComplete:
		// A completed proof batch is still an unsealed SM-2 working state.  It
		// carries only the accepted batch document and the complete owner facts
		// for that batch; binding/focus authority is not allowed until the
		// orchestrator performs the explicit merge/seal transition.
		if l.Manifest == nil || l.Attempt < 0 || l.Attempt > 1 || l.StablePrefixHash == "" || l.LogicalRequestID == "" || l.Document == nil || l.DocumentHash == "" || l.ActiveBatchID == "" || l.FeasibilityBatchID == "" || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 || len(l.Rejections) != l.Attempt || l.Failure != nil || l.MergedDocument != nil || l.MergedDocumentHash != "" {
			return fmt.Errorf("completed runtime semantic proof batch has invalid shape")
		}
		var batchDocument *RuntimeSemanticPlanDocument
		for idx := range l.BatchDocuments {
			if l.BatchDocuments[idx].BatchID == l.ActiveBatchID {
				value := l.BatchDocuments[idx].Clone()
				batchDocument = &value
				break
			}
		}
		if batchDocument == nil || batchDocument.PlanStep != RuntimeSemanticPlanStepProofBatch || batchDocument.BatchID != l.ActiveBatchID {
			return fmt.Errorf("completed runtime semantic proof batch requires active proof-batch document")
		}
		if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(*batchDocument, *validatedManifest); err != nil {
			return err
		}
		if l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.DocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.Document) || l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, *batchDocument) {
			return fmt.Errorf("completed runtime semantic proof batch lineage does not match contents")
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, l.ActiveBatchID)
		if !found || batch.State != RuntimeSemanticPlanBatchAccepted || l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.CoverageOrderDocument == nil {
			return fmt.Errorf("completed runtime semantic proof batch has invalid accepted batch/order identity")
		}
		if err := validateRuntimeSemanticPendingForBatch(l, batch); err != nil {
			return err
		}
		if len(l.Feasibilities) != len(batchDocument.Proposals) || l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
			return fmt.Errorf("completed runtime semantic proof batch requires complete owner facts")
		}
		if err := validateRuntimeSemanticFeasibilitySetWithValidatedManifest(*validatedManifest, *batchDocument, l.Feasibilities, true); err != nil {
			return err
		}
	case RuntimeSemanticPlanningSealed:
		if l.Manifest == nil || l.CoverageOrderDocument == nil || l.StablePrefixHash == "" || l.LogicalRequestID == "" || l.Document == nil || l.DocumentHash == "" || l.Document.PlanStep != RuntimeSemanticPlanStepProofBatch || l.Document.BatchID == "" || l.ActiveBatchID != "" || l.MergedDocument == nil || l.FeasibilityBatchID == "" || len(l.Feasibilities) != len(l.Document.Proposals) || l.FeasibilitySetHash == "" || l.SealedBindingSetHash == "" || len(l.SealedLineageRefs) == 0 || len(l.Rejections) != l.Attempt || l.Failure != nil {
			return fmt.Errorf("sealed runtime semantic planning ledger requires document, complete feasibility and binding hash")
		}
		if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(*l.Document, *validatedManifest); err != nil {
			return err
		}
		if l.DocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.Document) || l.MergedDocumentHash != l.DocumentHash || StableRuntimeSemanticPlanDocumentHash(*l.MergedDocument) != l.DocumentHash || l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, *l.Document) || l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
			return fmt.Errorf("sealed runtime semantic planning lineage does not match contents")
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, l.Document.BatchID)
		if !found || batch.State != RuntimeSemanticPlanBatchAccepted || batch.DocumentHash != l.DocumentHash {
			return fmt.Errorf("sealed runtime semantic planning document is not bound to accepted batch")
		}
		if _, completed := completed[l.Document.BatchID]; !completed {
			return fmt.Errorf("sealed runtime semantic planning batch is not in completed history")
		}
		if err := validateRuntimeSemanticPendingAfterCompleted(l); err != nil {
			return err
		}
		expectedLineage := normalizeOrderedStrings([]string{
			l.Manifest.Hash,
			StableRuntimeSemanticPlanCoverageOrderHash(*l.CoverageOrderDocument),
			l.DocumentHash,
			l.FeasibilityBatchID,
			l.FeasibilitySetHash,
			l.SealedBindingSetHash,
		})
		if !sameOrderedRuntimeSemanticStrings(l.SealedLineageRefs, expectedLineage) {
			return fmt.Errorf("sealed runtime semantic lineage references do not match authority")
		}
		if err := validateRuntimeSemanticFeasibilitySetWithValidatedManifest(*validatedManifest, *l.Document, l.Feasibilities, true); err != nil {
			return err
		}
	case RuntimeSemanticPlanningFailed:
		if l.Failure == nil || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 {
			return fmt.Errorf("failed runtime semantic planning ledger requires failure and no sealed binding")
		}
		if err := l.Failure.Validate(); err != nil {
			return err
		}
		if l.Manifest == nil {
			if l.Document != nil || l.DocumentHash != "" || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" {
				return fmt.Errorf("failed runtime semantic planning ledger has downstream facts without a manifest")
			}
			break
		}
		if l.Document == nil {
			if l.DocumentHash != "" || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" {
				return fmt.Errorf("failed runtime semantic planning ledger has owner facts without a document")
			}
			break
		}
		if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(*l.Document, *validatedManifest); err != nil {
			return err
		}
		if l.DocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.Document) {
			return fmt.Errorf("failed runtime semantic planning document hash does not match contents")
		}
		// During a proof-batch owner probe the canonical ledger document remains
		// the coverage-order document while the active batch document is stored
		// separately.  A typed owner rejection can therefore legitimately leave
		// FeasibilityBatchID bound to that active batch.  Validate the same
		// document that the feasibility dispatcher used instead of treating the
		// failure transition as an internal contract defect.
		feasibilityDocument := l.Document
		hasFeasibilityDocument := true
		if l.FeasibilityBatchID != "" && l.ActiveBatchID != "" {
			hasFeasibilityDocument = false
			for idx := range l.BatchDocuments {
				if l.BatchDocuments[idx].BatchID == l.ActiveBatchID {
					value := l.BatchDocuments[idx].Clone()
					feasibilityDocument = &value
					hasFeasibilityDocument = true
					break
				}
			}
		}
		if hasFeasibilityDocument && l.FeasibilityBatchID != "" && l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, *feasibilityDocument) {
			return fmt.Errorf("failed runtime semantic feasibility batch identity does not match document")
		}
		if l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
			return fmt.Errorf("failed runtime semantic feasibility set hash does not match observations")
		}
		if hasFeasibilityDocument {
			if err := validateRuntimeSemanticFeasibilitySetWithValidatedManifest(*validatedManifest, *feasibilityDocument, l.Feasibilities, false); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("runtime semantic planning ledger has invalid phase %q", l.Phase)
	}
	return nil
}
