package orchestrator

import (
	"context"
	"sort"
	"strings"
	"sync"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	stepInputKeyExecutionContextPackage = "executionContextPackage"
	executionContextPackageVersion      = "workorch.execution_context.v1"
)

func (o *RuntimeOrchestrator) enrichLLMStepInputContext(ctx context.Context, run *runModel.Run, step *runModel.Step, workspace *workspaceModel.Workspace) {
	if o == nil || run == nil || step == nil || step.Kind != llmWorkspaceExecutionStepKind {
		return
	}
	if step.Input == nil {
		step.Input = map[string]any{}
	}
	step.Input[stepInputKeyExecutionContextPackage] = o.runExecutionContextPackage(ctx, run, step, workspace)
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

	var (
		manifest     []map[string]any
		result       map[string]any
		deliverables []map[string]any
		failures     []map[string]any
		wg           sync.WaitGroup
	)
	wg.Add(4)
	go func() {
		defer wg.Done()
		manifest = o.executionContextWorkspaceFileManifest(ctx, run, step, workspace)
	}()
	go func() {
		defer wg.Done()
		result = o.latestAcceptedResultContext(ctx, run)
	}()
	go func() {
		defer wg.Done()
		deliverables = o.currentDeliverableContexts(ctx, run)
	}()
	go func() {
		defer wg.Done()
		failures = o.recentFailureContexts(ctx, run, step.ID, 6)
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
	if len(failures) > 0 {
		ret["recentFailureSummary"] = failures
	}
	if selected := selectedContextPackage(executionContext); len(selected) > 0 {
		ret["selectedContext"] = selected
	}
	if governanceHistory := mapSliceFromValue(executionContext["governanceHistory"]); len(governanceHistory) > 0 {
		ret["governanceHistory"] = limitedMapSlice(governanceHistory, 6)
	}
	return ret
}

func (o *RuntimeOrchestrator) executionContextWorkspaceFileManifest(ctx context.Context, run *runModel.Run, step *runModel.Step, workspace *workspaceModel.Workspace) []map[string]any {
	if step != nil {
		if manifest := mapSliceFromValue(step.Input[stepInputKeyWorkspaceFileManifest]); len(manifest) > 0 {
			return manifest
		}
	}
	if workspace != nil {
		return workspaceFileManifestFromDirectory(workspace.WorkingDirectory)
	}
	return o.workspaceFileManifest(ctx, run)
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
