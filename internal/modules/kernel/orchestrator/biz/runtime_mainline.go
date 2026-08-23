package orchestrator

// Runtime mainline is owned by Biz.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeDefaultTargetSnapshotBytes = 256 * 1024

type runtimePrerequisiteResolverClass string

const (
	runtimePrerequisiteResolverExternal runtimePrerequisiteResolverClass = "external_owner"
	runtimePrerequisiteResolverLocal    runtimePrerequisiteResolverClass = "orchestrator_local"
	runtimePrerequisiteResolverUnknown  runtimePrerequisiteResolverClass = "unreachable"
)

// runtimePrerequisiteResolverClassification is the production inventory
// boundary for resolver cases. Keep it adjacent to the resolver so a newly
// added PrerequisiteKind cannot become an undocumented fourth path.
func runtimePrerequisiteResolverClassification(kind runtimecontract.PrerequisiteKind) runtimePrerequisiteResolverClass {
	switch kind {
	case runtimecontract.PrerequisiteKindManifest, runtimecontract.PrerequisiteKindTargetSnapshot,
		runtimecontract.PrerequisiteKindTargetSelection, runtimecontract.PrerequisiteKindProviderCeiling,
		runtimecontract.PrerequisiteKindCapability, runtimecontract.PrerequisiteKindOperationAdmissibility,
		runtimecontract.PrerequisiteKindCapabilityState:
		return runtimePrerequisiteResolverExternal
	case runtimecontract.PrerequisiteKindFailureTarget, runtimecontract.PrerequisiteKindValidationScratch:
		return runtimePrerequisiteResolverLocal
	default:
		return runtimePrerequisiteResolverUnknown
	}
}

func (m *Biz) requestAndSealRuntimeCandidate(
	ctx context.Context,
	aggregate runtime.RuntimeAggregate,
	stablePrefixHash string,
	candidateAttempt int,
) (runtime.RuntimeAggregate, error) {
	if m == nil || m.runtimeController == nil || m.EventHub() == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	switch aggregate.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment, runtimecontract.ConvergenceStateNeedGovernance:
	default:
		return runtime.RuntimeAggregate{}, fmt.Errorf("convergence state %s does not request an LLM candidate", aggregate.Decision.State)
	}
	if err := runtimeEnsureCandidateFeasible(aggregate); err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(
		aggregate.Facts,
		aggregate.Decision,
		runtime.ActiveRejectedOperationShapes(aggregate),
	)
	manifest, manifestErr := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(candidateContext))
	if manifestErr != nil {
		return m.runtimeRecordFailure(ctx, aggregate, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_candidate_manifest_invalid",
			Signature: "runtime-candidate-manifest-invalid", Message: manifestErr.Error(),
		})
	}
	// This is the persisted-authority half of the P0/P2/P4 preflight. It runs
	// before BeginRuntimeCandidateAttempt so an internally incomplete sealed
	// context cannot spend the model correction budget. The LLM Block performs
	// the second, rendered-section audit gate immediately before provider I/O.
	if err := aggregate.Definition.Validate(); err != nil {
		return m.runtimeRecordFailure(ctx, aggregate, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_context_integrity",
			Signature: "runtime-context-integrity", Message: err.Error(),
		})
	}
	if err := candidateContext.Validate(aggregate.Decision); err != nil {
		return m.runtimeRecordFailure(ctx, aggregate, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_context_integrity",
			Signature: "runtime-context-integrity", Message: err.Error(),
		})
	}
	// The LLM owner is the sole renderer, so the rendered C0-C5/P0-P4 integrity
	// gate must run through its typed EventHub contract before SM-3 creates an
	// attempt. A persisted proposal or document-recovery session has already passed this
	// boundary; re-rendering it on restart would add a non-authoritative failure
	// point to an otherwise idempotent seal path.
	// Runtime preflight acquisition guarantees a sealed provider ceiling before
	// any provider request. Persisted proposals do not re-enter this boundary.
	if candidateContext.Resources.ProviderCeiling != nil {
		if _, proposalPersisted := runtimePendingCandidateProposal(aggregate, candidateAttempt); !proposalPersisted {
			previousFailure, previousErr := runtimePreviousCandidateFailure(aggregate, candidateAttempt)
			if previousErr != nil {
				return runtime.RuntimeAggregate{}, previousErr
			}
			preflight, preflightErr := PreflightRuntimeCandidate(ctx, m.EventHub(), llmevents.PreflightRuntimeCandidateCommand{
				WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
				DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
				Decision:     aggregate.Decision, Transaction: aggregate.Decision.AuthorityTransaction(), CandidateAttempt: candidateAttempt,
				PreviousFailure:  previousFailure,
				StablePrefixHash: stablePrefixHash, CandidateManifest: manifest, DefinitionContext: aggregate.Definition, CandidateContext: candidateContext,
			})
			if preflightErr != nil {
				return m.runtimeRecordFailure(ctx, aggregate, runtimecontract.FailureFact{
					Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_context_integrity",
					Signature: "runtime-context-integrity", Message: preflightErr.Error(),
				})
			}
			if preflight.Feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryContextIntegrity {
				return m.runtimeRecordFailure(ctx, aggregate, runtimecontract.FailureFact{
					Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_context_integrity",
					Signature: preflight.Feedback.OriginalDiagnostic.Signature, Message: preflight.Feedback.OriginalDiagnostic.OriginalMessage,
				})
			}
		}
	}
	candidate, proposalPersisted := runtimePendingCandidateProposal(aggregate, candidateAttempt)
	if !proposalPersisted {
		var candidateReady bool
		var requestErr error
		aggregate, candidate, candidateReady, requestErr = m.requestRuntimeCandidateDocument(ctx, aggregate, candidateContext, manifest, stablePrefixHash, candidateAttempt)
		if requestErr != nil {
			return runtime.RuntimeAggregate{}, requestErr
		}
		if !candidateReady {
			return aggregate, nil
		}
	}
	if len(aggregate.Facts.Resources.OperationAdmissibilities) > 0 {
		workspace, workspaceErr := (workspaceSnapshotReader{hub: m.EventHub()}).Read(ctx, aggregate.Facts.WorkspaceID)
		if workspaceErr != nil {
			return m.runtimeRejectCandidate(ctx, aggregate, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassInternal, Code: "workspace_resolution_failed", Reason: workspaceErr.Error(),
			})
		}
		if ownerInfeasibility, stale, prerequisiteRequest, probeErr := m.probeRuntimeCandidateFeasibility(ctx, aggregate, workspace.DeliverableDirectory, candidate, manifest, candidateAttempt); probeErr != nil {
			return m.runtimeRejectCandidate(ctx, aggregate, candidateAttempt, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "operation_feasibility_probe_invalid", Reason: probeErr.Error(),
			})
		} else if stale != nil {
			return m.runtimeCandidateStale(ctx, aggregate, candidateAttempt, *stale)
		} else if prerequisiteRequest != nil {
			return m.runtimeCandidatePrerequisite(ctx, aggregate, candidateAttempt, *prerequisiteRequest)
		} else if ownerInfeasibility != nil {
			return m.runtimeOwnerFeasibility(ctx, aggregate, candidateAttempt, *ownerInfeasibility)
		}
	}
	record, bound := runtimeCandidateAttemptRecord(aggregate, candidateAttempt)
	if !bound {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: pending candidate attempt has no persisted request-session binding", common.ErrConflict)
	}
	sealed, sealErr := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventCandidateSealed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Attempt: candidateAttempt, Manifest: &manifest, LogicalRequestID: record.LogicalRequestID, Candidate: &candidate},
	})
	if sealErr != nil {
		return runtime.RuntimeAggregate{}, sealErr
	}
	return sealed.Aggregate, nil
}

