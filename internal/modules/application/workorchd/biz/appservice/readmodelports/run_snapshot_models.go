package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

type runSnapshotLoadMode string

const (
	runSnapshotLoadProjection    runSnapshotLoadMode = "projection"
	runSnapshotLoadObservability runSnapshotLoadMode = "observability"
)

func loadRunSnapshotModels(ctx context.Context, readers RunSnapshotReaders, item *runmodel.Run, loadMode runSnapshotLoadMode, recovery bool) (runSnapshotModels, error) {
	var models runSnapshotModels
	tasks := []func() error{
		func() error {
			var err error
			models.Workspace, err = readers.Workspaces.Get(ctx, item.WorkspaceID)
			return err
		},
		func() error {
			var err error
			models.Steps, err = readers.Steps.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			models.ToolCalls, err = readers.ToolCalls.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			models.Patches, err = readers.Patches.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			models.Validations, err = readers.Validations.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			models.Checkpoints, err = readers.Checkpoints.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			models.Results, err = readers.Results.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			models.Artifacts, err = readers.Artifacts.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			models.ArtifactContinuations, err = readers.ArtifactContinuations.ListByRun(ctx, item.ID)
			return err
		},
	}
	if readers.CoverageWorkItems != nil {
		tasks = append(tasks, func() error {
			var err error
			models.CoverageWorkItems, err = readers.CoverageWorkItems.ListByRun(ctx, item.ID)
			return err
		})
	}
	tasks = appendRunSnapshotObservabilityTasks(ctx, tasks, readers, item, loadMode, &models)
	tasks = appendRunSnapshotGovernanceTasks(ctx, tasks, readers, item, &models)
	tasks = appendRunSnapshotCapabilityTasks(ctx, tasks, readers, recovery, &models)
	if err := readmodelcore.RunTasks(tasks...); err != nil {
		return runSnapshotModels{}, err
	}
	return models, nil
}
