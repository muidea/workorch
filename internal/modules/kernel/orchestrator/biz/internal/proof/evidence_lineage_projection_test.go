package proof

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func compareEvidenceLineageProjection(t testing.TB, facts rc.RuntimeFacts, evidence rc.EvidenceFact, ids []common.CoverageWorkItemID, lineage []rc.RuntimeCoverageProofLineage) {
	t.Helper()
	before, _ := json.Marshal(facts)
	for _, id := range ids {
		got, gotFound := CurrentCWIBindingForFocus(facts, id)
		want, wantFound := referenceLineageBeforeProjectionCurrentCWIBindingForFocus(facts, id)
		if gotFound != wantFound || !reflect.DeepEqual(got, want) {
			t.Fatal("current binding projection differs from complete normalization")
		}
		if EvidenceAppliesToCoverageWorkItemWithLineage(facts, evidence, id, lineage) != referenceLineageBeforeProjectionEvidenceAppliesToCoverageWorkItemWithLineage(facts, evidence, id, lineage) {
			t.Fatal("evidence attribution differs from complete normalization")
		}
		if EvidenceHasEquivalentCoverageProofLineage(facts, evidence, id, lineage) != referenceLineageBeforeProjectionEvidenceHasEquivalentCoverageProofLineage(facts, evidence, id, lineage) {
			t.Fatal("equivalent resolution differs from complete normalization")
		}
	}
	if EvidenceAppliesToCoverageIDsWithLineage(facts, evidence, ids, lineage) != referenceLineageBeforeProjectionEvidenceAppliesToCoverageIDsWithLineage(facts, evidence, ids, lineage) {
		t.Fatal("set attribution differs from complete normalization")
	}
	after, _ := json.Marshal(facts)
	if !bytes.Equal(before, after) {
		t.Fatal("proof projection changed raw Facts")
	}
}

func TestEvidenceLineageProjectionAllFactsFieldsAndBindingFences(t *testing.T) {
	binding := proofTestCoverageBinding("binding-original", "catalog-original")
	facts := proofTestCurrentFacts(binding)
	evidence := rc.EvidenceFact{ID: "evidence-original", Kind: rc.EvidenceKindStateChange, Quality: common.EvidenceQualityValid,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, BindingRefs: []string{binding.ID}}
	ids := []common.CoverageWorkItemID{"cwi-1", " cwi-1 ", "other"}
	if !EvidenceAppliesToCoverageWorkItemWithLineage(facts, evidence, "cwi-1", nil) {
		t.Fatal("fixture must attribute ordinary evidence")
	}
	compareEvidenceLineageProjection(t, facts, evidence, ids, nil)
	for i := 0; i < reflect.TypeOf(facts).NumField(); i++ {
		changed := (rc.RuntimeStateRecord{Facts: facts}).Clone().Facts
		field := reflect.ValueOf(&changed).Elem().Field(i)
		field.Set(reflect.Zero(field.Type()))
		compareEvidenceLineageProjection(t, changed, evidence, ids, nil)
	}
	for _, mutate := range []func(*rc.RuntimeFacts){
		func(f *rc.RuntimeFacts) { f.Coverage[0].CurrentBindingRef.BindingHash = "stale" },
		func(f *rc.RuntimeFacts) { f.Coverage[0].CWIConvergenceState = rc.RuntimeCWIConvergenceStateUnbound },
		func(f *rc.RuntimeFacts) { f.Focus.CoverageEvidenceBindings[0].Hash = "raw hash drift" },
		func(f *rc.RuntimeFacts) { f.Focus.CoverageEvidenceBindings = nil },
		func(f *rc.RuntimeFacts) { f.Coverage = append(f.Coverage, f.Coverage[0]) },
		func(f *rc.RuntimeFacts) { f.CWILedgerSchemaVersion = " " + f.CWILedgerSchemaVersion + " " },
		func(f *rc.RuntimeFacts) { f.EvidenceRevision = " evidence/1 " },
		func(f *rc.RuntimeFacts) { f.WorkspaceID, f.RunID = "other-workspace", "other-run" },
	} {
		changed := (rc.RuntimeStateRecord{Facts: facts}).Clone().Facts
		mutate(&changed)
		compareEvidenceLineageProjection(t, changed, evidence, ids, nil)
	}
	body, _ := json.Marshal(facts)
	var loaded rc.RuntimeFacts
	if err := json.Unmarshal(body, &loaded); err != nil {
		t.Fatal(err)
	}
	compareEvidenceLineageProjection(t, loaded, evidence, ids, nil)
	compareEvidenceLineageProjection(t, facts, evidence, ids, nil)
}

func evidenceLineageArchive(t testing.TB) ([]byte, string, map[string]struct{ RuntimeState rc.RuntimeStateRecord }) {
	t.Helper()
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState rc.RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil || len(runs) == 0 {
		t.Fatalf("invalid archive: %v", err)
	}
	return body, path, runs
}

func TestEvidenceLineageProjectionReadOnlyArchive(t *testing.T) {
	body, path, runs := evidenceLineageArchive(t)
	for _, run := range runs {
		facts := run.RuntimeState.Facts
		focuses := append([]rc.CoverageFocus{facts.Focus}, facts.RemainingFocuses...)
		focuses = append(focuses, facts.CompletedFocuses...)
		for _, focus := range focuses {
			probe := facts
			probe.Focus = focus
			for _, e := range append(append([]rc.EvidenceFact(nil), facts.Evidence.Accepted...), facts.Evidence.Rejected...) {
				compareEvidenceLineageProjection(t, probe, e, focus.CoverageWorkItemIDs, run.RuntimeState.CoverageProofLineage)
			}
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("read-only proof replay changed archive")
	}
}

func BenchmarkEvidenceLineageProjectionPublicArchive(b *testing.B) {
	_, _, runs := evidenceLineageArchive(b)
	for _, run := range runs {
		r := run.RuntimeState
		if len(r.Facts.Evidence.Accepted) == 0 || len(r.Facts.Focus.CoverageWorkItemIDs) == 0 {
			b.Fatal("archive needs accepted evidence and current focus")
		}
		id := r.Facts.Focus.CoverageWorkItemIDs[0]
		e := r.Facts.Evidence.Accepted[0]
		b.Run("complete_normalization", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				referenceLineageBeforeProjectionEvidenceAppliesToCoverageWorkItemWithLineage(r.Facts, e, id, r.CoverageProofLineage)
			}
		})
		b.Run("required_projection", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				EvidenceAppliesToCoverageWorkItemWithLineage(r.Facts, e, id, r.CoverageProofLineage)
			}
		})
	}
}
