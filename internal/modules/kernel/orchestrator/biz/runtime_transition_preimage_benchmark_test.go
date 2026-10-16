package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This read-only benchmark compares the old independent DTO/aggregate/Commit
// gates to the single-Commit complete-load product, including all transition
// gates. It models neither owner I/O nor a successful live Run.
type runtimeArchivePreimageRepository struct {
	record runtimecontract.RuntimeStateRecord
	loads  int
	saves  int
}

func (r *runtimeArchivePreimageRepository) Load(_ context.Context, _ common.WorkspaceID, _ common.RunID) (runtime.RuntimeAggregate, error) {
	r.loads++
	if err := r.record.Validate(); err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	aggregate := runtimeAggregateFromRecord(r.record)
	return aggregate, aggregate.Validate()
}

func (r *runtimeArchivePreimageRepository) Save(_ context.Context, _ runtime.RuntimeAggregate, _ uint64) error {
	r.saves++
	return nil
}

type runtimeArchiveCompletePreimageRepository struct {
	*runtimeArchivePreimageRepository
	joint bool
}

type runtimeArchiveTransactionRepository struct {
	runtimeArchiveCompletePreimageRepository
}

func (r runtimeArchiveTransactionRepository) LoadOwnerStateOriginal(_ context.Context, _ common.WorkspaceID, _ common.RunID) (runtimecontract.RuntimeStateRecord, runtime.RuntimeAggregate, error) {
	r.loads++
	return r.record, runtimeAggregateFromRecord(r.record), nil
}

// Includes the initial Load, concrete private-event reducer, complete after-
// image, real Commit reload interface, every transition gate and CAS call.
// Both arms use the same input and event; owner I/O is not simulated as CPU.
func BenchmarkRuntimePublicArchiveOwnerStateTransaction(b *testing.B) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		b.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var runs map[string]struct {
		RuntimeState runtimecontract.RuntimeStateRecord
	}
	if err := json.Unmarshal(body, &runs); err != nil || len(runs) == 0 {
		b.Fatalf("invalid archive: %v", err)
	}
	for id, run := range runs {
		for _, name := range []string{"independent_event", "cold_transaction", "joint_transaction"} {
			joint := name != "independent_event"
			b.Run(id+"/"+name, func(b *testing.B) {
				record := run.RuntimeState.Clone()
				original := record.Clone()
				base := &runtimeArchivePreimageRepository{record: record}
				complete := runtimeArchiveCompletePreimageRepository{runtimeArchivePreimageRepository: base, joint: true}
				var repository runtime.RuntimeStateRepository = complete
				if joint {
					repository = runtimeArchiveTransactionRepository{complete}
				}
				controller, err := runtime.NewRuntimeController(repository)
				if err != nil {
					b.Fatal(err)
				}
				failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed", Signature: "benchmark-only", Message: "exact private-event comparison"}
				event := runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventFailureObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: record.Facts.WorkspaceID, RunID: record.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{FailureFact: &failure}}
				b.ReportAllocs()
				for b.Loop() {
					if name == "cold_transaction" {
						controller, err = runtime.NewRuntimeController(repository)
						if err != nil {
							b.Fatal(err)
						}
						controller.ReleaseHistoricalCompilation()
					}
					if _, err := controller.ApplyInternalEvent(context.Background(), event); err != nil {
						b.Fatal(err)
					}
				}
				if base.loads != 2*b.N || base.saves != b.N || !reflect.DeepEqual(record, original) {
					b.Fatalf("read/CAS/original mismatch: %d/%d", base.loads, base.saves)
				}
			})
		}
	}
}

func (r runtimeArchiveCompletePreimageRepository) LoadTransitionPreimage(_ context.Context, request runtime.RuntimeTransitionPreimageRequest) (runtime.RuntimeTransitionPreimage, error) {
	r.loads++
	if r.joint {
		return request.CompileOwnerRecord(r.record, runtimeAggregateFromRecord(r.record))
	}
	if err := r.record.Validate(); err != nil {
		return runtime.RuntimeTransitionPreimage{}, err
	}
	return request.Compile(runtimeAggregateFromRecord(r.record))
}

func BenchmarkRuntimePublicArchiveSingleCommitPreimage(b *testing.B) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		b.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var runs map[string]struct {
		RuntimeState runtimecontract.RuntimeStateRecord
	}
	if err := json.Unmarshal(body, &runs); err != nil || len(runs) == 0 {
		b.Fatalf("invalid or empty archive: %v", err)
	}
	for id, run := range runs {
		for _, name := range []string{"independent", "single_commit", "joint_reload"} {
			b.Run(id+"/"+name, func(b *testing.B) {
				record := run.RuntimeState.Clone()
				original := record.Clone()
				next := runtimeAggregateFromRecord(record)
				next.StateRevision++
				base := &runtimeArchivePreimageRepository{record: record}
				var repository runtime.RuntimeStateRepository = base
				if name != "independent" {
					repository = runtimeArchiveCompletePreimageRepository{runtimeArchivePreimageRepository: base, joint: name == "joint_reload"}
				}
				coordinator, err := runtime.NewRuntimeTransitionCoordinator(repository)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				for b.Loop() {
					if err := coordinator.Commit(context.Background(), next, record.StateRevision); err != nil {
						b.Fatal(err)
					}
				}
				if base.loads != b.N || base.saves != b.N || !reflect.DeepEqual(record, original) {
					b.Fatal("load/CAS count or original archive changed")
				}
			})
		}
	}
}
