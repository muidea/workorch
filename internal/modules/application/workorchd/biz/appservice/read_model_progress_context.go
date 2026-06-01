package appservice

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	"github.com/muidea/workorch/internal/pkg/common"
)

func mapWorkspaceTaskProgress(snapshot WorkspaceSnapshot) TaskProgress {
	language := workspaceSnapshotLanguage(snapshot)
	ret := TaskProgress{
		WorkspaceID: snapshot.Workspace.ID,
		State:       snapshot.Workspace.State,
		Recovery: RecoveryPlan{
			Mode:       "idle",
			NextAction: "start_workspace_execution",
			Reason:     localizedWorkspaceProgressText(language, "no_active_run"),
		},
		RepairLoop: RepairLoop{
			State: "inactive",
		},
	}
	if snapshot.Workspace.ActiveRunID != "" {
		ret.RunID = snapshot.Workspace.ActiveRunID
		ret.Recovery.RunID = snapshot.Workspace.ActiveRunID
		ret.Recovery.Mode = "continue"
		ret.Recovery.NextAction = "inspect_active_run"
		ret.Recovery.Reason = localizedWorkspaceProgressText(language, "active_run")
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = canonicalReadModelStepKind(snapshot.Step.Kind)
		ret.CurrentStepState = snapshot.Step.State
		if ret.Recovery.StepID == "" {
			ret.Recovery.StepID = snapshot.Step.ID
		}
	}
	for _, item := range snapshot.Decisions {
		if isResolvedDecisionState(item.State) {
			continue
		}
		ret.Blockers = append(ret.Blockers, ProgressBlocker{
			ID:          item.ID,
			Kind:        "decision",
			State:       item.State,
			Reason:      firstNonEmptyReadModel(item.Question, item.Reason),
			EvidenceRef: item.StepID,
		})
		if ret.Recovery.BlockingTicketID == "" {
			ret.Recovery.Mode = "wait_decision"
			ret.Recovery.NextAction = "resolve_decision_ticket"
			ret.Recovery.BlockingTicketID = item.ID
			ret.Recovery.StepID = item.StepID
			ret.Recovery.Reason = firstNonEmptyReadModel(item.Question, item.Reason)
		}
	}
	for _, item := range snapshot.Approvals {
		if isResolvedApprovalState(item.State) {
			continue
		}
		ret.Blockers = append(ret.Blockers, ProgressBlocker{
			ID:          item.ID,
			Kind:        "approval",
			State:       item.State,
			Reason:      item.Reason,
			EvidenceRef: item.StepID,
		})
		if ret.Recovery.BlockingApprovalID == "" && ret.Recovery.BlockingTicketID == "" {
			ret.Recovery.Mode = "wait_approval"
			ret.Recovery.NextAction = "resolve_approval"
			ret.Recovery.BlockingApprovalID = item.ID
			ret.Recovery.StepID = item.StepID
			ret.Recovery.Reason = item.Reason
		}
	}
	ret.Milestones = []ProgressMilestone{
		{ID: "workspace_defined", Title: localizedWorkspaceProgressText(language, "workspace_defined"), State: "completed", EvidenceRefs: []string{snapshot.Workspace.ID}},
		{ID: "formal_result", Title: localizedMilestoneTitle(language, "formal_result"), State: progressStateFromCount(len(snapshot.Results), false, snapshot.Workspace.State == string(common.WorkspaceStateCompleted)), EvidenceRefs: idsFromResults(snapshot.Results)},
	}
	ret.Items = []ProgressItem{
		{ID: "runs", Kind: "run", Title: localizedWorkspaceProgressText(language, "runs"), State: progressStateFromCount(len(snapshot.Runs), false, false)},
		{ID: "results", Kind: "result", Title: localizedWorkspaceProgressText(language, "results"), State: progressStateFromCount(len(snapshot.Results), false, false), EvidenceRefs: idsFromResults(snapshot.Results)},
		{ID: "artifacts", Kind: "artifact", Title: localizedWorkspaceProgressText(language, "artifacts"), State: progressStateFromCount(len(snapshot.Artifacts), false, false), EvidenceRefs: idsFromArtifacts(snapshot.Artifacts)},
	}
	ret.CurrentMilestone = currentProgressMilestone(ret.Milestones)
	ret.EvidenceRefs = compactUniqueStrings(append(idsFromResults(snapshot.Results), idsFromArtifacts(snapshot.Artifacts)...))
	ret.Summary = taskProgressSummary(ret, nil)
	return ret
}

func mapTaskProgress(snapshot RunSnapshot) TaskProgress {
	language := snapshotLanguage(snapshot)
	latestCheckpoint := latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints)
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	ret := TaskProgress{
		WorkspaceID: snapshot.Run.WorkspaceID,
		RunID:       snapshot.Run.ID,
		State:       snapshot.Run.State,
		Recovery: RecoveryPlan{
			Mode:       "continue",
			NextAction: localizedRecoveryNextAction(language),
			RunID:      snapshot.Run.ID,
		},
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = canonicalReadModelStepKind(snapshot.Step.Kind)
		ret.CurrentStepState = snapshot.Step.State
		ret.Recovery.StepID = snapshot.Step.ID
	}
	if result := terminalTaskProgressResult(snapshot); result != nil {
		ret.CurrentStepID = firstNonEmptyReadModel(result.SourceRef, result.ID)
		ret.CurrentStepKind = "result/" + strings.TrimSpace(firstNonEmptyReadModel(result.SourceType, "terminal"))
		ret.CurrentStepState = result.State
		ret.Recovery.StepID = ret.CurrentStepID
	}
	if latestCheckpoint != nil {
		ret.Recovery.CheckpointID = latestCheckpoint.ID
	}

	ret.Blockers = taskProgressBlockers(snapshot)
	ret.Milestones = []ProgressMilestone{
		{ID: "run_created", Title: localizedMilestoneTitle(language, "run_created"), State: "completed", EvidenceRefs: []string{snapshot.Run.ID}},
		{ID: "execution_steps", Title: localizedMilestoneTitle(language, "execution_steps"), State: stepProgressState(snapshot), EvidenceRefs: idsFromSteps(snapshot.Steps)},
		{ID: "governance", Title: localizedMilestoneTitle(language, "governance"), State: governanceProgressState(snapshot), EvidenceRefs: governanceEvidenceRefs(snapshot)},
		{ID: "validation", Title: localizedMilestoneTitle(language, "validation"), State: validationProgressState(snapshot), EvidenceRefs: idsFromValidations(snapshot.ValidationResults)},
		{ID: "formal_result", Title: localizedMilestoneTitle(language, "formal_result"), State: resultProgressState(snapshot), EvidenceRefs: append(idsFromResults(results), idsFromArtifacts(artifacts)...)},
	}
	ret.Items = taskProgressItems(snapshot)
	ret.CurrentMilestone = currentProgressMilestone(ret.Milestones)
	ret.EvidenceRefs = taskProgressEvidenceRefs(snapshot)
	ret.Recovery = taskProgressRecovery(snapshot, latestCheckpoint, ret.Blockers, ret.Recovery)
	ret.BlockingTicketID = ret.Recovery.BlockingTicketID
	ret.BlockingApprovalID = ret.Recovery.BlockingApprovalID
	ret.RepairLoop = mapTaskProgressRepairLoop(snapshot, ret.Recovery)
	ret.Summary = taskProgressSummary(ret, &snapshot)
	return ret
}

func mapWorkspaceContextBundle(snapshot WorkspaceSnapshot) ContextBundle {
	language := workspaceSnapshotLanguage(snapshot)
	progress := snapshot.TaskProgress
	ret := ContextBundle{
		WorkspaceID:  snapshot.Workspace.ID,
		Mode:         "workspace",
		Summary:      localizedWorkspaceProgressText(language, "context_summary"),
		TaskProgress: progress,
		Recovery:     progress.Recovery,
		Sections: []ContextSection{
			{
				ID:       "workspace",
				Kind:     "workspace",
				Title:    localizedWorkspaceProgressText(language, "workspace"),
				Priority: 100,
				State:    snapshot.Workspace.State,
				Summary:  snapshot.Workspace.Title,
				Data: map[string]any{
					"id":                   snapshot.Workspace.ID,
					"title":                snapshot.Workspace.Title,
					"description":          snapshot.Workspace.Description,
					"deliverableDirectory": snapshot.Workspace.DeliverableDirectory,
				},
			},
			{
				ID:           "task_progress",
				Kind:         "task_progress",
				Title:        localizedWorkspaceProgressText(language, "task_progress"),
				Priority:     90,
				State:        progress.State,
				Summary:      progress.Summary,
				EvidenceRefs: progress.EvidenceRefs,
			},
		},
	}
	ret.CompressionCandidates = contextCompressionCandidates(ret)
	ret.CompressedContext = compressedContextFromBundle(ret, language)
	return ret
}

