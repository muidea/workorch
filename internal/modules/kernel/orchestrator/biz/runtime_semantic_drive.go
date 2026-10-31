package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/requestcontent"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// driveRuntimeSemanticPlanning is the SM-4 recovery dispatcher for the
// persisted SM-2/SM-3 semantic sub-ledger. It never mutates the aggregate
// directly: every phase change is submitted through the Orchestrator-private
// internal-event dispatcher.
func (m *Biz) driveRuntimeSemanticPlanning(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	switch ledger.Phase {
	case runtimecontract.RuntimeSemanticPlanningRequired:
		return m.driveRuntimeSemanticRequired(ctx, aggregate)
	case runtimecontract.RuntimeSemanticPlanningOrderRequestOpen, runtimecontract.RuntimeSemanticPlanningRequestOpen, runtimecontract.RuntimeSemanticPlanningCorrectionRequired:
		return m.requestRuntimeSemanticPlanDocument(ctx, aggregate)
	case runtimecontract.RuntimeSemanticPlanningOrderAccepted:
		return m.driveRuntimeSemanticOrderAccepted(ctx, aggregate, ledger)
	case runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen:
		return m.requestRuntimeSemanticPlanDocument(ctx, aggregate)
	case runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted:
		updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventSemanticFeasibilityRequired, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{}})
		return updated.Aggregate, err
	case runtimecontract.RuntimeSemanticPlanningProofBatchFeasibilityComplete:
		return m.driveRuntimeSemanticFeasibilityComplete(ctx, aggregate, ledger)
	case runtimecontract.RuntimeSemanticPlanningFeasibilityRequired:
		return m.driveRuntimeSemanticFeasibilityRequired(ctx, aggregate, ledger)
	case runtimecontract.RuntimeSemanticPlanningSealed, runtimecontract.RuntimeSemanticPlanningFailed:
		return aggregate, nil
	default:
		return runtime.RuntimeAggregate{}, fmt.Errorf("unsupported runtime semantic planning phase %q", ledger.Phase)
	}
}

func (m *Biz) driveRuntimeSemanticRequired(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	if updated, handled, err := m.driveRuntimeImplementationWorkScope(ctx, aggregate); handled || err != nil {
		return updated, err
	}
	if retirement, found := runtime.NextRuntimeCWIBindingRetirementFact(aggregate); found {
		updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind: runtime.RuntimeInternalEventCWIBindingRetired, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{CWIBindingRetirement: &retirement},
		})
		return updated.Aggregate, err
	}
	if gap, found := runtime.NextRuntimeSemanticCatalogScopeGap(aggregate); found {
		updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind: runtime.RuntimeInternalEventSemanticCatalogScopeInvalidated, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{SemanticCatalogScopeGap: &gap},
		})
		return updated.Aggregate, err
	}
	if runtime.RuntimeSemanticPrerequisitesPendingForAggregate(aggregate) {
		return m.prepareRuntimeSemanticPrerequisites(ctx, aggregate)
	}
	if runtime.RuntimeImplementationWorkPurposePending(aggregate) && aggregate.ImplementationWork.ScopePreview == nil {
		if current, err := runtime.CurrentRuntimeImplementationWorkReservation(aggregate); err != nil {
			return runtime.RuntimeAggregate{}, err
		} else if current == nil {
			return m.failRuntimeSemanticContract(ctx, aggregate, "implementation_work_scope_missing", "implementation purpose requires complete current owner scope before a semantic generation", nil)
		}
	}
	if runtime.RuntimeImplementationWorkPurposePending(aggregate) {
		if err := runtimecontract.ValidateRuntimeImplementationWorkInputCatalogs(aggregate.Facts, aggregate.Definition.CapabilityContract); err != nil {
			return runtime.RuntimeAggregate{}, err
		}
	}
	if aggregate.Facts.Resources.ProviderCeiling == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_ceiling_missing", "semantic planning has no sealed provider ceiling prerequisite", nil)
	}
	eligibleCoverage := runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts)
	if len(eligibleCoverage) == 0 {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_no_eligible_cwi", "semantic planning was requested without an eligible canonical CWI", nil)
	}
	repairInput, repairRequired, repairErr := runtime.CurrentRuntimeValidationFailureRepairGenerationInput(aggregate)
	if repairErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_validation_failure_repair_invalid", repairErr.Error(), nil)
	}
	identity := semanticplan.ManifestIdentity{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Definition: aggregate.Definition, DefinitionSeed: aggregate.Facts.DefinitionSeed,
		DefinitionFocuses: aggregate.Facts.DefinitionFocuses,
		Execution:         aggregate.Facts.Activation.ExecutionContract, Admissibilities: aggregate.Facts.Resources.OperationAdmissibilities,
		EligibleCoverage: eligibleCoverage,
	}
	identity.InputReviewInformation, repairErr = runtime.RuntimeInputReviewInformation(aggregate)
	if repairErr != nil {
		return runtime.RuntimeAggregate{}, repairErr
	}
	if repairRequired {
		// A repair purpose is CWI-local and cannot replace unrelated eligible
		// Definition proofs in the same generation.
		identity.EligibleCoverage = []common.CoverageWorkItemID{repairInput.CoverageWorkItemID}
		identity.ValidationFailureRepair = &repairInput
	}
	if current, workErr := runtime.CurrentRuntimeImplementationWorkReservation(aggregate); workErr != nil {
		return runtime.RuntimeAggregate{}, workErr
	} else if current != nil {
		if repairRequired || current.ScopeObservation == nil {
			return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic work scope is unsettled or exact repair is current", common.ErrConflict)
		}
		identity.ImplementationWork = current
		identity.EligibleCoverage = []common.CoverageWorkItemID{current.CoverageWorkItemID}
	}
	if preview := aggregate.ImplementationWork.ScopePreview; preview != nil {
		if repairRequired || preview.Observation == nil {
			return runtime.RuntimeAggregate{}, fmt.Errorf("%w: implementation purpose preview is incomplete or conflicts with exact repair", common.ErrConflict)
		}
		identity.ImplementationPurposePreview = preview
		identity.ImplementationWorkRounds = aggregate.ImplementationWork.Rounds
		identity.EligibleCoverage = []common.CoverageWorkItemID{preview.Request.CoverageWorkItemID}
	}
	manifest, err := semanticplan.CompileManifest(identity)
	if err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_manifest_invalid", err.Error(), nil)
	}
	stablePrefixHash := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	// This seals request intent, not a provider session. The shared semantic
	// request path must pass rendered preflight before creating that session.
	requestIdentityHash, identityErr := runtime.SemanticRequestIdentityHash(aggregate.SemanticPlanning)
	if identityErr != nil {
		return runtime.RuntimeAggregate{}, identityErr
	}
	logicalRequestID := runtimeSemanticLogicalRequestID(aggregate, stablePrefixHash, manifest.Hash, requestIdentityHash)
	started, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanRequestStarted, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: stablePrefixHash, LogicalRequestID: logicalRequestID},
	})
	return started.Aggregate, err
}

