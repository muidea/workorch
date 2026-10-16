package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOwnerQueryRealReadRevisionAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runs.json")
	runs := filerepository.NewRunRepository(path)
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	start := func() *runmodule.Module {
		t.Helper()
		m := runmodule.New()
		if err := m.Configure(runOwnerTestDependencies(filerepository.NewRunRepository(path), nil, nil, hub)); err != nil {
			t.Fatal(err)
		}
		if err := m.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = m.Stop(ctx) })
		return m
	}
	m := start()
	repo := runtimeStateRepository{hub: hub}
	before := runtimeAggregate(t, 1)
	if err := repo.Save(ctx, before, 0); err != nil {
		t.Fatal(err)
	}
	// Compare with the independent ordinary reload, including the established
	// wire conversion's nil/empty representation, not the pre-persistence value.
	expected, err := repo.Load(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	counted := &runtimeOwnerTransactionFaultRepository{runtimeStateRepository: repo}
	c, _ := runtime.NewRuntimeController(counted)
	for range 2 {
		got, err := c.Load(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
		if err != nil || !reflect.DeepEqual(got, expected) {
			t.Fatalf("real query: %v", err)
		}
	}
	next := expected.CloneRaw()
	next.StateRevision++
	if err := repo.Save(ctx, next, before.StateRevision); err != nil {
		t.Fatal(err)
	}
	expected, err = repo.Load(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Load(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
	if err != nil || !reflect.DeepEqual(got, expected) || counted.loads != 3 || counted.saves != 0 || counted.rereads != 0 {
		t.Fatalf("query reused owner state: %v", err)
	}
	if err := repo.Save(ctx, next, before.StateRevision); err == nil {
		t.Fatal("query bypassed stale CAS")
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	c.ReleaseHistoricalCompilation()
	start()
	fresh, _ := runtime.NewRuntimeController(runtimeStateRepository{hub: hub})
	got, err = fresh.Load(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
	if err != nil || !reflect.DeepEqual(got, expected) {
		t.Fatalf("fresh owner/controller query: %v", err)
	}
}

type runtimeQueryOracleRepository struct {
	*runtimeArchivePreimageRepository
	converted runtime.RuntimeAggregate
}

func (r *runtimeQueryOracleRepository) LoadOwnerStateOriginal(context.Context, common.WorkspaceID, common.RunID) (runtimecontract.RuntimeStateRecord, runtime.RuntimeAggregate, error) {
	r.loads++
	return r.record.Clone(), r.converted.CloneRaw(), nil
}

func TestRuntimeOwnerQueryReadOnlyArchiveOracle(t *testing.T) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct {
		RuntimeState runtimecontract.RuntimeStateRecord
	}
	if err := json.Unmarshal(body, &runs); err != nil || len(runs) == 0 {
		t.Fatalf("invalid archive: %v", err)
	}
	for _, run := range runs {
		raw := run.RuntimeState
		a := runtimeAggregateFromRecord(raw)
		r := &runtimeQueryOracleRepository{&runtimeArchivePreimageRepository{record: raw}, a}
		c, _ := runtime.NewRuntimeController(r)
		for _, mutate := range []func(*runtimecontract.RuntimeStateRecord, *runtime.RuntimeAggregate){
			func(*runtimecontract.RuntimeStateRecord, *runtime.RuntimeAggregate) {},
			func(r *runtimecontract.RuntimeStateRecord, _ *runtime.RuntimeAggregate) { r.ResultFeedbackVersion = 0 },
			func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
				a.OperationReceipts[0].Fence++
			},
			func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
				a.SemanticPlanning.History[0].Snapshot.Manifest.OwnerCatalogs[0].SnapshotID = "same hash raw drift"
			},
			func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
				a.ImplementationWork.Rounds[0].Hash = "drift"
			},
			func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
				a.ExecutionBudget.Policy.PolicyRevision = "drift"
			},
			func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
				a.DeliveryAcknowledgements = []runtimecontract.RuntimeDeliveryAcknowledgement{{}}
			},
			func(r *runtimecontract.RuntimeStateRecord, _ *runtime.RuntimeAggregate) {
				r.CompletedDispatches[0].CandidateHash = "same-id raw drift"
			},
			func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
				a.CompletedDispatches[0].DispatchAuthorityHash = "same-id raw drift"
			},
			func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
				for i := range a.CompletedDispatches {
					if len(a.CompletedDispatches[i].OperationAdmissibilities) != 0 {
						a.CompletedDispatches[i].OperationAdmissibilities[0].ObservedRevision = "same-snapshot raw drift"
						return
					}
				}
				t.Fatal("archive has no sealed catalog")
			},
			func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
				a.OperationReceipts[0].BeforeHash = "same-execution-key raw drift"
			},
			func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
				a.CompletedDispatches = append(a.CompletedDispatches, a.CompletedDispatches[0])
			},
			func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
				a.OperationReceipts = append(a.OperationReceipts, a.OperationReceipts[0])
			},
			func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
				a.ProcessedEventIDs = nil
			},
		} {
			// Warm the same controller, then change the real raw/converted input.
			r.record, r.converted = raw.Clone(), a.CloneRaw()
			if _, err := c.Load(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID); err != nil {
				t.Fatal(err)
			}
			mutate(&r.record, &r.converted)
			got, err := c.Load(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
			want := referenceOwnerStateReload(r.record, r.converted)
			if fmt.Sprint(err) != fmt.Sprint(want) {
				t.Fatalf("first error: %v != %v", err, want)
			}
			if err != nil && !reflect.DeepEqual(got, runtime.RuntimeAggregate{}) {
				t.Fatal("partial result")
			}
		}
		if r.saves != 0 {
			t.Fatal("query reached CAS")
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(body, after) {
		t.Fatal("archive changed")
	}
}

func BenchmarkRuntimePublicArchiveOwnerQuery(b *testing.B) {
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
		for _, name := range []string{"independent", "cold_joint", "shared_joint"} {
			b.Run(id+"/"+name, func(b *testing.B) {
				base := &runtimeArchivePreimageRepository{record: run.RuntimeState.Clone()}
				var repo runtime.RuntimeStateRepository = base
				if name != "independent" {
					repo = runtimeArchiveTransactionRepository{runtimeArchiveCompletePreimageRepository{runtimeArchivePreimageRepository: base}}
				}
				c, _ := runtime.NewRuntimeController(repo)
				if name == "cold_joint" {
					c.ReleaseHistoricalCompilation()
				}
				b.ReportAllocs()
				for b.Loop() {
					if _, err := c.Load(context.Background(), run.RuntimeState.Facts.WorkspaceID, run.RuntimeState.Facts.RunID); err != nil {
						b.Fatal(err)
					}
				}
				if base.loads != b.N || base.saves != 0 {
					b.Fatal("query read/CAS count")
				}
			})
		}
	}
}
