package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func settlementTestInput(t *testing.T) (*Biz, workspaceevents.WorkspaceExecutionIntentCommand, workspaceFreshIntentSettled) {
	t.Helper()
	current, cmd := reservationTestInput(t)
	repo := memory.NewWorkspaceRepository()
	if err := repo.Create(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	owner := &Biz{workspaces: repo}
	fact, err := workspaceFreshIntentReservationFact(cmd, current.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, first, err := owner.submitWorkspaceFreshIntentReserved(context.Background(), fact); err != nil || !first {
		t.Fatal("fixture reservation", err)
	}
	_, q, err := freshIntentReceiptIdentity(cmd)
	if err != nil {
		t.Fatal(err)
	}
	// Pure consumer tests use a typed owner observation. The separate paired
	// owner test below obtains the original from the real Run owner instead.
	observation := runevents.QueryWorkspaceExecutionIntentReceiptResult{
		WorkspaceID: q.WorkspaceID, RequestID: q.RequestID, PayloadHash: q.PayloadHash, ObservedAt: time.Now().UTC(),
		Receipt: &runmodel.WorkspaceExecutionIntentReceipt{
			Version: runmodel.WorkspaceExecutionIntentReceiptVersion, PayloadHash: q.PayloadHash, RecordedAt: time.Now().UTC(),
			Admission: runtimecontract.RuntimeExecutionIntentAdmission{WorkspaceID: cmd.WorkspaceID, RequestID: cmd.RequestID, RunID: "original-run", Kind: cmd.Kind,
				Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated},
		},
	}
	return owner, cmd, workspaceFreshIntentSettled{Command: cmd, Observation: observation}
}

func TestFreshSettlementConcurrentCASPreservesOriginalAndPermitsNextReservation(t *testing.T) {
	owner, cmd, fact := settlementTestInput(t)
	ctx := context.Background()
	before, _ := owner.workspaces.Get(ctx, cmd.WorkspaceID)
	// Mutable admission drift must not make the original unresolvable.
	before.StartupInput = nil
	before.ActivationAdmission = nil
	if err := owner.workspaces.CompareAndSwap(ctx, before, before.Revision); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 24 {
		group.Go(func() {
			if _, err := owner.submitWorkspaceFreshIntentSettled(ctx, fact); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	stored, _ := owner.workspaces.Get(ctx, cmd.WorkspaceID)
	fence := stored.PreparationFence
	if stored.Revision != before.Revision+1 || fence.Pending != nil || fence.PreparationEpoch != 1 || len(fence.Settled) != 1 || fence.Settled[0].Receipt != *fact.Observation.Receipt {
		t.Fatalf("not exactly-once original settlement: %+v", stored)
	}
	replay, _ := workspaceFreshIntentReservationFact(cmd, 1)
	if value, first, err := owner.submitWorkspaceFreshIntentReserved(ctx, replay); err != nil || first || value.Revision != stored.Revision {
		t.Fatalf("settled identity granted another permit: %v %v", first, err)
	}
	// Reinstating the original admission here is a fixture operation, not
	// reconciliation: settlement itself never mutates definition/admission.
	original, _ := reservationTestInput(t)
	stored.StartupInput = original.StartupInput
	stored.ActivationAdmission = original.ActivationAdmission
	if err := owner.workspaces.CompareAndSwap(ctx, stored, stored.Revision); err != nil {
		t.Fatal(err)
	}
	cmd2 := cmd.Normalized()
	cmd2.RequestID = "next"
	reserve2, _ := workspaceFreshIntentReservationFact(cmd2, stored.Revision)
	next, first, err := owner.submitWorkspaceFreshIntentReserved(ctx, reserve2)
	if err != nil || !first || next.PreparationFence.PreparationEpoch != 2 || len(next.PreparationFence.Settled) != 1 {
		t.Fatal("next distinct reservation failed", err)
	}
	if _, first, err := owner.submitWorkspaceFreshIntentReserved(ctx, replay); err != nil || first {
		t.Fatal("new pending hid old settled identity", err)
	}
	receipt, err := owner.reconcileWorkspaceFreshIntent(ctx, cmd)
	if err != nil || receipt == nil || *receipt != *fact.Observation.Receipt {
		t.Fatal("settled replay lost original", err)
	}
	receipt.Admission.RunID = "caller-mutated"
	after, _ := owner.workspaces.Get(ctx, cmd.WorkspaceID)
	if !reflect.DeepEqual(next, after) {
		t.Fatal("settled replay wrote or aliased storage")
	}
}

func TestFreshSettlementAllOriginalKindsAndHistoryCapacity(t *testing.T) {
	for _, reason := range []runtimecontract.RuntimeExecutionIntentReason{
		runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
		runtimecontract.RuntimeExecutionIntentReasonPredecessorSupersedeRequested,
		runtimecontract.RuntimeExecutionIntentReasonTerminalPredecessorSuccessorRequested,
	} {
		t.Run(string(reason), func(t *testing.T) {
			owner, cmd, fact := settlementTestInput(t)
			fact.Observation.Receipt.Admission.Reason = reason
			if reason != runtimecontract.RuntimeExecutionIntentReasonNewRunActivated {
				fact.Observation.Receipt.Admission.Outcome = runtimecontract.RuntimeExecutionIntentReplacementPending
			}
			current, err := owner.submitWorkspaceFreshIntentSettled(context.Background(), fact)
			if err != nil || current.PreparationFence.Settled[0].Receipt != *fact.Observation.Receipt {
				t.Fatal("legitimate original not preserved", err)
			}
			// Construct a full pure-consumer history to exercise the bounded
			// reserve guard without manufacturing production Run receipts.
			for i := 1; i < workspacemodel.WorkspaceFreshIntentHistoryLimit; i++ {
				entry := current.PreparationFence.Settled[0]
				entry.Reservation.RequestID = fmt.Sprintf("history-%d", i)
				entry.Receipt.Admission.RequestID = entry.Reservation.RequestID
				current.PreparationFence.Settled = append(current.PreparationFence.Settled, entry)
			}
			current.PreparationFence.PreparationEpoch = workspacemodel.WorkspaceFreshIntentHistoryLimit
			if err := current.PreparationFence.Validate(current.ID); err != nil {
				t.Fatal(err)
			}
			original, _ := workspaceFreshIntentReservationFact(cmd, current.Revision)
			if transition, err := reduceWorkspaceFreshIntentReserved(current, original); err != nil || !transition.Noop {
				t.Fatal("capacity hid original settled identity", err)
			}
			cmd.RequestID = "overflow"
			overflow, _ := workspaceFreshIntentReservationFact(cmd, current.Revision)
			before := current.PreparationFence.Clone()
			if _, err := reduceWorkspaceFreshIntentReserved(current, overflow); err == nil || !reflect.DeepEqual(before, current.PreparationFence) {
				t.Fatal("capacity bypassed or evicted history")
			}
		})
	}
}

func TestFreshSettlementRejectsUnknownDriftAndWrongOriginalWithoutMutation(t *testing.T) {
	for _, scenario := range []string{"unknown", "query_scope", "query_request", "query_hash", "observation_time", "receipt_hash", "receipt_scope", "receipt_request", "receipt_kind", "receipt_time", "receipt_version", "replay_reason", "payload", "missing_reservation", "legacy", "cancelled", "parent_scope"} {
		t.Run(scenario, func(t *testing.T) {
			owner, cmd, fact := settlementTestInput(t)
			ctx := context.Background()
			switch scenario {
			case "unknown":
				fact.Observation.Receipt = nil
			case "query_scope":
				fact.Observation.WorkspaceID = "other"
			case "query_request":
				fact.Observation.RequestID = "other"
			case "query_hash":
				fact.Observation.PayloadHash = strings.Repeat("a", 64)
			case "observation_time":
				fact.Observation.ObservedAt = time.Time{}
			case "receipt_hash":
				fact.Observation.Receipt.PayloadHash = strings.Repeat("a", 64)
			case "receipt_scope":
				fact.Observation.Receipt.Admission.WorkspaceID = "other"
			case "receipt_request":
				fact.Observation.Receipt.Admission.RequestID = "other"
			case "receipt_kind":
				fact.Observation.Receipt.Admission.Kind = runtimecontract.RuntimeExecutionIntentResume
			case "receipt_time":
				fact.Observation.Receipt.RecordedAt = time.Time{}
			case "receipt_version":
				fact.Observation.Receipt.Version = "unknown"
			case "replay_reason":
				fact.Observation.Receipt.Admission.Reason = runtimecontract.RuntimeExecutionIntentReasonPendingRunActivated
			case "payload":
				fact.Command = cmd.Normalized()
				fact.Command.Bootstrap.Activation.ProviderSelection.Target.ModelID = "changed"
			case "missing_reservation", "legacy":
				current, _ := reservationTestInput(t)
				if scenario == "legacy" {
					current.PreparationFence = nil
				}
				repo := memory.NewWorkspaceRepository()
				if err := repo.Create(ctx, current); err != nil {
					t.Fatal(err)
				}
				owner.workspaces = repo
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "parent_scope":
				ctx = common.ContextWithRuntimeWorkspaceID(ctx, "other")
			}
			before, _ := owner.workspaces.Get(context.Background(), cmd.WorkspaceID)
			if _, err := owner.submitWorkspaceFreshIntentSettled(ctx, fact); err == nil {
				t.Fatal("bad observation settled")
			}
			after, _ := owner.workspaces.Get(context.Background(), cmd.WorkspaceID)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("rejected observation mutated storage")
			}
		})
	}
}

func TestFreshSettlementCASWriteCutsRecoverSameOriginalWithoutSecondPermit(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "lost_reply"}[afterCommit], func(t *testing.T) {
			owner, cmd, fact := settlementTestInput(t)
			repo := owner.workspaces
			owner.workspaces = &reservationFaultRepository{WorkspaceRepository: repo, afterCommit: afterCommit}
			if _, err := owner.submitWorkspaceFreshIntentSettled(context.Background(), fact); !errors.Is(err, errReservationLostReply) {
				t.Fatal(err)
			}
			read, _ := repo.Get(context.Background(), cmd.WorkspaceID)
			if (read.PreparationFence.Pending == nil) != afterCommit {
				t.Fatal("wrong cut state")
			}
			restarted := &Biz{workspaces: repo}
			read, err := restarted.submitWorkspaceFreshIntentSettled(context.Background(), fact)
			if err != nil || len(read.PreparationFence.Settled) != 1 || read.PreparationFence.Pending != nil || read.PreparationFence.PreparationEpoch != 1 {
				t.Fatal("recovery lost original", err)
			}
			reserve, _ := workspaceFreshIntentReservationFact(cmd, read.Revision)
			if _, first, err := restarted.submitWorkspaceFreshIntentReserved(context.Background(), reserve); err != nil || first {
				t.Fatal("recovery granted duplicate forward", err)
			}
			fact.Observation.Receipt.RecordedAt = fact.Observation.Receipt.RecordedAt.Add(time.Second)
			if _, err := restarted.submitWorkspaceFreshIntentSettled(context.Background(), fact); err == nil {
				t.Fatal("changed original rewrote completed history")
			}
		})
	}
}

func TestFreshSettlementReconcileUnknownAndErrorsPreservePendingWithoutForward(t *testing.T) {
	for _, scenario := range []string{"unknown", "error", "wrong_type", "payload_conflict"} {
		t.Run(scenario, func(t *testing.T) {
			owner, cmd, _ := settlementTestInput(t)
			hub := magicevent.NewHub(16)
			defer hub.Terminate(context.Background())
			owner.Base = basebiz.New(ModuleID, hub, nil)
			observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
			queries, forwards := 0, 0
			ownerErr := errors.New("fixture owner error")
			if err := observer.Subscribe(runevents.TopicQueryWorkspaceExecutionIntentReceipt, func(ev magicevent.Event, result magicevent.Result) {
				queries++
				q := ev.Data().(runevents.QueryWorkspaceExecutionIntentReceiptCommand)
				switch scenario {
				case "error":
					result.Set(nil, &cd.Error{Code: cd.Unexpected, Cause: ownerErr})
				case "wrong_type":
					result.Set(runevents.ListRunsByWorkspaceResult{}, nil)
				default:
					result.Set(runevents.QueryWorkspaceExecutionIntentReceiptResult{WorkspaceID: q.WorkspaceID, RequestID: q.RequestID, PayloadHash: q.PayloadHash, ObservedAt: time.Now().UTC()}, nil)
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer observer.Unsubscribe(runevents.TopicQueryWorkspaceExecutionIntentReceipt)
			if err := observer.Subscribe(runevents.TopicRunCommandWorkspaceExecutionIntent, func(_ magicevent.Event, result magicevent.Result) { forwards++; result.Set(nil, nil) }); err != nil {
				t.Fatal(err)
			}
			defer observer.Unsubscribe(runevents.TopicRunCommandWorkspaceExecutionIntent)
			before, _ := owner.workspaces.Get(context.Background(), cmd.WorkspaceID)
			if scenario == "payload_conflict" {
				cmd = cmd.Normalized()
				cmd.Bootstrap.Activation.ProviderSelection.Target.ModelID = "other"
			}
			for range 2 {
				receipt, err := owner.reconcileWorkspaceFreshIntent(context.Background(), cmd)
				if receipt != nil || (err == nil) != (scenario == "unknown") || (scenario == "error" && !errors.Is(err, ownerErr)) {
					t.Fatalf("unexpected reconciliation: %+v %v", receipt, err)
				}
			}
			after, _ := owner.workspaces.Get(context.Background(), cmd.WorkspaceID)
			wantQueries := 2
			if scenario == "payload_conflict" {
				wantQueries = 0
			}
			if forwards != 0 || queries != wantQueries || !reflect.DeepEqual(before, after) {
				t.Fatal("reconciliation forwarded, retried or released pending")
			}
		})
	}
}

func TestFreshSettlementPairsRealRunOriginalWithWorkspaceReservation(t *testing.T) {
	owner, cmd, _ := settlementTestInput(t)
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	owner.Base = basebiz.New(ModuleID, hub, nil)
	runs := memory.NewRunOwnerRepositorySet()
	runOwner := runbiz.New()
	if err := runOwner.Configure(runbiz.Dependencies{OwnerRepositories: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	defer runOwner.Stop(ctx)
	if receipt, err := owner.reconcileWorkspaceFreshIntent(ctx, cmd); err != nil || receipt != nil {
		t.Fatal("empty inventory was not unknown", err)
	}
	d := cmd.Bootstrap.Definition.Normalized()
	forward := runevents.WorkspaceExecutionIntentCommand{WorkspaceID: cmd.WorkspaceID, RequestID: cmd.RequestID, Kind: cmd.Kind,
		GoalID: d.GoalID, DoDID: d.DoDID, DefinitionRevision: d.Revision, StartupInputHash: d.StartupInputHash, Bootstrap: cmd.Bootstrap}
	// Explicit test setup only. Neither reconciliation nor its private reducer
	// sends this effectful command.
	r := hub.Send(magicevent.NewEventWithContext(runevents.TopicRunCommandWorkspaceExecutionIntent, ModuleID, runevents.CommandDestination, nil, ctx, forward))
	if r == nil || r.Error() != nil {
		t.Fatalf("real acceptance: %v", r)
	}
	data, resultErr := r.Get()
	accepted, ok := data.(runevents.WorkspaceExecutionIntentResult)
	if resultErr != nil || !ok {
		t.Fatal("missing original response")
	}
	before, _ := runs.Runs().List(ctx)
	for range 2 {
		receipt, err := owner.reconcileWorkspaceFreshIntent(ctx, cmd)
		if err != nil || receipt == nil || receipt.Admission != accepted.RuntimeExecutionIntentAdmission {
			t.Fatal("did not settle real original", err)
		}
	}
	after, _ := runs.Runs().List(ctx)
	stored, _ := owner.workspaces.Get(ctx, cmd.WorkspaceID)
	if !reflect.DeepEqual(before, after) || stored.PreparationFence.Pending != nil || len(stored.PreparationFence.Settled) != 1 {
		t.Fatal("reconciliation mutated Run or lost settlement")
	}
}

func TestFreshSettlementColdProcessPreservesOriginalAcrossWriteCuts(t *testing.T) {
	for _, cut := range []string{"before", "after"} {
		t.Run(cut, func(t *testing.T) {
			owner, cmd, fact := settlementTestInput(t)
			ctx := context.Background()
			root := t.TempDir()
			path := filepath.Join(root, "workspaces.json")
			input, _ := reservationTestInput(t)
			repo := filerepo.NewWorkspaceRepository(path)
			if err := repo.Create(ctx, input); err != nil {
				t.Fatal(err)
			}
			owner.workspaces = repo
			reserve, _ := workspaceFreshIntentReservationFact(cmd, input.Revision)
			if _, first, err := owner.submitWorkspaceFreshIntentReserved(ctx, reserve); err != nil || !first {
				t.Fatal("reserve on disk", err)
			}
			owner.workspaces = &reservationFaultRepository{WorkspaceRepository: repo, afterCommit: cut == "after"}
			if _, err := owner.submitWorkspaceFreshIntentSettled(ctx, fact); !errors.Is(err, errReservationLostReply) {
				t.Fatal("write cut did not fire", err)
			}
			// Only test-owned, credential-free typed fixture inputs are shared
			// with the helper; no live configuration is read or copied.
			encoded, err := json.Marshal(fact)
			if err != nil {
				t.Fatal(err)
			}
			factPath := filepath.Join(root, "observation.json")
			if err := os.WriteFile(factPath, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			child := exec.Command(os.Args[0], "-test.run=^TestFreshSettlementColdProcessHelper$", "-test.count=1")
			child.Env = append(os.Environ(), "WORKORCH_TEST_SETTLEMENT_PATH="+path, "WORKORCH_TEST_SETTLEMENT_FACT="+factPath, "WORKORCH_TEST_SETTLEMENT_CUT="+cut)
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatalf("cold process: %s %v", output, err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatal("cold replay changed disk bytes", err)
			}
		})
	}
}

func TestFreshSettlementColdProcessHelper(t *testing.T) {
	path := os.Getenv("WORKORCH_TEST_SETTLEMENT_PATH")
	if path == "" {
		return // subprocess-only fixture, not an omitted production assertion
	}
	encoded, err := os.ReadFile(os.Getenv("WORKORCH_TEST_SETTLEMENT_FACT"))
	if err != nil {
		t.Fatal(err)
	}
	var fact workspaceFreshIntentSettled
	if err := json.Unmarshal(encoded, &fact); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := filerepo.NewWorkspaceRepository(path)
	owner := &Biz{workspaces: repo}
	current, err := repo.Get(ctx, fact.Command.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	before := current.Revision
	reserve, _ := workspaceFreshIntentReservationFact(fact.Command, current.Revision)
	if _, first, err := owner.submitWorkspaceFreshIntentReserved(ctx, reserve); err != nil || first {
		t.Fatal("restart granted a second forward permit", err)
	}
	if os.Getenv("WORKORCH_TEST_SETTLEMENT_CUT") == "after" {
		if current.PreparationFence.Pending != nil || len(current.PreparationFence.Settled) != 1 {
			t.Fatal("lost settled original on cold decode")
		}
		receipt, err := owner.reconcileWorkspaceFreshIntent(ctx, fact.Command)
		if err != nil || receipt == nil || *receipt != *fact.Observation.Receipt {
			t.Fatal("cold replay did not return original", err)
		}
		if _, err := owner.submitWorkspaceFreshIntentSettled(ctx, fact); err != nil {
			t.Fatal(err)
		}
		public, err := owner.recoverExecutionIntent(recoveryEvent(fact.Command))
		if err != nil || public.Admission == nil || runtimecontract.RuntimeExecutionIntentAdmission(*public.Admission) != fact.Observation.Receipt.Admission {
			t.Fatal("cold public recovery lost sealed original without bootstrap compilation", err)
		}
	} else {
		if current.PreparationFence.Pending == nil || len(current.PreparationFence.Settled) != 0 {
			t.Fatal("lost pending on cold decode")
		}
		// An absent query provider is an error, never evidence to release it.
		if receipt, err := owner.reconcileWorkspaceFreshIntent(ctx, fact.Command); err == nil || receipt != nil {
			t.Fatal("missing owner cleared pending")
		}
		if _, err := owner.recoverExecutionIntent(recoveryEvent(fact.Command)); err == nil {
			t.Fatal("cold public recovery ignored unavailable Run owner")
		}
		other := fact.Command
		other.RequestID = "new-after-restart"
		public, err := owner.recoverExecutionIntent(recoveryEvent(other))
		if err != nil || public.Admission == nil || public.Admission.PendingRequestID != fact.Command.RequestID || public.Admission.Outcome != runtimecontract.RuntimeExecutionIntentFreshUnresolved {
			t.Fatal("cold pending did not block a new public identity", err)
		}
	}
	after, err := repo.Get(ctx, fact.Command.WorkspaceID)
	if err != nil || after.Revision != before {
		t.Fatal("replay changed revision", err)
	}
}
