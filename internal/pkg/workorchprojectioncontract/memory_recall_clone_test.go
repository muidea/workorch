package workorchprojectioncontract

import "testing"

func TestCloneMemoryRecallTracesDetachesCollections(t *testing.T) {
	input := []MemoryRecallTrace{{
		CandidateMemoryIDs: []string{"memory-1"},
		SelectedMemoryIDs:  []string{"memory-1"},
		TypeQuota:          map[string]int{"semantic": 2},
	}}
	got := CloneMemoryRecallTraces(input)
	input[0].CandidateMemoryIDs[0] = "mutated"
	input[0].SelectedMemoryIDs[0] = "mutated"
	input[0].TypeQuota["semantic"] = 99
	if got[0].CandidateMemoryIDs[0] != "memory-1" || got[0].SelectedMemoryIDs[0] != "memory-1" || got[0].TypeQuota["semantic"] != 2 {
		t.Fatalf("memory recall clone shares source storage: %+v", got[0])
	}
}
