package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func runtimeAssessmentAuthorityInputHashReference(value RuntimeAssessmentAuthorityInput) string {
	value = value.Normalized()
	payload, _ := json.Marshal(value)
	digest := sha256.Sum256(append([]byte("runtime-assessment-authority-input/v1\x00"), payload...))
	return hex.EncodeToString(digest[:])
}

func TestAssessmentAuthorityInputHashPreservesExactCanonicalEncoding(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	values := []RuntimeAssessmentAuthorityInput{
		{},
		{Purpose: RuntimeAssessmentPurposeDefinitionProof, SourceSliceID: "slice", SourceScope: "dodItem", SourceID: "source", SourceText: "  原始判据\r\n含 NUL\x00与\xff  ", Focus: CoverageFocus{ID: binding.FocusID, CoverageEvidenceBindings: []CoverageEvidenceBinding{binding}}},
		{Purpose: RuntimeAssessmentPurposeOwnerFactAcquisition, SourceText: strings.Repeat("完整长判据", 8192), OperationAdmissibilities: []RuntimeOperationAdmissibility{{OwnerModuleID: "owner", PrerequisiteID: "prerequisite", SnapshotID: "snapshot"}}},
	}
	for _, value := range values {
		before, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		want := runtimeAssessmentAuthorityInputHashReference(value)
		for range 2 {
			if got := StableRuntimeAssessmentAuthorityInputHash(value); got != want {
				t.Fatalf("assessment authority hash 编码漂移: got=%s want=%s", got, want)
			}
		}
		after, err := json.Marshal(value)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("hash 改写原始 authority input")
		}
		canonical, err := json.Marshal(value.Normalized())
		if err != nil {
			t.Fatal(err)
		}
		var reloaded RuntimeAssessmentAuthorityInput
		if err := json.Unmarshal(canonical, &reloaded); err != nil {
			t.Fatal(err)
		}
		if got := StableRuntimeAssessmentAuthorityInputHash(reloaded); got != runtimeAssessmentAuthorityInputHashReference(reloaded) {
			t.Fatal("JSON 重载后的 assessment hash 偏离原编码合同")
		}
		if utf8.ValidString(value.SourceText) && StableRuntimeAssessmentAuthorityInputHash(reloaded) != want {
			t.Fatal("合法 UTF-8 canonical 重载改变 assessment hash")
		}
	}
}

func FuzzAssessmentAuthorityInputHashEncoding(f *testing.F) {
	for _, text := range []string{"", "中文\r\n\x00", "invalid\xff", strings.Repeat("x", 1023), strings.Repeat("x", 1024)} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		value := RuntimeAssessmentAuthorityInput{Purpose: RuntimeAssessmentPurposeDefinitionProof, SourceScope: "dodItem", SourceID: "source", SourceText: text}
		if got, want := StableRuntimeAssessmentAuthorityInputHash(value), runtimeAssessmentAuthorityInputHashReference(value); got != want {
			t.Fatalf("完整 JSON 域前缀编码漂移: got=%s want=%s", got, want)
		}
	})
}

var runtimeAssessmentAuthorityInputHashSink string

func BenchmarkAssessmentAuthorityInputHashEncoding(b *testing.B) {
	value := RuntimeAssessmentAuthorityInput{Purpose: RuntimeAssessmentPurposeDefinitionProof, SourceText: strings.Repeat("完整长判据", 8192)}
	for _, test := range []struct {
		name string
		hash func(RuntimeAssessmentAuthorityInput) string
	}{{"reference", runtimeAssessmentAuthorityInputHashReference}, {"current", StableRuntimeAssessmentAuthorityInputHash}} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				runtimeAssessmentAuthorityInputHashSink = test.hash(value)
			}
		})
	}
}
