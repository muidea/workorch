// Package eventmapping contains pure, stateless conversions between the
// workorchd EventHub DTOs and process-facing port DTOs.
package eventmapping

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func WorkspaceDTO(item projectioncontract.Workspace) (workorchdevents.CLIWorkspaceInfo, error) {
	hints, err := workorcheventpayload.FromMap(item.ContextHints)
	if err != nil {
		return workorchdevents.CLIWorkspaceInfo{}, err
	}
	policies, err := workorcheventpayload.FromMapSlice(item.EvidencePolicies)
	if err != nil {
		return workorchdevents.CLIWorkspaceInfo{}, err
	}
	ret := workorchdevents.CLIWorkspaceInfo{
		Revision: item.Revision,
		ID:       common.WorkspaceID(item.ID), Title: item.Title, Description: item.Description, DeliverableDirectory: item.DeliverableDirectory,
		State: common.WorkspaceState(item.State), ActiveRunID: common.RunID(item.ActiveRunID), BlockingTicketID: common.TicketID(item.BlockingTicketID), BlockingApprovalID: common.ApprovalID(item.BlockingApprovalID),
		TemplateID: item.TemplateID, ProfileID: item.ProfileID, DefaultSoulID: common.SoulID(item.DefaultSoulID), SoulBindings: soulbinding.FromStringMap(item.SoulBindings),
		ContextHints: hints, EvidencePolicies: policies,
		Projection: item.Projection, ViewModel: WorkspaceViewModelDTO(item.ViewModel),
	}
	if item.DefaultLLMSelection != nil {
		value := item.DefaultLLMSelection.Clone()
		ret.DefaultLLMSelection = &value
	}
	return ret, nil
}

func WorkspaceDTOs(items []projectioncontract.Workspace) ([]workorchdevents.CLIWorkspaceInfo, error) {
	if items == nil {
		return nil, nil
	}
	ret := make([]workorchdevents.CLIWorkspaceInfo, 0, len(items))
	for _, item := range items {
		dto, err := WorkspaceDTO(item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, dto)
	}
	return ret, nil
}

func WorkspaceProjection(item workorchdevents.CLIWorkspaceInfo) (projectioncontract.Workspace, error) {
	if err := soulbinding.Validate(item.SoulBindings); err != nil {
		return projectioncontract.Workspace{}, err
	}
	view, err := WorkspaceViewModelProjection(item.ViewModel)
	if err != nil {
		return projectioncontract.Workspace{}, err
	}
	ret := projectioncontract.Workspace{
		Revision: item.Revision,
		ID:       string(item.ID), Title: item.Title, Description: item.Description, DeliverableDirectory: item.DeliverableDirectory,
		State: string(item.State), ActiveRunID: string(item.ActiveRunID), BlockingTicketID: string(item.BlockingTicketID), BlockingApprovalID: string(item.BlockingApprovalID),
		TemplateID: item.TemplateID, ProfileID: item.ProfileID, DefaultSoulID: string(item.DefaultSoulID), SoulBindings: soulbinding.StringMap(item.SoulBindings),
		ContextHints: item.ContextHints.Map(), EvidencePolicies: workorcheventpayload.MapSlice(item.EvidencePolicies), Projection: item.Projection, ViewModel: view,
	}
	if item.DefaultLLMSelection != nil {
		value := item.DefaultLLMSelection.Clone()
		ret.DefaultLLMSelection = &value
	}
	return projectioncontract.CloneWorkspace(ret), nil
}

func WorkspaceViewModelDTO(item projectioncontract.WorkspaceViewModel) workorchdevents.CLIWorkspaceViewModelInfo {
	var sections []projectioncontract.WorkspaceViewModelSection
	if item.Sections != nil {
		sections = make([]projectioncontract.WorkspaceViewModelSection, 0, len(item.Sections))
	}
	for name, section := range item.Sections {
		sections = append(sections, projectioncontract.WorkspaceViewModelSection{Name: name, Revision: section.Revision, WorkspaceRevision: section.WorkspaceRevision, SourceEventRevision: section.SourceEventRevision, ProjectedSourceRevision: section.ProjectedSourceRevision, State: section.State})
	}
	sort.Slice(sections, func(i, j int) bool { return sections[i].Name < sections[j].Name })
	return workorchdevents.CLIWorkspaceViewModelInfo{WorkspaceID: common.WorkspaceID(item.WorkspaceID), State: item.State, PromptMode: item.PromptMode, Revision: item.Revision, StreamGeneration: item.StreamGeneration, GlobalRevision: item.GlobalRevision, WorkspaceRevision: item.WorkspaceRevision, SourceEventRevision: item.SourceEventRevision, ProjectedSourceRevision: item.ProjectedSourceRevision, UpdatedAt: item.UpdatedAt, RebuiltAt: item.RebuiltAt, Degraded: item.Degraded, StaleReason: item.StaleReason, Error: item.Error, FailedWorkspaceIds: slices.Clone(item.FailedWorkspaceIds), DegradedSince: item.DegradedSince, Sections: sections}
}

