package workorchprojectioncontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestGovernanceCloneHelpersDetachMutableValues(t *testing.T) {
	decision := Decision{
		ID: "decision-1", AllowedRecoveryActions: []string{"retry"},
		Context:                 workorcheventpayload.New(workorcheventpayload.StringField("context", "original")),
		ResolutionPayload:       workorcheventpayload.New(workorcheventpayload.StringField("resolution", "original")),
		AutoGovernance:          &AutoGovernanceSummary{EvidenceRefs: []string{"evidence-1"}},
		DefinitionClarification: &DefinitionClarification{OffendingRefs: []string{"dod:item-1"}},
	}
	clonedDecision := CloneDecision(decision)
	clonedDecision.AllowedRecoveryActions[0] = "mutated"
	clonedDecision.Context = workorcheventpayload.New(workorcheventpayload.StringField("context", "mutated"))
	clonedDecision.AutoGovernance.EvidenceRefs[0] = "mutated"
	clonedDecision.DefinitionClarification.OffendingRefs[0] = "mutated"
	if decision.AllowedRecoveryActions[0] != "retry" || decision.AutoGovernance.EvidenceRefs[0] != "evidence-1" || decision.DefinitionClarification.OffendingRefs[0] != "dod:item-1" {
		t.Fatal("decision clone retained mutable storage")
	}

	approval := Approval{Context: workorcheventpayload.New(workorcheventpayload.StringField("context", "original")), AutoGovernance: &AutoGovernanceSummary{EvidenceRefs: []string{"evidence-1"}}}
	clonedApproval := CloneApproval(approval)
	clonedApproval.AutoGovernance.EvidenceRefs[0] = "mutated"
	if approval.AutoGovernance.EvidenceRefs[0] != "evidence-1" {
		t.Fatal("approval clone retained mutable storage")
	}
	if CloneDecisions(nil) != nil || CloneApprovals(nil) != nil {
		t.Fatal("nil governance collections must remain nil")
	}
}
