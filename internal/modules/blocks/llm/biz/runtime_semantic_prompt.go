package llm

import (
	"encoding/json"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeSemanticPrompt struct {
	definition string
	capability string
	authority  string
	tail       string
	slices     string
}

func buildRuntimeSemanticPrompt(request llmmodels.RuntimeCandidateRequest, prepared runtimeSemanticPreparedRequest) (runtimeSemanticPrompt, error) {
	definitionJSON, err := json.Marshal(runtimeCandidateDefinitionProjection{
		Revision: prepared.context.Definition.Revision, GoalID: prepared.context.Definition.GoalID,
		GoalDefinition: prepared.context.Definition.GoalDefinition, DoDID: prepared.context.Definition.DoDID,
		DoDDefinition: prepared.context.Definition.DoDDefinition, ChangeImpact: prepared.context.Definition.ChangeImpact,
	})
	if err != nil {
		return runtimeSemanticPrompt{}, err
	}
	capabilityJSON, err := json.Marshal(runtimeSemanticCapabilityProjection{
		CapabilityKinds:   prepared.context.Definition.CapabilityContract,
		ExecutionContract: prepared.context.Activation.ExecutionContract,
	})
	if err != nil {
		return runtimeSemanticPrompt{}, err
	}
	manifestProjection, err := runtimeSemanticManifestForProvider(prepared.manifest, prepared.context.PlanStep, prepared.context.BatchCoverageWorkItemIDs)
	if err != nil {
		return runtimeSemanticPrompt{}, err
	}
	authorityJSON, err := json.Marshal(runtimeSemanticAuthorityProjection{
		WorkspaceID:       string(prepared.context.WorkspaceID),
		RunID:             string(prepared.context.RunID),
		Soul:              runtimeCandidateSoul(prepared.context.Activation, "planning"),
		DefinitionSeed:    runtimeSemanticDefinitionSeedForProvider(prepared.context.DefinitionSeed),
		Manifest:          manifestProjection,
		Schema:            json.RawMessage(compactSemanticPlanPromptSchema([]byte(prepared.schema))),
		OutputInstruction: runtimeSemanticOutputInstruction(prepared.context, prepared.manifest),
		PlanStep:          prepared.context.PlanStep,
		BatchID:           prepared.context.BatchID,
		BatchSequence:     runtimeSemanticBatchSequence(prepared.context),
		BatchCoverageIDs:  prepared.context.BatchCoverageWorkItemIDs,
	})
	if err != nil {
		return runtimeSemanticPrompt{}, err
	}
	tailJSON, err := json.Marshal(runtimeSemanticTailProjection{
		Attempt:             request.CandidateAttempt,
		PreviousRejection:   cloneRuntimeSemanticCorrectionObservation(request.SemanticCorrectionObservation),
		CorrectionDirective: cloneRuntimeSemanticCorrectionDirective(request.SemanticCorrectionDirective),
		ExistingTarget:      runtimeSemanticExistingTargetForProvider(prepared.manifest, request.SemanticCorrectionObservation),
	})
	if err != nil {
		return runtimeSemanticPrompt{}, err
	}
	slicesJSON, err := json.Marshal(runtimeSemanticSlicesForProvider(
		prepared.context.DefinitionSeed.Slices,
		runtimeSemanticVisibleCoverageIDs(prepared.context, prepared.manifest),
	))
	if err != nil {
		return runtimeSemanticPrompt{}, err
	}
	return runtimeSemanticPrompt{
		definition: string(definitionJSON), capability: string(capabilityJSON),
		authority: string(authorityJSON), tail: string(tailJSON), slices: string(slicesJSON),
	}, nil
}

func runtimeSemanticVisibleCoverageIDs(context runtimecontract.RuntimeSemanticPlanRequestContext, manifest runtimecontract.RuntimeSemanticPlanManifest) []common.CoverageWorkItemID {
	if context.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
		return context.BatchCoverageWorkItemIDs
	}
	ret := make([]common.CoverageWorkItemID, 0, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		ret = append(ret, item.ID)
	}
	return ret
}

func newRuntimeSemanticProviderRequest(request llmmodels.RuntimeCandidateRequest, prepared runtimeSemanticPreparedRequest, prompt runtimeSemanticPrompt) llmmodels.ProviderRequest {
	context := prepared.context
	ceiling := prepared.ceiling
	return llmmodels.ProviderRequest{
		WorkspaceID:     string(context.WorkspaceID),
		RunID:           string(context.RunID),
		Kind:            "runtime_semantic_plan",
		ReasoningEffort: ceiling.Selection.ReasoningEffort,
		Schema:          prepared.schema,
		PromptContext: &llmmodels.PromptContext{
			SchemaVersion: "workorch.runtime_semantic_plan_prompt",
			Sections: []llmmodels.PromptSection{
				{ID: "C0.runtime-semantic-contract", Name: "Runtime semantic plan contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 0, SourceRefs: []string{"runtime://semantic-plan-contract"}, Content: runtimeSemanticPlanContractCatalog()},
				{ID: "C1.goal-dod", Name: "Goal and DoD", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 1, DefinitionRevision: context.Definition.Revision, SourceRefs: []string{"runtime://definition/" + context.Definition.Revision}, Content: prompt.definition},
				{ID: "C2.capability-contract", Name: "Capability contract", Role: "system", Cacheability: "stable", Protected: true, DisplayOrder: 2, SourceRefs: []string{"runtime://execution-contract/" + context.Definition.Revision}, Content: prompt.capability},
				{ID: "C3.semantic-authority", Name: "Current semantic plan authority", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 3, SourceRefs: []string{"runtime://semantic-manifest/" + prepared.manifest.Hash}, Content: prompt.authority},
				{ID: "C4.semantic-tail", Name: "Semantic plan correction", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 4, SourceRefs: []string{"runtime://semantic-request/" + strings.TrimSpace(request.RequestIdentity.LogicalRequestID)}, Content: prompt.tail},
				{ID: "C5.acceptance-slices", Name: "Acceptance slice references", Role: "user", Cacheability: "volatile", DisplayOrder: 5, SourceRefs: []string{"runtime://definition-seed/" + context.DefinitionSeed.Hash}, Content: prompt.slices},
			},
		},
		ContextBudget: &llmmodels.ProviderContextBudget{
			ProviderID:               ceiling.Selection.Target.ProviderID,
			ModelID:                  ceiling.Selection.Target.ModelID,
			CapabilityRevision:       ceiling.CapabilityRevision,
			ConfigurationRevision:    ceiling.ConfigurationRevision,
			ContextWindowTokens:      ceiling.ContextWindowTokens,
			ProviderMaxOutputTokens:  ceiling.ProviderMaxOutputTokens,
			EffectiveMaxOutputTokens: ceiling.EffectiveMaxOutputTokens,
			MaxBytes:                 ceiling.MaxBytes,
			MaxRequestBytes:          ceiling.MaxBytes,
			MaxTokens:                ceiling.MaxInputTokens,
			EstimatedOutputTokens:    ceiling.EffectiveMaxOutputTokens,
			CapabilitySource:         "sealed_ceiling",
			ProfileMode:              "runtime_typed_ceiling",
			CachePlan: &llmmodels.ProviderCachePlan{
				SchemaVersion:            "workorch.provider_cache_plan",
				Strategy:                 "stable_prefix_dynamic_tail",
				StablePrefixHash:         prepared.stablePrefix,
				RenderedStablePrefixHash: prepared.stablePrefix,
			},
		},
	}
}
