package appservice

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runports"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const stuckRunningRunFailureReason = runports.StuckRunningRunFailureReason
const orphanDispatchedStepRecoveryReason = runports.OrphanDispatchedStepRecoveryReason
const staleDispatchedStepRecoveryReason = runports.StaleDispatchedStepRecoveryReason
const maxOrphanDispatchedStepRecoveryIterations = runports.MaxOrphanDispatchedStepRecoveryIterations
const DefaultStaleDispatchedStepMinAge = runports.DefaultStaleDispatchedStepMinAge

func RecoverStuckRunningRunsWithSession(ctx context.Context, session *RuntimeSession) (ret []Run, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]Run, error) {
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
	return withSharedBackendSession(ctx, backend, dataRoot, RecoverStuckRunningRunsWithSession)
}

func RecoverStartupInactiveRunningWorkspacesWithSession(ctx context.Context, session *RuntimeSession) (ret []Run, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]Run, error) {
		return runports.RecoverStartupInactiveRunningWorkspaceViews(ctx, hub)
	})
}

func RecoverStartupInactiveRunningWorkspacesWithBackend(ctx context.Context, backend string, dataRoot string) ([]Run, error) {
	return withSharedBackendSession(ctx, backend, dataRoot, RecoverStartupInactiveRunningWorkspacesWithSession)
}

func RecoverStaleDispatchedRunsWithSession(ctx context.Context, session *RuntimeSession, minAge time.Duration, now time.Time) (ret []Run, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) ([]Run, error) {
		return runports.RecoverStaleRuntimeGapRunViews(ctx, hub, func(recoverCtx context.Context, current *runmodel.Run, recovery runports.RuntimeGapRecovery) error {
			if session == nil || current == nil {
				return common.ErrInvalidArgument
			}
			return runports.RecoverRuntimeGap(recoverCtx, hub, current, recovery, nil)
		}, minAge, now)
	})
}

func RecoverStaleDispatchedRunsWithBackend(ctx context.Context, backend string, dataRoot string, minAge time.Duration, now time.Time) ([]Run, error) {
	return withSharedBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]Run, error) {
		return RecoverStaleDispatchedRunsWithSession(ctx, session, minAge, now)
	})
}
