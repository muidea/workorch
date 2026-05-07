package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceSnapshotBackend interface {
	Snapshot(ctx context.Context, id common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error)
}

func FetchAll(be backend.Backend) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		workspaces, err := be.Workspace().List(ctx)
		if err != nil {
			return WorkspaceListMsg{Error: err}
		}
		return WorkspaceListMsg{Workspaces: workspaces}
	}
}

func FetchWorkspaceContext(be backend.Backend, workspaceID string) tea.Cmd {
	if workspaceID == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		wsID := common.WorkspaceID(workspaceID)
		if snapshotBackend, ok := be.Workspace().(workspaceSnapshotBackend); ok {
			snapshot, err := snapshotBackend.Snapshot(ctx, wsID)
			if err != nil {
				return WorkspaceContextMsg{Error: err}
			}
			if snapshot != nil {
				activeRunID := snapshot.Workspace.ActiveRunID
				if activeRunID == "" {
					activeRunID = snapshot.Outcome.RunID
				}
				activeRun := selectWorkspaceRun(activeRunID, snapshot.Runs)
				var status *backend.RunStatusInfo
				var goal *backend.GoalInfo
				var dod *backend.DoDInfo
				if activeRun != nil {
					status, _ = be.Run().Status(ctx, activeRun.ID)
					if activeRun.GoalID != "" {
						goal, _ = be.Goal().Get(ctx, activeRun.GoalID)
					}
					if activeRun.DoDID != "" {
						dod, _ = be.DoD().Get(ctx, activeRun.DoDID)
					}
				}
				return WorkspaceContextMsg{
					Workspace: &snapshot.Workspace,
					Snapshot:  snapshot,
					Runs:      snapshot.Runs,
					ActiveRun: activeRun,
					Status:    status,
					Goal:      goal,
					DoD:       dod,
					Decisions: snapshot.Decisions,
					Approvals: snapshot.Approvals,
				}
			}
		}

		workspace, err := be.Workspace().Get(ctx, wsID)
		if err != nil {
			return WorkspaceContextMsg{Error: err}
		}
		runs, err := be.Run().List(ctx, wsID)
		if err != nil {
			return WorkspaceContextMsg{Error: err}
		}
		run := selectWorkspaceRun(workspace.ActiveRunID, runs)
		if run != nil {
			fullRun, err := be.Run().Get(ctx, run.ID)
			if err == nil && fullRun != nil {
				run = fullRun
			}
		}
		var status *backend.RunStatusInfo
		var goal *backend.GoalInfo
		var dod *backend.DoDInfo
		if run != nil {
			status, _ = be.Run().Status(ctx, run.ID)
			if run.GoalID != "" {
				goal, _ = be.Goal().Get(ctx, run.GoalID)
			}
			if run.DoDID != "" {
				dod, _ = be.DoD().Get(ctx, run.DoDID)
			}
		}
		decisions, err := be.Decision().List(ctx, wsID)
		if err != nil {
			return WorkspaceContextMsg{Error: err}
		}
		approvals, err := be.Approval().List(ctx, wsID)
		if err != nil {
			return WorkspaceContextMsg{Error: err}
		}
		return WorkspaceContextMsg{
			Workspace: workspace,
			Runs:      runs,
			ActiveRun: run,
			Status:    status,
			Goal:      goal,
			DoD:       dod,
			Decisions: decisions,
			Approvals: approvals,
		}
	}
}

func CreateWorkspace(be backend.Backend, req backend.CreateWorkspaceRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ws, err := be.Workspace().Create(ctx, req)
		return WorkspaceCreatedMsg{Workspace: ws, Error: err}
	}
}

func FetchDecisions(be backend.Backend, workspaceID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		items, err := be.Decision().List(ctx, common.WorkspaceID(workspaceID))
		return DecisionListMsg{Decisions: items, Error: err}
	}
}

func selectWorkspaceRun(activeRunID common.RunID, runs []backend.RunInfo) *backend.RunInfo {
	if len(runs) == 0 {
		return nil
	}
	if activeRunID != "" {
		for i := range runs {
			if runs[i].ID == activeRunID {
				return &runs[i]
			}
		}
	}
	for i := len(runs) - 1; i >= 0; i-- {
		switch runs[i].State {
		case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
			continue
		default:
			return &runs[i]
		}
	}
	return &runs[len(runs)-1]
}

func FetchDecision(be backend.Backend, ticketID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		item, err := be.Decision().Get(ctx, common.TicketID(ticketID))
		return DecisionDetailMsg{Decision: item, Error: err}
	}
}

