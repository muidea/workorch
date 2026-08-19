package activationcore

import (
	"math"
	"strings"

	memorycommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
)

const (
	RuntimeMemoryContextMaxItems           = 8
	RuntimeMemoryContextMaxBytes           = 16 * 1024
	RuntimeMemoryContextMaxEstimatedTokens = 4096
	RuntimeMemorySelectionCandidateLimit   = workorchmemorycontract.RecallSelectionMaxCandidateCount
	RuntimeMemorySelectionPolicyVersion    = "activation-memory-selection/v1"
)

var runtimeMemoryTypeQuotas = map[string]int{
	memorycommon.MemoryTypeDecision:        1,
	memorycommon.MemoryTypeExecutionLesson: 2,
	memorycommon.MemoryTypeProjectContext:  2,
	memorycommon.MemoryTypeDoDFeedback:     2,
	memorycommon.MemoryTypeUserPreference:  1,
	memorycommon.MemoryTypeSkillFeedback:   1,
	memorycommon.MemoryTypeToolFeedback:    1,
	memorycommon.MemoryTypeRunSummary:      2,
}

type RuntimeMemorySelection struct {
	Candidates      []*memorymodel.MemoryItem
	Selected        []*memorymodel.MemoryItem
	Excluded        []*memorymodel.MemoryItem
	TypeQuota       []workorchmemorycontract.TypeQuotaEntry
	SelectionScores []workorchmemorycontract.RecallSelectionScore
	RetrievedHits   []workorchmemorycontract.RecallHit
	ContextBudget   workorchmemorycontract.RecallContextBudget
}

func SelectRuntimeMemoryContext(candidates []*memorymodel.MemoryItem) (RuntimeMemorySelection, error) {
	unique := make([]*memorymodel.MemoryItem, 0, len(candidates))
	seen := make(map[common.MemoryID]struct{}, len(candidates))
	for _, item := range candidates {
		if item == nil || item.ID == "" {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		unique = append(unique, memorymodel.CloneMemoryItem(item))
	}
	if len(unique) > RuntimeMemorySelectionCandidateLimit {
		return RuntimeMemorySelection{}, common.ErrInvalidArgument
	}

	ret := RuntimeMemorySelection{
		Candidates: unique,
		TypeQuota:  runtimeMemoryTypeQuotaEntries(),
		ContextBudget: workorchmemorycontract.RecallContextBudget{
			MaxItems: RuntimeMemoryContextMaxItems, MaxBytes: RuntimeMemoryContextMaxBytes,
			MaxEstimatedTokens: RuntimeMemoryContextMaxEstimatedTokens,
		},
	}
	usedByType := make(map[string]int, len(runtimeMemoryTypeQuotas))
	for _, item := range unique {
		reason := "selected_within_context_budget"
		selected := true
		memoryType := strings.TrimSpace(item.MemoryType)
		quota := runtimeMemoryTypeQuotas[memoryType]
		if quota <= 0 {
			quota = 1
		}
		itemBytes := runtimeMemoryContextBytes(item)
		itemTokens := estimatedMemoryTokens(itemBytes)
		switch {
		case !MemoryVisibleToLLM(item):
			selected, reason = false, "excluded_visibility_policy"
		case len(ret.Selected) >= RuntimeMemoryContextMaxItems:
			selected, reason = false, "excluded_item_limit"
		case usedByType[memoryType] >= quota:
			selected, reason = false, "excluded_type_quota"
		case ret.ContextBudget.UsedBytes+itemBytes > ret.ContextBudget.MaxBytes:
			selected, reason = false, "excluded_byte_budget"
		case ret.ContextBudget.EstimatedTokens+itemTokens > ret.ContextBudget.MaxEstimatedTokens:
			selected, reason = false, "excluded_token_budget"
		}
		ret.SelectionScores = append(ret.SelectionScores, workorchmemorycontract.RecallSelectionScore{
			ID: item.ID, Score: runtimeMemorySelectionScore(item), Reason: reason,
		})
		if !selected {
			ret.Excluded = append(ret.Excluded, memorymodel.CloneMemoryItem(item))
			continue
		}
		usedByType[memoryType]++
		ret.Selected = append(ret.Selected, memorymodel.CloneMemoryItem(item))
		ret.ContextBudget.UsedItems++
		ret.ContextBudget.UsedBytes += itemBytes
		ret.ContextBudget.EstimatedTokens += itemTokens
		ret.RetrievedHits = append(ret.RetrievedHits, workorchmemorycontract.RecallHit{
			ID: item.ID, Summary: item.Summary, VectorScore: item.VectorScore, VectorDimension: len(item.Vector),
		})
	}
	return ret, nil
}

func runtimeMemoryTypeQuotaEntries() []workorchmemorycontract.TypeQuotaEntry {
	return []workorchmemorycontract.TypeQuotaEntry{
		{Type: memorycommon.MemoryTypeDecision, Quota: runtimeMemoryTypeQuotas[memorycommon.MemoryTypeDecision]},
		{Type: memorycommon.MemoryTypeDoDFeedback, Quota: runtimeMemoryTypeQuotas[memorycommon.MemoryTypeDoDFeedback]},
		{Type: memorycommon.MemoryTypeExecutionLesson, Quota: runtimeMemoryTypeQuotas[memorycommon.MemoryTypeExecutionLesson]},
		{Type: memorycommon.MemoryTypeProjectContext, Quota: runtimeMemoryTypeQuotas[memorycommon.MemoryTypeProjectContext]},
		{Type: memorycommon.MemoryTypeRunSummary, Quota: runtimeMemoryTypeQuotas[memorycommon.MemoryTypeRunSummary]},
		{Type: memorycommon.MemoryTypeSkillFeedback, Quota: runtimeMemoryTypeQuotas[memorycommon.MemoryTypeSkillFeedback]},
		{Type: memorycommon.MemoryTypeToolFeedback, Quota: runtimeMemoryTypeQuotas[memorycommon.MemoryTypeToolFeedback]},
		{Type: memorycommon.MemoryTypeUserPreference, Quota: runtimeMemoryTypeQuotas[memorycommon.MemoryTypeUserPreference]},
	}
}

func runtimeMemoryContextBytes(item *memorymodel.MemoryItem) int {
	if item == nil {
		return 0
	}
	value := len(item.ID) + len(item.MemoryType) + len(strings.TrimSpace(item.Summary)) + len(strings.TrimSpace(item.Content)) + len(item.Visibility)
	for _, tag := range item.Tags {
		value += len(strings.TrimSpace(tag))
	}
	return value
}

func estimatedMemoryTokens(byteCount int) int {
	if byteCount <= 0 {
		return 0
	}
	return (byteCount + 3) / 4
}

func runtimeMemorySelectionScore(item *memorymodel.MemoryItem) int {
	if item == nil {
		return 0
	}
	vectorScore := item.VectorScore
	if math.IsNaN(vectorScore) || math.IsInf(vectorScore, 0) {
		vectorScore = 0
	}
	return item.Importance*100 + item.Confidence + int(math.Round(vectorScore*1000))
}
