package workspace

import (
	"context"
	"errors"
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
	"sync"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func freshAccepted(c runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
	return runevents.WorkspaceExecutionIntentResult{RuntimeExecutionIntentAdmission: runtimecontract.RuntimeExecutionIntentAdmission{
		WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, Kind: c.Kind, RunID: "original-run",
		Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
	}}
}

func recoveryEvent(c workspaceevents.WorkspaceExecutionIntentCommand) magicevent.Event {
	return magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandRecoverExecutionIntent, workspaceExecutionIngressSource, workspaceevents.CommandDestination, nil, context.Background(), workspaceevents.RecoverWorkspaceExecutionIntentCommand{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, Kind: c.Kind})
}

func TestFreshIngressConcurrentDuplicatesReserveBeforeExactlyOneForward(t *testing.T) {
	var f *workspaceExecutionIntentFixture
	f = newWorkspaceExecutionIntentFixture(t, func(c runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		w, err := f.workspaces.Get(context.Background(), c.WorkspaceID)
		if err != nil || w.PreparationFence.Pending == nil || w.PreparationFence.Pending.RequestID != c.RequestID || w.PreparationFence.PreparationEpoch != 1 {
			t.Error("forward preceded durable reservation", err)
		}
		return freshAccepted(c)
	})
	b := f.seedWorkspace(t, "ingress-concurrent", "", true)
	c := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "ingress-concurrent", RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &b}
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			r, err := f.execute(context.Background(), c)
			if err != nil || (r.Outcome != runtimecontract.RuntimeExecutionIntentFreshUnresolved && r.Reason != runtimecontract.RuntimeExecutionIntentReasonNewRunActivated) {
				t.Errorf("duplicate: %+v %v", r, err)
			}
		})
	}
	group.Wait()
	w, _ := f.workspaces.Get(context.Background(), c.WorkspaceID)
	if f.received != 1 || w.PreparationFence.PreparationEpoch != 1 || w.PreparationFence.Pending != nil || len(w.PreparationFence.Settled) != 1 {
		t.Fatalf("received=%d fence=%+v", f.received, w.PreparationFence)
	}
	changed := c.Normalized()
	changed.Bootstrap.Activation.ProviderSelection.Target.ModelID = "other-model"
	if _, err := f.execute(context.Background(), changed); err == nil {
		t.Fatal("same key different payload accepted")
	}
	resume := c
	resume.Kind, resume.Bootstrap = runtimecontract.RuntimeExecutionIntentResume, nil
	if _, err := f.execute(context.Background(), resume); err == nil {
		t.Fatal("resume reused fresh request identity")
	}
}

type ingressFaultHub struct {
	magicevent.Hub
	forwardCut string
	queryCut   string
}

func (h *ingressFaultHub) Send(ev magicevent.Event) magicevent.Result {
	if ev.ID() == runevents.TopicRunCommandWorkspaceExecutionIntent {
		if h.forwardCut == "before" {
			return nil
		}
		if h.forwardCut == "after" {
			h.Hub.Send(ev)
			return nil
		}
	}
	if ev.ID() == runevents.TopicQueryWorkspaceExecutionIntentReceipt && h.queryCut != "" {
		if h.queryCut == "nil" {
			return nil
		}
		r := magicevent.NewResult(ev.ID(), ev.Destination(), ev.Source())
		if h.queryCut == "wrong_type" {
			r.Set("not an owner receipt", nil)
		} else {
			r.Set(nil, cd.NewError(cd.Unexpected, "query failed"))
		}
		return r
	}
	return h.Hub.Send(ev)
}