func WorkspaceViewModelProjection(item workorchdevents.CLIWorkspaceViewModelInfo) (projectioncontract.WorkspaceViewModel, error) {
	var sections map[string]projectioncontract.WorkspaceViewModelSection
	if item.Sections != nil {
		sections = make(map[string]projectioncontract.WorkspaceViewModelSection, len(item.Sections))
	}
	for _, section := range item.Sections {
		name := strings.TrimSpace(section.Name)
		if name == "" {
			return projectioncontract.WorkspaceViewModel{}, fmt.Errorf("%w: CLI workspace view section requires name", common.ErrInvalidArgument)
		}
		if _, exists := sections[name]; exists {
			return projectioncontract.WorkspaceViewModel{}, fmt.Errorf("%w: duplicate CLI workspace view section %q", common.ErrInvalidArgument, name)
		}
		sections[name] = projectioncontract.WorkspaceViewModelSection{Name: name, Revision: section.Revision, WorkspaceRevision: section.WorkspaceRevision, SourceEventRevision: section.SourceEventRevision, ProjectedSourceRevision: section.ProjectedSourceRevision, State: section.State}
	}
	return projectioncontract.WorkspaceViewModel{WorkspaceID: string(item.WorkspaceID), State: item.State, PromptMode: item.PromptMode, Revision: item.Revision, StreamGeneration: item.StreamGeneration, GlobalRevision: item.GlobalRevision, WorkspaceRevision: item.WorkspaceRevision, SourceEventRevision: item.SourceEventRevision, ProjectedSourceRevision: item.ProjectedSourceRevision, UpdatedAt: item.UpdatedAt, RebuiltAt: item.RebuiltAt, Degraded: item.Degraded, StaleReason: item.StaleReason, Error: item.Error, FailedWorkspaceIds: slices.Clone(item.FailedWorkspaceIds), DegradedSince: item.DegradedSince, Sections: sections}, nil
}

func TemplateDTO(item workorchmanagementcontract.WorkspaceTemplate) (workorchdevents.CLITemplateInfo, error) {
	hints, err := workorcheventpayload.FromMap(item.ContextHints)
	if err != nil {
		return workorchdevents.CLITemplateInfo{}, err
	}
	policies, err := workorcheventpayload.FromMapSlice(item.EvidencePolicies)
	if err != nil {
		return workorchdevents.CLITemplateInfo{}, err
	}
	return workorchdevents.CLITemplateInfo{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: common.SoulID(item.DefaultSoulID), SoulBindings: cliSoulBindingsFromStringMap(item.SoulBindings), ContextHints: hints, EvidencePolicies: policies}, nil
}

func TemplateDTOs(items []workorchmanagementcontract.WorkspaceTemplate) ([]workorchdevents.CLITemplateInfo, error) {
	if items == nil {
		return nil, nil
	}
	ret := make([]workorchdevents.CLITemplateInfo, 0, len(items))
	for _, item := range items {
		dto, err := TemplateDTO(item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, dto)
	}
	return ret, nil
}

func TemplateManagement(item workorchdevents.CLITemplateInfo) (workorchmanagementcontract.WorkspaceTemplate, error) {
	if err := soulbinding.Validate(item.SoulBindings); err != nil {
		return workorchmanagementcontract.WorkspaceTemplate{}, err
	}
	return workorchmanagementcontract.WorkspaceTemplate{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: string(item.DefaultSoulID), SoulBindings: soulbinding.StringMap(item.SoulBindings), ContextHints: item.ContextHints.Map(), EvidencePolicies: workorcheventpayload.MapSlice(item.EvidencePolicies)}, nil
}

