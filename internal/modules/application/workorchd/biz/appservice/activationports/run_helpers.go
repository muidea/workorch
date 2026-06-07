package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func RunObservableAfterStartConflict(ctx context.Context, hub events.Hub, runID common.RunID) (bool, error) {
	if hub == nil || runID == "" {
		return false, common.ErrInvalidArgument
	}
	run, err := ownerreadports.ReadRunByOwnerCommand(ctx, hub, runID)
	if err != nil {
		return false, err
	}
	if run == nil {
		return false, nil
	}
	return activationcore.IsObservableRunState(run.State), nil
}

func NextWorkspaceExecutionRunID(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) (common.RunID, error) {
	if hub == nil || workspaceID == "" {
		return "", common.ErrInvalidArgument
	}
	items, err := ownerreadports.ListRunsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
	if err != nil {
		return "", err
	}
	maxSeq := 0
	for _, item := range items {
		if item == nil {
			continue
		}
		if seq, ok := activationcore.ParseNumericRunID(item.ID); ok && seq > maxSeq {
			maxSeq = seq
		}
	}
	return common.RunID(activationcore.FormatNumericRunID(maxSeq + 1)), nil
}

func RecordActivationMemoryRecallTrace(ctx context.Context, hub events.Hub, runID common.RunID, workspaceID common.WorkspaceID, executionContext map[string]any, occurredAt string) error {
	if hub == nil || runID == "" || workspaceID == "" {
		return nil
	}
	trace := activationcore.BuildActivationMemoryRecallTrace(runID, workspaceID, executionContext, occurredAt)
	if trace == nil {
		return nil
	}
	return ownercommandports.RecordMemoryRecallTrace(ctx, hub, trace)
}
