package workspace

import (
	"context"
	"reflect"
	"sync"
	"testing"

	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// A lost original reply must not require resending the old execution POST to
// unlock a later explicitly authorized Restart. Recovery only consumes the
// original Run receipt; the later request still needs its own reservation.
func TestNewRestartReconcilesPendingOriginalBeforeItsOwnSingleForward(t *testing.T) {
	for _, cut := range []string{"forward_reply", "query_error", "settlement_before", "settlement_reply"} {
		t.Run(cut, func(t *testing.T) {
			ctx := context.Background()
			f := newWorkspaceExecutionIntentFixture(t, freshAccepted)
			b := f.seedWorkspace(t, "next-restart", "prior-run", true)
			original := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "next-restart", RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &b}
			h := &ingressFaultHub{Hub: f.hub}
			switch cut {
			case "forward_reply":
				h.forwardCut = "after"
			case "query_error":
				h.queryCut = "error"
			default:
				f.biz.workspaces = &ingressCASFault{WorkspaceRepository: f.workspaces, settlement: true, after: cut == "settlement_reply"}
			}
			f.biz.Base = basebiz.New(ModuleID, h, nil)
			if _, err := f.execute(ctx, original); err == nil {
				t.Fatal("fault did not propagate")
			}
			before, _ := f.workspaces.Get(ctx, original.WorkspaceID)
			if f.received != 1 {
				t.Fatal("original not accepted exactly once")
			}
			originalReceipt := f.receipts[original.RequestID]
			f.biz.Base = basebiz.New(ModuleID, f.hub, nil)
			f.biz.workspaces = f.workspaces
			next := original
			next.RequestID = "new-explicit-restart"
			var group sync.WaitGroup
			for range 16 {
				group.Go(func() {
					r, err := f.biz.recoverExecutionIntent(recoveryEvent(next))
					if err != nil || r.Admission != nil {
						t.Errorf("new request inherited unresolved/original result: %+v %v", r, err)
					}
				})
			}
			group.Wait()
			settled, _ := f.workspaces.Get(ctx, original.WorkspaceID)
			wantRevision := before.Revision
			if before.PreparationFence.Pending != nil {
				wantRevision++
			}
			if settled.Revision != wantRevision || settled.PreparationFence.Pending != nil || settled.PreparationFence.PreparationEpoch != 1 || len(settled.PreparationFence.Settled) != 1 || settled.PreparationFence.Settled[0].Receipt != originalReceipt || f.received != 1 {
				t.Fatal("recovery altered original, forwarded, or granted a new permit")
			}
			// Neither bootstrap nor admission may be rewritten by settlement.
			expected := *before
			expected.Revision = settled.Revision
			expected.PreparationFence = settled.PreparationFence.Clone()
			if !reflect.DeepEqual(&expected, settled) {
				t.Fatal("settlement changed non-fence owner fields")
			}
			for range 2 {
				r, err := f.execute(ctx, next)
				if err != nil || r.RequestID != next.RequestID || r.Reason != runtimecontract.RuntimeExecutionIntentReasonNewRunActivated {
					t.Fatal("fresh request failed after original settlement", r, err)
				}
			}
			after, _ := f.workspaces.Get(ctx, original.WorkspaceID)
			if f.received != 2 || after.PreparationFence.PreparationEpoch != 2 || len(after.PreparationFence.Settled) != 2 || after.PreparationFence.Settled[0].Receipt != originalReceipt {
				t.Fatal("new Restart bypassed its reservation or duplicated original")
			}
		})
	}
}

