package governancecore

import decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"

func NormalizeRuntimeRepairRecoveryActionByTicketPolicy(ticket *decisionmodel.Ticket, action string) string {
	action = NormalizeRuntimeRepairRecoveryAction(action)
	if action == "terminate" {
		return action
	}
	if ticket == nil || ticket.Context == nil {
		return action
	}
	allowed := RuntimeRepairAllowedRecoveryActions(ticket.Context)
	if len(allowed) == 0 || ContainsRuntimeRepairRecoveryAction(allowed, action) {
		return action
	}
	for _, candidate := range []string{"replan", "provide_information", "retry_same_step", "terminate"} {
		if ContainsRuntimeRepairRecoveryAction(allowed, candidate) {
			return candidate
		}
	}
	return ""
}
