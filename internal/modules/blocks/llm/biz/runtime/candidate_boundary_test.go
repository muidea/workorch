package runtime

import (
	"context"
	"errors"
	"fmt"
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
	if providerCalls != 1 || response.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan || response.ExpectedDocumentHash != request.ExpectedDocument.Hash || response.DocumentOutcome == nil || string(response.DocumentOutcome.Content) != `{"documentKind":"semantic_plan"}` {
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

func TestRequestCandidateRejectsExpectedDocumentTokenMismatchBeforeProvider(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	mismatched := request.ExpectedDocument.Normalized()
	mismatched.MaxOutputTokens--
	mismatched.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(mismatched)
	request.ExpectedDocument = &mismatched
	providerCalls := 0
	_, err := RequestCandidate(context.Background(), candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		providerCalls++
		return generationResult([]byte(`{"operations":[]}`), nil)
	}), request)
	if err == nil || !strings.Contains(err.Error(), "max output tokens do not match") || providerCalls != 0 {
		t.Fatalf("token mismatch reached provider: calls=%d err=%v", providerCalls, err)
	}
}

func TestRequestCandidateRetryIdentityBindsDetachedCorrectionInputBeforeProvider(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	request.CandidateAttempt = 1
	request.CandidateCorrectionObservation = &runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations", Actual: "missing_operations", Required: []string{"operations"}, ManifestHash: request.CandidateManifest.Hash,
		},
	}
	request.CandidateCorrection = (&llmmodels.RuntimeCandidateCorrectionInput{
		ManifestHash: request.CandidateManifest.Hash,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField},
		Witness: runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField,
			Path:      "$.operations", RejectedValue: "field_absent", AlternativeValue: "field_present",
		},
	}).Clone()
	request.CandidateCorrectionObservation.Diagnostic.Objectives = append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), request.CandidateCorrection.Objectives...)
	request.CandidateCorrectionObservation.SealedCorrectionHash = runtimecontract.StableRuntimeCandidateSealedCorrectionHash(
		request.CandidateCorrection.ManifestHash, request.CandidateCorrection.Objectives, &request.CandidateCorrection.Witness,
	)
	sealCandidateCorrectionRequestForBoundaryTest(&request)

	providerCalls := 0
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		providerCalls++
		return generationResult([]byte(`{"operations":[]}`), nil)
	})
	if _, err := RequestCandidate(context.Background(), provider, request); err != nil || providerCalls != 1 {
		t.Fatalf("valid detached correction input did not reach provider exactly once: calls=%d err=%v", providerCalls, err)
	}

	for name, mutate := range map[string]func(*llmmodels.RuntimeCandidateRequest){
		"witness": func(value *llmmodels.RuntimeCandidateRequest) {
			value.CandidateCorrection.Witness.AlternativeValue = "different_field_present"
		},
		"objective": func(value *llmmodels.RuntimeCandidateRequest) {
			value.CandidateCorrection.Objectives = []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionRemoveForbiddenField}
		},
		"manifest": func(value *llmmodels.RuntimeCandidateRequest) {
			value.CandidateCorrection.ManifestHash = "stale-manifest"
		},
	} {
		t.Run(name, func(t *testing.T) {
			drift := request
			drift.CandidateCorrection = request.CandidateCorrection.Clone()
			sealCandidateCorrectionRequestForBoundaryTest(&drift)
			mutate(&drift)
			// Re-seal the request/session identity around the tampered detached
			// input. The pairwise observation fence, rather than a stale outer
			// request ID, must still stop it before provider I/O.
			sealCandidateCorrectionRequestForBoundaryTest(&drift)
			if _, err := RequestCandidate(context.Background(), provider, drift); err == nil || providerCalls != 1 {
				t.Fatalf("mismatched %s correction reached provider: calls=%d err=%v", name, providerCalls, err)
			}
		})
	}

	missing := request
	missing.CandidateCorrection = nil
	if _, err := RequestCandidate(context.Background(), provider, missing); err == nil || !strings.Contains(err.Error(), "previous failure and correction") || providerCalls != 1 {
		t.Fatalf("missing detached correction input reached provider: calls=%d err=%v", providerCalls, err)
	}
	initial := request
	initial.CandidateAttempt = 0
	if _, err := RequestCandidate(context.Background(), provider, initial); err == nil || !strings.Contains(err.Error(), "initial candidate attempt must not carry correction lineage") || providerCalls != 1 {
		t.Fatalf("attempt 0 detached correction input reached provider: calls=%d err=%v", providerCalls, err)
	}
}

