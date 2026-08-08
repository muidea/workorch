package orchestrator

// Owner Biz candidate adapter tests.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	llmruntime "github.com/muidea/workorch/internal/modules/blocks/llm/biz/runtime"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type eventCandidateProviderFunc func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error)

func (f eventCandidateProviderFunc) GenerateCandidate(ctx context.Context, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
	return f(ctx, request)
}

func eventGenerationResult(payload []byte, err error) (llmmodels.RuntimeCandidateGenerationResult, error) {
	if err != nil {
		return llmmodels.RuntimeCandidateGenerationResult{
			Status:        llmmodels.RuntimeCandidateStatusFailed,
			FailureReason: err.Error(),
			Audit: &llmmodels.ProviderRequestAudit{
				Status:            llmmodels.RuntimeCandidateStatusFailed,
				ProviderSubmitted: true,
				FailureClass:      "provider",
			},
		}, err
	}
	return llmmodels.RuntimeCandidateGenerationResult{
		Payload: payload,
		Status:  llmmodels.RuntimeCandidateStatusCompleted,
		Usage:   &llmaudit.ProviderUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
		Audit: &llmmodels.ProviderRequestAudit{
			Status:            llmmodels.RuntimeCandidateStatusCompleted,
			ProviderSubmitted: true,
			Target:            runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			Operation:         "chat_completions",
		},
	}, nil
}

func TestRequestRuntimeCandidatePreservesSemanticDocumentKindAcrossEventHub(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	llm := llmmodule.New()
	providerCalls := 0
	if err := llm.Configure(llmbiz.Dependencies{
		EventHub: hub,
		CandidateProvider: eventCandidateProviderFunc(func(_ context.Context, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
			providerCalls++
			if request.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan {
				t.Fatalf("provider document kind = %q", request.DocumentKind)
			}
			return eventGenerationResult([]byte(`{"documentKind":"semantic_plan"}`), nil)
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)
	command := eventSemanticCandidateCommand(t)
	response, err := RequestRuntimeCandidate(ctx, hub, command)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || response.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan || string(response.DocumentContent) != `{"documentKind":"semantic_plan"}` {
		t.Fatalf("semantic response=%+v providerCalls=%d", response, providerCalls)
	}
}

func eventSemanticCandidateCommand(t *testing.T) llmevents.RequestRuntimeCandidateCommand {
	t.Helper()
	definition := runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
		CapabilityContract: []string{runtimecontract.StepKindFileWrite},
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}
	activation := runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: target}, ExecutionContract: runtimecontract.RuntimeExecutionContract{OperationContractRevision: runtimecontract.RuntimeOperationContractRevision()}}
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
		ProviderCeiling: &runtimecontract.ProviderCeiling{Selection: runtimecontract.ProviderInferenceSelection{Target: target}, ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048, MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1"},
		PlanStep:        runtimecontract.RuntimeSemanticPlanStepProofBatch, BatchID: "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}
	return llmevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan,
		RequestIdentity:  runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "semantic-request-1", Sequence: 1, PhysicalCallID: "semantic-call-1"},
		StablePrefixHash: runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, activation.ExecutionContract),
		SemanticManifest: manifest, SemanticContext: context,
	}
}

