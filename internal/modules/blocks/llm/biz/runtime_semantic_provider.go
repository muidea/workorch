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
	WorkspaceID       string                                  `json:"workspaceId"`
	RunID             string                                  `json:"runId"`
	DefinitionSeed    runtimeSemanticDefinitionSeedProjection `json:"definitionSeed"`
	Manifest          runtimeSemanticManifestProjection       `json:"manifest"`
	Schema            json.RawMessage                         `json:"schema"`
	OutputInstruction string                                  `json:"outputInstruction"`
	PlanStep          runtimecontract.RuntimeSemanticPlanStep `json:"planStep"`
	BatchID           string                                  `json:"batchId,omitempty"`
	BatchSequence     *int                                    `json:"batchSequence,omitempty"`
	BatchCoverageIDs  []common.CoverageWorkItemID             `json:"batchCoverageWorkItemIds,omitempty"`
}

type runtimeSemanticDefinitionSeedProjection struct {
	SchemaVersion      string `json:"schemaVersion"`
	Hash               string `json:"hash"`
	DefinitionRevision string `json:"definitionRevision"`
	PolicyHash         string `json:"policyHash"`
}

type runtimeSemanticManifestProjection struct {
	SchemaVersion             string                                          `json:"schemaVersion"`
	Hash                      string                                          `json:"hash"`
	DefinitionRevision        string                                          `json:"definitionRevision"`
	DefinitionSeedHash        string                                          `json:"definitionSeedHash"`
	OperationContractRevision string                                          `json:"operationContractRevision"`
	CoverageItems             []runtimecontract.RuntimeCoverageItem           `json:"coverageItems"`
	AllowedOwnerModuleIDs     []string                                        `json:"allowedOwnerModuleIds,omitempty"`
	AllowedOperationKinds     []string                                        `json:"allowedOperationKinds,omitempty"`
	AllowedTargetPolicies     []runtimecontract.RuntimeOperationTargetPolicy  `json:"allowedTargetPolicies,omitempty"`
	OwnerCatalogs             []runtimecontract.RuntimeOperationAdmissibility `json:"ownerCatalogs,omitempty"`
}

