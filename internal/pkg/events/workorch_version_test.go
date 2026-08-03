package events

import "testing"

func TestWorkorchEventVersionIsExplicit(t *testing.T) {
	if WorkorchEventVersion != "workorch.event.v1" {
		t.Fatalf("Workorch event version = %q", WorkorchEventVersion)
	}
}
