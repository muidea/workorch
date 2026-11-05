package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func compareAssessmentAuthoritySinglePass(t testing.TB, a RuntimeAssessmentAuthorityInput, transaction AuthorityTransaction, bindings []CoverageEvidenceBinding) {
	t.Helper()
	before := cloneRuntimeAssessmentAuthorityInputRaw(a)
	wantErr := referenceAssessmentAuthorityBeforeSinglePass(a, transaction, bindings)
	got, err := validateAndNormalizeRuntimeAssessmentAuthorityInput(a, transaction, bindings)
	if fmt.Sprint(err) != fmt.Sprint(wantErr) || fmt.Sprint(a.Validate(transaction, bindings)) != fmt.Sprint(wantErr) {
		t.Fatalf("authority first error changed: %v != %v", err, wantErr)
	}
	if err != nil {
		if !reflect.DeepEqual(got, RuntimeAssessmentAuthorityInput{}) {
			t.Fatal("rejected authority exposed a usable product")
		}
	} else if !reflect.DeepEqual(got, a.Normalized()) {
		t.Fatal("authority product changed")
	}
	if !reflect.DeepEqual(before, a) {
		t.Fatal("authority validation mutated input")
	}
	// Mutating a successful product must not affect the input used by other
	// consumers, a subsequent cold query or the sealed transaction.
	if len(got.Focus.CoverageWorkItemIDs) != 0 {
		got.Focus.CoverageWorkItemIDs[0] = "changed"
	}
	if len(got.OperationAdmissibilities) != 0 {
		got.OperationAdmissibilities[0].OwnerModuleID = "changed"
	}
	if len(got.ApplicabilityObservations) != 0 && got.ApplicabilityObservations[0].CurrentDependencies != nil {
		got.ApplicabilityObservations[0].CurrentDependencies.SetHash = "changed"
	}
	if !reflect.DeepEqual(before, a) {
		t.Fatal("authority product aliases original")
	}
}

func compareAssessmentAdjudicationSinglePass(t testing.TB, a RuntimeAssessmentAdjudication) {
	t.Helper()
	before := cloneRuntimeAssessmentAdjudicationRaw(a)
	wantErr := referenceAssessmentAdjudicationBeforeSinglePass(a)
	got, err := validateAndNormalizeRuntimeAssessmentAdjudication(a)
	if fmt.Sprint(err) != fmt.Sprint(wantErr) || fmt.Sprint(a.ValidateRaw()) != fmt.Sprint(wantErr) {
		t.Fatalf("adjudication first error changed: %v != %v", err, wantErr)
	}
	if err != nil {
		if !reflect.DeepEqual(got, RuntimeAssessmentAdjudication{}) {
			t.Fatal("rejected adjudication exposed a usable product")
		}
	} else if !reflect.DeepEqual(got, a.Normalized()) {
		t.Fatal("adjudication product changed")
	}
	if len(got.CoverageWorkItemIDs) != 0 {
		got.CoverageWorkItemIDs[0] = "changed"
	}
	if len(got.Proofs) != 0 {
		got.Proofs[0].EvidenceFactHash = "changed"
		if got.Proofs[0].OwnerRevisionSet != nil {
			got.Proofs[0].OwnerRevisionSet.SetHash = "changed"
		}
	}
	if len(got.OwnerFactSources) != 0 && got.OwnerFactSources[0].OwnerRevisionSet != nil {
		got.OwnerFactSources[0].OwnerRevisionSet.SetHash = "changed"
	}
	if !reflect.DeepEqual(before, a) {
		t.Fatal("adjudication validation/product mutated input")
	}
}