type runtimeSemanticSliceProjection struct {
	ID                    string                      `json:"id"`
	Title                 string                      `json:"title,omitempty"`
	Scope                 string                      `json:"scope,omitempty"`
	Priority              int                         `json:"priority,omitempty"`
	CoverageWorkItemIDs   []common.CoverageWorkItemID `json:"coverageWorkItemIds"`
	RequiredEvidenceKinds []string                    `json:"requiredEvidenceKinds,omitempty"`
	CompletionPolicy      string                      `json:"completionPolicy,omitempty"`
	ResultPolicy          string                      `json:"resultPolicy,omitempty"`
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
	Attempt             int                                                 `json:"attempt"`
	PreviousRejection   *runtimeSemanticDiagnosticProjection                `json:"previousRejection,omitempty"`
	CorrectionDirective *runtimecontract.RuntimeSemanticCorrectionDirective `json:"correctionDirective,omitempty"`
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
	if request.CandidateAttempt == 0 && (request.PreviousSemanticRejection != nil || request.SemanticCorrectionDirective != nil) {
		return llmmodels.ProviderRequest{}, fmt.Errorf("initial runtime semantic plan request must not carry correction lineage")
	}
	if request.CandidateAttempt == 1 {
		if request.PreviousSemanticRejection == nil || request.SemanticCorrectionDirective == nil {
			return llmmodels.ProviderRequest{}, fmt.Errorf("runtime semantic plan correction requires rejection and directive")
		}
		if err := request.PreviousSemanticRejection.Validate(); err != nil {
			return llmmodels.ProviderRequest{}, err
		}
		if err := request.SemanticCorrectionDirective.ValidateAgainst(manifest, *request.PreviousSemanticRejection); err != nil {
			return llmmodels.ProviderRequest{}, err
		}
		if request.SemanticCorrectionDirective.PlanStep != context.PlanStep || request.SemanticCorrectionDirective.StablePrefixHash != strings.TrimSpace(request.StablePrefixHash) {
			return llmmodels.ProviderRequest{}, fmt.Errorf("runtime semantic plan correction identity is stale")
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
	if err := ceiling.Selection.Target.Validate(); err != nil {
		return llmmodels.ProviderRequest{}, fmt.Errorf("runtime semantic plan provider ceiling target invalid: %w", err)
	}
	schema, err := runtimecontract.RuntimeSemanticPlanJSONSchemaForRequest(manifest, context.PlanStep, context.BatchID, context.BatchSequence, context.BatchCoverageWorkItemIDs)
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
	outputInstruction := "Return exactly one complete compact semantic_plan JSON object and finish it in this response. Emit only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, or unrequested proposals, and do not repeat the Goal, DoD, manifest, authority, or input context. For every manifest coverage item, return one bounded proof declaration for each required evidence kind. Each proposal has exactly one requiredEvidenceKinds value; a CWI requiring state_change and validation therefore has two proposals with the same coverageWorkItemId. Omit optional arrays and fields when empty. A validation evidence proof must use only operationKind=mcp/tool_call or operationKind=validation/managed_session and a non-durable effect (none or ephemeral_session); file/read, file/write, shell/command, git/write, and patch/apply cannot claim validation evidence. Do not return operations, runtime actions, coverage credit, terminal state, retry policy, governance tickets, receipts, or owner results."
	if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		outputInstruction = "Return exactly one complete compact semantic_plan JSON object with planStep=coverage_order and a complete coverageOrder, and finish it in this response. Emit only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, or repeated Goal, DoD, manifest, authority, or input context. Do not return proposals or any operation, target, recipe, feasibility, action, credit, terminal, retry, governance, or receipt field."
	} else if context.BatchID != "" {
		outputInstruction = "Return exactly one complete compact semantic_plan JSON object with planStep=proof_batch and proposals only for the supplied batch coverage IDs, and finish it in this response. Emit only schema fields; do not add prose, preface, suffix, markdown fences, unknown fields, unrequested proposals, or repeated Goal, DoD, manifest, authority, or input context. Every batch CWI must have one proof for each required evidence kind; each proposal has exactly one requiredEvidenceKinds value, so a CWI requiring state_change and validation has two proposals. Echo the sealed batch identity exactly. For validation evidence use only mcp/tool_call or validation/managed_session with a non-durable effect; never use file/read, file/write, shell/command, git/write, or patch/apply for validation. Do not return coverageOrder, add or omit batch coverage, or claim coverage outside the batch."
	}
	var batchSequence *int
	if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch && context.BatchID != "" {
		value := context.BatchSequence
		batchSequence = &value
	}
	authorityJSON, err := json.Marshal(runtimeSemanticAuthorityProjection{
		WorkspaceID: string(context.WorkspaceID), RunID: string(context.RunID),
		DefinitionSeed: runtimeSemanticDefinitionSeedForProvider(context.DefinitionSeed), Manifest: runtimeSemanticManifestForProvider(manifest, context.PlanStep, context.BatchCoverageWorkItemIDs), Schema: json.RawMessage(promptSchema),
		OutputInstruction: outputInstruction, PlanStep: context.PlanStep, BatchID: context.BatchID, BatchSequence: batchSequence, BatchCoverageIDs: context.BatchCoverageWorkItemIDs,
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	tailJSON, err := json.Marshal(runtimeSemanticTailProjection{
		Attempt: request.CandidateAttempt, PreviousRejection: runtimeProviderSafeSemanticDiagnostic(request.PreviousSemanticRejection),
		CorrectionDirective: cloneRuntimeSemanticCorrectionDirective(request.SemanticCorrectionDirective),
	})
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	visibleCoverageIDs := context.BatchCoverageWorkItemIDs
	if context.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		visibleCoverageIDs = make([]common.CoverageWorkItemID, 0, len(manifest.CoverageItems))
		for _, item := range manifest.CoverageItems {
			visibleCoverageIDs = append(visibleCoverageIDs, item.ID)
		}
	}
	slicesJSON, err := json.Marshal(runtimeSemanticSlicesForProvider(context.DefinitionSeed.Slices, visibleCoverageIDs))
	if err != nil {
		return llmmodels.ProviderRequest{}, err
	}
	providerRequest := llmmodels.ProviderRequest{
		WorkspaceID: string(context.WorkspaceID), RunID: string(context.RunID), Kind: "runtime_semantic_plan",
		ReasoningEffort: ceiling.Selection.ReasoningEffort,
		Schema:          schema,
		PromptContext: &llmmodels.PromptContext{SchemaVersion: "workorch.runtime_semantic_plan_prompt.v1", Sections: []llmmodels.PromptSection{
			{ID: "C0.runtime-semantic-contract", Name: "Runtime semantic plan contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 0, SourceRefs: []string{"runtime://semantic-plan-contract/v1"}, Content: runtimeSemanticPlanContractCatalog()},
			{ID: "C1.goal-dod", Name: "Goal and DoD", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 1, DefinitionRevision: context.Definition.Revision, SourceRefs: []string{"runtime://definition/" + context.Definition.Revision}, Content: string(definitionJSON)},
			{ID: "C2.capability-contract", Name: "Capability contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 2, SourceRefs: []string{"runtime://execution-contract/" + context.Definition.Revision}, Content: string(capabilityJSON)},
			{ID: "C3.semantic-authority", Name: "Current semantic plan authority", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 3, SourceRefs: []string{"runtime://semantic-manifest/" + manifest.Hash}, Content: string(authorityJSON)},
			{ID: "C4.semantic-tail", Name: "Semantic plan correction", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 4, SourceRefs: []string{"runtime://semantic-request/" + strings.TrimSpace(request.RequestIdentity.LogicalRequestID)}, Content: string(tailJSON)},
			{ID: "C5.acceptance-slices", Name: "Acceptance slice references", Role: "user", Cacheability: "volatile", DisplayOrder: 5, SourceRefs: []string{"runtime://definition-seed/" + context.DefinitionSeed.Hash}, Content: string(slicesJSON)},
		}},
		ContextBudget: &llmmodels.ProviderContextBudget{
			ProviderID: ceiling.Selection.Target.ProviderID, ModelID: ceiling.Selection.Target.ModelID,
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
	providerRequest.MaxOutputTokens = ceiling.EffectiveMaxOutputTokens
	if request.ExpectedDocument != nil {
		providerRequest.MaxOutputTokens = request.ExpectedDocument.Normalized().MaxOutputTokens
	}
	providerRequest.ContextBudget.EffectiveMaxOutputTokens = providerRequest.MaxOutputTokens
	providerRequest.ContextBudget.EstimatedOutputTokens = providerRequest.MaxOutputTokens
	attachProviderCachePlan(&providerRequest)
	if providerRequest.ContextBudget.CachePlan != nil {
		providerRequest.ContextBudget.CachePlan.StablePrefixHash = wantPrefix
	}
	populateRuntimeCandidateContextBudget(providerRequest.ContextBudget, providerRequest.PromptContext)
	if err := validateRuntimeSemanticContextIntegrity(context, manifest, request.CandidateAttempt, request.PreviousSemanticRejection, request.SemanticCorrectionDirective, providerRequest.ContextBudget, providerRequest.PromptContext); err != nil {
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

func runtimeSemanticDefinitionSeedForProvider(seed runtimecontract.RuntimeDefinitionSeed) runtimeSemanticDefinitionSeedProjection {
	seed = seed.Normalized()
	return runtimeSemanticDefinitionSeedProjection{
		SchemaVersion: seed.SchemaVersion, Hash: seed.Hash,
		DefinitionRevision: seed.DefinitionRevision, PolicyHash: seed.PolicyHash,
	}
}

func runtimeSemanticManifestForProvider(manifest runtimecontract.RuntimeSemanticPlanManifest, step runtimecontract.RuntimeSemanticPlanStep, batchCoverageIDs []common.CoverageWorkItemID) runtimeSemanticManifestProjection {
	manifest = manifest.Normalized()
	selected := make(map[common.CoverageWorkItemID]struct{}, len(batchCoverageIDs))
	for _, id := range batchCoverageIDs {
		selected[id] = struct{}{}
	}
	items := make([]runtimecontract.RuntimeCoverageItem, 0, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		if step == runtimecontract.RuntimeSemanticPlanStepProofBatch {
			if _, found := selected[item.ID]; !found {
				continue
			}
		}
		items = append(items, item)
	}
	projection := runtimeSemanticManifestProjection{
		SchemaVersion: manifest.SchemaVersion, Hash: manifest.Hash,
		DefinitionRevision: manifest.DefinitionRevision, DefinitionSeedHash: manifest.DefinitionSeedHash,
		OperationContractRevision: manifest.OperationContractRevision, CoverageItems: items,
	}
	if step != runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		projection.AllowedOwnerModuleIDs = append([]string(nil), manifest.AllowedOwnerModuleIDs...)
		projection.AllowedOperationKinds = append([]string(nil), manifest.AllowedOperationKinds...)
		projection.AllowedTargetPolicies = append([]runtimecontract.RuntimeOperationTargetPolicy(nil), manifest.AllowedTargetPolicies...)
		projection.OwnerCatalogs = append([]runtimecontract.RuntimeOperationAdmissibility(nil), manifest.OwnerCatalogs...)
	}
	return projection
}

func runtimeSemanticSlicesForProvider(slices []runtimecontract.RuntimeAcceptanceSlice, visibleCoverageIDs []common.CoverageWorkItemID) []runtimeSemanticSliceProjection {
	selected := make(map[common.CoverageWorkItemID]struct{}, len(visibleCoverageIDs))
	for _, id := range visibleCoverageIDs {
		selected[id] = struct{}{}
	}
	ret := make([]runtimeSemanticSliceProjection, 0, len(slices))
	for _, value := range runtimecontract.NormalizeRuntimeAcceptanceSlices(slices) {
		coverageIDs := make([]common.CoverageWorkItemID, 0, len(value.CoverageWorkItemIDs))
		for _, id := range value.CoverageWorkItemIDs {
			if _, found := selected[id]; found {
				coverageIDs = append(coverageIDs, id)
			}
		}
		if len(coverageIDs) == 0 {
			continue
		}
		ret = append(ret, runtimeSemanticSliceProjection{
			ID: value.ID, Title: value.Title, Scope: value.Scope, Priority: value.Priority,
			CoverageWorkItemIDs: coverageIDs, RequiredEvidenceKinds: append([]string(nil), value.RequiredEvidenceKinds...),
			CompletionPolicy: value.CompletionPolicy, ResultPolicy: value.ResultPolicy,
		})
	}
	return ret
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
	return "Return exactly one complete compact JSON object and finish it in this response. Emit only schema fields; never add prose, a preface or suffix, markdown fences, unknown fields, unrequested proposals, or repeated Goal, DoD, manifest, authority, or input context. You propose a bounded semantic proof plan for a sealed WorkOrch definition. The proposal is not authority and has no side effects. Use only the owner modules, operation kinds, target policies, evidence kinds, effect scopes, declarations, and identities supplied in C3. Each proposal is one independently feasible proof and has exactly one evidence kind. Group proofs by coverageWorkItemId: return one proof for every required evidence kind of each coverage item, with no extra CWI or duplicate evidence kind. Validation evidence is receipt-backed and may use only mcp/tool_call or validation/managed_session with effect none or ephemeral_session; file/read, file/write, shell/command, git/write, and patch/apply are never validation proof operations. Never emit operation execution, runtime action, coverage state or credit, terminal/retry/governance decisions, receipts, or owner observations."
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

func validateRuntimeSemanticContextIntegrity(context runtimecontract.RuntimeSemanticPlanRequestContext, manifest runtimecontract.RuntimeSemanticPlanManifest, attempt int, previous *runtimecontract.RuntimeOriginalDiagnostic, directive *runtimecontract.RuntimeSemanticCorrectionDirective, budget *llmmodels.ProviderContextBudget, prompt *llmmodels.PromptContext) error {
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
	if attempt == 1 && (previous == nil || directive == nil) {
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
