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
	ledger.CorrectionHash = ""
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
		if ledger.Phase == runtimecontract.RuntimeSemanticPlanningRequestOpen && ledger.Manifest != nil && ledger.Manifest.Hash == manifest.Hash && ledger.StablePrefixHash == stablePrefixHash && ledger.LogicalRequestID == logicalRequestID && ledger.Attempt == 0 {
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
		if ledger.Phase == runtimecontract.RuntimeSemanticPlanningProposalAccepted && ledger.LogicalRequestID == logicalRequestID && ledger.DocumentHash == documentHash {
			return false, nil
		}
		if (ledger.Phase != runtimecontract.RuntimeSemanticPlanningRequestOpen && ledger.Phase != runtimecontract.RuntimeSemanticPlanningCorrectionRequired) || ledger.Manifest == nil || ledger.LogicalRequestID != logicalRequestID {
			return false, fmt.Errorf("%w: semantic plan document has no matching request", common.ErrConflict)
		}
		if err := document.ValidateAgainstManifest(*ledger.Manifest); err != nil {
			return false, err
		}
		ledger.Phase = runtimecontract.RuntimeSemanticPlanningProposalAccepted
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
		if ledger.Phase != runtimecontract.RuntimeSemanticPlanningProposalAccepted || ledger.Manifest == nil || ledger.Document == nil {
			return false, fmt.Errorf("%w: semantic feasibility has no accepted proposal", common.ErrConflict)
		}
		ledger.Phase = runtimecontract.RuntimeSemanticPlanningFeasibilityRequired
		ledger.FeasibilityBatchID = runtimecontract.StableRuntimeSemanticFeasibilityBatchID(*ledger.Manifest, *ledger.Document)
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
		var proposal *runtimecontract.RuntimeSemanticProofProposal
		for idx := range ledger.Document.Proposals {
			if ledger.Document.Proposals[idx].CoverageWorkItemID == observation.CoverageWorkItemID {
				value := ledger.Document.Proposals[idx]
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
		return true, nil
	})
}

// requireSemanticPlanCorrection records the complete attempt-0 rejection and
// opens exactly one correction identity under the same manifest/prefix.
func (c *RuntimeController) requireSemanticPlanCorrection(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, stablePrefixHash, logicalRequestID, correctionHash string, rejection runtimecontract.RuntimeOriginalDiagnostic) (RuntimeAggregate, error) {
	stablePrefixHash = strings.TrimSpace(stablePrefixHash)
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	correctionHash = strings.TrimSpace(correctionHash)
	if stablePrefixHash == "" || logicalRequestID == "" || correctionHash == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if err := rejection.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if correctionHash != runtimecontract.StableRuntimeSemanticPlanCorrectionHash(rejection) {
		return RuntimeAggregate{}, fmt.Errorf("%w: semantic correction identity does not match rejection", common.ErrConflict)
	}
	return c.commitSemanticPlanning(ctx, workspaceID, runID, func(_ RuntimeAggregate, ledger *runtimecontract.RuntimeSemanticPlanningLedger) (bool, error) {
		if ledger.Phase == runtimecontract.RuntimeSemanticPlanningCorrectionRequired && ledger.LogicalRequestID == logicalRequestID && ledger.CorrectionHash == correctionHash {
			return false, nil
		}
		if ledger.Phase != runtimecontract.RuntimeSemanticPlanningRequestOpen || ledger.Attempt != 0 || ledger.Manifest == nil || ledger.StablePrefixHash != stablePrefixHash {
			return false, fmt.Errorf("%w: semantic plan correction has no matching attempt 0", common.ErrConflict)
		}
		ledger.Phase = runtimecontract.RuntimeSemanticPlanningCorrectionRequired
		ledger.Attempt = 1
		ledger.LogicalRequestID = logicalRequestID
		ledger.CorrectionHash = correctionHash
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
