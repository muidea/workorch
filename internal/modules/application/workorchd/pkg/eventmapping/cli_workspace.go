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
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func WorkspaceDTO(item workorchport.WorkspaceInfo) workorchdevents.CLIWorkspaceInfo {
	bindings := soulbinding.FromSoulIDMap(item.SoulBindings)
	ret := workorchdevents.CLIWorkspaceInfo{
		ID: item.ID, Title: item.Title, Description: item.Description, DeliverableDirectory: item.DeliverableDirectory,
		State: item.State, ActiveRunID: item.ActiveRunID, BlockingTicketID: item.BlockingTicketID, BlockingApprovalID: item.BlockingApprovalID,
		TemplateID: item.TemplateID, ProfileID: item.ProfileID, DefaultSoulID: item.DefaultSoulID, SoulBindings: bindings,
		ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies),
		Projection: item.Projection, ViewModel: WorkspaceViewModelDTO(item.ViewModel),
	}
	if item.DefaultLLMTarget != nil {
		value := item.DefaultLLMTarget.Clone()
		ret.DefaultLLMTarget = &value
	}
	return ret
}

func WorkspaceDTOs(items []workorchport.WorkspaceInfo) []workorchdevents.CLIWorkspaceInfo {
	if items == nil {
		return nil
	}
	ret := make([]workorchdevents.CLIWorkspaceInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, WorkspaceDTO(item))
	}
	return ret
}

func WorkspacePort(item workorchdevents.CLIWorkspaceInfo) (workorchport.WorkspaceInfo, error) {
	var bindings map[string]common.SoulID
	if item.SoulBindings != nil {
		bindings = make(map[string]common.SoulID, len(item.SoulBindings))
	}
	for _, binding := range item.SoulBindings {
		stage := strings.TrimSpace(binding.Stage)
		if stage == "" || binding.SoulID == "" {
			return workorchport.WorkspaceInfo{}, fmt.Errorf("%w: invalid CLI workspace soul binding", common.ErrInvalidArgument)
		}
		if _, exists := bindings[stage]; exists {
			return workorchport.WorkspaceInfo{}, fmt.Errorf("%w: duplicate CLI workspace soul binding %q", common.ErrInvalidArgument, stage)
		}
		bindings[stage] = binding.SoulID
	}
	view, err := WorkspaceViewModelPort(item.ViewModel)
	if err != nil {
		return workorchport.WorkspaceInfo{}, err
	}
	ret := workorchport.WorkspaceInfo{
		ID: item.ID, Title: item.Title, Description: item.Description, DeliverableDirectory: item.DeliverableDirectory,
		State: item.State, ActiveRunID: item.ActiveRunID, BlockingTicketID: item.BlockingTicketID, BlockingApprovalID: item.BlockingApprovalID,
		TemplateID: item.TemplateID, ProfileID: item.ProfileID, DefaultSoulID: item.DefaultSoulID, SoulBindings: bindings,
		ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies), Projection: item.Projection, ViewModel: view,
	}
	if item.DefaultLLMTarget != nil {
		value := item.DefaultLLMTarget.Clone()
		ret.DefaultLLMTarget = &value
	}
	return ret, nil
}

func WorkspaceViewModelDTO(item workorchport.WorkspaceViewModelInfo) workorchdevents.CLIWorkspaceViewModelInfo {
	var sections []projectioncontract.WorkspaceViewModelSection
	if item.Sections != nil {
		sections = make([]projectioncontract.WorkspaceViewModelSection, 0, len(item.Sections))
	}
	for name, section := range item.Sections {
		sections = append(sections, projectioncontract.WorkspaceViewModelSection{Name: name, Revision: section.Revision, WorkspaceRevision: section.WorkspaceRevision, SourceEventRevision: section.SourceEventRevision, ProjectedSourceRevision: section.ProjectedSourceRevision, State: section.State})
	}
	sort.Slice(sections, func(i, j int) bool { return sections[i].Name < sections[j].Name })
	return workorchdevents.CLIWorkspaceViewModelInfo{WorkspaceID: item.WorkspaceID, State: item.State, PromptMode: item.PromptMode, Revision: item.Revision, GlobalRevision: item.GlobalRevision, WorkspaceRevision: item.WorkspaceRevision, SourceEventRevision: item.SourceEventRevision, ProjectedSourceRevision: item.ProjectedSourceRevision, UpdatedAt: item.UpdatedAt, RebuiltAt: item.RebuiltAt, Degraded: item.Degraded, StaleReason: item.StaleReason, Error: item.Error, Sections: sections}
}

