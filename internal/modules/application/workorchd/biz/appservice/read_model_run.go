package appservice

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func readRunSnapshot(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunSnapshot, error) {
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	var workspace *workspacemodel.Workspace
	var steps []*runmodel.Step
	var toolCalls []*runmodel.ToolCall
	var patches []*runmodel.Patch
	var validations []*runmodel.ValidationResult
	var checkpoints []*runmodel.RuntimeCheckpoint
	var results []*resultmodel.Result
	var artifacts []*resultmodel.Artifact
	var artifactContinuations []*orchestratormodel.ArtifactContinuation
	var eventRecords []*eventstreammodel.EventRecord
	var llmUsageEventRecords []*eventstreammodel.EventRecord
	var memoryRecallTraces []*memorymodel.MemoryRecallTrace
	var decisions []Decision
	var selectedTicket *decisionmodel.Ticket
	var selectedResolution *decisionmodel.Resolution
	var approvals []Approval
	var selectedApproval *approvalmodel.Approval
	var mcpCapabilityItems []*mcpmodel.Capability
	tasks := []func() error{
		func() error {
			var err error
			workspace, err = deps.workspaces.Get(ctx, item.WorkspaceID)
			return err
		},
		func() error {
			var err error
			steps, err = deps.steps.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			toolCalls, err = deps.toolCalls.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			patches, err = deps.patches.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			validations, err = deps.validations.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			checkpoints, err = deps.checkpoints.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			results, err = deps.results.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			artifacts, err = deps.artifacts.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			artifactContinuations, err = deps.artifactContinuations.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			eventRecords, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{RunID: item.ID, Limit: readModelEventRecordLimit})
			return err
		},
		func() error {
			var err error
			llmUsageEventRecords, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{RunID: item.ID, Topic: events.TopicRunLLMUsageRecorded, Limit: readModelLLMUsageRecordLimit})
			return err
		},
		func() error {
			var err error
			if deps.memoryRecalls != nil {
				memoryRecallTraces, err = deps.memoryRecalls.List(ctx, repository.MemoryRecallTraceQuery{WorkspaceID: item.WorkspaceID, RunID: item.ID})
			}
			return err
		},
		func() error {
			var err error
			decisions, selectedTicket, selectedResolution, err = decisionsForRun(ctx, deps, item)
			return err
		},
		func() error {
			var err error
			approvals, selectedApproval, err = approvalsForRun(ctx, deps, item)
			return err
		},
	}
	if deps.mcpCapabilities != nil {
		tasks = append(tasks, func() error {
			var err error
			mcpCapabilityItems, err = deps.mcpCapabilities.List(ctx)
			return err
		})
	}
	if err := runReadModelTasks(tasks...); err != nil {
		return RunSnapshot{}, err
	}

	observedItems := observed.BuildWorkorchSnapshot(workspace, item, steps, selectedTicket, selectedResolution, selectedApproval, deps.published)
	stepViews := presenter.MapSteps(steps)
	currentStep := currentRunSnapshotStep(
		stepViews,
		decisionStepID(selectedTicket),
		approvalStepID(selectedApproval),
		mapRuntimeCheckpoints(checkpoints),
	)

	ret := RunSnapshot{
		Workspace:               mapWorkspaceWithRunProjection(workspace, item),
		Run:                     mapRun(item),
		Step:                    currentStep,
		Steps:                   stepViews,
		ToolCalls:               mapToolCalls(toolCalls),
		Patches:                 mapPatches(patches),
		GitOperations:           mapGitOperationTraces(stepViews),
		WebSources:              mapWebSourceTraces(stepViews),
		LLMContextBudgets:       mapLLMContextBudgetTraces(stepViews),
		LLMUsageTraces:          mapLLMUsageTracesFromEventRecords(mapEventRecords(llmUsageEventRecords), string(item.ID)),
		ValidationResults:       mapValidationResults(validations),
		RuntimeCheckpoints:      mapRuntimeCheckpoints(checkpoints),
		Results:                 mapResults(results),
		Artifacts:               mapArtifacts(artifacts),
		ArtifactContinuations:   mapArtifactContinuations(artifactContinuations),
		CoverageWorkItems:       readModelMapSliceValue(item.ExecutionContext["coverageWorkItems"]),
		CoverageWorkItemContext: cloneReadModelMap(readModelMapValue(item.ExecutionContext["coverageWorkItemContext"])),
		MemoryRecallTraces:      mapMemoryRecallTraces(memoryRecallTraces),
		EventRecords:            mapProjectedEventRecords(eventRecords),
		Decisions:               decisions,
		Approvals:               approvals,
		Observed:                presenter.MapObserved(observedItems),
	}
	ret.LLMUsageSummary = summarizeLLMUsage(ret.LLMUsageTraces)
	ret.Outcome = mapRunOutcome(ret)
	capabilityCatalog := runtimeCapabilityCatalogFromItems(ret.Run.ExecutionContext, ret.ToolCalls, mcpCapabilityItems)
	ret.SelectedCapabilities = selectedRuntimeCapabilities(ret.Run.ExecutionContext, capabilityCatalog)
	ret.UsedCapabilities = usedRuntimeCapabilities(ret, capabilityCatalog)
	ret.TaskProgress = mapTaskProgress(ret)
	ret.RepairLoop = ret.TaskProgress.RepairLoop
	ret.TaskDecompositions = mapTaskDecompositions(ret)
	ret.TaskPlans = mapTaskPlans(ret)
	ret.ClosureTraces = mapClosureTraces(ret)
	ret.DoDEvidence = mapDoDEvidence(ret)
	ret.ResultQuality = mapResultQuality(ret)
	ret.ExecutionSections = mapExecutionSections(ret)
	return ret, nil
}

