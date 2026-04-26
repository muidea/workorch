package appservice

import (
	"context"
	"sort"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type RunSnapshot struct {
	Workspace          Workspace                `json:"workspace"`
	Run                Run                      `json:"run"`
	Step               *presenter.StepView      `json:"step,omitempty"`
	Steps              []presenter.StepView     `json:"steps,omitempty"`
	ToolCalls          []ToolCall               `json:"toolCalls,omitempty"`
	Patches            []Patch                  `json:"patches,omitempty"`
	ValidationResults  []ValidationResult       `json:"validationResults,omitempty"`
	RuntimeCheckpoints []RuntimeCheckpoint      `json:"runtimeCheckpoints,omitempty"`
	Decisions          []Decision               `json:"decisions,omitempty"`
	Approvals          []Approval               `json:"approvals,omitempty"`
	Observed           []presenter.ObservedView `json:"observed,omitempty"`
}

type RunStatus struct {
	ID                 string `json:"id"`
	WorkspaceID        string `json:"workspaceId"`
	State              string `json:"state"`
	CurrentStepID      string `json:"currentStepId,omitempty"`
	CurrentStepKind    string `json:"currentStepKind,omitempty"`
	CurrentStepState   string `json:"currentStepState,omitempty"`
	StepCount          int    `json:"stepCount"`
	DecisionCount      int    `json:"decisionCount"`
	ApprovalCount      int    `json:"approvalCount"`
	BlockingTicketID   string `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string `json:"blockingApprovalId,omitempty"`
	FailureReason      string `json:"failureReason,omitempty"`
}

type RunObservability struct {
	Run              Run                    `json:"run"`
	Progress         RunProgress            `json:"progress"`
	Evidence         RuntimeEvidenceSummary `json:"evidence"`
	LatestCheckpoint *RuntimeCheckpoint     `json:"latestCheckpoint,omitempty"`
	Timeline         []RuntimeTimelineItem  `json:"timeline,omitempty"`
}

type RunProgress struct {
	State            string `json:"state"`
	CurrentStepID    string `json:"currentStepId,omitempty"`
	CurrentStepKind  string `json:"currentStepKind,omitempty"`
	CurrentStepState string `json:"currentStepState,omitempty"`
	StepTotal        int    `json:"stepTotal"`
	StepDispatched   int    `json:"stepDispatched"`
	StepCompleted    int    `json:"stepCompleted"`
	StepFailed       int    `json:"stepFailed"`
	Blocked          bool   `json:"blocked"`
	Terminal         bool   `json:"terminal"`
}

type RuntimeEvidenceSummary struct {
	ToolCallTotal          int `json:"toolCallTotal"`
	ToolCallRunning        int `json:"toolCallRunning"`
	ToolCallCompleted      int `json:"toolCallCompleted"`
	ToolCallFailed         int `json:"toolCallFailed"`
	PatchTotal             int `json:"patchTotal"`
	PatchApplied           int `json:"patchApplied"`
	PatchFailed            int `json:"patchFailed"`
	ValidationTotal        int `json:"validationTotal"`
	ValidationPassed       int `json:"validationPassed"`
	ValidationFailed       int `json:"validationFailed"`
	ValidationWarning      int `json:"validationWarning"`
	RuntimeCheckpointTotal int `json:"runtimeCheckpointTotal"`
	EventTotal             int `json:"eventTotal"`
}

type RuntimeTimelineItem struct {
	Source     string         `json:"source"`
	Type       string         `json:"type"`
	ID         string         `json:"id,omitempty"`
	Topic      string         `json:"topic,omitempty"`
	RunState   string         `json:"runState,omitempty"`
	StepID     string         `json:"stepId,omitempty"`
	StepState  string         `json:"stepState,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	OccurredAt string         `json:"occurredAt,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

type ToolCall struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspaceId"`
	RunID       string         `json:"runId"`
	StepID      string         `json:"stepId,omitempty"`
	ToolName    string         `json:"toolName"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	State       string         `json:"state"`
	StartedAt   string         `json:"startedAt,omitempty"`
	EndedAt     string         `json:"endedAt,omitempty"`
	ExitSummary string         `json:"exitSummary,omitempty"`
	Output      map[string]any `json:"output,omitempty"`
	Error       string         `json:"error,omitempty"`
}

type Patch struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	RunID       string `json:"runId"`
	StepID      string `json:"stepId,omitempty"`
	ToolCallID  string `json:"toolCallId,omitempty"`
	PatchType   string `json:"patchType"`
	TargetRef   string `json:"targetRef,omitempty"`
	Summary     string `json:"summary,omitempty"`
	State       string `json:"state"`
	CreatedAt   string `json:"createdAt,omitempty"`
	AppliedAt   string `json:"appliedAt,omitempty"`
}

type ValidationResult struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspaceId"`
	RunID          string `json:"runId"`
	StepID         string `json:"stepId,omitempty"`
	ValidationKind string `json:"validationKind"`
	TargetRef      string `json:"targetRef,omitempty"`
	State          string `json:"state"`
	Summary        string `json:"summary,omitempty"`
	EvidenceRef    string `json:"evidenceRef,omitempty"`
	StartedAt      string `json:"startedAt,omitempty"`
	EndedAt        string `json:"endedAt,omitempty"`
}

type RuntimeCheckpoint struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	RunID              string         `json:"runId"`
	CheckpointType     string         `json:"checkpointType"`
	CurrentStepID      string         `json:"currentStepId,omitempty"`
	RunStateSnapshot   string         `json:"runStateSnapshot"`
	BlockingTicketID   string         `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string         `json:"blockingApprovalId,omitempty"`
	ThreadRef          string         `json:"threadRef,omitempty"`
	OccurredAt         string         `json:"occurredAt,omitempty"`
	RestorableUntil    string         `json:"restorableUntil,omitempty"`
	Data               map[string]any `json:"data,omitempty"`
}

type WorkspaceSnapshot struct {
	Workspace Workspace  `json:"workspace"`
	Runs      []Run      `json:"runs,omitempty"`
	Decisions []Decision `json:"decisions,omitempty"`
	Approvals []Approval `json:"approvals,omitempty"`
}

type readModelDependencies struct {
	workspaces  repository.WorkspaceRepository
	runs        repository.RunRepository
	steps       repository.StepRepository
	toolCalls   repository.ToolCallRepository
	patches     repository.PatchRepository
	validations repository.ValidationResultRepository
	checkpoints repository.RuntimeCheckpointRepository
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	approvals   repository.ApprovalRepository
	published   []events.WorkorchEvent
}

func ReadRunSnapshotWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunSnapshot, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunSnapshot{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readRunSnapshot(ctx, deps, common.RunID(runID))
}

func ReadRunSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunSnapshot, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return RunSnapshot{}, err
	}
	return ReadRunSnapshotWithSession(ctx, session, runID)
}

func ReadRunSnapshotFromEnv(ctx context.Context, runID string) (RunSnapshot, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return RunSnapshot{}, err
	}
	return ReadRunSnapshotWithSession(ctx, session, runID)
}

func ReadRunStatusWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunStatus, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunStatus{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	snapshot, err := readRunSnapshot(ctx, deps, common.RunID(runID))
	if err != nil {
		return RunStatus{}, err
	}
	return mapRunStatus(snapshot), nil
}

func ReadRunStatusWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunStatus, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return RunStatus{}, err
	}
	return ReadRunStatusWithSession(ctx, session, runID)
}

func ReadRunStatusFromEnv(ctx context.Context, runID string) (RunStatus, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return RunStatus{}, err
	}
	return ReadRunStatusWithSession(ctx, session, runID)
}

func ReadRunObservabilityWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunObservability, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunObservability{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	snapshot, err := readRunSnapshot(ctx, deps, common.RunID(runID))
	if err != nil {
		return RunObservability{}, err
	}
	return mapRunObservability(snapshot), nil
}

func ReadRunObservabilityWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunObservability, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return RunObservability{}, err
	}
	return ReadRunObservabilityWithSession(ctx, session, runID)
}

func ReadRunObservabilityFromEnv(ctx context.Context, runID string) (RunObservability, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return RunObservability{}, err
	}
	return ReadRunObservabilityWithSession(ctx, session, runID)
}

func ReadWorkspaceSnapshotWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret WorkspaceSnapshot, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID))
}

func ReadWorkspaceSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (WorkspaceSnapshot, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceSnapshotFromEnv(ctx context.Context, workspaceID string) (WorkspaceSnapshot, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
}

func readModelDependenciesFromSession(session *RuntimeSession) (readModelDependencies, error) {
	exports, err := session.Exports()
	if err != nil {
		return readModelDependencies{}, err
	}
	if exports.Repositories.Workspaces == nil ||
		exports.Repositories.Runs == nil ||
		exports.Repositories.Steps == nil ||
		exports.Repositories.ToolCalls == nil ||
		exports.Repositories.Patches == nil ||
		exports.Repositories.ValidationResults == nil ||
		exports.Repositories.RuntimeCheckpoints == nil ||
		exports.Repositories.Tickets == nil ||
		exports.Repositories.Resolutions == nil ||
		exports.Repositories.Approvals == nil {
		return readModelDependencies{}, common.ErrInvalidArgument
	}

	var published []events.WorkorchEvent
	if session.Runtime != nil {
		published = session.Runtime.PublishedWorkorchEvents()
	}

	return readModelDependencies{
		workspaces:  exports.Repositories.Workspaces,
		runs:        exports.Repositories.Runs,
		steps:       exports.Repositories.Steps,
		toolCalls:   exports.Repositories.ToolCalls,
		patches:     exports.Repositories.Patches,
		validations: exports.Repositories.ValidationResults,
		checkpoints: exports.Repositories.RuntimeCheckpoints,
		tickets:     exports.Repositories.Tickets,
		resolutions: exports.Repositories.Resolutions,
		approvals:   exports.Repositories.Approvals,
		published:   published,
	}, nil
}

func readRunSnapshot(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunSnapshot, error) {
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	workspace, err := deps.workspaces.Get(ctx, item.WorkspaceID)
	if err != nil {
		return RunSnapshot{}, err
	}
	steps, err := deps.steps.ListByRun(ctx, item.ID)
	if err != nil {
		return RunSnapshot{}, err
	}
	toolCalls, err := deps.toolCalls.ListByRun(ctx, item.ID)
	if err != nil {
		return RunSnapshot{}, err
	}
	patches, err := deps.patches.ListByRun(ctx, item.ID)
	if err != nil {
		return RunSnapshot{}, err
	}
	validations, err := deps.validations.ListByRun(ctx, item.ID)
	if err != nil {
		return RunSnapshot{}, err
	}
	checkpoints, err := deps.checkpoints.ListByRun(ctx, item.ID)
	if err != nil {
		return RunSnapshot{}, err
	}
	decisions, selectedTicket, selectedResolution, err := decisionsForRun(ctx, deps, item)
	if err != nil {
		return RunSnapshot{}, err
	}
	approvals, selectedApproval, err := approvalsForRun(ctx, deps, item)
	if err != nil {
		return RunSnapshot{}, err
	}

	currentStep := lastStep(steps)
	observedItems := observed.BuildWorkorchSnapshot(workspace, item, steps, selectedTicket, selectedResolution, selectedApproval, deps.published)

	return RunSnapshot{
		Workspace:          mapWorkspaceWithRunProjection(workspace, item),
		Run:                mapRun(item),
		Step:               presenter.MapStep(currentStep),
		Steps:              presenter.MapSteps(steps),
		ToolCalls:          mapToolCalls(toolCalls),
		Patches:            mapPatches(patches),
		ValidationResults:  mapValidationResults(validations),
		RuntimeCheckpoints: mapRuntimeCheckpoints(checkpoints),
		Decisions:          decisions,
		Approvals:          approvals,
		Observed:           presenter.MapObserved(observedItems),
	}, nil
}

func readWorkspaceSnapshot(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) (WorkspaceSnapshot, error) {
	item, err := deps.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}

	runItems, err := deps.runs.List(ctx)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	runs := make([]Run, 0, len(runItems))
	projectionRun := selectWorkspaceProjectionRun(item, runItems)
	for _, runItem := range runItems {
		if runItem.WorkspaceID != workspaceID {
			continue
		}
		runs = append(runs, mapRun(runItem))
	}

	ticketItems, err := deps.tickets.List(ctx)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	decisions := make([]Decision, 0, len(ticketItems))
	for _, ticket := range ticketItems {
		if ticket.WorkspaceID != workspaceID {
			continue
		}
		resolution, err := resolutionForTicket(ctx, deps.resolutions, ticket)
		if err != nil {
			return WorkspaceSnapshot{}, err
		}
		decisions = append(decisions, mapDecision(ticket, resolution))
	}

	approvalItems, err := deps.approvals.List(ctx)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	approvals := make([]Approval, 0, len(approvalItems))
	for _, approval := range approvalItems {
		if approval.WorkspaceID != workspaceID {
			continue
		}
		approvals = append(approvals, mapApproval(approval))
	}

	return WorkspaceSnapshot{
		Workspace: mapWorkspaceWithRunProjection(item, projectionRun),
		Runs:      runs,
		Decisions: decisions,
		Approvals: approvals,
	}, nil
}

func selectWorkspaceProjectionRun(workspace *workspacemodel.Workspace, runs []*runmodel.Run) *runmodel.Run {
	if workspace == nil || len(runs) == 0 {
		return nil
	}
	var latest *runmodel.Run
	for _, item := range runs {
		if item == nil || item.WorkspaceID != workspace.ID {
			continue
		}
		if workspace.ActiveRunID != "" && item.ID == workspace.ActiveRunID {
			return item
		}
		if latest == nil || string(item.ID) > string(latest.ID) {
			latest = item
		}
	}
	return latest
}

func mapWorkspaceWithRunProjection(workspace *workspacemodel.Workspace, run *runmodel.Run) Workspace {
	ret := mapWorkspace(workspace)
	if run == nil {
		return ret
	}
	ret.State = string(workspaceProjectionState(run.State))
	if ret.ActiveRunID == "" {
		ret.ActiveRunID = string(run.ID)
	}
	return ret
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

func decisionsForRun(ctx context.Context, deps readModelDependencies, run *runmodel.Run) ([]Decision, *decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	items, err := deps.tickets.List(ctx)
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
		resolution, err := resolutionForTicket(ctx, deps.resolutions, item)
		if err != nil {
			return nil, nil, nil, err
		}
		ret = append(ret, mapDecision(item, resolution))
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

func approvalsForRun(ctx context.Context, deps readModelDependencies, run *runmodel.Run) ([]Approval, *approvalmodel.Approval, error) {
	items, err := deps.approvals.List(ctx)
	if err != nil {
		return nil, nil, err
	}

	ret := make([]Approval, 0, len(items))
	var selectedApproval *approvalmodel.Approval
	for _, item := range items {
		if item.RunID != run.ID {
			continue
		}
		ret = append(ret, mapApproval(item))
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

func mapToolCalls(items []*runmodel.ToolCall) []ToolCall {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ToolCall, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, ToolCall{
			ID:          string(item.ID),
			WorkspaceID: string(item.WorkspaceID),
			RunID:       string(item.RunID),
			StepID:      string(item.StepID),
			ToolName:    item.ToolName,
			Arguments:   cloneReadModelMap(item.Arguments),
			State:       string(item.State),
			StartedAt:   item.StartedAt,
			EndedAt:     item.EndedAt,
			ExitSummary: item.ExitSummary,
			Output:      cloneReadModelMap(item.Output),
			Error:       item.Error,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapPatches(items []*runmodel.Patch) []Patch {
	if len(items) == 0 {
		return nil
	}
	ret := make([]Patch, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, Patch{
			ID:          string(item.ID),
			WorkspaceID: string(item.WorkspaceID),
			RunID:       string(item.RunID),
			StepID:      string(item.StepID),
			ToolCallID:  string(item.ToolCallID),
			PatchType:   item.PatchType,
			TargetRef:   item.TargetRef,
			Summary:     item.Summary,
			State:       string(item.State),
			CreatedAt:   item.CreatedAt,
			AppliedAt:   item.AppliedAt,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapValidationResults(items []*runmodel.ValidationResult) []ValidationResult {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ValidationResult, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, ValidationResult{
			ID:             string(item.ID),
			WorkspaceID:    string(item.WorkspaceID),
			RunID:          string(item.RunID),
			StepID:         string(item.StepID),
			ValidationKind: item.ValidationKind,
			TargetRef:      item.TargetRef,
			State:          string(item.State),
			Summary:        item.Summary,
			EvidenceRef:    item.EvidenceRef,
			StartedAt:      item.StartedAt,
			EndedAt:        item.EndedAt,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint) []RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	ret := make([]RuntimeCheckpoint, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, RuntimeCheckpoint{
			ID:                 string(item.ID),
			WorkspaceID:        string(item.WorkspaceID),
			RunID:              string(item.RunID),
			CheckpointType:     string(item.CheckpointType),
			CurrentStepID:      string(item.CurrentStepID),
			RunStateSnapshot:   string(item.RunStateSnapshot),
			BlockingTicketID:   string(item.BlockingTicketID),
			BlockingApprovalID: string(item.BlockingApprovalID),
			ThreadRef:          item.ThreadRef,
			OccurredAt:         item.OccurredAt,
			RestorableUntil:    item.RestorableUntil,
			Data:               cloneReadModelMap(item.Data),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func cloneReadModelMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func mapRunStatus(snapshot RunSnapshot) RunStatus {
	ret := RunStatus{
		ID:                 snapshot.Run.ID,
		WorkspaceID:        snapshot.Run.WorkspaceID,
		State:              snapshot.Run.State,
		StepCount:          len(snapshot.Steps),
		DecisionCount:      len(snapshot.Decisions),
		ApprovalCount:      len(snapshot.Approvals),
		BlockingTicketID:   snapshot.Run.BlockingTicketID,
		BlockingApprovalID: snapshot.Run.BlockingApprovalID,
		FailureReason:      snapshot.Run.FailureReason,
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = snapshot.Step.Kind
		ret.CurrentStepState = snapshot.Step.State
	}
	return ret
}

func mapRunObservability(snapshot RunSnapshot) RunObservability {
	timeline := runtimeTimeline(snapshot)
	return RunObservability{
		Run:              snapshot.Run,
		Progress:         runtimeProgress(snapshot),
		Evidence:         runtimeEvidenceSummary(snapshot, len(timelineEvents(timeline))),
		LatestCheckpoint: latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints),
		Timeline:         timeline,
	}
}

func runtimeProgress(snapshot RunSnapshot) RunProgress {
	ret := RunProgress{
		State:     snapshot.Run.State,
		StepTotal: len(snapshot.Steps),
		Blocked:   snapshot.Run.BlockingTicketID != "" || snapshot.Run.BlockingApprovalID != "",
		Terminal:  isTerminalRunState(snapshot.Run.State),
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = snapshot.Step.Kind
		ret.CurrentStepState = snapshot.Step.State
	}
	for _, step := range snapshot.Steps {
		switch step.State {
		case string(common.StepStateDispatched):
			ret.StepDispatched++
		case string(common.StepStateCompleted):
			ret.StepCompleted++
		case string(common.StepStateFailed):
			ret.StepFailed++
		}
	}
	return ret
}

func runtimeEvidenceSummary(snapshot RunSnapshot, eventTotal int) RuntimeEvidenceSummary {
	ret := RuntimeEvidenceSummary{
		ToolCallTotal:          len(snapshot.ToolCalls),
		PatchTotal:             len(snapshot.Patches),
		ValidationTotal:        len(snapshot.ValidationResults),
		RuntimeCheckpointTotal: len(snapshot.RuntimeCheckpoints),
		EventTotal:             eventTotal,
	}
	for _, item := range snapshot.ToolCalls {
		switch item.State {
		case string(common.ToolCallStateRunning):
			ret.ToolCallRunning++
		case string(common.ToolCallStateCompleted):
			ret.ToolCallCompleted++
		case string(common.ToolCallStateFailed):
			ret.ToolCallFailed++
		}
	}
	for _, item := range snapshot.Patches {
		switch item.State {
		case string(common.PatchStateApplied):
			ret.PatchApplied++
		case string(common.PatchStateFailed):
			ret.PatchFailed++
		}
	}
	for _, item := range snapshot.ValidationResults {
		switch item.State {
		case string(common.ValidationResultStatePassed):
			ret.ValidationPassed++
		case string(common.ValidationResultStateFailed):
			ret.ValidationFailed++
		case string(common.ValidationResultStateWarning):
			ret.ValidationWarning++
		}
	}
	return ret
}

func latestRuntimeCheckpoint(items []RuntimeCheckpoint) *RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	latest := items[0]
	for _, item := range items[1:] {
		if item.OccurredAt > latest.OccurredAt {
			latest = item
		}
	}
	return &latest
}

func runtimeTimeline(snapshot RunSnapshot) []RuntimeTimelineItem {
	items := make([]RuntimeTimelineItem, 0, len(snapshot.RuntimeCheckpoints)+len(snapshot.Observed))
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		items = append(items, RuntimeTimelineItem{
			Source:     "checkpoint",
			Type:       checkpoint.CheckpointType,
			ID:         checkpoint.ID,
			RunState:   checkpoint.RunStateSnapshot,
			StepID:     checkpoint.CurrentStepID,
			OccurredAt: checkpoint.OccurredAt,
			Data:       cloneReadModelMap(checkpoint.Data),
		})
	}
	for _, item := range snapshot.Observed {
		if item.RunID != "" && item.RunID != snapshot.Run.ID {
			continue
		}
		items = append(items, RuntimeTimelineItem{
			Source:     "event",
			Type:       item.SubjectType,
			ID:         item.SubjectID,
			Topic:      item.Topic,
			StepID:     item.StepID,
			OccurredAt: item.OccurredAt,
			Data:       cloneReadModelMap(item.Payload),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].OccurredAt == items[j].OccurredAt {
			return items[i].Source < items[j].Source
		}
		return items[i].OccurredAt < items[j].OccurredAt
	})
	return items
}

func timelineEvents(items []RuntimeTimelineItem) []RuntimeTimelineItem {
	ret := make([]RuntimeTimelineItem, 0, len(items))
	for _, item := range items {
		if item.Source == "event" {
			ret = append(ret, item)
		}
	}
	return ret
}

func isTerminalRunState(state string) bool {
	switch common.RunState(state) {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}
