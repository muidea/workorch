package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type SkillHubGlobalLister func(context.Context) ([]workorchmanagementcontract.Skill, error)
type SkillHubWorkspaceLister func(context.Context, string, string) ([]workorchmanagementcontract.Skill, error)

type skillHubCatalog struct {
	hub           magicevent.Hub
	globalLister  SkillHubGlobalLister
	workspaceList SkillHubWorkspaceLister
}

func NewSkillHubCatalog(hub magicevent.Hub, globalLister SkillHubGlobalLister, workspaceLister SkillHubWorkspaceLister) contextregistrycore.SkillHubSkillCatalog {
	return skillHubCatalog{
		hub:           hub,
		globalLister:  globalLister,
		workspaceList: workspaceLister,
	}
}

func (catalog skillHubCatalog) ListGlobalSkills(ctx context.Context) ([]workorchmanagementcontract.Skill, error) {
	if catalog.globalLister == nil {
		return nil, nil
	}
	return catalog.globalLister(ctx)
}

func (catalog skillHubCatalog) ListWorkspaceSkills(ctx context.Context, workspaceID string) ([]workorchmanagementcontract.Skill, error) {
	if catalog.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	if catalog.workspaceList == nil {
		return nil, nil
	}
	workspace, err := workspaceports.ReadWorkspaceRecord(ctx, catalog.hub, common.WorkspaceID(strings.TrimSpace(workspaceID)))
	if err != nil {
		return nil, err
	}
	return catalog.workspaceList(ctx, string(workspace.ID), workspace.DeliverableDirectory)
}
