package workorchmemorycontract

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RecallSelectionMaxCandidateCount = 200
	RecallSelectionMaxSelectedCount  = 16
)

// RecallContextBudget records the deterministic context budget applied before
// selected memory content is sealed into a Runtime activation.
type RecallContextBudget struct {
	MaxItems           int `json:"maxItems"`
	MaxBytes           int `json:"maxBytes"`
	MaxEstimatedTokens int `json:"maxEstimatedTokens"`
	UsedItems          int `json:"usedItems"`
	UsedBytes          int `json:"usedBytes"`
	EstimatedTokens    int `json:"estimatedTokens"`
}

func (b RecallContextBudget) Validate() error {
	if b.MaxItems <= 0 || b.MaxBytes <= 0 || b.MaxEstimatedTokens <= 0 ||
		b.UsedItems < 0 || b.UsedBytes < 0 || b.EstimatedTokens < 0 ||
		b.UsedItems > b.MaxItems || b.UsedBytes > b.MaxBytes || b.EstimatedTokens > b.MaxEstimatedTokens {
		return fmt.Errorf("%w: invalid memory recall context budget", common.ErrInvalidArgument)
	}
	return nil
}

// RecallSelectionEvidence is the immutable, owner-neutral audit fact produced
// by activation selection and later persisted by the Memory owner. It is not
// Runtime authority and cannot close coverage or select a Runtime action.
type RecallSelectionEvidence struct {
	Policy                 string                   `json:"policy,omitempty"`
	Status                 string                   `json:"status,omitempty"`
	Reason                 string                   `json:"reason,omitempty"`
	ProviderID             string                   `json:"providerId,omitempty"`
	ModelID                string                   `json:"modelId,omitempty"`
	Operation              string                   `json:"operation,omitempty"`
	QueryTextPreview       string                   `json:"queryTextPreview,omitempty"`
	QueryTextSHA256        string                   `json:"queryTextSha256,omitempty"`
	MinScore               float64                  `json:"minScore,omitempty"`
	Limit                  int                      `json:"limit,omitempty"`
	EmbeddingModel         string                   `json:"embeddingModel,omitempty"`
	VectorDimension        int                      `json:"vectorDimension,omitempty"`
	RefreshSource          string                   `json:"refreshSource,omitempty"`
	SelectionPolicyVersion string                   `json:"selectionPolicyVersion,omitempty"`
	CandidateMemoryIDs     []common.MemoryID        `json:"candidateMemoryIds,omitempty"`
	SelectedMemoryIDs      []common.MemoryID        `json:"selectedMemoryIds,omitempty"`
	ExcludedMemoryIDs      []common.MemoryID        `json:"excludedMemoryIds,omitempty"`
	TypeQuota              []TypeQuotaEntry         `json:"typeQuota,omitempty"`
	SelectionScores        []RecallSelectionScore   `json:"selectionScores,omitempty"`
	RetrievedMemoryIDs     []common.MemoryID        `json:"retrievedMemoryIds,omitempty"`
	RetrievedMemoryHits    []RecallHit              `json:"retrievedMemoryHits,omitempty"`
	ContextBudget          RecallContextBudget      `json:"contextBudget"`
	SemanticRecall         *RecallSelectionEvidence `json:"semanticRecall,omitempty"`
	FallbackFrom           string                   `json:"fallbackFrom,omitempty"`
}

func (e RecallSelectionEvidence) Empty() bool {
	return strings.TrimSpace(e.Policy) == "" && strings.TrimSpace(e.Status) == "" &&
		len(e.CandidateMemoryIDs) == 0 && len(e.SelectedMemoryIDs) == 0 &&
		len(e.RetrievedMemoryIDs) == 0 && e.SemanticRecall == nil
}

func (e RecallSelectionEvidence) Normalized() RecallSelectionEvidence {
	return normalizeRecallSelectionEvidence(e, 0)
}

