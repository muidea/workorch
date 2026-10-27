package repository_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func executionReceiptRepositorySource() *runmodel.Run {
	b := runtimecontract.RuntimeBootstrap{Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal", DoDID: "dod", Revision: "def", StartupInputHash: "startup"}}
	b = b.Clone()
	return &runmodel.Run{WorkspaceID: "ws", State: common.RunStateCompleted, GoalID: "goal", DoDID: "dod", DefinitionRevision: "def", StartupInputHash: "startup", ActivationRequestID: "request", RuntimeBootstrap: &b}
}

func TestExecutionReceiptRepositoriesPreserveOriginalResponseAndRejectHistoryMutation(t *testing.T) {
	for _, backend := range []string{"memory", "file", "scoped"} {
		t.Run(backend, func(t *testing.T) {
			root := t.TempDir()
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws")
			open := func() repository.RunRepository {
				switch backend {
				case "file":
					return filerepo.NewRunRepository(filepath.Join(root, "runs.json"))
				case "scoped":
					return workorchrepo.NewWorkspaceScopedFileRepositorySet(root).Runs
				default:
					return memory.NewRunRepository()
				}
			}
			runs := open()
			read := func() *runmodel.WorkspaceExecutionIntentReceipt {
				t.Helper()
				r, err := runs.(repository.RunExecutionIntentReceiptReader).ReadWorkspaceExecutionIntentReceipt(ctx, "ws", "request")
				if err != nil {
					t.Fatal(err)
				}
				return r
			}
			if read() != nil {
				t.Fatal("empty history was not unknown")
			}
			item := executionReceiptRepositorySource()
			if err := runs.CreateNextWorkspaceRun(ctx, item); err != nil {
				t.Fatal(err)
			}
			if read() != nil {
				t.Fatal("legacy lifecycle fabricated receipt")
			}
			hash, err := runmodel.ExecutionIntentPayloadHash(item.WorkspaceID, item.ActivationRequestID, *item.RuntimeBootstrap)
			if err != nil {
				t.Fatal(err)
			}
			r := runmodel.WorkspaceExecutionIntentReceipt{Version: runmodel.WorkspaceExecutionIntentReceiptVersion, PayloadHash: hash, RecordedAt: time.Now().UTC(), Admission: runtimecontract.RuntimeExecutionIntentAdmission{
				WorkspaceID: "ws", RunID: item.ID, RequestID: "request", Kind: runtimecontract.RuntimeExecutionIntentRestart,
				Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
			}}
			item.ExecutionIntentReceipts = []runmodel.WorkspaceExecutionIntentReceipt{r}
			if runs.Update(ctx, item) == nil {
				t.Fatal("Update appended receipt without CAS")
			}
			if runs.CompareAndSwap(ctx, item, item.Revision+1) == nil {
				t.Fatal("stale revision appended receipt")
			}
			if err := runs.CompareAndSwap(ctx, item, item.Revision); err != nil {
				t.Fatal(err)
			}
			item.ExecutionIntentReceipts[0].PayloadHash = "caller mutated"
			if backend != "memory" {
				runs = open()
			}
			if got := read(); got == nil || !reflect.DeepEqual(*got, r) {
				t.Fatalf("reopen lost original: %+v", got)
			}
			got := read()
			got.Admission.RequestID = "caller mutated"
			if read().Admission.RequestID != "request" {
				t.Fatal("read aliases owner history")
			}
			for _, mutation := range []string{"erase", "rewrite", "timestamp", "scope", "duplicate"} {
				next, err := runs.GetByWorkspaceID(ctx, "ws", item.ID)
				if err != nil {
					t.Fatal(err)
				}
				switch mutation {
				case "erase":
					next.ExecutionIntentReceipts = nil
				case "rewrite":
					next.ExecutionIntentReceipts[0].PayloadHash = strings.Repeat("b", 64)
				case "timestamp":
					next.ExecutionIntentReceipts[0].RecordedAt = next.ExecutionIntentReceipts[0].RecordedAt.Add(time.Second)
				case "scope":
					next.ExecutionIntentReceipts[0].Admission.WorkspaceID = "other"
				case "duplicate":
					next.ExecutionIntentReceipts = append(next.ExecutionIntentReceipts, r)
				}
				if runs.Update(ctx, next) == nil || runs.CompareAndSwap(ctx, next, next.Revision) == nil {
					t.Fatalf("%s changed receipt history", mutation)
				}
			}
			next, _ := runs.GetByWorkspaceID(ctx, "ws", item.ID)
			next.FailureReason = "unrelated metadata"
			if err := runs.CompareAndSwap(ctx, next, next.Revision); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*read(), r) {
				t.Fatal("metadata changed receipt")
			}
			wrongScope := common.ContextWithRuntimeWorkspaceID(context.Background(), "other")
			reader := runs.(repository.RunExecutionIntentReceiptReader)
			if _, err := reader.ReadWorkspaceExecutionIntentReceipt(wrongScope, "ws", "request"); !errors.Is(err, common.ErrConflict) {
				t.Fatal("wrong context accepted")
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if _, err := reader.ReadWorkspaceExecutionIntentReceipt(cancelled, "ws", "request"); !errors.Is(err, context.Canceled) {
				t.Fatal("cancelled read accepted")
			}
			if err := runs.Delete(ctx, item.ID); err != nil {
				t.Fatal(err)
			}
			if backend != "memory" {
				runs = open()
			}
			if read() != nil {
				t.Fatal("missing removed record fabricated receipt")
			}
			h, err := runs.(repository.RunPreparationHistoryReader).ObserveWorkspacePreparationHistory(ctx, "ws")
			if err != nil || h.GenerationHead != 1 {
				t.Fatal("absence erased durable historical watermark")
			}
		})
	}
}

func TestExecutionReceiptFileRawCorruptionIsRejectedWithoutRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.json")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws")
	runs := filerepo.NewRunRepository(path)
	item := executionReceiptRepositorySource()
	if err := runs.CreateNextWorkspaceRun(ctx, item); err != nil {
		t.Fatal(err)
	}
	hash, err := runmodel.ExecutionIntentPayloadHash(item.WorkspaceID, item.ActivationRequestID, *item.RuntimeBootstrap)
	if err != nil {
		t.Fatal(err)
	}
	item.ExecutionIntentReceipts = []runmodel.WorkspaceExecutionIntentReceipt{{Version: runmodel.WorkspaceExecutionIntentReceiptVersion, PayloadHash: hash, RecordedAt: time.Now().UTC(), Admission: runtimecontract.RuntimeExecutionIntentAdmission{
		WorkspaceID: "ws", RunID: item.ID, RequestID: "request", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
	}}}
	if err := runs.CompareAndSwap(ctx, item, item.Revision); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := strings.Replace(string(bytes), runmodel.WorkspaceExecutionIntentReceiptVersion, "workorch-execution-intent-receipt/unknown", 1)
	if corrupt == string(bytes) {
		t.Fatal("fixture did not corrupt receipt")
	}
	// Use a cold physical coordinate: file repositories share an owner-local
	// cache for a path, so constructing a second handle is not a disk reload.
	coldPath := filepath.Join(t.TempDir(), "runs.json")
	head, err := os.ReadFile(path + ".workspace-generations")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coldPath+".workspace-generations", head, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coldPath, []byte(corrupt), 0600); err != nil {
		t.Fatal(err)
	}
	reader := filerepo.NewRunRepository(coldPath)
	if _, err := reader.ReadWorkspaceExecutionIntentReceipt(ctx, "ws", "request"); err == nil {
		t.Fatal("malformed history became unknown")
	}
	if _, err := reader.List(ctx); err == nil {
		t.Fatal("malformed history silently normalized")
	}
	after, _ := os.ReadFile(coldPath)
	if string(after) != corrupt {
		t.Fatal("read repaired persistent history")
	}
}
