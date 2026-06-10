package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

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

func FetchConfiguration(be backend.Backend) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg, err := be.Config().Get(ctx)
		if err != nil {
			return ConfigMsg{Error: err}
		}
		return ConfigMsg{Config: cfg}
	}
}

func FetchWorkspaceContext(be backend.Backend, workspaceID string) tea.Cmd {
	return fetchWorkspaceContext(be, workspaceID, false)
}

func FetchWorkspaceContextForSwitch(be backend.Backend, workspaceID string) tea.Cmd {
	return fetchWorkspaceContext(be, workspaceID, true)
}

func fetchWorkspaceContext(be backend.Backend, workspaceID string, resetMainViewport bool) tea.Cmd {
	if workspaceID == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		wsID := common.WorkspaceID(workspaceID)
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		snapshot, err := be.Workspace().Snapshot(ctx, wsID)
		if err != nil {
			return WorkspaceContextMsg{WorkspaceID: workspaceID, ResetMainViewport: resetMainViewport, Error: err}
		}
		if snapshot != nil {
			liveRunID := snapshot.Workspace.ActiveRunID
			activeRun := selectWorkspaceRun(liveRunID, snapshot.Runs)
			displayRun := activeRun
			if displayRun == nil && snapshot.Outcome.RunID != "" {
				displayRun = selectWorkspaceRun(snapshot.Outcome.RunID, snapshot.Runs)
			}
			status := runStatusFromWorkspaceSnapshot(snapshot, displayRun)
			goal := resolveWorkspaceContextGoal(snapshot, displayRun, nil)
			dod := resolveWorkspaceContextDoD(snapshot, displayRun, nil)
			return WorkspaceContextMsg{
				WorkspaceID:       workspaceID,
				ResetMainViewport: resetMainViewport,
				Workspace:         &snapshot.Workspace,
				Snapshot:          snapshot,
				Runs:              snapshot.Runs,
				ActiveRun:         activeRun,
				DisplayRun:        displayRun,
				Status:            status,
				Goal:              goal,
				DoD:               dod,
				Decisions:         snapshot.Decisions,
				Approvals:         snapshot.Approvals,
				WorkspaceHasLive:  snapshot.Workspace.ActiveRunID != "",
			}
		}

		return WorkspaceContextMsg{WorkspaceID: workspaceID, ResetMainViewport: resetMainViewport, Error: common.ErrInvalidArgument}
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
		wsID := common.WorkspaceID(workspaceID)
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		snapshot, err := be.Workspace().Snapshot(ctx, wsID)
		if err != nil {
			return DecisionListMsg{WorkspaceID: workspaceID, Error: err}
		}
		var items []backend.DecisionInfo
		if snapshot != nil {
			items = snapshot.Decisions
		}
		return DecisionListMsg{WorkspaceID: workspaceID, Decisions: items, Error: err}
	}
}

func resolveWorkspaceContextGoal(snapshot *backend.WorkspaceSnapshotInfo, run *backend.RunInfo, current *backend.GoalInfo) *backend.GoalInfo {
	if current != nil && strings.TrimSpace(current.Text) != "" {
		return current
	}
	if snapshot == nil || len(snapshot.Goals) == 0 {
		return current
	}
	if run != nil && run.GoalID != "" {
		for i := range snapshot.Goals {
			if snapshot.Goals[i].ID == run.GoalID {
				return &snapshot.Goals[i]
			}
		}
	}
	return &snapshot.Goals[len(snapshot.Goals)-1]
}

func resolveWorkspaceContextDoD(snapshot *backend.WorkspaceSnapshotInfo, run *backend.RunInfo, current *backend.DoDInfo) *backend.DoDInfo {
	if current != nil && len(current.Items) > 0 {
		return current
	}
	if snapshot == nil || len(snapshot.DoDs) == 0 {
		return current
	}
	if run != nil && run.DoDID != "" {
		for i := range snapshot.DoDs {
			if snapshot.DoDs[i].ID == run.DoDID {
				return &snapshot.DoDs[i]
			}
		}
	}
	return &snapshot.DoDs[len(snapshot.DoDs)-1]
}

