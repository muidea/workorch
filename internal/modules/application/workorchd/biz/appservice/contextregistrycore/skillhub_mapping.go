package contextregistrycore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func MapSkillHubStatusItem(item SkillHubStatusItem, scope string, workspaceID string, status SkillHubStatus, instruction string, metadata map[string]string) workorchmanagementcontract.Skill {
	sourceSkillID := strings.TrimSpace(item.SkillID)
	if sourceSkillID == "" {
		return workorchmanagementcontract.Skill{}
	}
	return workorchmanagementcontract.Skill{
		ID:           SkillHubSkillRef(scope, workspaceID, sourceSkillID),
		Name:         FirstNonEmpty(metadata["name"], sourceSkillID),
		Version:      FirstNonEmpty(item.Version, item.LocalVersion, item.RepoVersion, metadata["version"]),
		Description:  FirstNonEmpty(metadata["description"], strings.TrimSpace(item.Message)),
		TriggerRules: FrontMatterStringList(metadata["triggerRules"], metadata["trigger_rules"], metadata["triggers"]),
		Instruction:  instruction,
		RequiredToolKinds: FrontMatterStringList(
			metadata["requiredToolKinds"],
			metadata["required_tool_kinds"],
			metadata["requiredTools"],
			metadata["required_tools"],
		),
		RequiredMCPServerRefs: FrontMatterStringList(
			metadata["requiredMCPServerRefs"],
			metadata["required_mcp_server_refs"],
			metadata["requiredMcpServers"],
			metadata["required_mcp_servers"],
		),
		Constraints:      FrontMatterStringList(metadata["constraints"]),
		Source:           SkillSourceSkillHub,
		Scope:            scope,
		WorkspaceID:      strings.TrimSpace(workspaceID),
		Status:           item.Status,
		SourceRepository: item.SourceRepository,
		SourcePath:       item.SourcePath,
		TargetPath:       item.TargetPath,
		LocalPath:        item.LocalPath,
		RepoPath:         item.RepoPath,
		GlobalPath:       status.GlobalPath,
		ProjectPath:      status.ProjectPath,
		ContentHash:      FirstNonEmpty(item.ActualHash, item.AppliedHash, item.SourceHash),
		SourceHash:       item.SourceHash,
		AppliedHash:      item.AppliedHash,
		ActualHash:       item.ActualHash,
		Agent:            item.Agent,
		ManagedBy:        SkillManagedBySkillHub,
		SourceSkillID:    sourceSkillID,
	}
}

func MapSkillHubStatusItemWithLocalInstruction(item SkillHubStatusItem, scope string, workspaceID string, status SkillHubStatus) workorchmanagementcontract.Skill {
	instruction, metadata := ReadSkillHubInstructionAndMetadata(item)
	return MapSkillHubStatusItem(item, scope, workspaceID, status, instruction, metadata)
}
