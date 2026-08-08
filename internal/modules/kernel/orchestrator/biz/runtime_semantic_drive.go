package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
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
		// Semantic Planning consumes committed owner facts. It must not create
		// a second prerequisite path: capability-state acquisition (especially
		// Git) is always committed before operation admissibility by the shared
		// Orchestrator dispatcher.
		if runtimeSemanticPrerequisitesPending(aggregate.Facts.Prerequisites.Pending) {
			return m.prepareRuntimePrerequisites(ctx, aggregate)
		}
		if aggregate.Facts.Resources.ProviderCeiling == nil {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_ceiling_missing", "semantic planning has no sealed provider ceiling prerequisite", nil)
		}
		manifest, err := runtimecontract.CompileRuntimeSemanticPlanManifest(
			aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Definition,
			aggregate.Facts.DefinitionSeed, aggregate.Facts.Activation.ExecutionContract,
			aggregate.Facts.Resources.OperationAdmissibilities,
		)
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
			Kind:        runtime.RuntimeInternalEventSemanticPlanRequestStarted,
			WorkspaceID: ensured.Facts.WorkspaceID, RunID: ensured.Facts.RunID,
			SemanticManifest: &manifest, StablePrefixHash: stablePrefixHash, LogicalRequestID: session.LogicalRequestID,
		})
		return started.Aggregate, err
	case runtimecontract.RuntimeSemanticPlanningOrderRequestOpen, runtimecontract.RuntimeSemanticPlanningRequestOpen, runtimecontract.RuntimeSemanticPlanningCorrectionRequired:
		return m.requestRuntimeSemanticPlanDocument(ctx, aggregate)
	case runtimecontract.RuntimeSemanticPlanningOrderAccepted:
		batchRequestKey := fmt.Sprintf("semantic-proof-batch-%d", ledger.NextBatchSequence)
		logicalRequestID := runtimeSemanticLogicalRequestID(aggregate, ledger.StablePrefixHash, ledger.Manifest.Hash, batchRequestKey)
		return m.compileRuntimeSemanticPlanBatch(ctx, aggregate, logicalRequestID, 4)
	case runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen:
		return m.requestRuntimeSemanticPlanDocument(ctx, aggregate)
	case runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted:
		updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventSemanticFeasibilityRequired, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID})
		return updated.Aggregate, err
	case runtimecontract.RuntimeSemanticPlanningProofBatchFeasibilityComplete:
		var activeBatch *runtimecontract.RuntimeSemanticPlanBatch
		for idx := range ledger.Batches {
			if ledger.Batches[idx].BatchID == ledger.ActiveBatchID {
				value := ledger.Batches[idx].Clone()
				activeBatch = &value
				break
			}
		}
		if activeBatch == nil {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_batch_identity_missing", "completed semantic proof batch has no active batch identity", nil)
		}
		batchResultFound := false
		for _, result := range ledger.BatchFeasibilityResults {
			if result.BatchID == activeBatch.BatchID {
				batchResultFound = true
				break
			}
		}
		if !batchResultFound {
			batchDocument, found := runtimeSemanticBatchDocument(ledger.BatchDocuments, activeBatch.BatchID)
			if !found {
				return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_batch_document_missing", "completed semantic proof batch has no persisted document", nil)
			}
			result := runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{
				BatchID: activeBatch.BatchID, BatchHash: activeBatch.Hash,
				DocumentHash:  runtimecontract.StableRuntimeSemanticPlanDocumentHash(batchDocument),
				Feasibilities: append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), ledger.Feasibilities...),
			}
			result.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
			return m.observeRuntimeSemanticPlanBatchFeasibility(ctx, aggregate, result)
		}
		if len(ledger.PendingCoverageIDs) != 0 {
			batchRequestKey := fmt.Sprintf("semantic-proof-batch-%d", ledger.NextBatchSequence)
			logicalRequestID := runtimeSemanticLogicalRequestID(aggregate, ledger.StablePrefixHash, ledger.Manifest.Hash, batchRequestKey)
			return m.compileRuntimeSemanticPlanBatch(ctx, aggregate, logicalRequestID, 4)
		}
		merged, err := m.mergeRuntimeSemanticPlanBatchDocuments(ctx, aggregate)
		if err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		merged, err = m.mergeRuntimeSemanticPlanBatchFeasibilities(ctx, merged)
		if err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		sealed, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventSemanticContractSealed, WorkspaceID: merged.Facts.WorkspaceID, RunID: merged.Facts.RunID})
		return sealed.Aggregate, err
	case runtimecontract.RuntimeSemanticPlanningProposalAccepted:
		updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind:        runtime.RuntimeInternalEventSemanticFeasibilityRequired,
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		})
		return updated.Aggregate, err
	case runtimecontract.RuntimeSemanticPlanningFeasibilityRequired:
		return m.driveRuntimeSemanticFeasibility(ctx, aggregate)
	case runtimecontract.RuntimeSemanticPlanningSealed, runtimecontract.RuntimeSemanticPlanningFailed:
		return aggregate, nil
	default:
		return runtime.RuntimeAggregate{}, fmt.Errorf("unsupported runtime semantic planning phase %q", ledger.Phase)
	}
}