// requestRuntimeCandidateDocument owns the provider-I/O part of one sealed
// candidate request. C4 deliberately persists the session and physical call
// before EventHub provider I/O; no candidate attempt exists until a complete
// document is stored. It returns candidateReady only after the subsequent C5
// admission transition has persisted a proposal.
func (m *Biz) requestRuntimeCandidateDocument(
	ctx context.Context,
	aggregate runtime.RuntimeAggregate,
	candidateContext runtimecontract.RuntimeCandidateContext,
	manifest runtimecontract.RuntimeCandidateContractManifest,
	stablePrefixHash string,
	candidateAttempt int,
) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	previousFailure, err := runtimePreviousCandidateFailure(aggregate, candidateAttempt)
	if err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(manifest, aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(candidateContext)); err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	correctionHash := runtimecontract.StableRuntimeCandidateRequestCorrectionHash(previousFailure)
	expected := runtimeOperationsExpectedDocument(aggregate, manifest.Hash, candidateAttempt)
	ensured, session, err := m.runtimeEnsureCandidateRequestWithExpected(ctx, aggregate, runtimecontract.RuntimeProviderDocumentOperations, stablePrefixHash, manifest.Hash, correctionHash, expected)
	if err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	aggregate = ensured
	progress, progressErr := m.resumeRuntimeCandidateDocumentSession(ctx, aggregate, manifest, session, candidateAttempt)
	if progress.handled {
		return progress.aggregate, progress.candidate, progress.ready, progressErr
	}

	sequence := session.NextSequence
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, sequence)
	requestIdentity := runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: session.LogicalRequestID, Sequence: sequence, PhysicalCallID: physicalCallID}
	if err := requestIdentity.Validate(); err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	submitted, err := m.runtimeCandidateSession(ctx, aggregate, runtime.RuntimeInternalEventCandidateRequestSubmitted, session.LogicalRequestID, sequence, physicalCallID, nil)
	if err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	aggregate = submitted
	response, requestErr := RequestRuntimeCandidate(ctx, m.EventHub(), llmevents.RequestRuntimeCandidateCommand{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DocumentKind:     runtimecontract.RuntimeProviderDocumentOperations,
		RequestIdentity:  requestIdentity,
		ExpectedDocument: session.ExpectedDocument,
		Decision:         aggregate.Decision, Transaction: aggregate.Decision.AuthorityTransaction(), CandidateAttempt: candidateAttempt,
		PreviousFailure: previousFailure, StablePrefixHash: stablePrefixHash, CandidateManifest: manifest, DefinitionContext: aggregate.Definition,
		CandidateContext: candidateContext,
	})
	if requestErr != nil {
		settlement, settlementErr := runtimeProviderBudgetSettlement(string(aggregate.Facts.WorkspaceID), string(aggregate.Facts.RunID), session.LogicalRequestID, physicalCallID, nil, nil)
		if settlementErr != nil {
			return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, settlementErr
		}
		return m.terminateRuntimeCandidateRequestSubmissionUnknown(ctx, aggregate, session.LogicalRequestID, settlement)
	}
	return m.handleRuntimeCandidateProviderResponse(ctx, aggregate, manifest, session, response, candidateAttempt, sequence, physicalCallID)
}

type runtimeCandidateDocumentProgress struct {
	aggregate runtime.RuntimeAggregate
	candidate runtimecontract.RuntimeCandidate
	ready     bool
	handled   bool
}

func (m *Biz) resumeRuntimeCandidateDocumentSession(ctx context.Context, aggregate runtime.RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, session runtimecontract.RuntimeCandidateRequestSession, candidateAttempt int) (runtimeCandidateDocumentProgress, error) {
	if aggregate.Decision.State == runtimecontract.ConvergenceStateFailed || aggregate.Decision.State == runtimecontract.ConvergenceStateComplete {
		return runtimeCandidateDocumentProgress{aggregate: aggregate, handled: true}, nil
	}
	if session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		updated, candidate, ready, err := m.admitRuntimeCompletedCandidateDocument(ctx, aggregate, manifest, session.LogicalRequestID, candidateAttempt)
		return runtimeCandidateDocumentProgress{aggregate: updated, candidate: candidate, ready: ready, handled: true}, err
	}
	if session.State == runtimecontract.RuntimeCandidateRequestSessionTerminal {
		if !runtime.ProviderRequestTerminalSessionApplied(aggregate, session) {
			return runtimeCandidateDocumentProgress{handled: true}, fmt.Errorf("%w: operations provider request is terminal without its coordinated SM-2 failure", common.ErrConflict)
		}
		return runtimeCandidateDocumentProgress{aggregate: aggregate, handled: true}, nil
	}
	if !session.ProviderSubmitted {
		return runtimeCandidateDocumentProgress{}, nil
	}
	settlement, settlementErr := runtimeProviderBudgetSettlement(string(aggregate.Facts.WorkspaceID), string(aggregate.Facts.RunID), session.LogicalRequestID, session.LastPhysicalCallID, nil, nil)
	if settlementErr != nil {
		return runtimeCandidateDocumentProgress{handled: true}, settlementErr
	}
	failed, err := m.runtimeCoordinateProviderRequestTerminal(ctx, aggregate, session.LogicalRequestID, runtime.RuntimeProviderRequestTerminalInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown,
		TerminalCode: "provider_submission_unknown", TerminalSignature: "provider-submission-unknown",
		Message: "provider request was submitted without a durable outcome",
	}, settlement)
	return runtimeCandidateDocumentProgress{aggregate: failed, handled: true}, err
}

