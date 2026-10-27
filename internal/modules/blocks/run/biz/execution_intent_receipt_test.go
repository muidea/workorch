package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func receiptRestartCommand(t *testing.T) runevents.WorkspaceExecutionIntentCommand {
	t.Helper()
	bootstrap := restartTestBootstrap(t, "ws-receipt")
	return runevents.WorkspaceExecutionIntentCommand{WorkspaceID: "ws-receipt", RequestID: "original-request", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap}
}

func receiptQueryCommand(t *testing.T, c runevents.WorkspaceExecutionIntentCommand) runevents.QueryWorkspaceExecutionIntentReceiptCommand {
	t.Helper()
	hash, err := runevents.WorkspaceExecutionIntentPayloadHash(c)
	if err != nil {
		t.Fatal(err)
	}
	return runevents.QueryWorkspaceExecutionIntentReceiptCommand{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, PayloadHash: hash}
}

func queryReceipt(t *testing.T, owner *Biz, c runevents.QueryWorkspaceExecutionIntentReceiptCommand) runevents.QueryWorkspaceExecutionIntentReceiptResult {
	t.Helper()
	data, err := owner.executeCommand(magicevent.NewEventWithContext(runevents.TopicQueryWorkspaceExecutionIntentReceipt, "blocks/workspace", runevents.CommandDestination, nil, context.Background(), c))
	r, ok := data.(runevents.QueryWorkspaceExecutionIntentReceiptResult)
	if err != nil || !ok || r.ValidateFor(c) != nil {
		t.Fatalf("receipt query: %+v %v", data, err)
	}
	return r
}

// The cut wrapper intercepts only the new receipt CAS, not lifecycle writes.
type executionReceiptCutRepository struct {
	repository.RunRepository
	mode     string
	casCalls int
}

func (r *executionReceiptCutRepository) ReadWorkspaceExecutionIntentReceipt(ctx context.Context, ws common.WorkspaceID, id string) (*runmodel.WorkspaceExecutionIntentReceipt, error) {
	return r.RunRepository.(repository.RunExecutionIntentReceiptReader).ReadWorkspaceExecutionIntentReceipt(ctx, ws, id)
}

func (r *executionReceiptCutRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, revision uint64) error {
	current, err := r.RunRepository.GetByWorkspaceID(ctx, item.WorkspaceID, item.ID)
	if err != nil {
		return err
	}
	if len(item.ExecutionIntentReceipts) == len(current.ExecutionIntentReceipts) {
		return r.RunRepository.CompareAndSwap(ctx, item, revision)
	}
	r.casCalls++
	if r.mode == "before" {
		return errors.New("fixture before receipt CAS")
	}
	if err := r.RunRepository.CompareAndSwap(ctx, item, revision); err != nil {
		return err
	}
	if r.mode == "after" {
		return errors.New("fixture receipt CAS reply lost")
	}
	return nil
}

