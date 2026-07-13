package backend

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	client "github.com/muidea/workorch/internal/pkg/workorchclient"
)

// remoteBackend implements Backend using an HTTP client to a running workorch --server.
type remoteBackend struct {
	cl   *client.Client
	ws   *remoteWorkspaceBackend
	goal *remoteGoalBackend
	dod  *remoteDoDBackend
	run  *remoteRunBackend
	dec  *remoteDecisionBackend
	appr *remoteApprovalBackend
	soul *remoteSoulBackend
	sk   *remoteSkillBackend
	mcp  *remoteMCPBackend
	mem  *remoteMemoryBackend
	tmpl *remoteTemplateBackend
	cfg  *remoteConfigBackend
	evt  *remoteEventsBackend
}

type RemoteOption func(*remoteOptions)

type remoteOptions struct {
	eventStreamView string
}

// NewRemote creates a Backend that communicates with a remote workorch --server via HTTP.
func NewRemote(addr string) Backend {
	return NewRemoteWithOptions(addr)
}

func NewRemoteProjectionEvents(addr string) Backend {
	return NewRemoteWithOptions(addr, WithRemoteEventStreamView("projection"))
}

func NewRemoteWithOptions(addr string, opts ...RemoteOption) Backend {
	options := remoteOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	cl := client.New(addr)
	b := &remoteBackend{cl: cl}
	b.ws = &remoteWorkspaceBackend{cl: cl}
	b.goal = &remoteGoalBackend{cl: cl}
	b.dod = &remoteDoDBackend{cl: cl}
	b.run = &remoteRunBackend{cl: cl}
	b.dec = &remoteDecisionBackend{cl: cl}
	b.appr = &remoteApprovalBackend{cl: cl}
	b.soul = &remoteSoulBackend{cl: cl}
	b.sk = &remoteSkillBackend{cl: cl}
	b.mcp = &remoteMCPBackend{cl: cl}
	b.mem = &remoteMemoryBackend{cl: cl}
	b.tmpl = &remoteTemplateBackend{cl: cl}
	b.cfg = &remoteConfigBackend{cl: cl}
	b.evt = &remoteEventsBackend{cl: cl, streamView: strings.TrimSpace(options.eventStreamView)}
	return b
}

func WithRemoteEventStreamView(view string) RemoteOption {
	return func(options *remoteOptions) {
		if options != nil {
			options.eventStreamView = strings.TrimSpace(view)
		}
	}
}

func (b *remoteBackend) Workspace() WorkspaceBackend { return b.ws }
func (b *remoteBackend) Goal() GoalBackend           { return b.goal }
func (b *remoteBackend) DoD() DoDBackend             { return b.dod }
func (b *remoteBackend) Run() RunBackend             { return b.run }
func (b *remoteBackend) Decision() DecisionBackend   { return b.dec }
func (b *remoteBackend) Approval() ApprovalBackend   { return b.appr }
func (b *remoteBackend) Soul() SoulBackend           { return b.soul }
func (b *remoteBackend) Skill() SkillBackend         { return b.sk }
func (b *remoteBackend) MCP() MCPBackend             { return b.mcp }
func (b *remoteBackend) Memory() MemoryBackend       { return b.mem }
func (b *remoteBackend) Template() TemplateBackend   { return b.tmpl }
func (b *remoteBackend) Config() ConfigBackend       { return b.cfg }
func (b *remoteBackend) Events() EventsBackend       { return b.evt }

// ── Remote Workspace ──

type remoteWorkspaceBackend struct{ cl *client.Client }

func workspaceInfoFromClient(ws *client.Workspace) WorkspaceInfo {
	if ws == nil {
		return WorkspaceInfo{}
	}
	return WorkspaceInfo{
		ID:                   common.WorkspaceID(ws.ID),
		Title:                ws.Title,
		Description:          ws.Description,
		DeliverableDirectory: ws.DeliverableDirectory,
		State:                common.WorkspaceState(ws.State),
		ActiveRunID:          common.RunID(ws.ActiveRunID),
		BlockingTicketID:     common.TicketID(ws.BlockingTicketID),
		BlockingApprovalID:   common.ApprovalID(ws.BlockingApprovalID),
		TemplateID:           ws.TemplateID,
		ProfileID:            ws.ProfileID,
		DefaultSoulID:        common.SoulID(ws.DefaultSoulID),
		SoulBindings:         commonSoulBindings(ws.SoulBindings),
		Projection:           projectionInfoFromClient(ws.Projection),
		ViewModel:            workspaceViewModelInfoFromClient(ws.ViewModel),
	}
}

func projectionInfoFromClient(input client.ProjectionStatus) ProjectionInfo {
	return ProjectionInfo{
		State:                   input.State,
		GlobalRevision:          input.GlobalRevision,
		WorkspaceRevision:       input.WorkspaceRevision,
		SourceEventRevision:     input.SourceEventRevision,
		ProjectedSourceRevision: input.ProjectedSourceRevision,
		UpdatedAt:               input.UpdatedAt,
		RebuiltAt:               input.RebuiltAt,
		Degraded:                input.Degraded,
		StaleReason:             input.StaleReason,
		Error:                   input.Error,
	}
}

func workspaceViewModelInfoFromClient(input client.WorkspaceViewModel) WorkspaceViewModelInfo {
	ret := WorkspaceViewModelInfo{
		WorkspaceID:             common.WorkspaceID(input.WorkspaceID),
		State:                   input.State,
		PromptMode:              input.PromptMode,
		Revision:                input.Revision,
		GlobalRevision:          input.GlobalRevision,
		WorkspaceRevision:       input.WorkspaceRevision,
		SourceEventRevision:     input.SourceEventRevision,
		ProjectedSourceRevision: input.ProjectedSourceRevision,
		UpdatedAt:               input.UpdatedAt,
		RebuiltAt:               input.RebuiltAt,
		Degraded:                input.Degraded,
		StaleReason:             input.StaleReason,
		Error:                   input.Error,
	}
	if len(input.Sections) > 0 {
		ret.Sections = make(map[string]WorkspaceViewModelSectionInfo, len(input.Sections))
		for key, value := range input.Sections {
			ret.Sections[key] = WorkspaceViewModelSectionInfo{
				Name:                    value.Name,
				Revision:                value.Revision,
				WorkspaceRevision:       value.WorkspaceRevision,
				SourceEventRevision:     value.SourceEventRevision,
				ProjectedSourceRevision: value.ProjectedSourceRevision,
				State:                   value.State,
			}
		}
	}
	return ret
}

func (b *remoteWorkspaceBackend) Create(ctx context.Context, req CreateWorkspaceRequest) (*WorkspaceInfo, error) {
	ws, err := b.cl.CreateWorkspace(ctx, req.Title, req.Description, req.DeliverableDirectory, req.TemplateID)
	if err != nil {
		return nil, err
	}
	info := workspaceInfoFromClient(ws)
	return &info, nil
}

func (b *remoteWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*WorkspaceInfo, error) {
	ws, err := b.cl.GetWorkspace(ctx, string(id))
	if err != nil {
		return nil, err
	}
	info := workspaceInfoFromClient(ws)
	return &info, nil
}

func (b *remoteWorkspaceBackend) List(ctx context.Context) ([]WorkspaceInfo, error) {
	projection, err := b.cl.ListWorkspaceProjection(ctx)
	if err != nil {
		return nil, err
	}
	list := projection.Workspaces
	result := make([]WorkspaceInfo, 0, len(list))
	for _, ws := range list {
		item := workspaceInfoFromClient(&ws)
		result = append(result, item)
	}
	return result, nil
}

func (b *remoteWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*WorkspaceSnapshotInfo, error) {
	projection, err := b.cl.GetWorkspaceSnapshotProjection(ctx, string(id))
	if err != nil {
		return nil, err
	}
	snapshot := workspaceSnapshotInfoFromClient(projection.Snapshot)
	snapshot.Projection = projectionInfoFromClient(projection.Projection)
	snapshot.Workspace.Projection = snapshot.Projection
	return &snapshot, nil
}

func workspaceSnapshotInfoFromClient(input client.WorkspaceSnapshot) WorkspaceSnapshotInfo {
	return WorkspaceSnapshotInfo{
		Workspace:               workspaceInfoFromClient(&input.Workspace),
		ViewModel:               workspaceViewModelInfoFromClient(input.ViewModel),
		Projection:              projectionInfoFromClient(input.Workspace.Projection),
		Outcome:                 workspaceOutcomeInfoFromClient(input.Outcome),
		TaskProgress:            taskProgressInfoFromClient(input.TaskProgress),
		RunHeartbeat:            runHeartbeatInfoFromClient(input.RunHeartbeat),
		ConvergenceStatus:       convergenceStatusInfoFromClient(input.ConvergenceStatus),
		RunCommand:              runCommandStatusInfoFromClient(input.RunCommand),
		SchedulerDiagnostics:    schedulerDiagnosticsInfoFromClient(input.SchedulerDiagnostics),
		ContextBundle:           clonePayloadMap(input.ContextBundle),
		CoverageSummary:         clonePayloadMap(input.CoverageSummary),
		GapBaseline:             gapBaselineInfoFromClient(input.GapBaseline),
		Acceptance:              acceptanceProjectionInfoFromClient(input.Acceptance),
		TaskDecompositions:      taskDecompositionInfosFromClient(input.TaskDecompositions),
		TaskPlans:               taskPlanInfosFromClient(input.TaskPlans),
		Goals:                   goalInfosFromClient(input.Goals),
		DoDs:                    dodInfosFromClient(input.DoDs),
		Runs:                    runInfosFromClient(input.Runs),
		Results:                 resultInfosFromClient(input.Results),
		Artifacts:               artifactInfosFromClient(input.Artifacts),
		ArtifactContinuations:   artifactContinuationInfosFromClient(input.ArtifactContinuations),
		Patches:                 patchInfosFromClient(input.Patches),
		MemoryRecallTraces:      memoryRecallTraceInfosFromClient(input.MemoryRecallTraces),
		LLMContextBudgets:       llmContextBudgetTraceInfosFromClient(input.LLMContextBudgets),
		LLMContextBudgetSummary: llmContextBudgetSummaryInfoFromClient(input.LLMContextBudgetSummary),
		LLMUsageTraces:          llmUsageTraceInfosFromClient(input.LLMUsageTraces),
		LLMUsageSummary:         llmUsageSummaryInfoFromClient(input.LLMUsageSummary),
		EventRecords:            eventRecordInfosFromClient(input.EventRecords),
		Decisions:               decisionInfosFromClient(input.Decisions),
		Approvals:               approvalInfosFromClient(input.Approvals),
	}
}

