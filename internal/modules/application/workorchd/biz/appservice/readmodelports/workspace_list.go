package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type WorkspaceListWorkspaceReader interface {
	List(ctx context.Context) ([]*workspacemodel.Workspace, error)
}

type WorkspaceListRunReader interface {
	List(ctx context.Context) ([]*runmodel.Run, error)
}

type WorkspaceListResultReader interface {
	ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error)
}

type WorkspaceListEventRecordReader interface {
	List(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
}

func ListProjectedWorkspaces(
	ctx context.Context,
	workspaces WorkspaceListWorkspaceReader,
	runs WorkspaceListRunReader,
	results WorkspaceListResultReader,
	eventRecords WorkspaceListEventRecordReader,
) ([]appmodelcore.Workspace, error) {
	var workspaceItems []*workspacemodel.Workspace
	var runItems []*runmodel.Run
	var eventRecordItems []*eventstreammodel.EventRecord
	if err := readmodelcore.RunTasks(
		func() error {
			var err error
			workspaceItems, err = workspaces.List(ctx)
			return err
		},
		func() error {
			var err error
			runItems, err = runs.List(ctx)
			return err
		},
		func() error {
			var err error
			eventRecordItems, err = eventRecords.List(ctx, repository.EventRecordQuery{Limit: readmodelcore.DefaultEventRecordLimit})
			return err
		},
	); err != nil {
		return nil, err
	}
	ret := make([]appmodelcore.Workspace, 0, len(workspaceItems))
	for _, item := range workspaceItems {
		resultItems, err := results.ListByWorkspace(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		projectionRun := readmodelprojection.SelectWorkspaceProjectionRun(item, runItems, resultItems, eventRecordItems)
		ret = append(ret, readmodelprojection.MapWorkspaceWithRunProjection(item, projectionRun))
	}
	return ret, nil
}
