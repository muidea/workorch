package orchestrator

// Owner Biz candidate adapter tests.

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	llmruntimev2 "github.com/muidea/workorch/internal/modules/blocks/llm/biz/runtimev2"
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
		WorkspaceID: "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateContext:  candidateContextForDecision(decision),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Failure == nil || response.Failure.Class != runtimecontract.RuntimeCandidateFailureClassProvider || response.Candidate.StateChange != nil {
		t.Fatalf("response = %+v", response)
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
		WorkspaceID: "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateContext:  candidateContextForDecision(decision),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Stale == nil || response.Stale.ObservationID != observation.ObservationID || response.Failure != nil || response.Candidate.StateChange != nil {
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
			return eventGenerationResult([]byte(`{"operations":[{"id":"op-1","kind":"patch/apply","coverageWorkItemIds":["cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"targetRef","string":"main.go"},{"name":"patchType","string":"file_write"},{"name":"content","string":"package main\n"}]}]}`), nil)
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Stop(ctx)

	base := orchestratorevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-1",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply"}},
		CandidateContext:  candidateContextForDecision(decision),
	}
	first, err := RequestRuntimeCandidate(ctx, hub, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Candidate.Validate(decision); err != nil {
		t.Fatal(err)
	}
	retry := base
	retry.CandidateAttempt = 1
	retry.PreviousFailure = &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassSchema, Reason: `json: unknown field "candidate"`}
	second, err := RequestRuntimeCandidate(ctx, hub, retry)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Candidate.Validate(decision); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 2 || captured[1].PreviousFailure == nil || captured[1].PreviousFailure.Reason != `json: unknown field "candidate"` || captured[0].Transaction != captured[1].Transaction || captured[0].StablePrefixHash != captured[1].StablePrefixHash || captured[0].Decision.AuthoritySnapshot.RequiredOutputHash != captured[1].Decision.AuthoritySnapshot.RequiredOutputHash {
		t.Fatalf("candidate retries changed sealed contract: %+v", captured)
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
			return eventGenerationResult([]byte(`{"operations":[{"id":"mcp-read","kind":"mcp/tool_call","coverageWorkItemIds":["cwi-1"],"parameters":[{"name":"serverId","string":"mcp-1"},{"name":"tool","string":"repo.summary"},{"name":"input.path","string":"."}]}]}`), nil)
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
	facts := runtimecontract.RuntimeFacts{
		Activation: activation, WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus:    runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
	}
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedValidation, Action: runtimecontract.RuntimeActionProduceValidation, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			Kind: runtimecontract.RequiredOutputKindValidation, AcceptedKinds: []string{runtimecontract.StepKindMCPToolCall},
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
	response, err := RequestRuntimeCandidate(ctx, hub, orchestratorevents.RequestRuntimeCandidateCommand{
		WorkspaceID: "ws-1", RunID: "run-1", Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: "stable-prefix-mcp",
		DefinitionContext: runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{runtimecontract.StepKindMCPToolCall}},
		CandidateContext:  context,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Candidate.Validation == nil || len(response.Candidate.Validation.Operations) != 1 || response.Candidate.Validation.Operations[0].Operation.Kind != runtimecontract.StepKindMCPToolCall {
		t.Fatalf("read-only MCP validation candidate = %+v", response.Candidate)
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
		CandidateContext:  candidateContextForDecision(decision),
	})
	if err == nil {
		t.Fatal("candidate retry without previous failure was accepted")
	}
}

func sealedCandidateTestDecision(t *testing.T) runtimecontract.RuntimeDecision {
	t.Helper()
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus:    runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
	}
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Action: runtimecontract.RuntimeActionProduceStateChange, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			Kind:                 runtimecontract.RequiredOutputKindStateChange,
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

func candidateContextForDecision(decision runtimecontract.RuntimeDecision) runtimecontract.RuntimeCandidateContext {
	return runtimecontract.RuntimeCandidateContext{
		WorkspaceID: decision.AuthoritySnapshot.WorkspaceID, RunID: decision.AuthoritySnapshot.RunID,
		DefinitionRevision: decision.AuthoritySnapshot.DefinitionRevision, EvidenceRevision: decision.AuthoritySnapshot.EvidenceRevision,
		Focus:      decision.Focus,
		Activation: runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
	}
}

var _ llmruntimev2.CandidateProvider = eventCandidateProviderFunc(nil)
