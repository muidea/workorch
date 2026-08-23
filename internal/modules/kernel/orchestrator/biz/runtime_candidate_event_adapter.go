package orchestrator

// Candidate adapter is internal to the Runtime owner Biz.

import (
	"context"
	"fmt"
	"strings"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// PreflightRuntimeCandidate asks the LLM owner to render the sealed request
// and validate C0-C5/P0-P4 before a candidate attempt is persisted. It is a
// typed read-only EventHub exchange: no provider call and no candidate can be
// produced by this path.
func PreflightRuntimeCandidate(ctx context.Context, hub magicevent.Hub, command llmevents.PreflightRuntimeCandidateCommand) (llmevents.PreflightRuntimeCandidateResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" {
		return llmevents.PreflightRuntimeCandidateResult{}, common.ErrInvalidArgument
	}
	if err := validateRuntimeCandidatePreflightCommand(command); err != nil {
		return llmevents.PreflightRuntimeCandidateResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		llmevents.TopicPreflightRuntimeCandidate,
		ModuleID,
		runtimecontract.RuntimeModuleLLM,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return llmevents.PreflightRuntimeCandidateResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return llmevents.PreflightRuntimeCandidateResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return llmevents.PreflightRuntimeCandidateResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(llmevents.PreflightRuntimeCandidateResult)
	if !ok {
		return llmevents.PreflightRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an invalid runtime candidate preflight result")
	}
	if response.DocumentKind != command.DocumentKind {
		return llmevents.PreflightRuntimeCandidateResult{}, fmt.Errorf("runtime candidate preflight document kind does not match request")
	}
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		if err := runtimecontract.ValidateAuthorityTransaction(command.Decision, response.Transaction); err != nil {
			return llmevents.PreflightRuntimeCandidateResult{}, err
		}
	}
	if err := validateRuntimeProviderPreflightFeedback(command, response.Feedback); err != nil {
		return llmevents.PreflightRuntimeCandidateResult{}, err
	}
	if response.Audit != nil && response.Audit.ProviderSubmitted {
		return llmevents.PreflightRuntimeCandidateResult{}, fmt.Errorf("runtime candidate preflight must not submit a provider request")
	}
	return response, nil
}

func validateRuntimeProviderPreflightFeedback(command llmevents.PreflightRuntimeCandidateCommand, feedback runtimecontract.RuntimeFeedback) error {
	if err := feedback.Validate(); err != nil {
		return fmt.Errorf("LLM block returned invalid provider preflight feedback: %w", err)
	}
	if feedback.Domain != runtimecontract.RuntimeFeedbackDomainProviderPreflight || feedback.WorkspaceID != command.WorkspaceID || feedback.RunID != command.RunID {
		return fmt.Errorf("LLM block returned provider preflight feedback outside the requested scope")
	}
	wantAuthority := ""
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		wantAuthority = command.Transaction.AuthoritySnapshotID
	}
	if feedback.AuthoritySnapshotID != wantAuthority || feedback.DispatchID != "" {
		return fmt.Errorf("LLM block returned provider preflight feedback with mismatched authority identity")
	}
	switch feedback.StableCategory {
	case runtimecontract.RuntimeFeedbackCategoryAccepted:
		if feedback.OriginalDiagnostic != nil {
			return fmt.Errorf("accepted provider preflight feedback carried a failure diagnostic")
		}
	case runtimecontract.RuntimeFeedbackCategoryContextIntegrity:
		if feedback.OriginalDiagnostic == nil || feedback.OriginalDiagnostic.OwnerModuleID != runtimecontract.RuntimeModuleLLM || feedback.OriginalDiagnostic.OriginalClass != "context_integrity" {
			return fmt.Errorf("provider preflight context-integrity feedback is missing its owner diagnostic")
		}
	default:
		return fmt.Errorf("LLM block returned unknown provider preflight feedback category %q", feedback.StableCategory)
	}
	return nil
}

