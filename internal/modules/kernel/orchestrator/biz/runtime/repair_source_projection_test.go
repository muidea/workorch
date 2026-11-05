package runtime

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func assertRepairSourceProjection(t testing.TB, a RuntimeAggregate) {
	t.Helper()
	before := a.CloneRaw()
	want := repairProjectionReferenceValidateConfirmedRepairMandatoryCommitments(a)
	got := validateConfirmedRepairMandatoryCommitments(a)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("mandatory first error differs: %v != %v", got, want)
	}
	dispatches := append([]rc.RuntimeDispatchRecord(nil), a.CompletedDispatches...)
	if a.ActiveDispatch != nil {
		dispatches = append(dispatches, *a.ActiveDispatch)
	}
	for _, d := range dispatches {
		want, werr := repairProjectionReferenceConfirmedRepairCommitmentRequiredByRejectedObservation(a, d)
		got, gerr := confirmedRepairCommitmentRequiredByRejectedObservation(a, d)
		if got != want || fmt.Sprint(gerr) != fmt.Sprint(werr) {
			t.Fatalf("requirement/source differs for %s: %v/%v != %v/%v", d.ID, got, gerr, want, werr)
		}
	}
	for _, record := range a.RepairSurfaceConfirmations {
		wantFact, wantReceipt, wantOp, werr := repairProjectionReferenceConfirmedRepairSourceRejection(a, record.Confirmation)
		fact, receipt, op, err := confirmedRepairSourceRejection(a, record.Confirmation)
		if fmt.Sprint(err) != fmt.Sprint(werr) || !reflect.DeepEqual(fact, wantFact) || !reflect.DeepEqual(receipt, wantReceipt) || !reflect.DeepEqual(op, wantOp) {
			t.Fatalf("source reverse proof differs: %v != %v", err, werr)
		}
		if len(fact.TargetRefs) > 0 {
			fact.TargetRefs[0] = "caller copy"
		}
		if fact.Validation != nil && len(fact.Validation.ExecutedChecks) > 0 {
			fact.Validation.ExecutedChecks[0] = "caller copy"
		}
	}
	for _, binding := range a.OperationGovernanceBindings {
		want, werr := repairProjectionReferenceConfirmedRepairGovernanceEvidenceRevision(a, binding)
		got, err := confirmedRepairGovernanceEvidenceRevision(a, binding)
		if got != want || fmt.Sprint(err) != fmt.Sprint(werr) {
			t.Fatalf("governance source differs: %s/%v != %s/%v", got, err, want, werr)
		}
	}
	if !reflect.DeepEqual(before, a) {
		t.Fatal("projection changed raw input or returned an aliased evidence fact")
	}
}

func TestRepairSourceProjectionOriginalRulesAndRawMutations(t *testing.T) {
	fixture := newConfirmedRepairMultiOperationFixture(t)
	source := fixture.repository.aggregate.CloneRaw()
	if source.ActiveDispatch == nil {
		t.Fatal("fixture lacks sealed repair")
	}
	required, err := repairProjectionReferenceConfirmedRepairCommitmentRequiredByRejectedObservation(source, *source.ActiveDispatch)
	if err != nil || !required {
		t.Fatalf("original source not positive: %v/%v", required, err)
	}
	positiveSources := 0
	for _, record := range source.RepairSurfaceConfirmations {
		if _, _, _, err := repairProjectionReferenceConfirmedRepairSourceRejection(source, record.Confirmation); err == nil {
			positiveSources++
		}
	}
	if positiveSources == 0 {
		t.Fatal("fixture has no receipt-derived source positive")
	}
	for _, name := range []string{"original", "reload", "missing-evidence", "empty-evidence", "empty-id", "duplicate-body-first", "duplicate-body-last", "duplicate-id", "duplicate-dispatch", "reverse", "missing-receipts", "wrong-fence", "evidence-body", "operation-binding", "omitted-commitment", "foreign-workspace", "foreign-run", "missing-processed", "first-error"} {
		t.Run(name, func(t *testing.T) {
			a := source.CloneRaw()
			switch name {
			case "reload":
				body, err := json.Marshal(ownerStateRecord(a))
				if err != nil {
					t.Fatal(err)
				}
				var r rc.RuntimeStateRecord
				if err := json.Unmarshal(body, &r); err != nil {
					t.Fatal(err)
				}
				a = RuntimeAggregateFromRecord(r)
			case "missing-evidence":
				a.Facts.Evidence.Rejected = nil
			case "empty-evidence":
				a.Facts.Evidence.Rejected = []rc.EvidenceFact{}
			case "empty-id":
				a.Facts.Evidence.Rejected[0].ID = ""
			case "duplicate-body-first", "duplicate-body-last":
				duplicate := a.CloneRaw().Facts.Evidence.Rejected[0]
				duplicate.Summary = "different duplicate body"
				if name == "duplicate-body-first" {
					a.Facts.Evidence.Rejected = append([]rc.EvidenceFact{duplicate}, a.Facts.Evidence.Rejected...)
				} else {
					a.Facts.Evidence.Rejected = append(a.Facts.Evidence.Rejected, duplicate)
				}
			case "duplicate-id":
				a.Facts.Evidence.Rejected = append(a.Facts.Evidence.Rejected, a.Facts.Evidence.Rejected...)
			case "duplicate-dispatch":
				a.CompletedDispatches = append(a.CompletedDispatches, a.CompletedDispatches...)
			case "reverse":
				slices.Reverse(a.CompletedDispatches)
				slices.Reverse(a.Facts.Evidence.Rejected)
			case "missing-receipts":
				a.OperationReceipts = nil
			case "wrong-fence":
				for i := range a.OperationReceipts {
					a.OperationReceipts[i].Fence++
				}
			case "evidence-body":
				a.Facts.Evidence.Rejected[0].Summary = "different raw evidence"
			case "operation-binding":
				a.ActiveDispatch.Candidate.StateChange.Operations[0].Operation.BindingRefs = nil
			case "omitted-commitment":
				a.ActiveDispatch.RepairCommitment = nil
				a.ActiveDispatch.RepairCommitmentHash = ""
			case "foreign-workspace":
				a.Facts.WorkspaceID = "foreign"
			case "foreign-run":
				a.Facts.RunID = "foreign"
			case "missing-processed":
				a.ProcessedEventIDs = nil
			case "first-error":
				a.ActiveDispatch.RepairCommitment = nil
				a.CompletedDispatches = append(a.CompletedDispatches, a.CompletedDispatches...)
				a.Facts.Evidence.Rejected[0].Summary = "drift"
			}
			assertRepairSourceProjection(t, a)
		})
	}
	// Every Facts field is allowed to differ between pure invocations. Unrelated
	// values cannot affect the query, and related values must keep the old result.
	fields := reflect.TypeOf(source.Facts)
	for i := 0; i < fields.NumField(); i++ {
		t.Run("facts/"+fields.Field(i).Name, func(t *testing.T) {
			a := source.CloneRaw()
			field := reflect.ValueOf(&a.Facts).Elem().Field(i)
			field.Set(reflect.Zero(field.Type()))
			assertRepairSourceProjection(t, a)
		})
	}
}