func TestFreshIngressForwardAndQueryCutsNeverResendOrReleaseUnknown(t *testing.T) {
	for _, cut := range []string{"before", "after", "query_nil", "query_wrong_type", "query_error"} {
		t.Run(cut, func(t *testing.T) {
			f := newWorkspaceExecutionIntentFixture(t, freshAccepted)
			b := f.seedWorkspace(t, "ingress-cut", "", true)
			c := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "ingress-cut", RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &b}
			h := &ingressFaultHub{Hub: f.hub}
			if cut == "before" || cut == "after" {
				h.forwardCut = cut
			} else {
				h.queryCut = cut[len("query_"):]
			}
			f.biz.Base = basebiz.New(ModuleID, h, nil)
			if _, err := f.execute(context.Background(), c); err == nil {
				t.Fatal("fault did not propagate")
			}
			before, _ := f.workspaces.Get(context.Background(), c.WorkspaceID)
			if before.PreparationFence.Pending == nil {
				t.Fatal("fault removed pending")
			}
			// A valid full payload must still be able to resolve the old identity
			// after current startup/admission and formal rows have disappeared.
			before.StartupInput, before.ActivationAdmission = nil, nil
			if err := f.workspaces.CompareAndSwap(context.Background(), before, before.Revision); err != nil {
				t.Fatal(err)
			}
			h.forwardCut, h.queryCut = "", ""
			for range 2 {
				r, err := f.execute(context.Background(), c)
				if err != nil {
					t.Fatal(err)
				}
				if cut == "before" && (r.Outcome != runtimecontract.RuntimeExecutionIntentFreshUnresolved || r.PendingRequestID != c.RequestID) {
					t.Fatal("unknown treated as resolved", r)
				}
				if cut != "before" && r.Reason != runtimecontract.RuntimeExecutionIntentReasonNewRunActivated {
					t.Fatal("lost original", r)
				}
			}
			recovery, err := f.biz.recoverExecutionIntent(recoveryEvent(c))
			if err != nil || recovery.Admission == nil {
				t.Fatal("public preflight recovery failed", err)
			}
			wantForwards := 1
			if cut == "before" {
				wantForwards = 0
			}
			if f.received != wantForwards {
				t.Fatal("recovery re-forwarded", f.received)
			}
			if cut == "before" {
				other := c
				other.RequestID = "new"
				r, err := f.execute(context.Background(), other)
				if err != nil || r.RequestID != "new" || r.PendingRequestID != "original" || r.Outcome != runtimecontract.RuntimeExecutionIntentFreshUnresolved {
					t.Fatal("pending bypassed", r, err)
				}
			}
		})
	}
}

type ingressCASFault struct {
	repository.WorkspaceRepository
	settlement bool
	after      bool
}

func (r *ingressCASFault) CompareAndSwap(ctx context.Context, w *workspacemodel.Workspace, revision uint64) error {
	f := w.PreparationFence
	if f != nil && ((r.settlement && len(f.Settled) == 1) || (!r.settlement && f.Pending != nil)) {
		if r.after {
			if err := r.WorkspaceRepository.CompareAndSwap(ctx, w, revision); err != nil {
				return err
			}
		}
		return errReservationLostReply
	}
	return r.WorkspaceRepository.CompareAndSwap(ctx, w, revision)
}

func TestFreshIngressReservationAndSettlementCASReplyCuts(t *testing.T) {
	for _, settlement := range []bool{false, true} {
		for _, after := range []bool{false, true} {
			t.Run(map[bool]string{false: "reservation", true: "settlement"}[settlement]+map[bool]string{false: "_before", true: "_after"}[after], func(t *testing.T) {
				f := newWorkspaceExecutionIntentFixture(t, freshAccepted)
				b := f.seedWorkspace(t, "ingress-cas", "", true)
				c := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "ingress-cas", RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &b}
				f.biz.workspaces = &ingressCASFault{WorkspaceRepository: f.workspaces, settlement: settlement, after: after}
				if _, err := f.execute(context.Background(), c); !errors.Is(err, errReservationLostReply) {
					t.Fatal("wrong failure", err)
				}
				f.biz.workspaces = f.workspaces
				r, err := f.execute(context.Background(), c)
				if err != nil {
					t.Fatal(err)
				}
				want := 1
				if !settlement && after {
					want = 0
					if r.Outcome != runtimecontract.RuntimeExecutionIntentFreshUnresolved {
						t.Fatal("lost reservation reply granted forward", r)
					}
				}
				w, _ := f.workspaces.Get(context.Background(), c.WorkspaceID)
				if f.received != want || w.PreparationFence.PreparationEpoch != 1 {
					t.Fatal("duplicate effect or epoch", f.received, w.PreparationFence)
				}
			})
		}
	}
}