func RequestRuntimeCandidate(ctx context.Context, hub magicevent.Hub, command llmevents.RequestRuntimeCandidateCommand) (llmevents.RequestRuntimeCandidateResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" {
		return llmevents.RequestRuntimeCandidateResult{}, common.ErrInvalidArgument
	}
	if err := command.RequestIdentity.Validate(); err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, err
	}
	if err := validateRuntimeCandidateCommand(command); err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	response, err := sendRuntimeCandidateRequest(ctx, hub, command)
	if err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, err
	}
	if err := response.Validate(); err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an invalid runtime candidate result: %w", err)
	}
	wantExpectedHash, err := validateRuntimeCandidateResponseIdentity(command, response)
	if err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, err
	}
	if response.Stale != nil {
		return normalizeRuntimeCandidateStaleResponse(response)
	}
	if response.Feedback != nil {
		return normalizeRuntimeCandidateFeedbackResponse(command, response)
	}
	if response.DocumentOutcome != nil {
		return normalizeRuntimeCandidateDocumentOutcome(response, wantExpectedHash)
	}
	return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("runtime candidate response returned no exhaustive LLM owner outcome")
}

func sendRuntimeCandidateRequest(ctx context.Context, hub magicevent.Hub, command llmevents.RequestRuntimeCandidateCommand) (llmevents.RequestRuntimeCandidateResult, error) {
	result := hub.Send(magicevent.NewEventWithContext(
		llmevents.TopicRequestRuntimeCandidate,
		ModuleID,
		runtimecontract.RuntimeModuleLLM,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return llmevents.RequestRuntimeCandidateResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return llmevents.RequestRuntimeCandidateResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(llmevents.RequestRuntimeCandidateResult)
	if !ok {
		return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an invalid runtime candidate result")
	}
	return response, nil
}

func validateRuntimeCandidateResponseIdentity(command llmevents.RequestRuntimeCandidateCommand, response llmevents.RequestRuntimeCandidateResult) (string, error) {
	if response.WorkspaceID != command.WorkspaceID || response.RunID != command.RunID {
		return "", fmt.Errorf("runtime candidate result scope does not match request")
	}
	if response.DocumentKind != command.DocumentKind {
		return "", fmt.Errorf("runtime candidate result document kind does not match request")
	}
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		if err := runtimecontract.ValidateAuthorityTransaction(command.Decision, response.Transaction); err != nil {
			return "", err
		}
	}
	if !runtimecontract.SameRuntimeCandidateRequestIdentity(response.RequestIdentity, command.RequestIdentity) {
		return "", fmt.Errorf("runtime candidate result identity does not match request")
	}
	wantExpectedHash := ""
	if command.ExpectedDocument != nil {
		wantExpectedHash = command.ExpectedDocument.Normalized().Hash
	}
	if strings.TrimSpace(response.ExpectedDocumentHash) != wantExpectedHash {
		return "", fmt.Errorf("runtime candidate result expected-document identity does not match request")
	}
	return wantExpectedHash, nil
}

func normalizeRuntimeCandidateStaleResponse(response llmevents.RequestRuntimeCandidateResult) (llmevents.RequestRuntimeCandidateResult, error) {
	if response.DocumentOutcome != nil {
		return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned stale together with provider document outcome")
	}
	if response.Feedback != nil {
		return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned stale together with provider feedback")
	}
	stale := response.Stale.Normalized()
	if err := stale.Validate(); err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an invalid runtime prerequisite stale: %w", err)
	}
	response.Stale = &stale
	return response, nil
}

func normalizeRuntimeCandidateFeedbackResponse(command llmevents.RequestRuntimeCandidateCommand, response llmevents.RequestRuntimeCandidateResult) (llmevents.RequestRuntimeCandidateResult, error) {
	if response.DocumentOutcome != nil {
		return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an output fragment together with provider feedback")
	}
	if err := validateRuntimeProviderRequestFeedback(command, *response.Feedback); err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, err
	}
	return response, nil
}

func normalizeRuntimeCandidateDocumentOutcome(response llmevents.RequestRuntimeCandidateResult, wantExpectedHash string) (llmevents.RequestRuntimeCandidateResult, error) {
	outcome := response.DocumentOutcome.Normalized()
	if err := outcome.Validate(); err != nil {
		return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned an invalid provider document outcome: %w", err)
	}
	if outcome.Framing.ExpectedDocumentHash != wantExpectedHash {
		return llmevents.RequestRuntimeCandidateResult{}, fmt.Errorf("LLM block returned a provider document outcome for another expected document")
	}
	response.DocumentOutcome = &outcome
	return response, nil
}

