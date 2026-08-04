package workorchprojectioncontract

type MemoryRecallTrace struct {
	ID                     string                       `json:"id"`
	WorkspaceID            string                       `json:"workspaceId"`
	RunID                  string                       `json:"runId"`
	StepID                 string                       `json:"stepId,omitempty"`
	GraphNodeID            string                       `json:"graphNodeId,omitempty"`
	Stage                  string                       `json:"stage,omitempty"`
	Source                 string                       `json:"source"`
	Policy                 string                       `json:"policy,omitempty"`
	Status                 string                       `json:"status"`
	ProviderID             string                       `json:"providerId,omitempty"`
	Operation              string                       `json:"operation,omitempty"`
	QueryTextPreview       string                       `json:"queryTextPreview,omitempty"`
	QueryTextSha256        string                       `json:"queryTextSha256,omitempty"`
	MinScore               float64                      `json:"minScore,omitempty"`
	Limit                  int                          `json:"limit,omitempty"`
	EmbeddingModel         string                       `json:"embeddingModel,omitempty"`
	VectorDimension        int                          `json:"vectorDimension,omitempty"`
	RefreshSource          string                       `json:"refreshSource,omitempty"`
	SelectionPolicyVersion string                       `json:"selectionPolicyVersion,omitempty"`
	CandidateMemoryIDs     []string                     `json:"candidateMemoryIds,omitempty"`
	SelectedMemoryIDs      []string                     `json:"selectedMemoryIds,omitempty"`
	ExcludedMemoryIDs      []string                     `json:"excludedMemoryIds,omitempty"`
	TypeQuota              map[string]int               `json:"typeQuota,omitempty"`
	SelectionScores        []MemoryRecallSelectionScore `json:"selectionScores,omitempty"`
	RetrievedMemoryIDs     []string                     `json:"retrievedMemoryIds,omitempty"`
	RetrievedMemoryHits    []MemoryRecallHit            `json:"retrievedMemoryHits,omitempty"`
	Reason                 string                       `json:"reason,omitempty"`
	OccurredAt             string                       `json:"occurredAt,omitempty"`
	EvidenceRef            string                       `json:"evidenceRef,omitempty"`
}

type MemoryRecallSelectionScore struct {
	ID     string `json:"id"`
	Score  int    `json:"score"`
	Reason string `json:"reason,omitempty"`
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
