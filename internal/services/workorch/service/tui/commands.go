package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

func FetchAll(be backend.Backend, requestSeq ...int64) tea.Cmd {
	seq := optionalRequestSeq(requestSeq)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		workspaces, err := be.Workspace().List(ctx)
		if err != nil {
			return WorkspaceListMsg{RequestSeq: seq, Error: err}
		}
		projection := backend.ProjectionInfo{}
		for _, ws := range workspaces {
			if ws.Projection.GlobalRevision > projection.GlobalRevision {
				projection = ws.Projection
			}
		}
		return WorkspaceListMsg{RequestSeq: seq, Projection: projection, Workspaces: workspaces}
	}
}

func RetryWorkspaceList() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(300 * time.Millisecond)
		return WorkspaceListRetryMsg{}
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

func FetchWorkspaceContext(be backend.Backend, workspaceID string, requestSeq ...int64) tea.Cmd {
	// Active refresh prefers lightweight RuntimeView projection.
	return fetchWorkspaceRuntimeView(be, workspaceID, false, optionalRequestSeq(requestSeq))
}

func FetchWorkspaceContextForSwitch(be backend.Backend, workspaceID string, requestSeq ...int64) tea.Cmd {
	// Workspace switch still reads full snapshot for goals/DoD/artifacts, then
	// applies RuntimeView activity/progress as the authoritative runtime section.
	return fetchWorkspaceContext(be, workspaceID, true, optionalRequestSeq(requestSeq))
}

func FetchWorkspaceRuntimeView(be backend.Backend, workspaceID string, requestSeq ...int64) tea.Cmd {
	return fetchWorkspaceRuntimeView(be, workspaceID, false, optionalRequestSeq(requestSeq))
}

func RetryWorkspaceContext(workspaceID string, resetMainViewport bool) tea.Cmd {
	if workspaceID == "" {
		return nil
	}
	return func() tea.Msg {
		time.Sleep(300 * time.Millisecond)
		return WorkspaceContextRetryMsg{WorkspaceID: workspaceID, ResetMainViewport: resetMainViewport}
	}
}

func fetchWorkspaceContext(be backend.Backend, workspaceID string, resetMainViewport bool, requestSeq int64) tea.Cmd {
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
			return WorkspaceContextMsg{RequestSeq: requestSeq, WorkspaceID: workspaceID, ResetMainViewport: resetMainViewport, Error: err}
		}
		if snapshot != nil {
			liveRunID := snapshot.Workspace.ActiveRunID
			var activeRun *backend.RunInfo
			if liveRunID != "" {
				activeRun = selectWorkspaceRun(liveRunID, snapshot.Runs)
			}
			displayRun := activeRun
			if displayRun == nil && snapshot.Outcome.RunID != "" {
				displayRun = selectWorkspaceRun(snapshot.Outcome.RunID, snapshot.Runs)
			}
			var runtimeView *backend.WorkspaceRuntimeViewInfo
			if view, viewErr := be.Workspace().RuntimeView(ctx, backend.ReadWorkspaceRuntimeViewRequest{WorkspaceID: wsID, ActivityLimit: 50}); viewErr == nil {
				runtimeView = view
				if displayRun == nil && view != nil && view.DisplayRunID != "" {
					displayRun = selectWorkspaceRun(view.DisplayRunID, snapshot.Runs)
				}
			}
			status := runStatusFromWorkspaceSnapshot(snapshot, displayRun)
			goal := resolveWorkspaceContextGoal(snapshot, displayRun, nil)
			dod := resolveWorkspaceContextDoD(snapshot, displayRun, nil)
			decisions := filterSnapshotDecisionsForRun(snapshot, displayRun)
			approvals := filterSnapshotApprovalsForRun(snapshot, displayRun)
			return WorkspaceContextMsg{
				RequestSeq:        requestSeq,
				WorkspaceID:       workspaceID,
				ResetMainViewport: resetMainViewport,
				Workspace:         &snapshot.Workspace,
				Projection:        snapshot.Projection,
				Snapshot:          snapshot,
				RuntimeView:       runtimeView,
				Runs:              snapshot.Runs,
				ActiveRun:         activeRun,
				DisplayRun:        displayRun,
				Status:            status,
				Goal:              goal,
				DoD:               dod,
				Decisions:         decisions,
				Approvals:         approvals,
				WorkspaceHasLive:  snapshot.Workspace.ActiveRunID != "",
			}
		}

		return WorkspaceContextMsg{RequestSeq: requestSeq, WorkspaceID: workspaceID, ResetMainViewport: resetMainViewport, Error: common.ErrInvalidArgument}
	}
}

