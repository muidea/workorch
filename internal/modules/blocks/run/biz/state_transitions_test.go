package run

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunStateTransitionsRemainOwnedByRunBlock(t *testing.T) {
	if !runStateCanTransition(common.RunStatePending, common.RunStateActive) {
		t.Fatal("pending -> active must be accepted")
	}
	if runStateCanTransition(common.RunStateCompleted, common.RunStateActive) {
		t.Fatal("terminal -> active must be rejected")
	}
}