func validateRuntimeProviderRequestFeedback(command llmevents.RequestRuntimeCandidateCommand, feedback runtimecontract.RuntimeFeedback) error {
	if err := llmevents.ValidateRuntimeProviderRequestFeedback(feedback); err != nil {
		return fmt.Errorf("LLM block returned invalid provider feedback: %w", err)
	}
	if feedback.WorkspaceID != command.WorkspaceID || feedback.RunID != command.RunID {
		return fmt.Errorf("LLM block returned provider feedback outside the requested scope")
	}
	wantAuthority := ""
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		wantAuthority = command.Transaction.AuthoritySnapshotID
	}
	if feedback.AuthoritySnapshotID != wantAuthority {
		return fmt.Errorf("LLM block returned provider feedback with mismatched authority identity")
	}
	return nil
}

// runtimeProviderRequestFailure is an Orchestrator-owner private adapter. The
// RuntimeCandidateFailure it returns never crosses EventHub and is used only by
// the existing SM-3 transition table.
func runtimeProviderRequestFailure(feedback runtimecontract.RuntimeFeedback) runtimecontract.RuntimeCandidateFailure {
	diagnostic := feedback.OriginalDiagnostic
	failure := runtimecontract.RuntimeCandidateFailure{}
	if diagnostic == nil {
		return failure
	}
	failure.Class = runtimecontract.RuntimeCandidateFailureClass(diagnostic.OriginalClass)
	failure.Code = diagnostic.OriginalCode
	failure.Signature = diagnostic.Signature
	failure.Reason = diagnostic.OriginalMessage
	failure.TargetRefs = append([]string(nil), diagnostic.TargetRefs...)
	if len(diagnostic.FieldViolations) != 0 {
		violation := diagnostic.FieldViolations[0]
		failure.Diagnostic = &runtimecontract.RuntimeCandidateFailureDiagnostic{
			OperationID: diagnostic.OperationID, Path: violation.Path, Actual: violation.Actual,
			Allowed: append([]string(nil), violation.Allowed...), Required: append([]string(nil), violation.Required...), Forbidden: append([]string(nil), violation.Forbidden...),
		}
	}
	return failure
}

func validateRuntimeCandidateRetryContract(attempt int, previousFailure *runtimecontract.RuntimeCandidateFailure) error {
	if attempt < 0 || attempt > 1 {
		return fmt.Errorf("runtime candidate attempt must be 0 or 1")
	}
	if attempt == 0 {
		if previousFailure != nil {
			return fmt.Errorf("initial runtime candidate attempt must not carry a previous failure")
		}
		return nil
	}
	if previousFailure == nil || previousFailure.Class == "" || strings.TrimSpace(previousFailure.Reason) == "" {
		return fmt.Errorf("runtime candidate retry requires the previous failure")
	}
	return nil
}

func validateRuntimeCandidatePreflightCommand(command llmevents.PreflightRuntimeCandidateCommand) error {
	switch command.DocumentKind {
	case runtimecontract.RuntimeProviderDocumentOperations:
		if command.SemanticManifest.Hash != "" || command.SemanticContext.WorkspaceID != "" || command.SemanticContext.RunID != "" || command.PreviousSemanticRejection != nil || command.SemanticCorrectionDirective != nil {
			return fmt.Errorf("operations candidate preflight must not carry semantic plan fields")
		}
		if command.Decision.AuthoritySnapshot.WorkspaceID != command.WorkspaceID || command.Decision.AuthoritySnapshot.RunID != command.RunID {
			return fmt.Errorf("runtime candidate preflight scope does not match authority snapshot")
		}
		if err := runtimecontract.ValidateAuthorityTransaction(command.Decision, command.Transaction); err != nil {
			return err
		}
		if err := command.DefinitionContext.Validate(); err != nil {
			return err
		}
		if command.DefinitionContext.Normalized().Revision != command.Transaction.DefinitionRevision {
			return fmt.Errorf("runtime candidate preflight definition revision does not match authority transaction")
		}
		if err := command.CandidateContext.Validate(command.Decision); err != nil {
			return err
		}
		if err := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(command.CandidateManifest, command.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(command.CandidateContext)); err != nil {
			return err
		}
		return validateRuntimeCandidateRetryContract(command.CandidateAttempt, command.PreviousFailure)
	case runtimecontract.RuntimeProviderDocumentSemanticPlan:
		if command.CandidateManifest.Hash != "" || command.CandidateContext.WorkspaceID != "" || command.CandidateContext.RunID != "" || command.DefinitionContext.Revision != "" || command.PreviousFailure != nil {
			return fmt.Errorf("semantic plan preflight must not carry operations fields")
		}
		if command.SemanticContext.WorkspaceID != command.WorkspaceID || command.SemanticContext.RunID != command.RunID {
			return fmt.Errorf("runtime semantic plan preflight scope mismatch")
		}
		if err := command.SemanticContext.Validate(command.SemanticManifest); err != nil {
			return err
		}
		return validateRuntimeSemanticRetryContract(command.CandidateAttempt, command.SemanticManifest, command.SemanticContext.PlanStep, command.StablePrefixHash, command.PreviousSemanticRejection, command.SemanticCorrectionDirective)
	default:
		return fmt.Errorf("runtime candidate preflight requires explicit document kind")
	}
}

