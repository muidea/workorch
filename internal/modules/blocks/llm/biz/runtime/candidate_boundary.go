package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CandidateProvider produces a typed Runtime candidate generation result.
// Implementations must return sealed audit (target/revision/budget/status) for both
// successful and failed provider submissions so callers can persist usage audit.
type CandidateProvider interface {
	GenerateCandidate(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error)
}

func RequestCandidate(ctx context.Context, provider CandidateProvider, request llmmodels.RuntimeCandidateRequest) (response llmmodels.RuntimeCandidateResponse, err error) {
	defer func() {
		if request.ExpectedDocument != nil {
			response.ExpectedDocumentHash = request.ExpectedDocument.Normalized().Hash
		}
	}()
	if provider == nil {
		return llmmodels.RuntimeCandidateResponse{}, fmt.Errorf("candidate provider is required")
	}
	if err := validateCandidateRequest(request); err != nil {
		return llmmodels.RuntimeCandidateResponse{}, candidateRequestError(CandidateFailureClassInternal, err, nil, "", "")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	generation, err := provider.GenerateCandidate(ctx, request)
	if err != nil {
		if generation.ReachedProviderLimit {
			return providerDocumentResponse(request, generation), nil
		}
		if stale := generation.PrerequisiteStale; stale != nil {
			staleCopy := stale.Normalized()
			if validateErr := staleCopy.Validate(); validateErr != nil {
				return llmmodels.RuntimeCandidateResponse{}, candidateRequestError(CandidateFailureClassInternal, fmt.Errorf("invalid LLM prerequisite stale: %w", validateErr), generation.Audit, generation.RequestID, generation.Status)
			}
			return llmmodels.RuntimeCandidateResponse{
				Transaction: request.Transaction, DocumentKind: request.DocumentKind, RequestIdentity: request.RequestIdentity,
				Audit: generation.Audit, Usage: generation.Usage,
				PrerequisiteStale: &staleCopy,
			}, candidateRequestErrorWithStale(err, generation.Audit, generation.RequestID, generation.Status, staleCopy)
		}
		failureClass := runtimecontract.RuntimeCandidateFailureClass(strings.TrimSpace(generation.FailureClass))
		if failureClass == "" {
			failureClass = CandidateFailureClassProvider
		}
		code, signature := providerFailureIdentity(err)
		return llmmodels.RuntimeCandidateResponse{
			Transaction: request.Transaction, DocumentKind: request.DocumentKind, RequestIdentity: request.RequestIdentity,
			Audit: generation.Audit,
			Usage: generation.Usage,
		}, candidateRequestErrorWithIdentity(failureClass, code, signature, err, generation.Audit, generation.RequestID, generation.Status)
	}
	return providerDocumentResponse(request, generation), nil
}

func providerDocumentResponse(request llmmodels.RuntimeCandidateRequest, generation llmmodels.RuntimeCandidateGenerationResult) llmmodels.RuntimeCandidateResponse {
	payload := append([]byte(nil), generation.Payload...)
	disposition := inspectProviderCandidateDocumentFraming(payload)
	outcome := runtimecontract.RuntimeProviderDocumentOutcome{
		ExpectedDocumentHash: request.ExpectedDocument.Normalized().Hash,
		ResponseHash:         runtimecontract.StableRuntimeProviderResponseHash(payload),
		ResponseBytes:        int64(len(payload)),
	}
	response := llmmodels.RuntimeCandidateResponse{
		Transaction: request.Transaction, DocumentKind: request.DocumentKind, RequestIdentity: request.RequestIdentity,
		Audit: generation.Audit, Usage: generation.Usage,
	}
	switch disposition {
	case runtimecontract.RuntimeCandidateDocumentComplete:
		outcome.Disposition = runtimecontract.RuntimeProviderDocumentComplete
		response.DocumentContent = payload
	case runtimecontract.RuntimeCandidateDocumentIncomplete:
		outcome.Disposition = runtimecontract.RuntimeProviderDocumentIncomplete
		outcome.Code = "incomplete_json_document"
		outcome.Signature = "provider-document-incomplete"
		if generation.ReachedProviderLimit || candidatePayloadReachedOutputLimit(generation, io.ErrUnexpectedEOF) {
			outcome.Code = "max_output_tokens"
			outcome.Signature = "provider-output-limit"
		}
	case runtimecontract.RuntimeCandidateDocumentEmpty:
		outcome.Disposition = runtimecontract.RuntimeProviderDocumentEmpty
		outcome.Code = "provider_final_output_missing"
		outcome.Signature = "provider-final-output-missing"
	case runtimecontract.RuntimeCandidateDocumentInvalid:
		outcome.Disposition = runtimecontract.RuntimeProviderDocumentInvalid
		outcome.Code = "provider_final_output_invalid"
		outcome.Signature = "provider-final-output-invalid"
	}
	response.DocumentOutcome = &outcome
	return response
}

func inspectProviderCandidateDocumentFraming(payload []byte) runtimecontract.RuntimeCandidateDocumentDisposition {
	normalized, _ := runtimecontract.NormalizeRuntimeCandidatePayloadDocument(payload)
	return runtimecontract.InspectRuntimeCandidateDocumentFraming(normalized)
}

// providerFinalOutputInvalidError keeps a provider-declared final output that
// is not JSON out of SM-3 correction and continuation. The raw text remains
// provider-owned and is deliberately not copied into cross-owner feedback.
func providerFinalOutputInvalidError(generation llmmodels.RuntimeCandidateGenerationResult) error {
	return candidateRequestErrorWithIdentity(
		CandidateFailureClassProvider,
		"provider_final_output_invalid",
		"provider-final-output-invalid",
		fmt.Errorf("provider final output is not a JSON document"),
		generation.Audit,
		generation.RequestID,
		generation.Status,
	)
}

func providerFailureIdentity(err error) (string, string) {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "body idle timeout"):
		return "provider_body_idle_timeout", "provider-body-idle-timeout"
	case strings.Contains(message, "provider request timeout") || strings.Contains(message, "context deadline exceeded"):
		return "provider_transport_timeout", "provider-transport-timeout"
	case strings.Contains(message, "empty response"):
		return "provider_empty_response", "provider-empty-response"
	default:
		return "provider_request_failed", "provider-request-failed"
	}
}

