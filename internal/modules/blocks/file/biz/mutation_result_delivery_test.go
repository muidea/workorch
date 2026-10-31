//go:build linux

package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mutationDeliveryFixture(t *testing.T, kind string) (contract.RuntimeOperationExecutionContext, fileevents.SealedRuntimeOperation, string, any) {
	t.Helper()
	root := t.TempDir()
	before, after := "before\n", "after\n"
	op := contract.RuntimeOperation{ID: "op-1", Kind: kind, TargetRefs: []string{"target.txt"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}
	topic := fileevents.TopicExecuteFileEditOperation
	switch kind {
	case contract.StepKindFileEdit:
		op.Parameters = []contract.OperationParameter{{Name: "oldText", String: &before}, {Name: "newText", String: &after}}
	case contract.StepKindFileWrite:
		op.Parameters = []contract.OperationParameter{{Name: "content", String: &after}}
		topic = fileevents.TopicExecuteFileWriteOperation
	case contract.RuntimeBlockKindPatchApply:
		variant := patchTypeTextReplace
		op.Parameters = []contract.OperationParameter{{Name: "patchType", String: &variant}, {Name: "oldText", String: &before}, {Name: "newText", String: &after}}
		topic = fileevents.TopicExecutePatchApplyOperation
	}
	if kind != contract.StepKindFileWrite {
		if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte(before), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sealed, err := fileevents.SealRuntimeOperation(op)
	if err != nil {
		t.Fatal(err)
	}
	scope := contract.RuntimeOperationExecutionContext{WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1", OperationID: op.ID,
		Transaction:         contract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", DefinitionRevision: "definition-1"},
		CoverageWorkItemIDs: op.CoverageWorkItemIDs, TargetRefs: op.TargetRefs, EvidenceKind: contract.EvidenceKindStateChange,
		WorkspaceDeliverableDirectory: root, WorkspaceInternalDirectory: t.TempDir(),
		Execution: fileBizTestExecutionEnvelope(op, contract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-1/run-1/dispatch-1/op-1", Fence: 1, ExpectedEffectScope: contract.RuntimeEffectWorkspaceDurable})}
	var cmd any
	switch kind {
	case contract.StepKindFileEdit:
		cmd = fileevents.ExecuteFileEditOperationCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed, ResultDeliveryProtocolRef: fileevents.MutationResultDeliveryV1}
	case contract.StepKindFileWrite:
		cmd = fileevents.ExecuteFileWriteOperationCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed, ResultDeliveryProtocolRef: fileevents.MutationResultDeliveryV1}
	default:
		cmd = fileevents.ExecutePatchApplyOperationCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed, ResultDeliveryProtocolRef: fileevents.MutationResultDeliveryV1}
	}
	return scope, sealed, topic, cmd
}

type deliveryReceiptObserver struct {
	mu       sync.Mutex
	calls    int
	receipts map[string]contract.RuntimeOperationReceipt
	fail     func(int, contract.RuntimeOperationReceipt) *cd.Error
}

func observeDeliveryReceipts(t *testing.T, hub magicevent.Hub, fail func(int, contract.RuntimeOperationReceipt) *cd.Error) *deliveryReceiptObserver {
	t.Helper()
	o := &deliveryReceiptObserver{receipts: map[string]contract.RuntimeOperationReceipt{}, fail: fail}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(e magicevent.Event, r magicevent.Result) {
		o.mu.Lock()
		defer o.mu.Unlock()
		c := e.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		v, ok := o.receipts[c.ExecutionKey]
		out := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: ok}
		if ok {
			out.Receipt = &v
		}
		r.Set(out, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(e magicevent.Event, r magicevent.Result) {
		o.mu.Lock()
		defer o.mu.Unlock()
		c := e.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		o.calls++
		if o.fail != nil {
			if err := o.fail(o.calls, c.Receipt); err != nil {
				r.Set(nil, err)
				return
			}
		}
		o.receipts[c.Receipt.ExecutionKey] = c.Receipt.Clone()
		r.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: c.Receipt, StateRevision: 7}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	return o
}
func transientDeliveryFailure() *cd.Error {
	return &cd.Error{Code: cd.Unexpected, Message: "raw transient CAS failure\nline two", Cause: &contract.RuntimeReceiptDeliveryError{Retryable: true, Cause: errors.New("raw transient CAS failure\nline two")}}
}
func startDeliveryFileOwner(t *testing.T, hub magicevent.Hub) *Biz {
	t.Helper()
	o, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = o.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { o.Teardown(context.Background()) })
	return o
}
func sendDelivery(hub magicevent.Hub, topic string, cmd any) (any, *cd.Error) {
	return hub.Send(magicevent.NewEventWithContext(topic, contract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), context.Background(), cmd)).Get()
}

