package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func semanticProposalSortFullReference(values []RuntimeSemanticProofProposal) []RuntimeSemanticProofProposal {
	ret := make([]RuntimeSemanticProofProposal, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.CoverageWorkItemID != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].CoverageWorkItemID != ret[j].CoverageWorkItemID {
			return ret[i].CoverageWorkItemID < ret[j].CoverageWorkItemID
		}
		return StableRuntimeSemanticProofProposalHash(ret[i]) < StableRuntimeSemanticProofProposalHash(ret[j])
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func semanticProposalSortFixture(count int, sameCWI bool) []RuntimeSemanticProofProposal {
	values := make([]RuntimeSemanticProofProposal, 0, count)
	for index := range count {
		id := " cwi "
		if !sameCWI {
			id = fmt.Sprintf(" cwi-%02d ", count-index)
		}
		values = append(values, RuntimeSemanticProofProposal{
			CoverageWorkItemID: common.CoverageWorkItemID(id), OwnerModuleID: " owner ", OperationKind: " kind ", DeclarationRef: fmt.Sprintf(" declaration-%02d ", index),
			Target:                RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{" b ", "a", "a"}},
			RequiredEvidenceKinds: []string{EvidenceKindValidation, EvidenceKindValidation}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession}, RequiredCheckRefs: []string{" check ", "check"},
			ValidationInputReview: &RuntimeValidationInputReview{SubjectHash: "subject", Checks: []RuntimeValidationInputCheckReview{{
				RequiredCheckRef: "check", Conclusion: "consistent", Reason: " 原报告\r\n\x00 ", InputTargetRefs: []string{"input"},
				Obligations: []RuntimeValidationInputObligationReview{{CriterionText: " 原判据 ", Finding: "covered", Reason: "原理由", InputTargetRefs: []string{"input"}}},
			}}},
		})
	}
	return values
}

func assertSemanticProposalSortReference(t testing.TB, values []RuntimeSemanticProofProposal) {
	t.Helper()
	before, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	want := semanticProposalSortFullReference(values)
	for range 2 {
		got := normalizeRuntimeSemanticProofProposals(values)
		if !reflect.DeepEqual(got, want) {
			t.Fatal("proposal 排序偏离原完整稳定排序")
		}
		for _, proposal := range got {
			if stableRuntimeSemanticProofProposalHashCanonical(proposal) != semanticProposalHashFullEncodingReference(proposal) {
				t.Fatal("本次规范化 proposal 编码偏离原 hash")
			}
		}
	}
	after, err := json.Marshal(values)
	if err != nil || string(before) != string(after) {
		t.Fatal("排序改写原 proposal bytes")
	}
	var reloaded []RuntimeSemanticProofProposal
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalizeRuntimeSemanticProofProposals(reloaded), semanticProposalSortFullReference(reloaded)) {
		t.Fatal("JSON 重载后排序偏离原路径")
	}
}

func TestSemanticProposalSortMatchesOriginalStableOrder(t *testing.T) {
	mixed := semanticProposalSortFixture(32, true)
	for index := range mixed {
		mixed[index].CoverageWorkItemID = common.CoverageWorkItemID(fmt.Sprintf(" cwi-%d ", index%3))
	}
	mixed = append(mixed, RuntimeSemanticProofProposal{CoverageWorkItemID: " \t "})
	equal := semanticProposalSortFixture(1, true)
	other := equal[0].Normalized()
	other.ValidationInputReview.Checks[0].InputTargetRefs = []string{}
	other.ValidationInputReview.Checks[0].Obligations = nil
	equal[0] = other
	other = other.Normalized()
	other.ValidationInputReview.Checks[0].InputTargetRefs = nil
	// nil/empty 不参与原 hash，但原稳定排序须保留各自 raw review 表示的先后。
	equal = append(equal, other, equal[0])
	if semanticProposalHashFullEncodingReference(equal[0]) != semanticProposalHashFullEncodingReference(equal[1]) || reflect.DeepEqual(equal[0], equal[1]) {
		t.Fatal("缺少同 hash、不同 raw 表示的稳定排序前像")
	}
	long := semanticProposalSortFixture(8, true)
	for index := range long {
		long[index].DeclarationRef += strings.Repeat("原", 32768)
		long[index].ValidationInputReview.Checks[0].Reason = string([]byte{0xff, 0x80}) + "\r\n\x00" + strings.Repeat("报告", 4096)
	}
	for name, values := range map[string][]RuntimeSemanticProofProposal{
		"nil": nil, "empty": {}, "blank": {{CoverageWorkItemID: " \t "}},
		"single": semanticProposalSortFixture(1, true), "same-cwi": semanticProposalSortFixture(32, true),
		"distinct-cwi": semanticProposalSortFixture(32, false), "mixed": mixed, "equal-hash": equal, "long-raw": long,
	} {
		t.Run(name, func(t *testing.T) { assertSemanticProposalSortReference(t, values) })
	}
}