func (m *Biz) driveRuntimeSemanticOrderAccepted(ctx context.Context, aggregate runtime.RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger) (runtime.RuntimeAggregate, error) {
	if ledger.Phase != runtimecontract.RuntimeSemanticPlanningOrderAccepted || ledger.Manifest == nil || ledger.CoverageOrderDocument == nil {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic order-accepted drive requires complete generation identity", common.ErrConflict)
	}
	expectedGenerationID := runtimecontract.StableRuntimeSemanticGenerationID(ledger.Manifest.Hash, ledger.LogicalRequestID)
	expectedOrderHash := runtimecontract.StableRuntimeSemanticPlanDocumentHash(*ledger.CoverageOrderDocument)
	if expectedGenerationID == "" || expectedOrderHash == "" {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic order-accepted drive has incomplete immutable lineage", common.ErrConflict)
	}
	projected, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventEligibleFocusProjected, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{SemanticOrderManifestHash: ledger.Manifest.Hash, SemanticCoverageOrder: append([]common.CoverageWorkItemID(nil), ledger.CoverageOrderDocument.CoverageOrder...)},
	})
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	aggregate = projected.Aggregate
	ledger = aggregate.SemanticPlanning.Clone()
	// EligibleFocusProjected is an owner-private SM-2 event and may retire the
	// generation when a normal Definition closure has changed. Hand that new
	// phase back to the outer SM-4 drive immediately; never continue with the
	// pre-event manifest/order snapshot (or dereference the retired nil values).
	if ledger.Phase != runtimecontract.RuntimeSemanticPlanningOrderAccepted {
		return aggregate, nil
	}
	if ledger.Manifest == nil || ledger.CoverageOrderDocument == nil ||
		runtimecontract.StableRuntimeSemanticGenerationID(ledger.Manifest.Hash, ledger.LogicalRequestID) != expectedGenerationID ||
		runtimecontract.StableRuntimeSemanticPlanDocumentHash(*ledger.CoverageOrderDocument) != expectedOrderHash {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic generation changed during eligible focus projection", common.ErrConflict)
	}
	// EligibleFocusProjected may advance the current CWI within an accepted
	// order and project a new focus-scoped TargetSelection prerequisite. That
	// owner fact is part of the authority transaction used to derive the next
	// proof batch's immutable RequestLogicalID, so it must settle before batch
	// compilation—not merely before the physical provider call.
	if runtime.RuntimeSemanticPrerequisitesPendingForAggregate(aggregate) {
		return m.prepareRuntimeSemanticPrerequisites(ctx, aggregate)
	}
	batchRequestKey := fmt.Sprintf("semantic-proof-batch-%d", ledger.NextBatchSequence)
	logicalRequestID := runtimeSemanticLogicalRequestID(aggregate, ledger.StablePrefixHash, ledger.Manifest.Hash, runtime.SemanticRequestIdentityHashForKey(ledger, batchRequestKey))
	return m.compileRuntimeSemanticPlanBatch(ctx, aggregate, logicalRequestID, 4)
}

func (m *Biz) driveRuntimeSemanticFeasibilityComplete(ctx context.Context, aggregate runtime.RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger) (runtime.RuntimeAggregate, error) {
	activeBatch := activeRuntimeSemanticBatch(ledger)
	if activeBatch == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_batch_identity_missing", "completed semantic proof batch has no active batch identity", nil)
	}
	if activeBatch.State == runtimecontract.RuntimeSemanticPlanBatchFeasibilityComplete {
		advanced, err := m.advanceRuntimeSemanticPlanBatch(ctx, aggregate, *activeBatch, runtimecontract.RuntimeSemanticPlanBatchAccepted)
		if err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		aggregate, ledger = advanced, advanced.SemanticPlanning.Clone()
		if refreshed := activeRuntimeSemanticBatch(ledger); refreshed != nil {
			activeBatch = refreshed
		}
	}
	if !runtimeSemanticBatchFeasibilityResultExists(ledger.BatchFeasibilityResults, activeBatch.BatchID) {
		return m.observeCompletedRuntimeSemanticBatch(ctx, aggregate, ledger, *activeBatch)
	}
	merged, err := m.mergeRuntimeSemanticPlanFocusBatchDocument(ctx, aggregate, *activeBatch)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	sealed, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventSemanticContractSealed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: merged.Facts.WorkspaceID, RunID: merged.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{}})
	return sealed.Aggregate, err
}

func activeRuntimeSemanticBatch(ledger runtimecontract.RuntimeSemanticPlanningLedger) *runtimecontract.RuntimeSemanticPlanBatch {
	for idx := range ledger.Batches {
		if ledger.Batches[idx].BatchID == ledger.ActiveBatchID {
			value := ledger.Batches[idx].Clone()
			return &value
		}
	}
	return nil
}

func runtimeSemanticBatchFeasibilityResultExists(results []runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult, batchID string) bool {
	for _, result := range results {
		if result.BatchID == batchID {
			return true
		}
	}
	return false
}

func (m *Biz) observeCompletedRuntimeSemanticBatch(ctx context.Context, aggregate runtime.RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger, batch runtimecontract.RuntimeSemanticPlanBatch) (runtime.RuntimeAggregate, error) {
	document, found := runtimeSemanticBatchDocument(ledger.BatchDocuments, batch.BatchID)
	if !found {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_batch_document_missing", "completed semantic proof batch has no persisted document", nil)
	}
	result := runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{
		BatchID: batch.BatchID, BatchHash: batch.Hash,
		DocumentHash:  runtimecontract.StableRuntimeSemanticPlanDocumentHash(document),
		Feasibilities: append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), ledger.Feasibilities...),
	}
	result.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
	return m.observeRuntimeSemanticPlanBatchFeasibility(ctx, aggregate, result)
}

func (m *Biz) driveRuntimeSemanticFeasibilityRequired(ctx context.Context, aggregate runtime.RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger) (runtime.RuntimeAggregate, error) {
	if ledger.ActiveBatchID == "" && ledger.MergedDocument != nil {
		sealed, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind: runtime.RuntimeInternalEventSemanticContractSealed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{},
		})
		return sealed.Aggregate, err
	}
	return m.driveRuntimeSemanticFeasibility(ctx, aggregate)
}

func runtimeSemanticBatchDocument(values []runtimecontract.RuntimeSemanticPlanDocument, batchID string) (runtimecontract.RuntimeSemanticPlanDocument, bool) {
	return semanticplan.DocumentByBatchID(values, batchID)
}

func runtimeSemanticPrerequisitesPending(values []runtimecontract.RuntimePrerequisite) bool {
	return runtime.SemanticPlanningPrerequisitesPending(values)
}

func runtimeSemanticLogicalRequestID(aggregate runtime.RuntimeAggregate, stablePrefixHash, manifestHash, correctionHash string) string {
	return runtimecontract.StableRuntimeCandidateRequestSessionID(
		runtimecontract.RuntimeProviderDocumentSemanticPlan,
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		aggregate.Decision.AuthorityTransaction(),
		stablePrefixHash,
		manifestHash,
		correctionHash,
	)
}

