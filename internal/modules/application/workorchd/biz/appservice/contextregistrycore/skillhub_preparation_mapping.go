package contextregistrycore

import (
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
	"slices"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

// MapPreparationSkillHubItem only projects already observed bytes. Status,
// directory hashes and a successful projection do not confer currentness.
func MapPreparationSkillHubItem(item SkillHubStatusItem, scope, workspaceID string, status SkillHubStatus, instruction []byte, metadata skillhubpreparation.SkillHubPreparationMetadata) workorchmanagementcontract.Skill {
	value := workorchmanagementcontract.Skill{
		ID: SkillHubSkillRef(scope, workspaceID, item.SkillID), Name: FirstNonEmpty(metadata.Name, item.SkillID),
		// Prefer metadata from the actual installed bytes. Repository version
		// is a distinct diagnostic field, never an installed-content fallback.
		Version:     FirstNonEmpty(metadata.Version, item.LocalVersion, item.Version),
		Description: FirstNonEmpty(metadata.Description, item.Message), Instruction: string(instruction),
		TriggerRules: slices.Clone(metadata.TriggerRules), ReferenceRefs: slices.Clone(metadata.ReferenceRefs),
		RequiredToolKinds: slices.Clone(metadata.RequiredToolKinds), RequiredMCPServerRefs: slices.Clone(metadata.RequiredMCPServerRefs),
		RequiredCapabilityRefs: slices.Clone(metadata.RequiredCapabilityRefs), Constraints: slices.Clone(metadata.Constraints),
		Source: SkillSourceSkillHub, Scope: scope, WorkspaceID: workspaceID, Status: item.Status,
		SourceRepository: item.SourceRepository, SourcePath: item.SourcePath, TargetPath: item.TargetPath,
		LocalPath: item.LocalPath, RepoPath: item.RepoPath, GlobalPath: status.GlobalPath, ProjectPath: status.ProjectPath,
		SourceHash: item.SourceHash, AppliedHash: item.AppliedHash, ActualHash: item.ActualHash,
		Agent: item.Agent, ManagedBy: SkillManagedBySkillHub, SourceSkillID: item.SkillID,
	}
	value.ContentHash = workorchskillcontract.StableContentHash(skillContent(value))
	return value
}
