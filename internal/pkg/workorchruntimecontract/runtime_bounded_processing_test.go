package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestDefinitionProofCompilationRetainsOnlyExactDetachedDeclaration(t *testing.T) {
	r := semanticDefinitionSessionFixture(t)
	item, source := r.Facts.DefinitionSeed.CoverageItems[0], r.Facts.DefinitionFocuses[0]
	s := newRuntimeSemanticValidationSession()
	revision := r.Facts.DefinitionSeed.DefinitionRevision
	want, err := compileRuntimeSemanticDefinitionProofObligation(revision, item, source)
	if err != nil {
		t.Fatal(err)
	}
	first, err := compileRuntimeDefinitionProofInSession(revision, item, source, s)
	if err != nil || !reflect.DeepEqual(first, want) {
		t.Fatal("declaration differs", err)
	}
	first.RequiredEvidenceKinds[0] = "caller mutation"
	again, err := compileRuntimeDefinitionProofInSession(revision, item, source, s)
	if err != nil || !reflect.DeepEqual(again, want) || len(s.definitionProofs) != 1 {
		t.Fatal("output aliases retained pure product", err)
	}
	kept := s
	if len(kept.definitionProofs) != 1 {
		t.Fatal("pure declaration lost within transaction")
	}
	for _, mutate := range []func(*RuntimeCoverageItem, *RuntimeDefinitionFocus){
		func(i *RuntimeCoverageItem, _ *RuntimeDefinitionFocus) { i.TargetText = "different original" },
		func(_ *RuntimeCoverageItem, f *RuntimeDefinitionFocus) { f.RequiredEvidenceKinds = []string{"foreign"} },
		func(_ *RuntimeCoverageItem, f *RuntimeDefinitionFocus) { f.EvidenceDeclarationRef = "different source" },
	} {
		i, f := cloneRuntimeCoverageItemRaw(item), cloneRuntimeDefinitionFocusRaw(source)
		mutate(&i, &f)
		actual, gotErr := compileRuntimeDefinitionProofInSession(revision, i, f, kept)
		expected, wantErr := compileRuntimeSemanticDefinitionProofObligation(revision, i, f)
		if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) || !reflect.DeepEqual(actual, expected) {
			t.Fatal("changed raw input borrowed another declaration", gotErr, wantErr)
		}
	}
	if !reflect.DeepEqual(s.definitionProofs[0].result, want) {
		t.Fatal("caller changed saved declaration")
	}
}

func TestAbsentHistoricalConsumersDoNotCloneSourceSuperset(t *testing.T) {
	s := RuntimeLineageAssertionSources{Facts: RuntimeFacts{Coverage: []RuntimeCoverageItemFact{{ID: "ordinary"}}}}
	compiler := &runtimeHistoricalSourceCompiler{}
	if err := ValidateRuntimeHistoricalBehaviorCompletions(s.Facts, s.Completed); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeAssessmentAdjudications(s.Facts, nil); err != nil {
		t.Fatal(err)
	}
	if len(compiler.images) != 0 {
		t.Fatal("vacuous consumer cloned complete owner graph")
	}
	s.Facts.Coverage[0].HistoricalAcceptanceProofs = []RuntimeHistoricalAcceptanceProof{{}}
	if err := ValidateRuntimeHistoricalBehaviorCompletions(s.Facts, s.Completed); err == nil {
		t.Fatal("orphan proof skipped its source gate")
	}
	completed := []RuntimeDispatchRecord{{ID: "assessment", State: RuntimeDispatchStateCommitted, Candidate: RuntimeCandidate{Assessment: &AssessmentCandidate{}}}}
	if err := ValidateRuntimeAssessmentAdjudications(s.Facts, completed); err == nil {
		t.Fatal("committed consumer with missing fact skipped its gate")
	}
	if len(compiler.images) != 0 {
		t.Fatal("failed consumers retained a success")
	}
}

func TestFormalValueStreamingHashMatchesCompleteOriginalJSON(t *testing.T) {
	for _, value := range []any{nil, " 原文\x00\r\n<>&\u2028" + string([]byte{0xff}), []string{}, []string(nil), strings.Repeat("长原文\r\n", 65536), map[string]any{"z": "last", "a": []int{1, 2}}, validationOutputStorageFixture(), math.NaN(), func() {}} {
		kind := "original-domain\x00"
		payload, _ := json.Marshal(value)
		digest := sha256.Sum256(append([]byte(kind+"\x00"), payload...))
		want := kind + "-sha256:" + hex.EncodeToString(digest[:])
		if got := stableFormalValueHash(kind, value); got != want {
			t.Fatal("hash changed exact original JSON bytes")
		}
	}
}

func TestFeedbackHashReadOnlyEncodingKeepsEveryOriginalField(t *testing.T) {
	var feedback RuntimeFeedback
	sequence := 0
	populateRuntimeStateRawCloneFixture(reflect.ValueOf(&feedback).Elem(), map[reflect.Type]int{}, &sequence)
	feedback.OriginalDiagnostic.OriginalMessage = strings.Repeat("原始诊断\x00\r\n", 65536)
	before := feedback.CloneOriginal()
	if got, want := StableRuntimeFeedbackHash(feedback), referenceRuntimeFeedbackHashBeforeReadOnlyEncoding(feedback); got != want || !reflect.DeepEqual(feedback, before) {
		t.Fatal("read-only encoding changed original bytes or hash")
	}
	for i := range reflect.TypeOf(*feedback.OriginalDiagnostic).NumField() {
		changed := feedback.CloneOriginal()
		field := reflect.ValueOf(changed.OriginalDiagnostic).Elem().Field(i)
		field.Set(reflect.Zero(field.Type()))
		if got, want := StableRuntimeFeedbackHash(changed), referenceRuntimeFeedbackHashBeforeReadOnlyEncoding(changed); got != want {
			t.Fatalf("diagnostic field %d encoding differs", i)
		}
	}
}

func BenchmarkFormalValueHashCompleteOriginal(b *testing.B) {
	value := validationOutputStorageFixture()
	for _, mode := range []string{"original", "streamed"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "original" {
					payload, _ := json.Marshal(value)
					_ = sha256.Sum256(append([]byte("runtime-validation\x00"), payload...))
				} else {
					_ = stableFormalValueHash("runtime-validation", value)
				}
			}
		})
	}
}

func BenchmarkDefinitionProofExactOwnerReuse(b *testing.B) {
	r := semanticDefinitionSessionFixture(b)
	i, f := r.Facts.DefinitionSeed.CoverageItems[0], r.Facts.DefinitionFocuses[0]
	for _, mode := range []string{"cold", "owner_pure_reuse"} {
		b.Run(mode, func(b *testing.B) {
			s := newRuntimeSemanticValidationSession()
			if mode == "cold" {
				s = nil
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := compileRuntimeDefinitionProofInSession(r.Facts.DefinitionSeed.DefinitionRevision, i, f, s); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