func (m *Biz) requestRuntimeSemanticPlanDocument(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Manifest == nil || ledger.LogicalRequestID == "" || aggregate.Facts.Resources.ProviderCeiling == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_context_invalid", "semantic request is missing manifest, request identity, or provider ceiling", nil)
	}
	manifest := ledger.Manifest.Clone()
	expected, expectedErr := runtime.SemanticExpectedDocument(aggregate)
	if expectedErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_expected_document_invalid", expectedErr.Error(), nil)
	}
	// Check immutable receipt provenance before opening a provider transaction.
	// Missing execution facts are a semantic contract failure, not a fabricated
	// physical call or a provider-budget settlement.
	semanticContext := runtimecontract.RuntimeSemanticPlanRequestContext{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Definition: aggregate.Definition, DefinitionSeed: aggregate.Facts.DefinitionSeed,
		Activation: aggregate.Facts.Activation, ProviderCeiling: aggregate.Facts.Resources.ProviderCeiling,
		PlanStep: expected.PlanStep, BatchID: expected.BatchID, BatchSequence: expected.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), expected.BatchCoverageWorkItemIDs...),
	}
	feedback, feedbackErr := runtime.SemanticExecutionFeedback(aggregate, semanticContext, manifest)
	if feedbackErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_context_invalid", feedbackErr.Error(), nil)
	}
	semanticContext.ExecutionFeedback = feedback
	progress, progressErr := runtimecontract.BuildRuntimeSemanticCoverageProgress(semanticContext, manifest, aggregate.Facts, aggregate.StateRevision)
	if progressErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_context_invalid", progressErr.Error(), nil)
	}
	semanticContext.CoverageProgress = progress
	history, historyErr := runtime.SemanticExecutionHistory(aggregate, semanticContext, manifest)
	if historyErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_context_invalid", historyErr.Error(), nil)
	}
	semanticContext.ExecutionHistory = history
	budget, budgetErr := runtime.SemanticExecutionBudget(aggregate, semanticContext, manifest)
	if budgetErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_context_invalid", budgetErr.Error(), nil)
	}
	semanticContext.ExecutionBudget = budget
	workContext, workErr := runtime.SemanticWorkContext(aggregate, manifest)
	if workErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_work_context_invalid", workErr.Error(), nil)
	}
	semanticContext.WorkContext = workContext
	requestIdentityHash, identityErr := runtime.SemanticRequestIdentityHash(ledger)
	if identityErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_identity_invalid", identityErr.Error(), nil)
	}
	if runtimeSemanticLogicalRequestID(aggregate, ledger.StablePrefixHash, manifest.Hash, requestIdentityHash) != ledger.LogicalRequestID {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic request intent differs from current authority", common.ErrConflict)
	}
	previous, correction, correctionErr := runtimeSemanticCorrectionContext(ledger)
	if correctionErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_correction_lineage_invalid", correctionErr.Error(), nil)
	}
	correctionObservation, observationErr := requestcontent.ProjectRuntimeSemanticCorrectionObservation(previous)
	if observationErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_correction_observation_invalid", observationErr.Error(), previous)
	}
	if _, contentErr := requestcontent.BuildSemanticPlanRequestContent(requestcontent.SemanticPlanRequestInput{
		Manifest: manifest, StablePrefixHash: ledger.StablePrefixHash, Attempt: ledger.Attempt,
		PreviousRejection: previous, CorrectionDirective: correction,
		RemainingRecoveryAttempt: expected.MaxRecoveryAttempts - ledger.Attempt,
	}); contentErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_context_invalid", contentErr.Error(), nil)
	}
	if contractErr := runtimecontract.ValidateRuntimeSemanticRequestContract(manifest, expected.PlanStep, expected.BatchCoverageWorkItemIDs); contractErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_contract_conflict", contractErr.Error(), nil)
	}
	// All semantic ingresses seal only request intent. A persisted session is
	// therefore proof of prior preflight; ensure below still checks its exact
	// transaction, manifest, prefix, correction and expected-document identity.
	if _, persisted := runtimeCandidateRequestSession(aggregate, ledger.LogicalRequestID); !persisted {
		preflight, err := PreflightRuntimeCandidate(ctx, m.EventHub(), llmevents.PreflightRuntimeCandidateCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			DocumentKind:     runtimecontract.RuntimeProviderDocumentSemanticPlan,
			CandidateAttempt: ledger.Attempt, StablePrefixHash: ledger.StablePrefixHash,
			SemanticManifest: manifest, SemanticContext: semanticContext,
			SemanticCorrectionObservation: correctionObservation,
			SemanticCorrectionDirective:   correction,
		})
		if err != nil {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_context_integrity", err.Error(), nil)
		}
		if preflight.Feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryContextIntegrity {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_context_integrity", preflight.Feedback.OriginalDiagnostic.OriginalMessage, preflight.Feedback.OriginalDiagnostic)
		}
	}
	ensured, session, ensureErr := m.runtimeEnsureCandidateRequestWithExpected(ctx, aggregate, runtimecontract.RuntimeProviderDocumentSemanticPlan, ledger.StablePrefixHash, manifest.Hash, requestIdentityHash, expected)
	if ensureErr != nil {
		return runtime.RuntimeAggregate{}, ensureErr
	}
	if session.LogicalRequestID != ledger.LogicalRequestID || session.ManifestHash != manifest.Hash || session.StablePrefixHash != ledger.StablePrefixHash || session.ExpectedDocument == nil {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic request session differs from its sealed intent", common.ErrConflict)
	}
	aggregate = ensured
	ledger = aggregate.SemanticPlanning.Clone()
	if session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		return m.admitRuntimeCompletedSemanticPlanDocument(ctx, aggregate, session)
	}
	if session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal {
		if !runtime.ProviderRequestTerminalSessionApplied(aggregate, session) {
			return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic provider request is terminal without its coordinated SM-2 failure", common.ErrConflict)
		}
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_terminal", "semantic provider request terminated without a complete document", nil)
	}
	if session.ProviderSubmitted {
		settlement, settlementErr := runtimeProviderBudgetSettlement(string(aggregate.Facts.WorkspaceID), string(aggregate.Facts.RunID), session.LogicalRequestID, session.LastPhysicalCallID, nil, nil)
		if settlementErr != nil {
			return runtime.RuntimeAggregate{}, settlementErr
		}
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_submission_unknown", "semantic provider submission has no durable outcome", nil, settlement)
	}

	sequence := session.NextSequence
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, sequence)
	submitted, err := m.runtimeCandidateSession(ctx, aggregate, runtime.RuntimeInternalEventCandidateRequestSubmitted, session.LogicalRequestID, sequence, physicalCallID, nil)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	aggregate = submitted
	response, err := RequestRuntimeCandidate(ctx, m.EventHub(), llmevents.RequestRuntimeCandidateCommand{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DocumentKind:     runtimecontract.RuntimeProviderDocumentSemanticPlan,
		RequestIdentity:  runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: session.LogicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID},
		ExpectedDocument: session.ExpectedDocument,
		CandidateAttempt: ledger.Attempt, StablePrefixHash: ledger.StablePrefixHash,
		SemanticManifest: manifest, SemanticContext: semanticContext,
		SemanticCorrectionObservation: correctionObservation,
		SemanticCorrectionDirective:   correction,
	})
	if err != nil {
		settlement, settlementErr := runtimeProviderBudgetSettlement(string(aggregate.Facts.WorkspaceID), string(aggregate.Facts.RunID), session.LogicalRequestID, physicalCallID, nil, nil)
		if settlementErr != nil {
			return runtime.RuntimeAggregate{}, settlementErr
		}
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_submission_unknown", "semantic provider submission has no trusted physical outcome", nil, settlement)
	}
	settlement, settlementErr := runtimeProviderBudgetSettlement(string(aggregate.Facts.WorkspaceID), string(aggregate.Facts.RunID), session.LogicalRequestID, physicalCallID, response.Audit, response.Usage)
	if settlementErr != nil || settlement.Status == runtime.RuntimeProviderBudgetSettlementUnknown {
		unknown, unknownErr := runtimeProviderBudgetSettlement(string(aggregate.Facts.WorkspaceID), string(aggregate.Facts.RunID), session.LogicalRequestID, physicalCallID, nil, nil)
		if unknownErr != nil {
			return runtime.RuntimeAggregate{}, unknownErr
		}
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_submission_unknown", "semantic provider submission has no trusted physical outcome", nil, unknown)
	}
	if response.Feedback != nil {
		if response.Feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryProviderTransient && settlement.Status != runtime.RuntimeProviderBudgetSettlementConfirmed {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_submission_fact_invalid", "unsubmitted semantic provider result cannot request transport recovery", response.Feedback.OriginalDiagnostic, settlement)
		}
		return m.handleRuntimeSemanticProviderFeedback(ctx, aggregate, session, *response.Feedback, physicalCallID, settlement)
	}
	if response.DocumentOutcome == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_outcome_missing", "semantic provider returned no typed document outcome", nil, settlement)
	}
	if settlement.Status != runtime.RuntimeProviderBudgetSettlementConfirmed {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_submission_fact_invalid", "unsubmitted semantic provider result cannot carry a provider document", nil, settlement)
	}
	return m.handleRuntimeSemanticDocumentOutcome(ctx, aggregate, session, response, sequence, physicalCallID, settlement)
}