func (m *Biz) handleRuntimeCandidateProviderResponse(ctx context.Context, aggregate runtime.RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, session runtimecontract.RuntimeCandidateRequestSession, response llmevents.RequestRuntimeCandidateResult, candidateAttempt, sequence int, physicalCallID string) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	settlement, settlementErr := runtimeProviderBudgetSettlement(string(aggregate.Facts.WorkspaceID), string(aggregate.Facts.RunID), session.LogicalRequestID, physicalCallID, response.Audit, response.Usage)
	if settlementErr != nil || settlement.Status == runtime.RuntimeProviderBudgetSettlementUnknown {
		unknown, unknownErr := runtimeProviderBudgetSettlement(string(aggregate.Facts.WorkspaceID), string(aggregate.Facts.RunID), session.LogicalRequestID, physicalCallID, nil, nil)
		if unknownErr != nil {
			return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, unknownErr
		}
		return m.terminateRuntimeCandidateRequestSubmissionUnknown(ctx, aggregate, session.LogicalRequestID, unknown)
	}
	if response.Transaction != aggregate.Decision.AuthorityTransaction() {
		return m.terminateRuntimeCandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, "provider_response_transaction_mismatch", "runtime candidate response changed the sealed authority transaction", nil, settlement)
	}
	if response.Stale != nil {
		updated, staleErr := m.runtimeCandidateSession(ctx, aggregate, runtime.RuntimeInternalEventCandidateRequestStaleObserved, session.LogicalRequestID, 0, "", response.Stale, settlement)
		return updated, runtimecontract.RuntimeCandidate{}, false, staleErr
	}
	if response.Feedback != nil {
		if response.Feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryProviderTransient && settlement.Status != runtime.RuntimeProviderBudgetSettlementConfirmed {
			return m.terminateRuntimeCandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, "provider_submission_fact_invalid", "unsubmitted provider result cannot request transport recovery", response.Feedback.OriginalDiagnostic, settlement)
		}
		return m.handleRuntimeCandidateProviderFeedback(ctx, aggregate, session, *response.Feedback, physicalCallID, settlement)
	}
	if response.DocumentOutcome == nil {
		return m.terminateRuntimeCandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, "provider_document_outcome_missing", "candidate response did not carry a typed provider document outcome", nil, settlement)
	}
	if settlement.Status != runtime.RuntimeProviderBudgetSettlementConfirmed {
		return m.terminateRuntimeCandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, "provider_submission_fact_invalid", "unsubmitted provider result cannot carry a provider document", nil, settlement)
	}
	return m.handleRuntimeCandidateDocumentOutcome(ctx, aggregate, manifest, session, response, candidateAttempt, sequence, physicalCallID, settlement)
}

func (m *Biz) handleRuntimeCandidateProviderFeedback(ctx context.Context, aggregate runtime.RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession, feedback runtimecontract.RuntimeFeedback, physicalCallID string, settlement *runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	providerFailure := runtimeProviderRequestFailure(feedback)
	if feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryProviderTransient {
		return m.handleRuntimeCandidateTransientFeedback(ctx, aggregate, session, providerFailure, feedback.OriginalDiagnostic, physicalCallID, settlement)
	}
	if feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryProviderAuthentication || feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryProviderConfiguration {
		outcomeClass := runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration
		terminalCode := "provider_configuration"
		if feedback.StableCategory == runtimecontract.RuntimeFeedbackCategoryProviderAuthentication {
			outcomeClass = runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication
			terminalCode = "provider_authentication"
		}
		failed, err := m.runtimeCoordinateProviderRequestTerminal(ctx, aggregate, session.LogicalRequestID, runtime.RuntimeProviderRequestTerminalInput{
			DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, OutcomeClass: outcomeClass,
			TerminalCode: terminalCode, TerminalSignature: strings.ReplaceAll(terminalCode, "_", "-"),
			Message: providerFailure.Reason, OriginalDiagnostic: feedback.OriginalDiagnostic,
		}, settlement)
		return failed, runtimecontract.RuntimeCandidate{}, false, err
	}
	return m.terminateRuntimeCandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, "provider_protocol_failure", providerFailure.Reason, feedback.OriginalDiagnostic, settlement)
}

func (m *Biz) handleRuntimeCandidateTransientFeedback(ctx context.Context, aggregate runtime.RuntimeAggregate, session runtimecontract.RuntimeCandidateRequestSession, failure runtimecontract.RuntimeCandidateFailure, originalDiagnostic *runtimecontract.RuntimeOriginalDiagnostic, physicalCallID string, settlement *runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	outcome := runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient, Code: firstNonEmptyString(failure.Code, "provider_transient"), Signature: firstNonEmptyString(failure.Signature, "provider-transient"),
	}
	updated, err := m.runtimeCoordinateProviderRequestRecovery(ctx, aggregate, session.LogicalRequestID, physicalCallID, runtime.RuntimeProviderRequestRecoveryInput{
		DocumentKind:     runtimecontract.RuntimeProviderDocumentOperations,
		TransportOutcome: &outcome, Message: failure.Reason, OriginalDiagnostic: originalDiagnostic,
	}, settlement)
	return updated, runtimecontract.RuntimeCandidate{}, false, err
}

func (m *Biz) handleRuntimeCandidateDocumentOutcome(ctx context.Context, aggregate runtime.RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, session runtimecontract.RuntimeCandidateRequestSession, response llmevents.RequestRuntimeCandidateResult, candidateAttempt, sequence int, physicalCallID string, settlement *runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	framing := response.DocumentOutcome.Framing
	switch framing.Disposition {
	case runtimecontract.RuntimeProviderDocumentIncomplete:
		updated, recoveryErr := m.runtimeCoordinateProviderRequestRecovery(ctx, aggregate, session.LogicalRequestID, physicalCallID, runtime.RuntimeProviderRequestRecoveryInput{
			DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, DocumentFraming: &framing,
		}, settlement)
		if recoveryErr != nil {
			return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, recoveryErr
		}
		persisted, found := runtimeCandidateRequestSession(updated, session.LogicalRequestID)
		if !found {
			return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("runtime candidate request session %q disappeared after recovery fact", session.LogicalRequestID)
		}
		if persisted.State == runtimecontract.RuntimeCandidateRequestSessionTerminal {
			if err := m.markRuntimeCandidateRequestContentFailed(ctx, updated, session.LogicalRequestID); err != nil {
				return updated, runtimecontract.RuntimeCandidate{}, false, err
			}
		}
		return updated, runtimecontract.RuntimeCandidate{}, false, nil
	case runtimecontract.RuntimeProviderDocumentInvalid, runtimecontract.RuntimeProviderDocumentEmpty:
		return m.terminateRuntimeCandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, framing.Code, "provider did not return one complete authority document", nil, settlement)
	case runtimecontract.RuntimeProviderDocumentComplete:
		if len(response.DocumentOutcome.Content) == 0 {
			return m.terminateRuntimeCandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, "provider_document_missing", "complete candidate response did not carry its raw document", nil, settlement)
		}
	default:
		return m.terminateRuntimeCandidateRequestProtocol(ctx, aggregate, session.LogicalRequestID, "provider_document_outcome_invalid", "candidate response carried an unknown provider document outcome", nil, settlement)
	}
	completed, _, _, completeErr := m.persistAndCompleteRuntimeCandidateRequestDocument(ctx, aggregate, session.LogicalRequestID, sequence, physicalCallID, response.DocumentOutcome.Content, settlement)
	if completeErr != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, completeErr
	}
	return m.admitRuntimeCompletedCandidateDocument(ctx, completed, manifest, session.LogicalRequestID, candidateAttempt)
}

