package activationports

// MemorySelectionEvidence is the typed audit evidence for activation memory selection.
// It replaces ad-hoc map[string]any contracts on the selection path.
type MemorySelectionEvidence struct {
	Policy              string                   `json:"policy,omitempty"`
	Status              string                   `json:"status,omitempty"`
	Reason              string                   `json:"reason,omitempty"`
	ProviderID          string                   `json:"providerId,omitempty"`
	ModelID             string                   `json:"modelId,omitempty"`
	Operation           string                   `json:"operation,omitempty"`
	QueryTextPreview    string                   `json:"queryTextPreview,omitempty"`
	QueryTextSHA256     string                   `json:"queryTextSha256,omitempty"`
	MinScore            float64                  `json:"minScore,omitempty"`
	Limit               int                      `json:"limit,omitempty"`
	RetrievedMemoryIDs  []string                 `json:"retrievedMemoryIds,omitempty"`
	CandidateMemoryIDs  []string                 `json:"candidateMemoryIds,omitempty"`
	ExcludedMemoryIDs   []string                 `json:"excludedMemoryIds,omitempty"`
	RetrievedMemoryHits []MemorySelectionHit     `json:"retrievedMemoryHits,omitempty"`
	EmbeddingModel      string                   `json:"embeddingModel,omitempty"`
	VectorDimension     int                      `json:"vectorDimension,omitempty"`
	SemanticRecall      *MemorySelectionEvidence `json:"semanticRecall,omitempty"`
	FallbackFrom        string                   `json:"fallbackFrom,omitempty"`
}

// MemorySelectionHit is one scored memory item in selection evidence.
type MemorySelectionHit struct {
	ID    string  `json:"id,omitempty"`
	Score float64 `json:"score,omitempty"`
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
	if e.CandidateMemoryIDs != nil {
		out["candidateMemoryIds"] = append([]string(nil), e.CandidateMemoryIDs...)
	}
	if e.ExcludedMemoryIDs != nil {
		out["excludedMemoryIds"] = append([]string(nil), e.ExcludedMemoryIDs...)
	}
	if len(e.RetrievedMemoryHits) > 0 {
		hits := make([]map[string]any, 0, len(e.RetrievedMemoryHits))
		for _, hit := range e.RetrievedMemoryHits {
			hits = append(hits, map[string]any{"id": hit.ID, "score": hit.Score})
		}
		out["retrievedMemoryHits"] = hits
	}
	if e.EmbeddingModel != "" {
		out["embeddingModel"] = e.EmbeddingModel
	}
	if e.VectorDimension != 0 {
		out["vectorDimension"] = e.VectorDimension
	}
	if e.SemanticRecall != nil {
		out["semanticRecall"] = e.SemanticRecall.Map()
	}
	if e.FallbackFrom != "" {
		out["fallbackFrom"] = e.FallbackFrom
	}
	return out
}
