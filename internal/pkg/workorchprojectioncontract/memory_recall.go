package workorchprojectioncontract

import "github.com/muidea/workorch/internal/pkg/workorchmemorycontract"

type MemoryRecallTrace struct {
	ID                     string                                          `json:"id"`
	WorkspaceID            string                                          `json:"workspaceId"`
	RunID                  string                                          `json:"runId"`
	StepID                 string                                          `json:"stepId,omitempty"`
	GraphNodeID            string                                          `json:"graphNodeId,omitempty"`
	Stage                  string                                          `json:"stage,omitempty"`
	Source                 string                                          `json:"source"`
	Policy                 string                                          `json:"policy,omitempty"`
	Status                 string                                          `json:"status"`
	ProviderID             string                                          `json:"providerId,omitempty"`
	ModelID                string                                          `json:"modelId,omitempty"`
	Operation              string                                          `json:"operation,omitempty"`
	QueryTextPreview       string                                          `json:"queryTextPreview,omitempty"`
	QueryTextSha256        string                                          `json:"queryTextSha256,omitempty"`
	MinScore               float64                                         `json:"minScore,omitempty"`
	Limit                  int                                             `json:"limit,omitempty"`
	EmbeddingModel         string                                          `json:"embeddingModel,omitempty"`
	VectorDimension        int                                             `json:"vectorDimension,omitempty"`
	RefreshSource          string                                          `json:"refreshSource,omitempty"`
	SelectionPolicyVersion string                                          `json:"selectionPolicyVersion,omitempty"`
	CandidateMemoryIDs     []string                                        `json:"candidateMemoryIds,omitempty"`
	SelectedMemoryIDs      []string                                        `json:"selectedMemoryIds,omitempty"`
	ExcludedMemoryIDs      []string                                        `json:"excludedMemoryIds,omitempty"`
	TypeQuota              map[string]int                                  `json:"typeQuota,omitempty"`
	SelectionScores        []workorchmemorycontract.RecallSelectionScore   `json:"selectionScores,omitempty"`
	RetrievedMemoryIDs     []string                                        `json:"retrievedMemoryIds,omitempty"`
	RetrievedMemoryHits    []workorchmemorycontract.RecallHit              `json:"retrievedMemoryHits,omitempty"`
	ContextBudget          workorchmemorycontract.RecallContextBudget      `json:"contextBudget"`
	SemanticRecall         *workorchmemorycontract.RecallSelectionEvidence `json:"semanticRecall,omitempty"`
	FallbackFrom           string                                          `json:"fallbackFrom,omitempty"`
	Reason                 string                                          `json:"reason,omitempty"`
	OccurredAt             string                                          `json:"occurredAt,omitempty"`
	EvidenceRef            string                                          `json:"evidenceRef,omitempty"`
}

// CloneStringIntMap detaches a small owner-neutral projection map.
func CloneStringIntMap(input map[string]int) map[string]int {
	if input == nil {
		return nil
	}
	ret := make(map[string]int, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func CloneMemoryRecallTraces(items []MemoryRecallTrace) []MemoryRecallTrace {
	if items == nil {
		return nil
	}
	ret := make([]MemoryRecallTrace, 0, len(items))
	for _, item := range items {
		item.CandidateMemoryIDs = append([]string(nil), item.CandidateMemoryIDs...)
		item.SelectedMemoryIDs = append([]string(nil), item.SelectedMemoryIDs...)
		item.ExcludedMemoryIDs = append([]string(nil), item.ExcludedMemoryIDs...)
		item.TypeQuota = CloneStringIntMap(item.TypeQuota)
		item.SelectionScores = append([]workorchmemorycontract.RecallSelectionScore(nil), item.SelectionScores...)
		item.RetrievedMemoryIDs = append([]string(nil), item.RetrievedMemoryIDs...)
		item.RetrievedMemoryHits = append([]workorchmemorycontract.RecallHit(nil), item.RetrievedMemoryHits...)
		item.SemanticRecall = workorchmemorycontract.CloneRecallSelectionEvidence(item.SemanticRecall)
		ret = append(ret, item)
	}
	return ret
}