func TestSemanticProposalSortKeepsIsolationAndRecomputesLaterCalls(t *testing.T) {
	values := semanticProposalSortFixture(16, true)
	before, _ := json.Marshal(values)
	got := normalizeRuntimeSemanticProofProposals(values)
	got[0].Target.ExactTargetRefs[0] = "changed"
	got[0].RequiredEvidenceKinds[0] = "changed"
	got[0].RequiredEffectScopes[0] = RuntimeEffectNone
	got[0].RequiredCheckRefs[0] = "changed"
	got[0].ValidationInputReview.Checks[0].InputTargetRefs[0] = "changed"
	got[0].ValidationInputReview.Checks[0].Obligations[0].InputTargetRefs[0] = "changed"
	after, _ := json.Marshal(values)
	if string(before) != string(after) {
		t.Fatal("排序返回值泄漏原 proposal 嵌套集合")
	}
	assertSemanticProposalSortReference(t, values)
	oldHash := StableRuntimeSemanticProofProposalHash(values[0])
	values[0].DeclarationRef = "新的原声明"
	values[0].ValidationInputReview.Checks[0].Reason = "新的原报告"
	if StableRuntimeSemanticProofProposalHash(values[0]) == oldHash {
		t.Fatal("后续输入变化未重算 proposal 身份")
	}
	assertSemanticProposalSortReference(t, values)
}

func TestSemanticProposalSortReducesRepeatedEncodingAllocations(t *testing.T) {
	for _, sameCWI := range []bool{true, false} {
		values := semanticProposalSortFixture(32, sameCWI)
		original := testing.AllocsPerRun(20, func() { _ = semanticProposalSortFullReference(values) })
		current := testing.AllocsPerRun(20, func() { _ = normalizeRuntimeSemanticProofProposals(values) })
		if current >= original {
			t.Fatalf("本次排序分配未减少：sameCWI=%v current=%v original=%v", sameCWI, current, original)
		}
	}
}

func TestSemanticProposalSortSealedLedgerRetainsIdentity(t *testing.T) {
	ledger := semanticSealedLedgerFixture(t)
	if ledger.Document == nil {
		t.Fatal("缺少有效 sealed ledger 前像")
	}
	assertSemanticProposalSortReference(t, ledger.Document.Proposals)
	body, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeSemanticPlanningLedger
	if err := json.Unmarshal(body, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("完整 sealed ledger 重载拒绝：%v", err)
	}
	if StableRuntimeSemanticPlanDocumentHash(*reloaded.Document) != semanticDocumentHashFullEncodingReference(*reloaded.Document) {
		t.Fatal("排序导致原 document identity 漂移")
	}
}

func FuzzSemanticProposalSortReference(f *testing.F) {
	f.Add(" cwi ", "原声明\r\n", " input ", "原报告", uint8(3))
	f.Add("", string([]byte{0xff, 0x80}), "\x00", "", uint8(0))
	f.Fuzz(func(t *testing.T, cwi, declaration, target, reason string, mode uint8) {
		values := semanticProposalSortFixture(8, true)
		for index := range values {
			values[index].CoverageWorkItemID = common.CoverageWorkItemID(cwi)
			if index%3 == 0 {
				values[index].CoverageWorkItemID += "-other"
			}
			values[index].DeclarationRef = declaration + strings.Repeat("x", index%3)
			values[index].Target.ExactTargetRefs = []string{target, "", target}
			values[index].ValidationInputReview.Checks[0].Reason = reason
			if mode%2 == 0 && index%2 == 0 {
				values[index].ValidationInputReview = nil
			}
		}
		assertSemanticProposalSortReference(t, values)
	})
}

func BenchmarkSemanticProposalStableSort(b *testing.B) {
	for _, sameCWI := range []bool{true, false} {
		values := semanticProposalSortFixture(32, sameCWI)
		for _, mode := range []string{"reference", "current"} {
			b.Run(fmt.Sprintf("same-cwi=%v/%s", sameCWI, mode), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if mode == "reference" {
						_ = semanticProposalSortFullReference(values)
					} else {
						_ = normalizeRuntimeSemanticProofProposals(values)
					}
				}
			})
		}
	}
}