func validateRuntimeCandidateCommand(command llmevents.RequestRuntimeCandidateCommand) error {
	preflight := llmevents.PreflightRuntimeCandidateCommand{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, DocumentKind: command.DocumentKind,
		Decision: command.Decision, Transaction: command.Transaction, CandidateAttempt: command.CandidateAttempt,
		PreviousFailure: command.PreviousFailure, StablePrefixHash: command.StablePrefixHash,
		CandidateManifest: command.CandidateManifest, DefinitionContext: command.DefinitionContext, CandidateContext: command.CandidateContext,
		SemanticManifest: command.SemanticManifest, SemanticContext: command.SemanticContext, PreviousSemanticRejection: command.PreviousSemanticRejection, SemanticCorrectionDirective: command.SemanticCorrectionDirective,
	}
	if err := validateRuntimeCandidatePreflightCommand(preflight); err != nil {
		return err
	}
	if command.ExpectedDocument == nil {
		return fmt.Errorf("runtime candidate command requires expected document")
	}
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		expected := command.ExpectedDocument.Normalized()
		if err := expected.Validate(); err != nil || expected.LogicalRequestID != command.RequestIdentity.LogicalRequestID || expected.DocumentKind != command.DocumentKind || expected.ManifestHash != command.SemanticManifest.Hash || expected.StablePrefixHash != command.StablePrefixHash || expected.CorrectionAttempt != command.CandidateAttempt || expected.PlanStep != command.SemanticContext.PlanStep {
			return fmt.Errorf("semantic runtime candidate command has invalid expected document")
		}
	} else {
		expected := command.ExpectedDocument.Normalized()
		if err := expected.Validate(); err != nil || expected.LogicalRequestID != command.RequestIdentity.LogicalRequestID || expected.DocumentKind != command.DocumentKind || expected.ManifestHash != command.CandidateManifest.Hash || expected.StablePrefixHash != command.StablePrefixHash || expected.CorrectionAttempt != command.CandidateAttempt {
			return fmt.Errorf("operations runtime candidate command has invalid expected document")
		}
	}
	return nil
}

func validateRuntimeSemanticRetryContract(attempt int, manifest runtimecontract.RuntimeSemanticPlanManifest, planStep runtimecontract.RuntimeSemanticPlanStep, stablePrefixHash string, rejection *runtimecontract.RuntimeOriginalDiagnostic, directive *runtimecontract.RuntimeSemanticCorrectionDirective) error {
	if attempt < 0 || attempt > 1 {
		return fmt.Errorf("runtime semantic plan attempt must be 0 or 1")
	}
	if attempt == 0 {
		if rejection != nil || directive != nil {
			return fmt.Errorf("initial runtime semantic plan attempt must not carry correction lineage")
		}
		return nil
	}
	if rejection == nil || directive == nil {
		return fmt.Errorf("runtime semantic plan retry requires rejection and correction directive")
	}
	if err := rejection.Validate(); err != nil {
		return err
	}
	if err := directive.ValidateAgainst(manifest, *rejection); err != nil {
		return err
	}
	if directive.PlanStep != planStep || directive.StablePrefixHash != strings.TrimSpace(stablePrefixHash) {
		return fmt.Errorf("runtime semantic plan correction identity is stale")
	}
	return nil
}
