package activationcore

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

func MCPServerForWorkspace(item *mcpmodel.Server, workspace *workspacemodel.Workspace) *mcpmodel.Server {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.Args = contextregistrycore.CloneStrings(item.Args)
	cloned.AllowedTools = contextregistrycore.CloneStrings(item.AllowedTools)
	cloned.AllowedResources = contextregistrycore.CloneStrings(item.AllowedResources)
	cloned.AllowedPrompts = contextregistrycore.CloneStrings(item.AllowedPrompts)
	cloned.Env = CloneMCPEnvForWorkspace(item.Env, workspace)
	return &cloned
}

func CloneMCPEnvForWorkspace(env map[string]string, workspace *workspacemodel.Workspace) map[string]string {
	if env == nil {
		return nil
	}
	ret := make(map[string]string, len(env))
	for key, value := range env {
		ret[key] = ExpandWorkspaceContextPlaceholders(value, workspace)
	}
	return ret
}

func ExpandWorkspaceContextPlaceholders(value string, workspace *workspacemodel.Workspace) string {
	if workspace == nil {
		return value
	}
	value = expandManagedConfigPlaceholders(value, workspace)
	replacements := map[string]string{
		"{workspaceId}":                   string(workspace.ID),
		"{workspaceDeliverableDirectory}": workspace.DeliverableDirectory,
		"{workspaceInternalDirectory}":    workspace.InternalDirectory,
	}
	for old, replacement := range replacements {
		value = strings.ReplaceAll(value, old, replacement)
	}
	return value
}

func expandManagedConfigPlaceholders(value string, workspace *workspacemodel.Workspace) string {
	return workorchmanagedconfig.ExpandPlaceholders(value, workspace.InternalDirectory)
}