func TestFreshIngressRealRunOwnerOriginalRecoveryIsReadOnlyForRun(t *testing.T) {
	f := newWorkspaceExecutionIntentFixture(t, freshAccepted)
	b := f.seedWorkspace(t, "ingress-real", "", true)
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	runs := memory.NewRunOwnerRepositorySet()
	runOwner := runbiz.New()
	if err := runOwner.Configure(runbiz.Dependencies{OwnerRepositories: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	defer runOwner.Stop(ctx)
	f.biz.Base = basebiz.New(ModuleID, &ingressFaultHub{Hub: hub, forwardCut: "after"}, nil)
	c := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "ingress-real", RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &b}
	if _, err := f.execute(ctx, c); err == nil {
		t.Fatal("lost Run reply did not fail")
	}
	before, _ := runs.Runs().List(ctx)
	if len(before) != 1 {
		t.Fatal("Run not actually accepted", len(before))
	}
	for range 2 {
		r, err := f.biz.recoverExecutionIntent(recoveryEvent(c))
		if err != nil || r.Admission == nil || r.Admission.RunID != before[0].ID || r.Admission.Reason != runtimecontract.RuntimeExecutionIntentReasonNewRunActivated {
			t.Fatal("original recovery", r, err)
		}
	}
	after, _ := runs.Runs().List(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("recovery changed Run")
	}
	w, _ := f.workspaces.Get(ctx, c.WorkspaceID)
	if w.PreparationFence.Pending != nil || len(w.PreparationFence.Settled) != 1 {
		t.Fatal("real original not settled")
	}
}

func TestFreshIngressMissingCreationFenceBlocksRestartWithoutChangingResume(t *testing.T) {
	f := newWorkspaceExecutionIntentFixture(t, func(c runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
		r := freshAccepted(c)
		r.Reason = runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted
		return r
	})
	b := f.seedWorkspace(t, "legacy-ingress", "original-run", true)
	w, _ := f.workspaces.Get(context.Background(), "legacy-ingress")
	w.PreparationFence = nil
	// A separate fixture repository creates a legacy row; production CAS must
	// not erase or add the creation marker of an existing Workspace.
	legacy := memory.NewWorkspaceRepository()
	if err := legacy.Create(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	f.biz.workspaces = legacy
	c := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: w.ID, RequestID: "fresh", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &b}
	r, err := f.execute(context.Background(), c)
	if err != nil || r.Reason != runtimecontract.RuntimeExecutionIntentReasonCreationFenceMissing || f.received != 0 {
		t.Fatal("legacy fresh not blocked", r, err)
	}
	recovered, err := f.biz.recoverExecutionIntent(recoveryEvent(c))
	if err != nil || recovered.Admission == nil || recovered.Admission.Reason != r.Reason {
		t.Fatal("legacy recovery changed boundary", recovered, err)
	}
	c.Kind, c.Bootstrap = runtimecontract.RuntimeExecutionIntentResume, nil
	r, err = f.execute(context.Background(), c)
	if err != nil || r.Reason != runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted || f.received != 1 {
		t.Fatal("legacy Resume changed", r, err)
	}
	after, _ := legacy.Get(context.Background(), w.ID)
	if !reflect.DeepEqual(w, after) {
		t.Fatal("legacy marker/admission mutated")
	}
}

func TestExecutionRecoveryKeepsOriginalHistoryAheadOfNewPendingAndRejectsForeignScope(t *testing.T) {
	f := newWorkspaceExecutionIntentFixture(t, freshAccepted)
	b := f.seedWorkspace(t, "recovery-history", "", true)
	c := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "recovery-history", RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &b}
	accepted, err := f.execute(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := f.workspaces.Get(context.Background(), c.WorkspaceID)
	c2 := c
	c2.RequestID = "next"
	fact, err := workspaceFreshIntentReservationFact(c2, w.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, first, err := f.biz.submitWorkspaceFreshIntentReserved(context.Background(), fact); err != nil || !first {
		t.Fatal("new fixture reservation", err)
	}
	r, err := f.biz.recoverExecutionIntent(recoveryEvent(c))
	if err != nil || r.Admission == nil || *r.Admission != accepted {
		t.Fatal("new pending hid old original", r, err)
	}
	newRequest := c
	newRequest.RequestID = "third"
	r, err = f.biz.recoverExecutionIntent(recoveryEvent(newRequest))
	if err != nil || r.Admission == nil || r.Admission.RequestID != "third" || r.Admission.PendingRequestID != "next" {
		t.Fatal("new request bypassed pending", r, err)
	}
	before, _ := f.workspaces.Get(context.Background(), c.WorkspaceID)
	for _, scenario := range []string{"source", "scope", "cancelled", "type"} {
		ev := recoveryEvent(c)
		switch scenario {
		case "source":
			ev = magicevent.NewEventWithContext(ev.ID(), "unknown", ev.Destination(), nil, context.Background(), ev.Data())
		case "scope":
			ev = magicevent.NewEventWithContext(ev.ID(), ev.Source(), ev.Destination(), nil, common.ContextWithRuntimeWorkspaceID(context.Background(), "other"), ev.Data())
		case "cancelled":
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			ev = magicevent.NewEventWithContext(ev.ID(), ev.Source(), ev.Destination(), nil, ctx, ev.Data())
		case "type":
			ev = magicevent.NewEventWithContext(ev.ID(), ev.Source(), ev.Destination(), nil, context.Background(), "invalid")
		}
		if _, err := f.biz.recoverExecutionIntent(ev); err == nil {
			t.Fatal("invalid recovery accepted", scenario)
		}
	}
	after, _ := f.workspaces.Get(context.Background(), c.WorkspaceID)
	if f.received != 1 || !reflect.DeepEqual(before, after) {
		t.Fatal("recovery mutated/forwarded")
	}
}
