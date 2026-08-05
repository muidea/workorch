package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// MemoryRecallTraceInfosFromProjection converts immutable memory recall
// projections into strongly typed process-port values.
func MemoryRecallTraceInfosFromProjection(items []projectioncontract.MemoryRecallTrace) []MemoryRecallTraceInfo {
	ret := make([]MemoryRecallTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryRecallTraceInfo{
			ID:                     common.MemoryRecallTraceID(item.ID),
			WorkspaceID:            common.WorkspaceID(item.WorkspaceID),
			RunID:                  common.RunID(item.RunID),
			StepID:                 common.StepID(item.StepID),
			GraphNodeID:            item.GraphNodeID,
			Stage:                  item.Stage,
			Source:                 item.Source,
			Policy:                 item.Policy,
			Status:                 item.Status,
			ProviderID:             item.ProviderID,
			Operation:              item.Operation,
			QueryTextPreview:       item.QueryTextPreview,
			QueryTextSha256:        item.QueryTextSha256,
			MinScore:               item.MinScore,
			Limit:                  item.Limit,
			EmbeddingModel:         item.EmbeddingModel,
			VectorDimension:        item.VectorDimension,
			RefreshSource:          item.RefreshSource,
			SelectionPolicyVersion: item.SelectionPolicyVersion,
			CandidateMemoryIDs:     memoryIDsFromProjection(item.CandidateMemoryIDs),
			SelectedMemoryIDs:      memoryIDsFromProjection(item.SelectedMemoryIDs),
			ExcludedMemoryIDs:      memoryIDsFromProjection(item.ExcludedMemoryIDs),
			TypeQuota:              projectioncontract.CloneStringIntMap(item.TypeQuota),
			SelectionScores:        append([]workorchmemorycontract.RecallSelectionScore(nil), item.SelectionScores...),
			RetrievedMemoryIDs:     memoryIDsFromProjection(item.RetrievedMemoryIDs),
			RetrievedMemoryHits:    append([]workorchmemorycontract.RecallHit(nil), item.RetrievedMemoryHits...),
			Reason:                 item.Reason,
			OccurredAt:             item.OccurredAt,
			EvidenceRef:            item.EvidenceRef,
		})
	}
	return ret
}

// WebSourceTraceInfosFromProjection detaches slice storage while preserving
// every owner-neutral web source field.
func WebSourceTraceInfosFromProjection(items []projectioncontract.WebSourceTrace) []WebSourceTraceInfo {
	ret := make([]WebSourceTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, WebSourceTraceInfo{
			Query:          item.Query,
			ProviderID:     item.ProviderID,
			SearchedAt:     item.SearchedAt,
			Sources:        append([]projectioncontract.WebSource(nil), item.Sources...),
			FetchedSources: append([]projectioncontract.FetchedWebSource(nil), item.FetchedSources...),
			EvidenceRefs:   append([]string(nil), item.EvidenceRefs...),
			ApprovalID:     common.ApprovalID(item.ApprovalID),
			PolicyDecision: item.PolicyDecision,
		})
	}
	return ret
}

func LLMContextBudgetTraceInfosFromProjection(items []projectioncontract.LLMContextBudgetTrace) []LLMContextBudgetTraceInfo {
	ret := make([]LLMContextBudgetTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, LLMContextBudgetTraceInfo{
			StepID:                common.StepID(item.StepID),
			ProviderID:            item.ProviderID,
			Model:                 item.Model,
			CapabilityRevision:    item.CapabilityRevision,
			ConfigurationRevision: item.ConfigurationRevision,
			MaxBytes:              item.MaxBytes,
			ActualBytes:           item.ActualBytes,
			MaxTokens:             item.MaxTokens,
			EstimatedInputTokens:  item.EstimatedInputTokens,
			EstimatedOutputTokens: item.EstimatedOutputTokens,
			ActualTokens:          item.ActualTokens,
			ShrinkLevel:           item.ShrinkLevel,
			Truncated:             item.Truncated,
			Rules:                 append([]string(nil), item.Rules...),
			SourceRefs:            append([]string(nil), item.SourceRefs...),
			IncludedSections:      append([]string(nil), item.IncludedSections...),
			NormalizedSections:    append([]string(nil), item.NormalizedSections...),
			CompressedSections:    append([]string(nil), item.CompressedSections...),
			OmittedSections:       append([]string(nil), item.OmittedSections...),
			GoalDoDCompressed:     item.GoalDoDCompressed,
			OverBudget:            item.OverBudget,
			P0Overflow:            item.P0Overflow,
			ProfileMode:           item.ProfileMode,
			CapabilitySource:      item.CapabilitySource,
			ContextUtilization:    item.ContextUtilization,
			MaxRequestBytes:       item.MaxRequestBytes,
			StageBudgetRatio:      item.StageBudgetRatio,
			StageBudgetSource:     item.StageBudgetSource,
			TracePolicy:           item.TracePolicy,
			PromptCaptured:        item.PromptCaptured,
			Estimator:             item.Estimator,
			ProviderSubmitted:     item.ProviderSubmitted,
			ContextGateAction:     item.ContextGateAction,
			ContextGateReason:     item.ContextGateReason,
			BudgetFailureClass:    item.BudgetFailureClass,
			RedactedPaths:         append([]string(nil), item.RedactedPaths...),
			ProtectedFields:       append([]string(nil), item.ProtectedFields...),
			IntegrityWarnings:     append([]string(nil), item.IntegrityWarnings...),
			SectionHashes:         cloneLLMContextBudgetSections(item.SectionHashes),
			Profile:               item.Profile.Clone(),
			CachePlan:             item.CachePlan.Clone(),
		})
	}
	return ret
}