func runtimeOperationsExpectedDocument(aggregate runtime.RuntimeAggregate, manifestHash string, correctionAttempt int) runtimecontract.RuntimeExpectedDocumentContract {
	execution := aggregate.Facts.Activation.ExecutionContract.Normalized()
	maxOutputTokens := 0
	if aggregate.Facts.Resources.ProviderCeiling != nil {
		maxOutputTokens = aggregate.Facts.Resources.ProviderCeiling.EffectiveMaxOutputTokens
	}
	maxRecoveryAttempts := execution.CandidateDocumentRecovery.MaxAttempts
	if maxRecoveryAttempts <= 0 {
		maxRecoveryAttempts = 1
	}
	return runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:  runtimecontract.RuntimeProviderDocumentOperations,
		ManifestHash:  manifestHash, CorrectionAttempt: correctionAttempt,
		MaxOutputTokens: maxOutputTokens, MaxRecoveryAttempts: maxRecoveryAttempts,
	}
}

// admitRuntimeCompletedCandidateDocument is the C5 boundary used by C4's
// production switch. The raw document must already be durable and the session
// must already be document_complete before attempt 0/1 is allocated.
func (m *Biz) admitRuntimeCompletedCandidateDocument(ctx context.Context, aggregate runtime.RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, logicalRequestID string, candidateAttempt int) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	session, found := runtimeCandidateRequestSession(aggregate, logicalRequestID)
	if !found {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("runtime candidate request session %q is missing", logicalRequestID)
	}
	if session.ManifestHash != manifest.Hash {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("%w: completed candidate document manifest does not match its request session", common.ErrConflict)
	}
	document, err := loadCompletedRuntimeCandidateRequestDocument(ctx, m.evidence.ArtifactContinuations, aggregate, logicalRequestID)
	if err != nil {
		failed, failureErr := m.runtimeRecordFailure(ctx, aggregate, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "provider_document_content_invalid",
			Signature: "provider-document-content-invalid", Message: "persisted complete provider document is unavailable or fails its integrity check",
		})
		return failed, runtimecontract.RuntimeCandidate{}, false, failureErr
	}
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, runtime.ActiveRejectedOperationShapes(aggregate))
	if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(manifest, aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(candidateContext)); err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	// Raw provider bytes stay in the C4 artifact for audit/restart integrity.
	// C5 may remove one exact outer ```json fence before strict decoding; this
	// cannot admit prose, alternate fence forms, multiple JSON values, or any
	// manifest/schema violation.
	decodedDocument, _ := runtimecontract.NormalizeRuntimeCandidatePayloadDocument(document)
	candidate, decodeErr := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, decodedDocument)
	if decodeErr != nil {
		failure := *runtimeCandidateSchemaFailure(decodeErr)
		failed, failureErr := m.runtimeCandidateDocument(ctx, aggregate, runtime.RuntimeInternalEventCandidateDocumentRejected, candidateAttempt, session.StablePrefixHash, manifest, session.LogicalRequestID, nil, &failure)
		return failed.Aggregate, runtimecontract.RuntimeCandidate{}, false, failureErr
	}
	if validateErr := candidate.ValidateWithActivation(aggregate.Decision, candidateContext.Activation); validateErr != nil {
		failure := runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid", Signature: "candidate-contract-invalid", Reason: validateErr.Error(),
		}
		failed, failureErr := m.runtimeCandidateDocument(ctx, aggregate, runtime.RuntimeInternalEventCandidateDocumentRejected, candidateAttempt, session.StablePrefixHash, manifest, session.LogicalRequestID, nil, &failure)
		return failed.Aggregate, runtimecontract.RuntimeCandidate{}, false, failureErr
	}
	pendingEvent, err := m.runtimeCandidateDocument(ctx, aggregate, runtime.RuntimeInternalEventCandidateAttemptBegun, candidateAttempt, session.StablePrefixHash, manifest, session.LogicalRequestID, nil, nil)
	if err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	pending := pendingEvent.Aggregate
	proposal, proposalErr := m.runtimeCandidateDocument(ctx, pending, runtime.RuntimeInternalEventCandidateProposalObserved, candidateAttempt, "", manifest, session.LogicalRequestID, &candidate, nil)
	if proposalErr != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, proposalErr
	}
	if proposal.Admission == nil || !proposal.Admission.Admission.Accepted() {
		return proposal.Aggregate, runtimecontract.RuntimeCandidate{}, false, nil
	}
	persisted, ok := runtimePendingCandidateProposal(proposal.Aggregate, candidateAttempt)
	if !ok {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("persisted runtime candidate proposal is missing")
	}
	return proposal.Aggregate, persisted, true, nil
}

func runtimeCandidateRequestSession(aggregate runtime.RuntimeAggregate, logicalRequestID string) (runtimecontract.RuntimeCandidateRequestSession, bool) {
	for _, session := range aggregate.CandidateRequestSessions {
		if session.LogicalRequestID == logicalRequestID {
			return session.Clone(), true
		}
	}
	return runtimecontract.RuntimeCandidateRequestSession{}, false
}

func (m *Biz) terminateRuntimeCandidateRequestProtocol(ctx context.Context, aggregate runtime.RuntimeAggregate, logicalRequestID, code, message string, originalDiagnostic *runtimecontract.RuntimeOriginalDiagnostic, budgetSettlement ...*runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	failed, failureErr := m.runtimeCoordinateProviderRequestTerminal(ctx, aggregate, logicalRequestID, runtime.RuntimeProviderRequestTerminalInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol,
		TerminalCode: code, TerminalSignature: strings.ReplaceAll(code, "_", "-"),
		Message: message, OriginalDiagnostic: originalDiagnostic,
	}, firstRuntimeProviderBudgetSettlement(budgetSettlement))
	return failed, runtimecontract.RuntimeCandidate{}, false, failureErr
}

