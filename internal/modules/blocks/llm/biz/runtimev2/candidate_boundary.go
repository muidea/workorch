package runtimev2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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
	candidate, err := DecodeCandidatePayload(request.Decision.Action, generation.Payload)
	if err != nil {
		if candidatePayloadReachedOutputLimit(generation, err) {
			fragment := runtimecontract.RuntimeCandidateOutputFragment{Content: append([]byte(nil), generation.Payload...), ReachedProviderLimit: true}
			return llmmodels.RuntimeCandidateResponse{Transaction: request.Transaction, OutputFragment: &fragment, Audit: generation.Audit, Usage: generation.Usage, RequestID: generation.RequestID, Status: firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusCompleted)}, nil
		}
		code, signature, reason := candidateSchemaFailureIdentity(generation, err)
		return llmmodels.RuntimeCandidateResponse{
			Transaction: request.Transaction,
			Audit:       generation.Audit,
			Usage:       generation.Usage,
			RequestID:   generation.RequestID,
			Status:      firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusCompleted),
		}, candidateRequestErrorWithIdentity(CandidateFailureClassSchema, code, signature, errors.New(reason), generation.Audit, generation.RequestID, generation.Status)
	}
	if err := candidate.ValidateWithActivation(request.Decision, request.CandidateContext.Activation); err != nil {
		return llmmodels.RuntimeCandidateResponse{
			Transaction: request.Transaction,
			Audit:       generation.Audit,
			Usage:       generation.Usage,
			RequestID:   generation.RequestID,
			Status:      firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusCompleted),
		}, candidateRequestError(CandidateFailureClassContract, err, generation.Audit, generation.RequestID, generation.Status)
	}
	return llmmodels.RuntimeCandidateResponse{
		Transaction: request.Transaction,
		Candidate:   candidate,
		Audit:       generation.Audit,
		Usage:       generation.Usage,
		RequestID:   generation.RequestID,
		Status:      firstNonEmpty(generation.Status, llmmodels.RuntimeCandidateStatusCompleted),
	}, nil
}

