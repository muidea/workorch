package activationcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func SoulContext(item *soulmodel.Soul) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{
		"id":                   string(item.ID),
		"name":                 item.Name,
		"description":          item.Description,
		"businessDomains":      contextregistrycore.CloneStrings(item.BusinessDomains),
		"soulMd":               item.SoulMD,
		"allowedSkillRefs":     contextregistrycore.CloneStrings(item.AllowedSkillRefs),
		"allowedMCPServerRefs": contextregistrycore.CloneStrings(item.AllowedMCPServerRefs),
		"allowedToolKinds":     contextregistrycore.CloneStrings(item.AllowedToolKinds),
	}
}

func SkillContexts(items []workorchmanagementcontract.Skill) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":                     item.ID,
			"name":                   item.Name,
			"version":                item.Version,
			"description":            item.Description,
			"triggerRules":           contextregistrycore.CloneStrings(item.TriggerRules),
			"instruction":            item.Instruction,
			"referenceRefs":          contextregistrycore.CloneStrings(item.ReferenceRefs),
			"requiredToolKinds":      contextregistrycore.CloneStrings(item.RequiredToolKinds),
			"requiredMCPServerRefs":  contextregistrycore.CloneStrings(item.RequiredMCPServerRefs),
			"requiredCapabilityRefs": contextregistrycore.CloneStrings(item.RequiredCapabilityRefs),
			"constraints":            contextregistrycore.CloneStrings(item.Constraints),
			"source":                 item.Source,
			"scope":                  item.Scope,
			"workspaceId":            item.WorkspaceID,
			"status":                 item.Status,
			"sourceRepository":       item.SourceRepository,
			"sourcePath":             item.SourcePath,
			"targetPath":             item.TargetPath,
			"localPath":              item.LocalPath,
			"repoPath":               item.RepoPath,
			"globalPath":             item.GlobalPath,
			"projectPath":            item.ProjectPath,
			"contentHash":            item.ContentHash,
			"sourceHash":             item.SourceHash,
			"appliedHash":            item.AppliedHash,
			"actualHash":             item.ActualHash,
			"agent":                  item.Agent,
			"managedBy":              item.ManagedBy,
			"sourceSkillId":          item.SourceSkillID,
		})
	}
	return ret
}