func candidateManifestForEventRequest(t *testing.T, decision runtimecontract.RuntimeDecision, context runtimecontract.RuntimeCandidateContext) runtimecontract.RuntimeCandidateContractManifest {
	t.Helper()
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(context.Resources))
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestRequestRuntimeCandidateReturnsTypedProviderFailure(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	background := magictask.NewBackgroundRoutine(1)
	defer background.Shutdown(ctx)
	llm := llmmodule.New()
	if err := llm.Configure(llmbiz.Dependencies{
		EventHub: hub,
		CandidateProvider: eventCandidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
			return eventGenerationResult(nil, errors.New("provider unavailable"))
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)
	decision := sealedCandidateTestDecision(t)
	response, err := RequestRuntimeCandidate(ctx, hub, llmevents.RequestRuntimeCandidateCommand{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		WorkspaceID:  "ws-1", RunID: "run-1", RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "candidate-request-provider-failure", Sequence: 1, PhysicalCallID: "candidate-call-provider-failure"},
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContextForDecision(decision)), CandidateContext: candidateContextForDecision(decision),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Feedback == nil || response.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainProviderRequest || response.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryProviderProtocol || response.Feedback.OriginalDiagnostic == nil || response.Feedback.OriginalDiagnostic.OriginalClass != string(runtimecontract.RuntimeCandidateFailureClassProvider) || len(response.DocumentContent) != 0 {
		t.Fatalf("response = %+v", response)
	}
	if strings.Contains(response.Feedback.OriginalDiagnostic.OriginalMessage, "provider unavailable") {
		t.Fatalf("provider feedback leaked raw transport error: %+v", response.Feedback)
	}
}

func TestRuntimeProviderRequestFeedbackValidationIsExhaustiveAndScoped(t *testing.T) {
	decision := sealedCandidateTestDecision(t)
	command := llmevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Transaction: decision.AuthorityTransaction(),
	}
	tests := []struct {
		name     string
		class    runtimecontract.RuntimeCandidateFailureClass
		category runtimecontract.RuntimeFeedbackCategory
	}{
		{name: "authentication", class: runtimecontract.RuntimeCandidateFailureClassProviderAuthentication, category: runtimecontract.RuntimeFeedbackCategoryProviderAuthentication},
		{name: "configuration", class: runtimecontract.RuntimeCandidateFailureClassProviderConfiguration, category: runtimecontract.RuntimeFeedbackCategoryProviderConfiguration},
		{name: "transient", class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, category: runtimecontract.RuntimeFeedbackCategoryProviderTransient},
		{name: "protocol", class: runtimecontract.RuntimeCandidateFailureClassProvider, category: runtimecontract.RuntimeFeedbackCategoryProviderProtocol},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			feedback, err := runtimecontract.RuntimeFeedbackFromCandidateFailure(
				runtimecontract.RuntimeFeedbackScope{WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.Transaction.AuthoritySnapshotID},
				runtimecontract.RuntimeFeedbackDomainProviderRequest, tc.category, runtimecontract.RuntimeModuleLLM,
				runtimecontract.RuntimeCandidateFailure{Class: tc.class, Code: string(tc.category), Signature: "provider-feedback-1", Reason: "safe provider diagnostic"},
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateRuntimeProviderRequestFeedback(command, feedback); err != nil {
				t.Fatalf("valid feedback rejected: %v", err)
			}
			mismatched := feedback.CloneOriginal()
			mismatched.WorkspaceID = "ws-other"
			if err := validateRuntimeProviderRequestFeedback(command, mismatched); err == nil {
				t.Fatal("workspace-mismatched provider feedback was accepted")
			}
			mismatched = feedback.CloneOriginal()
			mismatched.StableCategory = runtimecontract.RuntimeFeedbackCategory("unknown_provider_category")
			if err := validateRuntimeProviderRequestFeedback(command, mismatched); err == nil {
				t.Fatal("unknown provider feedback category was accepted")
			}
		})
	}
}

