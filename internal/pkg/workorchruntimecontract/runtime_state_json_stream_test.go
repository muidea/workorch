package workorchruntimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"testing"
)

func populateStreamJSONFixture(v reflect.Value, parents map[reflect.Type]bool) {
	if !v.CanSet() || parents[v.Type()] {
		return
	}
	parents[v.Type()] = true
	defer delete(parents, v.Type())
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			populateStreamJSONFixture(v.Field(i), parents)
		}
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		populateStreamJSONFixture(v.Elem(), parents)
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		populateStreamJSONFixture(v.Index(0), parents)
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
		k, value := reflect.New(v.Type().Key()).Elem(), reflect.New(v.Type().Elem()).Elem()
		populateStreamJSONFixture(k, parents)
		populateStreamJSONFixture(value, parents)
		v.SetMapIndex(k, value)
	case reflect.String:
		v.SetString(" original <&\"\n中文\u2028\u2029\xff ")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(9)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.25)
	}
}

func assertWriteRuntimeStateRecordJSONMatchesMarshal(t *testing.T, state RuntimeStateRecord) {
	t.Helper()
	want, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := WriteRuntimeStateRecordJSON(&got, state); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got.Bytes()) {
		t.Fatalf("streaming JSON differs from encoding/json.Marshal (want %d bytes, got %d bytes)", len(want), got.Len())
	}
}

func TestWriteRuntimeStateRecordJSONSchemaFieldGuards(t *testing.T) {
	assertFields := func(typ reflect.Type, want []struct {
		name string
		tag  string
	}) {
		t.Helper()
		if typ.NumField() != len(want) {
			t.Fatalf("%s field count changed: got %d, want %d; update WriteRuntimeStateRecordJSON", typ.Name(), typ.NumField(), len(want))
		}
		for i, expected := range want {
			field := typ.Field(i)
			if field.Name != expected.name || field.Tag.Get("json") != expected.tag {
				t.Fatalf("%s field[%d] mismatch: got (%q, json=%q), want (%q, json=%q)", typ.Name(), i, field.Name, field.Tag.Get("json"), expected.name, expected.tag)
			}
		}
	}

	assertFields(reflect.TypeFor[RuntimeStateRecord](), []struct {
		name string
		tag  string
	}{
		{"Definition", ""},
		{"CoverageProofLineage", ""},
		{"OperationReceipts", ""},
		{"ProcessedEventIDs", ""},
		{"CompletedDispatches", ""},
		{"Reconciliation", ""},
		{"CandidateAttempts", ""},
		{"CandidateRequestSessions", ""},
		{"SemanticPlanning", ""},
		{"Facts", ""},
		{"ExecutionBudget", ""},
		{"RepairSurfaceConfirmations", ""},
		{"Diagnostics", ""},
		{"PrerequisiteInvalidations", ""},
		{"GovernanceRequests", ""},
		{"GovernanceResolutions", ""},
		{"OperationGovernanceBindings", ""},
		{"OperationRecoveryAuthorizations", ""},
		{"RecoverableWaits", ""},
		{"DeliveryAcknowledgements", ""},
		{"ImplementationWork", ""},
		{"DefinitionClarification", ""},
		{"TerminalMaterialSeal", ""},
		{"ActiveDispatch", ""},
		{"Decision", ""},
		{"DriveLease", ""},
		{"Termination", ""},
		{"ResultFeedbackVersion", ",omitempty"},
		{"StateRevision", ""},
		{"EvidenceSequence", ""},
	})

	assertFields(reflect.TypeFor[RuntimeSemanticPlanningLedger](), []struct {
		name string
		tag  string
	}{
		{"SchemaVersion", ""},
		{"DefinitionSeedHash", ""},
		{"HistoryArchive", ",omitempty"},
		{"History", ""},
		{"Manifest", ""},
		{"StablePrefixHash", ""},
		{"CompiledOrder", ",omitempty"},
		{"CompiledProofBatches", ",omitempty"},
		{"CoverageOrderDocument", ""},
		{"FailureInputReview", ",omitempty"},
		{"BatchDocuments", ""},
		{"BatchFeasibilityResults", ""},
		{"MergedDocument", ""},
		{"MergedDocumentHash", ""},
		{"Document", ""},
		{"DocumentHash", ""},
		{"Feasibilities", ""},
		{"FeasibilitySetHash", ""},
		{"SealedBindingSetHash", ""},
		{"SealedLineageRefs", ""},
		{"Batches", ""},
		{"Phase", ""},
		{"Attempt", ""},
		{"LogicalRequestID", ""},
		{"CorrectionDirective", ""},
		{"ActiveBatchID", ""},
		{"NextBatchSequence", ""},
		{"PendingCoverageIDs", ""},
		{"CompletedBatchIDs", ""},
		{"FeasibilityBatchID", ""},
		{"Rejections", ""},
		{"FeasibilityGaps", ""},
		{"CatalogRefreshAttempts", ""},
		{"Failure", ""},
	})

	assertFields(reflect.TypeFor[RuntimeSemanticHistoryArchive](), []struct {
		name string
		tag  string
	}{
		{"ProtocolRef", ""},
		{"GenerationCount", ""},
		{"Segments", ""},
	})

	assertFields(reflect.TypeFor[RuntimeFacts](), []struct {
		name string
		tag  string
	}{
		{"WorkspaceID", ""},
		{"RunID", ""},
		{"DefinitionRevision", ""},
		{"CWILedgerSchemaVersion", ""},
		{"DefinitionSeed", ""},
		{"DefinitionFocuses", ""},
		{"Activation", ""},
		{"ConvergencePolicy", ""},
		{"RecoveryPolicy", ""},
		{"ExecutionBudgetPolicy", ""},
		{"Slices", ""},
		{"CompletedFocuses", ""},
		{"Assessments", ""},
		{"Evidence", ""},
		{"EvidenceApplicabilityObservations", ",omitempty"},
		{"Capabilities", ""},
		{"ReconciliationContract", ""},
		{"RemainingFocuses", ""},
		{"Coverage", ""},
		{"CWISuspensions", ",omitempty"},
		{"Resources", ""},
		{"Prerequisites", ""},
		{"EvidenceRevision", ""},
		{"Focus", ""},
		{"Feasibility", ""},
		{"Failure", ""},
		{"Repair", ""},
		{"RepairDirective", ""},
		{"Recovery", ""},
		{"StateChange", ""},
	})

	assertFields(reflect.TypeFor[RuntimeExecutionBudgetLedger](), []struct {
		name string
		tag  string
	}{
		{"Policy", ""},
		{"Consumptions", ""},
		{"Reconciliations", ""},
		{"Allocations", ""},
	})

	assertFields(reflect.TypeFor[RuntimeCWIReconciliationLedger](), []struct {
		name string
		tag  string
	}{
		{"History", ""},
		{"Request", ""},
		{"RequestHash", ""},
		{"Batch", ""},
		{"Results", ""},
		{"Reopened", ""},
	})
}

