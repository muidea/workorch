package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeCandidateDefinitionProjection struct {
	Revision       string                                         `json:"revision"`
	GoalID         common.GoalID                                  `json:"goalId"`
	GoalDefinition string                                         `json:"goalDefinition"`
	DoDID          common.DoDID                                   `json:"doDId"`
	DoDDefinition  string                                         `json:"doDDefinition"`
	ChangeImpact   *runtimecontract.RuntimeDefinitionChangeImpact `json:"changeImpact,omitempty"`
}

type runtimeCandidateResourceProjection struct {
	Manifest          *runtimecontract.WorkspaceManifest `json:"manifest,omitempty"`
	TargetSnapshots   []runtimecontract.TargetSnapshot   `json:"targetSnapshots,omitempty"`
	FailureTarget     *runtimecontract.FailureTarget     `json:"failureTarget,omitempty"`
	ValidationScratch *runtimecontract.ValidationScratch `json:"validationScratch,omitempty"`
}

type runtimeCandidateActivationProjection struct {
	DialogLanguage   string                                        `json:"dialogLanguage,omitempty"`
	CurrentStage     string                                        `json:"currentStage"`
	Soul             *runtimecontract.RuntimeSoulContext           `json:"soul,omitempty"`
	Skills           []runtimecontract.RuntimeSkillContext         `json:"skills,omitempty"`
	MCPServerIDs     []common.MCPServerID                          `json:"mcpServerIds,omitempty"`
	MCPCapabilities  []runtimecontract.RuntimeMCPCapabilityContext `json:"mcpCapabilities,omitempty"`
	Memories         []runtimecontract.RuntimeMemoryContext        `json:"memories,omitempty"`
	EvidencePolicies []runtimecontract.RuntimeEvidencePolicy       `json:"evidencePolicies,omitempty"`
	Continuation     runtimecontract.RuntimeContinuationContext    `json:"continuation"`
}

type runtimeCandidateContextProjection struct {
	WorkspaceID        common.WorkspaceID                      `json:"workspaceId"`
	RunID              common.RunID                            `json:"runId"`
	DefinitionRevision string                                  `json:"definitionRevision"`
	EvidenceRevision   string                                  `json:"evidenceRevision"`
	Focus              runtimecontract.CoverageFocus           `json:"focus"`
	Evidence           runtimecontract.EvidenceLedger          `json:"evidence"`
	Assessments        []runtimecontract.RuntimeAssessmentFact `json:"assessments,omitempty"`
	Failure            *runtimecontract.FailureFact            `json:"failure,omitempty"`
	Capabilities       runtimecontract.RuntimeCapabilities     `json:"capabilities"`
	Resources          runtimeCandidateResourceProjection      `json:"resources"`
	Activation         runtimeCandidateActivationProjection    `json:"activation"`
}

type runtimeCandidateOutputContractProjection struct {
	Action                  runtimecontract.RuntimeAction                 `json:"action"`
	Schema                  json.RawMessage                               `json:"schema"`
	RequiredTopLevelExample string                                        `json:"requiredTopLevelExample"`
	ForbiddenTopLevelFields []string                                      `json:"forbiddenTopLevelFields"`
	OperationContracts      []runtimeCandidateOperationContractProjection `json:"operationContracts,omitempty"`
}

type runtimeCandidateOperationContractProjection struct {
	Kind                string                                         `json:"kind"`
	Parameters          []runtimeCandidateOperationParameterProjection `json:"parameters"`
	ForbiddenParameters []string                                       `json:"forbiddenParameters,omitempty"`
	Rules               []string                                       `json:"rules,omitempty"`
}

