package orchestrator

import (
	"errors"
	"testing"
)

func TestRuntimeDriveBatchYieldsCommittedProgressWithoutCumulativeStop(t *testing.T) {
	var revision uint64 = 1
	for range 4 {
		next := revision + runtimeDriveTransitionBatchSize
		err := runtimeDriveBatchYield(revision, next)
		var pending *runtimePersistedDrivePendingError
		var invariant *runtimeInvariantError
		if !errors.As(err, &pending) || pending.Revision != next || errors.As(err, &invariant) {
			t.Fatal("useful committed work became a cumulative failure", err)
		}
		revision = pending.Revision
	}
	for _, values := range [][2]uint64{{0, 1}, {4, 4}, {4, 3}} {
		var invariant *runtimeInvariantError
		if !errors.As(runtimeDriveBatchYield(values[0], values[1]), &invariant) {
			t.Fatal("no-progress or missing original revision yielded execution")
		}
	}
}
