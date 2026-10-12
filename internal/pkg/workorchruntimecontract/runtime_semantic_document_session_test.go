package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// 禁用同次成功集合，固定原完整 document validator 路径作为复用边界的 oracle。
func referenceSemanticDocumentWithoutSession(document RuntimeSemanticPlanDocument, validated runtimeValidatedSemanticManifest) error {
	validated.documents = nil
	return validateRuntimeSemanticPlanDocumentAgainstValidatedManifestUncached(document, validated)
}

func TestSemanticDocumentSessionRequiresExactRawInputAndManifest(t *testing.T) {
	ledger := semanticBindingIndexedMultiBatchFixture(t, 2)
	validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	original := ledger.BatchDocuments[0]
	if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(original, validated); err != nil {
		t.Fatal(err)
	}
	if len(validated.documents.values) != 1 {
		t.Fatal("完整成功校验未记录同次原值")
	}
	for _, scenario := range []string{"exact", "reload", "declaration", "duplicate_member", "empty_member", "unknown_coverage", "manifest_hash", "manifest_body", "raw_whitespace", "failed_then_corrected"} {
		t.Run(scenario, func(t *testing.T) {
			document := cloneRuntimeSemanticPlanDocumentRaw(original)
			current := validated
			switch scenario {
			case "reload":
				data, err := json.Marshal(document)
				if err != nil || json.Unmarshal(data, &document) != nil {
					t.Fatal("JSON 重载失败")
				}
			case "declaration", "failed_then_corrected":
				document.Proposals[0].DeclarationRef = "unsealed"
			case "duplicate_member":
				document.BatchCoverageWorkItemIDs = append(document.BatchCoverageWorkItemIDs, document.BatchCoverageWorkItemIDs[0])
			case "empty_member":
				document.BatchCoverageWorkItemIDs = append(document.BatchCoverageWorkItemIDs, "")
			case "unknown_coverage":
				document.Proposals[0].CoverageWorkItemID = "unknown"
			case "manifest_hash":
				current.manifest.Hash = "changed"
			case "manifest_body":
				current.manifest = cloneRuntimeSemanticPlanManifestRaw(current.manifest)
				current.manifest.AllowedOperationKinds = nil
			case "raw_whitespace":
				document.Proposals[0].DeclarationRef = " " + document.Proposals[0].DeclarationRef + " "
			}
			before, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			got := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, current)
			want := referenceSemanticDocumentWithoutSession(document, current)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("完整 raw 门禁首个错误改变: got=%v want=%v", got, want)
			}
			after, err := json.Marshal(document)
			if err != nil || string(before) != string(after) {
				t.Fatal("同次成功集合改写原 document")
			}
			if scenario == "failed_then_corrected" {
				document = cloneRuntimeSemanticPlanDocumentRaw(original)
				if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, current); err != nil {
					t.Fatalf("失败污染后续完整校验: %v", err)
				}
			}
		})
	}
	fresh, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
	if err != nil || fresh.documents == validated.documents || len(fresh.documents.values) != 0 {
		t.Fatal("成功结果跨顶层调用共享")
	}
	changed := cloneRuntimeSemanticPlanDocumentRaw(original)
	changed.Proposals[0].DeclarationRef = "after-success"
	if validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(changed, validated) == nil {
		t.Fatal("成功后的原值变化借用旧校验")
	}
	if !reflect.DeepEqual(validated.documents.values[0], original) {
		t.Fatal("成功集合未保存 detached 原值")
	}
}

func TestSemanticDocumentSessionDetachesCallerValues(t *testing.T) {
	ledger := semanticBindingIndexedMultiBatchFixture(t, 2)
	validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	document := cloneRuntimeSemanticPlanDocumentRaw(ledger.BatchDocuments[0])
	original := cloneRuntimeSemanticPlanDocumentRaw(document)
	if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, validated); err != nil {
		t.Fatal(err)
	}
	document.Proposals[0].DeclarationRef = "caller-mutation"
	document.BatchCoverageWorkItemIDs[0] = "caller-mutation"
	if !reflect.DeepEqual(validated.documents.values[0], original) || validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, validated) == nil {
		t.Fatal("调用者修改污染成功前像")
	}
}

