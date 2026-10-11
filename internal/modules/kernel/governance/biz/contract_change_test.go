package governance

import (
	"context"
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
)

func TestContractChangeCannotUseAutoGovernancePolicy(t *testing.T) {
	for _, ticket := range []*decisionmodel.Ticket{
		{ID: "change", DecisionKind: decisionmodel.ContractChangeDecisionKind},
		{ID: "corrupt-kind", DecisionKind: "replan", ContractChangeRevisions: []decisionmodel.ContractChangeRevision{}},
	} {
		result := (&Service{}).evaluateTicket(context.Background(), ticket, nil, nil, nil, nil)
		if result.canAutoResolve || result.policyBlocked != "contract_change_requires_exact_approval" || result.budgetAfter != 0 {
			t.Fatalf("提案材料进入自动治理：%+v", result)
		}
	}
}
