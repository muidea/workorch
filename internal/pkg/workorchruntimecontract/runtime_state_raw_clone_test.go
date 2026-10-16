package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestRuntimeStateRecordCloneCompletePopulatedGraph(t *testing.T) {
	var original RuntimeStateRecord
	sequence := 0
	populateRuntimeStateRawCloneFixture(reflect.ValueOf(&original).Elem(), map[reflect.Type]int{}, &sequence)
	cloned := original.Clone()
	if !reflect.DeepEqual(original, cloned) {
		t.Fatal("complete populated raw graph changed")
	}
	assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(original), reflect.ValueOf(cloned), "complete RuntimeStateRecord")
}

func TestRuntimeStateRecordCloneGovernanceAuthorizationRawShape(t *testing.T) {
	for _, fields := range [][]workorcheventpayload.Field{nil, {}, {
		{Name: " raw field \x00", Value: workorcheventpayload.Value{
			Kind:   workorcheventpayload.ValueArray,
			Object: []workorcheventpayload.Field{},
			Array: []workorcheventpayload.Value{{Kind: workorcheventpayload.ValueObject,
				Object: []workorcheventpayload.Field{{Name: "nested", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueString, String: " original \x00", Array: []workorcheventpayload.Value{}}}},
			}},
		}},
	}} {
		payload := workorcheventpayload.Payload{Fields: fields}
		original := RuntimeStateRecord{OperationReceipts: []RuntimeOperationReceipt{{Governance: &RuntimeGovernanceRequirement{AuthorizationContext: payload}}}}
		original.Facts.Failure = &FailureFact{GovernanceDependency: &ExternalDependency{AuthorizationContext: payload}}
		cloned := original.Clone()
		if !reflect.DeepEqual(original, cloned) {
			t.Fatal("raw authorization nil/empty, ordering or original bytes changed")
		}
		assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(original), reflect.ValueOf(cloned), "governance authorization")
		if len(fields) > 0 {
			cloned.OperationReceipts[0].Governance.AuthorizationContext.Fields[0].Value.Array[0].Object[0].Value.String = "caller mutation"
			if fields[0].Value.Array[0].Object[0].Value.String != " original \x00" {
				t.Fatal("receipt authorization mutation changed original context")
			}
		}
	}
}

func TestRuntimeStateRecordClonePreservesRawShapeAndDetachesAllReferences(t *testing.T) {
	t.Parallel()

	var original RuntimeStateRecord
	sequence := 0
	populateRuntimeStateRawCloneFixture(reflect.ValueOf(&original).Elem(), map[reflect.Type]int{}, &sequence)

	// Keep explicit normalization regressions beside the structural graph walk.
	// These values are deliberately invalid as semantic Runtime state: Run owns
	// only opaque physical persistence and Clone must not repair them.
	original.Definition.GoalDefinition = "  preserve goal whitespace  \n"
	original.Definition.CapabilityContract = []string{}
	original.ProcessedEventIDs = []string{" event-z ", " event-z ", " event-a "}
	original.Diagnostics = nil
	original.OperationReceipts = []RuntimeOperationReceipt{
		{ExecutionKey: " receipt-z ", TargetRefs: []string{" target-b ", " target-b ", " target-a "}},
		{ExecutionKey: " receipt-z ", TargetRefs: []string{}},
	}
	original.PrerequisiteInvalidations = []RuntimePrerequisiteInvalidationRecord{
		{Signature: " invalidation-z ", PrerequisiteIDs: []string{" prerequisite-b ", " prerequisite-b ", " prerequisite-a "}},
		{Signature: " invalidation-z ", ObservationIDs: []string{}},
	}

	cloned := original.Clone()
	if !reflect.DeepEqual(cloned, original) {
		t.Fatalf("RuntimeStateRecord.Clone changed raw shape\noriginal: %#v\nclone:    %#v", original, cloned)
	}
	if cloned.Definition.CapabilityContract == nil || cloned.OperationReceipts[1].TargetRefs == nil ||
		cloned.PrerequisiteInvalidations[1].ObservationIDs == nil {
		t.Fatal("RuntimeStateRecord.Clone collapsed a non-nil empty collection")
	}
	assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(original), reflect.ValueOf(cloned), "RuntimeStateRecord")
}

