package runtimev2

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type candidateProviderFunc func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error)

func (f candidateProviderFunc) GenerateCandidate(ctx context.Context, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
	return f(ctx, request)
}

func generationResult(payload []byte, err error) (llmmodels.RuntimeCandidateGenerationResult, error) {
	if err != nil {
		return llmmodels.RuntimeCandidateGenerationResult{Status: llmmodels.RuntimeCandidateStatusFailed, FailureReason: err.Error()}, err
	}
	return llmmodels.RuntimeCandidateGenerationResult{
		Payload: payload,
		Status:  llmmodels.RuntimeCandidateStatusCompleted,
		Audit:   &llmmodels.ProviderRequestAudit{Status: llmmodels.RuntimeCandidateStatusCompleted, ProviderSubmitted: true},
	}, nil
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

func TestDecodeCandidatePayloadRejectsLegacyCandidateWrappers(t *testing.T) {
	for name, payload := range map[string]string{
		"candidate":      `{"candidate":{"operations":[{"id":"op-1","kind":"patch/apply","coverageWorkItemIds":["cwi-1"]}]}}`,
		"candidates":     `{"candidates":[{"operations":[{"id":"op-1","kind":"patch/apply","coverageWorkItemIds":["cwi-1"]}]}]}`,
		"executionGraph": `{"executionGraph":{"operations":[{"id":"op-1","kind":"patch/apply","coverageWorkItemIds":["cwi-1"]}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeCandidatePayload(runtimecontract.RuntimeActionProduceStateChange, []byte(payload)); err == nil {
				t.Fatalf("legacy %s wrapper was accepted", name)
			}
		})
	}
}

func TestDecodeCandidatePayloadPreservesRequiredOperationRef(t *testing.T) {
	candidate, err := DecodeCandidatePayload(runtimecontract.RuntimeActionProduceStateChange, []byte(`{
		"operations":[{
			"id":"git-init",
			"kind":"git/write",
			"requiredOperationRef":"repo-init",
			"coverageWorkItemIds":["cwi-1"],
			"parameters":[{"name":"operation","string":"init"}]
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if candidate.StateChange == nil || len(candidate.StateChange.Operations) != 1 {
		t.Fatalf("state-change candidate = %+v", candidate)
	}
	if got := candidate.StateChange.Operations[0].Operation.RequiredOperationRef; got != "repo-init" {
		t.Fatalf("required operation ref = %q, want repo-init", got)
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
			"parameters":[
				{"name":"targetRef","string":"main.go"},
				{"name":"patchType","string":"file_write"},
				{"name":"content","string":"package main\n"}
			]
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

func TestRequestCandidateRejectsFilePathAliasAsContractFailureBeforeDispatch(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return generationResult([]byte(`{"operations":[{"id":"patch-1","kind":"patch/apply","coverageWorkItemIds":["cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"filePath","string":"main.go"},{"name":"patchType","string":"file_write"},{"name":"content","string":"package main\n"}]}]}`), nil)
	})
	_, err := RequestCandidate(context.Background(), provider, request)
	if err == nil {
		t.Fatal("filePath alias candidate was accepted")
	}
	failure := CandidateFailureFromError(err)
	if failure.Class != CandidateFailureClassContract || !strings.Contains(failure.Reason, `does not accept parameter "filePath"`) {
		t.Fatalf("candidate failure = %+v", failure)
	}
	retry, retryErr := RetrySameContract(request, failure)
	if retryErr != nil {
		t.Fatal(retryErr)
	}
	if retry.Transaction != request.Transaction || retry.StablePrefixHash != request.StablePrefixHash || retry.PreviousFailure == nil || retry.PreviousFailure.Class != CandidateFailureClassContract {
		t.Fatalf("contract retry changed authority: initial=%+v retry=%+v", request, retry)
	}
}

func TestRequestCandidateClassifiesOutputLimitSchemaTruncation(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return llmmodels.RuntimeCandidateGenerationResult{
			Payload: []byte(`{"operations":[{"id":"patch-1"`), Status: llmmodels.RuntimeCandidateStatusCompleted,
			Usage: &llmmodels.ProviderUsage{OutputTokens: 4096, TotalTokens: 24000},
			Audit: &llmmodels.ProviderRequestAudit{
				Status: llmmodels.RuntimeCandidateStatusCompleted, ProviderSubmitted: true,
				ContextBudget: &llmmodels.ProviderContextBudget{EffectiveMaxOutputTokens: 4096},
			},
		}, nil
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err != nil {
		t.Fatal(err)
	}
	if response.OutputFragment == nil || !response.OutputFragment.ReachedProviderLimit || string(response.OutputFragment.Content) != `{"operations":[{"id":"patch-1"` {
		t.Fatalf("truncated candidate output fragment = %+v", response.OutputFragment)
	}
}

func TestRequestCandidatePreservesTypedPrerequisiteStaleWithoutCandidateFailure(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: "provider_ceiling:llm", Kind: runtimecontract.PrerequisiteKindProviderCeiling,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, ContentHash: "provider-ceiling-v1",
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
			SubjectRef: "ai-proxy/gpt-4o", Revision: "config-v1",
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: observation.PrerequisiteID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		SubjectRef: "ai-proxy/gpt-4o", ExpectedRevision: "config-v1", ObservedRevision: "config-v2",
		Reason: "provider configuration changed",
	}
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return llmmodels.RuntimeCandidateGenerationResult{
			Status: llmmodels.RuntimeCandidateStatusFailed, PrerequisiteStale: &stale,
		}, errors.New("sealed provider configuration drift")
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err == nil {
		t.Fatal("typed prerequisite stale was converted into a successful candidate")
	}
	if response.PrerequisiteStale == nil || response.PrerequisiteStale.ObservationID != observation.ObservationID || response.Candidate.StateChange != nil {
		t.Fatalf("candidate response lost stale boundary: %+v", response)
	}
	if extracted := CandidatePrerequisiteStaleFromError(err); extracted == nil || extracted.ObservedRevision != "config-v2" {
		t.Fatalf("candidate error did not retain typed stale: %v", err)
	}
}

func TestCandidateRetryIsExactlyOnceAndKeepsSealedContract(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedValidation)
	failure := runtimecontract.RuntimeCandidateFailure{Class: CandidateFailureClassSchema, Reason: `json: unknown field "candidate"`}
	retry, err := RetrySameContract(request, failure)
	if err != nil {
		t.Fatal(err)
	}
	if retry.CandidateAttempt != 1 || retry.PreviousFailure == nil || !reflect.DeepEqual(*retry.PreviousFailure, failure) || retry.Transaction != request.Transaction || retry.StablePrefixHash != request.StablePrefixHash || retry.Decision.AuthoritySnapshot.RequiredOutputHash != request.Decision.AuthoritySnapshot.RequiredOutputHash {
		t.Fatalf("retry changed sealed request: initial=%+v retry=%+v", request, retry)
	}
	if _, err := RetrySameContract(retry, failure); err == nil {
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
	retry, err := RetrySameContract(request, runtimecontract.RuntimeCandidateFailure{Class: CandidateFailureClassSchema, Reason: first.Reason})
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
	provider := candidateProviderFunc(func(_ context.Context, got llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		if got.Transaction != request.Transaction || got.Decision.AuthoritySnapshot.ID != request.Decision.AuthoritySnapshot.ID || got.Decision.RequiredOutput.Kind != runtimecontract.RequiredOutputKindValidation {
			t.Fatalf("provider request changed sealed authority: %+v", got)
		}
		return generationResult([]byte(`{"operations":[{"id":"validate-1","kind":"validation/managed_session","coverageWorkItemIds":["cwi-1"],"parameters":[{"name":"command","string":"true"}]}]}`), nil)
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
		"disposition":"satisfied",
		"items":[{
			"coverageWorkItemId":"cwi-1",
			"satisfied":true,
			"evidenceRefs":["evidence-1"]
		}],
		"summary":"current focus is satisfied",
		"evidenceRefs":["evidence-1"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := candidate.Validate(request.Decision); err != nil {
		t.Fatal(err)
	}
	if candidate.Assessment == nil || candidate.Assessment.Disposition != runtimecontract.AssessmentDispositionSatisfied || candidate.Assessment.FocusID != "focus-1" {
		t.Fatalf("assessment candidate = %+v", candidate)
	}
	if len(candidate.Assessment.Items) != 1 || candidate.Assessment.Items[0].CoverageWorkItemID != "cwi-1" || !candidate.Assessment.Items[0].Satisfied || len(candidate.Assessment.Items[0].EvidenceRefs) != 1 || candidate.Assessment.Items[0].EvidenceRefs[0] != "evidence-1" {
		t.Fatalf("assessment item = %+v", candidate.Assessment.Items)
	}
}

func TestDecodeTypedAssessmentCandidateRejectsLegacyTopLevelSatisfied(t *testing.T) {
	_, err := DecodeCandidatePayload(runtimecontract.RuntimeActionProduceAssessment, []byte(`{
		"focusId":"focus-1",
		"coverageWorkItemIds":["cwi-1"],
		"disposition":"satisfied",
		"satisfied":true,
		"summary":"current focus is satisfied",
		"evidenceRefs":["evidence-1"]
	}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "satisfied"`) {
		t.Fatalf("legacy top-level satisfied error = %v", err)
	}
}

func candidateRequest(t *testing.T, state runtimecontract.ConvergenceState) llmmodels.RuntimeCandidateRequest {
	t.Helper()
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus:      runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
		Coverage:   []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Activation: runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
	}
	decision := runtimecontract.RuntimeDecision{State: state, Focus: facts.Focus}
	switch state {
	case runtimecontract.ConvergenceStateNeedStateChange:
		decision.Action = runtimecontract.RuntimeActionProduceStateChange
		decision.RequiredOutput = runtimecontract.RequiredOutputContract{
			Kind: runtimecontract.RequiredOutputKindStateChange, AcceptedKinds: []string{"patch/apply"},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
		}
	case runtimecontract.ConvergenceStateNeedValidation:
		decision.Action = runtimecontract.RuntimeActionProduceValidation
		decision.RequiredOutput = runtimecontract.RequiredOutputContract{
			Kind: runtimecontract.RequiredOutputKindValidation, AcceptedKinds: []string{"validation/managed_session"},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
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
		CandidateContext:  runtimecontract.RuntimeCandidateContextForDecision(facts, sealed, nil),
	}
}
