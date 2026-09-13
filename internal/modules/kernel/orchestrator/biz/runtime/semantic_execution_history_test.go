package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSemanticExecutionHistoryUsesOwnerReceiptWithoutAwardingCredit(t *testing.T) {
	for _, outcome := range []runtimecontract.RuntimeOperationOutcome{runtimecontract.RuntimeOperationOutcomeAccepted, runtimecontract.RuntimeOperationOutcomeRejected} {
		t.Run(string(outcome), func(t *testing.T) { testSemanticExecutionHistoryReceipt(t, outcome) })
	}
}

func testSemanticExecutionHistoryReceipt(t *testing.T, outcome runtimecontract.RuntimeOperationOutcome) {
	t.Helper()
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	a, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	a, err = beginAndAcceptCandidate(t, controller, a, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("RAW_PARAMETER_NOT_FOR_PROVIDER")},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	opID := runtimeTestActiveOperationID(t, a, 0)
	a, err = controller.claimOperationExecution(ctx, a.Facts.WorkspaceID, a.Facts.RunID, opID)
	if err != nil {
		t.Fatal(err)
	}
	execution, _ := operationExecutionRecord(a.ActiveDispatch.OperationExecutions, opID)
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: opID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true, TargetRefs: []string{"target.txt"}, BeforeHash: "before", AfterHash: "after",
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "RAW_SUMMARY_NOT_FOR_PROVIDER"},
	}
	if outcome == runtimecontract.RuntimeOperationOutcomeRejected {
		receipt.Outcome, receipt.ObservedEffectScope, receipt.Changed = outcome, runtimecontract.RuntimeEffectNone, false
		receipt.BeforeHash, receipt.AfterHash = "", ""
		receipt.Feedback = runtimeOperationFeedbackForTest(t, a, opID, runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionRejected, Code: "write_rejected", Signature: "write-rejected",
			Message: "RAW_DIAGNOSTIC_NOT_FOR_PROVIDER",
		})
	}
	a, _, err = controller.commitOperationReceipt(ctx, a.Facts.WorkspaceID, a.Facts.RunID, a.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleFile, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if a.SemanticPlanning.Manifest == nil {
		t.Fatal("fixture missing sealed semantic manifest")
	}
	m := a.SemanticPlanning.Manifest.Clone()
	c := runtimecontract.RuntimeSemanticPlanRequestContext{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, Definition: a.Definition, DefinitionSeed: a.Facts.DefinitionSeed, PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder}
	c.CoverageProgress, err = runtimecontract.BuildRuntimeSemanticCoverageProgress(c, m, a.Facts, a.StateRevision)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := SemanticExecutionHistory(a, c, m)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.TotalReceipts != 1 || len(baseline.Receipts) != 1 || baseline.Receipts[0].ObservedState != outcome || c.CoverageProgress.Satisfied != 0 {
		t.Fatal("receipt was lost or awarded credit before evidence")
	}
	if baseline.Receipts[0].ExpectedEffectScope != execution.ExpectedEffectScope || !reflect.DeepEqual(baseline.Receipts[0].RequestedTargetRefs, []string{"target.txt"}) {
		t.Fatal("history lost sealed expectation or invented a missing diagnostic")
	}
	if outcome == runtimecontract.RuntimeOperationOutcomeRejected {
		if baseline.Receipts[0].Diagnostic == nil || baseline.Receipts[0].Diagnostic.OriginalCode != "write_rejected" || baseline.Receipts[0].ObservedEffectScope != runtimecontract.RuntimeEffectNone || baseline.Receipts[0].ExpectedEffectScope == baseline.Receipts[0].ObservedEffectScope {
			t.Fatal("rejected work lost its diagnostic or expected effect replaced the actual observation")
		}
	} else if baseline.Receipts[0].Diagnostic != nil {
		t.Fatal("accepted work acquired an invented diagnostic")
	}
	wire, _ := json.Marshal(baseline)
	if strings.Contains(string(wire), "RAW_") {
		t.Fatal("raw receipt or operation parameters escaped")
	}
	after, _ := json.Marshal(a)
	if string(after) != string(encoded) {
		t.Fatal("projection changed aggregate")
	}
	var restored RuntimeAggregate
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	replayed, err := SemanticExecutionHistory(restored, c, m)
	if err != nil || !reflect.DeepEqual(replayed, baseline) {
		t.Fatalf("reload changed history: %v", err)
	}
	wrongSeed := c
	wrongSeed.DefinitionSeed.Hash = "other-seed"
	if _, err := SemanticExecutionHistory(a, wrongSeed, m); err == nil {
		t.Fatal("history accepted a foreign Definition seed")
	}
	staleManifest := m.Clone()
	staleManifest.Hash = "retired-manifest"
	if _, err := SemanticExecutionHistory(a, c, staleManifest); err == nil {
		t.Fatal("history accepted a stale manifest")
	}
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"receipt missing":   func(v *RuntimeAggregate) { v.OperationReceipts = nil },
		"receipt duplicate": func(v *RuntimeAggregate) { v.OperationReceipts = append(v.OperationReceipts, v.OperationReceipts[0]) },
		"receipt hash":      func(v *RuntimeAggregate) { v.OperationReceipts[0].AfterHash = "other" },
		"commit missing": func(v *RuntimeAggregate) {
			v.ActiveDispatch.OperationExecutions[0].State = runtimecontract.OperationExecutionClaimed
		},
		"scope": func(v *RuntimeAggregate) { v.Facts.RunID = "foreign" },
	} {
		t.Run(name, func(t *testing.T) {
			var changed RuntimeAggregate
			if err := json.Unmarshal(encoded, &changed); err != nil {
				t.Fatal(err)
			}
			mutate(&changed)
			if _, err := SemanticExecutionHistory(changed, c, m); err == nil {
				t.Fatal("invalid source admitted")
			}
		})
	}
}
