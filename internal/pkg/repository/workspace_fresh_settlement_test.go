package repository_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFreshSettlementRepositoriesPreserveHistoryAndRejectRewrite(t *testing.T) {
	for _, backend := range []string{"memory", "file", "scoped"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			var repo repository.WorkspaceRepository
			switch backend {
			case "file":
				repo = file.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
			case "scoped":
				repo = workorchrepo.NewWorkspaceScopedFileRepositorySet(root).Workspaces
			default:
				repo = memory.NewWorkspaceRepository()
			}
			current := &workspace.Workspace{ID: "ws", PreparationFence: &workspace.WorkspacePreparationFence{
				Version: workspace.WorkspacePreparationFenceVersion, WorkspaceID: "ws", CreationEpoch: strings.Repeat("a", 32),
			}}
			if err := repo.Create(ctx, current); err != nil {
				t.Fatal(err)
			}
			entry := workspace.WorkspaceFreshIntentSettlement{
				Reservation: workspace.WorkspaceFreshIntentReservation{RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, StartupInputHash: "startup", PayloadHash: strings.Repeat("b", 64), RunPayloadHash: strings.Repeat("c", 64)},
				Receipt: runmodel.WorkspaceExecutionIntentReceipt{Version: runmodel.WorkspaceExecutionIntentReceiptVersion, PayloadHash: strings.Repeat("c", 64), RecordedAt: time.Now().UTC(),
					Admission: runtimecontract.RuntimeExecutionIntentAdmission{WorkspaceID: "ws", RequestID: "original", RunID: "run", Kind: runtimecontract.RuntimeExecutionIntentRestart,
						Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated}},
			}
			current.PreparationFence.Pending = &entry.Reservation
			current.PreparationFence.PreparationEpoch = 1
			if err := repo.CompareAndSwap(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			current.PreparationFence.Pending = nil
			current.PreparationFence.Settled = []workspace.WorkspaceFreshIntentSettlement{entry}
			if err := repo.Update(ctx, current); err == nil {
				t.Fatal("ordinary metadata update settled an intent")
			}
			if err := repo.CompareAndSwap(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			baseline, _ := repo.Get(ctx, "ws")
			current.PreparationFence.Settled[0].Receipt.PayloadHash = "caller-mutated"
			for _, source := range []string{"get", "list"} {
				read, err := repo.Get(ctx, "ws")
				if source == "list" {
					items, listErr := repo.List(ctx)
					if listErr != nil || len(items) != 1 {
						t.Fatal("history list", listErr)
					}
					read = items[0]
				}
				if err != nil || !reflect.DeepEqual(read, baseline) {
					t.Fatal("stored receipt aliases caller", err)
				}
				read.PreparationFence.Settled[0].Receipt.Admission.RunID = "read-mutated"
			}
			for _, mutation := range []string{"erase", "rewrite_receipt", "rewrite_payload", "skip_pending", "reopen"} {
				read, _ := repo.Get(ctx, "ws")
				switch mutation {
				case "erase":
					read.PreparationFence.Settled = nil
					read.PreparationFence.PreparationEpoch = 0
				case "rewrite_receipt":
					read.PreparationFence.Settled[0].Receipt.Admission.RunID = "changed"
				case "rewrite_payload":
					read.PreparationFence.Settled[0].Reservation.PayloadHash = strings.Repeat("d", 64)
				case "skip_pending":
					other := entry
					other.Reservation.RequestID = "other"
					other.Receipt.Admission.RequestID = "other"
					read.PreparationFence.Settled = append(read.PreparationFence.Settled, other)
					read.PreparationFence.PreparationEpoch++
				case "reopen":
					read.PreparationFence.Pending = &entry.Reservation
					read.PreparationFence.Settled = nil
				}
				if repo.Update(ctx, read) == nil || repo.CompareAndSwap(ctx, read, read.Revision) == nil {
					t.Fatalf("%s rewrote original history", mutation)
				}
			}
			after, _ := repo.Get(ctx, "ws")
			if !reflect.DeepEqual(baseline, after) {
				t.Fatal("rejected writes changed storage")
			}
			after.Title = "metadata"
			if err := repo.Update(ctx, after); err != nil {
				t.Fatal(err)
			}
			read, _ := repo.Get(ctx, "ws")
			if !reflect.DeepEqual(read.PreparationFence, baseline.PreparationFence) {
				t.Fatal("metadata lost settled history")
			}
		})
	}
}

func TestFreshSettlementRawCorruptionFailsClosedWithoutRepair(t *testing.T) {
	entry := workspace.WorkspaceFreshIntentSettlement{
		Reservation: workspace.WorkspaceFreshIntentReservation{RequestID: "original", Kind: runtimecontract.RuntimeExecutionIntentRestart, StartupInputHash: "startup", PayloadHash: strings.Repeat("b", 64), RunPayloadHash: strings.Repeat("c", 64)},
		Receipt: runmodel.WorkspaceExecutionIntentReceipt{Version: runmodel.WorkspaceExecutionIntentReceiptVersion, PayloadHash: strings.Repeat("c", 64), RecordedAt: time.Now().UTC(),
			Admission: runtimecontract.RuntimeExecutionIntentAdmission{WorkspaceID: "ws", RequestID: "original", RunID: "run", Kind: runtimecontract.RuntimeExecutionIntentRestart,
				Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated}},
	}
	for _, scenario := range []string{"lost_history", "duplicate", "pending_collision", "receipt_scope", "receipt_request", "receipt_version", "unknown_field"} {
		t.Run(scenario, func(t *testing.T) {
			fence := &workspace.WorkspacePreparationFence{Version: workspace.WorkspacePreparationFenceVersion, WorkspaceID: "ws", CreationEpoch: strings.Repeat("a", 32), PreparationEpoch: 1,
				Settled: []workspace.WorkspaceFreshIntentSettlement{entry}}
			switch scenario {
			case "lost_history":
				fence.Settled = nil
			case "duplicate":
				fence.Settled = append(fence.Settled, entry)
				fence.PreparationEpoch++
			case "pending_collision":
				pending := entry.Reservation
				fence.Pending = &pending
				fence.PreparationEpoch++
			case "receipt_scope":
				fence.Settled[0].Receipt.Admission.WorkspaceID = "other"
			case "receipt_request":
				fence.Settled[0].Receipt.Admission.RequestID = "other"
			case "receipt_version":
				fence.Settled[0].Receipt.Version = "unknown"
			}
			encoded, err := json.Marshal(&workspace.Workspace{ID: "ws", Revision: 1, PreparationFence: fence})
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "unknown_field" {
				encoded = []byte(strings.Replace(string(encoded), `"Receipt":{`, `"Receipt":{"Trusted":true,`, 1))
			}
			before := append(append([]byte(`{"ws":`), encoded...), '}')
			path := filepath.Join(t.TempDir(), "workspaces.json")
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			repo := file.NewWorkspaceRepository(path)
			if _, err := repo.Get(context.Background(), "ws"); err == nil {
				t.Fatal("corrupt raw settlement accepted")
			}
			if _, err := repo.List(context.Background()); err == nil {
				t.Fatal("corrupt raw settlement listed")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatal("raw read repaired corrupt history", err)
			}
		})
	}
}
