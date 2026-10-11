package governanceports

import (
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
)

func TestContractChangeCannotUseOrdinaryDecisionAdmission(t *testing.T) {
	for _, ticket := range []*decisionmodel.Ticket{
		{DecisionKind: decisionmodel.ContractChangeDecisionKind},
		{DecisionKind: "replan", ContractChangeRevisions: []decisionmodel.ContractChangeRevision{}},
	} {
		if !definitionDecisionUsesDedicatedAdmission(ticket) {
			t.Fatal("合同变更材料降级为普通决议")
		}
	}
}
