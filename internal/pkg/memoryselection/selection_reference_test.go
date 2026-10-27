package memoryselection

import (
	"math"
	"strings"

	memorycommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
)

const (
	ReferenceMemoryContextMaxItems           = 8
	ReferenceMemoryContextMaxBytes           = 16 * 1024
	ReferenceMemoryContextMaxEstimatedTokens = 4096
	ReferenceMemorySelectionCandidateLimit   = workorchmemorycontract.RecallSelectionMaxCandidateCount
	ReferenceMemorySelectionPolicyVersion    = "activation-memory-selection/v1"
)

var referenceMemoryTypeQuotas = map[string]int{
	memorycommon.MemoryTypeDecision:        1,
	memorycommon.MemoryTypeExecutionLesson: 2,
	memorycommon.MemoryTypeProjectContext:  2,
	memorycommon.MemoryTypeDoDFeedback:     2,
	memorycommon.MemoryTypeUserPreference:  1,
	memorycommon.MemoryTypeSkillFeedback:   1,
	memorycommon.MemoryTypeToolFeedback:    1,
	memorycommon.MemoryTypeRunSummary:      2,
}

type ReferenceMemorySelection struct {
	Candidates      []*memorymodel.MemoryItem
	Selected        []*memorymodel.MemoryItem
	Excluded        []*memorymodel.MemoryItem
	TypeQuota       []workorchmemorycontract.TypeQuotaEntry
	SelectionScores []workorchmemorycontract.RecallSelectionScore
	RetrievedHits   []workorchmemorycontract.RecallHit
	ContextBudget   workorchmemorycontract.RecallContextBudget
}

func SelectReferenceMemoryContext(candidates []*memorymodel.MemoryItem) (ReferenceMemorySelection, error) {
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
	if len(unique) > ReferenceMemorySelectionCandidateLimit {
		return ReferenceMemorySelection{}, common.ErrInvalidArgument
	}

	ret := ReferenceMemorySelection{
		Candidates: unique,
		TypeQuota:  referenceMemoryTypeQuotaEntries(),
		ContextBudget: workorchmemorycontract.RecallContextBudget{
			MaxItems: ReferenceMemoryContextMaxItems, MaxBytes: ReferenceMemoryContextMaxBytes,
			MaxEstimatedTokens: ReferenceMemoryContextMaxEstimatedTokens,
		},
	}
	usedByType := make(map[string]int, len(referenceMemoryTypeQuotas))
	for _, item := range unique {
		reason := "selected_within_context_budget"
		selected := true
		memoryType := strings.TrimSpace(item.MemoryType)
		quota := referenceMemoryTypeQuotas[memoryType]
		if quota <= 0 {
			quota = 1
		}
		itemBytes := referenceMemoryContextBytes(item)
		itemTokens := referenceEstimatedMemoryTokens(itemBytes)
		switch {
		case !referenceMemoryVisibleToLLM(item):
			selected, reason = false, "excluded_visibility_policy"
		case len(ret.Selected) >= ReferenceMemoryContextMaxItems:
			selected, reason = false, "excluded_item_limit"
		case usedByType[memoryType] >= quota:
			selected, reason = false, "excluded_type_quota"
		case ret.ContextBudget.UsedBytes+itemBytes > ret.ContextBudget.MaxBytes:
			selected, reason = false, "excluded_byte_budget"
		case ret.ContextBudget.EstimatedTokens+itemTokens > ret.ContextBudget.MaxEstimatedTokens:
			selected, reason = false, "excluded_token_budget"
		}
		ret.SelectionScores = append(ret.SelectionScores, workorchmemorycontract.RecallSelectionScore{
			ID: item.ID, Score: referenceMemorySelectionScore(item), Reason: reason,
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

func referenceMemoryTypeQuotaEntries() []workorchmemorycontract.TypeQuotaEntry {
	return []workorchmemorycontract.TypeQuotaEntry{
		{Type: memorycommon.MemoryTypeDecision, Quota: referenceMemoryTypeQuotas[memorycommon.MemoryTypeDecision]},
		{Type: memorycommon.MemoryTypeDoDFeedback, Quota: referenceMemoryTypeQuotas[memorycommon.MemoryTypeDoDFeedback]},
		{Type: memorycommon.MemoryTypeExecutionLesson, Quota: referenceMemoryTypeQuotas[memorycommon.MemoryTypeExecutionLesson]},
		{Type: memorycommon.MemoryTypeProjectContext, Quota: referenceMemoryTypeQuotas[memorycommon.MemoryTypeProjectContext]},
		{Type: memorycommon.MemoryTypeRunSummary, Quota: referenceMemoryTypeQuotas[memorycommon.MemoryTypeRunSummary]},
		{Type: memorycommon.MemoryTypeSkillFeedback, Quota: referenceMemoryTypeQuotas[memorycommon.MemoryTypeSkillFeedback]},
		{Type: memorycommon.MemoryTypeToolFeedback, Quota: referenceMemoryTypeQuotas[memorycommon.MemoryTypeToolFeedback]},
		{Type: memorycommon.MemoryTypeUserPreference, Quota: referenceMemoryTypeQuotas[memorycommon.MemoryTypeUserPreference]},
	}
}

func referenceMemoryContextBytes(item *memorymodel.MemoryItem) int {
	if item == nil {
		return 0
	}
	value := len(item.ID) + len(item.MemoryType) + len(strings.TrimSpace(item.Summary)) + len(strings.TrimSpace(item.Content)) + len(item.Visibility)
	for _, tag := range item.Tags {
		value += len(strings.TrimSpace(tag))
	}
	return value
}

func referenceEstimatedMemoryTokens(byteCount int) int {
	if byteCount <= 0 {
		return 0
	}
	return (byteCount + 3) / 4
}

func referenceMemorySelectionScore(item *memorymodel.MemoryItem) int {
	if item == nil {
		return 0
	}
	vectorScore := item.VectorScore
	if math.IsNaN(vectorScore) || math.IsInf(vectorScore, 0) {
		vectorScore = 0
	}
	return item.Importance*100 + item.Confidence + int(math.Round(vectorScore*1000))
}

func referenceMemoryVisibleToLLM(item *memorymodel.MemoryItem) bool {
	if item == nil {
		return false
	}
	if strings.TrimSpace(item.MemoryType) != "tool_feedback" {
		return true
	}
	if referenceMemoryHasTag(item, "accepted_input") {
		return true
	}
	return !referenceMemoryHasTag(item, "provider_retry")
}

func referenceMemoryHasTag(item *memorymodel.MemoryItem, tag string) bool {
	if item == nil {
		return false
	}
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return false
	}
	for _, candidate := range item.Tags {
		if strings.TrimSpace(strings.ToLower(candidate)) == tag {
			return true
		}
	}
	return false
}
