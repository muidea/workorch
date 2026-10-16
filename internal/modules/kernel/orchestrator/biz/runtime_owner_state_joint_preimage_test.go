package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The reference deliberately completes the two independent public gates; it
// cannot receive the joint compiler's private semantic/historical context.
func referenceOwnerStateReload(record runtimecontract.RuntimeStateRecord, converted runtime.RuntimeAggregate) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("load Runtime state: %w", err)
	}
	return converted.Validate()
}

type runtimeJointPreimageOracleRepository struct {
	runtimeArchivePreimageRepository
	converted runtime.RuntimeAggregate
	seen      error
	mode      string
}

func (r *runtimeJointPreimageOracleRepository) LoadTransitionPreimage(_ context.Context, request runtime.RuntimeTransitionPreimageRequest) (runtime.RuntimeTransitionPreimage, error) {
	r.loads++
	product, err := request.CompileOwnerRecord(r.record, r.converted)
	r.seen = err
	if err != nil {
		return product, err
	}
	if r.mode == "recompile" {
		if _, err := request.CompileOwnerRecord(r.record, r.converted); err == nil {
			return runtime.RuntimeTransitionPreimage{}, fmt.Errorf("duplicate joint compile accepted")
		}
	}
	if r.mode == "serialized" {
		wire, _ := json.Marshal(product)
		product = runtime.RuntimeTransitionPreimage{}
		if err := json.Unmarshal(wire, &product); err != nil {
			return product, err
		}
	}
	return product, nil
}

func assertOwnerStateJointPreimage(t testing.TB, raw runtimecontract.RuntimeStateRecord, converted runtime.RuntimeAggregate, next runtime.RuntimeAggregate) {
	t.Helper()
	originalRaw, originalConverted := raw.Clone(), converted.CloneRaw()
	want := referenceOwnerStateReload(raw.Clone(), converted.CloneRaw())
	for range 2 { // A later call must not inherit the preceding successful gate.
		repository := &runtimeJointPreimageOracleRepository{runtimeArchivePreimageRepository: runtimeArchivePreimageRepository{record: raw}, converted: converted}
		coordinator, err := runtime.NewRuntimeTransitionCoordinator(repository)
		if err != nil {
			t.Fatal(err)
		}
		_ = coordinator.Commit(context.Background(), next, next.StateRevision-1)
		if repository.loads != 1 || fmt.Sprint(repository.seen) != fmt.Sprint(want) {
			t.Fatalf("joint gate first error differs: got=%v want=%v loads=%d", repository.seen, want, repository.loads)
		}
		if want != nil && repository.saves != 0 {
			t.Fatal("rejected raw/converted preimage reached CAS")
		}
	}
	if !reflect.DeepEqual(raw, originalRaw) || !reflect.DeepEqual(converted, originalConverted) {
		t.Fatal("joint compiler changed raw DTO or converted aggregate")
	}
}

func TestRuntimeOwnerStateJointPreimageCompleteOrderAndDetachedInputs(t *testing.T) {
	base := runtimeAggregate(t, 4)
	raw := runtimeStateRecordFromAggregate(base)
	next := base.CloneRaw()
	next.StateRevision++
	assertOwnerStateJointPreimage(t, raw, base, next)
	for _, mutate := range []func(*runtimecontract.RuntimeStateRecord, *runtime.RuntimeAggregate){
		func(r *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
			r.ResultFeedbackVersion = 0
			a.ResultFeedbackVersion = 0
		},
		func(r *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
			r.Facts.WorkspaceID = ""
			a.EvidenceSequence++
		},
		func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) { a.ResultFeedbackVersion = 0 },
		func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) { a.EvidenceSequence++ },
		func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
			a.Facts.DefinitionRevision = "raw\x00变化"
		},
		func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
			a.SemanticPlanning.DefinitionSeed.PolicyHash = "same-hash-raw-drift"
		},
		func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
			a.Decision.AuthoritySnapshot.RunID = "wrong-scope"
		},
		func(_ *runtimecontract.RuntimeStateRecord, a *runtime.RuntimeAggregate) {
			a.ExecutionBudget.Policy.PolicyRevision = "raw-budget-drift"
		},
	} {
		r, a := raw.Clone(), base.CloneRaw()
		mutate(&r, &a)
		assertOwnerStateJointPreimage(t, r, a, next)
	}
	for _, mode := range []string{"recompile", "serialized"} {
		repository := &runtimeJointPreimageOracleRepository{runtimeArchivePreimageRepository: runtimeArchivePreimageRepository{record: raw}, converted: base, mode: mode}
		coordinator, _ := runtime.NewRuntimeTransitionCoordinator(repository)
		err := coordinator.Commit(context.Background(), next, base.StateRevision)
		if (err == nil) != (mode == "recompile") || repository.saves != map[string]int{"recompile": 1, "serialized": 0}[mode] {
			t.Fatalf("joint one-shot product mode=%s err=%v saves=%d", mode, err, repository.saves)
		}
	}
}

func TestRuntimeOwnerStateJointPreimageReadOnlyPublicArchive(t *testing.T) {
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
		base := runtimeAggregateFromRecord(raw)
		next := base.CloneRaw()
		next.StateRevision++
		if err := referenceOwnerStateReload(raw, base); err != nil {
			t.Fatal("original archive not valid", err)
		}
		assertOwnerStateJointPreimage(t, raw, base, next)
		for _, mutate := range []func(*runtime.RuntimeAggregate){
			func(a *runtime.RuntimeAggregate) { a.OperationReceipts[0].Fence++ },
			func(a *runtime.RuntimeAggregate) { a.CompletedDispatches[0].DispatchAuthorityHash = "drift" },
			func(a *runtime.RuntimeAggregate) {
				a.SemanticPlanning.History[0].Snapshot.Manifest.OwnerCatalogs[0].SnapshotID = "same-hash-different-raw"
			},
			func(a *runtime.RuntimeAggregate) { a.ImplementationWork.Rounds[0].Hash = "drift" },
			func(a *runtime.RuntimeAggregate) { a.RepairSurfaceConfirmations = nil; a.OperationReceipts = nil },
		} {
			a := base.CloneRaw()
			mutate(&a)
			assertOwnerStateJointPreimage(t, raw, a, next)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || string(body) != string(after) {
		t.Fatal("read-only archive changed")
	}
}

func FuzzRuntimeOwnerStateJointPreimage(f *testing.F) {
	f.Add(uint8(0), "raw\x00")
	f.Add(uint8(1), "变化")
	f.Add(uint8(2), "")
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		base := runtimeAggregate(t, 4)
		record := runtimeStateRecordFromAggregate(base)
		next := base.CloneRaw()
		next.StateRevision++
		switch arm % 4 {
		case 0:
			base.Facts.DefinitionRevision = raw
		case 1:
			base.SemanticPlanning.DefinitionSeed.PolicyHash = raw
		case 2:
			record.Facts.RunID = common.RunID(raw)
		case 3:
			base.Decision.AuthoritySnapshot.RunID = common.RunID(raw)
		}
		assertOwnerStateJointPreimage(t, record, base, next)
	})
}
