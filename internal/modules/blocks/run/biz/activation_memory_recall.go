package run

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeActivationMemoryRecall struct {
	RunID            common.RunID
	WorkspaceID      common.WorkspaceID
	RuntimeBootstrap *runtimecontract.RuntimeBootstrap
}

// publishRuntimeActivationMemoryRecall records the memory selection sealed
// in a newly-created Runtime bootstrap. Run owns the activation handoff;
// blocks/memory owns RecallTrace persistence and the recorded notification.
// The operation is intentionally idempotent and best-effort because the Run
// record is already durable when this cross-owner observability fact is sent.
func (m *Biz) publishRuntimeActivationMemoryRecall(ctx context.Context, command runtimeActivationMemoryRecall, occurredAt string) {
	if m == nil || m.EventHub() == nil || command.RuntimeBootstrap == nil {
		return
	}
	trace := runtimeActivationMemoryRecallTrace(command.RunID, command.WorkspaceID, command.RuntimeBootstrap, occurredAt)
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
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(memoryevents.Contracts(), event.ID(), eventcontract.LaneScope{
		SubjectType: "memory_recall", SubjectID: string(trace.ID), WorkspaceID: string(trace.WorkspaceID), RunID: string(trace.RunID),
	}))
	response := m.EventHub().Send(event)
	if response == nil {
		return
	}
	if response.Error() != nil {
		return
	}
	data, err := response.Get()
	if err != nil {
		return
	}
	ack, ok := data.(memoryevents.UpsertMemoryRecallTraceResult)
	if !ok {
		return
	}
	_ = ack.Validate(trace.WorkspaceID, trace.RunID, trace.ID)
}

func runtimeActivationMemoryRecallTrace(runID common.RunID, workspaceID common.WorkspaceID, bootstrap *runtimecontract.RuntimeBootstrap, occurredAt string) *memorymodel.MemoryRecallTrace {
	if runID == "" || workspaceID == "" || bootstrap == nil {
		return nil
	}
	activation := bootstrap.Activation.Normalized()
	if activation.MemorySelection == nil {
		return nil
	}
	selection := activation.MemorySelection.Normalized()
	traceID := common.MemoryRecallTraceID(fmt.Sprintf("%s/memory-recall/activation", runID))
	return &memorymodel.MemoryRecallTrace{
		ID: traceID, WorkspaceID: workspaceID, RunID: runID, Stage: "planning", Source: "activation",
		Policy: selection.Policy, Status: selection.Status, ProviderID: selection.ProviderID, ModelID: selection.ModelID, Operation: selection.Operation,
		QueryTextPreview: selection.QueryTextPreview, QueryTextSha256: selection.QueryTextSHA256, MinScore: selection.MinScore,
		Limit: selection.Limit, EmbeddingModel: selection.EmbeddingModel, VectorDimension: selection.VectorDimension,
		RefreshSource: selection.RefreshSource, SelectionPolicyVersion: selection.SelectionPolicyVersion,
		CandidateMemoryIDs: append([]common.MemoryID(nil), selection.CandidateMemoryIDs...),
		SelectedMemoryIDs:  append([]common.MemoryID(nil), selection.SelectedMemoryIDs...),
		ExcludedMemoryIDs:  append([]common.MemoryID(nil), selection.ExcludedMemoryIDs...),
		TypeQuota:          memoryTypeQuotaMap(selection.TypeQuota), SelectionScores: append([]workorchmemorycontract.RecallSelectionScore(nil), selection.SelectionScores...),
		RetrievedMemoryIDs: append([]common.MemoryID(nil), selection.RetrievedMemoryIDs...), RetrievedMemoryHits: append([]workorchmemorycontract.RecallHit(nil), selection.RetrievedMemoryHits...),
		ContextBudget: selection.ContextBudget, SemanticRecall: cloneRuntimeMemorySelectionEvidence(selection.SemanticRecall), FallbackFrom: selection.FallbackFrom,
		Reason:     selection.Reason,
		OccurredAt: strings.TrimSpace(occurredAt), EvidenceRef: string(traceID),
	}
}

func memoryTypeQuotaMap(values []workorchmemorycontract.TypeQuotaEntry) map[string]int {
	if values == nil {
		return nil
	}
	ret := make(map[string]int, len(values))
	for _, value := range values {
		ret[value.Type] = value.Quota
	}
	return ret
}

func cloneRuntimeMemorySelectionEvidence(input *workorchmemorycontract.RecallSelectionEvidence) *workorchmemorycontract.RecallSelectionEvidence {
	if input == nil {
		return nil
	}
	value := input.Normalized()
	return &value
}
