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
	if runtimeSemanticPrerequisitesPending(aggregate.Facts.Prerequisites.Pending) {
		return m.prepareRuntimePrerequisites(ctx, aggregate)
	}
	if aggregate.Facts.Resources.ProviderCeiling == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_ceiling_missing", "semantic planning has no sealed provider ceiling prerequisite", nil)
	}
	eligibleCoverage := runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts)
	if len(eligibleCoverage) == 0 {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_no_eligible_cwi", "semantic planning was requested without an eligible canonical CWI", nil)
	}
	manifest, err := semanticplan.CompileManifest(semanticplan.ManifestIdentity{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Definition: aggregate.Definition, DefinitionSeed: aggregate.Facts.DefinitionSeed,
		Execution: aggregate.Facts.Activation.ExecutionContract, Admissibilities: aggregate.Facts.Resources.OperationAdmissibilities,
		EligibleCoverage: eligibleCoverage,
	})
	if err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_manifest_invalid", err.Error(), nil)
	}
	stablePrefixHash := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	expected := runtimeSemanticExpectedDocument(aggregate, runtimecontract.RuntimeSemanticPlanStepCoverageOrder, nil, 0)
	ensured, session, err := m.runtimeEnsureCandidateRequestWithExpected(ctx, aggregate, runtimecontract.RuntimeProviderDocumentSemanticPlan, stablePrefixHash, manifest.Hash, "", expected)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	started, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanRequestStarted, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: ensured.Facts.WorkspaceID, RunID: ensured.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: stablePrefixHash, LogicalRequestID: session.LogicalRequestID},
	})
	return started.Aggregate, err
}

func (m *Biz) driveRuntimeSemanticOrderAccepted(ctx context.Context, aggregate runtime.RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger) (runtime.RuntimeAggregate, error) {
	projected, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventEligibleFocusProjected, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{SemanticOrderManifestHash: ledger.Manifest.Hash, SemanticCoverageOrder: append([]common.CoverageWorkItemID(nil), ledger.CoverageOrderDocument.CoverageOrder...)},
	})
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	aggregate = projected.Aggregate
	ledger = aggregate.SemanticPlanning.Clone()
	batchRequestKey := fmt.Sprintf("semantic-proof-batch-%d", ledger.NextBatchSequence)
	logicalRequestID := runtimeSemanticLogicalRequestID(aggregate, ledger.StablePrefixHash, ledger.Manifest.Hash, batchRequestKey)
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
	for _, value := range values {
		switch value.Kind {
		case runtimecontract.PrerequisiteKindProviderCeiling,
			runtimecontract.PrerequisiteKindCapabilityState,
			runtimecontract.PrerequisiteKindOperationAdmissibility:
			return true
		}
	}
	return false
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

func runtimeSemanticExpectedDocument(aggregate runtime.RuntimeAggregate, step runtimecontract.RuntimeSemanticPlanStep, batch *runtimecontract.RuntimeSemanticPlanBatch, correctionAttempt int) runtimecontract.RuntimeExpectedDocumentContract {
	execution := aggregate.Facts.Activation.ExecutionContract.Normalized()
	maxOutputTokens := 0
	if aggregate.Facts.Resources.ProviderCeiling != nil {
		maxOutputTokens = aggregate.Facts.Resources.ProviderCeiling.EffectiveMaxOutputTokens
	}
	maxRecoveryAttempts := execution.CandidateDocumentRecovery.MaxAttempts
	if maxRecoveryAttempts <= 0 {
		maxRecoveryAttempts = 1
	}
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:  runtimecontract.RuntimeProviderDocumentSemanticPlan,
		PlanStep:      step, DefinitionSeedHash: aggregate.Facts.DefinitionSeed.Hash,
		CorrectionAttempt: correctionAttempt, MaxOutputTokens: maxOutputTokens, MaxRecoveryAttempts: maxRecoveryAttempts,
	}
	if batch != nil {
		expected.BatchID = batch.BatchID
		expected.BatchSequence = batch.Sequence
		expected.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
		expected.CoverageOrderHash = batch.CoverageOrderHash
	}
	return expected
}

func runtimeSemanticExpectedDocumentForLedger(aggregate runtime.RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger) (runtimecontract.RuntimeExpectedDocumentContract, error) {
	if ledger.Manifest == nil {
		return runtimecontract.RuntimeExpectedDocumentContract{}, fmt.Errorf("semantic expected document requires a manifest")
	}
	if ledger.ActiveBatchID == "" {
		return runtimeSemanticExpectedDocument(aggregate, runtimecontract.RuntimeSemanticPlanStepCoverageOrder, nil, ledger.Attempt), nil
	}
	for idx := range ledger.Batches {
		if ledger.Batches[idx].BatchID == ledger.ActiveBatchID {
			batch := ledger.Batches[idx].Clone()
			return runtimeSemanticExpectedDocument(aggregate, runtimecontract.RuntimeSemanticPlanStepProofBatch, &batch, ledger.Attempt), nil
		}
	}
	return runtimecontract.RuntimeExpectedDocumentContract{}, fmt.Errorf("semantic expected document has no persisted active batch")
}

