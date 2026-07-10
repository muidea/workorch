package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
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
	if err := sendActivationMagicCommand[memoryevents.UpsertMemoryRecallTraceCommand, memoryevents.MemoryCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, trace.WorkspaceID), hub, memoryevents.TopicMemoryRecallTraceCommandUpsert, memoryevents.CommandDestination, "memory_recall", string(trace.ID), trace.WorkspaceID, trace.RunID, memoryevents.UpsertMemoryRecallTraceCommandFromModel(trace)); err != nil {
		return err
	}
	return events.PublishByContract(ctx, hub, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        memoryevents.TopicMemoryRecallRecorded,
		OccurredAt:   trace.OccurredAt,
		SourceModule: modulecommon.EventSourceModuleID,
		SubjectType:  "memory_recall",
		SubjectID:    string(trace.ID),
		WorkspaceID:  string(trace.WorkspaceID),
		RunID:        string(trace.RunID),
		Payload: events.MemoryRecallPayload(
			trace.ID,
			trace.Stage,
			trace.Source,
			trace.Policy,
			trace.Status,
			trace.ProviderID,
			trace.Operation,
			trace.RetrievedMemoryIDs,
			trace.Reason,
		),
	})
}
