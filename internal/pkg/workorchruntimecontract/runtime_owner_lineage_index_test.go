package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Each field mutation is checked against the independent public validator,
// with a real successful raw node already retained under the same identity.
func checkOwnerLineageNode[T any](t *testing.T, raw T, clone func(T) T, cold, indexed func(T) error, retained func() []T) {
	t.Helper()
	if err := cold(raw); err != nil {
		t.Fatal("invalid node fixture", err)
	}
	if err := indexed(raw); err != nil {
		t.Fatal(err)
	}
	if len(retained()) != 1 {
		t.Fatal("missing historical node")
	}
	stored := clone(retained()[0])
	assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(raw), reflect.ValueOf(retained()[0]), "lineage node")
	compare := func(value T) {
		t.Helper()
		before := clone(value)
		want, got := cold(value), indexed(value)
		if fmt.Sprint(want) != fmt.Sprint(got) {
			t.Fatalf("node first error changed: %v != %v", got, want)
		}
		if !reflect.DeepEqual(value, before) {
			t.Fatal("validator changed original raw node")
		}
		if !reflect.DeepEqual(stored, retained()[0]) {
			t.Fatal("caller mutation changed retained node")
		}
		for _, entry := range retained() {
			if reflect.DeepEqual(value, entry) {
				assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(value), reflect.ValueOf(entry), "retained raw node")
			}
		}
	}
	changed := clone(raw)
	ownerLineageFieldMutations(reflect.ValueOf(&changed).Elem(), func() { compare(changed) })
	if !reflect.DeepEqual(raw, changed) {
		t.Fatal("mutation walker did not restore input")
	}
	wire, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var loaded T
	if err := json.Unmarshal(wire, &loaded); err != nil {
		t.Fatal(err)
	}
	compare(loaded)
}

// Preserve identity/hash fields unless that exact field is being tested. In
// particular, nil/empty changes must not become an exact-raw cache hit.
func ownerLineageFieldMutations(v reflect.Value, check func()) {
	if !v.IsValid() || !v.CanSet() {
		return
	}
	old := reflect.New(v.Type()).Elem()
	old.Set(v)
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			ownerLineageFieldMutations(v.Field(i), check)
		}
	case reflect.Pointer:
		if !v.IsNil() {
			v.SetZero()
			check()
			v.Set(old)
			ownerLineageFieldMutations(v.Elem(), check)
		}
	case reflect.Slice:
		if v.IsNil() {
			v.Set(reflect.MakeSlice(v.Type(), 0, 0))
		} else {
			v.SetZero()
		}
		check()
		v.Set(old)
		for i := 0; i < v.Len(); i++ {
			ownerLineageFieldMutations(v.Index(i), check)
		}
	case reflect.Map:
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		} else {
			v.SetZero()
		}
		check()
		v.Set(old)
	case reflect.String:
		v.SetString(v.String() + " raw\x00变化 ")
		check()
		v.Set(old)
	case reflect.Bool:
		v.SetBool(!v.Bool())
		check()
		v.Set(old)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(v.Int() + 1)
		check()
		v.Set(old)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(v.Uint() + 1)
		check()
		v.Set(old)
	}
}