func TemplateManagements(items []workorchdevents.CLITemplateInfo) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	if items == nil {
		return nil, nil
	}
	ret := make([]workorchmanagementcontract.WorkspaceTemplate, 0, len(items))
	for _, item := range items {
		template, err := TemplateManagement(item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, template)
	}
	return ret, nil
}

func cliSoulBindingsFromStringMap(input map[string]string) []soulbinding.Binding {
	return soulbinding.FromStringMap(input)
}

func SnapshotDTO(item projectioncontract.WorkspaceSnapshot) (workorchdevents.CLIWorkspaceSnapshotInfo, error) {
	workspace, err := WorkspaceDTO(item.Workspace)
	if err != nil {
		return workorchdevents.CLIWorkspaceSnapshotInfo{}, err
	}
	step, err := stepViewDTO(item.Step)
	if err != nil {
		return workorchdevents.CLIWorkspaceSnapshotInfo{}, err
	}
	gap, err := gapBaselineDTO(item.GapBaseline)
	if err != nil {
		return workorchdevents.CLIWorkspaceSnapshotInfo{}, err
	}
	progress, err := taskProgressDTO(item.TaskProgress)
	if err != nil {
		return workorchdevents.CLIWorkspaceSnapshotInfo{}, err
	}
	artifacts, err := artifactDTOs(item.Artifacts)
	if err != nil {
		return workorchdevents.CLIWorkspaceSnapshotInfo{}, err
	}
	continuations, err := artifactContinuationDTOs(item.ArtifactContinuations)
	if err != nil {
		return workorchdevents.CLIWorkspaceSnapshotInfo{}, err
	}
	patches, err := patchDTOs(item.Patches)
	if err != nil {
		return workorchdevents.CLIWorkspaceSnapshotInfo{}, err
	}
	decompositions, err := taskDecompositionDTOs(item.TaskDecompositions)
	if err != nil {
		return workorchdevents.CLIWorkspaceSnapshotInfo{}, err
	}
	plans, err := taskPlanDTOs(item.TaskPlans)
	if err != nil {
		return workorchdevents.CLIWorkspaceSnapshotInfo{}, err
	}
	coverageSummary, err := workorcheventpayload.FromMap(item.CoverageSummary)
	if err != nil {
		return workorchdevents.CLIWorkspaceSnapshotInfo{}, err
	}
	return workorchdevents.CLIWorkspaceSnapshotInfo{
		Workspace: workspace, ViewModel: WorkspaceViewModelDTO(item.ViewModel), Projection: projectioncontract.CloneProjectionStatus(item.Workspace.Projection),
		Run: cloneRunProjection(item.Run), Step: step, Result: cloneResultInfo(item.Result),
		Outcome: projectioncontract.CloneWorkspaceOutcome(item.Outcome), TaskProgress: progress, RunHeartbeat: item.RunHeartbeat, ConvergenceStatus: projectioncontract.CloneConvergenceStatus(item.ConvergenceStatus),
		RunCommand: projectioncontract.CloneRunCommandStatus(item.RunCommand), SchedulerDiagnostics: projectioncontract.CloneSchedulerDiagnostics(item.SchedulerDiagnostics), ContextBundle: contextBundleDTO(item.ContextBundle), CoverageSummary: coverageSummary, GapBaseline: gap, AuthorityDiagnostics: projectioncontract.CloneGapDiagnostics(item.AuthorityDiagnostics),
		Acceptance: projectioncontract.CloneAcceptanceProjection(item.Acceptance), TaskDecompositions: decompositions, TaskPlans: plans, ExecutionSections: projectioncontract.CloneExecutionSections(item.ExecutionSections),
		Goals: workorchmanagementcontract.CloneGoals(item.Goals), DoDs: workorchmanagementcontract.CloneDoDs(item.DoDs), Runs: projectioncontract.CloneRuns(item.Runs),
		Results: projectioncontract.CloneResults(item.Results), Artifacts: artifacts, ArtifactContinuations: continuations,
		Patches: patches, MemoryRecallTraces: memoryRecallTraceDTOs(item.MemoryRecallTraces), WebSources: projectioncontract.CloneWebSourceTraces(item.WebSources),
		LLMContextBudgets: projectioncontract.CloneLLMContextBudgetTraces(item.LLMContextBudgets), LLMContextBudgetSummary: item.LLMContextBudgetSummary,
		LLMUsageTraces: projectioncontract.CloneLLMUsageTraces(item.LLMUsageTraces), LLMUsageSummary: projectioncontract.CloneLLMUsageSummary(item.LLMUsageSummary),
		EventRecords: projectioncontract.CloneEventRecords(item.EventRecords), Decisions: projectioncontract.CloneDecisions(item.Decisions), Approvals: projectioncontract.CloneApprovals(item.Approvals),
	}, nil
}

