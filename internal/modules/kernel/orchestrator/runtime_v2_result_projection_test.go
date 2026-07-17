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
		Facts: runtimecontract.RuntimeFacts{
			Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			WorkspaceID:        "ws-1",
			RunID:              "run-1",
			DefinitionRevision: "definition-1",
			Assessments: []runtimecontract.RuntimeAssessmentFact{{
				FocusID: "focus-1", Scope: "dodItem", RequirementID: "dod-001", RequirementText: "行为已验证",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Satisfied: true,
				Disposition: runtimecontract.AssessmentDispositionSatisfied, EvidenceRefs: []string{"event-1"},
			}}},
		Decision: runtimecontract.RuntimeDecision{
			Action:            runtimecontract.RuntimeActionComplete,
			AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-1"},
			Terminal:          &runtimecontract.TerminalOutcome{Code: "coverage_satisfied", Summary: "coverage satisfied"},
		},
	}
	item := runtimeV2ResultFromAggregate(aggregate)
	if item.ID != "ws-1/run-1/result/dod-assessment" || item.State != common.ResultStateAccepted || !item.DoDAssessment.Satisfied ||
		len(item.DoDAssessment.Items) != 1 || item.DoDAssessment.Items[0].Item != "行为已验证" || len(item.EvidenceRefs) != 1 {
		t.Fatalf("runtime v2 result = %+v", item)
	}
	if item.RuntimeCloseout == nil || item.RuntimeCloseout.Disposition != runtimecontract.AssessmentDispositionSatisfied ||
		len(item.RuntimeCloseout.SatisfiedCoverageWorkItemIDs) != 1 || item.RuntimeCloseout.SatisfiedCoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatalf("runtime closeout = %+v", item.RuntimeCloseout)
	}
}

func TestRuntimeV2ResultProjectionBuildsStructuredRejectedCloseout(t *testing.T) {
	aggregate := runtimev2.RuntimeAggregate{
		Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal-1", DoDID: "dod-1", Revision: "definition-1"},
		Facts: runtimecontract.RuntimeFacts{
			Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			WorkspaceID:        "ws-1",
			RunID:              "run-1",
			DefinitionRevision: "definition-1",
			Focus: runtimecontract.CoverageFocus{
				ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
			},
			Assessments: []runtimecontract.RuntimeAssessmentFact{{
				ID: "assessment-1", FocusID: "focus-1", Scope: "dodItem", RequirementID: "dod-001", RequirementText: "行为一",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
				Disposition:         runtimecontract.AssessmentDispositionRepairableGap,
				Summary:             "implementation incomplete",
				Items: []runtimecontract.RuntimeAssessmentItemFact{
					{CoverageWorkItemID: "cwi-1", Satisfied: true, EvidenceRefs: []string{"ev-1"}},
					{CoverageWorkItemID: "cwi-2", Satisfied: false, Reason: "missing implementation"},
				},
				Gap: &runtimecontract.RuntimeAssessmentGap{
					CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-2"},
					MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
					TargetRefs:           []string{"main.go"},
					Reason:               "tools not implemented",
					FailureCode:          "implementation_gap",
					FailureSignature:     "sig-impl",
				},
				EvidenceRefs: []string{"ev-1"},
			}},
			Repair: runtimecontract.RuntimeRepairLedger{LastCycleID: "cycle-1"},
		},
		Decision: runtimecontract.RuntimeDecision{
			Action:            runtimecontract.RuntimeActionFail,
			AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-1"},
			Terminal:          &runtimecontract.TerminalOutcome{Code: "no_progress_exhausted", Summary: "repair budget exhausted"},
		},
	}
	item := runtimeV2ResultFromAggregate(aggregate)
	if item.State != common.ResultStateRejected || item.RuntimeCloseout == nil {
		t.Fatalf("rejected result = %+v", item)
	}
	closeout := item.RuntimeCloseout
	if closeout.DefinitionRevision != "definition-1" || closeout.FailureCode != "implementation_gap" {
		t.Fatalf("closeout identity = %+v", closeout)
	}
	if len(closeout.SatisfiedCoverageWorkItemIDs) != 1 || closeout.SatisfiedCoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatalf("satisfied = %v", closeout.SatisfiedCoverageWorkItemIDs)
	}
	if len(closeout.UnsatisfiedCoverageWorkItemIDs) != 1 || closeout.UnsatisfiedCoverageWorkItemIDs[0] != "cwi-2" {
		t.Fatalf("unsatisfied = %v", closeout.UnsatisfiedCoverageWorkItemIDs)
	}
	if len(closeout.Gaps) != 1 || len(closeout.MissingEvidenceKinds) != 1 || closeout.MissingEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("gaps/missing = %+v %v", closeout.Gaps, closeout.MissingEvidenceKinds)
	}
	if len(closeout.TargetRefs) != 1 || closeout.TargetRefs[0] != "main.go" {
		t.Fatalf("target refs = %v", closeout.TargetRefs)
	}
	if closeout.Disposition != runtimecontract.AssessmentDispositionRepairableGap {
		t.Fatalf("disposition = %q", closeout.Disposition)
	}
}
