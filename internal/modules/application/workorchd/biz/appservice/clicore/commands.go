package clicore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
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
		LLMProviderID:         StringValue(payload, "llmProviderId"),
		DialogLanguage:        StringValue(payload, "dialogLanguage"),
		SoulID:                StringValue(payload, "soulId"),
		SoulBindings:          StringMapValue(payload, "soulBindings"),
		SkillIDs:              StringSliceValue(payload, "skillIds"),
		MCPServerIDs:          StringSliceValue(payload, "mcpServerIds"),
		CapabilityIDs:         StringSliceValue(payload, "capabilityIds"),
		MemoryIDs:             StringSliceValue(payload, "memoryIds"),
		Priority:              IntValue(payload, "priority"),
		Restart:               BoolValue(payload, "restart"),
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
	cmd := contextregistrycore.CreateMemoryItemCommand{
		MemoryID:            FirstStringValue(payload, "memoryId", "ID", "id"),
		RunID:               StringValue(payload, "runId"),
		SourceType:          StringValue(payload, "sourceType"),
		SourceRef:           StringValue(payload, "sourceRef"),
		MemoryType:          StringValue(payload, "memoryType"),
		Content:             StringValue(payload, "content"),
		Summary:             StringValue(payload, "summary"),
		Tags:                StringSliceValue(payload, "tags"),
		Importance:          IntValue(payload, "importance"),
		Confidence:          IntValue(payload, "confidence"),
		Visibility:          StringValue(payload, "visibility"),
		EmbeddingProviderID: StringValue(payload, "embeddingProviderId"),
		EmbeddingModel:      StringValue(payload, "embeddingModel"),
		EmbeddingText:       StringValue(payload, "embeddingText"),
		Vector:              FloatSliceValue(payload, "vector"),
	}
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

func SoulCommandFromPayload(payload map[string]any) contextregistrycore.Soul {
	return contextregistrycore.Soul{
		ID:                          FirstStringValue(payload, "id", "ID"),
		Name:                        StringValue(payload, "name"),
		Description:                 StringValue(payload, "description"),
		BusinessDomains:             StringSliceValue(payload, "businessDomains"),
		SoulMD:                      FirstStringValue(payload, "soulMd", "soulMD"),
		AllowedSkillRefs:            StringSliceValue(payload, "allowedSkillRefs"),
		AllowedMCPServerRefs:        StringSliceValue(payload, "allowedMCPServerRefs"),
		AllowedCapabilityRefs:       StringSliceValue(payload, "allowedCapabilityRefs"),
		AllowedCapabilityCategories: StringSliceValue(payload, "allowedCapabilityCategories"),
		AllowedToolKinds:            StringSliceValue(payload, "allowedToolKinds"),
	}
}

func SkillCommandFromPayload(payload map[string]any) contextregistrycore.Skill {
	return contextregistrycore.Skill{
		ID:                     FirstStringValue(payload, "id", "ID"),
		Name:                   StringValue(payload, "name"),
		Version:                StringValue(payload, "version"),
		Description:            StringValue(payload, "description"),
		TriggerRules:           StringSliceValue(payload, "triggerRules"),
		Instruction:            StringValue(payload, "instruction"),
		ReferenceRefs:          StringSliceValue(payload, "referenceRefs"),
		RequiredToolKinds:      StringSliceValue(payload, "requiredToolKinds"),
		RequiredMCPServerRefs:  StringSliceValue(payload, "requiredMCPServerRefs"),
		RequiredCapabilityRefs: StringSliceValue(payload, "requiredCapabilityRefs"),
		Constraints:            StringSliceValue(payload, "constraints"),
		Source:                 StringValue(payload, "source"),
		Scope:                  StringValue(payload, "scope"),
		WorkspaceID:            StringValue(payload, "workspaceId"),
		Status:                 StringValue(payload, "status"),
		SourceRepository:       StringValue(payload, "sourceRepository"),
		SourcePath:             StringValue(payload, "sourcePath"),
		TargetPath:             StringValue(payload, "targetPath"),
		LocalPath:              StringValue(payload, "localPath"),
		RepoPath:               StringValue(payload, "repoPath"),
		GlobalPath:             StringValue(payload, "globalPath"),
		ProjectPath:            StringValue(payload, "projectPath"),
		ContentHash:            StringValue(payload, "contentHash"),
		SourceHash:             StringValue(payload, "sourceHash"),
		AppliedHash:            StringValue(payload, "appliedHash"),
		ActualHash:             StringValue(payload, "actualHash"),
		Agent:                  StringValue(payload, "agent"),
		ManagedBy:              StringValue(payload, "managedBy"),
		SourceSkillID:          StringValue(payload, "sourceSkillId"),
	}
}

func MCPServerCommandFromPayload(payload map[string]any) contextregistrycore.MCPServer {
	return contextregistrycore.MCPServer{
		ID:                    FirstStringValue(payload, "id", "ID"),
		Name:                  StringValue(payload, "name"),
		Transport:             StringValue(payload, "transport"),
		Endpoint:              StringValue(payload, "endpoint"),
		Command:               StringValue(payload, "command"),
		Args:                  StringSliceValue(payload, "args"),
		Env:                   StringMapValue(payload, "env"),
		Enabled:               BoolValue(payload, "enabled"),
		AllowedTools:          StringSliceValue(payload, "allowedTools"),
		AllowedResources:      StringSliceValue(payload, "allowedResources"),
		AllowedPrompts:        StringSliceValue(payload, "allowedPrompts"),
		DefaultApprovalPolicy: StringValue(payload, "defaultApprovalPolicy"),
		SessionPolicy:         StringValue(payload, "sessionPolicy"),
		TimeoutMillis:         IntValue(payload, "timeoutMillis"),
	}
}

func WorkspaceTemplateFromPayload(payload map[string]any) workspacecore.WorkspaceTemplate {
	return workspacecore.WorkspaceTemplate{
		ID:               FirstStringValue(payload, "id", "ID"),
		Name:             StringValue(payload, "name"),
		Description:      StringValue(payload, "description"),
		GoalText:         StringValue(payload, "goalText"),
		DoDItems:         StringSliceValue(payload, "doDItems"),
		DefaultSoulID:    StringValue(payload, "defaultSoulId"),
		SoulBindings:     StringMapValue(payload, "soulBindings"),
		ContextHints:     MapValue(payload, "contextHints"),
		EvidencePolicies: MapSliceValue(payload, "evidencePolicies"),
	}
}