func fetchWorkspaceRuntimeView(be backend.Backend, workspaceID string, resetMainViewport bool, requestSeq int64) tea.Cmd {
	if workspaceID == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		wsID := common.WorkspaceID(workspaceID)
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		view, err := be.Workspace().RuntimeView(ctx, backend.ReadWorkspaceRuntimeViewRequest{
			WorkspaceID:   wsID,
			ActivityLimit: 50,
		})
		if err != nil {
			return WorkspaceRuntimeViewMsg{RequestSeq: requestSeq, WorkspaceID: workspaceID, ResetMainViewport: resetMainViewport, Error: err}
		}
		return WorkspaceRuntimeViewMsg{RequestSeq: requestSeq, WorkspaceID: workspaceID, ResetMainViewport: resetMainViewport, View: view}
	}
}

func optionalRequestSeq(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	return values[0]
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
		items, err := be.Decision().List(ctx, wsID)
		if err != nil {
			return DecisionListMsg{WorkspaceID: workspaceID, Error: err}
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
		ConvergenceStatus:  cloneConvergenceStatusInfoPtr(snapshot.ConvergenceStatus),
	}
	if snapshot.TaskProgress.RunID == "" || snapshot.TaskProgress.RunID == run.ID {
		if snapshot.TaskProgress.State != "" {
			status.State = common.RunState(snapshot.TaskProgress.State)
		}
		status.CurrentStepID = snapshot.TaskProgress.Recovery.StepID
		status.CurrentActionLabel = snapshot.TaskProgress.CurrentActionLabel
		status.DiagnosticLineage = snapshot.TaskProgress.DiagnosticLineage
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

func selectSnapshotDisplayRun(snapshot *backend.WorkspaceSnapshotInfo) *backend.RunInfo {
	if snapshot == nil {
		return nil
	}
	if snapshot.Workspace.ActiveRunID != "" {
		return selectWorkspaceRun(snapshot.Workspace.ActiveRunID, snapshot.Runs)
	}
	if snapshot.Outcome.RunID != "" {
		return selectWorkspaceRun(snapshot.Outcome.RunID, snapshot.Runs)
	}
	return nil
}

func filterSnapshotDecisionsForRun(snapshot *backend.WorkspaceSnapshotInfo, run *backend.RunInfo) []backend.DecisionInfo {
	items := make([]backend.DecisionInfo, 0)
	if snapshot == nil || run == nil || run.ID == "" {
		return items
	}
	for _, item := range snapshot.Decisions {
		if item.WorkspaceID != "" && item.WorkspaceID != snapshot.Workspace.ID {
			continue
		}
		if item.RunID == run.ID {
			items = append(items, item)
		}
	}
	return items
}

func filterSnapshotApprovalsForRun(snapshot *backend.WorkspaceSnapshotInfo, run *backend.RunInfo) []backend.ApprovalInfo {
	items := make([]backend.ApprovalInfo, 0)
	if snapshot == nil || run == nil || run.ID == "" {
		return items
	}
	for _, item := range snapshot.Approvals {
		if item.WorkspaceID != "" && item.WorkspaceID != snapshot.Workspace.ID {
			continue
		}
		if item.RunID == run.ID {
			items = append(items, item)
		}
	}
	return items
}

func FetchDecision(be backend.Backend, workspaceID string, ticketID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		wsID := common.WorkspaceID(workspaceID)
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		item, err := be.Decision().Get(ctx, wsID, common.TicketID(ticketID))
		if err != nil {
			return DecisionDetailMsg{WorkspaceID: workspaceID, Error: err}
		}
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
		items, err := be.Approval().List(ctx, wsID)
		if err != nil {
			return ApprovalListMsg{WorkspaceID: workspaceID, Error: err}
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
		item, err := be.Approval().Get(ctx, wsID, common.ApprovalID(approvalID))
		if err != nil {
			return ApprovalDetailMsg{WorkspaceID: workspaceID, Error: err}
		}
		if item == nil {
			err = common.ErrNotFound
		}
		return ApprovalDetailMsg{WorkspaceID: workspaceID, Approval: item, Error: err}
	}
}

func FetchBlockers(be backend.Backend, workspaceID string) tea.Cmd {
	if workspaceID == "" {
		return nil
	}
	return tea.Batch(FetchDecisions(be, workspaceID), FetchApprovals(be, workspaceID))
}

func WaitForProjectionSignal(
	projectionCh <-chan backend.WorkspaceProjectionChangedInfo,
	statusCh <-chan backend.SubscriptionStatusInfo,
) tea.Cmd {
	return func() tea.Msg {
		select {
		case info, ok := <-projectionCh:
			if !ok {
				return ErrorMsg{Error: context.Canceled}
			}
			return WorkspaceProjectionChangedMsg{Info: info}
		case status, ok := <-statusCh:
			if !ok {
				return ErrorMsg{Error: context.Canceled}
			}
			return SubscriptionStatusMsg{Status: status}
		}
	}
}

func SubmitGoal(be backend.Backend, workspaceID string, text string, dodItems []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		wsID := common.WorkspaceID(workspaceID)
		_, err := be.Goal().Define(ctx, wsID, text)
		if err != nil {
			return RunStartedMsg{WorkspaceID: workspaceID, Error: err}
		}
		if len(dodItems) == 0 {
			dodItems = backend.DefaultDoDItemsForWorkspace(ctx, be, wsID)
		}
		_, err = be.DoD().Define(ctx, wsID, dodItems)
		if err != nil {
			return RunStartedMsg{WorkspaceID: workspaceID, Error: err}
		}
		run, err := be.Run().Command(ctx, backend.WorkspaceRunCommand{
			WorkspaceID: wsID,
			Intent:      backend.RunCommandIntentRestart,
			Reason:      "user_requested",
			Source:      "tui",
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

func ResumeRun(be backend.Backend, workspaceID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if strings.TrimSpace(workspaceID) == "" {
			return RunControlMsg{Message: "No active workspace. Create or select a workspace first."}
		}
		wsID := common.WorkspaceID(workspaceID)
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		resumed, err := be.Run().Command(ctx, backend.WorkspaceRunCommand{
			WorkspaceID: wsID,
			Intent:      backend.RunCommandIntentResume,
			Reason:      "user_requested",
			Source:      "tui",
		})
		if err != nil {
			return RunControlMsg{WorkspaceID: workspaceID, Error: err}
		}
		return RunControlMsg{Run: resumed, WorkspaceID: workspaceID, Message: "Workspace resume requested"}
	}
}

// RestartRun requests an explicit workspace-scoped fresh activation. It is
// intentionally distinct from ResumeRun: the active Run is never addressed by
// ID and the application/owners decide whether normal supersede or the
// non-resumable fresh-activation fact path applies.
func RestartRun(be backend.Backend, workspaceID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if strings.TrimSpace(workspaceID) == "" {
			return RunControlMsg{Message: "No active workspace. Create or select a workspace first."}
		}
		wsID := common.WorkspaceID(workspaceID)
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		run, err := be.Run().Command(ctx, backend.WorkspaceRunCommand{
			WorkspaceID: wsID,
			Intent:      backend.RunCommandIntentRestart,
			Reason:      "user_requested_fresh_activation",
			Source:      "tui",
		})
		if err != nil {
			return RunControlMsg{WorkspaceID: workspaceID, Error: err}
		}
		return RunControlMsg{Run: run, WorkspaceID: workspaceID, Message: "Fresh workspace activation requested"}
	}
}

func ResolveDecision(be backend.Backend, workspaceID string, ticketID string, answer string) tea.Cmd {
	return ResolveDecisionWithType(be, workspaceID, ticketID, inferDecisionResolutionType(answer), answer)
}

func ResolveDecisionWithType(be backend.Backend, workspaceID string, ticketID string, resolutionType string, answer string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		wsID := common.WorkspaceID(workspaceID)
		if wsID == "" {
			return DecisionResolvedMsg{Error: fmt.Errorf("%w: workspace is required", common.ErrInvalidArgument)}
		}
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		if !backend.IsSupportedDecisionResolutionType(resolutionType) {
			resolutionType = inferDecisionResolutionType(answer)
		}
		current, _ := be.Decision().Get(ctx, wsID, common.TicketID(ticketID))
		if current != nil && !decisionResolutionAllowed(current, resolutionType) {
			return DecisionResolvedMsg{WorkspaceID: wsID, Error: fmt.Errorf("%w: decision action %q is not allowed for ticket %s", common.ErrInvalidArgument, resolutionType, ticketID)}
		}
		decision, err := be.Decision().Resolve(ctx, backend.ResolveDecisionRequest{
			WorkspaceID:    wsID,
			TicketID:       common.TicketID(ticketID),
			ResolutionType: resolutionType,
			Payload: workorcheventpayload.New(
				workorcheventpayload.StringField("answer", answer),
				workorcheventpayload.StringField("operator", "tui"),
			),
		})
		if err != nil {
			return DecisionResolvedMsg{WorkspaceID: wsID, Error: err}
		}
		return DecisionResolvedMsg{WorkspaceID: wsID, Decision: decision, Error: err}
	}
}

func decisionResolutionAllowed(item *backend.DecisionInfo, resolutionType string) bool {
	if item == nil || len(item.AllowedRecoveryActions) == 0 {
		return true
	}
	for _, allowed := range item.AllowedRecoveryActions {
		if allowed == resolutionType {
			return true
		}
	}
	return false
}

func inferDecisionResolutionType(answer string) string {
	semantic := backend.NormalizeDecisionResolution("auto", workorcheventpayload.New(
		workorcheventpayload.StringField("answer", answer),
		workorcheventpayload.StringField("operator", "tui"),
	))
	return semantic.ResolutionType
}

func ResolveApproval(be backend.Backend, workspaceID string, approvalID string, approve bool, payload ...workorcheventpayload.Payload) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		wsID := common.WorkspaceID(workspaceID)
		if wsID == "" {
			return ApprovalResolvedMsg{Error: fmt.Errorf("%w: workspace is required", common.ErrInvalidArgument)}
		}
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, wsID)
		var approval *backend.ApprovalInfo
		var err error
		if approve {
			approval, err = be.Approval().Approve(ctx, wsID, common.ApprovalID(approvalID), payload...)
		} else {
			approval, err = be.Approval().Reject(ctx, wsID, common.ApprovalID(approvalID), payload...)
		}
		if err != nil && errors.Is(err, common.ErrConflict) && wsID != "" {
			current, getErr := be.Approval().Get(ctx, wsID, common.ApprovalID(approvalID))
			if getErr == nil && current != nil {
				if current.WorkspaceID == "" {
					current.WorkspaceID = wsID
				}
				switch {
				case approve && current.State == string(common.ApprovalStateApproved):
					approval, err = current, nil
				case !approve && current.State == string(common.ApprovalStateRejected):
					approval, err = current, nil
				}
			}
		}
		return ApprovalResolvedMsg{WorkspaceID: wsID, Approval: approval, Error: err}
	}
}

func approvalResolutionPayload(rawInput string) workorcheventpayload.Payload {
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		return workorcheventpayload.New(workorcheventpayload.StringField("source", "tui"))
	}
	return workorcheventpayload.New(
		workorcheventpayload.StringField("rawInput", rawInput),
		workorcheventpayload.StringField("source", "tui"),
	)
}