func normalizeRecallSelectionEvidence(e RecallSelectionEvidence, depth int) RecallSelectionEvidence {
	e.Policy = strings.TrimSpace(e.Policy)
	e.Status = strings.TrimSpace(e.Status)
	e.Reason = strings.TrimSpace(e.Reason)
	e.ProviderID = strings.TrimSpace(e.ProviderID)
	e.ModelID = strings.TrimSpace(e.ModelID)
	e.Operation = strings.TrimSpace(e.Operation)
	e.QueryTextPreview = strings.TrimSpace(e.QueryTextPreview)
	e.QueryTextSHA256 = strings.TrimSpace(e.QueryTextSHA256)
	e.EmbeddingModel = strings.TrimSpace(e.EmbeddingModel)
	e.RefreshSource = strings.TrimSpace(e.RefreshSource)
	e.SelectionPolicyVersion = strings.TrimSpace(e.SelectionPolicyVersion)
	e.FallbackFrom = strings.TrimSpace(e.FallbackFrom)
	e.CandidateMemoryIDs = cloneMemoryIDs(e.CandidateMemoryIDs)
	e.SelectedMemoryIDs = cloneMemoryIDs(e.SelectedMemoryIDs)
	e.ExcludedMemoryIDs = cloneMemoryIDs(e.ExcludedMemoryIDs)
	e.RetrievedMemoryIDs = cloneMemoryIDs(e.RetrievedMemoryIDs)
	e.TypeQuota = append([]TypeQuotaEntry(nil), e.TypeQuota...)
	for idx := range e.TypeQuota {
		e.TypeQuota[idx].Type = strings.TrimSpace(e.TypeQuota[idx].Type)
	}
	sort.Slice(e.TypeQuota, func(i, j int) bool { return e.TypeQuota[i].Type < e.TypeQuota[j].Type })
	e.SelectionScores = append([]RecallSelectionScore(nil), e.SelectionScores...)
	for idx := range e.SelectionScores {
		e.SelectionScores[idx].Reason = strings.TrimSpace(e.SelectionScores[idx].Reason)
	}
	e.RetrievedMemoryHits = append([]RecallHit(nil), e.RetrievedMemoryHits...)
	for idx := range e.RetrievedMemoryHits {
		e.RetrievedMemoryHits[idx].Summary = strings.TrimSpace(e.RetrievedMemoryHits[idx].Summary)
	}
	if e.SemanticRecall != nil && depth == 0 {
		value := normalizeRecallSelectionEvidence(*e.SemanticRecall, depth+1)
		e.SemanticRecall = &value
	} else if depth > 0 {
		e.SemanticRecall = nil
	}
	return e
}

