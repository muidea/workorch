package appservice

import (
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func mapLLMContextBudgetTraces(steps []presenter.StepView) []LLMContextBudgetTrace {
	if len(steps) == 0 {
		return nil
	}
	ret := make([]LLMContextBudgetTrace, 0)
	for _, step := range steps {
		if step.Output == nil || step.Kind != llmcommon.StepKindWorkspaceExecution {
			continue
		}
		budget := readModelMapValue(step.Output["contextBudget"])
		if len(budget) == 0 {
			continue
		}
		usage := readModelMapValue(step.Output["llmUsage"])
		ret = append(ret, LLMContextBudgetTrace{
			StepID:                step.ID,
			ProviderID:            firstNonEmptyReadModel(readModelStringValue(budget["providerId"]), readModelStringValue(usage["providerId"])),
			Model:                 firstNonEmptyReadModel(readModelStringValue(budget["model"]), readModelStringValue(usage["model"])),
			MaxBytes:              readModelIntValue(budget["maxBytes"]),
			ActualBytes:           readModelIntValue(budget["actualBytes"]),
			MaxTokens:             readModelIntValue(budget["maxTokens"]),
			EstimatedInputTokens:  readModelIntValue(budget["estimatedInputTokens"]),
			EstimatedOutputTokens: readModelIntValue(budget["estimatedOutputTokens"]),
			ActualTokens:          readModelIntValue(budget["actualTokens"]),
			ShrinkLevel:           readModelIntValue(budget["shrinkLevel"]),
			Truncated:             readModelBoolValue(budget["truncated"]),
			Rules:                 readModelStringSliceValue(budget["rules"]),
			SourceRefs:            readModelStringSliceValue(budget["sourceRefs"]),
			IncludedSections:      readModelStringSliceValue(budget["includedSections"]),
			NormalizedSections:    readModelStringSliceValue(budget["normalizedSections"]),
			CompressedSections:    readModelStringSliceValue(budget["compressedSections"]),
			OmittedSections:       readModelStringSliceValue(budget["omittedSections"]),
			GoalDoDCompressed:     readModelBoolValue(budget["goalDoDCompressed"]),
			OverBudget:            readModelBoolValue(budget["overBudget"]),
			P0Overflow:            readModelBoolValue(budget["p0Overflow"]),
			ProfileMode:           readModelStringValue(budget["profileMode"]),
			CapabilitySource:      readModelStringValue(budget["capabilitySource"]),
			ContextUtilization:    readModelFloatValue(budget["contextUtilizationRatio"]),
			MaxRequestBytes:       readModelIntValue(budget["maxRequestBytes"]),
			StageBudgetRatio:      readModelFloatValue(budget["stageBudgetRatio"]),
			StageBudgetSource:     readModelStringValue(budget["stageBudgetSource"]),
			TracePolicy:           readModelStringValue(budget["tracePolicy"]),
			PromptCaptured:        readModelBoolValue(budget["promptCaptured"]),
			Estimator:             readModelStringValue(budget["estimator"]),
			Profile:               readModelMapValue(budget["profile"]),
			CachePlan:             readModelMapValue(budget["cachePlan"]),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapLLMUsageTracesFromEventRecords(records []EventRecord, runID string) []LLMUsageTrace {
	if len(records) == 0 {
		return nil
	}
	ret := make([]LLMUsageTrace, 0)
	for _, item := range records {
		if item.Topic != events.TopicRunLLMUsageRecorded {
			continue
		}
		if runID != "" && item.RunID != runID {
			continue
		}
		payload := cloneReadModelMap(item.Payload)
		if len(payload) == 0 {
			continue
		}
		estimated := readModelBoolValue(payload[events.PayloadKeyEstimated])
		usageSource := readModelStringValue(payload[events.PayloadKeyUsageSource])
		if strings.TrimSpace(usageSource) == "" {
			if estimated {
				usageSource = llmcommon.UsageSourceEstimated
			} else {
				usageSource = llmcommon.UsageSourceProviderNative
			}
		}
		trace := LLMUsageTrace{
			ID:            item.ID,
			WorkspaceID:   firstNonEmptyReadModel(item.WorkspaceID, readModelStringValue(payload[events.PayloadKeyWorkspaceID])),
			RunID:         firstNonEmptyReadModel(item.RunID, readModelStringValue(payload[events.PayloadKeyRunID])),
			StepID:        firstNonEmptyReadModel(item.StepID, readModelStringValue(payload[events.PayloadKeyStepID])),
			ProviderID:    readModelStringValue(payload[events.PayloadKeyProviderID]),
			Provider:      readModelStringValue(payload[events.PayloadKeyProvider]),
			Model:         readModelStringValue(payload[events.PayloadKeyModel]),
			Operation:     readModelStringValue(payload[events.PayloadKeyOperation]),
			InputTokens:   readModelIntValue(payload[events.PayloadKeyInputTokens]),
			OutputTokens:  readModelIntValue(payload[events.PayloadKeyOutputTokens]),
			TotalTokens:   readModelIntValue(payload[events.PayloadKeyTotalTokens]),
			Estimated:     estimated,
			UsageSource:   usageSource,
			RequestBytes:  readModelIntValue(payload[events.PayloadKeyRequestBytes]),
			ResponseBytes: readModelIntValue(payload[events.PayloadKeyResponseBytes]),
			Streaming:     readModelBoolValue(payload[events.PayloadKeyStreaming]),
			ContextBudget: readModelMapValue(payload[events.PayloadKeyContextBudget]),
			CacheUsage:    readModelMapValue(payload[events.PayloadKeyCacheUsage]),
			StartedAt:     readModelStringValue(payload[events.PayloadKeyStartedAt]),
			CompletedAt:   readModelStringValue(payload[events.PayloadKeyCompletedAt]),
			DurationMs:    readModelIntValue(payload[events.PayloadKeyDurationMs]),
			Status:        readModelStringValue(payload[events.PayloadKeyStatus]),
			FailureClass:  readModelStringValue(payload[events.PayloadKeyFailureClass]),
			OccurredAt:    item.OccurredAt,
			EvidenceRef:   firstNonEmptyReadModel(item.StepID, item.SubjectID),
		}
		ret = append(ret, trace)
	}
	if len(ret) == 0 {
		return nil
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].OccurredAt == ret[j].OccurredAt {
			return ret[i].ID < ret[j].ID
		}
		return ret[i].OccurredAt < ret[j].OccurredAt
	})
	return ret
}

func summarizeLLMUsage(items []LLMUsageTrace) LLMUsageSummary {
	ret := LLMUsageSummary{}
	breakdownByKey := map[string]*LLMUsageProviderModelBreakdown{}
	for _, item := range items {
		ret.EntryTotal++
		ret.InputTokens += item.InputTokens
		ret.OutputTokens += item.OutputTokens
		ret.TotalTokens += item.TotalTokens
		if item.Estimated {
			ret.EstimatedEntries++
		}
		if strings.TrimSpace(item.UsageSource) == llmcommon.UsageSourceProviderNative {
			ret.ProviderNativeEntries++
		}
		if item.Status == "failed" {
			ret.FailedEntries++
		}
		cacheReadTokens := readModelIntValue(item.CacheUsage["cacheReadTokens"])
		cacheWriteTokens := readModelIntValue(item.CacheUsage["cacheWriteTokens"])
		cacheCreationTokens := readModelIntValue(item.CacheUsage["cacheCreationTokens"])
		cacheableInputTokens := readModelIntValue(item.CacheUsage["cacheableInputTokens"])
		nonCacheableInputTokens := readModelIntValue(item.CacheUsage["nonCacheableInputTokens"])
		ret.CacheReadTokens += cacheReadTokens
		ret.CacheWriteTokens += cacheWriteTokens
		ret.CacheCreationTokens += cacheCreationTokens
		ret.CacheableInputTokens += cacheableInputTokens
		ret.NonCacheableInputTokens += nonCacheableInputTokens
		if len(item.CacheUsage) > 0 {
			if readModelBoolValue(item.CacheUsage["estimated"]) {
				ret.CacheEstimatedEntries++
			}
			if strings.TrimSpace(readModelStringValue(item.CacheUsage["source"])) == llmcommon.UsageSourceProviderNative {
				ret.CacheProviderEntries++
			}
		}
		key := strings.TrimSpace(item.ProviderID) + "|" + strings.TrimSpace(item.Provider) + "|" + strings.TrimSpace(item.Model)
		entry, ok := breakdownByKey[key]
		if !ok {
			entry = &LLMUsageProviderModelBreakdown{
				ProviderID: strings.TrimSpace(item.ProviderID),
				Provider:   strings.TrimSpace(item.Provider),
				Model:      strings.TrimSpace(item.Model),
			}
			breakdownByKey[key] = entry
		}
		entry.EntryTotal++
		entry.InputTokens += item.InputTokens
		entry.OutputTokens += item.OutputTokens
		entry.TotalTokens += item.TotalTokens
		entry.CacheReadTokens += cacheReadTokens
		entry.CacheWriteTokens += cacheWriteTokens
		if item.Estimated {
			entry.EstimatedEntries++
		}
		if item.Status == "failed" {
			entry.FailedEntries++
		}
	}
	if len(breakdownByKey) > 0 {
		ret.ProviderModelBreakdown = make([]LLMUsageProviderModelBreakdown, 0, len(breakdownByKey))
		for _, item := range breakdownByKey {
			ret.ProviderModelBreakdown = append(ret.ProviderModelBreakdown, *item)
		}
		sort.SliceStable(ret.ProviderModelBreakdown, func(i, j int) bool {
			left := ret.ProviderModelBreakdown[i]
			right := ret.ProviderModelBreakdown[j]
			if left.ProviderID != right.ProviderID {
				return left.ProviderID < right.ProviderID
			}
			if left.Provider != right.Provider {
				return left.Provider < right.Provider
			}
			if left.Model != right.Model {
				return left.Model < right.Model
			}
			return false
		})
	}
	ret.CacheReadInputRatio = readModelRatio(ret.CacheReadTokens, ret.InputTokens)
	ret.CacheableInputRatio = readModelRatio(ret.CacheableInputTokens, ret.InputTokens)
	ret.NonCacheableInputRatio = readModelRatio(ret.NonCacheableInputTokens, ret.InputTokens)
	for idx := range ret.ProviderModelBreakdown {
		ret.ProviderModelBreakdown[idx].CacheReadInputRatio = readModelRatio(ret.ProviderModelBreakdown[idx].CacheReadTokens, ret.ProviderModelBreakdown[idx].InputTokens)
	}
	return ret
}

func readModelRatio(part, total int) float64 {
	if total <= 0 || part <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}
