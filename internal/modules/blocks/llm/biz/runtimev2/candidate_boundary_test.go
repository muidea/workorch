package runtimev2

import (
	"context"
	"errors"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type candidateProviderFunc func(context.Context, llmmodels.RuntimeCandidateRequest) ([]byte, error)

func (f candidateProviderFunc) GenerateCandidate(ctx context.Context, request llmmodels.RuntimeCandidateRequest) ([]byte, error) {
	return f(ctx, request)
}

func TestDecodeCandidatePayloadRejectsAuthorityFields(t *testing.T) {
	_, err := DecodeCandidatePayload(runtimecontract.RuntimeActionProduceStateChange, []byte(`{
		"operations":[{"id":"op-1","kind":"patch/apply","coverageWorkItemIds":["cwi-1"]}],
		"convergenceState":"complete"
	}`))
	if err == nil {
		t.Fatal("candidate payload changed convergence authority")
	}
}

func TestValidateCandidateAttemptAcceptsOnlyCurrentActionCandidate(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	candidate, err := DecodeCandidatePayload(request.Decision.Action, []byte(`{
		"operations":[{
			"id":"op-1",
			"kind":"patch/apply",
			"coverageWorkItemIds":["cwi-1"],
			"targetRefs":["main.go"],
			"parameters":[{"name":"targetRef","string":"main.go"}]
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	outcome := ValidateCandidateAttempt(request, llmmodels.RuntimeCandidateResponse{Transaction: request.Transaction, Candidate: candidate})
	if outcome.Disposition != CandidateDispositionAccepted {
		t.Fatalf("outcome = %+v, want accepted", outcome)
	}

	wrong := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate-1", Kind: "validation/managed_session", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}}}}}
	outcome = ValidateCandidateAttempt(request, llmmodels.RuntimeCandidateResponse{Transaction: request.Transaction, Candidate: wrong})
	if outcome.Disposition != CandidateDispositionRetrySameContract {
		t.Fatalf("wrong action outcome = %+v, want same-contract retry", outcome)
	}
}

func TestCandidateRetryIsExactlyOnceAndKeepsSealedContract(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedValidation)
	retry, err := RetrySameContract(request)
	if err != nil {
		t.Fatal(err)
	}
	if retry.CandidateAttempt != 1 || retry.Transaction != request.Transaction || retry.StablePrefixHash != request.StablePrefixHash || retry.Decision.AuthoritySnapshot.RequiredOutputHash != request.Decision.AuthoritySnapshot.RequiredOutputHash {
		t.Fatalf("retry changed sealed request: initial=%+v retry=%+v", request, retry)
	}
	if _, err := RetrySameContract(retry); err == nil {
		t.Fatal("third candidate request was allowed")
	}

	invalid := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{}}
	outcome := ValidateCandidateAttempt(retry, llmmodels.RuntimeCandidateResponse{Transaction: retry.Transaction, Candidate: invalid})
	if outcome.Disposition != CandidateDispositionRequestGovernance {
		t.Fatalf("second invalid outcome = %+v, want governed wait", outcome)
	}
}

func TestCandidateFailuresDoNotSwitchAuthorityAction(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedValidation)
	first := CandidateFailureOutcome(request, CandidateFailureClassSchema, errors.New("invalid candidate schema"))
	if first.Disposition != CandidateDispositionRetrySameContract || request.Decision.Action != runtimecontract.RuntimeActionProduceValidation {
		t.Fatalf("first failure = %+v request=%+v", first, request)
	}
	retry, err := RetrySameContract(request)
	if err != nil {
		t.Fatal(err)
	}
	second := CandidateFailureOutcome(retry, CandidateFailureClassProvider, errors.New("provider failed"))
	if second.Disposition != CandidateDispositionRequestGovernance || retry.Decision.Action != runtimecontract.RuntimeActionProduceValidation || retry.Transaction != request.Transaction {
		t.Fatalf("second failure changed authority: outcome=%+v retry=%+v", second, retry)
	}
}

func TestCandidateTransactionMismatchIsTerminal(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	response := llmmodels.RuntimeCandidateResponse{Transaction: request.Transaction}
	response.Transaction.FocusID = "stale-focus"
	outcome := ValidateCandidateAttempt(request, response)
	if outcome.Disposition != CandidateDispositionTerminal || outcome.FailureClass != CandidateFailureClassInternal {
		t.Fatalf("outcome = %+v, want terminal internal mismatch", outcome)
	}
}

func TestCandidateRequestRejectsWeakenedGoalDoDContext(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedValidation)
	request.DefinitionContext.DoDDefinition = ""
	outcome := CandidateFailureOutcome(request, CandidateFailureClassSchema, errors.New("schema failure"))
	if outcome.Disposition != CandidateDispositionTerminal || outcome.FailureClass != CandidateFailureClassInternal {
		t.Fatalf("outcome = %+v, want terminal incomplete definition contract", outcome)
	}
}

func TestRequestCandidateProviderReceivesOnlySealedTypedRequest(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedValidation)
	provider := candidateProviderFunc(func(_ context.Context, got llmmodels.RuntimeCandidateRequest) ([]byte, error) {
		if got.Transaction != request.Transaction || got.Decision.AuthoritySnapshot.ID != request.Decision.AuthoritySnapshot.ID || got.Decision.RequiredOutput.Kind != runtimecontract.RequiredOutputKindValidation {
			t.Fatalf("provider request changed sealed authority: %+v", got)
		}
		return []byte(`{"operations":[{"id":"validate-1","kind":"validation/managed_session","coverageWorkItemIds":["cwi-1"]}]}`), nil
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := ValidateCandidateAttempt(request, response); outcome.Disposition != CandidateDispositionAccepted {
		t.Fatalf("candidate outcome = %+v", outcome)
	}
}

func TestDecodeTypedAssessmentCandidate(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedAssessment)
	candidate, err := DecodeCandidatePayload(request.Decision.Action, []byte(`{
		"focusId":"focus-1",
		"coverageWorkItemIds":["cwi-1"],
		"satisfied":true,
		"summary":"current focus is satisfied",
		"evidenceRefs":["evidence-1"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := candidate.Validate(request.Decision); err != nil {
		t.Fatal(err)
	}
	if candidate.Assessment == nil || !candidate.Assessment.Satisfied || candidate.Assessment.FocusID != "focus-1" {
		t.Fatalf("assessment candidate = %+v", candidate)
	}
}

func candidateRequest(t *testing.T, state runtimecontract.ConvergenceState) llmmodels.RuntimeCandidateRequest {
	t.Helper()
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
	}
	decision := runtimecontract.RuntimeDecision{State: state, Focus: facts.Focus}
	switch state {
	case runtimecontract.ConvergenceStateNeedStateChange:
		decision.Action = runtimecontract.RuntimeActionProduceStateChange
		decision.RequiredOutput = runtimecontract.RequiredOutputContract{
			Kind: runtimecontract.RequiredOutputKindStateChange, AcceptedKinds: []string{"patch/apply"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		}
	case runtimecontract.ConvergenceStateNeedValidation:
		decision.Action = runtimecontract.RuntimeActionProduceValidation
		decision.RequiredOutput = runtimecontract.RequiredOutputContract{
			Kind: runtimecontract.RequiredOutputKindValidation, AcceptedKinds: []string{"validation/managed_session"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		}
	case runtimecontract.ConvergenceStateNeedAssessment:
		decision.Action = runtimecontract.RuntimeActionProduceAssessment
		decision.RequiredOutput = runtimecontract.RequiredOutputContract{
			Kind: runtimecontract.RequiredOutputKindAssessment, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		}
	default:
		t.Fatalf("unsupported test state %s", state)
	}
	sealed, err := runtimecontract.SealRuntimeDecision(facts, decision, 0)
	if err != nil {
		t.Fatal(err)
	}
	return llmmodels.RuntimeCandidateRequest{
		Decision: sealed, Transaction: sealed.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply", "validation/managed_session"}},
		CandidateContext:  runtimecontract.RuntimeCandidateContextFromFacts(facts),
	}
}
