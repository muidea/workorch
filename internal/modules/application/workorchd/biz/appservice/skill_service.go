package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func CreateSkillWithSession(ctx context.Context, session *RuntimeSession, cmd CreateSkillCommand) (ret Skill, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (Skill, error) {
		return contextregistryports.CreateSkill(ctx, hub, cmd)
	})
}

func CreateSkillWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateSkillCommand) (Skill, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Skill, error) {
		return CreateSkillWithSession(ctx, session, cmd)
	})
}
