package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func BuildWorkspaceExecutionContext(ctx context.Context, request WorkspaceExecutionContextRequest) (map[string]any, error) {
	workspace := request.Workspace
	goal := request.Goal
	dod := request.DoD
	if request.Hub == nil || workspace == nil || goal == nil || dod == nil || goal.WorkspaceID != workspace.ID || dod.WorkspaceID != workspace.ID {
		return nil, common.ErrInvalidArgument
	}
	definitions, err := WorkspaceExecutionDefinitionsContext(ctx, request.Hub, workspace.ID, goal, dod)
	if err != nil {
		return nil, err
	}
	effectiveGoal := definitions.EffectiveGoal
	effectiveDoD := definitions.EffectiveDoD
	definitionChangeImpact := definitions.DefinitionChangeImpact
	command := request.Command
	dialogLanguage := activationcore.DialogLanguage(command.DialogLanguage, request.DefaultDialogLanguage)
	contextData := activationcore.WorkspaceExecutionContextBaseContext(activationcore.WorkspaceExecutionContextBase{
		WorkspaceID:            workspace.ID,
		Goal:                   effectiveGoal,
		DoD:                    effectiveDoD,
		GoalContexts:           definitions.GoalContexts,
		DoDContexts:            definitions.DoDContexts,
		DefinitionChangeImpact: definitionChangeImpact,
		ExecutionContract:      WorkspaceExecutionContract(ctx, request.Hub, workspace),
		DialogLanguage:         dialogLanguage,
		EvidencePolicies:       workspace.EvidencePolicies,
	})
	toolHints := WorkspaceToolContextHintsFor(ctx, request.Hub, workspace)
	if err := workspacecore.EnsureManagedConfigs(workspace.InternalDirectory, toolHints.Template, toolHints.Workspace); err != nil {
		return nil, err
	}
	defaultMCPServerIDs := activationcore.DefaultMCPServerIDsFromContextHints(toolHints.Effective)
	defaultCapabilityIDs := activationcore.DefaultCapabilityIDsFromContextHints(toolHints.Effective)
	memoryIDs := activationcore.NormalizeStringIDs(command.MemoryIDs)
	bindings, err := loadWorkspaceExecutionContextBindings(ctx, request, effectiveGoal, effectiveDoD, memoryIDs)
	if err != nil {
		return nil, err
	}
	defaultSoul := bindings.defaultSoul
	memories := bindings.memories
	retrievalEvidence := bindings.retrievalEvidence
	governanceHistory := bindings.governanceHistory
	soulRouting := bindings.soulRouting
	appendSoulRoutingContext(contextData, defaultSoul, soulRouting)

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
		return nil, err
	}
	skills := skillResult.Skills
	skillIDs := skillResult.IDs
	appendSkillSelectionContext(contextData, skillResult)

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
		return nil, err
	}
	appendMCPSelectionContext(contextData, mcpSelection)

	if err := ValidateSoulRoutingCapabilitySelection(ctx, request.Hub, soulRouting, skillIDs, mcpSelection.MCPServerIDs, mcpSelection.CapabilityIDs, activationcore.CapabilityCategories(mcpSelection.Capabilities), activationcore.RequiredToolKinds(skills)); err != nil {
		return nil, err
	}

	appendMemorySelectionContext(contextData, memories, retrievalEvidence)
	if err := mergeWorkspaceExecutionContinuations(ctx, contextData, workspace.ID, effectiveGoal.ID, effectiveDoD.ID, definitionChangeImpact, governanceHistory, request); err != nil {
		return nil, err
	}

	return contextData, nil
}