func sealCandidateCorrectionRequestForBoundaryTest(request *llmmodels.RuntimeCandidateRequest) {
	if request == nil || request.CandidateCorrection == nil || request.CandidateCorrectionObservation == nil {
		return
	}
	correctionHash := request.CandidateCorrection.StableRequestCorrectionHash(request.CandidateCorrectionObservation)
	logicalRequestID := runtimecontract.StableRuntimeCandidateRequestSessionID(
		request.DocumentKind, request.CandidateContext.WorkspaceID, request.CandidateContext.RunID,
		request.Transaction, request.StablePrefixHash, request.CandidateManifest.Hash, correctionHash,
	)
	request.RequestIdentity = runtimecontract.RuntimeCandidateRequestIdentity{
		LogicalRequestID: logicalRequestID, Sequence: 1,
		PhysicalCallID: runtimecontract.StableRuntimeCandidatePhysicalCallID(logicalRequestID, 1),
	}
	expected := request.ExpectedDocument.Normalized()
	expected.LogicalRequestID = logicalRequestID
	expected.CorrectionAttempt = 1
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	request.ExpectedDocument = &expected
}

func semanticBoundaryRequest(t *testing.T) llmmodels.RuntimeCandidateRequest {
	t.Helper()
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
		CapabilityContract: []string{runtimecontract.StepKindFileWrite},
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}
	activation := runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: target}, ExecutionContract: runtimecontract.RuntimeExecutionContract{
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		StateChangePolicy:         runtimecontract.RuntimeStateChangeAllowedButNotRequired,
	}}
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
		ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
			CoverageWorkItemID: "cwi-1", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		}},
		OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	context := runtimecontract.RuntimeSemanticPlanRequestContext{
		WorkspaceID: "ws-1", RunID: "run-1", Definition: definition, DefinitionSeed: seed, Activation: activation,
		ProviderCeiling: &runtimecontract.ProviderCeiling{Selection: runtimecontract.ProviderInferenceSelection{Target: target}, ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048, MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1"},
		PlanStep:        runtimecontract.RuntimeSemanticPlanStepProofBatch, BatchID: "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}
	request := llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan,
		RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{
			LogicalRequestID: "semantic-request-1", Sequence: 1, PhysicalCallID: "semantic-call-1",
		},
		StablePrefixHash: runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, activation.ExecutionContract),
		SemanticManifest: manifest, SemanticContext: context,
	}
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:  request.DocumentKind, PlanStep: context.PlanStep,
		ManifestHash: manifest.Hash, DefinitionSeedHash: seed.Hash,
		LogicalRequestID: request.RequestIdentity.LogicalRequestID, StablePrefixHash: request.StablePrefixHash,
		BatchID: context.BatchID, BatchSequence: context.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), context.BatchCoverageWorkItemIDs...), CoverageOrderHash: "order-1",
		MaxOutputTokens: 2048, MaxRecoveryAttempts: 2,
	}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	request.ExpectedDocument = &expected
	return request
}

