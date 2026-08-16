package contextregistryports

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func mcpServerCommandFromMutation(cmd workorchmanagementcontract.MCPServerMutation) mcpevents.CreateServerCommand {
	return mcpevents.CreateServerCommand{
		ServerID:              common.MCPServerID(strings.TrimSpace(cmd.ID)),
		Name:                  cmd.Name,
		Transport:             cmd.Transport,
		Endpoint:              cmd.Endpoint,
		Command:               cmd.Command,
		Args:                  contextregistrycore.CloneStrings(cmd.Args),
		Env:                   mcpevents.EnvironmentVariablesFromMap(cmd.Env),
		Enabled:               cmd.Enabled,
		AllowedTools:          contextregistrycore.CloneStrings(cmd.AllowedTools),
		AllowedResources:      contextregistrycore.CloneStrings(cmd.AllowedResources),
		AllowedPrompts:        contextregistrycore.CloneStrings(cmd.AllowedPrompts),
		DefaultApprovalPolicy: cmd.DefaultApprovalPolicy,
		SessionPolicy:         cmd.SessionPolicy,
		TimeoutMillis:         cmd.TimeoutMillis,
	}
}

func mcpCapabilityCommandFromMutation(cmd workorchmanagementcontract.MCPCapabilityMutation) (mcpevents.CreateCapabilityCommand, error) {
	schema, err := workorcheventpayload.FromMap(cmd.Schema)
	if err != nil {
		return mcpevents.CreateCapabilityCommand{}, fmt.Errorf("invalid mcp capability schema: %w", err)
	}
	return mcpevents.CreateCapabilityCommand{
		CapabilityID:                 common.MCPCapabilityID(strings.TrimSpace(cmd.ID)),
		ServerID:                     common.MCPServerID(strings.TrimSpace(cmd.ServerID)),
		Kind:                         cmd.Kind,
		Name:                         cmd.Name,
		DisplayName:                  cmd.DisplayName,
		Description:                  cmd.Description,
		Category:                     cmd.Category,
		Tags:                         contextregistrycore.CloneStrings(cmd.Tags),
		UsageHint:                    cmd.UsageHint,
		InputSummary:                 cmd.InputSummary,
		OutputSummary:                cmd.OutputSummary,
		StageHints:                   contextregistrycore.CloneStrings(cmd.StageHints),
		RiskLevel:                    cmd.RiskLevel,
		EffectScope:                  cmd.EffectScope,
		IdempotencyClass:             cmd.IdempotencyClass,
		ValidationProofApplicability: cmd.ValidationProofApplicability,
		Schema:                       schema,
		URI:                          cmd.URI,
		Enabled:                      cmd.Enabled,
		ApprovalPolicy:               cmd.ApprovalPolicy,
	}, nil
}

func mcpCapabilityQueryFromView(query contextregistrycore.MCPCapabilityQuery) mcpevents.CapabilityQuery {
	return mcpevents.CapabilityQuery{
		ServerID:  common.MCPServerID(strings.TrimSpace(query.ServerID)),
		Kind:      strings.TrimSpace(query.Kind),
		Category:  strings.TrimSpace(query.Category),
		Stage:     strings.TrimSpace(query.Stage),
		RiskLevel: strings.TrimSpace(query.RiskLevel),
		Enabled:   query.Enabled,
		Query:     strings.TrimSpace(query.Query),
	}
}
