package workorchprojectioncontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
)

func TestCloneMemoryRecallTracesDetachesCollections(t *testing.T) {
	input := []MemoryRecallTrace{{
		CandidateMemoryIDs: []string{"memory-1"},
		SelectedMemoryIDs:  []string{"memory-1"},
		TypeQuota:          map[string]int{"semantic": 2},
		SemanticRecall: &workorchmemorycontract.RecallSelectionEvidence{
			Policy: "semantic", Status: "degraded", CandidateMemoryIDs: []common.MemoryID{"memory-2"},
		},
	}}
	got := CloneMemoryRecallTraces(input)
	input[0].CandidateMemoryIDs[0] = "mutated"
	input[0].SelectedMemoryIDs[0] = "mutated"
	input[0].TypeQuota["semantic"] = 99
	input[0].SemanticRecall.CandidateMemoryIDs[0] = "mutated"
	if got[0].CandidateMemoryIDs[0] != "memory-1" || got[0].SelectedMemoryIDs[0] != "memory-1" || got[0].TypeQuota["semantic"] != 2 || got[0].SemanticRecall.CandidateMemoryIDs[0] != "memory-2" {
		t.Fatalf("memory recall clone shares source storage: %+v", got[0])
	}
}