func LLMUsageTraceInfosFromProjection(items []projectioncontract.LLMUsageTrace) []LLMUsageTraceInfo {
	ret := make([]LLMUsageTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, LLMUsageTraceInfo{
			ID:            item.ID,
			WorkspaceID:   common.WorkspaceID(item.WorkspaceID),
			RunID:         common.RunID(item.RunID),
			StepID:        common.StepID(item.StepID),
			ProviderID:    item.ProviderID,
			Provider:      item.Provider,
			APISurface:    item.APISurface,
			Model:         item.Model,
			Operation:     item.Operation,
			InputTokens:   item.InputTokens,
			OutputTokens:  item.OutputTokens,
			TotalTokens:   item.TotalTokens,
			Estimated:     item.Estimated,
			UsageSource:   item.UsageSource,
			RequestBytes:  item.RequestBytes,
			ResponseBytes: item.ResponseBytes,
			Streaming:     item.Streaming,
			ContextBudget: item.ContextBudget.Clone(),
			CacheUsage:    item.CacheUsage.Clone(),
			CacheMarkers:  item.CacheMarkers.Clone(),
			StartedAt:     item.StartedAt,
			CompletedAt:   item.CompletedAt,
			DurationMs:    item.DurationMs,
			Status:        item.Status,
			FailureClass:  item.FailureClass,
			OccurredAt:    item.OccurredAt,
			EvidenceRef:   item.EvidenceRef,
		})
	}
	return ret
}

func LLMUsageSummaryInfoFromProjection(item projectioncontract.LLMUsageSummary) LLMUsageSummaryInfo {
	return LLMUsageSummaryInfo{
		EntryTotal:              item.EntryTotal,
		InputTokens:             item.InputTokens,
		OutputTokens:            item.OutputTokens,
		TotalTokens:             item.TotalTokens,
		EstimatedEntries:        item.EstimatedEntries,
		ProviderNativeEntries:   item.ProviderNativeEntries,
		FailedEntries:           item.FailedEntries,
		CacheReadTokens:         item.CacheReadTokens,
		CacheWriteTokens:        item.CacheWriteTokens,
		CacheCreationTokens:     item.CacheCreationTokens,
		CacheableInputTokens:    item.CacheableInputTokens,
		NonCacheableInputTokens: item.NonCacheableInputTokens,
		CacheReadInputRatio:     item.CacheReadInputRatio,
		CacheableInputRatio:     item.CacheableInputRatio,
		NonCacheableInputRatio:  item.NonCacheableInputRatio,
		CacheEstimatedEntries:   item.CacheEstimatedEntries,
		CacheProviderEntries:    item.CacheProviderEntries,
		ProviderModelBreakdown:  llmUsageBreakdownInfosFromProjection(item.ProviderModelBreakdown),
	}
}

func EventRecordInfosFromProjection(items []projectioncontract.EventRecord) []EventRecordInfo {
	ret := make([]EventRecordInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, EventRecordInfo{
			ID:             common.EventRecordID(item.ID),
			Version:        item.Version,
			Topic:          item.Topic,
			OccurredAt:     item.OccurredAt,
			SourceModule:   item.SourceModule,
			SubjectType:    item.SubjectType,
			SubjectID:      item.SubjectID,
			WorkspaceID:    common.WorkspaceID(item.WorkspaceID),
			RunID:          common.RunID(item.RunID),
			StepID:         common.StepID(item.StepID),
			Revision:       item.Revision,
			Payload:        item.Payload.Clone(),
			PayloadSummary: item.PayloadSummary.Clone(),
			Summary:        CompactEventRecordPayloadSummary(item.Topic, item.Payload),
			PayloadRef:     item.PayloadRef,
		})
	}
	return ret
}

func memoryIDsFromProjection(values []string) []common.MemoryID {
	ret := make([]common.MemoryID, 0, len(values))
	for _, value := range values {
		ret = append(ret, common.MemoryID(value))
	}
	return ret
}

func llmUsageBreakdownInfosFromProjection(items []projectioncontract.LLMUsageProviderModelBreakdown) []LLMUsageProviderModelBreakdownInfo {
	ret := make([]LLMUsageProviderModelBreakdownInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, LLMUsageProviderModelBreakdownInfo{
			ProviderID:              item.ProviderID,
			Provider:                item.Provider,
			APISurface:              item.APISurface,
			Model:                   item.Model,
			ProviderMode:            item.ProviderMode,
			EntryTotal:              item.EntryTotal,
			InputTokens:             item.InputTokens,
			OutputTokens:            item.OutputTokens,
			TotalTokens:             item.TotalTokens,
			CacheReadTokens:         item.CacheReadTokens,
			CacheWriteTokens:        item.CacheWriteTokens,
			CacheCreationTokens:     item.CacheCreationTokens,
			CacheablePromptCount:    item.CacheablePromptCount,
			ActualCacheHitCount:     item.ActualCacheHitCount,
			CacheCreationCount:      item.CacheCreationCount,
			PromptCacheKeySentCount: item.PromptCacheKeySentCount,
			CacheControlSentCount:   item.CacheControlSentCount,
			AutoPrefixExpectedCount: item.AutoPrefixExpectedCount,
			CacheReadInputRatio:     item.CacheReadInputRatio,
			EstimatedEntries:        item.EstimatedEntries,
			FailedEntries:           item.FailedEntries,
		})
	}
	return ret
}

func cloneLLMContextBudgetSections(items []projectioncontract.LLMContextBudgetSection) []projectioncontract.LLMContextBudgetSection {
	ret := append([]projectioncontract.LLMContextBudgetSection(nil), items...)
	for idx := range ret {
		ret[idx].SourceRefs = append([]string(nil), items[idx].SourceRefs...)
	}
	return ret
}
