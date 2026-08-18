package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeCheckpointQueryCommand(ctx context.Context, ev magicevent.Event) (any, error) {
	switch ev.ID() {
	case runevents.TopicRunCommandGetRuntimeCheckpoint:
		cmd, ok := ev.Data().(runevents.GetRuntimeCheckpointCommand)
		if !ok || cmd.CheckpointID == "" || m.checkpoints == nil {
			return nil, common.ErrInvalidArgument
		}
		item, err := m.checkpoints.Get(ctx, cmd.CheckpointID)
		if err != nil {
			return nil, err
		}
		if cmd.RunID != "" && cmd.RunID != item.RunID {
			return nil, common.ErrConflict
		}
		if cmd.WorkspaceID != "" && item.WorkspaceID != cmd.WorkspaceID {
			return nil, common.ErrNotFound
		}
		if err := m.validateRunReadScope(ctx, item.RunID, cmd.WorkspaceID); err != nil {
			return nil, err
		}
		return runevents.GetRuntimeCheckpointResult{Checkpoint: item}, nil
	case runevents.TopicRunCommandListCheckpointsByRun:
		cmd, ok := ev.Data().(runevents.ListRuntimeCheckpointsByRunCommand)
		if !ok || cmd.RunID == "" || m.checkpoints == nil {
			return nil, common.ErrInvalidArgument
		}
		if err := m.validateRunReadScope(ctx, cmd.RunID, cmd.WorkspaceID); err != nil {
			return nil, err
		}
		items, err := m.checkpoints.ListByRun(ctx, cmd.RunID)
		if err != nil {
			return nil, err
		}
		return runevents.ListRuntimeCheckpointsByRunResult{Checkpoints: items}, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}