func runtimeSemanticCorrectionContext(ledger runtimecontract.RuntimeSemanticPlanningLedger) (*runtimecontract.RuntimeOriginalDiagnostic, *runtimecontract.RuntimeSemanticCorrectionDirective, error) {
	if ledger.Attempt != 1 {
		return nil, nil, nil
	}
	if len(ledger.Rejections) != 1 || ledger.CorrectionDirective == nil {
		return nil, nil, fmt.Errorf("semantic correction has no exact attempt-0 diagnostic")
	}
	previous := ledger.Rejections[0].CloneOriginal()
	value := ledger.CorrectionDirective.Clone()
	return previous, &value, nil
}

func (m *Biz) handleRuntimeSemanticProviderFeedback(ctx context.Context, aggregate runtime.RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession, feedback runtimecontract.RuntimeFeedback, physicalCallID string, settlement *runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, error) {
	providerFailure := runtimeProviderRequestFailure(feedback)
	providerDiagnostic := feedback.OriginalDiagnostic.CloneOriginal()
	if feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryProviderTransient {
		outcomeClass, terminalCode, ok := runtimeSemanticProviderTerminal(feedback.StableCategory)
		if !ok {
			return runtime.RuntimeAggregate{}, fmt.Errorf("%w: unsupported semantic provider feedback category %q", common.ErrInvalidArgument, feedback.StableCategory)
		}
		return m.runtimeCoordinateProviderRequestTerminal(ctx, aggregate, session.LogicalRequestID, runtime.RuntimeProviderRequestTerminalInput{
			DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan,
			OutcomeClass: outcomeClass,
			TerminalCode: terminalCode, TerminalSignature: strings.ReplaceAll(terminalCode, "_", "-"),
			Message: providerFailure.Reason, OriginalDiagnostic: providerDiagnostic,
		}, settlement)
	}
	outcome := runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient,
		Code:  firstNonEmptyString(providerFailure.Code, "provider_transient"), Signature: firstNonEmptyString(providerFailure.Signature, "provider-transient"),
	}
	return m.runtimeCoordinateProviderRequestRecovery(ctx, aggregate, session.LogicalRequestID, physicalCallID, runtime.RuntimeProviderRequestRecoveryInput{
		DocumentKind:     runtimecontract.RuntimeProviderDocumentSemanticPlan,
		TransportOutcome: &outcome, Message: providerFailure.Reason, OriginalDiagnostic: providerDiagnostic,
	}, settlement)
}

// runtimeSemanticProviderTerminal is the only semantic-provider terminal
// classifier. The LLM owner's private OriginalCode/OriginalClass remain exact
// nested diagnostics, but cannot select an SM-2 or SM-3 transition.
func runtimeSemanticProviderTerminal(category runtimecontract.RuntimeFeedbackCategory) (runtimecontract.RuntimeCandidateRequestOutcomeClass, string, bool) {
	switch category {
	case runtimecontract.RuntimeFeedbackCategoryProviderAuthentication:
		return runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication, "semantic_provider_authentication", true
	case runtimecontract.RuntimeFeedbackCategoryProviderConfiguration:
		return runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration, "semantic_provider_configuration", true
	case runtimecontract.RuntimeFeedbackCategoryProviderProtocol:
		return runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol, "semantic_provider_contract_error", true
	default:
		return runtimecontract.RuntimeCandidateRequestOutcomeNone, "", false
	}
}

func (m *Biz) handleRuntimeSemanticDocumentOutcome(ctx context.Context, aggregate runtime.RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession, response llmevents.RequestRuntimeCandidateResult, sequence int, physicalCallID string, settlement *runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, error) {
	framing := response.DocumentOutcome.Framing
	switch framing.Disposition {
	case runtimecontract.RuntimeProviderDocumentIncomplete, runtimecontract.RuntimeProviderDocumentInvalid, runtimecontract.RuntimeProviderDocumentEmpty:
		updated, recoveryErr := m.runtimeCoordinateProviderRequestRecovery(ctx, aggregate, session.LogicalRequestID, physicalCallID, runtime.RuntimeProviderRequestRecoveryInput{
			DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan, DocumentFraming: &framing,
		}, settlement)
		if recoveryErr != nil {
			return runtime.RuntimeAggregate{}, recoveryErr
		}
		_, ok := runtimeCandidateRequestSession(updated, session.LogicalRequestID)
		if !ok {
			return runtime.RuntimeAggregate{}, fmt.Errorf("semantic request session disappeared after recovery fact")
		}
		return updated, nil
	case runtimecontract.RuntimeProviderDocumentComplete:
		if len(response.DocumentOutcome.Content) == 0 {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_document_missing", "complete semantic outcome did not carry its document", nil, settlement)
		}
	default:
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_outcome_invalid", "semantic provider returned an unknown document outcome", nil, settlement)
	}
	completed, _, _, err := m.persistAndCompleteRuntimeCandidateRequestDocument(ctx, aggregate, session.LogicalRequestID, sequence, physicalCallID, response.DocumentOutcome.Content, settlement)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	return m.admitRuntimeCompletedSemanticPlanDocument(ctx, completed, session)
}

