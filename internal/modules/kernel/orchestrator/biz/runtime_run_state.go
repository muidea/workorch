package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) CancelRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	return o.closeRun(ctx, runID, common.RunStateCancelled, "")
}

func (o *RuntimeOrchestrator) SupersedeRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	return o.closeRun(ctx, runID, common.RunStateSuperseded, "")
}

func (o *RuntimeOrchestrator) FailRun(ctx context.Context, runID common.RunID, reason string) error {
	if runID == "" || reason == "" {
		return common.ErrInvalidArgument
	}
	return o.closeRun(ctx, runID, common.RunStateFailed, reason)
}

func (o *RuntimeOrchestrator) failLoadedRun(ctx context.Context, run *runModel.Run, reason string) error {
	if reason == "" {
		return common.ErrInvalidArgument
	}
	return o.closeLoadedRun(ctx, run, common.RunStateFailed, reason)
}

func (o *RuntimeOrchestrator) supersedeLoadedRun(ctx context.Context, run *runModel.Run) error {
	return o.closeLoadedRun(ctx, run, common.RunStateSuperseded, "")
}

func (o *RuntimeOrchestrator) replanLoadedRun(ctx context.Context, run *runModel.Run, ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) error {
	if o == nil || run == nil || run.ID == "" || o.runs == nil {
		return common.ErrInvalidArgument
	}
	successorID, err := o.runStateManager().nextReplanRunID(ctx, run.ID)
	if err != nil {
		return err
	}
	successorContext := cloneStepPayload(run.ExecutionContext)
	if successorContext == nil {
		successorContext = map[string]any{}
	}
	delete(successorContext, "compressedContext")
	if history := mapSliceFromValue(successorContext["governanceHistory"]); len(history) > 0 {
		successorContext["governanceHistory"] = compactGovernanceHistoryForRuntimeContext(history, 6)
	}
	delete(successorContext, "replanContinuation")
	replanContext := map[string]any{
		"predecessorRunId": string(run.ID),
		"reason":           "decision_replan",
	}
	if timeline, digest := o.graphManager().currentRunProcessEvidenceContext(ctx, run, ""); len(timeline) > 0 {
		replanContext[stepInputKeyProcessEvidenceTimeline] = timeline
		if len(digest) > 0 {
			replanContext[stepInputKeyCurrentProgressDigest] = digest
		}
	}
	if ticket != nil {
		replanContext["ticketId"] = string(ticket.ID)
		replanContext["ticketStepId"] = string(ticket.StepID)
		replanContext["ticketReason"] = ticket.Reason
		if len(ticket.Context) > 0 {
			ticketContext := compactFailureContextForRuntimeContext(ticket.Context)
			replanContext["ticketContext"] = ticketContext
			replanContext["failureContext"] = ticketContext
		}
	}
	if resolution != nil {
		replanContext["resolutionId"] = string(resolution.ID)
		replanContext["resolutionType"] = resolution.Type
		replanContext["resolutionPayload"] = compactDecisionPayloadForRuntimeContext(resolution.Payload)
	}
	o.runStateManager().hydrateReplanContextWithInheritedPrimaryFailure(ctx, run, replanContext)
	successorContext["replanContinuation"] = compactReplanContinuationForRuntimeContext(replanContext)
	successor := &runModel.Run{
		ID:               successorID,
		WorkspaceID:      run.WorkspaceID,
		GoalID:           run.GoalID,
		DoDID:            run.DoDID,
		InitialStepKind:  run.InitialStepKind,
		ExecutionContext: successorContext,
		State:            common.RunStateCreated,
		Priority:         run.Priority,
	}
	if successor.InitialStepKind == "" {
		successor.InitialStepKind = initialStepKind(run)
	}
	o.coverageLedger().prepareReplanSuccessor(successor)
	if contract := o.convergenceManager().build(successor, nil, nil, map[string]any{"input": successor.ExecutionContext}, nil); contract.State != "" {
		successor.ExecutionContext["convergenceContract"] = o.convergenceManager().compact(contract)
	}
	successor.ExecutionContext["compressedContext"] = compressedContextForCreatedRuntimeRun(successor)
	if err := o.closeLoadedRun(ctx, run, common.RunStateSuperseded, ""); err != nil {
		return err
	}
	if err := o.runs.Create(ctx, successor); err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunCreated(ctx, successor); err != nil {
		return err
	}
	return o.StartRun(ctx, successor.ID)
}

