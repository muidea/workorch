package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/qualitycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/qualityports"
	"github.com/muidea/workorch/internal/pkg/events"
)

type GoalDoDQualityCheckCommand = qualitycore.Command
type GoalDoDQualityIssue = qualitycore.Issue
type GoalDoDQualityCheck = qualitycore.Check

func CheckGoalDoDQualityWithSession(ctx context.Context, session *RuntimeSession, cmd GoalDoDQualityCheckCommand) (ret GoalDoDQualityCheck, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (GoalDoDQualityCheck, error) {
		return qualityports.CheckGoalDoDQuality(ctx, hub, cmd)
	})
}

func CheckGoalDoDQualityWithBackend(ctx context.Context, backend string, dataRoot string, cmd GoalDoDQualityCheckCommand) (GoalDoDQualityCheck, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (GoalDoDQualityCheck, error) {
		return CheckGoalDoDQualityWithSession(ctx, session, cmd)
	})
}

func CheckGoalDoDQualityFromEnv(ctx context.Context, cmd GoalDoDQualityCheckCommand) (GoalDoDQualityCheck, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (GoalDoDQualityCheck, error) {
		return CheckGoalDoDQualityWithSession(ctx, session, cmd)
	})
}
