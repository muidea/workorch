package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (o *RuntimeOrchestrator) publishRunStateChanged(ctx context.Context, run *runModel.Run) error {
	return o.publishRunStateChangedAtStep(ctx, run, "")
}

func (o *RuntimeOrchestrator) publishRunCreated(ctx context.Context, run *runModel.Run) error {
	if o.eventHub == nil || run == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicRunCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "run",
		SubjectID:    string(run.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		Payload: events.RunPayload(
			run.GoalID,
			run.DoDID,
			run.State,
			run.BlockingTicketID,
			run.BlockingApprovalID,
			run.FailureReason,
		),
	})
}

func (o *RuntimeOrchestrator) publishRunStateChangedAtStep(ctx context.Context, run *runModel.Run, currentStepID common.StepID) error {
	if err := o.recordRunCheckpointAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	if o.eventHub == nil || run == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicRunStateChanged,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "run",
		SubjectID:    string(run.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		Payload: events.RunPayload(
			run.GoalID,
			run.DoDID,
			run.State,
			run.BlockingTicketID,
			run.BlockingApprovalID,
			run.FailureReason,
		),
	})
}

func (o *RuntimeOrchestrator) publishResultAccepted(ctx context.Context, item *resultmodel.Result) error {
	if item == nil || o.eventHub == nil {
		return nil
	}
	artifactRefs := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		ID:           fmt.Sprintf("%s/event/%s", item.ID, resultEventStateSuffix(item.State)),
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicResultAccepted,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "result",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		Payload: events.ResultPayload(
			item.GoalID,
			item.DoDID,
			item.State,
			item.Title,
			item.Summary,
			item.SourceType,
			item.SourceRef,
			item.EvidenceRefs,
			artifactRefs,
		),
	})
}

func resultEventStateSuffix(state common.ResultState) string {
	switch state {
	case common.ResultStateAccepted:
		return "accepted"
	case common.ResultStateRejected:
		return "rejected"
	case common.ResultStateSuperseded:
		return "superseded"
	default:
		suffix := strings.TrimSpace(string(state))
		if suffix == "" {
			return "recorded"
		}
		return suffix
	}
}

func (o *RuntimeOrchestrator) publishArtifactCreated(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil || o.eventHub == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		ID:           fmt.Sprintf("%s/event/created", item.ID),
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicArtifactCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "artifact",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		Payload: events.ArtifactPayload(
			item.ResultID,
			item.StepID,
			item.ToolCallID,
			item.PatchID,
			item.ValidationResultID,
			item.ArtifactType,
			item.Name,
			item.Summary,
			item.URI,
			item.ContentType,
			item.State,
			item.EvidenceRefs,
		),
	})
}

func (o *RuntimeOrchestrator) publishPatchChanged(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, patch *runModel.Patch, topic string, failure error) error {
	if o.eventHub == nil || run == nil || patch == nil {
		return nil
	}
	var stepID common.StepID
	var toolCallID common.ToolCallID
	if step != nil {
		stepID = step.ID
	}
	if toolCall != nil {
		toolCallID = toolCall.ID
	}
	if stepID == "" {
		stepID = patch.StepID
	}
	if toolCallID == "" {
		toolCallID = patch.ToolCallID
	}
	addedLines, deletedLines := patchLineStats(patch)
	contentHash, diffHash := patchContentHashes(patch)
	approvalID := patch.ApprovalID
	if topic == events.TopicPatchReverted && patch.RevertApprovalID != "" {
		approvalID = patch.RevertApprovalID
	}
	errText := ""
	if failure != nil {
		errText = failure.Error()
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		ID:           fmt.Sprintf("%s/event/%s", patch.ID, patchEventName(topic)),
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "patch",
		SubjectID:    string(patch.ID),
		WorkspaceID:  string(patch.WorkspaceID),
		RunID:        string(patch.RunID),
		StepID:       string(stepID),
		Payload: events.PatchPayload(
			patch.ID,
			stepID,
			toolCallID,
			patch.PatchType,
			patch.TargetRef,
			patch.Summary,
			patch.State,
			addedLines,
			deletedLines,
			contentHash,
			diffHash,
			approvalID,
			"",
			errText,
		),
	})
}

