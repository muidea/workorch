package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeSemanticCapabilityProjection struct {
	CapabilityKinds   []string                                 `json:"capabilityKinds"`
	ExecutionContract runtimecontract.RuntimeExecutionContract `json:"executionContract"`
}

type runtimeSemanticAuthorityProjection struct {
	WorkspaceID       string                                      `json:"workspaceId"`
	RunID             string                                      `json:"runId"`
	DefinitionSeed    runtimecontract.RuntimeDefinitionSeed       `json:"definitionSeed"`
	Manifest          runtimecontract.RuntimeSemanticPlanManifest `json:"manifest"`
	Schema            json.RawMessage                             `json:"schema"`
	OutputInstruction string                                      `json:"outputInstruction"`
	PlanStep          runtimecontract.RuntimeSemanticPlanStep     `json:"planStep"`
	BatchID           string                                      `json:"batchId,omitempty"`
	BatchCoverageIDs  []common.CoverageWorkItemID                 `json:"batchCoverageWorkItemIds,omitempty"`
}

type runtimeSemanticDiagnosticProjection struct {
	OwnerModuleID    string                                  `json:"ownerModuleId"`
	Class            string                                  `json:"class"`
	Code             string                                  `json:"code"`
	Signature        string                                  `json:"signature"`
	OperationID      string                                  `json:"operationId,omitempty"`
	TargetRefs       []string                                `json:"targetRefs,omitempty"`
	ExpectedRevision string                                  `json:"expectedRevision,omitempty"`
	ObservedRevision string                                  `json:"observedRevision,omitempty"`
	FieldViolations  []runtimecontract.RuntimeFieldViolation `json:"fieldViolations,omitempty"`
	OmittedReason    string                                  `json:"omittedReason,omitempty"`
	Truncated        bool                                    `json:"truncated,omitempty"`
	Redacted         bool                                    `json:"redacted,omitempty"`
}

type runtimeSemanticTailProjection struct {
	Attempt            int                                                        `json:"attempt"`
	PreviousRejection  *runtimeSemanticDiagnosticProjection                       `json:"previousRejection,omitempty"`
	OutputContinuation *runtimecontract.RuntimeCandidateOutputContinuationRequest `json:"outputContinuation,omitempty"`
}

