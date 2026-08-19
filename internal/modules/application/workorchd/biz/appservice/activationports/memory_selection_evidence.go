package activationports

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

// Map converts typed evidence into a stable audit map for legacy persistence bridges.
// Prefer MemorySelectionEvidence itself at new call sites.
func (e MemorySelectionEvidence) Map() map[string]any {
	if e.Policy == "" && e.Status == "" && e.Reason == "" && e.ProviderID == "" && len(e.RetrievedMemoryIDs) == 0 && e.SemanticRecall == nil {
		return map[string]any{}
	}
	out := map[string]any{}
	if e.Policy != "" {
		out["policy"] = e.Policy
	}
	if e.Status != "" {
		out["status"] = e.Status
	}
	if e.Reason != "" {
		out["reason"] = e.Reason
	}
	if e.ProviderID != "" {
		out["providerId"] = e.ProviderID
	}
	if e.ModelID != "" {
		out["modelId"] = e.ModelID
	}
	if e.Operation != "" {
		out["operation"] = e.Operation
	}
	if e.QueryTextPreview != "" {
		out["queryTextPreview"] = e.QueryTextPreview
	}
	if e.QueryTextSHA256 != "" {
		out["queryTextSha256"] = e.QueryTextSHA256
	}
	if e.MinScore != 0 {
		out["minScore"] = e.MinScore
	}
	if e.Limit != 0 {
		out["limit"] = e.Limit
	}
	if e.RetrievedMemoryIDs != nil {
		out["retrievedMemoryIds"] = append([]string(nil), e.RetrievedMemoryIDs...)
	}
	if e.SelectedMemoryIDs != nil {
		out["selectedMemoryIds"] = append([]string(nil), e.SelectedMemoryIDs...)
	}
	if e.CandidateMemoryIDs != nil {
		out["candidateMemoryIds"] = append([]string(nil), e.CandidateMemoryIDs...)
	}
	if e.ExcludedMemoryIDs != nil {
		out["excludedMemoryIds"] = append([]string(nil), e.ExcludedMemoryIDs...)
	}
	if e.TypeQuota != nil {
		out["typeQuota"] = append([]workorchmemorycontract.TypeQuotaEntry(nil), e.TypeQuota...)
	}
	if e.SelectionScores != nil {
		out["selectionScores"] = append([]workorchmemorycontract.RecallSelectionScore(nil), e.SelectionScores...)
	}
	out["contextBudget"] = e.ContextBudget
	if len(e.RetrievedMemoryHits) > 0 {
		hits := make([]map[string]any, 0, len(e.RetrievedMemoryHits))
		for _, hit := range e.RetrievedMemoryHits {
			value := map[string]any{"id": hit.ID, "score": hit.Score}
			if hit.Summary != "" {
				value["summary"] = hit.Summary
			}
			if hit.VectorDimension != 0 {
				value["vectorDimension"] = hit.VectorDimension
			}
			hits = append(hits, value)
		}
		out["retrievedMemoryHits"] = hits
	}
	if e.EmbeddingModel != "" {
		out["embeddingModel"] = e.EmbeddingModel
	}
	if e.VectorDimension != 0 {
		out["vectorDimension"] = e.VectorDimension
	}
	if e.RefreshSource != "" {
		out["refreshSource"] = e.RefreshSource
	}
	if e.SelectionPolicyVersion != "" {
		out["selectionPolicyVersion"] = e.SelectionPolicyVersion
	}
	if e.SemanticRecall != nil {
		out["semanticRecall"] = e.SemanticRecall.Map()
	}
	if e.FallbackFrom != "" {
		out["fallbackFrom"] = e.FallbackFrom
	}
	return out
}