// compileRuntimeSemanticPlanBatch is the Biz-side adapter for the owner-local
// batch compiler. It reads only the persisted order document and submits the
// resulting fact through the private event path; it never mutates the ledger.
func (m *Biz) compileRuntimeSemanticPlanBatch(ctx context.Context, aggregate runtime.RuntimeAggregate, logicalRequestID string, maxItems int) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Manifest == nil || ledger.CoverageOrderDocument == nil || ledger.Document == nil || ledger.Document.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic batch requires persisted coverage order", common.ErrConflict)
	}
	sequence := ledger.NextBatchSequence
	if limit := aggregate.Facts.ConvergencePolicy.MaxCandidateContextItems; limit > 0 && limit < maxItems {
		maxItems = limit
	}
	batch, err := semanticplan.CompileFocusBatch(
		*ledger.Manifest,
		*ledger.CoverageOrderDocument,
		sequence,
		logicalRequestID,
		runtime.SemanticPlanActiveCoverageIDsForDrive(aggregate.Facts, *ledger.CoverageOrderDocument, ledger.PendingCoverageIDs),
		ledger.PendingCoverageIDs,
		maxItems,
	)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanBatchCompiled, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticPlanBatch: &batch},
	})
	return result.Aggregate, err
}

func (m *Biz) mergeRuntimeSemanticPlanFocusBatchDocument(ctx context.Context, aggregate runtime.RuntimeAggregate, batch runtimecontract.RuntimeSemanticPlanBatch) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Manifest == nil {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic focus batch merge has no manifest", common.ErrConflict)
	}
	document, found := runtimeSemanticBatchDocument(ledger.BatchDocuments, batch.BatchID)
	if !found {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic focus batch merge has no batch document", common.ErrConflict)
	}
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(*ledger.Manifest, batch, document)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID,
			RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged},
	})
	return result.Aggregate, err
}

func (m *Biz) advanceRuntimeSemanticPlanBatch(ctx context.Context, aggregate runtime.RuntimeAggregate, batch runtimecontract.RuntimeSemanticPlanBatch, next runtimecontract.RuntimeSemanticPlanBatchState) (runtime.RuntimeAggregate, error) {
	if batch.BatchID == "" {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic batch identity is empty", common.ErrInvalidArgument)
	}
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanBatchAdvanced, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticPlanBatch: &batch, SemanticPlanBatchNextState: next},
	})
	return result.Aggregate, err
}

func (m *Biz) observeRuntimeSemanticPlanBatchFeasibility(ctx context.Context, aggregate runtime.RuntimeAggregate, result runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult) (runtime.RuntimeAggregate, error) {
	if result.BatchID == "" {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic batch feasibility identity is empty", common.ErrInvalidArgument)
	}
	eventResult, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanBatchFeasibilityObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticPlanBatchFeasibilityResult: &result},
	})
	return eventResult.Aggregate, err
}

func (m *Biz) admitRuntimeCompletedSemanticPlanDocument(ctx context.Context, aggregate runtime.RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Manifest == nil || ledger.LogicalRequestID != session.LogicalRequestID {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: complete semantic document has no matching planning request", common.ErrConflict)
	}
	payload, err := loadCompletedRuntimeCandidateRequestDocument(ctx, m.evidence.ArtifactContinuations, aggregate, session.LogicalRequestID)
	if err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_document_content_invalid", "persisted semantic provider document is unavailable or fails integrity", nil)
	}
	semanticDocumentHash := runtimecontract.StableRuntimeProviderResponseHash(payload)
	payload, _ = runtimecontract.NormalizeRuntimeCandidatePayloadDocument(payload)
	if runtimecontract.RuntimeSemanticPayloadIsDefinitionGap(payload) {
		if session.ExpectedDocument == nil {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_expected_document_missing", "Definition gap document has no sealed expected document", nil)
		}
		gap, gapErr := runtimecontract.DecodeRuntimeDefinitionGapPayloadWithManifest(*ledger.Manifest, *session.ExpectedDocument, payload)
		if gapErr == nil {
			admission := runtimecontract.EvaluateRuntimeDefinitionGapAdmission(*ledger.Manifest, *session.ExpectedDocument, gap)
			if !admission.Admitted() {
				if admission.Code == "semantic_request_contract_conflict" {
					diagnostic := runtimecontract.RuntimeOriginalDiagnosticFromDefinitionGap(gap)
					return m.failRuntimeSemanticContract(ctx, aggregate, admission.Code, admission.Message, &diagnostic)
				}
				if admission.Code == runtimecontract.RuntimeDefinitionGapAdmissionValidationRepairForbidden {
					diagnostic := runtimecontract.RuntimeOriginalDiagnosticFromDefinitionGap(gap)
					return m.failRuntimeSemanticContract(ctx, aggregate, "repair_surface_unavailable", admission.Message, &diagnostic)
				}
				gapErr = fmt.Errorf("runtime Definition gap admission %s (%s): %s", admission.Disposition, admission.Code, admission.Message)
				return m.handleRuntimeSemanticDocumentDecodeFailure(ctx, aggregate, session, ledger, gapErr)
			}
			diagnostic := runtimecontract.RuntimeOriginalDiagnosticFromDefinitionGap(gap)
			observation := runtimecontract.RuntimeDefinitionGapObservation{
				SchemaVersion: runtimecontract.RuntimeDefinitionClarificationSchemaCurrent,
				WorkspaceID:   aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
				DefinitionRevision: aggregate.Definition.Normalized().Revision, StartupInputHash: aggregate.Definition.Normalized().StartupInputHash,
				AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, FocusRef: aggregate.Facts.Focus.ID,
				LogicalRequestID: session.LogicalRequestID, SemanticDocumentHash: semanticDocumentHash,
				GapKind: gap.GapKind, OffendingItemRefs: append([]string(nil), gap.OffendingItemRefs...), OriginalDiagnostic: diagnostic,
			}
			observation.ObservationID = runtimecontract.StableRuntimeDefinitionGapObservationID(observation)
			updated, submitErr := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind:      runtime.RuntimeInternalEventDefinitionGapObserved,
				Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
				Authority: &runtime.RuntimeInternalAuthorityPayload{DefinitionGap: &observation},
			})
			return updated.Aggregate, submitErr
		}
		// An explicitly selected but malformed gap branch never opens a ticket.
		// Feed it into the same bounded semantic rejection path as any other
		// malformed complete document.
		return m.handleRuntimeSemanticDocumentDecodeFailure(ctx, aggregate, session, ledger, gapErr)
	}
	document, decodeErr := runtimecontract.DecodeRuntimeSemanticPlanPayloadWithManifest(*ledger.Manifest, payload)
	if decodeErr == nil {
		updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind: runtime.RuntimeInternalEventSemanticPlanEvaluated, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{LogicalRequestID: session.LogicalRequestID, SemanticDocument: &document},
		})
		return updated.Aggregate, err
	}
	return m.handleRuntimeSemanticDocumentDecodeFailure(ctx, aggregate, session, ledger, decodeErr)
}

