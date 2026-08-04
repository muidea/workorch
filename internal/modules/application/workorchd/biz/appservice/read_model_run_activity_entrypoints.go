package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
)

func ListRunStepsWithSession(ctx context.Context, session *runtimecore.Session, query readmodelcore.StepQuery) (ret readmodelcore.StepPage, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (readmodelcore.StepPage, error) {
		return readmodelports.ReadRunStepsPage(ctx, hub, query)
	})
}

func ListRunStepsWithBackend(ctx context.Context, backend string, dataRoot string, query readmodelcore.StepQuery) (readmodelcore.StepPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.StepPage, error) {
		return ListRunStepsWithSession(ctx, session, query)
	})
}

func ListRunToolCallsWithSession(ctx context.Context, session *runtimecore.Session, query readmodelcore.ToolCallQuery) (ret readmodelcore.ToolCallPage, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (readmodelcore.ToolCallPage, error) {
		return readmodelports.ReadRunToolCallsPage(ctx, hub, query)
	})
}

func ListRunToolCallsWithBackend(ctx context.Context, backend string, dataRoot string, query readmodelcore.ToolCallQuery) (readmodelcore.ToolCallPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.ToolCallPage, error) {
		return ListRunToolCallsWithSession(ctx, session, query)
	})
}

func ListRunRuntimeCheckpointsWithSession(ctx context.Context, session *runtimecore.Session, query readmodelcore.RuntimeCheckpointQuery) (ret readmodelcore.RuntimeCheckpointPage, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (readmodelcore.RuntimeCheckpointPage, error) {
		return readmodelports.ReadRunRuntimeCheckpointsPage(ctx, hub, query)
	})
}

func ListRunRuntimeCheckpointsWithBackend(ctx context.Context, backend string, dataRoot string, query readmodelcore.RuntimeCheckpointQuery) (readmodelcore.RuntimeCheckpointPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.RuntimeCheckpointPage, error) {
		return ListRunRuntimeCheckpointsWithSession(ctx, session, query)
	})
}
