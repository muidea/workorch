package memory

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
)

func (m *Biz) publishMemoryChanged(ctx context.Context, topic string, item *memorymodel.MemoryItem) {
	if m == nil || m.EventHub() == nil || item == nil {
		return
	}
	event := magicevent.NewEventWithContext(topic, ModuleID, memoryevents.NotificationDestination, magicevent.NewHeader(), ctx, memoryevents.MemoryChangedDataFromModel(item))
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(memoryevents.Contracts(), topic, eventcontract.LaneScope{
		SubjectType: "memory", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID),
	}))
	m.PostEvent(event)
}

func (m *Biz) publishMemoryRecallRecorded(ctx context.Context, trace *memorymodel.MemoryRecallTrace) {
	if m == nil || m.EventHub() == nil || trace == nil {
		return
	}
	event := magicevent.NewEventWithContext(
		memoryevents.TopicMemoryRecallRecorded,
		ModuleID,
		memoryevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		memoryevents.MemoryRecallRecordedData{
			TraceID:            trace.ID,
			WorkspaceID:        trace.WorkspaceID,
			RunID:              trace.RunID,
			StepID:             trace.StepID,
			Stage:              trace.Stage,
			Source:             trace.Source,
			Policy:             trace.Policy,
			Status:             trace.Status,
			ProviderID:         trace.ProviderID,
			Operation:          trace.Operation,
			RetrievedMemoryIDs: append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...),
			SelectionEvidence:  memoryRecallSelectionEvidence(trace),
			Reason:             trace.Reason,
			OccurredAt:         trace.OccurredAt,
		},
	)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(memoryevents.Contracts(), event.ID(), eventcontract.LaneScope{
		SubjectType: "memory_recall",
		SubjectID:   string(trace.ID),
		WorkspaceID: string(trace.WorkspaceID),
		RunID:       string(trace.RunID),
		StepID:      string(trace.StepID),
	}))
	m.PostEvent(event)
}

func memoryRecallSelectionEvidence(trace *memorymodel.MemoryRecallTrace) *workorchmemorycontract.RecallSelectionEvidence {
	if trace == nil || trace.Source != "activation" {
		return nil
	}
	return workorchmemorycontract.CloneRecallSelectionEvidence(&workorchmemorycontract.RecallSelectionEvidence{
		Policy: trace.Policy, Status: trace.Status, Reason: trace.Reason, ProviderID: trace.ProviderID, ModelID: trace.ModelID,
		Operation: trace.Operation, QueryTextPreview: trace.QueryTextPreview, QueryTextSHA256: trace.QueryTextSha256,
		MinScore: trace.MinScore, Limit: trace.Limit, EmbeddingModel: trace.EmbeddingModel, VectorDimension: trace.VectorDimension,
		RefreshSource: trace.RefreshSource, SelectionPolicyVersion: trace.SelectionPolicyVersion,
		CandidateMemoryIDs: append([]common.MemoryID(nil), trace.CandidateMemoryIDs...),
		SelectedMemoryIDs:  append([]common.MemoryID(nil), trace.SelectedMemoryIDs...), ExcludedMemoryIDs: append([]common.MemoryID(nil), trace.ExcludedMemoryIDs...),
		TypeQuota: memoryRecallTypeQuotaEntries(trace.TypeQuota), SelectionScores: append([]workorchmemorycontract.RecallSelectionScore(nil), trace.SelectionScores...),
		RetrievedMemoryIDs: append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...), RetrievedMemoryHits: append([]workorchmemorycontract.RecallHit(nil), trace.RetrievedMemoryHits...),
		ContextBudget: trace.ContextBudget, SemanticRecall: workorchmemorycontract.CloneRecallSelectionEvidence(trace.SemanticRecall), FallbackFrom: trace.FallbackFrom,
	})
}

func memoryRecallTypeQuotaEntries(values map[string]int) []workorchmemorycontract.TypeQuotaEntry {
	if values == nil {
		return nil
	}
	ret := make([]workorchmemorycontract.TypeQuotaEntry, 0, len(values))
	for memoryType, quota := range values {
		ret = append(ret, workorchmemorycontract.TypeQuotaEntry{Type: memoryType, Quota: quota})
	}
	return ret
}
