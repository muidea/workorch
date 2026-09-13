package workorchruntimecontract

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func semanticExecutionBudgetFixture(t *testing.T) (RuntimeSemanticPlanRequestContext, RuntimeSemanticPlanManifest, RuntimeExecutionBudgetView) {
	t.Helper()
	c, m, facts := semanticProgressFixture()
	c.Activation.ExecutionContract.ExecutionLimits = &RuntimeExecutionLimits{ProtocolRef: RuntimeExecutionLimitsV1, MaxProviderCalls: 8, MaxCapabilityOperations: 4, MaxProofBatchesPerCWI: 3, MaxDurableDispatchesPerCWI: 2}
	var err error
	c.CoverageProgress, err = BuildRuntimeSemanticCoverageProgress(c, m, facts, 9)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := RuntimeExecutionBudgetPolicyWithLimits(DefaultRuntimeConvergencePolicy(), DefaultRuntimeRecoveryPolicy(), c.Activation.ExecutionContract.ExecutionLimits)
	if err != nil {
		t.Fatal(err)
	}
	view, err := BuildRuntimeExecutionBudgetView(c.WorkspaceID, c.RunID, 9, RuntimeExecutionBudgetLedger{Policy: policy}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for i := range view.Dimensions {
		if view.Dimensions[i].Dimension == RuntimeBudgetDimensionProviderCall {
			view.Dimensions[i].Confirmed, view.Dimensions[i].Reserved, view.Dimensions[i].Unknown, view.Dimensions[i].Remaining = 2, 1, 1, 4
		}
	}
	view.ViewHash = StableRuntimeExecutionBudgetViewHash(view)
	return c, m, view
}

func TestSemanticExecutionBudgetProjectsPhysicalAndOriginalCWIUsage(t *testing.T) {
	c, m, view := semanticExecutionBudgetFixture(t)
	b, err := BuildRuntimeSemanticExecutionBudget(c, m, view, map[common.CoverageWorkItemID]int{"done": 3, "current": 2}, map[common.CoverageWorkItemID]int{"done": 2, "current": 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.PhysicalDimensions) != 2 || b.PhysicalDimensions[0].Remaining != 4 || b.PhysicalDimensions[0].Unknown != 1 || len(b.VisibleProofBatches) != 1 ||
		b.VisibleProofBatches[0].Used != 2 || *b.VisibleProofBatches[0].Remaining != 1 || c.CoverageProgress.Satisfied != 1 {
		t.Fatalf("budget lost usage or changed acceptance: %+v", b)
	}
	if b.DurableDispatchLimit == nil || *b.DurableDispatchLimit != 2 || len(b.VisibleDurableDispatches) != 1 || b.VisibleDurableDispatches[0].Used != 1 || *b.VisibleDurableDispatches[0].Remaining != 1 {
		t.Fatalf("budget lost original durable seals: %+v", b)
	}
	encoded, _ := json.Marshal(b)
	var restored RuntimeSemanticExecutionBudget
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b, restored) || restored.Validate(c, m) != nil {
		t.Fatal("reload changed observation")
	}
	cloned := b.Clone()
	*cloned.ProofBatchLimit = 7
	*cloned.VisibleProofBatches[0].Remaining = 7
	cloned.PhysicalDimensions[0].Unknown = 7
	*cloned.DurableDispatchLimit = 7
	*cloned.VisibleDurableDispatches[0].Remaining = 7
	if *b.DurableDispatchLimit != 2 || *b.VisibleDurableDispatches[0].Remaining != 1 {
		t.Fatal("durable observation clone aliases source")
	}
	if *b.ProofBatchLimit != 3 || *b.VisibleProofBatches[0].Remaining != 1 || b.PhysicalDimensions[0].Unknown != 1 {
		t.Fatal("clone aliases source")
	}
	c.PlanStep, c.BatchID, c.BatchCoverageWorkItemIDs = RuntimeSemanticPlanStepCoverageOrder, "", nil
	order, err := BuildRuntimeSemanticExecutionBudget(c, m, view, map[common.CoverageWorkItemID]int{"current": 3}, map[common.CoverageWorkItemID]int{"current": 2})
	if err != nil || len(order.VisibleProofBatches) != 2 || *order.VisibleProofBatches[0].Remaining != 0 {
		t.Fatalf("order omitted exhausted CWI: %v", err)
	}
	if len(order.VisibleDurableDispatches) != 2 || *order.VisibleDurableDispatches[0].Remaining != 0 || order.VisibleDurableDispatches[1].Used != 0 {
		t.Fatal("durable budget exhaustion leaked across original CWIs")
	}
	c.Activation.ExecutionContract.ExecutionLimits = nil
	unbounded, err := BuildRuntimeSemanticExecutionBudget(c, m, view, nil, map[common.CoverageWorkItemID]int{"current": 2})
	if err != nil || unbounded.ProofBatchLimit != nil || unbounded.VisibleProofBatches[0].Remaining != nil || unbounded.PhysicalDimensions[0].Remaining != 4 {
		t.Fatalf("absent additional cap was treated as unlimited execution: %v", err)
	}
	if unbounded.DurableDispatchLimit != nil || unbounded.VisibleDurableDispatches[0].Remaining != nil || unbounded.VisibleDurableDispatches[0].Used != 2 {
		t.Fatal("absent durable cap lost usage or invented a remaining allowance")
	}
	// A valid independently sealed policy need not equal today's default.
	// Reading it neither raises nor narrows its authority.
	for i := range view.Dimensions {
		if view.Dimensions[i].Dimension == RuntimeBudgetDimensionProviderCall {
			view.Dimensions[i].Limit, view.Dimensions[i].Remaining = 5000, 4996
		}
	}
	view.ViewHash = StableRuntimeExecutionBudgetViewHash(view)
	custom, err := BuildRuntimeSemanticExecutionBudget(c, m, view, nil, nil)
	if err != nil || custom.PhysicalDimensions[0].Limit != 5000 {
		t.Fatalf("read model imposed unsealed default limit: %v", err)
	}
}

func TestSemanticExecutionBudgetRejectsTamperedAndIncompleteObservations(t *testing.T) {
	c, m, view := semanticExecutionBudgetFixture(t)
	b, err := BuildRuntimeSemanticExecutionBudget(c, m, view, map[common.CoverageWorkItemID]int{"current": 2}, map[common.CoverageWorkItemID]int{"current": 1})
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeSemanticExecutionBudget){
		"capacity": func(v *RuntimeSemanticExecutionBudget) {
			v.PolicyRevision = strings.Repeat("x", RuntimeSemanticExecutionBudgetMaxBytes)
		},
		"scope":    func(v *RuntimeSemanticExecutionBudget) { v.RunID = "foreign" },
		"revision": func(v *RuntimeSemanticExecutionBudget) { v.SourceStateRevision++ },
		"limit": func(v *RuntimeSemanticExecutionBudget) {
			v.PhysicalDimensions[0].Limit++
			v.PhysicalDimensions[0].Remaining++
		},
		"missing dimension":   func(v *RuntimeSemanticExecutionBudget) { v.PhysicalDimensions = v.PhysicalDimensions[:1] },
		"duplicate dimension": func(v *RuntimeSemanticExecutionBudget) { v.PhysicalDimensions[1] = v.PhysicalDimensions[0] },
		"unknown refunded":    func(v *RuntimeSemanticExecutionBudget) { v.PhysicalDimensions[0].Remaining++ },
		"overflow":            func(v *RuntimeSemanticExecutionBudget) { v.PhysicalDimensions[0].Confirmed = math.MaxInt64 },
		"negative":            func(v *RuntimeSemanticExecutionBudget) { v.PhysicalDimensions[0].Reserved = -1 },
		"exhausted":           func(v *RuntimeSemanticExecutionBudget) { v.PhysicalDimensions[0].Exhausted = true },
		"cap missing":         func(v *RuntimeSemanticExecutionBudget) { v.ProofBatchLimit = nil },
		"cap raised":          func(v *RuntimeSemanticExecutionBudget) { *v.ProofBatchLimit++ },
		"missing CWI":         func(v *RuntimeSemanticExecutionBudget) { v.VisibleProofBatches = nil },
		"foreign CWI":         func(v *RuntimeSemanticExecutionBudget) { v.VisibleProofBatches[0].CoverageWorkItemID = "done" },
		"proof refund":        func(v *RuntimeSemanticExecutionBudget) { *v.VisibleProofBatches[0].Remaining++ },
		"proof overspend":     func(v *RuntimeSemanticExecutionBudget) { v.VisibleProofBatches[0].Used = 4 },
		"durable cap missing": func(v *RuntimeSemanticExecutionBudget) { v.DurableDispatchLimit = nil },
		"durable cap raised":  func(v *RuntimeSemanticExecutionBudget) { *v.DurableDispatchLimit++ },
		"durable CWI missing": func(v *RuntimeSemanticExecutionBudget) { v.VisibleDurableDispatches = nil },
		"durable CWI foreign": func(v *RuntimeSemanticExecutionBudget) { v.VisibleDurableDispatches[0].CoverageWorkItemID = "done" },
		"durable refund":      func(v *RuntimeSemanticExecutionBudget) { *v.VisibleDurableDispatches[0].Remaining++ },
		"durable overspend":   func(v *RuntimeSemanticExecutionBudget) { v.VisibleDurableDispatches[0].Used = 3 },
		"durable negative":    func(v *RuntimeSemanticExecutionBudget) { v.VisibleDurableDispatches[0].Used = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			changed := b.Clone()
			mutate(&changed)
			changed.Hash = StableRuntimeSemanticExecutionBudgetHash(changed)
			if changed.Validate(c, m) == nil {
				t.Fatal("tampered budget accepted")
			}
		})
	}
	for _, usage := range []map[common.CoverageWorkItemID]int{{"done": 4}, {"foreign": 1}, {"remaining": -1}} {
		if _, err := BuildRuntimeSemanticExecutionBudget(c, m, view, usage, nil); err == nil {
			t.Fatal("hidden invalid usage silently omitted")
		}
	}
	for _, usage := range []map[common.CoverageWorkItemID]int{{"done": 3}, {"foreign": 1}, {"remaining": -1}} {
		if _, err := BuildRuntimeSemanticExecutionBudget(c, m, view, nil, usage); err == nil {
			t.Fatal("hidden invalid durable usage silently omitted")
		}
	}
	view.Dimensions[0].Remaining--
	if _, err := BuildRuntimeSemanticExecutionBudget(c, m, view, nil, nil); err == nil {
		t.Fatal("invalid source hash accepted")
	}
}
