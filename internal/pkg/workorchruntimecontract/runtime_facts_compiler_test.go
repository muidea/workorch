package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func assertRuntimeFactsCompiler(t testing.TB, f RuntimeFacts, s *runtimeSemanticValidationSession) {
	t.Helper()
	before, _ := json.Marshal(f)
	want := referenceRuntimeFactsBeforeOwnerCompilation(f)
	for range 2 {
		got := f.validateInSemanticSession(s)
		// The unchanged recovery-policy validator ranges over a map. A
		// completely zero policy has multiple invalid fields and no defined
		// first member. Compare its existing rejection class, not map order.
		policyFailure := got != nil && want != nil &&
			strings.HasPrefix(got.Error(), "runtime facts recovery policy:") &&
			strings.HasPrefix(want.Error(), "runtime facts recovery policy:")
		if fmt.Sprint(got) != fmt.Sprint(want) && !policyFailure {
			t.Fatalf("Facts first-error changed: %v != %v", got, want)
		}
	}
	after, _ := json.Marshal(f)
	if string(before) != string(after) {
		t.Fatal("Facts validation changed raw input")
	}
}

func TestFactsCatalogCompilerPreservesCollectionRulesAndRawDrift(t *testing.T) {
	a := runtimeOwnerCatalogSurfaceReuseFixture(false)
	b := cloneRuntimeOperationAdmissibilityRaw(a)
	b.PrerequisiteID = "second-prerequisite-same-owner"
	b.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(b)
	s := newRuntimeSemanticValidationSession()
	for _, catalogs := range [][]RuntimeOperationAdmissibility{nil, {}, {a}, {b, a}, {a, a}} {
		for range 2 {
			got, err := compileRuntimeFactsCatalogsInSession(catalogs, s)
			wantErr := referenceRuntimeFactsCatalogCollectionBeforeCompilation(catalogs)
			if fmt.Sprint(err) != fmt.Sprint(wantErr) || err == nil && !reflect.DeepEqual(got, NormalizeRuntimeOperationAdmissibilities(catalogs)) {
				t.Fatal("Facts collection became a semantic-owner collection or changed first error", err, wantErr)
			}
		}
	}
	for _, mutate := range []func(*RuntimeOperationAdmissibility){
		func(v *RuntimeOperationAdmissibility) { v.SnapshotID = "same body, wrong identity" },
		func(v *RuntimeOperationAdmissibility) { v.ValidationSurfaces[0].CheckRefs = nil },
		func(v *RuntimeOperationAdmissibility) {
			v.ValidationSurfaces = append(v.ValidationSurfaces, v.ValidationSurfaces[0])
		},
		func(v *RuntimeOperationAdmissibility) { v.ObservedRevision = "another current revision" },
	} {
		changed := cloneRuntimeOperationAdmissibilityRaw(a)
		mutate(&changed)
		_, got := compileRuntimeFactsCatalogsInSession([]RuntimeOperationAdmissibility{changed}, s)
		want := referenceRuntimeFactsCatalogCollectionBeforeCompilation([]RuntimeOperationAdmissibility{changed})
		if got == nil || fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatal("raw drift borrowed prior catalog success", got, want)
		}
	}
	// A subsequent raw caller mutation must not change a retained source.
	a.ValidationSurfaces[0].CheckRefs[0] = "caller changed input"
	for _, entries := range s.catalogs {
		for _, e := range entries {
			if reflect.DeepEqual(e.raw, a) {
				t.Fatal("retained catalog aliases caller")
			}
		}
	}
}

func TestFactsCompilerFullArchiveAndEveryCurrentField(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		base := run.RuntimeState.Facts
		if err := referenceRuntimeFactsBeforeOwnerCompilation(base); err != nil {
			t.Fatal("fixed source invalid before optimization", err)
		}
		s := newRuntimeSemanticValidationSession()
		assertRuntimeFactsCompiler(t, base, s)
		for i := 0; i < reflect.TypeOf(base).NumField(); i++ {
			changed := run.RuntimeState.Clone().Facts
			field := reflect.ValueOf(&changed).Elem().Field(i)
			field.Set(reflect.Zero(field.Type()))
			assertRuntimeFactsCompiler(t, changed, s)
		}
		var reloaded RuntimeFacts
		raw, _ := json.Marshal(base)
		if err := json.Unmarshal(raw, &reloaded); err != nil {
			t.Fatal(err)
		}
		assertRuntimeFactsCompiler(t, reloaded, newRuntimeSemanticValidationSession())
		// Independent owner calls/cold reloads get no Facts admission result.
		if err := base.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func BenchmarkFactsCompilerFullArchive(b *testing.B) {
	_, _, runs := runtimeStateConsumerArchive(b)
	for _, run := range runs {
		for _, warm := range []bool{false, true} {
			b.Run(fmt.Sprint(warm), func(b *testing.B) {
				s := newRuntimeSemanticValidationSession()
				if err := run.RuntimeState.Facts.validateInSemanticSession(s); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				for b.Loop() {
					var err error
					if warm {
						err = run.RuntimeState.Facts.validateInSemanticSession(s)
					} else {
						err = referenceRuntimeFactsBeforeOwnerCompilation(run.RuntimeState.Facts)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func TestFormalCatalogConsumptionStillAdmitsChangedInputs(t *testing.T) {
	original := runtimeOwnerCatalogSurfaceReuseFixture(false)
	if err := original.Validate(); err != nil {
		t.Fatal(err)
	}
	previous := RuntimeStateRecord{Facts: RuntimeFacts{Resources: RuntimeResourceFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{original}}}}
	session := newRuntimeSemanticValidationSession()
	session.committedPreimage = &previous
	got, err := compileRuntimeFactsCatalogsInSession([]RuntimeOperationAdmissibility{original}, session)
	if err != nil || !reflect.DeepEqual(got, NormalizeRuntimeOperationAdmissibilities([]RuntimeOperationAdmissibility{original})) {
		t.Fatal("formal catalog projection changed", err)
	}
	if len(session.catalogs) != 0 {
		t.Fatal("formal result created a second admission cache")
	}
	for _, arm := range []string{"body", "rehashed", "duplicate", "scope"} {
		t.Run(arm, func(t *testing.T) {
			changed := cloneRuntimeOperationAdmissibilityRaw(original)
			values := []RuntimeOperationAdmissibility{changed}
			switch arm {
			case "body", "rehashed":
				changed.ValidationSurfaces[0].CheckRefs = nil
				if arm == "rehashed" {
					changed.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(changed)
				}
				values[0] = changed
			case "duplicate":
				values = append(values, changed)
			case "scope":
				values[0].OwnerModuleID = "foreign"
			}
			if _, err := compileRuntimeFactsCatalogsInSession(values, session); err == nil {
				t.Fatal("changed input consumed original result")
			}
		})
	}
	// Consumer output never aliases either original catalog or frozen source.
	got[0].ValidationSurfaces[0].CheckRefs[0] = "caller"
	if !reflect.DeepEqual(original, previous.Facts.Resources.OperationAdmissibilities[0]) {
		t.Fatal("consumer mutated formal source")
	}
}