func goalInfosFromClient(items []client.Goal) []GoalInfo {
	ret := make([]GoalInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, GoalInfo{ID: common.GoalID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), Text: item.Text})
	}
	return ret
}

func dodInfosFromClient(items []client.DoD) []DoDInfo {
	ret := make([]DoDInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, DoDInfo{ID: common.DoDID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), Items: append([]string(nil), item.Items...), State: item.State})
	}
	return ret
}

func runInfosFromClient(items []client.Run) []RunInfo {
	ret := make([]RunInfo, 0, len(items))
	for _, item := range items {
		if mapped := remoteRunInfoFromClientRun(&item); mapped != nil {
			ret = append(ret, *mapped)
		}
	}
	return ret
}

func workspaceOutcomeInfoFromClient(input client.RunOutcome) WorkspaceOutcomeInfo {
	return WorkspaceOutcomeInfo{
		WorkspaceID:  common.WorkspaceID(input.WorkspaceID),
		State:        input.State,
		RunID:        common.RunID(input.RunID),
		ResultID:     common.ResultID(input.ResultID),
		Accepted:     input.Accepted,
		Terminal:     input.Terminal,
		Summary:      input.Summary,
		Reason:       input.Reason,
		EvidenceRefs: append([]string(nil), input.EvidenceRefs...),
		ArtifactRefs: append([]string(nil), input.ArtifactRefs...),
	}
}

func runHeartbeatInfoFromClient(input client.RunHeartbeat) RunHeartbeatInfo {
	return RunHeartbeatInfo{
		WorkspaceID:               common.WorkspaceID(input.WorkspaceID),
		RunID:                     common.RunID(input.RunID),
		State:                     input.State,
		ActivityState:             input.ActivityState,
		LatestStepID:              common.StepID(input.LatestStepID),
		LatestStepKind:            input.LatestStepKind,
		LatestStepState:           input.LatestStepState,
		LatestEvidenceRef:         input.LatestEvidenceRef,
		LatestEventTopic:          input.LatestEventTopic,
		LatestEventRevision:       input.LatestEventRevision,
		LatestEventAt:             input.LatestEventAt,
		LatestLLMUsageAt:          input.LatestLLMUsageAt,
		LatestResultAt:            input.LatestResultAt,
		LatestArtifactAt:          input.LatestArtifactAt,
		LatestPatchAt:             input.LatestPatchAt,
		LatestEffectiveProgressAt: input.LatestEffectiveProgressAt,
		IneffectiveRetry:          input.IneffectiveRetry,
		Stalled:                   input.Stalled,
		Summary:                   input.Summary,
	}
}

func runCommandStatusInfoFromClient(input *client.RunCommandStatus) *RunCommandStatusInfo {
	if input == nil {
		return nil
	}
	return &RunCommandStatusInfo{
		WorkspaceID:        common.WorkspaceID(input.WorkspaceID),
		RunID:              common.RunID(input.RunID),
		State:              input.State,
		Intent:             input.Intent,
		Reason:             input.Reason,
		RequestID:          input.RequestID,
		IdempotencyKey:     input.IdempotencyKey,
		Source:             input.Source,
		DefinitionRevision: input.DefinitionRevision,
		DecisionID:         common.TicketID(input.DecisionID),
		AcceptedAt:         input.AcceptedAt,
		RejectedAt:         input.RejectedAt,
		Error:              input.Error,
		Revision:           input.Revision,
	}
}

func gapBaselineInfoFromClient(input *client.GapBaseline) *GapBaselineInfo {
	if input == nil {
		return nil
	}
	ret := &GapBaselineInfo{
		WorkspaceID:           common.WorkspaceID(input.WorkspaceID),
		RunID:                 common.RunID(input.RunID),
		DefinitionRevision:    input.DefinitionRevision,
		BaselineRevision:      input.BaselineRevision,
		CurrentFocus:          clonePayloadMap(input.CurrentFocus),
		OpenCoverageWorkItems: clonePayloadMapSlice(input.OpenCoverageWorkItems),
		AcceptedEvidenceRefs:  append([]string(nil), input.AcceptedEvidenceRefs...),
		RejectedEvidenceRefs:  append([]string(nil), input.RejectedEvidenceRefs...),
		NextAction:            GapNextActionInfo{Kind: input.NextAction.Kind, Reason: input.NextAction.Reason},
	}
	for _, item := range input.TodoItems {
		ret.TodoItems = append(ret.TodoItems, GapTodoItemInfo{
			ID:                    item.ID,
			Source:                item.Source,
			SourceRef:             item.SourceRef,
			CoverageWorkItemIDs:   append([]string(nil), item.CoverageWorkItemIDs...),
			RequiredEvidenceKinds: append([]string(nil), item.RequiredEvidenceKinds...),
			Status:                item.Status,
			Priority:              item.Priority,
			Reason:                item.Reason,
			Metadata:              clonePayloadMap(item.Metadata),
		})
	}
	return ret
}

func acceptanceProjectionInfoFromClient(input client.AcceptanceProjection) AcceptanceProjectionInfo {
	ret := AcceptanceProjectionInfo{
		ActiveSliceID: input.ActiveSliceID,
		Summary: AcceptanceSummaryInfo{
			AcceptedSlices: input.Summary.AcceptedSlices,
			ActiveSlices:   input.Summary.ActiveSlices,
			OpenSlices:     input.Summary.OpenSlices,
			FailedSlices:   input.Summary.FailedSlices,
		},
	}
	for _, item := range input.Slices {
		ret.Slices = append(ret.Slices, AcceptanceSliceInfo{
			ID:                    item.ID,
			Title:                 item.Title,
			State:                 item.State,
			Priority:              item.Priority,
			Scope:                 item.Scope,
			CoverageWorkItemIDs:   append([]string(nil), item.CoverageWorkItemIDs...),
			RequiredEvidenceKinds: append([]string(nil), item.RequiredEvidenceKinds...),
			CompletionPolicy:      item.CompletionPolicy,
			ResultPolicy:          item.ResultPolicy,
			CreatedAt:             item.CreatedAt,
			ActivatedAt:           item.ActivatedAt,
			ClosedAt:              item.ClosedAt,
		})
	}
	if input.CurrentFocus != nil {
		ret.CurrentFocus = &CoverageFocusInfo{
			ID:                    input.CurrentFocus.ID,
			SliceID:               input.CurrentFocus.SliceID,
			CoverageWorkItemIDs:   append([]string(nil), input.CurrentFocus.CoverageWorkItemIDs...),
			FailureClass:          input.CurrentFocus.FailureClass,
			FailureSignature:      input.CurrentFocus.FailureSignature,
			RequiredEvidenceKinds: append([]string(nil), input.CurrentFocus.RequiredEvidenceKinds...),
			AcceptedEvidenceRefs:  append([]string(nil), input.CurrentFocus.AcceptedEvidenceRefs...),
			RejectedEvidenceRefs:  append([]string(nil), input.CurrentFocus.RejectedEvidenceRefs...),
			TargetRefs:            append([]string(nil), input.CurrentFocus.TargetRefs...),
			State:                 input.CurrentFocus.State,
			Attempt:               input.CurrentFocus.Attempt,
		}
	}
	if input.Budget != nil {
		ret.Budget = &ConvergenceBudgetInfo{
			State:                       input.Budget.State,
			Decision:                    input.Budget.Decision,
			MaxRepairAttemptsPerFocus:   input.Budget.MaxRepairAttemptsPerFocus,
			MaxLLMRoundsPerSlice:        input.Budget.MaxLLMRoundsPerSlice,
			MaxRepeatedFailureSignature: input.Budget.MaxRepeatedFailureSignature,
			MaxValidationOnlyRetries:    input.Budget.MaxValidationOnlyRetries,
			MaxNoCoverageDeltaRounds:    input.Budget.MaxNoCoverageDeltaRounds,
			MaxContextBytesPerRepair:    input.Budget.MaxContextBytesPerRepair,
			RepairAttempts:              input.Budget.RepairAttempts,
			LLMRounds:                   input.Budget.LLMRounds,
			RepeatedFailureSignatures:   input.Budget.RepeatedFailureSignatures,
			ValidationOnlyRetries:       input.Budget.ValidationOnlyRetries,
			NoCoverageDeltaRounds:       input.Budget.NoCoverageDeltaRounds,
			LastDecisionReason:          input.Budget.LastDecisionReason,
		}
	}
	return ret
}