func ownerLineageConfirmationFixture(t *testing.T) RuntimeRepairSurfaceConfirmationRecord {
	t.Helper()
	observation := RuntimeValidationFailureObservation{
		WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority", DispatchID: "dispatch", OperationID: "validate",
		FailureSignature: "build-failure", TargetRole: "implementation", DiagnosticClass: ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("b", 64),
	}
	r, err := NewRuntimeRepairSurfaceConfirmationRecord(RuntimeRepairSurfaceConfirmation{
		RequestID: "repair-request-1", IssuedAtStateRevision: 7, ExpectedSubjectRevision: "tree-v1", Observation: observation,
		OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Disposition: RuntimeRepairSurfaceConfirmed,
		ConfirmedTargetRefs: []string{"main.go"}, ObservedRevision: "tree-v1", Reason: "original owner explanation",
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOwnerLineageIndexCompleteRawNodes(t *testing.T) {
	t.Run("dispatch", func(t *testing.T) {
		d := runtimeDispatchIntrinsicFixture(t, 0)
		runtimeDispatchMarkCommittedForTest(&d)
		index := &runtimeOwnerLineageIndex{}
		checkOwnerLineageNode(t, d, cloneRuntimeDispatchRecordRaw, RuntimeDispatchRecord.ValidateIntrinsic, index.dispatch,
			func() []RuntimeDispatchRecord { return index.dispatches[d.ID] })
	})
	t.Run("catalog", func(t *testing.T) {
		for _, a := range semanticProofCatalogValuesFixture(t) {
			index := &runtimeOwnerLineageIndex{}
			checkOwnerLineageNode(t, a, cloneRuntimeOperationAdmissibilityRaw, RuntimeOperationAdmissibility.Validate, index.catalog,
				func() []RuntimeOperationAdmissibility { return index.catalogs[a.SnapshotID] })
		}
	})
	t.Run("receipt", func(t *testing.T) {
		r := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch/op-1", 1, "op-1")
		set, _ := ownerDependencyUnionFixture()
		r.Evidence.OwnerRevisionSet = &set
		index := &runtimeOwnerLineageIndex{}
		checkOwnerLineageNode(t, r, cloneRuntimeOperationReceiptRaw, RuntimeOperationReceipt.Validate, func(value RuntimeOperationReceipt) error {
			err := index.receipt(value)
			if got, want := index.receiptHash(value), StableRuntimeOperationReceiptHash(value); got != want {
				t.Fatal("raw receipt hash product changed", got, want)
			}
			return err
		},
			func() []RuntimeOperationReceipt {
				var raw []RuntimeOperationReceipt
				for _, entry := range index.receipts[r.ExecutionKey] {
					raw = append(raw, entry.raw)
				}
				return raw
			})
	})
	t.Run("confirmation", func(t *testing.T) {
		r := ownerLineageConfirmationFixture(t)
		index := &runtimeOwnerLineageIndex{}
		checkOwnerLineageNode(t, r, cloneRuntimeRepairSurfaceConfirmationRecordRaw, RuntimeRepairSurfaceConfirmationRecord.Validate, index.confirmation,
			func() []RuntimeRepairSurfaceConfirmationRecord { return index.confirmations[r.Signature] })
	})
}

func TestOwnerLineageIndexDoesNotRetainActiveOrFailedNodes(t *testing.T) {
	index := &runtimeOwnerLineageIndex{}
	d := runtimeDispatchIntrinsicFixture(t, 0)
	if err := index.dispatch(d); err != nil || len(index.dispatches) != 0 {
		t.Fatal("active success retained", err)
	}
	d.CandidateHash = "same-id drift"
	if err := index.dispatch(d); err == nil || len(index.dispatches) != 0 {
		t.Fatal("active drift admitted")
	}
	r := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch/op-1", 0, "op-1")
	if index.receipt(r) == nil || len(index.receipts) != 0 {
		t.Fatal("failed node retained")
	}
	s := newRuntimeSemanticValidationSession()
	if runtimeOwnerLineageIndexFor(s) != nil {
		t.Fatal("ordinary validator enabled owner index")
	}
	s.ownerHistory = &runtimeOwnerHistoricalSemanticCompiler{lineage: index}
	if runtimeOwnerLineageIndexFor(s) != index {
		t.Fatal("owner-private index not selected")
	}
	index.receipts = map[string][]runtimeOwnerLineageReceipt{"over limit": make([]runtimeOwnerLineageReceipt, runtimeOwnerHistoricalEntryLimit+1)}
	if retainRuntimeOwnerHistoricalSubtrees(s) != nil {
		t.Fatal("lineage nodes escaped whole-cache limit")
	}
}

func TestOwnerLineageIndexLifetimeAndExactRawSuccess(t *testing.T) {
	r := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch/op-1", 1, "op-1")
	index := &runtimeOwnerLineageIndex{}
	if err := index.receipt(r); err != nil {
		t.Fatal(err)
	}
	changed := cloneRuntimeOperationReceiptRaw(r)
	changed.Evidence.Summary = " " + r.Evidence.Summary + " "
	if err := index.receipt(changed); err != nil || len(index.receipts[r.ExecutionKey]) != 2 {
		t.Fatal("normalized-equal raw receipt reused old success", err)
	}
	for _, exit := range []string{"error", "panic", "scope", "close"} {
		s := newRuntimeSemanticValidationSession()
		s.ownerHistory = &runtimeOwnerHistoricalSemanticCompiler{lineage: index}
		c := &RuntimeOwnerHistoricalCompiler{workspace: "ws", run: "run", semantic: s}
		sentinel := errors.New("real owner gate failure")
		switch exit {
		case "error", "panic":
			func() {
				defer func() {
					if p := recover(); (exit == "panic") != (p == sentinel) {
						t.Fatal("panic changed", p)
					}
				}()
				err := c.Compile("ws", "run", func(func(RuntimeOwnerStateSourceImage) error) error {
					if exit == "panic" {
						panic(sentinel)
					}
					return sentinel
				})
				if err != sentinel {
					t.Fatal("error changed", err)
				}
			}()
			if c.semantic != nil {
				t.Fatal("failure retained historical nodes")
			}
		case "scope":
			if err := c.Compile("other", "run", func(func(RuntimeOwnerStateSourceImage) error) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if c.semantic.ownerHistory.lineage != nil {
				t.Fatal("other scope inherited nodes")
			}
		case "close":
			c.Close()
			if c.semantic != nil {
				t.Fatal("closed owner retained nodes")
			}
			if err := c.Compile("ws", "run", func(func(RuntimeOwnerStateSourceImage) error) error { return nil }); err != nil || c.semantic != nil {
				t.Fatal("closed compiler resurrected nodes", err)
			}
		}
		wire, _ := json.Marshal(c)
		var fresh RuntimeOwnerHistoricalCompiler
		if err := json.Unmarshal(wire, &fresh); err != nil || fresh.semantic != nil {
			t.Fatal("restart inherited node products", err)
		}
	}
}

func TestOwnerLineageIndexCollectionAndJoinGatesRerun(t *testing.T) {
	d := runtimeDispatchIntrinsicFixture(t, 0)
	runtimeDispatchMarkCommittedForTest(&d)
	index := &runtimeOwnerLineageIndex{}
	if err := index.dispatch(d); err != nil {
		t.Fatal(err)
	}
	for _, history := range [][]RuntimeDispatchRecord{{d}, {d, d}, {d, {}}, {{}, d}} {
		for _, scope := range []common.WorkspaceID{d.WorkspaceID, "other"} {
			got := validateRuntimeDispatchHistoryShapeInIndex(scope, d.RunID, nil, history, index)
			want := ValidateRuntimeDispatchHistoryShape(scope, d.RunID, nil, history)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatal("collection/scope first error changed", got, want)
			}
		}
	}
	r := runtimeOperationReceiptLineageFixture("exec/ws/run/dispatch/op-1", 1, "op-1")
	if err := index.receipt(r); err != nil {
		t.Fatal(err)
	}
	for _, receipts := range [][]RuntimeOperationReceipt{{r}, {r, r}, {r, {}}, {{}, r}} {
		for _, joined := range []bool{true, false} {
			dispatch := RuntimeDispatchRecord{ID: "dispatch", OperationExecutions: []RuntimeOperationExecutionRecord{runtimeOperationReceiptLineageExecution(r, OperationExecutionOwnerCommitted)}}
			if !joined {
				dispatch.OperationExecutions[0].OwnerReceiptHash = "other"
			}
			got := validateRuntimeOperationReceiptExecutionLineageInIndex(&dispatch, nil, receipts, index)
			want := ValidateRuntimeOperationReceiptExecutionLineage(&dispatch, nil, receipts)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatal("receipt join/duplicate first error changed", got, want)
			}
		}
	}
	c := ownerLineageConfirmationFixture(t)
	if err := index.confirmation(c); err != nil {
		t.Fatal(err)
	}
	for _, values := range [][]RuntimeRepairSurfaceConfirmationRecord{{c}, {c, c}, {c, {}}, {{}, c}} {
		got := validateRuntimeRepairSurfaceConfirmationRecordsInIndex(values, index)
		want := ValidateRuntimeRepairSurfaceConfirmationRecords(values)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatal("confirmation order/duplicate error changed", got, want)
		}
	}
}

func TestOwnerLineageIndexReadOnlyArchive(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		r := run.RuntimeState
		index := &runtimeOwnerLineageIndex{}
		for range 2 {
			if err := runtimeValidateLineageOperationsInIndex(historicalSessionArchiveSources(r), index); err != nil {
				t.Fatal(err)
			}
		}
		if len(index.dispatches) == 0 || len(index.receipts) == 0 || len(index.catalogs) == 0 || len(index.confirmations) != 0 { // confirmation collection is compiled separately
			t.Fatal("archive did not exercise sealed lineage nodes")
		}
		for arm := uint8(0); arm < 18; arm++ {
			changed := r.Clone()
			mutateRuntimeStateConsumerSession(&changed, arm, "same identity raw drift")
			sources := historicalSessionArchiveSources(changed)
			got, want := runtimeValidateLineageOperationsInIndex(sources, index), runtimeValidateLineageOperations(sources)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("archive lineage first error changed in arm %d: %v != %v", arm, got, want)
			}
		}
	}
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func FuzzOwnerLineageIndexRawNodes(f *testing.F) {
	for arm := uint8(0); arm < 8; arm++ {
		f.Add(arm, " raw\x00原文 ")
	}
	f.Fuzz(func(t *testing.T, arm uint8, raw string) {
		if len(raw) > 128 {
			t.Skip()
		}
		d := runtimeDispatchIntrinsicFixture(t, 0)
		runtimeDispatchMarkCommittedForTest(&d)
		index := &runtimeOwnerLineageIndex{}
		if err := index.dispatch(d); err != nil {
			t.Fatal(err)
		}
		changed := cloneRuntimeDispatchRecordRaw(d)
		switch arm % 8 {
		case 0:
			changed.CandidateHash = raw
		case 1:
			changed.Candidate.StateChange.Operations[0].Operation.TargetRefs[0] = raw
		case 2:
			*changed.Candidate.StateChange.Operations[0].Operation.Parameters[0].String = raw
		case 3:
			changed.OperationExecutions[0].OwnerReceiptHash = raw
		case 4:
			changed.CompletedOperationIDs = append(changed.CompletedOperationIDs, raw)
		case 5:
			changed.OperationTargetAuthorizations[0].Target.ExactTargetRefs[0] = raw
		case 6:
			changed.WorkspaceID = common.WorkspaceID(raw)
		case 7:
			changed.OperationAdmissibilities = []RuntimeOperationAdmissibility{}
		}
		got, want := index.dispatch(changed), changed.ValidateIntrinsic()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatal("raw node drift first error differs", got, want)
		}
		if !reflect.DeepEqual(index.dispatches[d.ID][0], d) {
			t.Fatal("mutation changed original retained node")
		}
	})
}
