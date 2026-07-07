package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	stepInputKeyExecutionContextPackage = "executionContextPackage"
	stepInputKeyEvidenceLedger          = "evidenceLedger"
	executionContextPackageVersion      = "workorch.execution_context.v1"
	executionContextPackageHistoryKey   = "workspaceHistoryChecked"
	stepInputMemorySelectionPolicy      = "step_input_dynamic_workspace_memory"
	stepInputMemorySelectionVersion     = "workorch.memory.step_input.v1"
	stepInputMemoryLimit                = 8
	stepInputHistoricalRunMemoryMax     = 2
)

func (o *RuntimeOrchestrator) enrichLLMStepInputContext(ctx context.Context, run *runModel.Run, step *runModel.Step, workspace *workspaceModel.Workspace) {
	if o == nil || run == nil || step == nil || !isLLMWorkspaceStepKind(step.Kind) {
		return
	}
	if step.Input == nil {
		step.Input = map[string]any{}
	}
	policy := o.RuntimePolicy()
	applyLLMOutputModeSelection(ctx, policy.LLMOutput, o.evidence.ArtifactContinuations, run, step.Input, nil, 0)
	pkg := runExecutionContextPackage(ctx, runtimeExecutionContextPackageDeps(o), run, step, workspace)
	step.Input[stepInputKeyExecutionContextPackage] = pkg
	if policyValues := llmOutputPolicyValues(policy); len(policyValues) > 0 {
		step.Input["llmOutputPolicy"] = policyValues
	}
	if timeline := mapSliceFromValue(pkg[stepInputKeyProcessEvidenceTimeline]); len(timeline) > 0 {
		step.Input[stepInputKeyProcessEvidenceTimeline] = timeline
	}
	if ledger := mapFromValue(pkg[stepInputKeyEvidenceLedger]); len(ledger) > 0 {
		step.Input[stepInputKeyEvidenceLedger] = ledger
	}
	if digest := mapFromValue(pkg[stepInputKeyCurrentProgressDigest]); len(digest) > 0 {
		step.Input[stepInputKeyCurrentProgressDigest] = digest
	}
	promoteRepairActionPackageContext(step.Input)
	realignLLMStepInputTargetSnapshotAuthority(run, step)
	_ = o.recordLLMStepInputMemoryRecallTrace(ctx, run, step, pkg)
}

func realignLLMStepInputTargetSnapshotAuthority(run *runModel.Run, step *runModel.Step) {
	if run == nil || step == nil || len(step.Input) == 0 {
		return
	}
	if dispatchInputUsesLocalAssessmentContract(run, step.Input) {
		return
	}
	contract := BuildConvergenceContract(run, step, nil, map[string]any{"input": step.Input}, nil)
	if contract.RuntimeStageMode != "target_snapshot_acquisition" {
		return
	}
	step.Input = ApplyConvergenceContract(step.Input, contract)
	step.Input = attachStagePacketToStepInput(run, step.ID, step.Input)
	step.Input = synchronizeCurrentRequiredOutputMirrors(step.Input)
	step.Input = attachCurrentActionContractToInput(run, step.ID, step.Input)
}

type executionContextPackageDeps struct {
	policy      RuntimePolicy
	workspaces  repository.WorkspaceRepository
	steps       repository.StepRepository
	runs        repository.RunRepository
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	approvals   repository.ApprovalRepository
	evidence    repository.EvidenceRecordRepository
	memoryItems repository.MemoryItemRepository
	graph       *graphSubsystem
	results     *resultSubsystem
}

func runtimeExecutionContextPackageDeps(o *RuntimeOrchestrator) executionContextPackageDeps {
	if o == nil {
		return executionContextPackageDeps{}
	}
	return executionContextPackageDeps{
		policy:      o.RuntimePolicy(),
		workspaces:  o.workspaces,
		steps:       o.steps,
		runs:        o.runs,
		tickets:     o.tickets,
		resolutions: o.resolutions,
		approvals:   o.approvals,
		evidence:    o.evidence.EvidenceRecords,
		memoryItems: o.evidence.MemoryItems,
		graph:       o.graphManager(),
		results:     o.resultManager(),
	}
}

func promoteRepairActionPackageContext(input map[string]any) {
	if len(input) == 0 {
		return
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		return
	}
	if digest := mapFromValue(input[stepInputKeyCurrentProgressDigest]); currentProgressDigestIsActionableFailure(digest) {
		progress := compactLLMContextMap(digest, 0)
		if currentProgressDigestShouldOverrideRepairPackage(digest) ||
			(len(mapFromValue(input["currentExecutionProgress"])) == 0 && len(mapFromValue(pkg["currentExecutionProgress"])) == 0) {
			input["currentExecutionProgress"] = progress
			pkg["currentExecutionProgress"] = progress
		}
		if currentFailure := currentFailureFromProgressDigest(digest); len(currentFailure) > 0 {
			if currentProgressDigestShouldOverrideRepairPackage(digest) ||
				(len(mapFromValue(input["currentFailure"])) == 0 && len(mapFromValue(pkg["currentFailure"])) == 0) {
				input["currentFailure"] = currentFailure
				pkg["currentFailure"] = currentFailure
			}
		}
		input["repairActionPackage"] = pkg
		return
	}
	if len(mapFromValue(input["currentFailure"])) == 0 {
		if currentFailure := mapFromValue(pkg["currentFailure"]); len(currentFailure) > 0 {
			input["currentFailure"] = compactLLMContextMap(currentFailure, 1)
		}
	}
	if digest := mapFromValue(input[stepInputKeyCurrentProgressDigest]); repairProgressDigestIsInformative(digest) &&
		len(mapFromValue(input["currentExecutionProgress"])) == 0 &&
		len(mapFromValue(pkg["currentExecutionProgress"])) == 0 {
		progress := compactLLMContextMap(digest, 0)
		input["currentExecutionProgress"] = progress
		pkg["currentExecutionProgress"] = progress
		input["repairActionPackage"] = pkg
		return
	}
	if len(mapFromValue(input["currentExecutionProgress"])) == 0 {
		if progress := mapFromValue(pkg["currentExecutionProgress"]); len(progress) > 0 {
			input["currentExecutionProgress"] = compactLLMContextMap(progress, 0)
		}
	}
}

func currentProgressDigestIsActionableFailure(digest map[string]any) bool {
	if len(digest) == 0 {
		return false
	}
	failure := mapFromValue(digest["lastObservedFailure"])
	if len(failure) == 0 {
		return false
	}
	switch strings.TrimSpace(stringFromMap(failure, "evidenceValidity")) {
	case "actionable_failure", "actionable_gap":
		return true
	case "stale_or_rejected", "diagnostic_only":
		return false
	}
	switch strings.TrimSpace(stringFromMap(failure, "failureClass")) {
	case failureClassShellCommand, failureClassPatchApply:
		return true
	default:
		return false
	}
}

func currentProgressDigestIsActionableGap(digest map[string]any) bool {
	failure := mapFromValue(digest["lastObservedFailure"])
	return strings.TrimSpace(stringFromMap(failure, "evidenceValidity")) == "actionable_gap"
}

func currentProgressDigestShouldOverrideRepairPackage(digest map[string]any) bool {
	if currentProgressDigestIsActionableGap(digest) {
		return true
	}
	failure := mapFromValue(digest["lastObservedFailure"])
	switch strings.TrimSpace(stringFromMap(failure, "failureClass")) {
	case failureClassShellCommand, failureClassPatchApply:
		return true
	default:
		return false
	}
}

func currentFailureFromProgressDigest(digest map[string]any) map[string]any {
	failure := mapFromValue(digest["lastObservedFailure"])
	if len(failure) == 0 {
		return nil
	}
	ret := map[string]any{}
	add := func(key string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			ret[key] = value
		}
	}
	add("failedStepId", stringFromMap(failure, "stepId"))
	add("failedStepKind", stringFromMap(failure, "kind"))
	add("failureClass", stringFromMap(failure, "failureClass"))
	add("failureSignature", stringFromMap(failure, "failureSignature"))
	add("reason", firstNonEmptyString(
		stringFromMap(failure, "concreteFailureDiagnostic"),
		stringFromMap(failure, "failureReason"),
		stringFromMap(digest, "currentGap"),
	))
	add("source", stringFromMap(failure, "source"))
	add("evidenceValidity", stringFromMap(failure, "evidenceValidity"))
	add("targetRef", stringFromMap(failure, "targetRef"))
	add("targetScope", firstNonEmptyString(stringFromMap(failure, "targetScope"), stringFromMap(failure, "failureTargetScope")))
	add("failureTargetScope", firstNonEmptyString(stringFromMap(failure, "failureTargetScope"), stringFromMap(failure, "targetScope")))
	if len(ret) == 0 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func runExecutionContextPackage(ctx context.Context, deps executionContextPackageDeps, run *runModel.Run, step *runModel.Step, workspace *workspaceModel.Workspace) map[string]any {
	if run == nil || step == nil {
		return nil
	}
	executionContext := cloneStepPayload(run.ExecutionContext)
	if executionContext == nil {
		executionContext = map[string]any{}
	}
	ret := map[string]any{
		"version":     executionContextPackageVersion,
		"workspaceId": string(run.WorkspaceID),
		"runId":       string(run.ID),
		"stepId":      string(step.ID),
		"stage":       firstNonEmptyString(stringFromMap(step.Input, stepInputKeyRuntimeStage), stringFromMap(step.Input, stepInputKeySoulStage)),
		"stageMode":   stringFromMap(step.Input, stepInputKeyRuntimeStageMode),
	}
	if policy := llmOutputPolicyValues(deps.policy); len(policy) > 0 {
		ret["llmOutputPolicy"] = policy
	}
	if mode := strings.TrimSpace(stringFromMap(step.Input, "outputMode")); mode != "" {
		ret["outputMode"] = mode
	}
	if selector := mapFromValue(step.Input["outputModeSelector"]); len(selector) > 0 {
		ret["outputModeSelector"] = selector
	}
	if contract := mapFromValue(step.Input["artifactOutputContract"]); len(contract) > 0 {
		ret["artifactOutputContract"] = contract
	}
	if continuation := mapFromValue(step.Input["artifactContinuationContext"]); len(continuation) > 0 {
		ret["artifactContinuationContext"] = continuation
	}
	mergeExecutionDefinitionsIntoPackage(ret, executionContext)
	if context := mapFromValue(executionContext[coverageWorkItemContextKey]); len(context) > 0 {
		ret[coverageWorkItemContextKey] = context
	}
	if baseline := mapFromValue(executionContext[gapBaselineKey]); len(baseline) > 0 {
		ret[gapBaselineKey] = baseline
	}
	if acceptance := runtimeAcceptanceSignalFromExecutionContext(executionContext); len(acceptance) > 0 {
		ret["acceptance"] = acceptance
	}
	if items := mapSliceFromValue(executionContext[coverageWorkItemsKey]); len(items) > 0 {
		ret[coverageWorkItemsKey] = items
	}
	stableContext := stableExecutionContextPackageFromStep(step)

	var (
		manifest           []map[string]any
		result             map[string]any
		deliverables       []map[string]any
		failures           []map[string]any
		processTimeline    []map[string]any
		evidenceLedger     map[string]any
		progressDigest     map[string]any
		currentGovernance  []map[string]any
		refreshedMemories  []map[string]any
		memoryRetrieval    map[string]any
		acceptedResultRead bool
		deliverablesRead   bool
		wg                 sync.WaitGroup
	)
	wg.Add(8)
	go func() {
		defer wg.Done()
		manifest = executionContextWorkspaceFileManifest(ctx, deps.workspaces, run, step, workspace)
	}()
	go func() {
		defer wg.Done()
		if stableContext.latestAcceptedResult != nil || stableContext.latestAcceptedResultChecked {
			result = stableContext.latestAcceptedResult
			return
		}
		if deps.results != nil {
			result = deps.results.latestAcceptedResultContext(ctx, run)
		}
		acceptedResultRead = true
	}()
	go func() {
		defer wg.Done()
		if len(stableContext.currentDeliverables) > 0 || stableContext.currentDeliverablesChecked {
			deliverables = stableContext.currentDeliverables
			return
		}
		if deps.results != nil {
			deliverables = deps.results.currentDeliverableContexts(ctx, run)
		}
		deliverablesRead = true
	}()
	go func() {
		defer wg.Done()
		if shouldReadRecentFailureHistory(step) {
			failures = recentFailureContexts(ctx, deps.steps, run, step.ID, 6)
		}
	}()
	go func() {
		defer wg.Done()
		if shouldReadProcessEvidenceTimeline(step) {
			if deps.graph != nil {
				processTimeline, progressDigest = deps.graph.currentRunProcessEvidenceContext(ctx, run, step.ID)
			}
		}
	}()
	go func() {
		defer wg.Done()
		evidenceLedger = runEvidenceLedgerContext(ctx, deps.evidence, run)
	}()
	go func() {
		defer wg.Done()
		currentGovernance = currentRunGovernanceHistory(ctx, deps.tickets, deps.resolutions, deps.approvals, run)
	}()
	go func() {
		defer wg.Done()
		refreshedMemories, memoryRetrieval = refreshedStepInputMemoryContext(ctx, deps.memoryItems, deps.runs, run, step)
	}()
	wg.Wait()

	if len(manifest) > 0 {
		ret["workspaceFileManifest"] = manifest
	}
	if result != nil {
		ret["latestAcceptedResult"] = result
	}
	if len(deliverables) > 0 {
		ret["currentDeliverables"] = deliverables
	}
	historyChecked := workspaceHistoryCheckedPackage(stableContext, acceptedResultRead, deliverablesRead)
	if len(historyChecked) > 0 {
		ret[executionContextPackageHistoryKey] = historyChecked
	}
	if len(failures) > 0 {
		ret["recentFailureSummary"] = failures
	}
	if len(processTimeline) > 0 {
		if shouldCompactProcessEvidenceForStateChangingRepair(step) {
			processTimeline = compactProcessEvidenceTimelineForStateChangingRepair(processTimeline, step)
			progressDigest = currentProgressDigestFromProcessEvidenceTimeline(processTimeline)
			if currentProgressDigest := currentProgressDigestFromCurrentExecutionProgress(step.Input, progressDigest); len(currentProgressDigest) > 0 {
				progressDigest = currentProgressDigest
			}
		}
		ret[stepInputKeyProcessEvidenceTimeline] = processTimeline
	}
	if len(evidenceLedger) > 0 {
		ret[stepInputKeyEvidenceLedger] = evidenceLedger
	}
	if len(progressDigest) > 0 {
		ret[stepInputKeyCurrentProgressDigest] = progressDigest
	}
	if len(processTimeline) == 0 {
		if inheritedTimeline, inheritedDigest := inheritedContinuationProcessEvidence(executionContext); len(inheritedTimeline) > 0 {
			ret[stepInputKeyProcessEvidenceTimeline] = inheritedTimeline
			if len(inheritedDigest) > 0 {
				ret[stepInputKeyCurrentProgressDigest] = inheritedDigest
			}
		}
	}
	selected := mergeSelectedContextRuntimeCapabilities(stableContext.selectedContext, selectedContextPackage(executionContext))
	if len(memoryRetrieval) > 0 {
		if selected == nil {
			selected = map[string]any{}
		}
		if len(refreshedMemories) > 0 {
			selected["memories"] = refreshedMemories
		} else {
			delete(selected, "memories")
		}
		selected["memoryRetrieval"] = memoryRetrieval
		selected = removeExcludedSelectedContextMemories(selected, memoryRetrieval)
	}
	if len(selected) > 0 {
		ret["selectedContext"] = selected
	}
	governanceHistory := mapSliceFromValue(executionContext["governanceHistory"])
	if len(currentGovernance) > 0 {
		governanceHistory = append(currentGovernance, governanceHistory...)
	}
	if len(governanceHistory) > 0 {
		ret["governanceHistory"] = compactGovernanceHistoryForRuntimeContext(governanceHistory, 6)
	}
	if shouldCompactExecutionContextPackageForRepair(step) {
		ret = compactExecutionContextPackageForRepair(ret, step, executionContext)
	}
	return ret
}

