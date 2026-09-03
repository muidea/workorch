package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/requestcontent"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeSemanticBatchDocument is the runtime-owner-local lookup for the
// active proof document. The drive package has a similarly shaped helper for
// its own adapter layer; keeping this lookup here avoids exposing a registry or
// cross-package mutable ledger accessor.
func runtimeSemanticBatchDocument(values []runtimecontract.RuntimeSemanticPlanDocument, batchID string) (runtimecontract.RuntimeSemanticPlanDocument, bool) {
	return semanticplan.DocumentByBatchID(values, batchID)
}

func runtimeSemanticBatch(values []runtimecontract.RuntimeSemanticPlanBatch, batchID string) (runtimecontract.RuntimeSemanticPlanBatch, bool) {
	return semanticplan.BatchByID(values, batchID)
}

func validateSemanticManifestIdentity(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeSemanticPlanManifest) error {
	manifest = manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return err
	}
	if aggregate.Facts.CWILedgerSchemaVersion != runtimecontract.RuntimeCWILedgerSchemaCurrent {
		return fmt.Errorf("%w: runtime semantic manifest requires the current CWI ledger", common.ErrConflict)
	}
	eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts)
	if len(eligible) == 0 {
		return fmt.Errorf("%w: runtime semantic manifest has no eligible CWI", common.ErrConflict)
	}
	repairInput, repairRequired, err := CurrentRuntimeValidationFailureRepairGenerationInput(aggregate)
	if err != nil {
		return err
	}
	identity := semanticplan.ManifestIdentity{
		WorkspaceID:       aggregate.Facts.WorkspaceID,
		RunID:             aggregate.Facts.RunID,
		Definition:        aggregate.Definition,
		DefinitionSeed:    aggregate.Facts.DefinitionSeed,
		DefinitionFocuses: aggregate.Facts.DefinitionFocuses,
		Execution:         aggregate.Facts.Activation.ExecutionContract,
		Admissibilities:   aggregate.Facts.Resources.OperationAdmissibilities,
		EligibleCoverage:  eligible,
	}
	if repairRequired {
		identity.EligibleCoverage = []common.CoverageWorkItemID{repairInput.CoverageWorkItemID}
		identity.ValidationFailureRepair = &repairInput
	}
	return semanticplan.ValidateManifestIdentity(identity, manifest)
}

// semanticCatalogInvalidationLineage binds a retired semantic generation to
// the durable prerequisite invalidation(s) that made its owner catalog stale.
// A missing record is possible only when an invalid aggregate reaches
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
	if aggregate.ActiveDispatch != nil {
		return fmt.Errorf("%w: semantic planning generation cannot retire an active dispatch", common.ErrConflict)
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
	if err := retireSemanticPlanningRequestSessions(aggregate, ledger, "semantic_catalog_invalidated"); err != nil {
		return err
	}
	aggregate.SemanticPlanning = newLedger
	return nil
}

func retireSemanticPlanningRequestSessions(aggregate *RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger, outcomeCode string) error {
	if aggregate == nil || ledger.Manifest == nil {
		return common.ErrInvalidArgument
	}
	outcomeCode = strings.TrimSpace(outcomeCode)
	if outcomeCode == "" {
		outcomeCode = "semantic_generation_retired"
	}
	manifestHash := ledger.Manifest.Hash
	// Retirement may close only sessions whose last physical call is already
	// durably settled. A submitted prepared/recovery/transport request still
	// owns reserved physical-call allocations; clearing its submission marker
	// here would strand that budget and make startup recovery unable to classify
	// the call as submission-unknown. Scan first so the failure is mutation-free.
	for idx := range aggregate.CandidateRequestSessions {
		session := aggregate.CandidateRequestSessions[idx].Clone()
		if session.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan || session.ManifestHash != manifestHash {
			continue
		}
		if session.ProviderSubmitted {
			switch session.State {
			case runtimecontract.RuntimeCandidateRequestSessionPrepared,
				runtimecontract.RuntimeCandidateRequestSessionRecoveryPending,
				runtimecontract.RuntimeCandidateRequestSessionTransportPending:
				return fmt.Errorf("%w: semantic generation cannot retire unresolved provider request %q", common.ErrConflict, session.LogicalRequestID)
			}
		}
	}
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
		// Framing describes only the active document/recovery state. A stale
		// retirement is a distinct non-document terminal and keeps its immutable
		// provider history in audit/settlement facts rather than this live slot.
		session.LastDocumentFraming = nil
		session.LastOutcome = runtimecontract.RuntimeCandidateRequestOutcome{
			Class:     runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale,
			Code:      outcomeCode,
			Signature: outcomeCode + "/" + manifestHash + "/" + strings.TrimSpace(session.LogicalRequestID),
		}
	}
	return nil
}