func FetchApprovals(be backend.Backend, workspaceID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		items, err := be.Approval().List(ctx, common.WorkspaceID(workspaceID))
		return ApprovalListMsg{Approvals: items, Error: err}
	}
}

func FetchApproval(be backend.Backend, approvalID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		item, err := be.Approval().Get(ctx, common.ApprovalID(approvalID))
		return ApprovalDetailMsg{Approval: item, Error: err}
	}
}

func FetchBlockers(be backend.Backend, workspaceID string) tea.Cmd {
	if workspaceID == "" {
		return nil
	}
	return tea.Batch(FetchDecisions(be, workspaceID), FetchApprovals(be, workspaceID))
}

func WaitForBackendEvent(ch <-chan backend.Event) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			return ErrorMsg{Error: context.Canceled}
		}
		return BackendEventMsg{Event: evt}
	}
}

func SubmitGoal(be backend.Backend, workspaceID string, text string, dodItems []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		goal, err := be.Goal().Define(ctx, common.WorkspaceID(workspaceID), text)
		if err != nil {
			return RunStartedMsg{Error: err}
		}
		if len(dodItems) == 0 {
			dodItems = backend.DefaultDoDItemsForWorkspace(ctx, be, common.WorkspaceID(workspaceID))
		}
		dod, err := be.DoD().Define(ctx, common.WorkspaceID(workspaceID), dodItems)
		if err != nil {
			return RunStartedMsg{Error: err}
		}
		run, err := be.Run().Activate(ctx, common.WorkspaceID(workspaceID), backend.ActivateRunRequest{
			GoalID: goal.ID,
			DoDID:  dod.ID,
		})
		if err != nil {
			return RunStartedMsg{Error: err}
		}
		return RunStartedMsg{Run: run}
	}
}

func ResolveDecision(be backend.Backend, ticketID string, answer string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		resolutionType := inferDecisionResolutionType(answer)
		decision, err := be.Decision().Resolve(ctx, backend.ResolveDecisionRequest{
			TicketID:       common.TicketID(ticketID),
			ResolutionType: resolutionType,
			Payload:        map[string]any{"answer": answer, "operator": "tui"},
		})
		if err != nil {
			return DecisionResolvedMsg{Error: err}
		}
		if decision.RunID != "" {
			_, err = be.Run().Resume(ctx, decision.RunID)
		}
		return DecisionResolvedMsg{Decision: decision, Error: err}
	}
}

func inferDecisionResolutionType(answer string) string {
	text := strings.ToLower(strings.TrimSpace(answer))
	switch {
	case text == "":
		return decisioncommon.ResolutionTypeProvideMissingInfo
	case containsAny(text, "reject", "reject current path", "abort", "stop", "cancel", "拒绝", "终止", "停止", "取消", "不要继续"):
		return decisioncommon.ResolutionTypeRejectCurrentPath
	case containsAny(text, "replan", "重新规划", "重新计划", "重新制定计划", "重做计划", "换方案", "重新构建", "重建"):
		return decisioncommon.ResolutionTypeReplan
	default:
		return decisioncommon.ResolutionTypeProvideMissingInfo
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func ResolveApproval(be backend.Backend, approvalID string, approve bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var approval *backend.ApprovalInfo
		var err error
		resumeHandled := false
		if approve {
			approval, err = be.Approval().Approve(ctx, common.ApprovalID(approvalID))
		} else {
			approval, err = be.Approval().Reject(ctx, common.ApprovalID(approvalID))
		}
		if err != nil && errors.Is(err, common.ErrConflict) {
			approval, err = be.Approval().Get(ctx, common.ApprovalID(approvalID))
			if err == nil && approval != nil {
				switch {
				case approve && approval.State == string(common.ApprovalStateApproved):
					err = resumeApprovalRun(ctx, be, approval)
					resumeHandled = true
				case !approve && approval.State == string(common.ApprovalStateRejected):
					err = resumeApprovalRun(ctx, be, approval)
					resumeHandled = true
				}
			}
		}
		if err == nil && !resumeHandled && approval != nil && approval.RunID != "" {
			_, err = be.Run().Resume(ctx, approval.RunID)
		}
		return ApprovalResolvedMsg{Approval: approval, Error: err}
	}
}

func resumeApprovalRun(ctx context.Context, be backend.Backend, approval *backend.ApprovalInfo) error {
	if approval == nil || approval.RunID == "" {
		return nil
	}
	run, err := be.Run().Get(ctx, approval.RunID)
	if err != nil {
		return err
	}
	if run == nil || run.State != common.RunStateBlockedOnApproval {
		return nil
	}
	_, err = be.Run().Resume(ctx, approval.RunID)
	return err
}
