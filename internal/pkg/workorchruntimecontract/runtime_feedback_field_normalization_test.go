package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func fieldViolationNormalizationFullReference(v RuntimeFieldViolation) RuntimeFieldViolation {
	v = v.CloneOriginal()
	v.Path = strings.TrimSpace(v.Path)
	v.Actual = strings.TrimSpace(v.Actual)
	v.Allowed = normalizeOrderedStrings(v.Allowed)
	v.Required = normalizeOrderedStrings(v.Required)
	v.Forbidden = normalizeOrderedStrings(v.Forbidden)
	return v
}

func originalDiagnosticNormalizationFullReference(d *RuntimeOriginalDiagnostic) *RuntimeOriginalDiagnostic {
	if d == nil {
		return nil
	}
	canonical := d.CloneOriginal()
	canonical.SchemaVersion = strings.TrimSpace(canonical.SchemaVersion)
	canonical.OwnerModuleID = strings.TrimSpace(canonical.OwnerModuleID)
	canonical.OriginalClass = strings.TrimSpace(canonical.OriginalClass)
	canonical.OriginalCode = strings.TrimSpace(canonical.OriginalCode)
	canonical.OriginalMessage = strings.TrimSpace(canonical.OriginalMessage)
	canonical.Signature = strings.TrimSpace(canonical.Signature)
	canonical.OperationID = strings.TrimSpace(canonical.OperationID)
	canonical.TargetRefs = normalizeOrderedStrings(canonical.TargetRefs)
	canonical.ExpectedRevision = strings.TrimSpace(canonical.ExpectedRevision)
	canonical.ObservedRevision = strings.TrimSpace(canonical.ObservedRevision)
	for index := range canonical.FieldViolations {
		canonical.FieldViolations[index] = fieldViolationNormalizationFullReference(canonical.FieldViolations[index])
	}
	canonical.SemanticViolations = normalizeRuntimeSemanticContractViolations(canonical.SemanticViolations)
	canonical.DetailsRef = strings.TrimSpace(canonical.DetailsRef)
	canonical.DetailsHash = strings.TrimSpace(canonical.DetailsHash)
	canonical.ContentType = strings.TrimSpace(canonical.ContentType)
	canonical.OmittedReason = strings.TrimSpace(canonical.OmittedReason)
	if canonical.TargetObservation != nil {
		value := canonical.TargetObservation.Normalized()
		canonical.TargetObservation = &value
	}
	if canonical.ValidationDiagnostic != nil {
		value := canonical.ValidationDiagnostic.Normalized()
		canonical.ValidationDiagnostic = &value
	}
	if canonical.ValidationFailureObservation != nil {
		value := canonical.ValidationFailureObservation.Normalized()
		canonical.ValidationFailureObservation = &value
	}
	if canonical.GovernanceDependency != nil {
		value := canonical.GovernanceDependency.Normalized()
		canonical.GovernanceDependency = &value
	}
	return canonical
}

func assertDiagnosticNormalizationDetached(t *testing.T, original, normalized reflect.Value) {
	t.Helper()
	if !original.IsValid() || !normalized.IsValid() {
		return
	}
	switch original.Kind() {
	case reflect.Pointer:
		if original.IsNil() || normalized.IsNil() {
			return
		}
		if original.Pointer() == normalized.Pointer() {
			t.Fatal("诊断规范化泄漏原嵌套 pointer")
		}
		assertDiagnosticNormalizationDetached(t, original.Elem(), normalized.Elem())
	case reflect.Slice:
		if original.Len() == 0 || normalized.Len() == 0 {
			return
		}
		if original.Pointer() == normalized.Pointer() {
			t.Fatal("诊断规范化泄漏原嵌套 slice")
		}
		for index := range min(original.Len(), normalized.Len()) {
			assertDiagnosticNormalizationDetached(t, original.Index(index), normalized.Index(index))
		}
	case reflect.Struct:
		for index := range original.NumField() {
			assertDiagnosticNormalizationDetached(t, original.Field(index), normalized.Field(index))
		}
	}
}

func assertDiagnosticFieldNormalizationReference(t *testing.T, value *RuntimeOriginalDiagnostic) {
	t.Helper()
	before, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	want := originalDiagnosticNormalizationFullReference(value)
	for range 2 {
		got := value.Normalized()
		if !reflect.DeepEqual(got, want) {
			t.Fatal("诊断比较副本偏离原完整规范化／有序集合／nil/empty 表示")
		}
		assertDiagnosticNormalizationDetached(t, reflect.ValueOf(value), reflect.ValueOf(got))
	}
	if value != nil {
		if !reflect.DeepEqual(value.CloneOriginal(), value) {
			t.Fatal("原诊断 raw Clone 不再保全原值")
		}
		gotErr, wantErr := value.Normalized().Validate(), want.Validate()
		if (gotErr == nil) != (wantErr == nil) || (gotErr != nil && gotErr.Error() != wantErr.Error()) {
			t.Fatalf("完整诊断拒绝原因漂移：got=%v want=%v", gotErr, wantErr)
		}
	}
	after, err := json.Marshal(value)
	if err != nil || string(before) != string(after) {
		t.Fatal("诊断比较副本改写原 bytes")
	}
	var reloaded *RuntimeOriginalDiagnostic
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.Normalized(), originalDiagnosticNormalizationFullReference(reloaded)) {
		t.Fatal("诊断重载后规范化漂移")
	}
}

