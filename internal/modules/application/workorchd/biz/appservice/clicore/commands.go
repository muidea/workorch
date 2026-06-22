package clicore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
)

func CreateWorkspaceCommandFromPayload(payload map[string]any) appmodelcore.CreateWorkspaceCommand {
	return appmodelcore.CreateWorkspaceCommand{
		Title:                StringValue(payload, "title"),
		Description:          StringValue(payload, "description"),
		DeliverableDirectory: StringValue(payload, "deliverableDirectory"),
		TemplateID:           StringValue(payload, "templateId"),
	}
}

func ActivateWorkspaceExecutionCommandFromPayload(payload map[string]any) appmodelcore.ActivateWorkspaceExecutionCommand {
	return appmodelcore.ActivateWorkspaceExecutionCommand{
		GoalID:                StringValue(payload, "goalId"),
		DoDID:                 StringValue(payload, "doDId"),
		DialogLanguage:        StringValue(payload, "dialogLanguage"),
		SoulID:                StringValue(payload, "soulId"),
		SkillIDs:              StringSliceValue(payload, "skillIds"),
		MCPServerIDs:          StringSliceValue(payload, "mcpServerIds"),
		Priority:              IntValue(payload, "priority"),
		ExecutionContextPatch: MapValue(payload, "executionContextPatch"),
	}
}

func ResolveDecisionCommandFromPayload(payload map[string]any) appmodelcore.ResolveDecisionCommand {
	return appmodelcore.ResolveDecisionCommand{
		ResolutionType:    StringValue(payload, "resolutionType"),
		ResolutionPayload: FirstMapValue(payload, "payload", "resolutionPayload"),
	}
}

func BatchResolveDecisionCommandFromPayload(payload map[string]any) appmodelcore.BatchResolveDecisionCommand {
	return appmodelcore.BatchResolveDecisionCommand{
		TicketIDs:         StringSliceValue(payload, "ticketIds"),
		ResolutionType:    StringValue(payload, "resolutionType"),
		ResolutionPayload: FirstMapValue(payload, "payload", "resolutionPayload"),
	}
}

func BatchResolveApprovalCommandFromPayload(payload map[string]any) appmodelcore.BatchResolveApprovalCommand {
	return appmodelcore.BatchResolveApprovalCommand{
		ApprovalIDs: StringSliceValue(payload, "approvalIds"),
		Action:      StringValue(payload, "action"),
		Payload:     FirstMapValue(payload, "payload", "resolutionPayload"),
	}
}

func ResolveApprovalCommandFromPayload(action string, payload map[string]any) appmodelcore.ResolveApprovalCommand {
	return appmodelcore.ResolveApprovalCommand{
		Action:  action,
		Payload: FirstMapValue(payload, "payload", "resolutionPayload"),
	}
}

func MemoryCommandFromPayload(payload map[string]any) contextregistrycore.CreateMemoryItemCommand {
	cmd := *DecodeOrZero[contextregistrycore.CreateMemoryItemCommand](payload)
	if cmd.MemoryID == "" {
		cmd.MemoryID = FirstStringValue(payload, "memoryId", "ID", "id")
	}
	return cmd
}

func MemorySearchQueryFromPayload(workspaceID string, payload map[string]any) contextregistrycore.MemoryQuery {
	return contextregistrycore.MemoryQuery{
		WorkspaceID: workspaceID,
		Query:       StringValue(payload, "query"),
		Tags:        StringSliceValue(payload, "tags"),
		Types:       StringSliceValue(payload, "types"),
		Limit:       IntValue(payload, "limit"),
	}
}