func mapRunContextBundle(snapshot RunSnapshot) ContextBundle {
	progress := runSnapshotTaskProgress(snapshot)
	timeline := runtimeTimeline(snapshot)
	recentEvidence := contextRecentEvidence(timeline, 12)
	ret := ContextBundle{
		WorkspaceID:      snapshot.Run.WorkspaceID,
		RunID:            snapshot.Run.ID,
		Mode:             "run_recovery",
		Summary:          contextBundleSummary(snapshot, progress),
		TaskProgress:     progress,
		Recovery:         progress.Recovery,
		LatestCheckpoint: latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints),
		RecentEvidence:   recentEvidence,
	}
	ret.Sections = []ContextSection{
		contextWorkspaceSection(snapshot),
		contextRunSection(snapshot),
		contextGoalSection(snapshot),
		contextDoDSection(snapshot),
		contextExecutionContextSection(snapshot),
		contextTaskProgressSection(progress),
		contextTaskDecompositionSection(snapshot.TaskDecompositions),
		contextExecutionSectionsSection(snapshot.ExecutionSections),
		contextMemoryRecallSection(snapshot.MemoryRecallTraces),
		contextGovernanceSection(snapshot),
		contextFormalOutputSection(snapshot),
		contextRecentEvidenceSection(recentEvidence),
		contextRecoverySection(progress.Recovery),
	}
	ret.Sections = compactContextSections(ret.Sections)
	ret.CompressionCandidates = contextCompressionCandidates(ret)
	ret.CompressedContext = compressedContextFromBundle(ret, snapshotLanguage(snapshot))
	return ret
}

func compressedContextFromBundle(bundle ContextBundle, language string) CompressedContext {
	sectionRefs := make([]string, 0, len(bundle.Sections))
	for _, section := range bundle.Sections {
		sectionRefs = append(sectionRefs, section.ID)
	}
	sourceEvidenceRefs := compactUniqueStrings(append(bundle.TaskProgress.EvidenceRefs, contextEvidenceRefs(bundle.RecentEvidence)...))
	return CompressedContext{
		Version:            "workorch.compressedContext.v1",
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
		SourceSectionRefs:  compactUniqueStrings(sectionRefs),
		SourceEvidenceRefs: sourceEvidenceRefs,
		RetentionPolicy:    localizedRetentionPolicy(language),
	}
}

func compressedContextFromRunSnapshot(snapshot RunSnapshot) CompressedContext {
	language := snapshotLanguage(snapshot)
	progress := runSnapshotTaskProgress(snapshot)
	timeline := runtimeTimeline(snapshot)
	recentEvidence := contextRecentEvidence(timeline, 12)
	sourceEvidenceRefs := compactUniqueStrings(append(progress.EvidenceRefs, contextEvidenceRefs(recentEvidence)...))
	return CompressedContext{
		Version:            "workorch.compressedContext.v1",
		Mode:               "deterministic_recovery_summary",
		WorkspaceID:        snapshot.Run.WorkspaceID,
		RunID:              snapshot.Run.ID,
		Summary:            contextBundleSummary(snapshot, progress),
		CurrentMilestone:   progress.CurrentMilestone,
		Milestones:         progress.Milestones,
		Blockers:           progress.Blockers,
		Recovery:           progress.Recovery,
		RecentEvidence:     lastContextEvidence(recentEvidence, 6),
		ResultRefs:         resultRefsFromTaskProgress(progress),
		SourceSectionRefs:  compressedContextSourceSectionRefs(snapshot, recentEvidence),
		SourceEvidenceRefs: sourceEvidenceRefs,
		RetentionPolicy:    localizedRetentionPolicy(language),
	}
}

func runSnapshotTaskProgress(snapshot RunSnapshot) TaskProgress {
	if snapshot.TaskProgress.RunID == snapshot.Run.ID && snapshot.TaskProgress.WorkspaceID == snapshot.Run.WorkspaceID {
		return snapshot.TaskProgress
	}
	return mapTaskProgress(snapshot)
}

func runSnapshotTaskDecompositions(snapshot RunSnapshot) []TaskDecomposition {
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions
	}
	return mapTaskDecompositions(snapshot)
}

func compressedContextSourceSectionRefs(snapshot RunSnapshot, recentEvidence []ContextEvidence) []string {
	sections := []ContextSection{
		{ID: "workspace", Priority: 100},
		{ID: "run", Priority: 100},
	}
	if len(readModelMapValue(snapshot.Run.ExecutionContext["goal"])) > 0 {
		sections = append(sections, ContextSection{ID: "goal", Priority: 95})
	}
	if len(readModelMapValue(snapshot.Run.ExecutionContext["dod"])) > 0 {
		sections = append(sections, ContextSection{ID: "dod", Priority: 95})
	}
	if hasNonDefinitionExecutionContext(snapshot.Run.ExecutionContext) {
		sections = append(sections, ContextSection{ID: "execution_context", Priority: 85})
	}
	sections = append(sections, ContextSection{ID: "task_progress", Priority: 90})
	if len(snapshot.TaskDecompositions) > 0 {
		sections = append(sections, ContextSection{ID: "task_decomposition", Priority: 85})
	}
	if len(snapshot.ExecutionSections) > 0 {
		sections = append(sections, ContextSection{ID: "execution_sections", Priority: 84})
	}
	if len(snapshot.MemoryRecallTraces) > 0 {
		sections = append(sections, ContextSection{ID: "memory_recall", Priority: 83})
	}
	if len(snapshot.Decisions) > 0 || len(snapshot.Approvals) > 0 {
		sections = append(sections, ContextSection{ID: "governance", Priority: 80})
	}
	if len(runScopedResults(snapshot)) > 0 || len(runScopedArtifacts(snapshot)) > 0 {
		sections = append(sections, ContextSection{ID: "formal_outputs", Priority: 90})
	}
	if len(recentEvidence) > 0 {
		sections = append(sections, ContextSection{ID: "recent_evidence", Priority: 70})
	}
	sections = append(sections, ContextSection{ID: "recovery", Priority: 100})
	sections = compactContextSections(sections)
	refs := make([]string, 0, len(sections))
	for _, section := range sections {
		refs = append(refs, section.ID)
	}
	return compactUniqueStrings(refs)
}

func hasNonDefinitionExecutionContext(executionContext map[string]any) bool {
	for key := range executionContext {
		if key != "goal" && key != "dod" && key != "compressedContext" {
			return true
		}
	}
	return false
}