func TestSemanticDocumentSessionKeepsCompilerOwnerFactGates(t *testing.T) {
	ledger, _ := assessmentPurposeGenerationForTest(t, "cwi-source", "document-session-request", false)
	validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	document := *ledger.Document
	if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, validated); err != nil {
		t.Fatal(err)
	}
	uncached := validated
	uncached.documents = nil
	for _, scenario := range []string{"valid", "missing_owner", "duplicate_owner", "wrong_owner", "unsupported_owner", "document_after_success"} {
		t.Run(scenario, func(t *testing.T) {
			current := cloneRuntimeSemanticPlanDocumentRaw(document)
			facts := cloneRuntimeSemanticPlanningLedgerRaw(ledger).Feasibilities
			switch scenario {
			case "missing_owner":
				facts = nil
			case "duplicate_owner":
				facts = append(facts, facts[0])
			case "wrong_owner":
				facts[0].OwnerModuleID = RuntimeModuleShell
			case "unsupported_owner":
				facts[0].Status = RuntimeSemanticFeasibilityStatus("unsupported")
			case "document_after_success":
				current.Proposals[0].DeclarationRef = "unsealed"
			}
			beforeDocument, _ := json.Marshal(current)
			beforeFacts, _ := json.Marshal(facts)
			want, wantContract, wantErr := compileRuntimeSemanticBindingsWithValidatedManifest(uncached, ledger.Batches[0].RequestLogicalID, current, facts)
			for range 2 {
				got, gotContract, gotErr := compileRuntimeSemanticBindingsWithValidatedManifest(validated, ledger.Batches[0].RequestLogicalID, current, facts)
				if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotContract, wantContract) || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
					t.Fatalf("document 同次复用改变 compiler 产物或完整拒绝: got=%v want=%v", gotErr, wantErr)
				}
				if scenario != "valid" && gotErr == nil {
					t.Fatal("document 成功复用绕过 owner facts 或后续 document 门禁")
				}
			}
			afterDocument, _ := json.Marshal(current)
			afterFacts, _ := json.Marshal(facts)
			if string(beforeDocument) != string(afterDocument) || string(beforeFacts) != string(afterFacts) {
				t.Fatal("compiler 同次复用修改原 document 或 owner facts")
			}
		})
	}
}

func TestSemanticDocumentSessionReducesRepeatedValidationAllocations(t *testing.T) {
	ledger := semanticBindingIndexedMultiBatchFixture(t, 4)
	validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	document := ledger.BatchDocuments[0]
	if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, validated); err != nil {
		t.Fatal(err)
	}
	current := testing.AllocsPerRun(20, func() {
		if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, validated); err != nil {
			panic(err)
		}
	})
	original := testing.AllocsPerRun(20, func() {
		if err := referenceSemanticDocumentWithoutSession(document, validated); err != nil {
			panic(err)
		}
	})
	if current >= original {
		t.Fatalf("相同原值复用没有减少分配: current=%v original=%v", current, original)
	}
	t.Logf("allocations current=%v original=%v", current, original)
}

func BenchmarkSemanticDocumentSession(b *testing.B) {
	ledger := semanticBindingIndexedMultiBatchFixture(b, 4)
	for _, reference := range []bool{false, true} {
		name := "same_call"
		if reference {
			name = "reference"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
				if err != nil {
					b.Fatal(err)
				}
				for range 4 {
					if reference {
						err = referenceSemanticDocumentWithoutSession(ledger.BatchDocuments[0], validated)
					} else {
						err = validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(ledger.BatchDocuments[0], validated)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func FuzzSemanticDocumentSessionMatchesFullValidation(f *testing.F) {
	ledger := semanticBindingIndexedMultiBatchFixture(f, 2)
	for _, seed := range []string{"", "changed", " ", "中文", "\xff"} {
		f.Add(seed, uint8(0))
	}
	f.Fuzz(func(t *testing.T, value string, mode uint8) {
		if len(value) > 512 {
			return
		}
		validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
		if err != nil {
			t.Fatal(err)
		}
		original := ledger.BatchDocuments[0]
		if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(original, validated); err != nil {
			t.Fatal(err)
		}
		document := cloneRuntimeSemanticPlanDocumentRaw(original)
		switch mode % 5 {
		case 0:
			document.Proposals[0].DeclarationRef = value
		case 1:
			document.Proposals[0].Target.ExactTargetRefs = []string{value}
		case 2:
			document.BatchCoverageWorkItemIDs = append(document.BatchCoverageWorkItemIDs, common.CoverageWorkItemID(value))
		case 3:
			document.ManifestHash = value
		case 4:
			document.Proposals[0].RequiredEvidenceKinds = []string{value}
		}
		got := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, validated)
		want := referenceSemanticDocumentWithoutSession(document, validated)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("完整校验首个错误改变: got=%v want=%v", got, want)
		}
	})
}
