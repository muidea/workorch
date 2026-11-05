package runtime

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func assertApplicabilitySingleSource(t *testing.T, a RuntimeAggregate) {
	t.Helper()
	before := a.CloneRaw()
	want, wantSource, wantRequired, wantErr := referenceBeforeSingleSourceCompileRuntimeEvidenceApplicabilityRead(a)
	got, source, required, err := compileRuntimeEvidenceApplicabilityRead(a)
	if got != want || !reflect.DeepEqual(source, wantSource) || required != wantRequired || fmt.Sprint(err) != fmt.Sprint(wantErr) {
		t.Fatalf("request/source/required/first error changed: %v != %v", err, wantErr)
	}
	for _, fact := range a.Facts.Evidence.Accepted {
		want, wantErr := referenceBeforeSingleSourceCompileRuntimeEvidenceApplicabilityRequest(a, fact.ID)
		got, err := compileRuntimeEvidenceApplicabilityRequest(a, fact.ID)
		if got != want || fmt.Sprint(err) != fmt.Sprint(wantErr) {
			t.Fatalf("independent observation request changed: %v != %v", err, wantErr)
		}
	}
	if required {
		source.Operation.TargetRefs[0] = "caller mutation"
		source.OriginalDependencies.Entries[0].Revision = "caller mutation"
		source.ApplicabilitySource.OwnerRevisionSet.Entries[0].Revision = "caller mutation"
	}
	if !reflect.DeepEqual(a, before) {
		t.Fatal("source result aliases original owner image")
	}
}

func TestApplicabilitySingleSourceMatchesOriginalComposition(t *testing.T) {
	base, observation := applicabilityTransitionFixture(t)
	if _, _, required, err := compileRuntimeEvidenceApplicabilityRead(base); err != nil || !required {
		t.Fatalf("fixture needs a real pending source: %v", err)
	}
	for _, name := range []string{"original", "reload", "duplicate", "missing-receipt", "receipt-body", "missing-dispatch", "dependency", "no-dependency", "observed", "old-revision", "old-generation", "wrong-hash", "many-observations", "stale", "wrong-workspace", "empty-revision", "empty-source", "empty"} {
		t.Run(name, func(t *testing.T) {
			a := base.CloneRaw()
			switch name {
			case "reload":
				a = runtimeTestEvidenceAggregateRoundTrip(t, a)
			case "duplicate":
				a.Facts.Evidence.Accepted = append(a.Facts.Evidence.Accepted, a.Facts.Evidence.Accepted[0])
			case "missing-receipt":
				a.OperationReceipts = nil
			case "receipt-body":
				a.OperationReceipts[0].AfterHash = "changed"
			case "missing-dispatch":
				a.CompletedDispatches = nil
			case "dependency":
				a.Facts.Evidence.Accepted[0].ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision = "changed"
			case "no-dependency":
				a.Facts.Evidence.Accepted[0].ApplicabilityDependencies = nil
			case "observed", "old-revision", "old-generation", "wrong-hash", "many-observations", "stale":
				o := observation.CloneRaw()
				switch name {
				case "old-revision":
					o.Request.EvidenceRevision = "earlier-revision"
				case "old-generation":
					o.Request.ReconciliationRequestHash = "earlier-generation"
				case "wrong-hash":
					o.Request.SourceEvidenceHash = "same-id-other-body"
				case "stale":
					o.CurrentDependencies.Entries[0].Revision = "changed"
					changed := o.CurrentDependencies.WithHash()
					o.CurrentDependencies = &changed
				}
				a.Facts.EvidenceApplicabilityObservations = append(a.Facts.EvidenceApplicabilityObservations, o)
				if name == "many-observations" {
					for range 32 {
						a.Facts.EvidenceApplicabilityObservations = append(a.Facts.EvidenceApplicabilityObservations, o.CloneRaw())
					}
				}
			case "wrong-workspace":
				a.Facts.WorkspaceID = "another-workspace"
			case "empty-revision":
				a.Facts.EvidenceRevision = ""
			case "empty-source":
				a.Facts.Evidence.Accepted[0].ID = ""
			case "empty":
				a = RuntimeAggregate{}
			}
			assertApplicabilitySingleSource(t, a)
		})
	}
	// No per-call source product is reused by the next revision.
	assertApplicabilitySingleSource(t, base)
	base.StateRevision++
	assertApplicabilitySingleSource(t, base)
}

func TestApplicabilitySingleSourceCompleteArchive(t *testing.T) {
	a, _ := reconciliationScanArchive(t)
	assertApplicabilitySingleSource(t, a)
	for _, h := range a.Reconciliation.History {
		changed := a.CloneRaw()
		changed.Reconciliation.RequestHash = h.RequestHash
		assertApplicabilitySingleSource(t, changed)
	}
}

func BenchmarkApplicabilityOriginalSourceCompilation(b *testing.B) {
	a, _ := reconciliationScanArchive(b)
	var ids []common.EvidenceRecordID
	for _, fact := range a.Facts.Evidence.Accepted {
		if fact.ApplicabilityDependencies == nil {
			continue
		}
		if _, err := referenceBeforeSingleSourceCompileRuntimeEvidenceApplicabilityRequest(a, fact.ID); err == nil {
			ids = append(ids, fact.ID)
		}
	}
	if len(ids) == 0 {
		b.Skip("archive has no applicable original source")
	}
	for _, name := range []string{"reference", "current"} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for _, id := range ids {
					if name == "reference" {
						request, err := referenceBeforeSingleSourceCompileRuntimeEvidenceApplicabilityRequest(a, id)
						if err != nil {
							b.Fatal(err)
						}
						if _, err := compileRuntimeOperationReadSource(a, request.DispatchID, request.OperationID, request.SourceReceiptHash); err != nil {
							b.Fatal(err)
						}
					} else if _, _, err := compileRuntimeEvidenceApplicabilitySource(a, id); err != nil {
						b.Fatal(err)
					}
				}
			}
			b.ReportMetric(float64(len(ids)), "sources/op")
		})
	}
}