func TestAssessmentSinglePassOriginalErrorsAndProducts(t *testing.T) {
	focus := CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}.Normalized()
	identity := RuntimeEquivalentAuthorityInputIdentity{
		DefinitionRevision: "definition-1", FocusHash: StableRuntimeAuthorityFocusHash(focus),
		EvidenceRevision: "1", CapabilityStateHash: "capability-1",
		CatalogRevisionHash: StableRuntimeOperationAdmissibilityCatalogRevisionHash(nil),
	}.Normalized()
	transaction := AuthorityTransaction{FocusID: focus.ID, State: ConvergenceStateNeedAssessment, AuthorityInputID: StableRuntimeEquivalentAuthorityInputID(identity)}
	authority := RuntimeAssessmentAuthorityInput{AuthorityInput: identity, Purpose: RuntimeAssessmentPurposeDefinitionProof,
		SourceSliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", SourceText: "behavior is verified", Focus: focus}.Normalized()
	adjudication := RuntimeAssessmentAdjudication{DispatchID: "dispatch-1", AuthorityInputID: transaction.AuthorityInputID, FocusID: focus.ID,
		CoverageWorkItemIDs: focus.CoverageWorkItemIDs, Disposition: AssessmentDispositionSatisfied,
		Proofs: []RuntimeAssessmentAdjudicationProof{{CoverageWorkItemID: "cwi-1", EvidenceKind: EvidenceKindValidation, EvidenceFactID: "evidence-1", EvidenceFactHash: "fact-hash-1"}}}.Normalized()
	if err := authority.Validate(transaction, nil); err != nil {
		t.Fatal(err)
	}
	if err := adjudication.ValidateRaw(); err != nil {
		t.Fatal(err)
	}
	compareAssessmentAuthoritySinglePass(t, authority, transaction, nil)
	compareAssessmentAdjudicationSinglePass(t, adjudication)
	// Each raw field participates in rejection or canonical projection; these
	// mutations are not normalized before comparison with the original validator.
	for i := 0; i < reflect.TypeFor[RuntimeAssessmentAuthorityInput]().NumField(); i++ {
		t.Run("authority-zero-"+reflect.TypeFor[RuntimeAssessmentAuthorityInput]().Field(i).Name, func(t *testing.T) {
			a := cloneRuntimeAssessmentAuthorityInputRaw(authority)
			reflect.ValueOf(&a).Elem().Field(i).SetZero()
			compareAssessmentAuthoritySinglePass(t, a, transaction, nil)
		})
	}
	for i := 0; i < reflect.TypeFor[RuntimeAssessmentAdjudication]().NumField(); i++ {
		t.Run("adjudication-zero-"+reflect.TypeFor[RuntimeAssessmentAdjudication]().Field(i).Name, func(t *testing.T) {
			a := cloneRuntimeAssessmentAdjudicationRaw(adjudication)
			reflect.ValueOf(&a).Elem().Field(i).SetZero()
			compareAssessmentAdjudicationSinglePass(t, a)
		})
	}
	for _, mutate := range []func(*RuntimeAssessmentAuthorityInput){
		func(a *RuntimeAssessmentAuthorityInput) { a.SourceText += " " },
		func(a *RuntimeAssessmentAuthorityInput) {
			a.OperationAdmissibilities = []RuntimeOperationAdmissibility{}
		},
		func(a *RuntimeAssessmentAuthorityInput) {
			a.Focus.CoverageWorkItemIDs = append(a.Focus.CoverageWorkItemIDs, "cwi-1")
		},
		func(a *RuntimeAssessmentAuthorityInput) { a.AuthorityInput.FocusHash = "other-focus" },
		func(a *RuntimeAssessmentAuthorityInput) { a.Purpose = "unsupported"; a.SourceText = "" },
	} {
		a := cloneRuntimeAssessmentAuthorityInputRaw(authority)
		mutate(&a)
		compareAssessmentAuthoritySinglePass(t, a, transaction, nil)
	}
	for _, mutate := range []func(*RuntimeAssessmentAdjudication){
		func(a *RuntimeAssessmentAdjudication) { a.Proofs = append(a.Proofs, a.Proofs[0]) },
		func(a *RuntimeAssessmentAdjudication) { a.Proofs[0].EvidenceFactHash += " " },
		func(a *RuntimeAssessmentAdjudication) { a.Proofs[0].OwnerRevisionSet = &RuntimeOwnerRevisionSet{} },
		func(a *RuntimeAssessmentAdjudication) { a.Proofs = []RuntimeAssessmentAdjudicationProof{} },
		func(a *RuntimeAssessmentAdjudication) { a.Disposition = "unsupported"; a.DispatchID = "" },
	} {
		a := cloneRuntimeAssessmentAdjudicationRaw(adjudication)
		mutate(&a)
		compareAssessmentAdjudicationSinglePass(t, a)
	}
	wrong := transaction
	wrong.AuthorityInputID = "another-workspace-authority"
	compareAssessmentAuthoritySinglePass(t, authority, wrong, nil)
	wrong.State = ConvergenceStateNeedStateChange
	compareAssessmentAuthoritySinglePass(t, authority, wrong, nil)
}

func TestAssessmentSinglePassCompleteArchive(t *testing.T) {
	body, path, runs := applicabilityScanArchive(t)
	count := 0
	for _, run := range runs {
		for _, dispatch := range run.RuntimeState.CompletedDispatches {
			if dispatch.AssessmentAuthorityInput == nil {
				continue
			}
			count++
			a := *dispatch.AssessmentAuthorityInput
			compareAssessmentAuthoritySinglePass(t, a, dispatch.Transaction, dispatch.CoverageBindings)
			if dispatch.AssessmentAdjudication != nil {
				compareAssessmentAdjudicationSinglePass(t, *dispatch.AssessmentAdjudication)
			}
			wrong := dispatch.Transaction
			wrong.FocusID += "-another-workspace"
			compareAssessmentAuthoritySinglePass(t, a, wrong, dispatch.CoverageBindings)
			compareAssessmentAuthoritySinglePass(t, a, dispatch.Transaction, nil)
			a.SourceText += " "
			compareAssessmentAuthoritySinglePass(t, a, dispatch.Transaction, dispatch.CoverageBindings)
			// Exact JSON reload cannot inherit a previous successful product.
			encoded, err := json.Marshal(dispatch)
			if err != nil {
				t.Fatal(err)
			}
			var loaded RuntimeDispatchRecord
			if err := json.Unmarshal(encoded, &loaded); err != nil {
				t.Fatal(err)
			}
			compareAssessmentAuthoritySinglePass(t, *loaded.AssessmentAuthorityInput, loaded.Transaction, loaded.CoverageBindings)
			if loaded.AssessmentAdjudication != nil {
				compareAssessmentAdjudicationSinglePass(t, *loaded.AssessmentAdjudication)
			}
		}
	}
	if count == 0 {
		t.Fatal("archive has no assessment authority")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("replay changed original archive")
	}
}
