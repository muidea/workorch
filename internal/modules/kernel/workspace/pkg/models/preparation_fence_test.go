package workspace

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationFenceRawValidationAndDetachedTemplateView(t *testing.T) {
	fence := &WorkspacePreparationFence{Version: WorkspacePreparationFenceVersion, WorkspaceID: "ws", CreationEpoch: strings.Repeat("a", 32), PreparationEpoch: 1,
		Pending: &WorkspaceFreshIntentReservation{RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, StartupInputHash: "startup", PayloadHash: strings.Repeat("b", 64), RunPayloadHash: strings.Repeat("c", 64)}}
	if err := fence.Validate("ws"); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"version", "scope", "creation", "epoch", "missing", "request", "oversize", "kind", "startup", "hash", "run_hash"} {
		changed := fence.Clone()
		switch scenario {
		case "version":
			changed.Version = " " + changed.Version
		case "scope":
			changed.WorkspaceID = "other"
		case "creation":
			changed.CreationEpoch = strings.ToUpper(changed.CreationEpoch)
		case "epoch":
			changed.PreparationEpoch = 0
		case "missing":
			changed.Pending = nil
		case "request":
			changed.Pending.RequestID = " original "
		case "oversize":
			changed.Pending.RequestID = strings.Repeat("x", 257)
		case "kind":
			changed.Pending.Kind = runtimecontract.RuntimeExecutionIntentResume
		case "startup":
			changed.Pending.StartupInputHash = " startup "
		case "hash":
			changed.Pending.PayloadHash = strings.ToUpper(changed.Pending.PayloadHash)
		case "run_hash":
			changed.Pending.RunPayloadHash = ""
		}
		if changed.Validate("ws") == nil {
			t.Fatalf("%s normalized into success", scenario)
		}
	}
	baseline := fence.Clone()
	template := &WorkspaceTemplate{ID: "template", Name: "template"}
	for _, alreadyBound := range []bool{false, true} {
		w := &Workspace{ID: "ws", TemplateID: template.ID, PreparationFence: fence}
		if alreadyBound {
			w.TemplateBindingHash = WorkspaceTemplateSnapshotHash(template)
		}
		next, err := WorkspaceForCurrentTemplate(w, template)
		if err != nil {
			t.Fatal(err)
		}
		next.PreparationFence.Pending.RequestID = "changed-view"
		if !reflect.DeepEqual(fence, baseline) {
			t.Fatal("detached authoring view aliases owner fence")
		}
	}
}

func settledFenceTestEntry(requestID string) WorkspaceFreshIntentSettlement {
	return WorkspaceFreshIntentSettlement{
		Reservation: WorkspaceFreshIntentReservation{RequestID: requestID, Kind: runtimecontract.RuntimeExecutionIntentRestart, StartupInputHash: "startup", PayloadHash: strings.Repeat("b", 64), RunPayloadHash: strings.Repeat("c", 64)},
		Receipt: runmodel.WorkspaceExecutionIntentReceipt{Version: runmodel.WorkspaceExecutionIntentReceiptVersion, PayloadHash: strings.Repeat("c", 64), RecordedAt: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC),
			Admission: runtimecontract.RuntimeExecutionIntentAdmission{WorkspaceID: "ws", RunID: "run", RequestID: requestID, Kind: runtimecontract.RuntimeExecutionIntentRestart,
				Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated}},
	}
}

