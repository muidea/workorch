package runtimev2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CandidateProvider produces a typed Runtime V2 candidate generation result.
// Implementations must return sealed audit (target/revision/budget/status) for both
// successful and failed provider submissions so callers can persist usage audit.
type CandidateProvider interface {
	GenerateCandidate(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error)
}

func RequestCandidate(ctx context.Context, provider CandidateProvider, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateResponse, error) {
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
		if stale := generation.PrerequisiteStale; stale != nil {
			staleCopy := stale.Normalized()
			if validateErr := staleCopy.Validate(); validateErr != nil {
				return llmmodels.RuntimeCandidateResponse{}, candidateRequestError(CandidateFailureClassInternal, fmt.Errorf("invalid LLM prerequisite stale: %w", validateErr), generation.Audit, generation.RequestID, generation.Status)
			}
			return llmmodels.RuntimeCandidateResponse{
				Transaction: request.Transaction,
				Audit:       generation.Audit, Usage: generation.Usage,
				RequestID:         generation.RequestID,
				Status:            firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusFailed),
				PrerequisiteStale: &staleCopy,
			}, candidateRequestErrorWithStale(err, generation.Audit, generation.RequestID, generation.Status, staleCopy)
		}
		failureClass := runtimecontract.RuntimeCandidateFailureClass(strings.TrimSpace(generation.FailureClass))
		if failureClass == "" {
			failureClass = CandidateFailureClassProvider
		}
		return llmmodels.RuntimeCandidateResponse{
			Transaction: request.Transaction,
			Audit:       generation.Audit,
			Usage:       generation.Usage,
			RequestID:   generation.RequestID,
			Status:      firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusFailed),
		}, candidateRequestError(failureClass, err, generation.Audit, generation.RequestID, generation.Status)
	}
	// A suffix request intentionally uses raw text output rather than the
	// structured JSON response mode. Its bytes are owned and sequenced by the
	// Orchestrator; decode occurs only after the complete document is assembled.
	if request.OutputContinuation != nil {
		if err := request.OutputContinuation.Validate(); err != nil {
			return llmmodels.RuntimeCandidateResponse{}, candidateRequestError(CandidateFailureClassInternal, err, generation.Audit, generation.RequestID, generation.Status)
		}
		fragment := runtimecontract.RuntimeCandidateOutputFragment{Content: append([]byte(nil), generation.Payload...), ReachedProviderLimit: candidatePayloadReachedOutputLimit(generation, io.ErrUnexpectedEOF)}
		if err := fragment.Validate(); err != nil {
			return llmmodels.RuntimeCandidateResponse{}, candidateRequestError(CandidateFailureClassSchema, err, generation.Audit, generation.RequestID, generation.Status)
		}
		return llmmodels.RuntimeCandidateResponse{Transaction: request.Transaction, OutputFragment: &fragment, Audit: generation.Audit, Usage: generation.Usage, RequestID: generation.RequestID, Status: firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusCompleted)}, nil
	}
	manifest := request.CandidateManifest
	if manifestErr := runtimecontract.ValidateRuntimeCandidateContractManifestForDecision(manifest, request.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(request.CandidateContext.Resources)); manifestErr != nil {
		return llmmodels.RuntimeCandidateResponse{
			Transaction: request.Transaction,
			Audit:       generation.Audit,
			Usage:       generation.Usage,
			RequestID:   generation.RequestID,
			Status:      firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusCompleted),
		}, candidateRequestError(CandidateFailureClassInternal, fmt.Errorf("runtime candidate contract manifest: %w", manifestErr), generation.Audit, generation.RequestID, generation.Status)
	}
	candidate, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, generation.Payload)
	if err != nil {
		switch runtimecontract.RuntimeCandidateDocumentDispositionForError(err) {
		case runtimecontract.RuntimeCandidateDocumentIncomplete:
			// EOF is structural evidence that the first document is incomplete.
			// Provider output-limit telemetry is retained only as audit metadata;
			// a mid-token/network-cut body must follow the same continuation path.
			fragment := runtimecontract.RuntimeCandidateOutputFragment{Content: append([]byte(nil), generation.Payload...), ReachedProviderLimit: candidatePayloadReachedOutputLimit(generation, err)}
			return llmmodels.RuntimeCandidateResponse{Transaction: request.Transaction, OutputFragment: &fragment, Audit: generation.Audit, Usage: generation.Usage, RequestID: generation.RequestID, Status: firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusCompleted)}, nil
		case runtimecontract.RuntimeCandidateDocumentEmpty:
			return llmmodels.RuntimeCandidateResponse{
				Transaction: request.Transaction,
				Audit:       generation.Audit,
				Usage:       generation.Usage,
				RequestID:   generation.RequestID,
				Status:      firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusCompleted),
			}, candidateRequestErrorWithIdentity(CandidateFailureClassProvider, "provider_empty_response", "provider-empty-response", err, generation.Audit, generation.RequestID, generation.Status)
		}
		code, signature, reason := candidateSchemaFailureIdentity(generation, err)
		var payloadDiagnostic *runtimecontract.RuntimeCandidatePayloadDiagnostic
		errors.As(err, &payloadDiagnostic)
		return llmmodels.RuntimeCandidateResponse{
				Transaction: request.Transaction,
				Audit:       generation.Audit,
				Usage:       generation.Usage,
				RequestID:   generation.RequestID,
				Status:      firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusCompleted),
			}, candidateRequestErrorWithDocument(
				candidateRequestErrorWithDiagnosticIdentity(CandidateFailureClassSchema, code, signature, runtimecontract.RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(payloadDiagnostic), errors.New(reason), generation.Audit, generation.RequestID, generation.Status),
				generation.Payload,
			)
	}
	// The LLM Block owns transport and strict wire decoding only. A complete
	// document is returned unchanged to the Orchestrator, which is the single
	// SM-3 evaluator for candidate semantics and correction disposition.
	return llmmodels.RuntimeCandidateResponse{
		Transaction:     request.Transaction,
		Candidate:       candidate,
		DocumentContent: append([]byte(nil), generation.Payload...),
		Audit:           generation.Audit,
		Usage:           generation.Usage,
		RequestID:       generation.RequestID,
		Status:          firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusCompleted),
	}, nil
}

