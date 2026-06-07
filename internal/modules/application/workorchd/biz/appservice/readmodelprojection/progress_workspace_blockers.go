package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func applyWorkspaceProgressBlockers(progress *readmodelcore.TaskProgress, snapshot readmodelcore.WorkspaceSnapshot) {
	for _, item := range snapshot.Decisions {
		if IsResolvedDecisionState(item.State) {
			continue
		}
		progress.Blockers = append(progress.Blockers, readmodelcore.ProgressBlocker{
			ID:          item.ID,
			Kind:        "decision",
			State:       item.State,
			Reason:      firstNonEmptyTimelineString(item.Question, item.Reason),
			EvidenceRef: item.StepID,
		})
		if progress.Recovery.BlockingTicketID == "" {
			progress.Recovery.Mode = "wait_decision"
			progress.Recovery.NextAction = "resolve_decision_ticket"
			progress.Recovery.BlockingTicketID = item.ID
			progress.Recovery.StepID = item.StepID
			progress.Recovery.Reason = firstNonEmptyTimelineString(item.Question, item.Reason)
		}
	}
	applyWorkspaceProgressApprovalBlockers(progress, snapshot)
}
