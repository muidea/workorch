package events

import (
	"errors"
	"reflect"
	"testing"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
)

func TestUpsertMemoryRecallTraceCommandFromModelUsesStableConcreteQuotaEntries(t *testing.T) {
	trace := &memorymodel.MemoryRecallTrace{
		ID:          "trace-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		TypeQuota:   map[string]int{"semantic": 2, "episodic": 1},
	}
	command := UpsertMemoryRecallTraceCommandFromModel(trace)
	want := []workorchmemorycontract.TypeQuotaEntry{{Type: "episodic", Quota: 1}, {Type: "semantic", Quota: 2}}
	if !reflect.DeepEqual(command.TypeQuota, want) {
		t.Fatalf("TypeQuota = %+v, want stable entries %+v", command.TypeQuota, want)
	}
	trace.TypeQuota["episodic"] = 9
	if command.TypeQuota[0].Quota != 1 {
		t.Fatalf("command quota changed through model map mutation: %+v", command.TypeQuota)
	}
	if err := command.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestUpsertMemoryRecallTraceCommandRejectsInvalidQuotaEntries(t *testing.T) {
	base := UpsertMemoryRecallTraceCommand{TraceID: "trace-1", WorkspaceID: "ws-1", RunID: "run-1"}
	tests := []struct {
		name    string
		entries []workorchmemorycontract.TypeQuotaEntry
	}{
		{name: "blank type", entries: []workorchmemorycontract.TypeQuotaEntry{{Type: " ", Quota: 1}}},
		{name: "negative quota", entries: []workorchmemorycontract.TypeQuotaEntry{{Type: "semantic", Quota: -1}}},
		{name: "duplicate type", entries: []workorchmemorycontract.TypeQuotaEntry{{Type: "semantic", Quota: 1}, {Type: "semantic", Quota: 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := base
			command.TypeQuota = tt.entries
			if err := command.Validate(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Validate error = %v, want invalid argument", err)
			}
		})
	}
}

func TestMemoryRecallTraceResultRoundTripDoesNotShareMutableFields(t *testing.T) {
	model := &memorymodel.MemoryRecallTrace{
		ID: "trace-1", WorkspaceID: "ws-1", RunID: "run-1",
		CandidateMemoryIDs: []common.MemoryID{"memory-1"}, TypeQuota: map[string]int{"semantic": 2},
		SelectionScores:     []workorchmemorycontract.RecallSelectionScore{{ID: "memory-1", Score: 8}},
		RetrievedMemoryHits: []workorchmemorycontract.RecallHit{{ID: "memory-1", Summary: "proof"}},
	}
	dto := MemoryRecallTraceFromModel(model)
	model.CandidateMemoryIDs[0] = "changed"
	model.TypeQuota["semantic"] = 9
	model.RetrievedMemoryHits[0].Summary = "changed"
	if dto.CandidateMemoryIDs[0] != "memory-1" || dto.TypeQuota[0].Quota != 2 || dto.RetrievedMemoryHits[0].Summary != "proof" {
		t.Fatalf("result DTO shares model state: %#v", dto)
	}

	roundTrip, err := dto.Model()
	if err != nil {
		t.Fatalf("Model: %v", err)
	}
	dto.CandidateMemoryIDs[0] = "dto-changed"
	dto.TypeQuota[0].Quota = 10
	dto.SelectionScores[0].Score = 1
	if roundTrip.CandidateMemoryIDs[0] != "memory-1" || roundTrip.TypeQuota["semantic"] != 2 || roundTrip.SelectionScores[0].Score != 8 {
		t.Fatalf("model shares result DTO state: %#v", roundTrip)
	}
}
