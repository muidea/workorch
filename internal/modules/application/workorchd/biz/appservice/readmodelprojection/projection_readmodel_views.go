package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: readmodelcore/projectioncontract view helpers.

// source: compressed_context.go

func compressedContextFromBundle(bundle projectioncontract.ContextBundle, retentionPolicy string) projectioncontract.CompressedContext {
	return projectioncontract.CompressedContext{
		Version:            "workorch.compressedContext",
		Mode:               "deterministic_recovery_summary",
		WorkspaceID:        bundle.WorkspaceID,
		RunID:              bundle.RunID,
		Summary:            bundle.Summary,
		CurrentMilestone:   bundle.TaskProgress.CurrentMilestone,
		Milestones:         bundle.TaskProgress.Milestones,
		Blockers:           bundle.TaskProgress.Blockers,
		Recovery:           bundle.Recovery,
		RecentEvidence:     lastContextEvidence(bundle.RecentEvidence, 6),
		ResultRefs:         resultRefsFromTaskProgress(bundle.TaskProgress),
		SourceSectionRefs:  compressedContextSectionRefsFromBundle(bundle.Sections),
		SourceEvidenceRefs: compressedContextSourceEvidenceRefs(bundle.TaskProgress, bundle.RecentEvidence),
		RetentionPolicy:    retentionPolicy,
	}
}

func compressedContextFromRunSnapshot(snapshot readmodelcore.RunSnapshotState, progress projectioncontract.TaskProgress, recentEvidence []projectioncontract.ContextEvidence, summary string, retentionPolicy string) projectioncontract.CompressedContext {
	return projectioncontract.CompressedContext{
		Version:            "workorch.compressedContext",
		Mode:               "deterministic_recovery_summary",
		WorkspaceID:        snapshot.Run.WorkspaceID,
		RunID:              snapshot.Run.ID,
		Summary:            summary,
		CurrentMilestone:   progress.CurrentMilestone,
		Milestones:         progress.Milestones,
		Blockers:           progress.Blockers,
		Recovery:           progress.Recovery,
		RecentEvidence:     lastContextEvidence(recentEvidence, 6),
		ResultRefs:         resultRefsFromTaskProgress(progress),
		SourceSectionRefs:  compressedContextSourceSectionRefs(snapshot, recentEvidence),
		SourceEvidenceRefs: compressedContextSourceEvidenceRefs(progress, recentEvidence),
		RetentionPolicy:    retentionPolicy,
	}
}

// source: compressed_context_dynamic_source_sections.go

func appendCompressedContextDynamicSourceSections(sections []projectioncontract.ContextSection, snapshot readmodelcore.RunSnapshotState, recentEvidence []projectioncontract.ContextEvidence) []projectioncontract.ContextSection {
	sections = append(sections, projectioncontract.ContextSection{ID: "task_progress", Priority: 90})
	if len(snapshot.TaskDecompositions) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "task_decomposition", Priority: 85})
	}
	if len(snapshot.ExecutionSections) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "execution_sections", Priority: 84})
	}
	if len(snapshot.MemoryRecallTraces) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "memory_recall", Priority: 83})
	}
	if len(snapshot.Decisions) > 0 || len(snapshot.Approvals) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "governance", Priority: 80})
	}
	if len(runScopedResults(snapshot)) > 0 || len(runScopedArtifacts(snapshot)) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "formal_outputs", Priority: 90})
	}
	if len(recentEvidence) > 0 {
		sections = append(sections, projectioncontract.ContextSection{ID: "recent_evidence", Priority: 70})
	}
	return sections
}

// source: compressed_context_sections.go

func compressedContextSourceSectionRefs(snapshot readmodelcore.RunSnapshotState, recentEvidence []projectioncontract.ContextEvidence) []string {
	sections := compressedContextSourceSections(snapshot, recentEvidence)
	refs := make([]string, 0, len(sections))
	for _, section := range sections {
		refs = append(refs, section.ID)
	}
	return compactUniqueStrings(refs)
}

// source: compressed_context_source_sections.go

func compressedContextSourceSections(snapshot readmodelcore.RunSnapshotState, recentEvidence []projectioncontract.ContextEvidence) []projectioncontract.ContextSection {
	sections := compressedContextBaseSourceSections()
	sections = appendCompressedContextDynamicSourceSections(sections, snapshot, recentEvidence)
	sections = append(sections, projectioncontract.ContextSection{ID: "recovery", Priority: 100})
	return compactContextSections(sections)
}

func compressedContextBaseSourceSections() []projectioncontract.ContextSection {
	return []projectioncontract.ContextSection{
		{ID: "workspace", Priority: 100},
		{ID: "run", Priority: 100},
	}
}

// source: context_bundle_definition_sections.go

func contextGoalSection(snapshot readmodelcore.RunSnapshotState) projectioncontract.ContextSection {
	return projectioncontract.ContextSection{
		ID:       "goal",
		Kind:     "goal",
		Title:    "Goal",
		Priority: 95,
	}
}

func contextDoDSection(snapshot readmodelcore.RunSnapshotState) projectioncontract.ContextSection {
	return projectioncontract.ContextSection{
		ID:       "dod",
		Kind:     "dod",
		Title:    "Definition of Done",
		Priority: 95,
	}
}

// source: context_bundle_formal_output_section.go

func contextFormalOutputSection(snapshot readmodelcore.RunSnapshotState) projectioncontract.ContextSection {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	if len(results) == 0 && len(artifacts) == 0 {
		return projectioncontract.ContextSection{}
	}
	return projectioncontract.ContextSection{
		ID:           "formal_outputs",
		Kind:         "formal_outputs",
		Title:        "Formal outputs",
		Priority:     90,
		State:        resultProgressState(snapshot),
		Summary:      formalOutputSummary(results),
		EvidenceRefs: append(idsFromResults(results), idsFromArtifacts(artifacts)...),
		Data: map[string]any{
			"results":   results,
			"artifacts": artifacts,
		},
	}
}

