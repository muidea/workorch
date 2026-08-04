package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/qualitycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/qualityports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
)

func CheckGoalDoDQualityWithSession(ctx context.Context, session *runtimecore.Session, cmd qualitycore.Command) (ret qualitycore.Check, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (qualitycore.Check, error) {
		return qualityports.CheckGoalDoDQuality(ctx, hub, cmd)
	})
}

func CheckGoalDoDQualityWithBackend(ctx context.Context, backend string, dataRoot string, cmd qualitycore.Command) (qualitycore.Check, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (qualitycore.Check, error) {
		return CheckGoalDoDQualityWithSession(ctx, session, cmd)
	})
}

func CheckGoalDoDQualityFromEnv(ctx context.Context, cmd qualitycore.Command) (qualitycore.Check, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (qualitycore.Check, error) {
		return CheckGoalDoDQualityWithSession(ctx, session, cmd)
	})
}
