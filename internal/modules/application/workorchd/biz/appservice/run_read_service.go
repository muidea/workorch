package appservice

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadRunWithSession(ctx context.Context, session *runtimecore.Session, runID string) (ret appmodelcore.Run, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (appmodelcore.Run, error) {
		return runports.ReadRunView(ctx, hub, common.RunID(runID))
	})
}

func ReadRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (appmodelcore.Run, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Run, error) {
		return ReadRunWithSession(ctx, session, runID)
	})
}

func ReadRunFromEnv(ctx context.Context, runID string) (appmodelcore.Run, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Run, error) {
		return ReadRunWithSession(ctx, session, runID)
	})
}

func ListRunsWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string) (ret []appmodelcore.Run, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) ([]appmodelcore.Run, error) {
		return listRunViews(ctx, deps, common.WorkspaceID(strings.TrimSpace(workspaceID)))
	})
}

func ListRunsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]appmodelcore.Run, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]appmodelcore.Run, error) {
		return ListRunsWithSession(ctx, session, workspaceID)
	})
}

func ListRunsFromEnv(ctx context.Context, workspaceID string) ([]appmodelcore.Run, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) ([]appmodelcore.Run, error) {
		return ListRunsWithSession(ctx, session, workspaceID)
	})
}

func listRunViews(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) ([]appmodelcore.Run, error) {
	var items []*runmodel.Run
	var err error
	if workspaceID == "" {
		items, err = deps.runs.List(ctx)
	} else {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		items, err = deps.runs.ListByWorkspaceID(ctx, workspaceID)
	}
	if err != nil {
		return nil, err
	}
	ret := make([]appmodelcore.Run, 0, len(items))
	for _, item := range items {
		view := runtimecore.MapRun(item)
		hydrated, err := hydrateRunCurrentStep(ctx, deps, view)
		if err != nil {
			return nil, err
		}
		ret = append(ret, hydrated)
	}
	return ret, nil
}

func hydrateRunCurrentStep(ctx context.Context, deps readModelDependencies, item appmodelcore.Run) (appmodelcore.Run, error) {
	if item.ID == "" || !runListCurrentStepHydrationRequired(item.State) {
		return item, nil
	}
	status, err := readRunStatus(common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(item.WorkspaceID)), deps, common.RunID(item.ID))
	if err != nil {
		return appmodelcore.Run{}, err
	}
	item.CurrentStepID = status.CurrentStepID
	item.CurrentStepKind = status.CurrentStepKind
	item.CurrentStepState = status.CurrentStepState
	return item, nil
}

func runListCurrentStepHydrationRequired(state string) bool {
	switch common.RunState(strings.TrimSpace(state)) {
	case common.RunStateActive, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
		return true
	default:
		return false
	}
}