// source: context_bundle_governance_sections.go

func contextGovernanceSection(snapshot readmodelcore.RunSnapshotState, title string) projectioncontract.ContextSection {
	if len(snapshot.Decisions) == 0 && len(snapshot.Approvals) == 0 {
		return projectioncontract.ContextSection{}
	}
	return projectioncontract.ContextSection{
		ID:           "governance",
		Kind:         "governance",
		Title:        title,
		Priority:     80,
		State:        governanceProgressState(snapshot),
		Summary:      "decision tickets and approvals that affected run progress",
		EvidenceRefs: governanceEvidenceRefs(snapshot),
		Data: map[string]any{
			"decisions": snapshot.Decisions,
			"approvals": snapshot.Approvals,
		},
	}
}

// source: context_bundle_helpers.go

func contextBundleSummary(snapshot readmodelcore.RunSnapshotState, progress projectioncontract.TaskProgress) string {
	if progress.Summary != "" {
		return progress.Summary
	}
	return "run " + snapshot.Run.State + " context bundle for recovery and compression"
}

func currentStepID(snapshot readmodelcore.RunSnapshotState) string {
	if result := terminalTaskProgressResult(snapshot); result != nil {
		return firstNonEmptyTimelineString(result.SourceRef, result.ID)
	}
	if snapshot.Step == nil {
		return ""
	}
	return snapshot.Step.ID
}

func terminalTaskProgressResult(snapshot readmodelcore.RunSnapshotState) *projectioncontract.Result {
	if !isTerminalRunState(snapshot.Run.State) {
		return nil
	}
	return primaryTaskProgressResult(runScopedResults(snapshot))
}

// source: context_bundle_sections.go

func contextWorkspaceSection(snapshot readmodelcore.RunSnapshotState) projectioncontract.ContextSection {
	return projectioncontract.ContextSection{
		ID:       "workspace",
		Kind:     "workspace",
		Title:    "Workspace",
		Priority: 100,
		State:    snapshot.Workspace.State,
		Summary:  snapshot.Workspace.Title,
		Data: map[string]any{
			"id":                   snapshot.Workspace.ID,
			"title":                snapshot.Workspace.Title,
			"description":          snapshot.Workspace.Description,
			"deliverableDirectory": snapshot.Workspace.DeliverableDirectory,
		},
	}
}

// source: context_run_section.go

func contextRunSection(snapshot readmodelcore.RunSnapshotState) projectioncontract.ContextSection {
	return projectioncontract.ContextSection{
		ID:       "run",
		Kind:     "run",
		Title:    "Run",
		Priority: 100,
		State:    snapshot.Run.State,
		Summary:  snapshot.Run.FailureReason,
		EvidenceRefs: compactUniqueStrings([]string{
			snapshot.Run.ID,
			snapshot.Run.BlockingTicketID,
			snapshot.Run.BlockingApprovalID,
		}),
		Data: map[string]any{
			"id":                 snapshot.Run.ID,
			"workspaceId":        snapshot.Run.WorkspaceID,
			"goalId":             snapshot.Run.GoalID,
			"doDId":              snapshot.Run.DoDID,
			"state":              snapshot.Run.State,
			"currentStepId":      currentStepID(snapshot),
			"blockingTicketId":   snapshot.Run.BlockingTicketID,
			"blockingApprovalId": snapshot.Run.BlockingApprovalID,
			"failureReason":      snapshot.Run.FailureReason,
		},
	}
}

// source: coverage_quality.go

