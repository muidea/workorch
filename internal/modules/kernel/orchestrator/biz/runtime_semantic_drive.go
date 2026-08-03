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
		if aggregate.Facts.Resources.ProviderCeiling == nil {
			return m.acquireRuntimeSemanticProviderCeiling(ctx, aggregate)
		}
		if prerequisite, found := pendingRuntimeSemanticOwnerCatalog(aggregate); found {
			return m.acquireRuntimeSemanticOwnerCatalog(ctx, aggregate, prerequisite)
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
		ensured, session, err := m.runtimeEnsureCandidateRequest(ctx, aggregate, runtimecontract.RuntimeProviderDocumentSemanticPlan, stablePrefixHash, manifest.Hash, "")
		if err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		started, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
			Kind:        runtime.RuntimeInternalEventSemanticPlanRequestStarted,
			WorkspaceID: ensured.Facts.WorkspaceID, RunID: ensured.Facts.RunID,
			SemanticManifest: &manifest, StablePrefixHash: stablePrefixHash, LogicalRequestID: session.LogicalRequestID,
		})
		return started.Aggregate, err
	case runtimecontract.RuntimeSemanticPlanningRequestOpen, runtimecontract.RuntimeSemanticPlanningCorrectionRequired:
		return m.requestRuntimeSemanticPlanDocument(ctx, aggregate)
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

func pendingRuntimeSemanticOwnerCatalog(aggregate runtime.RuntimeAggregate) (runtimecontract.RuntimePrerequisite, bool) {
	for _, prerequisite := range aggregate.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility {
			return prerequisite, true
		}
	}
	return runtimecontract.RuntimePrerequisite{}, false
}

func (m *Biz) acquireRuntimeSemanticOwnerCatalog(ctx context.Context, aggregate runtime.RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite) (runtime.RuntimeAggregate, error) {
	workspace, err := (workspaceEventRepository{hub: m.EventHub()}).Get(ctx, aggregate.Facts.WorkspaceID)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	result, err := m.resolveRuntimePrerequisite(ctx, aggregate, workspace.DeliverableDirectory, workspace.InternalDirectory, prerequisite)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	committed, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventPrerequisiteObserved,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, PrerequisiteResult: &result,
	})
	return committed.Aggregate, err
}

func (m *Biz) acquireRuntimeSemanticProviderCeiling(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	var prerequisite *runtimecontract.RuntimePrerequisite
	for idx := range aggregate.Facts.Prerequisites.Pending {
		if aggregate.Facts.Prerequisites.Pending[idx].Kind == runtimecontract.PrerequisiteKindProviderCeiling {
			value := aggregate.Facts.Prerequisites.Pending[idx]
			prerequisite = &value
			break
		}
	}
	if prerequisite == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_ceiling_missing", "semantic planning has no sealed provider ceiling prerequisite", nil)
	}
	workspace, err := (workspaceEventRepository{hub: m.EventHub()}).Get(ctx, aggregate.Facts.WorkspaceID)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	result, err := m.resolveRuntimePrerequisite(ctx, aggregate, workspace.DeliverableDirectory, workspace.InternalDirectory, *prerequisite)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	committed, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventPrerequisiteObserved,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, PrerequisiteResult: &result,
	})
	return committed.Aggregate, err
}