func mapTaskDecompositions(snapshot RunSnapshot) []TaskDecomposition {
	if len(snapshot.Steps) == 0 {
		return nil
	}
	stepByNodeRoot := taskExecutionStepsByRoot(snapshot.Steps)
	ret := make([]TaskDecomposition, 0)
	for _, step := range snapshot.Steps {
		graph := taskGraphFromValue(step.Output["executionGraph"])
		nodes := readModelMapSliceValue(graph["nodes"])
		if len(nodes) == 0 {
			continue
		}
		rootStepID := step.ID
		executedByNode := stepByNodeRoot[rootStepID]
		item := TaskDecomposition{
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
			taskNode := taskDecompositionNode(node, executedByNode)
			item.Nodes = append(item.Nodes, taskNode)
			item.EvidenceRefs = append(item.EvidenceRefs, taskNode.EvidenceRefs...)
			switch taskNode.State {
			case "completed":
				item.NodeCompleted++
			case "failed":
				item.NodeFailed++
			case "running":
				item.NodeRunning++
			}
		}
		item.State = taskDecompositionState(item)
		item.EvidenceRefs = compactUniqueStrings(item.EvidenceRefs)
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapTaskPlans(snapshot RunSnapshot) []TaskPlan {
	decompositions := runSnapshotTaskDecompositions(snapshot)
	if len(decompositions) == 0 {
		return nil
	}
	stepByID := map[string]presenter.StepView{}
	for _, step := range snapshot.Steps {
		stepByID[step.ID] = step
	}
	occurrences := timelineStepOccurrences(snapshot)
	grouped := map[string][]TaskPlan{}
	for _, decomposition := range decompositions {
		if strings.TrimSpace(decomposition.RootStepID) == "" {
			continue
		}
		step, hasStep := stepByID[decomposition.RootStepID]
		anchorStepID := strings.TrimSpace(decomposition.RootStepID)
		stage := ""
		state := taskPlanStateFromDecomposition(decomposition.State)
		occurredAt := strings.TrimSpace(occurrences[decomposition.RootStepID])
		if hasStep {
			if root := strings.TrimSpace(readModelStringValue(step.Input["graphRootStepId"])); root != "" {
				anchorStepID = root
			}
			stage = strings.TrimSpace(readModelStringValue(step.Output["stage"]))
			if stage == "" {
				stage = strings.TrimSpace(readModelStringValue(step.Input["stage"]))
			}
			state = taskPlanStateFromStep(step.State)
		}
		plan := TaskPlan{
			WorkspaceID:      snapshot.Run.WorkspaceID,
			RunID:            snapshot.Run.ID,
			AnchorStepID:     anchorStepID,
			RevisionStepID:   decomposition.RootStepID,
			State:            state,
			Stage:            stage,
			Summary:          decomposition.Summary,
			GraphBranch:      decomposition.GraphBranch,
			NodeTotal:        decomposition.NodeTotal,
			NodeCompleted:    decomposition.NodeCompleted,
			NodeFailed:       decomposition.NodeFailed,
			NodeRunning:      decomposition.NodeRunning,
			EvidenceRefs:     compactUniqueStrings(append([]string{decomposition.RootStepID}, decomposition.EvidenceRefs...)),
			OccurredAt:       occurredAt,
			SourceProjection: "task_decomposition",
		}
		grouped[anchorStepID] = append(grouped[anchorStepID], plan)
	}
	if len(grouped) == 0 {
		return nil
	}
	ret := make([]TaskPlan, 0)
	anchors := make([]string, 0, len(grouped))
	for anchor := range grouped {
		anchors = append(anchors, anchor)
	}
	sort.Strings(anchors)
	for _, anchor := range anchors {
		plans := grouped[anchor]
		sort.SliceStable(plans, func(i, j int) bool {
			leftAt := strings.TrimSpace(plans[i].OccurredAt)
			rightAt := strings.TrimSpace(plans[j].OccurredAt)
			if leftAt == rightAt {
				return plans[i].RevisionStepID < plans[j].RevisionStepID
			}
			if leftAt == "" {
				return false
			}
			if rightAt == "" {
				return true
			}
			return leftAt < rightAt
		})
		for idx := range plans {
			plans[idx].Revision = idx + 1
			plans[idx].ID = fmt.Sprintf("%s/plan/%d", plans[idx].AnchorStepID, plans[idx].Revision)
			if plans[idx].Revision > 1 && plans[idx].State == "accepted" {
				plans[idx].State = "revised"
			}
			ret = append(ret, plans[idx])
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapClosureTraces(snapshot RunSnapshot) []ClosureTrace {
	ret := make([]ClosureTrace, 0)
	for _, step := range snapshot.Steps {
		trace, ok := closureTraceFromStep(step)
		if ok {
			ret = append(ret, trace)
		}
	}
	return ret
}

func closureTraceFromStep(step presenter.StepView) (ClosureTrace, bool) {
	input := step.Input
	output := step.Output
	trace := ClosureTrace{
		StepID: step.ID,
		Kind:   canonicalReadModelStepKind(step.Kind),
		State:  step.State,
	}
	trace.GraphFollowUp = readModelBoolValue(input["graphFollowUp"])
	trace.AssessmentOnly = readModelBoolValue(input["assessmentOnly"]) || readModelBoolValue(input["providerRetryAssessmentOnly"])
	trace.GraphExpansionLimitFollowUp = readModelBoolValue(input["graphExpansionLimitFollowUp"])
	trace.GraphFollowUpExpansionLimitExceeded = readModelBoolValue(output["graphFollowUpExpansionLimitExceeded"])
	trace.GraphFollowUpExpansionCount = firstPositiveInt(
		readModelIntValue(output["graphFollowUpExpansionCount"]),
		readModelIntValue(input["graphFollowUpExpansionCount"]),
	)
	trace.GraphFollowUpExpansionLimit = firstPositiveInt(
		readModelIntValue(output["graphFollowUpExpansionLimit"]),
		readModelIntValue(input["graphFollowUpExpansionLimit"]),
	)
	trace.IgnoredExecutionGraph = readModelBoolValue(output["ignoredExecutionGraph"])
	trace.Reason = strings.TrimSpace(firstNonEmptyReadModel(
		readModelStringValue(input["assessmentOnlyReason"]),
		readModelStringValue(output["error"]),
		readModelStringValue(output["reason"]),
	))
	trace.MissingDoDItems = readModelMissingDoDItemsFromStepOutput(output)
	trace.EvidenceRefs = compactUniqueStrings([]string{step.ID})
	ok := trace.GraphFollowUp ||
		trace.AssessmentOnly ||
		trace.GraphExpansionLimitFollowUp ||
		trace.GraphFollowUpExpansionLimitExceeded ||
		trace.IgnoredExecutionGraph ||
		len(trace.MissingDoDItems) > 0
	return trace, ok
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func taskPlanStateFromStep(state string) string {
	switch strings.TrimSpace(state) {
	case string(common.StepStateCompleted):
		return "accepted"
	case string(common.StepStateFailed):
		return "rejected"
	case string(common.StepStateDispatched):
		return "proposed"
	default:
		return "proposed"
	}
}

func taskPlanStateFromDecomposition(state string) string {
	switch strings.TrimSpace(state) {
	case "completed":
		return "accepted"
	case "failed":
		return "rejected"
	default:
		return "proposed"
	}
}

func taskExecutionStepsByRoot(steps []presenter.StepView) map[string]map[string]presenter.StepView {
	ret := map[string]map[string]presenter.StepView{}
	for _, step := range steps {
		nodeID := readModelStringValue(step.Input["graphNodeId"])
		if nodeID == "" {
			continue
		}
		rootStepID := readModelStringValue(step.Input["graphRootStepId"])
		if rootStepID == "" {
			rootStepID = graphRootStepIDFromStepID(step.ID)
		}
		if rootStepID == "" {
			continue
		}
		if ret[rootStepID] == nil {
			ret[rootStepID] = map[string]presenter.StepView{}
		}
		ret[rootStepID][nodeID] = step
	}
	return ret
}

func taskGraphFromValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneReadModelMap(typed)
	default:
		return nil
	}
}

func taskDecompositionNode(raw map[string]any, executedByNode map[string]presenter.StepView) TaskDecompositionNode {
	nodeID := readModelStringValue(raw["id"])
	input, _ := raw["input"].(map[string]any)
	ret := TaskDecompositionNode{
		ID:        nodeID,
		Kind:      canonicalReadModelStepKind(readModelStringValue(raw["kind"])),
		Title:     taskNodeTitle(raw),
		State:     "pending",
		DependsOn: readModelStringSliceValue(raw["dependsOn"]),
		Branch:    readModelStringValue(raw["branch"]),
		Input:     cloneReadModelMap(input),
	}
	if executedByNode != nil {
		if step, ok := executedByNode[nodeID]; ok {
			ret.StepID = step.ID
			ret.StepState = step.State
			ret.State = taskNodeStateFromStep(step.State)
			ret.EvidenceRefs = compactUniqueStrings([]string{nodeID, step.ID})
			return ret
		}
	}
	ret.EvidenceRefs = compactUniqueStrings([]string{nodeID})
	return ret
}

func taskNodeTitle(raw map[string]any) string {
	if title := readModelStringValue(raw["summary"]); title != "" {
		return title
	}
	input, _ := raw["input"].(map[string]any)
	if command := readModelStringValue(input["command"]); command != "" {
		return command
	}
	if tool := readModelStringValue(input["tool"]); tool != "" {
		return tool
	}
	if operation := readModelStringValue(input["operation"]); operation != "" {
		return operation
	}
	return readModelStringValue(raw["kind"])
}

func taskNodeStateFromStep(state string) string {
	switch state {
	case string(common.StepStateCompleted):
		return "completed"
	case string(common.StepStateFailed):
		return "failed"
	case string(common.StepStateDispatched):
		return "running"
	default:
		if state != "" {
			return state
		}
		return "pending"
	}
}

func taskDecompositionState(item TaskDecomposition) string {
	if item.NodeFailed > 0 {
		return "failed"
	}
	if item.NodeTotal > 0 && item.NodeCompleted == item.NodeTotal {
		return "completed"
	}
	if item.NodeCompleted > 0 || item.NodeRunning > 0 {
		return "running"
	}
	return "pending"
}

func mapExecutionSections(snapshot RunSnapshot) []ExecutionSection {
	if len(snapshot.TaskDecompositions) == 0 {
		return nil
	}
	dodItems := runDoDItems(snapshot)
	refs := executionEvidenceRefsByStep(snapshot)
	ret := make([]ExecutionSection, 0)
	for _, decomposition := range snapshot.TaskDecompositions {
		for _, node := range decomposition.Nodes {
			section := ExecutionSection{
				ID:              decomposition.ID + "/section/" + node.ID,
				WorkspaceID:     snapshot.Run.WorkspaceID,
				RunID:           snapshot.Run.ID,
				Title:           firstNonEmptyReadModel(node.Title, node.ID, node.Kind),
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
			ret = append(ret, section)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

type executionEvidenceRefs struct {
	evidenceByStep      map[string][]string
	decisionsByStep     map[string][]string
	approvalsByStep     map[string][]string
	toolCallsByStep     map[string][]string
	patchesByStep       map[string][]string
	validationsByStep   map[string][]string
	resultsByStep       map[string][]string
	artifactsByStep     map[string][]string
	memoryRecallsByStep map[string][]string
	memoryIDsByStep     map[string][]string
}

func executionEvidenceRefsByStep(snapshot RunSnapshot) executionEvidenceRefs {
	ret := executionEvidenceRefs{
		evidenceByStep:      map[string][]string{},
		decisionsByStep:     map[string][]string{},
		approvalsByStep:     map[string][]string{},
		toolCallsByStep:     map[string][]string{},
		patchesByStep:       map[string][]string{},
		validationsByStep:   map[string][]string{},
		resultsByStep:       map[string][]string{},
		artifactsByStep:     map[string][]string{},
		memoryRecallsByStep: map[string][]string{},
		memoryIDsByStep:     map[string][]string{},
	}
	add := func(target map[string][]string, stepID string, refs ...string) {
		stepID = strings.TrimSpace(stepID)
		if stepID == "" {
			return
		}
		for _, ref := range refs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			target[stepID] = append(target[stepID], ref)
			ret.evidenceByStep[stepID] = append(ret.evidenceByStep[stepID], ref)
		}
	}
	for _, item := range snapshot.Decisions {
		add(ret.decisionsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.Approvals {
		add(ret.approvalsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.ToolCalls {
		add(ret.toolCallsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.Patches {
		add(ret.patchesByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.ValidationResults {
		add(ret.validationsByStep, item.StepID, item.ID, item.EvidenceRef)
	}
	for _, item := range snapshot.Results {
		if item.SourceType == "dod_assessment" {
			add(ret.resultsByStep, item.SourceRef, append([]string{item.ID}, item.EvidenceRefs...)...)
		}
	}
	for _, item := range snapshot.Artifacts {
		add(ret.artifactsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.MemoryRecallTraces {
		add(ret.memoryRecallsByStep, item.StepID, item.ID, item.EvidenceRef)
		add(ret.memoryIDsByStep, item.StepID, item.RetrievedMemoryIDs...)
	}
	for stepID, refs := range ret.evidenceByStep {
		ret.evidenceByStep[stepID] = compactUniqueStrings(refs)
	}
	return ret
}

func runDoDItems(snapshot RunSnapshot) []string {
	if snapshot.Run.DoDID == "" {
		return nil
	}
	for _, item := range snapshot.Results {
		if item.DoDID != snapshot.Run.DoDID || item.DoDAssessment == nil {
			continue
		}
		ret := make([]string, 0, len(item.DoDAssessment.Items))
		for _, assessmentItem := range item.DoDAssessment.Items {
			if strings.TrimSpace(assessmentItem.Item) != "" {
				ret = append(ret, assessmentItem.Item)
			}
		}
		if len(ret) > 0 {
			return ret
		}
	}
	return nil
}

func relatedDoDItemsForSection(node TaskDecompositionNode, dodItems []string) []string {
	if len(dodItems) == 0 {
		return nil
	}
	haystack := strings.ToLower(strings.Join([]string{node.ID, node.Title, node.Kind}, " "))
	ret := make([]string, 0)
	for _, item := range dodItems {
		if strings.Contains(haystack, strings.ToLower(strings.TrimSpace(item))) {
			ret = append(ret, item)
		}
	}
	return ret
}

func executionSectionSummary(node TaskDecompositionNode) string {
	switch node.State {
	case "completed":
		return "section completed with linked runtime evidence"
	case "failed":
		return "section failed; inspect linked evidence and governance records"
	case "running":
		return "section is running"
	default:
		return "section is planned"
	}
}

func graphRootStepIDFromStepID(stepID string) string {
	idx := strings.Index(stepID, "/block/")
	if idx <= 0 {
		return ""
	}
	return stepID[:idx]
}

func lastContextEvidence(items []ContextEvidence, limit int) []ContextEvidence {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	start := len(items) - limit
	if start < 0 {
		start = 0
	}
	return append([]ContextEvidence(nil), items[start:]...)
}

func resultRefsFromTaskProgress(progress TaskProgress) []string {
	var refs []string
	for _, item := range progress.Items {
		if item.Kind == "result" || item.Kind == "artifact" {
			refs = append(refs, item.ID)
			refs = append(refs, item.EvidenceRefs...)
		}
	}
	return compactUniqueStrings(refs)
}

func contextBundleSummary(snapshot RunSnapshot, progress TaskProgress) string {
	if progress.Summary != "" {
		return progress.Summary
	}
	return "run " + snapshot.Run.State + " context bundle for recovery and compression"
}

func contextWorkspaceSection(snapshot RunSnapshot) ContextSection {
	return ContextSection{
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

func contextRunSection(snapshot RunSnapshot) ContextSection {
	return ContextSection{
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

func contextGoalSection(snapshot RunSnapshot) ContextSection {
	goal, _ := snapshot.Run.ExecutionContext["goal"].(map[string]any)
	return ContextSection{
		ID:       "goal",
		Kind:     "goal",
		Title:    "Goal",
		Priority: 95,
		Summary:  readModelStringValue(goal["text"]),
		Data:     cloneReadModelMap(goal),
	}
}

func contextDoDSection(snapshot RunSnapshot) ContextSection {
	dod, _ := snapshot.Run.ExecutionContext["dod"].(map[string]any)
	return ContextSection{
		ID:       "dod",
		Kind:     "dod",
		Title:    "Definition of Done",
		Priority: 95,
		State:    readModelStringValue(dod["state"]),
		Summary:  contextDoDSummary(dod),
		Data:     cloneReadModelMap(dod),
	}
}

func contextExecutionContextSection(snapshot RunSnapshot) ContextSection {
	data := cloneReadModelMap(snapshot.Run.ExecutionContext)
	delete(data, "goal")
	delete(data, "dod")
	delete(data, "compressedContext")
	delete(data, "coverageWorkItems")
	return ContextSection{
		ID:       "execution_context",
		Kind:     "execution_context",
		Title:    "Execution context",
		Priority: 85,
		Summary:  "selected Soul, Skill, MCP, Memory, and provider context",
		Data:     data,
	}
}

func contextTaskProgressSection(progress TaskProgress) ContextSection {
	return ContextSection{
		ID:           "task_progress",
		Kind:         "task_progress",
		Title:        "Task progress",
		Priority:     90,
		State:        progress.State,
		Summary:      progress.Summary,
		EvidenceRefs: progress.EvidenceRefs,
		Data: map[string]any{
			"currentMilestone": progress.CurrentMilestone,
			"milestones":       progress.Milestones,
			"blockers":         progress.Blockers,
			"recovery":         progress.Recovery,
		},
	}
}

func contextTaskDecompositionSection(items []TaskDecomposition) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0, len(items))
	state := "completed"
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
		if item.State == "failed" {
			state = "failed"
			continue
		}
		if item.State != "completed" && state != "failed" {
			state = "running"
		}
	}
	return ContextSection{
		ID:           "task_decomposition",
		Kind:         "task_decomposition",
		Title:        "Task decomposition",
		Priority:     85,
		State:        state,
		Summary:      strconv.Itoa(len(items)) + " decomposition graphs",
		EvidenceRefs: refs,
		Data: map[string]any{
			"taskDecompositions": items,
		},
	}
}

func contextExecutionSectionsSection(items []ExecutionSection) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0)
	state := "completed"
	for _, item := range items {
		refs = append(refs, item.ActualEvidenceRefs...)
		if item.State == "failed" {
			state = "failed"
			continue
		}
		if item.State != "completed" && state != "failed" {
			state = "running"
		}
	}
	return ContextSection{
		ID:           "execution_sections",
		Kind:         "execution_sections",
		Title:        "Execution sections",
		Priority:     84,
		State:        state,
		Summary:      strconv.Itoa(len(items)) + " execution sections",
		EvidenceRefs: compactUniqueStrings(refs),
		Data: map[string]any{
			"executionSections": items,
		},
	}
}

func contextMemoryRecallSection(items []MemoryRecallTrace) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0)
	memoryIDs := make([]string, 0)
	status := "selected"
	for _, item := range items {
		refs = append(refs, item.ID, item.EvidenceRef)
		memoryIDs = append(memoryIDs, item.RetrievedMemoryIDs...)
		if item.Status == "degraded" {
			status = "degraded"
		}
	}
	return ContextSection{
		ID:           "memory_recall",
		Kind:         "memory_recall",
		Title:        "Memory recall",
		Priority:     83,
		State:        status,
		Summary:      strconv.Itoa(len(items)) + " memory recall traces",
		EvidenceRefs: compactUniqueStrings(append(refs, memoryIDs...)),
		Data: map[string]any{
			"memoryRecallTraces": items,
			"memoryIds":          compactUniqueStrings(memoryIDs),
		},
	}
}

func contextGovernanceSection(snapshot RunSnapshot) ContextSection {
	language := snapshotLanguage(snapshot)
	if len(snapshot.Decisions) == 0 && len(snapshot.Approvals) == 0 {
		return ContextSection{}
	}
	return ContextSection{
		ID:           "governance",
		Kind:         "governance",
		Title:        localizedMilestoneTitle(language, "governance"),
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

func contextFormalOutputSection(snapshot RunSnapshot) ContextSection {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	if len(results) == 0 && len(artifacts) == 0 {
		return ContextSection{}
	}
	return ContextSection{
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

func contextRecentEvidenceSection(items []ContextEvidence) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0, len(items))
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
	}
	return ContextSection{
		ID:           "recent_evidence",
		Kind:         "recent_evidence",
		Title:        "Recent evidence",
		Priority:     70,
		Summary:      "latest runtime timeline items retained for recovery",
		EvidenceRefs: compactUniqueStrings(refs),
		Data: map[string]any{
			"items": items,
		},
	}
}

func contextRecoverySection(recovery RecoveryPlan) ContextSection {
	return ContextSection{
		ID:       "recovery",
		Kind:     "recovery",
		Title:    "Recovery recommendation",
		Priority: 100,
		State:    recovery.Mode,
		Summary:  recovery.NextAction,
		EvidenceRefs: compactUniqueStrings([]string{
			recovery.StepID,
			recovery.BlockingTicketID,
			recovery.BlockingApprovalID,
			recovery.CheckpointID,
		}),
		Data: map[string]any{
			"mode":               recovery.Mode,
			"nextAction":         recovery.NextAction,
			"runId":              recovery.RunID,
			"stepId":             recovery.StepID,
			"blockingTicketId":   recovery.BlockingTicketID,
			"blockingApprovalId": recovery.BlockingApprovalID,
			"checkpointId":       recovery.CheckpointID,
			"reason":             recovery.Reason,
		},
	}
}

func contextRecentEvidence(timeline []RuntimeTimelineItem, limit int) []ContextEvidence {
	if len(timeline) == 0 || limit <= 0 {
		return nil
	}
	start := len(timeline) - limit
	if start < 0 {
		start = 0
	}
	ret := make([]ContextEvidence, 0, len(timeline)-start)
	for _, item := range timeline[start:] {
		ret = append(ret, ContextEvidence{
			ID:         firstNonEmptyReadModel(item.ID, item.Topic, item.StepID),
			Kind:       firstNonEmptyReadModel(item.Source+"/"+item.Type, item.Source),
			State:      firstNonEmptyReadModel(item.StepState, item.RunState),
			Summary:    item.Summary,
			OccurredAt: item.OccurredAt,
			EvidenceRefs: compactUniqueStrings([]string{
				item.ID,
				item.StepID,
				item.Topic,
			}),
			Data: compactContextEvidenceData(item),
		})
	}
	return ret
}

func compactContextEvidenceData(item RuntimeTimelineItem) map[string]any {
	data := map[string]any{}
	if item.Topic != "" {
		data["topic"] = item.Topic
	}
	if item.RunState != "" {
		data["runState"] = item.RunState
	}
	if item.StepState != "" {
		data["stepState"] = item.StepState
	}
	if item.Summary != "" {
		data["summary"] = item.Summary
	}
	if len(data) == 0 {
		return nil
	}
	return data
}

func contextCompressionCandidates(bundle ContextBundle) []ContextCompressionCandidate {
	var ret []ContextCompressionCandidate
	if len(bundle.RecentEvidence) > 8 {
		ret = append(ret, ContextCompressionCandidate{
			SectionID:     "recent_evidence",
			Reason:        "recent evidence exceeds compact recovery window",
			Retention:     "retain latest evidence ids, states, summaries, and timestamps",
			SourceRefs:    contextEvidenceRefs(bundle.RecentEvidence),
			SuggestedMode: "summarize",
		})
	}
	if len(bundle.TaskProgress.Items) > 10 {
		ret = append(ret, ContextCompressionCandidate{
			SectionID:     "task_progress",
			Reason:        "task progress contains many low-level items",
			Retention:     "retain milestones, blockers, recovery, and accepted result refs",
			SourceRefs:    bundle.TaskProgress.EvidenceRefs,
			SuggestedMode: "milestone_summary",
		})
	}
	for _, section := range bundle.Sections {
		if section.ID == "execution_context" && len(section.Data) > 4 {
			ret = append(ret, ContextCompressionCandidate{
				SectionID:     section.ID,
				Reason:        "execution context catalog can be summarized after activation",
				Retention:     "retain selected ids and policy evidence, summarize verbose catalog entries",
				SuggestedMode: "catalog_summary",
			})
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func contextEvidenceRefs(items []ContextEvidence) []string {
	var refs []string
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
	}
	return compactUniqueStrings(refs)
}

func compactContextSections(items []ContextSection) []ContextSection {
	ret := make([]ContextSection, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		item.EvidenceRefs = compactUniqueStrings(item.EvidenceRefs)
		ret = append(ret, item)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].Priority == ret[j].Priority {
			return ret[i].ID < ret[j].ID
		}
		return ret[i].Priority > ret[j].Priority
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func currentStepID(snapshot RunSnapshot) string {
	if result := terminalTaskProgressResult(snapshot); result != nil {
		return firstNonEmptyReadModel(result.SourceRef, result.ID)
	}
	if snapshot.Step == nil {
		return ""
	}
	return snapshot.Step.ID
}

func terminalTaskProgressResult(snapshot RunSnapshot) *Result {
	if !isTerminalRunState(snapshot.Run.State) {
		return nil
	}
	return primaryTaskProgressResult(runScopedResults(snapshot))
}

func contextDoDSummary(dod map[string]any) string {
	if len(dod) == 0 {
		return ""
	}
	switch items := dod["items"].(type) {
	case []string:
		return strconv.Itoa(len(items)) + " DoD items"
	case []any:
		return strconv.Itoa(len(items)) + " DoD items"
	default:
		return ""
	}
}

func formalOutputSummary(items []Result) string {
	if result := primaryTaskProgressResult(items); result != nil {
		return result.Summary
	}
	return ""
}

func taskProgressBlockers(snapshot RunSnapshot) []ProgressBlocker {
	var ret []ProgressBlocker
	for _, item := range snapshot.Decisions {
		if isResolvedDecisionState(item.State) {
			continue
		}
		ret = append(ret, ProgressBlocker{
			ID:          item.ID,
			Kind:        "decision",
			State:       item.State,
			Reason:      firstNonEmptyReadModel(item.Question, item.Reason),
			EvidenceRef: item.StepID,
		})
	}
	for _, item := range snapshot.Approvals {
		if isResolvedApprovalState(item.State) {
			continue
		}
		ret = append(ret, ProgressBlocker{
			ID:          item.ID,
			Kind:        "approval",
			State:       item.State,
			Reason:      item.Reason,
			EvidenceRef: item.StepID,
		})
	}
	return ret
}

func taskProgressItems(snapshot RunSnapshot) []ProgressItem {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	ret := make([]ProgressItem, 0, len(snapshot.Steps)+len(snapshot.ToolCalls)+len(snapshot.ValidationResults)+len(results)+len(artifacts)+len(snapshot.Decisions)+len(snapshot.Approvals))
	recoveredHistoricalFailures := acceptedTerminalRun(snapshot)
	for _, item := range snapshot.Steps {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "step", Title: firstNonEmptyReadModel(item.Kind, item.ID), State: recoveredProgressItemState(item.State, recoveredHistoricalFailures), EvidenceRefs: []string{item.ID}})
	}
	for _, item := range snapshot.ToolCalls {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "tool_call", Title: firstNonEmptyReadModel(item.ToolName, item.ID), State: recoveredProgressItemState(item.State, recoveredHistoricalFailures), EvidenceRefs: []string{item.ID, item.StepID}})
	}
	for _, item := range snapshot.Patches {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "patch", Title: firstNonEmptyReadModel(item.TargetRef, item.PatchType, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.ToolCallID, item.StepID}})
	}
	for _, item := range snapshot.ValidationResults {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "validation_result", Title: firstNonEmptyReadModel(item.ValidationKind, item.TargetRef, item.ID), State: recoveredProgressItemState(item.State, recoveredHistoricalFailures), EvidenceRefs: []string{item.ID, item.EvidenceRef, item.StepID}})
	}
	for _, item := range results {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "result", Title: firstNonEmptyReadModel(item.Title, item.SourceType, item.ID), State: item.State, EvidenceRefs: compactUniqueStrings(append([]string{item.ID, item.SourceRef}, item.EvidenceRefs...))})
	}
	for _, item := range artifacts {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "artifact", Title: firstNonEmptyReadModel(item.Name, item.URI, item.ArtifactType, item.ID), State: item.State, EvidenceRefs: compactUniqueStrings(append([]string{item.ID, item.ResultID, item.StepID, item.ToolCallID, item.PatchID, item.ValidationResultID}, item.EvidenceRefs...))})
	}
	for _, item := range snapshot.Decisions {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "decision", Title: firstNonEmptyReadModel(item.Question, item.DecisionKind, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.StepID}})
	}
	for _, item := range snapshot.Approvals {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "approval", Title: firstNonEmptyReadModel(item.Reason, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.StepID}})
	}
	for idx := range ret {
		ret[idx].EvidenceRefs = compactUniqueStrings(ret[idx].EvidenceRefs)
	}
	return ret
}

func recoveredProgressItemState(state string, recoveredHistoricalFailures bool) string {
	if recoveredHistoricalFailures && strings.TrimSpace(state) == "failed" {
		return "recovered"
	}
	return state
}

func taskProgressRecovery(snapshot RunSnapshot, latestCheckpoint *RuntimeCheckpoint, blockers []ProgressBlocker, fallback RecoveryPlan) RecoveryPlan {
	ret := fallback
	ret.RunID = snapshot.Run.ID
	ret.BlockingTicketID = snapshot.Run.BlockingTicketID
	ret.BlockingApprovalID = snapshot.Run.BlockingApprovalID
	if latestCheckpoint != nil {
		ret.CheckpointID = latestCheckpoint.ID
		if ret.BlockingTicketID == "" && ret.BlockingApprovalID == "" && latestCheckpoint.CurrentStepID != "" {
			ret.StepID = latestCheckpoint.CurrentStepID
		} else if ret.StepID == "" {
			ret.StepID = latestCheckpoint.CurrentStepID
		}
		if ret.BlockingTicketID == "" {
			ret.BlockingTicketID = latestCheckpoint.BlockingTicketID
		}
		if ret.BlockingApprovalID == "" {
			ret.BlockingApprovalID = latestCheckpoint.BlockingApprovalID
		}
		if ret.FailureClass == "" {
			ret.FailureClass = strings.TrimSpace(readModelStringValue(latestCheckpoint.Data["failureClass"]))
		}
		if ret.FailureSignature == "" {
			ret.FailureSignature = strings.TrimSpace(readModelStringValue(latestCheckpoint.Data["failureSignature"]))
		}
	}
	if ret.BlockingTicketID != "" {
		ret.Mode = "wait_decision"
		ret.NextAction = "resolve_decision_ticket"
		ret.Reason = blockerReason(blockers, "decision", ret.BlockingTicketID)
		return ret
	}
	if ret.BlockingApprovalID != "" {
		ret.Mode = "wait_approval"
		ret.NextAction = "resolve_approval"
		ret.Reason = blockerReason(blockers, "approval", ret.BlockingApprovalID)
		return ret
	}
	switch common.RunState(snapshot.Run.State) {
	case common.RunStateCompleted:
		ret.Mode = "none"
		ret.NextAction = "inspect_result"
		ret.Reason = "run completed"
	case common.RunStateFailed:
		ret.Mode = "inspect_failure"
		ret.NextAction = "inspect_failure_reason_and_evidence"
		ret.Reason = snapshot.Run.FailureReason
	case common.RunStateCancelled, common.RunStateSuperseded:
		ret.Mode = "restart"
		ret.NextAction = "start_new_run"
		ret.Reason = "run is terminal but not completed"
	case common.RunStateValidating:
		ret.Mode = "continue"
		ret.NextAction = "continue_validation"
		ret.Reason = "run is validating"
	default:
		ret.Mode = "continue"
		ret.NextAction = "continue_run"
		ret.Reason = "run is not terminal"
	}
	return ret
}

func taskProgressSummary(progress TaskProgress, snapshot *RunSnapshot) string {
	if snapshot != nil {
		results := runScopedResults(*snapshot)
		artifacts := runScopedArtifacts(*snapshot)
		if result := primaryTaskProgressResult(results); result != nil && result.Summary != "" {
			return result.Summary
		}
		if snapshot.Run.FailureReason != "" {
			return snapshot.Run.FailureReason
		}
		changedFiles := appliedPatchTargetCount(snapshot.Patches)
		summary := "run " + snapshot.Run.State + " with " + strconv.Itoa(len(snapshot.Steps)) + " steps, " + strconv.Itoa(len(results)) + " results, " + strconv.Itoa(len(artifacts)) + " artifacts"
		if changedFiles > 0 {
			summary += ", " + strconv.Itoa(changedFiles) + " changed files"
		}
		return summary
	}
	return "workspace " + progress.State + " with " + strconv.Itoa(len(progress.Blockers)) + " open blockers"
}

func appliedPatchTargetCount(items []Patch) int {
	if len(items) == 0 {
		return 0
	}
	targets := map[string]struct{}{}
	for _, item := range items {
		if item.State != string(common.PatchStateApplied) {
			continue
		}
		target := strings.TrimSpace(item.TargetRef)
		if target == "" {
			target = strings.TrimSpace(item.ID)
		}
		if target == "" {
			continue
		}
		targets[target] = struct{}{}
	}
	return len(targets)
}

func mapTaskProgressRepairLoop(snapshot RunSnapshot, recovery RecoveryPlan) RepairLoop {
	ret := RepairLoop{
		State:      "inactive",
		NextAction: recovery.NextAction,
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
			ret.MissingDoDItems = readModelMissingDoDItemsFromStepOutput(failedStep.Output)
		}
	}
	projectRepairGroundingObservability(&ret, snapshot)

	if ticketID, reason, escalationContext, ok := repairEscalationProjection(snapshot, recovery); ok {
		ret.Escalated = true
		ret.EscalationTicketID = ticketID
		ret.EscalationReason = reason
		ret.EvidenceRefs = append(ret.EvidenceRefs, ticketID)
		ret.State = "escalated_decision"
		if items := readModelMapSliceValue(escalationContext["missingDoDItems"]); len(items) > 0 {
			ret.MissingDoDItems = items
		}
		if attempts := readModelMapSliceValue(escalationContext["recentRepairAttempts"]); len(attempts) > 0 {
			ret.RecentAttempts = attempts
		}
		if suggestions := readModelStringSliceValue(escalationContext["recommendedNextActions"]); len(suggestions) > 0 {
			ret.RecommendedNextSteps = suggestions
		}
	} else {
		switch recovery.Mode {
		case "wait_decision":
			ret.State = "awaiting_decision"
		case "wait_approval":
			ret.State = "awaiting_approval"
		case "none":
			if attemptCount > 0 {
				ret.State = "completed_after_repair"
			}
		case "inspect_failure", "restart":
			ret.State = "terminated"
		default:
			if attemptCount > 0 || lastFailure != nil {
				ret.State = "repairing"
			}
		}
	}

	ret.EvidenceRefs = compactUniqueStrings(append(ret.EvidenceRefs, ret.LastRepairStepID, ret.LastFailureStepID))
	return ret
}

func projectRepairGroundingObservability(ret *RepairLoop, snapshot RunSnapshot) {
	if ret == nil || strings.TrimSpace(ret.LastRepairStepID) == "" {
		return
	}
	repairStep := stepByID(snapshot.Steps, ret.LastRepairStepID)
	if repairStep == nil {
		return
	}
	actionPackage := readModelMapValue(repairStep.Input["repairActionPackage"])
	targetDiagnostics := readModelMapValue(actionPackage["targetRelevanceDiagnostics"])
	ret.LastFailureTargetScope = strings.TrimSpace(readModelFirstNonEmptyString(
		readModelFailureTargetScopeFromStep(stepByID(snapshot.Steps, ret.LastFailureStepID)),
		readModelStringValue(repairStep.Input["failureTargetScope"]),
		readModelStringValue(actionPackage["failureTargetScope"]),
		readModelStringValue(targetDiagnostics["failureTargetScope"]),
	))
	if failedStep := stepByID(snapshot.Steps, ret.LastFailureStepID); failedStep != nil {
		ret.StructuredFailureSignals = readModelStructuredFailureSignalsFromStep(failedStep, ret.LastFailureClass, ret.LastFailureSignature)
	}
	if len(ret.StructuredFailureSignals) > 0 {
		// Current failed step wins. Repair-step inputs may still contain inherited stale signals.
	} else if signals := readModelMapValue(repairStep.Input["structuredFailureSignals"]); len(signals) > 0 {
		ret.StructuredFailureSignals = signals
	} else if signals := readModelMapValue(actionPackage["structuredFailureSignals"]); len(signals) > 0 {
		ret.StructuredFailureSignals = signals
	}
	if compaction := readModelMapValue(repairStep.Input["repairInputCompaction"]); len(compaction) > 0 {
		ret.RepairInputCompaction = compaction
	}
	if repeated := readModelMapValue(actionPackage["repeatedTargetRepairMode"]); len(repeated) > 0 {
		ret.RepeatedFailureReason = strings.TrimSpace(readModelStringValue(repeated["reason"]))
	}
	ret.LastRepairInputBytes = readModelJSONSize(repairStep.Input)
}

func readModelStructuredFailureSignalsFromStep(step *presenter.StepView, failureClass string, failureSignature string) map[string]any {
	if step == nil {
		return nil
	}
	if signals := readModelMapValue(step.Output["structuredFailureSignals"]); len(signals) > 0 {
		return signals
	}
	output := step.Output
	sources := []string{}
	targetRefs := []string{}
	messages := []string{}
	addSourceMessage := func(source string, value string) {
		value = strings.TrimSpace(value)
		if source == "" || value == "" {
			return
		}
		sources = append(sources, source)
		messages = append(messages, readModelTruncateString(value, 512))
	}
	addTarget := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			targetRefs = append(targetRefs, value)
		}
	}
	addSourceMessage("failureClass", readModelFirstNonEmptyString(failureClass, readModelStringValue(output["failureClass"])))
	addSourceMessage("failureSignature", readModelFirstNonEmptyString(failureSignature, readModelStringValue(output["failureSignature"])))
	for _, key := range []string{"error", "reason", "stderr", "stdout", "stage"} {
		addSourceMessage(key, readModelStringValue(output[key]))
	}
	addTarget(readModelStringValue(output["targetRef"]))
	addTarget(readModelStringValue(output["path"]))
	diagnostics := readModelMapValue(output["runtimeDiagnostics"])
	for _, key := range []string{"targetRef", "conflictRef"} {
		addTarget(readModelStringValue(diagnostics[key]))
	}
	for _, signature := range []string{failureSignature, readModelStringValue(output["failureSignature"])} {
		for _, target := range readModelTargetsFromFailureSignature(signature) {
			addTarget(target)
		}
	}
	ret := map[string]any{}
	if sources = compactUniqueStrings(sources); len(sources) > 0 {
		ret["sources"] = readModelLimitStrings(sources, 8)
	}
	if targetRefs = compactUniqueStrings(targetRefs); len(targetRefs) > 0 {
		ret["targetRefs"] = readModelLimitStrings(targetRefs, 16)
	}
	if messages = compactUniqueStrings(messages); len(messages) > 0 {
		ret["messages"] = readModelLimitStrings(messages, 10)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func readModelFailureTargetScopeFromStep(step *presenter.StepView) string {
	if step == nil {
		return ""
	}
	signature := readModelStringValue(step.Output["failureSignature"])
	if scope := readModelFailureTargetScope(signature); scope != "" {
		return scope
	}
	return strings.TrimSpace(readModelFirstNonEmptyString(
		readModelStringValue(step.Output["targetRef"]),
		readModelStringValue(step.Output["path"]),
	))
}

func readModelFailureTargetScope(signature string) string {
	for _, target := range readModelTargetsFromFailureSignature(signature) {
		return target
	}
	return ""
}

func readModelTargetsFromFailureSignature(signature string) []string {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return nil
	}
	var ret []string
	for _, segment := range strings.Split(signature, "|") {
		key, value, ok := strings.Cut(segment, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "failureTargets":
			for _, target := range strings.Split(value, ",") {
				if target = strings.TrimSpace(target); target != "" {
					ret = append(ret, target)
				}
			}
		case "targetRef", "conflictRef":
			if value != "" {
				ret = append(ret, value)
			}
		}
	}
	return compactUniqueStrings(ret)
}

func readModelLimitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func readModelTruncateString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func readModelJSONSize(value any) int {
	if value == nil {
		return 0
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return len(encoded)
}

func readModelFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

type repairFailureProjection struct {
	stepID      string
	class       string
	signature   string
	occurredAt  string
	evidenceRef string
}

func repairAttemptProjection(steps []presenter.StepView) (count int, lastAttempt int, lastStepID string) {
	for _, step := range steps {
		if !readModelBoolValue(step.Input["runtimeRepairFollowUp"]) {
			continue
		}
		count++
		attempt := readModelIntValue(step.Input["runtimeRepairAttempt"])
		if attempt <= 0 {
			attempt = 1
		}
		if attempt > lastAttempt || (attempt == lastAttempt && step.ID > lastStepID) {
			lastAttempt = attempt
			lastStepID = step.ID
		}
	}
	if count > 0 && lastAttempt == 0 {
		lastAttempt = count
	}
	return count, lastAttempt, lastStepID
}

func latestRepairFailureProjection(snapshot RunSnapshot) *repairFailureProjection {
	var latest *repairFailureProjection
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		class := strings.TrimSpace(readModelStringValue(checkpoint.Data["failureClass"]))
		signature := strings.TrimSpace(readModelStringValue(checkpoint.Data["failureSignature"]))
		event := strings.TrimSpace(readModelStringValue(checkpoint.Data["event"]))
		if class == "" && signature == "" && event != "step_failed" {
			continue
		}
		candidate := &repairFailureProjection{
			stepID:      checkpoint.CurrentStepID,
			class:       class,
			signature:   signature,
			occurredAt:  checkpoint.OccurredAt,
			evidenceRef: checkpoint.ID,
		}
		if latest == nil || candidate.occurredAt > latest.occurredAt || (candidate.occurredAt == latest.occurredAt && candidate.evidenceRef > latest.evidenceRef) {
			latest = candidate
		}
	}
	if latest != nil {
		return latest
	}
	for _, step := range snapshot.Steps {
		class := strings.TrimSpace(readModelStringValue(step.Output["failureClass"]))
		signature := strings.TrimSpace(readModelStringValue(step.Output["failureSignature"]))
		if class == "" && signature == "" {
			continue
		}
		latest = &repairFailureProjection{
			stepID:      step.ID,
			class:       class,
			signature:   signature,
			evidenceRef: step.ID,
		}
	}
	return latest
}

