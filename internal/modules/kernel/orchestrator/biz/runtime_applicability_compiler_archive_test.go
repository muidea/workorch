package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimePublicArchiveApplicabilityReadOracle(t *testing.T) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit read-only public fullflow archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct {
		RuntimeState runtimecontract.RuntimeStateRecord
	}
	if err := json.Unmarshal(body, &runs); err != nil || len(runs) != 1 {
		t.Fatalf("expected one complete archived Run: %v", err)
	}
	for _, run := range runs {
		base := runtimeAggregateFromRecord(run.RuntimeState)
		repo := &runtimeArchivePreimageRepository{record: run.RuntimeState}
		controller, _ := runtime.NewRuntimeController(repo)
		for _, arm := range []string{"valid", "history", "receipt", "budget", "dispatch", "scope", "duplicate", "first_error"} {
			t.Run(arm, func(t *testing.T) {
				if _, _, _, err := controller.CompileEvidenceApplicabilityRead(base); err != nil {
					t.Fatal(err)
				}
				a := base.CloneRaw()
				switch arm {
				case "history":
					a.SemanticPlanning.History[0].Snapshot.Manifest.OwnerCatalogs[0].SnapshotID = "same hash raw drift"
				case "receipt":
					a.OperationReceipts[0].BeforeHash = "same identity raw drift"
				case "budget":
					a.ExecutionBudget.Policy.PolicyRevision = "current drift"
				case "dispatch":
					a.CompletedDispatches[0].DispatchAuthorityHash = "same identity raw drift"
				case "scope":
					a.Facts.RunID = "unrelated-run"
				case "duplicate":
					a.OperationReceipts = append(a.OperationReceipts, a.OperationReceipts[0])
				case "first_error":
					a.ResultFeedbackVersion = 0
					a.OperationReceipts[0].BeforeHash = "second error"
				}
				before := a.CloneRaw()
				want, wantSource, wantRequired, wantErr := runtime.CompileRuntimeEvidenceApplicabilityRead(a)
				got, source, required, err := controller.CompileEvidenceApplicabilityRead(a)
				if fmt.Sprint(err) != fmt.Sprint(wantErr) || required != wantRequired || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(source, wantSource) || !reflect.DeepEqual(a, before) {
					t.Fatalf("public archive cold/owner result differs: %v / %v", err, wantErr)
				}
			})
		}
		if repo.loads != 0 || repo.saves != 0 {
			t.Fatal("pure read compiler reached owner I/O")
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(body, after) {
		t.Fatal("original archive changed")
	}
}

// The archive is read-only diagnostic input. This benchmark includes the first
// complete compile; it never treats a retained subtree as current authority.
func BenchmarkRuntimePublicArchiveApplicabilityRead(b *testing.B) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		b.Skip("explicit read-only public fullflow archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var runs map[string]struct {
		RuntimeState runtimecontract.RuntimeStateRecord
	}
	if err := json.Unmarshal(body, &runs); err != nil || len(runs) != 1 {
		b.Fatalf("expected one complete archived Run: %v", err)
	}
	for _, run := range runs {
		a := runtimeAggregateFromRecord(run.RuntimeState)
		for _, name := range []string{"cold", "owner_history"} {
			b.Run(name, func(b *testing.B) {
				repo := &runtimeArchivePreimageRepository{record: run.RuntimeState}
				controller, _ := runtime.NewRuntimeController(repo)
				compile := runtime.CompileRuntimeEvidenceApplicabilityRead
				if name == "owner_history" {
					compile = controller.CompileEvidenceApplicabilityRead
				}
				b.ReportAllocs()
				for b.Loop() {
					if _, _, _, err := compile(a); err != nil {
						b.Fatal(err)
					}
				}
				if repo.loads != 0 || repo.saves != 0 {
					b.Fatal("pure compilation performed owner I/O")
				}
			})
		}
	}
}
