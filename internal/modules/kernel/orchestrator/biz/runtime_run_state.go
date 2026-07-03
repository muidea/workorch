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
	resetConvergenceBudgetForReplanSuccessor(successorContext)
	successorContext["replanBudgetResetPending"] = true
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
		ID:                 successorID,
		WorkspaceID:        run.WorkspaceID,
		GoalID:             run.GoalID,
		DoDID:              run.DoDID,
		DefinitionRevision: run.DefinitionRevision,
		InitialStepKind:    run.InitialStepKind,
		ExecutionContext:   successorContext,
		State:              common.RunStateCreated,
		Priority:           run.Priority,
	}
	if successor.InitialStepKind == "" {
		successor.InitialStepKind = initialStepKind(run)
	}
	o.coverageLedger().prepareReplanSuccessor(successor, ctx)
	if contract := o.convergenceManager().build(successor, nil, nil, map[string]any{"input": successor.ExecutionContext}, nil); convergenceContractShouldPersist(contract) {
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
			"mode":       "resume",
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
		return "恢复执行_run"
	}
	return "resume_run"
}

func localizedRuntimeRetentionPolicy(language string) string {
	if prefersChinese(language) {
		return "保留恢复点、阻塞项、里程碑、最新证据摘要及结果（Result）/交付物（Artifact）引用；需要时按源引用重新加载完整证据"
	}
	return "Retain recovery checkpoints, blockers, milestones, latest evidence summary, and Result/Artifact refs; reload full evidence by source refs when needed."
}

func (o *RuntimeOrchestrator) closeRun(ctx context.Context, runID common.RunID, state common.RunState, reason string) error {
	r, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return err
	}
	return o.closeLoadedRun(ctx, r, state, reason)
}

func (o *RuntimeOrchestrator) closeLoadedRun(ctx context.Context, r *runModel.Run, state common.RunState, reason string) error {
	if r == nil || r.ID == "" {
		return common.ErrInvalidArgument
	}
	if !canCloseRunAsUnsuccessfulTerminal(r.State, state) {
		return common.ErrConflict
	}
	_ = o.finalizeManagedValidationResourcesForRun(ctx, r, "released", "run_terminal")
	if err := o.dismissOpenDecisionBlocker(ctx, r); err != nil {
		return err
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
	workspace, changed, err := o.runStateManager().releaseWorkspaceForTerminalRun(ctx, r, workspaceStateForTerminalRun(state))
	if err != nil || !changed {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, r.ID)
}

func (o *RuntimeOrchestrator) dismissOpenDecisionBlocker(ctx context.Context, r *runModel.Run) error {
	if o == nil || o.tickets == nil || r == nil || r.BlockingTicketID == "" {
		return nil
	}
	ticket, err := o.tickets.Get(ctx, r.BlockingTicketID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil
		}
		return err
	}
	if ticket.RunID != r.ID || ticket.State != common.DecisionTicketStateOpen {
		return nil
	}
	ticket.State = common.DecisionTicketStateDismissed
	ticket.UpdatedAt = runtimeEvidenceTimestamp()
	return o.tickets.Update(ctx, ticket)
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

func (o *RuntimeOrchestrator) reconcileWorkspaceActiveRun(ctx context.Context, workspaceID common.WorkspaceID) error {
	if o == nil || workspaceID == "" {
		return nil
	}
	ws, activeRunID, changed, err := o.runStateManager().reconcileWorkspaceActiveRun(ctx, workspaceID)
	if err != nil || !changed {
		return err
	}
	return o.eventPublisher().publishWorkspaceStateChanged(ctx, ws, activeRunID)
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

func isActiveRunProgressState(state common.RunState) bool {
	switch state {
	case common.RunStateCreated, common.RunStateRunning, common.RunStateValidating:
		return true
	default:
		return false
	}
}

func (o *RuntimeOrchestrator) persistRunProgressUpdate(ctx context.Context, run *runModel.Run) error {
	if o == nil || o.runs == nil || run == nil {
		return common.ErrInvalidArgument
	}
	next := run
	readCtx := ctx
	if run.WorkspaceID != "" {
		readCtx = common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
	}
	current, err := o.runtimeEventRun(readCtx, run.ID)
	if err == nil && current != nil {
		next = mergeRunProgressUpdate(current, run)
	} else if err != nil && !errors.Is(err, common.ErrNotFound) {
		return err
	}
	normalizeRunConvergenceOwnerState(next)
	return o.runs.Update(ctx, next)
}

func mergeRunProgressUpdate(current *runModel.Run, progress *runModel.Run) *runModel.Run {
	if current == nil {
		return progress
	}
	if progress == nil {
		return current
	}
	next := *current
	if progress.ExecutionContext != nil {
		next.ExecutionContext = cloneStepPayload(current.ExecutionContext)
		if next.ExecutionContext == nil {
			next.ExecutionContext = map[string]any{}
		}
		for _, key := range managedRunProgressExecutionContextKeys() {
			if _, ok := progress.ExecutionContext[key]; !ok {
				delete(next.ExecutionContext, key)
			}
		}
		for key, value := range progress.ExecutionContext {
			if key == "" {
				continue
			}
			next.ExecutionContext[key] = cloneStepInputValue(value)
		}
	}
	return &next
}

func managedRunProgressExecutionContextKeys() []string {
	return []string{
		coverageWorkItemsKey,
		coverageWorkItemContextKey,
		gapBaselineKey,
		convergenceAuthorityKey,
		coverageFocusKey,
		convergenceBudgetKey,
		acceptanceSlicesKey,
		activeSliceIDKey,
		"acceptance",
		"currentActionContract",
		"currentTargetSnapshots",
		"replanBudgetResetPending",
		"repairActionPackage",
		"requiredOutput",
		"targetEvidence",
		"targetSnapshots",
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

func canCloseRunAsUnsuccessfulTerminal(current common.RunState, target common.RunState) bool {
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
		case common.RunStateCreated, common.RunStateRunning, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStateValidating:
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