type CandidateRequestError struct {
	Class      runtimecontract.RuntimeCandidateFailureClass
	Code       string
	Signature  string
	Diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic
	Err        error
	Audit      *llmmodels.ProviderRequestAudit
	RequestID  string
	Status     string
	Stale      *runtimecontract.RuntimePrerequisiteStale
}

func (e *CandidateRequestError) Error() string {
	if e == nil || e.Err == nil {
		return "candidate request failed"
	}
	return e.Err.Error()
}

func (e *CandidateRequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func CandidateFailureFromError(err error) runtimecontract.RuntimeCandidateFailure {
	failure := runtimecontract.RuntimeCandidateFailure{Class: CandidateFailureClassInternal, Reason: "candidate request failed"}
	if err == nil {
		return failure
	}
	var typed *CandidateRequestError
	if errors.As(err, &typed) {
		failure.Class = typed.Class
		failure.Code = typed.Code
		failure.Signature = typed.Signature
		failure.Diagnostic = typed.Diagnostic.Clone()
	}
	// CandidateRequestError may wrap an HTTP client error containing an
	// endpoint, credential header, or raw provider fragment. Cross-owner Runtime
	// feedback is deliberately audit-safe; detailed transport facts remain in
	// the LLM owner's sealed request audit and never become an Orchestrator
	// diagnostic string.
	switch failure.Class {
	case CandidateFailureClassProviderAuthentication:
		failure.Reason = "provider authentication failed"
	case CandidateFailureClassProviderConfiguration:
		failure.Reason = "provider configuration failed"
	case CandidateFailureClassProviderTransient:
		failure.Reason = "provider request failed transiently"
	case CandidateFailureClassProvider:
		switch failure.Code {
		case "provider_final_output_invalid":
			failure.Reason = "provider final output is not a JSON document"
		case "provider_final_output_missing":
			failure.Reason = "provider produced no final output text"
		default:
			failure.Reason = "provider response failed"
		}
	case CandidateFailureClassSchema, CandidateFailureClassContract:
		failure.Reason = "candidate document rejected by typed contract"
	default:
		failure.Reason = "candidate request failed"
	}
	return failure
}

// CandidateAuditFromError extracts sealed request audit carried on a CandidateRequestError.
func CandidateAuditFromError(err error) (*llmmodels.ProviderRequestAudit, string, string) {
	var typed *CandidateRequestError
	if errors.As(err, &typed) && typed != nil {
		return typed.Audit, typed.RequestID, typed.Status
	}
	return nil, "", ""
}

// CandidatePrerequisiteStaleFromError extracts an owner-confirmed stale
// observation without turning it into a candidate/provider failure.
func CandidatePrerequisiteStaleFromError(err error) *runtimecontract.RuntimePrerequisiteStale {
	var typed *CandidateRequestError
	if !errors.As(err, &typed) || typed == nil || typed.Stale == nil {
		return nil
	}
	stale := typed.Stale.Normalized()
	return &stale
}

func candidateRequestError(class runtimecontract.RuntimeCandidateFailureClass, err error, audit *llmmodels.ProviderRequestAudit, requestID, status string) error {
	return &CandidateRequestError{Class: class, Err: err, Audit: audit, RequestID: requestID, Status: status}
}

func candidateRequestErrorWithIdentity(class runtimecontract.RuntimeCandidateFailureClass, code, signature string, err error, audit *llmmodels.ProviderRequestAudit, requestID, status string) error {
	return candidateRequestErrorWithDiagnosticIdentity(class, code, signature, nil, err, audit, requestID, status)
}

func candidateRequestErrorWithDiagnosticIdentity(class runtimecontract.RuntimeCandidateFailureClass, code, signature string, diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic, err error, audit *llmmodels.ProviderRequestAudit, requestID, status string) error {
	return &CandidateRequestError{
		Class: class, Code: strings.TrimSpace(code), Signature: strings.TrimSpace(signature), Diagnostic: diagnostic.Clone(),
		Err: err, Audit: audit, RequestID: requestID, Status: status,
	}
}

func candidatePayloadReachedOutputLimit(generation llmmodels.RuntimeCandidateGenerationResult, decodeErr error) bool {
	if decodeErr == nil || (!errors.Is(decodeErr, io.ErrUnexpectedEOF) && runtimecontract.RuntimeCandidateDocumentDispositionForError(decodeErr) != runtimecontract.RuntimeCandidateDocumentIncomplete) ||
		generation.Usage == nil || generation.Audit == nil || generation.Audit.ContextBudget == nil {
		return false
	}
	limit := generation.Audit.ContextBudget.EffectiveMaxOutputTokens
	return limit > 0 && generation.Usage.OutputTokens >= limit
}

func candidateRequestErrorWithStale(err error, audit *llmmodels.ProviderRequestAudit, requestID, status string, stale runtimecontract.RuntimePrerequisiteStale) error {
	stale = stale.Normalized()
	return &CandidateRequestError{Class: CandidateFailureClassInternal, Err: err, Audit: audit, RequestID: requestID, Status: status, Stale: &stale}
}

const (
	CandidateFailureClassSchema                 = runtimecontract.RuntimeCandidateFailureClassSchema
	CandidateFailureClassContract               = runtimecontract.RuntimeCandidateFailureClassContract
	CandidateFailureClassProvider               = runtimecontract.RuntimeCandidateFailureClassProvider
	CandidateFailureClassProviderTransient      = runtimecontract.RuntimeCandidateFailureClassProviderTransient
	CandidateFailureClassProviderAuthentication = runtimecontract.RuntimeCandidateFailureClassProviderAuthentication
	CandidateFailureClassProviderConfiguration  = runtimecontract.RuntimeCandidateFailureClassProviderConfiguration
	CandidateFailureClassInternal               = runtimecontract.RuntimeCandidateFailureClassInternal
)

func validateCandidateRequest(request llmmodels.RuntimeCandidateRequest) error {
	if err := request.RequestIdentity.Validate(); err != nil {
		return fmt.Errorf("candidate request identity: %w", err)
	}
	if request.CandidateAttempt < 0 || request.CandidateAttempt > 1 {
		return fmt.Errorf("candidate attempt must be 0 or 1")
	}
	if strings.TrimSpace(request.StablePrefixHash) == "" {
		return fmt.Errorf("candidate request requires stable prefix hash")
	}
	if request.DocumentKind != runtimecontract.RuntimeProviderDocumentOperations && request.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan {
		return fmt.Errorf("candidate request requires explicit document kind")
	}
	if request.ExpectedDocument != nil {
		expected := request.ExpectedDocument.Normalized()
		if err := expected.Validate(); err != nil {
			return fmt.Errorf("candidate expected document: %w", err)
		}
		manifestHash := request.CandidateManifest.Hash
		if request.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			manifestHash = request.SemanticManifest.Hash
		}
		if expected.DocumentKind != request.DocumentKind || expected.LogicalRequestID != request.RequestIdentity.LogicalRequestID || expected.ManifestHash != manifestHash || expected.StablePrefixHash != request.StablePrefixHash {
			return fmt.Errorf("candidate expected document identity does not match request")
		}
	}
	switch request.DocumentKind {
	case runtimecontract.RuntimeProviderDocumentOperations:
		return validateOperationsDocumentRequest(request)
	case runtimecontract.RuntimeProviderDocumentSemanticPlan:
		return validateSemanticPlanDocumentRequest(request)
	default:
		return fmt.Errorf("candidate request requires explicit document kind")
	}
}

