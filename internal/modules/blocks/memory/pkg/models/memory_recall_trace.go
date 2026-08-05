package memory

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
)

type MemoryRecallTrace struct {
	ID                     common.MemoryRecallTraceID
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
	SelectionScores        []workorchmemorycontract.RecallSelectionScore
	RetrievedMemoryIDs     []common.MemoryID
	RetrievedMemoryHits    []workorchmemorycontract.RecallHit
	Reason                 string
	OccurredAt             string
	EvidenceRef            string
}