func TestFileMutationOriginalResultRedeliveryNeverReexecutes(t *testing.T) {
	for _, kind := range []string{contract.StepKindFileEdit, contract.StepKindFileWrite, contract.RuntimeBlockKindPatchApply} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			startDeliveryFileOwner(t, hub)
			observed := observeDeliveryReceipts(t, hub, func(n int, _ contract.RuntimeOperationReceipt) *cd.Error {
				if n == 1 {
					return transientDeliveryFailure()
				}
				return nil
			})
			scope, _, topic, cmd := mutationDeliveryFixture(t, kind)
			data, err := sendDelivery(hub, topic, cmd)
			if err != nil {
				t.Fatal(err)
			}
			receipt := data.(fileevents.ExecuteOperationResult).Receipt
			if observed.calls != 2 || receipt.BeforeHash == receipt.AfterHash || receipt.Evidence.OwnerRevisionSet == nil {
				t.Fatalf("not exactly original result: calls=%d receipt=%+v", observed.calls, receipt)
			}
			if len(observed.receipts) != 1 {
				t.Fatal("duplicate trusted receipt")
			}
			store, e := openMutationResultStore(scope.WorkspaceInternalDirectory, mutationResultKey(scope), false)
			if e != nil {
				t.Fatal(e)
			}
			defer store.close()
			raw, e := store.read("receipt.json")
			if e != nil {
				t.Fatal(e)
			}
			var original contract.RuntimeOperationReceipt
			json.Unmarshal(raw, &original)
			if !contract.SameRuntimeOperationReceipt(original, receipt) {
				t.Fatal("redelivery changed original receipt")
			}
			if e = os.WriteFile(filepath.Join(scope.WorkspaceDeliverableDirectory, "target.txt"), []byte("external\n"), 0600); e != nil {
				t.Fatal(e)
			}
			if _, err = sendDelivery(hub, topic, cmd); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(filepath.Join(scope.WorkspaceDeliverableDirectory, "target.txt"))
			if string(b) != "external\n" || observed.calls != 2 {
				t.Fatal("duplicate executed physical effect")
			}
		})
	}
}

func TestFileMutationReceiptAdmissionFailureRetriesOnlyOriginalMaterial(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	startDeliveryFileOwner(t, hub)
	observed := observeDeliveryReceipts(t, hub, func(n int, _ contract.RuntimeOperationReceipt) *cd.Error {
		if n == 1 {
			return cd.NewError(cd.Timeout, "event admission timed out or was canceled")
		}
		return nil
	})
	scope, _, topic, cmd := mutationDeliveryFixture(t, contract.StepKindFileEdit)
	if _, err := sendDelivery(hub, topic, cmd); err != nil || observed.calls != 2 {
		t.Fatalf("original receipt admission redelivery: %v / %d", err, observed.calls)
	}
	body, err := os.ReadFile(filepath.Join(scope.WorkspaceDeliverableDirectory, "target.txt"))
	if err != nil || string(body) != "after\n" || len(observed.receipts) != 1 {
		t.Fatalf("admission recovery repeated effect or receipt: %v", err)
	}
	for _, failure := range []error{
		cd.NewError(cd.Timeout, "owner validation deadline"),
		cd.NewError(cd.Unexpected, "event admission timed out or was canceled"),
		&contract.RuntimeReceiptDeliveryError{Retryable: false, Cause: cd.NewError(cd.Timeout, "event admission timed out or was canceled")},
		common.ErrConflict,
	} {
		if retryableMutationReceiptDelivery(context.Background(), failure) {
			t.Fatalf("deterministic/unknown rejection became transport retry: %v", failure)
		}
	}
}