type CandidateRequestError struct {
	Class     CandidateFailureClass
	Code      string
	Signature string
	Err       error
	Audit     *llmmodels.ProviderRequestAudit
	RequestID string
	Status    string
	Stale     *runtimecontract.RuntimePrerequisiteStale
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

func candidateRequestError(class CandidateFailureClass, err error, audit *llmmodels.ProviderRequestAudit, requestID, status string) error {
	return &CandidateRequestError{Class: class, Err: err, Audit: audit, RequestID: requestID, Status: status}
}

func candidateRequestErrorWithIdentity(class CandidateFailureClass, code, signature string, err error, audit *llmmodels.ProviderRequestAudit, requestID, status string) error {
	return &CandidateRequestError{
		Class: class, Code: strings.TrimSpace(code), Signature: strings.TrimSpace(signature),
		Err: err, Audit: audit, RequestID: requestID, Status: status,
	}
}

func candidateSchemaFailureIdentity(generation llmmodels.RuntimeCandidateGenerationResult, decodeErr error) (string, string, string) {
	reason := "candidate payload schema is invalid"
	if decodeErr != nil {
		reason = decodeErr.Error()
	}
	if candidatePayloadReachedOutputLimit(generation, decodeErr) {
		return "candidate_response_truncated", "candidate-response-truncated", "provider response reached the sealed output token limit before candidate JSON completed: " + reason
	}
	return "candidate_schema_invalid", "candidate-schema-invalid", reason
}

func candidatePayloadReachedOutputLimit(generation llmmodels.RuntimeCandidateGenerationResult, decodeErr error) bool {
	if decodeErr == nil || (!errors.Is(decodeErr, io.ErrUnexpectedEOF) && !strings.Contains(strings.ToLower(decodeErr.Error()), "unexpected eof")) ||
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

type CandidateDisposition string

const (
	CandidateDispositionAccepted          CandidateDisposition = "accepted"
	CandidateDispositionRetrySameContract CandidateDisposition = "retry_same_contract"
	CandidateDispositionRequestGovernance CandidateDisposition = "request_governance"
	CandidateDispositionTerminal          CandidateDisposition = "terminal"
)

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

type CandidateAttemptOutcome struct {
	Disposition  CandidateDisposition
	FailureClass CandidateFailureClass
	Reason       string
}

func ValidateCandidateAttempt(request llmmodels.RuntimeCandidateRequest, response llmmodels.RuntimeCandidateResponse) CandidateAttemptOutcome {
	if err := validateCandidateRequest(request); err != nil {
		return CandidateAttemptOutcome{Disposition: CandidateDispositionTerminal, FailureClass: CandidateFailureClassInternal, Reason: err.Error()}
	}
	if err := runtimecontract.ValidateAuthorityTransaction(request.Decision, response.Transaction); err != nil {
		return CandidateAttemptOutcome{Disposition: CandidateDispositionTerminal, FailureClass: CandidateFailureClassInternal, Reason: err.Error()}
	}
	if err := response.Candidate.ValidateWithActivation(request.Decision, request.CandidateContext.Activation); err != nil {
		if request.CandidateAttempt == 0 {
			return CandidateAttemptOutcome{Disposition: CandidateDispositionRetrySameContract, FailureClass: CandidateFailureClassContract, Reason: err.Error()}
		}
		return CandidateAttemptOutcome{Disposition: CandidateDispositionRequestGovernance, FailureClass: CandidateFailureClassContract, Reason: err.Error()}
	}
	return CandidateAttemptOutcome{Disposition: CandidateDispositionAccepted}
}

func CandidateFailureOutcome(request llmmodels.RuntimeCandidateRequest, class CandidateFailureClass, err error) CandidateAttemptOutcome {
	if requestErr := validateCandidateRequest(request); requestErr != nil {
		return CandidateAttemptOutcome{Disposition: CandidateDispositionTerminal, FailureClass: CandidateFailureClassInternal, Reason: requestErr.Error()}
	}
	reason := "candidate request failed"
	if err != nil {
		reason = err.Error()
	}
	if class == CandidateFailureClassInternal {
		return CandidateAttemptOutcome{Disposition: CandidateDispositionTerminal, FailureClass: class, Reason: reason}
	}
	if request.CandidateAttempt == 0 {
		return CandidateAttemptOutcome{Disposition: CandidateDispositionRetrySameContract, FailureClass: class, Reason: reason}
	}
	return CandidateAttemptOutcome{Disposition: CandidateDispositionRequestGovernance, FailureClass: class, Reason: reason}
}

func RetrySameContract(request llmmodels.RuntimeCandidateRequest, failure runtimecontract.RuntimeCandidateFailure) (llmmodels.RuntimeCandidateRequest, error) {
	if err := validateCandidateRequest(request); err != nil {
		return llmmodels.RuntimeCandidateRequest{}, err
	}
	if request.CandidateAttempt != 0 {
		return llmmodels.RuntimeCandidateRequest{}, fmt.Errorf("same-contract candidate retry is already exhausted")
	}
	if failure.Class == "" || strings.TrimSpace(failure.Reason) == "" {
		return llmmodels.RuntimeCandidateRequest{}, fmt.Errorf("same-contract candidate retry requires the previous failure")
	}
	retry := request
	retry.CandidateAttempt = 1
	retry.PreviousFailure = &failure
	if retry.Transaction != request.Transaction || retry.StablePrefixHash != request.StablePrefixHash || retry.Decision.AuthoritySnapshot.ID != request.Decision.AuthoritySnapshot.ID || retry.Decision.AuthoritySnapshot.RequiredOutputHash != request.Decision.AuthoritySnapshot.RequiredOutputHash {
		return llmmodels.RuntimeCandidateRequest{}, fmt.Errorf("same-contract retry changed sealed authority")
	}
	return retry, nil
}

func DecodeCandidatePayload(action runtimecontract.RuntimeAction, payload []byte) (runtimecontract.RuntimeCandidate, error) {
	switch action {
	case runtimecontract.RuntimeActionProduceStateChange:
		var value operationCandidateDTO
		if err := strictDecode(payload, &value); err != nil {
			return runtimecontract.RuntimeCandidate{}, err
		}
		operations, err := value.stateChangingOperations()
		if err != nil {
			return runtimecontract.RuntimeCandidate{}, err
		}
		return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: operations}}, nil
	case runtimecontract.RuntimeActionProduceValidation:
		var value operationCandidateDTO
		if err := strictDecode(payload, &value); err != nil {
			return runtimecontract.RuntimeCandidate{}, err
		}
		operations, err := value.validationOperations()
		if err != nil {
			return runtimecontract.RuntimeCandidate{}, err
		}
		return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: operations}}, nil
	case runtimecontract.RuntimeActionProduceAssessment:
		var value assessmentCandidateDTO
		if err := strictDecode(payload, &value); err != nil {
			return runtimecontract.RuntimeCandidate{}, err
		}
		items := make([]runtimecontract.RuntimeAssessmentItemFact, 0, len(value.Items))
		for _, item := range value.Items {
			items = append(items, runtimecontract.RuntimeAssessmentItemFact{
				CoverageWorkItemID: item.CoverageWorkItemID, Satisfied: item.Satisfied, Reason: item.Reason,
				EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
			})
		}
		var gap *runtimecontract.RuntimeAssessmentGap
		if value.Gap != nil {
			gap = &runtimecontract.RuntimeAssessmentGap{
				CoverageWorkItemIDs:  append([]common.CoverageWorkItemID(nil), value.Gap.CoverageWorkItemIDs...),
				MissingEvidenceKinds: append([]string(nil), value.Gap.MissingEvidenceKinds...),
				TargetRefs:           append([]string(nil), value.Gap.TargetRefs...),
				EvidenceRefs:         append([]string(nil), value.Gap.EvidenceRefs...),
				Reason:               value.Gap.Reason, FailureClass: runtimecontract.FailureClass(value.Gap.FailureClass),
				FailureCode: value.Gap.FailureCode, FailureSignature: value.Gap.FailureSignature,
			}
		}
		return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: value.FocusID, SliceID: value.SliceID, CoverageWorkItemIDs: value.CoverageWorkItemIDs,
			Disposition: runtimecontract.AssessmentDisposition(value.Disposition),
			Items:       items, Gap: gap, Summary: value.Summary, EvidenceRefs: value.EvidenceRefs, ArtifactRefs: value.ArtifactRefs,
		}}, nil
	case runtimecontract.RuntimeActionRequestGovernance:
		var value governanceCandidateDTO
		if err := strictDecode(payload, &value); err != nil {
			return runtimecontract.RuntimeCandidate{}, err
		}
		return runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
			Kind: value.ExternalDependency.Kind, SubjectRef: value.ExternalDependency.SubjectRef, Reason: value.ExternalDependency.Reason,
		}}}, nil
	default:
		return runtimecontract.RuntimeCandidate{}, fmt.Errorf("action %s does not accept an LLM candidate", action)
	}
}

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

