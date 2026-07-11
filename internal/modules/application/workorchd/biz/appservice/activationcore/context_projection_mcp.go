package activationcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
)

func MCPServerContexts(items []*mcpmodel.Server) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, map[string]any{
			"id":                    string(item.ID),
			"name":                  item.Name,
			"transport":             item.Transport,
			"enabled":               item.Enabled,
			"command":               item.Command,
			"args":                  contextregistrycore.CloneStrings(item.Args),
			"env":                   RedactedMCPEnvContext(item.Env),
			"allowedTools":          contextregistrycore.CloneStrings(item.AllowedTools),
			"allowedResources":      contextregistrycore.CloneStrings(item.AllowedResources),
			"defaultApprovalPolicy": item.DefaultApprovalPolicy,
			"timeoutMillis":         item.TimeoutMillis,
		})
	}
	return ret
}

func RedactedMCPEnvContext(env map[string]string) map[string]any {
	if len(env) == 0 {
		return nil
	}
	ret := make(map[string]any, len(env))
	for key, value := range env {
		ret[key] = map[string]any{
			"present":  value != "",
			"redacted": value != "",
		}
	}
	return ret
}

func MCPCapabilityContexts(items []*mcpmodel.Capability) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, map[string]any{
			"id":             string(item.ID),
			"serverId":       string(item.ServerID),
			"kind":           item.Kind,
			"name":           item.Name,
			"displayName":    item.DisplayName,
			"description":    item.Description,
			"category":       item.Category,
			"tags":           contextregistrycore.CloneStrings(item.Tags),
			"usageHint":      item.UsageHint,
			"inputSummary":   item.InputSummary,
			"outputSummary":  item.OutputSummary,
			"stageHints":     contextregistrycore.CloneStrings(item.StageHints),
			"riskLevel":      item.RiskLevel,
			"uri":            item.URI,
			"enabled":        item.Enabled,
			"approvalPolicy": item.ApprovalPolicy,
			"status":         item.Status,
		})
	}
	return ret
}