func TestWriteRuntimeStateRecordJSONExactByteIdentity(t *testing.T) {
	assertWriteRuntimeStateRecordJSONMatchesMarshal(t, RuntimeStateRecord{})
	assertWriteRuntimeStateRecordJSONMatchesMarshal(t, RuntimeStateRecord{
		ProcessedEventIDs: []string{},
		SemanticPlanning: RuntimeSemanticPlanningLedger{
			FailureInputReview:   &RuntimeValidationFailureInputReview{},
			CompiledOrder:        &RuntimeSemanticCompiledOrder{},
			CompiledProofBatches: []RuntimeSemanticCompiledProofBatch{{}},
			History:              []RuntimeSemanticPlanningGeneration{},
			HistoryArchive: &RuntimeSemanticHistoryArchive{
				Segments: [][]RuntimeSemanticPlanningGeneration{{}, nil},
			},
		},
	})

	var filled RuntimeStateRecord
	populateStreamJSONFixture(reflect.ValueOf(&filled).Elem(), map[reflect.Type]bool{})
	assertWriteRuntimeStateRecordJSONMatchesMarshal(t, filled)

	// Verify both omitted optional fields and empty-vs-nil slices match encoding/json.
	filled.ResultFeedbackVersion = 0
	filled.CompletedDispatches = []RuntimeDispatchRecord{}
	filled.ProcessedEventIDs = nil
	filled.SemanticPlanning.FailureInputReview = nil
	filled.SemanticPlanning.CompiledOrder = nil
	filled.SemanticPlanning.CompiledProofBatches = nil
	filled.SemanticPlanning.HistoryArchive = nil
	assertWriteRuntimeStateRecordJSONMatchesMarshal(t, filled)

	t.Run("FixedArchive", func(t *testing.T) {
		_, _, runs := runtimeStateConsumerArchive(t)
		for _, run := range runs {
			assertWriteRuntimeStateRecordJSONMatchesMarshal(t, run.RuntimeState)
		}
	})
}