func SnapshotPort(item workorchdevents.CLIWorkspaceSnapshotInfo) (projectioncontract.WorkspaceSnapshot, error) {
	workspace, err := WorkspaceProjection(item.Workspace)
	if err != nil {
		return projectioncontract.WorkspaceSnapshot{}, err
	}
	view, err := WorkspaceViewModelProjection(item.ViewModel)
	if err != nil {
		return projectioncontract.WorkspaceSnapshot{}, err
	}
	workspace.Projection = projectioncontract.CloneProjectionStatus(item.Projection)
	return projectioncontract.CloneWorkspaceSnapshot(projectioncontract.WorkspaceSnapshot{
		Workspace: workspace, ViewModel: view, Run: cloneRunProjection(item.Run), Step: stepViewProjection(item.Step), Result: cloneResultInfo(item.Result), Outcome: projectioncontract.CloneWorkspaceOutcome(item.Outcome), TaskProgress: taskProgressProjection(item.TaskProgress), RunHeartbeat: item.RunHeartbeat,
		ConvergenceStatus: projectioncontract.CloneConvergenceStatus(item.ConvergenceStatus), RunCommand: projectioncontract.CloneRunCommandStatus(item.RunCommand), SchedulerDiagnostics: projectioncontract.CloneSchedulerDiagnostics(item.SchedulerDiagnostics), ContextBundle: contextBundlePort(item.ContextBundle), CoverageSummary: item.CoverageSummary.Map(), GapBaseline: gapBaselineProjection(item.GapBaseline), AuthorityDiagnostics: projectioncontract.CloneGapDiagnostics(item.AuthorityDiagnostics),
		Acceptance: projectioncontract.CloneAcceptanceProjection(item.Acceptance), TaskDecompositions: taskDecompositionProjections(item.TaskDecompositions), TaskPlans: taskPlanProjections(item.TaskPlans), ExecutionSections: projectioncontract.CloneExecutionSections(item.ExecutionSections),
		Goals: workorchmanagementcontract.CloneGoals(item.Goals), DoDs: workorchmanagementcontract.CloneDoDs(item.DoDs), Runs: projectioncontract.CloneRuns(item.Runs),
		Results: projectioncontract.CloneResults(item.Results), Artifacts: artifactProjections(item.Artifacts), ArtifactContinuations: artifactContinuationProjections(item.ArtifactContinuations),
		Patches: patchProjections(item.Patches), MemoryRecallTraces: memoryRecallTraceProjections(item.MemoryRecallTraces), WebSources: projectioncontract.CloneWebSourceTraces(item.WebSources),
		LLMContextBudgets: projectioncontract.CloneLLMContextBudgetTraces(item.LLMContextBudgets), LLMContextBudgetSummary: item.LLMContextBudgetSummary,
		LLMUsageTraces: projectioncontract.CloneLLMUsageTraces(item.LLMUsageTraces), LLMUsageSummary: projectioncontract.CloneLLMUsageSummary(item.LLMUsageSummary),
		EventRecords: projectioncontract.CloneEventRecords(item.EventRecords), Decisions: projectioncontract.CloneDecisions(item.Decisions), Approvals: projectioncontract.CloneApprovals(item.Approvals),
	}), nil
}

