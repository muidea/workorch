package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
)

func CreateSkillWithSession(ctx context.Context, session *RuntimeSession, cmd CreateSkillCommand) (ret Skill, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (Skill, error) {
		return contextregistryports.CreateSkill(ctx, hub, cmd)
	})
}

func CreateSkillWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateSkillCommand) (Skill, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Skill, error) {
		return CreateSkillWithSession(ctx, session, cmd)
	})
}
