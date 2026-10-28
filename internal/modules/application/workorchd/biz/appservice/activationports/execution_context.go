package activationports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type workspaceRuntimeActivationBuild struct {
	EffectiveGoal          *workspacemodel.Goal
	EffectiveDoD           *workspacemodel.DoD
	DefinitionRevision     string
	DefinitionChangeImpact *runtimecontract.RuntimeDefinitionChangeImpact
	RuntimeActivation      runtimecontract.RuntimeActivationContext
}

func buildWorkspaceRuntimeActivation(ctx context.Context, request workspaceRuntimeActivationRequest) (workspaceRuntimeActivationBuild, error) {
	workspace := request.Workspace
	template := request.Template
	goal := request.Goal
	dod := request.DoD
	if request.Hub == nil || workspace == nil || template == nil || goal == nil || dod == nil || goal.WorkspaceID != workspace.ID || dod.WorkspaceID != workspace.ID || strings.TrimSpace(template.ID) != strings.TrimSpace(workspace.TemplateID) {
		return workspaceRuntimeActivationBuild{}, common.ErrInvalidArgument
	}
	var definitions WorkspaceExecutionDefinitionContext
	if workspace.StartupInput != nil && workspace.ActivationAdmission != nil && workspace.ActivationAdmission.State == workspacemodel.WorkspaceActivationEligible {
		definitions = sealedWorkspaceExecutionDefinitionsContext(goal, dod)
	} else {
		var err error
		definitions, err = WorkspaceExecutionDefinitionsContext(ctx, request.Hub, workspace.ID, goal, dod)
		if err != nil {
			return workspaceRuntimeActivationBuild{}, err
		}
	}
	effectiveGoal := definitions.EffectiveGoal
	effectiveDoD := definitions.EffectiveDoD
	definitionChangeImpact := definitions.DefinitionChangeImpact
	dialogLanguage := activationcore.DialogLanguage("", request.DefaultDialogLanguage)
	executionContract := activationcore.WorkspaceExecutionContractForTemplate(workspace, template)
	if err := applyConfiguredRuntimeRepairLimit(ctx, executionContract); err != nil {
		return workspaceRuntimeActivationBuild{}, err
	}
	toolHints := WorkspaceToolContextHintsFromTemplate(workspace, template)
	if err := workspacecore.EnsureManagedConfigs(workspace.InternalDirectory, toolHints.Template, toolHints.Workspace); err != nil {
		return workspaceRuntimeActivationBuild{}, err
	}
	defaultMCPServerIDs := activationcore.DefaultMCPServerIDsFromContextHints(toolHints.Effective)
	defaultCapabilityIDs := activationcore.DefaultCapabilityIDsFromContextHints(toolHints.Effective)
	bindings, err := loadWorkspaceRuntimeActivationBindings(ctx, request, effectiveGoal, effectiveDoD)
	if err != nil {
		return workspaceRuntimeActivationBuild{}, err
	}
	defaultSoul := bindings.defaultSoul
	executionSoul := workspaceExecutionSoul(defaultSoul, bindings.stageSouls)
	memories := bindings.memories
	governanceHistory := bindings.governanceHistory
	soulRouting := bindings.soulRouting

	skillResult, err := SelectWorkspaceExecutionSkills(ctx, WorkspaceSkillSelectionRequest{
		Hub:             request.Hub,
		WorkspaceID:     workspace.ID,
		Goal:            effectiveGoal,
		DoD:             effectiveDoD,
		Soul:            executionSoul,
		SkillHubCatalog: request.SkillHubCatalog,
	})
	if err != nil {
		return workspaceRuntimeActivationBuild{}, err
	}
	skills := skillResult.Skills
	skillIDs := skillResult.IDs

	mcpSelection, err := SelectedWorkspaceMCP(ctx, WorkspaceMCPSelectionRequest{
		Hub:                  request.Hub,
		Workspace:            workspace,
		DefaultMCPServerIDs:  defaultMCPServerIDs,
		DefaultCapabilityIDs: defaultCapabilityIDs,
		Skills:               skills,
	})
	if err != nil {
		return workspaceRuntimeActivationBuild{}, err
	}

	if err := validateSelectedMCPCapabilityOwnership(mcpSelection); err != nil {
		return workspaceRuntimeActivationBuild{}, err
	}
	toolKinds := append(activationcore.RequiredToolKinds(skills), runtimeMCPCapabilityToolKinds(mcpSelection.Capabilities)...)
	if err := ValidateSoulRoutingCapabilitySelection(ctx, request.Hub, soulRouting, skillIDs, mcpSelection.MCPServerIDs, mcpSelection.CapabilityIDs, activationcore.CapabilityCategories(mcpSelection.Capabilities), toolKinds); err != nil {
		return workspaceRuntimeActivationBuild{}, err
	}

	continuation, err := mergeWorkspaceExecutionContinuations(definitionChangeImpact, governanceHistory)
	if err != nil {
		return workspaceRuntimeActivationBuild{}, err
	}

	providerSelection, err := resolveActivationProviderSelection(ctx, request.Hub, workspace.DefaultLLMSelection)
	if err != nil {
		return workspaceRuntimeActivationBuild{}, err
	}

	runtimeActivation, err := RuntimeActivationContext(
		providerSelection,
		dialogLanguage,
		defaultSoul,
		bindings.stageSouls,
		soulRouting,
		skills,
		mcpSelection.MCPServerIDs,
		mcpSelection.Capabilities,
		memories,
		bindings.memorySelection,
		workspace.EvidencePolicies,
		executionContract,
		continuation,
	)
	if err != nil {
		return workspaceRuntimeActivationBuild{}, err
	}

	profiles, err := definitionconfiguration.Compile(workspace, template)
	if err != nil {
		return workspaceRuntimeActivationBuild{}, err
	}
	if workspace.StartupInput != nil && profiles.ProfileContractsHash != workspace.StartupInput.RuntimeProfileContractsHash {
		return workspaceRuntimeActivationBuild{}, common.ErrConflict
	}
	runtimeActivation.ExecutionContract.ProfileContracts = profiles.ProfileContracts
	return workspaceRuntimeActivationBuild{
		EffectiveGoal: effectiveGoal,
		EffectiveDoD:  effectiveDoD,
		DefinitionRevision: workspacecontracts.ExecutionDefinitionRevision(
			string(effectiveGoal.ID), effectiveGoal.DefinitionRevision,
			string(effectiveDoD.ID), effectiveDoD.DefinitionRevision,
		),
		DefinitionChangeImpact: RuntimeDefinitionChangeImpact(definitionChangeImpact),
		RuntimeActivation:      runtimeActivation,
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
		executionContract["candidateDocumentRecovery"] = map[string]any{
			"maxAttempts": configuration.Runtime.LLMOutput.MaxDocumentRecoveryAttempts,
		}
	}
	return nil
}