func (m *Biz) handleRuntimeSemanticDocumentDecodeFailure(ctx context.Context, aggregate runtime.RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession, ledger runtimecontract.RuntimeSemanticPlanningLedger, decodeErr error) (runtime.RuntimeAggregate, error) {
	diagnostic := runtimeSemanticPayloadDiagnostic(decodeErr)
	return m.handleRuntimeSemanticDocumentDiagnostic(ctx, aggregate, session, ledger, diagnostic)
}

func (m *Biz) handleRuntimeSemanticDocumentDiagnostic(ctx context.Context, aggregate runtime.RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession, ledger runtimecontract.RuntimeSemanticPlanningLedger, diagnostic runtimecontract.RuntimeOriginalDiagnostic) (runtime.RuntimeAggregate, error) {
	if ledger.Attempt >= 1 {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_plan_retry_exhausted", diagnostic.OriginalMessage, &diagnostic)
	}
	if session.ExpectedDocument == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_expected_document_missing", "semantic correction has no sealed expected document", nil)
	}
	expected := session.ExpectedDocument.Clone()
	expected.CorrectionAttempt = 1
	expected.LogicalRequestID = ""
	expected.Hash = ""
	if ledger.Manifest == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "runtime_contract_defect", "semantic plan rejection has no sealed manifest", &diagnostic)
	}
	diagnostic = runtimeSemanticCorrectionDiagnostic(diagnostic, *ledger.Manifest, expected)
	if len(diagnostic.SemanticViolations) == 0 {
		return m.failRuntimeSemanticContract(ctx, aggregate, "runtime_contract_defect", "semantic plan rejection has no sealed typed correction delta", &diagnostic)
	}
	observation, observationErr := requestcontent.ProjectRuntimeSemanticCorrectionObservation(&diagnostic)
	if observationErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "runtime_contract_defect", observationErr.Error(), &diagnostic)
	}
	directive, deriveErr := runtimecontract.DeriveRuntimeSemanticCorrectionDirective(*ledger.Manifest, expected, ledger.StablePrefixHash, *observation)
	if deriveErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "runtime_contract_defect", deriveErr.Error(), &diagnostic)
	}
	logicalRequestID := runtimeSemanticLogicalRequestID(aggregate, ledger.StablePrefixHash, ledger.Manifest.Hash, runtime.SemanticRequestIdentityHashForKey(ledger, directive.Hash))
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanCorrectionRequired, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{StablePrefixHash: ledger.StablePrefixHash, LogicalRequestID: logicalRequestID,
			ExpectedDocument: &expected, SemanticDiagnostic: &diagnostic},
	})
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	if updated.Aggregate.StateRevision <= aggregate.StateRevision {
		return runtime.RuntimeAggregate{}, fmt.Errorf("semantic correction did not persist its private transition")
	}
	return updated.Aggregate, nil
}

func runtimeSemanticCorrectionDiagnostic(diagnostic runtimecontract.RuntimeOriginalDiagnostic, manifest runtimecontract.RuntimeSemanticPlanManifest, expected runtimecontract.RuntimeExpectedDocumentContract) runtimecontract.RuntimeOriginalDiagnostic {
	if len(diagnostic.SemanticViolations) != 0 {
		return diagnostic
	}
	path := "$"
	actual := "invalid_document"
	if len(diagnostic.FieldViolations) != 0 && strings.TrimSpace(diagnostic.FieldViolations[0].Path) != "" {
		path = diagnostic.FieldViolations[0].Path
		if strings.TrimSpace(diagnostic.FieldViolations[0].Actual) != "" {
			actual = diagnostic.FieldViolations[0].Actual
		}
	}
	var violation runtimecontract.RuntimeSemanticContractViolation
	switch expected.PlanStep {
	case runtimecontract.RuntimeSemanticPlanStepCoverageOrder:
		requiredIDs := make([]common.CoverageWorkItemID, 0, len(manifest.CoverageItems))
		for _, item := range manifest.Normalized().CoverageItems {
			requiredIDs = append(requiredIDs, item.ID)
		}
		if len(requiredIDs) == 0 {
			return diagnostic
		}
		violation = runtimecontract.RuntimeSemanticContractViolation{
			Kind: runtimecontract.RuntimeSemanticContractViolationCoverageOrderDocument, Path: path, Actual: actual,
			RequiredCoverageWorkItemIDs: requiredIDs,
		}
	case runtimecontract.RuntimeSemanticPlanStepProofBatch:
		if len(expected.BatchCoverageWorkItemIDs) == 0 {
			return diagnostic
		}
		violation = runtimecontract.RuntimeSemanticContractViolation{
			Kind: runtimecontract.RuntimeSemanticContractViolationProofBatchDocument, Path: path, Actual: actual,
			RequiredCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), expected.BatchCoverageWorkItemIDs...),
		}
	default:
		return diagnostic
	}
	diagnostic.SemanticViolations = []runtimecontract.RuntimeSemanticContractViolation{violation}
	return diagnostic
}