func TestRequestRuntimeCandidateForwardsCompleteSchemaRejectionDocumentOnly(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	llm := llmmodule.New()
	payload := []byte(`{"operations":[],"unexpected":true}`)
	if err := llm.Configure(llmbiz.Dependencies{
		EventHub: hub,
		CandidateProvider: eventCandidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
			return eventGenerationResult(payload, nil)
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)
	decision := sealedCandidateTestDecision(t)
	response, err := RequestRuntimeCandidate(ctx, hub, llmevents.RequestRuntimeCandidateCommand{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		WorkspaceID:  "ws-1", RunID: "run-1", RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "candidate-request-complete-schema", Sequence: 1, PhysicalCallID: "candidate-call-complete-schema"},
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContextForDecision(decision)), CandidateContext: candidateContextForDecision(decision),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Feedback != nil || string(response.DocumentContent) != string(payload) {
		t.Fatalf("complete schema document handoff = %+v", response)
	}
}

func TestRequestRuntimeCandidateReturnsTypedPrerequisiteStaleWithoutCandidateFailure(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	llm := llmmodule.New()
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
	if err := llm.Configure(llmbiz.Dependencies{
		EventHub: hub,
		CandidateProvider: eventCandidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
			return llmmodels.RuntimeCandidateGenerationResult{
				Status: llmmodels.RuntimeCandidateStatusFailed, PrerequisiteStale: &stale,
			}, errors.New("sealed provider configuration drift")
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)
	decision := sealedCandidateTestDecision(t)
	response, err := RequestRuntimeCandidate(ctx, hub, llmevents.RequestRuntimeCandidateCommand{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		WorkspaceID:  "ws-1", RunID: "run-1", RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "candidate-request-stale", Sequence: 1, PhysicalCallID: "candidate-call-stale"},
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContextForDecision(decision)), CandidateContext: candidateContextForDecision(decision),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Stale == nil || response.Stale.ObservationID != observation.ObservationID || response.Feedback != nil || len(response.DocumentContent) != 0 {
		t.Fatalf("candidate response = %+v, want typed stale only", response)
	}
}

func TestRequestRuntimeCandidateUsesTypedSealedEventHubContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	background := magictask.NewBackgroundRoutine(2)
	defer background.Shutdown(ctx)

	decision := sealedCandidateTestDecision(t)
	var captured []llmmodels.RuntimeCandidateRequest
	llm := llmmodule.New()
	if err := llm.Configure(llmbiz.Dependencies{
		EventHub: hub,
		CandidateProvider: eventCandidateProviderFunc(func(_ context.Context, request llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
			captured = append(captured, request)
			return eventGenerationResult([]byte(`{"operations":[{"id":"op-1","kind":"patch/apply","bindingRefs":["binding/test/focus-1/cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main\n"}]}]}`), nil)
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)

	base := llmevents.RequestRuntimeCandidateCommand{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		WorkspaceID:  "ws-1", RunID: "run-1", RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "candidate-request-1", Sequence: 1, PhysicalCallID: "physical-call-1"},
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContextForDecision(decision)), CandidateContext: candidateContextForDecision(decision),
	}
	first, err := RequestRuntimeCandidate(ctx, hub, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(base.CandidateManifest, first.DocumentContent); err != nil {
		t.Fatal(err)
	}
	if len(first.DocumentContent) == 0 {
		t.Fatal("complete candidate result did not carry the synchronous raw document")
	}
	retry := base
	retry.CandidateAttempt = 1
	retry.RequestIdentity.Sequence = 2
	retry.RequestIdentity.PhysicalCallID = "physical-call-2"
	retry.PreviousFailure = &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Reason: `json: unknown field "candidate"`,
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.unexpected", Actual: "unexpected", Allowed: []string{"operations"}, Forbidden: []string{"unexpected"},
		},
	}
	second, err := RequestRuntimeCandidate(ctx, hub, retry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(base.CandidateManifest, second.DocumentContent); err != nil {
		t.Fatal(err)
	}
	if len(second.DocumentContent) == 0 {
		t.Fatal("complete correction candidate result did not carry the synchronous raw document")
	}
	if first.RequestIdentity.LogicalRequestID != base.RequestIdentity.LogicalRequestID || first.RequestIdentity.Sequence != 1 || first.RequestIdentity.PhysicalCallID != "physical-call-1" || second.RequestIdentity.LogicalRequestID != base.RequestIdentity.LogicalRequestID || second.RequestIdentity.Sequence != 2 || second.RequestIdentity.PhysicalCallID != "physical-call-2" {
		t.Fatalf("candidate response identity was not preserved: first=%+v second=%+v", first, second)
	}
	if len(captured) != 2 || captured[0].RequestIdentity.LogicalRequestID != base.RequestIdentity.LogicalRequestID || captured[0].RequestIdentity.Sequence != 1 || captured[0].RequestIdentity.PhysicalCallID != "physical-call-1" || captured[1].RequestIdentity.LogicalRequestID != base.RequestIdentity.LogicalRequestID || captured[1].RequestIdentity.Sequence != 2 || captured[1].RequestIdentity.PhysicalCallID != "physical-call-2" || captured[0].CandidateAttempt != 0 || captured[0].PreviousFailure != nil || captured[1].CandidateAttempt != 1 || captured[1].PreviousFailure == nil || captured[1].PreviousFailure.Code != "candidate_schema_invalid" || captured[1].PreviousFailure.Diagnostic == nil || captured[1].PreviousFailure.Diagnostic.Path != "$.unexpected" || captured[0].Transaction != captured[1].Transaction || captured[0].StablePrefixHash != captured[1].StablePrefixHash || captured[0].CandidateManifest.Hash != base.CandidateManifest.Hash || captured[1].CandidateManifest.Hash != base.CandidateManifest.Hash || captured[0].Decision.AuthoritySnapshot.RequiredOutputHash != captured[1].Decision.AuthoritySnapshot.RequiredOutputHash {
		t.Fatalf("candidate retries changed sealed contract: %+v", captured)
	}
}

func TestRequestRuntimeCandidateRejectsManifestAuthorityDriftBeforeEventHub(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(1)
	defer hub.Terminate(ctx)
	decision := sealedCandidateTestDecision(t)
	candidateContext := candidateContextForDecision(decision)
	manifest := candidateManifestForEventRequest(t, decision, candidateContext)
	manifest.TargetRefs = []string{"other.go"}
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	_, err := RequestRuntimeCandidate(ctx, hub, llmevents.RequestRuntimeCandidateCommand{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		WorkspaceID:  "ws-1", RunID: "run-1", RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "candidate-request-manifest-drift", Sequence: 1, PhysicalCallID: "candidate-call-manifest-drift"},
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1", CandidateManifest: manifest,
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateContext:  candidateContext,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match sealed authority") {
		t.Fatalf("manifest authority drift error = %v", err)
	}
}

func TestRequestRuntimeCandidateRejectsIncompleteRequestSessionIdentity(t *testing.T) {
	decision := sealedCandidateTestDecision(t)
	for name, identity := range map[string]struct {
		logical  string
		sequence int
		physical string
	}{
		"empty":   {},
		"partial": {logical: "candidate-request-1"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := RequestRuntimeCandidate(context.Background(), magicevent.NewHub(1), llmevents.RequestRuntimeCandidateCommand{
				DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
				WorkspaceID:  "ws-1", RunID: "run-1", RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: identity.logical, Sequence: identity.sequence, PhysicalCallID: identity.physical},
				Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
				DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
				CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContextForDecision(decision)), CandidateContext: candidateContextForDecision(decision),
			})
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("request-session identity err = %v, want invalid argument", err)
			}
		})
	}
}