func currentProgressDigestFromCurrentExecutionProgress(input map[string]any, previous map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	progress := mapFromValue(input["currentExecutionProgress"])
	if len(progress) == 0 {
		progress = mapFromValue(mapFromValue(input["repairActionPackage"])["currentExecutionProgress"])
	}
	if len(progress) == 0 {
		return nil
	}
	for _, key := range []string{"latestActionEvidence", "completedGraphEvidence"} {
		for _, item := range mapSliceFromValue(progress[key]) {
			if !progressEvidenceItemCanPromoteCurrentFailure(item) {
				continue
			}
			output := mapFromValue(item["outputSummary"])
			if len(output) == 0 {
				output = mapFromValue(item["output"])
			}
			signals := mergeStructuredFailureSignals(
				structuredFailureSignalsFromFailureOutput(output, nil, "", ""),
				structuredFailureSignalsFromFailureOutput(item, nil, "", ""),
			)
			if len(output) == 0 || !structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
				continue
			}
			reason := truncateLLMContextString(firstNonEmptyString(
				stringFromMap(output, "stderrPreview"),
				stringFromMap(output, "stdoutPreview"),
				stringFromMap(output, "stderr"),
				stringFromMap(output, "stdout"),
				stringFromMap(output, "error"),
				stringFromMap(output, "reason"),
				stringFromMap(output, "summary"),
			))
			if strings.TrimSpace(reason) == "" {
				continue
			}
			ret := cloneStepPayload(previous)
			if ret == nil {
				ret = map[string]any{}
			}
			failure := map[string]any{
				"stepId":           stringFromMap(item, "stepId"),
				"kind":             stringFromMap(item, "kind"),
				"state":            stringFromMap(item, "state"),
				"failureReason":    reason,
				"evidenceValidity": "actionable_failure",
			}
			if failure["kind"] == "shell/command" {
				failure["failureClass"] = failureClassShellCommand
			} else if class := strings.TrimSpace(stringFromMap(progress, "failureClass")); class != "" {
				failure["failureClass"] = class
			}
			if source := strings.TrimSpace(stringFromMap(item, "source")); source != "" {
				failure["source"] = source
			}
			if nodeID := strings.TrimSpace(stringFromMap(item, "graphNodeId")); nodeID != "" {
				failure["graphNodeId"] = nodeID
			}
			if targetRef := firstNonEmptyString(
				stringFromMap(item, "targetRef"),
				stringFromMap(output, "targetRef"),
			); targetRef != "" {
				failure["targetRef"] = targetRef
			}
			if targetScope := firstNonEmptyString(
				stringFromMap(item, "failureTargetScope"),
				stringFromMap(item, "targetScope"),
				stringFromMap(output, "failureTargetScope"),
				stringFromMap(output, "targetScope"),
				stringFromMap(progress, "failureTargetScope"),
				stringFromMap(progress, "targetScope"),
			); targetScope != "" {
				failure["targetScope"] = targetScope
				failure["failureTargetScope"] = targetScope
			}
			ret["lastObservedFailure"] = failure
			ret["currentGap"] = reason
			ret["nextRequiredAction"] = "address_last_failure_under_current_stage_contract"
			return ret
		}
	}
	return nil
}

func progressEvidenceItemCanPromoteCurrentFailure(item map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromMap(item, "source")) == "latest_successful_action_evidence" {
		return false
	}
	switch strings.TrimSpace(stringFromMap(item, "state")) {
	case string(common.StepStateFailed):
		return true
	case string(common.StepStateSucceeded):
		if strings.TrimSpace(stringFromMap(item, "evidenceValidity")) == "actionable_failure" {
			return true
		}
		output := mapFromValue(item["outputSummary"])
		if len(output) == 0 {
			output = mapFromValue(item["output"])
		}
		return completedProgressEvidenceOutputHasFailureSignal(output)
	default:
		return false
	}
}

func completedProgressEvidenceOutputHasFailureSignal(output map[string]any) bool {
	if len(output) == 0 {
		return false
	}
	return completedShellCommandOutputHasFailureSignal(&runModel.Step{
		Kind:   "shell/command",
		State:  common.StepStateSucceeded,
		Output: output,
	})
}

func removeExcludedSelectedContextMemories(selected map[string]any, retrieval map[string]any) map[string]any {
	if len(selected) == 0 || len(retrieval) == 0 {
		return selected
	}
	selectedIDs := map[string]bool{}
	for _, id := range stringSliceFromValue(retrieval["selectedMemoryIds"]) {
		id = strings.TrimSpace(id)
		if id != "" {
			selectedIDs[id] = true
		}
	}
	excludedIDs := map[string]bool{}
	for _, id := range stringSliceFromValue(retrieval["excludedMemoryIds"]) {
		id = strings.TrimSpace(id)
		if id != "" {
			excludedIDs[id] = true
		}
	}
	if len(selectedIDs) == 0 {
		delete(selected, "memories")
		return selected
	}
	memories := mapSliceFromValue(selected["memories"])
	if len(memories) == 0 {
		delete(selected, "memories")
		return selected
	}
	filtered := make([]map[string]any, 0, len(memories))
	for _, memory := range memories {
		id := strings.TrimSpace(firstNonEmptyString(stringFromMap(memory, "id"), stringFromMap(memory, "ID")))
		if id == "" || !selectedIDs[id] || excludedIDs[id] {
			continue
		}
		filtered = append(filtered, memory)
	}
	if len(filtered) == 0 {
		delete(selected, "memories")
		return selected
	}
	selected["memories"] = filtered
	return selected
}

func runEvidenceLedgerContext(ctx context.Context, records repository.EvidenceRecordRepository, run *runModel.Run) map[string]any {
	if records == nil || run == nil || run.ID == "" {
		return nil
	}
	items, err := records.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID), run.ID)
	if err != nil || len(items) == 0 {
		return nil
	}
	return compactEvidenceLedgerForLLMContext(run, items)
}