func diagnosticFieldNormalizationFixture(count int) *RuntimeOriginalDiagnostic {
	value := validRuntimeFeedback().OriginalDiagnostic
	value.FieldViolations = nil
	for range count {
		value.FieldViolations = append(value.FieldViolations, RuntimeFieldViolation{Path: " $.operations[0] ", Actual: " original ", Allowed: []string{" b ", "a", " b ", " "}, Required: []string{" command "}, Forbidden: []string{" payload "}})
	}
	return value
}

func TestDiagnosticFieldNormalizationMatchesOriginalAndRawIdentity(t *testing.T) {
	complete := diagnosticFieldNormalizationFixture(16)
	complete.SemanticViolations = []RuntimeSemanticContractViolation{{Allowed: []string{" b ", "a"}, Required: []string{" required "}}}
	complete.TargetObservation = &RuntimeFailureTargetObservation{TargetRefs: []string{" b ", "a"}, EvidenceRefs: []string{" evidence "}}
	complete.ValidationDiagnostic = &ValidationFailureDiagnostic{Reason: " 原报告\r\n\x00 ", TargetRefs: []string{" b ", "a"}, InputLocations: []RuntimeValidationInputLocation{{}}}
	complete.ValidationFailureObservation = &RuntimeValidationFailureObservation{CandidateTargetRefs: []string{" b "}, ValidationScopeTargetRefs: []string{" b ", "a"}}
	complete.GovernanceDependency = &ExternalDependency{Kind: " decision ", Reason: " 原理由 "}
	for _, value := range []*RuntimeOriginalDiagnostic{nil, {}, complete,
		{FieldViolations: []RuntimeFieldViolation{}}, {FieldViolations: []RuntimeFieldViolation{{Allowed: []string{}, Required: []string{}, Forbidden: []string{}}}},
		{FieldViolations: []RuntimeFieldViolation{{Path: " 原路径 ", Actual: string([]byte{0xff, 0x80}), Allowed: []string{" \x00 ", " ", strings.Repeat("原", 32768)}}}},
	} {
		assertDiagnosticFieldNormalizationReference(t, value)
	}
	value := diagnosticFieldNormalizationFixture(2)
	feedback := validRuntimeFeedback()
	feedback.OriginalDiagnostic = value
	originalHash := StableRuntimeFeedbackHash(feedback)
	got := value.Normalized()
	got.FieldViolations[0].Allowed[0] = "changed"
	got.FieldViolations[0].Required[0] = "changed"
	got.FieldViolations[0].Forbidden[0] = "changed"
	if StableRuntimeFeedbackHash(feedback) != originalHash {
		t.Fatal("诊断比较副本修改原持久 hash")
	}
	value.FieldViolations[0].Actual = "新的原事实"
	if StableRuntimeFeedbackHash(feedback) == originalHash {
		t.Fatal("后续诊断事实未重新计算 raw hash")
	}
	assertDiagnosticFieldNormalizationReference(t, value)
}

func TestDiagnosticFieldNormalizationReducesDiscardedCopies(t *testing.T) {
	value := diagnosticFieldNormalizationFixture(16)
	original := testing.AllocsPerRun(20, func() { _ = originalDiagnosticNormalizationFullReference(value) })
	current := testing.AllocsPerRun(20, func() { _ = value.Normalized() })
	if current >= original {
		t.Fatalf("诊断比较副本分配未减少：current=%v original=%v", current, original)
	}
}

func FuzzDiagnosticFieldNormalizationReference(f *testing.F) {
	f.Add(" $.path ", " 原事实\r\n\x00 ", " b ", "a")
	f.Add("", string([]byte{0xff, 0x80}), "", "")
	f.Fuzz(func(t *testing.T, path, actual, first, second string) {
		value := diagnosticFieldNormalizationFixture(3)
		value.FieldViolations[0] = RuntimeFieldViolation{Path: path, Actual: actual, Allowed: []string{first, second, first, " "}, Required: []string{second}, Forbidden: []string{first}}
		assertDiagnosticFieldNormalizationReference(t, value)
		if !reflect.DeepEqual(value.FieldViolations[0].Normalized(), fieldViolationNormalizationFullReference(value.FieldViolations[0])) {
			t.Fatal("字段违规独立规范化漂移")
		}
	})
}

func BenchmarkDiagnosticFieldNormalization(b *testing.B) {
	value := diagnosticFieldNormalizationFixture(16)
	for _, mode := range []string{"reference", "current"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if mode == "reference" {
					_ = originalDiagnosticNormalizationFullReference(value)
				} else {
					_ = value.Normalized()
				}
			}
		})
	}
}