func runtimeSemanticBatchDocument(values []runtimecontract.RuntimeSemanticPlanDocument, batchID string) (runtimecontract.RuntimeSemanticPlanDocument, bool) {
	batchID = strings.TrimSpace(batchID)
	for _, value := range values {
		if value.BatchID == batchID {
			return value.Clone(), true
		}
	}
	return runtimecontract.RuntimeSemanticPlanDocument{}, false
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
	maxOutputTokens := execution.SemanticPlanMaxOutputTokens
	if maxOutputTokens <= 0 && aggregate.Facts.Resources.ProviderCeiling != nil {
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
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_terminal", "semantic provider request terminated without a complete document", nil)
	}
	if session.ProviderSubmitted {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_submission_unknown", "semantic provider submission has no durable outcome", nil)
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
	var previous *runtimecontract.RuntimeOriginalDiagnostic
	var correction *runtimecontract.RuntimeSemanticCorrectionDirective
	if ledger.Attempt == 1 {
		if len(ledger.Rejections) != 1 || ledger.CorrectionDirective == nil {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_correction_lineage_invalid", "semantic correction has no exact attempt-0 diagnostic", nil)
		}
		previous = ledger.Rejections[0].CloneOriginal()
		value := ledger.CorrectionDirective.Clone()
		correction = &value
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
	submitted, err := m.runtimeCandidateSession(ctx, aggregate, runtime.RuntimeInternalEventCandidateRequestSubmitted, session.LogicalRequestID, sequence, physicalCallID, nil, nil)
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
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_contract_error", err.Error(), nil)
	}
	if response.Feedback != nil {
		providerFailure := runtimeProviderRequestFailure(*response.Feedback)
		if response.Feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryProviderTransient {
			outcome := runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient, Code: firstNonEmptyString(providerFailure.Code, "provider_transient"), Signature: firstNonEmptyString(providerFailure.Signature, "provider-transient")}
			updated, recordErr := m.runtimeCandidateSession(ctx, aggregate, runtime.RuntimeInternalEventCandidateRequestOutcomeObserved, session.LogicalRequestID, 0, physicalCallID, &outcome, nil)
			if recordErr != nil {
				return runtime.RuntimeAggregate{}, recordErr
			}
			if providerFailure.Code == "provider_body_idle_timeout" {
				return m.failRuntimeSemanticContract(ctx, updated, "semantic_provider_body_idle_timeout", providerFailure.Reason, nil)
			}
			recovered, consumed, budgetErr := m.runtimeConsumeRecoveryBudget(ctx, updated, runtimecontract.RuntimeRecoveryBudgetProviderTransport)
			if budgetErr != nil {
				return runtime.RuntimeAggregate{}, budgetErr
			}
			if consumed {
				return recovered, nil
			}
			return m.failRuntimeSemanticContract(ctx, updated, "semantic_provider_transport_exhausted", providerFailure.Reason, nil)
		}
		return m.failRuntimeSemanticContract(ctx, aggregate, firstNonEmptyString(providerFailure.Code, "semantic_provider_failure"), providerFailure.Reason, nil)
	}
	if response.DocumentOutcome == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_outcome_missing", "semantic provider returned no typed document outcome", nil)
	}
	switch response.DocumentOutcome.Disposition {
	case runtimecontract.RuntimeProviderDocumentIncomplete:
		updated, recoveryErr := m.runtimeCandidateRequestRecoveryRequired(ctx, aggregate, session.LogicalRequestID, physicalCallID, *response.DocumentOutcome)
		if recoveryErr != nil {
			return runtime.RuntimeAggregate{}, recoveryErr
		}
		persisted, ok := runtimeCandidateRequestSession(updated, session.LogicalRequestID)
		if !ok {
			return runtime.RuntimeAggregate{}, fmt.Errorf("semantic request session disappeared after recovery fact")
		}
		if persisted.ExpectedDocument == nil || persisted.IncompleteOutcomes > persisted.ExpectedDocument.MaxRecoveryAttempts {
			return m.failRuntimeSemanticContract(ctx, updated, "semantic_document_recovery_exhausted", "semantic document remained incomplete after its sealed whole-document recovery budget", nil)
		}
		return updated, nil
	case runtimecontract.RuntimeProviderDocumentInvalid, runtimecontract.RuntimeProviderDocumentEmpty:
		return m.failRuntimeSemanticContract(ctx, aggregate, response.DocumentOutcome.Code, "semantic provider did not return one complete authority document", nil)
	case runtimecontract.RuntimeProviderDocumentComplete:
		if len(response.DocumentContent) == 0 {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_document_missing", "complete semantic outcome did not carry its document", nil)
		}
	default:
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_outcome_invalid", "semantic provider returned an unknown document outcome", nil)
	}
	completed, _, _, err := m.persistAndCompleteRuntimeCandidateRequestDocument(ctx, aggregate, session.LogicalRequestID, sequence, physicalCallID, response.DocumentContent)
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
	if ledger.Manifest == nil || ledger.Document == nil || ledger.Document.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic batch requires persisted coverage order", common.ErrConflict)
	}
	sequence := ledger.NextBatchSequence
	start := 0
	if len(ledger.Batches) != 0 {
		start = 0
		for _, existing := range ledger.Batches {
			start += len(existing.CoverageWorkItemIDs)
		}
	}
	batch, err := runtimecontract.CompileRuntimeSemanticPlanBatchFromOffset(*ledger.Manifest, *ledger.Document, sequence, start, logicalRequestID, maxItems)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventSemanticPlanBatchCompiled,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticPlanBatch: &batch,
	})
	return result.Aggregate, err
}