func compactEvidenceLedgerForLLMContext(run *runModel.Run, records []*orchestratormodel.EvidenceRecord) map[string]any {
	if len(records) == 0 {
		return nil
	}
	selected := records
	var historical map[string]any
	if len(records) > maxLLMContextSliceItems {
		recentLimit := maxLLMContextSliceItems - 1
		historical = compactHistoricalEvidenceRecordSummary(run, records[:len(records)-recentLimit])
		selected = records[len(records)-recentLimit:]
	}
	items := make([]map[string]any, 0, min(len(selected)+1, maxLLMContextSliceItems))
	if len(historical) > 0 {
		historical["order"] = 1
		items = append(items, historical)
	}
	statusCounts := map[string]int{}
	kindCounts := map[string]int{}
	for _, record := range records {
		if record == nil {
			continue
		}
		if status := strings.TrimSpace(record.Status); status != "" {
			statusCounts[status]++
		}
		if kind := strings.TrimSpace(record.Kind); kind != "" {
			kindCounts[kind]++
		}
	}
	offset := len(items)
	for idx, record := range selected {
		item := compactEvidenceRecordForLLMContext(record, idx+1+offset)
		if len(item) > 0 {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil
	}
	ledger := map[string]any{
		"schemaVersion": "workorch.evidence_ledger_context.v1",
		"summary": map[string]any{
			"totalCount":   len(records),
			"statusCounts": statusCounts,
			"kindCounts":   kindCounts,
		},
		"items":            items,
		"compactionPolicy": "recent_complete_evidence_with_historical_summary_refs; raw payloads omitted and addressable through payloadRef/sourceRef",
	}
	if run != nil {
		if run.ID != "" {
			ledger["runId"] = string(run.ID)
		}
		if run.WorkspaceID != "" {
			ledger["workspaceId"] = string(run.WorkspaceID)
		}
		if revision := strings.TrimSpace(run.DefinitionRevision); revision != "" {
			ledger["definitionRevision"] = revision
		}
	}
	return ledger
}

func compactHistoricalEvidenceRecordSummary(run *runModel.Run, records []*orchestratormodel.EvidenceRecord) map[string]any {
	if len(records) == 0 {
		return nil
	}
	summary := map[string]any{
		"sourceRef":      evidenceLedgerHistorySourceRef(run),
		"summaryKind":    "compact_history",
		"historicalOnly": true,
		"itemCount":      len(records),
	}
	statusCounts := map[string]int{}
	kindCounts := map[string]int{}
	sourceRefs := []string{}
	evidenceRefs := []string{}
	var firstRef, lastRef string
	for _, record := range records {
		if record == nil {
			continue
		}
		ref := evidenceRecordPromptRef(record)
		if ref != "" {
			if firstRef == "" {
				firstRef = ref
			}
			lastRef = ref
			evidenceRefs = append(evidenceRefs, ref)
		}
		if source := strings.TrimSpace(record.SourceRef); source != "" {
			sourceRefs = append(sourceRefs, source)
		}
		if status := strings.TrimSpace(record.Status); status != "" {
			statusCounts[status]++
		}
		if kind := strings.TrimSpace(record.Kind); kind != "" {
			kindCounts[kind]++
		}
	}
	if firstRef != "" {
		summary["firstEvidenceRef"] = firstRef
	}
	if lastRef != "" {
		summary["lastEvidenceRef"] = lastRef
	}
	if refs := limitedStrings(compactUniqueStrings(evidenceRefs), maxLLMContextSliceItems); len(refs) > 0 {
		summary["evidenceRefs"] = refs
	}
	if refs := limitedStrings(compactUniqueStrings(sourceRefs), maxLLMContextSliceItems); len(refs) > 0 {
		summary["sourceRefs"] = refs
	}
	if len(statusCounts) > 0 {
		summary["statusCounts"] = statusCounts
	}
	if len(kindCounts) > 0 {
		summary["kindCounts"] = kindCounts
	}
	summary["summary"] = "Older evidence records are compacted; use evidenceRefs/sourceRefs for lineage and recent items for proof strength details."
	return summary
}

func evidenceLedgerHistorySourceRef(run *runModel.Run) string {
	if run == nil || run.ID == "" {
		return "history://run/evidence-ledger"
	}
	return "history://runs/" + string(run.ID) + "/evidence-ledger"
}

func compactEvidenceRecordForLLMContext(record *orchestratormodel.EvidenceRecord, order int) map[string]any {
	if record == nil || record.ID == "" {
		return nil
	}
	item := map[string]any{
		"order":       order,
		"evidenceRef": evidenceRecordPromptRef(record),
		"kind":        strings.TrimSpace(record.Kind),
		"status":      strings.TrimSpace(record.Status),
		"sourceRef":   strings.TrimSpace(record.SourceRef),
	}
	if record.StepID != "" {
		item["stepId"] = string(record.StepID)
	}
	if revision := strings.TrimSpace(record.DefinitionRevision); revision != "" {
		item["definitionRevision"] = revision
	}
	if sourceType := strings.TrimSpace(record.SourceType); sourceType != "" {
		item["sourceType"] = sourceType
	}
	if subjectID := strings.TrimSpace(record.SubjectID); subjectID != "" {
		item["subjectId"] = subjectID
	}
	if refs := compactUniqueStrings(record.CoverageItemRefs); len(refs) > 0 {
		item["coverageItemRefs"] = refs
	}
	if len(record.Supersedes) > 0 {
		refs := make([]string, 0, len(record.Supersedes))
		for _, ref := range record.Supersedes {
			if strings.TrimSpace(string(ref)) != "" {
				refs = append(refs, string(ref))
			}
		}
		if refs := compactUniqueStrings(refs); len(refs) > 0 {
			item["supersedes"] = refs
		}
	}
	if record.CreatedAt != "" {
		item["createdAt"] = record.CreatedAt
	}
	if record.UpdatedAt != "" && record.UpdatedAt != record.CreatedAt {
		item["updatedAt"] = record.UpdatedAt
	}
	mergeEvidenceRecordPromptMetadata(item, record.Metadata)
	return item
}

func evidenceRecordPromptRef(record *orchestratormodel.EvidenceRecord) string {
	if record == nil || record.ID == "" {
		return ""
	}
	return "evidence://" + string(record.ID)
}

func mergeEvidenceRecordPromptMetadata(item map[string]any, metadata map[string]any) {
	if len(item) == 0 || len(metadata) == 0 {
		return
	}
	for _, key := range []string{
		"evidenceRef",
		"evidenceQuality",
		"evidenceReason",
		"validationKind",
		"validationState",
		"proofKind",
		"coverageAlignment",
		"targetRef",
		"targetRefs",
		"coverageTargets",
		"coverageWorkItemIds",
		"observedClaims",
		"limitations",
		"evidenceTags",
		"zeroWork",
		"summary",
		"reason",
		"failureClass",
		"failureSignature",
		"violatedRule",
		"violationType",
		"recoverability",
		"retryHint",
		"targetScope",
		"graphRootStepId",
		"graphNodeId",
		"patchType",
		"beforeHash",
		"afterHash",
		"contentHash",
		"snapshotIntegrity",
		"snapshotValidity",
	} {
		if value, ok := metadata[key]; ok && value != nil {
			item[key] = compactLLMContextValue(value, 1)
		}
	}
	for _, key := range []string{"sourceOutputRef", "rejectedOutputRef"} {
		if ref := strings.TrimSpace(stringFromMap(metadata, key)); ref != "" {
			item["payloadRef"] = ref
			break
		}
	}
}

func shouldCompactExecutionContextPackageForRepair(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if len(mapFromValue(step.Input["repairActionPackage"])) == 0 {
		return false
	}
	return runtimeStageFromStepInput(step.Input) == runtimeStageRepair ||
		boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) ||
		boolValue(step.Input["stateChangingOutputRequired"])
}

func compactExecutionContextPackageForRepair(pkg map[string]any, step *runModel.Step, executionContext map[string]any) map[string]any {
	if len(pkg) == 0 {
		return pkg
	}
	focus := repairCoverageFocusFromInput(step.Input)
	if len(focus) == 0 {
		focus = mapFromValue(mapFromValue(pkg["acceptance"])["currentFocus"])
	}
	if len(focus) > 0 {
		if context := mapFromValue(pkg[coverageWorkItemContextKey]); len(context) > 0 {
			pkg[coverageWorkItemContextKey] = compactCoverageWorkItemContextForRepairFocus(context, focus)
		}
		if items := mapSliceFromValue(pkg[coverageWorkItemsKey]); len(items) > 0 {
			pkg[coverageWorkItemsKey] = compactCoverageWorkItemsForRepairFocus(items, focus, 8)
		}
	}
	if selected := mapFromValue(pkg["selectedContext"]); len(selected) > 0 {
		revision := coverageDefinitionRevision(executionContext)
		selected = compactSelectedContextForCurrentDefinition(selected, revision)
		selected = compactSelectedContextForTargetSnapshotBoundary(selected, step.Input)
		if len(selected) == 0 {
			delete(pkg, "selectedContext")
		} else {
			pkg["selectedContext"] = selected
		}
	}
	return pkg
}

