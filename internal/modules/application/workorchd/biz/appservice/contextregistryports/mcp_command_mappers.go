package contextregistryports

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpcontracts "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
)

func mcpServerCommandFromView(cmd contextregistrycore.MCPServer) mcpcontracts.CreateServerCommand {
	return mcpcontracts.CreateServerCommand{
		ServerID:              common.MCPServerID(strings.TrimSpace(cmd.ID)),
		Name:                  cmd.Name,
		Transport:             cmd.Transport,
		Endpoint:              cmd.Endpoint,
		Command:               cmd.Command,
		Args:                  contextregistrycore.CloneStrings(cmd.Args),
		Env:                   cloneStringMap(cmd.Env),
		Enabled:               cmd.Enabled,
		AllowedTools:          contextregistrycore.CloneStrings(cmd.AllowedTools),
		AllowedResources:      contextregistrycore.CloneStrings(cmd.AllowedResources),
		AllowedPrompts:        contextregistrycore.CloneStrings(cmd.AllowedPrompts),
		DefaultApprovalPolicy: cmd.DefaultApprovalPolicy,
		SessionPolicy:         cmd.SessionPolicy,
		TimeoutMillis:         cmd.TimeoutMillis,
	}
}

func mcpCapabilityCommandFromView(cmd contextregistrycore.MCPCapability) mcpcontracts.CreateCapabilityCommand {
	return mcpcontracts.CreateCapabilityCommand{
		ID:             common.MCPCapabilityID(strings.TrimSpace(cmd.ID)),
		ServerID:       common.MCPServerID(strings.TrimSpace(cmd.ServerID)),
		Kind:           cmd.Kind,
		Name:           cmd.Name,
		DisplayName:    cmd.DisplayName,
		Description:    cmd.Description,
		Category:       cmd.Category,
		Tags:           contextregistrycore.CloneStrings(cmd.Tags),
		UsageHint:      cmd.UsageHint,
		InputSummary:   cmd.InputSummary,
		OutputSummary:  cmd.OutputSummary,
		StageHints:     contextregistrycore.CloneStrings(cmd.StageHints),
		RiskLevel:      cmd.RiskLevel,
		Schema:         cloneMap(cmd.Schema),
		URI:            cmd.URI,
		Enabled:        cmd.Enabled,
		ApprovalPolicy: cmd.ApprovalPolicy,
		Status:         cmd.Status,
	}
}

func mcpCapabilityQueryFromView(query contextregistrycore.MCPCapabilityQuery) mcpcontracts.CapabilityQuery {
	return mcpcontracts.CapabilityQuery{
		ServerID:  common.MCPServerID(strings.TrimSpace(query.ServerID)),
		Kind:      strings.TrimSpace(query.Kind),
		Category:  strings.TrimSpace(query.Category),
		Stage:     strings.TrimSpace(query.Stage),
		RiskLevel: strings.TrimSpace(query.RiskLevel),
		Enabled:   query.Enabled,
		Query:     strings.TrimSpace(query.Query),
	}
}
