package activationcore

import (
	"context"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type WorkspaceTemplateReader interface {
	Get(context.Context, string) (*repository.WorkspaceTemplate, error)
	List(context.Context) ([]*repository.WorkspaceTemplate, error)
}

func WorkspaceExecutionContract(ctx context.Context, templates WorkspaceTemplateReader, workspace *workspacemodel.Workspace) map[string]any {
	contract := map[string]any{
		"stateChangePolicy": "allowed_but_not_required",
	}
	if workspace == nil {
		return contract
	}
	hints := WorkspaceExecutionContractHints(ctx, templates, workspace)
	if value, ok := hints["stateChangePolicy"].(string); ok && strings.TrimSpace(value) != "" {
		contract["stateChangePolicy"] = strings.TrimSpace(value)
	}
	if profileContracts, ok := hints["profileContracts"].(map[string]any); ok && len(profileContracts) > 0 {
		contract["profileContracts"] = cloneContextValue(profileContracts)
	}
	if policies, ok := hints["capabilityUsagePolicies"].([]any); ok && len(policies) > 0 {
		contract["capabilityUsagePolicies"] = cloneContextValue(policies)
	}
	if policies, ok := hints["validationPolicies"].([]any); ok && len(policies) > 0 {
		contract["validationPolicies"] = cloneContextValue(policies)
	}
	if runtimeRepair, ok := hints["runtimeRepair"].(map[string]any); ok && len(runtimeRepair) > 0 {
		contract["runtimeRepair"] = cloneContextValue(runtimeRepair)
	}
	if policy, ok := hints["acceptanceSlicePolicy"].(map[string]any); ok && len(policy) > 0 {
		contract["acceptanceSlicePolicy"] = cloneContextValue(policy)
	}
	if policy, ok := hints["templateStagePolicies"].(map[string]any); ok && len(policy) > 0 {
		contract["templateStagePolicies"] = cloneContextValue(policy)
	}
	return contract
}

func WorkspaceExecutionContractHints(ctx context.Context, templates WorkspaceTemplateReader, workspace *workspacemodel.Workspace) map[string]any {
	if workspace == nil {
		return nil
	}
	workspaceHints := cloneContextMapDeep(workspace.ContextHints.Map())
	templateHints := MatchingWorkspaceTemplateContextHints(ctx, templates, workspace)
	if len(templateHints) == 0 {
		return workspaceHints
	}
	return MergeTemplateWorkspaceContextHintsForExecutionContract(templateHints, workspaceHints)
}