func compactCoverageWorkItemContextForRepairFocus(context map[string]any, focus map[string]any) map[string]any {
	if len(context) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{"schemaVersion", "definitionRevision", "summary", "omittedCount", "excludedCount", "historicalCount", "omittedStatusDistribution", "omittedSourceScopeDistribution"} {
		if value, ok := context[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if current := mapFromValue(context["currentFocus"]); len(current) > 0 {
		ret["currentFocus"] = compactCoverageWorkItemForRepairFocus(current, focus, coverageWorkItemMatchesFocus(current, focus))
	}
	if currentItems := compactCoverageWorkItemsForRepairFocus(mapSliceFromValue(context["currentFocusItems"]), focus, 4); len(currentItems) > 0 {
		ret["currentFocusItems"] = currentItems
	}
	if unresolved := compactCoverageWorkItemsForRepairFocus(mapSliceFromValue(context["unresolvedItems"]), focus, 8); len(unresolved) > 0 {
		ret["unresolvedItems"] = unresolved
	}
	if ledger := compactCoverageWorkItemsForRepairFocus(mapSliceFromValue(context["ledgerSummary"]), focus, 8); len(ledger) > 0 {
		ret["ledgerSummary"] = ledger
	}
	if historical := compactCoverageWorkItemsForRepairFocus(mapSliceFromValue(context["historicalItems"]), focus, 4); len(historical) > 0 {
		ret["historicalItems"] = historical
	}
	ret["compactionPolicy"] = "current focus keeps evidence refs; non-focus items keep summary counts and omit rejected evidence details"
	return ret
}

func compactCoverageWorkItemsForRepairFocus(items []map[string]any, focus map[string]any, limit int) []map[string]any {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	ret := make([]map[string]any, 0, min(len(items), limit))
	added := map[string]bool{}
	add := func(item map[string]any) {
		if len(item) == 0 || len(ret) >= limit {
			return
		}
		key := firstNonEmptyString(stringFromMap(item, "id"), stringFromMap(item, "normalizedSourceKey"), stringFromMap(item, "targetText"))
		if key != "" && added[key] {
			return
		}
		if key != "" {
			added[key] = true
		}
		ret = append(ret, compactCoverageWorkItemForRepairFocus(item, focus, coverageWorkItemMatchesFocus(item, focus)))
	}
	for _, item := range items {
		if coverageWorkItemMatchesFocus(item, focus) {
			add(item)
		}
	}
	for _, item := range items {
		add(item)
	}
	return ret
}

func compactCoverageWorkItemForRepairFocus(item map[string]any, focus map[string]any, current bool) map[string]any {
	if len(item) == 0 {
		return nil
	}
	if current {
		ret := compactLLMContextMap(item, 0)
		ret["focusScoped"] = true
		return ret
	}
	ret := map[string]any{}
	for _, key := range []string{
		"id",
		"definitionRevision",
		"source",
		"sourceScope",
		"sourceRequirementId",
		"sourcePath",
		"normalizedSourceKey",
		"coverageWorkItemIds",
		"structuredTargetRefs",
		"targetRefs",
		"targetText",
		"status",
		"currentGap",
		"requiredEvidenceKinds",
		"missingEvidenceKinds",
		"evidenceRole",
		"legacyEvidenceDemotions",
	} {
		if value, ok := item[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if count := len(stringSliceFromValue(item["acceptedEvidenceRefs"])); count > 0 {
		ret["acceptedEvidenceCount"] = count
	}
	if count := len(stringSliceFromValue(item["rejectedEvidenceRefs"])); count > 0 {
		ret["rejectedEvidenceCount"] = count
	}
	if byKind := mapStringSliceValue(item["acceptedEvidenceByKind"]); len(byKind) > 0 {
		ret["acceptedEvidenceByKindCount"] = mapStringSliceCounts(byKind)
	}
	if byKind := mapStringSliceValue(item["acceptedEvidenceBindingByKind"]); len(byKind) > 0 {
		ret["acceptedEvidenceBindingByKindCount"] = mapStringSliceCounts(byKind)
	}
	if byKind := mapStringSliceValue(item["supportingEvidenceByKind"]); len(byKind) > 0 {
		ret["supportingEvidenceByKindCount"] = mapStringSliceCounts(byKind)
	}
	if byKind := mapStringSliceValue(item["rejectedEvidenceByKind"]); len(byKind) > 0 {
		ret["rejectedEvidenceByKindCount"] = mapStringSliceCounts(byKind)
	}
	if len(ret) > 0 {
		ret["focusScoped"] = false
		ret["evidenceDetailOmitted"] = true
	}
	return ret
}

func coverageWorkItemMatchesFocus(item map[string]any, focus map[string]any) bool {
	if len(item) == 0 || len(focus) == 0 {
		return false
	}
	focusIDs := stringSet(stringSliceFromValue(focus["coverageWorkItemIds"]))
	if id := strings.TrimSpace(stringFromMap(item, "id")); id != "" && focusIDs[id] {
		return true
	}
	if id := strings.TrimSpace(stringFromMap(item, "coverageWorkItemId")); id != "" && focusIDs[id] {
		return true
	}
	for _, focusRef := range stringSliceFromValue(focus["targetRefs"]) {
		for _, key := range []string{"targetRef", "sourcePath", "targetText", "normalizedSourceKey", "sourceRequirementId"} {
			if coverageRepairTokenMatches(focusRef, stringFromMap(item, key)) {
				return true
			}
		}
	}
	for id := range focusIDs {
		for _, key := range []string{"coverageWorkItemId", "id", "sourceRequirementId", "normalizedSourceKey"} {
			if coverageRepairTokenMatches(id, stringFromMap(item, key)) {
				return true
			}
		}
	}
	return false
}

func mapStringSliceCounts(input map[string][]string) map[string]int {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]int{}
	for key, values := range input {
		if strings.TrimSpace(key) == "" || len(values) == 0 {
			continue
		}
		ret[key] = len(values)
	}
	return ret
}

func compactSelectedContextForCurrentDefinition(selected map[string]any, currentRevision string) map[string]any {
	if len(selected) == 0 {
		return nil
	}
	ret := cloneStepPayload(selected)
	memories := mapSliceFromValue(ret["memories"])
	if len(memories) == 0 {
		return ret
	}
	filtered := make([]map[string]any, 0, min(len(memories), stepInputMemoryLimit))
	omitted := 0
	for idx, memory := range memories {
		if !selectedMemoryCompatibleWithCurrentDefinition(memory, currentRevision) {
			omitted++
			continue
		}
		filtered = append(filtered, compactSelectedMemoryForRepair(memory))
		if len(filtered) >= stepInputMemoryLimit {
			omitted += len(memories) - idx - 1
			break
		}
	}
	if len(filtered) == 0 {
		delete(ret, "memories")
	} else {
		ret["memories"] = filtered
	}
	if omitted > 0 || strings.TrimSpace(currentRevision) != "" {
		ret["memoryCompaction"] = map[string]any{
			"policy":             "current_definition_high_value_only",
			"definitionRevision": strings.TrimSpace(currentRevision),
			"omittedCount":       omitted,
		}
	}
	return ret
}

func mergeSelectedContextRuntimeCapabilities(stable map[string]any, current map[string]any) map[string]any {
	ret := map[string]any{}
	if len(stable) > 0 {
		ret = cloneStepPayload(stable)
	}
	if len(current) == 0 {
		if len(ret) == 0 {
			return nil
		}
		return ret
	}
	if len(ret) == 0 {
		return cloneStepPayload(current)
	}
	for _, key := range []string{"skills", "mcpServers", "mcpCapabilities", "capabilityIds"} {
		if value, ok := current[key]; ok && value != nil {
			ret[key] = cloneLLMContextValue(value)
		}
	}
	for _, key := range []string{"memories", "memoryRetrieval"} {
		if _, ok := ret[key]; ok {
			continue
		}
		if value, ok := current[key]; ok && value != nil {
			ret[key] = cloneLLMContextValue(value)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func selectedMemoryCompatibleWithCurrentDefinition(memory map[string]any, currentRevision string) bool {
	if len(memory) == 0 {
		return false
	}
	if selectedMemoryMarkedStale(memory) || selectedMemoryLowValue(memory) {
		return false
	}
	revision := selectedMemoryDefinitionRevision(memory)
	currentRevision = strings.TrimSpace(currentRevision)
	if revision != "" && currentRevision != "" && revision != currentRevision {
		return false
	}
	if value, ok := memory["currentDefinitionCompatible"]; ok && !boolValue(value) {
		return false
	}
	return strings.TrimSpace(firstNonEmptyString(stringFromMap(memory, "summary"), stringFromMap(memory, "content"))) != ""
}

func compactSelectedMemoryForRepair(memory map[string]any) map[string]any {
	ret := map[string]any{}
	for _, key := range []string{
		"id",
		"memoryType",
		"summary",
		"content",
		"tags",
		"importance",
		"confidence",
		"visibility",
		"vectorScore",
		"definitionRevision",
		"goalDoDRevision",
		"sourceRef",
		"runId",
	} {
		if value, ok := memory[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if provenance := staticMemoryContextProvenance(memory); provenance != "" {
		ret["provenance"] = provenance
	}
	return ret
}

func compactSelectedContextForTargetSnapshotBoundary(selected map[string]any, input map[string]any) map[string]any {
	if len(selected) == 0 || len(input) == 0 {
		return selected
	}
	if strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind")) != orchestratormodel.NextActionTypeStateChange {
		return selected
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if len(exactPatchSafeRepairTargetSnapshots(mergeRepairTargetSnapshots(mapSliceFromValue(pkg["targetSnapshots"]), repairActionTargetSnapshots(input)))) == 0 {
		return selected
	}
	memories := mapSliceFromValue(selected["memories"])
	if len(memories) == 0 {
		return selected
	}
	filtered := make([]map[string]any, 0, len(memories))
	omitted := 0
	for _, memory := range memories {
		if selectedMemoryReadOnlyGraphNoise(memory) && !selectedMemoryHasTargetSnapshotProvenance(memory) {
			omitted++
			continue
		}
		filtered = append(filtered, memory)
	}
	next := cloneStepPayload(selected)
	if len(filtered) == 0 {
		delete(next, "memories")
	} else {
		next["memories"] = filtered
	}
	if omitted > 0 {
		compaction := mapFromValue(next["memoryCompaction"])
		if compaction == nil {
			compaction = map[string]any{}
		}
		compaction["targetSnapshotBoundaryPolicy"] = "exclude_read_only_graph_noise_without_target_snapshot_provenance"
		compaction["omittedReadOnlyGraphMemoryCount"] = omitted
		next["memoryCompaction"] = compaction
	}
	return next
}

func selectedMemoryReadOnlyGraphNoise(memory map[string]any) bool {
	if len(memory) == 0 {
		return false
	}
	for _, key := range []string{"kind", "memoryType", "evidenceKind", "violatedRule", "violationType", "summary", "content"} {
		value := strings.ToLower(strings.TrimSpace(stringFromMap(memory, key)))
		if value == "" {
			continue
		}
		if strings.Contains(value, "read_only_graph") ||
			strings.Contains(value, "same_graph_file_read_before_patch") ||
			strings.Contains(value, "file/read") && strings.Contains(value, "patch/apply") {
			return true
		}
	}
	for _, tag := range stringSliceFromValue(memory["tags"]) {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "read_only_graph" || tag == "same_graph_file_read_before_patch" {
			return true
		}
	}
	return false
}

func selectedMemoryHasTargetSnapshotProvenance(memory map[string]any) bool {
	if len(memory) == 0 {
		return false
	}
	for _, source := range []map[string]any{memory, mapFromValue(memory["metadata"]), mapFromValue(memory["context"])} {
		if strings.TrimSpace(stringFromMap(source, "targetSnapshotRef")) != "" ||
			strings.TrimSpace(stringFromMap(source, "targetSnapshotEvidenceRef")) != "" ||
			strings.TrimSpace(stringFromMap(source, "evidenceRef")) != "" && strings.TrimSpace(stringFromMap(source, "targetRef")) != "" && strings.TrimSpace(stringFromMap(source, "contentHash")) != "" {
			return true
		}
		if strings.TrimSpace(stringFromMap(source, "evidenceKind")) == orchestratormodel.EvidenceKindTargetSnapshot ||
			strings.TrimSpace(stringFromMap(source, "kind")) == orchestratormodel.EvidenceKindTargetSnapshot {
			return true
		}
		for _, tag := range stringSliceFromValue(source["tags"]) {
			if strings.TrimSpace(tag) == orchestratormodel.EvidenceKindTargetSnapshot {
				return true
			}
		}
	}
	return false
}

func selectedMemoryDefinitionRevision(memory map[string]any) string {
	for _, source := range []map[string]any{memory, mapFromValue(memory["metadata"]), mapFromValue(memory["context"])} {
		for _, key := range []string{"definitionRevision", "goalDoDRevision", "goalRevision", "doDRevision", "revision"} {
			if value := strings.TrimSpace(stringFromMap(source, key)); value != "" {
				return value
			}
		}
	}
	return ""
}

func selectedMemoryMarkedStale(memory map[string]any) bool {
	for _, key := range []string{"status", "state", "validity", "evidenceValidity", "cacheability"} {
		switch strings.TrimSpace(strings.ToLower(stringFromMap(memory, key))) {
		case "stale", "stale_or_rejected", "rejected", "superseded", "obsolete", "expired", "diagnostic_only", "provider_retry_noise":
			return true
		}
	}
	return false
}

func selectedMemoryLowValue(memory map[string]any) bool {
	confidence := floatFromValue(memory["confidence"])
	importance := floatFromValue(memory["importance"])
	if confidence > 0 && confidence < 0.2 {
		return true
	}
	if importance > 0 && importance < 0.2 {
		return true
	}
	return false
}

func shouldCompactProcessEvidenceForStateChangingRepair(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	return boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) &&
		boolValue(step.Input["stateChangingOutputRequired"]) &&
		len(mapFromValue(step.Input["repairActionPackage"])) > 0
}

func compactProcessEvidenceTimelineForStateChangingRepair(input []map[string]any, step *runModel.Step) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	targetStepID := currentFailureStepIDFromRepairStep(step)
	selected := selectProcessEvidenceForStateChangingRepair(input, targetStepID)
	if len(selected) == 0 {
		selected = input
	}
	if len(selected) > 8 {
		selected = selected[len(selected)-8:]
	}
	ret := make([]map[string]any, 0, len(selected))
	for _, item := range selected {
		if len(item) == 0 {
			continue
		}
		next := map[string]any{}
		for _, key := range []string{
			"order",
			"sourceRef",
			"stepId",
			"kind",
			"actionKind",
			"state",
			"stage",
			"graphNodeId",
			"graphRootStepId",
			"summary",
			"inputSummary",
			"failureClass",
			"failureSignature",
			"failureReason",
			"evidenceValidity",
			"outputRef",
			"payloadRef",
		} {
			if value, ok := item[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
		}
		if refs := stringSliceFromValue(item["evidenceRefs"]); len(refs) > 0 {
			next["evidenceRefs"] = refs
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
	}
	return ret
}

func currentFailureStepIDFromRepairStep(step *runModel.Step) common.StepID {
	if step == nil || step.Input == nil {
		return ""
	}
	input := step.Input
	candidates := []string{
		stringFromMap(input, "failedStepId"),
		stringFromMap(input, stepInputKeyRepairOfStepID),
		stringFromMap(input, stepInputKeyRepairOfRootStepID),
	}
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		candidates = append(candidates,
			stringFromMap(pkg, "failedStepId"),
			stringFromMap(pkg, stepInputKeyRepairOfStepID),
			stringFromMap(pkg, stepInputKeyRepairOfRootStepID),
		)
	}
	if progress := mapFromValue(input["currentExecutionProgress"]); len(progress) > 0 {
		candidates = append(candidates,
			stringFromMap(progress, "failedStepId"),
			stringFromMap(progress, "currentFailedStepId"),
			stringFromMap(progress, "latestFailedStepId"),
		)
	}
	return common.StepID(strings.TrimSpace(firstNonEmptyString(candidates...)))
}

func selectProcessEvidenceForStateChangingRepair(input []map[string]any, targetStepID common.StepID) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	target := strings.TrimSpace(string(targetStepID))
	target = processEvidenceActionableRepairTarget(input, target)
	selected := make([]map[string]any, 0, min(len(input), 8))
	seen := map[string]bool{}
	add := func(item map[string]any) {
		if len(item) == 0 {
			return
		}
		if target != "" && strings.TrimSpace(stringFromMap(item, "state")) == string(common.StepStateFailed) && !processEvidenceMatchesRepairTarget(item, target) {
			return
		}
		key := strings.TrimSpace(firstNonEmptyString(
			stringFromMap(item, "sourceRef"),
			stringFromMap(item, "stepId"),
			fmt.Sprintf("%v", item["order"]),
		))
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		selected = append(selected, item)
	}
	if target != "" {
		for idx := len(input) - 1; idx >= 0; idx-- {
			item := input[idx]
			if strings.TrimSpace(stringFromMap(item, "stepId")) == target {
				add(item)
			}
		}
		for idx := len(input) - 1; idx >= 0; idx-- {
			item := input[idx]
			if strings.TrimSpace(stringFromMap(item, "graphRootStepId")) == target {
				add(item)
			}
		}
	}
	for idx := len(input) - 1; idx >= 0 && len(selected) < 4; idx-- {
		item := input[idx]
		state := strings.TrimSpace(stringFromMap(item, "state"))
		actionKind := strings.TrimSpace(stringFromMap(item, "actionKind"))
		if state == string(common.StepStateSucceeded) && actionKind != "" && actionKind != "runtime" {
			add(item)
		}
	}
	for idx := len(input) - 1; idx >= 0 && len(selected) < 6; idx-- {
		item := input[idx]
		if strings.TrimSpace(stringFromMap(item, "state")) == string(common.StepStateFailed) {
			add(item)
		}
	}
	for idx := len(input) - 1; idx >= 0 && len(selected) < 8; idx-- {
		add(input[idx])
	}
	sort.Slice(selected, func(i, j int) bool {
		return compareProcessEvidenceOrder(selected[i], selected[j]) < 0
	})
	return selected
}

func processEvidenceActionableRepairTarget(input []map[string]any, target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return target
	}
	for _, item := range input {
		if !processEvidenceMatchesRepairTarget(item, target) {
			continue
		}
		if processEvidenceItemIsActionableFailure(item) {
			return target
		}
	}
	for idx := len(input) - 1; idx >= 0; idx-- {
		item := input[idx]
		if !processEvidenceItemIsActionableFailure(item) {
			continue
		}
		if stepID := strings.TrimSpace(stringFromMap(item, "stepId")); stepID != "" {
			return stepID
		}
	}
	return target
}

func processEvidenceMatchesRepairTarget(item map[string]any, target string) bool {
	if len(item) == 0 || strings.TrimSpace(target) == "" {
		return false
	}
	return strings.TrimSpace(stringFromMap(item, "stepId")) == target ||
		strings.TrimSpace(stringFromMap(item, "graphRootStepId")) == target
}

func compareProcessEvidenceOrder(left, right map[string]any) int {
	leftOrder := intFromAny(left["order"])
	rightOrder := intFromAny(right["order"])
	switch {
	case leftOrder < rightOrder:
		return -1
	case leftOrder > rightOrder:
		return 1
	}
	leftStep := strings.TrimSpace(stringFromMap(left, "stepId"))
	rightStep := strings.TrimSpace(stringFromMap(right, "stepId"))
	switch {
	case leftStep < rightStep:
		return -1
	case leftStep > rightStep:
		return 1
	default:
		return 0
	}
}

func mergeExecutionDefinitionsIntoPackage(pkg map[string]any, executionContext map[string]any) {
	if len(pkg) == 0 || len(executionContext) == 0 {
		return
	}
	for _, key := range []string{
		"goalId",
		"doDId",
		"goalDefinitionPolicy",
		"doDDefinitionPolicy",
	} {
		if value, ok := executionContext[key]; ok && value != nil {
			pkg[key] = compactLLMContextValue(value, 1)
		}
	}
	if value := mapFromValue(executionContext["rejectedResultContinuation"]); len(value) > 0 {
		pkg["rejectedResultContinuation"] = compactRejectedResultContinuationForRuntimeContext(value)
	}
	if value := mapFromValue(executionContext["replanContinuation"]); len(value) > 0 {
		if len(mapFromValue(executionContext["rejectedResultContinuation"])) > 0 &&
			!replanContinuationCarriesActionableFailure(executionContext) {
			pkg["replanContinuation"] = compactHistoricalReplanContinuationForRuntimeContext(value)
		} else {
			pkg["replanContinuation"] = compactReplanContinuationForRuntimeContext(value)
		}
	}
	if value, ok := executionContext["executionContract"]; ok && value != nil {
		pkg["executionContract"] = cloneLLMContextValue(value)
	}
	if value, ok := executionContext["definitionChangeImpact"]; ok && value != nil {
		pkg["definitionChangeImpact"] = compactLLMContextValue(value, 1)
	}
	for _, key := range []string{
		"goal",
		"goals",
		"dod",
		"dods",
	} {
		if value, ok := executionContext[key]; ok && value != nil {
			pkg[key] = compactLLMContextValue(value, 1)
		}
	}
}

func cloneLLMContextValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		ret := make(map[string]any, len(typed))
		for key, item := range typed {
			ret[key] = cloneLLMContextValue(item)
		}
		return ret
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			cloned := cloneLLMContextValue(item)
			if mapped, ok := cloned.(map[string]any); ok {
				ret = append(ret, mapped)
			}
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneLLMContextValue(item))
		}
		return ret
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func refreshedStepInputMemoryContext(
	ctx context.Context,
	memories repository.MemoryItemRepository,
	runs repository.RunRepository,
	run *runModel.Run,
	step *runModel.Step,
) ([]map[string]any, map[string]any) {
	if memories == nil || run == nil || run.WorkspaceID == "" {
		return nil, nil
	}
	items, err := memories.ListByWorkspaceID(ctx, run.WorkspaceID)
	runStates := stepInputMemoryRunStates(ctx, runs, run.WorkspaceID)
	refreshSource := stepInputMemoryRefreshSource(step)
	if err != nil {
		return nil, map[string]any{
			"policy":                 stepInputMemorySelectionPolicy,
			"selectionPolicyVersion": stepInputMemorySelectionVersion,
			"status":                 "degraded",
			"refreshSource":          refreshSource,
			"reason":                 err.Error(),
		}
	}
	selected, candidates, excluded := selectStepInputMemoryItemsForStep(items, run.ID, runStates, step, stepInputMemoryLimit)
	contexts := memoryItemContextMaps(selected, run.ID)
	status := "selected"
	if len(selected) == 0 {
		status = "none_selected"
	}
	return contexts, map[string]any{
		"policy":                 stepInputMemorySelectionPolicy,
		"selectionPolicyVersion": stepInputMemorySelectionVersion,
		"status":                 status,
		"refreshSource":          refreshSource,
		"limit":                  stepInputMemoryLimit,
		"retrievedMemoryIds":     memoryModelItemIDs(selected),
		"retrievedMemoryHits":    memoryModelRetrievalHits(selected),
		"candidateMemoryIds":     memoryModelItemIDs(candidates),
		"selectedMemoryIds":      memoryModelItemIDs(selected),
		"excludedMemoryIds":      memoryModelItemIDs(excluded),
		"typeQuota":              stepInputMemoryTypeQuota(),
		"selectionScores":        memoryModelSelectionScores(candidates, run.ID, runStates),
		"reason": fmt.Sprintf(
			"refreshSource=%s policy=%s selected=%d candidates=%d excluded=%d",
			refreshSource,
			stepInputMemorySelectionVersion,
			len(selected),
			len(candidates),
			len(excluded),
		),
	}
}

func stepInputMemoryRunStates(ctx context.Context, runs repository.RunRepository, workspaceID common.WorkspaceID) map[common.RunID]common.RunState {
	if runs == nil || workspaceID == "" {
		return nil
	}
	items, err := runs.ListByWorkspaceID(ctx, workspaceID)
	if err != nil || len(items) == 0 {
		return nil
	}
	ret := make(map[common.RunID]common.RunState, len(items))
	for _, run := range items {
		if run == nil || run.ID == "" {
			continue
		}
		ret[run.ID] = run.State
	}
	return ret
}

func stepInputMemoryRefreshSource(step *runModel.Step) string {
	if step == nil || step.Input == nil {
		return "step_input"
	}
	if boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
		return "repair"
	}
	if boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) {
		return "provider_retry"
	}
	if boolValue(step.Input[stepInputKeyAssessmentOnly]) || boolValue(step.Input["providerRetryAssessmentOnly"]) {
		return "assessment"
	}
	if stage := strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRuntimeStage)); stage != "" {
		return stage
	}
	if stage := strings.TrimSpace(stringFromMap(step.Input, stepInputKeySoulStage)); stage != "" {
		return stage
	}
	return "step_input"
}