func TestFileMutationSavedResultSurvivesFreshOwnerAndLostResponse(t *testing.T) {
	for _, mode := range []string{"restart_before_commit", "lost_commit_response"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			owner := startDeliveryFileOwner(t, hub)
			observed := observeDeliveryReceipts(t, hub, nil)
			scope, sealed, topic, cmd := mutationDeliveryFixture(t, contract.StepKindFileEdit)
			observed.fail = func(n int, r contract.RuntimeOperationReceipt) *cd.Error {
				if mode == "lost_commit_response" {
					observed.receipts[r.ExecutionKey] = r.Clone()
				}
				return transientDeliveryFailure()
			}
			_, callErr := sendDelivery(hub, topic, cmd)
			if mode == "lost_commit_response" {
				if callErr != nil || observed.calls != 1 {
					t.Fatalf("lost response not reconciled: %v / %d", callErr, observed.calls)
				}
				return
			}
			if callErr == nil || observed.calls != mutationResultDeliveryLimit {
				t.Fatalf("unbounded/missing failure %v %d", callErr, observed.calls)
			}
			owner.Teardown(ctx)
			startDeliveryFileOwner(t, hub)
			// Simulate an original attempt reserved just before shutdown, but never
			// committed. The persisted total is not reset by constructing a new owner.
			observed.fail = nil
			_, err := sendDelivery(hub, fileevents.TopicDeliverMutationResult, fileevents.DeliverMutationResultCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed})
			if err == nil || observed.calls != mutationResultDeliveryLimit {
				t.Fatalf("restart reset transport budget: %v calls=%d", err, observed.calls)
			}
			b, _ := os.ReadFile(filepath.Join(scope.WorkspaceDeliverableDirectory, "target.txt"))
			if string(b) != "after\n" {
				t.Fatal("effect changed after restart")
			}
		})
	}
	// An effect/receipt saved before any submission may be delivered by a new
	// owner, even when the current file has since changed.
	t.Run("saved_before_submission", func(t *testing.T) {
		ctx := context.Background()
		hub := magicevent.NewHub(16)
		t.Cleanup(func() { hub.Terminate(ctx) })
		owner := startDeliveryFileOwner(t, hub)
		observed := observeDeliveryReceipts(t, hub, nil)
		scope, sealed, topic, cmd := mutationDeliveryFixture(t, contract.StepKindFileEdit)
		data, err := sendDelivery(hub, topic, cmd)
		if err != nil {
			t.Fatal(err)
		}
		original := data.(fileevents.ExecuteOperationResult).Receipt
		observed.receipts = map[string]contract.RuntimeOperationReceipt{}
		observed.calls = 0
		// Cut immediately before first attempt publication, using the actual
		// original File result from the preceding handler.
		if e := os.Remove(filepath.Join(scope.WorkspaceInternalDirectory, "file-operation-results", mutationResultKey(scope), "attempt-1")); e != nil {
			t.Fatal(e)
		}
		owner.Teardown(ctx)
		startDeliveryFileOwner(t, hub)
		os.WriteFile(filepath.Join(scope.WorkspaceDeliverableDirectory, "target.txt"), []byte("external\n"), 0600)
		_, err = sendDelivery(hub, fileevents.TopicDeliverMutationResult, fileevents.DeliverMutationResultCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed})
		if err != nil || observed.calls != 1 || !contract.SameRuntimeOperationReceipt(observed.receipts[original.ExecutionKey], original) {
			t.Fatalf("original material recovery failed: %v", err)
		}
		b, _ := os.ReadFile(filepath.Join(scope.WorkspaceDeliverableDirectory, "target.txt"))
		if string(b) != "external\n" {
			t.Fatal("recovery touched file")
		}
	})
}

