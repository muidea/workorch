package runtime

import (
	"context"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
	"testing"
)

// Invalid originals must also round-trip exactly: conversion cannot normalize
// them into an admissible image. Populate every exported field, including nil
// versus empty collections, so a future added field cannot silently disappear.
func fillOwnerRawValue(v reflect.Value, depth int, empty bool) {
	if !v.CanSet() || depth == 0 {
		return
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(" 原\x00文 ")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(7)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(7)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			fillOwnerRawValue(v.Field(i), depth-1, empty)
		}
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fillOwnerRawValue(v.Elem(), depth-1, empty)
	case reflect.Slice:
		size := 1
		if empty {
			size = 0
		}
		v.Set(reflect.MakeSlice(v.Type(), size, size))
		if size != 0 {
			fillOwnerRawValue(v.Index(0), depth-1, empty)
		}
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
	}
}
func TestOwnerRecordConversionPreservesEveryRawField(t *testing.T) {
	for _, empty := range []bool{false, true} {
		var original rc.RuntimeStateRecord
		fillOwnerRawValue(reflect.ValueOf(&original).Elem(), 8, empty)
		aggregate := RuntimeAggregateFromRecord(original)
		got := RuntimeStateRecordFromAggregate(aggregate)
		if !reflect.DeepEqual(got, original) {
			t.Fatal("conversion dropped or normalized an original field", empty)
		}
		aggregate.Facts.WorkspaceID = "caller mutation"
		if !reflect.DeepEqual(got, original) {
			t.Fatal("conversion aliases owner input")
		}
	}
	var nilRecord rc.RuntimeStateRecord
	if got := RuntimeStateRecordFromAggregate(RuntimeAggregateFromRecord(nilRecord)); !reflect.DeepEqual(got, nilRecord) {
		t.Fatal("nil collection changed")
	}
}

func TestOwnerTransactionAdmitsSameOriginalOnceButRereadsChangedInput(t *testing.T) {
	base := stateChangeAggregate(t)
	calls := 0
	tx := &ownerStateTransaction{validate: func(r *rc.RuntimeStateRecord, a RuntimeAggregate) error {
		calls++
		return independentOwnerStateImage(r, a)
	}}
	raw := RuntimeStateRecordFromAggregate(base)
	if err := tx.admit(&raw, base); err != nil {
		t.Fatal(err)
	}
	detached := raw.Clone()
	tx.original = &detached
	for range 3 {
		if err := tx.admit(&raw, base); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("same event input was admitted repeatedly", calls)
	}
	changed := base.CloneRaw()
	changed.StateRevision++
	if err := tx.admit(nil, changed); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("changed after-image skipped owner admission")
	}
	changed = base.CloneRaw()
	changed.ExecutionBudget.Policy.PolicyRevision = "corrupt same identity"
	if err := tx.admit(nil, changed); err == nil {
		t.Fatal("different raw source borrowed prior result")
	}
	if calls != 3 {
		t.Fatal("source drift did not reach sole admission")
	}
	raw.ResultFeedbackVersion = 0
	if err := tx.admit(&raw, base); err == nil {
		t.Fatal("changed original paired with unchanged conversion")
	}
}

// This aggregate-only rule must move into the canonical record admission when
// the duplicate aggregate composition is retired. A normal lease with no
// recovery request is still invalid after administrative termination.
func TestOwnerAdmissionKeepsAdministrativeTerminationLeaseFence(t *testing.T) {
	ctx := context.Background()
	repo := &controllerRepository{}
	controller, err := NewRuntimeController(repo)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, lease, _, err := controller.scheduleDriveLease(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, rc.RuntimeDriveLease{
		WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID,
		Reason: rc.RuntimeDriveLeaseReasonPersistedPending, FailureSignature: "original-normal-drive",
		DueAt: "2026-11-04T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	terminated, err := controller.requestTermination(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, rc.RuntimeTerminationCancelled, "blocks/run", "operator cancel")
	if err != nil {
		t.Fatal(err)
	}
	if err := terminated.Validate(); err != nil {
		t.Fatal("legal no-lease termination", err)
	}
	terminated.DriveLease = &lease
	if err := validateRuntimeAggregateSM4DriveLease(terminated); err == nil {
		t.Fatal("original aggregate oracle accepted noncanonical lease")
	}
	raw := RuntimeStateRecordFromAggregate(terminated)
	if err := raw.Validate(); err == nil {
		t.Fatal("canonical record lost administrative terminal lease fence")
	}
	if err := terminated.Validate(); err == nil {
		t.Fatal("owner accepted a noncanonical administrative terminal lease")
	}
	if _, err := AdmitRuntimeOwnerStateRecord(raw); err == nil {
		t.Fatal("cold recovery accepted the lease")
	}
}