func TestNewRestartCannotClearAnUnknownOriginal(t *testing.T) {
	f := newWorkspaceExecutionIntentFixture(t, freshAccepted)
	b := f.seedWorkspace(t, "unknown-restart", "prior-run", true)
	original := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "unknown-restart", RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &b}
	f.biz.Base = basebiz.New(ModuleID, &ingressFaultHub{Hub: f.hub, forwardCut: "before"}, nil)
	if _, err := f.execute(context.Background(), original); err == nil {
		t.Fatal("fault did not propagate")
	}
	f.biz.Base = basebiz.New(ModuleID, f.hub, nil)
	before, _ := f.workspaces.Get(context.Background(), original.WorkspaceID)
	next := original
	next.RequestID = "new-explicit-restart"
	for range 2 {
		r, err := f.biz.recoverExecutionIntent(recoveryEvent(next))
		if err != nil || r.Admission == nil || r.Admission.Outcome != runtimecontract.RuntimeExecutionIntentFreshUnresolved || r.Admission.RequestID != next.RequestID || r.Admission.PendingRequestID != original.RequestID {
			t.Fatal("unknown original was bypassed", r, err)
		}
	}
	after, _ := f.workspaces.Get(context.Background(), original.WorkspaceID)
	if f.received != 0 || !reflect.DeepEqual(before, after) {
		t.Fatal("unknown recovery mutated or forwarded")
	}
}

func TestNewRestartOriginalSettlementDoesNotBypassCurrentAdmission(t *testing.T) {
	f := newWorkspaceExecutionIntentFixture(t, freshAccepted)
	b := f.seedWorkspace(t, "changed-admission", "prior-run", true)
	original := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "changed-admission", RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &b}
	f.biz.Base = basebiz.New(ModuleID, &ingressFaultHub{Hub: f.hub, forwardCut: "after"}, nil)
	if _, err := f.execute(context.Background(), original); err == nil {
		t.Fatal("fault did not propagate")
	}
	current, _ := f.workspaces.Get(context.Background(), original.WorkspaceID)
	current.StartupInput = nil
	current.ActivationAdmission = nil
	if err := f.workspaces.CompareAndSwap(context.Background(), current, current.Revision); err != nil {
		t.Fatal(err)
	}
	f.biz.Base = basebiz.New(ModuleID, f.hub, nil)
	next := original
	next.RequestID = "new-explicit-restart"
	r, err := f.biz.recoverExecutionIntent(recoveryEvent(next))
	if err != nil || r.Admission != nil {
		t.Fatal("original receipt incorrectly depends on current admission", r, err)
	}
	if r, err := f.execute(context.Background(), next); err != nil || r.Reason != runtimecontract.RuntimeExecutionIntentReasonDefinitionAdmissionMissing {
		t.Fatal("new request bypassed missing current admission", r, err)
	}
	after, _ := f.workspaces.Get(context.Background(), original.WorkspaceID)
	if f.received != 1 || after.PreparationFence.Pending != nil || len(after.PreparationFence.Settled) != 1 || after.PreparationFence.PreparationEpoch != 1 || after.StartupInput != nil || after.ActivationAdmission != nil {
		t.Fatal("recovery rebuilt input or forwarded without new admission")
	}
}

func TestNewRestartOriginalQueryFailuresPreserveReservation(t *testing.T) {
	for _, cut := range []string{"nil", "error", "wrong_type", "wrong_hash"} {
		t.Run(cut, func(t *testing.T) {
			f := newWorkspaceExecutionIntentFixture(t, freshAccepted)
			b := f.seedWorkspace(t, "failed-query", "prior-run", true)
			original := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "failed-query", RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &b}
			f.biz.Base = basebiz.New(ModuleID, &ingressFaultHub{Hub: f.hub, forwardCut: "after"}, nil)
			if _, err := f.execute(context.Background(), original); err == nil {
				t.Fatal("fault did not propagate")
			}
			h := &ingressFaultHub{Hub: f.hub, queryCut: cut}
			if cut == "wrong_hash" {
				h.queryCut = ""
				receipt := f.receipts[original.RequestID]
				receipt.PayloadHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				f.receipts[original.RequestID] = receipt
			}
			f.biz.Base = basebiz.New(ModuleID, h, nil)
			before, _ := f.workspaces.Get(context.Background(), original.WorkspaceID)
			next := original
			next.RequestID = "new-explicit-restart"
			if _, err := f.biz.recoverExecutionIntent(recoveryEvent(next)); err == nil {
				t.Fatal("failed/tampered original query granted continuation")
			}
			after, _ := f.workspaces.Get(context.Background(), original.WorkspaceID)
			if f.received != 1 || !reflect.DeepEqual(before, after) {
				t.Fatal("failed query cleared pending or forwarded")
			}
		})
	}
}