func taskProgressInfoFromClient(input client.TaskProgress) TaskProgressInfo {
	return TaskProgressInfo{
		WorkspaceID:        common.WorkspaceID(input.WorkspaceID),
		RunID:              common.RunID(input.RunID),
		State:              input.State,
		Summary:            input.Summary,
		CurrentActionLabel: input.CurrentActionLabel,
		DiagnosticLineage:  common.StepID(input.DiagnosticLineage),
		Recovery: RecoveryPlanInfo{
			Mode:               input.Recovery.Mode,
			NextAction:         input.Recovery.NextAction,
			RunID:              common.RunID(input.Recovery.RunID),
			StepID:             common.StepID(input.Recovery.StepID),
			BlockingTicketID:   common.TicketID(input.Recovery.BlockingTicketID),
			BlockingApprovalID: common.ApprovalID(input.Recovery.BlockingApprovalID),
			CheckpointID:       common.CheckpointID(input.Recovery.CheckpointID),
			FailureClass:       input.Recovery.FailureClass,
			FailureSignature:   input.Recovery.FailureSignature,
			Reason:             input.Recovery.Reason,
		},
		RepairLoop: RepairLoopInfo{
			State:                    input.RepairLoop.State,
			AttemptCount:             input.RepairLoop.AttemptCount,
			LastAttempt:              input.RepairLoop.LastAttempt,
			LastRepairStepID:         common.StepID(input.RepairLoop.LastRepairStepID),
			LastFailureStepID:        common.StepID(input.RepairLoop.LastFailureStepID),
			LastFailureClass:         input.RepairLoop.LastFailureClass,
			LastFailureSignature:     input.RepairLoop.LastFailureSignature,
			LastFailureTargetScope:   input.RepairLoop.LastFailureTargetScope,
			LastFailureAt:            input.RepairLoop.LastFailureAt,
			StructuredFailureSignals: clonePayloadMap(input.RepairLoop.StructuredFailureSignals),
			RepairInputCompaction:    clonePayloadMap(input.RepairLoop.RepairInputCompaction),
			LastRepairInputBytes:     input.RepairLoop.LastRepairInputBytes,
			RepeatedFailureReason:    input.RepairLoop.RepeatedFailureReason,
			NextAction:               input.RepairLoop.NextAction,
			Escalated:                input.RepairLoop.Escalated,
			EscalationReason:         input.RepairLoop.EscalationReason,
			EscalationTicketID:       common.TicketID(input.RepairLoop.EscalationTicketID),
			EvidenceRefs:             append([]string(nil), input.RepairLoop.EvidenceRefs...),
		},
		AssessmentOutcome: AssessmentOutcomeInfo{
			State:                       input.AssessmentOutcome.State,
			AcceptedSlices:              input.AssessmentOutcome.AcceptedSlices,
			AcceptedResultCount:         input.AssessmentOutcome.AcceptedResultCount,
			RejectedGlobalAssessments:   input.AssessmentOutcome.RejectedGlobalAssessments,
			SatisfiedCoverageCount:      input.AssessmentOutcome.SatisfiedCoverageCount,
			RemainingCoverageCount:      input.AssessmentOutcome.RemainingCoverageCount,
			NewlySatisfiedCoverageCount: input.AssessmentOutcome.NewlySatisfiedCoverageCount,
			LatestAcceptedResultID:      common.ResultID(input.AssessmentOutcome.LatestAcceptedResultID),
			LatestRejectedResultID:      common.ResultID(input.AssessmentOutcome.LatestRejectedResultID),
			LatestRejectedReason:        input.AssessmentOutcome.LatestRejectedReason,
		},
	}
}

func taskDecompositionInfosFromClient(items []client.TaskDecomposition) []TaskDecompositionInfo {
	ret := make([]TaskDecompositionInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, TaskDecompositionInfo{
			ID:            item.ID,
			WorkspaceID:   common.WorkspaceID(item.WorkspaceID),
			RunID:         common.RunID(item.RunID),
			RootStepID:    common.StepID(item.RootStepID),
			State:         item.State,
			Summary:       item.Summary,
			GraphBranch:   item.GraphBranch,
			NodeTotal:     item.NodeTotal,
			NodeCompleted: item.NodeCompleted,
			NodeFailed:    item.NodeFailed,
			NodeRunning:   item.NodeRunning,
			Nodes:         taskDecompositionNodeInfosFromClient(item.Nodes),
			EvidenceRefs:  append([]string(nil), item.EvidenceRefs...),
		})
	}
	return ret
}

func taskPlanInfosFromClient(items []client.TaskPlan) []TaskPlanInfo {
	ret := make([]TaskPlanInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, TaskPlanInfo{
			ID:                item.ID,
			WorkspaceID:       common.WorkspaceID(item.WorkspaceID),
			RunID:             common.RunID(item.RunID),
			AnchorStepID:      common.StepID(item.AnchorStepID),
			RevisionStepID:    common.StepID(item.RevisionStepID),
			Revision:          item.Revision,
			State:             item.State,
			Stage:             item.Stage,
			SourceOutputState: item.SourceOutputState,
			Summary:           item.Summary,
			GraphBranch:       item.GraphBranch,
			NodeTotal:         item.NodeTotal,
			NodeCompleted:     item.NodeCompleted,
			NodeFailed:        item.NodeFailed,
			NodeRunning:       item.NodeRunning,
			Nodes:             taskDecompositionNodeInfosFromClient(item.Nodes),
			RetryOf:           item.RetryOf,
			RetryAttempt:      item.RetryAttempt,
			FailureClass:      item.FailureClass,
			FailureSignature:  item.FailureSignature,
			TargetScope:       item.TargetScope,
			BlockingTicketID:  common.TicketID(item.BlockingTicketID),
			EvidenceRefs:      append([]string(nil), item.EvidenceRefs...),
			OccurredAt:        item.OccurredAt,
			SourceProjection:  item.SourceProjection,
		})
	}
	return ret
}

func taskDecompositionNodeInfosFromClient(items []client.TaskDecompositionNode) []TaskDecompositionNodeInfo {
	ret := make([]TaskDecompositionNodeInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, TaskDecompositionNodeInfo{
			ID:               item.ID,
			Kind:             item.Kind,
			Title:            item.Title,
			State:            item.State,
			DependsOn:        append([]string(nil), item.DependsOn...),
			Branch:           item.Branch,
			StepID:           common.StepID(item.StepID),
			StepState:        item.StepState,
			RetryOf:          item.RetryOf,
			RetryAttempt:     item.RetryAttempt,
			FailureClass:     item.FailureClass,
			FailureSignature: item.FailureSignature,
			TargetScope:      item.TargetScope,
			BlockingTicketID: common.TicketID(item.BlockingTicketID),
			EvidenceRefs:     append([]string(nil), item.EvidenceRefs...),
			CoverageTargets:  append([]string(nil), item.CoverageTargets...),
			Input:            clonePayloadMap(item.Input),
		})
	}
	return ret
}

func resultInfosFromClient(items []client.Result) []ResultInfo {
	ret := make([]ResultInfo, 0, len(items))
	for _, item := range items {
		summary := item.SummaryDetail
		ret = append(ret, ResultInfo{
			ID:           common.ResultID(item.ID),
			WorkspaceID:  common.WorkspaceID(item.WorkspaceID),
			RunID:        common.RunID(item.RunID),
			GoalID:       common.GoalID(item.GoalID),
			DoDID:        common.DoDID(item.DoDID),
			State:        common.ResultState(item.State),
			Title:        item.Title,
			Summary:      item.Summary,
			SourceType:   item.SourceType,
			SourceRef:    item.SourceRef,
			EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
			ArtifactRefs: append([]string(nil), item.ArtifactRefs...),
			SummaryDetail: &ResultSummaryInfo{
				GoalOutcome:           summary.GoalOutcome,
				ImplementationSummary: summary.ImplementationSummary,
				VerificationSummary:   summary.VerificationSummary,
				EvidenceRefs:          append([]string(nil), summary.EvidenceRefs...),
				ArtifactRefs:          append([]string(nil), summary.ArtifactRefs...),
				KnownLimitations:      append([]string(nil), summary.KnownLimitations...),
				ResidualRisks:         append([]string(nil), summary.ResidualRisks...),
				NextActions:           append([]string(nil), summary.NextActions...),
				Diagnostics:           clonePayloadMap(summary.Diagnostics),
			},
		})
	}
	return ret
}

func artifactInfosFromClient(items []client.Artifact) []ArtifactInfo {
	ret := make([]ArtifactInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, ArtifactInfo{
			ID:           common.ArtifactID(item.ID),
			WorkspaceID:  common.WorkspaceID(item.WorkspaceID),
			RunID:        common.RunID(item.RunID),
			ResultID:     common.ResultID(item.ResultID),
			ArtifactType: item.ArtifactType,
			Category:     item.Category,
			Name:         item.Name,
			Summary:      item.Summary,
			URI:          item.URI,
			ContentType:  item.ContentType,
			State:        common.ArtifactState(item.State),
			EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
		})
	}
	return ret
}

