package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRecordLineageSourcesRetainEveryRawField(t *testing.T) {
	var record RuntimeStateRecord
	sequence := 0
	populateRuntimeStateRawCloneFixture(reflect.ValueOf(&record).Elem(), map[reflect.Type]int{}, &sequence)
	source := reflect.ValueOf(runtimeLineageSourcesFromRecord(record))
	names := map[string]string{
		"Work": "ImplementationWork", "Semantic": "SemanticPlanning", "Active": "ActiveDispatch",
		"Completed": "CompletedDispatches", "Receipts": "OperationReceipts", "Governance": "OperationGovernanceBindings",
		"Budget": "ExecutionBudget", "Resolutions": "GovernanceResolutions", "Attempts": "CandidateAttempts",
		"Sessions": "CandidateRequestSessions", "ProofLineage": "CoverageProofLineage", "Confirmations": "RepairSurfaceConfirmations",
	}
	for i := 0; i < source.NumField(); i++ {
		name := source.Type().Field(i).Name
		field := name
		if mapped := names[name]; mapped != "" {
			field = mapped
		}
		want := reflect.ValueOf(record).FieldByName(field)
		if !want.IsValid() || !reflect.DeepEqual(source.Field(i).Interface(), want.Interface()) {
			t.Fatalf("record source omitted or normalized %s", name)
		}
	}
}

func inputReviewCompilerFixture(t *testing.T) (RuntimeSemanticPlanningLedger, CoverageEvidenceBinding) {
	t.Helper()
	manifest, document, facts, _ := semanticInputReviewFixture(t)
	ids := []common.CoverageWorkItemID{}
	for _, item := range manifest.CoverageItems {
		ids = append(ids, item.ID)
	}
	order := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: ids}
	batch := semanticContractTestPendingBatch(t, manifest, order, 0, "review-compiler-request", document.BatchCoverageWorkItemIDs, document.BatchCoverageWorkItemIDs)
	document.BatchID, document.BatchSequence = batch.BatchID, batch.Sequence
	batch.State, batch.DocumentHash = RuntimeSemanticPlanBatchAccepted, StableRuntimeSemanticPlanDocumentHash(document)
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	ledger := RuntimeSemanticPlanningLedger{Manifest: &manifest, Document: &document,
		DocumentHash: batch.DocumentHash, FeasibilitySetHash: StableRuntimeSemanticFeasibilitySetHash(facts),
		Batches: []RuntimeSemanticPlanBatch{batch}, BatchDocuments: []RuntimeSemanticPlanDocument{document},
		Feasibilities: facts, FeasibilityBatchID: StableRuntimeSemanticFeasibilityBatchID(manifest, document)}
	entries, err := runtimeAssessmentPurposePreimagesFromSemanticLedger(ledger, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		for _, binding := range entry.bindings {
			if _, err := CompileRuntimeBindingValidationInputReview(binding, ledger); err == nil {
				return ledger, binding
			}
		}
	}
	t.Fatal("fixture has no accepted validation-input review")
	return RuntimeSemanticPlanningLedger{}, CoverageEvidenceBinding{}
}

func TestHistoricalInputReviewCompilerExactBindingLedgerAndDetachedResult(t *testing.T) {
	ledger, binding := inputReviewCompilerFixture(t)
	session := &runtimeHistoricalPreimageSession{}
	compare := func(b CoverageEvidenceBinding, l RuntimeSemanticPlanningLedger) {
		t.Helper()
		got, err := compileRuntimeBindingValidationInputReviewInSession(b, l, session)
		want, wantErr := referenceLineagePreimageCompileRuntimeBindingValidationInputReview(b, l)
		if fmt.Sprint(err) != fmt.Sprint(wantErr) || !reflect.DeepEqual(got, want) {
			t.Fatalf("review projection/result/first error changed: %v / %v", err, wantErr)
		}
		if got.InputReview != nil {
			got.InputReview.Files[0].Content = "caller mutation"
		}
	}
	compare(binding, ledger)
	compare(binding, ledger)
	if len(session.reviews) != 1 {
		t.Fatal("same complete raw sources did not share the local pure projection")
	}
	changedBinding := cloneCoverageEvidenceBindingRaw(binding)
	ownerLineageFieldMutations(reflect.ValueOf(&changedBinding).Elem(), func() { compare(changedBinding, ledger) })
	reviewCount := len(session.reviews)
	changedLedger := cloneRuntimeSemanticPlanningLedgerRaw(ledger)
	changedLedger.Manifest.OwnerCatalogs[0].SnapshotID = "same hash but different complete raw source"
	compare(binding, changedLedger)
	compare(binding, ledger)
	if len(session.reviews) != reviewCount {
		t.Fatal("failed ledger added a cached successful projection")
	}
	// A distinct but valid raw preimage must replace the complete per-ledger
	// container, even when its additional field does not affect the result.
	changedLedger = cloneRuntimeSemanticPlanningLedgerRaw(ledger)
	changedLedger.StablePrefixHash = "different raw ledger"
	compare(binding, changedLedger)
	if session.ledger.StablePrefixHash != changedLedger.StablePrefixHash || len(session.reviews) != 1 {
		t.Fatal("new complete raw ledger retained old projection container")
	}
	compare(binding, ledger)
}