func TestFileMutationGuardMissingMaterialAndSemanticRejectionFailClosed(t *testing.T) {
	for _, mode := range []string{"guard_only", "rejected", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			owner := startDeliveryFileOwner(t, hub)
			observed := observeDeliveryReceipts(t, hub, func(int, contract.RuntimeOperationReceipt) *cd.Error {
				return &cd.Error{Code: cd.Unexpected, Message: "semantic rejection\noriginal"}
			})
			scope, sealed, topic, cmd := mutationDeliveryFixture(t, contract.StepKindFileEdit)
			if mode != "rejected" {
				s, _, _, e := owner.prepareMutationResult(ctx, fileevents.MutationResultDeliveryV1, scope, sealed.Operation)
				if e != nil {
					t.Fatal(e)
				}
				s.close()
			}
			if mode == "cancelled" {
				c, cancel := context.WithCancel(ctx)
				cancel()
				_, e := hub.Send(magicevent.NewEventWithContext(topic, contract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), c, cmd)).Get()
				if e == nil {
					t.Fatal("cancel accepted")
				}
			}
			_, err := sendDelivery(hub, topic, cmd)
			if err == nil {
				t.Fatal("missing material/rejected was accepted")
			}
			_, err = sendDelivery(hub, fileevents.TopicDeliverMutationResult, fileevents.DeliverMutationResultCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed})
			if err == nil {
				t.Fatal("recovery accepted unavailable result")
			}
			want := 0
			if mode == "rejected" {
				want = 1
			}
			if observed.calls != want {
				t.Fatalf("rejection retried: %d", observed.calls)
			}
			var failure *contract.RuntimeReceiptDeliveryError
			if !errors.As(err, &failure) || failure.Diagnostic == nil || failure.Diagnostic.Validate() != nil {
				t.Fatalf("diagnostic not preserved: %v", err)
			}
			d := failure.Diagnostic
			b, e := os.ReadFile(filepath.Join(scope.WorkspaceInternalDirectory, "file-operation-results", mutationResultKey(scope), filepath.Base(d.DetailsRef)))
			if e != nil {
				t.Fatal(e)
			}
			hash := sha256.Sum256(b)
			if int64(len(b)) != d.DetailsBytes || hex.EncodeToString(hash[:]) != d.DetailsHash {
				t.Fatal("raw diagnostic byte/hash mismatch")
			}
			content, _ := os.ReadFile(filepath.Join(scope.WorkspaceDeliverableDirectory, "target.txt"))
			if want == 0 && string(content) != "before\n" {
				t.Fatal("guard was used as execution permission")
			}
		})
	}
}