func TestRepairSourceProjectionCompleteArchive(t *testing.T) {
	a, _ := reconciliationScanArchive(t)
	assertRepairSourceProjection(t, a)
	for _, mutate := range []func(*RuntimeAggregate){
		func(a *RuntimeAggregate) { a.Facts.Evidence.Rejected = nil },
		func(a *RuntimeAggregate) {
			a.Facts.Evidence.Rejected = append(a.Facts.Evidence.Rejected, a.Facts.Evidence.Rejected...)
		},
		func(a *RuntimeAggregate) {
			a.CompletedDispatches = append(a.CompletedDispatches, a.CompletedDispatches...)
		},
		func(a *RuntimeAggregate) { a.OperationReceipts = nil },
		func(a *RuntimeAggregate) {
			for i := range a.OperationReceipts {
				a.OperationReceipts[i].Fence++
			}
		},
		func(a *RuntimeAggregate) { a.Facts.WorkspaceID = "different"; a.Facts.RunID = "different" },
	} {
		v := a.CloneRaw()
		mutate(&v)
		assertRepairSourceProjection(t, v)
	}
}

// This isolates the accepted-governance evidence projection, without claiming
// a synthetic fact is admissible as an owner receipt or a governance grant.
func TestRepairSourceProjectionGovernanceCanonicalCollection(t *testing.T) {
	binding := rc.RuntimeOperationGovernanceBinding{GovernanceSnapshotID: "snapshot", GovernanceDispatchID: "dispatch", Grant: &rc.RuntimeOperationGovernanceGrant{ResolutionID: "resolution", ResolutionHash: "original", EvidenceRefs: []string{"ref"}}}
	id := common.EvidenceRecordID("governance-resolution:resolution:snapshot")
	fact := rc.EvidenceFact{ID: id, Kind: " " + rc.EvidenceKindGovernance + " ", SourceRef: " governance-resolution/resolution ", DispatchID: " dispatch ", AuthoritySnapshotID: " snapshot ", ContentHash: " original ", EvidenceRefs: []string{" ref "}, Revision: " revision "}
	for _, name := range []string{"original", "empty-id", "duplicate-id", "missing-fence", "duplicate-fence", "body-drift"} {
		a := RuntimeAggregate{Facts: rc.RuntimeFacts{Evidence: rc.EvidenceLedger{Accepted: []rc.EvidenceFact{fact}}}, ProcessedEventIDs: []string{string(id)}}
		switch name {
		case "empty-id":
			a.Facts.Evidence.Accepted[0].ID = ""
		case "duplicate-id":
			a.Facts.Evidence.Accepted = append(a.Facts.Evidence.Accepted, fact)
		case "missing-fence":
			a.ProcessedEventIDs = nil
		case "duplicate-fence":
			a.ProcessedEventIDs = append(a.ProcessedEventIDs, string(id))
		case "body-drift":
			a.Facts.Evidence.Accepted[0].ContentHash = "drift"
		}
		before := a.CloneRaw()
		want, werr := repairProjectionReferenceConfirmedRepairGovernanceEvidenceRevision(a, binding)
		got, err := confirmedRepairGovernanceEvidenceRevision(a, binding)
		if got != want || fmt.Sprint(err) != fmt.Sprint(werr) {
			t.Fatalf("%s governance projection differs: %s/%v != %s/%v", name, got, err, want, werr)
		}
		if name == "original" && (got != "revision" || err != nil) {
			t.Fatal("original governance positive lost")
		}
		if !reflect.DeepEqual(before, a) {
			t.Fatal("governance projection mutated original")
		}
	}
}
