package orchestrator

import (
	"testing"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeV2ResultProjectionBuildsAcceptedDoDAssessment(t *testing.T) {
	aggregate := runtimev2.RuntimeAggregate{
		Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal-1", DoDID: "dod-1"},
		Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-1", RunID: "run-1", Assessments: []runtimecontract.RuntimeAssessmentFact{{
			FocusID: "focus-1", Scope: "dodItem", RequirementID: "dod-001", RequirementText: "行为已验证",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Satisfied: true, EvidenceRefs: []string{"event-1"},
		}}},
		Decision: runtimecontract.RuntimeDecision{Action: runtimecontract.RuntimeActionComplete, AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-1"}, Terminal: &runtimecontract.TerminalOutcome{Summary: "coverage satisfied"}},
	}
	item := runtimeV2ResultFromAggregate(aggregate)
	if item.ID != "ws-1/run-1/result/dod-assessment" || item.State != common.ResultStateAccepted || !item.DoDAssessment.Satisfied ||
		len(item.DoDAssessment.Items) != 1 || item.DoDAssessment.Items[0].Item != "行为已验证" || len(item.EvidenceRefs) != 1 {
		t.Fatalf("runtime v2 result = %+v", item)
	}
}