func runStatusFromWorkspaceSnapshot(snapshot *backend.WorkspaceSnapshotInfo, run *backend.RunInfo) *backend.RunStatusInfo {
	if snapshot == nil || run == nil {
		return nil
	}
	status := &backend.RunStatusInfo{
		ID:                 run.ID,
		WorkspaceID:        run.WorkspaceID,
		State:              run.State,
		BlockingTicketID:   run.BlockingTicketID,
		BlockingApprovalID: run.BlockingApprovalID,
		FailureReason:      run.FailureReason,
	}
	if snapshot.TaskProgress.RunID == "" || snapshot.TaskProgress.RunID == run.ID {
		if snapshot.TaskProgress.State != "" {
			status.State = common.RunState(snapshot.TaskProgress.State)
		}
		status.CurrentStepID = snapshot.TaskProgress.Recovery.StepID
		if status.BlockingTicketID == "" {
			status.BlockingTicketID = snapshot.TaskProgress.Recovery.BlockingTicketID
		}
		if status.BlockingApprovalID == "" {
			status.BlockingApprovalID = snapshot.TaskProgress.Recovery.BlockingApprovalID
		}
		if status.FailureReason == "" {
			status.FailureReason = snapshot.TaskProgress.Recovery.Reason
		}
	}
	return status
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

func FetchDecision(be backend.Backend, workspaceID string, ticketID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		wsID := common.WorkspaceID(workspaceID)
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		snapshot, err := be.Workspace().Snapshot(ctx, wsID)
		if err != nil {
			return DecisionDetailMsg{WorkspaceID: workspaceID, Error: err}
		}
		item := findSnapshotDecision(snapshot, common.TicketID(ticketID))
		if item == nil {
			err = common.ErrNotFound
		}
		return DecisionDetailMsg{WorkspaceID: workspaceID, Decision: item, Error: err}
	}
}

func FetchApprovals(be backend.Backend, workspaceID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		wsID := common.WorkspaceID(workspaceID)
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		snapshot, err := be.Workspace().Snapshot(ctx, wsID)
		if err != nil {
			return ApprovalListMsg{WorkspaceID: workspaceID, Error: err}
		}
		var items []backend.ApprovalInfo
		if snapshot != nil {
			items = snapshot.Approvals
		}
		return ApprovalListMsg{WorkspaceID: workspaceID, Approvals: items, Error: err}
	}
}

func FetchApproval(be backend.Backend, workspaceID string, approvalID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		wsID := common.WorkspaceID(workspaceID)
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		snapshot, err := be.Workspace().Snapshot(ctx, wsID)
		if err != nil {
			return ApprovalDetailMsg{WorkspaceID: workspaceID, Error: err}
		}
		item := findSnapshotApproval(snapshot, common.ApprovalID(approvalID))
		if item == nil {
			err = common.ErrNotFound
		}
		return ApprovalDetailMsg{WorkspaceID: workspaceID, Approval: item, Error: err}
	}
}

func findSnapshotDecision(snapshot *backend.WorkspaceSnapshotInfo, ticketID common.TicketID) *backend.DecisionInfo {
	if snapshot == nil || ticketID == "" {
		return nil
	}
	for idx := range snapshot.Decisions {
		if snapshot.Decisions[idx].ID == ticketID {
			return &snapshot.Decisions[idx]
		}
	}
	return nil
}

func findSnapshotApproval(snapshot *backend.WorkspaceSnapshotInfo, approvalID common.ApprovalID) *backend.ApprovalInfo {
	if snapshot == nil || approvalID == "" {
		return nil
	}
	for idx := range snapshot.Approvals {
		if snapshot.Approvals[idx].ID == approvalID {
			return &snapshot.Approvals[idx]
		}
	}
	return nil
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
		wsID := common.WorkspaceID(workspaceID)
		goal, err := be.Goal().Define(ctx, wsID, text)
		if err != nil {
			return RunStartedMsg{WorkspaceID: workspaceID, Error: err}
		}
		if len(dodItems) == 0 {
			dodItems = backend.DefaultDoDItemsForWorkspace(ctx, be, wsID)
		}
		dod, err := be.DoD().Define(ctx, wsID, dodItems)
		if err != nil {
			return RunStartedMsg{WorkspaceID: workspaceID, Error: err}
		}
		run, err := be.Run().Activate(ctx, wsID, backend.ActivateRunRequest{
			GoalID: goal.ID,
			DoDID:  dod.ID,
		})
		if err != nil {
			return RunStartedMsg{WorkspaceID: workspaceID, Error: err}
		}
		if run != nil && run.WorkspaceID == "" {
			run.WorkspaceID = wsID
		}
		return RunStartedMsg{Run: run, WorkspaceID: workspaceID}
	}
}