func validateOperationsDocumentRequest(request llmmodels.RuntimeCandidateRequest) error {
	if request.SemanticManifest.Hash != "" || request.SemanticContext.WorkspaceID != "" || request.SemanticContext.RunID != "" || request.PreviousSemanticRejection != nil || request.SemanticCorrectionDirective != nil {
		return fmt.Errorf("operations document request must not carry semantic plan fields")
	}
	if request.CandidateAttempt == 0 && request.PreviousFailure != nil {
		return fmt.Errorf("initial candidate attempt must not carry a previous failure")
	}
	if request.CandidateAttempt == 1 && (request.PreviousFailure == nil || request.PreviousFailure.Class == "" || strings.TrimSpace(request.PreviousFailure.Reason) == "") {
		return fmt.Errorf("candidate retry requires the previous failure")
	}
	if request.ExpectedDocument == nil || request.ExpectedDocument.Normalized().CorrectionAttempt != request.CandidateAttempt {
		return fmt.Errorf("operations request requires a matching sealed expected document")
	}
	if err := request.DefinitionContext.Validate(); err != nil {
		return err
	}
	if request.DefinitionContext.Normalized().Revision != request.Transaction.DefinitionRevision {
		return fmt.Errorf("candidate request definition revision does not match authority transaction")
	}
	if err := request.CandidateContext.Validate(request.Decision); err != nil {
		return err
	}
	if err := runtimecontract.ValidateAuthorityTransaction(request.Decision, request.Transaction); err != nil {
		return err
	}
	switch request.Decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment, runtimecontract.ConvergenceStateNeedGovernance:
		return nil
	default:
		return fmt.Errorf("convergence state %s cannot be sent to the LLM candidate boundary", request.Decision.State)
	}
}

