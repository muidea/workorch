// Package eventmapping contains pure, stateless conversions between the
// workorchd EventHub DTOs and process-facing port DTOs.
package eventmapping

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
)

func WorkspaceDTO(item workorchport.WorkspaceInfo) workorchdevents.CLIWorkspaceInfo {
	var bindings []workorchdevents.CLISoulBinding
	if item.SoulBindings != nil {
		bindings = make([]workorchdevents.CLISoulBinding, 0, len(item.SoulBindings))
	}
	for stage, soulID := range item.SoulBindings {
		bindings = append(bindings, workorchdevents.CLISoulBinding{Stage: stage, SoulID: soulID})
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Stage < bindings[j].Stage })
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
	var sections []workorchdevents.CLIWorkspaceViewModelSection
	if item.Sections != nil {
		sections = make([]workorchdevents.CLIWorkspaceViewModelSection, 0, len(item.Sections))
	}
	for name, section := range item.Sections {
		sections = append(sections, workorchdevents.CLIWorkspaceViewModelSection{Name: name, Revision: section.Revision, WorkspaceRevision: section.WorkspaceRevision, SourceEventRevision: section.SourceEventRevision, ProjectedSourceRevision: section.ProjectedSourceRevision, State: section.State})
	}
	sort.Slice(sections, func(i, j int) bool { return sections[i].Name < sections[j].Name })
	return workorchdevents.CLIWorkspaceViewModelInfo{WorkspaceID: item.WorkspaceID, State: item.State, PromptMode: item.PromptMode, Revision: item.Revision, GlobalRevision: item.GlobalRevision, WorkspaceRevision: item.WorkspaceRevision, SourceEventRevision: item.SourceEventRevision, ProjectedSourceRevision: item.ProjectedSourceRevision, UpdatedAt: item.UpdatedAt, RebuiltAt: item.RebuiltAt, Degraded: item.Degraded, StaleReason: item.StaleReason, Error: item.Error, Sections: sections}
}

func WorkspaceViewModelPort(item workorchdevents.CLIWorkspaceViewModelInfo) (workorchport.WorkspaceViewModelInfo, error) {
	var sections map[string]workorchport.WorkspaceViewModelSectionInfo
	if item.Sections != nil {
		sections = make(map[string]workorchport.WorkspaceViewModelSectionInfo, len(item.Sections))
	}
	for _, section := range item.Sections {
		name := strings.TrimSpace(section.Name)
		if name == "" {
			return workorchport.WorkspaceViewModelInfo{}, fmt.Errorf("%w: CLI workspace view section requires name", common.ErrInvalidArgument)
		}
		if _, exists := sections[name]; exists {
			return workorchport.WorkspaceViewModelInfo{}, fmt.Errorf("%w: duplicate CLI workspace view section %q", common.ErrInvalidArgument, name)
		}
		sections[name] = workorchport.WorkspaceViewModelSectionInfo{Name: name, Revision: section.Revision, WorkspaceRevision: section.WorkspaceRevision, SourceEventRevision: section.SourceEventRevision, ProjectedSourceRevision: section.ProjectedSourceRevision, State: section.State}
	}
	return workorchport.WorkspaceViewModelInfo{WorkspaceID: item.WorkspaceID, State: item.State, PromptMode: item.PromptMode, Revision: item.Revision, GlobalRevision: item.GlobalRevision, WorkspaceRevision: item.WorkspaceRevision, SourceEventRevision: item.SourceEventRevision, ProjectedSourceRevision: item.ProjectedSourceRevision, UpdatedAt: item.UpdatedAt, RebuiltAt: item.RebuiltAt, Degraded: item.Degraded, StaleReason: item.StaleReason, Error: item.Error, Sections: sections}, nil
}

func TemplateDTO(item workorchport.TemplateInfo) workorchdevents.CLITemplateInfo {
	return workorchdevents.CLITemplateInfo{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: item.DefaultSoulID, SoulBindings: workspaceevents.SoulBindingsFromStringMap(item.SoulBindings), ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies)}
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
	if err := workspaceevents.ValidateSoulBindings(item.SoulBindings); err != nil {
		return workorchport.TemplateInfo{}, err
	}
	return workorchport.TemplateInfo{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: item.DefaultSoulID, SoulBindings: workspaceevents.StringMapFromBindings(item.SoulBindings), ContextHints: item.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies)}, nil
}

func SnapshotDTO(item workorchport.WorkspaceSnapshotInfo) workorchdevents.CLIWorkspaceSnapshotInfo {
	return workorchdevents.CLIWorkspaceSnapshotInfo{
		Workspace: WorkspaceDTO(item.Workspace), ViewModel: WorkspaceViewModelDTO(item.ViewModel), Projection: item.Projection,
		Outcome: item.Outcome, TaskProgress: item.TaskProgress, RunHeartbeat: item.RunHeartbeat, ConvergenceStatus: item.ConvergenceStatus,
		RunCommand: item.RunCommand, SchedulerDiagnostics: item.SchedulerDiagnostics, ContextBundle: item.ContextBundle.Clone(), CoverageSummary: item.CoverageSummary.Clone(), GapBaseline: item.GapBaseline,
		Acceptance: item.Acceptance, TaskDecompositions: slices.Clone(item.TaskDecompositions), TaskPlans: slices.Clone(item.TaskPlans),
		Goals: slices.Clone(item.Goals), DoDs: slices.Clone(item.DoDs), Runs: slices.Clone(item.Runs),
		Results: slices.Clone(item.Results), Artifacts: slices.Clone(item.Artifacts), ArtifactContinuations: slices.Clone(item.ArtifactContinuations),
		Patches: slices.Clone(item.Patches), MemoryRecallTraces: slices.Clone(item.MemoryRecallTraces), WebSources: slices.Clone(item.WebSources),
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
		ConvergenceStatus: item.ConvergenceStatus, RunCommand: item.RunCommand, SchedulerDiagnostics: item.SchedulerDiagnostics, ContextBundle: item.ContextBundle.Clone(), CoverageSummary: item.CoverageSummary.Clone(), GapBaseline: item.GapBaseline,
		Acceptance: item.Acceptance, TaskDecompositions: slices.Clone(item.TaskDecompositions), TaskPlans: slices.Clone(item.TaskPlans),
		Goals: slices.Clone(item.Goals), DoDs: slices.Clone(item.DoDs), Runs: slices.Clone(item.Runs),
		Results: slices.Clone(item.Results), Artifacts: slices.Clone(item.Artifacts), ArtifactContinuations: slices.Clone(item.ArtifactContinuations),
		Patches: slices.Clone(item.Patches), MemoryRecallTraces: slices.Clone(item.MemoryRecallTraces), WebSources: slices.Clone(item.WebSources),
		LLMContextBudgets: slices.Clone(item.LLMContextBudgets), LLMContextBudgetSummary: item.LLMContextBudgetSummary,
		LLMUsageTraces: slices.Clone(item.LLMUsageTraces), LLMUsageSummary: item.LLMUsageSummary,
		EventRecords: slices.Clone(item.EventRecords), Decisions: slices.Clone(item.Decisions), Approvals: slices.Clone(item.Approvals),
	}, nil
}