func TestDecodeCandidatePayloadRejectsProviderOperationID(t *testing.T) {
	_, err := runtimecontract.DecodeRuntimeCandidatePayload(runtimecontract.ConvergenceStateNeedStateChange, []byte(`{
		"operations":[{"id":"op-1","kind":"patch/apply","bindingRefs":["binding/test/cwi-1"]}]
	}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "id"`) {
		t.Fatalf("candidate payload operation id was accepted: %v", err)
	}
}

func TestDecodeCandidatePayloadRejectsConvergenceAuthorityField(t *testing.T) {
	_, err := runtimecontract.DecodeRuntimeCandidatePayload(runtimecontract.ConvergenceStateNeedStateChange, []byte(`{
		"operations":[{"kind":"patch/apply","bindingRefs":["binding/test/cwi-1"]}],
		"convergenceState":"complete"
	}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "convergenceState"`) {
		t.Fatalf("candidate payload convergence authority was accepted: %v", err)
	}
}

func TestDecodeCandidatePayloadRejectsLegacyCandidateWrappers(t *testing.T) {
	for name, payload := range map[string]string{
		"candidate":      `{"candidate":{"operations":[{"kind":"patch/apply","bindingRefs":["binding/test/cwi-1"]}]}}`,
		"candidates":     `{"candidates":[{"operations":[{"kind":"patch/apply","bindingRefs":["binding/test/cwi-1"]}]}]}`,
		"executionGraph": `{"executionGraph":{"operations":[{"kind":"patch/apply","bindingRefs":["binding/test/cwi-1"]}]}}`,
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
		"operations":[{"kind":"patch/apply","coverageWorkItemIds":["cwi-1"]}]
	}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "coverageWorkItemIds"`) {
		t.Fatalf("raw candidate coverage was accepted: %v", err)
	}
}

func TestDecodeCandidatePayloadPreservesRequiredOperationRefWithoutInventingAuthority(t *testing.T) {
	candidate, err := runtimecontract.DecodeRuntimeCandidatePayload(runtimecontract.ConvergenceStateNeedStateChange, []byte(`{
		"operations":[{
			"kind":"git/write",
			"requiredOperationRef":"repo-init",
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
	if operation := candidate.StateChange.Operations[0].Operation; operation.ID != "" || len(operation.DependsOn) != 0 {
		t.Fatalf("wire decoder invented operation authority: %+v", operation)
	}
}

func TestRequestCandidateForwardsCompleteFilePathAliasDocument(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	payload := []byte(`{"operations":[{"kind":"patch/apply","bindingRefs":["binding/test/cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"filePath","string":"main.go"},{"name":"patchType","string":"file_write"},{"name":"content","string":"package main\n"}]}]}`)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return generationResult(payload, nil)
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err != nil || response.DocumentOutcome == nil || string(response.DocumentOutcome.Content) != string(payload) {
		t.Fatalf("complete alias document was not forwarded unchanged: response=%+v err=%v", response, err)
	}
}

func TestRequestCandidateForwardsCompleteSchemaInvalidDocuments(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	validOperation := `{"kind":"patch/apply","bindingRefs":["binding/test/cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main\n"}]}`
	cases := []struct {
		name    string
		payload string
	}{
		{name: "unknown top level", payload: `{"operations":[` + validOperation + `],"unexpected":true}`},
		{name: "missing required operations", payload: `{}`},
		{name: "wrong operations container", payload: `{"operations":{}}`},
		{name: "wrong target refs container", payload: `{"operations":[{"kind":"patch/apply","bindingRefs":["binding/test/cwi-1"],"targetRefs":"main.go","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main\n"}]}]}`},
		{name: "trailing json", payload: `{"operations":[` + validOperation + `]} {}`},
		{name: "field alias", payload: `{"operations":[{"kind":"patch/apply","bindingRefs":["binding/test/cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"filePath","string":"main.go"},{"name":"patchType","string":"file_write"},{"name":"content","string":"package main\n"}]}]}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			calls := 0
			provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
				calls++
				return generationResult([]byte(testCase.payload), nil)
			})
			response, err := RequestCandidate(context.Background(), provider, request)
			if err != nil || calls != 1 || response.DocumentOutcome == nil || string(response.DocumentOutcome.Content) != testCase.payload {
				t.Fatalf("schema-invalid complete document was not forwarded: calls=%d response=%+v err=%v", calls, response, err)
			}
		})
	}
}

func TestRequestCandidateClassifiesProviderProseAsProtocolOutcome(t *testing.T) {
	request := semanticBoundaryRequest(t)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return generationResult([]byte("The user wants me to generate a semantic plan."), nil)
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err != nil || response.DocumentOutcome == nil || response.DocumentOutcome.Framing.Disposition != runtimecontract.RuntimeProviderDocumentInvalid || response.DocumentOutcome.Framing.Code != "provider_final_output_invalid" {
		t.Fatalf("provider prose outcome: response=%+v err=%v", response, err)
	}
	if len(response.DocumentOutcome.Content) != 0 {
		t.Fatalf("provider prose leaked into candidate response: %+v", response)
	}
}

func TestRequestCandidatePreservesExactOuterJSONFenceForSM3Normalization(t *testing.T) {
	request := semanticBoundaryRequest(t)
	payload := []byte("```json\n{\"documentKind\":\"semantic_plan\"}\n```")
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return generationResult(payload, nil)
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err != nil {
		t.Fatalf("exact JSON fence was rejected by provider boundary: %v", err)
	}
	if response.DocumentOutcome == nil || string(response.DocumentOutcome.Content) != string(payload) {
		t.Fatalf("provider boundary changed raw fenced document: %+v", response.DocumentOutcome)
	}
}

func TestRequestCandidateClassifiesOutputLimitSchemaTruncation(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return llmmodels.RuntimeCandidateGenerationResult{
			Payload: []byte(`{"operations":[{"kind":"patch/apply"`), Status: llmmodels.RuntimeCandidateStatusCompleted,
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
	if response.DocumentOutcome == nil || response.DocumentOutcome.Framing.Disposition != runtimecontract.RuntimeProviderDocumentIncomplete || response.DocumentOutcome.Framing.Code != "max_output_tokens" || response.DocumentOutcome.Content != nil {
		t.Fatalf("truncated candidate outcome = %+v", response.DocumentOutcome)
	}
}

func TestRequestCandidateClassifiesProviderOutputLimitForWholeDocumentRecovery(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	payload := []byte(`{"operations":[`)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return llmmodels.RuntimeCandidateGenerationResult{
			Payload: payload, ReachedProviderLimit: true,
			Status: llmmodels.RuntimeCandidateStatusCompleted,
			Audit:  &llmmodels.ProviderRequestAudit{Status: llmmodels.RuntimeCandidateStatusCompleted, ProviderSubmitted: true},
		}, errors.New("llm provider response incomplete: max_output_tokens")
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err != nil {
		t.Fatalf("output-limit fragment returned error: %v", err)
	}
	if response.DocumentOutcome == nil || response.DocumentOutcome.Framing.Disposition != runtimecontract.RuntimeProviderDocumentIncomplete || response.DocumentOutcome.Framing.Code != "max_output_tokens" || response.DocumentOutcome.Framing.Signature != "provider-output-limit" || response.DocumentOutcome.Framing.ResponseBytes != int64(len(payload)) || response.DocumentOutcome.Framing.ResponseHash != runtimecontract.StableRuntimeProviderResponseHash(payload) || response.DocumentOutcome.Content != nil {
		t.Fatalf("output-limit response = %+v", response.DocumentOutcome)
	}
}

func TestRequestCandidatePrioritizesProviderOutputLimitOverPayloadShape(t *testing.T) {
	request := semanticBoundaryRequest(t)
	for name, payload := range map[string][]byte{
		"provider prose":          []byte("The user wants me to generate a semantic plan."),
		"complete-looking object": []byte(`{"documentKind":"semantic_plan"}`),
	} {
		t.Run(name, func(t *testing.T) {
			provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
				return llmmodels.RuntimeCandidateGenerationResult{
					Payload: payload, ReachedProviderLimit: true,
					Status: llmmodels.RuntimeCandidateStatusCompleted,
					Audit:  &llmmodels.ProviderRequestAudit{Status: llmmodels.RuntimeCandidateStatusCompleted, ProviderSubmitted: true},
				}, errors.New("llm provider response incomplete: max_output_tokens")
			})
			response, err := RequestCandidate(context.Background(), provider, request)
			if err != nil || response.DocumentOutcome == nil || response.DocumentOutcome.Framing.Disposition != runtimecontract.RuntimeProviderDocumentIncomplete || response.DocumentOutcome.Framing.Code != "max_output_tokens" || response.DocumentOutcome.Framing.ResponseHash != runtimecontract.StableRuntimeProviderResponseHash(payload) || response.DocumentOutcome.Framing.ResponseBytes != int64(len(payload)) || response.DocumentOutcome.Content != nil {
				t.Fatalf("output-limit result: response=%+v err=%v", response, err)
			}
		})
	}
}

func TestRequestCandidateClassifiesOutputLimitWithoutFinalText(t *testing.T) {
	request := semanticBoundaryRequest(t)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return llmmodels.RuntimeCandidateGenerationResult{
			ReachedProviderLimit: true,
			Status:               llmmodels.RuntimeCandidateStatusCompleted,
			Audit:                &llmmodels.ProviderRequestAudit{Status: llmmodels.RuntimeCandidateStatusCompleted, ProviderSubmitted: true},
		}, errors.New("llm provider response incomplete: max_output_tokens")
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err != nil || response.DocumentOutcome == nil || response.DocumentOutcome.Framing.Disposition != runtimecontract.RuntimeProviderDocumentIncomplete || response.DocumentOutcome.Framing.Code != "max_output_tokens" || response.DocumentOutcome.Framing.Signature != "provider-output-limit" || response.DocumentOutcome.Framing.ResponseHash != "" || response.DocumentOutcome.Framing.ResponseBytes != 0 || response.DocumentOutcome.Content != nil {
		t.Fatalf("missing final output result: response=%+v err=%v", response, err)
	}
}

func TestRequestCandidateRejectsContradictoryProviderOutputLimitGeneration(t *testing.T) {
	request := semanticBoundaryRequest(t)
	tests := []struct {
		name       string
		generation llmmodels.RuntimeCandidateGenerationResult
	}{
		{
			name: "failed generation and audit",
			generation: llmmodels.RuntimeCandidateGenerationResult{
				ReachedProviderLimit: true,
				Status:               llmmodels.RuntimeCandidateStatusFailed,
				FailureClass:         string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol),
				Audit: &llmmodels.ProviderRequestAudit{
					Status: llmmodels.RuntimeCandidateStatusFailed, FailureClass: string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol), ProviderSubmitted: true,
				},
			},
		},
		{
			name: "unsubmitted completed audit",
			generation: llmmodels.RuntimeCandidateGenerationResult{
				ReachedProviderLimit: true,
				Status:               llmmodels.RuntimeCandidateStatusCompleted,
				Audit:                &llmmodels.ProviderRequestAudit{Status: llmmodels.RuntimeCandidateStatusCompleted},
			},
		},
		{
			name: "missing audit",
			generation: llmmodels.RuntimeCandidateGenerationResult{
				ReachedProviderLimit: true,
				Status:               llmmodels.RuntimeCandidateStatusCompleted,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
				return tt.generation, errors.New("llm provider response incomplete: max_output_tokens")
			})
			response, err := RequestCandidate(context.Background(), provider, request)
			if err == nil || response.DocumentOutcome != nil || response.Audit != nil {
				t.Fatalf("contradictory output-limit result was accepted: response=%+v err=%v", response, err)
			}
			failure := CandidateFailureFromError(err)
			if failure.Class != CandidateFailureClassInternal {
				t.Fatalf("failure class = %q, want internal", failure.Class)
			}
		})
	}
}

func TestRequestCandidateClassifiesMidTokenEOFAsIncompleteDocumentWithoutProviderLimit(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return llmmodels.RuntimeCandidateGenerationResult{
			Payload: []byte(`{"operations":[{"kind":"patch/app`), Status: llmmodels.RuntimeCandidateStatusCompleted,
			Audit: &llmmodels.ProviderRequestAudit{Status: llmmodels.RuntimeCandidateStatusCompleted, ProviderSubmitted: true},
		}, nil
	})
	response, err := RequestCandidate(context.Background(), provider, request)
	if err != nil {
		t.Fatal(err)
	}
	if response.DocumentOutcome == nil || response.DocumentOutcome.Framing.Disposition != runtimecontract.RuntimeProviderDocumentIncomplete || response.DocumentOutcome.Framing.Code != "incomplete_json_document" || response.DocumentOutcome.Content != nil {
		t.Fatalf("mid-token candidate outcome = %+v", response.DocumentOutcome)
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
	if err != nil || response.DocumentOutcome == nil || response.DocumentOutcome.Framing.Disposition != runtimecontract.RuntimeProviderDocumentEmpty || response.DocumentOutcome.Framing.Code != "provider_final_output_missing" {
		t.Fatalf("empty provider response outcome = %+v err=%v", response.DocumentOutcome, err)
	}
	if response.DocumentOutcome.Content != nil {
		t.Fatalf("empty provider response leaked complete document = %q", response.DocumentOutcome.Content)
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

func TestRequestCandidatePreservesTypedProviderGuardIdentity(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedStateChange)
	provider := candidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
		return llmmodels.RuntimeCandidateGenerationResult{
			Status:       llmmodels.RuntimeCandidateStatusFailed,
			FailureClass: string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol),
		}, typedProviderIdentityError{code: "response_byte_limit_exceeded"}
	})
	_, err := RequestCandidate(context.Background(), provider, request)
	if err == nil {
		t.Fatal("typed provider protocol failure was accepted")
	}
	failure := CandidateFailureFromError(err)
	if failure.Class != runtimecontract.RuntimeCandidateFailureClassProviderProtocol ||
		failure.Code != "response_byte_limit_exceeded" || failure.Signature != "response-byte-limit-exceeded" ||
		failure.Reason != "provider response violated the protocol contract" {
		t.Fatalf("typed provider failure = %+v", failure)
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
	if response.PrerequisiteStale == nil || response.PrerequisiteStale.ObservationID != observation.ObservationID || response.DocumentOutcome != nil {
		t.Fatalf("candidate response lost stale boundary: %+v", response)
	}
	if extracted := CandidatePrerequisiteStaleFromError(err); extracted == nil || extracted.ObservedRevision != "config-v2" {
		t.Fatalf("candidate error did not retain typed stale: %v", err)
	}
}

