package orchestrator

// Owner Biz candidate adapter tests.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	llmruntimev2 "github.com/muidea/workorch/internal/modules/blocks/llm/biz/runtimev2"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
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
		Usage:   &llmmodels.ProviderUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
		Audit: &llmmodels.ProviderRequestAudit{
			Status:            llmmodels.RuntimeCandidateStatusCompleted,
			ProviderSubmitted: true,
			Target:            runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			Operation:         "chat_completions",
		},
	}, nil
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
	if err := llm.Configure(llmmodule.Dependencies{
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
	response, err := RequestRuntimeCandidate(ctx, hub, orchestratorevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", LogicalRequestID: "candidate-request-provider-failure", RequestSequence: 1, PhysicalCallID: "candidate-call-provider-failure",
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContextForDecision(decision)), CandidateContext: candidateContextForDecision(decision),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Failure == nil || response.Failure.Class != runtimecontract.RuntimeCandidateFailureClassProvider || len(response.DocumentContent) != 0 {
		t.Fatalf("response = %+v", response)
	}
}

func TestRequestRuntimeCandidateForwardsCompleteSchemaRejectionDocumentOnly(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	llm := llmmodule.New()
	payload := []byte(`{"operations":[],"unexpected":true}`)
	if err := llm.Configure(llmmodule.Dependencies{
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
	response, err := RequestRuntimeCandidate(ctx, hub, orchestratorevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", LogicalRequestID: "candidate-request-complete-schema", RequestSequence: 1, PhysicalCallID: "candidate-call-complete-schema",
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContextForDecision(decision)), CandidateContext: candidateContextForDecision(decision),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Failure != nil || string(response.DocumentContent) != string(payload) {
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
	if err := llm.Configure(llmmodule.Dependencies{
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
	response, err := RequestRuntimeCandidate(ctx, hub, orchestratorevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", LogicalRequestID: "candidate-request-stale", RequestSequence: 1, PhysicalCallID: "candidate-call-stale",
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContextForDecision(decision)), CandidateContext: candidateContextForDecision(decision),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Stale == nil || response.Stale.ObservationID != observation.ObservationID || response.Failure != nil || len(response.DocumentContent) != 0 {
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
	if err := llm.Configure(llmmodule.Dependencies{
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

	base := orchestratorevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", LogicalRequestID: "candidate-request-1", RequestSequence: 1, PhysicalCallID: "physical-call-1",
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
	retry.RequestSequence = 2
	retry.PhysicalCallID = "physical-call-2"
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
	if first.LogicalRequestID != base.LogicalRequestID || first.RequestSequence != 1 || first.PhysicalCallID != "physical-call-1" || second.LogicalRequestID != base.LogicalRequestID || second.RequestSequence != 2 || second.PhysicalCallID != "physical-call-2" {
		t.Fatalf("candidate response identity was not preserved: first=%+v second=%+v", first, second)
	}
	if len(captured) != 2 || captured[0].LogicalRequestID != base.LogicalRequestID || captured[0].RequestSequence != 1 || captured[0].PhysicalCallID != "physical-call-1" || captured[1].LogicalRequestID != base.LogicalRequestID || captured[1].RequestSequence != 2 || captured[1].PhysicalCallID != "physical-call-2" || captured[0].CandidateAttempt != 0 || captured[0].PreviousFailure != nil || captured[1].CandidateAttempt != 1 || captured[1].PreviousFailure == nil || captured[1].PreviousFailure.Code != "candidate_schema_invalid" || captured[1].PreviousFailure.Diagnostic == nil || captured[1].PreviousFailure.Diagnostic.Path != "$.unexpected" || captured[0].Transaction != captured[1].Transaction || captured[0].StablePrefixHash != captured[1].StablePrefixHash || captured[0].CandidateManifest.Hash != base.CandidateManifest.Hash || captured[1].CandidateManifest.Hash != base.CandidateManifest.Hash || captured[0].Decision.AuthoritySnapshot.RequiredOutputHash != captured[1].Decision.AuthoritySnapshot.RequiredOutputHash {
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
	_, err := RequestRuntimeCandidate(ctx, hub, orchestratorevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", LogicalRequestID: "candidate-request-manifest-drift", RequestSequence: 1, PhysicalCallID: "candidate-call-manifest-drift",
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
			_, err := RequestRuntimeCandidate(context.Background(), magicevent.NewHub(1), orchestratorevents.RequestRuntimeCandidateCommand{
				WorkspaceID: "ws-1", RunID: "run-1", LogicalRequestID: identity.logical, RequestSequence: identity.sequence, PhysicalCallID: identity.physical,
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
	if err := llm.Configure(llmmodule.Dependencies{
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
		Target:                   runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
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
	response, err := PreflightRuntimeCandidate(ctx, hub, orchestratorevents.PreflightRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		DefinitionContext: definition,
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContext), CandidateContext: candidateContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !response.Ready || response.Audit == nil || response.Audit.ProviderSubmitted || providerCalls != 0 {
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
	if err := llm.Configure(llmmodule.Dependencies{
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
		Target:                   runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
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
	response, err := PreflightRuntimeCandidate(ctx, hub, orchestratorevents.PreflightRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		CandidateManifest: candidateManifestForEventRequest(t, decision, candidateContext), DefinitionContext: definition, CandidateContext: candidateContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Ready || response.Reason == "" || response.Audit == nil || response.Audit.ProviderSubmitted || providerCalls != 0 {
		t.Fatalf("not-ready preflight response=%+v providerCalls=%d", response, providerCalls)
	}
	budget := response.Audit.ContextBudget
	if budget.ContextGateAction != "fail_fast" || budget.ContextGateReason != "runtime_v2_provider_ceiling_exceeded" || budget.BudgetFailureClass != "provider_ceiling_exceeded" || len(budget.SectionHashes) == 0 || len(budget.SourceRefs) == 0 {
		t.Fatalf("not-ready preflight audit=%+v", budget)
	}

	select {
	case recorded := <-audits:
		if recorded.ProviderSubmitted || recorded.RequestID == "" || recorded.StepID != decision.AuthoritySnapshot.ID || recorded.ContextBudget.ContextGateReason != "runtime_v2_provider_ceiling_exceeded" || len(recorded.ContextBudget.SectionHashes) == 0 {
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

func TestRequestRuntimeCandidatePreservesSealedReadOnlyMCPMetadata(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	llm := llmmodule.New()
	if err := llm.Configure(llmmodule.Dependencies{
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
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
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
	response, err := RequestRuntimeCandidate(ctx, hub, orchestratorevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", LogicalRequestID: "candidate-request-mcp", RequestSequence: 1, PhysicalCallID: "candidate-call-mcp",
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
	_, err := RequestRuntimeCandidate(ctx, hub, orchestratorevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-other", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
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
	_, err := RequestRuntimeCandidate(ctx, hub, orchestratorevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1, StablePrefixHash: "stable-prefix-1",
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
	_, err := RequestRuntimeCandidate(context.Background(), hub, orchestratorevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
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
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
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
		Activation:            runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		RequiredState:         decision.State,
		RequiredEffectScopes:  append([]runtimecontract.RuntimeEffectScope(nil), required.RequiredEffectScopes...),
		RequiredEvidenceKinds: append([]string(nil), required.RequiredEvidenceKinds...),
	}
	context.AdmissibleOperationKinds = runtimecontract.FeasibleOperationKindsWithActivation(decision.State, decision.RequiredOutput, context.Capabilities, context.Activation.MCPCapabilities)
	return context
}

var _ llmruntimev2.CandidateProvider = eventCandidateProviderFunc(nil)