type runtimeCandidateOperationParameterProjection struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type runtimeCandidateRetryProjection struct {
	PreviousFailure runtimecontract.RuntimeCandidateFailure `json:"previousFailure"`
	Instruction     string                                  `json:"instruction"`
}

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
	definitionContext := request.DefinitionContext.Normalized()
	candidateContext := request.CandidateContext.Normalized()
	wantPrefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definitionContext, candidateContext.Activation.ExecutionContract)
	if strings.TrimSpace(request.StablePrefixHash) != wantPrefix {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate stable prefix mismatch")
	}
	ceiling := candidateContext.Resources.ProviderCeiling
	if ceiling == nil || strings.TrimSpace(ceiling.ProviderID) == "" || ceiling.MaxBytes <= 0 {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime candidate provider ceiling is missing")
	}
	definitionJSON, err := json.Marshal(runtimeCandidateDefinitionProjection{
		Revision: definitionContext.Revision, GoalID: definitionContext.GoalID,
		GoalDefinition: definitionContext.GoalDefinition, DoDID: definitionContext.DoDID,
		DoDDefinition: definitionContext.DoDDefinition, ChangeImpact: definitionContext.ChangeImpact,
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	capabilityJSON, err := json.Marshal(struct {
		Kinds             []string                                 `json:"kinds"`
		ExecutionContract runtimecontract.RuntimeExecutionContract `json:"executionContract"`
	}{definitionContext.CapabilityContract, candidateContext.Activation.ExecutionContract})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	dynamicJSON, err := json.Marshal(struct {
		Decision       runtimecontract.RuntimeDecision          `json:"decision"`
		Transaction    runtimecontract.AuthorityTransaction     `json:"transaction"`
		Attempt        int                                      `json:"candidateAttempt"`
		OutputContract runtimeCandidateOutputContractProjection `json:"outputContract"`
		Retry          *runtimeCandidateRetryProjection         `json:"retry,omitempty"`
		Context        runtimeCandidateContextProjection        `json:"context"`
	}{
		request.Decision,
		request.Transaction,
		request.CandidateAttempt,
		runtimeV2CandidateOutputContract(request.Decision),
		runtimeV2CandidateRetry(request.CandidateAttempt, request.PreviousFailure),
		runtimeV2CandidateContextProjection(candidateContext, request.Decision.Action),
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest := llmmodels.ProviderRequest{
		WorkspaceID: string(candidateContext.WorkspaceID), RunID: string(candidateContext.RunID),
		StepID: request.Transaction.AuthoritySnapshotID, Kind: "runtime_v2_candidate",
		Schema: runtimeV2CandidateSchema(request.Decision.Action),
		PromptContext: &llmmodels.PromptContext{SchemaVersion: "workorch.runtime_candidate_prompt.v1", Sections: []llmmodels.PromptSection{
			{ID: "C0.runtime-candidate-contract", Name: "Runtime candidate contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 0, Content: runtimeV2CandidateContractCatalog()},
			{ID: "C1.goal-dod", Name: "Goal and DoD", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 1, DefinitionRevision: definitionContext.Revision, Content: string(definitionJSON)},
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

func runtimeV2CandidateContextProjection(context runtimecontract.RuntimeCandidateContext, action runtimecontract.RuntimeAction) runtimeCandidateContextProjection {
	context = context.Normalized()
	activation := context.Activation
	stage := runtimeV2CandidateStage(action)
	return runtimeCandidateContextProjection{
		WorkspaceID: context.WorkspaceID, RunID: context.RunID,
		DefinitionRevision: context.DefinitionRevision, EvidenceRevision: context.EvidenceRevision,
		Focus: context.Focus, Evidence: context.Evidence, Assessments: context.Assessments, Failure: context.Failure,
		Capabilities: context.Capabilities,
		Resources: runtimeCandidateResourceProjection{
			Manifest: context.Resources.Manifest, TargetSnapshots: context.Resources.TargetSnapshots,
			FailureTarget: context.Resources.FailureTarget, ValidationScratch: context.Resources.ValidationScratch,
		},
		Activation: runtimeCandidateActivationProjection{
			DialogLanguage: activation.DialogLanguage, CurrentStage: stage, Soul: runtimeV2CandidateSoul(activation, stage),
			Skills: activation.Skills, MCPServerIDs: activation.MCPServerIDs, MCPCapabilities: activation.MCPCapabilities,
			Memories: activation.Memories, EvidencePolicies: activation.EvidencePolicies, Continuation: activation.Continuation,
		},
	}
}

func runtimeV2CandidateStage(action runtimecontract.RuntimeAction) string {
	switch action {
	case runtimecontract.RuntimeActionProduceStateChange:
		return "execution"
	case runtimecontract.RuntimeActionProduceValidation, runtimecontract.RuntimeActionProduceAssessment:
		return "validation"
	case runtimecontract.RuntimeActionRequestGovernance:
		return "decision"
	default:
		return ""
	}
}

func runtimeV2CandidateSoul(activation runtimecontract.RuntimeActivationContext, stage string) *runtimecontract.RuntimeSoulContext {
	for _, binding := range activation.StageSoulBindings {
		if binding.Stage != stage {
			continue
		}
		value := binding.Soul
		return &value
	}
	if activation.DefaultSoul == nil {
		return nil
	}
	value := *activation.DefaultSoul
	return &value
}

func runtimeV2CandidateSchema(action runtimecontract.RuntimeAction) string {
	switch action {
	case runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeActionProduceValidation:
		return `{"type":"object","additionalProperties":false,"required":["operations"],"properties":{"operations":{"type":"array","minItems":1,"items":{"type":"object","additionalProperties":false,"required":["id","kind","coverageWorkItemIds"],"properties":{"id":{"type":"string"},"kind":{"type":"string"},"dependsOn":{"type":"array","items":{"type":"string"}},"coverageWorkItemIds":{"type":"array","minItems":1,"items":{"type":"string"}},"targetRefs":{"type":"array","items":{"type":"string"}},"parameters":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["name"],"properties":{"name":{"type":"string"},"string":{"type":"string"},"integer":{"type":"integer"},"boolean":{"type":"boolean"},"stringList":{"type":"array","items":{"type":"string"}}}}}}}}}}`
	case runtimecontract.RuntimeActionProduceAssessment:
		return `{"type":"object","additionalProperties":false,"required":["focusId","coverageWorkItemIds","satisfied","summary"],"properties":{"focusId":{"type":"string"},"coverageWorkItemIds":{"type":"array","minItems":1,"items":{"type":"string"}},"satisfied":{"type":"boolean"},"summary":{"type":"string"},"evidenceRefs":{"type":"array","items":{"type":"string"}},"artifactRefs":{"type":"array","items":{"type":"string"}}}}`
	case runtimecontract.RuntimeActionRequestGovernance:
		return `{"type":"object","additionalProperties":false,"required":["externalDependency"],"properties":{"externalDependency":{"type":"object","additionalProperties":false,"required":["kind","reason"],"properties":{"kind":{"type":"string"},"subjectRef":{"type":"string"},"reason":{"type":"string"}}}}}`
	default:
		return ""
	}
}

func runtimeV2CandidateContractCatalog() string {
	return strings.Join([]string{
		"Return exactly one JSON object for the requested Runtime V2 action. The JSON object itself is the candidate; never wrap it in another object.",
		"The only legal top-level shapes are:",
		"produce_state_change or produce_validation: " + runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceStateChange),
		"produce_assessment: " + runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceAssessment),
		"request_governance: " + runtimeV2CandidateSchema(runtimecontract.RuntimeActionRequestGovernance),
		"Forbidden top-level fields include candidate, candidates, executionGraph, convergenceState, focus, authority, transaction, outputMode, result, and runtimeState.",
		"Use the requested action, required output kinds, coverage work item IDs, and available capabilities from the current runtime authority. Do not invent or modify convergence state, focus, authority, transaction, or output mode. Do not use markdown fences or explanatory text.",
	}, "\n")
}

func runtimeV2CandidateOutputContract(decision runtimecontract.RuntimeDecision) runtimeCandidateOutputContractProjection {
	return runtimeCandidateOutputContractProjection{
		Action:                  decision.Action,
		Schema:                  json.RawMessage(runtimeV2CandidateSchema(decision.Action)),
		RequiredTopLevelExample: runtimeV2CandidateTopLevelExample(decision.Action),
		ForbiddenTopLevelFields: []string{"candidate", "candidates", "executionGraph", "convergenceState", "focus", "authority", "transaction", "outputMode", "result", "runtimeState"},
		OperationContracts:      runtimeV2CandidateOperationContracts(decision.RequiredOutput.AcceptedKinds),
	}
}

func runtimeV2CandidateOperationContracts(kinds []string) []runtimeCandidateOperationContractProjection {
	ret := make([]runtimeCandidateOperationContractProjection, 0, len(kinds))
	for _, kind := range kinds {
		contract, ok := runtimecontract.RuntimeOperationContractForKind(kind)
		if !ok {
			continue
		}
		projection := runtimeCandidateOperationContractProjection{
			Kind: contract.Kind, ForbiddenParameters: append([]string(nil), contract.ForbiddenParameters...), Rules: append([]string(nil), contract.Rules...),
			Parameters: make([]runtimeCandidateOperationParameterProjection, 0, len(contract.Parameters)),
		}
		for _, parameter := range contract.Parameters {
			projection.Parameters = append(projection.Parameters, runtimeCandidateOperationParameterProjection{
				Name: parameter.Name, Type: string(parameter.Type), Required: parameter.Required, Description: parameter.Description,
			})
		}
		ret = append(ret, projection)
	}
	return ret
}

func runtimeV2CandidateTopLevelExample(action runtimecontract.RuntimeAction) string {
	switch action {
	case runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeActionProduceValidation:
		return `{"operations":[...]}`
	case runtimecontract.RuntimeActionProduceAssessment:
		return `{"focusId":"...","coverageWorkItemIds":["..."],"satisfied":false,"summary":"..."}`
	case runtimecontract.RuntimeActionRequestGovernance:
		return `{"externalDependency":{"kind":"...","reason":"..."}}`
	default:
		return `{}`
	}
}

func runtimeV2CandidateRetry(attempt int, failure *runtimecontract.RuntimeCandidateFailure) *runtimeCandidateRetryProjection {
	if attempt != 1 || failure == nil {
		return nil
	}
	previous := *failure
	previous.Reason = boundedRuntimeCandidateFailureReason(previous.Reason)
	instruction := "Retry the exact same sealed authority contract. Return only the legal top-level object for the requested action."
	if previous.Class == runtimecontract.RuntimeCandidateFailureClassSchema || previous.Class == runtimecontract.RuntimeCandidateFailureClassContract {
		instruction += " The previous candidate shape was rejected; do not repeat any wrapper or field named candidate, candidates, or executionGraph."
	}
	return &runtimeCandidateRetryProjection{PreviousFailure: previous, Instruction: instruction}
}

func boundedRuntimeCandidateFailureReason(reason string) string {
	const maxBytes = 1024
	reason = strings.TrimSpace(reason)
	if len(reason) <= maxBytes {
		return reason
	}
	return reason[:maxBytes] + "...[truncated]"
}
