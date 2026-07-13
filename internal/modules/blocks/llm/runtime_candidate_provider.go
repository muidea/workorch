package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Module) GenerateCandidate(ctx context.Context, request llmmodels.RuntimeCandidateRequest) ([]byte, error) {
	if m == nil || m.providerConfigLoader == nil {
		return nil, fmt.Errorf("runtime candidate provider is not configured")
	}
	providerRequest, err := runtimeV2ProviderRequest(request)
	if err != nil {
		return nil, err
	}
	providerID := request.CandidateContext.Resources.ProviderCeiling.ProviderID
	completion, _, _, err := m.completeProvider(ctx, providerID, providerRequest)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), completion.Payload...), nil
}

func runtimeV2ProviderRequest(request llmmodels.RuntimeCandidateRequest) (llmmodels.ProviderRequest, error) {
	wantPrefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(request.DefinitionContext, request.CandidateContext.Activation.ExecutionContract)
	if strings.TrimSpace(request.StablePrefixHash) != wantPrefix {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate stable prefix mismatch")
	}
	ceiling := request.CandidateContext.Resources.ProviderCeiling
	if ceiling == nil || strings.TrimSpace(ceiling.ProviderID) == "" || ceiling.MaxBytes <= 0 {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate provider ceiling is missing")
	}
	definitionJSON, err := json.Marshal(request.DefinitionContext)
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	capabilityJSON, err := json.Marshal(struct {
		Kinds             []string                                 `json:"kinds"`
		ExecutionContract runtimecontract.RuntimeExecutionContract `json:"executionContract"`
	}{request.DefinitionContext.CapabilityContract, request.CandidateContext.Activation.ExecutionContract})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	dynamicJSON, err := json.Marshal(struct {
		Decision    runtimecontract.RuntimeDecision         `json:"decision"`
		Transaction runtimecontract.AuthorityTransaction    `json:"transaction"`
		Attempt     int                                     `json:"candidateAttempt"`
		Context     runtimecontract.RuntimeCandidateContext `json:"context"`
	}{request.Decision, request.Transaction, request.CandidateAttempt, request.CandidateContext})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest := llmmodels.ProviderRequest{
		WorkspaceID: string(request.CandidateContext.WorkspaceID), RunID: string(request.CandidateContext.RunID),
		StepID: request.Transaction.AuthoritySnapshotID, Kind: "runtime_v2_candidate",
		Schema: runtimeV2CandidateSchema(request.Decision.Action),
		PromptContext: &llmmodels.PromptContext{SchemaVersion: "workorch.runtime_candidate_prompt.v1", Sections: []llmmodels.PromptSection{
			{ID: "C0.runtime-candidate-contract", Name: "Runtime candidate contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 0, Content: "Return exactly one JSON candidate matching the requested action. Do not return or modify convergence state, focus, authority, transaction, or output mode."},
			{ID: "C1.goal-dod", Name: "Goal and DoD", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 1, DefinitionRevision: request.DefinitionContext.Revision, Content: string(definitionJSON)},
			{ID: "C2.capability-contract", Name: "Capability contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 2, Content: string(capabilityJSON)},
			{ID: "C3.runtime-authority", Name: "Current runtime authority", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 3, Content: string(dynamicJSON)},
		}},
		ContextBudget: &llmmodels.ProviderContextBudget{MaxBytes: ceiling.MaxBytes, MaxRequestBytes: ceiling.MaxBytes, ProfileMode: "runtime_v2_typed_ceiling", CachePlan: &llmmodels.ProviderCachePlan{
			SchemaVersion: "workorch.provider_cache_plan.v1", Strategy: "stable_prefix_dynamic_tail",
			StablePrefixHash: wantPrefix, RenderedStablePrefixHash: wantPrefix,
		}},
	}
	payload, err := json.Marshal(providerRequest)
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest.ContextBudget.ActualBytes = len(payload)
	providerRequest.ContextBudget.OverBudget = len(payload) > ceiling.MaxBytes
	if providerRequest.ContextBudget.OverBudget {
		providerRequest.ContextBudget.ContextGateAction = "fail_fast"
		providerRequest.ContextBudget.ContextGateReason = "runtime_v2_provider_ceiling_exceeded"
		providerRequest.ContextBudget.BudgetFailureClass = "provider_ceiling_exceeded"
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate request exceeds provider ceiling: %d > %d", len(payload), ceiling.MaxBytes)
	}
	return providerRequest, nil
}

func runtimeV2CandidateSchema(action runtimecontract.RuntimeAction) string {
	switch action {
	case runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeActionProduceValidation:
		return `{"type":"object","additionalProperties":false,"required":["operations"],"properties":{"operations":{"type":"array","minItems":1,"items":{"type":"object","required":["id","kind","coverageWorkItemIds"],"properties":{"id":{"type":"string"},"kind":{"type":"string"},"dependsOn":{"type":"array","items":{"type":"string"}},"coverageWorkItemIds":{"type":"array","items":{"type":"string"}},"targetRefs":{"type":"array","items":{"type":"string"}},"parameters":{"type":"array"}}}}}}`
	case runtimecontract.RuntimeActionProduceAssessment:
		return `{"type":"object","additionalProperties":false,"required":["focusId","coverageWorkItemIds","satisfied","summary"],"properties":{"focusId":{"type":"string"},"coverageWorkItemIds":{"type":"array","minItems":1,"items":{"type":"string"}},"satisfied":{"type":"boolean"},"summary":{"type":"string"},"evidenceRefs":{"type":"array","items":{"type":"string"}},"artifactRefs":{"type":"array","items":{"type":"string"}}}}`
	case runtimecontract.RuntimeActionRequestGovernance:
		return `{"type":"object","additionalProperties":false,"required":["externalDependency"],"properties":{"externalDependency":{"type":"object","additionalProperties":false,"required":["kind","reason"],"properties":{"kind":{"type":"string"},"subjectRef":{"type":"string"},"reason":{"type":"string"}}}}}`
	default:
		return ""
	}
}
