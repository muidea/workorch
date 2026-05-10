package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
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

const maxOperatorInputQueueItems = 20

func ContinueRun(be backend.Backend, workspaceID string, runID string, operatorInput string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if strings.TrimSpace(workspaceID) == "" {
			return RunControlMsg{Message: "No active workspace. Create or select a workspace first."}
		}
		wsID := common.WorkspaceID(workspaceID)
		targetRunID := common.RunID(runID)
		if targetRunID == "" {
			workspace, err := be.Workspace().Get(ctx, wsID)
			if err != nil {
				return RunControlMsg{WorkspaceID: workspaceID, Error: err}
			}
			runs, err := be.Run().List(ctx, wsID)
			if err != nil {
				return RunControlMsg{WorkspaceID: workspaceID, Error: err}
			}
			run := selectWorkspaceRun(workspace.ActiveRunID, runs)
			if run == nil {
				return activateWorkspaceRun(ctx, be, wsID, nil, operatorInput)
			}
			targetRunID = run.ID
		}
		run, err := be.Run().Get(ctx, targetRunID)
		if err != nil {
			return RunControlMsg{WorkspaceID: workspaceID, Error: err}
		}
		if run == nil {
			return activateWorkspaceRun(ctx, be, wsID, nil, operatorInput)
		}
		switch run.State {
		case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
			return activateWorkspaceRun(ctx, be, wsID, run, operatorInput)
		case common.RunStateValidating:
			return RunControlMsg{Run: run, WorkspaceID: workspaceID, Message: "Run is validating; refreshing workspace state."}
		default:
			resumed, err := be.Run().Resume(ctx, run.ID, operatorInputPatch(run, operatorInput))
			if err != nil {
				return RunControlMsg{Run: run, WorkspaceID: workspaceID, Error: err}
			}
			return RunControlMsg{Run: resumed, WorkspaceID: workspaceID, Message: "Continuing active run"}
		}
	}
}

func RestartWorkspaceRun(be backend.Backend, workspaceID string, operatorInput string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if strings.TrimSpace(workspaceID) == "" {
			return RunControlMsg{Message: "No active workspace. Create or select a workspace first."}
		}
		return activateWorkspaceRun(ctx, be, common.WorkspaceID(workspaceID), nil, operatorInput)
	}
}

func activateWorkspaceRun(ctx context.Context, be backend.Backend, workspaceID common.WorkspaceID, run *backend.RunInfo, operatorInput string) tea.Msg {
	activated, err := be.Run().Activate(ctx, workspaceID, backend.ActivateRunRequest{
		ExecutionContextPatch: operatorInputPatch(run, operatorInput),
	})
	if err != nil {
		return RunControlMsg{Run: run, WorkspaceID: string(workspaceID), Error: err}
	}
	return RunControlMsg{Run: activated, WorkspaceID: string(workspaceID), Message: "Restarting workspace run"}
}

func operatorInputPatch(run *backend.RunInfo, value string) map[string]any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	queue := operatorInputQueueFromRun(run)
	queue = append(queue, map[string]any{
		"source":      "tui",
		"content":     value,
		"submittedAt": time.Now().UTC().Format(time.RFC3339Nano),
	})
	if len(queue) > maxOperatorInputQueueItems {
		queue = append([]map[string]any(nil), queue[len(queue)-maxOperatorInputQueueItems:]...)
	}
	return map[string]any{
		"operatorInputQueue": queue,
		"latestOperatorInput": map[string]any{
			"source":  "tui",
			"content": value,
		},
	}
}

func operatorInputQueueFromRun(run *backend.RunInfo) []map[string]any {
	if run == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	rawItems, ok := run.ExecutionContext["operatorInputQueue"].([]any)
	if ok {
		ret := make([]map[string]any, 0, len(rawItems))
		for _, raw := range rawItems {
			if item, ok := raw.(map[string]any); ok && strings.TrimSpace(stringValueFromMap(item, "content")) != "" {
				ret = append(ret, item)
			}
		}
		return ret
	}
	if items, ok := run.ExecutionContext["operatorInputQueue"].([]map[string]any); ok {
		ret := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if strings.TrimSpace(stringValueFromMap(item, "content")) != "" {
				ret = append(ret, item)
			}
		}
		return ret
	}
	return nil
}

func stringValueFromMap(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return value
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
	semantic := governancecommon.NormalizeDecisionResolution("auto", map[string]any{
		"answer":   answer,
		"operator": "tui",
	})
	return semantic.ResolutionType
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
