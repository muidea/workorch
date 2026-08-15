package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// selectDisplayRunID chooses the run that should drive RuntimeView history and
// progress. Priority:
// 1. current active run
// 2. current decision/approval blocker run
// 3. current outcome run
// 4. current TaskProgress run
// 5. latest workspace-owned run
//
// Historical runs may drive activity/summary only; they must never become the
// sole authority for active prompt mode or control state.
func selectDisplayRunID(snapshot readmodelcore.WorkspaceSnapshotState) string {
	if id := strings.TrimSpace(snapshot.Workspace.ActiveRunID); id != "" {
		return id
	}
	if id := blockerRunID(snapshot); id != "" {
		return id
	}
	if id := strings.TrimSpace(snapshot.Outcome.RunID); id != "" {
		return id
	}
	if id := strings.TrimSpace(snapshot.TaskProgress.RunID); id != "" {
		return id
	}
	if snapshot.Run != nil {
		if id := strings.TrimSpace(snapshot.Run.ID); id != "" {
			return id
		}
	}
	return latestWorkspaceRunID(snapshot.Runs, snapshot.Workspace.ID)
}

func blockerRunID(snapshot readmodelcore.WorkspaceSnapshotState) string {
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	ticketID := strings.TrimSpace(snapshot.Workspace.BlockingTicketID)
	if ticketID == "" && snapshot.Run != nil {
		ticketID = strings.TrimSpace(snapshot.Run.BlockingTicketID)
	}
	if ticketID != "" {
		if id := openDecisionRunID(snapshot.Decisions, ticketID, activeRunID); id != "" {
			return id
		}
	}
	approvalID := strings.TrimSpace(snapshot.Workspace.BlockingApprovalID)
	if approvalID == "" && snapshot.Run != nil {
		approvalID = strings.TrimSpace(snapshot.Run.BlockingApprovalID)
	}
	if approvalID != "" {
		if id := openApprovalRunID(snapshot.Approvals, approvalID, activeRunID); id != "" {
			return id
		}
	}
	return ""
}

func openDecisionRunID(items []projectioncontract.Decision, ticketID, preferredRunID string) string {
	ticketID = strings.TrimSpace(ticketID)
	preferredRunID = strings.TrimSpace(preferredRunID)
	for _, item := range items {
		if strings.TrimSpace(item.ID) != ticketID {
			continue
		}
		if isOpenGovernanceState(item.State) {
			if preferredRunID != "" && strings.TrimSpace(item.RunID) == preferredRunID {
				return preferredRunID
			}
			return strings.TrimSpace(item.RunID)
		}
	}
	return ""
}

func openApprovalRunID(items []projectioncontract.Approval, approvalID, preferredRunID string) string {
	approvalID = strings.TrimSpace(approvalID)
	preferredRunID = strings.TrimSpace(preferredRunID)
	for _, item := range items {
		if strings.TrimSpace(item.ID) != approvalID {
			continue
		}
		if isOpenGovernanceState(item.State) {
			if preferredRunID != "" && strings.TrimSpace(item.RunID) == preferredRunID {
				return preferredRunID
			}
			return strings.TrimSpace(item.RunID)
		}
	}
	return ""
}

func isOpenGovernanceState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "open", "pending", "waiting", "blocked":
		return true
	default:
		return false
	}
}

func latestWorkspaceRunID(runs []appmodelcore.Run, workspaceID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	var best appmodelcore.Run
	found := false
	for _, run := range runs {
		if workspaceID != "" && strings.TrimSpace(run.WorkspaceID) != "" && strings.TrimSpace(run.WorkspaceID) != workspaceID {
			continue
		}
		if !found || runIsNewer(run, best) {
			best = run
			found = true
		}
	}
	if !found {
		return ""
	}
	return strings.TrimSpace(best.ID)
}

func runIsNewer(candidate, current appmodelcore.Run) bool {
	candidateStamp := firstNonEmptyTimelineString(candidate.EndedAt, candidate.StartedAt, candidate.CreatedAt)
	currentStamp := firstNonEmptyTimelineString(current.EndedAt, current.StartedAt, current.CreatedAt)
	if candidateStamp != currentStamp {
		return candidateStamp > currentStamp
	}
	return strings.TrimSpace(candidate.ID) > strings.TrimSpace(current.ID)
}

func selectDisplayRun(snapshot readmodelcore.WorkspaceSnapshotState) *appmodelcore.Run {
	displayRunID := selectDisplayRunID(snapshot)
	if displayRunID == "" {
		return nil
	}
	if snapshot.Run != nil && strings.TrimSpace(snapshot.Run.ID) == displayRunID {
		run := *snapshot.Run
		return &run
	}
	for idx := range snapshot.Runs {
		if strings.TrimSpace(snapshot.Runs[idx].ID) == displayRunID {
			run := snapshot.Runs[idx]
			return &run
		}
	}
	return &appmodelcore.Run{
		ID:          displayRunID,
		WorkspaceID: strings.TrimSpace(snapshot.Workspace.ID),
	}
}