func TestExecutionReceiptOriginalResponseSurvivesOwnerReopenAndWriteCuts(t *testing.T) {
	for _, cut := range []string{"none", "before", "after"} {
		t.Run(cut, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "runs.json")
			c := receiptRestartCommand(t)
			q := receiptQueryCommand(t, c)
			runs := &executionReceiptCutRepository{RunRepository: filerepo.NewRunRepository(path), mode: cut}
			hub := magicevent.NewHub(32)
			owner := New()
			if err := owner.Configure(runBizTestDependencies(runs, nil, nil, hub, nil)); err != nil {
				t.Fatal(err)
			}
			response, err := owner.restartWorkspaceExecutionIntent(ctx, c)
			if (cut == "none" && err != nil) || (cut != "none" && err == nil) || runs.casCalls != 1 {
				t.Fatalf("initial response=%+v err=%v receipt CAS=%d", response, err, runs.casCalls)
			}
			if err := owner.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			hub.Terminate(ctx)
			child := exec.Command(os.Args[0], "-test.run=^TestExecutionReceiptReadOnlyProcessReopen$", "-test.count=1")
			child.Env = append(os.Environ(), "WORKORCH_TEST_EXEC_RECEIPT_PATH="+path, "WORKORCH_TEST_EXEC_RECEIPT_CUT="+cut)
			if output, err := child.CombinedOutput(); err != nil {
				t.Fatalf("fresh process receipt read: %s %v", output, err)
			}
			// A fresh owner reads the persisted response. It never invokes the
			// explicit Restart replay path, which would publish continuation.
			reopened := filerepo.NewRunRepository(path)
			hub = magicevent.NewHub(32)
			defer hub.Terminate(ctx)
			owner = New()
			if err := owner.Configure(runBizTestDependencies(reopened, nil, nil, hub, nil)); err != nil {
				t.Fatal(err)
			}
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			defer owner.Stop(ctx)
			var continuations atomic.Int64
			observer := magicevent.NewSimpleObserver("receipt-query-audit", hub)
			if err := observer.Subscribe(runevents.TopicRunContinuationRequested, func(magicevent.Event, magicevent.Result) { continuations.Add(1) }); err != nil {
				t.Fatal(err)
			}
			defer observer.Unsubscribe(runevents.TopicRunContinuationRequested)
			before, _ := reopened.List(ctx)
			if len(before) != 1 || before[0].State != common.RunStateActive {
				t.Fatal("fixture did not reach real activation")
			}
			for i := 0; i < 2; i++ {
				r := hub.Send(magicevent.NewEventWithContext(runevents.TopicQueryWorkspaceExecutionIntentReceipt, "blocks/workspace", runevents.CommandDestination, nil, ctx, q))
				if r == nil || r.Error() != nil {
					t.Fatalf("registered receipt query: %v", r)
				}
				data, err := r.Get()
				value, ok := data.(runevents.QueryWorkspaceExecutionIntentReceiptResult)
				if err != nil || !ok || value.ValidateFor(q) != nil {
					t.Fatalf("registered receipt result: %+v %v", data, err)
				}
				if cut == "before" {
					if value.Receipt != nil {
						t.Fatal("activation snapshot was promoted to missing response")
					}
				} else if value.Receipt == nil || value.Receipt.Admission.RunID != before[0].ID || value.Receipt.Admission.Reason != runtimecontract.RuntimeExecutionIntentReasonNewRunActivated {
					t.Fatalf("lost original response: %+v", value)
				}
				if value.Receipt != nil {
					value.Receipt.PayloadHash = "caller mutation"
				}
			}
			after, _ := reopened.List(ctx)
			if !reflect.DeepEqual(before, after) || continuations.Load() != 0 {
				t.Fatal("read-only reconciliation mutated state or notified continuation")
			}
			if cut == "none" {
				replay, err := owner.restartWorkspaceExecutionIntent(ctx, c)
				if err != nil || replay.Reason != runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted {
					t.Fatalf("existing explicit replay changed: %+v %v", replay, err)
				}
				if queryReceipt(t, owner, q).Receipt.Admission != response.RuntimeExecutionIntentAdmission {
					t.Fatal("later explicit replay rewrote original response")
				}
			} else if cut == "before" {
				if _, err := owner.restartWorkspaceExecutionIntent(ctx, c); err != nil {
					t.Fatal(err)
				}
				if queryReceipt(t, owner, q).Receipt != nil {
					t.Fatal("explicit replay backfilled a missing original response")
				}
			}
		})
	}
}