func (o *RuntimeOrchestrator) publishPatchPreview(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, preview map[string]any) error {
	if o.eventHub == nil || run == nil || step == nil || len(preview) == 0 {
		return nil
	}
	toolCallID := common.ToolCallID("")
	if toolCall != nil {
		toolCallID = toolCall.ID
	}
	targetRef := stringFromAny(preview["targetRef"])
	patchType := stringFromAny(preview["patchType"])
	subjectID := fmt.Sprintf("%s/patch-preview", step.ID)
	return o.eventHub.Publish(ctx, events.Envelope{
		ID:           fmt.Sprintf("%s/event/patch-previewed", subjectID),
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicPatchPreviewed,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "patch",
		SubjectID:    subjectID,
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		StepID:       string(step.ID),
		Payload: events.PatchPayload(
			"",
			step.ID,
			toolCallID,
			patchType,
			targetRef,
			stringFromAny(preview["summary"]),
			common.PatchStatePreviewed,
			intFromAny(preview["addedLines"]),
			intFromAny(preview["deletedLines"]),
			stringFromAny(preview["contentHash"]),
			stringFromAny(preview["diffHash"]),
			common.ApprovalID(stringFromAny(preview["approvalId"])),
			stringFromAny(preview["risk"]),
			"",
		),
	})
}

func patchEventName(topic string) string {
	topic = strings.TrimPrefix(strings.TrimSpace(topic), "/patch/")
	if topic == "" {
		return "changed"
	}
	return topic
}

func patchLineStats(patch *runModel.Patch) (int, int) {
	if patch == nil {
		return 0, 0
	}
	if strings.TrimSpace(patch.Diff) != "" {
		filePatch, err := parseSingleFileUnifiedDiff(patch.Diff)
		if err == nil {
			return unifiedDiffStats(filePatch)
		}
	}
	if strings.TrimSpace(patch.Content) != "" {
		return countTextLines(patch.Content), 0
	}
	return 0, 0
}

func patchContentHashes(patch *runModel.Patch) (string, string) {
	if patch == nil {
		return "", ""
	}
	contentHash := ""
	diffHash := ""
	if patch.Content != "" {
		sum := sha256.Sum256([]byte(patch.Content))
		contentHash = hex.EncodeToString(sum[:])
	}
	if patch.Diff != "" {
		sum := sha256.Sum256([]byte(patch.Diff))
		diffHash = hex.EncodeToString(sum[:])
	}
	return contentHash, diffHash
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return text
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func (o *RuntimeOrchestrator) publishMemoryChanged(ctx context.Context, topic string, item *memorymodel.MemoryItem) error {
	if item == nil || o.eventHub == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "memory",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		Payload:      events.MemoryPayload(item.MemoryType, item.Summary, item.Content, item.SourceType, item.SourceRef, item.Tags, item.Importance, item.Confidence, item.Visibility),
	})
}

func (o *RuntimeOrchestrator) publishWorkspaceStateChanged(ctx context.Context, workspace *workspaceModel.Workspace, runID common.RunID) error {
	if o.eventHub == nil || workspace == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicWorkspaceStateChanged,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "workspace",
		SubjectID:    string(workspace.ID),
		WorkspaceID:  string(workspace.ID),
		RunID:        string(runID),
		Payload:      events.WorkspacePayload(workspace.Title, workspace.Description, workspace.DeliverableDirectory, workspace.InternalDirectory, workspace.State, workspace.ActiveRunID),
	})
}

func (o *RuntimeOrchestrator) publishStepState(ctx context.Context, run *runModel.Run, step *runModel.Step, topic string) error {
	if o.eventHub == nil || run == nil || step == nil {
		return nil
	}
	if err := o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "step",
		SubjectID:    string(step.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		StepID:       string(step.ID),
		Payload:      events.StepPayload(step.Kind, step.State, runtimeStepEventInput(run, step), runtimeStepEventOutput(run, step)),
	}); err != nil {
		return err
	}
	if topic == events.TopicStepStateChanged {
		if err := o.publishLLMStreamProgressFromStep(ctx, run, step); err != nil {
			return err
		}
		if err := o.publishLLMUsageFromStep(ctx, run, step); err != nil {
			return err
		}
	}
	return nil
}

func runtimeStepEventInput(run *runModel.Run, step *runModel.Step) map[string]any {
	if run == nil || step == nil {
		return nil
	}
	ret := map[string]any{
		"inputRef": runtimeRawPayloadRef(run.WorkspaceID, run.ID, "steps", string(step.ID), "input.json"),
	}
	if summary := compactStepInputForLLMContext(step.Input); len(summary) > 0 {
		ret["summary"] = summary
	}
	return ret
}

func runtimeStepEventOutput(run *runModel.Run, step *runModel.Step) map[string]any {
	if run == nil || step == nil || len(step.Output) == 0 {
		return nil
	}
	ret := map[string]any{
		"outputRef": runtimeRawPayloadRef(run.WorkspaceID, run.ID, "steps", string(step.ID), "output.json"),
	}
	if summary := compactStepOutputForLLMContext(step.Output); len(summary) > 0 {
		ret["summary"] = summary
	}
	return ret
}

