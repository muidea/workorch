package run

import (
	"context"
	"fmt"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// publishRuntimeActivationMemoryRecall records the memory selection sealed
// in a newly-created Runtime bootstrap. Run owns the activation handoff;
// blocks/memory owns RecallTrace persistence and the recorded notification.
// The operation is intentionally idempotent and best-effort because the Run
// record is already durable when this cross-owner observability fact is sent.
func (m *Biz) publishRuntimeActivationMemoryRecall(ctx context.Context, command runevents.CreateRunCommand) {
	if m == nil || m.EventHub() == nil || command.RuntimeBootstrap == nil {
		return
	}
	trace := runtimeActivationMemoryRecallTrace(command.RunID, command.WorkspaceID, command.RuntimeBootstrap, time.Now().UTC().Format(time.RFC3339Nano))
	if trace == nil {
		return
	}
	commandCtx, err := common.BindRuntimeWorkspaceContext(ctx, trace.WorkspaceID)
	if err != nil {
		return
	}
	event := magicevent.NewEventWithContext(
		memoryevents.TopicMemoryRecallTraceCommandUpsert,
		ModuleID,
		memoryevents.CommandDestination,
		magicevent.NewHeader(),
		commandCtx,
		memoryevents.UpsertMemoryRecallTraceCommandFromModel(trace),
	)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(memoryevents.Contracts(), event.ID(), workorchevents.EventHubLaneScope{
		SubjectType: "memory_recall", SubjectID: string(trace.ID), WorkspaceID: string(trace.WorkspaceID), RunID: string(trace.RunID),
	}))
	response := m.EventHub().Send(event)
	if response == nil {
		return
	}
	// Do not let an observability failure alter the already-persisted Run
	// command result. The memory owner still receives the concrete typed event,
	// and subsequent owner-local replay can safely upsert the same trace.
	_ = response.Error()
}

func runtimeActivationMemoryRecallTrace(runID common.RunID, workspaceID common.WorkspaceID, bootstrap *runtimecontract.RuntimeBootstrap, occurredAt string) *memorymodel.MemoryRecallTrace {
	if runID == "" || workspaceID == "" || bootstrap == nil {
		return nil
	}
	activation := bootstrap.Activation.Normalized()
	if len(activation.Memories) == 0 {
		return nil
	}
	traceID := common.MemoryRecallTraceID(fmt.Sprintf("%s/memory-recall/activation", runID))
	ids := make([]common.MemoryID, 0, len(activation.Memories))
	hits := make([]memorymodel.MemoryRecallHit, 0, len(activation.Memories))
	for _, item := range activation.Memories {
		if item.ID == "" {
			continue
		}
		ids = append(ids, item.ID)
		hits = append(hits, memorymodel.MemoryRecallHit{ID: item.ID, Summary: item.Summary, VectorScore: item.VectorScore})
	}
	if len(ids) == 0 {
		return nil
	}
	return &memorymodel.MemoryRecallTrace{
		ID: traceID, WorkspaceID: workspaceID, RunID: runID, Stage: "planning", Source: "activation",
		Policy: "runtime_typed_activation", Status: "selected", Operation: "typed_activation_snapshot", RefreshSource: "activation",
		SelectedMemoryIDs: append([]common.MemoryID(nil), ids...), RetrievedMemoryIDs: append([]common.MemoryID(nil), ids...), RetrievedMemoryHits: hits,
		OccurredAt: strings.TrimSpace(occurredAt), EvidenceRef: string(traceID),
	}
}