func (e RecallSelectionEvidence) Validate() error {
	if e.SemanticRecall != nil && e.SemanticRecall.SemanticRecall != nil {
		return fmt.Errorf("%w: nested memory recall evidence exceeds one level", common.ErrInvalidArgument)
	}
	e = e.Normalized()
	if e.Empty() || e.Policy == "" || e.Status == "" {
		return fmt.Errorf("%w: memory recall selection requires policy and status", common.ErrInvalidArgument)
	}
	if len(e.CandidateMemoryIDs) > RecallSelectionMaxCandidateCount ||
		len(e.SelectedMemoryIDs) > RecallSelectionMaxSelectedCount ||
		len(e.RetrievedMemoryIDs) > RecallSelectionMaxSelectedCount ||
		len(e.ExcludedMemoryIDs) > RecallSelectionMaxCandidateCount ||
		len(e.SelectionScores) > RecallSelectionMaxCandidateCount ||
		len(e.RetrievedMemoryHits) > RecallSelectionMaxSelectedCount {
		return fmt.Errorf("%w: memory recall selection exceeds bounded collections", common.ErrInvalidArgument)
	}
	if e.Limit < 0 || e.VectorDimension < 0 || math.IsNaN(e.MinScore) || math.IsInf(e.MinScore, 0) {
		return fmt.Errorf("%w: memory recall selection has invalid numeric fields", common.ErrInvalidArgument)
	}
	if err := e.ContextBudget.Validate(); err != nil {
		return err
	}
	candidates, err := uniqueMemoryIDs(e.CandidateMemoryIDs)
	if err != nil {
		return err
	}
	selected, err := uniqueMemoryIDs(e.SelectedMemoryIDs)
	if err != nil {
		return err
	}
	excluded, err := uniqueMemoryIDs(e.ExcludedMemoryIDs)
	if err != nil {
		return err
	}
	retrieved, err := uniqueMemoryIDs(e.RetrievedMemoryIDs)
	if err != nil {
		return err
	}
	for id := range selected {
		if _, ok := candidates[id]; !ok {
			return fmt.Errorf("%w: selected memory %s is not a candidate", common.ErrInvalidArgument, id)
		}
		if _, ok := excluded[id]; ok {
			return fmt.Errorf("%w: selected memory %s is also excluded", common.ErrInvalidArgument, id)
		}
	}
	for id := range excluded {
		if _, ok := candidates[id]; !ok {
			return fmt.Errorf("%w: excluded memory %s is not a candidate", common.ErrInvalidArgument, id)
		}
	}
	for id := range candidates {
		_, isSelected := selected[id]
		_, isExcluded := excluded[id]
		if isSelected == isExcluded {
			return fmt.Errorf("%w: candidate memory %s is not uniquely selected or excluded", common.ErrInvalidArgument, id)
		}
	}
	if !reflect.DeepEqual(selected, retrieved) {
		return fmt.Errorf("%w: selected and retrieved memory identities differ", common.ErrInvalidArgument)
	}
	if e.ContextBudget.UsedItems != len(e.SelectedMemoryIDs) {
		return fmt.Errorf("%w: memory recall budget item count mismatch", common.ErrInvalidArgument)
	}
	quotaTypes := make(map[string]struct{}, len(e.TypeQuota))
	for _, quota := range e.TypeQuota {
		if quota.Type == "" || quota.Quota < 0 {
			return fmt.Errorf("%w: invalid memory recall type quota", common.ErrInvalidArgument)
		}
		if _, ok := quotaTypes[quota.Type]; ok {
			return fmt.Errorf("%w: duplicate memory recall type quota", common.ErrInvalidArgument)
		}
		quotaTypes[quota.Type] = struct{}{}
	}
	scoreIDs := make(map[common.MemoryID]struct{}, len(e.SelectionScores))
	for _, score := range e.SelectionScores {
		if score.ID == "" || strings.TrimSpace(score.Reason) == "" {
			return fmt.Errorf("%w: invalid memory recall selection score", common.ErrInvalidArgument)
		}
		if _, ok := candidates[score.ID]; !ok {
			return fmt.Errorf("%w: scored memory is not a candidate", common.ErrInvalidArgument)
		}
		if _, ok := scoreIDs[score.ID]; ok {
			return fmt.Errorf("%w: duplicate memory recall selection score", common.ErrInvalidArgument)
		}
		scoreIDs[score.ID] = struct{}{}
	}
	if len(scoreIDs) != len(candidates) {
		return fmt.Errorf("%w: memory recall scores do not cover all candidates", common.ErrInvalidArgument)
	}
	hitIDs := make(map[common.MemoryID]struct{}, len(e.RetrievedMemoryHits))
	for _, hit := range e.RetrievedMemoryHits {
		if hit.ID == "" || hit.VectorDimension < 0 || math.IsNaN(hit.VectorScore) || math.IsInf(hit.VectorScore, 0) {
			return fmt.Errorf("%w: invalid memory recall hit", common.ErrInvalidArgument)
		}
		if _, ok := retrieved[hit.ID]; !ok {
			return fmt.Errorf("%w: recall hit is not retrieved", common.ErrInvalidArgument)
		}
		if _, ok := hitIDs[hit.ID]; ok {
			return fmt.Errorf("%w: duplicate memory recall hit", common.ErrInvalidArgument)
		}
		hitIDs[hit.ID] = struct{}{}
	}
	if len(hitIDs) != len(retrieved) {
		return fmt.Errorf("%w: memory recall hits do not cover all retrieved memories", common.ErrInvalidArgument)
	}
	if e.FallbackFrom != "" && e.SemanticRecall == nil {
		return fmt.Errorf("%w: memory recall fallback requires source evidence", common.ErrInvalidArgument)
	}
	if e.SemanticRecall != nil {
		if err := e.SemanticRecall.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func CloneRecallSelectionEvidence(input *RecallSelectionEvidence) *RecallSelectionEvidence {
	if input == nil {
		return nil
	}
	value := input.Normalized()
	return &value
}

func SameRecallSelectionEvidence(left, right RecallSelectionEvidence) bool {
	return reflect.DeepEqual(left.Normalized(), right.Normalized())
}

func cloneMemoryIDs(values []common.MemoryID) []common.MemoryID {
	if values == nil {
		return nil
	}
	return append([]common.MemoryID(nil), values...)
}

func uniqueMemoryIDs(values []common.MemoryID) (map[common.MemoryID]struct{}, error) {
	ret := make(map[common.MemoryID]struct{}, len(values))
	for _, id := range values {
		if id == "" {
			return nil, fmt.Errorf("%w: empty memory identity", common.ErrInvalidArgument)
		}
		if _, ok := ret[id]; ok {
			return nil, fmt.Errorf("%w: duplicate memory identity", common.ErrInvalidArgument)
		}
		ret[id] = struct{}{}
	}
	return ret, nil
}