func (o *RuntimeOrchestrator) publishLLMStreamProgressFromStep(ctx context.Context, run *runModel.Run, step *runModel.Step) error {
	if o.eventHub == nil || run == nil || step == nil || len(step.Output) == 0 {
		return nil
	}
	if !strings.HasPrefix(strings.TrimSpace(step.Kind), "llm/") {
		return nil
	}
	streamEvents := streamEventMapSlice(step.Output["streamEvents"])
	providerID, model := llmStreamProviderAndModel(step.Output)
	chunkCount := len(streamEvents)
	receivedBytes := 0
	elapsedMs := 0
	if chunkCount > 0 {
		for _, item := range streamEvents {
			receivedBytes += len(stringFromAny(item["delta"]))
			if value := intFromAny(item["elapsedMs"]); value > elapsedMs {
				elapsedMs = value
			}
			if payload, ok := item["payload"].(map[string]any); ok {
				if value := intFromAny(payload["elapsedMs"]); value > elapsedMs {
					elapsedMs = value
				}
			}
		}
	} else if diagnostics := streamDiagnosticsMap(step.Output["streamDiagnostics"]); len(diagnostics) > 0 {
		chunkCount = intFromAny(diagnostics["receivedEventCount"])
		if chunkCount == 0 {
			chunkCount = intFromAny(diagnostics["receivedDeltaCount"])
		}
		receivedBytes = intFromAny(diagnostics["receivedBytes"])
		elapsedMs = intFromAny(diagnostics["elapsedMs"])
	} else {
		return nil
	}
	status := string(step.State)
	if status == "" {
		status = stringFromAny(step.Output["status"])
	}
	if status == "" {
		status = "completed"
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		ID:           fmt.Sprintf("%s/event/llm-stream-progress", step.ID),
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicRunLLMStreamProgress,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "llm_stream_progress",
		SubjectID:    string(step.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		StepID:       string(step.ID),
		Payload: events.LLMStreamProgressPayload(
			run.WorkspaceID,
			run.ID,
			step.ID,
			providerID,
			model,
			chunkCount,
			receivedBytes,
			elapsedMs,
			status,
			streamProgressSummary(step.Output),
		),
	})
}

func streamEventMapSlice(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return typed
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			ret = append(ret, mapped)
		}
		return ret
	default:
		return nil
	}
}

func streamDiagnosticsMap(raw any) map[string]any {
	switch typed := raw.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func llmStreamProviderAndModel(output map[string]any) (string, string) {
	contextBudget, ok := output["contextBudget"].(map[string]any)
	if !ok {
		return "", ""
	}
	return stringFromAny(contextBudget["providerId"]), stringFromAny(contextBudget["model"])
}

func streamProgressSummary(output map[string]any) string {
	if output == nil {
		return ""
	}
	stage := stringFromAny(output["stage"])
	status := stringFromAny(output["status"])
	switch {
	case stage != "" && status != "":
		return stage + " - " + status
	case stage != "":
		return stage
	default:
		return status
	}
}

func (o *RuntimeOrchestrator) publishLLMUsageFromStep(ctx context.Context, run *runModel.Run, step *runModel.Step) error {
	if o.eventHub == nil || run == nil || step == nil || len(step.Output) == 0 {
		return nil
	}
	if !strings.HasPrefix(strings.TrimSpace(step.Kind), "llm/") {
		return nil
	}
	usage, ok := step.Output["llmUsage"].(map[string]any)
	if !ok || len(usage) == 0 {
		return nil
	}
	usagePayload := cloneStepPayload(usage)
	usagePayload[events.PayloadKeyWorkspaceID] = string(run.WorkspaceID)
	usagePayload[events.PayloadKeyRunID] = string(run.ID)
	usagePayload[events.PayloadKeyStepID] = string(step.ID)
	if stringFromAny(usagePayload[events.PayloadKeyStatus]) == "" {
		usagePayload[events.PayloadKeyStatus] = string(step.State)
	}
	usageData, ok := events.DecodeLLMUsagePayload(usagePayload)
	if !ok {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		ID:           fmt.Sprintf("%s/event/llm-usage-recorded", step.ID),
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicRunLLMUsageRecorded,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "llm_usage",
		SubjectID:    string(step.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		StepID:       string(step.ID),
		Payload:      events.LLMUsagePayload(usageData),
	})
}