func RuntimeViewDTO(item projectioncontract.WorkspaceRuntimeView) (workorchdevents.CLIWorkspaceRuntimeViewInfo, error) {
	progress, err := taskProgressDTO(item.TaskProgress)
	if err != nil {
		return workorchdevents.CLIWorkspaceRuntimeViewInfo{}, err
	}
	return workorchdevents.CLIWorkspaceRuntimeViewInfo{
		WorkspaceID: common.WorkspaceID(item.WorkspaceID), ActiveRunID: common.RunID(item.ActiveRunID), DisplayRunID: common.RunID(item.DisplayRunID),
		Projection: projectioncontract.CloneProjectionStatus(item.Projection), Revision: item.Revision, PromptMode: item.PromptMode,
		RunState: common.RunState(item.RunState), TaskProgress: progress, RunHeartbeat: item.RunHeartbeat,
		ConvergenceStatus: projectioncontract.CloneConvergenceStatus(item.ConvergenceStatus), Acceptance: projectioncontract.CloneAcceptanceProjection(item.Acceptance),
		RunCommand: projectioncontract.CloneRunCommandStatus(item.RunCommand), BlockingDecision: cloneDecision(item.BlockingDecision),
		BlockingApproval: cloneApproval(item.BlockingApproval), Outcome: projectioncontract.CloneWorkspaceOutcome(item.Outcome),
		Activity: projectioncontract.CloneWorkspaceActivityFeed(item.Activity),
	}, nil
}

func RuntimeViewPort(item workorchdevents.CLIWorkspaceRuntimeViewInfo) projectioncontract.WorkspaceRuntimeView {
	return projectioncontract.CloneWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID: string(item.WorkspaceID), ActiveRunID: string(item.ActiveRunID), DisplayRunID: string(item.DisplayRunID),
		Projection: projectioncontract.CloneProjectionStatus(item.Projection), Revision: item.Revision, PromptMode: item.PromptMode,
		RunState: string(item.RunState), TaskProgress: taskProgressProjection(item.TaskProgress), RunHeartbeat: item.RunHeartbeat,
		ConvergenceStatus: projectioncontract.CloneConvergenceStatus(item.ConvergenceStatus), Acceptance: projectioncontract.CloneAcceptanceProjection(item.Acceptance),
		RunCommand: projectioncontract.CloneRunCommandStatus(item.RunCommand), BlockingDecision: cloneDecision(item.BlockingDecision),
		BlockingApproval: cloneApproval(item.BlockingApproval), Outcome: projectioncontract.CloneWorkspaceOutcome(item.Outcome),
		Activity: projectioncontract.CloneWorkspaceActivityFeed(item.Activity),
	})
}

func cloneDecision(item *projectioncontract.Decision) *projectioncontract.Decision {
	if item == nil {
		return nil
	}
	value := projectioncontract.CloneDecision(*item)
	return &value
}

func cloneApproval(item *projectioncontract.Approval) *projectioncontract.Approval {
	if item == nil {
		return nil
	}
	value := projectioncontract.CloneApproval(*item)
	return &value
}

func cloneRunProjection(item *projectioncontract.Run) *projectioncontract.Run {
	if item == nil {
		return nil
	}
	ret := projectioncontract.CloneRun(*item)
	return &ret
}

func cloneResultInfo(item *projectioncontract.Result) *projectioncontract.Result {
	if item == nil {
		return nil
	}
	ret := projectioncontract.CloneResult(*item)
	return &ret
}

// The CLI EventHub graph remains map-free. It carries the bounded context
// identity/summary needed by the process-facing CLI rather than embedding the
// HTTP projection contract (whose section data intentionally permits maps).
func contextBundleDTO(item *projectioncontract.ContextBundle) workorcheventpayload.Payload {
	if item == nil {
		return workorcheventpayload.Payload{}
	}
	return workorcheventpayload.New(
		workorcheventpayload.StringField("workspaceId", item.WorkspaceID),
		workorcheventpayload.StringField("runId", item.RunID),
		workorcheventpayload.StringField("mode", item.Mode),
		workorcheventpayload.StringField("summary", item.Summary),
	)
}

func contextBundlePort(item workorcheventpayload.Payload) *projectioncontract.ContextBundle {
	workspaceID, _ := item.String("workspaceId")
	runID, _ := item.String("runId")
	mode, _ := item.String("mode")
	summary, _ := item.String("summary")
	if workspaceID == "" && runID == "" && mode == "" && summary == "" {
		return nil
	}
	return &projectioncontract.ContextBundle{WorkspaceID: workspaceID, RunID: runID, Mode: mode, Summary: summary}
}

