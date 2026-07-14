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

type CandidateProvider interface {
	GenerateCandidate(context.Context, llmmodels.RuntimeCandidateRequest) ([]byte, error)
}

func RequestCandidate(ctx context.Context, provider CandidateProvider, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateResponse, error) {
	if provider == nil {
		return llmmodels.RuntimeCandidateResponse{}, fmt.Errorf("candidate provider is required")
	}
	if err := validateCandidateRequest(request); err != nil {
		return llmmodels.RuntimeCandidateResponse{}, candidateRequestError(CandidateFailureClassInternal, err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := provider.GenerateCandidate(ctx, request)
	if err != nil {
		return llmmodels.RuntimeCandidateResponse{}, candidateRequestError(CandidateFailureClassProvider, err)
	}
	candidate, err := DecodeCandidatePayload(request.Decision.Action, payload)
	if err != nil {
		return llmmodels.RuntimeCandidateResponse{}, candidateRequestError(CandidateFailureClassSchema, err)
	}
	if err := candidate.Validate(request.Decision); err != nil {
		return llmmodels.RuntimeCandidateResponse{}, candidateRequestError(CandidateFailureClassContract, err)
	}
	return llmmodels.RuntimeCandidateResponse{Transaction: request.Transaction, Candidate: candidate}, nil
}

type CandidateRequestError struct {
	Class CandidateFailureClass
	Err   error
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
	}
	return failure
}

func candidateRequestError(class CandidateFailureClass, err error) error {
	return &CandidateRequestError{Class: class, Err: err}
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
	CandidateFailureClassSchema   = runtimecontract.RuntimeCandidateFailureClassSchema
	CandidateFailureClassContract = runtimecontract.RuntimeCandidateFailureClassContract
	CandidateFailureClassProvider = runtimecontract.RuntimeCandidateFailureClassProvider
	CandidateFailureClassInternal = runtimecontract.RuntimeCandidateFailureClassInternal
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
	if err := response.Candidate.Validate(request.Decision); err != nil {
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
		return runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: value.FocusID, CoverageWorkItemIDs: value.CoverageWorkItemIDs, Satisfied: value.Satisfied,
			Summary: value.Summary, EvidenceRefs: value.EvidenceRefs, ArtifactRefs: value.ArtifactRefs,
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
	ID                  string                      `json:"id"`
	Kind                string                      `json:"kind"`
	DependsOn           []string                    `json:"dependsOn,omitempty"`
	CoverageWorkItemIDs []common.CoverageWorkItemID `json:"coverageWorkItemIds,omitempty"`
	TargetRefs          []string                    `json:"targetRefs,omitempty"`
	Parameters          []operationParameterDTO     `json:"parameters,omitempty"`
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
			ID: value.ID, Kind: value.Kind, DependsOn: value.DependsOn, CoverageWorkItemIDs: value.CoverageWorkItemIDs, TargetRefs: value.TargetRefs, Parameters: parameters,
		})
	}
	return ret, nil
}

type governanceCandidateDTO struct {
	ExternalDependency externalDependencyDTO `json:"externalDependency"`
}

type assessmentCandidateDTO struct {
	FocusID             string                      `json:"focusId"`
	CoverageWorkItemIDs []common.CoverageWorkItemID `json:"coverageWorkItemIds"`
	Satisfied           bool                        `json:"satisfied"`
	Summary             string                      `json:"summary"`
	EvidenceRefs        []string                    `json:"evidenceRefs,omitempty"`
	ArtifactRefs        []string                    `json:"artifactRefs,omitempty"`
}

type externalDependencyDTO struct {
	Kind       string `json:"kind"`
	SubjectRef string `json:"subjectRef,omitempty"`
	Reason     string `json:"reason"`
}
