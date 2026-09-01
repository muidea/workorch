package orchestrator

// Runtime mainline is owned by Biz.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/requestcontent"
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
	case runtimecontract.PrerequisiteKindValidationScratch:
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
	manifest, manifestErr := runtime.BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if manifestErr != nil {
		return m.runtimeRecordFailure(ctx, aggregate, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_candidate_manifest_invalid",
			Signature: "runtime-candidate-manifest-invalid", Message: manifestErr.Error(),
		})
	}
	if candidateAttempt == 1 {
		if err := runtime.ValidatePersistedCandidateCorrection(aggregate, manifest); err != nil {
			return m.runtimeRecordFailure(ctx, aggregate, runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassCandidateExhausted, Code: "candidate_authority_exhausted",
				Signature: "candidate-authority-exhausted", Message: err.Error(),
			})
		}
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
	// any provider request. Persisted proposals and the exact request session do
	// not re-enter this boundary.
	if candidateContext.Resources.ProviderCeiling != nil {
		_, proposalPersisted := runtimePendingCandidateProposal(aggregate, candidateAttempt)
		previousFailure, previousErr := runtimePreviousCandidateFailure(aggregate, candidateAttempt)
		if previousErr != nil {
			return runtime.RuntimeAggregate{}, previousErr
		}
		correctionDirective, correctionErr := runtimePreviousCandidateCorrection(aggregate, candidateAttempt)
		if correctionErr != nil {
			return runtime.RuntimeAggregate{}, correctionErr
		}
		correctionObservation, observationErr := requestcontent.ProjectRuntimeCandidateCorrectionObservation(previousFailure, correctionDirective)
		if observationErr != nil {
			return runtime.RuntimeAggregate{}, observationErr
		}
		correctionInput := runtimeCandidateCorrectionInputFromDirective(correctionDirective)
		correctionHash := correctionInput.StableRequestCorrectionHash(correctionObservation)
		requestSessionPersisted := runtimeCurrentOperationsCandidateRequestSessionPersisted(
			aggregate,
			stablePrefixHash,
			manifest,
			correctionHash,
			candidateAttempt,
		)
		if !proposalPersisted && !requestSessionPersisted {
			preflight, preflightErr := PreflightRuntimeCandidate(ctx, m.EventHub(), llmevents.PreflightRuntimeCandidateCommand{
				WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
				DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
				Decision:     aggregate.Decision, Transaction: aggregate.Decision.AuthorityTransaction(), CandidateAttempt: candidateAttempt,
				CandidateCorrectionObservation: correctionObservation, CandidateCorrection: correctionInput,
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
	if err := runtime.ValidateRuntimeCandidateContractManifestForAggregate(aggregate, manifest); err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	correctionDirective, err := runtimePreviousCandidateCorrection(aggregate, candidateAttempt)
	if err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	correctionInput := runtimeCandidateCorrectionInputFromDirective(correctionDirective)
	correctionObservation, err := requestcontent.ProjectRuntimeCandidateCorrectionObservation(previousFailure, correctionDirective)
	if err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	correctionHash := correctionInput.StableRequestCorrectionHash(correctionObservation)
	expected, err := runtimeOperationsExpectedDocument(aggregate, manifest, candidateAttempt)
	if err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
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
		CandidateCorrectionObservation: correctionObservation, CandidateCorrection: correctionInput,
		StablePrefixHash: stablePrefixHash, CandidateManifest: manifest, DefinitionContext: aggregate.Definition,
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
	if session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentConformanceRejected {
		if session.LastDocumentFraming == nil {
			return runtimeCandidateDocumentProgress{handled: true}, fmt.Errorf("%w: document-conformance session has no framing fact", common.ErrConflict)
		}
		failure := runtimeCandidateDocumentConformanceFailure(string(session.LastDocumentFraming.Disposition))
		failed, err := m.runtimeCandidateDocument(ctx, aggregate, runtime.RuntimeInternalEventCandidateDocumentRejected, candidateAttempt, session.StablePrefixHash, manifest, session.LogicalRequestID, nil, &failure)
		return runtimeCandidateDocumentProgress{aggregate: failed.Aggregate, handled: true}, err
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
		rejected, rejectErr := m.runtimeRejectCandidateRequestDocumentConformance(ctx, aggregate, session.LogicalRequestID, physicalCallID, framing, settlement)
		if rejectErr != nil {
			return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, rejectErr
		}
		persisted, found := runtimeCandidateRequestSession(rejected, session.LogicalRequestID)
		if !found {
			return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, fmt.Errorf("runtime candidate conformance session disappeared")
		}
		progress, progressErr := m.resumeRuntimeCandidateDocumentSession(ctx, rejected, manifest, persisted, candidateAttempt)
		return progress.aggregate, progress.candidate, progress.ready, progressErr
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

func runtimeOperationsExpectedDocument(aggregate runtime.RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, correctionAttempt int) (runtimecontract.RuntimeExpectedDocumentContract, error) {
	execution := aggregate.Facts.Activation.ExecutionContract.Normalized()
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		return runtimecontract.RuntimeExpectedDocumentContract{}, fmt.Errorf("compile runtime candidate wire contract: %w", err)
	}
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
		ManifestHash:  manifest.Hash, WireContractHash: wire.Hash, CorrectionAttempt: correctionAttempt,
		MaxOutputTokens: maxOutputTokens, MaxRecoveryAttempts: maxRecoveryAttempts,
	}, nil
}

// admitRuntimeCompletedCandidateDocument is the strict local admission boundary
// for a completed provider document. The document must already be available
// through the Orchestrator-private restart carrier and the session must already
// be document_complete before attempt 0/1 is allocated.
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
	if err := runtime.ValidateRuntimeCandidateContractManifestForAggregate(aggregate, manifest); err != nil {
		return runtime.RuntimeAggregate{}, runtimecontract.RuntimeCandidate{}, false, err
	}
	// Complete provider bytes remain only in an Orchestrator-private restart
	// carrier. They do not enter audit, EventHub JSON, eventstream/projection, or
	// any C4 prompt projection.
	// Normalization may remove one exact outer ```json fence before strict decoding; this
	// cannot admit prose, alternate fence forms, multiple JSON values, or any
	// manifest/schema violation.
	decodedDocument, _ := runtimecontract.NormalizeRuntimeCandidatePayloadDocument(document)
	candidate, decodeErr := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, decodedDocument)
	if decodeErr != nil {
		failure := *runtimeCandidateSchemaFailure(decodeErr)
		var rejectedCandidate *runtimecontract.RuntimeCandidate
		if runtimeCandidateHasDecodedDocument(candidate) {
			candidateCopy := candidate
			rejectedCandidate = &candidateCopy
		}
		if shape, nonconformant := runtimeCompletedCandidateDocumentShape(decodedDocument); nonconformant {
			failure = runtimeCandidateDocumentConformanceFailure(shape)
			rejectedCandidate = nil
		}
		failed, failureErr := m.runtimeCandidateDocument(ctx, aggregate, runtime.RuntimeInternalEventCandidateDocumentRejected, candidateAttempt, session.StablePrefixHash, manifest, session.LogicalRequestID, rejectedCandidate, &failure)
		return failed.Aggregate, runtimecontract.RuntimeCandidate{}, false, failureErr
	}
	if validateErr := candidate.ValidateWithActivation(aggregate.Decision, candidateContext.Activation); validateErr != nil {
		failure := runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid", Signature: "candidate-contract-invalid", Reason: validateErr.Error(),
		}
		failed, failureErr := m.runtimeCandidateDocument(ctx, aggregate, runtime.RuntimeInternalEventCandidateDocumentRejected, candidateAttempt, session.StablePrefixHash, manifest, session.LogicalRequestID, &candidate, &failure)
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

func runtimeCandidateHasDecodedDocument(candidate runtimecontract.RuntimeCandidate) bool {
	count := 0
	for _, present := range []bool{candidate.StateChange != nil, candidate.Validation != nil, candidate.Assessment != nil, candidate.Governance != nil} {
		if present {
			count++
		}
	}
	return count == 1
}

func runtimeCandidateRequestSession(aggregate runtime.RuntimeAggregate, logicalRequestID string) (runtimecontract.RuntimeCandidateRequestSession, bool) {
	for _, session := range aggregate.CandidateRequestSessions {
		if session.LogicalRequestID == logicalRequestID {
			return session.Clone(), true
		}
	}
	return runtimecontract.RuntimeCandidateRequestSession{}, false
}

// runtimeCurrentOperationsCandidateRequestSessionPersisted recognizes only the
// exact SM-3 request identity that requestRuntimeCandidateDocument would ensure
// for the current authority transaction. The production order is preflight ->
// request-session CAS -> provider submission, so this durable session is the
// restart proof that preflight has already passed. A stale prefix, manifest,
// correction lineage, transaction, or expected-document attempt cannot suppress
// a new preflight.
func runtimeCurrentOperationsCandidateRequestSessionPersisted(
	aggregate runtime.RuntimeAggregate,
	stablePrefixHash string,
	manifest runtimecontract.RuntimeCandidateContractManifest,
	correctionHash string,
	candidateAttempt int,
) bool {
	transaction := aggregate.Decision.AuthorityTransaction()
	manifest = manifest.Normalized()
	manifestHash := manifest.Hash
	logicalRequestID := runtimecontract.StableRuntimeCandidateRequestSessionID(
		runtimecontract.RuntimeProviderDocumentOperations,
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		transaction,
		stablePrefixHash,
		manifestHash,
		correctionHash,
	)
	session, found := runtimeCandidateRequestSession(aggregate, logicalRequestID)
	if !found || session.ExpectedDocument == nil {
		return false
	}
	persistedExpected := session.ExpectedDocument.Normalized()
	currentExpectedValue, err := runtimeOperationsExpectedDocument(aggregate, manifest, candidateAttempt)
	if err != nil {
		return false
	}
	currentExpected := currentExpectedValue.Normalized()
	currentExpected.DocumentKind = runtimecontract.RuntimeProviderDocumentOperations
	currentExpected.LogicalRequestID = logicalRequestID
	currentExpected.StablePrefixHash = strings.TrimSpace(stablePrefixHash)
	currentExpected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(currentExpected)
	return session.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations &&
		session.Transaction == transaction &&
		session.StablePrefixHash == strings.TrimSpace(stablePrefixHash) &&
		session.ManifestHash == strings.TrimSpace(manifestHash) &&
		session.CorrectionHash == strings.TrimSpace(correctionHash) &&
		persistedExpected.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations &&
		persistedExpected.LogicalRequestID == logicalRequestID &&
		persistedExpected.StablePrefixHash == strings.TrimSpace(stablePrefixHash) &&
		persistedExpected.ManifestHash == strings.TrimSpace(manifestHash) &&
		persistedExpected.CorrectionAttempt == candidateAttempt &&
		persistedExpected.Hash == currentExpected.Hash
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

func runtimePreviousCandidateAttemptRecord(aggregate runtime.RuntimeAggregate, candidateAttempt int) (runtimecontract.RuntimeCandidateAttemptRecord, error) {
	if candidateAttempt != 1 {
		return runtimecontract.RuntimeCandidateAttemptRecord{}, fmt.Errorf("runtime candidate correction record requires attempt 1")
	}
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt == 0 && runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, record) == contractID {
			return record, nil
		}
	}
	return runtimecontract.RuntimeCandidateAttemptRecord{}, fmt.Errorf("runtime candidate retry has no first attempt record")
}

func runtimePreviousCandidateCorrection(aggregate runtime.RuntimeAggregate, candidateAttempt int) (*runtimecontract.RuntimeCandidateCorrectionDirective, error) {
	if candidateAttempt == 0 {
		return nil, nil
	}
	record, err := runtimePreviousCandidateAttemptRecord(aggregate, candidateAttempt)
	if err != nil {
		return nil, err
	}
	if !runtime.CandidateAttemptIsCorrectableOnce(record) {
		return nil, fmt.Errorf("runtime candidate retry has no valid persisted correction directive")
	}
	return record.Correction.Clone(), nil
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

func runtimeCandidateDocumentConformanceFailure(shape string) runtimecontract.RuntimeCandidateFailure {
	shape = strings.TrimSpace(shape)
	return runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema,
		Code:  "candidate_document_nonconformant", Signature: "candidate-document-nonconformant/" + strings.ReplaceAll(shape, "_", "-"),
		Reason: "provider response must contain exactly one manifest-conformant JSON document",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$", Actual: shape, Required: []string{runtimecontract.RuntimeCandidateDocumentShapeConformant},
			Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionConformExpectedDocument},
		},
	}
}

