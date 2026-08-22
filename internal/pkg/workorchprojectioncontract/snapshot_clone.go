package workorchprojectioncontract

import workorchmanagementcontract "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"

func CloneWorkspaceSnapshot(item WorkspaceSnapshot) WorkspaceSnapshot {
	item.Workspace = CloneWorkspace(item.Workspace)
	item.ViewModel = CloneWorkspaceViewModel(item.ViewModel)
	item.Run = cloneRunProjectionPointer(item.Run)
	item.Step = CloneStepView(item.Step)
	item.Result = cloneResultProjectionPointer(item.Result)
	item.Outcome = CloneWorkspaceOutcome(item.Outcome)
	item.TaskProgress = CloneTaskProgress(item.TaskProgress)
	item.ConvergenceStatus = CloneConvergenceStatus(item.ConvergenceStatus)
	item.RunCommand = CloneRunCommandStatus(item.RunCommand)
	item.ContextBundle = CloneContextBundle(item.ContextBundle)
	item.CoverageSummary = cloneJSONMap(item.CoverageSummary)
	item.GapBaseline = CloneGapBaseline(item.GapBaseline)
	item.AuthorityDiagnostics = CloneGapDiagnostics(item.AuthorityDiagnostics)
	item.Acceptance = CloneAcceptanceProjection(item.Acceptance)
	item.TaskDecompositions = CloneTaskDecompositions(item.TaskDecompositions)
	item.TaskPlans = CloneTaskPlans(item.TaskPlans)
	item.ExecutionSections = cloneExecutionSections(item.ExecutionSections)
	item.MemoryRecallTraces = CloneMemoryRecallTraces(item.MemoryRecallTraces)
	item.Goals = workorchmanagementcontract.CloneGoals(item.Goals)
	item.DoDs = workorchmanagementcontract.CloneDoDs(item.DoDs)
	item.Runs = CloneRuns(item.Runs)
	item.Results = CloneResults(item.Results)
	item.Artifacts = CloneArtifacts(item.Artifacts)
	item.ArtifactContinuations = CloneArtifactContinuations(item.ArtifactContinuations)
	item.Patches = ClonePatches(item.Patches)
	item.WebSources = CloneWebSourceTraces(item.WebSources)
	item.LLMContextBudgets = CloneLLMContextBudgetTraces(item.LLMContextBudgets)
	item.LLMUsageTraces = CloneLLMUsageTraces(item.LLMUsageTraces)
	item.LLMUsageSummary = CloneLLMUsageSummary(item.LLMUsageSummary)
	item.EventRecords = CloneEventRecords(item.EventRecords)
	item.Decisions = CloneDecisions(item.Decisions)
	item.Approvals = CloneApprovals(item.Approvals)
	return item
}

func CloneRunSnapshot(item RunSnapshot) RunSnapshot {
	item.Workspace = CloneWorkspace(item.Workspace)
	item.Run = CloneRun(item.Run)
	item.Outcome = cloneRunOutcome(item.Outcome)
	item.TaskProgress = CloneTaskProgress(item.TaskProgress)
	item.RepairLoop = CloneRepairLoop(item.RepairLoop)
	item.TaskDecompositions = CloneTaskDecompositions(item.TaskDecompositions)
	item.TaskPlans = CloneTaskPlans(item.TaskPlans)
	item.Step = CloneStepView(item.Step)
	item.Steps = cloneStepViews(item.Steps)
	item.ToolCalls = cloneToolCalls(item.ToolCalls)
	item.Patches = ClonePatches(item.Patches)
	item.GitOperations = cloneGitOperationTraces(item.GitOperations)
	item.WebSources = CloneWebSourceTraces(item.WebSources)
	item.ValidationResults = cloneValidationResults(item.ValidationResults)
	item.RuntimeCheckpoints = cloneRuntimeCheckpoints(item.RuntimeCheckpoints)
	item.ConvergenceStatus = CloneConvergenceStatus(item.ConvergenceStatus)
	item.Results = CloneResults(item.Results)
	item.ClosureTraces = cloneClosureTraces(item.ClosureTraces)
	item.DoDEvidence = cloneDoDEvidenceTraces(item.DoDEvidence)
	item.ResultQuality = cloneResultQualityTraces(item.ResultQuality)
	item.Artifacts = CloneArtifacts(item.Artifacts)
	item.ArtifactContinuations = CloneArtifactContinuations(item.ArtifactContinuations)
	item.ExecutionSections = cloneExecutionSections(item.ExecutionSections)
	item.LLMContextBudgets = CloneLLMContextBudgetTraces(item.LLMContextBudgets)
	item.LLMUsageTraces = CloneLLMUsageTraces(item.LLMUsageTraces)
	item.LLMUsageSummary = CloneLLMUsageSummary(item.LLMUsageSummary)
	item.CoverageWorkItems = cloneJSONMaps(item.CoverageWorkItems)
	item.CoverageWorkItemContext = cloneJSONMap(item.CoverageWorkItemContext)
	item.GapBaseline = CloneGapBaseline(item.GapBaseline)
	item.AuthorityDiagnostics = CloneGapDiagnostics(item.AuthorityDiagnostics)
	item.Acceptance = CloneAcceptanceProjection(item.Acceptance)
	item.MemoryRecallTraces = CloneMemoryRecallTraces(item.MemoryRecallTraces)
	item.SelectedCapabilities = cloneRuntimeCapabilities(item.SelectedCapabilities)
	item.UsedCapabilities = cloneRuntimeCapabilities(item.UsedCapabilities)
	item.EventRecords = CloneEventRecords(item.EventRecords)
	item.Decisions = CloneDecisions(item.Decisions)
	item.Approvals = CloneApprovals(item.Approvals)
	item.Observed = cloneObservedViews(item.Observed)
	return item
}

