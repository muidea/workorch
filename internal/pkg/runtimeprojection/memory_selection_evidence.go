package runtimeprojection

import "github.com/muidea/workorch/internal/pkg/workorchmemorycontract"

// MemorySelectionEvidence is the typed audit evidence for activation memory selection.
// It replaces ad-hoc map[string]any contracts on the selection path.
type MemorySelectionEvidence struct {
	Policy                 string                                        `json:"policy,omitempty"`
	Status                 string                                        `json:"status,omitempty"`
	Reason                 string                                        `json:"reason,omitempty"`
	ProviderID             string                                        `json:"providerId,omitempty"`
	ModelID                string                                        `json:"modelId,omitempty"`
	Operation              string                                        `json:"operation,omitempty"`
	QueryTextPreview       string                                        `json:"queryTextPreview,omitempty"`
	QueryTextSHA256        string                                        `json:"queryTextSha256,omitempty"`
	MinScore               float64                                       `json:"minScore,omitempty"`
	Limit                  int                                           `json:"limit,omitempty"`
	RetrievedMemoryIDs     []string                                      `json:"retrievedMemoryIds,omitempty"`
	CandidateMemoryIDs     []string                                      `json:"candidateMemoryIds,omitempty"`
	RetrievedMemoryHits    []MemorySelectionHit                          `json:"retrievedMemoryHits,omitempty"`
	EmbeddingModel         string                                        `json:"embeddingModel,omitempty"`
	VectorDimension        int                                           `json:"vectorDimension,omitempty"`
	RefreshSource          string                                        `json:"refreshSource,omitempty"`
	SelectionPolicyVersion string                                        `json:"selectionPolicyVersion,omitempty"`
	SemanticRecall         *MemorySelectionEvidence                      `json:"semanticRecall,omitempty"`
	FallbackFrom           string                                        `json:"fallbackFrom,omitempty"`
	SelectedMemoryIDs      []string                                      `json:"selectedMemoryIds,omitempty"`
	ExcludedMemoryIDs      []string                                      `json:"excludedMemoryIds,omitempty"`
	TypeQuota              []workorchmemorycontract.TypeQuotaEntry       `json:"typeQuota,omitempty"`
	SelectionScores        []workorchmemorycontract.RecallSelectionScore `json:"selectionScores,omitempty"`
	ContextBudget          workorchmemorycontract.RecallContextBudget    `json:"contextBudget"`
}

// MemorySelectionHit is one scored memory item in selection evidence.
type MemorySelectionHit struct {
	ID              string  `json:"id,omitempty"`
	Summary         string  `json:"summary,omitempty"`
	Score           float64 `json:"score,omitempty"`
	VectorDimension int     `json:"vectorDimension,omitempty"`
}
