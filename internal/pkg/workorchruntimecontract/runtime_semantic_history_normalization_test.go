package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func newHistoryNormalizationSession() *runtimeSemanticValidationSession {
	s := newRuntimeSemanticValidationSession()
	return s
}

func compareHistoryNormalization(t testing.TB, raw RuntimeSemanticPlanningLedger, s *runtimeSemanticValidationSession) {
	t.Helper()
	before := cloneRuntimeSemanticPlanningLedgerRaw(raw)
	left, right := cloneRuntimeSemanticPlanningLedgerRaw(raw), cloneRuntimeSemanticPlanningLedgerRaw(raw)
	got := validateRuntimeSemanticPlanningLedgerInSession(left, s)
	want := validateRuntimeSemanticPlanningLedgerNormalized(right, referenceSemanticLedgerNormalization(right), newRuntimeSemanticValidationSession())
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("normalization changed complete result/first error: %v != %v", got, want)
	}
	if !reflect.DeepEqual(raw, before) {
		t.Fatal("validator changed original input")
	}
}

func TestSemanticHistoryNormalizationMatchesColdRawAndCurrentGates(t *testing.T) {
	base := assessmentPreimageSessionFixture(t)
	for _, arm := range []string{"valid", "body", "rehashed_body", "retirement", "duplicate", "order", "nested_empty", "nil_empty", "seed", "current", "supplied_normalized"} {
		t.Run(arm, func(t *testing.T) {
			s := newHistoryNormalizationSession()
			if err := validateRuntimeSemanticPlanningLedgerInSession(cloneRuntimeSemanticPlanningLedgerRaw(base), s); err != nil {
				t.Fatal(err)
			}
			retained := cloneRuntimeSemanticPlanningGenerationRaw(base.History[0].Normalized())
			changed := cloneRuntimeSemanticPlanningLedgerRaw(base)
			switch arm {
			case "body", "rehashed_body":
				m := changed.History[0].Snapshot.Manifest
				m.OwnerCatalogs[0].ObservedRevision = "same-hash changed original body"
				if arm == "rehashed_body" {
					m.Hash = StableRuntimeSemanticPlanManifestHash(*m)
				}
			case "retirement":
				changed.History[0].RetiredAtStateRevision = 0
			case "duplicate":
				changed.History = append(changed.History, changed.History[0])
			case "order":
				changed.History[0], changed.History[1] = changed.History[1], changed.History[0]
			case "nested_empty":
				changed.History[0].Snapshot.History = []RuntimeSemanticPlanningGeneration{}
			case "nil_empty":
				changed.History[0].ReopenedCoverageIDs = []common.CoverageWorkItemID{}
			case "seed":
				changed.DefinitionSeed.PolicyHash = "current source drift"
			case "current":
				changed.SchemaVersion = "invalid"
			case "supplied_normalized":
				raw := changed.History[0]
				normalized := raw.Normalized()
				normalized.RetiredAtStateRevision = 0
				got := validateRuntimeSemanticPlanningGenerationNormalized(raw, normalized, s)
				want := validateRuntimeSemanticPlanningGenerationComplete(raw, normalized, newRuntimeSemanticValidationSession())
				if got == nil || fmt.Sprint(got) != fmt.Sprint(want) {
					t.Fatal("raw hit hid a bad caller-supplied normalized value", got, want)
				}
			}
			compareHistoryNormalization(t, changed, s)
			compareHistoryNormalization(t, changed, s)
			wire, _ := json.Marshal(changed)
			var reloaded RuntimeSemanticPlanningLedger
			if err := json.Unmarshal(wire, &reloaded); err != nil {
				t.Fatal(err)
			}
			compareHistoryNormalization(t, reloaded, newHistoryNormalizationSession())
			public := base.Normalized()
			public.History[0].Snapshot.Manifest.OwnerCatalogs[0].ObservedRevision = "public caller mutation"
			if reflect.DeepEqual(public.History[0], retained) {
				t.Fatal("public normalization exposed private storage")
			}
		})
	}
}

func TestSemanticHistoryNormalizationSegmentsAndIndependentNormalizedSource(t *testing.T) {
	base := semanticSegmentedHistoryFixture(t, 33)
	s := newHistoryNormalizationSession()
	compareHistoryNormalization(t, base, s)
	compareHistoryNormalization(t, base, s)
	base.HistoryArchive.Segments[0][0].Snapshot.Manifest.OwnerCatalogs[0].SnapshotID = "same hash changed archive"
	compareHistoryNormalization(t, base, s)

	// Complete validation of a supplied pair is not a certificate that the
	// second member is the normalization of the first member.
	g := assessmentPreimageSessionFixture(t).History[0]
	other := g.Normalized()
	other.RetiredAtStateRevision++
	s = newHistoryNormalizationSession()
	if err := validateRuntimeSemanticPlanningGenerationNormalized(g, other, s); err != nil {
		t.Fatal(err)
	}
	if got := normalizeRuntimeSemanticGenerationForValidation(g, s); !reflect.DeepEqual(got, g.Normalized()) {
		t.Fatal("supplied normalized value replaced canonical normalization")
	}
}

func TestSemanticHistoryNormalizationReadOnlyArchive(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		s := newHistoryNormalizationSession()
		compareHistoryNormalization(t, run.RuntimeState.SemanticPlanning, s)
		compareHistoryNormalization(t, run.RuntimeState.SemanticPlanning, s)
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func BenchmarkSemanticHistoryNormalization(b *testing.B) {
	for _, count := range []int{1, 8, 0} {
		// Explicit archive only; zero selects its complete original history.
		name := fmt.Sprint(count)
		if count == 0 {
			name = "complete"
		}
		b.Run(name, func(b *testing.B) {
			_, _, runs := runtimeStateConsumerArchive(b)
			for _, run := range runs {
				base := run.RuntimeState.SemanticPlanning.WithoutHistory()
				for _, g := range run.RuntimeState.SemanticPlanning.AllHistory() {
					base.History = append(base.History, g)
					if count > 0 && len(base.History) == count {
						break
					}
				}
				// Benchmark only the exact normalization consumer. Full ledger
				// storage/lineage validation is covered independently above.
				b.Logf("complete generations=%d", len(base.History))
				s := newHistoryNormalizationSession()
				for _, g := range base.History {
					if err := validateRuntimeSemanticPlanningGenerationNormalized(g, g.Normalized(), s); err != nil {
						b.Fatal(err)
					}
				}
				for _, name := range []string{"original", "owner_private"} {
					b.Run(name, func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							if name == "original" {
								_ = referenceSemanticLedgerNormalization(base)
							} else {
								_ = base.normalizedForValidation(s)
							}
						}
					})
				}
			}
		})
	}
}