func validateSemanticPlanDocumentRequest(request llmmodels.RuntimeCandidateRequest) error {
	if request.CandidateManifest.Hash != "" || request.CandidateContext.WorkspaceID != "" || request.CandidateContext.RunID != "" || request.DefinitionContext.Revision != "" || request.PreviousFailure != nil {
		return fmt.Errorf("semantic plan document request must not carry operations fields")
	}
	if request.CandidateAttempt == 0 && (request.PreviousSemanticRejection != nil || request.SemanticCorrectionDirective != nil) {
		return fmt.Errorf("initial semantic plan attempt must not carry correction lineage")
	}
	if request.CandidateAttempt == 1 {
		if request.PreviousSemanticRejection == nil || request.SemanticCorrectionDirective == nil {
			return fmt.Errorf("semantic plan retry requires rejection and correction directive")
		}
		if err := request.PreviousSemanticRejection.Validate(); err != nil {
			return err
		}
		if err := request.SemanticCorrectionDirective.ValidateAgainst(request.SemanticManifest, *request.PreviousSemanticRejection); err != nil {
			return err
		}
	}
	if err := request.SemanticContext.Validate(request.SemanticManifest); err != nil {
		return err
	}
	if request.ExpectedDocument == nil {
		return fmt.Errorf("semantic plan request requires a sealed expected document")
	}
	expected := request.ExpectedDocument.Normalized()
	context := request.SemanticContext.Normalized()
	if request.CandidateAttempt == 1 && (request.SemanticCorrectionDirective.PlanStep != context.PlanStep || request.SemanticCorrectionDirective.StablePrefixHash != strings.TrimSpace(request.StablePrefixHash)) {
		return fmt.Errorf("semantic correction identity is stale")
	}
	if expected.PlanStep != context.PlanStep || expected.CorrectionAttempt != request.CandidateAttempt || expected.DefinitionSeedHash != context.DefinitionSeed.Hash || expected.BatchID != context.BatchID || expected.BatchSequence != context.BatchSequence || !sameCoverageWorkItemIDs(expected.BatchCoverageWorkItemIDs, context.BatchCoverageWorkItemIDs) {
		return fmt.Errorf("semantic expected document does not match request context")
	}
	return nil
}

func sameCoverageWorkItemIDs(left, right []common.CoverageWorkItemID) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