// runtimeCompletedCandidateDocumentShape classifies only defects after the
// first complete JSON value. Empty/non-JSON framing remains an LLM-owner fact;
// this pure SM-3 helper never reclassifies those raw outcomes.
func runtimeCompletedCandidateDocumentShape(document []byte) (string, bool) {
	decoder := json.NewDecoder(bytes.NewReader(document))
	var first json.RawMessage
	if err := decoder.Decode(&first); err != nil {
		return "", false
	}
	firstEnd := decoder.InputOffset()
	root := bytes.TrimSpace(first)
	if len(root) == 0 || root[0] != '{' {
		return runtimecontract.RuntimeCandidateDocumentShapeNonObjectJSONValue, true
	}
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	switch {
	case err == nil:
		return runtimecontract.RuntimeCandidateDocumentShapeMultipleJSONValues, true
	case errors.Is(err, io.EOF):
		return "", false
	default:
		if runtimeCandidateTrailingOperationFields(document, firstEnd) {
			return runtimecontract.RuntimeCandidateDocumentShapeTrailingOperationFields, true
		}
		return runtimecontract.RuntimeCandidateDocumentShapeTrailingContent, true
	}
}

func runtimeCandidateTrailingOperationFields(document []byte, firstEnd int64) bool {
	if firstEnd < 0 || firstEnd >= int64(len(document)) {
		return false
	}
	tail := bytes.TrimSpace(document[firstEnd:])
	if len(tail) == 0 || tail[0] != ',' {
		return false
	}
	for _, field := range [][]byte{
		[]byte(`"requiredOperationRef"`), []byte(`"kind"`), []byte(`"bindingRefs"`),
		[]byte(`"targetRefs"`), []byte(`"parameters"`),
	} {
		if bytes.Contains(tail, field) {
			return true
		}
	}
	return false
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
			first, firstErr := runtimePreviousCandidateAttemptRecord(aggregate, attempt)
			if firstErr != nil {
				return runtime.RuntimeAggregate{}, firstErr
			}
			if !runtime.CandidateAttemptIsCorrectableOnce(first) {
				code := "runtime_contract_defect"
				message := "complete candidate rejection has no manifest-bound correction witness"
				if first.Correction != nil && first.Correction.TerminalCode != "" {
					code = first.Correction.TerminalCode
				}
				if first.Failure != nil && strings.TrimSpace(first.Failure.Reason) != "" {
					message = first.Failure.Reason
				}
				class := runtimecontract.FailureClassRuntimeContractMisalignment
				if code == "candidate_retry_exhausted" || code == "candidate_authority_exhausted" {
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
	manifest, err := runtime.BuildRuntimeCandidateContractManifestForAggregate(aggregate)
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
