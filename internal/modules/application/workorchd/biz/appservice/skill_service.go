package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func CreateSkillWithSession(ctx context.Context, session *runtimecore.Session, cmd workorchmanagementcontract.SkillMutation) (ret workorchmanagementcontract.Skill, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.Skill, error) {
		return contextregistryports.CreateSkill(ctx, hub, cmd)
	})
}

func CreateSkillWithBackend(ctx context.Context, backend string, dataRoot string, cmd workorchmanagementcontract.SkillMutation) (workorchmanagementcontract.Skill, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.Skill, error) {
		return CreateSkillWithSession(ctx, session, cmd)
	})
}
