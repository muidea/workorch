package orchestrator

import (
	"context"
	"encoding/json"
	"github.com/muidea/workorch/internal/pkg/common"
	"os"
	"reflect"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Replays only the pure owner-dependency observation lane on an independently
// persisted in-memory copy. Unchanged dependencies are a controlled input,
// not a claim that a live resource owner was queried or is still unchanged.
// No provider, capability execution, project file or original archive is used
// as an output surface.
func TestRuntimePublicArchiveApplicabilityDebtDrainsOnUnchangedDependencies(t *testing.T) {
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
		if err := base.Validate(); err != nil {
			t.Fatal(err)
		}
		repo := &inMemoryRuntimeStateRepository{aggregate: &base}
		controller, err := runtime.NewRuntimeController(repo)
		if err != nil {
			t.Fatal(err)
		}
		before := base.CloneRaw()
		seen := map[string]bool{}
		current, err := controller.Load(context.Background(), base.Facts.WorkspaceID, base.Facts.RunID)
		if err != nil {
			t.Fatal(err)
		}
		for {
			request, source, required, err := current.EvidenceApplicabilityRead()
			if err != nil {
				t.Fatal(err)
			}
			if !required {
				break
			}
			if seen[string(request.SourceEvidenceID)] || len(seen) >= len(before.Facts.Evidence.Accepted) {
				t.Fatal("unchanged observation lane repeated a source or failed to drain")
			}
			seen[string(request.SourceEvidenceID)] = true
			observation := runtimecontract.RuntimeEvidenceApplicabilityObservation{
				Request: request, RecordedAtStateRevision: current.StateRevision + 1,
				CurrentDependencies: runtimecontract.CloneRuntimeOwnerRevisionSet(&source.ApplicabilitySource.OwnerRevisionSet),
			}
			event := runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventEvidenceApplicabilityObserved,
				Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: current.Facts.WorkspaceID, RunID: current.Facts.RunID},
				Authority: &runtime.RuntimeInternalAuthorityPayload{EvidenceApplicability: &observation}}
			result, err := controller.ApplyInternalEvent(context.Background(), event)
			if err != nil {
				t.Fatal(err)
			}
			current = result.Aggregate
			// Exact duplicate replay and fresh process use the same persisted
			// observation; neither may add another revision or repeat a read.
			controller, err = runtime.NewRuntimeController(repo)
			if err != nil {
				t.Fatal(err)
			}
			duplicate, err := controller.ApplyInternalEvent(context.Background(), event)
			if err != nil || !reflect.DeepEqual(duplicate.Aggregate, current) {
				t.Fatal("duplicate/reload changed observation", err)
			}
			t.Logf("observation source=%s revision=%d state=%s", request.SourceEvidenceID, current.StateRevision, current.Decision.State)
		}
		if len(seen) == 0 {
			t.Fatal("archive did not exercise a pending observation")
		}
		if current.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite {
			for _, debt := range current.Decision.Prerequisites {
				if debt.Kind == runtimecontract.RuntimeEvidenceApplicabilityPrerequisiteV1 {
					t.Fatal("drained read lane left an applicability debt projection")
				}
			}
		}
		if !reflect.DeepEqual(before.ExecutionBudget, current.ExecutionBudget) || !reflect.DeepEqual(before.OperationReceipts, current.OperationReceipts) ||
			!reflect.DeepEqual(before.Facts.Coverage, current.Facts.Coverage) || len(current.DeliveryAcknowledgements) != len(before.DeliveryAcknowledgements) {
			t.Fatal("read-only observation invented execution, CWI credit, budget consumption or ACK")
		}
		for _, cwi := range current.Facts.Coverage {
			t.Logf("CWI=%s state=%s convergence=%s dependencies=%v binding=%v", cwi.ID, cwi.State, cwi.CWIConvergenceState, cwi.PrerequisiteCoverageWorkItemIDs, cwi.CurrentBindingRef != nil)
		}
		t.Logf("drained=%d final revision=%d state=%s", len(seen), current.StateRevision, current.Decision.State)
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(body, after) {
		t.Fatal("original archive changed")
	}
}

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
		for _, arm := range []string{"valid", "history", "receipt", "budget", "dispatch", "scope", "duplicate", "first_error"} {
			t.Run(arm, func(t *testing.T) {
				if _, err := runtime.AdmitRuntimeOwnerStateRecord(runtime.RuntimeStateRecordFromAggregate(base)); err != nil {
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
				want, wantSource, wantRequired, wantErr := archiveColdApplicabilityRead(a)
				admitted, err := runtime.AdmitRuntimeOwnerStateRecord(runtime.RuntimeStateRecordFromAggregate(a))
				var got runtimecontract.RuntimeEvidenceApplicabilityRequest
				var source runtime.RuntimeOperationReadSource
				var required bool
				if err == nil {
					got, source, required, err = admitted.EvidenceApplicabilityRead()
				}
				if (err == nil) != (wantErr == nil) || required != wantRequired || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(source, wantSource) || !reflect.DeepEqual(a, before) {
					t.Fatalf("public archive cold/owner result differs: %v / %v", err, wantErr)
				}
			})
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(body, after) {
		t.Fatal("original archive changed")
	}
}

// The archive is read-only diagnostic input. Cold and owner_admit_and_read
// include the first complete admission; owner_result measures consumption only,
// not end-to-end cost. No retained subtree becomes current authority.
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
		admitted, err := runtime.AdmitRuntimeOwnerStateRecord(run.RuntimeState)
		if err != nil {
			b.Fatal(err)
		}
		for _, name := range []string{"cold", "owner_admit_and_read", "owner_result"} {
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var err error
					switch name {
					case "cold":
						_, _, _, err = archiveColdApplicabilityRead(a)
					case "owner_admit_and_read":
						var value runtime.RuntimeAggregate
						value, err = runtime.AdmitRuntimeOwnerStateRecord(run.RuntimeState)
						if err == nil {
							_, _, _, err = value.EvidenceApplicabilityRead()
						}
					default:
						_, _, _, err = admitted.EvidenceApplicabilityRead()
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// Independent cold query oracle in the caller tests. Production consumers use
// the typed result returned by the sole owner admission instead.
func archiveColdApplicabilityRead(a runtime.RuntimeAggregate) (runtimecontract.RuntimeEvidenceApplicabilityRequest, runtime.RuntimeOperationReadSource, bool, error) {
	if err := a.Validate(); err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, runtime.RuntimeOperationReadSource{}, false, err
	}
	q, required, err := runtimeNextEvidenceApplicabilityRequestForTest(a)
	if err != nil || !required {
		return q, runtime.RuntimeOperationReadSource{}, required, err
	}
	source, err := runtime.CompileRuntimeOperationReadSource(a, q.DispatchID, q.OperationID, q.SourceReceiptHash)
	return q, source, required, err
}

func runtimeNextEvidenceApplicabilityRequestForTest(a runtime.RuntimeAggregate) (runtimecontract.RuntimeEvidenceApplicabilityRequest, bool, error) {
	admitted, err := runtime.AdmitRuntimeOwnerStateRecord(runtimeStateRecordFromAggregate(a))
	if err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, false, err
	}
	request, _, required, err := admitted.EvidenceApplicabilityRead()
	return request, required, err
}

// Historical effects never request a current next-read. This independent
// test-only oracle checks the immutable source using the production receipt
// query boundary, without adding an unused production raw request facade.
func runtimeCompileEvidenceApplicabilityRequestForTest(a runtime.RuntimeAggregate, id common.EvidenceRecordID) (runtimecontract.RuntimeEvidenceApplicabilityRequest, error) {
	if err := a.Validate(); err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, err
	}
	for _, fact := range a.Facts.Evidence.Accepted {
		if fact.ID != id || fact.ApplicabilityDependencies == nil {
			continue
		}
		for _, receipt := range a.OperationReceipts {
			if fact.SourceRef != "runtime-receipt/"+receipt.ExecutionKey || fact.OperationID != receipt.OperationID {
				continue
			}
			hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
			if _, err := runtime.CompileRuntimeOperationReadSource(a, fact.DispatchID, fact.OperationID, hash); err != nil {
				return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, err
			}
			q := (runtimecontract.RuntimeEvidenceApplicabilityRequest{
				ProtocolRef:           runtimecontract.RuntimeEvidenceApplicabilityObservationV1,
				IssuedAtStateRevision: a.StateRevision, EvidenceRevision: a.Facts.EvidenceRevision,
				SourceEvidenceID: fact.ID, SourceEvidenceHash: runtimecontract.StableRuntimeEvidenceFactHash(fact), SourceReceiptHash: hash,
				DispatchID: fact.DispatchID, OperationID: fact.OperationID, ReconciliationRequestHash: a.Reconciliation.RequestHash,
				PurposeRef: fact.ApplicabilityDependencies.PurposeRef,
			}).WithID()
			return q, q.ValidateRaw()
		}
	}
	return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, common.ErrConflict
}