func CloneRunObservability(item RunObservability) RunObservability {
	item.Run = CloneRun(item.Run)
	item.Outcome = cloneRunOutcome(item.Outcome)
	item.TaskProgress = CloneTaskProgress(item.TaskProgress)
	item.RepairLoop = CloneRepairLoop(item.RepairLoop)
	item.Acceptance = CloneAcceptanceProjection(item.Acceptance)
	item.TaskDecompositions = CloneTaskDecompositions(item.TaskDecompositions)
	item.TaskPlans = CloneTaskPlans(item.TaskPlans)
	item.ExecutionSections = cloneExecutionSections(item.ExecutionSections)
	item.LLMContextBudgets = CloneLLMContextBudgetTraces(item.LLMContextBudgets)
	item.LLMUsageTraces = CloneLLMUsageTraces(item.LLMUsageTraces)
	item.LLMUsageSummary = CloneLLMUsageSummary(item.LLMUsageSummary)
	item.MemoryRecallTraces = CloneMemoryRecallTraces(item.MemoryRecallTraces)
	item.GitOperations = cloneGitOperationTraces(item.GitOperations)
	item.WebSources = CloneWebSourceTraces(item.WebSources)
	item.ClosureTraces = cloneClosureTraces(item.ClosureTraces)
	item.DoDEvidence = cloneDoDEvidenceTraces(item.DoDEvidence)
	item.ResultQuality = cloneResultQualityTraces(item.ResultQuality)
	item.LatestCheckpoint = cloneRuntimeCheckpointPointer(item.LatestCheckpoint)
	item.SelectedCapabilities = cloneRuntimeCapabilities(item.SelectedCapabilities)
	item.UsedCapabilities = cloneRuntimeCapabilities(item.UsedCapabilities)
	item.Timeline = cloneRuntimeTimelineItems(item.Timeline)
	return item
}

func cloneRunProjectionPointer(item *Run) *Run {
	if item == nil {
		return nil
	}
	ret := CloneRun(*item)
	return &ret
}

func cloneResultProjectionPointer(item *Result) *Result {
	if item == nil {
		return nil
	}
	ret := CloneResult(*item)
	return &ret
}

func cloneRunOutcome(item RunOutcome) RunOutcome {
	item.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	item.ArtifactRefs = append([]string(nil), item.ArtifactRefs...)
	return item
}

func cloneStepViews(items []StepView) []StepView {
	if items == nil {
		return nil
	}
	ret := make([]StepView, 0, len(items))
	for i := range items {
		ret = append(ret, *CloneStepView(&items[i]))
	}
	return ret
}

func cloneToolCalls(items []ToolCall) []ToolCall {
	ret := append([]ToolCall(nil), items...)
	for i := range ret {
		ret[i].Arguments = cloneJSONMap(items[i].Arguments)
		ret[i].Output = cloneJSONMap(items[i].Output)
	}
	return ret
}

func cloneValidationResults(items []ValidationResult) []ValidationResult {
	ret := append([]ValidationResult(nil), items...)
	for i := range ret {
		ret[i].EvidenceTags = append([]string(nil), items[i].EvidenceTags...)
	}
	return ret
}

func cloneRuntimeCheckpoints(items []RuntimeCheckpoint) []RuntimeCheckpoint {
	ret := append([]RuntimeCheckpoint(nil), items...)
	for i := range ret {
		ret[i].Data = cloneJSONMap(items[i].Data)
	}
	return ret
}