func stepByID(steps []presenter.StepView, stepID string) *presenter.StepView {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return nil
	}
	for idx := range steps {
		if steps[idx].ID == stepID {
			return &steps[idx]
		}
	}
	return nil
}

func readModelMissingDoDItemsFromStepOutput(output map[string]any) []map[string]any {
	if len(output) == 0 {
		return nil
	}
	diagnostics := readModelMapValue(output["runtimeDiagnostics"])
	if items := readModelMapSliceValue(diagnostics["missingDoDItems"]); len(items) > 0 {
		return items
	}
	if failedOutput := readModelMapValue(output["failedStepOutput"]); len(failedOutput) > 0 {
		if items := readModelMissingDoDItemsFromStepOutput(failedOutput); len(items) > 0 {
			return items
		}
	}
	return nil
}

func repairEscalationProjection(snapshot RunSnapshot, recovery RecoveryPlan) (ticketID string, reason string, context map[string]any, ok bool) {
	if recovery.BlockingTicketID == "" {
		return "", "", nil, false
	}
	for _, item := range snapshot.Decisions {
		if item.ID != recovery.BlockingTicketID {
			continue
		}
		context := readModelMapValue(item.Context)
		if readModelIntValue(context["attempts"]) > 0 ||
			strings.TrimSpace(readModelStringValue(context["failureClass"])) != "" ||
			strings.TrimSpace(readModelStringValue(context["failureSignature"])) != "" {
			return item.ID, firstNonEmptyReadModel(item.Reason, item.Question), context, true
		}
		return "", "", nil, false
	}
	return "", "", nil, false
}

