package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadSkillWithSession(ctx context.Context, session *RuntimeSession, skillID string, query ...SkillQuery) (ret Skill, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (Skill, error) {
		return contextregistryports.ReadSkillView(ctx, hub, skillID, contextregistrycore.FirstSkillQuery(query), skillHubCatalogForEventHub(hub))
	})
}

func ReadSkillWithBackend(ctx context.Context, backend string, dataRoot string, skillID string, query ...SkillQuery) (Skill, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Skill, error) {
		return ReadSkillWithSession(ctx, session, skillID, query...)
	})
}

func ListSkillsWithSession(ctx context.Context, session *RuntimeSession, query ...SkillQuery) (ret []Skill, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]Skill, error) {
		return contextregistryports.ListSkillsView(ctx, hub, contextregistrycore.FirstSkillQuery(query), skillHubCatalogForEventHub(hub))
	})
}

func ListSkillsWithBackend(ctx context.Context, backend string, dataRoot string, query ...SkillQuery) ([]Skill, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]Skill, error) {
		return ListSkillsWithSession(ctx, session, query...)
	})
}
