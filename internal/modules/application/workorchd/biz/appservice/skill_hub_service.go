package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
)

const (
	skillSourceRepository = contextregistrycore.SkillSourceRepository
	skillSourceSkillHub   = contextregistrycore.SkillSourceSkillHub

	skillScopeRepository = contextregistrycore.SkillScopeRepository
	skillScopeGlobal     = contextregistrycore.SkillScopeGlobal
	skillScopeWorkspace  = contextregistrycore.SkillScopeWorkspace

	skillManagedByWorkorch = contextregistrycore.SkillManagedBy
	skillManagedBySkillHub = contextregistrycore.SkillManagedBySkillHub
)

type SkillQuery = contextregistrycore.SkillQuery

type SkillHubCommandRunner = contextregistryports.SkillHubCommandRunner

var skillHubRunner SkillHubCommandRunner = contextregistryports.SkillHubExecCommandRunner{}

type skillHubStatus = contextregistrycore.SkillHubStatus

func skillHubCatalogForEventHub(hub magicevent.Hub) contextregistrycore.SkillHubSkillCatalog {
	return contextregistryports.NewSkillHubCatalog(hub, listSkillHubGlobalSkills, listSkillHubWorkspaceSkills)
}

func listSkillHubGlobalSkills(ctx context.Context) ([]Skill, error) {
	return contextregistryports.ListSkillHubGlobalSkills(ctx, skillHubRunner)
}

func listSkillHubWorkspaceSkills(ctx context.Context, workspaceID string, deliverableDirectory string) ([]Skill, error) {
	return contextregistryports.ListSkillHubWorkspaceSkills(ctx, skillHubRunner, workspaceID, deliverableDirectory)
}
