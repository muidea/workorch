package orchestrator

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This opt-in diagnostic performs pure archive admission only. It never starts
// a hub, opens a repository, drives Runtime or calls a provider/capability. The
// explicit path is supplied by the operator; ordinary CI does not read local
// runtime data. No fixture is regenerated from current defaults.
func TestRuntimeArchiveReadOnlyCompatibility(t *testing.T) {
	path := os.Getenv("WORKORCH_READONLY_RUN_ARCHIVE")
	if path == "" {
		t.Skip("set WORKORCH_READONLY_RUN_ARCHIVE to an explicit archived run.json for read-only admission")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records map[string]runmodel.Run
	if err := json.Unmarshal(before, &records); err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Fatal("archive contains no Run records")
	}
	for key, record := range records {
		if key != string(record.ID) || record.RuntimeBootstrap == nil || record.RuntimeState == nil {
			t.Fatal("archive requires exact Run identity, bootstrap and Runtime state")
		}
		if err := runtimecontract.ValidateRuntimeBootstrapRaw(*record.RuntimeBootstrap); err != nil {
			t.Fatalf("archived bootstrap no longer admits: %v", err)
		}
		if err := record.RuntimeState.Validate(); err != nil {
			t.Fatalf("archived Runtime record no longer admits: %v", err)
		}
		aggregate := runtimeAggregateFromRecord(*record.RuntimeState)
		if err := aggregate.Validate(); err != nil {
			t.Fatalf("archived Runtime authority no longer admits: %v", err)
		}
		if aggregate.ResultFeedbackVersion != record.RuntimeState.ResultFeedbackVersion {
			t.Fatal("archive admission upgraded the sealed Result renderer")
		}
		reloaded := runtimeStateRecordFromAggregate(aggregate)
		if err := reloaded.Validate(); err != nil {
			t.Fatalf("archived state mapping no longer admits: %v", err)
		}
		t.Logf("Run %s: state revision %d, sealed feedback version %d unchanged", key, aggregate.StateRevision, aggregate.ResultFeedbackVersion)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("archive changed during read-only admission: %v", err)
	}
	t.Logf("archive unchanged: sha256=%x", sha256.Sum256(before))
}