func TestWriteRuntimeStateRecordJSONWithPreimageExactByteIdentity(t *testing.T) {
	assertPreimageSequence := func(t *testing.T, sequence []RuntimeStateRecord) {
		t.Helper()
		var prev *RuntimeStateRecord
		var prevRaw []byte
		var prevSpans *RuntimeStateJSONSpans
		for step, curr := range sequence {
			want, err := json.Marshal(curr)
			if err != nil {
				t.Fatalf("step %d marshal: %v", step, err)
			}
			var got bytes.Buffer
			spans, err := WriteRuntimeStateRecordJSONWithPreimage(&got, curr, prev, prevRaw, prevSpans)
			if err != nil {
				t.Fatalf("step %d preimage stream: %v", step, err)
			}
			if !bytes.Equal(want, got.Bytes()) {
				t.Fatalf("step %d preimage JSON differs (want %d bytes, got %d bytes)", step, len(want), got.Len())
			}
			var full bytes.Buffer
			if _, err := WriteRuntimeStateRecordJSONWithPreimage(&full, curr, nil, nil, nil); err != nil {
				t.Fatalf("step %d full stream: %v", step, err)
			}
			if !bytes.Equal(full.Bytes(), got.Bytes()) {
				t.Fatalf("step %d preimage stream differs from full stream (want %d bytes, got %d bytes)", step, full.Len(), got.Len())
			}
			gotSum := SumRuntimeStateJSONWithPreimage(got.Bytes(), prevRaw, prevSpans, &spans)
			if wantSum := sha256.Sum256(want); gotSum != wantSum {
				t.Fatalf("step %d preimage SHA-256 differs from full SHA-256", step)
			}
			copyState := curr.Clone()
			prev = &copyState
			prevRaw = append([]byte(nil), got.Bytes()...)
			copySpans := spans
			prevSpans = &copySpans
		}
	}

	var base RuntimeStateRecord
	populateStreamJSONFixture(reflect.ValueOf(&base).Elem(), map[reflect.Type]bool{})
	base.ResultFeedbackVersion = 1

	step1 := base.Clone()
	step1.StateRevision++
	// Mutate only DriveLease while keeping Facts, SemanticPlanning, CompletedDispatches identical.
	step1.DriveLease = &RuntimeDriveLease{WorkspaceID: "ws-1", RunID: "run-1", Reason: RuntimeDriveLeaseReasonPersistedPending, FailureSignature: "sig-2", DueAt: "2026-11-16T00:00:00Z"}

	step1b := step1.Clone()
	step1b.StateRevision++
	// Mutate SemanticPlanning.Attempt while keeping Facts untouched (shifted spans) and SemanticPlanning subfields reusable.
	step1b.SemanticPlanning.Attempt++

	step2 := step1b.Clone()
	step2.StateRevision++
	// Append to Facts.EvidenceApplicabilityObservations, SemanticPlanning.History, CompletedDispatches, CandidateRequestSessions.
	extra := base.Clone()
	obsExtra := extra.Facts.EvidenceApplicabilityObservations[0]
	obsExtra.Request.EvidenceRevision = "appended-rev"
	step2.Facts.EvidenceApplicabilityObservations = append(step2.Facts.EvidenceApplicabilityObservations, obsExtra)
	histExtra := extra.SemanticPlanning.History[0]
	histExtra.Reason = "appended-generation"
	step2.SemanticPlanning.History = append(step2.SemanticPlanning.History, histExtra)
	dispExtra := extra.CompletedDispatches[0]
	dispExtra.ID = "dispatch-appended"
	step2.CompletedDispatches = append(step2.CompletedDispatches, dispExtra)
	sessExtra := extra.CandidateRequestSessions[0]
	sessExtra.LogicalRequestID = "req-appended"
	step2.CandidateRequestSessions = append(step2.CandidateRequestSessions, sessExtra)
	consExtra := extra.ExecutionBudget.Consumptions[0]
	consExtra.ConsumptionID = "cons-appended"
	step2.ExecutionBudget.Consumptions = append(step2.ExecutionBudget.Consumptions, consExtra)
	recExtra := extra.Reconciliation.History[0]
	recExtra.RequestHash = "rec-appended"
	step2.Reconciliation.History = append(step2.Reconciliation.History, recExtra)

	step2b := step2.Clone()
	step2b.StateRevision++
	// Mutate SemanticPlanning.ActiveBatchID after partial Facts/SemanticPlanning reuse to verify shifted spans stay exact.
	step2b.SemanticPlanning.ActiveBatchID = "batch-next"
	step2b.Facts.EvidenceApplicabilityObservations = append(step2b.Facts.EvidenceApplicabilityObservations, obsExtra)
	suspExtra := extra.Facts.CWISuspensions[0]
	suspExtra.SuspendedEvidenceRevision = "susp-appended"
	step2b.Facts.CWISuspensions = append(step2b.Facts.CWISuspensions, suspExtra)

	step3 := step2b.Clone()
	step3.StateRevision++
	step3.ResultFeedbackVersion = 0
	// Replace slice contents / toggle omitempty fields in reverse.
	step3.CompletedDispatches[0].ID = "dispatch-mutated-prefix"
	step3.Facts.EvidenceApplicabilityObservations = nil
	step3.Facts.CWISuspensions = nil
	step3.SemanticPlanning.HistoryArchive = nil
	step3.DriveLease = nil

	assertPreimageSequence(t, []RuntimeStateRecord{base, step1, step1b, step2, step2b, step3})
}