func TestPreparationFenceSettlementRawShapeCapacityAndClone(t *testing.T) {
	f := &WorkspacePreparationFence{Version: WorkspacePreparationFenceVersion, WorkspaceID: "ws", CreationEpoch: strings.Repeat("a", 32), PreparationEpoch: 1,
		Settled: []WorkspaceFreshIntentSettlement{settledFenceTestEntry("original")}}
	if err := f.Validate("ws"); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"epoch", "duplicate", "pending_reuse", "scope", "request", "kind", "reason", "hash", "run_hash", "time", "reservation"} {
		changed := f.Clone()
		switch scenario {
		case "epoch":
			changed.PreparationEpoch++
		case "duplicate":
			changed.Settled = append(changed.Settled, changed.Settled[0])
			changed.PreparationEpoch++
		case "pending_reuse":
			reservation := changed.Settled[0].Reservation
			changed.Pending = &reservation
			changed.PreparationEpoch++
		case "scope":
			changed.Settled[0].Receipt.Admission.WorkspaceID = "other"
		case "request":
			changed.Settled[0].Receipt.Admission.RequestID = "other"
		case "kind":
			changed.Settled[0].Receipt.Admission.Kind = runtimecontract.RuntimeExecutionIntentResume
		case "reason":
			changed.Settled[0].Receipt.Admission.Reason = runtimecontract.RuntimeExecutionIntentReasonPendingRunActivated
		case "hash":
			changed.Settled[0].Receipt.PayloadHash = "invalid"
		case "run_hash":
			changed.Settled[0].Receipt.PayloadHash = strings.Repeat("d", 64)
		case "time":
			changed.Settled[0].Receipt.RecordedAt = time.Time{}
		case "reservation":
			changed.Settled[0].Reservation.PayloadHash = "invalid"
		}
		if changed.Validate("ws") == nil {
			t.Fatalf("accepted raw settlement drift: %s", scenario)
		}
	}
	cloned := f.Clone()
	cloned.Settled[0].Receipt.Admission.RunID = "caller-mutated"
	cloned.Settled[0].Reservation.PayloadHash = "caller-mutated"
	if f.Settled[0] != settledFenceTestEntry("original") {
		t.Fatal("clone aliases settled history")
	}
	f.Settled = nil
	for i := range WorkspaceFreshIntentHistoryLimit {
		f.Settled = append(f.Settled, settledFenceTestEntry(fmt.Sprintf("request-%d", i)))
	}
	f.PreparationEpoch = WorkspaceFreshIntentHistoryLimit
	if err := f.Validate("ws"); err != nil {
		t.Fatal("full history is not readable", err)
	}
	reservation := settledFenceTestEntry("overflow").Reservation
	f.Pending = &reservation
	f.PreparationEpoch++
	if f.Validate("ws") == nil {
		t.Fatal("pending bypassed full history capacity")
	}
	f.Pending = nil
	f.Settled = append(f.Settled, settledFenceTestEntry("overflow"))
	if f.Validate("ws") == nil {
		t.Fatal("oversize history accepted")
	}
}

func TestPreparationFenceSettlementTransitionIsAppendOnly(t *testing.T) {
	entry := settledFenceTestEntry("original")
	before := &Workspace{ID: "ws", PreparationFence: &WorkspacePreparationFence{Version: WorkspacePreparationFenceVersion, WorkspaceID: "ws", CreationEpoch: strings.Repeat("a", 32), PreparationEpoch: 1, Pending: &entry.Reservation}}
	after := &Workspace{ID: "ws", PreparationFence: before.PreparationFence.Clone()}
	after.PreparationFence.Pending = nil
	after.PreparationFence.Settled = []WorkspaceFreshIntentSettlement{entry}
	if err := ValidatePreparationFenceTransition(before, after); err != nil {
		t.Fatal(err)
	}
	if ValidatePreparationFenceUpdate(before, after) == nil || ValidatePreparationFenceTransition(nil, after) == nil {
		t.Fatal("generic update/create synthesized settlement history")
	}
	for _, scenario := range []string{"drop", "rewrite_receipt", "rewrite_identity", "replace", "skip_pending", "settle_and_reserve", "evict"} {
		changed := &Workspace{ID: "ws", PreparationFence: after.PreparationFence.Clone()}
		base := after
		switch scenario {
		case "drop":
			changed.PreparationFence.Settled = nil
			changed.PreparationFence.PreparationEpoch = 0
		case "rewrite_receipt":
			changed.PreparationFence.Settled[0].Receipt.Admission.RunID = "changed"
		case "rewrite_identity":
			changed.PreparationFence.Settled[0].Reservation.PayloadHash = strings.Repeat("d", 64)
		case "replace":
			changed.PreparationFence.Settled[0] = settledFenceTestEntry("other")
		case "skip_pending":
			changed.PreparationFence.Settled = append(changed.PreparationFence.Settled, settledFenceTestEntry("other"))
			changed.PreparationFence.PreparationEpoch++
		case "settle_and_reserve":
			base = before
			reservation := settledFenceTestEntry("other").Reservation
			changed.PreparationFence.Pending = &reservation
			changed.PreparationFence.PreparationEpoch++
		case "evict":
			changed.PreparationFence.Settled = nil
			changed.PreparationFence.Pending = &entry.Reservation
		}
		if ValidatePreparationFenceTransition(base, changed) == nil {
			t.Fatalf("allowed immutable history change: %s", scenario)
		}
	}
}
