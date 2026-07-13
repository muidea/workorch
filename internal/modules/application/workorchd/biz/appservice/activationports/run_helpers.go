package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func RunObservableAfterStartConflict(ctx context.Context, hub magicevent.Hub, runID common.RunID) (bool, error) {
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

func NextWorkspaceExecutionRunID(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) (common.RunID, error) {
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

func RecordActivationMemoryRecallTrace(ctx context.Context, commandHub magicevent.Hub, runID common.RunID, workspaceID common.WorkspaceID, bootstrap *runtimecontract.RuntimeBootstrap, occurredAt string) error {
	if commandHub == nil || runID == "" || workspaceID == "" {
		return nil
	}
	trace := activationcore.BuildRuntimeV2ActivationMemoryRecallTrace(runID, workspaceID, bootstrap, occurredAt)
	if trace == nil {
		return nil
	}
	return sendActivationMagicCommand[memoryevents.UpsertMemoryRecallTraceCommand, memoryevents.MemoryCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, trace.WorkspaceID), commandHub, memoryevents.TopicMemoryRecallTraceCommandUpsert, memoryevents.CommandDestination, "memory_recall", string(trace.ID), trace.WorkspaceID, trace.RunID, memoryevents.UpsertMemoryRecallTraceCommandFromModel(trace))
}