func (m *Biz) advanceRuntimeSemanticPlanBatch(ctx context.Context, aggregate runtime.RuntimeAggregate, batch runtimecontract.RuntimeSemanticPlanBatch, next runtimecontract.RuntimeSemanticPlanBatchState) (runtime.RuntimeAggregate, error) {
	if batch.BatchID == "" {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic batch identity is empty", common.ErrInvalidArgument)
	}
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventSemanticPlanBatchAdvanced,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticPlanBatch: &batch, SemanticPlanBatchNextState: next,
	})
	return result.Aggregate, err
}

func (m *Biz) observeRuntimeSemanticPlanBatchFeasibility(ctx context.Context, aggregate runtime.RuntimeAggregate, result runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult) (runtime.RuntimeAggregate, error) {
	if result.BatchID == "" {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic batch feasibility identity is empty", common.ErrInvalidArgument)
	}
	eventResult, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventSemanticPlanBatchFeasibilityObserved,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticPlanBatchFeasibilityResult: &result,
	})
	return eventResult.Aggregate, err
}

func (m *Biz) mergeRuntimeSemanticPlanBatchDocuments(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Manifest == nil || ledger.Document == nil {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic merge inputs are incomplete", common.ErrConflict)
	}
	merged, err := runtimecontract.MergeRuntimeSemanticPlanBatchDocuments(*ledger.Manifest, *ledger.Document, ledger.BatchDocuments)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventSemanticPlanBatchDocumentsMerged, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, SemanticPlanMergedDocument: &merged})
	return result.Aggregate, err
}