func (m *Biz) terminateRuntimeCandidateRequestSubmissionUnknown(ctx context.Context, aggregate runtime.RuntimeAggregate, logicalRequestID string, settlement *runtime.RuntimeProviderBudgetSettlement) (runtime.RuntimeAggregate, runtimecontract.RuntimeCandidate, bool, error) {
	failed, failureErr := m.runtimeCoordinateProviderRequestTerminal(ctx, aggregate, logicalRequestID, runtime.RuntimeProviderRequestTerminalInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown,
		TerminalCode: "provider_submission_unknown", TerminalSignature: "provider-submission-unknown",
		Message: "provider request was authorized without a trusted physical submission outcome",
	}, settlement)
	return failed, runtimecontract.RuntimeCandidate{}, false, failureErr
}

func runtimePendingCandidateProposal(aggregate runtime.RuntimeAggregate, attempt int) (runtimecontract.RuntimeCandidate, bool) {
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	for _, record := range aggregate.CandidateAttempts {
		if runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, record) != contractID || record.Attempt != attempt || record.State != runtimecontract.RuntimeCandidateAttemptStatePending || record.Proposal == nil {
			continue
		}
		return *record.Proposal, true
	}
	return runtimecontract.RuntimeCandidate{}, false
}

func runtimeCandidateAttemptRecord(aggregate runtime.RuntimeAggregate, attempt int) (runtimecontract.RuntimeCandidateAttemptRecord, bool) {
	transaction := aggregate.Decision.AuthorityTransaction()
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt == attempt && record.Transaction == transaction {
			return record, true
		}
	}
	return runtimecontract.RuntimeCandidateAttemptRecord{}, false
}

func (m *Biz) probeRuntimeCandidateFeasibility(ctx context.Context, aggregate runtime.RuntimeAggregate, workspaceDirectory string, candidate runtimecontract.RuntimeCandidate, manifest runtimecontract.RuntimeCandidateContractManifest, candidateAttempt int) (*runtimecontract.RuntimeOwnerFeasibilityObservation, *runtimecontract.RuntimePrerequisiteStale, *runtimecontract.RuntimeOperationPrerequisiteRequest, error) {
	if m == nil || m.EventHub() == nil {
		return nil, nil, nil, common.ErrInvalidArgument
	}
	// Assessment candidates and governance candidates contain no executable
	// owner operation. Their authority is validated by their own seal contract,
	// so an unrelated persisted inventory must not make assessment impossible.
	if candidate.StateChange == nil && candidate.Validation == nil {
		return nil, nil, nil, nil
	}
	if len(aggregate.Facts.Resources.OperationAdmissibilities) == 0 {
		if runtimeRequiresOperationAdmissibility(aggregate.Facts.Prerequisites) {
			return nil, nil, nil, fmt.Errorf("runtime candidate has no persisted owner admissibility inventory")
		}
		// Focused reducer/receipt fixtures can construct facts directly without a
		// bootstrap prerequisite. Production activation always seals this
		// prerequisite, so such fixtures cannot bypass the production gate.
		return nil, nil, nil, nil
	}
	operations, err := orderedRuntimeCandidateOperations(candidate)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, operation := range operations {
		descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !ok || descriptor.OwnerModuleID == "" {
			return nil, nil, nil, fmt.Errorf("candidate operation %q has no canonical owner", operation.Kind)
		}
		inventory, found := runtimeOperationAdmissibilityForKind(aggregate.Facts.Resources.OperationAdmissibilities, descriptor.OwnerModuleID, operation.Kind)
		if !found {
			return nil, nil, nil, fmt.Errorf("candidate operation %q has no persisted owner admissibility inventory", operation.Kind)
		}
		if persisted, found := runtimeCandidateFeasibilityRecord(aggregate, candidateAttempt, operation.ID); found {
			if persisted.AdmissibilitySnapshotID != inventory.SnapshotID || persisted.AcquisitionBinding != inventory.AcquisitionBinding {
				return nil, nil, nil, fmt.Errorf("candidate operation %q feasibility snapshot does not match persisted owner inventory", operation.ID)
			}
			if persisted.Observation != nil && persisted.Observation.Outcome == runtimecontract.RuntimeOwnerFeasibilityFeasible {
				continue
			}
			return runtimeOwnerFeasibilityFromRecord(persisted)
		}
		observation, observed := runtimecontract.RuntimePrerequisiteObservationForID(aggregate.Facts.Resources.PrerequisiteObservations, inventory.PrerequisiteID)
		if !observed {
			return nil, nil, nil, fmt.Errorf("candidate operation %q has no freshness observation for owner admissibility", operation.Kind)
		}
		authorization, authorized := runtimeOperationAuthorizationForKind(manifest, operation.Kind)
		if !authorized {
			return nil, nil, nil, fmt.Errorf("candidate operation %q has no sealed operation authorization", operation.Kind)
		}
		response, probeErr := ProbeRuntimeOperationFeasibility(ctx, m.EventHub(), runtimecontract.RuntimeOperationFeasibilityRequest{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Transaction: aggregate.Decision.AuthorityTransaction(),
			WorkspaceDeliverableDirectory: workspaceDirectory, Operation: operation, Admissibility: inventory,
			CapabilityStateBinding: inventory.AcquisitionBinding, CandidateManifestHash: manifest.Hash,
			OperationAuthorization: authorization, Observation: &observation,
			CurrentTargetSnapshot: runtimeCurrentTargetSnapshot(aggregate.Facts, operation),
		})
		if probeErr != nil {
			return nil, nil, nil, probeErr
		}
		ownerObservation, observationErr := runtimecontract.RuntimeOwnerFeasibilityObservationFromOperation(response)
		if observationErr != nil {
			return nil, nil, nil, observationErr
		}
		record := runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID: operation.ID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
			CandidateManifestHash: response.CandidateManifestHash,
			Observation:           &ownerObservation, ReplayProof: response.ReplayProof,
		}
		recorded, recordErr := m.runtimeRecordCandidateFeasibility(ctx, aggregate, candidateAttempt, record)
		if recordErr != nil {
			return nil, nil, nil, recordErr
		}
		aggregate = recorded
		if response.Outcome == runtimecontract.RuntimeOwnerFeasibilityFeasible {
			continue
		}
		if response.Stale != nil {
			return nil, response.Stale, nil, nil
		}
		if response.PrerequisiteRequest != nil {
			return nil, nil, response.PrerequisiteRequest, nil
		}
		return &ownerObservation, nil, nil, nil
	}
	return nil, nil, nil, nil
}

