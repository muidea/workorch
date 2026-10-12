package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func candidatePayloadHashBufferFixtures(value string) (RuntimeOperation, []RuntimeCandidate) {
	integer, boolean := int64(-19), true
	operation := RuntimeOperation{
		ID: "op", Kind: StepKindFileWrite, RequiredOperationRef: "required", DependsOn: []string{"a", "b"},
		BindingRefs: []string{"binding"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}, TargetRefs: []string{"target.txt"},
		Parameters: []OperationParameter{{Name: "string", String: &value}, {Name: "integer", Integer: &integer}, {Name: "boolean", Boolean: &boolean}, {Name: "list", StringList: []string{"short", value}}, {Name: "empty"}},
	}
	assessment := &AssessmentCandidate{
		FocusID: "focus", SliceID: "slice", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}, Disposition: AssessmentDispositionRepairableGap,
		Items:   []RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi", Reason: value, EvidenceRefs: []string{"evidence"}, ArtifactRefs: []common.ArtifactID{"artifact"}}},
		Gap:     &RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}, MissingEvidenceKinds: []string{EvidenceKindValidation}, TargetRefs: []string{value}, EvidenceRefs: []string{"gap-evidence"}, Reason: value, FailureClass: FailureClassExecutionFailure, FailureCode: "failure", FailureSignature: "signature"},
		Summary: value, EvidenceRefs: []string{"evidence"}, ArtifactRefs: []string{"artifact"},
	}
	withoutGap := assessment.Normalized()
	withoutGap.Gap = nil
	return operation, []RuntimeCandidate{
		{},
		{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: operation}}}},
		{Validation: &ValidationCandidate{Operations: []ValidationOperation{{Operation: operation}}}},
		{Assessment: assessment},
		{Assessment: &withoutGap},
		{Governance: &GovernanceCandidate{ExternalDependency: ExternalDependency{Kind: "dependency", SubjectRef: value, Reason: value, RequestedAction: "execute", RiskLevel: "bounded"}}},
		// 保留现行 union 优先级，即使输入本身不具备执行准入条件。
		{StateChange: &StateChangeCandidate{}, Assessment: assessment},
	}
}

func assertCandidatePayloadHashBufferEquivalent(t testing.TB, value string) {
	t.Helper()
	operation, candidates := candidatePayloadHashBufferFixtures(value)
	originalOperation := operation.Normalized()
	beforeOperation, _ := json.Marshal(operation)
	for range 2 {
		if got, want := StableRuntimeOperationHash(operation), encodeRuntimeOperationHash(operation, sha256.New()); got != want {
			t.Fatal("完整 operation 长度/cardinality/正文 hash 漂移")
		}
	}
	afterOperation, _ := json.Marshal(operation)
	if string(beforeOperation) != string(afterOperation) || !reflect.DeepEqual(operation.Normalized(), originalOperation) || *operation.Parameters[0].String != value {
		t.Fatal("operation hash 改写原值")
	}
	for _, candidate := range candidates {
		before, _ := json.Marshal(candidate)
		for range 2 {
			if got, want := StableRuntimeCandidateHash(candidate), encodeRuntimeCandidateHash(candidate, sha256.New()); got != want {
				t.Fatal("完整 candidate 原 hash 编码漂移")
			}
		}
		after, _ := json.Marshal(candidate)
		if string(before) != string(after) {
			t.Fatal("candidate hash 改写原 bytes")
		}
	}
	body, _ := json.Marshal(operation)
	var reloaded RuntimeOperation
	if err := json.Unmarshal(body, &reloaded); err != nil {
		t.Fatal(err)
	}
	if StableRuntimeOperationHash(reloaded) != encodeRuntimeOperationHash(reloaded, sha256.New()) {
		t.Fatal("reload 后 hash 编码漂移")
	}
}

func TestCandidatePayloadHashBufferKeepsCompleteOriginalEncoding(t *testing.T) {
	for _, value := range []string{"", "原文\r\n", "A\x00B", "\xff\x80", strings.Repeat("x", 120), strings.Repeat("y", 121), strings.Repeat("正文", 32768)} {
		assertCandidatePayloadHashBufferEquivalent(t, value)
	}
	operation, candidates := candidatePayloadHashBufferFixtures("short")
	for _, candidate := range candidates[1:6] {
		current := testing.AllocsPerRun(20, func() { _ = StableRuntimeCandidateHash(candidate) })
		reference := testing.AllocsPerRun(20, func() { _ = encodeRuntimeCandidateHash(candidate, sha256.New()) })
		if current >= reference {
			t.Fatalf("candidate 短字段编码未减少分配: current=%v reference=%v", current, reference)
		}
	}
	current := testing.AllocsPerRun(20, func() { _ = StableRuntimeOperationHash(operation) })
	reference := testing.AllocsPerRun(20, func() { _ = encodeRuntimeOperationHash(operation, sha256.New()) })
	if current >= reference {
		t.Fatalf("operation 短字段编码未减少分配: current=%v reference=%v", current, reference)
	}
}

func FuzzCandidatePayloadHashBufferMatchesOriginalWriter(f *testing.F) {
	for _, value := range []string{"", "原文\r\n", "A\x00B", "\xff\x80", strings.Repeat("x", 120), strings.Repeat("y", 121)} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) { assertCandidatePayloadHashBufferEquivalent(t, value) })
}
