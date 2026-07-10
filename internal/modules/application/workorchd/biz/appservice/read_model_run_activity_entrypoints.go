package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
)

func ListRunStepsWithSession(ctx context.Context, session *RuntimeSession, query StepQuery) (ret StepPage, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (StepPage, error) {
		return readmodelports.ReadRunStepsPage(ctx, hub, query)
	})
}

func ListRunStepsWithBackend(ctx context.Context, backend string, dataRoot string, query StepQuery) (StepPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (StepPage, error) {
		return ListRunStepsWithSession(ctx, session, query)
	})
}

func ListRunToolCallsWithSession(ctx context.Context, session *RuntimeSession, query ToolCallQuery) (ret ToolCallPage, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (ToolCallPage, error) {
		return readmodelports.ReadRunToolCallsPage(ctx, hub, query)
	})
}

func ListRunToolCallsWithBackend(ctx context.Context, backend string, dataRoot string, query ToolCallQuery) (ToolCallPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (ToolCallPage, error) {
		return ListRunToolCallsWithSession(ctx, session, query)
	})
}

func ListRunRuntimeCheckpointsWithSession(ctx context.Context, session *RuntimeSession, query RuntimeCheckpointQuery) (ret RuntimeCheckpointPage, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (RuntimeCheckpointPage, error) {
		return readmodelports.ReadRunRuntimeCheckpointsPage(ctx, hub, query)
	})
}

func ListRunRuntimeCheckpointsWithBackend(ctx context.Context, backend string, dataRoot string, query RuntimeCheckpointQuery) (RuntimeCheckpointPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (RuntimeCheckpointPage, error) {
		return ListRunRuntimeCheckpointsWithSession(ctx, session, query)
	})
}