func runtimeSemanticRequestIdentityHash(ledger runtimecontract.RuntimeSemanticPlanningLedger) (string, error) {
	if ledger.CorrectionDirective != nil {
		return ledger.CorrectionDirective.Hash, nil
	}
	if ledger.ActiveBatchID == "" {
		return "", nil
	}
	for idx := range ledger.Batches {
		if ledger.Batches[idx].BatchID == ledger.ActiveBatchID {
			return fmt.Sprintf("semantic-proof-batch-%d", ledger.Batches[idx].Sequence), nil
		}
	}
	return "", fmt.Errorf("semantic request identity has no persisted active batch")
}

func (m *Biz) requestRuntimeSemanticPlanDocument(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Manifest == nil || ledger.LogicalRequestID == "" || aggregate.Facts.Resources.ProviderCeiling == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_context_invalid", "semantic request is missing manifest, request identity, or provider ceiling", nil)
	}
	manifest := ledger.Manifest.Clone()
	expected, expectedErr := runtimeSemanticExpectedDocumentForLedger(aggregate, ledger)
	if expectedErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_expected_document_invalid", expectedErr.Error(), nil)
	}
	requestIdentityHash, identityErr := runtimeSemanticRequestIdentityHash(ledger)
	if identityErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_identity_invalid", identityErr.Error(), nil)
	}
	ensured, session, ensureErr := m.runtimeEnsureCandidateRequestWithExpected(ctx, aggregate, runtimecontract.RuntimeProviderDocumentSemanticPlan, ledger.StablePrefixHash, manifest.Hash, requestIdentityHash, expected)
	if ensureErr != nil {
		return runtime.RuntimeAggregate{}, ensureErr
	}
	if session.LogicalRequestID != ledger.LogicalRequestID {
		return m.failRuntimeSemanticContract(ctx, ensured, "semantic_request_session_mismatch", "semantic expected document resolved a different logical request", nil)
	}
	aggregate = ensured
	ledger = aggregate.SemanticPlanning.Clone()
	if session.ManifestHash != manifest.Hash || session.StablePrefixHash != ledger.StablePrefixHash {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_session_mismatch", "semantic request session identity differs from the planning ledger", nil)
	}
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

	if session.ExpectedDocument == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_expected_document_missing", "semantic request session has no sealed expected document", nil)
	}
	expected = session.ExpectedDocument.Clone()
	semanticContext := runtimecontract.RuntimeSemanticPlanRequestContext{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Definition: aggregate.Definition, DefinitionSeed: aggregate.Facts.DefinitionSeed,
		Activation: aggregate.Facts.Activation, ProviderCeiling: aggregate.Facts.Resources.ProviderCeiling,
		PlanStep: expected.PlanStep, BatchID: expected.BatchID, BatchSequence: expected.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), expected.BatchCoverageWorkItemIDs...),
	}
	previous, correction, correctionErr := runtimeSemanticCorrectionContext(ledger)
	if correctionErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_correction_lineage_invalid", correctionErr.Error(), nil)
	}
	if _, contentErr := requestcontent.BuildSemanticPlanRequestContent(requestcontent.SemanticPlanRequestInput{
		Manifest: manifest, StablePrefixHash: ledger.StablePrefixHash, Attempt: ledger.Attempt,
		PreviousRejection: previous, CorrectionDirective: correction,
		RemainingRecoveryAttempt: expected.MaxRecoveryAttempts - ledger.Attempt,
	}); contentErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_context_invalid", contentErr.Error(), nil)
	}
	preflight, err := PreflightRuntimeCandidate(ctx, m.EventHub(), llmevents.PreflightRuntimeCandidateCommand{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DocumentKind:     runtimecontract.RuntimeProviderDocumentSemanticPlan,
		CandidateAttempt: ledger.Attempt, StablePrefixHash: ledger.StablePrefixHash,
		SemanticManifest: manifest, SemanticContext: semanticContext,
		PreviousSemanticRejection:   previous,
		SemanticCorrectionDirective: correction,
	})
	if err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_context_integrity", err.Error(), nil)
	}
	if preflight.Feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryContextIntegrity {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_context_integrity", preflight.Feedback.OriginalDiagnostic.OriginalMessage, preflight.Feedback.OriginalDiagnostic)
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
		PreviousSemanticRejection:   previous,
		SemanticCorrectionDirective: correction,
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
		return m.failRuntimeSemanticContract(ctx, aggregate, firstNonEmptyString(providerFailure.Code, "semantic_provider_failure"), providerFailure.Reason, providerDiagnostic, settlement)
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

func (m *Biz) handleRuntimeSemanticDocumentOutcome(ctx context.Context, aggregate runtime.RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession, response llmevents.RequestRuntimeCandidateResult, sequence int, physicalCallID string, settlement *runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, error) {
	framing := response.DocumentOutcome.Framing
	switch framing.Disposition {
	case runtimecontract.RuntimeProviderDocumentIncomplete:
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
	case runtimecontract.RuntimeProviderDocumentInvalid, runtimecontract.RuntimeProviderDocumentEmpty:
		return m.failRuntimeSemanticContract(ctx, aggregate, framing.Code, "semantic provider did not return one complete authority document", nil, settlement)
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
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
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
	directive, deriveErr := runtimecontract.DeriveRuntimeSemanticCorrectionDirective(*ledger.Manifest, expected, ledger.StablePrefixHash, diagnostic)
	if deriveErr != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "runtime_contract_defect", deriveErr.Error(), &diagnostic)
	}
	ensured, correctionSession, err := m.runtimeEnsureCandidateRequestWithExpected(ctx, aggregate, runtimecontract.RuntimeProviderDocumentSemanticPlan, ledger.StablePrefixHash, ledger.Manifest.Hash, directive.Hash, expected)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanCorrectionRequired, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{StablePrefixHash: ledger.StablePrefixHash, LogicalRequestID: correctionSession.LogicalRequestID,
			ExpectedDocument: &expected, SemanticDiagnostic: &diagnostic},
	})
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	if updated.Aggregate.StateRevision <= ensured.StateRevision {
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
			return m.failRuntimeSemanticContract(ctx, aggregate, semanticOwnerFeasibilityTerminalCode(fact), fact.OriginalDiagnostic.OriginalMessage, fact.OriginalDiagnostic)
		case runtimecontract.RuntimeSemanticFeasibilityInfeasible:
			if fact.OriginalDiagnostic == nil {
				return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_feasibility_failed", "semantic owner feasibility was not accepted", nil)
			}
			return m.failRuntimeSemanticContract(ctx, aggregate, semanticOwnerFeasibilityTerminalCode(fact), fact.OriginalDiagnostic.OriginalMessage, fact.OriginalDiagnostic)
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

// semanticOwnerFeasibilityTerminalCode returns only a closed internal code
// for the reducer's failure-class mapping. The nested owner diagnostic is
// preserved unchanged and remains the sole producer-authored detail.
func semanticOwnerFeasibilityTerminalCode(fact runtimecontract.RuntimeSemanticProofFeasibility) string {
	if fact.OriginalDiagnostic == nil {
		return "semantic_owner_feasibility_unknown"
	}
	code := strings.TrimSpace(fact.OriginalDiagnostic.OriginalCode)
	switch code {
	case "validation_recipe_unavailable", "validation_recipe_not_sealed", "validation_recipe_exhausted",
		"mcp_semantic_proof_unavailable", "mcp_recipe_not_admissible", "mcp_selector_not_admissible",
		"file_semantic_proof_unavailable", "git_semantic_proof_unavailable", "web_semantic_proof_unavailable",
		"ssh_semantic_proof_unavailable", "semantic_target_unavailable", "semantic_target_invalid",
		"semantic_target_not_regular", "prospective_target_exists", "prospective_target_unobservable",
		"semantic_owner_feasibility_infeasible", "mcp_recipe_invalid":
		return code
	case "semantic_provider_ceiling_missing", "semantic_catalog_stale", "mcp_catalog_unavailable",
		"mcp_capability_catalog_unavailable", "mcp_validation_input_binding_required", "web_catalog_unavailable", "git_repository_state_unavailable",
		"shell_policy_unavailable", "workspace_manifest_unavailable", "semantic_owner_catalog_missing":
		return code
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
	workspace, err := (workspaceSnapshotReader{hub: m.EventHub()}).Read(ctx, aggregate.Facts.WorkspaceID)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
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
	response, err := ProbeRuntimeSemanticProofFeasibility(ctx, m.EventHub(), runtimecontract.RuntimeSemanticProofFeasibilityRequest{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		FeasibilityBatchID: ledger.FeasibilityBatchID, WorkspaceDeliverableDirectory: workspace.DeliverableDirectory,
		Manifest: *ledger.Manifest, Proposal: proposal, OwnerCatalog: catalog, CatalogObservation: observation,
	})
	if err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_probe_invalid", err.Error(), nil)
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticFeasibilityObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{SemanticBatchID: ledger.FeasibilityBatchID, SemanticFeasibility: &response.Observation},
	})
	return updated.Aggregate, err
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
