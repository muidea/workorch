package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const (
	stepInputKeyExecutionContextPackage = "executionContextPackage"
	executionContextPackageVersion      = "workorch.execution_context.v1"
	executionContextPackageHistoryKey   = "workspaceHistoryChecked"
)

func (o *RuntimeOrchestrator) enrichLLMStepInputContext(ctx context.Context, run *runModel.Run, step *runModel.Step, workspace *workspaceModel.Workspace) {
	if o == nil || run == nil || step == nil || step.Kind != llmWorkspaceExecutionStepKind {
		return
	}
	if step.Input == nil {
		step.Input = map[string]any{}
	}
	pkg := o.runExecutionContextPackage(ctx, run, step, workspace)
	step.Input[stepInputKeyExecutionContextPackage] = pkg
	_ = o.recordLLMStepInputMemoryRecallTrace(ctx, run, step, pkg)
}

func (o *RuntimeOrchestrator) runExecutionContextPackage(ctx context.Context, run *runModel.Run, step *runModel.Step, workspace *workspaceModel.Workspace) map[string]any {
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
		"stage":       stringFromMap(step.Input, stepInputKeySoulStage),
	}
	stableContext := stableExecutionContextPackageFromStep(step)

	var (
		manifest           []map[string]any
		result             map[string]any
		deliverables       []map[string]any
		failures           []map[string]any
		currentGovernance  []map[string]any
		acceptedResultRead bool
		deliverablesRead   bool
		wg                 sync.WaitGroup
	)
	wg.Add(5)
	go func() {
		defer wg.Done()
		manifest = o.executionContextWorkspaceFileManifest(ctx, run, step, workspace)
	}()
	go func() {
		defer wg.Done()
		if stableContext.latestAcceptedResult != nil || stableContext.latestAcceptedResultChecked {
			result = stableContext.latestAcceptedResult
			return
		}
		result = o.latestAcceptedResultContext(ctx, run)
		acceptedResultRead = true
	}()
	go func() {
		defer wg.Done()
		if len(stableContext.currentDeliverables) > 0 || stableContext.currentDeliverablesChecked {
			deliverables = stableContext.currentDeliverables
			return
		}
		deliverables = o.currentDeliverableContexts(ctx, run)
		deliverablesRead = true
	}()
	go func() {
		defer wg.Done()
		if shouldReadRecentFailureHistory(step) {
			failures = o.recentFailureContexts(ctx, run, step.ID, 6)
		}
	}()
	go func() {
		defer wg.Done()
		currentGovernance = o.currentRunGovernanceHistory(ctx, run)
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
	if selected := stableContext.selectedContext; len(selected) > 0 {
		ret["selectedContext"] = selected
	} else if selected := selectedContextPackage(executionContext); len(selected) > 0 {
		ret["selectedContext"] = selected
	}
	governanceHistory := mapSliceFromValue(executionContext["governanceHistory"])
	if len(currentGovernance) > 0 {
		governanceHistory = append(currentGovernance, governanceHistory...)
	}
	if len(governanceHistory) > 0 {
		ret["governanceHistory"] = limitedMapSlice(governanceHistory, 6)
	}
	return ret
}

