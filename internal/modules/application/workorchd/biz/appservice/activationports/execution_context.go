package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspacecontracts "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/contracts"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type WorkspaceRuntimeActivationBuild struct {
	EffectiveGoal          *workspacemodel.Goal
	EffectiveDoD           *workspacemodel.DoD
	DefinitionRevision     string
	DefinitionChangeImpact *runtimecontract.RuntimeDefinitionChangeImpact
	RuntimeActivation      runtimecontract.RuntimeActivationContext
}

func BuildWorkspaceRuntimeActivation(ctx context.Context, request WorkspaceRuntimeActivationRequest) (WorkspaceRuntimeActivationBuild, error) {
	workspace := request.Workspace
	goal := request.Goal
	dod := request.DoD
	if request.Hub == nil || workspace == nil || goal == nil || dod == nil || goal.WorkspaceID != workspace.ID || dod.WorkspaceID != workspace.ID {
		return WorkspaceRuntimeActivationBuild{}, common.ErrInvalidArgument
	}
	definitions, err := WorkspaceExecutionDefinitionsContext(ctx, request.Hub, workspace.ID, goal, dod)
	if err != nil {
		return WorkspaceRuntimeActivationBuild{}, err
	}
	effectiveGoal := definitions.EffectiveGoal
	effectiveDoD := definitions.EffectiveDoD
	definitionChangeImpact := definitions.DefinitionChangeImpact
	command := request.Command
	dialogLanguage := activationcore.DialogLanguage(command.DialogLanguage, request.DefaultDialogLanguage)
	executionContract := WorkspaceExecutionContract(ctx, request.Hub, workspace)
	if err := applyConfiguredRuntimeRepairLimit(ctx, executionContract); err != nil {
		return WorkspaceRuntimeActivationBuild{}, err
	}
	toolHints := WorkspaceToolContextHintsFor(ctx, request.Hub, workspace)
	if err := workspacecore.EnsureManagedConfigs(workspace.InternalDirectory, toolHints.Template, toolHints.Workspace); err != nil {
		return WorkspaceRuntimeActivationBuild{}, err
	}
	defaultMCPServerIDs := activationcore.DefaultMCPServerIDsFromContextHints(toolHints.Effective)
	defaultCapabilityIDs := activationcore.DefaultCapabilityIDsFromContextHints(toolHints.Effective)
	memoryIDs := activationcore.NormalizeStringIDs(command.MemoryIDs)
	bindings, err := loadWorkspaceRuntimeActivationBindings(ctx, request, effectiveGoal, effectiveDoD, memoryIDs)
	if err != nil {
		return WorkspaceRuntimeActivationBuild{}, err
	}
	defaultSoul := bindings.defaultSoul
	memories := bindings.memories
	governanceHistory := bindings.governanceHistory
	soulRouting := bindings.soulRouting

	skillResult, err := SelectWorkspaceExecutionSkills(ctx, WorkspaceSkillSelectionRequest{
		Hub:             request.Hub,
		WorkspaceID:     workspace.ID,
		Goal:            effectiveGoal,
		DoD:             effectiveDoD,
		RequestedIDs:    command.SkillIDs,
		Soul:            defaultSoul,
		SkillHubCatalog: request.SkillHubCatalog,
	})
	if err != nil {
		return WorkspaceRuntimeActivationBuild{}, err
	}
	skills := skillResult.Skills
	skillIDs := skillResult.IDs

	mcpSelection, err := SelectedWorkspaceMCP(ctx, WorkspaceMCPSelectionRequest{
		Hub:                    request.Hub,
		Workspace:              workspace,
		RequestedMCPServerIDs:  command.MCPServerIDs,
		DefaultMCPServerIDs:    defaultMCPServerIDs,
		RequestedCapabilityIDs: command.CapabilityIDs,
		DefaultCapabilityIDs:   defaultCapabilityIDs,
		Skills:                 skills,
	})
	if err != nil {
		return WorkspaceRuntimeActivationBuild{}, err
	}

	if err := ValidateSoulRoutingCapabilitySelection(ctx, request.Hub, soulRouting, skillIDs, mcpSelection.MCPServerIDs, mcpSelection.CapabilityIDs, activationcore.CapabilityCategories(mcpSelection.Capabilities), activationcore.RequiredToolKinds(skills)); err != nil {
		return WorkspaceRuntimeActivationBuild{}, err
	}

	continuation, err := mergeWorkspaceExecutionContinuations(ctx, workspace.ID, effectiveGoal.ID, effectiveDoD.ID, definitionChangeImpact, governanceHistory, request)
	if err != nil {
		return WorkspaceRuntimeActivationBuild{}, err
	}

	providerSelection, err := resolveActivationProviderSelection(ctx, request.Hub, workspace.DefaultLLMSelection)
	if err != nil {
		return WorkspaceRuntimeActivationBuild{}, err
	}

	return WorkspaceRuntimeActivationBuild{
		EffectiveGoal: effectiveGoal,
		EffectiveDoD:  effectiveDoD,
		DefinitionRevision: workspacecontracts.ExecutionDefinitionRevision(
			string(effectiveGoal.ID), effectiveGoal.DefinitionRevision,
			string(effectiveDoD.ID), effectiveDoD.DefinitionRevision,
		),
		DefinitionChangeImpact: RuntimeDefinitionChangeImpact(definitionChangeImpact),
		RuntimeActivation: RuntimeActivationContext(
			providerSelection,
			dialogLanguage,
			defaultSoul,
			bindings.stageSouls,
			soulRouting,
			skills,
			mcpSelection.MCPServerIDs,
			mcpSelection.Capabilities,
			memories,
			workspace.EvidencePolicies,
			executionContract,
			continuation,
		),
	}, nil
}

