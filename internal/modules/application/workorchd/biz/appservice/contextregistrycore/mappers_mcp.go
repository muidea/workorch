package contextregistrycore

import mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"

func MapMCPServer(item *mcpmodel.Server) MCPServer {
	if item == nil {
		return MCPServer{}
	}
	return MCPServer{
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

func MCPToolsFromServer(server MCPServer) []MCPTool {
	ret := make([]MCPTool, 0, len(server.AllowedTools))
	for _, tool := range server.AllowedTools {
		ret = append(ret, MCPTool{ServerID: server.ID, Name: tool})
	}
	return ret
}

func MCPResourcesFromServer(server MCPServer) []MCPResource {
	ret := make([]MCPResource, 0, len(server.AllowedResources))
	for _, resource := range server.AllowedResources {
		ret = append(ret, MCPResource{ServerID: server.ID, URI: resource})
	}
	return ret
}

func MCPPromptsFromServer(server MCPServer) []MCPPrompt {
	ret := make([]MCPPrompt, 0, len(server.AllowedPrompts))
	for _, prompt := range server.AllowedPrompts {
		ret = append(ret, MCPPrompt{ServerID: server.ID, Name: prompt})
	}
	return ret
}

func MapMCPCapability(item *mcpmodel.Capability) MCPCapability {
	if item == nil {
		return MCPCapability{}
	}
	return MCPCapability{
		ID:             string(item.ID),
		ServerID:       string(item.ServerID),
		Kind:           item.Kind,
		Name:           item.Name,
		DisplayName:    item.DisplayName,
		Description:    item.Description,
		Category:       item.Category,
		Tags:           CloneStrings(item.Tags),
		UsageHint:      item.UsageHint,
		InputSummary:   item.InputSummary,
		OutputSummary:  item.OutputSummary,
		StageHints:     CloneStrings(item.StageHints),
		RiskLevel:      item.RiskLevel,
		Schema:         CloneMap(item.Schema),
		URI:            item.URI,
		Enabled:        item.Enabled,
		ApprovalPolicy: item.ApprovalPolicy,
		Status:         item.Status,
	}
}