func (m *Biz) driveRuntimeSemanticFeasibility(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	feasibilityDocument, found := runtimeSemanticActiveFeasibilityDocument(ledger)
	if ledger.Manifest == nil || !found {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_feasibility_context_invalid", "semantic feasibility has no persisted manifest or document", nil)
	}
	for _, fact := range ledger.Feasibilities {
		if prospective := fact.OwnerFactProspectiveSatisfactionObservation; prospective != nil {
			if prospective.Stale != nil {
				policy := aggregate.Facts.RecoveryPolicy.Normalized()
				if aggregate.Facts.Recovery.PrerequisiteRefreshes >= policy.MaxPrerequisiteRefreshes {
					return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_catalog_refresh_exhausted", prospective.Stale.Reason, nil)
				}
				updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
					Kind:      runtime.RuntimeInternalEventSemanticContractRequired,
					Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
					Authority: &runtime.RuntimeInternalAuthorityPayload{SemanticBatchID: ledger.FeasibilityBatchID, SemanticRefreshGeneration: ledger.CatalogRefreshAttempts},
				})
				return updated.Aggregate, err
			}
			if prospective.Satisfaction != nil && prospective.Satisfaction.Outcome == runtimecontract.RuntimeOwnerFactProspectiveWouldNotSatisfy {
				diagnostic, err := runtime.RuntimeProspectiveSatisfactionContractViolationDiagnostic(fact)
				if err != nil {
					return m.failRuntimeSemanticContract(ctx, aggregate, "runtime_contract_defect", err.Error(), nil)
				}
				return m.failRuntimeSemanticContract(ctx, aggregate, diagnostic.OriginalCode, diagnostic.OriginalMessage, &diagnostic)
			}
		}
		switch fact.Status {
		case runtimecontract.RuntimeSemanticFeasibilityStale:
			if fact.OriginalDiagnostic == nil {
				return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_feasibility_failed", "semantic owner feasibility omitted its original diagnostic", nil)
			}
			policy := aggregate.Facts.RecoveryPolicy.Normalized()
			if aggregate.Facts.Recovery.PrerequisiteRefreshes >= policy.MaxPrerequisiteRefreshes {
				return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_catalog_refresh_exhausted", fact.OriginalDiagnostic.OriginalMessage, fact.OriginalDiagnostic)
			}
			updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind: runtime.RuntimeInternalEventSemanticContractRequired, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{SemanticBatchID: ledger.FeasibilityBatchID, SemanticRefreshGeneration: ledger.CatalogRefreshAttempts},
			})
			return updated.Aggregate, err
		case runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired:
			// A semantic proof owner currently has no typed prerequisite request
			// contract.  Treat this status as a factual terminal instead of
			// retiring the already observed catalog.  Generic catalog refresh is
			// reserved for a real stale revision; a future prerequisite path must
			// introduce an explicit owner-grounded typed request.
			if fact.OriginalDiagnostic == nil {
				return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_feasibility_unknown", "semantic owner prerequisite fact omitted its original diagnostic", nil)
			}
			return m.failRuntimeSemanticContract(ctx, aggregate, semanticOwnerFeasibilityTerminalCode(fact.Status), fact.OriginalDiagnostic.OriginalMessage, fact.OriginalDiagnostic)
		case runtimecontract.RuntimeSemanticFeasibilityInfeasible:
			if fact.OriginalDiagnostic == nil {
				return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_feasibility_failed", "semantic owner feasibility was not accepted", nil)
			}
			return m.failRuntimeSemanticContract(ctx, aggregate, semanticOwnerFeasibilityTerminalCode(fact.Status), fact.OriginalDiagnostic.OriginalMessage, fact.OriginalDiagnostic)
		}
	}
	observed := make(map[string]struct{}, len(ledger.Feasibilities))
	for _, fact := range ledger.Feasibilities {
		observed[fact.ProposalHash] = struct{}{}
	}
	for _, proposal := range feasibilityDocument.Proposals {
		if _, done := observed[runtimecontract.StableRuntimeSemanticProofProposalHash(proposal)]; done {
			continue
		}
		return m.probeRuntimeSemanticProposal(ctx, aggregate, proposal)
	}
	sealed, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticContractSealed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{},
	})
	return sealed.Aggregate, err
}

// semanticOwnerFeasibilityTerminalCode maps only the owner-neutral, typed
// semantic status to an Orchestrator-local terminal code. Capability-private
// diagnostic codes remain nested audit facts and never select a Runtime
// failure class or transition.
func semanticOwnerFeasibilityTerminalCode(status runtimecontract.RuntimeSemanticFeasibilityStatus) string {
	switch status {
	case runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired:
		return "semantic_owner_prerequisite_required"
	case runtimecontract.RuntimeSemanticFeasibilityInfeasible:
		return "semantic_owner_feasibility_infeasible"
	default:
		return "semantic_owner_feasibility_unknown"
	}
}

func runtimeSemanticActiveFeasibilityDocument(ledger runtimecontract.RuntimeSemanticPlanningLedger) (runtimecontract.RuntimeSemanticPlanDocument, bool) {
	if ledger.ActiveBatchID != "" {
		return runtimeSemanticBatchDocument(ledger.BatchDocuments, ledger.ActiveBatchID)
	}
	if ledger.Document == nil {
		return runtimecontract.RuntimeSemanticPlanDocument{}, false
	}
	return ledger.Document.Clone(), true
}

func (m *Biz) probeRuntimeSemanticProposal(ctx context.Context, aggregate runtime.RuntimeAggregate, proposal runtimecontract.RuntimeSemanticProofProposal) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Manifest == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_probe_proposal_invalid", "semantic proposal has no sealed manifest", nil)
	}
	catalog, found := runtimeSemanticManifestOwnerCatalog(*ledger.Manifest, proposal.OwnerModuleID)
	if !found {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_catalog_missing", "semantic proposal has no sealed owner catalog", nil)
	}
	observation := runtimeSemanticCatalogObservation(aggregate, catalog)
	if observation == nil {
		// The owner cannot prove a proposal against a retired/missing catalog
		// observation. Reopen one fresh semantic generation through the private
		// SM-2 event; do not send a nil observation to a capability owner and do
		// not terminalize this as a generic runtime contract defect.
		refreshed, refreshErr := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind: runtime.RuntimeInternalEventSemanticCatalogInvalidated, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID,
				RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{ManifestHash: ledger.Manifest.Hash,
				LogicalRequestID: ledger.LogicalRequestID},
		})
		return refreshed.Aggregate, refreshErr
	}
	// Fresh sealed authority is a prerequisite for interpreting the proposal.
	// Once it is known to be current, reject a proposal outside the owner-authored
	// typed target constraint before any workspace or capability-owner I/O.
	if err := runtime.ValidateRuntimeSemanticProposalBeforeOwnerProbe(ledger, proposal); err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_probe_proposal_invalid", err.Error(), nil)
	}
	workspace, err := (workspaceSnapshotReader{hub: m.EventHub()}).Read(ctx, aggregate.Facts.WorkspaceID)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	response, err := ProbeRuntimeSemanticProofFeasibility(ctx, m.EventHub(), runtimecontract.RuntimeSemanticProofFeasibilityRequest{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		FeasibilityBatchID: ledger.FeasibilityBatchID, WorkspaceDeliverableDirectory: workspace.DeliverableDirectory,
		Manifest: *ledger.Manifest, Proposal: proposal, OwnerCatalog: catalog, CatalogObservation: observation,
	})
	if err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_probe_invalid", err.Error(), nil)
	}
	ownerFactAcquisition, requiresProspectiveSatisfaction := runtimeSemanticProspectiveOwnerFactAcquisition(*ledger.Manifest, proposal)
	if response.Observation.Status == runtimecontract.RuntimeSemanticFeasibilityFeasible && requiresProspectiveSatisfaction {
		if response.Observation.ProspectiveWorkspaceManifestDelta == nil {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_probe_invalid", "prospective owner-fact feasibility omitted the File-authored workspace manifest delta", nil)
		}
		requiredOwnerCatalog, found := runtimeSemanticManifestDependencyCatalog(*ledger.Manifest, ownerFactAcquisition.Requirement.OwnerModuleID)
		if !found {
			return m.failRuntimeSemanticContract(ctx, aggregate, "runtime_contract_defect", "prospective owner-fact satisfaction has no sealed required-owner dependency catalog", nil)
		}
		requiredOwnerObservation := runtimeSemanticCatalogObservation(aggregate, requiredOwnerCatalog)
		if requiredOwnerObservation == nil {
			refreshed, refreshErr := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind:  runtime.RuntimeInternalEventSemanticCatalogInvalidated,
				Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
				Authority: &runtime.RuntimeInternalAuthorityPayload{
					ManifestHash: ledger.Manifest.Hash, LogicalRequestID: ledger.LogicalRequestID,
				},
			})
			return refreshed.Aggregate, refreshErr
		}
		satisfactionResponse, satisfactionErr := ProbeRuntimeOwnerFactProspectiveSatisfaction(ctx, m.EventHub(), runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			FeasibilityBatchID: ledger.FeasibilityBatchID,
			Manifest:           *ledger.Manifest, Proposal: proposal, Requirement: ownerFactAcquisition.Requirement,
			ProspectiveSatisfactionContractRef: ownerFactAcquisition.ProspectiveSatisfactionContractRef,
			OwnerCatalog:                       requiredOwnerCatalog, CatalogObservation: requiredOwnerObservation,
			ValidationRecipes:                 runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(aggregate.Facts.Activation.EvidencePolicies),
			ProspectiveWorkspaceManifestDelta: response.Observation.ProspectiveWorkspaceManifestDelta.Clone(),
		})
		if satisfactionErr != nil {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_probe_invalid", satisfactionErr.Error(), nil)
		}
		prospectiveObservation := satisfactionResponse.Clone()
		response.Observation.OwnerFactProspectiveSatisfactionObservation = &prospectiveObservation
	}
	if err := response.Observation.ValidateCompositeForProposal(*ledger.Manifest, proposal); err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_probe_invalid", err.Error(), nil)
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticFeasibilityObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{SemanticBatchID: ledger.FeasibilityBatchID, SemanticFeasibility: &response.Observation},
	})
	return updated.Aggregate, err
}