// retireSemanticPlanningGenerationForCWIReopen starts a new semantic
// generation after SM-2 retires a CWI's current binding because its prior
// generation can no longer converge. The row may have held accepted credit or
// may have reached assessment with an unmet owner postcondition. Catalog facts
// are not presumed stale: immutable history records only the exact CWI trigger
// lineage, while the new generation recompiles its manifest from canonical
// eligible rows.
func retireSemanticPlanningGenerationForCWIReopen(aggregate *RuntimeAggregate, reopened []common.CoverageWorkItemID) error {
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
	if aggregate.ActiveDispatch != nil {
		return fmt.Errorf("%w: semantic planning generation cannot retire an active dispatch", common.ErrConflict)
	}
	if len(ledger.History) >= runtimecontract.RuntimeSemanticPlanningHistoryLimit {
		return fmt.Errorf("%w: semantic planning generation history exhausted", common.ErrConflict)
	}
	byCoverage := runtimeCoverageByID(aggregate.Facts.Coverage)
	seen := make(map[common.CoverageWorkItemID]struct{}, len(reopened))
	ids := make([]common.CoverageWorkItemID, 0, len(reopened))
	triggerRefs := make([]string, 0, len(reopened))
	for _, coverageID := range reopened {
		if coverageID == "" {
			continue
		}
		if _, duplicate := seen[coverageID]; duplicate {
			continue
		}
		item, found := byCoverage[coverageID]
		if !found || item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened || len(item.ReopenLineage) == 0 {
			return fmt.Errorf("%w: semantic CWI reopen %q has no canonical reopened lineage", common.ErrConflict, coverageID)
		}
		seen[coverageID] = struct{}{}
		ids = append(ids, coverageID)
		triggerRefs = append(triggerRefs, item.ReopenLineage[len(item.ReopenLineage)-1].TriggerRef)
	}
	if len(ids) == 0 {
		return nil
	}
	sort.SliceStable(ids, func(i, j int) bool { return string(ids[i]) < string(ids[j]) })
	sort.Strings(triggerRefs)
	snapshot := ledger.Clone()
	snapshot.History = nil
	history := runtimecontract.RuntimeSemanticPlanningGeneration{
		RetiredAtStateRevision: aggregate.StateRevision + 1,
		Reason:                 runtimecontract.RuntimeSemanticPlanningRetirementCWIReopened,
		ReopenedCoverageIDs:    ids,
		ReopenTriggerRefs:      triggerRefs,
		Snapshot:               snapshot,
	}
	if err := history.Validate(); err != nil {
		return err
	}
	newLedger := runtimecontract.RuntimeSemanticPlanningLedger{
		SchemaVersion:          ledger.SchemaVersion,
		Phase:                  runtimecontract.RuntimeSemanticPlanningRequired,
		DefinitionSeed:         ledger.DefinitionSeed.Clone(),
		CatalogRefreshAttempts: ledger.CatalogRefreshAttempts,
		History:                append(append([]runtimecontract.RuntimeSemanticPlanningGeneration(nil), ledger.History...), history),
	}
	if err := newLedger.Validate(); err != nil {
		return err
	}
	if err := retireSemanticPlanningRequestSessions(aggregate, ledger, "semantic_cwi_reopened"); err != nil {
		return err
	}
	aggregate.SemanticPlanning = newLedger
	return nil
}

// retireSemanticPlanningGenerationForBindingContradiction clears only the
// generation that sealed the contradicted binding. The binding row must
// already contain the matching no-effect retirement lineage.
func retireSemanticPlanningGenerationForBindingContradiction(aggregate *RuntimeAggregate, coverageID common.CoverageWorkItemID, causationRef string) error {
	if aggregate == nil || coverageID == "" || strings.TrimSpace(causationRef) == "" {
		return common.ErrInvalidArgument
	}
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningRequired {
		return nil
	}
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningFailed || ledger.Manifest == nil || aggregate.ActiveDispatch != nil {
		return fmt.Errorf("%w: semantic binding contradiction cannot retire the current generation", common.ErrConflict)
	}
	if len(ledger.History) >= runtimecontract.RuntimeSemanticPlanningHistoryLimit {
		return fmt.Errorf("%w: semantic planning generation history exhausted", common.ErrConflict)
	}
	item, found := runtimeCoverageByID(aggregate.Facts.Coverage)[coverageID]
	if !found || item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStatePlanning || item.CurrentBindingRef != nil || len(item.BindingRetirementLineage) == 0 ||
		item.BindingRetirementLineage[len(item.BindingRetirementLineage)-1].CausationRef != strings.TrimSpace(causationRef) {
		return fmt.Errorf("%w: semantic binding contradiction has no canonical retirement lineage", common.ErrConflict)
	}
	snapshot := ledger.Clone()
	snapshot.History = nil
	history := runtimecontract.RuntimeSemanticPlanningGeneration{
		RetiredAtStateRevision: aggregate.StateRevision + 1,
		Reason:                 runtimecontract.RuntimeSemanticPlanningRetirementBindingContradiction,
		ReopenedCoverageIDs:    []common.CoverageWorkItemID{coverageID},
		ReopenTriggerRefs:      []string{strings.TrimSpace(causationRef)},
		Snapshot:               snapshot,
	}
	if err := history.Validate(); err != nil {
		return err
	}
	newLedger := runtimecontract.RuntimeSemanticPlanningLedger{
		SchemaVersion:          ledger.SchemaVersion,
		Phase:                  runtimecontract.RuntimeSemanticPlanningRequired,
		DefinitionSeed:         ledger.DefinitionSeed.Clone(),
		CatalogRefreshAttempts: ledger.CatalogRefreshAttempts,
		History:                append(append([]runtimecontract.RuntimeSemanticPlanningGeneration(nil), ledger.History...), history),
	}
	if err := newLedger.Validate(); err != nil {
		return err
	}
	if err := retireSemanticPlanningRequestSessions(aggregate, ledger, "semantic_binding_contradiction"); err != nil {
		return err
	}
	aggregate.SemanticPlanning = newLedger
	return nil
}