func strictDecode(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("candidate payload contains multiple JSON values")
		}
		return err
	}
	return nil
}

type operationCandidateDTO struct {
	Operations []operationDTO `json:"operations"`
}

type operationDTO struct {
	ID                   string                      `json:"id"`
	Kind                 string                      `json:"kind"`
	RequiredOperationRef string                      `json:"requiredOperationRef,omitempty"`
	DependsOn            []string                    `json:"dependsOn,omitempty"`
	CoverageWorkItemIDs  []common.CoverageWorkItemID `json:"coverageWorkItemIds,omitempty"`
	TargetRefs           []string                    `json:"targetRefs,omitempty"`
	Parameters           []operationParameterDTO     `json:"parameters,omitempty"`
}

type operationParameterDTO struct {
	Name       string   `json:"name"`
	String     *string  `json:"string,omitempty"`
	Integer    *int64   `json:"integer,omitempty"`
	Boolean    *bool    `json:"boolean,omitempty"`
	StringList []string `json:"stringList,omitempty"`
}

func (d operationCandidateDTO) stateChangingOperations() ([]runtimecontract.StateChangingOperation, error) {
	operations, err := d.runtimeOperations()
	if err != nil {
		return nil, err
	}
	ret := make([]runtimecontract.StateChangingOperation, 0, len(operations))
	for _, operation := range operations {
		ret = append(ret, runtimecontract.StateChangingOperation{Operation: operation})
	}
	return ret, nil
}

