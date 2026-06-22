package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type governanceSessionDependencies struct {
	hub            events.Hub
	traces         governancecore.AutoGovernanceTraceReader
	workspaceScope common.WorkspaceID
}

func withReadModelSession[T any](ctx context.Context, session *RuntimeSession, read func(context.Context, readModelDependencies) (T, error)) (ret T, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return ret, err
	}
	defer session.CloseOnReturn(ctx, &err)
	return read(ctx, deps)
}

func withEventHubSession[T any](ctx context.Context, session *RuntimeSession, read func(context.Context, events.Hub) (T, error)) (ret T, err error) {
	hub, err := session.RequiredEventHub()
	if err != nil {
		return ret, err
	}
	defer session.CloseOnReturn(ctx, &err)
	return read(ctx, hub)
}

func withGovernanceSession[T any](ctx context.Context, session *RuntimeSession, read func(context.Context, governanceSessionDependencies) (T, error)) (ret T, err error) {
	hub, err := session.RequiredEventHub()
	if err != nil {
		return ret, err
	}
	deps := governanceSessionDependencies{
		hub:            hub,
		traces:         readmodelports.AutoGovernanceTraceReader{Hub: hub},
		workspaceScope: common.RuntimeWorkspaceIDFromContext(ctx),
	}
	defer session.CloseOnReturn(ctx, &err)
	return read(ctx, deps)
}

func withBackendSession[T any](ctx context.Context, backend string, dataRoot string, read func(context.Context, *RuntimeSession) (T, error)) (ret T, err error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return ret, err
	}
	defer session.CloseOnReturn(ctx, &err)
	return read(ctx, session)
}

func withEnvSession[T any](ctx context.Context, read func(context.Context, *RuntimeSession) (T, error)) (ret T, err error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return ret, err
	}
	defer session.CloseOnReturn(ctx, &err)
	return read(ctx, session)
}