func selectStepInputMemoryItems(items []*memorymodel.MemoryItem, currentRunID common.RunID, runStates map[common.RunID]common.RunState, limit int) ([]*memorymodel.MemoryItem, []*memorymodel.MemoryItem, []*memorymodel.MemoryItem) {
	if limit <= 0 || len(items) == 0 {
		return nil, nil, nil
	}
	candidateByKey := map[string]*memorymodel.MemoryItem{}
	for _, item := range items {
		if item == nil || item.ID == "" {
			continue
		}
		key := memorySelectionDuplicateKey(item)
		if key == "" {
			continue
		}
		if existing := candidateByKey[key]; existing == nil || memorySelectionCandidatePreferred(item, existing, currentRunID, runStates) {
			candidateByKey[key] = item
		}
	}
	candidates := make([]*memorymodel.MemoryItem, 0, len(candidateByKey))
	for _, item := range candidateByKey {
		candidates = append(candidates, item)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := stepInputMemoryScore(candidates[i], currentRunID, runStates)
		right := stepInputMemoryScore(candidates[j], currentRunID, runStates)
		if left != right {
			return left > right
		}
		return string(candidates[i].ID) > string(candidates[j].ID)
	})
	quota := stepInputMemoryTypeQuota()
	selected := make([]*memorymodel.MemoryItem, 0, min(len(candidates), limit))
	selectedIDs := map[common.MemoryID]struct{}{}
	typeCounts := map[string]int{}
	runCounts := map[common.RunID]int{}
	for _, item := range candidates {
		if len(selected) >= limit {
			break
		}
		if shouldExcludeStepInputMemoryItem(item) {
			continue
		}
		memoryType := strings.TrimSpace(item.MemoryType)
		if maxForType, ok := quota[memoryType]; ok && typeCounts[memoryType] >= maxForType {
			continue
		}
		if !canSelectMemoryFromRun(item, currentRunID, runCounts) {
			continue
		}
		selected = append(selected, item)
		selectedIDs[item.ID] = struct{}{}
		typeCounts[memoryType]++
		incrementSelectedMemoryRunCount(item, currentRunID, runCounts)
	}
	for _, item := range candidates {
		if len(selected) >= limit {
			break
		}
		if _, ok := selectedIDs[item.ID]; ok {
			continue
		}
		if shouldExcludeStepInputMemoryItem(item) {
			continue
		}
		if _, ok := quota[strings.TrimSpace(item.MemoryType)]; ok {
			continue
		}
		if !canSelectMemoryFromRun(item, currentRunID, runCounts) {
			continue
		}
		selected = append(selected, item)
		selectedIDs[item.ID] = struct{}{}
		incrementSelectedMemoryRunCount(item, currentRunID, runCounts)
	}
	excluded := make([]*memorymodel.MemoryItem, 0, len(candidates)-len(selected))
	for _, item := range candidates {
		if _, ok := selectedIDs[item.ID]; ok {
			continue
		}
		excluded = append(excluded, item)
	}
	return selected, candidates, excluded
}

