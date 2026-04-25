package demo

import (
	"context"
	"testing"
)

func mustRunAllWithBackend(t *testing.T, backend string, dataRoot string) []Slice {
	t.Helper()

	slices, err := RunAllWithBackend(context.Background(), backend, dataRoot)
	if err != nil {
		t.Fatalf("runAll %s: %v", backend, err)
	}
	return slices
}