func runtimeCurrentTargetSnapshot(facts runtimecontract.RuntimeFacts, operation runtimecontract.RuntimeOperation) *runtimecontract.TargetSnapshot {
	if !runtimecontract.RuntimeOperationRequiresCurrentTargetSnapshot(operation) {
		return nil
	}
	for _, targetRef := range operation.Normalized().TargetRefs {
		for _, snapshot := range facts.Resources.TargetSnapshots {
			if strings.TrimSpace(snapshot.TargetRef) == targetRef && snapshot.Complete() {
				copySnapshot := snapshot
				return &copySnapshot
			}
		}
	}
	return nil
}

func runtimeCandidateFeasibilityRecord(aggregate runtime.RuntimeAggregate, attempt int, operationID string) (runtimecontract.RuntimeCandidateFeasibilityRecord, bool) {
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	for _, candidateAttempt := range aggregate.CandidateAttempts {
		if runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, candidateAttempt) != contractID || candidateAttempt.Attempt != attempt {
			continue
		}
		for _, record := range candidateAttempt.Feasibilities {
			if record.OperationID == operationID {
				return record, true
			}
		}
	}
	return runtimecontract.RuntimeCandidateFeasibilityRecord{}, false
}

func runtimeOwnerFeasibilityFromRecord(record runtimecontract.RuntimeCandidateFeasibilityRecord) (*runtimecontract.RuntimeOwnerFeasibilityObservation, *runtimecontract.RuntimePrerequisiteStale, *runtimecontract.RuntimeOperationPrerequisiteRequest, error) {
	if record.Observation == nil {
		return nil, nil, nil, fmt.Errorf("persisted owner feasibility record lacks observation")
	}
	observation := record.Observation.Clone()
	if err := observation.Validate(); err != nil {
		return nil, nil, nil, err
	}
	switch observation.Outcome {
	case runtimecontract.RuntimeOwnerFeasibilityFeasible:
		return nil, nil, nil, nil
	case runtimecontract.RuntimeOwnerFeasibilityStale:
		return nil, observation.Stale, nil, nil
	case runtimecontract.RuntimeOwnerFeasibilityPrerequisiteRequired:
		return nil, nil, observation.PrerequisiteRequest, nil
	case runtimecontract.RuntimeOwnerFeasibilityInfeasible, runtimecontract.RuntimeOwnerFeasibilityContractDefect:
		return &observation, nil, nil, nil
	default:
		return nil, nil, nil, fmt.Errorf("persisted owner feasibility outcome %q is not terminal", observation.Outcome)
	}
}

func runtimeRequiresOperationAdmissibility(state runtimecontract.PrerequisiteState) bool {
	for _, prerequisite := range append(append([]runtimecontract.RuntimePrerequisite(nil), state.Pending...), state.Satisfied...) {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility {
			return true
		}
	}
	return false
}

func runtimeOperationAuthorizationForKind(manifest runtimecontract.RuntimeCandidateContractManifest, kind string) (runtimecontract.RuntimeCandidateOperationManifest, bool) {
	manifest = manifest.Normalized()
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, operation := range manifest.Operations {
		if operation.Kind == kind {
			return operation, true
		}
	}
	return runtimecontract.RuntimeCandidateOperationManifest{}, false
}

func runtimeOperationAdmissibilityForKind(values []runtimecontract.RuntimeOperationAdmissibility, owner, kind string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, value := range values {
		value = value.Normalized()
		if value.OwnerModuleID != owner {
			continue
		}
		for _, available := range value.OperationKinds {
			if available == canonical {
				return value, true
			}
		}
	}
	return runtimecontract.RuntimeOperationAdmissibility{}, false
}
func orderedRuntimeCandidateOperations(candidate runtimecontract.RuntimeCandidate) ([]runtimecontract.RuntimeOperation, error) {
	operations := []runtimecontract.RuntimeOperation{}
	if candidate.StateChange != nil {
		for _, value := range candidate.StateChange.Operations {
			operations = append(operations, value.Operation.Normalized())
		}
	} else if candidate.Validation != nil {
		for _, value := range candidate.Validation.Operations {
			operations = append(operations, value.Operation.Normalized())
		}
	}
	if len(operations) == 0 {
		return nil, fmt.Errorf("runtime candidate has no executable operations")
	}
	byID := make(map[string]runtimecontract.RuntimeOperation, len(operations))
	for _, operation := range operations {
		if operation.ID == "" {
			return nil, fmt.Errorf("runtime candidate operation requires id")
		}
		byID[operation.ID] = operation
	}
	completed := map[string]struct{}{}
	ordered := make([]runtimecontract.RuntimeOperation, 0, len(operations))
	for len(ordered) < len(byID) {
		ready := []string{}
		for id, operation := range byID {
			if _, done := completed[id]; done {
				continue
			}
			allDependenciesComplete := true
			for _, dependency := range operation.DependsOn {
				if _, exists := byID[dependency]; !exists {
					return nil, fmt.Errorf("runtime candidate operation %q depends on unknown operation %q", id, dependency)
				}
				if _, done := completed[dependency]; !done {
					allDependenciesComplete = false
					break
				}
			}
			if allDependenciesComplete {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("runtime candidate operation dependency graph contains a cycle")
		}
		sort.Strings(ready)
		for _, id := range ready {
			ordered = append(ordered, byID[id])
			completed[id] = struct{}{}
		}
	}
	return ordered, nil
}

func runtimePreviousCandidateFailure(aggregate runtime.RuntimeAggregate, candidateAttempt int) (*runtimecontract.RuntimeCandidateFailure, error) {
	if candidateAttempt == 0 {
		return nil, nil
	}
	if candidateAttempt != 1 {
		return nil, fmt.Errorf("runtime candidate attempt must be 0 or 1")
	}
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	for idx := range aggregate.CandidateAttempts {
		record := aggregate.CandidateAttempts[idx]
		if runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, record) != contractID || record.Attempt != 0 {
			continue
		}
		if record.State != runtimecontract.RuntimeCandidateAttemptStateFailed || record.Failure == nil {
			return nil, fmt.Errorf("runtime candidate retry requires a failed first attempt")
		}
		failure := *record.Failure
		return &failure, nil
	}
	return nil, fmt.Errorf("runtime candidate retry has no first attempt failure")
}

// runtimeCandidateSchemaFailure preserves the decoder's provider-safe,
// manifest-derived diagnostic for the persisted SM-3 attempt. The retry
// policy remains in requestAndSealRuntimeCandidateWithRetry; this helper
// only translates typed contract diagnostics into the candidate failure DTO.
func runtimeCandidateSchemaFailure(decodeErr error) *runtimecontract.RuntimeCandidateFailure {
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid",
		Signature: "candidate-schema-invalid", Reason: decodeErr.Error(),
	}
	var payloadDiagnostic *runtimecontract.RuntimeCandidatePayloadDiagnostic
	if errors.As(decodeErr, &payloadDiagnostic) {
		failure.Diagnostic = runtimecontract.RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(payloadDiagnostic)
		return failure
	}
	var contractDiagnostic *runtimecontract.RuntimeCandidateContractDiagnostic
	if errors.As(decodeErr, &contractDiagnostic) {
		failure.Diagnostic = runtimecontract.RuntimeCandidateFailureDiagnosticFromContractDiagnostic(contractDiagnostic)
	}
	return failure
}