// retireSemanticPlanningGenerationForEligibilityChange retires an immutable
// order when settled prerequisite bindings unlock CWI outside its manifest.
// The exact changed rows and their prerequisite binding refs are audit
// lineage; the next action is still derived by the normal SM-2 reducer.
func retireSemanticPlanningGenerationForEligibilityChange(aggregate *RuntimeAggregate, changed []common.CoverageWorkItemID) error {
	if aggregate == nil {
		return common.ErrInvalidArgument
	}
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningRequired {
		return nil
	}
	if ledger.Phase == runtimecontract.RuntimeSemanticPlanningFailed || ledger.Manifest == nil || aggregate.ActiveDispatch != nil {
		return fmt.Errorf("%w: semantic planning eligibility change cannot retire the current generation", common.ErrConflict)
	}
	if len(ledger.History) >= runtimecontract.RuntimeSemanticPlanningHistoryLimit {
		return fmt.Errorf("%w: semantic planning generation history exhausted", common.ErrConflict)
	}
	coverage := runtimeCoverageByID(aggregate.Facts.Coverage)
	eligible := make(map[common.CoverageWorkItemID]struct{})
	for _, id := range runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts) {
		eligible[id] = struct{}{}
	}
	seen := make(map[common.CoverageWorkItemID]struct{}, len(changed))
	ids := make([]common.CoverageWorkItemID, 0, len(changed))
	triggerRefs := make([]string, 0)
	for _, id := range changed {
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		if _, current := eligible[id]; !current {
			return fmt.Errorf("%w: semantic closure change %q is not currently eligible", common.ErrConflict, id)
		}
		item, found := coverage[id]
		if !found || len(item.PrerequisiteCoverageWorkItemIDs) == 0 {
			return fmt.Errorf("%w: semantic closure change %q has no prerequisite lineage", common.ErrConflict, id)
		}
		for _, prerequisiteID := range item.PrerequisiteCoverageWorkItemIDs {
			prerequisite, prerequisiteFound := coverage[prerequisiteID]
			if !prerequisiteFound || prerequisite.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled ||
				prerequisite.State != runtimecontract.RuntimeCoverageItemStateSatisfied || prerequisite.CurrentBindingRef == nil {
				return fmt.Errorf("%w: semantic closure change %q has no settled prerequisite binding", common.ErrConflict, id)
			}
			triggerRefs = append(triggerRefs, prerequisite.CurrentBindingRef.BindingID)
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(triggerRefs) == 0 {
		return fmt.Errorf("%w: semantic eligibility change has no typed lineage", common.ErrConflict)
	}
	sort.SliceStable(ids, func(i, j int) bool { return string(ids[i]) < string(ids[j]) })
	sort.Strings(triggerRefs)
	uniqueTriggerRefs := make([]string, 0, len(triggerRefs))
	for _, ref := range triggerRefs {
		if len(uniqueTriggerRefs) == 0 || uniqueTriggerRefs[len(uniqueTriggerRefs)-1] != ref {
			uniqueTriggerRefs = append(uniqueTriggerRefs, ref)
		}
	}
	triggerRefs = uniqueTriggerRefs
	snapshot := ledger.Clone()
	snapshot.History = nil
	history := runtimecontract.RuntimeSemanticPlanningGeneration{
		RetiredAtStateRevision:   aggregate.StateRevision + 1,
		Reason:                   runtimecontract.RuntimeSemanticPlanningRetirementEligibleClosureChanged,
		ChangedCoverageIDs:       ids,
		ClosureChangeTriggerRefs: triggerRefs,
		Snapshot:                 snapshot,
	}
	if err := history.Validate(); err != nil {
		return err
	}
	newLedger := runtimecontract.RuntimeSemanticPlanningLedger{
		SchemaVersion: ledger.SchemaVersion, Phase: runtimecontract.RuntimeSemanticPlanningRequired,
		DefinitionSeed: ledger.DefinitionSeed.Clone(), FeasibilityGaps: append([]runtimecontract.RuntimeSemanticFeasibilityGap(nil), ledger.FeasibilityGaps...),
		CatalogRefreshAttempts: ledger.CatalogRefreshAttempts,
		History:                append(append([]runtimecontract.RuntimeSemanticPlanningGeneration(nil), ledger.History...), history),
	}
	if err := newLedger.Validate(); err != nil {
		return err
	}
	if err := retireSemanticPlanningRequestSessions(aggregate, ledger, "semantic_eligible_closure_changed"); err != nil {
		return err
	}
	aggregate.SemanticPlanning = newLedger
	return nil
}

// retireSemanticPlanningForCurrentReopenedCWIs defers a CWI-triggered
// semantic reset until no immutable sealed dispatch remains. It is invoked by
// the same Orchestrator CAS that finishes receipt/reconciliation processing.
func retireSemanticPlanningForCurrentReopenedCWIs(aggregate *RuntimeAggregate) error {
	if aggregate == nil || aggregate.ActiveDispatch != nil {
		return nil
	}
	reopened := make([]common.CoverageWorkItemID, 0)
	for _, item := range aggregate.Facts.Coverage {
		if item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateReopened {
			reopened = append(reopened, item.ID)
		}
	}
	return retireSemanticPlanningGenerationForCWIReopen(aggregate, reopened)
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
	catalogs := append([]runtimecontract.RuntimeOperationAdmissibility(nil), aggregate.SemanticPlanning.Manifest.OwnerCatalogs...)
	catalogs = append(catalogs, aggregate.SemanticPlanning.Manifest.OwnerFactDependencyCatalogs...)
	for _, value := range catalogs {
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
func (c *planningTransactionComponent) compileSemanticPlanBatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, batch runtimecontract.RuntimeSemanticPlanBatch) (RuntimeAggregate, error) {
	transaction, err := c.beginSemanticPlanningMutation(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	changed, err := func(aggregate RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if ledger.Manifest == nil || ledger.CoverageOrderDocument == nil || ledger.Document == nil || ledger.Document.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder || ledger.ActiveBatchID != "" {
			return false, fmt.Errorf("%w: semantic batch requires an accepted coverage order", common.ErrConflict)
		}
		if ledger.NextBatchSequence != len(ledger.Batches) || batch.Sequence != ledger.NextBatchSequence {
			return false, fmt.Errorf("%w: semantic batch sequence is not the next contiguous sequence", common.ErrConflict)
		}
		batch = batch.Normalized()
		if batch.State != runtimecontract.RuntimeSemanticPlanBatchPending {
			return false, fmt.Errorf("%w: semantic batch compile requires owner-internal pending initialization", common.ErrConflict)
		}
		if err := batch.ValidateAgainstManifest(*ledger.Manifest); err != nil {
			return false, err
		}
		maxItems := aggregate.Facts.ConvergencePolicy.MaxCandidateContextItems
		if maxItems <= 0 {
			maxItems = len(ledger.Manifest.CoverageItems)
		}
		expected, err := semanticplan.CompileFocusBatch(
			*ledger.Manifest, *ledger.CoverageOrderDocument, batch.Sequence, batch.RequestLogicalID,
			SemanticPlanActiveCoverageIDsForDrive(aggregate.Facts, *ledger.CoverageOrderDocument, ledger.PendingCoverageIDs), ledger.PendingCoverageIDs, maxItems,
		)
		if err != nil {
			return false, err
		}
		if !runtimecontract.SameRuntimeSemanticPlanBatchImmutableIdentity(batch, expected) {
			return false, fmt.Errorf("%w: semantic batch identity does not match current focus and pending lineage", common.ErrConflict)
		}
		requesting, err := advanceRuntimeSemanticPlanBatch(expected, runtimecontract.RuntimeSemanticPlanBatchRequesting)
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
	}(transaction.aggregate, &transaction.ledger)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanningMutation(ctx, transaction, changed)
}

// SemanticPlanActiveCoverageIDsForDrive selects the owner-local anchor for the
// next bounded proof batch. Facts.Focus remains the execution focus until the
// complete semantic contract is sealed, so after its CWI leaves the pending
// set it must not be reused to expand a later planning batch. The provider
// order supplies only the next pending identity; it does not grant authority.
func SemanticPlanActiveCoverageIDsForDrive(facts runtimecontract.RuntimeFacts, order runtimecontract.RuntimeSemanticPlanDocument, pending []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	pendingSet := make(map[common.CoverageWorkItemID]struct{}, len(pending))
	for _, id := range pending {
		pendingSet[id] = struct{}{}
	}
	active := make([]common.CoverageWorkItemID, 0, len(facts.Focus.CoverageWorkItemIDs))
	for _, id := range facts.Focus.Normalized().CoverageWorkItemIDs {
		if _, found := pendingSet[id]; found {
			active = append(active, id)
		}
	}
	if len(active) != 0 {
		return active
	}
	for _, id := range order.Normalized().CoverageOrder {
		if _, found := pendingSet[id]; found {
			return []common.CoverageWorkItemID{id}
		}
	}
	return nil
}

// SemanticOperationAuthorityReady reports whether SM-3 may consume the
// canonical current binding without creating a new semantic generation. A
// catalog-invalidated generation can be required while an accepted durable
// effect keeps the current CWI's older binding lineage-locked. That current
// owner transaction must finish validation/assessment before a later eligible
// CWI may start a new semantic generation.
func SemanticOperationAuthorityReady(aggregate RuntimeAggregate) bool {
	// A retired owner catalog generation can temporarily coexist with its exact
	// no-effect binding until SM-4 delivers the typed CWIBindingRetired event.
	// That binding is immutable audit lineage, not reusable operation authority.
	// Let the existing SM-2 retirement reducer reopen the CWI before any
	// operations request or terminal projection consumes the old snapshot.
	if RuntimeCWIBindingRetirementPending(aggregate) {
		return false
	}
	if aggregate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningSealed {
		return true
	}
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired {
		return false
	}
	if validateCurrentCWIBindingFenceForEvidenceCommit(aggregate.Facts) != nil {
		return false
	}
	if len(runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts)) == 0 {
		return true
	}
	return currentRuntimeCWIBindingExecutionLocked(aggregate)
}

// SemanticPlanningPrerequisitesPending is the single typed pre-provider gate
// for mutable owner facts consumed by a semantic generation. TargetSelection
// is included so a fresh Definition selector is resolved by its owner before
// any provider document may propose a target.
func SemanticPlanningPrerequisitesPending(values []runtimecontract.RuntimePrerequisite) bool {
	for _, value := range values {
		switch value.Kind {
		case runtimecontract.PrerequisiteKindProviderCeiling,
			runtimecontract.PrerequisiteKindCapabilityState,
			runtimecontract.PrerequisiteKindOperationAdmissibility,
			runtimecontract.PrerequisiteKindTargetSelection:
			return true
		}
	}
	return false
}

func currentRuntimeCWIBindingExecutionLocked(aggregate RuntimeAggregate) bool {
	focus := aggregate.Facts.Focus.Normalized()
	if len(focus.CoverageWorkItemIDs) != 1 {
		return false
	}
	coverageID := focus.CoverageWorkItemIDs[0]
	probe := aggregate.Facts.Normalized()
	for index := range probe.Coverage {
		item := &probe.Coverage[index]
		if item.ID != coverageID || item.CurrentBindingRef == nil {
			continue
		}
		item.BindingRetirementLineage = append(item.BindingRetirementLineage, runtimecontract.RuntimeCWIBindingRetirementLineage{
			RetiredBindingRef: item.CurrentBindingRef.Clone(), CausationRef: "read-only/execution-lock-probe", Reason: "read-only execution lock probe",
		})
		var active *runtimecontract.RuntimeDispatchRecord
		if aggregate.ActiveDispatch != nil {
			value := sealedDispatchToRuntimeRecord(*aggregate.ActiveDispatch)
			active = &value
		}
		return runtimecontract.ValidateRuntimeCWIBindingRetirementsAgainstExecution(
			probe, active,
			sealedDispatchesToRuntimeRecords(aggregate.CompletedDispatches), aggregate.OperationReceipts,
		) != nil
	}
	return false
}

func (c *planningTransactionComponent) advanceSemanticPlanBatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, identity runtimecontract.RuntimeSemanticPlanBatch, next runtimecontract.RuntimeSemanticPlanBatchState) (RuntimeAggregate, error) {
	transaction, err := c.beginSemanticPlanningMutation(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	changed, err := func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
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
			advanced, err := advanceRuntimeSemanticPlanBatch(persisted, next)
			if err != nil {
				return false, err
			}
			ledger.Batches[idx] = advanced
			return true, nil
		}
		return false, fmt.Errorf("%w: semantic batch is missing", common.ErrConflict)
	}(transaction.aggregate, &transaction.ledger)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanningMutation(ctx, transaction, changed)
}