func memoryRecallTraceDTOs(items []projectioncontract.MemoryRecallTrace) []workorchdevents.CLIMemoryRecallTraceInfo {
	if items == nil {
		return nil
	}
	ret := make([]workorchdevents.CLIMemoryRecallTraceInfo, 0, len(items))
	for _, item := range items {
		quotas := make([]workorchmemorycontract.TypeQuotaEntry, 0, len(item.TypeQuota))
		for key, value := range item.TypeQuota {
			quotas = append(quotas, workorchmemorycontract.TypeQuotaEntry{Type: key, Quota: value})
		}
		sort.Slice(quotas, func(i, j int) bool { return quotas[i].Type < quotas[j].Type })
		ret = append(ret, workorchdevents.CLIMemoryRecallTraceInfo{
			ID: common.MemoryRecallTraceID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), RunID: common.RunID(item.RunID), StepID: common.StepID(item.StepID),
			GraphNodeID: item.GraphNodeID, Stage: item.Stage, Source: item.Source, Policy: item.Policy, Status: item.Status,
			ProviderID: item.ProviderID, Operation: item.Operation, QueryTextPreview: item.QueryTextPreview, QueryTextSha256: item.QueryTextSha256,
			MinScore: item.MinScore, Limit: item.Limit, EmbeddingModel: item.EmbeddingModel, VectorDimension: item.VectorDimension,
			RefreshSource: item.RefreshSource, SelectionPolicyVersion: item.SelectionPolicyVersion,
			CandidateMemoryIDs: memoryIDsFromStrings(item.CandidateMemoryIDs), SelectedMemoryIDs: memoryIDsFromStrings(item.SelectedMemoryIDs), ExcludedMemoryIDs: memoryIDsFromStrings(item.ExcludedMemoryIDs),
			TypeQuota: quotas, SelectionScores: slices.Clone(item.SelectionScores), RetrievedMemoryIDs: memoryIDsFromStrings(item.RetrievedMemoryIDs), RetrievedMemoryHits: slices.Clone(item.RetrievedMemoryHits),
			Reason: item.Reason, OccurredAt: item.OccurredAt, EvidenceRef: item.EvidenceRef,
		})
	}
	return ret
}

func memoryRecallTraceProjections(items []workorchdevents.CLIMemoryRecallTraceInfo) []projectioncontract.MemoryRecallTrace {
	if items == nil {
		return nil
	}
	ret := make([]projectioncontract.MemoryRecallTrace, 0, len(items))
	for _, item := range items {
		var quotas map[string]int
		if item.TypeQuota != nil {
			quotas = make(map[string]int, len(item.TypeQuota))
		}
		for _, quota := range item.TypeQuota {
			quotas[quota.Type] = quota.Quota
		}
		ret = append(ret, projectioncontract.MemoryRecallTrace{
			ID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID), StepID: string(item.StepID),
			GraphNodeID: item.GraphNodeID, Stage: item.Stage, Source: item.Source, Policy: item.Policy, Status: item.Status,
			ProviderID: item.ProviderID, Operation: item.Operation, QueryTextPreview: item.QueryTextPreview, QueryTextSha256: item.QueryTextSha256,
			MinScore: item.MinScore, Limit: item.Limit, EmbeddingModel: item.EmbeddingModel, VectorDimension: item.VectorDimension,
			RefreshSource: item.RefreshSource, SelectionPolicyVersion: item.SelectionPolicyVersion,
			CandidateMemoryIDs: memoryIDStrings(item.CandidateMemoryIDs), SelectedMemoryIDs: memoryIDStrings(item.SelectedMemoryIDs), ExcludedMemoryIDs: memoryIDStrings(item.ExcludedMemoryIDs),
			TypeQuota: quotas, SelectionScores: slices.Clone(item.SelectionScores), RetrievedMemoryIDs: memoryIDStrings(item.RetrievedMemoryIDs), RetrievedMemoryHits: slices.Clone(item.RetrievedMemoryHits),
			Reason: item.Reason, OccurredAt: item.OccurredAt, EvidenceRef: item.EvidenceRef,
		})
	}
	return ret
}

func memoryIDsFromStrings(items []string) []common.MemoryID {
	if items == nil {
		return nil
	}
	ret := make([]common.MemoryID, 0, len(items))
	for _, item := range items {
		ret = append(ret, common.MemoryID(item))
	}
	return ret
}

func memoryIDStrings(items []common.MemoryID) []string {
	if items == nil {
		return nil
	}
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, string(item))
	}
	return ret
}