func (m *Biz) requestAndSealRuntimeCandidateWithRetry(
	ctx context.Context,
	aggregate runtime.RuntimeAggregate,
	stablePrefixHash string,
) (runtime.RuntimeAggregate, error) {
	for range 2 {
		if aggregate.ActiveDispatch != nil {
			if aggregate.ActiveDispatch.Candidate.Governance != nil {
				return m.prepareRuntimeGovernanceRequest(ctx, aggregate)
			}
			return aggregate, nil
		}
		if aggregate.Decision.State == runtimecontract.ConvergenceStateFailed || aggregate.Decision.State == runtimecontract.ConvergenceStateComplete {
			return aggregate, nil
		}
		// A candidate proposal or its read-only owner feasibility probe may
		// invalidate a mutable prerequisite. That transition belongs to SM-2;
		// return it to the outer drive so prerequisite acquisition runs before
		// any new SM-3 candidate attempt is requested.
		if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite {
			return aggregate, nil
		}
		if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedGovernance {
			next, err := m.sealRuntimeGovernanceCandidate(ctx, aggregate, stablePrefixHash)
			if err != nil {
				return runtime.RuntimeAggregate{}, err
			}
			aggregate = next
			continue
		}
		attempt, ok := nextRuntimeCandidateAttempt(aggregate)
		if !ok {
			if runtimeCandidateContractExhausted(aggregate) {
				return m.runtimeRecordFailure(ctx, aggregate, runtimecontract.FailureFact{
					Class: runtimecontract.FailureClassCandidateExhausted, Code: "candidate_retry_exhausted",
					Signature: "candidate-retry-exhausted", Message: "candidate authority contract exhausted its sealed attempt 0/1 budget",
				})
			}
			return runtime.RuntimeAggregate{}, fmt.Errorf("%w: no candidate attempt is available for authority snapshot %s contract %s", common.ErrConflict, aggregate.Decision.AuthoritySnapshot.ID, runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction()))
		}
		if attempt == 1 {
			failure, failureErr := runtimePreviousCandidateFailure(aggregate, attempt)
			if failureErr != nil {
				return runtime.RuntimeAggregate{}, failureErr
			}
			// N4: complete rejections are sealed as correctable_once or
			// rejected_final. Missing objectives are runtime_contract_defect,
			// never candidate_correction_undefined.
			if !runtime.CandidateFailureIsCorrectableOnce(failure) {
				sealed, directive, sealErr := runtime.SealCandidateRejection(failure, runtimecontract.RuntimeCandidateContractManifest{})
				if sealErr != nil {
					return runtime.RuntimeAggregate{}, sealErr
				}
				code := "runtime_contract_defect"
				message := "complete candidate rejection is not correctable under the sealed manifest"
				if directive != nil && directive.TerminalCode != "" {
					code = directive.TerminalCode
				}
				if sealed != nil && strings.TrimSpace(sealed.Reason) != "" {
					message = sealed.Reason
				}
				class := runtimecontract.FailureClassRuntimeContractMisalignment
				if code == "candidate_retry_exhausted" {
					class = runtimecontract.FailureClassCandidateExhausted
				}
				return m.runtimeRecordFailure(ctx, aggregate, runtimecontract.FailureFact{
					Class: class, Code: code, Signature: code, Message: message,
				})
			}
		}
		next, err := m.requestAndSealRuntimeCandidate(ctx, aggregate, stablePrefixHash, attempt)
		if err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		aggregate = next
		// Owner infeasibility is a completed SM-2 fact handoff, not an SM-3
		// correction. End this candidate turn so the outer drive reloads the
		// newly reduced authority instead of manufacturing an attempt-1/second
		// request from the old loop frame.
		if runtimeHasOwnerFeasibilityInvalidation(aggregate) {
			return aggregate, nil
		}
	}
	return aggregate, nil
}

func runtimeHasOwnerFeasibilityInvalidation(aggregate runtime.RuntimeAggregate) bool {
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.State == runtimecontract.RuntimeCandidateAttemptStateInvalidated && attempt.OwnerFeasibility != nil {
			return true
		}
	}
	return false
}

func (m *Biz) sealRuntimeGovernanceCandidate(ctx context.Context, aggregate runtime.RuntimeAggregate, stablePrefixHash string) (runtime.RuntimeAggregate, error) {
	_ = stablePrefixHash // Governance has no provider request or candidate attempt.
	dependency, found := runtime.GovernanceDependencyForAggregate(aggregate)
	if !found {
		// A reducer request_governance state must be backed by a persisted
		// external dependency. Never synthesize a generic decision ticket from
		// a focus, error string, or feasibility summary; make the missing
		// contract visible to SM-2 as a deterministic terminal instead.
		return m.runtimeRecordFailure(ctx, aggregate, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "governance_dependency_missing",
			Signature: "governance-dependency-missing", Message: "runtime governance requires a sealed external dependency",
		})
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)))
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	candidate := runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: dependency},
	}
	sealed, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventGovernanceDispatchSealed, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{Manifest: &manifest, Candidate: &candidate},
	})
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	return sealed.Aggregate, nil
}

func nextRuntimeCandidateAttempt(aggregate runtime.RuntimeAggregate) (int, bool) {
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	var first *runtimecontract.RuntimeCandidateAttemptRecord
	var second *runtimecontract.RuntimeCandidateAttemptRecord
	matched := 0
	for idx := range aggregate.CandidateAttempts {
		record := &aggregate.CandidateAttempts[idx]
		if runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, *record) != contractID {
			continue
		}
		matched++
		if record.Attempt == 0 {
			if first != nil {
				return 0, false
			}
			first = record
		} else if record.Attempt == 1 {
			if second != nil {
				return 0, false
			}
			second = record
		}
	}
	if matched > 2 {
		return 0, false
	}
	if first == nil {
		return 0, true
	}
	switch first.State {
	case runtimecontract.RuntimeCandidateAttemptStatePending:
		return 0, true
	case runtimecontract.RuntimeCandidateAttemptStateAccepted:
		return 0, false
	case runtimecontract.RuntimeCandidateAttemptStateFailed:
		if second == nil || second.State == runtimecontract.RuntimeCandidateAttemptStatePending {
			return 1, true
		}
		return 0, false
	case runtimecontract.RuntimeCandidateAttemptStateInvalidated:
		return 0, true
	default:
		return 0, false
	}
}