func TestRequestCandidateProviderReceivesOnlySealedTypedRequest(t *testing.T) {
	request := candidateRequest(t, runtimecontract.ConvergenceStateNeedValidation)
	wantDocument := []byte(`{"operations":[{"kind":"validation/managed_session","bindingRefs":["binding/test/cwi-1"],"parameters":[{"name":"recipeId","string":"test-managed-validation"}]}]}`)
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
	if response.DocumentOutcome == nil || string(response.DocumentOutcome.Content) != string(wantDocument) {
		t.Fatalf("complete candidate document = %+v, want %q", response.DocumentOutcome, wantDocument)
	}
	candidate, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(request.CandidateManifest, response.DocumentOutcome.Content)
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
	binding := candidateRequestCoverageBinding(evidenceKind)
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds:    []string{evidenceKind},
			CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{binding},
			TargetRefs:               append([]string(nil), binding.TargetRefs...),
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired},
			EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
				ID: "test-managed-validation", AppliesTo: runtimecontract.StepKindShellCommand,
				CommandName: "true", RecipeDeclared: true, RecipeArgs: []string{"workorch-validation"},
			}},
		},
	}
	facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()
	facts.RecoveryPolicy = runtimecontract.DefaultRuntimeRecoveryPolicy()
	facts.ExecutionBudgetPolicy = runtimecontract.DefaultRuntimeExecutionBudgetPolicy()
	facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection:                facts.Activation.ProviderSelection,
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 8192,
		MaxInputTokens:           119808,
		MaxBytes:                 128 * 1024,
		CapabilityRevision:       "capability-1",
		ConfigurationRevision:    "config-1",
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
	request := llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{
			LogicalRequestID: "candidate-request-1", Sequence: 1, PhysicalCallID: "candidate-call-1",
		},
		Decision: sealed, Transaction: sealed.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply", "validation/managed_session"}},
		CandidateManifest: manifest, CandidateContext: context,
	}
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:  request.DocumentKind, ManifestHash: manifest.Hash,
		LogicalRequestID: request.RequestIdentity.LogicalRequestID, StablePrefixHash: request.StablePrefixHash,
		MaxOutputTokens: context.Resources.ProviderCeiling.EffectiveMaxOutputTokens, MaxRecoveryAttempts: 2,
	}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	request.ExpectedDocument = &expected
	return request
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