func WorkspaceViewModelPort(item workorchdevents.CLIWorkspaceViewModelInfo) (workorchport.WorkspaceViewModelInfo, error) {
	var sections map[string]projectioncontract.WorkspaceViewModelSection
	if item.Sections != nil {
		sections = make(map[string]projectioncontract.WorkspaceViewModelSection, len(item.Sections))
	}
	for _, section := range item.Sections {
		name := strings.TrimSpace(section.Name)
		if name == "" {
			return workorchport.WorkspaceViewModelInfo{}, fmt.Errorf("%w: CLI workspace view section requires name", common.ErrInvalidArgument)
		}
		if _, exists := sections[name]; exists {
			return workorchport.WorkspaceViewModelInfo{}, fmt.Errorf("%w: duplicate CLI workspace view section %q", common.ErrInvalidArgument, name)
		}
		sections[name] = projectioncontract.WorkspaceViewModelSection{Name: name, Revision: section.Revision, WorkspaceRevision: section.WorkspaceRevision, SourceEventRevision: section.SourceEventRevision, ProjectedSourceRevision: section.ProjectedSourceRevision, State: section.State}
	}
	return workorchport.WorkspaceViewModelInfo{WorkspaceID: item.WorkspaceID, State: item.State, PromptMode: item.PromptMode, Revision: item.Revision, GlobalRevision: item.GlobalRevision, WorkspaceRevision: item.WorkspaceRevision, SourceEventRevision: item.SourceEventRevision, ProjectedSourceRevision: item.ProjectedSourceRevision, UpdatedAt: item.UpdatedAt, RebuiltAt: item.RebuiltAt, Degraded: item.Degraded, StaleReason: item.StaleReason, Error: item.Error, Sections: sections}, nil
}

func TemplateDTO(item workorchport.TemplateInfo) workorchdevents.CLITemplateInfo {
	return workorchdevents.CLITemplateInfo{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: item.DefaultSoulID, SoulBindings: cliSoulBindingsFromStringMap(item.SoulBindings), ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies)}
}

func TemplateDTOs(items []workorchport.TemplateInfo) []workorchdevents.CLITemplateInfo {
	if items == nil {
		return nil
	}
	ret := make([]workorchdevents.CLITemplateInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, TemplateDTO(item))
	}
	return ret
}

func TemplatePort(item workorchdevents.CLITemplateInfo) (workorchport.TemplateInfo, error) {
	if err := soulbinding.Validate(item.SoulBindings); err != nil {
		return workorchport.TemplateInfo{}, err
	}
	return workorchport.TemplateInfo{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: item.DefaultSoulID, SoulBindings: soulbinding.StringMap(item.SoulBindings), ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies)}, nil
}

func cliSoulBindingsFromStringMap(input map[string]string) []soulbinding.Binding {
	return soulbinding.FromStringMap(input)
}

func SnapshotDTO(item workorchport.WorkspaceSnapshotInfo) workorchdevents.CLIWorkspaceSnapshotInfo {
	return workorchdevents.CLIWorkspaceSnapshotInfo{
		Workspace: WorkspaceDTO(item.Workspace), ViewModel: WorkspaceViewModelDTO(item.ViewModel), Projection: item.Projection,
		Outcome: item.Outcome, TaskProgress: item.TaskProgress, RunHeartbeat: item.RunHeartbeat, ConvergenceStatus: item.ConvergenceStatus,
		RunCommand: item.RunCommand, SchedulerDiagnostics: item.SchedulerDiagnostics, ContextBundle: contextBundleDTO(item.ContextBundle), CoverageSummary: item.CoverageSummary.Clone(), GapBaseline: item.GapBaseline,
		Acceptance: item.Acceptance, TaskDecompositions: slices.Clone(item.TaskDecompositions), TaskPlans: slices.Clone(item.TaskPlans),
		Goals: slices.Clone(item.Goals), DoDs: slices.Clone(item.DoDs), Runs: slices.Clone(item.Runs),
		Results: slices.Clone(item.Results), Artifacts: slices.Clone(item.Artifacts), ArtifactContinuations: slices.Clone(item.ArtifactContinuations),
		Patches: slices.Clone(item.Patches), MemoryRecallTraces: memoryRecallTraceDTOs(item.MemoryRecallTraces), WebSources: slices.Clone(item.WebSources),
		LLMContextBudgets: slices.Clone(item.LLMContextBudgets), LLMContextBudgetSummary: item.LLMContextBudgetSummary,
		LLMUsageTraces: slices.Clone(item.LLMUsageTraces), LLMUsageSummary: item.LLMUsageSummary,
		EventRecords: slices.Clone(item.EventRecords), Decisions: slices.Clone(item.Decisions), Approvals: slices.Clone(item.Approvals),
	}
}