func (m *Biz) mergeRuntimeSemanticPlanBatchFeasibilities(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Manifest == nil || ledger.MergedDocument == nil {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: semantic feasibility merge inputs are incomplete", common.ErrConflict)
	}
	values, err := runtimecontract.MergeRuntimeSemanticPlanBatchFeasibilityResults(*ledger.Manifest, *ledger.MergedDocument, ledger.Batches, ledger.BatchFeasibilityResults)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	result, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventSemanticPlanBatchFeasibilitiesMerged, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, SemanticPlanMergedFeasibilities: values})
	return result.Aggregate, err
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
	payload, _ = runtimecontract.NormalizeRuntimeCandidatePayloadDocument(payload)
	document, decodeErr := runtimecontract.DecodeRuntimeSemanticPlanPayloadWithManifest(*ledger.Manifest, payload)
	if decodeErr == nil {
		updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind:        runtime.RuntimeInternalEventSemanticPlanEvaluated,
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			LogicalRequestID: session.LogicalRequestID, SemanticDocument: &document,
		})
		return updated.Aggregate, err
	}
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
	if ledger.Manifest == nil || len(diagnostic.SemanticViolations) == 0 {
		return m.failRuntimeSemanticContract(ctx, aggregate, "runtime_contract_defect", "semantic plan rejection has no sealed typed correction delta", &diagnostic)
	}
	directive := runtimecontract.RuntimeSemanticCorrectionDirective{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      expected.PlanStep, ManifestHash: ledger.Manifest.Hash,
		DefinitionSeedHash: ledger.Manifest.DefinitionSeedHash, StablePrefixHash: ledger.StablePrefixHash,
		SourceRejectionHash: runtimecontract.StableRuntimeSemanticPlanCorrectionHash(diagnostic), Attempt: 1,
		Violations: append([]runtimecontract.RuntimeSemanticContractViolation(nil), diagnostic.SemanticViolations...),
	}
	directive.Hash = runtimecontract.StableRuntimeSemanticCorrectionDirectiveHash(directive)
	if err := directive.ValidateAgainst(*ledger.Manifest, diagnostic); err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "runtime_contract_defect", err.Error(), &diagnostic)
	}
	ensured, correctionSession, err := m.runtimeEnsureCandidateRequestWithExpected(ctx, aggregate, runtimecontract.RuntimeProviderDocumentSemanticPlan, ledger.StablePrefixHash, ledger.Manifest.Hash, directive.Hash, expected)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventSemanticPlanCorrectionRequired,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		StablePrefixHash: ledger.StablePrefixHash, LogicalRequestID: correctionSession.LogicalRequestID,
		SemanticCorrectionDirective: &directive, SemanticDiagnostic: &diagnostic,
	})
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	if updated.Aggregate.StateRevision <= ensured.StateRevision {
		return runtime.RuntimeAggregate{}, fmt.Errorf("semantic correction did not persist its private transition")
	}
	return updated.Aggregate, nil
}

