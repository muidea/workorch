package contracts

import "github.com/muidea/workorch/internal/pkg/common"

type CreateMemoryItemCommand struct {
	MemoryID            common.MemoryID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	SourceType          string
	SourceRef           string
	MemoryType          string
	Content             string
	Summary             string
	Tags                []string
	Importance          int
	Confidence          int
	Visibility          string
	EmbeddingProviderID string
	EmbeddingModel      string
	EmbeddingText       string
	Vector              []float64
}

type UpdateMemoryItemCommand = CreateMemoryItemCommand

type DeleteMemoryItemCommand struct {
	MemoryID common.MemoryID
}

type MemoryRecallHitCommand struct {
	ID              common.MemoryID
	Summary         string
	VectorScore     float64
	VectorDimension int
}

type MemoryRecallSelectionScoreCommand struct {
	ID     common.MemoryID
	Score  int
	Reason string
}

type UpsertMemoryRecallTraceCommand struct {
	TraceID                common.MemoryRecallTraceID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	StepID                 common.StepID
	GraphNodeID            string
	Stage                  string
	Source                 string
	Policy                 string
	Status                 string
	ProviderID             string
	Operation              string
	QueryTextPreview       string
	QueryTextSha256        string
	MinScore               float64
	Limit                  int
	EmbeddingModel         string
	VectorDimension        int
	RefreshSource          string
	SelectionPolicyVersion string
	CandidateMemoryIDs     []common.MemoryID
	SelectedMemoryIDs      []common.MemoryID
	ExcludedMemoryIDs      []common.MemoryID
	TypeQuota              map[string]int
	SelectionScores        []MemoryRecallSelectionScoreCommand
	RetrievedMemoryIDs     []common.MemoryID
	RetrievedMemoryHits    []MemoryRecallHitCommand
	Reason                 string
	OccurredAt             string
	EvidenceRef            string
}

type DeleteMemoryRecallTraceCommand struct {
	TraceID common.MemoryRecallTraceID
}
