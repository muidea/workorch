package appservice

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (Run, error) {
		return runports.ReadRunView(ctx, hub, common.RunID(runID))
	})
}

func ReadRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Run, error) {
		return ReadRunWithSession(ctx, session, runID)
	})
}

func ReadRunFromEnv(ctx context.Context, runID string) (Run, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Run, error) {
		return ReadRunWithSession(ctx, session, runID)
	})
}

func ListRunsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret []Run, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]Run, error) {
		return runports.ListRunViewsByWorkspace(ctx, hub, common.WorkspaceID(strings.TrimSpace(workspaceID)))
	})
}

func ListRunsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]Run, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]Run, error) {
		return ListRunsWithSession(ctx, session, workspaceID)
	})
}

func ListRunsFromEnv(ctx context.Context, workspaceID string) ([]Run, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) ([]Run, error) {
		return ListRunsWithSession(ctx, session, workspaceID)
	})
}