type CandidateRequestError struct {
	Class      CandidateFailureClass
	Code       string
	Signature  string
	Diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic
	Err        error
	Audit      *llmmodels.ProviderRequestAudit
	RequestID  string
	Status     string
	Stale      *runtimecontract.RuntimePrerequisiteStale
	// DocumentContent is a synchronous, in-process handoff for a complete
	// provider document that failed schema/contract validation. It must never
	// be included in audit, JSON serialization, or error text. The LLM owner
	// may forward it only to the Orchestrator request-content store, which
	// persists it before C5 allocates a candidate attempt.
	DocumentContent []byte
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
	failure.Reason = err.Error()
	var typed *CandidateRequestError
	if errors.As(err, &typed) {
		failure.Class = typed.Class
		failure.Code = typed.Code
		failure.Signature = typed.Signature
		failure.Diagnostic = typed.Diagnostic.Clone()
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

// CandidateDocumentContentFromError returns a defensive copy of the complete
// provider document attached to a schema/contract rejection. Provider,
// transport, authentication, configuration, stale and internal failures never
// expose a document through this boundary.
func CandidateDocumentContentFromError(err error) []byte {
	var typed *CandidateRequestError
	if !errors.As(err, &typed) || typed == nil || len(typed.DocumentContent) == 0 {
		return nil
	}
	switch typed.Class {
	case CandidateFailureClassSchema, CandidateFailureClassContract:
		return append([]byte(nil), typed.DocumentContent...)
	default:
		return nil
	}
}

func candidateRequestError(class CandidateFailureClass, err error, audit *llmmodels.ProviderRequestAudit, requestID, status string) error {
	return &CandidateRequestError{Class: class, Err: err, Audit: audit, RequestID: requestID, Status: status}
}

func candidateRequestErrorWithIdentity(class CandidateFailureClass, code, signature string, err error, audit *llmmodels.ProviderRequestAudit, requestID, status string) error {
	return candidateRequestErrorWithDiagnosticIdentity(class, code, signature, nil, err, audit, requestID, status)
}

func candidateRequestErrorWithDiagnosticIdentity(class CandidateFailureClass, code, signature string, diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic, err error, audit *llmmodels.ProviderRequestAudit, requestID, status string) error {
	return &CandidateRequestError{
		Class: class, Code: strings.TrimSpace(code), Signature: strings.TrimSpace(signature), Diagnostic: diagnostic.Clone(),
		Err: err, Audit: audit, RequestID: requestID, Status: status,
	}
}

func candidateRequestErrorWithDocument(err error, content []byte) error {
	var typed *CandidateRequestError
	if !errors.As(err, &typed) || typed == nil || len(content) == 0 {
		return err
	}
	switch typed.Class {
	case CandidateFailureClassSchema, CandidateFailureClassContract:
		typed.DocumentContent = append([]byte(nil), content...)
	}
	return typed
}

func candidateSchemaFailureIdentity(generation llmmodels.RuntimeCandidateGenerationResult, decodeErr error) (string, string, string) {
	reason := "candidate payload schema is invalid"
	if decodeErr != nil {
		reason = decodeErr.Error()
	}
	return "candidate_schema_invalid", "candidate-schema-invalid", reason
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type CandidateFailureClass = runtimecontract.RuntimeCandidateFailureClass

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
	if request.CandidateAttempt < 0 || request.CandidateAttempt > 1 {
		return fmt.Errorf("candidate attempt must be 0 or 1")
	}
	if request.CandidateAttempt == 0 && request.PreviousFailure != nil {
		return fmt.Errorf("initial candidate attempt must not carry a previous failure")
	}
	if request.CandidateAttempt == 1 && (request.PreviousFailure == nil || request.PreviousFailure.Class == "" || strings.TrimSpace(request.PreviousFailure.Reason) == "") {
		return fmt.Errorf("candidate retry requires the previous failure")
	}
	if strings.TrimSpace(request.StablePrefixHash) == "" {
		return fmt.Errorf("candidate request requires stable prefix hash")
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
	if request.OutputContinuation != nil {
		if err := request.OutputContinuation.Validate(); err != nil {
			return err
		}
	}
	switch request.Decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeActionProduceValidation, runtimecontract.RuntimeActionProduceAssessment, runtimecontract.RuntimeActionRequestGovernance:
		return nil
	default:
		return fmt.Errorf("action %s cannot be sent to the LLM candidate boundary", request.Decision.Action)
	}
}