func resolveActivationProviderSelection(ctx context.Context, hub magicevent.Hub, workspaceDefault *runtimecontract.ProviderInferenceSelection) (runtimecontract.ProviderInferenceSelection, error) {
	selection, err := configuredActivationProviderSelection(ctx, hub, workspaceDefault)
	if err != nil {
		return runtimecontract.ProviderInferenceSelection{}, err
	}
	validated, err := EnsureProviderInferenceSelection(ctx, hub, selection)
	if err != nil {
		return runtimecontract.ProviderInferenceSelection{}, err
	}
	return validated, nil
}

func configuredActivationProviderSelection(ctx context.Context, hub magicevent.Hub, workspaceDefault *runtimecontract.ProviderInferenceSelection) (runtimecontract.ProviderInferenceSelection, error) {
	var selection runtimecontract.ProviderInferenceSelection
	if workspaceDefault != nil {
		selection = workspaceDefault.Normalized()
	} else {
		active, err := configcore.GetActiveLLMConfiguration(ctx, hub)
		if err != nil {
			return runtimecontract.ProviderInferenceSelection{}, err
		}
		selection.Target = active.DefaultChat
	}
	if err := selection.Validate(); err != nil {
		return runtimecontract.ProviderInferenceSelection{}, err
	}
	return selection, nil
}