func artifactContinuationInfosFromClient(items []client.ArtifactContinuation) []ArtifactContinuationInfo {
	ret := make([]ArtifactContinuationInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, ArtifactContinuationInfo{
			ID:                common.ArtifactContinuationID(item.ID),
			WorkspaceID:       common.WorkspaceID(item.WorkspaceID),
			RunID:             common.RunID(item.RunID),
			RootStepID:        common.StepID(item.RootStepID),
			ArtifactID:        item.ArtifactID,
			TargetRef:         item.TargetRef,
			Operation:         item.Operation,
			State:             common.ArtifactContinuationState(item.State),
			ExpectedSequence:  item.ExpectedSequence,
			ReceivedSequences: append([]int(nil), item.ReceivedSequences...),
			FinalReceived:     item.FinalReceived,
			BytesWritten:      item.BytesWritten,
			RollingHash:       item.RollingHash,
			LastChunkHash:     item.LastChunkHash,
			CoverageTargets:   append([]string(nil), item.CoverageTargets...),
			EvidenceRefs:      append([]string(nil), item.EvidenceRefs...),
			FailureReason:     item.FailureReason,
			CreatedAt:         item.CreatedAt,
			UpdatedAt:         item.UpdatedAt,
			Progress:          clonePayloadMap(item.Progress),
		})
	}
	return ret
}

func patchInfosFromClient(items []client.Patch) []PatchInfo {
	ret := make([]PatchInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, PatchInfo{
			ID:               common.PatchID(item.ID),
			WorkspaceID:      common.WorkspaceID(item.WorkspaceID),
			RunID:            common.RunID(item.RunID),
			StepID:           common.StepID(item.StepID),
			ToolCallID:       common.ToolCallID(item.ToolCallID),
			ApprovalID:       common.ApprovalID(item.ApprovalID),
			RevertApprovalID: common.ApprovalID(item.RevertApprovalID),
			PatchType:        item.PatchType,
			TargetRef:        item.TargetRef,
			Summary:          item.Summary,
			State:            common.PatchState(item.State),
			AddedLines:       item.AddedLines,
			DeletedLines:     item.DeletedLines,
			BeforeHash:       item.BeforeHash,
			AfterHash:        item.AfterHash,
			CreatedAt:        item.CreatedAt,
			AppliedAt:        item.AppliedAt,
			RevertedAt:       item.RevertedAt,
		})
	}
	return ret
}

func memoryRecallTraceInfosFromClient(items []client.MemoryRecallTrace) []MemoryRecallTraceInfo {
	ret := make([]MemoryRecallTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryRecallTraceInfo{
			ID:                  common.MemoryRecallTraceID(item.ID),
			WorkspaceID:         common.WorkspaceID(item.WorkspaceID),
			RunID:               common.RunID(item.RunID),
			StepID:              common.StepID(item.StepID),
			GraphNodeID:         item.GraphNodeID,
			Stage:               item.Stage,
			Source:              item.Source,
			Policy:              item.Policy,
			Status:              item.Status,
			ProviderID:          item.ProviderID,
			Operation:           item.Operation,
			QueryTextPreview:    item.QueryTextPreview,
			QueryTextSha256:     item.QueryTextSha256,
			MinScore:            item.MinScore,
			Limit:               item.Limit,
			EmbeddingModel:      item.EmbeddingModel,
			VectorDimension:     item.VectorDimension,
			RetrievedMemoryIDs:  memoryIDsFromClient(item.RetrievedMemoryIDs),
			RetrievedMemoryHits: memoryRecallHitInfosFromClient(item.RetrievedMemoryHits),
			Reason:              item.Reason,
			OccurredAt:          item.OccurredAt,
			EvidenceRef:         item.EvidenceRef,
		})
	}
	return ret
}

func memoryRecallHitInfosFromClient(items []client.MemoryRecallHit) []MemoryRecallHitInfo {
	ret := make([]MemoryRecallHitInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryRecallHitInfo{
			ID:              common.MemoryID(item.ID),
			Summary:         item.Summary,
			VectorScore:     item.VectorScore,
			VectorDimension: item.VectorDimension,
		})
	}
	return ret
}

func memoryIDsFromClient(values []string) []common.MemoryID {
	ret := make([]common.MemoryID, 0, len(values))
	for _, value := range values {
		ret = append(ret, common.MemoryID(value))
	}
	return ret
}

func llmContextBudgetTraceInfosFromClient(items []client.LLMContextBudgetTrace) []LLMContextBudgetTraceInfo {
	ret := make([]LLMContextBudgetTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, LLMContextBudgetTraceInfo{
			StepID:                common.StepID(item.StepID),
			ProviderID:            item.ProviderID,
			Model:                 item.Model,
			MaxBytes:              item.MaxBytes,
			ActualBytes:           item.ActualBytes,
			MaxTokens:             item.MaxTokens,
			EstimatedInputTokens:  item.EstimatedInputTokens,
			EstimatedOutputTokens: item.EstimatedOutputTokens,
			ActualTokens:          item.ActualTokens,
			ShrinkLevel:           item.ShrinkLevel,
			Truncated:             item.Truncated,
			Rules:                 append([]string(nil), item.Rules...),
			SourceRefs:            append([]string(nil), item.SourceRefs...),
			IncludedSections:      append([]string(nil), item.IncludedSections...),
			NormalizedSections:    append([]string(nil), item.NormalizedSections...),
			CompressedSections:    append([]string(nil), item.CompressedSections...),
			OmittedSections:       append([]string(nil), item.OmittedSections...),
			GoalDoDCompressed:     item.GoalDoDCompressed,
			OverBudget:            item.OverBudget,
			P0Overflow:            item.P0Overflow,
			ProfileMode:           item.ProfileMode,
			CapabilitySource:      item.CapabilitySource,
			ContextUtilization:    item.ContextUtilization,
			MaxRequestBytes:       item.MaxRequestBytes,
			StageBudgetRatio:      item.StageBudgetRatio,
			StageBudgetSource:     item.StageBudgetSource,
			TracePolicy:           item.TracePolicy,
			PromptCaptured:        item.PromptCaptured,
			Estimator:             item.Estimator,
			Profile:               clonePayloadMap(item.Profile),
			CachePlan:             clonePayloadMap(item.CachePlan),
		})
	}
	return ret
}

func llmContextBudgetSummaryInfoFromClient(item client.LLMContextBudgetSummary) LLMContextBudgetSummaryInfo {
	return LLMContextBudgetSummaryInfo{
		TraceTotal:             item.TraceTotal,
		TruncatedCount:         item.TruncatedCount,
		OverBudgetCount:        item.OverBudgetCount,
		P0OverflowCount:        item.P0OverflowCount,
		GoalDoDCompressedCount: item.GoalDoDCompressedCount,
		PromptCapturedCount:    item.PromptCapturedCount,
		MaxShrinkLevel:         item.MaxShrinkLevel,
		MaxContextUtilization:  item.MaxContextUtilization,
		AvgContextUtilization:  item.AvgContextUtilization,
		MaxInputTokens:         item.MaxInputTokens,
		MaxRequestBytes:        item.MaxRequestBytes,
		RecentWindowSize:       item.RecentWindowSize,
		RecentTraceTotal:       item.RecentTraceTotal,
		RecentTruncatedCount:   item.RecentTruncatedCount,
		RecentOverBudgetCount:  item.RecentOverBudgetCount,
		RecentMaxUtilization:   item.RecentMaxUtilization,
		RecentAvgUtilization:   item.RecentAvgUtilization,
		RecentMaxInputTokens:   item.RecentMaxInputTokens,
		UtilizationTrend:       item.UtilizationTrend,
	}
}

func llmUsageTraceInfosFromClient(items []client.LLMUsageTrace) []LLMUsageTraceInfo {
	ret := make([]LLMUsageTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, LLMUsageTraceInfo{
			ID:            item.ID,
			WorkspaceID:   common.WorkspaceID(item.WorkspaceID),
			RunID:         common.RunID(item.RunID),
			StepID:        common.StepID(item.StepID),
			ProviderID:    item.ProviderID,
			Provider:      item.Provider,
			Model:         item.Model,
			Operation:     item.Operation,
			InputTokens:   item.InputTokens,
			OutputTokens:  item.OutputTokens,
			TotalTokens:   item.TotalTokens,
			Estimated:     item.Estimated,
			UsageSource:   item.UsageSource,
			RequestBytes:  item.RequestBytes,
			ResponseBytes: item.ResponseBytes,
			Streaming:     item.Streaming,
			ContextBudget: clonePayloadMap(item.ContextBudget),
			CacheUsage:    clonePayloadMap(item.CacheUsage),
			Status:        item.Status,
			FailureClass:  item.FailureClass,
			OccurredAt:    item.OccurredAt,
			EvidenceRef:   item.EvidenceRef,
		})
	}
	return ret
}

func llmUsageSummaryInfoFromClient(item client.LLMUsageSummary) LLMUsageSummaryInfo {
	return LLMUsageSummaryInfo{
		EntryTotal:              item.EntryTotal,
		InputTokens:             item.InputTokens,
		OutputTokens:            item.OutputTokens,
		TotalTokens:             item.TotalTokens,
		EstimatedEntries:        item.EstimatedEntries,
		ProviderNativeEntries:   item.ProviderNativeEntries,
		FailedEntries:           item.FailedEntries,
		CacheReadTokens:         item.CacheReadTokens,
		CacheWriteTokens:        item.CacheWriteTokens,
		CacheCreationTokens:     item.CacheCreationTokens,
		CacheableInputTokens:    item.CacheableInputTokens,
		NonCacheableInputTokens: item.NonCacheableInputTokens,
		CacheEstimatedEntries:   item.CacheEstimatedEntries,
		CacheProviderEntries:    item.CacheProviderEntries,
		ProviderModelBreakdown:  llmUsageProviderModelBreakdownInfosFromClient(item.ProviderModelBreakdown),
	}
}

