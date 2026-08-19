package memory

import (
	"reflect"

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
	ModelID                string
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
	ContextBudget          workorchmemorycontract.RecallContextBudget
	SemanticRecall         *workorchmemorycontract.RecallSelectionEvidence
	FallbackFrom           string
	Reason                 string
	OccurredAt             string
	EvidenceRef            string
}

func SameMemoryRecallTrace(left, right *MemoryRecallTrace) bool {
	return reflect.DeepEqual(CloneMemoryRecallTrace(left), CloneMemoryRecallTrace(right))
}

func CloneMemoryRecallTrace(item *MemoryRecallTrace) *MemoryRecallTrace {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.CandidateMemoryIDs = append([]common.MemoryID(nil), item.CandidateMemoryIDs...)
	cloned.SelectedMemoryIDs = append([]common.MemoryID(nil), item.SelectedMemoryIDs...)
	cloned.ExcludedMemoryIDs = append([]common.MemoryID(nil), item.ExcludedMemoryIDs...)
	if item.TypeQuota != nil {
		cloned.TypeQuota = make(map[string]int, len(item.TypeQuota))
		for key, value := range item.TypeQuota {
			cloned.TypeQuota[key] = value
		}
	}
	cloned.SelectionScores = append([]workorchmemorycontract.RecallSelectionScore(nil), item.SelectionScores...)
	cloned.RetrievedMemoryIDs = append([]common.MemoryID(nil), item.RetrievedMemoryIDs...)
	cloned.RetrievedMemoryHits = append([]workorchmemorycontract.RecallHit(nil), item.RetrievedMemoryHits...)
	if item.SemanticRecall != nil {
		value := item.SemanticRecall.Normalized()
		cloned.SemanticRecall = &value
	}
	return &cloned
}