func readRunRecoverySnapshot(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunSnapshot, error) {
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	var workspace *workspacemodel.Workspace
	var steps []*runmodel.Step
	var toolCalls []*runmodel.ToolCall
	var patches []*runmodel.Patch
	var validations []*runmodel.ValidationResult
	var checkpoints []*runmodel.RuntimeCheckpoint
	var results []*resultmodel.Result
	var artifacts []*resultmodel.Artifact
	var artifactContinuations []*orchestratormodel.ArtifactContinuation
	var eventRecords []*eventstreammodel.EventRecord
	var llmUsageEventRecords []*eventstreammodel.EventRecord
	var memoryRecallTraces []*memorymodel.MemoryRecallTrace
	var decisions []Decision
	var approvals []Approval
	err = runReadModelTasks(
		func() error {
			var err error
			workspace, err = deps.workspaces.Get(ctx, item.WorkspaceID)
			return err
		},
		func() error {
			var err error
			steps, err = deps.steps.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			toolCalls, err = deps.toolCalls.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			patches, err = deps.patches.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			validations, err = deps.validations.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			checkpoints, err = deps.checkpoints.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			results, err = deps.results.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			artifacts, err = deps.artifacts.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			artifactContinuations, err = deps.artifactContinuations.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			eventRecords, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{RunID: item.ID, Limit: readModelEventRecordLimit})
			return err
		},
		func() error {
			var err error
			llmUsageEventRecords, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{RunID: item.ID, Topic: events.TopicRunLLMUsageRecorded, Limit: readModelLLMUsageRecordLimit})
			return err
		},
		func() error {
			var err error
			if deps.memoryRecalls != nil {
				memoryRecallTraces, err = deps.memoryRecalls.List(ctx, repository.MemoryRecallTraceQuery{WorkspaceID: item.WorkspaceID, RunID: item.ID})
			}
			return err
		},
		func() error {
			var selectedTicket *decisionmodel.Ticket
			var selectedResolution *decisionmodel.Resolution
			var err error
			decisions, selectedTicket, selectedResolution, err = decisionsForRun(ctx, deps, item)
			_, _ = selectedTicket, selectedResolution
			return err
		},
		func() error {
			var selectedApproval *approvalmodel.Approval
			var err error
			approvals, selectedApproval, err = approvalsForRun(ctx, deps, item)
			_ = selectedApproval
			return err
		},
	)
	if err != nil {
		return RunSnapshot{}, err
	}
	stepViews := presenter.MapSteps(steps)
	currentStep := currentRunSnapshotStep(stepViews, "", "", mapRuntimeCheckpoints(checkpoints))
	ret := RunSnapshot{
		Workspace:             mapWorkspaceWithRunProjection(workspace, item),
		Run:                   mapRun(item),
		Step:                  currentStep,
		Steps:                 stepViews,
		ToolCalls:             mapToolCalls(toolCalls),
		Patches:               mapPatches(patches),
		LLMContextBudgets:     mapLLMContextBudgetTraces(stepViews),
		LLMUsageTraces:        mapLLMUsageTracesFromEventRecords(mapEventRecords(llmUsageEventRecords), string(item.ID)),
		ValidationResults:     mapValidationResults(validations),
		RuntimeCheckpoints:    mapRuntimeCheckpoints(checkpoints),
		Results:               mapResults(results),
		Artifacts:             mapArtifacts(artifacts),
		ArtifactContinuations: mapArtifactContinuations(artifactContinuations),
		MemoryRecallTraces:    mapMemoryRecallTraces(memoryRecallTraces),
		EventRecords:          mapProjectedEventRecords(eventRecords),
		Decisions:             decisions,
		Approvals:             approvals,
	}
	ret.LLMUsageSummary = summarizeLLMUsage(ret.LLMUsageTraces)
	ret.Outcome = mapRunOutcome(ret)
	ret.TaskProgress = mapTaskProgress(ret)
	ret.RepairLoop = ret.TaskProgress.RepairLoop
	ret.TaskPlans = mapTaskPlans(ret)
	ret.DoDEvidence = mapDoDEvidence(ret)
	ret.ResultQuality = mapResultQuality(ret)
	return ret, nil
}

