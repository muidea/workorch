package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	event "github.com/muidea/magicCommon/event"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Uses the complete fixed snapshot through the real typed Run owner read.
// All writes below initialize isolated benchmark storage, never the archive.
func BenchmarkRuntimeCommittedOwnerRead(b *testing.B) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		b.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var archive map[string]runmodel.Run
	if err := json.Unmarshal(body, &archive); err != nil || len(archive) != 1 {
		b.Fatalf("single-run archive: %v", err)
	}
	for _, raw := range archive {
		b.Run("complete_snapshot", func(b *testing.B) {
			ctx := context.Background()
			stored := runmodel.CloneRun(&raw)
			stored.WorkspaceRunGeneration = 0
			runs := memory.NewRunRepository()
			if err := runs.Create(ctx, stored); err != nil {
				b.Fatal(err)
			}
			hub := event.NewHub(16)
			b.Cleanup(func() { hub.Terminate(ctx) })
			owner := runmodule.New()
			if err := owner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
				b.Fatal(err)
			}
			if err := owner.Start(); err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = owner.Stop(ctx) })
			controller, _ := runtime.NewRuntimeController(runtimeStateRepository{hub: hub})
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				a, err := controller.Load(ctx, common.WorkspaceID(stored.WorkspaceID), stored.ID)
				if err != nil || a.StateRevision != raw.RuntimeState.StateRevision {
					b.Fatalf("formal read: %v", err)
				}
			}
		})
	}
}

// Measures the complete private event: real typed owner read, isolated original,
// reduction, postimage/transition checks and real opaque CAS. Fixture setup and
// owner lifecycle are excluded; no provider or capability effect is executed.
func BenchmarkRuntimeCommittedOwnerEvent(b *testing.B) {
	benchmarkRuntimeCommittedOwnerEvent(b, false, false)
}

// A nonterminal scheduling CAS isolates the ordinary persistence path from the
// terminal lifecycle fence calculated by the failure benchmark above.
func BenchmarkRuntimeCommittedOwnerLeaseEvent(b *testing.B) {
	benchmarkRuntimeCommittedOwnerEvent(b, true, false)
}

func BenchmarkRuntimeCommittedOwnerFileLeaseEvent(b *testing.B) {
	benchmarkRuntimeCommittedOwnerEvent(b, true, true)
}

func benchmarkRuntimeCommittedOwnerEvent(b *testing.B, scheduleLease, fileBacked bool) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		b.Skip("explicit fixed archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var archive map[string]runmodel.Run
	if err := json.Unmarshal(body, &archive); err != nil || len(archive) != 1 {
		b.Fatalf("single-run archive: %v", err)
	}
	for _, raw := range archive {
		original := runmodel.CloneRun(&raw)
		b.ReportAllocs()
		b.ResetTimer()
		b.StopTimer()
		for i := 0; i < b.N; i++ {
			ctx := context.Background()
			stored := runmodel.CloneRun(&raw)
			stored.WorkspaceRunGeneration = 0
			var runs repository.RunRepository = memory.NewRunRepository()
			var storage string
			if fileBacked {
				storage, err = os.MkdirTemp("/tmp/workorch-validation/", "owner-file-lease-")
				if err != nil {
					b.Fatal(err)
				}
				runs = filerepo.NewRunRepository(filepath.Join(storage, "runs.json"))
			}
			if err := runs.Create(ctx, stored); err != nil {
				b.Fatal(err)
			}
			hub := event.NewHub(16)
			owner := runmodule.New()
			if err := owner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
				b.Fatal(err)
			}
			if err := owner.Start(); err != nil {
				b.Fatal(err)
			}
			controller, _ := runtime.NewRuntimeController(runtimeStateRepository{hub: hub})
			failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed", Signature: "fixed-owner-event", Message: "isolated fixed-snapshot event benchmark"}
			input := runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventFailureObserved,
				Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: stored.WorkspaceID, RunID: stored.ID},
				Authority: &runtime.RuntimeInternalAuthorityPayload{FailureFact: &failure}}
			if scheduleLease {
				input.Kind = runtime.RuntimeInternalEventDriveLeaseScheduled
				input.Authority = nil
				input.Delivery = &runtime.RuntimeInternalDeliveryPayload{DriveLease: &runtimecontract.RuntimeDriveLease{
					WorkspaceID: stored.WorkspaceID, RunID: stored.ID,
					Reason:           runtimecontract.RuntimeDriveLeaseReasonPersistedPending,
					FailureSignature: "fixed-owner-scheduling-event", DueAt: "2026-11-05T00:00:00Z",
				}}
			}
			b.StartTimer()
			result, eventErr := controller.ApplyInternalEvent(ctx, input)
			b.StopTimer()
			if err := owner.Stop(ctx); err != nil {
				b.Fatal(err)
			}
			hub.Terminate(ctx)
			if fileBacked {
				filerepo.EvictMapFileCache(filepath.Join(storage, "runs.json"))
				filerepo.EvictMapFileCache(filepath.Join(storage, "runs.json.workspace-generations"))
				if err := os.RemoveAll(storage); err != nil {
					b.Fatal(err)
				}
			}
			if eventErr != nil || result.Aggregate.StateRevision != raw.RuntimeState.StateRevision+1 {
				b.Fatalf("complete owner event: %v", eventErr)
			}
			if scheduleLease && (!result.LeaseChanged || result.DriveLease == nil ||
				!reflect.DeepEqual(result.Aggregate.Facts, raw.RuntimeState.Facts) ||
				!reflect.DeepEqual(result.Aggregate.Decision, raw.RuntimeState.Decision)) {
				b.Fatal("scheduling benchmark must persist a lease without changing business authority")
			}
			if !reflect.DeepEqual(original, &raw) {
				b.Fatal("benchmark changed fixed original")
			}
		}
	}
}