func populateRuntimeStateRawCloneFixture(value reflect.Value, active map[reflect.Type]int, sequence *int) {
	if !value.IsValid() || !value.CanSet() {
		return
	}
	typeOfValue := value.Type()
	switch value.Kind() {
	case reflect.Pointer:
		if active[typeOfValue] != 0 {
			return
		}
		active[typeOfValue]++
		defer func() { active[typeOfValue]-- }()
		value.Set(reflect.New(typeOfValue.Elem()))
		populateRuntimeStateRawCloneFixture(value.Elem(), active, sequence)
	case reflect.Struct:
		if active[typeOfValue] != 0 {
			return
		}
		active[typeOfValue]++
		defer func() { active[typeOfValue]-- }()
		for idx := 0; idx < value.NumField(); idx++ {
			populateRuntimeStateRawCloneFixture(value.Field(idx), active, sequence)
		}
	case reflect.Slice:
		if active[typeOfValue] != 0 {
			return
		}
		active[typeOfValue]++
		defer func() { active[typeOfValue]-- }()
		length := 1
		if runtimeStateRawCloneScalarKind(typeOfValue.Elem().Kind()) {
			length = 3
		}
		value.Set(reflect.MakeSlice(typeOfValue, length, length))
		for idx := 0; idx < length; idx++ {
			populateRuntimeStateRawCloneFixture(value.Index(idx), active, sequence)
		}
	case reflect.Map:
		value.Set(reflect.MakeMapWithSize(typeOfValue, 1))
		key := reflect.New(typeOfValue.Key()).Elem()
		item := reflect.New(typeOfValue.Elem()).Elem()
		populateRuntimeStateRawCloneFixture(key, active, sequence)
		populateRuntimeStateRawCloneFixture(item, active, sequence)
		value.SetMapIndex(key, item)
	case reflect.Array:
		for idx := 0; idx < value.Len(); idx++ {
			populateRuntimeStateRawCloneFixture(value.Index(idx), active, sequence)
		}
	case reflect.String:
		*sequence++
		value.SetString(fmt.Sprintf(" raw-%04d  ", *sequence))
	case reflect.Bool:
		value.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		*sequence++
		value.SetInt(int64(*sequence + 1))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		*sequence++
		value.SetUint(uint64(*sequence + 1))
	case reflect.Float32, reflect.Float64:
		*sequence++
		value.SetFloat(float64(*sequence) + 0.25)
	}
}

func runtimeStateRawCloneScalarKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func assertRuntimeStateRawCloneDetached(t *testing.T, original, cloned reflect.Value, path string) {
	t.Helper()
	if !original.IsValid() || !cloned.IsValid() {
		return
	}
	switch original.Kind() {
	case reflect.Pointer:
		if original.IsNil() {
			return
		}
		if original.Pointer() == cloned.Pointer() {
			t.Fatalf("%s pointer aliases original storage", path)
		}
		assertRuntimeStateRawCloneDetached(t, original.Elem(), cloned.Elem(), path+".*")
	case reflect.Slice:
		if original.IsNil() {
			return
		}
		if original.Len() > 0 && original.Pointer() == cloned.Pointer() {
			t.Fatalf("%s slice aliases original storage", path)
		}
		for idx := 0; idx < original.Len(); idx++ {
			assertRuntimeStateRawCloneDetached(t, original.Index(idx), cloned.Index(idx), fmt.Sprintf("%s[%d]", path, idx))
		}
	case reflect.Map:
		if original.IsNil() {
			return
		}
		if original.Pointer() == cloned.Pointer() {
			t.Fatalf("%s map aliases original storage", path)
		}
	case reflect.Struct:
		for idx := 0; idx < original.NumField(); idx++ {
			assertRuntimeStateRawCloneDetached(t, original.Field(idx), cloned.Field(idx), path+"."+original.Type().Field(idx).Name)
		}
	case reflect.Array:
		for idx := 0; idx < original.Len(); idx++ {
			assertRuntimeStateRawCloneDetached(t, original.Index(idx), cloned.Index(idx), fmt.Sprintf("%s[%d]", path, idx))
		}
	case reflect.Interface:
		if !original.IsNil() {
			assertRuntimeStateRawCloneDetached(t, original.Elem(), cloned.Elem(), path+".(value)")
		}
	}
}
