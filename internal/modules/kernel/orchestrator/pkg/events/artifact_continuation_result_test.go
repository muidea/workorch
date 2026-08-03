package events

import (
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestArtifactContinuationResultUsesStableEntriesAndClones(t *testing.T) {
	model := &runtimecontract.ArtifactContinuation{
		ID: "continuation-1", WorkspaceID: "ws-1", RunID: "run-1",
		CoverageTargets: []string{"target-1"}, ReceivedSequences: []int{2, 0},
		ReceivedChunkHashes: map[int]string{2: "hash-2", 0: "hash-0"},
		RetryCounters:       map[string]int{"transport": 2, "candidate": 1}, EvidenceRefs: []string{"evidence-1"},
	}
	dto := ArtifactContinuationFromModel(model)
	if want := []ArtifactContinuationChunkHash{{Sequence: 0, Hash: "hash-0"}, {Sequence: 2, Hash: "hash-2"}}; !reflect.DeepEqual(dto.ReceivedChunkHashes, want) {
		t.Fatalf("chunk hashes = %#v, want %#v", dto.ReceivedChunkHashes, want)
	}
	if want := []ArtifactContinuationRetryCounter{{Key: "candidate", Count: 1}, {Key: "transport", Count: 2}}; !reflect.DeepEqual(dto.RetryCounters, want) {
		t.Fatalf("retry counters = %#v, want %#v", dto.RetryCounters, want)
	}
	model.CoverageTargets[0] = "changed"
	model.ReceivedChunkHashes[0] = "changed"
	if dto.CoverageTargets[0] != "target-1" || dto.ReceivedChunkHashes[0].Hash != "hash-0" {
		t.Fatalf("result DTO shares model state: %#v", dto)
	}

	roundTrip, err := dto.Model()
	if err != nil {
		t.Fatalf("Model: %v", err)
	}
	dto.ReceivedChunkHashes[0].Hash = "dto-changed"
	dto.EvidenceRefs[0] = "dto-changed"
	if roundTrip.ReceivedChunkHashes[0] != "hash-0" || roundTrip.EvidenceRefs[0] != "evidence-1" {
		t.Fatalf("model shares result DTO state: %#v", roundTrip)
	}
}

func TestArtifactContinuationResultRejectsDuplicateEntryKeys(t *testing.T) {
	base := ArtifactContinuation{ID: "continuation-1", WorkspaceID: "ws-1", RunID: "run-1"}
	base.ReceivedChunkHashes = []ArtifactContinuationChunkHash{{Sequence: 0}, {Sequence: 0}}
	if _, err := base.Model(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("duplicate sequence err = %v, want ErrInvalidArgument", err)
	}
	base.ReceivedChunkHashes = nil
	base.RetryCounters = []ArtifactContinuationRetryCounter{{Key: "transport"}, {Key: "transport"}}
	if _, err := base.Model(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("duplicate retry key err = %v, want ErrInvalidArgument", err)
	}
}
