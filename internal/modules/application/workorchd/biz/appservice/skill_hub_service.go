package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

const (
	skillSourceWorkOrchRegistry = contextregistrycore.SkillSourceWorkOrchRegistry
	skillSourceSkillHub         = contextregistrycore.SkillSourceSkillHub

	skillScopeWorkOrchRegistry = contextregistrycore.SkillScopeWorkOrchRegistry
	skillScopeGlobal           = contextregistrycore.SkillScopeGlobal
	skillScopeWorkspace        = contextregistrycore.SkillScopeWorkspace

	skillManagedByWorkorch = contextregistrycore.SkillManagedBy
	skillManagedBySkillHub = contextregistrycore.SkillManagedBySkillHub
)

var skillHubRunner contextregistryports.SkillHubCommandRunner = contextregistryports.SkillHubExecCommandRunner{}

func skillHubCatalogForEventHub(hub magicevent.Hub) contextregistrycore.SkillHubSkillCatalog {
	return contextregistryports.NewSkillHubCatalog(hub, listSkillHubGlobalSkills, listSkillHubWorkspaceSkills)
}

func listSkillHubGlobalSkills(ctx context.Context) ([]workorchmanagementcontract.Skill, error) {
	return contextregistryports.ListSkillHubGlobalSkills(ctx, skillHubRunner)
}

func listSkillHubWorkspaceSkills(ctx context.Context, workspaceID string, deliverableDirectory string) ([]workorchmanagementcontract.Skill, error) {
	return contextregistryports.ListSkillHubWorkspaceSkills(ctx, skillHubRunner, workspaceID, deliverableDirectory)
}
