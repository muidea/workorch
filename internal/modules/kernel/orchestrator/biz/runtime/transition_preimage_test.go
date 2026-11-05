package runtime

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeTransitionCoordinatorPreimageRetainsIndependentTerminalTransitionGate(t *testing.T) {
	previous := materialSealTerminalAggregate(t)
	seal, err := terminalMaterialSealFromAggregate(previous)
	if err != nil {
		t.Fatal(err)
	}
	previous.StateRevision++
	previous.TerminalMaterialSeal = &seal
	next := previous.CloneRaw()
	next.StateRevision++
	next.TerminalMaterialSeal = nil
	// Both complete states are independently valid. Only the cross-image gate
	// can reject deleting a committed terminal seal.
	if err := previous.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := next.Validate(); err != nil {
		t.Fatal(err)
	}
	original := previous.CloneRaw()
	{
		base := transitionCoordinatorRepository{previous: &previous}
		var repository RuntimeStateRepository = &base
		coordinator, err := NewRuntimeTransitionCoordinator(repository)
		if err != nil {
			t.Fatal(err)
		}
		if err := coordinator.Commit(context.Background(), next, previous.StateRevision); err == nil || !strings.Contains(err.Error(), "terminal material seal") {
			t.Fatalf("bypassed cross-image terminal invariant: %v", err)
		}
		if !reflect.DeepEqual(previous, original) {
			t.Fatal("transition rejection rewrote original source")
		}
	}
}
