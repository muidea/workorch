package activationcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
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
	// Registry environment values are references. Resolution belongs to the MCP
	// execution owner so neither activation nor Runtime snapshots contain a
	// credential value.
	_ = workspace
	cloned.Env = CloneMCPEnvironmentRefs(item.Env)
	return &cloned
}

func CloneMCPEnvironmentRefs(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	ret := make(map[string]string, len(env))
	for key, reference := range env {
		ret[key] = reference
	}
	return ret
}
