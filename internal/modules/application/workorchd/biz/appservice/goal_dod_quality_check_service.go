package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionquality"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
)

func CheckGoalDoDQualityWithSession(ctx context.Context, session *runtimecore.Session, cmd definitionquality.Command) (ret definitionquality.Check, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (definitionquality.Check, error) {
		return definitionquality.CheckGoalDoDQuality(ctx, hub, cmd)
	})
}

func CheckGoalDoDQualityWithBackend(ctx context.Context, backend string, dataRoot string, cmd definitionquality.Command) (definitionquality.Check, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (definitionquality.Check, error) {
		return CheckGoalDoDQualityWithSession(ctx, session, cmd)
	})
}

func CheckGoalDoDQualityFromEnv(ctx context.Context, cmd definitionquality.Command) (definitionquality.Check, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (definitionquality.Check, error) {
		return CheckGoalDoDQualityWithSession(ctx, session, cmd)
	})
}