func TestPreflightRuntimeCandidateRendersWithoutProviderSubmission(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	providerCalls := 0
	llm := llmmodule.New()
	if err := llm.Configure(llmbiz.Dependencies{
		EventHub: hub,
		CandidateProvider: eventCandidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
			providerCalls++
			return eventGenerationResult(nil, errors.New("provider must not be called by preflight"))
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)

	decision := sealedCandidateTestDecision(t)
	candidateContext := candidateContextForDecision(decision)
	candidateContext.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection:                runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 96 * 1024,
		CapabilityRevision:       "capability-1",
		ConfigurationRevision:    "config-1",
	}
	definition := runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, candidateContext.Activation.ExecutionContract)
	response, err := PreflightRuntimeCandidate(ctx, hub, llmevents.PreflightRuntimeCandidateCommand{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		WorkspaceID:  "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		DefinitionContext: definition,
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContext), CandidateContext: candidateContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryAccepted || response.Feedback.OriginalDiagnostic != nil || response.Audit == nil || response.Audit.ProviderSubmitted || providerCalls != 0 {
		t.Fatalf("preflight response=%+v providerCalls=%d", response, providerCalls)
	}
	if response.Audit.ContextBudget.ContextGateAction != "" || len(response.Audit.ContextBudget.SectionHashes) == 0 || len(response.Audit.ContextBudget.SourceRefs) == 0 || len(response.Audit.ContextBudget.ProtectedFields) == 0 {
		t.Fatalf("ready preflight audit=%+v", response.Audit.ContextBudget)
	}
}

