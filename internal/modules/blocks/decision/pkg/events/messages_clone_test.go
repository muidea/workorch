package events

import (
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
)

func TestDecisionCommandResultCloneDetachesOwnerState(t *testing.T) {
	item := &decisionmodel.Ticket{AllowedRecoveryActions: []string{"continue"}}
	cloned := (DecisionCommandResult{Ticket: item}).Clone()
	item.AllowedRecoveryActions[0] = "mutated"
	if cloned.Ticket.AllowedRecoveryActions[0] != "continue" {
		t.Fatal("cloned ticket shares recovery actions")
	}
}