func llmUsageProviderModelBreakdownInfosFromClient(items []client.LLMUsageProviderModelBreakdown) []LLMUsageProviderModelBreakdownInfo {
	ret := make([]LLMUsageProviderModelBreakdownInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, LLMUsageProviderModelBreakdownInfo{
			ProviderID:       item.ProviderID,
			Provider:         item.Provider,
			Model:            item.Model,
			EntryTotal:       item.EntryTotal,
			InputTokens:      item.InputTokens,
			OutputTokens:     item.OutputTokens,
			TotalTokens:      item.TotalTokens,
			CacheReadTokens:  item.CacheReadTokens,
			CacheWriteTokens: item.CacheWriteTokens,
			EstimatedEntries: item.EstimatedEntries,
			FailedEntries:    item.FailedEntries,
		})
	}
	return ret
}

func eventRecordInfosFromClient(items []client.EventRecord) []EventRecordInfo {
	ret := make([]EventRecordInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, EventRecordInfo{
			ID:             common.EventRecordID(item.ID),
			Version:        item.Version,
			Topic:          item.Topic,
			OccurredAt:     item.OccurredAt,
			SourceModule:   item.SourceModule,
			SubjectType:    item.SubjectType,
			SubjectID:      item.SubjectID,
			WorkspaceID:    common.WorkspaceID(item.WorkspaceID),
			RunID:          common.RunID(item.RunID),
			StepID:         common.StepID(item.StepID),
			Payload:        clonePayloadMap(item.Payload),
			PayloadSummary: clonePayloadMap(item.PayloadSummary),
			PayloadRef:     item.PayloadRef,
		})
	}
	return ret
}

func decisionInfosFromClient(items []client.Decision) []DecisionInfo {
	ret := make([]DecisionInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, DecisionInfo{
			ID:                 common.TicketID(item.ID),
			WorkspaceID:        common.WorkspaceID(item.WorkspaceID),
			RunID:              common.RunID(item.RunID),
			Reason:             item.Reason,
			Question:           item.Question,
			Context:            clonePayloadMap(item.Context),
			State:              item.State,
			ResolutionType:     item.ResolutionType,
			ContinuationAction: item.ContinuationAction,
			ResumeRunID:        common.RunID(item.ResumeRunID),
			SuccessorRunID:     common.RunID(item.SuccessorRunID),
			AutoGovernance:     mapRemoteAutoGovernanceSummary(item.AutoGovernance),
		})
	}
	return ret
}

func approvalInfosFromClient(items []client.Approval) []ApprovalInfo {
	ret := make([]ApprovalInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, ApprovalInfo{
			ID:             common.ApprovalID(item.ID),
			WorkspaceID:    common.WorkspaceID(item.WorkspaceID),
			RunID:          common.RunID(item.RunID),
			Reason:         item.Reason,
			Action:         item.Action,
			Context:        clonePayloadMap(item.Context),
			State:          item.State,
			AutoGovernance: mapRemoteAutoGovernanceSummary(item.AutoGovernance),
		})
	}
	return ret
}

func (b *remoteWorkspaceBackend) Delete(ctx context.Context, id common.WorkspaceID, purge bool) error {
	return b.cl.DeleteWorkspace(ctx, string(id), purge)
}

// ── Remote Goal ──

type remoteGoalBackend struct{ cl *client.Client }

func (b *remoteGoalBackend) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*GoalInfo, error) {
	g, err := b.cl.CreateGoal(ctx, string(wsID), text)
	if err != nil {
		return nil, err
	}
	return &GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}
func (b *remoteGoalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]GoalInfo, error) {
	list, err := b.cl.ListGoals(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]GoalInfo, 0, len(list))
	for _, g := range list {
		result = append(result, GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text})
	}
	return result, nil
}
func (b *remoteGoalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*GoalInfo, error) {
	g, err := b.cl.GetGoal(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}
func (b *remoteGoalBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*GoalInfo, error) {
	g, err := b.cl.UpdateGoal(ctx, string(wsID), string(id), text)
	if err != nil {
		return nil, err
	}
	return &GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}

// ── Remote DoD ──

type remoteDoDBackend struct{ cl *client.Client }

func (b *remoteDoDBackend) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*DoDInfo, error) {
	d, err := b.cl.CreateDoD(ctx, string(wsID), items)
	if err != nil {
		return nil, err
	}
	return &DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}
