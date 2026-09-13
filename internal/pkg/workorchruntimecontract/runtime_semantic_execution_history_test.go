package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSemanticExecutionHistoryScopeCapacityAndDetachedClone(t *testing.T) {
	c, m, facts := semanticProgressFixture()
	var err error
	c.CoverageProgress, err = BuildRuntimeSemanticCoverageProgress(c, m, facts, 9)
	if err != nil {
		t.Fatal(err)
	}
	var receipts []RuntimeSemanticReceiptObservation
	for i := 2; i <= 17; i++ {
		receipts = append(receipts, RuntimeSemanticReceiptObservation{
			ReceiptSequence: i, DispatchID: fmt.Sprint("dispatch-", i), OperationID: fmt.Sprint("op-", i), OperationKind: StepKindFileWrite, OwnerModuleID: RuntimeModuleFile,
			ReceiptRef: fmt.Sprint("runtime-receipt/execution-", i), ReceiptHash: fmt.Sprint("hash-", i), Fence: 1,
			BoundCoverageWorkItemIDs: []common.CoverageWorkItemID{"done"}, ObservedState: RuntimeOperationOutcomeAccepted, ObservedEffectScope: RuntimeEffectWorkspaceDurable,
			Changed: true, TargetRefs: []string{"source.txt"}, AfterHash: "after", ExpectedEffectScope: RuntimeEffectWorkspaceDurable, RequestedTargetRefs: []string{"source.txt"},
			Validation: ProjectRuntimeValidationExecutionFeedback(&RuntimeValidationObservation{RecipeID: "recipe", PassedChecks: []string{"check"}, OutputPreview: "RAW_NOT_FOR_PROVIDER"}),
		})
	}
	h, err := BuildRuntimeSemanticExecutionHistory(c, m, 17, receipts)
	if err != nil {
		t.Fatal(err)
	}
	if h.OmittedReceipts != 1 || h.Receipts[0].BoundCoverageWorkItemIDs[0] != "done" {
		t.Fatal("history lost completed CWI outside current manifest")
	}
	encoded, err := json.Marshal(h)
	if err != nil || strings.Contains(string(encoded), "RAW_NOT_FOR_PROVIDER") {
		t.Fatal("history leaked raw validation")
	}
	var restored RuntimeSemanticExecutionHistory
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.Validate(c, m) != nil || !reflect.DeepEqual(restored, h) {
		t.Fatal("same-version reload changed history")
	}
	cloned := h.Clone()
	cloned.Receipts[0].TargetRefs[0] = "other"
	cloned.Receipts[0].RequestedTargetRefs[0] = "other"
	cloned.Receipts[0].BoundCoverageWorkItemIDs[0] = "other"
	cloned.Receipts[0].Validation.PassedChecks[0] = "other"
	if h.Receipts[0].RequestedTargetRefs[0] != "source.txt" || h.Receipts[0].TargetRefs[0] != "source.txt" || h.Receipts[0].BoundCoverageWorkItemIDs[0] != "done" || h.Receipts[0].Validation.PassedChecks[0] != "check" {
		t.Fatal("clone aliases owner observations")
	}
	for name, mutate := range map[string]func(*RuntimeSemanticExecutionHistory){
		"scope":     func(v *RuntimeSemanticExecutionHistory) { v.RunID = "foreign" },
		"revision":  func(v *RuntimeSemanticExecutionHistory) { v.SourceStateRevision++ },
		"count":     func(v *RuntimeSemanticExecutionHistory) { v.OmittedReceipts = 0 },
		"drop":      func(v *RuntimeSemanticExecutionHistory) { v.Receipts = v.Receipts[1:]; v.OmittedReceipts++ },
		"reorder":   func(v *RuntimeSemanticExecutionHistory) { v.Receipts[0], v.Receipts[1] = v.Receipts[1], v.Receipts[0] },
		"duplicate": func(v *RuntimeSemanticExecutionHistory) { v.Receipts[1].ReceiptRef = v.Receipts[0].ReceiptRef },
		"owner":     func(v *RuntimeSemanticExecutionHistory) { v.Receipts[0].OwnerModuleID = RuntimeModuleShell },
		"CWI": func(v *RuntimeSemanticExecutionHistory) {
			v.Receipts[0].BoundCoverageWorkItemIDs = []common.CoverageWorkItemID{"foreign"}
		},
		"outcome":                 func(v *RuntimeSemanticExecutionHistory) { v.Receipts[0].ObservedState = "complete" },
		"missing expected effect": func(v *RuntimeSemanticExecutionHistory) { v.Receipts[0].ExpectedEffectScope = "" },
		"capacity": func(v *RuntimeSemanticExecutionHistory) {
			v.Receipts[0].AfterHash = strings.Repeat("x", RuntimeSemanticExecutionHistoryMaxBytes)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := h.Clone()
			mutate(&changed)
			changed.Hash = StableRuntimeSemanticExecutionHistoryHash(changed)
			if changed.Validate(c, m) == nil {
				t.Fatal("invalid self-consistent transport accepted")
			}
		})
	}
	if _, err := BuildRuntimeSemanticExecutionHistory(c, m, 0, nil); err != nil {
		t.Fatal(err)
	}
	if (RuntimeSemanticExecutionHistory{}).Validate(c, m) == nil {
		t.Fatal("missing observation defaulted to no execution")
	}
}