func runtimeCandidateContractExhausted(aggregate runtime.RuntimeAggregate) bool {
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	count := 0
	for _, record := range aggregate.CandidateAttempts {
		if runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, record) != contractID {
			continue
		}
		count++
		if record.State == runtimecontract.RuntimeCandidateAttemptStateAccepted {
			return false
		}
		if record.Attempt == 1 && record.State == runtimecontract.RuntimeCandidateAttemptStateFailed {
			return true
		}
	}
	return count >= 2
}

// runtimeEnsureCandidateFeasible blocks provider requests when the sealed required
// output has no concrete executable operation kind under current capabilities.
// Assessment/governance candidates are LLM-produced and skip this gate.
func runtimeEnsureCandidateFeasible(aggregate runtime.RuntimeAggregate) error {
	fact, err := runtimeFeasibilityFact(aggregate)
	if err != nil || fact == nil {
		return err
	}
	return fmt.Errorf("%s", fact.Reason)
}

// runtimeFeasibilityFact produces a persisted SM-2 input for a blocked
// candidate request. Callers must RecordFeasibilityFact and re-drive rather
// than returning this error to the process boundary.
func runtimeFeasibilityFact(aggregate runtime.RuntimeAggregate) (*runtimecontract.RuntimeFeasibilityFact, error) {
	switch aggregate.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation:
	default:
		return nil, nil
	}
	required := aggregate.Decision.RequiredOutput.Normalized()
	if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedValidation {
		if missing := runtimeMissingValidationDiscoverySources(aggregate.Facts); len(missing) > 0 {
			return &runtimecontract.RuntimeFeasibilityFact{
				FocusID: aggregate.Facts.Focus.ID, State: aggregate.Decision.State,
				Reason: fmt.Sprintf("runtime validation discovery prerequisites are not fresh: %s", strings.Join(missing, ", ")),
			}, nil
		}
	}
	if len(required.AcceptedKinds) == 0 {
		return &runtimecontract.RuntimeFeasibilityFact{
			FocusID: aggregate.Facts.Focus.ID, State: aggregate.Decision.State,
			Reason: fmt.Sprintf("runtime convergence state %s has no accepted operation kinds", aggregate.Decision.State),
		}, nil
	}
	activation := runtimecontract.RuntimeActivationForFacts(aggregate.Facts)
	capabilityFeasible := runtimecontract.FeasibleOperationKindsWithActivation(aggregate.Decision.State, required, aggregate.Facts.Capabilities, activation.OperationPolicies)
	ownerFeasible := runtimecontract.FeasibleOperationKindsWithOwnerValidationRecipes(aggregate.Decision.State, required, aggregate.Facts.Capabilities, activation.OperationPolicies, runtimecontract.RuntimeValidationRecipesFromOperationAdmissibilities(aggregate.Facts.Resources.OperationAdmissibilities))
	if len(ownerFeasible) == 0 {
		category := runtimecontract.RuntimeOwnerFeasibilityCategory("")
		reason := fmt.Sprintf("runtime convergence state %s has no feasible operation kinds under sealed capabilities", aggregate.Decision.State)
		if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedValidation &&
			runtimeRequiredKindsContain(capabilityFeasible, runtimecontract.RuntimeBlockKindManagedValidationSession) {
			category = runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable
			reason = "runtime validation has no Shell-owner-confirmed executable recipe"
		}
		return &runtimecontract.RuntimeFeasibilityFact{
			FocusID: aggregate.Facts.Focus.ID, State: aggregate.Decision.State,
			RequiredOperationKinds: append([]string(nil), required.AcceptedKinds...),
			MissingCapabilityKinds: append([]string(nil), required.AcceptedKinds...),
			Category:               category,
			Reason:                 reason,
		}, nil
	}
	return nil, nil
}

// runtimeMissingValidationDiscoverySources evaluates the profile-selected,
// owner-neutral discovery vocabulary against committed prerequisite facts. The
// bootstrap already requests each of these facts through typed owner commands;
// this gate makes a missing/stale observation a persisted SM-2 feasibility
// input instead of allowing an ungrounded provider request.
func runtimeMissingValidationDiscoverySources(facts runtimecontract.RuntimeFacts) []string {
	required := runtimecontract.RequiredRuntimeValidationDiscoverySources(
		facts.Activation.ExecutionContract.ValidationPolicies,
		runtimecontract.EvidenceKindValidation,
	)
	if len(required) == 0 {
		return nil
	}
	facts = facts.Normalized()
	missing := make([]string, 0, len(required))
	for _, source := range required {
		if runtimeValidationDiscoverySourceSatisfied(facts, source) {
			continue
		}
		missing = append(missing, source)
	}
	return missing
}

func runtimeValidationDiscoverySourceSatisfied(facts runtimecontract.RuntimeFacts, source string) bool {
	hasObservation := func(prerequisiteID string, kind runtimecontract.PrerequisiteKind) bool {
		observation, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisiteID)
		return found && observation.Kind == kind && observation.ContentHash != ""
	}
	switch strings.TrimSpace(source) {
	case runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest:
		if facts.Resources.Manifest == nil || strings.TrimSpace(facts.Resources.Manifest.Revision) == "" {
			return false
		}
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindManifest && hasObservation(prerequisite.ID, prerequisite.Kind) {
				return true
			}
		}
		return false
	case runtimecontract.RuntimeValidationDiscoverySourceProjectFiles:
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if prerequisite.Kind != runtimecontract.PrerequisiteKindManifest || !hasObservation(prerequisite.ID, prerequisite.Kind) {
				continue
			}
			observation, _ := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
			for _, dependency := range observation.Dependencies {
				if dependency.Domain == runtimecontract.RuntimeResourceWorkspaceTree {
					return true
				}
			}
		}
		return false
	case runtimecontract.RuntimeValidationDiscoverySourceDeclaredCommands:
		for _, inventory := range facts.Resources.OperationAdmissibilities {
			if len(inventory.OperationKinds) > 0 && hasObservation(inventory.PrerequisiteID, runtimecontract.PrerequisiteKindOperationAdmissibility) {
				return true
			}
		}
		return false
	case runtimecontract.RuntimeValidationDiscoverySourceAvailableCapabilities:
		if len(facts.Capabilities.StateChangeKinds)+len(facts.Capabilities.ValidationKinds)+len(facts.Capabilities.GovernanceKinds) == 0 {
			return false
		}
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindCapability && hasObservation(prerequisite.ID, prerequisite.Kind) {
				return true
			}
		}
		return false
	default:
		// An unknown profile source cannot be inferred by core. It remains
		// unsatisfied until a typed owner observation contract is introduced.
		return false
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
