package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func (m *Biz) executeCreateRunCommand(ctx context.Context, ev magicevent.Event) (runevents.RunCommandResult, error) {
	cmd, ok := ev.Data().(runevents.CreateRunCommand)
	if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	if err := m.CreateRun(ctx, cmd); err != nil {
		return runevents.RunCommandResult{}, err
	}
	// The Run owner emits the immutable activation-memory fact after the Run
	// record is durable. Memory remains the sole owner of RecallTrace
	// persistence; this notification is only the typed cross-owner handoff.
	// It is deliberately best-effort so an observability write cannot turn a
	// successful Run creation into an ambiguous side-effect error. Upsert is
	// keyed by the stable run activation trace ID, so duplicate create/replay
	// notifications remain idempotent.
	m.publishRuntimeActivationMemoryRecall(ctx, cmd)
	return runevents.RunCommandResult{}, nil
}

func (m *Biz) executeRunResourceCleanupCommand(ctx context.Context, ev magicevent.Event) (runevents.RunCommandResult, error) {
	switch ev.ID() {
	case runevents.TopicRunCommandPurgeRuntimeResources:
		cmd, ok := ev.Data().(runevents.PurgeRuntimeResourcesCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if _, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID); err != nil {
			return runevents.RunCommandResult{}, err
		}
		deleted, err := purgeRunRuntimeResources(ctx, m.steps, m.checkpoints, cmd.RunID)
		return runevents.RunCommandResult{DeletedCount: deleted}, err
	case runevents.TopicRunCommandDelete:
		cmd, ok := ev.Data().(runevents.DeleteRunCommand)
		if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if _, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID); err != nil {
			return runevents.RunCommandResult{}, err
		}
		if err := m.runs.Delete(ctx, cmd.RunID); err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{}, nil
	default:
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
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