func applyConfiguredRuntimeRepairLimit(ctx context.Context, executionContract map[string]any) error {
	configuration, err := configcore.LoadFromContext(ctx)
	if err != nil {
		return err
	}
	if configuration.Runtime.MaxRuntimeRepairAttempts > 0 && executionContract != nil {
		executionContract["maxRepairAttemptsPerFocus"] = configuration.Runtime.MaxRuntimeRepairAttempts
	}
	if executionContract != nil {
		llmOutput := configuration.Runtime.LLMOutput
		executionContract["semanticPlanMaxOutputTokens"] = llmOutput.SemanticPlanMaxOutputTokens
		executionContract["candidateMaxOutputTokens"] = llmOutput.CandidateMaxOutputTokens
		executionContract["candidateDocumentRecovery"] = map[string]any{
			"maxAttempts": configuration.Runtime.LLMOutput.MaxDocumentRecoveryAttempts,
		}
	}
	return nil
}

func resolveActivationProviderSelection(ctx context.Context, hub magicevent.Hub, workspaceDefault *runtimecontract.ProviderInferenceSelection) (runtimecontract.ProviderInferenceSelection, error) {
	selection, err := configuredActivationProviderSelection(ctx, workspaceDefault)
	if err != nil {
		return runtimecontract.ProviderInferenceSelection{}, err
	}
	validated, err := EnsureProviderInferenceSelection(ctx, hub, selection)
	if err != nil {
		return runtimecontract.ProviderInferenceSelection{}, err
	}
	return validated, nil
}

func configuredActivationProviderSelection(ctx context.Context, workspaceDefault *runtimecontract.ProviderInferenceSelection) (runtimecontract.ProviderInferenceSelection, error) {
	var selection runtimecontract.ProviderInferenceSelection
	if workspaceDefault != nil {
		selection = workspaceDefault.Normalized()
	} else {
		cfg, err := configcore.LoadFromContext(ctx)
		if err != nil {
			return runtimecontract.ProviderInferenceSelection{}, err
		}
		selection.Target = cfg.LLM.DefaultChat.ToRuntimeTarget()
		if selection.Target.ProviderID == "" && selection.Target.ModelID == "" {
			cfg, err = workorchconfig.Current()
			if err != nil {
				return runtimecontract.ProviderInferenceSelection{}, err
			}
			selection.Target = cfg.LLM.DefaultChat.ToRuntimeTarget()
		}
	}
	if err := selection.Validate(); err != nil {
		return runtimecontract.ProviderInferenceSelection{}, err
	}
	return selection, nil
}