func (m *Biz) driveRuntimeSemanticFeasibility(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	feasibilityDocument, found := runtimeSemanticActiveFeasibilityDocument(ledger)
	if ledger.Manifest == nil || !found {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_feasibility_context_invalid", "semantic feasibility has no persisted manifest or document", nil)
	}
	for _, fact := range ledger.Feasibilities {
		switch fact.Status {
		case runtimecontract.RuntimeSemanticFeasibilityStale, runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired:
			if fact.OriginalDiagnostic == nil {
				return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_feasibility_failed", "semantic owner feasibility omitted its original diagnostic", nil)
			}
			policy := aggregate.Facts.RecoveryPolicy.Normalized()
			if aggregate.Facts.Recovery.PrerequisiteRefreshes >= policy.MaxPrerequisiteRefreshes {
				return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_catalog_refresh_exhausted", fact.OriginalDiagnostic.OriginalMessage, fact.OriginalDiagnostic)
			}
			updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
				Kind:        runtime.RuntimeInternalEventSemanticContractRequired,
				WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
				SemanticBatchID: ledger.FeasibilityBatchID, SemanticRefreshGeneration: ledger.CatalogRefreshAttempts,
			})
			return updated.Aggregate, err
		case runtimecontract.RuntimeSemanticFeasibilityInfeasible:
			if fact.OriginalDiagnostic == nil {
				return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_feasibility_failed", "semantic owner feasibility was not accepted", nil)
			}
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_feasibility_failed", fact.OriginalDiagnostic.OriginalMessage, fact.OriginalDiagnostic)
		}
	}
	observed := make(map[common.CoverageWorkItemID]struct{}, len(ledger.Feasibilities))
	for _, fact := range ledger.Feasibilities {
		observed[fact.CoverageWorkItemID] = struct{}{}
	}
	for _, proposal := range feasibilityDocument.Proposals {
		if _, done := observed[proposal.CoverageWorkItemID]; done {
			continue
		}
		return m.probeRuntimeSemanticProposal(ctx, aggregate, proposal)
	}
	sealed, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventSemanticContractSealed,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
	})
	return sealed.Aggregate, err
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
	workspace, err := (workspaceEventRepository{hub: m.EventHub()}).Get(ctx, aggregate.Facts.WorkspaceID)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	catalog, found := runtimeSemanticManifestOwnerCatalog(*ledger.Manifest, proposal.OwnerModuleID)
	if !found {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_catalog_missing", "semantic proposal has no sealed owner catalog", nil)
	}
	observation := runtimeSemanticCatalogObservation(aggregate, catalog)
	response, err := ProbeRuntimeSemanticProofFeasibility(ctx, m.EventHub(), runtimecontract.RuntimeSemanticProofFeasibilityRequest{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		FeasibilityBatchID: ledger.FeasibilityBatchID, WorkspaceDeliverableDirectory: workspace.DeliverableDirectory,
		Manifest: *ledger.Manifest, Proposal: proposal, OwnerCatalog: catalog, CatalogObservation: observation,
	})
	if err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_owner_probe_invalid", err.Error(), nil)
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventSemanticFeasibilityObserved,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticBatchID: ledger.FeasibilityBatchID, SemanticFeasibility: &response.Observation,
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
	for _, observation := range aggregate.Facts.Resources.PrerequisiteObservations {
		if observation.PrerequisiteID == catalog.PrerequisiteID && observation.OwnerModuleID == catalog.OwnerModuleID && observation.ContentHash == catalog.SnapshotID {
			value := observation.Normalized()
			return &value
		}
	}
	return nil
}

func (m *Biz) failRuntimeSemanticContract(ctx context.Context, aggregate runtime.RuntimeAggregate, code, message string, original *runtimecontract.RuntimeOriginalDiagnostic) (runtime.RuntimeAggregate, error) {
	// Close the generic provider-I/O ledger before sealing the SM-2 semantic
	// terminal.  A crash between these two owner-private CAS transitions is
	// recoverable: the next drive observes the terminal request session and
	// deterministically seals the same semantic failure.  Leaving an open or
	// unresolved session beside a semantic terminal would create a second
	// apparent drive path and can trigger recovery busy-loops.
	if logicalRequestID := strings.TrimSpace(aggregate.SemanticPlanning.LogicalRequestID); logicalRequestID != "" {
		if session, found := runtimeCandidateRequestSession(aggregate, logicalRequestID); found &&
			session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete &&
			session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal {
			outcome := runtimeSemanticRequestTerminalOutcome(code)
			terminated, terminateErr := m.runtimeCandidateSession(ctx, aggregate, runtime.RuntimeInternalEventCandidateRequestTerminated, logicalRequestID, 0, "", &outcome, nil)
			if terminateErr != nil {
				return runtime.RuntimeAggregate{}, terminateErr
			}
			aggregate = terminated
		}
	}
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
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventSemanticContractFailed,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, SemanticDiagnostic: &diagnostic,
	})
	return updated.Aggregate, err
}

func runtimeSemanticRequestTerminalOutcome(code string) runtimecontract.RuntimeCandidateRequestOutcome {
	code = strings.TrimSpace(code)
	class := runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol
	switch code {
	case "semantic_provider_submission_unknown":
		class = runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown
	case "semantic_document_recovery_exhausted":
		class = runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted
	}
	return runtimecontract.RuntimeCandidateRequestOutcome{
		Class: class, Code: code, Signature: strings.ReplaceAll(code, "_", "-"),
	}
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