func TestExecutionReceiptReadOnlyProcessReopen(t *testing.T) {
	path := os.Getenv("WORKORCH_TEST_EXEC_RECEIPT_PATH")
	if path == "" {
		return
	} // Invoked with an exact test-only path by the cut matrix.
	ctx := context.Background()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	headBefore, err := os.ReadFile(path + ".workspace-generations")
	if err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	owner := New()
	if err := owner.Configure(runBizTestDependencies(filerepo.NewRunRepository(path), nil, nil, hub, nil)); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	q := receiptQueryCommand(t, receiptRestartCommand(t))
	r := queryReceipt(t, owner, q)
	if os.Getenv("WORKORCH_TEST_EXEC_RECEIPT_CUT") == "before" {
		if r.Receipt != nil {
			t.Fatal("fresh process fabricated missing original")
		}
	} else if r.Receipt == nil || r.Receipt.Admission.Reason != runtimecontract.RuntimeExecutionIntentReasonNewRunActivated {
		t.Fatal("fresh process lost persisted original response")
	}
	if err := owner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	headAfter, err := os.ReadFile(path + ".workspace-generations")
	if err != nil || string(before) != string(after) || string(headBefore) != string(headAfter) {
		t.Fatal("fresh process query changed raw persistence")
	}
}

func TestExecutionReceiptQueryRejectsWrongIdentityAndKeepsUnknownReadOnly(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	repos := memory.NewRunOwnerRepositorySet()
	owner := New()
	if err := owner.Configure(Dependencies{OwnerRepositories: repos, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	defer owner.Stop(ctx)
	c := receiptRestartCommand(t)
	q := receiptQueryCommand(t, c)
	if queryReceipt(t, owner, q).Receipt != nil {
		t.Fatal("absence was not unknown")
	}
	send := func(source string, data any, ctx context.Context) error {
		_, err := owner.executeCommand(magicevent.NewEventWithContext(runevents.TopicQueryWorkspaceExecutionIntentReceipt, source, runevents.CommandDestination, nil, ctx, data))
		return err
	}
	for _, source := range []string{"", "application/workorchd", "kernel/orchestrator", "blocks/workspace "} {
		if send(source, q, ctx) == nil {
			t.Fatal("wrong source accepted")
		}
	}
	if send("blocks/workspace", c, ctx) == nil || send("blocks/workspace", q, common.ContextWithRuntimeWorkspaceID(ctx, "other")) == nil {
		t.Fatal("wrong DTO or context scope accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if !errors.Is(send("blocks/workspace", q, cancelled), context.Canceled) {
		t.Fatal("cancelled read accepted")
	}
	if _, err := owner.restartWorkspaceExecutionIntent(ctx, c); err != nil {
		t.Fatal(err)
	}
	q.PayloadHash = strings.Repeat("f", 64)
	if !errors.Is(send("blocks/workspace", q, ctx), common.ErrConflict) {
		t.Fatal("wrong payload hash accepted")
	}
	q = receiptQueryCommand(t, c)
	before, _ := repos.Runs().List(ctx)
	original := owner.runs
	owner.runs = &fixedRunInventoryRepository{RunRepository: original}
	if !errors.Is(send("blocks/workspace", q, ctx), common.ErrConflict) {
		t.Fatal("ordinary list substituted for missing complete reader")
	}
	owner.runs = original
	owner.BeginShutdown()
	if queryReceipt(t, owner, q).Receipt == nil {
		t.Fatal("read-only drain lost original receipt")
	}
	after, _ := repos.Runs().List(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("query changed state")
	}
}

func TestExecutionReceiptQuerySharesFreshIntentOrdering(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	repos := memory.NewRunOwnerRepositorySet()
	owner := New()
	if err := owner.Configure(Dependencies{OwnerRepositories: repos, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	defer owner.Stop(ctx)
	c := receiptRestartCommand(t)
	q := receiptQueryCommand(t, c)
	owner.workspaceExecutionMu.Lock()
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		_, err := owner.executeCommand(magicevent.NewEventWithContext(runevents.TopicQueryWorkspaceExecutionIntentReceipt, "blocks/workspace", runevents.CommandDestination, nil, ctx, q))
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		owner.workspaceExecutionMu.Unlock()
		t.Fatalf("read bypassed fresh intent lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	owner.workspaceExecutionMu.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestExecutionReceiptQueryIncludesRetiredHistoryWithoutChangingVisibility(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	repos := memory.NewRunOwnerRepositorySet()
	owner := New()
	if err := owner.Configure(Dependencies{OwnerRepositories: repos, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Stop(ctx)
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, "retired-receipt")
	c := receiptRestartCommand(t)
	c.WorkspaceID = "retired-receipt"
	item := &runmodel.Run{WorkspaceID: c.WorkspaceID, State: common.RunStateCompleted, ActivationRequestID: c.RequestID,
		GoalID: c.GoalID, DoDID: c.DoDID, DefinitionRevision: c.DefinitionRevision, StartupInputHash: c.StartupInputHash, RuntimeBootstrap: c.Bootstrap,
	}
	if err := repos.Runs().CreateNextWorkspaceRun(ctx, item); err != nil {
		t.Fatal(err)
	}
	r := runmodel.WorkspaceExecutionIntentReceipt{Version: runmodel.WorkspaceExecutionIntentReceiptVersion, PayloadHash: receiptQueryCommand(t, c).PayloadHash, RecordedAt: time.Now().UTC(), Admission: runtimecontract.RuntimeExecutionIntentAdmission{
		WorkspaceID: item.WorkspaceID, RunID: item.ID, RequestID: c.RequestID, Kind: runtimecontract.RuntimeExecutionIntentRestart,
		Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
	}}
	item.ExecutionIntentReceipts = []runmodel.WorkspaceExecutionIntentReceipt{r}
	if err := repos.Runs().CompareAndSwap(ctx, item, item.Revision); err != nil {
		t.Fatal(err)
	}
	fence, err := repos.Runs().PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{WorkspaceID: item.WorkspaceID, DeletionID: "delete-receipt", DeletionGeneration: 1, DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "intent:receipt"})
	if err != nil {
		t.Fatal(err)
	}
	retireReadFixture(t, repos.Runs(), ctx, *fence)
	q := runevents.QueryWorkspaceExecutionIntentReceiptCommand{WorkspaceID: item.WorkspaceID, RequestID: r.Admission.RequestID, PayloadHash: r.PayloadHash}
	before, err := repos.Runs().(repository.RunExecutionIntentReceiptReader).ReadWorkspaceExecutionIntentReceipt(ctx, q.WorkspaceID, q.RequestID)
	if err != nil || before == nil {
		t.Fatal("retirement hid receipt from complete reader")
	}
	value := queryReceipt(t, owner, q)
	if value.Receipt == nil || *value.Receipt != r {
		t.Fatal("retired original receipt not returned")
	}
	visible, err := owner.executeCommand(magicevent.NewEventWithContext(runevents.TopicRunCommandListByWorkspace, "blocks/workspace", runevents.CommandDestination, nil, ctx, runevents.ListRunsByWorkspaceCommand{WorkspaceID: item.WorkspaceID}))
	if err != nil || len(visible.(runevents.ListRunsByWorkspaceResult).Runs) != 0 {
		t.Fatal("receipt read changed ordinary retirement visibility")
	}
}

func TestExecutionIntentPayloadHashBindsFullNormalizedCommand(t *testing.T) {
	c := receiptRestartCommand(t)
	first := receiptQueryCommand(t, c).PayloadHash
	// Calibrate the source compiler against the complete concrete command;
	// adding a future field cannot silently escape the persisted identity.
	bytes, err := json.Marshal(struct {
		Domain string
		Intent runevents.WorkspaceExecutionIntentCommand
	}{"workorch-run-execution-intent/v1", c.Normalized()})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(bytes)
	if hex.EncodeToString(digest[:]) != first {
		t.Fatal("source identity omitted a concrete intent field")
	}
	spaced := c
	spaced.RequestID = " " + c.RequestID + " "
	spaced.DefinitionRevision = " " + c.DefinitionRevision + " "
	if receiptQueryCommand(t, spaced).PayloadHash != first {
		t.Fatal("command normalization changed identity")
	}
	for _, mutation := range []string{"request", "workspace", "provider", "goal"} {
		changed := c.Normalized()
		switch mutation {
		case "request":
			changed.RequestID = "other"
		case "workspace":
			changed.WorkspaceID = "other"
		case "provider":
			changed.Bootstrap.Activation.ProviderSelection.Target.ModelID = "other"
		case "goal":
			changed.Bootstrap.Definition.GoalDefinition = "different original goal"
		}
		hash, err := runevents.WorkspaceExecutionIntentPayloadHash(changed)
		if err != nil || hash == first {
			t.Fatalf("full payload %s not bound: %v", mutation, err)
		}
	}
	bad := c
	bad.Bootstrap = nil
	if _, err := runevents.WorkspaceExecutionIntentPayloadHash(bad); err == nil {
		t.Fatal("invalid bootstrap hashed as accepted identity")
	}
}

func TestExecutionReceiptReducerRequiresExactDurableAcceptanceSource(t *testing.T) {
	c := receiptRestartCommand(t)
	runs := memory.NewRunRepository()
	owner := &Biz{runs: runs}
	response, err := owner.restartWorkspaceExecutionIntent(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	current, err := runs.GetByWorkspaceID(context.Background(), c.WorkspaceID, response.RunID)
	if err != nil {
		t.Fatal(err)
	}
	fact := executionReceiptObserved{Command: c.Normalized(), Receipt: current.ExecutionIntentReceipts[0]}
	without := runmodel.CloneRun(current)
	without.ExecutionIntentReceipts = nil
	if next, noop, err := reduceExecutionReceiptObserved(without, fact); err != nil || noop || len(next.ExecutionIntentReceipts) != 1 || len(without.ExecutionIntentReceipts) != 0 {
		t.Fatalf("valid immutable source: %+v %v %v", next, noop, err)
	}
	for _, mutation := range []string{"activation", "definition", "provider", "run", "hash", "command", "replacement"} {
		changed, changedFact := runmodel.CloneRun(without), fact
		changedFact.Command = fact.Command.Normalized()
		switch mutation {
		case "activation":
			changed.ActivationRequestID = "other"
		case "definition":
			changed.StartupInputHash = "other"
		case "provider":
			changed.RuntimeBootstrap.Activation.ProviderSelection.Target.ModelID = "other"
		case "run":
			changed.ID = "other"
		case "hash":
			changedFact.Receipt.PayloadHash = strings.Repeat("b", 64)
		case "command":
			changedFact.Command.RequestID = "other"
		case "replacement":
			changedFact.Receipt.Admission.Outcome = runtimecontract.RuntimeExecutionIntentReplacementPending
			changedFact.Receipt.Admission.Reason = runtimecontract.RuntimeExecutionIntentReasonPredecessorSupersedeRequested
		}
		if _, _, err := reduceExecutionReceiptObserved(changed, changedFact); err == nil {
			t.Fatalf("%s forged source accepted", mutation)
		}
	}
	before := runmodel.CloneRun(current)
	fact.Receipt.RecordedAt = fact.Receipt.RecordedAt.Add(time.Second)
	next, noop, err := reduceExecutionReceiptObserved(current, fact)
	if err != nil || !noop || !reflect.DeepEqual(next, before) {
		t.Fatal("duplicate replaced original response or timestamp")
	}
}

func TestExecutionReceiptFirstAdmissionRejectsKnownInvalidInputBeforeEffect(t *testing.T) {
	for _, scenario := range []string{"request", "workspace", "missing_complete_reader"} {
		t.Run(scenario, func(t *testing.T) {
			c := receiptRestartCommand(t)
			runs := memory.NewRunRepository()
			owner := &Biz{runs: runs}
			switch scenario {
			case "request":
				c.RequestID = strings.Repeat("r", 257)
			case "workspace":
				c.WorkspaceID = "invalid/scope"
			case "missing_complete_reader":
				owner.runs = &fixedRunInventoryRepository{RunRepository: runs}
			}
			if _, err := owner.restartWorkspaceExecutionIntent(context.Background(), c); err == nil {
				t.Fatal("known invalid fresh input accepted")
			}
			items, err := runs.List(context.Background())
			if err != nil || len(items) != 0 {
				t.Fatal("known refusal happened after Run effect")
			}
		})
	}
}