func (d operationCandidateDTO) validationOperations() ([]runtimecontract.ValidationOperation, error) {
	operations, err := d.runtimeOperations()
	if err != nil {
		return nil, err
	}
	ret := make([]runtimecontract.ValidationOperation, 0, len(operations))
	for _, operation := range operations {
		ret = append(ret, runtimecontract.ValidationOperation{Operation: operation})
	}
	return ret, nil
}

func (d operationCandidateDTO) runtimeOperations() ([]runtimecontract.RuntimeOperation, error) {
	ret := make([]runtimecontract.RuntimeOperation, 0, len(d.Operations))
	for _, value := range d.Operations {
		parameters := make([]runtimecontract.OperationParameter, 0, len(value.Parameters))
		for _, parameter := range value.Parameters {
			parameters = append(parameters, runtimecontract.OperationParameter{
				Name: parameter.Name, String: parameter.String, Integer: parameter.Integer, Boolean: parameter.Boolean, StringList: parameter.StringList,
			})
		}
		ret = append(ret, runtimecontract.RuntimeOperation{
			ID: value.ID, Kind: value.Kind, RequiredOperationRef: value.RequiredOperationRef, DependsOn: value.DependsOn,
			CoverageWorkItemIDs: value.CoverageWorkItemIDs, TargetRefs: value.TargetRefs, Parameters: parameters,
		})
	}
	return ret, nil
}

type governanceCandidateDTO struct {
	ExternalDependency externalDependencyDTO `json:"externalDependency"`
}

type assessmentCandidateDTO struct {
	FocusID             string                      `json:"focusId"`
	SliceID             string                      `json:"sliceId,omitempty"`
	CoverageWorkItemIDs []common.CoverageWorkItemID `json:"coverageWorkItemIds"`
	Disposition         string                      `json:"disposition,omitempty"`
	Items               []assessmentItemDTO         `json:"items,omitempty"`
	Gap                 *assessmentGapDTO           `json:"gap,omitempty"`
	Summary             string                      `json:"summary"`
	EvidenceRefs        []string                    `json:"evidenceRefs,omitempty"`
	ArtifactRefs        []string                    `json:"artifactRefs,omitempty"`
}

type assessmentItemDTO struct {
	CoverageWorkItemID common.CoverageWorkItemID `json:"coverageWorkItemId"`
	Satisfied          bool                      `json:"satisfied"`
	Reason             string                    `json:"reason,omitempty"`
	EvidenceRefs       []string                  `json:"evidenceRefs,omitempty"`
}

type assessmentGapDTO struct {
	CoverageWorkItemIDs  []common.CoverageWorkItemID `json:"coverageWorkItemIds"`
	MissingEvidenceKinds []string                    `json:"missingEvidenceKinds,omitempty"`
	TargetRefs           []string                    `json:"targetRefs,omitempty"`
	EvidenceRefs         []string                    `json:"evidenceRefs,omitempty"`
	Reason               string                      `json:"reason,omitempty"`
	FailureClass         string                      `json:"failureClass,omitempty"`
	FailureCode          string                      `json:"failureCode,omitempty"`
	FailureSignature     string                      `json:"failureSignature,omitempty"`
}

type externalDependencyDTO struct {
	Kind       string `json:"kind"`
	SubjectRef string `json:"subjectRef,omitempty"`
	Reason     string `json:"reason"`
}
