package legacy

import "testing"

func TestNewMinimalToolAdapterReturnsInstance(t *testing.T) {
	if got := NewMinimalToolAdapter(); got == nil {
		t.Fatal("expected tool adapter")
	}
}
