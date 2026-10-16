package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func conversionStoragePointers(value reflect.Value, pointers map[uintptr]bool) {
	if !value.IsValid() {
		return
	}
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if value.IsNil() {
			return
		}
		if value.Kind() == reflect.Pointer {
			if pointers[value.Pointer()] {
				return
			}
			pointers[value.Pointer()] = true
		}
		conversionStoragePointers(value.Elem(), pointers)
	case reflect.Slice:
		if value.Len() != 0 {
			pointers[value.Pointer()] = true
		}
		for i := 0; i < value.Len(); i++ {
			conversionStoragePointers(value.Index(i), pointers)
		}
	case reflect.Map:
		if !value.IsNil() {
			pointers[value.Pointer()] = true
		}
		iterator := value.MapRange()
		for iterator.Next() {
			conversionStoragePointers(iterator.Value(), pointers)
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			conversionStoragePointers(value.Field(i), pointers)
		}
	case reflect.Array:
		for i := 0; i < value.Len(); i++ {
			conversionStoragePointers(value.Index(i), pointers)
		}
	}
}

func assertConversionDetached(t testing.TB, source, output any) {
	t.Helper()
	original, copied := map[uintptr]bool{}, map[uintptr]bool{}
	conversionStoragePointers(reflect.ValueOf(source), original)
	conversionStoragePointers(reflect.ValueOf(output), copied)
	for pointer := range copied {
		if original[pointer] {
			t.Fatal("converted graph retains mutable source storage at", conversionSharedPath(reflect.ValueOf(output), pointer, "output"))
		}
	}
}

func conversionSharedPath(value reflect.Value, pointer uintptr, path string) string {
	if !value.IsValid() {
		return ""
	}
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			if value.Kind() == reflect.Pointer && value.Pointer() == pointer {
				return path
			}
			return conversionSharedPath(value.Elem(), pointer, path)
		}
	case reflect.Slice, reflect.Map:
		if !value.IsNil() && value.Pointer() == pointer {
			return path
		}
		if value.Kind() == reflect.Slice {
			for i := 0; i < value.Len(); i++ {
				if found := conversionSharedPath(value.Index(i), pointer, fmt.Sprintf("%s[%d]", path, i)); found != "" {
					return found
				}
			}
		} else {
			iterator := value.MapRange()
			for iterator.Next() {
				if found := conversionSharedPath(iterator.Value(), pointer, path+"[map-value]"); found != "" {
					return found
				}
			}
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if found := conversionSharedPath(value.Field(i), pointer, path+"."+value.Type().Field(i).Name); found != "" {
				return found
			}
		}
	case reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if found := conversionSharedPath(value.Index(i), pointer, fmt.Sprintf("%s[%d]", path, i)); found != "" {
				return found
			}
		}
	}
	return ""
}

func assertRuntimeConversionReference(t testing.TB, record runtimecontract.RuntimeStateRecord) {
	t.Helper()
	before, _ := json.Marshal(record)
	want := referenceRuntimeAggregateFromRecord(record.Clone())
	got := runtimeAggregateFromRecord(record)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("record→aggregate changed original projection at", conversionDifference(reflect.ValueOf(got), reflect.ValueOf(want), "aggregate"))
	}
	assertConversionDetached(t, record, got)
	expected := referenceRuntimeStateRecordFromAggregate(got.CloneRaw())
	aggregateBefore, _ := json.Marshal(got)
	saved := runtimeStateRecordFromAggregate(got)
	if !reflect.DeepEqual(saved, expected) {
		t.Fatal("aggregate→record changed original projection at", conversionDifference(reflect.ValueOf(saved), reflect.ValueOf(expected), "record"))
	}
	assertConversionDetached(t, got, saved)
	aggregateAfter, _ := json.Marshal(got)
	if string(aggregateBefore) != string(aggregateAfter) {
		t.Fatal("save conversion changed source aggregate bytes")
	}
	after, _ := json.Marshal(record)
	if string(before) != string(after) {
		t.Fatal("conversion changed original wire bytes")
	}
	if err, original := got.Validate(), want.Validate(); (err == nil) != (original == nil) || err != nil && err.Error() != original.Error() {
		t.Fatal("conversion changed full aggregate rejection", err, original)
	}
}