func runtimeSemanticProspectiveOwnerFactAcquisition(manifest runtimecontract.RuntimeSemanticPlanManifest, proposal runtimecontract.RuntimeSemanticProofProposal) (runtimecontract.RuntimeSemanticOwnerFactAcquisition, bool) {
	proposal = proposal.Normalized()
	for _, obligation := range manifest.Normalized().ProofObligations {
		if obligation.CoverageWorkItemID != proposal.CoverageWorkItemID || obligation.OwnerFactAcquisition == nil {
			continue
		}
		acquisition := obligation.OwnerFactAcquisition.Clone()
		if acquisition.AcquisitionOwnerModuleID == proposal.OwnerModuleID &&
			acquisition.AcquisitionTargetPolicy == runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget &&
			acquisition.ProspectiveSatisfactionContractRef != "" {
			return acquisition, true
		}
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false
	}
	return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false
}

func runtimeSemanticManifestDependencyCatalog(manifest runtimecontract.RuntimeSemanticPlanManifest, ownerModuleID string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	for _, catalog := range manifest.Normalized().OwnerFactDependencyCatalogs {
		if catalog.OwnerModuleID == ownerModuleID {
			return catalog, true
		}
	}
	return runtimecontract.RuntimeOperationAdmissibility{}, false
}

func runtimeSemanticManifestOwnerCatalog(manifest runtimecontract.RuntimeSemanticPlanManifest, ownerModuleID string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	for _, catalog := range manifest.Normalized().OwnerCatalogs {
		if catalog.OwnerModuleID == ownerModuleID {
			return catalog, true
		}
	}
	return runtimecontract.RuntimeOperationAdmissibility{}, false
}

func runtimeSemanticCatalogObservation(aggregate runtime.RuntimeAggregate, catalog runtimecontract.RuntimeOperationAdmissibility) *runtimecontract.RuntimePrerequisiteObservation {
	if observation, found := runtime.CurrentSemanticCatalogObservation(aggregate, catalog); found {
		value := observation.Normalized()
		return &value
	}
	return nil
}

func (m *Biz) failRuntimeSemanticContract(ctx context.Context, aggregate runtime.RuntimeAggregate, code, message string, original *runtimecontract.RuntimeOriginalDiagnostic, budgetSettlement ...*runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, error) {
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
		OriginalClass: "semantic_contract", OriginalCode: strings.TrimSpace(code),
		OriginalMessage: strings.TrimSpace(message), Signature: "semantic-contract/" + strings.ReplaceAll(strings.TrimSpace(code), "_", "-"),
	}
	if original != nil {
		diagnostic = *original.CloneOriginal()
		if diagnostic.OriginalCode == "" {
			diagnostic.OriginalCode = strings.TrimSpace(code)
		}
		if diagnostic.OriginalMessage == "" {
			diagnostic.OriginalMessage = strings.TrimSpace(message)
		}
	}
	if diagnostic.OriginalCode == "" || diagnostic.OriginalMessage == "" {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	// An open semantic provider request and its semantic-contract successor are
	// one owner-local SM-3/SM-2 transaction. A fresh reload therefore observes
	// either the submitted request (and classifies submission-unknown without a
	// second provider call) or the exact terminal request plus semantic failure.
	if logicalRequestID := strings.TrimSpace(aggregate.SemanticPlanning.LogicalRequestID); logicalRequestID != "" {
		if session, found := runtimeCandidateRequestSession(aggregate, logicalRequestID); found {
			switch session.State {
			case runtimecontract.RuntimeCandidateRequestSessionDocumentComplete:
				// The provider transaction is complete; semantic decode/admission may
				// still seal a semantic-contract failure through the ordinary SM-2 event.
			case runtimecontract.RuntimeCandidateRequestSessionTerminal:
				if aggregate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningFailed && aggregate.Facts.Failure != nil {
					return aggregate, nil
				}
				return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic provider request is terminal without its coordinated SM-2 failure", common.ErrConflict)
			default:
				return m.runtimeCoordinateProviderRequestTerminal(ctx, aggregate, logicalRequestID, runtime.RuntimeProviderRequestTerminalInput{
					DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan,
					TerminalCode: strings.TrimSpace(code), TerminalSignature: "semantic-contract/" + strings.ReplaceAll(strings.TrimSpace(code), "_", "-"),
					Message: strings.TrimSpace(message), OriginalDiagnostic: &diagnostic,
				}, firstRuntimeProviderBudgetSettlement(budgetSettlement))
			}
		}
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticContractFailed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{SemanticFailureCode: strings.TrimSpace(code), SemanticDiagnostic: &diagnostic},
	})
	return updated.Aggregate, err
}

func runtimeSemanticPayloadDiagnostic(err error) runtimecontract.RuntimeOriginalDiagnostic {
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, OriginalClass: "semantic_plan_rejection",
		OriginalCode: "semantic_plan_invalid", OriginalMessage: err.Error(), Signature: "semantic-plan/invalid-document",
	}
	var payloadDiagnostic *runtimecontract.RuntimeCandidatePayloadDiagnostic
	if errors.As(err, &payloadDiagnostic) {
		diagnostic.FieldViolations = []runtimecontract.RuntimeFieldViolation{{
			Path: payloadDiagnostic.Path, Actual: payloadDiagnostic.Actual,
			Allowed: append([]string(nil), payloadDiagnostic.Allowed...), Required: append([]string(nil), payloadDiagnostic.Required...), Forbidden: append([]string(nil), payloadDiagnostic.Forbidden...),
		}}
		diagnostic.SemanticViolations = append([]runtimecontract.RuntimeSemanticContractViolation(nil), payloadDiagnostic.SemanticViolations...)
	}
	return diagnostic
}
