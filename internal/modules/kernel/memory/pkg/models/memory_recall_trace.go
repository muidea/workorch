package memory

import "github.com/muidea/workorch/internal/pkg/common"

type MemoryRecallHit struct {
	ID              common.MemoryID
	Summary         string
	VectorScore     float64
	VectorDimension int
}

type MemoryRecallTrace struct {
	ID                  common.MemoryRecallTraceID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	StepID              common.StepID
	GraphNodeID         string
	Stage               string
	Source              string
	Policy              string
	Status              string
	ProviderID          string
	Operation           string
	QueryTextPreview    string
	QueryTextSha256     string
	MinScore            float64
	Limit               int
	EmbeddingModel      string
	VectorDimension     int
	RetrievedMemoryIDs  []common.MemoryID
	RetrievedMemoryHits []MemoryRecallHit
	Reason              string
	OccurredAt          string
	EvidenceRef         string
}
