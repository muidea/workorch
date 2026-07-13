package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runports"
)

func CreateRunWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runID string, goalID string, dodID string) (ret Run, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (Run, error) {
		request, err := createRunViewRequest(ctx, hub, workspaceID, runID, goalID, dodID)
		if err != nil {
			return Run{}, err
		}
		return runports.CreateRunView(ctx, request)
	})
}

func CreateRunWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string, goalID string, dodID string) (Run, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Run, error) {
		return CreateRunWithSession(ctx, session, workspaceID, runID, goalID, dodID)
	})
}

func CreateRunFromEnv(ctx context.Context, workspaceID string, runID string, goalID string, dodID string) (Run, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Run, error) {
		return CreateRunWithSession(ctx, session, workspaceID, runID, goalID, dodID)
	})
}

func StartRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (Run, error) {
		return runports.StartRunView(ctx, hub, runID)
	})
}

func StartRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	return withSharedBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Run, error) {
		return StartRunWithSession(ctx, session, runID)
	})
}

func StartRunFromEnv(ctx context.Context, runID string) (Run, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Run, error) {
		return StartRunWithSession(ctx, session, runID)
	})
}

func ResumeRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (Run, error) {
		return runports.ResumeRunView(ctx, hub, runID)
	})
}

func ResumeRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	return withSharedBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Run, error) {
		return ResumeRunWithSession(ctx, session, runID)
	})
}

func ResumeRunFromEnv(ctx context.Context, runID string) (Run, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Run, error) {
		return ResumeRunWithSession(ctx, session, runID)
	})
}
