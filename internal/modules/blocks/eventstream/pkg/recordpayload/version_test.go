package recordpayload

import "testing"

func TestWorkorchEventVersionIsExplicit(t *testing.T) {
	if WorkorchEventVersion != "workorch.event" {
		t.Fatalf("Workorch event version = %q", WorkorchEventVersion)
	}
}
