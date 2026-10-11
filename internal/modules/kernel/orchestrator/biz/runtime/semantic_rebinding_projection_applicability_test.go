package runtime

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 保留原完整路径作为独立 oracle，不把新的适用性判断用于 reference。
func runtimeSemanticRebindingProjectionReference(facts runtimecontract.RuntimeFacts) error {
	focuses := append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...)
	byCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(focuses))
	for _, focus := range focuses {
		focus = focus.Normalized()
		for _, coverageID := range focus.CoverageWorkItemIDs {
			byCoverage[coverageID] = focus
		}
	}
	for _, item := range facts.Coverage {
		if item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened || len(item.ReopenLineage) == 0 {
			continue
		}
		reason := item.ReopenLineage[len(item.ReopenLineage)-1].Normalized().Reason
		if !runtimeCWIReopenRequiresSemanticRebinding(reason) {
			continue
		}
		focus, found := byCoverage[item.ID]
		if !found {
			return fmt.Errorf("runtime CWI %q semantic rebinding has no canonical pending focus", item.ID)
		}
		expected, err := runtimeCWISemanticRebindingPendingFocus(facts, item.ID)
		if err != nil {
			return err
		}
		if !runtimecontract.SameRuntimeCoverageFocus(focus, expected) {
			return fmt.Errorf("runtime CWI %q semantic rebinding projection differs from immutable Definition seed", item.ID)
		}
	}
	return nil
}

func TestSemanticRebindingProjectionApplicabilityPreservesOriginalGate(t *testing.T) {
	base := stateChangeAggregate(t).CloneRaw()
	for _, scenario := range []string{"ordinary", "unrelated_reopen", "missing_lineage", "required_rebind", "raw_reason", "last_reason", "missing_focus", "missing_definition"} {
		t.Run(scenario, func(t *testing.T) {
			facts := base.CloneRaw().Facts
			item := &facts.Coverage[0]
			item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateReopened
			item.ReopenLineage = []runtimecontract.RuntimeCWIReopenLineage{{Reason: runtimeCWIReopenReasonValidationFailureRepairRequired}}
			switch scenario {
			case "ordinary":
				item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateBound
			case "unrelated_reopen":
				item.ReopenLineage[0].Reason = "other"
			case "missing_lineage":
				item.ReopenLineage = nil
			case "raw_reason":
				item.ReopenLineage[0].Reason = " " + item.ReopenLineage[0].Reason + " "
			case "last_reason":
				item.ReopenLineage = append(item.ReopenLineage, runtimecontract.RuntimeCWIReopenLineage{Reason: "other"})
			case "missing_focus":
				facts.Focus.CoverageWorkItemIDs = nil
				facts.RemainingFocuses, facts.CompletedFocuses = nil, nil
			case "missing_definition":
				facts.DefinitionFocuses = nil
			}
			before, err := json.Marshal(facts)
			if err != nil {
				t.Fatal(err)
			}
			want, got := runtimeSemanticRebindingProjectionReference(facts), validateRuntimeCWISemanticRebindingFocusProjections(facts)
			if (want == nil) != (got == nil) || want != nil && got.Error() != want.Error() {
				t.Fatalf("适用性判断改变原门禁：got=%v want=%v", got, want)
			}
			after, err := json.Marshal(facts)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("投影校验改写原始事实")
			}
			if strings.Contains(scenario, "required") || scenario == "raw_reason" || strings.HasPrefix(scenario, "missing_focus") || scenario == "missing_definition" {
				if got == nil {
					t.Fatal("需要重绑的旧 authority 未拒绝")
				}
			}
		})
	}
	currentAllocs := testing.AllocsPerRun(10, func() { _ = validateRuntimeCWISemanticRebindingFocusProjections(base.Facts) })
	referenceAllocs := testing.AllocsPerRun(10, func() { _ = runtimeSemanticRebindingProjectionReference(base.Facts) })
	if currentAllocs >= referenceAllocs {
		t.Fatalf("无重绑时仍复制无关 focus/binding：current=%g reference=%g", currentAllocs, referenceAllocs)
	}
}