func (b *remoteDoDBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DoDInfo, error) {
	list, err := b.cl.ListDoDs(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]DoDInfo, 0, len(list))
	for _, d := range list {
		result = append(result, DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State})
	}
	return result, nil
}
func (b *remoteDoDBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*DoDInfo, error) {
	d, err := b.cl.GetDoD(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}
func (b *remoteDoDBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*DoDInfo, error) {
	d, err := b.cl.UpdateDoD(ctx, string(wsID), string(id), items)
	if err != nil {
		return nil, err
	}
	return &DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}

// ── Remote Run ──

type remoteRunBackend struct{ cl *client.Client }

func (b *remoteRunBackend) Command(ctx context.Context, req WorkspaceRunCommand) (*RunInfo, error) {
	intent, err := ValidateWorkspaceRunCommand(req)
	if err != nil {
		return nil, err
	}
	switch intent {
	case RunCommandIntentResume:
		if req.ExpectedRunID == "" {
			return b.resumeWorkspaceExecution(ctx, req.WorkspaceID, mergeActivationRequest(req.Activation, req))
		}
		return b.runCommandViaResume(ctx, req, RunCommandIntentResume)
	case RunCommandIntentRestart:
		return b.restart(ctx, req.WorkspaceID, mergeActivationRequest(req.Activation, req))
	case RunCommandIntentReplan:
		return b.runCommandViaResume(ctx, req, intent)
	default:
		return nil, fmt.Errorf("%w: unsupported run command intent %q", common.ErrInvalidArgument, intent)
	}
}

func (b *remoteRunBackend) runCommandViaResume(ctx context.Context, req WorkspaceRunCommand, intent RunCommandIntent) (*RunInfo, error) {
	activation := mergeActivationRequest(req.Activation, req)
	act, err := b.cl.ResumeWorkspaceExecution(ctx, string(req.WorkspaceID), &client.ActivationRequest{
		RunID:              string(req.ExpectedRunID),
		GoalID:             string(activation.GoalID),
		DoDID:              string(activation.DoDID),
		LLMProviderID:      activation.LLMProviderID,
		SoulID:             string(activation.SoulID),
		SoulBindings:       commonSoulBindingStrings(activation.SoulBindings),
		SkillIDs:           skillIDStrings(activation.SkillIDs),
		MCPServerIDs:       mcpServerIDStrings(activation.MCPServerIDs),
		CapabilityIDs:      capabilityIDStrings(activation.CapabilityIDs),
		MemoryIDs:          memoryIDStrings(activation.MemoryIDs),
		DialogLanguage:     activation.DialogLanguage,
		Priority:           activation.Priority,
		Intent:             string(intent),
		RequestID:          activation.RequestID,
		Reason:             activation.Reason,
		Source:             normalizedRunCommandSource(activation.Source),
		DefinitionRevision: activation.DefinitionRevision,
		DecisionID:         string(activation.DecisionID),
	})
	if err != nil {
		return nil, err
	}
	if act == nil {
		return nil, nil
	}
	return remoteRunInfoFromClientRun(&act.Run), nil
}

func (b *remoteRunBackend) resumeWorkspaceExecution(ctx context.Context, wsID common.WorkspaceID, req ActivateRunRequest) (*RunInfo, error) {
	act, err := b.cl.ResumeWorkspaceExecution(ctx, string(wsID), &client.ActivationRequest{
		GoalID:             string(req.GoalID),
		DoDID:              string(req.DoDID),
		LLMProviderID:      req.LLMProviderID,
		SoulID:             string(req.SoulID),
		SoulBindings:       commonSoulBindingStrings(req.SoulBindings),
		SkillIDs:           skillIDStrings(req.SkillIDs),
		MCPServerIDs:       mcpServerIDStrings(req.MCPServerIDs),
		CapabilityIDs:      capabilityIDStrings(req.CapabilityIDs),
		MemoryIDs:          memoryIDStrings(req.MemoryIDs),
		DialogLanguage:     req.DialogLanguage,
		Priority:           req.Priority,
		RequestID:          req.RequestID,
		Reason:             req.Reason,
		Source:             normalizedRunCommandSource(req.Source),
		Intent:             string(RunCommandIntentResume),
		DefinitionRevision: req.DefinitionRevision,
		DecisionID:         string(req.DecisionID),
	})
	if err != nil {
		return nil, err
	}
	if act == nil {
		return nil, nil
	}
	return &RunInfo{
		ID: common.RunID(act.Run.ID), WorkspaceID: common.WorkspaceID(act.Run.WorkspaceID),
		GoalID: common.GoalID(act.Run.GoalID), DoDID: common.DoDID(act.Run.DoDID),
		State:              common.RunState(act.Run.State),
		BlockingTicketID:   common.TicketID(act.Run.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(act.Run.BlockingApprovalID),
		FailureReason:      act.Run.FailureReason,
	}, nil
}

func (b *remoteRunBackend) restart(ctx context.Context, wsID common.WorkspaceID, req ActivateRunRequest) (*RunInfo, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	act, err := b.cl.ResumeWorkspaceExecution(ctx, string(wsID), &client.ActivationRequest{
		GoalID:             string(req.GoalID),
		DoDID:              string(req.DoDID),
		LLMProviderID:      req.LLMProviderID,
		DialogLanguage:     req.DialogLanguage,
		SoulID:             string(req.SoulID),
		SoulBindings:       commonSoulBindingStrings(req.SoulBindings),
		SkillIDs:           skillIDStrings(req.SkillIDs),
		MCPServerIDs:       mcpServerIDStrings(req.MCPServerIDs),
		CapabilityIDs:      capabilityIDStrings(req.CapabilityIDs),
		MemoryIDs:          memoryIDStrings(req.MemoryIDs),
		Priority:           req.Priority,
		RequestID:          req.RequestID,
		Reason:             req.Reason,
		Source:             normalizedRunCommandSource(req.Source),
		Intent:             string(RunCommandIntentRestart),
		DefinitionRevision: req.DefinitionRevision,
		DecisionID:         string(req.DecisionID),
		Restart:            true,
	})
	if err != nil {
		return nil, err
	}
	if act == nil {
		return nil, nil
	}
	return remoteRunInfoFromClientRun(&act.Run), nil
}

func mergeActivationRequest(base ActivateRunRequest, req WorkspaceRunCommand) ActivateRunRequest {
	if req.Priority != 0 {
		base.Priority = req.Priority
	}
	if req.RequestID != "" {
		base.RequestID = req.RequestID
	}
	if req.Reason != "" {
		base.Reason = req.Reason
	}
	if req.Source != "" {
		base.Source = req.Source
	}
	if req.DefinitionRevision != "" {
		base.DefinitionRevision = req.DefinitionRevision
	}
	if req.DecisionID != "" {
		base.DecisionID = req.DecisionID
	}
	return base
}

func remoteRunInfoFromClientRun(r *client.Run) *RunInfo {
	if r == nil {
		return nil
	}
	return &RunInfo{
		ID: common.RunID(r.ID), WorkspaceID: common.WorkspaceID(r.WorkspaceID),
		GoalID: common.GoalID(r.GoalID), DoDID: common.DoDID(r.DoDID),
		DefinitionRevision: r.DefinitionRevision,
		State:              common.RunState(r.State),
		BlockingTicketID:   common.TicketID(r.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(r.BlockingApprovalID),
		FailureReason:      r.FailureReason,
	}
}
func (b *remoteRunBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunInfo, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	r, err := b.cl.GetRun(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &RunInfo{
		ID: common.RunID(r.ID), WorkspaceID: common.WorkspaceID(r.WorkspaceID),
		GoalID: common.GoalID(r.GoalID), DoDID: common.DoDID(r.DoDID),
		DefinitionRevision: r.DefinitionRevision,
		State:              common.RunState(r.State),
		BlockingTicketID:   common.TicketID(r.BlockingTicketID),
		BlockingApprovalID: common.ApprovalID(r.BlockingApprovalID),
		FailureReason:      r.FailureReason,
	}, nil
}
func (b *remoteRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]RunInfo, error) {
	list, err := b.cl.ListRuns(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]RunInfo, 0, len(list))
	for _, r := range list {
		result = append(result, RunInfo{
			ID: common.RunID(r.ID), WorkspaceID: common.WorkspaceID(r.WorkspaceID),
			GoalID: common.GoalID(r.GoalID), DoDID: common.DoDID(r.DoDID),
			DefinitionRevision: r.DefinitionRevision,
			State:              common.RunState(r.State),
			BlockingTicketID:   common.TicketID(r.BlockingTicketID),
			BlockingApprovalID: common.ApprovalID(r.BlockingApprovalID),
			FailureReason:      r.FailureReason,
		})
	}
	return result, nil
}
func (b *remoteRunBackend) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunStatusInfo, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	s, err := b.cl.GetRunStatus(ctx, string(wsID), string(id), "", 0)
	if err != nil {
		return nil, err
	}
	return &RunStatusInfo{
		ID: common.RunID(s.ID), WorkspaceID: common.WorkspaceID(s.WorkspaceID),
		State: common.RunState(s.State), StepCount: s.StepCount,
		CurrentStepID:      common.StepID(s.CurrentStepID),
		CurrentStepKind:    canonicalRuntimeStepKind(s.CurrentStepKind),
		CurrentStepState:   common.StepState(s.CurrentStepState),
		CurrentActionLabel: s.CurrentActionLabel,
		DiagnosticLineage:  common.StepID(s.DiagnosticLineage),
		DecisionCount:      s.DecisionCount, ApprovalCount: s.ApprovalCount,
		BlockingTicketID:     common.TicketID(s.BlockingTicketID),
		BlockingApprovalID:   common.ApprovalID(s.BlockingApprovalID),
		FailureReason:        s.FailureReason,
		ConvergenceStatus:    convergenceStatusInfoFromClient(s.ConvergenceStatus),
		SchedulerDiagnostics: schedulerDiagnosticsInfoFromClient(s.SchedulerDiagnostics),
	}, nil
}

func convergenceStatusInfoFromClient(input *client.ConvergenceStatus) *ConvergenceStatusInfo {
	if input == nil {
		return nil
	}
	return &ConvergenceStatusInfo{
		WorkspaceID:               common.WorkspaceID(input.WorkspaceID),
		RunID:                     common.RunID(input.RunID),
		RunState:                  common.RunState(input.RunState),
		CurrentFocusID:            input.CurrentFocusID,
		NextActionKind:            input.NextActionKind,
		CurrentGap:                input.CurrentGap,
		MissingEvidenceKinds:      append([]string(nil), input.MissingEvidenceKinds...),
		AcceptedEvidenceCount:     input.AcceptedEvidenceCount,
		RejectedEvidenceCount:     input.RejectedEvidenceCount,
		LatestEffectiveProgressAt: input.LatestEffectiveProgressAt,
		DecisionCause:             input.DecisionCause,
		Stalled:                   input.Stalled,
		IneffectiveRetry:          input.IneffectiveRetry,
	}
}

func schedulerDiagnosticsInfoFromClient(input *client.SchedulerDiagnostics) *SchedulerDiagnosticsInfo {
	if input == nil {
		return nil
	}
	ret := &SchedulerDiagnosticsInfo{
		Watchdog: WatchdogDiagnosticsInfo{
			TimerRegistrations: input.Watchdog.TimerRegistrations,
			LatestTickAt:       input.Watchdog.LatestTickAt,
			LatestDurationMs:   input.Watchdog.LatestDurationMs,
			LatestError:        input.Watchdog.LatestError,
			StaleRecoveryRuns:  input.Watchdog.StaleRecoveryRuns,
		},
		RecentContinuations:         continuationDiagnosticsFromClient(input.RecentContinuations),
		ContinuationFailureCounts:   continuationDiagnosticCountsFromClient(input.ContinuationFailureCounts),
		ContinuationNoActionCounts:  continuationDiagnosticCountsFromClient(input.ContinuationNoActionCounts),
		ContinuationCompletedCounts: continuationDiagnosticCountsFromClient(input.ContinuationCompletedCounts),
	}
	if input.LatestContinuation != nil {
		latest := continuationDiagnosticFromClient(*input.LatestContinuation)
		ret.LatestContinuation = &latest
	}
	return ret
}

func continuationDiagnosticsFromClient(input []client.ContinuationDiagnostic) []ContinuationDiagnosticInfo {
	if len(input) == 0 {
		return nil
	}
	ret := make([]ContinuationDiagnosticInfo, 0, len(input))
	for _, item := range input {
		ret = append(ret, continuationDiagnosticFromClient(item))
	}
	return ret
}

func continuationDiagnosticFromClient(input client.ContinuationDiagnostic) ContinuationDiagnosticInfo {
	return ContinuationDiagnosticInfo{
		WorkspaceID:        input.WorkspaceID,
		RunID:              input.RunID,
		Kind:               input.Kind,
		State:              input.State,
		Reason:             input.Reason,
		Detail:             input.Detail,
		Readback:           continuationReadbackDiagnosticFromClient(input.Readback),
		Recoverability:     input.Recoverability,
		RecommendedActions: append([]string(nil), input.RecommendedActions...),
		DedupeKey:          input.DedupeKey,
		Attempt:            input.Attempt,
		OccurredAt:         input.OccurredAt,
		Source:             input.Source,
	}
}

func continuationReadbackDiagnosticFromClient(input *client.ContinuationReadbackDiagnostic) *ContinuationReadbackDiagnosticInfo {
	if input == nil {
		return nil
	}
	return &ContinuationReadbackDiagnosticInfo{
		WorkspaceID:             input.WorkspaceID,
		RunID:                   input.RunID,
		Action:                  input.Action,
		Found:                   input.Found,
		OldRunState:             input.OldRunState,
		NewRunState:             input.NewRunState,
		OldBlockingTicketID:     input.OldBlockingTicketID,
		NewBlockingTicketID:     input.NewBlockingTicketID,
		OldBlockingApprovalID:   input.OldBlockingApprovalID,
		NewBlockingApprovalID:   input.NewBlockingApprovalID,
		SuccessorRunID:          input.SuccessorRunID,
		LatestStepID:            input.LatestStepID,
		Terminal:                input.Terminal,
		GovernedWait:            input.GovernedWait,
		Advanced:                input.Advanced,
		Reason:                  input.Reason,
		ProjectionRevision:      input.ProjectionRevision,
		SourceEventRevision:     input.SourceEventRevision,
		ProjectedSourceRevision: input.ProjectedSourceRevision,
	}
}

func continuationDiagnosticCountsFromClient(input []client.ContinuationDiagnosticCount) []ContinuationDiagnosticCountInfo {
	if len(input) == 0 {
		return nil
	}
	ret := make([]ContinuationDiagnosticCountInfo, 0, len(input))
	for _, item := range input {
		ret = append(ret, ContinuationDiagnosticCountInfo{
			Kind:   item.Kind,
			Reason: item.Reason,
			Count:  item.Count,
		})
	}
	return ret
}

// ── Remote Decision ──

type remoteDecisionBackend struct{ cl *client.Client }

func (b *remoteDecisionBackend) Resolve(ctx context.Context, req ResolveDecisionRequest) (*DecisionInfo, error) {
	d, err := b.cl.ResolveDecision(ctx, string(req.WorkspaceID), string(req.TicketID), req.ResolutionType, req.Payload)
	if err != nil {
		return nil, err
	}
	item := mapRemoteDecisionInfoPtr(d)
	return &item, nil
}
func (b *remoteDecisionBackend) BatchResolve(ctx context.Context, workspaceID common.WorkspaceID, ticketIDs []common.TicketID, resolutionType string, payload map[string]any) ([]DecisionInfo, error) {
	ids := make([]string, len(ticketIDs))
	for i, id := range ticketIDs {
		ids[i] = string(id)
	}
	list, err := b.cl.BatchResolveDecisions(ctx, string(workspaceID), ids, resolutionType, payload)
	if err != nil {
		return nil, err
	}
	result := make([]DecisionInfo, 0, len(list))
	for _, d := range list {
		result = append(result, mapRemoteDecisionInfo(d))
	}
	return result, nil
}
func (b *remoteDecisionBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]DecisionInfo, error) {
	list, err := b.cl.ListDecisions(ctx, string(wsID), "")
	if err != nil {
		return nil, err
	}
	result := make([]DecisionInfo, 0, len(list))
	for _, d := range list {
		result = append(result, mapRemoteDecisionInfo(d))
	}
	return result, nil
}
func (b *remoteDecisionBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*DecisionInfo, error) {
	d, err := b.cl.GetDecision(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	item := mapRemoteDecisionInfoPtr(d)
	return &item, nil
}

func mapRemoteDecisionInfoPtr(d *client.Decision) DecisionInfo {
	if d == nil {
		return DecisionInfo{}
	}
	return mapRemoteDecisionInfo(*d)
}

func mapRemoteDecisionInfo(d client.Decision) DecisionInfo {
	return DecisionInfo{
		ID:                 common.TicketID(d.ID),
		WorkspaceID:        common.WorkspaceID(d.WorkspaceID),
		RunID:              common.RunID(d.RunID),
		Reason:             d.Reason,
		Question:           d.Question,
		Context:            clonePayloadMap(d.Context),
		State:              d.State,
		ResolutionType:     d.ResolutionType,
		ContinuationAction: d.ContinuationAction,
		ResumeRunID:        common.RunID(d.ResumeRunID),
		SuccessorRunID:     common.RunID(d.SuccessorRunID),
		AutoGovernance:     mapRemoteAutoGovernanceSummary(d.AutoGovernance),
	}
}

func clonePayloadMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func clonePayloadMapSlice(input []map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, clonePayloadMap(item))
	}
	return ret
}