func PersistDoD(be backend.Backend, workspaceID string, items []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if strings.TrimSpace(workspaceID) == "" {
			return DoDUpdatedMsg{Error: common.ErrInvalidArgument}
		}
		dod, err := be.DoD().Define(ctx, common.WorkspaceID(workspaceID), items)
		if err != nil {
			return DoDUpdatedMsg{Error: err}
		}
		return DoDUpdatedMsg{DoD: dod}
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
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		targetRunID := common.RunID(runID)
		var run *backend.RunInfo
		snapshot, err := be.Workspace().Snapshot(ctx, wsID)
		if err != nil {
			return RunControlMsg{WorkspaceID: workspaceID, Error: err}
		}
		if snapshot == nil {
			return RunControlMsg{WorkspaceID: workspaceID, Error: common.ErrInvalidArgument}
		}
		if targetRunID != "" {
			run = findWorkspaceRun(targetRunID, snapshot.Runs)
			if !runBelongsToWorkspace(run, wsID) {
				run = nil
				targetRunID = ""
			}
		}
		if run == nil && snapshot.Workspace.ActiveRunID != "" {
			run = findWorkspaceRun(snapshot.Workspace.ActiveRunID, snapshot.Runs)
		}
		if run == nil {
			displayRun := selectWorkspaceDisplayRun(snapshot)
			if displayRun != nil && runStateIsTerminal(displayRun.State) {
				return RunControlMsg{Run: displayRun, WorkspaceID: workspaceID, Message: terminalRunContinueMessage(displayRun)}
			}
			return RunControlMsg{Run: displayRun, WorkspaceID: workspaceID, Message: "No active workspace run; submit a goal or use /run restart."}
		}
		switch run.State {
		case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
			return RunControlMsg{Run: run, WorkspaceID: workspaceID, Message: terminalRunContinueMessage(run)}
		case common.RunStateRunning:
			return RunControlMsg{Run: run, WorkspaceID: workspaceID, Message: "Run is already running; refreshing workspace state."}
		case common.RunStateBlockedOnDecision:
			return RunControlMsg{Run: run, WorkspaceID: workspaceID, Message: "Run is waiting for decision; answer the decision before continuing."}
		case common.RunStateBlockedOnApproval:
			return RunControlMsg{Run: run, WorkspaceID: workspaceID, Message: "Run is waiting for approval; resolve the approval before continuing."}
		case common.RunStateCreated:
			return RunControlMsg{Run: run, WorkspaceID: workspaceID, Message: "Run is created but not running; refreshing workspace state."}
		case common.RunStateValidating:
			return RunControlMsg{Run: run, WorkspaceID: workspaceID, Message: "Run is validating; refreshing workspace state."}
		default:
			resumed, err := be.Run().Resume(ctx, wsID, run.ID, operatorInputPatch(run, operatorInput))
			if err != nil {
				return RunControlMsg{Run: run, WorkspaceID: workspaceID, Error: err}
			}
			return RunControlMsg{Run: resumed, WorkspaceID: workspaceID, Message: "Continuing active run"}
		}
	}
}

func findWorkspaceRun(id common.RunID, runs []backend.RunInfo) *backend.RunInfo {
	if id == "" {
		return nil
	}
	for i := range runs {
		if runs[i].ID == id {
			return &runs[i]
		}
	}
	return nil
}

func selectWorkspaceDisplayRun(snapshot *backend.WorkspaceSnapshotInfo) *backend.RunInfo {
	if snapshot == nil {
		return nil
	}
	if run := findWorkspaceRun(snapshot.Workspace.ActiveRunID, snapshot.Runs); run != nil {
		return run
	}
	if run := findWorkspaceRun(snapshot.Outcome.RunID, snapshot.Runs); run != nil {
		return run
	}
	if run := findWorkspaceRun(snapshot.TaskProgress.RunID, snapshot.Runs); run != nil {
		return run
	}
	return selectWorkspaceRun("", snapshot.Runs)
}