func cloneRuntimeCheckpointPointer(item *RuntimeCheckpoint) *RuntimeCheckpoint {
	if item == nil {
		return nil
	}
	ret := *item
	ret.Data = cloneJSONMap(item.Data)
	return &ret
}

func cloneGitOperationTraces(items []GitOperationTrace) []GitOperationTrace {
	ret := append([]GitOperationTrace(nil), items...)
	for i := range ret {
		ret[i].ChangedFiles = append([]GitChangedFile(nil), items[i].ChangedFiles...)
		ret[i].StagedFiles = append([]GitChangedFile(nil), items[i].StagedFiles...)
		ret[i].EvidenceRefs = append([]string(nil), items[i].EvidenceRefs...)
	}
	return ret
}

func cloneClosureTraces(items []ClosureTrace) []ClosureTrace {
	ret := append([]ClosureTrace(nil), items...)
	for i := range ret {
		ret[i].MissingDoDItems = cloneJSONMaps(items[i].MissingDoDItems)
		ret[i].EvidenceRefs = append([]string(nil), items[i].EvidenceRefs...)
	}
	return ret
}

func cloneDoDEvidenceTraces(items []DoDEvidenceTrace) []DoDEvidenceTrace {
	ret := append([]DoDEvidenceTrace(nil), items...)
	for i := range ret {
		ret[i].Items = append([]DoDEvidenceItem(nil), items[i].Items...)
		for j := range ret[i].Items {
			ret[i].Items[j].EvidenceRefs = append([]string(nil), items[i].Items[j].EvidenceRefs...)
			ret[i].Items[j].MissingEvidenceRefs = append([]string(nil), items[i].Items[j].MissingEvidenceRefs...)
		}
	}
	return ret
}

func cloneResultQualityTraces(items []ResultQualityTrace) []ResultQualityTrace {
	ret := append([]ResultQualityTrace(nil), items...)
	for i := range ret {
		ret[i].EvidenceRefs = append([]string(nil), items[i].EvidenceRefs...)
		ret[i].Issues = append([]ResultQualityIssue(nil), items[i].Issues...)
		for j := range ret[i].Issues {
			ret[i].Issues[j].EvidenceRefs = append([]string(nil), items[i].Issues[j].EvidenceRefs...)
		}
	}
	return ret
}

func cloneExecutionSections(items []ExecutionSection) []ExecutionSection {
	ret := append([]ExecutionSection(nil), items...)
	for i := range ret {
		ret[i].RelatedDoDItems = append([]string(nil), items[i].RelatedDoDItems...)
		ret[i].PlannedStepRefs = append([]string(nil), items[i].PlannedStepRefs...)
		ret[i].ActualEvidenceRefs = append([]string(nil), items[i].ActualEvidenceRefs...)
		ret[i].DecisionRefs = append([]string(nil), items[i].DecisionRefs...)
		ret[i].ApprovalRefs = append([]string(nil), items[i].ApprovalRefs...)
		ret[i].ToolCallRefs = append([]string(nil), items[i].ToolCallRefs...)
		ret[i].PatchRefs = append([]string(nil), items[i].PatchRefs...)
		ret[i].ValidationRefs = append([]string(nil), items[i].ValidationRefs...)
		ret[i].ResultRefs = append([]string(nil), items[i].ResultRefs...)
		ret[i].ArtifactRefs = append([]string(nil), items[i].ArtifactRefs...)
		ret[i].MemoryRecallRefs = append([]string(nil), items[i].MemoryRecallRefs...)
		ret[i].MemoryIDs = append([]string(nil), items[i].MemoryIDs...)
	}
	return ret
}

func cloneRuntimeCapabilities(items []RuntimeCapability) []RuntimeCapability {
	ret := append([]RuntimeCapability(nil), items...)
	for i := range ret {
		ret[i].Tags = append([]string(nil), items[i].Tags...)
		ret[i].EvidenceRefs = append([]string(nil), items[i].EvidenceRefs...)
	}
	return ret
}

func cloneRuntimeTimelineItems(items []RuntimeTimelineItem) []RuntimeTimelineItem {
	ret := append([]RuntimeTimelineItem(nil), items...)
	for i := range ret {
		ret[i].Data = cloneJSONMap(items[i].Data)
		ret[i].Output = cloneJSONMap(items[i].Output)
	}
	return ret
}

func cloneObservedViews(items []ObservedView) []ObservedView {
	ret := append([]ObservedView(nil), items...)
	for i := range ret {
		ret[i].Payload = cloneJSONMap(items[i].Payload)
	}
	return ret
}
