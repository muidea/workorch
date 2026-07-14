package contextregistrycore

import (
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func MCPServerModelFromCommand(cmd MCPServer) *mcpmodel.Server {
	return &mcpmodel.Server{
		ID:                    common.MCPServerID(cmd.ID),
		Name:                  cmd.Name,
		Transport:             cmd.Transport,
		Endpoint:              cmd.Endpoint,
		Command:               cmd.Command,
		Args:                  CloneStrings(cmd.Args),
		Env:                   cloneStringMap(cmd.Env),
		Enabled:               cmd.Enabled,
		AllowedTools:          CloneStrings(cmd.AllowedTools),
		AllowedResources:      CloneStrings(cmd.AllowedResources),
		AllowedPrompts:        CloneStrings(cmd.AllowedPrompts),
		DefaultApprovalPolicy: cmd.DefaultApprovalPolicy,
		SessionPolicy:         cmd.SessionPolicy,
		TimeoutMillis:         cmd.TimeoutMillis,
	}
}
