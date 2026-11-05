package runtime

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func assertSemanticPurposeProjection(t *testing.T, raw rc.RuntimeSemanticPlanManifest) {
	t.Helper()
	original, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	full := raw.Normalized() // Frozen prior complete consumer oracle.
	got := runtimeSemanticPurposeManifest(raw)
	if got.Hash != full.Hash || !reflect.DeepEqual(got.ProofObligations, full.ProofObligations) {
		t.Fatal("purpose projection changed consumed identity/obligations")
	}
	after, _ := json.Marshal(raw)
	if string(original) != string(after) {
		t.Fatal("purpose consumer changed formal original")
	}
}

func TestSemanticPurposeProjectionKeepsExactConsumedFields(t *testing.T) {
	base := stateChangeAggregate(t).SemanticPlanning.Manifest
	if base == nil {
		t.Fatal("fixture requires sealed manifest")
	}
	assertSemanticPurposeProjection(t, *base)
	// Every manifest field is independently changed. Non-consumed data must
	// neither enter the purpose graph nor mutate the formal source. Consumed
	// fields retain the old canonicalization even on raw malformed input.
	typ := reflect.TypeOf(*base)
	for i := 0; i < typ.NumField(); i++ {
		t.Run(typ.Field(i).Name, func(t *testing.T) {
			raw := *(rc.RuntimeSemanticPlanningLedger{Manifest: base}).CloneRaw().Manifest
			field := reflect.ValueOf(&raw).Elem().Field(i)
			field.Set(reflect.Zero(field.Type()))
			assertSemanticPurposeProjection(t, raw)
		})
	}
}

func TestSemanticPurposeProjectionFixedArchive(t *testing.T) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit fixed read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var archive map[string]struct{ RuntimeState rc.RuntimeStateRecord }
	if err := json.Unmarshal(body, &archive); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range archive {
		ledger := item.RuntimeState.SemanticPlanning
		if ledger.Manifest != nil {
			assertSemanticPurposeProjection(t, *ledger.Manifest)
			count++
		}
		for _, generation := range ledger.AllHistory() {
			if generation.Snapshot.Manifest != nil {
				assertSemanticPurposeProjection(t, *generation.Snapshot.Manifest)
				count++
			}
		}
	}
	if count == 0 {
		t.Fatal("archive has no purpose lineage")
	}
}
