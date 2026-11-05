package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"testing"
)

func compareApplicabilityScan(t testing.TB, fact EvidenceFact, observations []RuntimeEvidenceApplicabilityObservation, cutoffs []string) {
	t.Helper()
	before := cloneEvidenceFactRaw(fact)
	beforeObservations := CloneRuntimeEvidenceApplicabilityObservations(observations)
	for _, cutoff := range cutoffs {
		got := []bool{
			RuntimeEvidenceHasStaleApplicability(fact, observations, cutoff),
			RuntimeEvidenceHasObservedCurrentApplicability(fact, observations, cutoff),
			RuntimeEvidenceHasObservedApplicabilityAtRevision(fact, observations, cutoff),
		}
		want := []bool{
			referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasStaleApplicability(fact, observations, cutoff),
			referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasObservedCurrentApplicability(fact, observations, cutoff),
			referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasObservedApplicabilityAtRevision(fact, observations, cutoff),
		}
		for _, fence := range []string{"", "generation-a", "generation-b"} {
			got = append(got, RuntimeEvidenceHasFreshApplicability(fact, observations, cutoff, fence))
			want = append(want, referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasFreshApplicability(fact, observations, cutoff, fence))
		}
		if !slices.Equal(got, want) {
			t.Fatalf("cutoff %q: predicates %v != original %v", cutoff, got, want)
		}
	}
	if !reflect.DeepEqual(before, fact) || !reflect.DeepEqual(beforeObservations, observations) {
		t.Fatal("query changed original input")
	}
}

func applicabilityScanFixture() (EvidenceFact, RuntimeEvidenceApplicabilityObservation) {
	fact := historicalEffectPurposeFixture()
	fact.ApplicabilityDependencies.PurposeRef = ""
	fact.ApplicabilityDependencies.OwnerModuleID = ""
	fact.ApplicabilityDependencies.OperationKind = ""
	fact.ApplicabilityDependencies.OwnerContractRef = ""
	observation := RuntimeEvidenceApplicabilityObservation{
		Request:             RuntimeEvidenceApplicabilityRequest{SourceEvidenceID: fact.ID, SourceEvidenceHash: StableRuntimeEvidenceFactHash(fact), EvidenceRevision: "2", ReconciliationRequestHash: "generation-a"},
		CurrentDependencies: CloneRuntimeOwnerRevisionSet(&fact.ApplicabilityDependencies.OwnerRevisionSet),
	}
	return fact, observation
}

func TestEvidenceApplicabilitySingleScanIndependentOracle(t *testing.T) {
	fact, observation := applicabilityScanFixture()
	for _, name := range []string{"current", "nil", "empty", "duplicate", "missing", "nil-dependencies", "changed-revision", "wrong-hash", "wrong-id", "future-stale", "past-stale", "another-generation", "invalid-set", "other-definition", "raw-whitespace"} {
		t.Run(name, func(t *testing.T) {
			values := []RuntimeEvidenceApplicabilityObservation{observation.CloneRaw()}
			switch name {
			case "nil":
				values = nil
			case "empty":
				values = []RuntimeEvidenceApplicabilityObservation{}
			case "duplicate":
				values = append(values, observation.CloneRaw())
			case "missing":
				values[0].Missing, values[0].CurrentDependencies = true, nil
			case "nil-dependencies":
				values[0].CurrentDependencies = nil
			case "changed-revision":
				values[0].CurrentDependencies.Entries[0].Revision += "-changed"
				*values[0].CurrentDependencies = values[0].CurrentDependencies.WithHash()
			case "wrong-hash":
				values[0].Request.SourceEvidenceHash = "different-source"
			case "wrong-id":
				values[0].Request.SourceEvidenceID = "another-workspace-evidence"
			case "future-stale", "past-stale":
				stale := observation.CloneRaw()
				stale.Missing, stale.CurrentDependencies = true, nil
				stale.Request.EvidenceRevision = "3"
				if name == "past-stale" {
					stale.Request.EvidenceRevision = "1"
				}
				values = append(values, stale)
			case "another-generation":
				values[0].Request.ReconciliationRequestHash = "generation-b"
			case "invalid-set":
				values[0].CurrentDependencies.SetHash = "invalid"
			case "other-definition":
				values[0].CurrentDependencies.DefinitionRevision = "other-definition"
				*values[0].CurrentDependencies = values[0].CurrentDependencies.WithHash()
			case "raw-whitespace":
				values[0].CurrentDependencies.Entries[0].Revision += " "
			}
			for _, f := range []EvidenceFact{fact, {ID: fact.ID}, cloneEvidenceFactRaw(fact)} {
				compareApplicabilityScan(t, f, values, []string{"", "1", "2", "3"})
				slices.Reverse(values)
				compareApplicabilityScan(t, f, values, []string{"", "1", "2", "3"})
			}
			// JSON reload and repeat are ordinary cold queries, not a retained
			// PASS result. Same evidence ID with changed raw bytes cannot hit.
			body, _ := json.Marshal(values)
			var loaded []RuntimeEvidenceApplicabilityObservation
			if err := json.Unmarshal(body, &loaded); err != nil {
				t.Fatal(err)
			}
			compareApplicabilityScan(t, fact, loaded, []string{"", "2"})
			changed := cloneEvidenceFactRaw(fact)
			changed.SourceRef = "same-id raw drift"
			compareApplicabilityScan(t, changed, loaded, []string{"", "2"})
			compareApplicabilityScan(t, fact, loaded, []string{"", "2"})
		})
	}
}

