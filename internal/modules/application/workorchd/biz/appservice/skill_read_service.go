package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func ReadSkillWithSession(ctx context.Context, session *runtimecore.Session, skillID string, query ...contextregistrycore.SkillQuery) (ret workorchmanagementcontract.Skill, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.Skill, error) {
		return contextregistryports.ReadSkillView(ctx, hub, skillID, contextregistrycore.FirstSkillQuery(query), skillHubCatalogForEventHub(hub))
	})
}

func ReadSkillWithBackend(ctx context.Context, backend string, dataRoot string, skillID string, query ...contextregistrycore.SkillQuery) (workorchmanagementcontract.Skill, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.Skill, error) {
		return ReadSkillWithSession(ctx, session, skillID, query...)
	})
}

func ListSkillsWithSession(ctx context.Context, session *runtimecore.Session, query ...contextregistrycore.SkillQuery) (ret []workorchmanagementcontract.Skill, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.Skill, error) {
		return contextregistryports.ListSkillsView(ctx, hub, contextregistrycore.FirstSkillQuery(query), skillHubCatalogForEventHub(hub))
	})
}

func ListSkillsWithBackend(ctx context.Context, backend string, dataRoot string, query ...contextregistrycore.SkillQuery) ([]workorchmanagementcontract.Skill, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]workorchmanagementcontract.Skill, error) {
		return ListSkillsWithSession(ctx, session, query...)
	})
}
