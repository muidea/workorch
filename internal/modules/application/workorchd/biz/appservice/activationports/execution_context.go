package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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

	return WorkspaceRuntimeActivationBuild{
		EffectiveGoal:          effectiveGoal,
		EffectiveDoD:           effectiveDoD,
		DefinitionRevision:     activationcore.DefinitionRevisionID(string(effectiveGoal.ID), string(effectiveDoD.ID)),
		DefinitionChangeImpact: RuntimeV2DefinitionChangeImpact(definitionChangeImpact),
		RuntimeActivation: RuntimeV2ActivationContext(
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
