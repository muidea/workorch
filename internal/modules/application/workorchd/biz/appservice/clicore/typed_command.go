package clicore

import (
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

func legacyCommandPayload(command workorchdevents.CLICommand) Command {
	return Command{
		CommandID:     command.CommandID,
		CorrelationID: command.CorrelationID,
		Actor:         command.Actor,
		IssuedAt:      command.IssuedAt,
		WorkspaceID:   command.WorkspaceID,
		RunID:         command.RunID,
		SubjectType:   command.SubjectType,
		SubjectID:     command.SubjectID,
		Command:       command.Command,
		Payload:       legacyCommandData(command),
	}
}

func legacyCommandData(command workorchdevents.CLICommand) map[string]any {
	switch command.Command {
	case workorchdevents.CommandWorkspaceCreate:
		if command.CreateWorkspace == nil {
			return nil
		}
		return map[string]any{
			"title":                command.CreateWorkspace.Title,
			"description":          command.CreateWorkspace.Description,
			"deliverableDirectory": command.CreateWorkspace.DeliverableDirectory,
			"templateId":           command.CreateWorkspace.TemplateID,
		}
	case workorchdevents.CommandWorkspaceDelete:
		return map[string]any{"purge": command.Purge}
	case workorchdevents.CommandGoalDefine, workorchdevents.CommandGoalUpdate:
		return map[string]any{"text": command.Text}
	case workorchdevents.CommandDoDDefine, workorchdevents.CommandDoDUpdate:
		return map[string]any{"items": append([]string(nil), command.Items...)}
	case workorchdevents.CommandDecisionResolve:
		return map[string]any{
			"resolutionType": command.ResolutionType,
			"payload":        cliDataMap(command.GovernanceData),
		}
	case workorchdevents.CommandDecisionBatch:
		return map[string]any{
			"ticketIds":      ticketIDs(command),
			"resolutionType": command.ResolutionType,
			"payload":        cliDataMap(command.GovernanceData),
		}
	case workorchdevents.CommandApprovalApprove, workorchdevents.CommandApprovalReject:
		return map[string]any{"payload": cliDataMap(command.GovernanceData)}
	case workorchdevents.CommandApprovalBatch:
		return map[string]any{
			"approvalIds": approvalIDs(command),
			"action":      command.ApprovalAction,
			"payload":     cliDataMap(command.GovernanceData),
		}
	case workorchdevents.CommandSoulCreate, workorchdevents.CommandSoulUpdate:
		if command.Soul == nil {
			return nil
		}
		return map[string]any{
			"id":              string(command.Soul.ID),
			"name":            command.Soul.Name,
			"description":     command.Soul.Description,
			"businessDomains": append([]string(nil), command.Soul.BusinessDomains...),
			"soulMd":          command.Soul.SoulMD,
		}
	case workorchdevents.CommandSkillCreate:
		if command.Skill == nil {
			return nil
		}
		return map[string]any{
			"id":          string(command.Skill.ID),
			"name":        command.Skill.Name,
			"version":     command.Skill.Version,
			"description": command.Skill.Description,
			"instruction": command.Skill.Instruction,
		}
	case workorchdevents.CommandMCPServerCreate, workorchdevents.CommandMCPServerUpdate:
		if command.MCPServer == nil {
			return nil
		}
		return map[string]any{
			"id":        string(command.MCPServer.ID),
			"name":      command.MCPServer.Name,
			"transport": command.MCPServer.Transport,
			"endpoint":  command.MCPServer.Endpoint,
			"enabled":   command.MCPServer.Enabled,
		}
	case workorchdevents.CommandMemoryCreate, workorchdevents.CommandMemoryUpdate:
		return legacyMemoryData(command)
	case workorchdevents.CommandMemorySearch:
		if command.MemorySearch == nil {
			return nil
		}
		return map[string]any{
			"query": command.MemorySearch.Query,
			"tags":  append([]string(nil), command.MemorySearch.Tags...),
			"types": append([]string(nil), command.MemorySearch.Types...),
			"limit": command.MemorySearch.Limit,
		}
	case workorchdevents.CommandTemplateCreate, workorchdevents.CommandTemplateUpdate:
		if command.Template == nil {
			return nil
		}
		return map[string]any{
			"id":               command.Template.ID,
			"name":             command.Template.Name,
			"description":      command.Template.Description,
			"goalText":         command.Template.GoalText,
			"doDItems":         append([]string(nil), command.Template.DoDItems...),
			"defaultSoulId":    string(command.Template.DefaultSoulID),
			"soulBindings":     command.Template.SoulBindings,
			"evidencePolicies": command.Template.EvidencePolicies,
		}
	default:
		return nil
	}
}

func cliDataMap(data workorchdevents.CLIData) map[string]any {
	if len(data.Fields) == 0 {
		return nil
	}
	ret := make(map[string]any, len(data.Fields))
	for _, field := range data.Fields {
		if field.Name == "" {
			continue
		}
		ret[field.Name] = cliDataAny(field.Value)
	}
	return ret
}

func cliDataAny(value workorchdevents.CLIDataValue) any {
	switch value.Kind {
	case "null":
		return nil
	case "bool":
		return value.Bool
	case "number":
		return value.Number
	case "object":
		return cliDataMap(value.Object)
	case "array":
		ret := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			ret = append(ret, cliDataAny(item))
		}
		return ret
	default:
		return value.String
	}
}

func legacyMemoryData(command workorchdevents.CLICommand) map[string]any {
	if command.Memory == nil {
		return nil
	}
	return map[string]any{
		"memoryId":            string(command.Memory.ID),
		"runId":               string(command.Memory.RunID),
		"sourceType":          command.Memory.SourceType,
		"sourceRef":           command.Memory.SourceRef,
		"memoryType":          command.Memory.MemoryType,
		"content":             command.Memory.Content,
		"summary":             command.Memory.Summary,
		"tags":                append([]string(nil), command.Memory.Tags...),
		"importance":          command.Memory.Importance,
		"confidence":          command.Memory.Confidence,
		"visibility":          command.Memory.Visibility,
		"embeddingProviderId": command.Memory.EmbeddingProviderID,
		"embeddingModel":      command.Memory.EmbeddingModel,
		"embeddingText":       command.Memory.EmbeddingText,
		"vector":              append([]float64(nil), command.Memory.Vector...),
	}
}

func ticketIDs(command workorchdevents.CLICommand) []string {
	ret := make([]string, 0, len(command.DecisionIDs))
	for _, id := range command.DecisionIDs {
		ret = append(ret, string(id))
	}
	return ret
}

func approvalIDs(command workorchdevents.CLICommand) []string {
	ret := make([]string, 0, len(command.ApprovalIDs))
	for _, id := range command.ApprovalIDs {
		ret = append(ret, string(id))
	}
	return ret
}
