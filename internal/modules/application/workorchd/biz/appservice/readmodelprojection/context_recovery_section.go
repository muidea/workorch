package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ContextRecoverySection(recovery readmodelcore.RecoveryPlan) readmodelcore.ContextSection {
	return readmodelcore.ContextSection{
		ID:       "recovery",
		Kind:     "recovery",
		Title:    "Recovery recommendation",
		Priority: 100,
		State:    recovery.Mode,
		Summary:  recovery.NextAction,
		EvidenceRefs: compactUniqueStrings([]string{
			recovery.StepID,
			recovery.BlockingTicketID,
			recovery.BlockingApprovalID,
			recovery.CheckpointID,
		}),
		Data: map[string]any{
			"mode":               recovery.Mode,
			"nextAction":         recovery.NextAction,
			"runId":              recovery.RunID,
			"stepId":             recovery.StepID,
			"blockingTicketId":   recovery.BlockingTicketID,
			"blockingApprovalId": recovery.BlockingApprovalID,
			"checkpointId":       recovery.CheckpointID,
			"reason":             recovery.Reason,
		},
	}
}