func memorySelectionCandidatePreferred(candidate *memorymodel.MemoryItem, existing *memorymodel.MemoryItem, currentRunID common.RunID, runStates map[common.RunID]common.RunState) bool {
	if candidate == nil {
		return false
	}
	if existing == nil {
		return true
	}
	candidateExcluded := shouldExcludeStepInputMemoryItem(candidate)
	existingExcluded := shouldExcludeStepInputMemoryItem(existing)
	if candidateExcluded != existingExcluded {
		return !candidateExcluded
	}
	candidateScore := stepInputMemoryScore(candidate, currentRunID, runStates)
	existingScore := stepInputMemoryScore(existing, currentRunID, runStates)
	if candidateScore != existingScore {
		return candidateScore > existingScore
	}
	return string(candidate.ID) > string(existing.ID)
}

func selectStepInputMemoryItemsForStep(items []*memorymodel.MemoryItem, currentRunID common.RunID, runStates map[common.RunID]common.RunState, step *runModel.Step, limit int) ([]*memorymodel.MemoryItem, []*memorymodel.MemoryItem, []*memorymodel.MemoryItem) {
	selected, candidates, excluded := selectStepInputMemoryItems(items, currentRunID, runStates, limit)
	if !strictRepairMemoryRecall(step) || len(selected) == 0 {
		return selected, candidates, excluded
	}
	filtered := make([]*memorymodel.MemoryItem, 0, len(selected))
	for _, item := range selected {
		if shouldExcludeStrictRepairMemoryItem(item, currentRunID, runStates) {
			excluded = append(excluded, item)
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered, candidates, excluded
}

func strictRepairMemoryRecall(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	stage := strings.TrimSpace(strings.ToLower(firstNonEmptyString(
		stringFromMap(step.Input, stepInputKeyRuntimeStage),
		stringFromMap(step.Input, stepInputKeySoulStage),
	)))
	mode := strings.TrimSpace(strings.ToLower(stringFromMap(step.Input, stepInputKeyRuntimeStageMode)))
	return stage == runtimeStageRepair && (mode == "strict_convergence" || boolValue(step.Input["stateChangingOutputRequired"]))
}

func shouldExcludeStrictRepairMemoryItem(item *memorymodel.MemoryItem, currentRunID common.RunID, runStates map[common.RunID]common.RunState) bool {
	if item == nil {
		return true
	}
	if item.RunID == "" || item.RunID == currentRunID {
		return false
	}
	if strings.TrimSpace(item.MemoryType) == memorycommon.MemoryTypeDoDFeedback && !memoryItemHasStrictRepairReferenceBoundary(item) {
		return true
	}
	switch runStates[item.RunID] {
	case common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return !memoryItemHasReferenceBoundary(item)
	default:
		return false
	}
}

func memoryItemHasStrictRepairReferenceBoundary(item *memorymodel.MemoryItem) bool {
	if item == nil {
		return false
	}
	for _, tag := range item.Tags {
		switch strings.TrimSpace(strings.ToLower(tag)) {
		case "accepted_input", "user_confirmed", "decision":
			return true
		}
	}
	text := strings.ToLower(item.Summary + "\n" + item.Content)
	for _, token := range []string{
		"scope:",
		"appliesto:",
		"targetref:",
		"evidenceref:",
		"accepted_input",
		"user_confirmed",
		"decision",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func canSelectMemoryFromRun(item *memorymodel.MemoryItem, currentRunID common.RunID, runCounts map[common.RunID]int) bool {
	if item == nil || item.RunID == "" || currentRunID == "" || item.RunID == currentRunID {
		return true
	}
	return runCounts[item.RunID] < stepInputHistoricalRunMemoryMax
}

func incrementSelectedMemoryRunCount(item *memorymodel.MemoryItem, currentRunID common.RunID, runCounts map[common.RunID]int) {
	if item == nil || item.RunID == "" || currentRunID == "" || item.RunID == currentRunID {
		return
	}
	runCounts[item.RunID]++
}

func stepInputMemoryTypeQuota() map[string]int {
	return map[string]int{
		memorycommon.MemoryTypeDoDFeedback:     2,
		memorycommon.MemoryTypeExecutionLesson: 2,
		memorycommon.MemoryTypeProjectContext:  2,
		memorycommon.MemoryTypeSkillFeedback:   1,
		memorycommon.MemoryTypeToolFeedback:    1,
		memorycommon.MemoryTypeDecision:        1,
		memorycommon.MemoryTypeRunSummary:      1,
	}
}

func shouldExcludeStepInputMemoryItem(item *memorymodel.MemoryItem) bool {
	if item == nil {
		return true
	}
	if memoryItemHasTransientProcessTag(item) {
		return true
	}
	memoryType := strings.TrimSpace(item.MemoryType)
	if memoryType == memorycommon.MemoryTypeDoDFeedback {
		return runtimeMemoryTextHasTransientSignal(item.Summary + "\n" + item.Content)
	}
	if memoryType == memorycommon.MemoryTypeExecutionLesson {
		return runtimeMemoryTextHasTransientSignal(item.Summary + "\n" + item.Content)
	}
	if memoryType == memorycommon.MemoryTypeToolFeedback {
		if memoryItemHasTag(item, "accepted_input") {
			return false
		}
		return memoryItemHasTag(item, "provider_retry") || runtimeMemoryTextHasTransientSignal(item.Summary+"\n"+item.Content)
	}
	return false
}

func memoryItemHasReferenceBoundary(item *memorymodel.MemoryItem) bool {
	if item == nil {
		return false
	}
	for _, tag := range item.Tags {
		switch strings.TrimSpace(strings.ToLower(tag)) {
		case "accepted_input", "reference_value", "decision", "user_confirmed":
			return true
		}
	}
	text := strings.ToLower(item.Summary + "\n" + item.Content)
	for _, token := range []string{
		"scope:",
		"precondition:",
		"appliesto:",
		"validuntil:",
		"evidenceref:",
		"reference_value",
		"referencevalue",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func memoryItemHasTransientProcessTag(item *memorymodel.MemoryItem) bool {
	if item == nil {
		return true
	}
	for _, tag := range item.Tags {
		switch strings.TrimSpace(strings.ToLower(tag)) {
		case "provider_retry", "transient", "runtime_plumbing", "schema_invalid", "parse_error", "rejected_output":
			return true
		}
	}
	return false
}

func memoryItemHasTag(item *memorymodel.MemoryItem, tag string) bool {
	if item == nil {
		return false
	}
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return false
	}
	for _, candidate := range item.Tags {
		if strings.TrimSpace(strings.ToLower(candidate)) == tag {
			return true
		}
	}
	return false
}

func stepInputMemoryScore(item *memorymodel.MemoryItem, currentRunID common.RunID, runStates map[common.RunID]common.RunState) int {
	if item == nil {
		return 0
	}
	score := item.Importance*10 + item.Confidence
	if item.RunID == currentRunID && currentRunID != "" {
		score += 10000
	}
	switch strings.TrimSpace(item.MemoryType) {
	case memorycommon.MemoryTypeDoDFeedback:
		score += 6000
	case memorycommon.MemoryTypeExecutionLesson:
		score += 5500
	case memorycommon.MemoryTypeProjectContext:
		score += 5000
	case memorycommon.MemoryTypeSkillFeedback:
		score += 4500
	case memorycommon.MemoryTypeToolFeedback:
		score += 4400
	case memorycommon.MemoryTypeDecision:
		score += 4000
	case memorycommon.MemoryTypeRunSummary:
		score += 1000
		if item.RunID != currentRunID || currentRunID == "" {
			score -= 750
		}
	}
	if memoryItemHasTag(item, "accepted_input") {
		score += 2000
	}
	if shouldExcludeStepInputMemoryItem(item) {
		score -= 7000
	}
	switch runStates[item.RunID] {
	case common.RunStateSuperseded:
		score -= 3000
	case common.RunStateFailed, common.RunStateCancelled:
		score -= 2000
	case common.RunStateCompleted:
		score += 500
	}
	return score
}

func memorySelectionDuplicateKey(item *memorymodel.MemoryItem) string {
	if item == nil {
		return ""
	}
	summary := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(item.Summary)), " "))
	if summary == "" {
		summary = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(item.Content)), " "))
	}
	return strings.TrimSpace(item.MemoryType) + "|" + summary
}

func memoryItemContextMaps(items []*memorymodel.MemoryItem, currentRunID common.RunID) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, map[string]any{
			"id":          string(item.ID),
			"memoryType":  item.MemoryType,
			"summary":     item.Summary,
			"content":     item.Content,
			"tags":        cloneStringsForExecutionContext(item.Tags),
			"importance":  item.Importance,
			"confidence":  item.Confidence,
			"visibility":  item.Visibility,
			"provenance":  memoryContextProvenance(item, currentRunID),
			"vectorScore": item.VectorScore,
		})
	}
	return ret
}

func memoryContextProvenance(item *memorymodel.MemoryItem, currentRunID common.RunID) string {
	if item == nil || item.RunID == "" {
		return "workspace"
	}
	if currentRunID != "" && item.RunID == currentRunID {
		return "current_run"
	}
	return "historical_run"
}

func memoryModelItemIDs(items []*memorymodel.MemoryItem) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil || item.ID == "" {
			continue
		}
		ret = append(ret, string(item.ID))
	}
	return ret
}

func memoryModelRetrievalHits(items []*memorymodel.MemoryItem) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item == nil || item.ID == "" {
			continue
		}
		ret = append(ret, map[string]any{
			"id":              string(item.ID),
			"summary":         item.Summary,
			"vectorScore":     item.VectorScore,
			"vectorDimension": len(item.Vector),
		})
	}
	return ret
}

func memoryModelSelectionScores(items []*memorymodel.MemoryItem, currentRunID common.RunID, runStates map[common.RunID]common.RunState) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item == nil || item.ID == "" {
			continue
		}
		ret = append(ret, map[string]any{
			"id":     string(item.ID),
			"score":  stepInputMemoryScore(item, currentRunID, runStates),
			"reason": stepInputMemoryScoreReason(item, currentRunID, runStates),
		})
	}
	return ret
}

func stepInputMemoryScoreReason(item *memorymodel.MemoryItem, currentRunID common.RunID, runStates map[common.RunID]common.RunState) string {
	if item == nil {
		return ""
	}
	parts := []string{strings.TrimSpace(item.MemoryType)}
	if item.RunID == currentRunID && currentRunID != "" {
		parts = append(parts, "current_run")
	} else if item.RunID != "" {
		parts = append(parts, "historical_run")
	}
	if item.MemoryType == memorycommon.MemoryTypeRunSummary && item.RunID != currentRunID {
		parts = append(parts, "run_summary_downgraded")
	}
	if memoryItemHasTag(item, "accepted_input") {
		parts = append(parts, "accepted_input_boost")
	}
	if shouldExcludeStepInputMemoryItem(item) {
		parts = append(parts, "excluded_low_value_process_memory")
	}
	if state := runStates[item.RunID]; state != "" {
		parts = append(parts, "run_state="+string(state))
		switch state {
		case common.RunStateSuperseded, common.RunStateFailed, common.RunStateCancelled:
			parts = append(parts, "run_state_downgraded")
		}
	}
	return strings.Join(parts, ":")
}

func currentRunGovernanceHistory(
	ctx context.Context,
	tickets repository.DecisionTicketRepository,
	resolutions repository.DecisionResolutionRepository,
	approvals repository.ApprovalRepository,
	run *runModel.Run,
) []map[string]any {
	if run == nil || run.ID == "" {
		return nil
	}
	var ret []map[string]any

	if tickets != nil {
		tickets, err := tickets.ListByRunID(ctx, run.ID)
		if err == nil {
			for _, t := range tickets {
				if t == nil {
					continue
				}
				item := map[string]any{
					"id":           string(t.ID),
					"kind":         "decision",
					"state":        string(t.State),
					"reason":       t.Reason,
					"decisionKind": string(t.DecisionKind),
					"question":     t.Question,
					"stepId":       string(t.StepID),
				}
				if t.State == common.DecisionTicketStateResolved && resolutions != nil {
					if resolution, err := resolutions.GetByTicketID(ctx, t.ID); err == nil && resolution != nil {
						item["resolutionId"] = string(resolution.ID)
						item["resolutionType"] = string(resolution.Type)
						item["resolutionPayload"] = cloneStepPayload(resolution.Payload)
					}
				}
				ret = append(ret, item)
			}
		}
	}

	if approvals != nil {
		approvals, err := approvals.ListByRunID(ctx, run.ID)
		if err == nil {
			for _, a := range approvals {
				if a == nil {
					continue
				}
				item := map[string]any{
					"id":     string(a.ID),
					"kind":   "approval",
					"state":  string(a.State),
					"reason": a.Reason,
					"stepId": string(a.StepID),
				}
				ret = append(ret, item)
			}
		}
	}

	return ret
}