// ── Remote Approval ──

type remoteApprovalBackend struct{ cl *client.Client }

func (b *remoteApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...map[string]any) (*ApprovalInfo, error) {
	a, err := b.cl.Approve(ctx, string(wsID), string(id), payload...)
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)}, nil
}
func (b *remoteApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...map[string]any) (*ApprovalInfo, error) {
	a, err := b.cl.Reject(ctx, string(wsID), string(id), payload...)
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)}, nil
}
func (b *remoteApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...map[string]any) ([]ApprovalInfo, error) {
	_idStrings := make([]string, len(ids))
	for i, id := range ids {
		_idStrings[i] = string(id)
	}
	list, err := b.cl.BatchResolveApprovals(ctx, string(wsID), _idStrings, action, payload...)
	if err != nil {
		return nil, err
	}
	result := make([]ApprovalInfo, 0, len(list))
	for _, a := range list {
		result = append(result, ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)})
	}
	return result, nil
}
func (b *remoteApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]ApprovalInfo, error) {
	list, err := b.cl.ListApprovals(ctx, string(wsID), "")
	if err != nil {
		return nil, err
	}
	result := make([]ApprovalInfo, 0, len(list))
	for _, a := range list {
		result = append(result, ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)})
	}
	return result, nil
}
func (b *remoteApprovalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*ApprovalInfo, error) {
	a, err := b.cl.GetApproval(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)}, nil
}

func mapRemoteAutoGovernanceSummary(summary *client.AutoGovernanceSummary) *AutoGovernanceSummaryInfo {
	if summary == nil {
		return nil
	}
	ret := &AutoGovernanceSummaryInfo{
		TraceID:           summary.TraceID,
		Action:            summary.Action,
		Status:            summary.Status,
		RiskLevel:         summary.RiskLevel,
		PolicyMatched:     summary.PolicyMatched,
		PolicyBlocked:     summary.PolicyBlocked,
		Confidence:        summary.Confidence,
		Reason:            summary.Reason,
		PromptFingerprint: summary.PromptFingerprint,
		OccurredAt:        summary.OccurredAt,
	}
	if len(summary.EvidenceRefs) > 0 {
		ret.EvidenceRefs = append([]string(nil), summary.EvidenceRefs...)
	}
	return ret
}

// ── Stub remote backends ──

type remoteSoulBackend struct{ cl *client.Client }

func (b *remoteSoulBackend) Create(ctx context.Context, s *SoulInfo) (*SoulInfo, error) {
	return s, nil
}
func (b *remoteSoulBackend) Get(ctx context.Context, id common.SoulID) (*SoulInfo, error) {
	return &SoulInfo{ID: id}, nil
}
func (b *remoteSoulBackend) List(ctx context.Context) ([]SoulInfo, error) { return nil, nil }
func (b *remoteSoulBackend) Update(ctx context.Context, id common.SoulID, s *SoulInfo) (*SoulInfo, error) {
	return s, nil
}
func (b *remoteSoulBackend) Delete(ctx context.Context, id common.SoulID) error { return nil }

type remoteSkillBackend struct{ cl *client.Client }

func (b *remoteSkillBackend) Create(ctx context.Context, s *SkillInfo) (*SkillInfo, error) {
	return s, nil
}
func (b *remoteSkillBackend) Get(ctx context.Context, id common.SkillID) (*SkillInfo, error) {
	return &SkillInfo{ID: id}, nil
}
func (b *remoteSkillBackend) List(ctx context.Context) ([]SkillInfo, error) { return nil, nil }

type remoteMCPBackend struct{ cl *client.Client }

func (b *remoteMCPBackend) CreateServer(ctx context.Context, s *MCPServerInfo) (*MCPServerInfo, error) {
	return s, nil
}
func (b *remoteMCPBackend) GetServer(ctx context.Context, id common.MCPServerID) (*MCPServerInfo, error) {
	return &MCPServerInfo{ID: id}, nil
}
func (b *remoteMCPBackend) ListServers(ctx context.Context) ([]MCPServerInfo, error) { return nil, nil }
func (b *remoteMCPBackend) UpdateServer(ctx context.Context, id common.MCPServerID, s *MCPServerInfo) (*MCPServerInfo, error) {
	return s, nil
}
func (b *remoteMCPBackend) ListCapabilities(ctx context.Context) ([]MCPCapabilityInfo, error) {
	return nil, nil
}

type remoteMemoryBackend struct{ cl *client.Client }

func (b *remoteMemoryBackend) Create(ctx context.Context, wsID common.WorkspaceID, m *MemoryItemInfo) (*MemoryItemInfo, error) {
	item, err := b.cl.CreateMemory(ctx, string(wsID), clientMemoryFromInfo(m))
	if err != nil {
		return nil, err
	}
	return memoryInfoFromClient(item), nil
}
func (b *remoteMemoryBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*MemoryItemInfo, error) {
	item, err := b.cl.GetMemory(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return memoryInfoFromClient(item), nil
}
func (b *remoteMemoryBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *MemoryItemInfo) (*MemoryItemInfo, error) {
	item, err := b.cl.UpdateMemory(ctx, string(wsID), string(id), clientMemoryFromInfo(m))
	if err != nil {
		return nil, err
	}
	return memoryInfoFromClient(item), nil
}
func (b *remoteMemoryBackend) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	return b.cl.DeleteMemory(ctx, string(wsID), string(id))
}
func (b *remoteMemoryBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]MemoryItemInfo, error) {
	return b.Search(ctx, wsID, MemorySearchRequest{})
}
func (b *remoteMemoryBackend) Search(ctx context.Context, wsID common.WorkspaceID, req MemorySearchRequest) ([]MemoryItemInfo, error) {
	items, err := b.cl.SearchMemory(ctx, client.MemorySearchQuery{
		WorkspaceID: string(wsID),
		Query:       req.Query,
		Tags:        append([]string(nil), req.Tags...),
		Types:       append([]string(nil), req.Types...),
		Limit:       req.Limit,
	})
	if err != nil {
		return nil, err
	}
	ret := make([]MemoryItemInfo, 0, len(items))
	for _, item := range items {
		info := memoryInfoFromClient(&item)
		ret = append(ret, *info)
	}
	return ret, nil
}

