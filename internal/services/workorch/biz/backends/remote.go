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
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchpayloadmapper"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// remoteBackend implements port.Backend using an HTTP client to a running workorch --server.
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

// NewRemote creates a port.Backend that communicates with a remote workorch --server via HTTP.
func NewRemote(addr string) port.Backend {
	return NewRemoteWithOptions(addr)
}

func NewRemoteProjectionEvents(addr string) port.Backend {
	return NewRemoteWithOptions(addr, WithRemoteEventStreamView("projection"))
}

func NewRemoteWithOptions(addr string, opts ...RemoteOption) port.Backend {
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

func (b *remoteBackend) Workspace() port.WorkspaceBackend { return b.ws }
func (b *remoteBackend) Goal() port.GoalBackend           { return b.goal }
func (b *remoteBackend) DoD() port.DoDBackend             { return b.dod }
func (b *remoteBackend) Run() port.RunBackend             { return b.run }
func (b *remoteBackend) Decision() port.DecisionBackend   { return b.dec }
func (b *remoteBackend) Approval() port.ApprovalBackend   { return b.appr }
func (b *remoteBackend) Soul() port.SoulBackend           { return b.soul }
func (b *remoteBackend) Skill() port.SkillBackend         { return b.sk }
func (b *remoteBackend) MCP() port.MCPBackend             { return b.mcp }
func (b *remoteBackend) Memory() port.MemoryBackend       { return b.mem }
func (b *remoteBackend) Template() port.TemplateBackend   { return b.tmpl }
func (b *remoteBackend) Config() port.ConfigBackend       { return b.cfg }
func (b *remoteBackend) Events() port.EventsBackend       { return b.evt }
func (b *remoteBackend) ProjectionEvents() port.WorkspaceProjectionEventsBackend {
	return &remoteProjectionEventsBackend{cl: b.cl}
}

// ── Remote Workspace ──

type remoteWorkspaceBackend struct{ cl *client.Client }

func workspaceInfoFromClient(ws *client.Workspace) port.WorkspaceInfo {
	if ws == nil {
		return port.WorkspaceInfo{}
	}
	return port.WorkspaceInfo{
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
		DefaultLLMTarget:     runtimeProviderTargetFromClient(ws.DefaultLLMTarget),
		DefaultSoulID:        common.SoulID(ws.DefaultSoulID),
		SoulBindings:         commonSoulBindings(ws.SoulBindings),
		ContextHints:         payloadFromRemoteMap(ws.ContextHints),
		EvidencePolicies:     payloadsFromRemoteMaps(ws.EvidencePolicies),
		Projection:           projectionInfoFromClient(ws.Projection),
		ViewModel:            workspaceViewModelInfoFromClient(ws.ViewModel),
	}
}

func runtimeProviderTargetFromClient(target *runtimecontract.ProviderModelTarget) *runtimecontract.ProviderModelTarget {
	if target == nil {
		return nil
	}
	value := target.Normalized()
	return &value
}

func projectionInfoFromClient(input client.ProjectionStatus) port.ProjectionInfo {
	return port.ProjectionInfo{
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

func workspaceViewModelInfoFromClient(input client.WorkspaceViewModel) port.WorkspaceViewModelInfo {
	ret := port.WorkspaceViewModelInfo{
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
		ret.Sections = make(map[string]port.WorkspaceViewModelSectionInfo, len(input.Sections))
		for key, value := range input.Sections {
			ret.Sections[key] = port.WorkspaceViewModelSectionInfo{
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

func (b *remoteWorkspaceBackend) Create(ctx context.Context, req port.CreateWorkspaceRequest) (*port.WorkspaceInfo, error) {
	ws, err := b.cl.CreateWorkspace(ctx, client.CreateWorkspaceRequest{
		Title:                req.Title,
		Description:          req.Description,
		DeliverableDirectory: req.DeliverableDirectory,
		TemplateID:           req.TemplateID,
	})
	if err != nil {
		return nil, err
	}
	info := workspaceInfoFromClient(ws)
	return &info, nil
}

func (b *remoteWorkspaceBackend) Get(ctx context.Context, id common.WorkspaceID) (*port.WorkspaceInfo, error) {
	ws, err := b.cl.GetWorkspace(ctx, string(id))
	if err != nil {
		return nil, err
	}
	info := workspaceInfoFromClient(ws)
	return &info, nil
}

func (b *remoteWorkspaceBackend) List(ctx context.Context) ([]port.WorkspaceInfo, error) {
	projection, err := b.cl.ListWorkspaceProjection(ctx)
	if err != nil {
		return nil, err
	}
	list := projection.Workspaces
	result := make([]port.WorkspaceInfo, 0, len(list))
	for _, ws := range list {
		item := workspaceInfoFromClient(&ws)
		result = append(result, item)
	}
	return result, nil
}

func (b *remoteWorkspaceBackend) Snapshot(ctx context.Context, id common.WorkspaceID) (*port.WorkspaceSnapshotInfo, error) {
	projection, err := b.cl.GetWorkspaceSnapshotProjection(ctx, string(id))
	if err != nil {
		return nil, err
	}
	snapshot := workspaceSnapshotInfoFromClient(projection.Snapshot)
	snapshot.Projection = projectionInfoFromClient(projection.Projection)
	snapshot.Workspace.Projection = snapshot.Projection
	return &snapshot, nil
}

func (b *remoteWorkspaceBackend) RuntimeView(ctx context.Context, req port.ReadWorkspaceRuntimeViewRequest) (*port.WorkspaceRuntimeViewInfo, error) {
	if b == nil || b.cl == nil {
		return nil, common.ErrInvalidArgument
	}
	if strings.TrimSpace(string(req.WorkspaceID)) == "" {
		return nil, common.ErrInvalidArgument
	}
	projection, err := b.cl.GetWorkspaceRuntimeView(ctx, string(req.WorkspaceID), req.ActivityLimit, req.BeforeCursor)
	if err != nil {
		return nil, err
	}
	view := workspaceRuntimeViewInfoFromClient(projection.RuntimeView)
	// Prefer envelope projection metadata when present.
	if status := projectionInfoFromClient(projection.Projection); status.WorkspaceRevision != 0 || status.GlobalRevision != 0 || status.State != "" {
		view.Projection = status
	}
	return &view, nil
}

func (b *remoteWorkspaceBackend) UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*port.WorkspaceInfo, error) {
	if b == nil || b.cl == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	var (
		ws  *client.Workspace
		err error
	)
	if target == nil {
		ws, err = b.cl.ClearWorkspaceLLMTarget(ctx, string(id))
	} else {
		value := target.Normalized()
		if err := value.Validate(); err != nil {
			return nil, err
		}
		ws, err = b.cl.UpdateWorkspaceLLMTarget(ctx, string(id), value)
	}
	if err != nil {
		return nil, err
	}
	info := workspaceInfoFromClient(ws)
	return &info, nil
}

func workspaceRuntimeViewInfoFromClient(input client.WorkspaceRuntimeView) port.WorkspaceRuntimeViewInfo {
	view := port.WorkspaceRuntimeViewInfo{
		WorkspaceID:       common.WorkspaceID(input.WorkspaceID),
		ActiveRunID:       common.RunID(input.ActiveRunID),
		DisplayRunID:      common.RunID(input.DisplayRunID),
		Projection:        projectionInfoFromWorkspaceViewModel(input.Projection),
		Revision:          input.Revision,
		PromptMode:        input.PromptMode,
		RunState:          common.RunState(input.RunState),
		TaskProgress:      taskProgressInfoFromClient(input.TaskProgress),
		RunHeartbeat:      runHeartbeatInfoFromClient(input.RunHeartbeat),
		ConvergenceStatus: convergenceStatusInfoFromClient(input.ConvergenceStatus),
		Acceptance:        acceptanceProjectionInfoFromClient(input.Acceptance),
		RunCommand:        runCommandStatusInfoFromClient(input.RunCommand),
		Outcome:           workspaceOutcomeInfoFromClient(input.Outcome),
		Activity:          workspaceActivityFeedInfoFromClient(input.Activity),
	}
	if input.BlockingDecision != nil {
		decision := decisionInfoFromClient(*input.BlockingDecision)
		view.BlockingDecision = &decision
	}
	if input.BlockingApproval != nil {
		approval := approvalInfoFromClient(*input.BlockingApproval)
		view.BlockingApproval = &approval
	}
	return view
}

func projectionInfoFromWorkspaceViewModel(input client.WorkspaceViewModel) port.ProjectionInfo {
	return port.ProjectionInfo{
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

func workspaceActivityFeedInfoFromClient(input client.WorkspaceActivityFeed) port.WorkspaceActivityFeedInfo {
	return port.WorkspaceActivityFeedInfo{
		WorkspaceID:      common.WorkspaceID(input.WorkspaceID),
		DisplayRunID:     common.RunID(input.DisplayRunID),
		ActivityRevision: input.ActivityRevision,
		Items:            workspaceActivityItemInfosFromClient(input.Items),
		Truncated:        input.Truncated,
		NextCursor:       input.NextCursor,
	}
}

func workspaceActivityItemInfosFromClient(items []client.WorkspaceActivityItem) []port.WorkspaceActivityItemInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]port.WorkspaceActivityItemInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.WorkspaceActivityItemInfo{
			ID:                   item.ID,
			WorkspaceID:          common.WorkspaceID(item.WorkspaceID),
			RunID:                common.RunID(item.RunID),
			StepID:               common.StepID(item.StepID),
			Kind:                 port.WorkspaceActivityKind(item.Kind),
			State:                item.State,
			Title:                item.Title,
			Summary:              item.Summary,
			OccurredAt:           item.OccurredAt,
			SourceEventRevision:  item.SourceEventRevision,
			RuntimeStateRevision: item.RuntimeStateRevision,
			EvidenceSequence:     item.EvidenceSequence,
		})
	}
	return ret
}

func decisionInfoFromClient(item client.Decision) port.DecisionInfo {
	return port.DecisionInfo{
		ID:                     common.TicketID(item.ID),
		WorkspaceID:            common.WorkspaceID(item.WorkspaceID),
		RunID:                  common.RunID(item.RunID),
		Reason:                 item.Reason,
		Question:               item.Question,
		Context:                item.Context.Clone(),
		AllowedRecoveryActions: append([]string(nil), item.AllowedRecoveryActions...),
		State:                  item.State,
		ResolutionType:         item.ResolutionType,
		ContinuationAction:     item.ContinuationAction,
		ResumeRunID:            common.RunID(item.ResumeRunID),
		SuccessorRunID:         common.RunID(item.SuccessorRunID),
		AutoGovernance:         mapRemoteAutoGovernanceSummary(item.AutoGovernance),
	}
}

func approvalInfoFromClient(item client.Approval) port.ApprovalInfo {
	return port.ApprovalInfo{
		ID:             common.ApprovalID(item.ID),
		WorkspaceID:    common.WorkspaceID(item.WorkspaceID),
		RunID:          common.RunID(item.RunID),
		Reason:         item.Reason,
		Action:         item.Action,
		Context:        item.Context.Clone(),
		State:          item.State,
		AutoGovernance: mapRemoteAutoGovernanceSummary(item.AutoGovernance),
	}
}

func workspaceSnapshotInfoFromClient(input client.WorkspaceSnapshot) port.WorkspaceSnapshotInfo {
	return port.WorkspaceSnapshotInfo{
		Workspace:               workspaceInfoFromClient(&input.Workspace),
		ViewModel:               workspaceViewModelInfoFromClient(input.ViewModel),
		Projection:              projectionInfoFromClient(input.Workspace.Projection),
		Outcome:                 workspaceOutcomeInfoFromClient(input.Outcome),
		TaskProgress:            taskProgressInfoFromClient(input.TaskProgress),
		RunHeartbeat:            runHeartbeatInfoFromClient(input.RunHeartbeat),
		ConvergenceStatus:       convergenceStatusInfoFromClient(input.ConvergenceStatus),
		RunCommand:              runCommandStatusInfoFromClient(input.RunCommand),
		SchedulerDiagnostics:    schedulerDiagnosticsInfoFromClient(input.SchedulerDiagnostics),
		ContextBundle:           payloadFromRemoteMap(input.ContextBundle),
		CoverageSummary:         payloadFromRemoteMap(input.CoverageSummary),
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

func goalInfosFromClient(items []workorchmanagementcontract.Goal) []port.GoalInfo {
	ret := make([]port.GoalInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.GoalInfo{ID: common.GoalID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), Text: item.Text})
	}
	return ret
}

func dodInfosFromClient(items []workorchmanagementcontract.DoD) []port.DoDInfo {
	ret := make([]port.DoDInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.DoDInfo{ID: common.DoDID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), Items: append([]string(nil), item.Items...), State: item.State})
	}
	return ret
}

func runInfosFromClient(items []client.Run) []port.RunInfo {
	ret := make([]port.RunInfo, 0, len(items))
	for _, item := range items {
		if mapped := remoteRunInfoFromClientRun(&item); mapped != nil {
			ret = append(ret, *mapped)
		}
	}
	return ret
}

func workspaceOutcomeInfoFromClient(input client.RunOutcome) port.WorkspaceOutcomeInfo {
	return port.WorkspaceOutcomeInfo{
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

func runHeartbeatInfoFromClient(input projectioncontract.RunHeartbeat) port.RunHeartbeatInfo {
	return port.RunHeartbeatInfo{
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

func runCommandStatusInfoFromClient(input *client.RunCommandStatus) *port.RunCommandStatusInfo {
	if input == nil {
		return nil
	}
	return &port.RunCommandStatusInfo{
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

func gapBaselineInfoFromClient(input *projectioncontract.GapBaseline) *port.GapBaselineInfo {
	if input == nil {
		return nil
	}
	ret := &port.GapBaselineInfo{
		WorkspaceID:           common.WorkspaceID(input.WorkspaceID),
		RunID:                 common.RunID(input.RunID),
		DefinitionRevision:    input.DefinitionRevision,
		BaselineRevision:      input.BaselineRevision,
		CurrentFocus:          payloadFromRemoteMap(input.CurrentFocus),
		OpenCoverageWorkItems: payloadsFromRemoteMaps(input.OpenCoverageWorkItems),
		AcceptedEvidenceRefs:  append([]string(nil), input.AcceptedEvidenceRefs...),
		RejectedEvidenceRefs:  append([]string(nil), input.RejectedEvidenceRefs...),
		NextAction:            port.GapNextActionInfo{Kind: input.NextAction.Kind, Reason: input.NextAction.Reason},
	}
	for _, item := range input.TodoItems {
		ret.TodoItems = append(ret.TodoItems, port.GapTodoItemInfo{
			ID:                    item.ID,
			Source:                item.Source,
			SourceRef:             item.SourceRef,
			CoverageWorkItemIDs:   append([]string(nil), item.CoverageWorkItemIDs...),
			RequiredEvidenceKinds: append([]string(nil), item.RequiredEvidenceKinds...),
			Status:                item.Status,
			Priority:              item.Priority,
			Reason:                item.Reason,
			Metadata:              payloadFromRemoteMap(item.Metadata),
		})
	}
	return ret
}

func acceptanceProjectionInfoFromClient(input projectioncontract.AcceptanceProjection) port.AcceptanceProjectionInfo {
	ret := port.AcceptanceProjectionInfo{
		ActiveSliceID: input.ActiveSliceID,
		Summary: port.AcceptanceSummaryInfo{
			AcceptedSlices: input.Summary.AcceptedSlices,
			ActiveSlices:   input.Summary.ActiveSlices,
			OpenSlices:     input.Summary.OpenSlices,
			FailedSlices:   input.Summary.FailedSlices,
		},
	}
	for _, item := range input.Slices {
		ret.Slices = append(ret.Slices, port.AcceptanceSliceInfo{
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
		ret.CurrentFocus = &port.CoverageFocusInfo{
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
		ret.Budget = &port.ConvergenceBudgetInfo{
			FocusID:                      input.Budget.FocusID,
			SliceID:                      input.Budget.SliceID,
			State:                        input.Budget.State,
			Decision:                     input.Budget.Decision,
			MaxRepairAttemptsPerFocus:    input.Budget.MaxRepairAttemptsPerFocus,
			MaxLLMRoundsPerSlice:         input.Budget.MaxLLMRoundsPerSlice,
			MaxRepeatedFailureSignature:  input.Budget.MaxRepeatedFailureSignature,
			MaxValidationOnlyRetries:     input.Budget.MaxValidationOnlyRetries,
			MaxNoCoverageDeltaRounds:     input.Budget.MaxNoCoverageDeltaRounds,
			MaxContextBytesPerRepair:     input.Budget.MaxContextBytesPerRepair,
			RepairAttempts:               input.Budget.RepairAttempts,
			LLMRounds:                    input.Budget.LLMRounds,
			RepeatedFailureSignatures:    input.Budget.RepeatedFailureSignatures,
			ValidationOnlyRetries:        input.Budget.ValidationOnlyRetries,
			NoCoverageDeltaRounds:        input.Budget.NoCoverageDeltaRounds,
			LastNoDeltaReason:            input.Budget.LastNoDeltaReason,
			LastDecisionReason:           input.Budget.LastDecisionReason,
			MaxProviderTransportAttempts: input.Budget.MaxProviderTransportAttempts,
			ProviderTransportAttempts:    input.Budget.ProviderTransportAttempts,
			MaxOperationRecoveryAttempts: input.Budget.MaxOperationRecoveryAttempts,
			OperationRecoveryAttempts:    input.Budget.OperationRecoveryAttempts,
			MaxCASReloadAttempts:         input.Budget.MaxCASReloadAttempts,
			CASReloadAttempts:            input.Budget.CASReloadAttempts,
			MaxTerminalDeliveryAttempts:  input.Budget.MaxTerminalDeliveryAttempts,
			TerminalDeliveryAttempts:     input.Budget.TerminalDeliveryAttempts,
			MaxPrerequisiteRefreshes:     input.Budget.MaxPrerequisiteRefreshes,
			PrerequisiteRefreshes:        input.Budget.PrerequisiteRefreshes,
		}
	}
	ret.RepairDirective = runtimeRepairDirectiveInfoFromProjection(input.RepairDirective)
	ret.SemanticPlanning = runtimeSemanticPlanningInfoFromProjection(input.SemanticPlanning)
	ret.Audit = runtimeAuditInfoFromClient(input.Audit)
	return ret
}

func runtimeRepairDirectiveInfoFromProjection(input *projectioncontract.RuntimeRepairDirective) *port.RuntimeRepairDirectiveInfo {
	if input == nil {
		return nil
	}
	ret := &port.RuntimeRepairDirectiveInfo{
		ID: input.ID, FocusID: input.FocusID, Trigger: input.Trigger, RequiredAction: input.RequiredAction,
		RequiredEffectScopes: append([]string(nil), input.RequiredEffectScopes...), RequiredEvidenceKinds: append([]string(nil), input.RequiredEvidenceKinds...),
		AllowedOperationKinds: append([]string(nil), input.AllowedOperationKinds...), CoverageWorkItemIDs: append([]string(nil), input.CoverageWorkItemIDs...),
		TargetRefs: append([]string(nil), input.TargetRefs...), SupersedesEvidenceRevision: input.SupersedesEvidenceRevision, RepairAttempt: input.RepairAttempt,
	}
	if input.Closure != nil {
		ret.Closure = &port.RuntimeRepairClosureInfo{
			ID: input.Closure.ID, GapSignature: input.Closure.GapSignature, BaselineEvidenceRevision: input.Closure.BaselineEvidenceRevision,
			CoverageWorkItemIDs: append([]string(nil), input.Closure.CoverageWorkItemIDs...), RequiredEvidenceKinds: append([]string(nil), input.Closure.RequiredEvidenceKinds...),
			GroundedTargetRefs: append([]string(nil), input.Closure.GroundedTargetRefs...), RequireValidationAfterStateChange: input.Closure.RequireValidationAfterStateChange,
		}
	}
	return ret
}

func runtimeSemanticPlanningInfoFromProjection(input *projectioncontract.RuntimeSemanticPlanning) *port.RuntimeSemanticPlanningInfo {
	if input == nil {
		return nil
	}
	ret := &port.RuntimeSemanticPlanningInfo{
		WorkspaceID: common.WorkspaceID(input.WorkspaceID), RunID: common.RunID(input.RunID),
		DefinitionRevision: input.DefinitionRevision, StateRevision: input.StateRevision,
		State: input.State, Phase: input.Phase, Attempt: input.Attempt,
		CatalogRefreshAttempts: input.CatalogRefreshAttempts, LogicalRequestID: input.LogicalRequestID,
		ManifestHash: input.ManifestHash, DocumentHash: input.DocumentHash,
		FeasibilityBatchID: input.FeasibilityBatchID, FeasibilitySetHash: input.FeasibilitySetHash,
		SealedBindingSetHash: input.SealedBindingSetHash,
	}
	for _, value := range input.Items {
		ret.Items = append(ret.Items, runtimeSemanticPlanningItemInfoFromProjection(value))
	}
	for _, value := range input.Rejections {
		ret.Rejections = append(ret.Rejections, runtimeSemanticDiagnosticInfoFromProjection(value))
	}
	for _, value := range input.FeasibilityGaps {
		gap := port.RuntimeSemanticFeasibilityGapInfo{
			RefreshGeneration: value.RefreshGeneration, ManifestHash: value.ManifestHash,
			DocumentHash: value.DocumentHash, FeasibilityBatchID: value.FeasibilityBatchID,
			FeasibilitySetHash: value.FeasibilitySetHash,
		}
		for _, item := range value.Items {
			gap.Items = append(gap.Items, runtimeSemanticPlanningItemInfoFromProjection(item))
		}
		for _, diagnostic := range value.Rejections {
			gap.Rejections = append(gap.Rejections, runtimeSemanticDiagnosticInfoFromProjection(diagnostic))
		}
		ret.FeasibilityGaps = append(ret.FeasibilityGaps, gap)
	}
	if input.Failure != nil {
		failure := runtimeSemanticDiagnosticInfoFromProjection(*input.Failure)
		ret.Failure = &failure
	}
	return ret
}

func runtimeSemanticPlanningItemInfoFromProjection(input projectioncontract.RuntimeSemanticPlanningItem) port.RuntimeSemanticPlanningItemInfo {
	ret := port.RuntimeSemanticPlanningItemInfo{
		CoverageWorkItemID: input.CoverageWorkItemID, ProposalHash: input.ProposalHash,
		OwnerModuleID: input.OwnerModuleID, OperationKind: input.OperationKind, Status: input.Status,
		TargetRefs: append([]string(nil), input.TargetRefs...), ValidationRecipeID: input.ValidationRecipeID,
		RequiredCheckRefs: append([]string(nil), input.RequiredCheckRefs...), ObservedRevision: input.ObservedRevision,
	}
	if input.Diagnostic != nil {
		diagnostic := runtimeSemanticDiagnosticInfoFromProjection(*input.Diagnostic)
		ret.Diagnostic = &diagnostic
	}
	return ret
}

func runtimeSemanticDiagnosticInfoFromProjection(input projectioncontract.RuntimeSemanticDiagnostic) port.RuntimeSemanticDiagnosticInfo {
	return port.RuntimeSemanticDiagnosticInfo{
		SchemaVersion: input.SchemaVersion, OwnerModuleID: input.OwnerModuleID,
		OriginalClass: input.OriginalClass, OriginalCode: input.OriginalCode,
		OriginalMessage: input.OriginalMessage, Signature: input.Signature,
		OperationID: input.OperationID, TargetRefs: append([]string(nil), input.TargetRefs...),
		ExpectedRevision: input.ExpectedRevision, ObservedRevision: input.ObservedRevision,
		FieldViolations: runtimecontract.CloneRuntimeFieldViolations(input.FieldViolations),
		DetailsRef:      input.DetailsRef, DetailsHash: input.DetailsHash, DetailsBytes: input.DetailsBytes,
		ContentType: input.ContentType, OmittedReason: input.OmittedReason,
		Truncated: input.Truncated, Redacted: input.Redacted,
	}
}

func runtimeAuditInfoFromClient(input *projectioncontract.RuntimeAudit) *projectioncontract.RuntimeAudit {
	return input
}

func taskProgressInfoFromClient(input client.TaskProgress) port.TaskProgressInfo {
	return port.TaskProgressInfo{
		WorkspaceID:        common.WorkspaceID(input.WorkspaceID),
		RunID:              common.RunID(input.RunID),
		State:              input.State,
		Summary:            input.Summary,
		CurrentStepID:      common.StepID(input.CurrentStepID),
		CurrentStepKind:    input.CurrentStepKind,
		CurrentStepState:   input.CurrentStepState,
		CurrentActionLabel: input.CurrentActionLabel,
		DiagnosticLineage:  common.StepID(input.DiagnosticLineage),
		CurrentMilestone:   input.CurrentMilestone,
		Milestones:         progressMilestonesFromClient(input.Milestones),
		Recovery: port.RecoveryPlanInfo{
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
		RepairLoop: port.RepairLoopInfo{
			State:                    input.RepairLoop.State,
			AttemptCount:             input.RepairLoop.AttemptCount,
			LastAttempt:              input.RepairLoop.LastAttempt,
			LastRepairStepID:         common.StepID(input.RepairLoop.LastRepairStepID),
			LastFailureStepID:        common.StepID(input.RepairLoop.LastFailureStepID),
			LastFailureClass:         input.RepairLoop.LastFailureClass,
			LastFailureSignature:     input.RepairLoop.LastFailureSignature,
			LastFailureTargetScope:   input.RepairLoop.LastFailureTargetScope,
			LastFailureAt:            input.RepairLoop.LastFailureAt,
			StructuredFailureSignals: payloadFromRemoteMap(input.RepairLoop.StructuredFailureSignals),
			RepairInputCompaction:    payloadFromRemoteMap(input.RepairLoop.RepairInputCompaction),
			LastRepairInputBytes:     input.RepairLoop.LastRepairInputBytes,
			RepeatedFailureReason:    input.RepairLoop.RepeatedFailureReason,
			NextAction:               input.RepairLoop.NextAction,
			Escalated:                input.RepairLoop.Escalated,
			EscalationReason:         input.RepairLoop.EscalationReason,
			EscalationTicketID:       common.TicketID(input.RepairLoop.EscalationTicketID),
			EvidenceRefs:             append([]string(nil), input.RepairLoop.EvidenceRefs...),
		},
		AssessmentOutcome: port.AssessmentOutcomeInfo{
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
		Runtime: runtimeProgressFromClient(input.Runtime),
	}
}

func progressMilestonesFromClient(items []projectioncontract.ProgressMilestone) []port.ProgressMilestoneInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]port.ProgressMilestoneInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.ProgressMilestoneInfo{
			ID: item.ID, Title: item.Title, State: item.State, EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
		})
	}
	return ret
}

func runtimeProgressFromClient(item *projectioncontract.RuntimeProgress) *port.RuntimeProgressInfo {
	if item == nil {
		return nil
	}
	return &port.RuntimeProgressInfo{
		StateRevision: item.StateRevision, EvidenceSequence: item.EvidenceSequence,
		CompletedDispatchCount: item.CompletedDispatchCount, ActiveDispatchID: item.ActiveDispatchID,
		CandidateAttemptCount: item.CandidateAttemptCount, AcceptedEvidenceCount: item.AcceptedEvidenceCount,
		AcceptedValidationEvidenceCount: item.AcceptedValidationEvidenceCount,
		RejectedEvidenceCount:           item.RejectedEvidenceCount, MissingEvidenceKinds: append([]string(nil), item.MissingEvidenceKinds...),
		CurrentFocusID: item.CurrentFocusID, ConvergenceState: item.ConvergenceState, TerminalCode: item.TerminalCode, FailureClass: item.FailureClass,
		FailureCode: item.FailureCode, FailureSignature: item.FailureSignature, FailureMessage: item.FailureMessage,
		RepairHintCodes:          append([]string(nil), item.RepairHintCodes...),
		PrerequisiteRefreshState: item.PrerequisiteRefreshState, PrerequisiteRefreshUsed: item.PrerequisiteRefreshUsed,
		PrerequisiteRefreshLimit:  item.PrerequisiteRefreshLimit,
		PrerequisiteInvalidations: prerequisiteRefreshInvalidationsFromClient(item.PrerequisiteInvalidations),
	}
}

func prerequisiteRefreshInvalidationsFromClient(items []projectioncontract.RuntimePrerequisiteRefreshInvalidation) []port.RuntimePrerequisiteRefreshInvalidationInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]port.RuntimePrerequisiteRefreshInvalidationInfo, 0, len(items))
	for _, item := range items {
		value := port.RuntimePrerequisiteRefreshInvalidationInfo{
			Signature: item.Signature, Source: item.Source, PrerequisiteIDs: append([]string(nil), item.PrerequisiteIDs...),
			ObservationIDs: append([]string(nil), item.ObservationIDs...), DispatchID: item.DispatchID, OperationID: item.OperationID,
			ReceiptHash: item.ReceiptHash, OwnerModuleID: item.OwnerModuleID, ResourceDomain: item.ResourceDomain,
			SubjectRef: item.SubjectRef, ExpectedRevision: item.ExpectedRevision, ObservedRevision: item.ObservedRevision,
			Reason: item.Reason,
		}
		for _, change := range item.ResourceChanges {
			value.ResourceChanges = append(value.ResourceChanges, port.RuntimePrerequisiteResourceChangeInfo{
				OwnerModuleID: change.OwnerModuleID, ResourceDomain: change.ResourceDomain, SubjectRef: change.SubjectRef,
				BeforeRevision: change.BeforeRevision, AfterRevision: change.AfterRevision,
			})
		}
		ret = append(ret, value)
	}
	return ret
}

func taskDecompositionInfosFromClient(items []client.TaskDecomposition) []port.TaskDecompositionInfo {
	ret := make([]port.TaskDecompositionInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.TaskDecompositionInfo{
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

func taskPlanInfosFromClient(items []client.TaskPlan) []port.TaskPlanInfo {
	ret := make([]port.TaskPlanInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.TaskPlanInfo{
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

func taskDecompositionNodeInfosFromClient(items []client.TaskDecompositionNode) []port.TaskDecompositionNodeInfo {
	ret := make([]port.TaskDecompositionNodeInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.TaskDecompositionNodeInfo{
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
			Input:            payloadFromRemoteMap(item.Input),
		})
	}
	return ret
}

func resultInfosFromClient(items []client.Result) []port.ResultInfo {
	ret := make([]port.ResultInfo, 0, len(items))
	for _, item := range items {
		summary := item.SummaryDetail
		ret = append(ret, port.ResultInfo{
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
			SummaryDetail: &port.ResultSummaryInfo{
				GoalOutcome:           summary.GoalOutcome,
				ImplementationSummary: summary.ImplementationSummary,
				VerificationSummary:   summary.VerificationSummary,
				EvidenceRefs:          append([]string(nil), summary.EvidenceRefs...),
				ArtifactRefs:          append([]string(nil), summary.ArtifactRefs...),
				KnownLimitations:      append([]string(nil), summary.KnownLimitations...),
				ResidualRisks:         append([]string(nil), summary.ResidualRisks...),
				NextActions:           append([]string(nil), summary.NextActions...),
				Diagnostics:           payloadFromRemoteMap(summary.Diagnostics),
			},
		})
	}
	return ret
}

func artifactInfosFromClient(items []projectioncontract.Artifact) []port.ArtifactInfo {
	ret := make([]port.ArtifactInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.ArtifactInfo{
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

func artifactContinuationInfosFromClient(items []projectioncontract.ArtifactContinuation) []port.ArtifactContinuationInfo {
	ret := make([]port.ArtifactContinuationInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.ArtifactContinuationInfo{
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
			Progress:          payloadFromRemoteMap(item.Progress),
		})
	}
	return ret
}

func patchInfosFromClient(items []projectioncontract.Patch) []port.PatchInfo {
	ret := make([]port.PatchInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.PatchInfo{
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
			Diagnostics:      payloadsFromRemoteMaps(item.Diagnostics),
			CreatedAt:        item.CreatedAt,
			AppliedAt:        item.AppliedAt,
			RevertedAt:       item.RevertedAt,
		})
	}
	return ret
}

func memoryRecallTraceInfosFromClient(items []client.MemoryRecallTrace) []port.MemoryRecallTraceInfo {
	ret := make([]port.MemoryRecallTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.MemoryRecallTraceInfo{
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

func memoryRecallHitInfosFromClient(items []projectioncontract.MemoryRecallHit) []port.MemoryRecallHitInfo {
	ret := make([]port.MemoryRecallHitInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.MemoryRecallHitInfo{
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

func llmContextBudgetTraceInfosFromClient(items []projectioncontract.LLMContextBudgetTrace) []port.LLMContextBudgetTraceInfo {
	ret := make([]port.LLMContextBudgetTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.LLMContextBudgetTraceInfo{
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
			Profile:               item.Profile.Clone(),
			CachePlan:             item.CachePlan.Clone(),
		})
	}
	return ret
}

func llmContextBudgetSummaryInfoFromClient(item projectioncontract.LLMContextBudgetSummary) port.LLMContextBudgetSummaryInfo {
	return port.LLMContextBudgetSummaryInfo{
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

func llmUsageTraceInfosFromClient(items []projectioncontract.LLMUsageTrace) []port.LLMUsageTraceInfo {
	ret := make([]port.LLMUsageTraceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.LLMUsageTraceInfo{
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
			ContextBudget: item.ContextBudget.Clone(),
			CacheUsage:    item.CacheUsage.Clone(),
			Status:        item.Status,
			FailureClass:  item.FailureClass,
			OccurredAt:    item.OccurredAt,
			EvidenceRef:   item.EvidenceRef,
		})
	}
	return ret
}

func llmUsageSummaryInfoFromClient(item projectioncontract.LLMUsageSummary) port.LLMUsageSummaryInfo {
	return port.LLMUsageSummaryInfo{
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

func llmUsageProviderModelBreakdownInfosFromClient(items []projectioncontract.LLMUsageProviderModelBreakdown) []port.LLMUsageProviderModelBreakdownInfo {
	ret := make([]port.LLMUsageProviderModelBreakdownInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.LLMUsageProviderModelBreakdownInfo{
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

func eventRecordInfosFromClient(items []projectioncontract.EventRecord) []port.EventRecordInfo {
	ret := make([]port.EventRecordInfo, 0, len(items))
	for _, item := range items {
		if len(item.Payload.Fields) == 0 {
			continue
		}
		typedPayload := workorchpayloadmapper.FromEventPayload(item.Payload)
		ret = append(ret, port.EventRecordInfo{
			ID:           common.EventRecordID(item.ID),
			Version:      item.Version,
			Topic:        item.Topic,
			OccurredAt:   item.OccurredAt,
			SourceModule: item.SourceModule,
			SubjectType:  item.SubjectType,
			SubjectID:    item.SubjectID,
			WorkspaceID:  common.WorkspaceID(item.WorkspaceID),
			RunID:        common.RunID(item.RunID),
			StepID:       common.StepID(item.StepID),
			Payload:      typedPayload,
			Summary:      typedPayload.CompactSummary(),
			PayloadRef:   item.PayloadRef,
		})
	}
	return ret
}

func decisionInfosFromClient(items []client.Decision) []port.DecisionInfo {
	ret := make([]port.DecisionInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.DecisionInfo{
			ID:                     common.TicketID(item.ID),
			WorkspaceID:            common.WorkspaceID(item.WorkspaceID),
			RunID:                  common.RunID(item.RunID),
			Reason:                 item.Reason,
			Question:               item.Question,
			Context:                item.Context.Clone(),
			AllowedRecoveryActions: append([]string(nil), item.AllowedRecoveryActions...),
			State:                  item.State,
			ResolutionType:         item.ResolutionType,
			ContinuationAction:     item.ContinuationAction,
			ResumeRunID:            common.RunID(item.ResumeRunID),
			SuccessorRunID:         common.RunID(item.SuccessorRunID),
			AutoGovernance:         mapRemoteAutoGovernanceSummary(item.AutoGovernance),
		})
	}
	return ret
}

func approvalInfosFromClient(items []client.Approval) []port.ApprovalInfo {
	ret := make([]port.ApprovalInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.ApprovalInfo{
			ID:             common.ApprovalID(item.ID),
			WorkspaceID:    common.WorkspaceID(item.WorkspaceID),
			RunID:          common.RunID(item.RunID),
			Reason:         item.Reason,
			Action:         item.Action,
			Context:        item.Context.Clone(),
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

func (b *remoteGoalBackend) Define(ctx context.Context, wsID common.WorkspaceID, text string) (*port.GoalInfo, error) {
	g, err := b.cl.CreateGoal(ctx, string(wsID), text)
	if err != nil {
		return nil, err
	}
	return &port.GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}
func (b *remoteGoalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.GoalInfo, error) {
	list, err := b.cl.ListGoals(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]port.GoalInfo, 0, len(list))
	for _, g := range list {
		result = append(result, port.GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text})
	}
	return result, nil
}
func (b *remoteGoalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.GoalID) (*port.GoalInfo, error) {
	g, err := b.cl.GetGoal(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &port.GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}
func (b *remoteGoalBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.GoalID, text string) (*port.GoalInfo, error) {
	g, err := b.cl.UpdateGoal(ctx, string(wsID), string(id), text)
	if err != nil {
		return nil, err
	}
	return &port.GoalInfo{ID: common.GoalID(g.ID), WorkspaceID: common.WorkspaceID(g.WorkspaceID), Text: g.Text}, nil
}

// ── Remote DoD ──

type remoteDoDBackend struct{ cl *client.Client }

func (b *remoteDoDBackend) Define(ctx context.Context, wsID common.WorkspaceID, items []string) (*port.DoDInfo, error) {
	d, err := b.cl.CreateDoD(ctx, string(wsID), items)
	if err != nil {
		return nil, err
	}
	return &port.DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}
func (b *remoteDoDBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.DoDInfo, error) {
	list, err := b.cl.ListDoDs(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]port.DoDInfo, 0, len(list))
	for _, d := range list {
		result = append(result, port.DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State})
	}
	return result, nil
}
func (b *remoteDoDBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.DoDID) (*port.DoDInfo, error) {
	d, err := b.cl.GetDoD(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &port.DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}
func (b *remoteDoDBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.DoDID, items []string) (*port.DoDInfo, error) {
	d, err := b.cl.UpdateDoD(ctx, string(wsID), string(id), items)
	if err != nil {
		return nil, err
	}
	return &port.DoDInfo{ID: common.DoDID(d.ID), WorkspaceID: common.WorkspaceID(d.WorkspaceID), Items: d.Items, State: d.State}, nil
}

// ── Remote Run ──

type remoteRunBackend struct{ cl *client.Client }

func (b *remoteRunBackend) Command(ctx context.Context, req port.WorkspaceRunCommand) (*port.RunInfo, error) {
	intent, err := port.ValidateWorkspaceRunCommand(req)
	if err != nil {
		return nil, err
	}
	switch intent {
	case port.RunCommandIntentResume:
		return b.runCommandViaActivation(ctx, req, port.RunCommandIntentResume)
	case port.RunCommandIntentRestart:
		return b.runCommandViaActivation(ctx, req, port.RunCommandIntentRestart)
	default:
		return nil, fmt.Errorf("%w: unsupported run command intent %q", common.ErrInvalidArgument, intent)
	}
}

func (b *remoteRunBackend) runCommandViaActivation(ctx context.Context, req port.WorkspaceRunCommand, intent port.RunCommandIntent) (*port.RunInfo, error) {
	payload := &client.ActivationRequest{
		Intent:    string(intent),
		RequestID: req.RequestID,
		Reason:    req.Reason,
		Source:    normalizedRunCommandSource(req.Source),
		Priority:  req.Priority,
	}
	act, err := b.cl.ResumeWorkspaceExecution(ctx, string(req.WorkspaceID), payload)
	if err != nil {
		return nil, err
	}
	if act == nil {
		return nil, nil
	}
	return remoteRunInfoFromClientRun(&act.Run), nil
}

func remoteRunInfoFromClientRun(r *client.Run) *port.RunInfo {
	if r == nil {
		return nil
	}
	return &port.RunInfo{
		ID: common.RunID(r.ID), WorkspaceID: common.WorkspaceID(r.WorkspaceID),
		GoalID: common.GoalID(r.GoalID), DoDID: common.DoDID(r.DoDID),
		DefinitionRevision:  r.DefinitionRevision,
		PredecessorRunID:    common.RunID(r.Lineage.PredecessorRunID),
		PredecessorResultID: common.ResultID(r.Lineage.PredecessorResultID),
		SuccessorRunID:      common.RunID(r.Lineage.SuccessorRunID),
		State:               common.RunState(r.State),
		BlockingTicketID:    common.TicketID(r.BlockingTicketID),
		BlockingApprovalID:  common.ApprovalID(r.BlockingApprovalID),
		FailureReason:       r.FailureReason,
		SealedLLMTarget:     runtimeProviderTargetFromClient(r.SealedLLMTarget),
		RuntimeActivation:   runtimeActivationStatusFromClient(r.RuntimeActivation),
	}
}

func runtimeActivationStatusFromClient(input *projectioncontract.RuntimeActivationStatus) *port.RuntimeActivationStatusInfo {
	if input == nil {
		return nil
	}
	return &port.RuntimeActivationStatusInfo{
		Disposition:            input.Disposition,
		ActualSchemaVersion:    input.ActualSchemaVersion,
		SupportedSchemaVersion: input.SupportedSchemaVersion,
	}
}
func (b *remoteRunBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*port.RunInfo, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	r, err := b.cl.GetRun(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return remoteRunInfoFromClientRun(r), nil
}
func (b *remoteRunBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.RunInfo, error) {
	list, err := b.cl.ListRuns(ctx, string(wsID))
	if err != nil {
		return nil, err
	}
	result := make([]port.RunInfo, 0, len(list))
	for _, r := range list {
		if item := remoteRunInfoFromClientRun(&r); item != nil {
			result = append(result, *item)
		}
	}
	return result, nil
}
func (b *remoteRunBackend) Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*port.RunStatusInfo, error) {
	if wsID == "" {
		return nil, fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	s, err := b.cl.GetRunStatus(ctx, string(wsID), string(id), "", 0)
	if err != nil {
		return nil, err
	}
	return &port.RunStatusInfo{
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

func convergenceStatusInfoFromClient(input *projectioncontract.ConvergenceStatus) *port.ConvergenceStatusInfo {
	if input == nil {
		return nil
	}
	return &port.ConvergenceStatusInfo{
		WorkspaceID:                   common.WorkspaceID(input.WorkspaceID),
		RunID:                         common.RunID(input.RunID),
		RunState:                      common.RunState(input.RunState),
		CurrentSliceID:                input.CurrentSliceID,
		CurrentFocusID:                input.CurrentFocusID,
		ConvergenceState:              input.ConvergenceState,
		SuggestedNextAction:           input.SuggestedNextAction,
		CurrentGap:                    input.CurrentGap,
		MissingEvidenceKinds:          append([]string(nil), input.MissingEvidenceKinds...),
		AcceptedEvidenceCount:         input.AcceptedEvidenceCount,
		RejectedEvidenceCount:         input.RejectedEvidenceCount,
		ClosedCoverageItemCount:       input.ClosedCoverageItemCount,
		RemainingCoverageItemCount:    input.RemainingCoverageItemCount,
		AssessmentDisposition:         input.AssessmentDisposition,
		RepairAttempts:                input.RepairAttempts,
		MaxRepairAttempts:             input.MaxRepairAttempts,
		RepeatedFailureSignatureCount: input.RepeatedFailureSignatureCount,
		LatestAcceptedEvidenceRef:     input.LatestAcceptedEvidenceRef,
		NextActivationRequired:        input.NextActivationRequired,
		GapSummary:                    input.GapSummary,
		LatestEffectiveProgressAt:     input.LatestEffectiveProgressAt,
		LastProgressAt:                input.LastProgressAt,
		DecisionCause:                 input.DecisionCause,
		Stalled:                       input.Stalled,
		IneffectiveRetry:              input.IneffectiveRetry,
	}
}

func schedulerDiagnosticsInfoFromClient(input *projectioncontract.SchedulerDiagnostics) *port.SchedulerDiagnosticsInfo {
	if input == nil {
		return nil
	}
	ret := &port.SchedulerDiagnosticsInfo{
		Watchdog: port.WatchdogDiagnosticsInfo{
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

func continuationDiagnosticsFromClient(input []projectioncontract.ContinuationDiagnostic) []port.ContinuationDiagnosticInfo {
	if len(input) == 0 {
		return nil
	}
	ret := make([]port.ContinuationDiagnosticInfo, 0, len(input))
	for _, item := range input {
		ret = append(ret, continuationDiagnosticFromClient(item))
	}
	return ret
}

func continuationDiagnosticFromClient(input projectioncontract.ContinuationDiagnostic) port.ContinuationDiagnosticInfo {
	return port.ContinuationDiagnosticInfo{
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

func continuationReadbackDiagnosticFromClient(input *projectioncontract.ContinuationReadbackDiagnostic) *port.ContinuationReadbackDiagnosticInfo {
	if input == nil {
		return nil
	}
	return &port.ContinuationReadbackDiagnosticInfo{
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

func continuationDiagnosticCountsFromClient(input []projectioncontract.ContinuationDiagnosticCount) []port.ContinuationDiagnosticCountInfo {
	if len(input) == 0 {
		return nil
	}
	ret := make([]port.ContinuationDiagnosticCountInfo, 0, len(input))
	for _, item := range input {
		ret = append(ret, port.ContinuationDiagnosticCountInfo{
			Kind:   item.Kind,
			Reason: item.Reason,
			Count:  item.Count,
		})
	}
	return ret
}

// ── Remote Decision ──

type remoteDecisionBackend struct{ cl *client.Client }

func (b *remoteDecisionBackend) Resolve(ctx context.Context, req port.ResolveDecisionRequest) (*port.DecisionInfo, error) {
	d, err := b.cl.ResolveDecision(ctx, string(req.WorkspaceID), string(req.TicketID), req.ResolutionType, req.Payload)
	if err != nil {
		return nil, err
	}
	item := mapRemoteDecisionInfoPtr(d)
	return &item, nil
}
func (b *remoteDecisionBackend) BatchResolve(ctx context.Context, workspaceID common.WorkspaceID, ticketIDs []common.TicketID, resolutionType string, payload workorcheventpayload.Payload) ([]port.DecisionInfo, error) {
	ids := make([]string, len(ticketIDs))
	for i, id := range ticketIDs {
		ids[i] = string(id)
	}
	list, err := b.cl.BatchResolveDecisions(ctx, string(workspaceID), ids, resolutionType, payload)
	if err != nil {
		return nil, err
	}
	result := make([]port.DecisionInfo, 0, len(list))
	for _, d := range list {
		result = append(result, mapRemoteDecisionInfo(d))
	}
	return result, nil
}
func (b *remoteDecisionBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.DecisionInfo, error) {
	list, err := b.cl.ListDecisions(ctx, string(wsID), "")
	if err != nil {
		return nil, err
	}
	result := make([]port.DecisionInfo, 0, len(list))
	for _, d := range list {
		result = append(result, mapRemoteDecisionInfo(d))
	}
	return result, nil
}
func (b *remoteDecisionBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.TicketID) (*port.DecisionInfo, error) {
	d, err := b.cl.GetDecision(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	item := mapRemoteDecisionInfoPtr(d)
	return &item, nil
}

func mapRemoteDecisionInfoPtr(d *client.Decision) port.DecisionInfo {
	if d == nil {
		return port.DecisionInfo{}
	}
	return mapRemoteDecisionInfo(*d)
}

func mapRemoteDecisionInfo(d client.Decision) port.DecisionInfo {
	return port.DecisionInfo{
		ID:                     common.TicketID(d.ID),
		WorkspaceID:            common.WorkspaceID(d.WorkspaceID),
		RunID:                  common.RunID(d.RunID),
		Reason:                 d.Reason,
		Question:               d.Question,
		Context:                d.Context.Clone(),
		AllowedRecoveryActions: append([]string(nil), d.AllowedRecoveryActions...),
		State:                  d.State,
		ResolutionType:         d.ResolutionType,
		ContinuationAction:     d.ContinuationAction,
		ResumeRunID:            common.RunID(d.ResumeRunID),
		SuccessorRunID:         common.RunID(d.SuccessorRunID),
		AutoGovernance:         mapRemoteAutoGovernanceSummary(d.AutoGovernance),
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

func payloadFromRemoteMap(input map[string]any) workorcheventpayload.Payload {
	payload, _ := workorcheventpayload.FromMap(input)
	return payload
}

func payloadsFromRemoteMaps(input []map[string]any) []workorcheventpayload.Payload {
	payloads, _ := workorcheventpayload.FromMapSlice(input)
	return payloads
}

// ── Remote Approval ──

type remoteApprovalBackend struct{ cl *client.Client }

func (b *remoteApprovalBackend) Approve(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*port.ApprovalInfo, error) {
	a, err := b.cl.Approve(ctx, string(wsID), string(id), payload...)
	if err != nil {
		return nil, err
	}
	return &port.ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)}, nil
}
func (b *remoteApprovalBackend) Reject(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID, payload ...workorcheventpayload.Payload) (*port.ApprovalInfo, error) {
	a, err := b.cl.Reject(ctx, string(wsID), string(id), payload...)
	if err != nil {
		return nil, err
	}
	return &port.ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)}, nil
}
func (b *remoteApprovalBackend) BatchResolve(ctx context.Context, wsID common.WorkspaceID, ids []common.ApprovalID, action string, payload ...workorcheventpayload.Payload) ([]port.ApprovalInfo, error) {
	_idStrings := make([]string, len(ids))
	for i, id := range ids {
		_idStrings[i] = string(id)
	}
	list, err := b.cl.BatchResolveApprovals(ctx, string(wsID), _idStrings, action, payload...)
	if err != nil {
		return nil, err
	}
	result := make([]port.ApprovalInfo, 0, len(list))
	for _, a := range list {
		result = append(result, port.ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)})
	}
	return result, nil
}
func (b *remoteApprovalBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.ApprovalInfo, error) {
	list, err := b.cl.ListApprovals(ctx, string(wsID), "")
	if err != nil {
		return nil, err
	}
	result := make([]port.ApprovalInfo, 0, len(list))
	for _, a := range list {
		result = append(result, port.ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)})
	}
	return result, nil
}
func (b *remoteApprovalBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.ApprovalID) (*port.ApprovalInfo, error) {
	a, err := b.cl.GetApproval(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return &port.ApprovalInfo{ID: common.ApprovalID(a.ID), WorkspaceID: common.WorkspaceID(a.WorkspaceID), RunID: common.RunID(a.RunID), Reason: a.Reason, Action: a.Action, Context: a.Context, State: a.State, AutoGovernance: mapRemoteAutoGovernanceSummary(a.AutoGovernance)}, nil
}

func mapRemoteAutoGovernanceSummary(summary *client.AutoGovernanceSummary) *port.AutoGovernanceSummaryInfo {
	if summary == nil {
		return nil
	}
	ret := &port.AutoGovernanceSummaryInfo{
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

func (b *remoteSoulBackend) Create(ctx context.Context, s *port.SoulInfo) (*port.SoulInfo, error) {
	return s, nil
}
func (b *remoteSoulBackend) Get(ctx context.Context, id common.SoulID) (*port.SoulInfo, error) {
	return &port.SoulInfo{ID: id}, nil
}
func (b *remoteSoulBackend) List(ctx context.Context) ([]port.SoulInfo, error) { return nil, nil }
func (b *remoteSoulBackend) Update(ctx context.Context, id common.SoulID, s *port.SoulInfo) (*port.SoulInfo, error) {
	return s, nil
}
func (b *remoteSoulBackend) Delete(ctx context.Context, id common.SoulID) error { return nil }

type remoteSkillBackend struct{ cl *client.Client }

func (b *remoteSkillBackend) Create(ctx context.Context, s *port.SkillInfo) (*port.SkillInfo, error) {
	return s, nil
}
func (b *remoteSkillBackend) Get(ctx context.Context, id common.SkillID) (*port.SkillInfo, error) {
	return &port.SkillInfo{ID: id}, nil
}
func (b *remoteSkillBackend) List(ctx context.Context) ([]port.SkillInfo, error) { return nil, nil }

type remoteMCPBackend struct{ cl *client.Client }

func (b *remoteMCPBackend) CreateServer(ctx context.Context, s *port.MCPServerInfo) (*port.MCPServerInfo, error) {
	return s, nil
}
func (b *remoteMCPBackend) GetServer(ctx context.Context, id common.MCPServerID) (*port.MCPServerInfo, error) {
	return &port.MCPServerInfo{ID: id}, nil
}
func (b *remoteMCPBackend) ListServers(ctx context.Context) ([]port.MCPServerInfo, error) {
	return nil, nil
}
func (b *remoteMCPBackend) UpdateServer(ctx context.Context, id common.MCPServerID, s *port.MCPServerInfo) (*port.MCPServerInfo, error) {
	return s, nil
}
func (b *remoteMCPBackend) ListCapabilities(ctx context.Context) ([]port.MCPCapabilityInfo, error) {
	return nil, nil
}

type remoteMemoryBackend struct{ cl *client.Client }

func (b *remoteMemoryBackend) Create(ctx context.Context, wsID common.WorkspaceID, m *port.MemoryItemInfo) (*port.MemoryItemInfo, error) {
	item, err := b.cl.CreateMemory(ctx, string(wsID), clientMemoryFromInfo(m))
	if err != nil {
		return nil, err
	}
	return memoryInfoFromClient(item), nil
}
func (b *remoteMemoryBackend) Get(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) (*port.MemoryItemInfo, error) {
	item, err := b.cl.GetMemory(ctx, string(wsID), string(id))
	if err != nil {
		return nil, err
	}
	return memoryInfoFromClient(item), nil
}
func (b *remoteMemoryBackend) Update(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID, m *port.MemoryItemInfo) (*port.MemoryItemInfo, error) {
	item, err := b.cl.UpdateMemory(ctx, string(wsID), string(id), clientMemoryFromInfo(m))
	if err != nil {
		return nil, err
	}
	return memoryInfoFromClient(item), nil
}
func (b *remoteMemoryBackend) Delete(ctx context.Context, wsID common.WorkspaceID, id common.MemoryID) error {
	return b.cl.DeleteMemory(ctx, string(wsID), string(id))
}
func (b *remoteMemoryBackend) List(ctx context.Context, wsID common.WorkspaceID) ([]port.MemoryItemInfo, error) {
	return b.Search(ctx, wsID, port.MemorySearchRequest{})
}
func (b *remoteMemoryBackend) Search(ctx context.Context, wsID common.WorkspaceID, req port.MemorySearchRequest) ([]port.MemoryItemInfo, error) {
	items, err := b.cl.SearchMemory(ctx, workorchmanagementcontract.MemorySearchQuery{
		WorkspaceID: string(wsID),
		Query:       req.Query,
		Tags:        append([]string(nil), req.Tags...),
		Types:       append([]string(nil), req.Types...),
		Limit:       req.Limit,
	})
	if err != nil {
		return nil, err
	}
	ret := make([]port.MemoryItemInfo, 0, len(items))
	for _, item := range items {
		info := memoryInfoFromClient(&item)
		ret = append(ret, *info)
	}
	return ret, nil
}

func clientMemoryFromInfo(info *port.MemoryItemInfo) *workorchmanagementcontract.MemoryMutation {
	if info == nil {
		return &workorchmanagementcontract.MemoryMutation{}
	}
	return &workorchmanagementcontract.MemoryMutation{
		MemoryID:            string(info.ID),
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

func memoryInfoFromClient(item *workorchmanagementcontract.MemoryItem) *port.MemoryItemInfo {
	if item == nil {
		return nil
	}
	return &port.MemoryItemInfo{
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
	}
}

type remoteTemplateBackend struct{ cl *client.Client }

func (b *remoteTemplateBackend) Create(ctx context.Context, t *port.TemplateInfo) (*port.TemplateInfo, error) {
	item, err := b.cl.CreateWorkspaceTemplate(ctx, clientTemplateFromInfo(t))
	if err != nil {
		return nil, err
	}
	return templateInfoFromClient(item), nil
}
func (b *remoteTemplateBackend) Get(ctx context.Context, id string) (*port.TemplateInfo, error) {
	item, err := b.cl.GetWorkspaceTemplate(ctx, id)
	if err != nil {
		return nil, err
	}
	return templateInfoFromClient(item), nil
}
func (b *remoteTemplateBackend) List(ctx context.Context) ([]port.TemplateInfo, error) {
	items, err := b.cl.ListWorkspaceTemplates(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]port.TemplateInfo, 0, len(items))
	for i := range items {
		if info := templateInfoFromClient(&items[i]); info != nil {
			ret = append(ret, *info)
		}
	}
	return ret, nil
}
func (b *remoteTemplateBackend) Update(ctx context.Context, id string, t *port.TemplateInfo) (*port.TemplateInfo, error) {
	item, err := b.cl.UpdateWorkspaceTemplate(ctx, id, clientTemplateFromInfo(t))
	if err != nil {
		return nil, err
	}
	return templateInfoFromClient(item), nil
}
func (b *remoteTemplateBackend) Delete(ctx context.Context, id string) error {
	return b.cl.DeleteWorkspaceTemplate(ctx, id)
}

func clientTemplateFromInfo(info *port.TemplateInfo) *workorchmanagementcontract.WorkspaceTemplate {
	if info == nil {
		return &workorchmanagementcontract.WorkspaceTemplate{}
	}
	return &workorchmanagementcontract.WorkspaceTemplate{
		ID:               info.ID,
		Name:             info.Name,
		Description:      info.Description,
		GoalText:         info.GoalText,
		DoDItems:         append([]string(nil), info.DoDItems...),
		DefaultSoulID:    string(info.DefaultSoulID),
		SoulBindings:     cloneStringMap(info.SoulBindings),
		ContextHints:     clonePayloadMap(info.ContextHints.Map()),
		EvidencePolicies: workorcheventpayload.MapSlice(info.EvidencePolicies),
	}
}

func templateInfoFromClient(item *workorchmanagementcontract.WorkspaceTemplate) *port.TemplateInfo {
	if item == nil {
		return nil
	}
	return &port.TemplateInfo{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    common.SoulID(item.DefaultSoulID),
		SoulBindings:     cloneStringMap(item.SoulBindings),
		ContextHints:     payloadFromRemoteMap(item.ContextHints),
		EvidencePolicies: payloadsFromRemoteMaps(item.EvidencePolicies),
	}
}

func clientProviderModelTarget(target *runtimecontract.ProviderModelTarget) *runtimecontract.ProviderModelTarget {
	if target == nil {
		return nil
	}
	value := target.Normalized()
	return &value
}

type remoteConfigBackend struct{ cl *client.Client }

func (b *remoteConfigBackend) Get(ctx context.Context) (*port.ServiceConfig, error) {
	cfg, err := b.cl.GetConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &port.ServiceConfig{}, nil
	}
	ret := &port.ServiceConfig{
		ServerAddr: cfg.Server.Addr,
		Backend:    cfg.Repository.Backend,
		DataDir:    cfg.Repository.DataDir,
		DefaultChat: runtimecontract.ProviderModelTarget{
			ProviderID: cfg.LLM.DefaultChat.ProviderID,
			ModelID:    cfg.LLM.DefaultChat.ModelID,
		},
		MaxConcurrentRuns: cfg.Server.MaxConcurrentRuns,
		AIProxyBaseURL:    cfg.LLM.AIProxy.BaseURL,
		LLMOutput: workorchconfig.LLMOutputRuntimeConfig{
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
	}
	ret.ChatModelTargets = chatModelTargetsFromClientConfig(cfg)
	return ret, nil
}

func (b *remoteConfigBackend) ListChatModelTargets(ctx context.Context) ([]port.ChatModelTargetOption, error) {
	configuration, err := b.cl.GetConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	catalog, err := b.cl.GetLLMModelCatalog(ctx, runtimecontract.BuiltinAIProxyProviderID)
	if err != nil {
		return nil, err
	}
	if configuration != nil && catalog != nil {
		configuration.LLM.Catalog = *catalog
	}
	return chatModelTargetsFromClientConfig(configuration), nil
}

func (b *remoteConfigBackend) RefreshChatModelTargets(ctx context.Context) ([]port.ChatModelTargetOption, error) {
	if _, err := b.cl.RefreshLLMModelCatalog(ctx, runtimecontract.BuiltinAIProxyProviderID); err != nil {
		return nil, err
	}
	return b.ListChatModelTargets(ctx)
}

func chatModelTargetsFromClientConfig(cfg *client.ServiceConfiguration) []port.ChatModelTargetOption {
	if cfg == nil {
		return nil
	}
	ret := make([]port.ChatModelTargetOption, 0)
	for _, model := range cfg.LLM.Catalog.Models {
		if !model.CapabilityReady || !stringSliceContains(model.Operations, "chat_completions") {
			continue
		}
		ret = append(ret, port.ChatModelTargetOption{ProviderID: model.ProviderID, ModelID: model.ModelID, DisplayName: model.ModelID, ContextWindowTokens: model.ContextWindowTokens, MaxOutputTokens: model.MaxOutputTokens, CapabilityReady: true, CapabilityRevision: model.CapabilityRevision, Source: cfg.LLM.Catalog.Source})
	}
	for _, provider := range cfg.LLM.CustomProviders {
		for _, model := range provider.Models {
			if !stringSliceContains(model.Operations, "chat_completions") {
				continue
			}
			ret = append(ret, port.ChatModelTargetOption{ProviderID: provider.ID, ModelID: model.ID, DisplayName: model.ID, ContextWindowTokens: model.ContextWindowTokens, MaxOutputTokens: model.MaxOutputTokens, CapabilityReady: true, Source: "configuration"})
		}
	}
	return port.NormalizeChatModelTargetOptions(ret)
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

type remoteEventsBackend struct {
	cl          *client.Client
	streamView  string
	mu          sync.Mutex
	lastEventID string
}

func (b *remoteEventsBackend) Subscribe(ctx context.Context, handler port.EventHandler) (func(), error) {
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

func (b *remoteEventsBackend) streamEvents(ctx context.Context, handler port.EventHandler) {
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

func (b *remoteEventsBackend) streamEventsOnce(ctx context.Context, handler port.EventHandler) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.cl.BaseURL()+"/api/v1/workorch/events/stream", nil)
	if err != nil {
		return false
	}
	query := req.URL.Query()
	if streamView := strings.TrimSpace(b.streamView); streamView != "" {
		query.Set("view", streamView)
	}
	if lastEventID := b.lastEventIDSnapshot(); lastEventID != "" {
		req.Header.Set("Last-port.Event-ID", lastEventID)
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
	handler(ctx, port.Event{
		Type: port.EventWorkspaceListChanged,
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

func (b *remoteEventsBackend) dispatchSSEData(ctx context.Context, lines []string, eventID string, handler port.EventHandler) {
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

// ── Remote Projection Events port.Backend (typed SSE) ──

type remoteProjectionEventsBackend struct {
	cl          *client.Client
	mu          sync.Mutex
	lastEventID string
}

func (b *remoteProjectionEventsBackend) SubscribeWorkspaceProjection(
	ctx context.Context,
	onChanged port.WorkspaceProjectionChangedHandler,
	onStatus port.SubscriptionStatusHandler,
) (func(), error) {
	if b == nil || b.cl == nil {
		return func() {}, fmt.Errorf("%w: remote projection backend unavailable", common.ErrInvalidArgument)
	}
	if onChanged == nil {
		return func() {}, fmt.Errorf("%w: projection changed handler required", common.ErrInvalidArgument)
	}
	streamCtx, cancel := context.WithCancel(ctx)
	var once sync.Once
	go b.streamProjectionEvents(streamCtx, onChanged, onStatus)
	return func() {
		once.Do(cancel)
	}, nil
}

func (b *remoteProjectionEventsBackend) streamProjectionEvents(
	ctx context.Context,
	onChanged port.WorkspaceProjectionChangedHandler,
	onStatus port.SubscriptionStatusHandler,
) {
	publishStatus := func(status port.SubscriptionStatusInfo) {
		if onStatus != nil {
			onStatus(ctx, status)
		}
	}
	publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionConnecting})
	backoff := 200 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return
		}
		connected, liveRevision := b.streamProjectionEventsOnce(ctx, onChanged, onStatus)
		if ctx.Err() != nil {
			return
		}
		if connected {
			backoff = 200 * time.Millisecond
			continue
		}
		publishStatus(port.SubscriptionStatusInfo{
			State:          port.SubscriptionReconnecting,
			StreamRevision: liveRevision,
			LastError:      "projection sse disconnected",
		})
		if backoff < 2*time.Second {
			backoff *= 2
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionConnecting, StreamRevision: liveRevision})
		}
	}
}

func (b *remoteProjectionEventsBackend) streamProjectionEventsOnce(
	ctx context.Context,
	onChanged port.WorkspaceProjectionChangedHandler,
	onStatus port.SubscriptionStatusHandler,
) (bool, int64) {
	publishStatus := func(status port.SubscriptionStatusInfo) {
		if onStatus != nil {
			onStatus(ctx, status)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.cl.BaseURL()+"/api/v1/workorch/events/stream", nil)
	if err != nil {
		publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionDegraded, LastError: err.Error()})
		return false, 0
	}
	query := req.URL.Query()
	query.Set("view", "projection")
	if lastEventID := b.lastEventIDSnapshot(); lastEventID != "" {
		req.Header.Set("Last-port.Event-ID", lastEventID)
		query.Set("lastEventId", lastEventID)
	}
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Accept", "text/event-stream")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionDegraded, LastError: err.Error()})
		return false, 0
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionDegraded, LastError: fmt.Sprintf("projection sse status %d", resp.StatusCode)})
		return false, 0
	}
	// Connected: only now report live.
	publishStatus(port.SubscriptionStatusInfo{State: port.SubscriptionLive})
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	var dataLines []string
	var eventID string
	var lastRevision int64
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return true, lastRevision
		default:
		}
		line := scanner.Text()
		if line == "" {
			info, ok := decodeProjectionSSE(dataLines, eventID)
			dataLines = nil
			eventID = ""
			if !ok {
				continue
			}
			b.rememberLastEventID(info.ID)
			if info.Revision.StreamRevision > lastRevision {
				lastRevision = info.Revision.StreamRevision
			}
			if onStatus != nil {
				onStatus(ctx, port.SubscriptionStatusInfo{
					State:            port.SubscriptionLive,
					StreamGeneration: info.Revision.StreamGeneration,
					StreamRevision:   info.Revision.StreamRevision,
					LastEventAt:      info.UpdatedAt,
				})
			}
			onChanged(ctx, info)
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
	return false, lastRevision
}

type workspaceProjectionChangedSSE struct {
	ID               string `json:"id"`
	StreamGeneration string `json:"streamGeneration,omitempty"`
	StreamRevision   int64  `json:"streamRevision"`
	WorkspaceID      string `json:"workspaceId"`
	RunID            string `json:"runId,omitempty"`
	Revision         struct {
		StreamGeneration        string `json:"streamGeneration,omitempty"`
		StreamRevision          int64  `json:"streamRevision,omitempty"`
		WorkspaceRevision       int64  `json:"workspaceRevision,omitempty"`
		ProjectedSourceRevision int64  `json:"projectedSourceRevision,omitempty"`
		RuntimeStateRevision    uint64 `json:"runtimeStateRevision,omitempty"`
		EvidenceSequence        uint64 `json:"evidenceSequence,omitempty"`
		ActivityRevision        int64  `json:"activityRevision,omitempty"`
	} `json:"revision"`
	ChangedSections []string `json:"changedSections"`
	ProjectionState string   `json:"projectionState"`
	UpdatedAt       string   `json:"updatedAt"`
}

// DecodeProjectionSSE is the production remote projection SSE decoder entry point.
// Cross-boundary contract tests must call this (not a copied DTO + json.Unmarshal)
// against frames produced by the server serializer (CR06).
func DecodeProjectionSSE(lines []string, eventID string) (port.WorkspaceProjectionChangedInfo, bool) {
	return decodeProjectionSSE(lines, eventID)
}

func decodeProjectionSSE(lines []string, eventID string) (port.WorkspaceProjectionChangedInfo, bool) {
	if len(lines) == 0 {
		return port.WorkspaceProjectionChangedInfo{}, false
	}
	var dto workspaceProjectionChangedSSE
	if err := json.Unmarshal([]byte(strings.Join(lines, "\n")), &dto); err != nil {
		return port.WorkspaceProjectionChangedInfo{}, false
	}
	// Accept both top-level concrete DTO and legacy envelope topic form.
	if strings.TrimSpace(dto.WorkspaceID) == "" {
		return port.WorkspaceProjectionChangedInfo{}, false
	}
	sections := make([]port.WorkspaceProjectionSection, 0, len(dto.ChangedSections))
	for _, section := range dto.ChangedSections {
		if strings.TrimSpace(section) == "" {
			continue
		}
		sections = append(sections, port.WorkspaceProjectionSection(section))
	}
	streamRevision := dto.StreamRevision
	if streamRevision == 0 {
		streamRevision = dto.Revision.StreamRevision
	}
	id := strings.TrimSpace(dto.ID)
	if id == "" {
		id = strings.TrimSpace(eventID)
	}
	streamGeneration := strings.TrimSpace(dto.StreamGeneration)
	if streamGeneration == "" {
		streamGeneration = strings.TrimSpace(dto.Revision.StreamGeneration)
	}
	return port.WorkspaceProjectionChangedInfo{
		ID:          id,
		WorkspaceID: common.WorkspaceID(dto.WorkspaceID),
		RunID:       common.RunID(dto.RunID),
		Revision: projectioncontract.WorkspaceRefreshRevision{
			StreamGeneration:        streamGeneration,
			StreamRevision:          streamRevision,
			WorkspaceRevision:       dto.Revision.WorkspaceRevision,
			ProjectedSourceRevision: dto.Revision.ProjectedSourceRevision,
			RuntimeStateRevision:    dto.Revision.RuntimeStateRevision,
			EvidenceSequence:        dto.Revision.EvidenceSequence,
			ActivityRevision:        dto.Revision.ActivityRevision,
		},
		ChangedSections: sections,
		ProjectionState: dto.ProjectionState,
		UpdatedAt:       dto.UpdatedAt,
	}, true
}

func (b *remoteProjectionEventsBackend) lastEventIDSnapshot() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastEventID
}

func (b *remoteProjectionEventsBackend) rememberLastEventID(id string) {
	if b == nil || strings.TrimSpace(id) == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastEventID = strings.TrimSpace(id)
}

// Ensure remoteBackend implements port.Backend.
var _ port.Backend = (*remoteBackend)(nil)