func (c *planningTransactionComponent) observeSemanticPlanBatchFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, result runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult) (RuntimeAggregate, error) {
	transaction, err := c.beginSemanticPlanningMutation(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	changed, err := func(aggregate RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
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
	}(transaction.aggregate, &transaction.ledger)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanningMutation(ctx, transaction, changed)
}

func (c *planningTransactionComponent) observeSemanticPlanBatchDocument(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, document runtimecontract.RuntimeSemanticPlanDocument) (RuntimeAggregate, error) {
	document = document.Normalized()
	if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch && len(document.CoverageOrder) != 0 {
		document.CoverageOrder = nil
		document = document.Normalized()
	}
	transaction, err := c.beginSemanticPlanningMutation(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	changed, err := func(aggregate RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
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
				advanced, advanceErr := advanceRuntimeSemanticPlanBatch(ledger.Batches[idx], runtimecontract.RuntimeSemanticPlanBatchDocumentComplete)
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
	}(transaction.aggregate, &transaction.ledger)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanningMutation(ctx, transaction, changed)
}

func (c *planningTransactionComponent) mergeSemanticPlanBatchDocuments(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, merged runtimecontract.RuntimeSemanticPlanDocument) (RuntimeAggregate, error) {
	transaction, err := c.beginSemanticPlanningMutation(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	changed, err := func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
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
	}(transaction.aggregate, &transaction.ledger)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanningMutation(ctx, transaction, changed)
}

func (c *planningTransactionComponent) mergeSemanticPlanBatchFeasibilities(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, values []runtimecontract.RuntimeSemanticProofFeasibility) (RuntimeAggregate, error) {
	transaction, err := c.beginSemanticPlanningMutation(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	changed, err := func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
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
	}(transaction.aggregate, &transaction.ledger)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanningMutation(ctx, transaction, changed)
}

func refreshableRuntimeSemanticFeasibility(values []runtimecontract.RuntimeSemanticProofFeasibility) (runtimecontract.RuntimeSemanticProofFeasibility, bool) {
	for _, value := range values {
		if value.Status == runtimecontract.RuntimeSemanticFeasibilityStale ||
			(value.OwnerFactProspectiveSatisfactionObservation != nil && value.OwnerFactProspectiveSatisfactionObservation.Stale != nil) {
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

func runtimeSemanticDependencyCatalogForOwner(manifest runtimecontract.RuntimeSemanticPlanManifest, ownerModuleID string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	for _, catalog := range manifest.Normalized().OwnerFactDependencyCatalogs {
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
func (c *planningTransactionComponent) startSemanticPlanRequest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, manifest runtimecontract.RuntimeSemanticPlanManifest, stablePrefixHash, logicalRequestID string) (RuntimeAggregate, error) {
	manifest = manifest.Normalized()
	stablePrefixHash = strings.TrimSpace(stablePrefixHash)
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	if stablePrefixHash == "" || logicalRequestID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	transaction, err := c.beginSemanticPlanningMutation(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	changed, err := func(aggregate RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
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
	}(transaction.aggregate, &transaction.ledger)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanningMutation(ctx, transaction, changed)
}

// evaluateSemanticPlan persists one complete accepted provider document. It
// cannot create feasibility, binding, dispatch, coverage or terminal facts.
func (c *planningTransactionComponent) evaluateSemanticPlan(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, logicalRequestID string, document runtimecontract.RuntimeSemanticPlanDocument) (RuntimeAggregate, error) {
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	document = canonicalRuntimeSemanticPlanDocument(document)
	transaction, err := c.beginSemanticPlanningMutation(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	changed, err := evaluateSemanticPlanMutation(transaction.aggregate, &transaction.ledger, logicalRequestID, document)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanningMutation(ctx, transaction, changed)
}

func canonicalRuntimeSemanticPlanDocument(document runtimecontract.RuntimeSemanticPlanDocument) runtimecontract.RuntimeSemanticPlanDocument {
	document = document.Normalized()
	// Proof responses are focus-local and cannot carry a second global order.
	if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch && len(document.CoverageOrder) != 0 {
		document.CoverageOrder = nil
		document = document.Normalized()
	}
	return document
}

func evaluateSemanticPlanMutation(aggregate RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger, logicalRequestID string, document runtimecontract.RuntimeSemanticPlanDocument) (bool, error) {
	documentHash := runtimecontract.StableRuntimeSemanticPlanDocumentHash(document)
	if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch && document.BatchID == "" {
		return false, fmt.Errorf("%w: semantic proof document requires active batch identity", common.ErrConflict)
	}
	if semanticPlanDocumentReplay(*ledger, logicalRequestID, document, documentHash) {
		return false, nil
	}
	if err := validateSemanticPlanDocumentRequest(*ledger, logicalRequestID, document); err != nil {
		return false, err
	}
	if err := document.ValidateAgainstManifest(*ledger.Manifest); err != nil {
		return false, err
	}
	if err := validateSemanticPlanDocumentPhase(*ledger, document); err != nil {
		return false, err
	}
	if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		applySemanticCoverageOrderDocument(ledger, document, documentHash)
		return true, nil
	}
	if document.BatchID == "" {
		return false, fmt.Errorf("%w: semantic proof document has no active batch", common.ErrConflict)
	}
	return applySemanticProofBatchDocument(ledger, document, documentHash)
}

func semanticPlanDocumentReplay(ledger runtimecontract.RuntimeSemanticPlanningLedger, logicalRequestID string, document runtimecontract.RuntimeSemanticPlanDocument, documentHash string) bool {
	if ledger.LogicalRequestID != logicalRequestID {
		return false
	}
	if document.BatchID == "" && ledger.DocumentHash == documentHash {
		return true
	}
	for _, existing := range ledger.BatchDocuments {
		if ledger.Phase == runtimecontract.RuntimeSemanticPlanningCorrectionRequired && existing.BatchID == ledger.ActiveBatchID {
			// The active document is the rejected attempt-0 baseline, not an
			// attempt-1 provider replay. It must pass the bounded-delta fence.
			continue
		}
		if existing.BatchID == document.BatchID && runtimecontract.StableRuntimeSemanticPlanDocumentHash(existing) == documentHash {
			return true
		}
	}
	return false
}

func validateSemanticPlanDocumentRequest(ledger runtimecontract.RuntimeSemanticPlanningLedger, logicalRequestID string, _ runtimecontract.RuntimeSemanticPlanDocument) error {
	validPhase := ledger.Phase == runtimecontract.RuntimeSemanticPlanningOrderRequestOpen ||
		ledger.Phase == runtimecontract.RuntimeSemanticPlanningRequestOpen ||
		ledger.Phase == runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen ||
		ledger.Phase == runtimecontract.RuntimeSemanticPlanningCorrectionRequired
	if !validPhase || ledger.Manifest == nil || ledger.LogicalRequestID != logicalRequestID {
		return fmt.Errorf("%w: semantic plan document has no matching request", common.ErrConflict)
	}
	return nil
}

func validateSemanticPlanDocumentPhase(ledger runtimecontract.RuntimeSemanticPlanningLedger, document runtimecontract.RuntimeSemanticPlanDocument) error {
	switch ledger.Phase {
	case runtimecontract.RuntimeSemanticPlanningOrderRequestOpen, runtimecontract.RuntimeSemanticPlanningRequestOpen:
		if document.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder || document.BatchID != "" {
			return fmt.Errorf("%w: initial semantic request requires coverage-order document", common.ErrConflict)
		}
	case runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen:
		if document.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch || document.BatchID == "" || document.BatchID != ledger.ActiveBatchID {
			return fmt.Errorf("%w: proof request document is not bound to active batch", common.ErrConflict)
		}
	case runtimecontract.RuntimeSemanticPlanningCorrectionRequired:
		return validateSemanticCorrectionDocumentIdentity(ledger, document)
	default:
		return fmt.Errorf("%w: semantic plan document cannot be evaluated from phase %q", common.ErrConflict, ledger.Phase)
	}
	return nil
}

func validateSemanticCorrectionDocumentIdentity(ledger runtimecontract.RuntimeSemanticPlanningLedger, document runtimecontract.RuntimeSemanticPlanDocument) error {
	if ledger.ActiveBatchID == "" {
		if document.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder || document.BatchID != "" {
			return fmt.Errorf("%w: coverage-order correction has invalid document identity", common.ErrConflict)
		}
		return nil
	}
	if document.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch || document.BatchID != ledger.ActiveBatchID {
		return fmt.Errorf("%w: proof-batch correction is outside active batch", common.ErrConflict)
	}
	return nil
}

func applySemanticCoverageOrderDocument(ledger *runtimecontract.RuntimeSemanticPlanningLedger, document runtimecontract.RuntimeSemanticPlanDocument, documentHash string) {
	order := document.Clone()
	ledger.CoverageOrderDocument = &order
	ledger.PendingCoverageIDs = append([]common.CoverageWorkItemID(nil), order.CoverageOrder...)
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningOrderAccepted
	ledger.Document = &document
	ledger.DocumentHash = documentHash
}

func applySemanticProofBatchDocument(ledger *runtimecontract.RuntimeSemanticPlanningLedger, document runtimecontract.RuntimeSemanticPlanDocument, documentHash string) (bool, error) {
	if ledger.ActiveBatchID != document.BatchID {
		return false, fmt.Errorf("%w: semantic batch document is not the active batch", common.ErrConflict)
	}
	for _, existing := range ledger.BatchDocuments {
		if existing.BatchID != document.BatchID {
			continue
		}
		if ledger.Phase == runtimecontract.RuntimeSemanticPlanningCorrectionRequired && document.BatchID == ledger.ActiveBatchID {
			if err := advanceSemanticProofBatchDocument(ledger, document.BatchID, documentHash); err != nil {
				return false, err
			}
			for idx := range ledger.BatchDocuments {
				if ledger.BatchDocuments[idx].BatchID == document.BatchID {
					ledger.BatchDocuments[idx] = document.Clone()
					break
				}
			}
			ledger.Phase = runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted
			return true, nil
		}
		if runtimecontract.StableRuntimeSemanticPlanDocumentHash(existing) == documentHash {
			return false, nil
		}
		return false, fmt.Errorf("%w: conflicting semantic batch document", common.ErrConflict)
	}
	if err := advanceSemanticProofBatchDocument(ledger, document.BatchID, documentHash); err != nil {
		return false, err
	}
	ledger.BatchDocuments = append(ledger.BatchDocuments, document.Clone())
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted
	return true, nil
}

func advanceSemanticProofBatchDocument(ledger *runtimecontract.RuntimeSemanticPlanningLedger, batchID, documentHash string) error {
	for idx := range ledger.Batches {
		if ledger.Batches[idx].BatchID != batchID {
			continue
		}
		advanced, err := advanceRuntimeSemanticPlanBatch(ledger.Batches[idx], runtimecontract.RuntimeSemanticPlanBatchDocumentComplete)
		if err != nil {
			return err
		}
		advanced.DocumentHash = documentHash
		advanced.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(advanced)
		ledger.Batches[idx] = advanced
		break
	}
	return nil
}

// requireSemanticFeasibility is the SM-2 handoff from an accepted proposal to
// a bounded owner-probe batch.
func (c *planningTransactionComponent) requireSemanticFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	transaction, err := c.beginSemanticPlanningMutation(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	changed, err := func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
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
				advanced, advanceErr := advanceRuntimeSemanticPlanBatch(ledger.Batches[idx], runtimecontract.RuntimeSemanticPlanBatchFeasibilityPending)
				if advanceErr != nil {
					return false, advanceErr
				}
				ledger.Batches[idx] = advanced
				break
			}
		}
		ledger.FeasibilityBatchID = runtimecontract.StableRuntimeSemanticFeasibilityBatchID(*ledger.Manifest, feasibilityDocument)
		return true, nil
	}(transaction.aggregate, &transaction.ledger)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanningMutation(ctx, transaction, changed)
}

// observeSemanticFeasibility persists one canonical owner's factual result.
// Conflicting duplicates are rejected instead of replacing owner history.
func (c *planningTransactionComponent) observeSemanticFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, batchID string, observation runtimecontract.RuntimeSemanticProofFeasibility) (RuntimeAggregate, error) {
	batchID = strings.TrimSpace(batchID)
	transaction, err := c.beginSemanticPlanningMutation(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	catalogRetired := false
	if transaction.ledger.Manifest != nil {
		gaps, current := semanticPlanningCatalogsCurrent(transaction.aggregate, *transaction.ledger.Manifest)
		if !current {
			if transaction.aggregate.ActiveDispatch != nil {
				return RuntimeAggregate{}, fmt.Errorf("%w: stale semantic feasibility cannot retire an active dispatch", common.ErrConflict)
			}
			if err := retireSemanticPlanningGeneration(&transaction.aggregate, runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated, gaps); err != nil {
				return RuntimeAggregate{}, err
			}
			facts, decision, reduceErr := ReduceRuntimeStateWithDirective(transaction.aggregate.Facts, nil, transaction.aggregate.CoverageProofLineage)
			if reduceErr != nil {
				return RuntimeAggregate{}, reduceErr
			}
			transaction.aggregate.Facts, transaction.aggregate.Decision = facts, decision
			transaction.ledger = transaction.aggregate.SemanticPlanning.Clone()
			catalogRetired = true
		}
	}
	changed := catalogRetired
	if !catalogRetired {
		changed, err = func(aggregate RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
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
				if existing.ProposalHash != observation.ProposalHash {
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
			proposal, found := runtimecontract.RuntimeSemanticProofProposalForHash(feasibilityDocument, observation.ProposalHash)
			if !found {
				return false, fmt.Errorf("%w: semantic feasibility references unknown proof", common.ErrConflict)
			}
			if err := observation.ValidateCompositeForProposal(*ledger.Manifest, proposal); err != nil {
				return false, err
			}
			if err := validateRuntimeSemanticProspectiveObservationRequest(aggregate, *ledger.Manifest, proposal, batchID, observation); err != nil {
				return false, err
			}
			if gaps := SemanticProspectiveFeasibilityFreshnessGaps(aggregate, *ledger.Manifest, observation); len(gaps) != 0 {
				return false, fmt.Errorf("%w: prospective semantic feasibility observation is no longer current", common.ErrConflict)
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
					factProposal, found := runtimecontract.RuntimeSemanticProofProposalForHash(feasibilityDocument, fact.ProposalHash)
					if !found || fact.Status != runtimecontract.RuntimeSemanticFeasibilityFeasible || fact.ValidateForProposal(*ledger.Manifest, factProposal) != nil {
						allFeasible = false
						break
					}
				}
			}
			if allFeasible && ledger.Phase == runtimecontract.RuntimeSemanticPlanningFeasibilityRequired {
				ledger.Phase = runtimecontract.RuntimeSemanticPlanningProofBatchFeasibilityComplete
				for idx := range ledger.Batches {
					if ledger.Batches[idx].BatchID == ledger.ActiveBatchID {
						advanced, advanceErr := advanceRuntimeSemanticPlanBatch(ledger.Batches[idx], runtimecontract.RuntimeSemanticPlanBatchFeasibilityComplete)
						if advanceErr != nil {
							return false, advanceErr
						}
						advanced, advanceErr = advanceRuntimeSemanticPlanBatch(advanced, runtimecontract.RuntimeSemanticPlanBatchAccepted)
						if advanceErr != nil {
							return false, advanceErr
						}
						ledger.Batches[idx] = advanced
						break
					}
				}
			}
			return true, nil
		}(transaction.aggregate, &transaction.ledger)
	}
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanningMutation(ctx, transaction, changed)
}

func validateRuntimeSemanticProspectiveObservationRequest(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeSemanticPlanManifest, proposal runtimecontract.RuntimeSemanticProofProposal, batchID string, feasibility runtimecontract.RuntimeSemanticProofFeasibility) error {
	feasibility = feasibility.Normalized()
	if feasibility.OwnerFactProspectiveSatisfactionObservation == nil || feasibility.ProspectiveWorkspaceManifestDelta == nil {
		return nil
	}
	var acquisition *runtimecontract.RuntimeSemanticOwnerFactAcquisition
	for _, obligation := range manifest.Normalized().ProofObligations {
		if obligation.CoverageWorkItemID == proposal.CoverageWorkItemID && obligation.OwnerFactAcquisition != nil {
			value := obligation.OwnerFactAcquisition.Clone()
			acquisition = &value
			break
		}
	}
	if acquisition == nil {
		return fmt.Errorf("%w: prospective semantic observation has no sealed owner-fact acquisition", common.ErrConflict)
	}
	catalog, found := runtimeSemanticDependencyCatalogForOwner(manifest, acquisition.Requirement.OwnerModuleID)
	if !found {
		return fmt.Errorf("%w: prospective semantic observation has no dependency catalog", common.ErrConflict)
	}
	observation, found := CurrentSemanticCatalogObservation(aggregate, catalog)
	if !found {
		return fmt.Errorf("%w: prospective semantic dependency observation is not current", common.ErrConflict)
	}
	request := runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		FeasibilityBatchID:                 strings.TrimSpace(batchID),
		Manifest:                           manifest,
		Proposal:                           proposal,
		Requirement:                        acquisition.Requirement,
		ProspectiveSatisfactionContractRef: acquisition.ProspectiveSatisfactionContractRef,
		OwnerCatalog:                       catalog,
		CatalogObservation:                 &observation,
		ValidationRecipes:                  runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(aggregate.Facts.Activation.EvidencePolicies),
		ProspectiveWorkspaceManifestDelta:  feasibility.ProspectiveWorkspaceManifestDelta.Clone(),
	}
	if err := feasibility.OwnerFactProspectiveSatisfactionObservation.ValidateForRequest(request); err != nil {
		return fmt.Errorf("%w: prospective semantic observation request identity: %v", common.ErrConflict, err)
	}
	return nil
}

// requireSemanticPlanCorrection records the complete attempt-0 rejection and
// opens exactly one correction identity under the same manifest/prefix.
func (c *planningTransactionComponent) requireSemanticPlanCorrection(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stablePrefixHash, logicalRequestID string, expected runtimecontract.RuntimeExpectedDocumentContract, rejection runtimecontract.RuntimeOriginalDiagnostic) (RuntimeAggregate, error) {
	stablePrefixHash = strings.TrimSpace(stablePrefixHash)
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	if stablePrefixHash == "" || logicalRequestID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if err := rejection.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	transaction, err := c.beginSemanticPlanningMutation(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	changed, err := func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if ledger.Manifest == nil {
			return false, fmt.Errorf("%w: semantic correction has no sealed manifest", common.ErrConflict)
		}
		expected = expected.Clone()
		expected.CorrectionAttempt = 1
		expected.LogicalRequestID = ""
		expected.Hash = ""
		observation, observationErr := requestcontent.ProjectRuntimeSemanticCorrectionObservation(&rejection)
		if observationErr != nil {
			return false, observationErr
		}
		directive, deriveErr := runtimecontract.DeriveRuntimeSemanticCorrectionDirective(*ledger.Manifest, expected, stablePrefixHash, *observation)
		if deriveErr != nil {
			return false, deriveErr
		}
		if err := directive.ValidateAgainstObservation(*ledger.Manifest, *observation); err != nil {
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
		if ledger.Phase != runtimecontract.RuntimeSemanticPlanningRequestOpen && ledger.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen {
			return false, fmt.Errorf("%w: semantic plan correction has no matching attempt 0", common.ErrConflict)
		}
		if ledger.Attempt != 0 || ledger.StablePrefixHash != stablePrefixHash {
			return false, fmt.Errorf("%w: semantic plan correction has no matching attempt 0", common.ErrConflict)
		}
		ledger.Phase = runtimecontract.RuntimeSemanticPlanningCorrectionRequired
		ledger.Attempt = 1
		ledger.LogicalRequestID = logicalRequestID
		value := directive.Clone()
		ledger.CorrectionDirective = &value
		ledger.Rejections = append(ledger.Rejections, *rejection.CloneOriginal())
		return true, nil
	}(transaction.aggregate, &transaction.ledger)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return c.commitSemanticPlanningMutation(ctx, transaction, changed)
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
func semanticPendingFocuses(facts runtimecontract.RuntimeFacts, pending []common.CoverageWorkItemID, sealed []runtimecontract.CoverageFocus, manifest runtimecontract.RuntimeSemanticPlanManifest) ([]runtimecontract.CoverageFocus, error) {
	sealedCoverage := make(map[common.CoverageWorkItemID]struct{})
	for _, focus := range sealed {
		for _, id := range focus.CoverageWorkItemIDs {
			sealedCoverage[id] = struct{}{}
		}
	}
	eligible := make(map[common.CoverageWorkItemID]struct{})
	for _, id := range runtimecontract.RuntimeCWIsEligibleForPlanning(facts) {
		eligible[id] = struct{}{}
	}
	// A validation-failure repair generation is deliberately CWI-local. Its
	// manifest cannot absorb unrelated Definition proofs merely because they
	// are concurrently eligible in the canonical queue. A normal cumulative
	// multi-batch manifest may also retain an already-settled predecessor as
	// immutable order/proof lineage, so scope by intersection rather than
	// treating that audit row as stale current eligibility.
	manifest = manifest.Normalized()
	manifestCoverage := make(map[common.CoverageWorkItemID]struct{}, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		if _, current := eligible[item.ID]; current {
			manifestCoverage[item.ID] = struct{}{}
		}
	}
	eligible = manifestCoverage
	sources := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus)
	for _, focus := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		for _, id := range focus.CoverageWorkItemIDs {
			if _, found := sources[id]; !found {
				sources[id] = focus.Normalized()
			}
		}
	}
	ret := make([]runtimecontract.CoverageFocus, 0, len(pending))
	seen := make(map[common.CoverageWorkItemID]struct{}, len(pending))
	for _, id := range pending {
		if _, alreadySealed := sealedCoverage[id]; alreadySealed {
			continue
		}
		if _, allowed := eligible[id]; !allowed {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("runtime semantic pending coverage %q is duplicated", id)
		}
		seen[id] = struct{}{}
		source, found := sources[id]
		if !found {
			return nil, fmt.Errorf("runtime semantic pending coverage %q has no bootstrap focus", id)
		}
		ret = append(ret, runtimeCWIPendingFocus(source, id))
	}
	for id := range eligible {
		if _, alreadySealed := sealedCoverage[id]; alreadySealed {
			continue
		}
		if _, found := seen[id]; !found {
			return nil, fmt.Errorf("runtime semantic pending lineage omits eligible coverage %q", id)
		}
	}
	return ret, nil
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
	if ledger.CoverageOrderDocument == nil {
		return fmt.Errorf("runtime semantic next focus has no retained coverage-order lineage")
	}
	// Settling the current CWI can unlock a dependent CWI that was correctly
	// absent from the immutable manifest/order. Retire that generation inside
	// the same owner-local CAS that advanced SM-2; a later projection event must
	// never be required to repair already-stale semantic authority.
	_, closureChanged, err := currentRuntimeCWIOrderFromSemanticLineage(aggregate.Facts, *ledger.Manifest, ledger.CoverageOrderDocument.CoverageOrder)
	if err != nil {
		return err
	}
	if len(closureChanged) != 0 {
		return retireSemanticPlanningGenerationForEligibilityChange(aggregate, closureChanged)
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

// semanticFailureClassForFailureCode lets the Orchestrator retain a bounded
// terminal outcome selected by its own persisted budget (for example refresh
// exhaustion) while preserving the complete producer diagnostic unchanged.
// The outer code is never copied into OriginalDiagnostic; it only supplies the
// stable class for these explicit owner-local terminal transitions.
func semanticFailureClassForFailureCode(code string, _ runtimecontract.RuntimeOriginalDiagnostic) runtimecontract.FailureClass {
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
	case "semantic_provider_body_idle_timeout", "semantic_provider_transport_exhausted":
		return runtimecontract.FailureClassProviderTransient
	case "semantic_provider_authentication":
		return runtimecontract.FailureClassProviderAuthentication
	case "semantic_provider_configuration":
		return runtimecontract.FailureClassProviderConfiguration
	case "semantic_provider_contract_error", "semantic_provider_failure", "semantic_provider_outcome_missing",
		"semantic_provider_outcome_invalid", "semantic_provider_document_missing":
		return runtimecontract.FailureClassProviderProtocol
	case "semantic_owner_feasibility_infeasible":
		return runtimecontract.FailureClassCandidateInfeasible
	case "semantic_provider_ceiling_missing", "semantic_catalog_stale", "semantic_owner_catalog_missing", "semantic_owner_prerequisite_required":
		return runtimecontract.FailureClassCapabilityUnavailable
	case "runtime_contract_defect", "semantic_context_integrity", "semantic_manifest_invalid", "semantic_request_context_invalid",
		"semantic_expected_document_invalid", "semantic_request_identity_invalid", "semantic_request_session_mismatch",
		"semantic_request_terminal", "semantic_expected_document_missing", "semantic_correction_lineage_invalid", "semantic_document_framing_mismatch",
		"semantic_feasibility_context_invalid", "semantic_batch_identity_missing", "semantic_batch_document_missing",
		"semantic_owner_probe_invalid", "semantic_probe_proposal_invalid", "prospective_satisfaction_contract_violated":
		return runtimecontract.FailureClassRuntimeContractMisalignment
	}
	// Unknown outer codes fail closed. The nested producer diagnostic is audit
	// data and can never select the Runtime failure class.
	return runtimecontract.FailureClassRuntimeContractMisalignment
}
