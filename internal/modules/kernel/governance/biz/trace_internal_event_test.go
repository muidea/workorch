package governance

import (
	"errors"
	"testing"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestGovernanceTraceInternalReducerOwnsMonotonicStatus(t *testing.T) {
	observed := governancemodel.AutoGovernanceTrace{
		ID: "trace-1", WorkspaceID: "ws-1", RunID: "run-1", ItemType: "ticket", ItemID: "ticket-1",
		Status: governancemodel.AutoGovernanceTraceStateResolved, OccurredAt: "2026-08-22T00:00:00Z",
	}
	event, err := normalizeGovernanceTraceInternalEvent(governanceTraceInternalEvent{
		Kind: governanceTraceInternalEventEvaluated, WorkspaceID: observed.WorkspaceID, TraceID: observed.ID, Evaluated: &observed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Evaluated.Status != "" {
		t.Fatalf("caller-selected status survived normalization: %+v", event.Evaluated)
	}
	evaluated, err := reduceGovernanceTraceInternalEvent(nil, event)
	if err != nil || evaluated.Next.Status != governancemodel.AutoGovernanceTraceStateEvaluated {
		t.Fatalf("evaluated transition = %+v err=%v", evaluated, err)
	}
	resolving, err := reduceGovernanceTraceInternalEvent(evaluated.Next, governanceTraceInternalEvent{
		Kind: governanceTraceInternalEventResolutionStarted, WorkspaceID: observed.WorkspaceID, TraceID: observed.ID,
		ResolutionStarted: &governanceTraceResolutionStartedFact{Action: "answer", OccurredAt: "2026-08-22T00:00:01Z"},
	})
	if err != nil || resolving.Next.Status != governancemodel.AutoGovernanceTraceStateResolving {
		t.Fatalf("resolving transition = %+v err=%v", resolving, err)
	}
	resolvedEvent := governanceTraceInternalEvent{
		Kind: governanceTraceInternalEventResolutionCompleted, WorkspaceID: observed.WorkspaceID, TraceID: observed.ID,
		ResolutionCompleted: &governanceTraceResolvedFact{Action: "answer", OccurredAt: "2026-08-22T00:00:02Z"},
	}
	resolved, err := reduceGovernanceTraceInternalEvent(resolving.Next, resolvedEvent)
	if err != nil || resolved.Next.Status != governancemodel.AutoGovernanceTraceStateResolved {
		t.Fatalf("resolved transition = %+v err=%v", resolved, err)
	}
	duplicate, err := reduceGovernanceTraceInternalEvent(resolved.Next, resolvedEvent)
	if err != nil || !duplicate.Noop || duplicate.Next.Status != governancemodel.AutoGovernanceTraceStateResolved {
		t.Fatalf("duplicate resolved transition = %+v err=%v", duplicate, err)
	}
	_, err = reduceGovernanceTraceInternalEvent(resolved.Next, governanceTraceInternalEvent{
		Kind: governanceTraceInternalEventResolutionBlocked, WorkspaceID: observed.WorkspaceID, TraceID: observed.ID,
		ResolutionBlocked: &governanceTraceBlockedFact{PolicyBlocked: "late_failure", Reason: "late", OccurredAt: "2026-08-22T00:00:03Z"},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("late blocked transition err = %v, want conflict", err)
	}
}

func TestGovernanceTraceInternalEventRequiresExactlyOneFact(t *testing.T) {
	_, err := normalizeGovernanceTraceInternalEvent(governanceTraceInternalEvent{
		Kind: governanceTraceInternalEventEvaluated, WorkspaceID: "ws-1", TraceID: "trace-1",
		Evaluated: &governancemodel.AutoGovernanceTrace{ID: "trace-1", WorkspaceID: "ws-1", RunID: "run-1", ItemType: "ticket", ItemID: "ticket-1", OccurredAt: "2026-08-22T00:00:00Z"},
		Escalated: &governanceTraceEscalatedFact{OccurredAt: "2026-08-22T00:00:01Z"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("multi-payload event err = %v, want invalid argument", err)
	}
}
