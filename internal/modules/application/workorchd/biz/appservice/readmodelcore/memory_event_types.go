package readmodelcore

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

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

type MemoryRecallHit struct {
	ID              string  `json:"id"`
	Summary         string  `json:"summary,omitempty"`
	VectorScore     float64 `json:"vectorScore,omitempty"`
	VectorDimension int     `json:"vectorDimension,omitempty"`
}

type MemoryRecallSelectionScore struct {
	ID     string `json:"id"`
	Score  int    `json:"score"`
	Reason string `json:"reason,omitempty"`
}

type EventRecord struct {
	ID             string                       `json:"id"`
	Version        string                       `json:"version"`
	Topic          string                       `json:"topic"`
	OccurredAt     string                       `json:"occurredAt,omitempty"`
	SourceModule   string                       `json:"sourceModule,omitempty"`
	SubjectType    string                       `json:"subjectType,omitempty"`
	SubjectID      string                       `json:"subjectId,omitempty"`
	WorkspaceID    string                       `json:"workspaceId,omitempty"`
	RunID          string                       `json:"runId,omitempty"`
	StepID         string                       `json:"stepId,omitempty"`
	Revision       int64                        `json:"revision,omitempty"`
	Payload        workorcheventpayload.Payload `json:"payload,omitempty"`
	PayloadSummary workorcheventpayload.Payload `json:"payloadSummary,omitempty"`
	PayloadRef     string                       `json:"payloadRef,omitempty"`
}