func readRunStatus(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunStatus, error) {
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return RunStatus{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
	var steps []*runmodel.Step
	var checkpoints []*runmodel.RuntimeCheckpoint
	var decisions []*decisionmodel.Ticket
	var approvals []*approvalmodel.Approval
	err = runReadModelTasks(
		func() error {
			var err error
			steps, err = deps.steps.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			if deps.checkpoints == nil {
				return nil
			}
			var err error
			checkpoints, err = deps.checkpoints.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			decisions, err = deps.tickets.ListByRunID(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			approvals, err = deps.approvals.ListByRunID(ctx, item.ID)
			return err
		},
	)
	if err != nil {
		return RunStatus{}, err
	}
	ret := RunStatus{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		State:              string(item.State),
		StepCount:          len(steps),
		DecisionCount:      len(decisions),
		ApprovalCount:      len(approvals),
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		FailureReason:      item.FailureReason,
	}
	if currentStep := currentRunStatusStep(item, steps, checkpoints, decisions, approvals); currentStep != nil {
		ret.CurrentStepID = string(currentStep.ID)
		ret.CurrentStepKind = canonicalReadModelStepKind(currentStep.Kind)
		ret.CurrentStepState = string(currentStep.State)
	}
	return ret, nil
}

func readRunTaskProgress(ctx context.Context, deps readModelDependencies, runID common.RunID) (TaskProgress, error) {
	snapshot, err := readRunSnapshot(ctx, deps, runID)
	if err != nil {
		return TaskProgress{}, err
	}
	return runSnapshotTaskProgress(snapshot), nil
}

func decisionsForRun(ctx context.Context, deps readModelDependencies, run *runmodel.Run) ([]Decision, *decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	items, err := deps.tickets.ListByRunID(ctx, run.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	resolutions, err := resolutionsForTickets(ctx, deps.resolutions, items)
	if err != nil {
		return nil, nil, nil, err
	}

	ret := make([]Decision, 0, len(items))
	var selectedTicket *decisionmodel.Ticket
	var selectedResolution *decisionmodel.Resolution
	for _, item := range items {
		if item.RunID != run.ID {
			continue
		}
		resolution := resolutions[item.ID]
		summary, err := latestAutoGovernanceSummary(ctx, deps.autoGovernanceTraces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
		if err != nil {
			return nil, nil, nil, err
		}
		ret = append(ret, mapDecision(item, resolution, summary))
		if run.BlockingTicketID != "" && item.ID == run.BlockingTicketID {
			selectedTicket = item
			selectedResolution = resolution
			continue
		}
		if selectedTicket == nil || run.BlockingTicketID == "" {
			selectedTicket = item
			selectedResolution = resolution
		}
	}
	return ret, selectedTicket, selectedResolution, nil
}

func resolutionsForTickets(ctx context.Context, resolutions repository.DecisionResolutionRepository, items []*decisionmodel.Ticket) (map[common.TicketID]*decisionmodel.Resolution, error) {
	ret := make(map[common.TicketID]*decisionmodel.Resolution)
	if resolutions == nil || len(items) == 0 {
		return ret, nil
	}
	var mu sync.Mutex
	err := runReadModelTasks(funcsForResolvedTickets(items, func(item *decisionmodel.Ticket) func() error {
		return func() error {
			resolution, err := resolutions.GetByTicketID(ctx, item.ID)
			if errors.Is(err, common.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			mu.Lock()
			ret[item.ID] = resolution
			mu.Unlock()
			return nil
		}
	})...)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func funcsForResolvedTickets(items []*decisionmodel.Ticket, build func(*decisionmodel.Ticket) func() error) []func() error {
	if len(items) == 0 || build == nil {
		return nil
	}
	ret := make([]func() error, 0, len(items))
	for _, item := range items {
		if item == nil || item.State != common.DecisionTicketStateResolved {
			continue
		}
		ret = append(ret, build(item))
	}
	return ret
}

func approvalsForRun(ctx context.Context, deps readModelDependencies, run *runmodel.Run) ([]Approval, *approvalmodel.Approval, error) {
	items, err := deps.approvals.ListByRunID(ctx, run.ID)
	if err != nil {
		return nil, nil, err
	}

	ret := make([]Approval, 0, len(items))
	var selectedApproval *approvalmodel.Approval
	for _, item := range items {
		if item.RunID != run.ID {
			continue
		}
		summary, err := latestAutoGovernanceSummary(ctx, deps.autoGovernanceTraces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
		if err != nil {
			return nil, nil, err
		}
		ret = append(ret, mapApproval(item, summary))
		if run.BlockingApprovalID != "" && item.ID == run.BlockingApprovalID {
			selectedApproval = item
			continue
		}
		if selectedApproval == nil || run.BlockingApprovalID == "" {
			selectedApproval = item
		}
	}
	return ret, selectedApproval, nil
}

func lastStep(steps []*runmodel.Step) *runmodel.Step {
	if len(steps) == 0 {
		return nil
	}
	return steps[len(steps)-1]
}

func currentRunStatusStep(run *runmodel.Run, steps []*runmodel.Step, checkpoints []*runmodel.RuntimeCheckpoint, decisions []*decisionmodel.Ticket, approvals []*approvalmodel.Approval) *runmodel.Step {
	if len(steps) == 0 {
		return nil
	}
	byID := make(map[common.StepID]*runmodel.Step, len(steps))
	for _, step := range steps {
		if step == nil || step.ID == "" {
			continue
		}
		byID[step.ID] = step
	}
	if run != nil && run.BlockingTicketID != "" {
		for _, item := range decisions {
			if item != nil && item.ID == run.BlockingTicketID && item.StepID != "" {
				if step := byID[item.StepID]; step != nil {
					return step
				}
			}
		}
	}
	if run != nil && run.BlockingApprovalID != "" {
		for _, item := range approvals {
			if item != nil && item.ID == run.BlockingApprovalID && item.StepID != "" {
				if step := byID[item.StepID]; step != nil {
					return step
				}
			}
		}
	}
	if checkpoint := latestRuntimeCheckpointModel(checkpoints); checkpoint != nil && checkpoint.CurrentStepID != "" {
		if step := byID[checkpoint.CurrentStepID]; step != nil {
			return step
		}
	}
	if step := latestNonTerminalRunStatusStep(steps); step != nil {
		return step
	}
	if step := terminalRunStatusStep(steps); step != nil {
		return step
	}
	return lastStep(steps)
}

func currentRunSnapshotStep(steps []presenter.StepView, decisionStep string, approvalStep string, checkpoints []RuntimeCheckpoint) *presenter.StepView {
	if decisionStep != "" {
		if step := stepByID(steps, decisionStep); step != nil {
			return step
		}
	}
	if approvalStep != "" {
		if step := stepByID(steps, approvalStep); step != nil {
			return step
		}
	}
	if checkpoint := latestRuntimeCheckpoint(checkpoints); checkpoint != nil {
		if step := stepByID(steps, checkpoint.CurrentStepID); step != nil {
			return step
		}
	}
	if len(steps) == 0 {
		return nil
	}
	if step := latestNonTerminalSnapshotStep(steps); step != nil {
		return step
	}
	if step := terminalSnapshotStep(steps); step != nil {
		return step
	}
	return &steps[len(steps)-1]
}

func latestNonTerminalRunStatusStep(steps []*runmodel.Step) *runmodel.Step {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if step == nil {
			continue
		}
		if step.State == common.StepStateDispatched || step.State == common.StepStatePending {
			return step
		}
	}
	return nil
}

func latestNonTerminalSnapshotStep(steps []presenter.StepView) *presenter.StepView {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		if steps[idx].State == string(common.StepStateDispatched) || steps[idx].State == string(common.StepStatePending) {
			return &steps[idx]
		}
	}
	return nil
}

func decisionStepID(item *decisionmodel.Ticket) string {
	if item == nil {
		return ""
	}
	return string(item.StepID)
}

func approvalStepID(item *approvalmodel.Approval) string {
	if item == nil {
		return ""
	}
	return string(item.StepID)
}

func terminalRunStatusStep(steps []*runmodel.Step) *runmodel.Step {
	referenced := map[common.StepID]bool{}
	for _, step := range steps {
		if previous := readModelPreviousStepID(step); previous != "" {
			referenced[previous] = true
		}
	}
	var terminals []*runmodel.Step
	for _, step := range steps {
		if step == nil || referenced[step.ID] {
			continue
		}
		terminals = append(terminals, step)
	}
	if len(terminals) == 0 {
		return nil
	}
	for _, step := range terminals {
		if step.State == common.StepStateDispatched {
			return step
		}
	}
	return terminals[len(terminals)-1]
}

func terminalSnapshotStep(steps []presenter.StepView) *presenter.StepView {
	referenced := map[string]bool{}
	for _, step := range steps {
		if previous := readModelPreviousStepIDSnapshot(step); previous != "" {
			referenced[previous] = true
		}
	}
	var terminals []*presenter.StepView
	for idx := range steps {
		if steps[idx].ID == "" || referenced[steps[idx].ID] {
			continue
		}
		terminals = append(terminals, &steps[idx])
	}
	if len(terminals) == 0 {
		return nil
	}
	for _, step := range terminals {
		if step.State == string(common.StepStateDispatched) {
			return step
		}
	}
	return terminals[len(terminals)-1]
}

func readModelPreviousStepID(step *runmodel.Step) common.StepID {
	if step == nil || step.Input == nil {
		return ""
	}
	for _, key := range []string{"previousStepId", "repairOfStepId", "failedStepId"} {
		value, _ := step.Input[key].(string)
		value = strings.TrimSpace(value)
		if value != "" && common.StepID(value) != step.ID {
			return common.StepID(value)
		}
	}
	return ""
}

func readModelPreviousStepIDSnapshot(step presenter.StepView) string {
	if step.Input == nil {
		return ""
	}
	value, _ := step.Input["previousStepId"].(string)
	return strings.TrimSpace(value)
}