func clientMemoryFromInfo(info *MemoryItemInfo) *client.MemoryItem {
	if info == nil {
		return &client.MemoryItem{}
	}
	return &client.MemoryItem{
		ID:                  string(info.ID),
		WorkspaceID:         string(info.WorkspaceID),
		RunID:               string(info.RunID),
		SourceType:          info.SourceType,
		SourceRef:           info.SourceRef,
		MemoryType:          info.MemoryType,
		Content:             info.Content,
		Summary:             info.Summary,
		Tags:                append([]string(nil), info.Tags...),
		Importance:          info.Importance,
		Confidence:          info.Confidence,
		Visibility:          info.Visibility,
		EmbeddingProviderID: info.EmbeddingProviderID,
		EmbeddingModel:      info.EmbeddingModel,
		EmbeddingText:       info.EmbeddingText,
		Vector:              append([]float64(nil), info.Vector...),
	}
}

func memoryInfoFromClient(item *client.MemoryItem) *MemoryItemInfo {
	if item == nil {
		return nil
	}
	return &MemoryItemInfo{
		ID:                  common.MemoryID(item.ID),
		WorkspaceID:         common.WorkspaceID(item.WorkspaceID),
		RunID:               common.RunID(item.RunID),
		SourceType:          item.SourceType,
		SourceRef:           item.SourceRef,
		MemoryType:          item.MemoryType,
		Content:             item.Content,
		Summary:             item.Summary,
		Tags:                append([]string(nil), item.Tags...),
		Importance:          item.Importance,
		Confidence:          item.Confidence,
		Visibility:          item.Visibility,
		EmbeddingProviderID: item.EmbeddingProviderID,
		EmbeddingModel:      item.EmbeddingModel,
		EmbeddingText:       item.EmbeddingText,
		VectorReady:         item.VectorReady,
		VectorDimension:     item.VectorDimension,
		VectorScore:         item.VectorScore,
		Vector:              append([]float64(nil), item.Vector...),
	}
}

type remoteTemplateBackend struct{ cl *client.Client }

func (b *remoteTemplateBackend) Create(ctx context.Context, t *TemplateInfo) (*TemplateInfo, error) {
	item, err := b.cl.CreateWorkspaceTemplate(ctx, clientTemplateFromInfo(t))
	if err != nil {
		return nil, err
	}
	return templateInfoFromClient(item), nil
}
func (b *remoteTemplateBackend) Get(ctx context.Context, id string) (*TemplateInfo, error) {
	item, err := b.cl.GetWorkspaceTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	return templateInfoFromClient(item), nil
}
func (b *remoteTemplateBackend) List(ctx context.Context) ([]TemplateInfo, error) {
	items, err := b.cl.ListWorkspaceTemplates(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]TemplateInfo, 0, len(items))
	for i := range items {
		if info := templateInfoFromClient(&items[i]); info != nil {
			ret = append(ret, *info)
		}
	}
	return ret, nil
}
func (b *remoteTemplateBackend) Update(ctx context.Context, id string, t *TemplateInfo) (*TemplateInfo, error) {
	item, err := b.cl.UpdateWorkspaceTemplate(ctx, id, clientTemplateFromInfo(t))
	if err != nil {
		return nil, err
	}
	return templateInfoFromClient(item), nil
}
func (b *remoteTemplateBackend) Delete(ctx context.Context, id string) error {
	return b.cl.DeleteWorkspaceTemplate(ctx, id)
}

func clientTemplateFromInfo(info *TemplateInfo) *client.WorkspaceTemplate {
	if info == nil {
		return &client.WorkspaceTemplate{}
	}
	return &client.WorkspaceTemplate{
		ID:               info.ID,
		Name:             info.Name,
		Description:      info.Description,
		GoalText:         info.GoalText,
		DoDItems:         append([]string(nil), info.DoDItems...),
		DefaultSoulID:    string(info.DefaultSoulID),
		SoulBindings:     cloneStringMap(info.SoulBindings),
		EvidencePolicies: cloneTemplateEvidencePolicies(info.EvidencePolicies),
	}
}

func templateInfoFromClient(item *client.WorkspaceTemplate) *TemplateInfo {
	if item == nil {
		return nil
	}
	return &TemplateInfo{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    common.SoulID(item.DefaultSoulID),
		SoulBindings:     cloneStringMap(item.SoulBindings),
		EvidencePolicies: cloneTemplateEvidencePolicies(item.EvidencePolicies),
	}
}

type remoteConfigBackend struct{ cl *client.Client }

func (b *remoteConfigBackend) Get(ctx context.Context) (*ServiceConfig, error) {
	cfg, err := b.cl.GetConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &ServiceConfig{}, nil
	}
	providers := make([]LLMProvider, 0, len(cfg.LLM.Providers))
	for _, item := range cfg.LLM.Providers {
		providers = append(providers, LLMProvider{
			ID:         item.ID,
			Provider:   item.Provider,
			APISurface: item.APISurface,
			BaseURL:    item.BaseURL,
			Model:      item.Model,
			Timeout:    item.Timeout,
			Disabled:   item.Disabled,
			Streaming:  item.Streaming,
		})
	}
	return &ServiceConfig{
		ServerAddr:        cfg.Server.Addr,
		Backend:           cfg.Repository.Backend,
		DataDir:           cfg.Repository.DataDir,
		DefaultProvider:   cfg.LLM.DefaultProviderID,
		MaxConcurrentRuns: cfg.Server.MaxConcurrentRuns,
		Providers:         providers,
		LLMOutput: LLMOutputConfig{
			MaxPatchContentBytes:               cfg.Runtime.LLMOutput.MaxPatchContentBytes,
			MaxGraphBytes:                      cfg.Runtime.LLMOutput.MaxGraphBytes,
			MaxGraphFileWrites:                 cfg.Runtime.LLMOutput.MaxGraphFileWrites,
			SchemaInvalidLargePreviewThreshold: cfg.Runtime.LLMOutput.SchemaInvalidLargePreviewThreshold,
			MaxArtifactChunkBytes:              cfg.Runtime.LLMOutput.MaxArtifactChunkBytes,
			PreviousChunkTailBytes:             cfg.Runtime.LLMOutput.PreviousChunkTailBytes,
			MaxChunkRetryAttempts:              cfg.Runtime.LLMOutput.MaxChunkRetryAttempts,
			MaxArtifactPlanAttempts:            cfg.Runtime.LLMOutput.MaxArtifactPlanAttempts,
			MaxArtifactReplanAttempts:          cfg.Runtime.LLMOutput.MaxArtifactReplanAttempts,
		},
	}, nil
}

type remoteEventsBackend struct {
	cl          *client.Client
	streamView  string
	mu          sync.Mutex
	lastEventID string
}

func (b *remoteEventsBackend) Subscribe(ctx context.Context, handler EventHandler) (func(), error) {
	if b == nil || b.cl == nil || handler == nil {
		return func() {}, nil
	}
	streamCtx, cancel := context.WithCancel(ctx)
	var once sync.Once
	go b.streamEvents(streamCtx, handler)
	return func() {
		once.Do(cancel)
	}, nil
}

func (b *remoteEventsBackend) streamEvents(ctx context.Context, handler EventHandler) {
	backoff := 200 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return
		}
		connected := b.streamEventsOnce(ctx, handler)
		if ctx.Err() != nil {
			return
		}
		if connected {
			backoff = 200 * time.Millisecond
		} else if backoff < 2*time.Second {
			backoff *= 2
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (b *remoteEventsBackend) streamEventsOnce(ctx context.Context, handler EventHandler) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.cl.BaseURL()+"/api/v1/workorch/events/stream", nil)
	if err != nil {
		return false
	}
	query := req.URL.Query()
	if streamView := strings.TrimSpace(b.streamView); streamView != "" {
		query.Set("view", streamView)
	}
	if lastEventID := b.lastEventIDSnapshot(); lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
		query.Set("lastEventId", lastEventID)
	}
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Accept", "text/event-stream")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false
	}
	handler(ctx, Event{
		Type: EventWorkspaceListChanged,
		Payload: map[string]any{
			"reason": "event_stream_connected",
		},
	})
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	var dataLines []string
	var eventID string
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return true
		default:
		}
		line := scanner.Text()
		if line == "" {
			b.dispatchSSEData(ctx, dataLines, eventID, handler)
			dataLines = nil
			eventID = ""
			continue
		}
		if strings.HasPrefix(line, "id:") {
			eventID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return true
}

func (b *remoteEventsBackend) dispatchSSEData(ctx context.Context, lines []string, eventID string, handler EventHandler) {
	if len(lines) == 0 {
		return
	}
	var envelope backendEventData
	if err := json.Unmarshal([]byte(strings.Join(lines, "\n")), &envelope); err != nil {
		return
	}
	evt := convertBackendEvent(envelope)
	if evt.Type == "" {
		return
	}
	if evt.ID == "" {
		evt.ID = strings.TrimSpace(eventID)
	}
	b.rememberLastEventID(evt.ID)
	handler(ctx, evt)
}

func (b *remoteEventsBackend) lastEventIDSnapshot() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastEventID
}

func (b *remoteEventsBackend) rememberLastEventID(id string) {
	if b == nil || strings.TrimSpace(id) == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastEventID = strings.TrimSpace(id)
}

// Ensure remoteBackend implements Backend.
var _ Backend = (*remoteBackend)(nil)