func compressedContextForCreatedRuntimeRun(run *runModel.Run) map[string]any {
	if run == nil || run.ID == "" || run.WorkspaceID == "" {
		return nil
	}
	language := runDialogLanguage(run)
	return map[string]any{
		"version":          "workorch.compressedContext.v1",
		"mode":             "deterministic_recovery_summary",
		"workspaceId":      string(run.WorkspaceID),
		"runId":            string(run.ID),
		"summary":          localizedRuntimeCreatedRunSummary(language),
		"currentMilestone": "execution_steps",
		"milestones": []map[string]any{
			{"id": "run_created", "title": localizedRuntimeMilestoneTitle(language, "run_created"), "state": "completed", "evidenceRefs": []string{string(run.ID)}},
			{"id": "execution_steps", "title": localizedRuntimeMilestoneTitle(language, "execution_steps"), "state": "pending"},
			{"id": "governance", "title": localizedRuntimeMilestoneTitle(language, "governance"), "state": "pending"},
			{"id": "validation", "title": localizedRuntimeMilestoneTitle(language, "validation"), "state": "pending"},
			{"id": "formal_result", "title": localizedRuntimeMilestoneTitle(language, "formal_result"), "state": "pending"},
		},
		"recovery": map[string]any{
			"mode":       "continue",
			"nextAction": localizedRuntimeRecoveryNextAction(language),
			"runId":      string(run.ID),
		},
		"sourceSectionRefs":  []string{"workspace", "run", "task_progress", "recovery"},
		"sourceEvidenceRefs": []string{string(run.ID)},
		"retentionPolicy":    localizedRuntimeRetentionPolicy(language),
	}
}

func localizedRuntimeCreatedRunSummary(language string) string {
	if prefersChinese(language) {
		return "运行（Run）已创建，尚无执行证据"
	}
	return "run created with no execution evidence yet"
}

func localizedRuntimeMilestoneTitle(language string, key string) string {
	if prefersChinese(language) {
		switch key {
		case "run_created":
			return "运行（Run）已创建"
		case "execution_steps":
			return "执行步骤"
		case "governance":
			return "决策与审批治理"
		case "validation":
			return "验证"
		case "formal_result":
			return "结果（Result）/交付物（Artifact）"
		}
	}
	switch key {
	case "run_created":
		return "Run Created"
	case "execution_steps":
		return "Execution Steps"
	case "governance":
		return "Decision and Approval Governance"
	case "validation":
		return "Validation"
	case "formal_result":
		return "Result / Artifact"
	default:
		return key
	}
}

func localizedRuntimeRecoveryNextAction(language string) string {
	if prefersChinese(language) {
		return "继续执行_run"
	}
	return "continue_run"
}

func localizedRuntimeRetentionPolicy(language string) string {
	if prefersChinese(language) {
		return "保留恢复点、阻塞项、里程碑、最新证据摘要及结果（Result）/交付物（Artifact）引用；需要时按源引用重新加载完整证据"
	}
	return "Retain recovery checkpoints, blockers, milestones, latest evidence summary, and Result/Artifact refs; reload full evidence by source refs when needed."
}

func (o *RuntimeOrchestrator) closeRun(ctx context.Context, runID common.RunID, state common.RunState, reason string) error {
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	return o.closeLoadedRun(ctx, r, state, reason)
}

func (o *RuntimeOrchestrator) closeLoadedRun(ctx context.Context, r *runModel.Run, state common.RunState, reason string) error {
	if r == nil || r.ID == "" {
		return common.ErrInvalidArgument
	}
	if !canCloseRunAs(r.State, state) {
		return common.ErrConflict
	}
	r.State = state
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	r.FailureReason = reason
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	if err := o.eventPublisher().publishRunStateChanged(ctx, r); err != nil {
		return err
	}

	return o.releaseWorkspaceForTerminalRun(ctx, r, workspaceStateForTerminalRun(state))
}