func runtimeSemanticPlanProviderRequest(request llmmodels.RuntimeCandidateRequest) (llmmodels.ProviderRequest, error) {
	if request.CandidateManifest.Hash != "" || request.CandidateContext.WorkspaceID != "" || request.CandidateContext.RunID != "" || request.DefinitionContext.Revision != "" || request.PreviousFailure != nil {
		return llmmodels.ProviderRequest{}, fmt.Errorf("semantic plan provider request must not carry operations fields")
	}
	context := request.SemanticContext.Clone()
	manifest := request.SemanticManifest.Clone()
	if err := context.Validate(manifest); err != nil {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime semantic plan context: %w", err)
	}
	if request.CandidateAttempt < 0 || request.CandidateAttempt > 1 {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime semantic plan attempt must be 0 or 1")
	}
	if request.CandidateAttempt == 0 && request.PreviousSemanticRejection != nil {
		return llmmodels.ProviderRequest{}, fmt.Errorf("initial runtime semantic plan request must not carry a previous rejection")
	}
	if request.CandidateAttempt == 1 {
		if request.PreviousSemanticRejection == nil {
			return llmmodels.ProviderRequest{}, fmt.Errorf("runtime semantic plan correction requires the previous rejection")
		}
		if err := request.PreviousSemanticRejection.Validate(); err != nil {
			return llmmodels.ProviderRequest{}, err
		}
	}
	wantPrefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(context.Definition, context.Activation.ExecutionContract)
	if strings.TrimSpace(request.StablePrefixHash) != wantPrefix {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime semantic plan stable prefix mismatch")
	}
	ceiling := context.ProviderCeiling
	if ceiling == nil || ceiling.MaxBytes <= 0 {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime semantic plan provider ceiling is missing")
	}
	if err := ceiling.Target.Validate(); err != nil {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime semantic plan provider ceiling target invalid: %w", err)
	}
	schema, err := runtimecontract.RuntimeSemanticPlanJSONSchema(manifest)
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	definitionJSON, err := json.Marshal(runtimeCandidateDefinitionProjection{
		Revision: context.Definition.Revision, GoalID: context.Definition.GoalID,
		GoalDefinition: context.Definition.GoalDefinition, DoDID: context.Definition.DoDID,
		DoDDefinition: context.Definition.DoDDefinition, ChangeImpact: context.Definition.ChangeImpact,
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	capabilityJSON, err := json.Marshal(runtimeSemanticCapabilityProjection{
		CapabilityKinds: context.Definition.CapabilityContract, ExecutionContract: context.Activation.ExecutionContract,
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	promptSchema := compactSemanticPlanPromptSchema([]byte(schema))
	outputInstruction := "Return exactly one compact semantic_plan JSON document. Propose one bounded proof declaration for every manifest coverage item. Omit optional arrays and fields when empty; do not repeat prose, descriptions, or markdown. Do not return operations, runtime actions, coverage credit, terminal state, retry policy, governance tickets, receipts, or owner results."
	if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		outputInstruction = "Return exactly one compact semantic_plan JSON document with planStep=coverage_order and a complete coverageOrder. Do not return proposals or any operation, target, recipe, feasibility, action, credit, terminal, retry, governance, receipt, or markdown field."
	} else if context.BatchID != "" {
		outputInstruction = "Return exactly one compact semantic_plan JSON document with planStep=proof_batch and proposals only for the supplied batch coverage IDs. Do not add, omit, reorder, or claim coverage outside the batch."
	}
	authorityJSON, err := json.Marshal(runtimeSemanticAuthorityProjection{
		WorkspaceID: string(context.WorkspaceID), RunID: string(context.RunID),
		DefinitionSeed: context.DefinitionSeed, Manifest: manifest, Schema: json.RawMessage(promptSchema),
		OutputInstruction: outputInstruction, PlanStep: context.PlanStep, BatchID: context.BatchID, BatchCoverageIDs: context.BatchCoverageWorkItemIDs,
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	tailJSON, err := json.Marshal(runtimeSemanticTailProjection{
		Attempt: request.CandidateAttempt, PreviousRejection: runtimeProviderSafeSemanticDiagnostic(request.PreviousSemanticRejection),
		OutputContinuation: request.OutputContinuation,
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	slicesJSON, err := json.Marshal(context.DefinitionSeed.Slices)
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest := llmmodels.ProviderRequest{
		WorkspaceID: string(context.WorkspaceID), RunID: string(context.RunID), Kind: "runtime_semantic_plan",
		Schema: schema,
		PromptContext: &llmmodels.PromptContext{SchemaVersion: "workorch.runtime_semantic_plan_prompt.v1", Sections: []llmmodels.PromptSection{
			{ID: "C0.runtime-semantic-contract", Name: "Runtime semantic plan contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 0, SourceRefs: []string{"runtime://semantic-plan-contract/v1"}, Content: runtimeSemanticPlanContractCatalog()},
			{ID: "C1.goal-dod", Name: "Goal and DoD", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 1, DefinitionRevision: context.Definition.Revision, SourceRefs: []string{"runtime://definition/" + context.Definition.Revision}, Content: string(definitionJSON)},
			{ID: "C2.capability-contract", Name: "Capability contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 2, SourceRefs: []string{"runtime://execution-contract/" + context.Definition.Revision}, Content: string(capabilityJSON)},
			{ID: "C3.semantic-authority", Name: "Current semantic plan authority", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 3, SourceRefs: []string{"runtime://semantic-manifest/" + manifest.Hash}, Content: string(authorityJSON)},
			{ID: "C4.semantic-tail", Name: "Semantic plan correction", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 4, SourceRefs: []string{"runtime://semantic-request/" + strings.TrimSpace(request.RequestIdentity.LogicalRequestID)}, Content: string(tailJSON)},
			{ID: "C5.acceptance-slices", Name: "Acceptance slice references", Role: "user", Cacheability: "volatile", DisplayOrder: 5, SourceRefs: []string{"runtime://definition-seed/" + context.DefinitionSeed.Hash}, Content: string(slicesJSON)},
		}},
		ContextBudget: &llmmodels.ProviderContextBudget{
			ProviderID: ceiling.Target.ProviderID, ModelID: ceiling.Target.ModelID,
			CapabilityRevision: ceiling.CapabilityRevision, ConfigurationRevision: ceiling.ConfigurationRevision,
			ContextWindowTokens: ceiling.ContextWindowTokens, ProviderMaxOutputTokens: ceiling.ProviderMaxOutputTokens,
			EffectiveMaxOutputTokens: ceiling.EffectiveMaxOutputTokens,
			MaxBytes:                 ceiling.MaxBytes, MaxRequestBytes: ceiling.MaxBytes, MaxTokens: ceiling.MaxInputTokens,
			EstimatedOutputTokens: ceiling.EffectiveMaxOutputTokens,
			CapabilitySource:      "sealed_ceiling", ProfileMode: "runtime_typed_ceiling",
			CachePlan: &llmmodels.ProviderCachePlan{
				SchemaVersion: "workorch.provider_cache_plan.v1", Strategy: "stable_prefix_dynamic_tail",
				StablePrefixHash: wantPrefix, RenderedStablePrefixHash: wantPrefix,
			},
		},
	}
	if budget := context.Activation.ExecutionContract.SemanticPlanMaxOutputTokens; budget > 0 && budget < ceiling.EffectiveMaxOutputTokens {
		providerRequest.MaxOutputTokens = budget
		providerRequest.ContextBudget.EffectiveMaxOutputTokens = budget
		providerRequest.ContextBudget.EstimatedOutputTokens = budget
	}
	if request.OutputContinuation != nil {
		providerRequest.Kind = "runtime_semantic_plan_output_continuation"
		providerRequest.Schema = ""
		providerRequest.RawTextOutput = true
		providerRequest.PromptContext.Sections[4].Content = string(tailJSON) + "\n\nReturn only the exact next bytes of the semantic_plan JSON document after outputContinuation.previousChunkTail. Do not repeat the tail, start a new JSON object, or use markdown."
	}
	attachProviderCachePlan(&providerRequest)
	if providerRequest.ContextBudget.CachePlan != nil {
		providerRequest.ContextBudget.CachePlan.StablePrefixHash = wantPrefix
	}
	populateRuntimeCandidateContextBudget(providerRequest.ContextBudget, providerRequest.PromptContext)
	if err := validateRuntimeSemanticContextIntegrity(context, manifest, request.CandidateAttempt, request.PreviousSemanticRejection, providerRequest.ContextBudget, providerRequest.PromptContext); err != nil {
		providerRequest.ContextBudget.ContextGateAction = "fail_fast"
		providerRequest.ContextBudget.ContextGateReason = "runtime_semantic_context_integrity_failed"
		providerRequest.ContextBudget.BudgetFailureClass = "context_integrity"
		return providerRequest, err
	}
	payload, err := json.Marshal(providerRequest)
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest.ContextBudget.ActualBytes = len(payload)
	providerRequest.ContextBudget.OverBudget = len(payload) > ceiling.MaxBytes
	if providerRequest.ContextBudget.OverBudget {
		providerRequest.ContextBudget.ContextGateAction = "fail_fast"
		providerRequest.ContextBudget.ContextGateReason = "runtime_provider_ceiling_exceeded"
		providerRequest.ContextBudget.BudgetFailureClass = "provider_ceiling_exceeded"
		return providerRequest, fmt.Errorf("runtime semantic plan request exceeds provider ceiling: %d > %d", len(payload), ceiling.MaxBytes)
	}
	return providerRequest, nil
}

// compactSemanticPlanPromptSchema removes prose-only JSON Schema metadata from
// the provider prompt. The full schema remains sealed in ProviderRequest.Schema
// and is still used by the local decoder/evaluator; only the redundant prompt
// projection is compacted.
func compactSemanticPlanPromptSchema(raw []byte) []byte {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	var compact func(any) any
	compact = func(item any) any {
		switch typed := item.(type) {
		case map[string]any:
			ret := make(map[string]any, len(typed))
			for key, child := range typed {
				switch key {
				case "description", "title", "$comment", "examples":
					continue
				default:
					ret[key] = compact(child)
				}
			}
			return ret
		case []any:
			ret := make([]any, len(typed))
			for idx, child := range typed {
				ret[idx] = compact(child)
			}
			return ret
		default:
			return item
		}
	}
	value = compact(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return encoded
}

func runtimeSemanticPlanContractCatalog() string {
	return "You propose a bounded semantic proof plan for a sealed WorkOrch definition. The proposal is not authority and has no side effects. Use only the owner modules, operation kinds, target policies, evidence kinds, effect scopes, declarations, and identities supplied in C3. Return one proposal for every coverage item and no extra item. Never emit operation execution, runtime action, coverage state or credit, terminal/retry/governance decisions, receipts, owner observations, or markdown."
}

func runtimeProviderSafeSemanticDiagnostic(value *runtimecontract.RuntimeOriginalDiagnostic) *runtimeSemanticDiagnosticProjection {
	if value == nil {
		return nil
	}
	return &runtimeSemanticDiagnosticProjection{
		OwnerModuleID: value.OwnerModuleID, Class: value.OriginalClass, Code: value.OriginalCode,
		Signature: value.Signature, OperationID: value.OperationID,
		TargetRefs: append([]string(nil), value.TargetRefs...), ExpectedRevision: value.ExpectedRevision,
		ObservedRevision: value.ObservedRevision, FieldViolations: cloneRuntimeFieldViolations(value.FieldViolations),
		OmittedReason: value.OmittedReason, Truncated: value.Truncated, Redacted: value.Redacted,
	}
}

func cloneRuntimeFieldViolations(values []runtimecontract.RuntimeFieldViolation) []runtimecontract.RuntimeFieldViolation {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimecontract.RuntimeFieldViolation, len(values))
	for idx, value := range values {
		ret[idx] = runtimecontract.RuntimeFieldViolation{
			Path: value.Path, Actual: value.Actual, Allowed: append([]string(nil), value.Allowed...),
			Required: append([]string(nil), value.Required...), Forbidden: append([]string(nil), value.Forbidden...),
		}
	}
	return ret
}

func validateRuntimeSemanticContextIntegrity(context runtimecontract.RuntimeSemanticPlanRequestContext, manifest runtimecontract.RuntimeSemanticPlanManifest, attempt int, previous *runtimecontract.RuntimeOriginalDiagnostic, budget *llmmodels.ProviderContextBudget, prompt *llmmodels.PromptContext) error {
	if budget == nil || prompt == nil {
		return fmt.Errorf("runtime semantic context integrity requires prompt and budget metadata")
	}
	missing := make([]string, 0, 6)
	contents := make(map[string]string, len(prompt.Sections))
	for _, section := range prompt.Sections {
		contents[promptSectionGroup(section.ID)] += strings.TrimSpace(section.Content)
	}
	for _, group := range []string{"C0", "C1", "C2", "C3", "C4"} {
		if contents[group] == "" {
			missing = append(missing, group)
		}
	}
	if context.Definition.GoalDefinition == "" || context.Definition.DoDDefinition == "" || len(manifest.CoverageItems) == 0 {
		missing = append(missing, "P0.contract")
	}
	if context.DefinitionSeed.Hash == "" || manifest.Hash == "" {
		missing = append(missing, "P2.coverage_scope")
	}
	if context.ProviderCeiling == nil {
		missing = append(missing, "P4.current_snapshots")
	}
	if attempt == 1 && previous == nil {
		missing = append(missing, "P1.current_failure")
	}
	if len(budget.RedactedPaths) > 0 && budget.RedactionApplied {
		missing = append(missing, "protected_context_redacted")
	}
	if len(missing) == 0 {
		return nil
	}
	budget.IntegrityWarnings = uniqueStrings(append(budget.IntegrityWarnings, missing...))
	return fmt.Errorf("runtime semantic context integrity missing or conflicted: %s", strings.Join(missing, ", "))
}

func runtimeRequestProviderCeiling(request llmmodels.RuntimeCandidateRequest) *runtimecontract.ProviderCeiling {
	if request.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		return request.SemanticContext.ProviderCeiling
	}
	return request.CandidateContext.Resources.ProviderCeiling
}

func runtimeRequestScope(request llmmodels.RuntimeCandidateRequest) (string, string) {
	if request.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		return string(request.SemanticContext.WorkspaceID), string(request.SemanticContext.RunID)
	}
	return string(request.CandidateContext.WorkspaceID), string(request.CandidateContext.RunID)
}
