package runtimev2

import (
	"fmt"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// These fixtures deliberately live in _test.go. Candidate admission and retry
// ownership belongs to Orchestrator; the LLM Block has no production decision
// helper for retry, governance or terminal routing.
type CandidateDisposition string

const (
	CandidateDispositionAccepted          CandidateDisposition = "accepted"
	CandidateDispositionRetrySameContract CandidateDisposition = "retry_same_contract"
	CandidateDispositionRequestGovernance CandidateDisposition = "request_governance"
	CandidateDispositionTerminal          CandidateDisposition = "terminal"
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