func mapDoDEvidence(snapshot readmodelcore.RunSnapshotState) []projectioncontract.DoDEvidenceTrace {
	evidenceIndex := runEvidenceRefIndex(snapshot)
	results := runScopedResults(snapshot)
	if len(results) == 0 {
		return nil
	}
	ret := make([]projectioncontract.DoDEvidenceTrace, 0, len(results))
	for _, item := range results {
		if item.DoDAssessment == nil {
			continue
		}
		if snapshot.Run.DoDID != "" && item.DoDID != snapshot.Run.DoDID {
			continue
		}
		ret = append(ret, dodEvidenceTraceFromResult(item, evidenceIndex))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: execution_section_item.go

func executionSectionFromNode(snapshot readmodelcore.RunSnapshotState, decomposition projectioncontract.TaskDecomposition, node projectioncontract.TaskDecompositionNode, dodItems []string, refs executionEvidenceRefs) projectioncontract.ExecutionSection {
	section := projectioncontract.ExecutionSection{
		ID:              decomposition.ID + "/section/" + node.ID,
		WorkspaceID:     snapshot.Run.WorkspaceID,
		RunID:           snapshot.Run.ID,
		Title:           firstNonEmptyTimelineString(node.Title, node.ID, node.Kind),
		State:           node.State,
		GraphRootStepID: decomposition.RootStepID,
		GraphNodeID:     node.ID,
		StepID:          node.StepID,
		StepKind:        node.Kind,
		RelatedDoDItems: relatedDoDItemsForSection(node, dodItems),
		PlannedStepRefs: compactUniqueStrings([]string{decomposition.RootStepID, node.ID}),
		Summary:         executionSectionSummary(node),
	}
	section.ActualEvidenceRefs = compactUniqueStrings(append(cloneReadModelStringSlice(node.EvidenceRefs), refs.evidenceByStep[node.StepID]...))
	section.DecisionRefs = compactUniqueStrings(refs.decisionsByStep[node.StepID])
	section.ApprovalRefs = compactUniqueStrings(refs.approvalsByStep[node.StepID])
	section.ToolCallRefs = compactUniqueStrings(refs.toolCallsByStep[node.StepID])
	section.PatchRefs = compactUniqueStrings(refs.patchesByStep[node.StepID])
	section.ValidationRefs = compactUniqueStrings(refs.validationsByStep[node.StepID])
	section.ResultRefs = compactUniqueStrings(refs.resultsByStep[node.StepID])
	section.ArtifactRefs = compactUniqueStrings(refs.artifactsByStep[node.StepID])
	section.MemoryRecallRefs = compactUniqueStrings(refs.memoryRecallsByStep[node.StepID])
	section.MemoryIDs = compactUniqueStrings(refs.memoryIDsByStep[node.StepID])
	return section
}

// source: execution_sections.go

func mapExecutionSections(snapshot readmodelcore.RunSnapshotState) []projectioncontract.ExecutionSection {
	if len(snapshot.TaskDecompositions) == 0 {
		return nil
	}
	dodItems := runDoDItems(snapshot)
	refs := executionEvidenceRefsByStep(snapshot)
	ret := make([]projectioncontract.ExecutionSection, 0)
	for _, decomposition := range snapshot.TaskDecompositions {
		for _, node := range decomposition.Nodes {
			ret = append(ret, executionSectionFromNode(snapshot, decomposition, node, dodItems, refs))
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: progress_blockers.go

func taskProgressBlockers(snapshot readmodelcore.RunSnapshotState) []projectioncontract.ProgressBlocker {
	var ret []projectioncontract.ProgressBlocker
	for _, item := range snapshot.Decisions {
		if isResolvedDecisionState(item.State) {
			continue
		}
		ret = append(ret, projectioncontract.ProgressBlocker{
			ID:          item.ID,
			Kind:        "decision",
			State:       item.State,
			Reason:      firstNonEmptyTimelineString(item.Question, item.Reason),
			EvidenceRef: item.StepID,
		})
	}
	for _, item := range snapshot.Approvals {
		if isResolvedApprovalState(item.State) {
			continue
		}
		ret = append(ret, projectioncontract.ProgressBlocker{
			ID:          item.ID,
			Kind:        "approval",
			State:       item.State,
			Reason:      item.Reason,
			EvidenceRef: item.StepID,
		})
	}
	return ret
}

// source: progress_items.go

func taskProgressItems(snapshot readmodelcore.RunSnapshotState) []projectioncontract.ProgressItem {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	ret := make([]projectioncontract.ProgressItem, 0, len(snapshot.Steps)+len(snapshot.ToolCalls)+len(snapshot.ValidationResults)+len(results)+len(artifacts)+len(snapshot.Decisions)+len(snapshot.Approvals))
	recoveredHistoricalFailures := acceptedTerminalRun(snapshot)
	for _, item := range snapshot.Steps {
		ret = append(ret, stepProgressItem(item, recoveredHistoricalFailures))
	}
	for _, item := range snapshot.ToolCalls {
		ret = append(ret, toolCallProgressItem(item, recoveredHistoricalFailures))
	}
	for _, item := range snapshot.Patches {
		ret = append(ret, patchProgressItem(item))
	}
	for _, item := range snapshot.ValidationResults {
		ret = append(ret, validationProgressItem(item, recoveredHistoricalFailures))
	}
	for _, item := range results {
		ret = append(ret, resultProgressItem(item))
	}
	for _, item := range artifacts {
		ret = append(ret, artifactProgressItem(item))
	}
	for _, item := range snapshot.Decisions {
		ret = append(ret, decisionProgressItem(item))
	}
	for _, item := range snapshot.Approvals {
		ret = append(ret, approvalProgressItem(item))
	}
	for idx := range ret {
		ret[idx].EvidenceRefs = compactUniqueStrings(ret[idx].EvidenceRefs)
	}
	return ret
}

// source: progress_milestones.go

func taskProgressMilestones(snapshot readmodelcore.RunSnapshotState, labels runTaskProgressLabels, results []projectioncontract.Result, artifacts []projectioncontract.Artifact) []projectioncontract.ProgressMilestone {
	return []projectioncontract.ProgressMilestone{
		{ID: "run_created", Title: labels.RunCreated, State: "completed", EvidenceRefs: []string{snapshot.Run.ID}},
		{ID: "execution_steps", Title: labels.ExecutionSteps, State: stepProgressState(snapshot), EvidenceRefs: idsFromSteps(snapshot.Steps)},
		{ID: "governance", Title: labels.Governance, State: governanceProgressState(snapshot), EvidenceRefs: governanceEvidenceRefs(snapshot)},
		{ID: "validation", Title: labels.Validation, State: validationProgressState(snapshot), EvidenceRefs: idsFromValidations(snapshot.ValidationResults)},
		{ID: "formal_result", Title: labels.FormalResult, State: resultProgressState(snapshot), EvidenceRefs: append(idsFromResults(results), idsFromArtifacts(artifacts)...)},
	}
}

// source: progress_projection.go

func mapTaskProgress(snapshot readmodelcore.RunSnapshotState, labels runTaskProgressLabels) projectioncontract.TaskProgress {
	latestCheckpoint := latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints)
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	ret := projectioncontract.TaskProgress{
		WorkspaceID: snapshot.Run.WorkspaceID,
		RunID:       snapshot.Run.ID,
		State:       snapshot.Run.State,
		Recovery: projectioncontract.RecoveryPlan{
			Mode:  "resume",
			RunID: snapshot.Run.ID,
		},
	}
	applyTaskProgressCurrentStep(&ret, snapshot)
	if latestCheckpoint != nil {
		ret.Recovery.CheckpointID = latestCheckpoint.ID
	}

	ret.Blockers = taskProgressBlockers(snapshot)
	ret.Milestones = taskProgressMilestones(snapshot, labels, results, artifacts)
	ret.Items = taskProgressItems(snapshot)
	ret.CurrentMilestone = currentProgressMilestone(ret.Milestones)
	ret.EvidenceRefs = taskProgressEvidenceRefs(snapshot)
	ret.Recovery = taskProgressRecovery(snapshot, latestCheckpoint, ret.Blockers, ret.Recovery)
	ret.BlockingTicketID = ret.Recovery.BlockingTicketID
	ret.BlockingApprovalID = ret.Recovery.BlockingApprovalID
	ret.RepairLoop = mapTaskProgressRepairLoop(snapshot, ret.Recovery)
	ret.AssessmentOutcome = taskProgressAssessmentOutcome(snapshot)
	ret.Summary = taskProgressSummary(ret, &snapshot)
	return ret
}

// source: progress_recovery.go

func taskProgressRecovery(snapshot readmodelcore.RunSnapshotState, latestCheckpoint *projectioncontract.RuntimeCheckpoint, blockers []projectioncontract.ProgressBlocker, fallback projectioncontract.RecoveryPlan) projectioncontract.RecoveryPlan {
	ret := fallback
	ret.RunID = snapshot.Run.ID
	ret.BlockingTicketID = ""
	ret.BlockingApprovalID = ""
	ret.BlockingResource = nil
	ret.SuggestedNavigation = ""
	if item := selectRunBlockingDecision(snapshot.Run, snapshot.Decisions); item != nil {
		applyDecisionRecoveryBlocker(&ret, item)
	} else if item := selectRunBlockingApproval(snapshot.Run, snapshot.Approvals); item != nil {
		applyApprovalRecoveryBlocker(&ret, item)
	}
	applyRecoveryCheckpoint(&ret, latestCheckpoint)
	if applyBlockingRecovery(&ret, blockers) {
		return ret
	}
	applyRunStateRecovery(&ret, snapshot)
	return ret
}

// source: progress_run_scoped_artifacts.go

func runScopedArtifacts(snapshot readmodelcore.RunSnapshotState) []projectioncontract.Artifact {
	if snapshot.Run.ID == "" {
		return snapshot.Artifacts
	}
	ret := make([]projectioncontract.Artifact, 0, len(snapshot.Artifacts))
	for _, item := range snapshot.Artifacts {
		if item.RunID == snapshot.Run.ID {
			ret = append(ret, item)
		}
	}
	return ret
}

// source: progress_workspace.go

func mapWorkspaceTaskProgress(snapshot readmodelcore.WorkspaceSnapshotState, labels workspaceTaskProgressLabels) projectioncontract.TaskProgress {
	if selectedRun := selectWorkspaceOutcomeRun(snapshot); selectedRun != nil && isTerminalRunState(selectedRun.State) {
		runSnapshot := readmodelcore.RunSnapshotState{
			Workspace:             snapshot.Workspace,
			Run:                   *selectedRun,
			Results:               snapshot.Results,
			Artifacts:             snapshot.Artifacts,
			ArtifactContinuations: snapshot.ArtifactContinuations,
			Patches:               snapshot.Patches,
			WebSources:            snapshot.WebSources,
			Decisions:             snapshot.Decisions,
			Approvals:             snapshot.Approvals,
			TaskDecompositions:    snapshot.TaskDecompositions,
			TaskPlans:             snapshot.TaskPlans,
			ExecutionSections:     snapshot.ExecutionSections,
			Acceptance:            snapshot.Acceptance,
		}
		return mapLocalizedTaskProgress(runSnapshot)
	}
	ret := newWorkspaceTaskProgressBase(snapshot, labels)
	applyWorkspaceProgressCurrentStep(&ret, snapshot)
	applyWorkspaceProgressBlockers(&ret, snapshot)
	ret.Milestones = workspaceProgressMilestones(snapshot, labels)
	ret.Items = workspaceProgressItems(snapshot, labels)
	ret.CurrentMilestone = currentProgressMilestone(ret.Milestones)
	ret.EvidenceRefs = compactUniqueStrings(append(idsFromResults(snapshot.Results), idsFromArtifacts(snapshot.Artifacts)...))
	ret.AssessmentOutcome = workspaceTaskProgressAssessmentOutcome(snapshot)
	ret.Summary = taskProgressSummary(ret, nil)
	return ret
}

// source: progress_workspace_base.go

func newWorkspaceTaskProgressBase(snapshot readmodelcore.WorkspaceSnapshotState, labels workspaceTaskProgressLabels) projectioncontract.TaskProgress {
	ret := projectioncontract.TaskProgress{
		WorkspaceID: snapshot.Workspace.ID,
		State:       snapshot.Workspace.State,
		Recovery: projectioncontract.RecoveryPlan{
			Mode:   "idle",
			Reason: labels.NoActiveRun,
		},
		RepairLoop: projectioncontract.RepairLoop{
			State: "inactive",
		},
	}
	if snapshot.Workspace.ActiveRunID != "" {
		ret.RunID = snapshot.Workspace.ActiveRunID
		ret.Recovery.RunID = snapshot.Workspace.ActiveRunID
		ret.Recovery.Mode = "resume"
		ret.Recovery.Reason = labels.ActiveRun
	}
	return ret
}

// source: repair_grounding_signals.go

func applyRepairGroundingStructuredSignals(ret *projectioncontract.RepairLoop, snapshot readmodelcore.RunSnapshotState, repairStep *projectioncontract.StepView, actionPackage map[string]any) {
	if ret == nil || repairStep == nil {
		return
	}
	if failedStep := stepByID(snapshot.Steps, ret.LastFailureStepID); failedStep != nil {
		ret.StructuredFailureSignals = readModelStructuredFailureSignalsFromStep(failedStep, ret.LastFailureClass, ret.LastFailureSignature)
	}
	if len(ret.StructuredFailureSignals) > 0 {
		// Current failed step wins. Repair-step inputs may still contain inherited stale signals.
		return
	}
	if signals := readModelMapValue(repairStep.Input["structuredFailureSignals"]); len(signals) > 0 {
		ret.StructuredFailureSignals = signals
		return
	}
	if signals := readModelMapValue(actionPackage["structuredFailureSignals"]); len(signals) > 0 {
		ret.StructuredFailureSignals = signals
	}
}

// source: repair_projection.go

func mapTaskProgressRepairLoop(snapshot readmodelcore.RunSnapshotState, recovery projectioncontract.RecoveryPlan) projectioncontract.RepairLoop {
	ret := projectioncontract.RepairLoop{
		State: "inactive",
	}
	attemptCount, lastAttempt, lastRepairStepID := repairAttemptProjection(snapshot.Steps)
	ret.AttemptCount = attemptCount
	ret.LastAttempt = lastAttempt
	ret.LastRepairStepID = lastRepairStepID

	lastFailure := latestRepairFailureProjection(snapshot)
	if lastFailure != nil {
		ret.LastFailureStepID = lastFailure.stepID
		ret.LastFailureClass = lastFailure.class
		ret.LastFailureSignature = lastFailure.signature
		ret.LastFailureAt = lastFailure.occurredAt
		ret.EvidenceRefs = append(ret.EvidenceRefs, lastFailure.evidenceRef)
		if failedStep := stepByID(snapshot.Steps, lastFailure.stepID); failedStep != nil {
			ret.MissingDoDItems = missingDoDItemsFromStepOutput(failedStep.Output)
		}
	}
	projectRepairGroundingObservability(&ret, snapshot)

	if ticketID, reason, escalationContext, ok := repairEscalationProjection(snapshot, recovery); ok {
		applyRepairEscalationProjection(&ret, ticketID, reason, escalationContext)
	} else {
		applyRepairLoopRecoveryState(&ret, recovery, attemptCount, lastFailure != nil)
	}

	ret.EvidenceRefs = compactUniqueStrings(append(ret.EvidenceRefs, ret.LastRepairStepID, ret.LastFailureStepID))
	return ret
}

// source: result_quality.go

func mapResultQuality(snapshot readmodelcore.RunSnapshotState) []projectioncontract.ResultQualityTrace {
	results := runScopedResults(snapshot)
	if len(results) == 0 {
		return nil
	}
	evidenceIndex := runEvidenceRefIndex(snapshot)
	weakValidationRefs := weakValidationEvidenceRefIndex(snapshot)
	requiredDoDLeafItems := runRequiredDoDLeafItems(snapshot)
	ret := make([]projectioncontract.ResultQualityTrace, 0, len(results))
	for _, item := range results {
		quality := newResultQualityTrace(item)
		summary := resultQualitySummaryFieldsFor(item)
		quality.EvidenceRefs = summary.evidenceRefs
		checkResultQualitySummaryFields(&quality, summary)
		checkResultQualityDoDAssessment(&quality, item, requiredDoDLeafItems, evidenceIndex, weakValidationRefs, summary.evidenceRefs)
		ret = append(ret, quality)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: result_scope.go

func runScopedResults(snapshot readmodelcore.RunSnapshotState) []projectioncontract.Result {
	if snapshot.Run.ID == "" {
		return snapshot.Results
	}
	ret := make([]projectioncontract.Result, 0, len(snapshot.Results))
	for _, item := range snapshot.Results {
		if item.RunID == snapshot.Run.ID {
			ret = append(ret, item)
		}
	}
	return ret
}

// source: run_snapshot_detail_artifacts.go

func projectRunSnapshotArtifactViews(ret *readmodelcore.RunSnapshotState) {
	if len(ret.Artifacts) == 0 {
		return
	}
	limit := min(len(ret.Artifacts), artifactPageLimit)
	projected := make([]projectioncontract.Artifact, 0, limit)
	for _, item := range ret.Artifacts[:limit] {
		projected = append(projected, projectArtifactView(item))
	}
	ret.Artifacts = projected
}

// source: run_snapshot_detail_governance.go

func projectRunSnapshotGovernanceDetails(ret *readmodelcore.RunSnapshotState) {
	if ret == nil {
		return
	}

	if len(ret.Decisions) > 0 {
		limit := min(len(ret.Decisions), decisionPageLimit)
		projected := make([]projectioncontract.Decision, 0, limit)
		for _, item := range ret.Decisions[:limit] {
			projected = append(projected, projectDecisionView(item))
		}
		ret.Decisions = projected
	}
	if len(ret.Approvals) > 0 {
		limit := min(len(ret.Approvals), approvalPageLimit)
		projected := make([]projectioncontract.Approval, 0, limit)
		for _, item := range ret.Approvals[:limit] {
			projected = append(projected, projectApprovalView(item))
		}
		ret.Approvals = projected
	}
}

// source: run_snapshot_detail_patches.go

func projectRunSnapshotPatchViews(ret *readmodelcore.RunSnapshotState) {
	if len(ret.Patches) == 0 {
		return
	}
	limit := min(len(ret.Patches), patchPageLimit)
	projected := make([]projectioncontract.Patch, 0, limit)
	for _, item := range ret.Patches[:limit] {
		projected = append(projected, projectPatchView(item))
	}
	ret.Patches = projected
}

// source: run_snapshot_detail_result_views.go

func projectRunSnapshotResultViews(ret *readmodelcore.RunSnapshotState) {
	if len(ret.Results) == 0 {
		return
	}
	limit := min(len(ret.Results), resultPageLimit)
	projected := make([]projectioncontract.Result, 0, limit)
	for _, item := range ret.Results[:limit] {
		projected = append(projected, projectResultView(item))
	}
	ret.Results = projected
}

// source: run_snapshot_detail_runtime.go

func projectRunSnapshotRuntimeDetails(ret *readmodelcore.RunSnapshotState) {
	if ret == nil {
		return
	}

	if len(ret.ToolCalls) > 0 {
		limit := min(len(ret.ToolCalls), toolCallPageLimit)
		projected := make([]projectioncontract.ToolCall, 0, limit)
		for _, item := range ret.ToolCalls[:limit] {
			projected = append(projected, projectToolCallView(item))
		}
		ret.ToolCalls = projected
	}
	if len(ret.RuntimeCheckpoints) > 0 {
		limit := min(len(ret.RuntimeCheckpoints), checkpointPageLimit)
		projected := make([]projectioncontract.RuntimeCheckpoint, 0, limit)
		for _, item := range ret.RuntimeCheckpoints[:limit] {
			projected = append(projected, projectRuntimeCheckpointView(item))
		}
		ret.RuntimeCheckpoints = projected
	}
}

// source: run_snapshot_detail_steps.go

func projectRunSnapshotStepDetails(ret *readmodelcore.RunSnapshotState) {
	if ret == nil {
		return
	}

	if ret.Step != nil {
		projected := projectStepView(*ret.Step)
		ret.Step = &projected
	}
	if len(ret.Steps) > 0 {
		limit := min(len(ret.Steps), stepPageLimit)
		projected := make([]projectioncontract.StepView, 0, limit)
		for _, item := range ret.Steps[:limit] {
			projected = append(projected, projectStepView(item))
		}
		ret.Steps = projected
	}
}

// source: run_snapshot_detail_validations.go

func projectRunSnapshotValidationViews(ret *readmodelcore.RunSnapshotState) {
	if len(ret.ValidationResults) == 0 {
		return
	}
	limit := min(len(ret.ValidationResults), validationPageLimit)
	projected := make([]projectioncontract.ValidationResult, 0, limit)
	for _, item := range ret.ValidationResults[:limit] {
		projected = append(projected, projectValidationResultView(item))
	}
	ret.ValidationResults = projected
}

// source: runtime_evidence_state_counts.go

func accumulateRuntimeEvidenceStates(ret *projectioncontract.RuntimeEvidenceSummary, snapshot readmodelcore.RunSnapshotState) {
	accumulateToolCallEvidenceStates(ret, snapshot.ToolCalls)
	accumulatePatchEvidenceStates(ret, snapshot.Patches)
	accumulateValidationEvidenceStates(ret, snapshot.ValidationResults)
}

// source: runtime_evidence_summary.go

func mapRuntimeEvidenceSummary(snapshot readmodelcore.RunSnapshotState, eventTotal int) projectioncontract.RuntimeEvidenceSummary {
	ret := projectioncontract.RuntimeEvidenceSummary{
		ToolCallTotal:          len(snapshot.ToolCalls),
		PatchTotal:             len(snapshot.Patches),
		ValidationTotal:        len(snapshot.ValidationResults),
		RuntimeCheckpointTotal: len(snapshot.RuntimeCheckpoints),
		ResultTotal:            len(snapshot.Results),
		ArtifactTotal:          len(snapshot.Artifacts),
		EventTotal:             eventTotal,
	}
	accumulateRuntimeEvidenceStates(&ret, snapshot)
	return ret
}

// source: snapshot_progress_helpers.go

func runSnapshotTaskProgress(snapshot readmodelcore.RunSnapshotState) projectioncontract.TaskProgress {
	if snapshot.TaskProgress.RunID == snapshot.Run.ID && snapshot.TaskProgress.WorkspaceID == snapshot.Run.WorkspaceID {
		return snapshot.TaskProgress
	}
	return mapLocalizedTaskProgress(snapshot)
}

func runSnapshotTaskDecompositions(snapshot readmodelcore.RunSnapshotState) []projectioncontract.TaskDecomposition {
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions
	}
	return mapTaskDecompositions(snapshot)
}

func mapTaskPlansFromSnapshot(snapshot readmodelcore.RunSnapshotState) []projectioncontract.TaskPlan {
	decompositions := runSnapshotTaskDecompositions(snapshot)
	return mapTaskPlans(snapshot, decompositions)
}

// source: task_decomposition_projection.go

func taskDecompositionFromStep(snapshot readmodelcore.RunSnapshotState, step projectioncontract.StepView, executedByNode map[string]projectioncontract.StepView) (projectioncontract.TaskDecomposition, bool) {
	graph := taskGraphFromValue(step.Output["executionGraph"])
	nodes := readModelMapSliceValue(graph["nodes"])
	if len(nodes) == 0 {
		return projectioncontract.TaskDecomposition{}, false
	}
	rootStepID := step.ID
	blockingTickets := taskBlockingTicketByStep(snapshot)
	rootMeta := taskLifecycleMetadataFromStep(step)
	if ticketID := blockingTickets[rootStepID]; ticketID != "" {
		rootMeta.blockingTicketID = ticketID
	}
	item := projectioncontract.TaskDecomposition{
		ID:          rootStepID + "/task-decomposition",
		WorkspaceID: snapshot.Run.WorkspaceID,
		RunID:       snapshot.Run.ID,
		RootStepID:  rootStepID,
		State:       "pending",
		Summary:     timelineStepSummary(step.Output),
		GraphBranch: readModelStringValue(step.Output["graphBranch"]),
		NodeTotal:   len(nodes),
		EvidenceRefs: compactUniqueStrings([]string{
			rootStepID,
		}),
	}
	for _, node := range nodes {
		addTaskDecompositionNode(&item, node, executedByNode, rootMeta, blockingTickets)
	}
	item.State = taskDecompositionState(item)
	item.EvidenceRefs = compactUniqueStrings(item.EvidenceRefs)
	return item, true
}

// source: task_plan_item_projection.go

func projectTaskPlan(snapshot readmodelcore.RunSnapshotState, decomposition projectioncontract.TaskDecomposition, step projectioncontract.StepView, hasStep bool, occurrences map[string]string) projectioncontract.TaskPlan {
	fields := taskPlanProjectionFields(decomposition, step, hasStep, occurrences)
	plan := projectioncontract.TaskPlan{
		WorkspaceID:      snapshot.Run.WorkspaceID,
		RunID:            snapshot.Run.ID,
		AnchorStepID:     fields.anchorStepID,
		RevisionStepID:   decomposition.RootStepID,
		State:            fields.state,
		Stage:            fields.stage,
		Summary:          decomposition.Summary,
		GraphBranch:      decomposition.GraphBranch,
		NodeTotal:        decomposition.NodeTotal,
		NodeCompleted:    decomposition.NodeCompleted,
		NodeFailed:       decomposition.NodeFailed,
		NodeRunning:      decomposition.NodeRunning,
		Nodes:            cloneTaskDecompositionNodes(decomposition.Nodes),
		EvidenceRefs:     compactUniqueStrings(append([]string{decomposition.RootStepID}, decomposition.EvidenceRefs...)),
		OccurredAt:       fields.occurredAt,
		SourceProjection: "task_decomposition",
	}
	if hasStep {
		meta := taskLifecycleMetadataFromStep(step)
		if ticketID := taskBlockingTicketByStep(snapshot)[step.ID]; ticketID != "" {
			meta.blockingTicketID = ticketID
		}
		applyTaskLifecycleToPlan(&plan, meta)
	}
	return plan
}

// source: task_projection.go

func mapTaskDecompositions(snapshot readmodelcore.RunSnapshotState) []projectioncontract.TaskDecomposition {
	if len(snapshot.Steps) == 0 {
		return nil
	}
	stepByNodeRoot := taskExecutionStepsByRoot(snapshot.Steps)
	ret := make([]projectioncontract.TaskDecomposition, 0)
	for _, step := range snapshot.Steps {
		item, ok := taskDecompositionFromStep(snapshot, step, stepByNodeRoot[step.ID])
		if !ok {
			continue
		}
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: timeline.go

func runtimeTimeline(snapshot readmodelcore.RunSnapshotState) []projectioncontract.RuntimeTimelineItem {
	items := make([]projectioncontract.RuntimeTimelineItem, 0, len(snapshot.Steps)+len(snapshot.RuntimeCheckpoints)+len(snapshot.Observed))
	stepOccurrences := timelineStepOccurrences(snapshot)
	for _, trace := range snapshot.MemoryRecallTraces {
		items = append(items, memoryRecallTimelineItem(trace, snapshot.Run.State))
	}
	for _, step := range snapshot.Steps {
		occurredAt := stepOccurrences[step.ID]
		stepDisplayState := firstNonEmptyTimelineString(step.State, snapshot.Run.State)
		items = append(items, stepRuntimeTimelineItem(step, stepDisplayState, occurredAt))
		items = append(items, streamTimelineItems(step, stepDisplayState, occurredAt)...)
	}
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		items = append(items, checkpointRuntimeTimelineItem(checkpoint))
	}
	items = append(items, persistedEventTimelineItems(snapshot)...)
	if len(snapshot.EventRecords) == 0 {
		items = append(items, observedEventTimelineItems(snapshot)...)
	}
	sortRuntimeTimelineItems(items)
	return items
}

// source: timeline_events.go

func persistedEventTimelineItems(snapshot readmodelcore.RunSnapshotState) []projectioncontract.RuntimeTimelineItem {
	ret := make([]projectioncontract.RuntimeTimelineItem, 0, len(snapshot.EventRecords))
	for _, item := range snapshot.EventRecords {
		if item.RunID != "" && item.RunID != snapshot.Run.ID {
			continue
		}
		if streamItem, ok := streamProgressTimelineItemFromPersistedEvent(item, snapshot.Run.State); ok {
			ret = append(ret, streamItem)
			continue
		}
		ret = append(ret, persistedEventTimelineItem(item))
	}
	return ret
}

func observedEventTimelineItems(snapshot readmodelcore.RunSnapshotState) []projectioncontract.RuntimeTimelineItem {
	ret := make([]projectioncontract.RuntimeTimelineItem, 0, len(snapshot.Observed))
	for _, item := range snapshot.Observed {
		if item.RunID != "" && item.RunID != snapshot.Run.ID {
			continue
		}
		if streamItem, ok := streamProgressTimelineItemFromObserved(item, snapshot.Run.State); ok {
			ret = append(ret, streamItem)
			continue
		}
		ret = append(ret, observedEventTimelineItem(item))
	}
	return ret
}

// source: workspace_snapshot_progress_helpers.go

func workspaceSnapshotTaskProgress(snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.TaskProgress {
	if snapshot.TaskProgress.WorkspaceID != "" {
		return snapshot.TaskProgress
	}
	return mapLocalizedWorkspaceTaskProgress(snapshot)
}

func workspaceContextBundleFromSnapshots(snapshot readmodelcore.WorkspaceSnapshotState, runSnapshot *readmodelcore.RunSnapshotState) projectioncontract.ContextBundle {
	if snapshot.TaskProgress.RunID != "" && runSnapshot != nil {
		return mapRunContextBundle(*runSnapshot)
	}
	return mapWorkspaceContextBundle(snapshot)
}

func workspaceTaskDecompositionsFromSnapshots(snapshot readmodelcore.WorkspaceSnapshotState, runSnapshot *readmodelcore.RunSnapshotState) []projectioncontract.TaskDecomposition {
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions
	}
	if snapshot.TaskProgress.RunID == "" || runSnapshot == nil {
		return nil
	}
	return runSnapshot.TaskDecompositions
}

// source: workspace_snapshot_run_merge.go

func workspaceSnapshotWithRunSnapshot(snapshot readmodelcore.WorkspaceSnapshotState, runSnapshot *readmodelcore.RunSnapshotState) readmodelcore.WorkspaceSnapshotState {
	ret := snapshot
	if runSnapshot == nil {
		ret.DisplayOutcome = mapWorkspaceOutcome(ret)
		ret.RunHeartbeat = mapRunHeartbeat(ret)
		ret = normalizeWorkspaceGovernanceProjection(ret)
		return workspaceSnapshotWithContextBundle(ret)
	}

	run := runSnapshot.Run
	ret.Run = &run
	ret.Step = runSnapshot.Step
	if result := primaryTaskProgressResult(runScopedResults(*runSnapshot)); result != nil {
		selected := *result
		ret.Result = &selected
	}
	ret.Results = mergeWorkspaceSnapshotResults(ret.Results, runSnapshot.Results)
	ret.Artifacts = mergeWorkspaceSnapshotArtifacts(ret.Artifacts, runSnapshot.Artifacts)
	ret.TaskProgress = runSnapshot.TaskProgress
	ret.CoverageSummary = coverageSummaryFromRunSnapshot(*runSnapshot)
	ret.GapBaseline = runSnapshot.GapBaseline
	ret.AuthorityDiagnostics = append([]projectioncontract.GapDiagnostic(nil), runSnapshot.AuthorityDiagnostics...)
	ret.Acceptance = runSnapshot.Acceptance
	ret.TaskDecompositions = runSnapshot.TaskDecompositions
	ret.TaskPlans = runSnapshot.TaskPlans
	ret.ExecutionSections = runSnapshot.ExecutionSections
	ret.MemoryRecallTraces = runSnapshot.MemoryRecallTraces
	ret.Patches = mergeWorkspaceSnapshotPatches(ret.Patches, runSnapshot.Patches)
	ret.ArtifactContinuations = mergeWorkspaceSnapshotArtifactContinuations(ret.ArtifactContinuations, runSnapshot.ArtifactContinuations)
	ret.Decisions = mergeWorkspaceSnapshotDecisions(ret.Decisions, runSnapshot.Decisions)
	ret.Approvals = mergeWorkspaceSnapshotApprovals(ret.Approvals, runSnapshot.Approvals)
	ret.WebSources = runSnapshot.WebSources
	ret.LLMContextBudgets = runSnapshot.LLMContextBudgets
	ret.LLMContextBudgetSummary = runSnapshot.LLMContextBudgetSummary
	ret.LLMUsageTraces = mergeWorkspaceLLMUsageTraces(ret.LLMUsageTraces, runSnapshot.LLMUsageTraces)
	ret.LLMUsageSummary = summarizeLLMUsage(ret.LLMUsageTraces)
	ret.DisplayOutcome = mapWorkspaceOutcome(ret)
	if runSnapshot.TaskProgress.WorkspaceID != "" {
		ret.TaskProgress = runSnapshot.TaskProgress
	}
	ret.RunHeartbeat = mapRunHeartbeat(ret)
	ret.ConvergenceStatus = mapWorkspaceConvergenceStatus(ret)
	ret = normalizeWorkspaceGovernanceProjection(ret)
	bundle := mapRunContextBundle(*runSnapshot)
	ret.ContextBundle = &bundle
	return ret
}

func mergeWorkspaceSnapshotDecisions(existing []projectioncontract.Decision, latest []projectioncontract.Decision) []projectioncontract.Decision {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]projectioncontract.Decision, 0, len(existing)+len(latest))
	for _, item := range existing {
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		if idx, ok := seen[item.ID]; ok {
			out[idx] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func mergeWorkspaceSnapshotApprovals(existing []projectioncontract.Approval, latest []projectioncontract.Approval) []projectioncontract.Approval {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]projectioncontract.Approval, 0, len(existing)+len(latest))
	for _, item := range existing {
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		if idx, ok := seen[item.ID]; ok {
			out[idx] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func workspaceSnapshotWithContextBundle(snapshot readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState {
	bundle := mapWorkspaceContextBundle(snapshot)
	snapshot.ContextBundle = &bundle
	return snapshot
}

// source: workspace_snapshot_summary.go

func projectWorkspaceSnapshotSummary(snapshot readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState {
	ret := projectWorkspaceSnapshotDetail(snapshot)
	ret.Run = nil
	ret.Step = nil
	ret.Result = nil
	ret.ContextBundle = nil
	ret.GapBaseline = nil
	ret.AuthorityDiagnostics = nil
	ret.Acceptance = projectioncontract.AcceptanceProjection{}
	ret.TaskProgress.Milestones = nil
	ret.TaskProgress.Items = nil
	ret.TaskProgress.EvidenceRefs = nil
	ret.TaskDecompositions = nil
	ret.TaskPlans = nil
	ret.ExecutionSections = nil
	ret.MemoryRecallTraces = nil
	ret.Goals = nil
	ret.DoDs = nil
	ret.Runs = nil
	ret.Results = nil
	ret.Artifacts = nil
	ret.ArtifactContinuations = nil
	ret.Patches = nil
	ret.WebSources = nil
	ret.LLMContextBudgets = nil
	ret.LLMUsageTraces = nil
	ret.EventRecords = nil
	return ret
}
