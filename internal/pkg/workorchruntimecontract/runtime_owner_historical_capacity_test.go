package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func runtimeOwnerRetainedCount(s *runtimeSemanticValidationSession) int {
	n := len(s.seeds) + len(s.workDefinitions) + len(s.workScopes) + len(s.workRounds) + len(s.definitionSources)
	if s.ownerHistory != nil {
		n += len(s.ownerHistory.generations) + len(s.ownerHistory.purposes) + s.ownerHistory.lineage.count()
	}
	for _, entries := range s.catalogs {
		n += len(entries)
	}
	for _, entries := range s.manifests {
		n += len(entries)
		for _, entry := range entries {
			if entry.validated.documents != nil {
				n += len(entry.validated.documents.values)
			}
		}
	}
	return n
}

func TestOwnerHistoricalCompilerBoundedRetentionIsDeterministic(t *testing.T) {
	s := newRuntimeSemanticValidationSession()
	for _, key := range []string{"last", "first"} {
		s.catalogs[key] = make([]runtimeSemanticCatalogValidationEntry, 90)
	}
	a, b := retainRuntimeOwnerHistoricalSubtrees(s), retainRuntimeOwnerHistoricalSubtrees(s)
	if !reflect.DeepEqual(a, b) || runtimeOwnerRetainedCount(a) != runtimeOwnerHistoricalEntryLimit || len(a.catalogs["first"]) != 90 || len(a.catalogs["last"]) != 38 {
		t.Fatal("map insertion/order affected the complete retained subset")
	}
	if len(s.catalogs["last"]) != 90 {
		t.Fatal("eviction changed original raw inputs")
	}
}

func TestOwnerHistoricalCompilerReadOnlyArchiveCapacity(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		s := newRuntimeSemanticValidationSession()
		s.ownerHistory = &runtimeOwnerHistoricalSemanticCompiler{}
		if err := compileRuntimeOwnerStateTransaction(func(gate func(RuntimeOwnerStateSourceImage) error) error {
			return gate(historicalCompilerImage(run.RuntimeState))
		}, s, nil); err != nil {
			t.Fatal(err)
		}
		t.Logf("scopes=%d definitions=%d seeds=%d rounds=%d sources=%d generations=%d purposes=%d lineage=%d", len(s.workScopes), len(s.workDefinitions), len(s.seeds), len(s.workRounds), len(s.definitionSources), len(s.ownerHistory.generations), len(s.ownerHistory.purposes), s.ownerHistory.lineage.count())
		catalogs, manifests, documents := 0, 0, 0
		for _, entries := range s.catalogs {
			catalogs += len(entries)
		}
		for _, entries := range s.manifests {
			manifests += len(entries)
			for _, entry := range entries {
				if entry.validated.documents != nil {
					documents += len(entry.validated.documents.values)
				}
			}
		}
		t.Logf("catalogs=%d manifests=%d documents=%d", catalogs, manifests, documents)
		kept := retainRuntimeOwnerHistoricalSubtrees(s)
		if count := runtimeOwnerRetainedCount(kept); count > runtimeOwnerHistoricalEntryLimit || count == 0 || len(kept.workStates) != 0 {
			t.Fatal("complete archive violated bounded pure retention", count)
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func BenchmarkOwnerHistoricalCompilerPublicArchive(b *testing.B) {
	_, _, runs := runtimeStateConsumerArchive(b)
	for _, run := range runs {
		for _, warm := range []bool{false, true} {
			name := "cold"
			if warm {
				name = "retained"
			}
			b.Run(name, func(b *testing.B) {
				c := &RuntimeOwnerHistoricalCompiler{}
				if err := historicalCompilerRecord(c, run.RuntimeState); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if !warm {
						c = &RuntimeOwnerHistoricalCompiler{}
					}
					if err := historicalCompilerRecord(c, run.RuntimeState); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