func TestPreflightRuntimeCandidateOverBudgetPublishesSecretFreeAuditWithoutProviderSubmission(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	providerCalls := 0
	llm := llmmodule.New()
	if err := llm.Configure(llmbiz.Dependencies{
		EventHub: hub,
		CandidateProvider: eventCandidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
			providerCalls++
			return eventGenerationResult(nil, errors.New("provider must not be called by preflight"))
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)

	auditObserver := magicevent.NewSimpleObserver("/llm/consumers/preflight-audit-test", hub)
	audits := make(chan llmevents.LLMUsageRecordedData, 1)
	auditObserver.Subscribe(llmevents.TopicLLMUsageRecorded, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(llmevents.LLMUsageRecordedData); ok {
			audits <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	defer auditObserver.Unsubscribe(llmevents.TopicLLMUsageRecorded)

	decision := sealedCandidateTestDecision(t)
	candidateContext := candidateContextForDecision(decision)
	candidateContext.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection:                runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		// A positive but deliberately too-small ceiling reaches the LLM
		// renderer and exercises the rendered not-ready branch.
		MaxBytes:              1,
		CapabilityRevision:    "capability-1",
		ConfigurationRevision: "config-1",
	}
	const secret = "preflight-secret-must-not-reach-audit"
	definition := runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: secret,
		DoDID: "dod-1", DoDDefinition: secret, CapabilityContract: []string{"patch/apply"},
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, candidateContext.Activation.ExecutionContract)
	response, err := PreflightRuntimeCandidate(ctx, hub, llmevents.PreflightRuntimeCandidateCommand{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		WorkspaceID:  "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContext), DefinitionContext: definition, CandidateContext: candidateContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryContextIntegrity || response.Feedback.OriginalDiagnostic == nil || response.Feedback.OriginalDiagnostic.OriginalMessage == "" || response.Audit == nil || response.Audit.ProviderSubmitted || providerCalls != 0 {
		t.Fatalf("not-ready preflight response=%+v providerCalls=%d", response, providerCalls)
	}
	if strings.Contains(response.Feedback.OriginalDiagnostic.OriginalMessage, secret) {
		t.Fatalf("preflight feedback leaked rendered definition content: %+v", response.Feedback)
	}
	budget := response.Audit.ContextBudget
	if budget.ContextGateAction != "fail_fast" || budget.ContextGateReason != "runtime_provider_ceiling_exceeded" || budget.BudgetFailureClass != "provider_ceiling_exceeded" || len(budget.SectionHashes) == 0 || len(budget.SourceRefs) == 0 {
		t.Fatalf("not-ready preflight audit=%+v", budget)
	}

	select {
	case recorded := <-audits:
		if recorded.ProviderSubmitted || recorded.RequestID == "" || recorded.StepID != decision.AuthoritySnapshot.ID || recorded.ContextBudget.ContextGateReason != "runtime_provider_ceiling_exceeded" || len(recorded.ContextBudget.SectionHashes) == 0 {
			t.Fatalf("recorded preflight audit=%+v", recorded)
		}
		payload, marshalErr := json.Marshal(recorded.ContextBudget.Payload())
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if strings.Contains(string(payload), secret) {
			t.Fatalf("preflight audit leaked rendered definition content: %s", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for preflight audit notification")
	}
}

func TestRuntimeProviderPreflightFeedbackRejectsUnknownCategoryAndScope(t *testing.T) {
	decision := sealedCandidateTestDecision(t)
	command := llmevents.PreflightRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Transaction: decision.AuthorityTransaction(),
	}
	feedback := runtimePreflightFeedbackForTest(t, command, "")
	if err := validateRuntimeProviderPreflightFeedback(command, feedback); err != nil {
		t.Fatal(err)
	}
	unknown := feedback.CloneOriginal()
	unknown.StableCategory = runtimecontract.RuntimeFeedbackCategory("unknown_preflight_category")
	unknown.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: "unknown", OriginalCode: "unknown", OriginalMessage: "unknown", Signature: "unknown",
	}
	if err := validateRuntimeProviderPreflightFeedback(command, unknown); err == nil {
		t.Fatal("unknown preflight category was accepted")
	}
	mismatched := feedback.CloneOriginal()
	mismatched.AuthoritySnapshotID = "authority-other"
	if err := validateRuntimeProviderPreflightFeedback(command, mismatched); err == nil {
		t.Fatal("authority-mismatched preflight feedback was accepted")
	}
}

func TestRequestRuntimeCandidatePreservesSealedReadOnlyMCPMetadata(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	llm := llmmodule.New()
	if err := llm.Configure(llmbiz.Dependencies{
		EventHub: hub,
		CandidateProvider: eventCandidateProviderFunc(func(context.Context, llmmodels.RuntimeCandidateRequest) (llmmodels.RuntimeCandidateGenerationResult, error) {
			return eventGenerationResult([]byte(`{"operations":[{"id":"mcp-read","kind":"mcp/tool_call","parameters":[{"name":"serverId","string":"mcp-1"},{"name":"tool","string":"repo.summary"},{"name":"input.path","string":"."}]}]}`), nil)
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)

	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{{
			ID: "mcp-1:tool:repo.summary", ServerID: "mcp-1", Kind: "tool", Name: "repo.summary", Enabled: true,
			EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
		}},
	}
	binding := eventTestCoverageBinding(runtimecontract.CoverageEvidenceBinding{
		ID: "binding/test/focus-1/cwi-1", CoverageWorkItemID: "cwi-1", FocusID: "focus-1", DefinitionRevision: "definition-1",
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "mcp-read",
		}},
		TargetSelectorRef: "test/mcp-read", TargetSelectorOwnerRef: "test-llm", TargetSelectorSubjectRef: "focus-1",
		DeclarationRef: "test/runtime-candidate-event-adapter",
	})
	facts := runtimecontract.RuntimeFacts{
		Activation: activation, WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{binding},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
	}
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedValidation, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{runtimecontract.StepKindMCPToolCall, runtimecontract.RuntimeBlockKindManagedValidationSession},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	context := candidateContextForDecision(decision)
	context.Activation = activation
	context.AdmissibleOperationKinds = runtimecontract.FeasibleOperationKindsWithActivation(decision.State, decision.RequiredOutput, context.Capabilities, activation.MCPCapabilities)
	response, err := RequestRuntimeCandidate(ctx, hub, llmevents.RequestRuntimeCandidateCommand{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		WorkspaceID:  "ws-1", RunID: "run-1", RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "candidate-request-mcp", Sequence: 1, PhysicalCallID: "candidate-call-mcp"},
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-mcp",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{runtimecontract.StepKindMCPToolCall, runtimecontract.RuntimeBlockKindManagedValidationSession}},
		CandidateManifest: candidateManifestForEventRequest(t, decision, context), CandidateContext: context,
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(candidateManifestForEventRequest(t, decision, context), response.DocumentContent)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Validation == nil || len(candidate.Validation.Operations) != 1 || candidate.Validation.Operations[0].Operation.Kind != runtimecontract.StepKindMCPToolCall {
		t.Fatalf("read-only MCP validation document = %s", response.DocumentContent)
	}
}

func TestRequestRuntimeCandidateRejectsScopeDriftBeforeEventHub(t *testing.T) {
	decision := sealedCandidateTestDecision(t)
	ctx := context.Background()
	hub := magicevent.NewHub(1)
	defer hub.Terminate(ctx)
	_, err := RequestRuntimeCandidate(ctx, hub, llmevents.RequestRuntimeCandidateCommand{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		WorkspaceID:  "ws-other", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
	})
	if err == nil {
		t.Fatal("scope drift was accepted")
	}
}

func TestRequestRuntimeCandidateRejectsRetryWithoutPreviousFailure(t *testing.T) {
	decision := sealedCandidateTestDecision(t)
	ctx := context.Background()
	hub := magicevent.NewHub(1)
	defer hub.Terminate(ctx)
	_, err := RequestRuntimeCandidate(ctx, hub, llmevents.RequestRuntimeCandidateCommand{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		WorkspaceID:  "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1, StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContextForDecision(decision)), CandidateContext: candidateContextForDecision(decision),
	})
	if err == nil {
		t.Fatal("candidate retry without previous failure was accepted")
	}
}

func TestRequestRuntimeCandidateRejectsDivergentSealedContextBeforeEventHub(t *testing.T) {
	decision := sealedCandidateTestDecision(t)
	candidateContext := candidateContextForDecision(decision)
	candidateContext.RequiredState = runtimecontract.ConvergenceStateNeedValidation
	hub := magicevent.NewHub(1)
	defer hub.Terminate(context.Background())
	_, err := RequestRuntimeCandidate(context.Background(), hub, llmevents.RequestRuntimeCandidateCommand{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		WorkspaceID:  "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContext), CandidateContext: candidateContext,
	})
	if err == nil || !strings.Contains(err.Error(), "required state") {
		t.Fatalf("divergent sealed context err = %v", err)
	}
}

func sealedCandidateTestDecision(t *testing.T) runtimecontract.RuntimeDecision {
	t.Helper()
	binding := eventTestCoverageBinding(runtimecontract.CoverageEvidenceBinding{
		ID: "binding/test/focus-1/cwi-1", CoverageWorkItemID: "cwi-1", FocusID: "focus-1", DefinitionRevision: "definition-1",
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind:          runtimecontract.EvidenceKindStateChange,
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
		}},
		TargetRefs:     []string{"main.go"},
		DeclarationRef: "test/runtime-candidate-event-adapter",
	})
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{binding},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
	}
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{"patch/apply"},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

func eventTestCoverageBinding(binding runtimecontract.CoverageEvidenceBinding) runtimecontract.CoverageEvidenceBinding {
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	return binding
}

func candidateContextForDecision(decision runtimecontract.RuntimeDecision) runtimecontract.RuntimeCandidateContext {
	required := decision.RequiredOutput.Normalized()
	context := runtimecontract.RuntimeCandidateContext{
		WorkspaceID: decision.AuthoritySnapshot.WorkspaceID, RunID: decision.AuthoritySnapshot.RunID,
		DefinitionRevision: decision.AuthoritySnapshot.DefinitionRevision, EvidenceRevision: decision.AuthoritySnapshot.EvidenceRevision,
		Focus:                 decision.Focus,
		Activation:            runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		RequiredState:         decision.State,
		RequiredEffectScopes:  append([]runtimecontract.RuntimeEffectScope(nil), required.RequiredEffectScopes...),
		RequiredEvidenceKinds: append([]string(nil), required.RequiredEvidenceKinds...),
	}
	context.AdmissibleOperationKinds = runtimecontract.FeasibleOperationKindsWithActivation(decision.State, decision.RequiredOutput, context.Capabilities, context.Activation.MCPCapabilities)
	return context
}

var _ llmruntime.CandidateProvider = eventCandidateProviderFunc(nil)
