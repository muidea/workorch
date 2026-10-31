package biz

import (
	"context"
	"errors"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"strings"
	"testing"
	"time"
)

func TestPreparationDiagnosticPreservesRawErrorAndEffectiveDeadline(t *testing.T) {
	ctx, stop := context.WithTimeout(context.Background(), time.Minute)
	defer stop()
	child, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()
	raw := errors.New("原始错误\x00\r\n" + string([]byte{0xff}))
	err := preparationStageError("registry_before", preparationStageContextError(child, "read", raw))
	d := preparationDiagnostic(ctx, "final", "mcpregistry", "observe", time.Now().UTC(), true, false, err)
	deadline, _ := child.Deadline()
	if d.Validate() != nil || string(d.ErrorBytes) != raw.Error() || !d.Deadline.Equal(deadline) || d.Stage != "registry_before" || d.Code != "owner_rejected" {
		t.Fatalf("lost exact failure: %+v", d)
	}
	empty := preparationDiagnostic(ctx, "initial", "soul", "observe", time.Now().UTC(), true, false, errors.New(""))
	if empty.Validate() != nil || empty.ErrorByteCount != 0 || empty.ErrorSHA256 == "" {
		t.Fatal("empty original error lost identity", empty)
	}
	tooLarge := preparationDiagnostic(ctx, "initial", "soul", "observe", time.Now().UTC(), true, false, errors.New(strings.Repeat("x", workspace.PreparationObservationErrorLimit+1)))
	if tooLarge.Validate() != nil || len(tooLarge.ErrorBytes) != 0 || tooLarge.ErrorByteCount != workspace.PreparationObservationErrorLimit+1 || tooLarge.NotSavedReason != "error_exceeds_storage_limit" || tooLarge.ErrorSHA256 == "" {
		t.Fatal("oversize error silently truncated", tooLarge)
	}
	for _, tc := range []struct {
		err                 error
		attempted, complete bool
		want                string
	}{
		{context.DeadlineExceeded, false, false, "budget_exhausted_before_call"},
		{context.DeadlineExceeded, true, false, "deadline_exceeded"},
		{context.Canceled, true, false, "cancelled"},
		{nil, true, true, "observed"},
	} {
		got := preparationDiagnostic(child, "final", "mcpregistry", "registry_before", time.Now().UTC(), tc.attempted, tc.complete, tc.err)
		if got.Code != tc.want || got.Validate() != nil {
			t.Fatal("failure classified incorrectly", got)
		}
	}
}

func TestPreparationFinalBudgetIsReservedFromInitialDeadline(t *testing.T) {
	// An already expired initial phase simulates exhausting the first budget;
	// final revalidation must inherit the original total, not this phase.
	total, stop := context.WithTimeout(context.Background(), preparationTotalBudget)
	defer stop()
	phases := newPreparationObservationPhases(total)
	phases.stopInitial()
	// Deterministic expired initial-phase cut, with the original request live.
	phases.initial, phases.stopInitial = context.WithDeadline(total, time.Now().Add(-time.Second))
	defer phases.stopInitial()
	initial := phases.initial
	final, done := phases.beginFinal()
	defer done()
	if initial.Err() != context.DeadlineExceeded || final.Err() != nil {
		t.Fatal("final inherited initial exhaustion")
	}
	fd, _ := final.Deadline()
	td, _ := total.Deadline()
	if fd.After(td) || time.Until(fd) > preparationFinalBudget {
		t.Fatal("unbounded final phase")
	}
	stop()
	if final.Err() != context.Canceled {
		t.Fatal("final detached from original request")
	}
}

func TestPreparationFinalOwnerBudgetExhaustedStartsNoQueries(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if deadline {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		} else {
			cancel()
		}
		defer cancel()
		// A nil application would panic if any query was actually invoked.
		var application *Biz
		original := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: "ws-cancelled", RequestID: "original", ExpectedWorkspaceRevision: 2}
		facts, diagnostics := application.observePreparationFinalOwners(ctx, original)
		if facts == nil || facts.WorkspaceID != original.WorkspaceID || facts.WorkspaceRevision != 3 || len(diagnostics) != 4 {
			t.Fatal("lost original scope or cancellation report")
		}
		for _, d := range diagnostics {
			want := "cancelled"
			if deadline {
				want = "budget_exhausted_before_call"
			}
			if d.Validate() != nil || d.Attempted || d.Code != want {
				t.Fatal("unstarted owner read misreported", d)
			}
		}
	}
}