func redactSensitiveMapSlice(input []map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		if item == nil {
			continue
		}
		ret = append(ret, common.RedactSensitiveMap(item))
	}
	return ret
}

type stableExecutionContextPackage struct {
	latestAcceptedResult        map[string]any
	latestAcceptedResultChecked bool
	currentDeliverables         []map[string]any
	currentDeliverablesChecked  bool
	selectedContext             map[string]any
}

func stableExecutionContextPackageFromStep(step *runModel.Step) stableExecutionContextPackage {
	if step == nil || step.Input == nil {
		return stableExecutionContextPackage{}
	}
	pkg := mapFromValue(step.Input[stepInputKeyExecutionContextPackage])
	if len(pkg) == 0 {
		return stableExecutionContextPackage{}
	}
	return stableExecutionContextPackageFromPackage(pkg)
}

func stableExecutionContextPackageFromPackage(pkg map[string]any) stableExecutionContextPackage {
	if len(pkg) == 0 {
		return stableExecutionContextPackage{}
	}
	historyChecked := mapFromValue(pkg[executionContextPackageHistoryKey])
	return stableExecutionContextPackage{
		latestAcceptedResult:        mapFromValue(pkg["latestAcceptedResult"]),
		latestAcceptedResultChecked: boolFromValue(historyChecked["latestAcceptedResult"]),
		currentDeliverables:         mapSliceFromValue(pkg["currentDeliverables"]),
		currentDeliverablesChecked:  boolFromValue(historyChecked["currentDeliverables"]),
		selectedContext:             mapFromValue(pkg["selectedContext"]),
	}
}

func stableExecutionContextPackagePayload(pkg stableExecutionContextPackage) map[string]any {
	ret := map[string]any{}
	if len(pkg.latestAcceptedResult) > 0 {
		ret["latestAcceptedResult"] = cloneStepPayload(pkg.latestAcceptedResult)
	}
	if len(pkg.currentDeliverables) > 0 {
		ret["currentDeliverables"] = limitedMapSlice(pkg.currentDeliverables, len(pkg.currentDeliverables))
	}
	historyChecked := workspaceHistoryCheckedPackage(pkg, false, false)
	if len(historyChecked) > 0 {
		ret[executionContextPackageHistoryKey] = historyChecked
	}
	if len(pkg.selectedContext) > 0 {
		ret["selectedContext"] = cloneStepPayload(pkg.selectedContext)
	}
	return ret
}

func workspaceHistoryCheckedPackage(pkg stableExecutionContextPackage, acceptedResultRead bool, deliverablesRead bool) map[string]any {
	ret := map[string]any{}
	if acceptedResultRead || pkg.latestAcceptedResultChecked || len(pkg.latestAcceptedResult) > 0 {
		ret["latestAcceptedResult"] = true
	}
	if deliverablesRead || pkg.currentDeliverablesChecked || len(pkg.currentDeliverables) > 0 {
		ret["currentDeliverables"] = true
	}
	return ret
}

func executionContextWorkspaceFileManifest(
	ctx context.Context,
	workspaces repository.WorkspaceRepository,
	run *runModel.Run,
	step *runModel.Step,
	workspace *workspaceModel.Workspace,
) []map[string]any {
	if step != nil {
		if manifest := mapSliceFromValue(step.Input[stepInputKeyWorkspaceFileManifest]); len(manifest) > 0 {
			return compactWorkspaceFileManifest(manifest)
		}
	}
	if !shouldIncludeWorkspaceManifestInExecutionContextPackage(run, step) {
		return nil
	}
	if workspace == nil && workspaces != nil && run != nil && run.WorkspaceID != "" {
		loaded, err := workspaces.Get(ctx, run.WorkspaceID)
		if err == nil {
			workspace = loaded
		}
	}
	if workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	return compactWorkspaceFileManifest(workspaceFileManifestFromDirectory(workspace.DeliverableDirectory))
}

func shouldIncludeWorkspaceManifestInExecutionContextPackage(run *runModel.Run, step *runModel.Step) bool {
	if step == nil {
		return false
	}
	if boolValue(step.Input["stateChangingOutputRequired"]) ||
		boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
		return true
	}
	if run != nil && len(mapFromValue(run.ExecutionContext["replanContinuation"])) > 0 {
		return true
	}
	switch runtimeStageFromStepInput(step.Input) {
	case runtimeStageExploration, runtimeStagePlanning:
		return true
	default:
		return false
	}
}

func recentFailureContexts(ctx context.Context, steps repository.StepRepository, run *runModel.Run, currentStepID common.StepID, limit int) []map[string]any {
	if steps == nil || run == nil || limit <= 0 {
		return nil
	}
	if currentStepID == initialStepID(run.ID) {
		return nil
	}
	items, err := steps.ListByRun(ctx, run.ID)
	if err != nil || len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, min(len(items), limit))
	for idx := len(items) - 1; idx >= 0; idx-- {
		step := items[idx]
		if step == nil || step.ID == currentStepID || step.State != common.StepStateFailed {
			continue
		}
		ret = append(ret, failureContextFromStep(step))
		if len(ret) >= limit {
			break
		}
	}
	return ret
}

func shouldReadRecentFailureHistory(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
		return true
	}
	for _, key := range []string{
		"failedStepId",
		"failedStepKind",
		"failedStepState",
		"failedStepOutput",
		"failureReason",
		failureOutputKeyClass,
		failureOutputKeySignature,
	} {
		if value, ok := step.Input[key]; ok && value != nil {
			return true
		}
	}
	return false
}

func shouldReadProcessEvidenceTimeline(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) ||
		boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) ||
		boolValue(step.Input[stepInputKeyAssessmentOnly]) ||
		boolValue(step.Input[stepInputKeyGraphLimitFollowUp]) ||
		boolValue(step.Input[stepInputKeyNoActionableFollowUp]) ||
		boolValue(step.Input["completionFollowUp"]) ||
		boolValue(step.Input["graphFollowUp"]) {
		return true
	}
	stage := strings.TrimSpace(strings.ToLower(stringFromMap(step.Input, stepInputKeyRuntimeStage)))
	switch stage {
	case runtimeStageExecution, runtimeStageVerification, runtimeStageAcceptance, runtimeStageRepair, runtimeStageReplan:
		return true
	}
	soulStage := strings.TrimSpace(strings.ToLower(stringFromMap(step.Input, stepInputKeySoulStage)))
	switch soulStage {
	case soulStageExecution, "validation", "verification", "artifact", "result", "final":
		return true
	default:
		return false
	}
}

func initialStepID(runID common.RunID) common.StepID {
	if runID == "" {
		return ""
	}
	return common.StepID(string(runID) + "/step-1")
}

func failureContextFromStep(step *runModel.Step) map[string]any {
	output := step.Output
	return map[string]any{
		"stepId":           string(step.ID),
		"kind":             step.Kind,
		"failureClass":     stringFromMap(output, failureOutputKeyClass),
		"failureSignature": stringFromMap(output, failureOutputKeySignature),
		"stage":            stringFromMap(output, "stage"),
		"error":            firstNonEmptyString(stringFromMap(output, "error"), stringFromMap(output, "reason"), stringFromMap(output, "stderr")),
	}
}

func selectedContextPackage(executionContext map[string]any) map[string]any {
	ret := map[string]any{}
	if skills := compactSkillContexts(mapSliceFromValue(executionContext["skills"])); len(skills) > 0 {
		ret["skills"] = skills
	}
	if servers := compactMCPServerContexts(mapSliceFromValue(executionContext["mcpServers"])); len(servers) > 0 {
		ret["mcpServers"] = servers
	}
	if capabilities := compactMCPCapabilityContexts(mapSliceFromValue(executionContext["mcpCapabilities"])); len(capabilities) > 0 {
		ret["mcpCapabilities"] = capabilities
	}
	if capabilityIDs := compactUniqueStrings(stringSliceFromValue(executionContext["capabilityIds"])); len(capabilityIDs) > 0 {
		ret["capabilityIds"] = capabilityIDs
	}
	if memories := compactMemoryContexts(mapSliceFromValue(executionContext["memories"])); len(memories) > 0 {
		ret["memories"] = memories
	}
	if retrieval := mapFromValue(executionContext["memoryRetrieval"]); len(retrieval) > 0 {
		ret["memoryRetrieval"] = retrieval
	}
	return ret
}

func (o *RuntimeOrchestrator) recordLLMStepInputMemoryRecallTrace(ctx context.Context, run *runModel.Run, step *runModel.Step, pkg map[string]any) error {
	if o == nil || o.evidence.MemoryRecallTraces == nil || run == nil || step == nil || run.ID == "" || run.WorkspaceID == "" || step.ID == "" {
		return nil
	}
	selected := mapFromValue(pkg["selectedContext"])
	retrieval := mapFromValue(selected["memoryRetrieval"])
	if len(retrieval) == 0 {
		return nil
	}
	memories := mapSliceFromValue(selected["memories"])
	traceID := common.MemoryRecallTraceID(fmt.Sprintf("%s/memory-recall/step-input", step.ID))
	stage := firstNonEmptyString(
		stringFromMap(pkg, "stage"),
		stringFromMap(step.Input, stepInputKeySoulStage),
		soulStageExecution,
	)
	trace := &memorymodel.MemoryRecallTrace{
		ID:                     traceID,
		WorkspaceID:            run.WorkspaceID,
		RunID:                  run.ID,
		StepID:                 step.ID,
		GraphNodeID:            stringFromMap(step.Input, stepInputKeyGraphNodeID),
		Stage:                  stage,
		Source:                 "step_input",
		Policy:                 memoryRecallTraceStringValue(retrieval["policy"]),
		Status:                 memoryRecallTraceStringValue(retrieval["status"]),
		ProviderID:             memoryRecallTraceStringValue(retrieval["providerId"]),
		Operation:              memoryRecallTraceStringValue(retrieval["operation"]),
		QueryTextPreview:       memoryRecallTraceStringValue(retrieval["queryTextPreview"]),
		QueryTextSha256:        memoryRecallTraceStringValue(retrieval["queryTextSha256"]),
		MinScore:               floatFromValue(retrieval["minScore"]),
		Limit:                  int(int64FromValue(retrieval["limit"])),
		EmbeddingModel:         memoryRecallTraceStringValue(retrieval["embeddingModel"]),
		VectorDimension:        int(int64FromValue(retrieval["vectorDimension"])),
		RefreshSource:          memoryRecallTraceStringValue(retrieval["refreshSource"]),
		SelectionPolicyVersion: memoryRecallTraceStringValue(retrieval["selectionPolicyVersion"]),
		CandidateMemoryIDs:     memoryRecallTraceIDs(retrieval["candidateMemoryIds"]),
		SelectedMemoryIDs:      memoryRecallTraceIDs(retrieval["selectedMemoryIds"]),
		ExcludedMemoryIDs:      memoryRecallTraceIDs(retrieval["excludedMemoryIds"]),
		TypeQuota:              memoryRecallTraceTypeQuota(retrieval["typeQuota"]),
		SelectionScores:        memoryRecallTraceSelectionScores(retrieval["selectionScores"]),
		RetrievedMemoryIDs:     memoryRecallTraceIDs(retrieval["retrievedMemoryIds"]),
		RetrievedMemoryHits:    memoryRecallTraceHits(retrieval["retrievedMemoryHits"]),
		Reason:                 memoryRecallTraceStringValue(retrieval["reason"]),
		OccurredAt:             time.Now().UTC().Format(time.RFC3339Nano),
		EvidenceRef:            string(traceID),
	}
	selectedIDsDeclared := memoryRecallTraceMapHasKey(retrieval, "selectedMemoryIds")
	selectedIDs := memoryRecallTraceIDSet(trace.SelectedMemoryIDs)
	excludedIDs := memoryRecallTraceIDSet(trace.ExcludedMemoryIDs)
	if len(trace.RetrievedMemoryIDs) == 0 {
		trace.RetrievedMemoryIDs = memoryRecallTraceMemoryIDsFromContexts(memories)
	}
	trace.RetrievedMemoryIDs = filterMemoryRecallTraceIDs(trace.RetrievedMemoryIDs, selectedIDs, excludedIDs, selectedIDsDeclared)
	if len(trace.SelectedMemoryIDs) == 0 {
		trace.SelectedMemoryIDs = append([]common.MemoryID(nil), trace.RetrievedMemoryIDs...)
	} else {
		trace.SelectedMemoryIDs = filterMemoryRecallTraceIDs(trace.SelectedMemoryIDs, nil, excludedIDs, false)
	}
	if len(trace.RetrievedMemoryHits) == 0 {
		trace.RetrievedMemoryHits = memoryRecallTraceHitsFromContexts(memories)
	}
	trace.RetrievedMemoryHits = filterMemoryRecallTraceHits(trace.RetrievedMemoryHits, selectedIDs, excludedIDs, selectedIDsDeclared)
	if err := o.evidence.MemoryRecallTraces.Upsert(ctx, trace); err != nil {
		return err
	}
	if o.eventHub == nil {
		return nil
	}
	return events.PublishByContract(ctx, o.eventHub, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        memoryevents.TopicMemoryRecallRecorded,
		OccurredAt:   trace.OccurredAt,
		SourceModule: "kernel/orchestrator",
		SubjectType:  "memory_recall",
		SubjectID:    string(trace.ID),
		WorkspaceID:  string(trace.WorkspaceID),
		RunID:        string(trace.RunID),
		StepID:       string(trace.StepID),
		Payload: events.MemoryRecallPayload(
			trace.ID,
			trace.Stage,
			trace.Source,
			trace.Policy,
			trace.Status,
			trace.ProviderID,
			trace.Operation,
			trace.RetrievedMemoryIDs,
			trace.Reason,
		),
	})
}