func primaryTaskProgressResult(items []Result) *Result {
	for idx := range items {
		if items[idx].State == string(common.ResultStateAccepted) {
			return &items[idx]
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &items[len(items)-1]
}

func stepProgressState(snapshot RunSnapshot) string {
	if len(snapshot.Steps) == 0 {
		return "pending"
	}
	if hasAcceptedRunResult(snapshot) && snapshot.Run.State == string(common.RunStateCompleted) {
		return "completed"
	}
	for _, item := range snapshot.Steps {
		if item.State == string(common.StepStateFailed) {
			return "failed"
		}
	}
	if runtimeProgress(snapshot).StepCompleted == len(snapshot.Steps) {
		return "completed"
	}
	return "running"
}

func governanceProgressState(snapshot RunSnapshot) string {
	if acceptedTerminalRun(snapshot) {
		return "completed"
	}
	if len(snapshot.Decisions) == 0 && len(snapshot.Approvals) == 0 {
		return "pending"
	}
	if snapshot.Run.BlockingTicketID != "" || snapshot.Run.BlockingApprovalID != "" {
		return "blocked"
	}
	for _, item := range snapshot.Decisions {
		if !isResolvedDecisionState(item.State) {
			return "blocked"
		}
	}
	for _, item := range snapshot.Approvals {
		if !isResolvedApprovalState(item.State) {
			return "blocked"
		}
	}
	return "completed"
}

func validationProgressState(snapshot RunSnapshot) string {
	if len(snapshot.ValidationResults) == 0 {
		if snapshot.Run.State == string(common.RunStateValidating) {
			return "running"
		}
		return "pending"
	}
	if hasAcceptedRunResult(snapshot) && snapshot.Run.State == string(common.RunStateCompleted) {
		return "completed"
	}
	for _, item := range snapshot.ValidationResults {
		if item.State == string(common.ValidationResultStateFailed) {
			return "failed"
		}
	}
	return "completed"
}

func acceptedTerminalRun(snapshot RunSnapshot) bool {
	return snapshot.Run.State == string(common.RunStateCompleted) && hasAcceptedRunResult(snapshot)
}

func resultProgressState(snapshot RunSnapshot) string {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	if len(results) == 0 && len(artifacts) == 0 {
		if isTerminalRunState(snapshot.Run.State) {
			return "failed"
		}
		return "pending"
	}
	for _, item := range results {
		if item.State == string(common.ResultStateAccepted) {
			return "completed"
		}
	}
	return progressStateFromCount(len(results)+len(artifacts), false, false)
}

func hasAcceptedRunResult(snapshot RunSnapshot) bool {
	for _, item := range runScopedResults(snapshot) {
		if item.State == string(common.ResultStateAccepted) {
			return true
		}
	}
	return false
}

func progressStateFromCount(count int, failed bool, completed bool) string {
	if failed {
		return "failed"
	}
	if completed || count > 0 {
		return "completed"
	}
	return "pending"
}

func currentProgressMilestone(items []ProgressMilestone) string {
	for _, item := range items {
		if item.State != "completed" {
			return item.ID
		}
	}
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1].ID
}

func taskProgressEvidenceRefs(snapshot RunSnapshot) []string {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	refs := []string{snapshot.Run.ID}
	if snapshot.Step != nil {
		refs = append(refs, snapshot.Step.ID)
	}
	refs = append(refs, idsFromSteps(snapshot.Steps)...)
	refs = append(refs, idsFromToolCalls(snapshot.ToolCalls)...)
	refs = append(refs, idsFromPatches(snapshot.Patches)...)
	refs = append(refs, idsFromValidations(snapshot.ValidationResults)...)
	refs = append(refs, idsFromCheckpoints(snapshot.RuntimeCheckpoints)...)
	refs = append(refs, idsFromResults(results)...)
	refs = append(refs, idsFromArtifacts(artifacts)...)
	refs = append(refs, governanceEvidenceRefs(snapshot)...)
	return compactUniqueStrings(refs)
}

func runScopedResults(snapshot RunSnapshot) []Result {
	if snapshot.Run.ID == "" {
		return snapshot.Results
	}
	ret := make([]Result, 0, len(snapshot.Results))
	for _, item := range snapshot.Results {
		if item.RunID == snapshot.Run.ID {
			ret = append(ret, item)
		}
	}
	return ret
}

func runScopedArtifacts(snapshot RunSnapshot) []Artifact {
	if snapshot.Run.ID == "" {
		return snapshot.Artifacts
	}
	ret := make([]Artifact, 0, len(snapshot.Artifacts))
	for _, item := range snapshot.Artifacts {
		if item.RunID == snapshot.Run.ID {
			ret = append(ret, item)
		}
	}
	return ret
}

func governanceEvidenceRefs(snapshot RunSnapshot) []string {
	refs := make([]string, 0, len(snapshot.Decisions)+len(snapshot.Approvals))
	for _, item := range snapshot.Decisions {
		refs = append(refs, item.ID, item.StepID, item.ResolutionID)
	}
	for _, item := range snapshot.Approvals {
		refs = append(refs, item.ID, item.StepID)
	}
	return compactUniqueStrings(refs)
}

func idsFromSteps(items []presenter.StepView) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromToolCalls(items []ToolCall) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromPatches(items []Patch) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromValidations(items []ValidationResult) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID, item.EvidenceRef)
	}
	return compactUniqueStrings(ret)
}

