package appservice

import (
	"context"
	"strings"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func readWorkspaceSnapshot(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID, cachedRunSnapshot *RunSnapshot) (WorkspaceSnapshot, error) {
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	var item *workspacemodel.Workspace
	if cachedRunSnapshot != nil && cachedRunSnapshot.Workspace.ID == string(workspaceID) {
		item = workspaceModelFromView(cachedRunSnapshot.Workspace)
	}

	var runItems []*runmodel.Run
	var goalItems []*workspacemodel.Goal
	var dodItems []*workspacemodel.DoD
	var resultItems []*resultmodel.Result
	var eventRecordItems []*eventstreammodel.EventRecord
	var llmUsageEventRecordItems []*eventstreammodel.EventRecord
	var artifactItems []*resultmodel.Artifact
	var ticketItems []*decisionmodel.Ticket
	var approvalItems []*approvalmodel.Approval
	var patchItems []*runmodel.Patch
	var artifactContinuationItems []*orchestratormodel.ArtifactContinuation
	tasks := []func() error{
		func() error {
			var err error
			runItems, err = deps.runs.ListByWorkspaceID(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			resultItems, err = deps.results.ListByWorkspace(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			eventRecordItems, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID, Limit: readModelEventRecordLimit})
			return err
		},
		func() error {
			var err error
			llmUsageEventRecordItems, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID, Topic: events.TopicRunLLMUsageRecorded, Limit: readModelLLMUsageRecordLimit})
			return err
		},
		func() error {
			var err error
			artifactItems, err = deps.artifacts.ListByWorkspace(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			ticketItems, err = deps.tickets.ListByWorkspaceID(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			approvalItems, err = deps.approvals.ListByWorkspaceID(ctx, workspaceID)
			return err
		},
	}
	if deps.goals != nil {
		tasks = append(tasks, func() error {
			var err error
			goalItems, err = deps.goals.ListByWorkspaceID(ctx, workspaceID)
			return err
		})
	}
	if deps.dods != nil {
		tasks = append(tasks, func() error {
			var err error
			dodItems, err = deps.dods.ListByWorkspaceID(ctx, workspaceID)
			return err
		})
	}
	if item == nil {
		tasks = append(tasks, func() error {
			var err error
			item, err = deps.workspaces.Get(ctx, workspaceID)
			return err
		})
	}
	err := runReadModelTasks(tasks...)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	runs := make([]Run, 0, len(runItems))
	projectionRun := selectWorkspaceProjectionRun(item, runItems, resultItems, eventRecordItems)
	for _, runItem := range runItems {
		if runItem.WorkspaceID != workspaceID {
			continue
		}
		runs = append(runs, mapRun(runItem))
		if deps.artifactContinuations != nil {
			items, err := deps.artifactContinuations.ListByRun(ctx, runItem.ID)
			if err != nil {
				return WorkspaceSnapshot{}, err
			}
			artifactContinuationItems = append(artifactContinuationItems, items...)
		}
		if deps.patches != nil {
			items, err := deps.patches.ListByRun(ctx, runItem.ID)
			if err != nil {
				return WorkspaceSnapshot{}, err
			}
			patchItems = append(patchItems, items...)
		}
	}
	decisions := make([]Decision, 0, len(ticketItems))
	resolutions, err := resolutionsForTickets(ctx, deps.resolutions, ticketItems)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	for _, ticket := range ticketItems {
		if ticket.WorkspaceID != workspaceID {
			continue
		}
		summary, err := latestAutoGovernanceSummary(ctx, deps.autoGovernanceTraces, ticket.WorkspaceID, ticket.RunID, "ticket", string(ticket.ID))
		if err != nil {
			return WorkspaceSnapshot{}, err
		}
		decisions = append(decisions, mapDecision(ticket, resolutions[ticket.ID], summary))
	}

	approvals := make([]Approval, 0, len(approvalItems))
	for _, approval := range approvalItems {
		if approval.WorkspaceID != workspaceID {
			continue
		}
		summary, err := latestAutoGovernanceSummary(ctx, deps.autoGovernanceTraces, approval.WorkspaceID, approval.RunID, "approval", string(approval.ID))
		if err != nil {
			return WorkspaceSnapshot{}, err
		}
		approvals = append(approvals, mapApproval(approval, summary))
	}

	ret := WorkspaceSnapshot{
		Workspace:             mapWorkspaceWithRunProjection(item, projectionRun),
		Goals:                 mapGoals(goalItems),
		DoDs:                  mapDoDs(dodItems),
		Runs:                  runs,
		Results:               mapResults(resultItems),
		Artifacts:             mapArtifacts(artifactItems),
		ArtifactContinuations: mapArtifactContinuations(artifactContinuationItems),
		Patches:               mapPatches(patchItems),
		EventRecords:          mapProjectedEventRecords(eventRecordItems),
		Decisions:             decisions,
		Approvals:             approvals,
	}
	ret.LLMUsageTraces = mapLLMUsageTracesFromEventRecords(mapEventRecords(llmUsageEventRecordItems), "")
	ret.LLMUsageSummary = summarizeLLMUsage(ret.LLMUsageTraces)
	ret.TaskProgress = mapWorkspaceTaskProgress(ret)
	if projectionRun != nil {
		var runSnapshot RunSnapshot
		if cachedRunSnapshot != nil &&
			cachedRunSnapshot.Run.ID == string(projectionRun.ID) &&
			cachedRunSnapshot.Run.WorkspaceID == string(workspaceID) {
			runSnapshot = *cachedRunSnapshot
		} else {
			var err error
			runSnapshot, err = readRunSnapshot(ctx, deps, projectionRun.ID)
			if err != nil {
				return WorkspaceSnapshot{}, err
			}
		}
		run := runSnapshot.Run
		step := runSnapshot.Step
		ret.Run = &run
		ret.Step = step
		if result := primaryTaskProgressResult(runScopedResults(runSnapshot)); result != nil {
			selected := *result
			ret.Result = &selected
		}
		ret.TaskProgress = runSnapshot.TaskProgress
		ret.CoverageSummary = coverageSummaryFromRunSnapshot(runSnapshot)
		ret.TaskDecompositions = runSnapshot.TaskDecompositions
		ret.ExecutionSections = runSnapshot.ExecutionSections
		ret.MemoryRecallTraces = runSnapshot.MemoryRecallTraces
		ret.Patches = mergeWorkspaceSnapshotPatches(ret.Patches, runSnapshot.Patches)
		ret.ArtifactContinuations = mergeWorkspaceSnapshotArtifactContinuations(ret.ArtifactContinuations, runSnapshot.ArtifactContinuations)
		ret.WebSources = runSnapshot.WebSources
		ret.LLMContextBudgets = runSnapshot.LLMContextBudgets
		ret.LLMUsageTraces = mergeWorkspaceLLMUsageTraces(ret.LLMUsageTraces, runSnapshot.LLMUsageTraces)
		ret.LLMUsageSummary = summarizeLLMUsage(ret.LLMUsageTraces)
	}
	ret.Outcome = mapWorkspaceOutcome(ret)
	return ret, nil
}

func workspaceModelFromView(item Workspace) *workspacemodel.Workspace {
	return &workspacemodel.Workspace{
		ID:                   common.WorkspaceID(item.ID),
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		State:                common.WorkspaceState(item.State),
		ActiveRunID:          common.RunID(item.ActiveRunID),
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultSoulID:        common.SoulID(item.DefaultSoulID),
		SoulBindings:         workspaceSoulBindingsFromView(item.SoulBindings),
	}
}

func workspaceSoulBindingsFromView(input map[string]string) map[string]common.SoulID {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		ret[key] = common.SoulID(value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func readWorkspaceTaskProgress(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) (TaskProgress, error) {
	snapshot, err := readWorkspaceSnapshot(ctx, deps, workspaceID, nil)
	if err != nil {
		return TaskProgress{}, err
	}
	if snapshot.TaskProgress.WorkspaceID != "" {
		return snapshot.TaskProgress, nil
	}
	return mapWorkspaceTaskProgress(snapshot), nil
}

func readWorkspaceContextBundle(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) (ContextBundle, error) {
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	snapshot, err := readWorkspaceSnapshot(ctx, deps, workspaceID, nil)
	if err != nil {
		return ContextBundle{}, err
	}
	if snapshot.TaskProgress.RunID != "" {
		runSnapshot, err := readRunSnapshot(ctx, deps, common.RunID(snapshot.TaskProgress.RunID))
		if err != nil {
			return ContextBundle{}, err
		}
		return mapRunContextBundle(runSnapshot), nil
	}
	return mapWorkspaceContextBundle(snapshot), nil
}

func readWorkspaceTaskDecompositions(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) ([]TaskDecomposition, error) {
	snapshot, err := readWorkspaceSnapshot(ctx, deps, workspaceID, nil)
	if err != nil {
		return nil, err
	}
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions, nil
	}
	if snapshot.TaskProgress.RunID == "" {
		return nil, nil
	}
	runSnapshot, err := readRunSnapshot(ctx, deps, common.RunID(snapshot.TaskProgress.RunID))
	if err != nil {
		return nil, err
	}
	return runSnapshot.TaskDecompositions, nil
}

func selectWorkspaceProjectionRun(workspace *workspacemodel.Workspace, runs []*runmodel.Run, results []*resultmodel.Result, eventRecords []*eventstreammodel.EventRecord) *runmodel.Run {
	if workspace == nil || len(runs) == 0 {
		return nil
	}
	if workspace.State == common.WorkspaceStateActive && strings.TrimSpace(string(workspace.ActiveRunID)) == "" {
		return nil
	}
	var latest *runmodel.Run
	var active *runmodel.Run
	runsByID := make(map[common.RunID]*runmodel.Run, len(runs))
	for _, item := range runs {
		if item == nil || item.WorkspaceID != workspace.ID {
			continue
		}
		runsByID[item.ID] = item
		if workspace.ActiveRunID != "" && item.ID == workspace.ActiveRunID {
			active = item
		}
		if latest == nil || string(item.ID) > string(latest.ID) {
			latest = item
		}
	}
	if active != nil && !isTerminalProjectionRunState(active.State) {
		return active
	}
	latestEvent := selectLatestRunEvent(workspace.ID, runsByID, eventRecords)
	if isTerminalProjectionState(string(workspace.State)) {
		if result, resultRun := selectTerminalResultRun(workspace.ID, runsByID, results); resultRun != nil &&
			(latestEvent == nil || resultModelNotBeforeEvent(result, latestEvent)) {
			return resultRun
		}
	}
	if latestEvent != nil {
		return runsByID[latestEvent.RunID]
	}
	if resultRun := selectAcceptedResultRun(workspace.ID, runsByID, results); resultRun != nil {
		return resultRun
	}
	if active != nil {
		return active
	}
	return latest
}

func selectTerminalResultRun(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, results []*resultmodel.Result) (*resultmodel.Result, *runmodel.Run) {
	var selected *resultmodel.Result
	for _, item := range results {
		if item == nil || item.WorkspaceID != workspaceID || item.RunID == "" {
			continue
		}
		switch item.State {
		case common.ResultStateAccepted, common.ResultStateRejected:
		default:
			continue
		}
		run := runs[item.RunID]
		if run == nil || !isTerminalProjectionRunState(run.State) {
			continue
		}
		if selected == nil || resultProjectionTime(item) >= resultProjectionTime(selected) {
			selected = item
		}
	}
	if selected == nil {
		return nil, nil
	}
	return selected, runs[selected.RunID]
}

func selectLatestRunByEvent(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, eventRecords []*eventstreammodel.EventRecord) *runmodel.Run {
	selected := selectLatestRunEvent(workspaceID, runs, eventRecords)
	if selected == nil {
		return nil
	}
	return runs[selected.RunID]
}

func selectLatestRunEvent(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, eventRecords []*eventstreammodel.EventRecord) *eventstreammodel.EventRecord {
	var selected *eventstreammodel.EventRecord
	for _, item := range eventRecords {
		if item == nil || item.WorkspaceID != workspaceID || item.RunID == "" {
			continue
		}
		if _, ok := runs[item.RunID]; !ok {
			continue
		}
		switch item.Topic {
		case events.TopicRunCreated, events.TopicRunStateChanged:
		default:
			continue
		}
		if selected == nil || item.OccurredAt >= selected.OccurredAt {
			selected = item
		}
	}
	if selected == nil {
		return nil
	}
	return selected
}

func mapWorkspaceWithRunProjection(workspace *workspacemodel.Workspace, run *runmodel.Run) Workspace {
	ret := mapWorkspace(workspace)
	if run == nil {
		return ret
	}
	if run.State == common.RunStateCreated {
		return ret
	}
	ret.State = string(workspaceProjectionState(run.State))
	if isTerminalProjectionRunState(run.State) {
		ret.ActiveRunID = ""
		ret.BlockingTicketID = ""
		ret.BlockingApprovalID = ""
	} else {
		ret.ActiveRunID = string(run.ID)
		ret.BlockingTicketID = string(run.BlockingTicketID)
		ret.BlockingApprovalID = string(run.BlockingApprovalID)
	}
	return ret
}

func selectAcceptedResultRun(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, results []*resultmodel.Result) *runmodel.Run {
	var selected *resultmodel.Result
	for _, item := range results {
		if item == nil || item.WorkspaceID != workspaceID || item.State != common.ResultStateAccepted || item.RunID == "" {
			continue
		}
		run := runs[item.RunID]
		if run == nil || run.State != common.RunStateCompleted {
			continue
		}
		if selected == nil || resultProjectionTime(item) >= resultProjectionTime(selected) {
			selected = item
		}
	}
	if selected == nil {
		return nil
	}
	return runs[selected.RunID]
}

func resultProjectionTime(item *resultmodel.Result) string {
	if item == nil {
		return ""
	}
	if item.UpdatedAt != "" {
		return item.UpdatedAt
	}
	return item.CreatedAt
}

func resultModelNotBeforeEvent(result *resultmodel.Result, event *eventstreammodel.EventRecord) bool {
	if result == nil || event == nil {
		return result != nil
	}
	resultAt := resultProjectionTime(result)
	eventAt := strings.TrimSpace(event.OccurredAt)
	if resultAt == "" || eventAt == "" {
		return resultAt >= eventAt
	}
	resultTime, resultErr := time.Parse(time.RFC3339Nano, resultAt)
	eventTime, eventErr := time.Parse(time.RFC3339Nano, eventAt)
	if resultErr == nil && eventErr == nil {
		return !resultTime.Before(eventTime)
	}
	return resultAt >= eventAt
}

func workspaceProjectionState(runState common.RunState) common.WorkspaceState {
	switch runState {
	case common.RunStateCreated:
		return common.WorkspaceStateReady
	case common.RunStateRunning:
		return common.WorkspaceStateRunning
	case common.RunStateBlockedOnDecision:
		return common.WorkspaceStateWaitingDecision
	case common.RunStateBlockedOnApproval:
		return common.WorkspaceStateBlocked
	case common.RunStateValidating:
		return common.WorkspaceStateValidating
	case common.RunStateCompleted:
		return common.WorkspaceStateCompleted
	case common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return common.WorkspaceStateFailed
	default:
		return common.WorkspaceStateActive
	}
}

func isTerminalProjectionRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}