func memoryRecallTraceMapHasKey(input map[string]any, key string) bool {
	if len(input) == 0 {
		return false
	}
	_, ok := input[key]
	return ok
}

func memoryRecallTraceIDSet(ids []common.MemoryID) map[common.MemoryID]struct{} {
	if len(ids) == 0 {
		return nil
	}
	ret := make(map[common.MemoryID]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		ret[id] = struct{}{}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func filterMemoryRecallTraceIDs(ids []common.MemoryID, selected map[common.MemoryID]struct{}, excluded map[common.MemoryID]struct{}, requireSelected bool) []common.MemoryID {
	if len(ids) == 0 {
		return nil
	}
	if requireSelected && len(selected) == 0 {
		return nil
	}
	ret := make([]common.MemoryID, 0, len(ids))
	seen := map[common.MemoryID]struct{}{}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := excluded[id]; ok {
			continue
		}
		if len(selected) > 0 {
			if _, ok := selected[id]; !ok {
				continue
			}
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ret = append(ret, id)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func filterMemoryRecallTraceHits(hits []memorymodel.MemoryRecallHit, selected map[common.MemoryID]struct{}, excluded map[common.MemoryID]struct{}, requireSelected bool) []memorymodel.MemoryRecallHit {
	if len(hits) == 0 {
		return nil
	}
	if requireSelected && len(selected) == 0 {
		return nil
	}
	ret := make([]memorymodel.MemoryRecallHit, 0, len(hits))
	seen := map[common.MemoryID]struct{}{}
	for _, hit := range hits {
		if hit.ID == "" {
			continue
		}
		if _, ok := excluded[hit.ID]; ok {
			continue
		}
		if len(selected) > 0 {
			if _, ok := selected[hit.ID]; !ok {
				continue
			}
		}
		if _, ok := seen[hit.ID]; ok {
			continue
		}
		seen[hit.ID] = struct{}{}
		ret = append(ret, hit)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func memoryRecallTraceIDs(value any) []common.MemoryID {
	switch typed := value.(type) {
	case []common.MemoryID:
		return append([]common.MemoryID(nil), typed...)
	case []string:
		ret := make([]common.MemoryID, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				ret = append(ret, common.MemoryID(item))
			}
		}
		return ret
	case []any:
		ret := make([]common.MemoryID, 0, len(typed))
		for _, item := range typed {
			if text := memoryRecallTraceStringValue(item); text != "" {
				ret = append(ret, common.MemoryID(text))
			}
		}
		return ret
	default:
		return nil
	}
}

func memoryRecallTraceHits(value any) []memorymodel.MemoryRecallHit {
	items := mapSliceFromValue(value)
	ret := make([]memorymodel.MemoryRecallHit, 0, len(items))
	for _, item := range items {
		id := common.MemoryID(memoryRecallTraceStringValue(item["id"]))
		if id == "" {
			continue
		}
		ret = append(ret, memorymodel.MemoryRecallHit{
			ID:              id,
			Summary:         memoryRecallTraceStringValue(item["summary"]),
			VectorScore:     floatFromValue(item["vectorScore"]),
			VectorDimension: int(int64FromValue(item["vectorDimension"])),
		})
	}
	return ret
}

func memoryRecallTraceTypeQuota(value any) map[string]int {
	if typed, ok := value.(map[string]int); ok {
		ret := make(map[string]int, len(typed))
		for key, quota := range typed {
			ret[key] = quota
		}
		return ret
	}
	input := mapFromValue(value)
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]int, len(input))
	for key, item := range input {
		if quota := int(int64FromValue(item)); quota > 0 {
			ret[key] = quota
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func memoryRecallTraceSelectionScores(value any) []memorymodel.MemoryRecallSelectionScore {
	items := mapSliceFromValue(value)
	if len(items) == 0 {
		return nil
	}
	ret := make([]memorymodel.MemoryRecallSelectionScore, 0, len(items))
	for _, item := range items {
		id := common.MemoryID(memoryRecallTraceStringValue(item["id"]))
		if id == "" {
			continue
		}
		ret = append(ret, memorymodel.MemoryRecallSelectionScore{
			ID:     id,
			Score:  int(int64FromValue(item["score"])),
			Reason: memoryRecallTraceStringValue(item["reason"]),
		})
	}
	return ret
}

func memoryRecallTraceMemoryIDsFromContexts(items []map[string]any) []common.MemoryID {
	ret := make([]common.MemoryID, 0, len(items))
	for _, item := range items {
		id := common.MemoryID(memoryRecallTraceStringValue(item["id"]))
		if id != "" {
			ret = append(ret, id)
		}
	}
	return ret
}

func memoryRecallTraceHitsFromContexts(items []map[string]any) []memorymodel.MemoryRecallHit {
	ret := make([]memorymodel.MemoryRecallHit, 0, len(items))
	for _, item := range items {
		id := common.MemoryID(memoryRecallTraceStringValue(item["id"]))
		if id == "" {
			continue
		}
		ret = append(ret, memorymodel.MemoryRecallHit{
			ID:              id,
			Summary:         memoryRecallTraceStringValue(item["summary"]),
			VectorScore:     floatFromValue(item["vectorScore"]),
			VectorDimension: int(int64FromValue(item["vectorDimension"])),
		})
	}
	return ret
}

func memoryRecallTraceStringValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
}

func compactSkillContexts(items []map[string]any) []map[string]any {
	ret := make([]map[string]any, 0, min(len(items), 8))
	for _, item := range items {
		ret = append(ret, pickExecutionContextFields(item, []string{
			"id", "name", "version", "description", "triggerRules", "instruction",
			"requiredToolKinds", "requiredMCPServerRefs", "requiredCapabilityRefs", "constraints", "source", "scope",
		}))
		if len(ret) >= 8 {
			break
		}
	}
	return ret
}

func compactMCPServerContexts(items []map[string]any) []map[string]any {
	ret := make([]map[string]any, 0, min(len(items), 8))
	for _, item := range items {
		ret = append(ret, pickExecutionContextFields(item, []string{
			"id", "name", "transport", "enabled", "command", "args", "env", "allowedTools", "allowedResources", "defaultApprovalPolicy", "timeoutMillis",
		}))
		if len(ret) >= 8 {
			break
		}
	}
	return ret
}

func compactMCPCapabilityContexts(items []map[string]any) []map[string]any {
	ret := make([]map[string]any, 0, min(len(items), 16))
	for _, item := range items {
		ret = append(ret, pickExecutionContextFields(item, []string{
			"id", "serverId", "kind", "name", "displayName", "description", "category",
			"tags", "usageHint", "inputSummary", "outputSummary", "stageHints", "riskLevel", "schema", "uri", "approvalPolicy", "status",
		}))
		if len(ret) >= 16 {
			break
		}
	}
	return ret
}

func compactMemoryContexts(items []map[string]any) []map[string]any {
	ret := make([]map[string]any, 0, min(len(items), 8))
	for _, item := range items {
		compact := pickExecutionContextFields(item, []string{
			"id", "memoryType", "summary", "content", "tags", "importance", "confidence", "visibility", "vectorScore",
		})
		if provenance := staticMemoryContextProvenance(item); provenance != "" {
			compact["provenance"] = provenance
		}
		ret = append(ret, compact)
		if len(ret) >= 8 {
			break
		}
	}
	return ret
}

func staticMemoryContextProvenance(item map[string]any) string {
	runID := strings.TrimSpace(stringFromMap(item, "runId"))
	if runID != "" {
		return "historical_or_selected_run"
	}
	if strings.TrimSpace(stringFromMap(item, "sourceRef")) != "" {
		return "workspace"
	}
	return ""
}

func pickExecutionContextFields(input map[string]any, fields []string) map[string]any {
	ret := map[string]any{}
	for _, field := range fields {
		value, ok := input[field]
		if !ok {
			continue
		}
		if text, ok := value.(string); ok {
			value = trimExecutionContextText(text)
			if value == "" {
				continue
			}
		}
		ret[field] = value
	}
	return ret
}

func mapSliceFromValue(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneStepPayload(item))
		}
		return ret
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			ret = append(ret, cloneStepPayload(mapped))
		}
		return ret
	default:
		return nil
	}
}

func limitedMapSlice(input []map[string]any, limit int) []map[string]any {
	if limit <= 0 || len(input) == 0 {
		return nil
	}
	if len(input) > limit {
		input = input[:limit]
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneStepPayload(item))
	}
	return ret
}

func resultSortKey(item *resultmodel.Result) string {
	if item == nil {
		return ""
	}
	if strings.TrimSpace(item.UpdatedAt) != "" {
		return item.UpdatedAt
	}
	return item.CreatedAt
}

func artifactSortKey(item *resultmodel.Artifact) string {
	if item == nil {
		return ""
	}
	if strings.TrimSpace(item.UpdatedAt) != "" {
		return item.UpdatedAt
	}
	return item.CreatedAt
}

func artifactIDsForExecutionContext(input []common.ArtifactID) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, 0, len(input))
	for _, id := range input {
		if strings.TrimSpace(string(id)) != "" {
			ret = append(ret, string(id))
		}
	}
	return ret
}

func cloneStringsForExecutionContext(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, 0, len(input))
	for _, value := range input {
		value = strings.TrimSpace(value)
		if value != "" {
			ret = append(ret, value)
		}
	}
	return ret
}

func trimExecutionContextText(value string) string {
	value = strings.TrimSpace(value)
	const maxLen = 1200
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}
