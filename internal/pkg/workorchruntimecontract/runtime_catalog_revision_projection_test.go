package workorchruntimecontract

import (
	"encoding/hex"
	"reflect"
	"slices"
	"testing"
)

// Frozen identity encoder: complete normalization followed by the original fields.
func referenceCatalogRevisionHashBeforeProjection(values []RuntimeOperationAdmissibility) string {
	values = NormalizeRuntimeOperationAdmissibilities(values)
	h := newRuntimeHashWriter()
	for _, inventory := range values {
		writeHashString(h, inventory.OwnerModuleID)
		writeHashString(h, inventory.SnapshotID)
		writeHashString(h, inventory.ObservedRevision)
	}
	return "catalog-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func assertCatalogRevisionProjection(t testing.TB, values []RuntimeOperationAdmissibility) {
	t.Helper()
	before := cloneRuntimeStateRawSlice(values, cloneRuntimeOperationAdmissibilityRaw)
	if got, want := StableRuntimeOperationAdmissibilityCatalogRevisionHash(values), referenceCatalogRevisionHashBeforeProjection(values); got != want {
		t.Fatalf("catalog identity encoding changed: %s != %s", got, want)
	}
	if !reflect.DeepEqual(values, before) {
		t.Fatal("identity hash changed raw catalog")
	}
}

func TestCatalogRevisionProjectionPreservesAllRawFieldsOrderAndDuplicates(t *testing.T) {
	original := runtimeOwnerCatalogSurfaceReuseFixture(true)
	for i := 0; i < reflect.TypeOf(original).NumField(); i++ {
		value := cloneRuntimeOperationAdmissibilityRaw(original)
		field := reflect.ValueOf(&value).Elem().Field(i)
		field.Set(reflect.Zero(field.Type()))
		assertCatalogRevisionProjection(t, []RuntimeOperationAdmissibility{value})
	}
	for _, values := range [][]RuntimeOperationAdmissibility{nil, {}, {original}, {original, original}} {
		assertCatalogRevisionProjection(t, values)
	}
	other := cloneRuntimeOperationAdmissibilityRaw(original)
	other.SnapshotID, other.ObservedRevision = "another snapshot", "another revision"
	for _, key := range []string{"catalog", " catalog ", "", " ", "other", string([]byte{0xff, 0x00})} {
		other.PrerequisiteID = key
		other.OwnerModuleID = " " + original.OwnerModuleID + " "
		values := []RuntimeOperationAdmissibility{original, other}
		assertCatalogRevisionProjection(t, values)
		slices.Reverse(values)
		assertCatalogRevisionProjection(t, values)
	}
}

func TestCatalogRevisionProjectionReadOnlyArchive(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		r := run.RuntimeState
		assertCatalogRevisionProjection(t, r.Facts.Resources.OperationAdmissibilities)
		dispatches := append([]RuntimeDispatchRecord(nil), r.CompletedDispatches...)
		if r.ActiveDispatch != nil {
			dispatches = append(dispatches, *r.ActiveDispatch)
		}
		for _, dispatch := range dispatches {
			assertCatalogRevisionProjection(t, dispatch.OperationAdmissibilities)
			if dispatch.AssessmentAuthorityInput != nil {
				assertCatalogRevisionProjection(t, dispatch.AssessmentAuthorityInput.OperationAdmissibilities)
			}
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func BenchmarkCatalogRevisionProjection(b *testing.B) {
	values := []RuntimeOperationAdmissibility{runtimeOwnerCatalogSurfaceReuseFixture(true)}
	for _, original := range []bool{true, false} {
		name := "projection"
		if original {
			name = "complete_normalization"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if original {
					referenceCatalogRevisionHashBeforeProjection(values)
				} else {
					StableRuntimeOperationAdmissibilityCatalogRevisionHash(values)
				}
			}
		})
	}
}
