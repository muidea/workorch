package contextregistrycore

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func ListSkillHubSkillsForQuery(ctx context.Context, catalog SkillHubSkillCatalog, query SkillQuery) []Skill {
	if catalog == nil {
		return nil
	}
	ret, err := catalog.ListGlobalSkills(ctx)
	if err != nil {
		ret = nil
	}
	workspaceID := strings.TrimSpace(query.WorkspaceID)
	if workspaceID == "" {
		return ret
	}
	workspaceSkills, err := catalog.ListWorkspaceSkills(ctx, workspaceID)
	if err != nil {
		return ret
	}
	return append(ret, workspaceSkills...)
}

func ReadSkillHubSkill(ctx context.Context, catalog SkillHubSkillCatalog, skillID string, query SkillQuery) (Skill, error) {
	if catalog == nil {
		return Skill{}, common.ErrNotFound
	}
	scope, refWorkspaceID, sourceSkillID, isSkillHubRef := ParseSkillHubSkillRef(skillID)
	if isSkillHubRef {
		switch scope {
		case SkillScopeGlobal:
			items, err := catalog.ListGlobalSkills(ctx)
			if err != nil {
				return Skill{}, err
			}
			return FindSkillByRef(items, skillID, sourceSkillID)
		case SkillScopeWorkspace:
			workspaceID := FirstNonEmpty(refWorkspaceID, query.WorkspaceID)
			if strings.TrimSpace(workspaceID) == "" {
				return Skill{}, common.ErrInvalidArgument
			}
			items, err := catalog.ListWorkspaceSkills(ctx, workspaceID)
			if err != nil {
				return Skill{}, err
			}
			return FindSkillByRef(items, skillID, sourceSkillID)
		}
		return Skill{}, common.ErrNotFound
	}

	if strings.TrimSpace(query.WorkspaceID) != "" {
		items, err := catalog.ListWorkspaceSkills(ctx, query.WorkspaceID)
		if err == nil {
			if item, err := FindSkillByRef(items, skillID, skillID); err == nil {
				return item, nil
			}
		}
	}
	items, err := catalog.ListGlobalSkills(ctx)
	if err == nil {
		if item, err := FindSkillByRef(items, skillID, skillID); err == nil {
			return item, nil
		}
	}
	return Skill{}, common.ErrNotFound
}
