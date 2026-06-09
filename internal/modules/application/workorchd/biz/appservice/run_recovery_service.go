package appservice

import (
	"context"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runports"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const stuckRunningRunFailureReason = runports.StuckRunningRunFailureReason
const orphanDispatchedStepRecoveryReason = runports.OrphanDispatchedStepRecoveryReason
const staleDispatchedStepRecoveryReason = runports.StaleDispatchedStepRecoveryReason
const maxOrphanDispatchedStepRecoveryIterations = runports.MaxOrphanDispatchedStepRecoveryIterations
const DefaultStaleDispatchedStepMinAge = runports.DefaultStaleDispatchedStepMinAge

func RecoverStuckRunningRunsWithSession(ctx context.Context, session *RuntimeSession) (ret []Run, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]Run, error) {
		return runports.RecoverStuckRunningRunViews(ctx, hub, func(resumeCtx context.Context, current *runmodel.Run) error {
			if session == nil || current == nil {
				return common.ErrInvalidArgument
			}
			_, err := ResumeRunWithSession(resumeCtx, session.Borrow(), string(current.ID))
			return err
		})
	})
}

func RecoverStuckRunningRunsWithBackend(ctx context.Context, backend string, dataRoot string) ([]Run, error) {
	return withBackendSession(ctx, backend, dataRoot, RecoverStuckRunningRunsWithSession)
}

func RecoverStaleDispatchedRunsWithSession(ctx context.Context, session *RuntimeSession, minAge time.Duration, now time.Time) (ret []Run, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]Run, error) {
		return runports.RecoverStaleDispatchedRunViews(ctx, hub, func(resumeCtx context.Context, current *runmodel.Run) error {
			if session == nil || current == nil {
				return common.ErrInvalidArgument
			}
			_, err := ResumeRunWithSession(resumeCtx, session.Borrow(), string(current.ID))
			return err
		}, minAge, now)
	})
}

func RecoverStaleDispatchedRunsWithBackend(ctx context.Context, backend string, dataRoot string, minAge time.Duration, now time.Time) ([]Run, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]Run, error) {
		return RecoverStaleDispatchedRunsWithSession(ctx, session, minAge, now)
	})
}