func mergeRunExecutionContextPatch(run *runModel.Run, patch map[string]any) {
	if run == nil || len(patch) == 0 {
		return
	}
	if run.ExecutionContext == nil {
		run.ExecutionContext = map[string]any{}
	}
	for key, value := range patch {
		if key == "" {
			continue
		}
		run.ExecutionContext[key] = value
	}
	if goalID, _ := patch["goalId"].(string); strings.TrimSpace(goalID) != "" {
		run.GoalID = common.GoalID(strings.TrimSpace(goalID))
	}
	if dodID, _ := patch["doDId"].(string); strings.TrimSpace(dodID) != "" {
		run.DoDID = common.DoDID(strings.TrimSpace(dodID))
	}
}

func (o *RuntimeOrchestrator) releaseWorkspaceForTerminalRun(ctx context.Context, run *runModel.Run, state common.WorkspaceState) error {
	if run == nil {
		return common.ErrInvalidArgument
	}
	ws, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil {
		return err
	}
	canRelease, err := o.runStateManager().canTerminalRunReleaseWorkspace(ctx, ws, run)
	if err != nil {
		return err
	}
	if !canRelease {
		return nil
	}
	ws.ActiveRunID = ""
	ws.State = state
	if err := o.workspaces.Update(ctx, ws); err != nil {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, ws, run.ID)
}

func (o *RuntimeOrchestrator) reconcileWorkspaceActiveRun(ctx context.Context, workspaceID common.WorkspaceID) error {
	if o == nil || workspaceID == "" {
		return nil
	}
	ws, err := o.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	if ws.ActiveRunID == "" {
		return nil
	}
	activeRun, err := o.runs.Get(ctx, ws.ActiveRunID)
	if err != nil {
		if !errors.Is(err, common.ErrNotFound) {
			return err
		}
		ws.ActiveRunID = ""
		if isActiveWorkspaceState(ws.State) {
			ws.State = common.WorkspaceStateActive
		}
		if err := o.workspaces.Update(ctx, ws); err != nil {
			return err
		}
		return o.eventPublisher().publishWorkspaceStateChanged(ctx, ws, "")
	}
	if activeRun.WorkspaceID != ws.ID || !isTerminalRunState(activeRun.State) {
		return nil
	}
	ws.ActiveRunID = ""
	ws.State = workspaceStateForTerminalRun(activeRun.State)
	if err := o.workspaces.Update(ctx, ws); err != nil {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, ws, activeRun.ID)
}

func (o *RuntimeOrchestrator) syncWorkspaceForActiveRun(ctx context.Context, run *runModel.Run, state common.WorkspaceState) error {
	if run == nil {
		return common.ErrInvalidArgument
	}
	ws, err := o.runStateManager().syncWorkspaceForActiveRunState(ctx, run, state, nil)
	if err != nil {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, ws, run.ID)
}

func workspaceStateForTerminalRun(state common.RunState) common.WorkspaceState {
	switch state {
	case common.RunStateCompleted:
		return common.WorkspaceStateCompleted
	case common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return common.WorkspaceStateFailed
	default:
		return common.WorkspaceStateActive
	}
}

func isTerminalRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func runParallel(tasks ...func() error) error {
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var ret error
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		if ret == nil {
			ret = err
		}
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		wg.Add(1)
		go func(task func() error) {
			defer wg.Done()
			recordErr(task())
		}(task)
	}
	wg.Wait()
	return ret
}

func canCloseRunAs(current common.RunState, target common.RunState) bool {
	switch target {
	case common.RunStateFailed:
		switch current {
		case common.RunStateCreated, common.RunStateRunning, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStateValidating:
			return true
		}
	case common.RunStateCancelled:
		switch current {
		case common.RunStateCreated, common.RunStateRunning, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStateValidating:
			return true
		}
	case common.RunStateSuperseded:
		switch current {
		case common.RunStateRunning, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStateValidating:
			return true
		}
	}
	return false
}

func isActiveWorkspaceState(state common.WorkspaceState) bool {
	switch state {
	case common.WorkspaceStateRunning, common.WorkspaceStateWaitingDecision, common.WorkspaceStateBlocked, common.WorkspaceStateValidating:
		return true
	default:
		return false
	}
}
