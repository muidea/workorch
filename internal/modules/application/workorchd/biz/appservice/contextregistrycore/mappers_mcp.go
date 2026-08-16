package contextregistrycore

import (
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func MapMCPServer(item *mcpmodel.Server) workorchmanagementcontract.MCPServer {
	if item == nil {
		return workorchmanagementcontract.MCPServer{}
	}
	return workorchmanagementcontract.MCPServer{
		ID:                    string(item.ID),
		Name:                  item.Name,
		Transport:             item.Transport,
		Endpoint:              item.Endpoint,
		Command:               item.Command,
		Args:                  CloneStrings(item.Args),
		Env:                   RedactStringMap(item.Env),
		Enabled:               item.Enabled,
		AllowedTools:          CloneStrings(item.AllowedTools),
		AllowedResources:      CloneStrings(item.AllowedResources),
		AllowedPrompts:        CloneStrings(item.AllowedPrompts),
		DefaultApprovalPolicy: item.DefaultApprovalPolicy,
		SessionPolicy:         item.SessionPolicy,
		TimeoutMillis:         item.TimeoutMillis,
	}
}

func MCPToolsFromServer(server workorchmanagementcontract.MCPServer) []workorchmanagementcontract.MCPTool {
	ret := make([]workorchmanagementcontract.MCPTool, 0, len(server.AllowedTools))
	for _, tool := range server.AllowedTools {
		ret = append(ret, workorchmanagementcontract.MCPTool{ServerID: server.ID, Name: tool})
	}
	return ret
}

func MCPResourcesFromServer(server workorchmanagementcontract.MCPServer) []workorchmanagementcontract.MCPResource {
	ret := make([]workorchmanagementcontract.MCPResource, 0, len(server.AllowedResources))
	for _, resource := range server.AllowedResources {
		ret = append(ret, workorchmanagementcontract.MCPResource{ServerID: server.ID, URI: resource})
	}
	return ret
}

func MCPPromptsFromServer(server workorchmanagementcontract.MCPServer) []workorchmanagementcontract.MCPPrompt {
	ret := make([]workorchmanagementcontract.MCPPrompt, 0, len(server.AllowedPrompts))
	for _, prompt := range server.AllowedPrompts {
		ret = append(ret, workorchmanagementcontract.MCPPrompt{ServerID: server.ID, Name: prompt})
	}
	return ret
}

func MapMCPCapability(item *mcpmodel.Capability) workorchmanagementcontract.MCPCapability {
	if item == nil {
		return workorchmanagementcontract.MCPCapability{}
	}
	return workorchmanagementcontract.MCPCapability{
		ID:                           string(item.ID),
		ServerID:                     string(item.ServerID),
		Kind:                         item.Kind,
		Name:                         item.Name,
		DisplayName:                  item.DisplayName,
		Description:                  item.Description,
		Category:                     item.Category,
		Tags:                         CloneStrings(item.Tags),
		UsageHint:                    item.UsageHint,
		InputSummary:                 item.InputSummary,
		OutputSummary:                item.OutputSummary,
		StageHints:                   CloneStrings(item.StageHints),
		RiskLevel:                    item.RiskLevel,
		EffectScope:                  item.EffectScope,
		IdempotencyClass:             item.IdempotencyClass,
		ValidationProofApplicability: item.ValidationProofApplicability,
		Schema:                       item.Schema.Map(),
		URI:                          item.URI,
		Enabled:                      item.Enabled,
		ApprovalPolicy:               item.ApprovalPolicy,
		Status:                       item.Status,
	}
}
