package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
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

func TestRequestCandidatePreservesSemanticDocumentKindThroughSingleBoundary(t *testing.T) {
	request := semanticBoundaryRequest(t)
	providerCalls := 0
	response, err := RequestCandidate(context.Background(), candidateProviderFunc(func(_ context.Context, observed llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		providerCalls++
		if observed.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan {
			t.Fatalf("provider document kind = %q", observed.DocumentKind)
		}
		return generationResult([]byte(`{"documentKind":"semantic_plan"}`), nil)
	}), request)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || response.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan || string(response.DocumentContent) != `{"documentKind":"semantic_plan"}` {
		t.Fatalf("semantic response=%+v providerCalls=%d", response, providerCalls)
	}
	request.DocumentKind = ""
	if _, err := RequestCandidate(context.Background(), candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		providerCalls++
		return generationResult(nil, nil)
	}), request); err == nil || !strings.Contains(err.Error(), "explicit document kind") {
		t.Fatalf("missing document kind error = %v", err)
	}
	if providerCalls != 1 {
		t.Fatalf("invalid document kind reached provider: calls=%d", providerCalls)
	}
}

func semanticBoundaryRequest(t *testing.T) llmmodels.RuntimeCandidateRequest {
	t.Helper()
	definition := runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
		CapabilityContract: []string{runtimecontract.StepKindFileWrite},
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}
	activation := runtimecontract.RuntimeActivationContext{ProviderTarget: target, ExecutionContract: runtimecontract.RuntimeExecutionContract{OperationContractRevision: runtimecontract.RuntimeOperationContractRevision()}}
	items := []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "works", Order: 1}}
	slices := []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}}
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, activation, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"main.go"}, SelectorRefs: []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path"}, ObservedRevision: "file-revision-1"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent, WorkspaceID: "ws-1", RunID: "run-1",
		DefinitionRevision: definition.Revision, DefinitionSeedHash: seed.Hash, OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems: items, AllowedOwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		OwnerCatalogs:         []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	context := runtimecontract.RuntimeSemanticPlanRequestContext{
		WorkspaceID: "ws-1", RunID: "run-1", Definition: definition, DefinitionSeed: seed, Activation: activation,
		ProviderCeiling: &runtimecontract.ProviderCeiling{Target: target, ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048, MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1"},
	}
	return llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan,
		RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{
			LogicalRequestID: "semantic-request-1", Sequence: 1, PhysicalCallID: "semantic-call-1",
		},
		StablePrefixHash: runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, activation.ExecutionContract),
		SemanticManifest: manifest, SemanticContext: context,
	}
}

func TestDecodeCandidatePayloadRejectsAuthorityFields(t *testing.T) {
	_, err := runtimecontract.DecodeRuntimeCandidatePayload(runtimecontract.ConvergenceStateNeedStateChange, []byte(`{
		"operations":[{"id":"op-1","kind":"patch/apply","bindingRefs":["binding/test/cwi-1"]}],
		"convergenceState":"complete"
	}`))
	if err == nil {
		t.Fatal("candidate payload changed convergence authority")
	}
}

