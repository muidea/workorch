package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func (m *Biz) executeRunResourceCleanupCommand(ctx context.Context, ev magicevent.Event) (any, error) {
	switch ev.ID() {
	case runevents.TopicRunCommandPurgeRuntimeResources:
		cmd, ok := ev.Data().(runevents.PurgeRuntimeResourcesCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" {
			return nil, common.ErrInvalidArgument
		}
		if _, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID); err != nil {
			return nil, err
		}
		deleted, err := purgeRunRuntimeResources(ctx, m.steps, m.checkpoints, cmd.RunID)
		return runevents.PurgeRuntimeResourcesResult{DeletedCount: deleted}, err
	case runevents.TopicRunCommandDelete:
		cmd, ok := ev.Data().(runevents.DeleteRunCommand)
		if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
			return nil, common.ErrInvalidArgument
		}
		if _, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID); err != nil {
			return nil, err
		}
		if err := m.runs.Delete(ctx, cmd.RunID); err != nil {
			return nil, err
		}
		return runevents.DeleteRunResult{Deleted: true}, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}

func purgeRunRuntimeResources(ctx context.Context, steps repository.StepRepository, checkpoints repository.RuntimeCheckpointRepository, runID common.RunID) (int, error) {
	if runID == "" {
		return 0, common.ErrInvalidArgument
	}
	deleted := 0
	if steps != nil {
		items, err := steps.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := steps.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	if checkpoints != nil {
		items, err := checkpoints.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := checkpoints.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}
