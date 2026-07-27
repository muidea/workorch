package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

type runSnapshotLoadMode string

const (
	runSnapshotLoadProjection    runSnapshotLoadMode = "projection"
	runSnapshotLoadCompact       runSnapshotLoadMode = "compact"
	runSnapshotLoadViewCompact   runSnapshotLoadMode = "view_compact"
	runSnapshotLoadObservability runSnapshotLoadMode = "observability"
)

const runSnapshotViewCompactStepLimit = 100

func loadRunSnapshotModels(ctx context.Context, readers RunSnapshotReaders, item *runmodel.Run, loadMode runSnapshotLoadMode, recovery bool) (runSnapshotModels, error) {
	var models runSnapshotModels
	tasks := []func() error{
		namedLoadTask("runSnapshot.workspace", func() error {
			var err error
			models.Workspace, err = readers.Workspaces.Get(ctx, item.WorkspaceID)
			return err
		}),
		namedLoadTask("runSnapshot.steps", func() error {
			var err error
			models.Steps, err = readers.Steps.ListByRun(ctx, item.ID)
			return err
		}),
		namedLoadTask("runSnapshot.toolCalls", func() error {
			var err error
			models.ToolCalls, err = readers.ToolCalls.ListByRun(ctx, item.ID)
			return err
		}),
		namedLoadTask("runSnapshot.patches", func() error {
			var err error
			models.Patches, err = readers.Patches.ListByRun(ctx, item.ID)
			return err
		}),
		namedLoadTask("runSnapshot.validations", func() error {
			var err error
			models.Validations, err = readers.Validations.ListByRun(ctx, item.ID)
			return err
		}),
		namedLoadTask("runSnapshot.checkpoints", func() error {
			var err error
			models.Checkpoints, err = readers.Checkpoints.ListByRun(ctx, item.ID)
			return err
		}),
		namedLoadTask("runSnapshot.results", func() error {
			var err error
			models.Results, err = readers.Results.ListByRun(ctx, item.ID)
			return err
		}),
		namedLoadTask("runSnapshot.artifacts", func() error {
			var err error
			models.Artifacts, err = readers.Artifacts.ListByRun(ctx, item.ID)
			return err
		}),
		namedLoadTask("runSnapshot.artifactContinuations", func() error {
			var err error
			models.ArtifactContinuations, err = readers.ArtifactContinuations.ListByRun(ctx, item.ID)
			return err
		}),
	}
	if readers.CoverageWorkItems != nil {
		tasks = append(tasks, namedLoadTask("runSnapshot.coverageWorkItems", func() error {
			var err error
			models.CoverageWorkItems, err = readers.CoverageWorkItems.ListByRun(ctx, item.ID)
			return err
		}))
	}
	tasks = appendRunSnapshotObservabilityTasks(ctx, tasks, readers, item, loadMode, &models)
	tasks = appendRunSnapshotGovernanceTasks(ctx, tasks, readers, item, &models)
	tasks = appendRunSnapshotCapabilityTasks(ctx, tasks, readers, recovery, &models)
	if err := readmodelcore.RunTasks(tasks...); err != nil {
		return runSnapshotModels{}, err
	}
	if loadMode == runSnapshotLoadViewCompact {
		models.Steps = selectRunSnapshotViewCompactSteps(models.Steps, models.Checkpoints, runSnapshotViewCompactStepLimit)
	}
	return models, nil
}

func selectRunSnapshotViewCompactSteps(steps []*runmodel.Step, checkpoints []*runmodel.RuntimeCheckpoint, limit int) []*runmodel.Step {
	if limit <= 0 || len(steps) <= limit {
		return steps
	}
	currentStepID := latestRuntimeCheckpointStepID(checkpoints)
	start := len(steps) - limit
	selected := append([]*runmodel.Step(nil), steps[start:]...)
	if currentStepID == "" {
		return selected
	}
	for _, item := range selected {
		if item != nil && string(item.ID) == currentStepID {
			return selected
		}
	}
	for _, item := range steps[:start] {
		if item == nil || string(item.ID) != currentStepID {
			continue
		}
		selected[0] = item
		return selected
	}
	return selected
}

func latestRuntimeCheckpointStepID(items []*runmodel.RuntimeCheckpoint) string {
	for idx := len(items) - 1; idx >= 0; idx-- {
		if items[idx] != nil && items[idx].CurrentStepID != "" {
			return string(items[idx].CurrentStepID)
		}
	}
	return ""
}