func TestFileMutationResultStoreScopeAndRestrictedBoundary(t *testing.T) {
	scope, sealed, _, _ := mutationDeliveryFixture(t, contract.StepKindFileEdit)
	s, err := openMutationResultStore(scope.WorkspaceInternalDirectory, mutationResultKey(scope), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	identity := mutationResultIdentity{fileevents.MutationResultDeliveryV1, scope, sealed.Operation}
	raw, _ := json.Marshal(identity)
	s.write("started.json", raw)
	if err = validateMutationResultIdentity(s, identity); err != nil {
		t.Fatalf("roundtrip identity: %v", err)
	}
	other := identity
	other.Context.RunID = "other"
	if validateMutationResultIdentity(s, other) == nil {
		t.Fatal("cross-run original result accepted")
	}
	root := filepath.Join(scope.WorkspaceInternalDirectory, "file-operation-results", mutationResultKey(scope))
	outside := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(outside, []byte("private"), 0600)
	os.Symlink(outside, filepath.Join(root, "receipt.json"))
	if _, err = s.read("receipt.json"); err == nil {
		t.Fatal("followed symlink")
	}
	if _, err = s.write("oversized", []byte(strings.Repeat("a", mutationResultMaxBytes+1))); err == nil {
		t.Fatal("silent truncation")
	}
	if created, err := s.write("started.json", []byte("replace")); err != nil || created {
		t.Fatal("overwrote immutable identity")
	}
}

func TestFileMutationOriginalDiagnosticBytesAndCapacitySurviveReload(t *testing.T) {
	scope, _, _, _ := mutationDeliveryFixture(t, contract.StepKindFileEdit)
	s, err := openMutationResultStore(scope.WorkspaceInternalDirectory, mutationResultKey(scope), true)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte{'o', 'r', 'i', 'g', '\n', 0, 0xff}
	original := mutationDeliveryFailure(s, scope, "dependencies", errors.New(string(raw)))
	s.close()
	s, err = openMutationResultStore(scope.WorkspaceInternalDirectory, mutationResultKey(scope), false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	var first, reloaded *contract.RuntimeReceiptDeliveryError
	if !errors.As(original, &first) || !errors.As(savedMutationFailure(s, scope), &reloaded) || first.Diagnostic.DetailsRef != reloaded.Diagnostic.DetailsRef || reloaded.Diagnostic.OriginalCode != "file_result_dependencies" {
		t.Fatal("restart lost first failure stage or original byte reference")
	}
	bytes, err := s.read(filepath.Base(reloaded.Diagnostic.DetailsRef))
	if err != nil || string(bytes) != string(raw) {
		t.Fatal("original diagnostic bytes were cleaned or rewritten")
	}
	for i := 1; i <= 8; i++ {
		failure := mutationDeliveryFailure(s, scope, "receipt_commit", errors.New(strings.Repeat("different", i)))
		var detail *contract.RuntimeReceiptDeliveryError
		if !errors.As(failure, &detail) || (i < 8 && detail.Diagnostic.OmittedReason != "") || (i == 8 && detail.Diagnostic.OmittedReason != "diagnostic_capacity_exceeded") {
			t.Fatalf("diagnostic capacity silently truncated or reset at %d", i)
		}
	}
	// Corrupting a referenced raw file must not turn its stored hash into trust.
	path := filepath.Join(scope.WorkspaceInternalDirectory, "file-operation-results", mutationResultKey(scope), filepath.Base(first.Diagnostic.DetailsRef))
	if err = os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if savedMutationFailure(s, scope) != nil {
		t.Fatal("tampered raw diagnostic reused after reload")
	}
}

func TestFileMutationTwoWorkspaceConcurrentResultsRemainIsolated(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	startDeliveryFileOwner(t, hub)
	observed := observeDeliveryReceipts(t, hub, nil)
	var scopes []contract.RuntimeOperationExecutionContext
	var commands []fileevents.ExecuteFileEditOperationCommand
	for _, id := range []string{"ws-alpha", "ws-beta"} {
		scope, sealed, _, _ := mutationDeliveryFixture(t, contract.StepKindFileEdit)
		scope.WorkspaceID = common.WorkspaceID(id)
		for i := range sealed.Operation.Parameters {
			if sealed.Operation.Parameters[i].Name == "newText" {
				body := id + "-after\n"
				sealed.Operation.Parameters[i].String = &body
			}
		}
		var err error
		sealed, err = fileevents.SealRuntimeOperation(sealed.Operation)
		if err != nil {
			t.Fatal(err)
		}
		scope.Execution.ExecutionKey = "exec/" + id + "/run-1/dispatch-1/op-1"
		scope.Execution = fileBizTestExecutionEnvelope(sealed.Operation, scope.Execution)
		scopes = append(scopes, scope)
		commands = append(commands, fileevents.ExecuteFileEditOperationCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed, ResultDeliveryProtocolRef: fileevents.MutationResultDeliveryV1})
	}
	var wg sync.WaitGroup
	for _, cmd := range commands {
		wg.Go(func() {
			if _, err := sendDelivery(hub, fileevents.TopicExecuteFileEditOperation, cmd); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if observed.calls != 2 || len(observed.receipts) != 2 || mutationResultKey(scopes[0]) == mutationResultKey(scopes[1]) {
		t.Fatal("concurrent workspace results collided")
	}
	for _, scope := range scopes {
		r := observed.receipts[scope.Execution.ExecutionKey]
		if r.ExecutionKey != scope.Execution.ExecutionKey || !r.Changed {
			t.Fatal("cross-workspace receipt")
		}
		body, err := os.ReadFile(filepath.Join(scope.WorkspaceDeliverableDirectory, "target.txt"))
		if err != nil || string(body) != string(scope.WorkspaceID)+"-after\n" {
			t.Fatal("workspace effect missing")
		}
	}
}

// The first OS process exits from the receipt consumer, after File has durably
// saved its original receipt and reserved the first submission, but before any
// consumer commit. The next process has no in-memory File or receipt state.
func TestFileMutationOriginalReceiptSurvivesProcessExitBeforeCommit(t *testing.T) {
	scope, sealed, _, _ := mutationDeliveryFixture(t, contract.StepKindFileEdit)
	root := t.TempDir()
	cmd := fileevents.ExecuteFileEditOperationCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed, ResultDeliveryProtocolRef: fileevents.MutationResultDeliveryV1}
	raw, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "command.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"exit_before_commit", "deliver_original"} {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFileMutationReceiptProcessChild$", "-test.timeout=12s")
		child.Env = append(os.Environ(), "WORKORCH_FILE_RECEIPT_PROCESS_ROOT="+root, "WORKORCH_FILE_RECEIPT_PROCESS_PHASE="+phase)
		out, childErr := child.CombinedOutput()
		cancel()
		if childErr != nil {
			t.Fatalf("%s: %v\n%s", phase, childErr, out)
		}
		if phase == "exit_before_commit" {
			body, readErr := os.ReadFile(filepath.Join(scope.WorkspaceDeliverableDirectory, "target.txt"))
			if readErr != nil || string(body) != "after\n" {
				t.Fatalf("first effect missing: %v / %q", readErr, body)
			}
			if err = os.WriteFile(filepath.Join(scope.WorkspaceDeliverableDirectory, "target.txt"), []byte("external\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	original, err := os.ReadFile(filepath.Join(root, "original.json"))
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := os.ReadFile(filepath.Join(root, "recovered.json"))
	if err != nil || string(original) != string(recovered) {
		t.Fatalf("process recovery changed original receipt: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(scope.WorkspaceDeliverableDirectory, "target.txt"))
	if err != nil || string(body) != "external\n" {
		t.Fatalf("receipt recovery re-executed or inferred the current file: %v / %q", err, body)
	}
}

func TestFileMutationReceiptProcessChild(t *testing.T) {
	root, phase := os.Getenv("WORKORCH_FILE_RECEIPT_PROCESS_ROOT"), os.Getenv("WORKORCH_FILE_RECEIPT_PROCESS_PHASE")
	if root == "" {
		return
	}
	raw, err := os.ReadFile(filepath.Join(root, "command.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cmd fileevents.ExecuteFileEditOperationCommand
	if err = json.Unmarshal(raw, &cmd); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	startDeliveryFileOwner(t, hub)
	observed := observeDeliveryReceipts(t, hub, nil)
	switch phase {
	case "exit_before_commit":
		observed.fail = func(_ int, receipt contract.RuntimeOperationReceipt) *cd.Error {
			b, marshalErr := json.Marshal(receipt)
			if marshalErr != nil {
				os.Exit(2)
			}
			if writeErr := os.WriteFile(filepath.Join(root, "original.json"), b, 0600); writeErr != nil {
				os.Exit(3)
			}
			os.Exit(0)
			return nil
		}
		sendDelivery(hub, fileevents.TopicExecuteFileEditOperation, cmd)
		t.Fatal("original receipt callback was not reached")
	case "deliver_original":
		_, callErr := sendDelivery(hub, fileevents.TopicDeliverMutationResult, fileevents.DeliverMutationResultCommand{RuntimeOperationExecutionContext: cmd.RuntimeOperationExecutionContext, SealedOperation: cmd.SealedOperation})
		if callErr != nil || observed.calls != 1 {
			t.Fatalf("new process delivery: %v / %d", callErr, observed.calls)
		}
		receipt, found := observed.receipts[cmd.Execution.ExecutionKey]
		if !found {
			t.Fatal("original receipt was not delivered")
		}
		b, marshalErr := json.Marshal(receipt)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err = os.WriteFile(filepath.Join(root, "recovered.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unexpected child phase %q", phase)
	}
}