func terminalRunContinueMessage(run *backend.RunInfo) string {
	if run == nil {
		return "No active workspace run; submit a goal or use /run restart."
	}
	switch run.State {
	case common.RunStateCompleted:
		return "Workspace run is completed; use /run restart to start a new run."
	case common.RunStateFailed:
		return "Workspace run is failed; inspect failure or use /run restart."
	case common.RunStateCancelled:
		return "Workspace run is cancelled; use /run restart to start a new run."
	case common.RunStateSuperseded:
		return "Workspace run is superseded; use /run restart to start a new run."
	default:
		return "No active workspace run; submit a goal or use /run restart."
	}
}

func runBelongsToWorkspace(run *backend.RunInfo, workspaceID common.WorkspaceID) bool {
	if run == nil {
		return false
	}
	return run.WorkspaceID == "" || run.WorkspaceID == workspaceID
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
	return ResolveDecisionWithType(be, ticketID, inferDecisionResolutionType(answer), answer)
}

func ResolveDecisionWithType(be backend.Backend, ticketID string, resolutionType string, answer string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if !decisioncommon.IsSupportedResolutionType(resolutionType) {
			resolutionType = inferDecisionResolutionType(answer)
		}
		decision, err := be.Decision().Resolve(ctx, backend.ResolveDecisionRequest{
			TicketID:       common.TicketID(ticketID),
			ResolutionType: resolutionType,
			Payload:        map[string]any{"answer": answer, "operator": "tui"},
		})
		if err != nil {
			return DecisionResolvedMsg{Error: err}
		}
		if decision.RunID != "" {
			resumeCtx := common.ContextWithRuntimeWorkspaceID(ctx, decision.WorkspaceID)
			_, err = be.Run().Resume(resumeCtx, decision.WorkspaceID, decision.RunID)
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

func ResolveApproval(be backend.Backend, workspaceID string, approvalID string, approve bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		wsID := common.WorkspaceID(workspaceID)
		if wsID != "" {
			ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		}
		var approval *backend.ApprovalInfo
		var err error
		resumeHandled := false
		if approve {
			approval, err = be.Approval().Approve(ctx, common.ApprovalID(approvalID))
		} else {
			approval, err = be.Approval().Reject(ctx, common.ApprovalID(approvalID))
		}
		if err != nil && errors.Is(err, common.ErrConflict) {
			var snapshot *backend.WorkspaceSnapshotInfo
			if wsID == "" {
				err = common.ErrInvalidArgument
			} else {
				snapshot, err = be.Workspace().Snapshot(ctx, wsID)
			}
			approval = findSnapshotApproval(snapshot, common.ApprovalID(approvalID))
			if err == nil && approval == nil {
				err = common.ErrNotFound
			}
			if err == nil && approval != nil {
				if approval.WorkspaceID == "" {
					approval.WorkspaceID = wsID
				}
				switch {
				case approve && approval.State == string(common.ApprovalStateApproved):
					err = resumeApprovalRunFromSnapshot(ctx, be, approval, snapshot)
					resumeHandled = true
				case !approve && approval.State == string(common.ApprovalStateRejected):
					err = resumeApprovalRunFromSnapshot(ctx, be, approval, snapshot)
					resumeHandled = true
				}
			}
		}
		if err == nil && !resumeHandled && approval != nil && approval.RunID != "" {
			resumeCtx := common.ContextWithRuntimeWorkspaceID(ctx, approval.WorkspaceID)
			_, err = be.Run().Resume(resumeCtx, approval.WorkspaceID, approval.RunID)
		}
		return ApprovalResolvedMsg{Approval: approval, Error: err}
	}
}

func resumeApprovalRunFromSnapshot(ctx context.Context, be backend.Backend, approval *backend.ApprovalInfo, snapshot *backend.WorkspaceSnapshotInfo) error {
	if approval == nil || approval.RunID == "" || snapshot == nil {
		return nil
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, approval.WorkspaceID)
	run := selectWorkspaceRun(approval.RunID, snapshot.Runs)
	if run == nil || run.State != common.RunStateBlockedOnApproval {
		return nil
	}
	_, err := be.Run().Resume(ctx, approval.WorkspaceID, approval.RunID)
	return err
}