func SnapshotPort(item workorchdevents.CLIWorkspaceSnapshotInfo) (workorchport.WorkspaceSnapshotInfo, error) {
	workspace, err := WorkspacePort(item.Workspace)
	if err != nil {
		return workorchport.WorkspaceSnapshotInfo{}, err
	}
	view, err := WorkspaceViewModelPort(item.ViewModel)
	if err != nil {
		return workorchport.WorkspaceSnapshotInfo{}, err
	}
	return workorchport.WorkspaceSnapshotInfo{
		Workspace: workspace, ViewModel: view, Projection: item.Projection, Outcome: item.Outcome, TaskProgress: item.TaskProgress, RunHeartbeat: item.RunHeartbeat,
		ConvergenceStatus: item.ConvergenceStatus, RunCommand: item.RunCommand, SchedulerDiagnostics: item.SchedulerDiagnostics, ContextBundle: contextBundlePort(item.ContextBundle), CoverageSummary: item.CoverageSummary.Clone(), GapBaseline: item.GapBaseline,
		Acceptance: item.Acceptance, TaskDecompositions: slices.Clone(item.TaskDecompositions), TaskPlans: slices.Clone(item.TaskPlans),
		Goals: slices.Clone(item.Goals), DoDs: slices.Clone(item.DoDs), Runs: slices.Clone(item.Runs),
		Results: slices.Clone(item.Results), Artifacts: slices.Clone(item.Artifacts), ArtifactContinuations: slices.Clone(item.ArtifactContinuations),
		Patches: slices.Clone(item.Patches), MemoryRecallTraces: memoryRecallTracePorts(item.MemoryRecallTraces), WebSources: slices.Clone(item.WebSources),
		LLMContextBudgets: slices.Clone(item.LLMContextBudgets), LLMContextBudgetSummary: item.LLMContextBudgetSummary,
		LLMUsageTraces: slices.Clone(item.LLMUsageTraces), LLMUsageSummary: item.LLMUsageSummary,
		EventRecords: slices.Clone(item.EventRecords), Decisions: slices.Clone(item.Decisions), Approvals: slices.Clone(item.Approvals),
	}, nil
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

func memoryRecallTraceDTOs(items []workorchport.MemoryRecallTraceInfo) []workorchdevents.CLIMemoryRecallTraceInfo {
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
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
			GraphNodeID: item.GraphNodeID, Stage: item.Stage, Source: item.Source, Policy: item.Policy, Status: item.Status,
			ProviderID: item.ProviderID, Operation: item.Operation, QueryTextPreview: item.QueryTextPreview, QueryTextSha256: item.QueryTextSha256,
			MinScore: item.MinScore, Limit: item.Limit, EmbeddingModel: item.EmbeddingModel, VectorDimension: item.VectorDimension,
			RefreshSource: item.RefreshSource, SelectionPolicyVersion: item.SelectionPolicyVersion,
			CandidateMemoryIDs: slices.Clone(item.CandidateMemoryIDs), SelectedMemoryIDs: slices.Clone(item.SelectedMemoryIDs), ExcludedMemoryIDs: slices.Clone(item.ExcludedMemoryIDs),
			TypeQuota: quotas, SelectionScores: slices.Clone(item.SelectionScores), RetrievedMemoryIDs: slices.Clone(item.RetrievedMemoryIDs), RetrievedMemoryHits: slices.Clone(item.RetrievedMemoryHits),
			Reason: item.Reason, OccurredAt: item.OccurredAt, EvidenceRef: item.EvidenceRef,
		})
	}
	return ret
}

func memoryRecallTracePorts(items []workorchdevents.CLIMemoryRecallTraceInfo) []workorchport.MemoryRecallTraceInfo {
	if items == nil {
		return nil
	}
	ret := make([]workorchport.MemoryRecallTraceInfo, 0, len(items))
	for _, item := range items {
		var quotas map[string]int
		if item.TypeQuota != nil {
			quotas = make(map[string]int, len(item.TypeQuota))
		}
		for _, quota := range item.TypeQuota {
			quotas[quota.Type] = quota.Quota
		}
		ret = append(ret, workorchport.MemoryRecallTraceInfo{
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
			GraphNodeID: item.GraphNodeID, Stage: item.Stage, Source: item.Source, Policy: item.Policy, Status: item.Status,
			ProviderID: item.ProviderID, Operation: item.Operation, QueryTextPreview: item.QueryTextPreview, QueryTextSha256: item.QueryTextSha256,
			MinScore: item.MinScore, Limit: item.Limit, EmbeddingModel: item.EmbeddingModel, VectorDimension: item.VectorDimension,
			RefreshSource: item.RefreshSource, SelectionPolicyVersion: item.SelectionPolicyVersion,
			CandidateMemoryIDs: slices.Clone(item.CandidateMemoryIDs), SelectedMemoryIDs: slices.Clone(item.SelectedMemoryIDs), ExcludedMemoryIDs: slices.Clone(item.ExcludedMemoryIDs),
			TypeQuota: quotas, SelectionScores: slices.Clone(item.SelectionScores), RetrievedMemoryIDs: slices.Clone(item.RetrievedMemoryIDs), RetrievedMemoryHits: slices.Clone(item.RetrievedMemoryHits),
			Reason: item.Reason, OccurredAt: item.OccurredAt, EvidenceRef: item.EvidenceRef,
		})
	}
	return ret
}