func (o *RuntimeOrchestrator) currentRunGovernanceHistory(ctx context.Context, run *runModel.Run) []map[string]any {
	if o == nil || run == nil || run.ID == "" {
		return nil
	}
	var ret []map[string]any

	if o.tickets != nil {
		tickets, err := o.tickets.ListByRunID(ctx, run.ID)
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
				if t.State == common.DecisionTicketStateResolved && o.resolutions != nil {
					if resolution, err := o.resolutions.GetByTicketID(ctx, t.ID); err == nil && resolution != nil {
						item["resolutionId"] = string(resolution.ID)
						item["resolutionType"] = string(resolution.Type)
						item["resolutionPayload"] = cloneStepPayload(resolution.Payload)
					}
				}
				ret = append(ret, item)
			}
		}
	}

	if o.approvals != nil {
		approvals, err := o.approvals.ListByRunID(ctx, run.ID)
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

func (o *RuntimeOrchestrator) currentGovernanceHistoryForLLMInput(ctx context.Context, run *runModel.Run) []map[string]any {
	if run == nil {
		return nil
	}
	history := mapSliceFromValue(run.ExecutionContext["governanceHistory"])
	if current := o.currentRunGovernanceHistory(ctx, run); len(current) > 0 {
		history = append(current, history...)
	}
	if len(history) == 0 {
		return nil
	}
	return limitedMapSlice(history, 6)
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

func (o *RuntimeOrchestrator) executionContextWorkspaceFileManifest(_ context.Context, _ *runModel.Run, step *runModel.Step, _ *workspaceModel.Workspace) []map[string]any {
	if step != nil {
		if manifest := mapSliceFromValue(step.Input[stepInputKeyWorkspaceFileManifest]); len(manifest) > 0 {
			return compactWorkspaceFileManifest(manifest)
		}
	}
	return nil
}

func (o *RuntimeOrchestrator) latestAcceptedResultContext(ctx context.Context, run *runModel.Run) map[string]any {
	if o == nil || run == nil || o.evidence.Results == nil {
		return nil
	}
	results, err := o.evidence.Results.ListByWorkspace(ctx, run.WorkspaceID)
	if err != nil || len(results) == 0 {
		return nil
	}
	sort.SliceStable(results, func(i, j int) bool {
		return resultSortKey(results[i]) > resultSortKey(results[j])
	})
	for _, item := range results {
		if item == nil || item.State != common.ResultStateAccepted {
			continue
		}
		return map[string]any{
			"id":           string(item.ID),
			"runId":        string(item.RunID),
			"goalId":       string(item.GoalID),
			"doDId":        string(item.DoDID),
			"title":        item.Title,
			"summary":      item.Summary,
			"sourceType":   item.SourceType,
			"sourceRef":    item.SourceRef,
			"evidenceRefs": cloneStringsForExecutionContext(item.EvidenceRefs),
			"artifactRefs": artifactIDsForExecutionContext(item.ArtifactRefs),
		}
	}
	return nil
}

func (o *RuntimeOrchestrator) currentDeliverableContexts(ctx context.Context, run *runModel.Run) []map[string]any {
	if o == nil || run == nil || o.evidence.Artifacts == nil {
		return nil
	}
	items, err := o.evidence.Artifacts.ListByWorkspace(ctx, run.WorkspaceID)
	if err != nil || len(items) == 0 {
		return nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		return artifactSortKey(items[i]) > artifactSortKey(items[j])
	})
	ret := make([]map[string]any, 0, min(len(items), 8))
	for _, item := range items {
		if item == nil || item.State != common.ArtifactStateAvailable {
			continue
		}
		ret = append(ret, map[string]any{
			"id":           string(item.ID),
			"runId":        string(item.RunID),
			"resultId":     string(item.ResultID),
			"stepId":       string(item.StepID),
			"type":         item.ArtifactType,
			"name":         item.Name,
			"summary":      item.Summary,
			"uri":          item.URI,
			"contentType":  item.ContentType,
			"evidenceRefs": cloneStringsForExecutionContext(item.EvidenceRefs),
		})
		if len(ret) >= 8 {
			break
		}
	}
	return ret
}

func (o *RuntimeOrchestrator) recentFailureContexts(ctx context.Context, run *runModel.Run, currentStepID common.StepID, limit int) []map[string]any {
	if o == nil || run == nil || o.steps == nil || limit <= 0 {
		return nil
	}
	if currentStepID == initialStepID(run.ID) {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, min(len(steps), limit))
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
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
		ID:                  traceID,
		WorkspaceID:         run.WorkspaceID,
		RunID:               run.ID,
		StepID:              step.ID,
		GraphNodeID:         stringFromMap(step.Input, stepInputKeyGraphNodeID),
		Stage:               stage,
		Source:              "step_input",
		Policy:              memoryRecallTraceStringValue(retrieval["policy"]),
		Status:              memoryRecallTraceStringValue(retrieval["status"]),
		ProviderID:          memoryRecallTraceStringValue(retrieval["providerId"]),
		Operation:           memoryRecallTraceStringValue(retrieval["operation"]),
		QueryTextPreview:    memoryRecallTraceStringValue(retrieval["queryTextPreview"]),
		QueryTextSha256:     memoryRecallTraceStringValue(retrieval["queryTextSha256"]),
		MinScore:            floatFromValue(retrieval["minScore"]),
		Limit:               int(int64FromValue(retrieval["limit"])),
		EmbeddingModel:      memoryRecallTraceStringValue(retrieval["embeddingModel"]),
		VectorDimension:     int(int64FromValue(retrieval["vectorDimension"])),
		RetrievedMemoryIDs:  memoryRecallTraceIDs(retrieval["retrievedMemoryIds"]),
		RetrievedMemoryHits: memoryRecallTraceHits(retrieval["retrievedMemoryHits"]),
		Reason:              memoryRecallTraceStringValue(retrieval["reason"]),
		OccurredAt:          time.Now().UTC().Format(time.RFC3339Nano),
		EvidenceRef:         string(traceID),
	}
	if len(trace.RetrievedMemoryIDs) == 0 {
		trace.RetrievedMemoryIDs = memoryRecallTraceMemoryIDsFromContexts(memories)
	}
	if len(trace.RetrievedMemoryHits) == 0 {
		trace.RetrievedMemoryHits = memoryRecallTraceHitsFromContexts(memories)
	}
	if err := o.evidence.MemoryRecallTraces.Upsert(ctx, trace); err != nil {
		return err
	}
	if o.eventHub == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicMemoryRecallRecorded,
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
			"id", "name", "transport", "enabled", "allowedTools", "allowedResources", "defaultApprovalPolicy", "timeoutMillis",
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
			"tags", "usageHint", "inputSummary", "outputSummary", "stageHints", "riskLevel", "uri", "approvalPolicy", "status",
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
		ret = append(ret, pickExecutionContextFields(item, []string{
			"id", "runId", "sourceType", "sourceRef", "memoryType", "summary", "content",
			"tags", "importance", "confidence", "visibility", "vectorScore",
		}))
		if len(ret) >= 8 {
			break
		}
	}
	return ret
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