func (m *Biz) requestRuntimeSemanticPlanDocument(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	ledger := aggregate.SemanticPlanning.Clone()
	if ledger.Manifest == nil || ledger.LogicalRequestID == "" || aggregate.Facts.Resources.ProviderCeiling == nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_context_invalid", "semantic request is missing manifest, request identity, or provider ceiling", nil)
	}
	manifest := ledger.Manifest.Clone()
	session, found := runtimeCandidateRequestSession(aggregate, ledger.LogicalRequestID)
	if !found {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_request_session_missing", "semantic request session is not persisted", nil)
	}
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

	semanticContext := runtimecontract.RuntimeSemanticPlanRequestContext{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Definition: aggregate.Definition, DefinitionSeed: aggregate.Facts.DefinitionSeed,
		Activation: aggregate.Facts.Activation, ProviderCeiling: aggregate.Facts.Resources.ProviderCeiling,
	}
	var previous *runtimecontract.RuntimeOriginalDiagnostic
	if ledger.Attempt == 1 {
		if len(ledger.Rejections) != 1 {
			return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_correction_lineage_invalid", "semantic correction has no exact attempt-0 diagnostic", nil)
		}
		previous = ledger.Rejections[0].CloneOriginal()
	}
	preflight, err := PreflightRuntimeCandidate(ctx, m.EventHub(), llmevents.PreflightRuntimeCandidateCommand{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DocumentKind:     runtimecontract.RuntimeProviderDocumentSemanticPlan,
		CandidateAttempt: ledger.Attempt, StablePrefixHash: ledger.StablePrefixHash,
		SemanticManifest: manifest, SemanticContext: semanticContext,
		PreviousSemanticRejection: previous,
	})
	if err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_context_integrity", err.Error(), nil)
	}
	if preflight.Feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryContextIntegrity {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_context_integrity", preflight.Feedback.OriginalDiagnostic.OriginalMessage, preflight.Feedback.OriginalDiagnostic)
	}

	sequence := session.NextSequence
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, sequence)
	var continuation *runtimecontract.RuntimeCandidateOutputContinuationRequest
	if len(session.Fragments) != 0 {
		policy := aggregate.Facts.Activation.ExecutionContract.Normalized().CandidateOutputContinuation
		request, nextCallID, continuationErr := prepareRuntimeCandidateRequestContinuation(ctx, m.evidence.ArtifactContinuations, aggregate, session.LogicalRequestID, policy.TailBytes)
		if continuationErr != nil {
			return runtime.RuntimeAggregate{}, continuationErr
		}
		continuation, physicalCallID = &request, nextCallID
	}
	submitted, err := m.runtimeCandidateSession(ctx, aggregate, runtime.RuntimeInternalEventCandidateRequestSubmitted, session.LogicalRequestID, sequence, physicalCallID, nil, nil, nil)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	aggregate = submitted
	response, err := RequestRuntimeCandidate(ctx, m.EventHub(), llmevents.RequestRuntimeCandidateCommand{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DocumentKind:     runtimecontract.RuntimeProviderDocumentSemanticPlan,
		RequestIdentity:  runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: session.LogicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID},
		CandidateAttempt: ledger.Attempt, StablePrefixHash: ledger.StablePrefixHash,
		SemanticManifest: manifest, SemanticContext: semanticContext,
		PreviousSemanticRejection: previous, OutputContinuation: continuation,
	})
	if err != nil {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_contract_error", err.Error(), nil)
	}
	if response.OutputFragment != nil {
		updated, _, err := m.persistAndRecordRuntimeCandidateRequestFragment(ctx, aggregate, session.LogicalRequestID, sequence, physicalCallID, response.OutputFragment.Content, response.OutputFragment.ReachedProviderLimit)
		if err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		assembled, err := assembleRuntimeCandidateRequestFragments(ctx, m.evidence.ArtifactContinuations, updated, session.LogicalRequestID)
		if err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		if runtimecontract.InspectRuntimeCandidateDocumentFraming(assembled) == runtimecontract.RuntimeCandidateDocumentIncomplete {
			persisted, ok := runtimeCandidateRequestSession(updated, session.LogicalRequestID)
			if !ok {
				return runtime.RuntimeAggregate{}, fmt.Errorf("semantic request session disappeared after fragment commit")
			}
			if len(persisted.Fragments)-1 >= updated.Facts.Activation.ExecutionContract.Normalized().CandidateOutputContinuation.MaxChunks {
				return m.failRuntimeSemanticContract(ctx, updated, "semantic_output_continuation_exhausted", "semantic document remained incomplete after its sealed continuation budget", nil)
			}
			return updated, nil
		}
		completed, _, _, err := m.persistAndCompleteRuntimeCandidateRequestDocument(ctx, updated, session.LogicalRequestID, sequence, physicalCallID, assembled)
		if err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		return m.admitRuntimeCompletedSemanticPlanDocument(ctx, completed, session)
	}
	if response.Feedback != nil {
		providerFailure := runtimeProviderRequestFailure(*response.Feedback)
		if response.Feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryProviderTransient {
			outcome := runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient, Code: firstNonEmptyString(providerFailure.Code, "provider_transient"), Signature: firstNonEmptyString(providerFailure.Signature, "provider-transient")}
			updated, recordErr := m.runtimeCandidateSession(ctx, aggregate, runtime.RuntimeInternalEventCandidateRequestOutcomeObserved, session.LogicalRequestID, 0, physicalCallID, &outcome, nil, nil)
			if recordErr != nil {
				return runtime.RuntimeAggregate{}, recordErr
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
	if len(response.DocumentContent) == 0 {
		return m.failRuntimeSemanticContract(ctx, aggregate, "semantic_provider_document_missing", "semantic provider returned no complete document", nil)
	}
	completed, _, _, err := m.persistAndCompleteRuntimeCandidateRequestDocument(ctx, aggregate, session.LogicalRequestID, sequence, physicalCallID, response.DocumentContent)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	return m.admitRuntimeCompletedSemanticPlanDocument(ctx, completed, session)
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
	correctionHash := runtimecontract.StableRuntimeSemanticPlanCorrectionHash(diagnostic)
	ensured, correctionSession, err := m.runtimeEnsureCandidateRequest(ctx, aggregate, runtimecontract.RuntimeProviderDocumentSemanticPlan, ledger.StablePrefixHash, ledger.Manifest.Hash, correctionHash)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventSemanticPlanCorrectionRequired,
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		StablePrefixHash: ledger.StablePrefixHash, LogicalRequestID: correctionSession.LogicalRequestID,
		CorrectionHash: correctionHash, SemanticDiagnostic: &diagnostic,
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
	if ledger.Manifest == nil || ledger.Document == nil {
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
	for _, proposal := range ledger.Document.Proposals {
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
			terminated, terminateErr := m.runtimeCandidateSession(ctx, aggregate, runtime.RuntimeInternalEventCandidateRequestTerminated, logicalRequestID, 0, "", &outcome, nil, nil)
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
	case "semantic_output_continuation_exhausted":
		class = runtimecontract.RuntimeCandidateRequestOutcomeContinuationExhausted
	}
	return runtimecontract.RuntimeCandidateRequestOutcome{
		Class: class, Code: code, Signature: strings.ReplaceAll(code, "_", "-"),
	}
}

func runtimeSemanticPayloadDiagnostic(err error) runtimecontract.RuntimeOriginalDiagnostic {
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, OriginalClass: "semantic_plan_rejection",
		OriginalCode: "semantic_plan_invalid", OriginalMessage: err.Error(), Signature: "semantic-plan/invalid-document",
	}
	var payloadDiagnostic *runtimecontract.RuntimeCandidatePayloadDiagnostic
	if errors.As(err, &payloadDiagnostic) {
		diagnostic.FieldViolations = []runtimecontract.RuntimeFieldViolation{{
			Path: payloadDiagnostic.Path, Actual: payloadDiagnostic.Actual,
			Allowed: append([]string(nil), payloadDiagnostic.Allowed...), Required: append([]string(nil), payloadDiagnostic.Required...), Forbidden: append([]string(nil), payloadDiagnostic.Forbidden...),
		}}
	}
	return diagnostic
}