func TestEvidenceApplicabilitySingleScanPreservesStaleAndFreshFences(t *testing.T) {
	fact, current := applicabilityScanFixture()
	stale := current.CloneRaw()
	stale.Request.EvidenceRevision = "1"
	stale.CurrentDependencies = nil
	stale.Missing = true
	values := []RuntimeEvidenceApplicabilityObservation{current, stale, current}
	if !RuntimeEvidenceHasStaleApplicability(fact, values, "2") || RuntimeEvidenceHasObservedCurrentApplicability(fact, values, "2") ||
		RuntimeEvidenceHasObservedApplicabilityAtRevision(fact, values, "2") || RuntimeEvidenceHasFreshApplicability(fact, values, "2", "generation-a") {
		t.Fatal("byte restoration or repeated query revived stale evidence")
	}
	values = []RuntimeEvidenceApplicabilityObservation{current}
	if !RuntimeEvidenceHasFreshApplicability(fact, values, "2", "generation-a") || !RuntimeEvidenceHasObservedApplicabilityAtRevision(fact, values, "2") ||
		RuntimeEvidenceHasFreshApplicability(fact, values, "2", "generation-b") || RuntimeEvidenceHasFreshApplicability(fact, values, "3", "generation-a") ||
		RuntimeEvidenceHasObservedApplicabilityAtRevision(fact, values, "") {
		t.Fatal("current query lost exact revision/reconciliation fence")
	}
}

func applicabilityScanArchive(t testing.TB) ([]byte, string, map[string]struct{ RuntimeState RuntimeStateRecord }) {
	t.Helper()
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil || len(runs) == 0 {
		t.Fatalf("invalid archive: %v", err)
	}
	return body, path, runs
}

func TestEvidenceApplicabilitySingleScanReadOnlyArchive(t *testing.T) {
	body, path, runs := applicabilityScanArchive(t)
	for _, run := range runs {
		facts := run.RuntimeState.Facts
		all := append(append([]EvidenceFact(nil), facts.Evidence.Accepted...), facts.Evidence.Rejected...)
		for _, fact := range all {
			compareApplicabilityScan(t, fact, facts.EvidenceApplicabilityObservations, []string{"", fact.Revision, facts.EvidenceRevision})
			changed := cloneEvidenceFactRaw(fact)
			changed.SourceRef += "same-id drift"
			compareApplicabilityScan(t, changed, facts.EvidenceApplicabilityObservations, []string{facts.EvidenceRevision})
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("read-only replay changed archive")
	}
}

func BenchmarkEvidenceApplicabilitySingleScan(b *testing.B) {
	fact, observation := applicabilityScanFixture()
	for _, size := range []int{1, 32, 512} {
		observations := make([]RuntimeEvidenceApplicabilityObservation, size)
		for i := range observations {
			observations[i] = observation.CloneRaw()
		}
		for _, original := range []bool{true, false} {
			b.Run(fmt.Sprintf("observations_%d/original_%t", size, original), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var got bool
					if original {
						got = referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasObservedApplicabilityAtRevision(fact, observations, "2")
					} else {
						got = RuntimeEvidenceHasObservedApplicabilityAtRevision(fact, observations, "2")
					}
					if !got {
						b.Fatal("current fixture rejected")
					}
				}
			})
		}
	}
}

func TestEvidenceApplicabilityRevisionWithoutConsumerDoesNotExpandFact(t *testing.T) {
	fact, observation := applicabilityScanFixture()
	values := []RuntimeEvidenceApplicabilityObservation{observation}
	allocations := testing.AllocsPerRun(20, func() {
		if RuntimeEvidenceHasObservedApplicabilityAtRevision(fact, values, "3") {
			t.Fatal("earlier observation supplied exact-revision proof")
		}
	})
	if allocations != 0 {
		t.Fatalf("no exact-revision consumer still expanded unrelated original fact: %v allocations", allocations)
	}
}

func FuzzEvidenceApplicabilitySingleScanOracle(f *testing.F) {
	for selector := uint8(0); selector < 8; selector++ {
		f.Add(selector, "source 原文\r\n", "2", "generation-a")
	}
	f.Fuzz(func(t *testing.T, selector uint8, raw, cutoff, generation string) {
		fact, observation := applicabilityScanFixture()
		values := []RuntimeEvidenceApplicabilityObservation{observation.CloneRaw(), observation.CloneRaw()}
		switch selector % 8 {
		case 0:
			fact.SourceRef = raw
		case 1:
			values[0].Request.SourceEvidenceHash = raw
		case 2:
			values[0].Request.EvidenceRevision = raw
		case 3:
			values[0].Request.ReconciliationRequestHash = raw
		case 4:
			values[0].CurrentDependencies.Entries[0].Revision = raw
			*values[0].CurrentDependencies = values[0].CurrentDependencies.WithHash()
		case 5:
			values[0].CurrentDependencies = nil
		case 6:
			values[0].CurrentDependencies.SetHash = raw
		case 7:
			fact.ApplicabilityDependencies = nil
		}
		compareApplicabilityScan(t, fact, values, []string{cutoff})
		if RuntimeEvidenceHasFreshApplicability(fact, values, cutoff, generation) != referenceApplicabilityBeforeSingleScanRuntimeEvidenceHasFreshApplicability(fact, values, cutoff, generation) {
			t.Fatal("arbitrary source/fence changed fresh proof")
		}
	})
}