func TestDecodeCandidatePayloadRejectsLegacyCandidateWrappers(t *testing.T) {
	for name, payload := range map[string]string{
		"candidate":      `{"candidate":{"operations":[{"id":"op-1","kind":"patch/apply","bindingRefs":["binding/test/cwi-1"]}]}}`,
		"candidates":     `{"candidates":[{"operations":[{"id":"op-1","kind":"patch/apply","bindingRefs":["binding/test/cwi-1"]}]}]}`,
		"executionGraph": `{"executionGraph":{"operations":[{"id":"op-1","kind":"patch/apply","bindingRefs":["binding/test/cwi-1"]}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := runtimecontract.DecodeRuntimeCandidatePayload(runtimecontract.ConvergenceStateNeedStateChange, []byte(payload)); err == nil {
				t.Fatalf("legacy %s wrapper was accepted", name)
			}
		})
	}
}

func TestDecodeCandidatePayloadRejectsRawCoverageWorkItemIDs(t *testing.T) {
	_, err := runtimecontract.DecodeRuntimeCandidatePayload(runtimecontract.ConvergenceStateNeedStateChange, []byte(`{
		"operations":[{"id":"op-1","kind":"patch/apply","coverageWorkItemIds":["cwi-1"]}]
	}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "coverageWorkItemIds"`) {
		t.Fatalf("raw candidate coverage was accepted: %v", err)
	}
}

func TestDecodeCandidatePayloadPreservesRequiredOperationRef(t *testing.T) {
	candidate, err := runtimecontract.DecodeRuntimeCandidatePayload(runtimecontract.ConvergenceStateNeedStateChange, []byte(`{
		"operations":[{
			"id":"git-init",
			"kind":"git/write",
			"requiredOperationRef":"repo-init",
			"dependsOn":["workspace-init"],
			"bindingRefs":["binding/test/cwi-1"],
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
	if got := candidate.StateChange.Operations[0].Operation.DependsOn; !reflect.DeepEqual(got, []string{"workspace-init"}) {
		t.Fatalf("operation dependency = %#v, want workspace-init", got)
	}
}

func TestRequestCandidateForwardsCompleteFilePathAliasDocument(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	payload := []byte(`{"operations":[{"id":"patch-1","kind":"patch/apply","bindingRefs":["binding/test/cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"filePath","string":"main.go"},{"name":"patchType","string":"file_write"},{"name":"content","string":"package main\n"}]}]}`)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return generationResult(payload, nil)
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err != nil || response.OutputFragment != nil || string(response.DocumentContent) != string(payload) {
		t.Fatalf("complete alias document was not forwarded unchanged: response=%+v err=%v", response, err)
	}
}

func TestRequestCandidateForwardsCompleteSchemaInvalidDocuments(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	validOperation := `{"id":"patch-1","kind":"patch/apply","bindingRefs":["binding/test/cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main\n"}]}`
	cases := []struct {
		name    string
		payload string
	}{
		{name: "unknown top level", payload: `{"operations":[` + validOperation + `],"unexpected":true}`},
		{name: "missing required operations", payload: `{}`},
		{name: "wrong operations container", payload: `{"operations":{}}`},
		{name: "wrong target refs container", payload: `{"operations":[{"id":"patch-1","kind":"patch/apply","bindingRefs":["binding/test/cwi-1"],"targetRefs":"main.go","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main\n"}]}]}`},
		{name: "trailing json", payload: `{"operations":[` + validOperation + `]} {}`},
		{name: "field alias", payload: `{"operations":[{"id":"patch-1","kind":"patch/apply","bindingRefs":["binding/test/cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"filePath","string":"main.go"},{"name":"patchType","string":"file_write"},{"name":"content","string":"package main\n"}]}]}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			calls := 0
			provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
				calls++
				return generationResult([]byte(testCase.payload), nil)
			})
			response, err := RequestCandidate(context.Background(), provider, request)
			if err != nil || calls != 1 || response.OutputFragment != nil || string(response.DocumentContent) != testCase.payload {
				t.Fatalf("schema-invalid complete document was not forwarded: calls=%d response=%+v err=%v", calls, response, err)
			}
		})
	}
}

func TestRequestCandidateClassifiesOutputLimitSchemaTruncation(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return llmmodels.RuntimeCandidateGenerationResult{
			Payload: []byte(`{"operations":[{"id":"patch-1"`), Status: llmmodels.RuntimeCandidateStatusCompleted,
			Usage: &llmaudit.ProviderUsage{OutputTokens: 4096, TotalTokens: 24000},
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
	if response.OutputFragment == nil || !response.OutputFragment.ReachedProviderLimit || string(response.OutputFragment.Content) != `{"operations":[{"id":"patch-1"` || len(response.DocumentContent) != 0 {
		t.Fatalf("truncated candidate output fragment = %+v", response.OutputFragment)
	}
}

func TestRequestCandidateClassifiesMidTokenEOFAsOutputFragmentWithoutProviderLimit(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return llmmodels.RuntimeCandidateGenerationResult{
			Payload: []byte(`{"operations":[{"id":"patch-1"`), Status: llmmodels.RuntimeCandidateStatusCompleted,
			Audit: &llmmodels.ProviderRequestAudit{Status: llmmodels.RuntimeCandidateStatusCompleted, ProviderSubmitted: true},
		}, nil
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err != nil {
		t.Fatal(err)
	}
	if response.OutputFragment == nil || response.OutputFragment.ReachedProviderLimit || string(response.OutputFragment.Content) != `{"operations":[{"id":"patch-1"` || len(response.DocumentContent) != 0 {
		t.Fatalf("mid-token candidate output fragment = %+v", response.OutputFragment)
	}
}

func TestRequestCandidateClassifiesEmptySuccessfulBodyAsProviderOutcome(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return llmmodels.RuntimeCandidateGenerationResult{
			Status: llmmodels.RuntimeCandidateStatusCompleted,
			Audit:  &llmmodels.ProviderRequestAudit{Status: llmmodels.RuntimeCandidateStatusCompleted, ProviderSubmitted: true},
		}, nil
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err == nil {
		t.Fatal("empty provider body was accepted")
	}
	failure := CandidateFailureFromError(err)
	if failure.Class != CandidateFailureClassProvider || failure.Code != "provider_empty_response" {
		t.Fatalf("empty provider response failure = %+v", failure)
	}
	if len(response.DocumentContent) != 0 {
		t.Fatalf("empty provider response leaked complete document = %q", response.DocumentContent)
	}
}

func TestCandidateFailureFromErrorDoesNotExposeRawProviderTransport(t *testing.T) {
	err := &CandidateRequestError{
		Class: CandidateFailureClassProviderTransient, Code: "provider_transport", Signature: "provider-transport",
		Err: fmt.Errorf("POST https://provider.example/v1/chat: Authorization: Bearer secret"),
	}
	failure := CandidateFailureFromError(err)
	if failure.Reason != "provider request failed transiently" || strings.Contains(failure.Reason, "http") || strings.Contains(strings.ToLower(failure.Reason), "bearer") {
		t.Fatalf("provider failure leaked raw transport: %+v", failure)
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
	if response.PrerequisiteStale == nil || response.PrerequisiteStale.ObservationID != observation.ObservationID || len(response.DocumentContent) != 0 {
		t.Fatalf("candidate response lost stale boundary: %+v", response)
	}
	if extracted := CandidatePrerequisiteStaleFromError(err); extracted == nil || extracted.ObservedRevision != "config-v2" {
		t.Fatalf("candidate error did not retain typed stale: %v", err)
	}
}

func TestRequestCandidateProviderReceivesOnlySealedTypedRequest(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedValidation)
	wantDocument := []byte(`{"operations":[{"id":"validate-1","kind":"validation/managed_session","bindingRefs":["binding/test/cwi-1"],"parameters":[{"name":"recipeId","string":"test-managed-validation"}]}]}`)
	provider := candidateProviderFunc(func(_ context.Context, got llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		if got.Transaction != request.Transaction || got.Decision.AuthoritySnapshot.ID != request.Decision.AuthoritySnapshot.ID || got.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
			t.Fatalf("provider request changed sealed authority: %+v", got)
		}
		return generationResult(wantDocument, nil)
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err != nil {
		t.Fatal(err)
	}
	if string(response.DocumentContent) != string(wantDocument) {
		t.Fatalf("complete candidate document = %q, want %q", response.DocumentContent, wantDocument)
	}
	candidate, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(request.CandidateManifest, response.DocumentContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := candidate.ValidateWithActivation(request.Decision, request.CandidateContext.Activation); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeTypedAssessmentCandidate(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedAssessment)
	candidate, err := runtimecontract.DecodeRuntimeCandidatePayload(request.Decision.State, []byte(`{
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
	_, err := runtimecontract.DecodeRuntimeCandidatePayload(runtimecontract.ConvergenceStateNeedAssessment, []byte(`{
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

// C9 / T21: assessment is an untrusted observation. The candidate schema must
// not grow an action/control channel through which a provider can select a
// runtime transition, retry budget, governance ticket, or terminal outcome.
func TestDecodeTypedAssessmentCandidateRejectsRuntimeAuthorityFields(t *testing.T) {
	for field, value := range map[string]string{
		"requiredState": `"need_state_change"`,
		"runtimeAction": `"produce_state_change"`,
		"retry":         `1`,
		"ticket":        `"decision-1"`,
		"terminal":      `{"code":"stop"}`,
	} {
		t.Run(field, func(t *testing.T) {
			payload := []byte(`{
				"focusId":"focus-1",
				"coverageWorkItemIds":["cwi-1"],
				"disposition":"repairable_gap",
				"summary":"observation retained for audit",
				"gap":{"coverageWorkItemIds":["cwi-1"],"reason":"coverage remains open"},
				"` + field + `":` + value + `
			}`)
			_, err := runtimecontract.DecodeRuntimeCandidatePayload(runtimecontract.ConvergenceStateNeedAssessment, payload)
			if err == nil || !strings.Contains(err.Error(), `unknown field "`+field+`"`) {
				t.Fatalf("authority field %q error = %v", field, err)
			}
		})
	}
}

func candidateRequest(t *testing.T, state runtimecontract.ConvergenceState) llmmodels.RuntimeCandidateRequest {
	t.Helper()
	evidenceKind := runtimecontract.EvidenceKindStateChange
	if state == runtimecontract.ConvergenceStateNeedValidation {
		evidenceKind = runtimecontract.EvidenceKindValidation
	}
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds:    []string{evidenceKind},
			CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{candidateRequestCoverageBinding(evidenceKind)},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
				ID: "test-managed-validation", AppliesTo: runtimecontract.StepKindShellCommand,
				CommandName: "true", RecipeDeclared: true, RecipeArgs: []string{"workorch-validation"},
			}},
		},
	}
	decision := runtimecontract.RuntimeDecision{State: state, Focus: facts.Focus}
	switch state {
	case runtimecontract.ConvergenceStateNeedStateChange:
		decision.State = runtimecontract.ConvergenceStateNeedStateChange
		decision.RequiredOutput = runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{"patch/apply"},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
		}
	case runtimecontract.ConvergenceStateNeedValidation:
		decision.State = runtimecontract.ConvergenceStateNeedValidation
		decision.RequiredOutput = runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{"validation/managed_session"},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
		}
	case runtimecontract.ConvergenceStateNeedAssessment:
		decision.State = runtimecontract.ConvergenceStateNeedAssessment
		decision.RequiredOutput = runtimecontract.RequiredOutputContract{
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		}
	default:
		t.Fatalf("unsupported test state %s", state)
	}
	sealed, err := runtimecontract.SealRuntimeDecision(facts, decision, 0)
	if err != nil {
		t.Fatal(err)
	}
	context := runtimecontract.RuntimeCandidateContextForDecision(facts, sealed, nil)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(sealed, runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(context))
	if err != nil {
		t.Fatal(err)
	}
	return llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{
			LogicalRequestID: "candidate-request-1", Sequence: 1, PhysicalCallID: "candidate-call-1",
		},
		Decision: sealed, Transaction: sealed.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply", "validation/managed_session"}},
		CandidateManifest: manifest, CandidateContext: context,
	}
}

func candidateRequestCoverageBinding(evidenceKind string) runtimecontract.CoverageEvidenceBinding {
	proof := runtimecontract.CoverageEvidenceProof{
		EvidenceKind:          evidenceKind,
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
		AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
	}
	if evidenceKind == runtimecontract.EvidenceKindValidation {
		proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession}
		proof.AllowedOperationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
		proof.ValidationRecipeID = "test-managed-validation"
	}
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding/test/cwi-1", CoverageWorkItemID: "cwi-1", FocusID: "focus-1", DefinitionRevision: "definition-1",
		Proofs:     []runtimecontract.CoverageEvidenceProof{proof},
		TargetRefs: []string{"main.go"}, DeclarationRef: "test/llm-candidate-boundary",
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	return binding
}
