package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadRunView(ctx context.Context, hub magicevent.Hub, runID common.RunID) (appmodelcore.Run, error) {
	item, err := ReadRun(ctx, hub, runID)
	if err != nil {
		return appmodelcore.Run{}, err
	}
	return RunView(item), nil
}

func ListRunViewsByWorkspace(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]appmodelcore.Run, error) {
	items, err := ListRunsByWorkspace(ctx, hub, workspaceID)
	if err != nil {
		return nil, err
	}
	return RunViews(items), nil
}

func RunView(item *runmodel.Run) appmodelcore.Run {
	return runtimecore.MapRun(item)
}

func RunViewWithFailureReason(item *runmodel.Run, reason string) appmodelcore.Run {
	ret := RunView(item)
	ret.FailureReason = reason
	return ret
}

func RunViews(items []*runmodel.Run) []appmodelcore.Run {
	ret := make([]appmodelcore.Run, 0, len(items))
	for _, item := range items {
		ret = append(ret, RunView(item))
	}
	return ret
}