func idsFromCheckpoints(items []RuntimeCheckpoint) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromResults(items []Result) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
		ret = append(ret, item.ArtifactRefs...)
	}
	return compactUniqueStrings(ret)
}

func idsFromArtifacts(items []Artifact) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
	}
	return compactUniqueStrings(ret)
}

func blockerReason(items []ProgressBlocker, kind string, id string) string {
	for _, item := range items {
		if item.Kind == kind && item.ID == id {
			return item.Reason
		}
	}
	return ""
}

func isResolvedDecisionState(state string) bool {
	return state == string(common.DecisionTicketStateResolved)
}

func isResolvedApprovalState(state string) bool {
	return state == string(common.ApprovalStateApproved) || state == string(common.ApprovalStateRejected) || state == string(common.ApprovalStateResolved)
}

func firstNonEmptyReadModel(values ...string) string {
	for _, item := range values {
		if item != "" {
			return item
		}
	}
	return ""
}

func compactUniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
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
		ret.CurrentStepKind = canonicalReadModelStepKind(snapshot.Step.Kind)
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

func isTerminalRunState(state string) bool {
	switch common.RunState(state) {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func snapshotLanguage(snapshot RunSnapshot) string {
	if snapshot.Run.ExecutionContext == nil {
		return defaultDialogLanguage()
	}
	if lang, ok := snapshot.Run.ExecutionContext["dialogLanguage"].(string); ok {
		if normalized := languageNameFromLocale(lang); normalized != "" {
			return normalized
		}
	}
	return defaultDialogLanguage()
}

func workspaceSnapshotLanguage(snapshot WorkspaceSnapshot) string {
	if snapshot.Workspace.ActiveRunID != "" {
		for _, item := range snapshot.Runs {
			if item.ID == snapshot.Workspace.ActiveRunID {
				return languageFromExecutionContext(item.ExecutionContext)
			}
		}
	}
	for idx := len(snapshot.Runs) - 1; idx >= 0; idx-- {
		if lang := languageFromExecutionContext(snapshot.Runs[idx].ExecutionContext); lang != "" {
			return lang
		}
	}
	return defaultDialogLanguage()
}

func languageFromExecutionContext(executionContext map[string]any) string {
	if executionContext == nil {
		return ""
	}
	if lang, ok := executionContext["dialogLanguage"].(string); ok {
		return languageNameFromLocale(lang)
	}
	return ""
}

func localizedWorkspaceProgressText(language string, key string) string {
	if prefersChinese(language) {
		switch key {
		case "no_active_run":
			return "工作空间尚无活跃运行（Run）"
		case "active_run":
			return "工作空间存在活跃运行（Run）投影"
		case "workspace_defined":
			return "工作空间（Workspace）已定义"
		case "runs":
			return "运行（Run）"
		case "results":
			return "结果（Result）"
		case "artifacts":
			return "交付物（Artifact）"
		case "context_summary":
			return "无活跃运行（Run）的工作空间上下文包"
		case "workspace":
			return "工作空间（Workspace）"
		case "task_progress":
			return "任务进度"
		}
	}
	switch key {
	case "no_active_run":
		return "workspace has no active Run"
	case "active_run":
		return "workspace has an active Run projection"
	case "workspace_defined":
		return "Workspace defined"
	case "runs":
		return "Runs"
	case "results":
		return "Results"
	case "artifacts":
		return "Artifacts"
	case "context_summary":
		return "workspace context bundle without active Run"
	case "workspace":
		return "Workspace"
	case "task_progress":
		return "Task progress"
	default:
		return key
	}
}