func TestRuntimeStateConversionReusesDetachedGraphsWithoutChangingOriginalProjection(t *testing.T) {
	for _, scenario := range []string{"zero", "valid", "populated", "reload", "empty", "malformed-raw"} {
		t.Run(scenario, func(t *testing.T) {
			var record runtimecontract.RuntimeStateRecord
			switch scenario {
			case "valid":
				record = referenceRuntimeStateRecordFromAggregate(runtimeAggregate(t, 9))
			case "populated", "reload", "malformed-raw":
				sequence := 0
				populateConversionRawFixture(reflect.ValueOf(&record).Elem(), map[reflect.Type]int{}, &sequence)
				if scenario == "reload" {
					body, err := json.Marshal(record)
					if err != nil || json.Unmarshal(body, &record) != nil {
						t.Fatal("raw reload failed", err)
					}
				}
				if scenario == "malformed-raw" {
					record.Facts.DefinitionRevision = " raw\x00\xff "
					record.Definition.CapabilityContract = []string{" duplicate ", " duplicate "}
				}
			case "empty":
				record.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{}
				record.CompletedDispatches = []runtimecontract.RuntimeDispatchRecord{}
				record.Facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{}
				record.Definition.CapabilityContract = []string{}
			}
			assertRuntimeConversionReference(t, record)
		})
	}
}

func TestRuntimeStateConversionExplicitArchiveEquivalence(t *testing.T) {
	path := os.Getenv("WORKORCH_RUNTIME_STATE_CONVERSION_ARCHIVE")
	if path == "" {
		t.Skip("explicit read-only Run archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct {
		RuntimeState runtimecontract.RuntimeStateRecord
	}
	if err := json.Unmarshal(body, &runs); err != nil || len(runs) == 0 {
		t.Fatal("invalid archive", err)
	}
	for _, run := range runs {
		assertRuntimeConversionReference(t, run.RuntimeState)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(body) != string(after) {
		t.Fatal("read-only equivalence changed archive", err)
	}
}

func FuzzRuntimeStateConversionMatchesOriginalProjection(f *testing.F) {
	f.Add(" raw ", uint8(0))
	f.Add("\x00bytes\xff", uint8(1))
	f.Fuzz(func(t *testing.T, value string, mode uint8) {
		if len(value) > 4096 {
			t.Skip()
		}
		record := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{DefinitionRevision: value}}
		if mode%2 == 0 {
			record.Definition.CapabilityContract = []string{value, value}
		} else {
			record.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{LogicalRequestID: value}}
		}
		assertRuntimeConversionReference(t, record)
	})
}

func BenchmarkRuntimeStateConversionDetachedCopy(b *testing.B) {
	var record runtimecontract.RuntimeStateRecord
	sequence := 0
	populateConversionRawFixture(reflect.ValueOf(&record).Elem(), map[reflect.Type]int{}, &sequence)
	aggregate := referenceRuntimeAggregateFromRecord(record.Clone())
	for _, direction := range []string{"save", "load"} {
		for _, original := range []bool{true, false} {
			b.Run(fmt.Sprintf("%s/original=%t", direction, original), func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if direction == "save" {
						if original {
							_ = referenceRuntimeStateRecordFromAggregate(aggregate)
						} else {
							_ = runtimeStateRecordFromAggregate(aggregate)
						}
					} else if original {
						_ = referenceRuntimeAggregateFromRecord(record.Clone())
					} else {
						_ = runtimeAggregateFromRecord(record)
					}
				}
			})
		}
	}
}

func conversionDifference(got, want reflect.Value, path string) string {
	if reflect.DeepEqual(got.Interface(), want.Interface()) {
		return ""
	}
	switch got.Kind() {
	case reflect.Struct:
		for i := 0; i < got.NumField(); i++ {
			if diff := conversionDifference(got.Field(i), want.Field(i), path+"."+got.Type().Field(i).Name); diff != "" {
				return diff
			}
		}
	case reflect.Pointer:
		if !got.IsNil() && !want.IsNil() {
			return conversionDifference(got.Elem(), want.Elem(), path)
		}
	case reflect.Slice:
		if got.IsNil() == want.IsNil() && got.Len() == want.Len() {
			for i := 0; i < got.Len(); i++ {
				if diff := conversionDifference(got.Index(i), want.Index(i), fmt.Sprintf("%s[%d]", path, i)); diff != "" {
					return diff
				}
			}
		}
	}
	return path
}
