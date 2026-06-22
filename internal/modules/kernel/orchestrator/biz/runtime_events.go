package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type eventSubsystem struct {
	hub         events.Hub
	checkpoints *checkpointSubsystem
	llmContext  *llmContextSubsystem
}

func newEventSubsystem(hub events.Hub, checkpoints *checkpointSubsystem, llmContext *llmContextSubsystem) *eventSubsystem {
	return &eventSubsystem{
		hub:         hub,
		checkpoints: checkpoints,
		llmContext:  llmContext,
	}
}

func (e *eventSubsystem) checkpointManager() *checkpointSubsystem {
	if e == nil || e.checkpoints == nil {
		return newCheckpointSubsystem(RuntimeEvidenceRepositories{}, nil)
	}
	return e.checkpoints
}

func (e *eventSubsystem) llmContextManager() *llmContextSubsystem {
	if e == nil || e.llmContext == nil {
		return newLLMContextSubsystem()
	}
	return e.llmContext
}

func (e *eventSubsystem) publishRunStateChanged(ctx context.Context, run *runModel.Run) error {
	return e.publishRunStateChangedAtStep(ctx, run, "")
}

func (e *eventSubsystem) publishRunCreated(ctx context.Context, run *runModel.Run) error {
	if e == nil || e.hub == nil || run == nil {
		return nil
	}
	return events.PublishByContract(ctx, e.hub, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        runevents.TopicRunCreated,
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

func (e *eventSubsystem) publishRunStateChangedAtStep(ctx context.Context, run *runModel.Run, currentStepID common.StepID) error {
	if err := e.checkpointManager().recordRunCheckpointAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	if e == nil || e.hub == nil || run == nil {
		return nil
	}
	return events.PublishByContract(ctx, e.hub, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        runevents.TopicRunStateChanged,
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

func (e *eventSubsystem) publishResultEvent(ctx context.Context, item *resultmodel.Result) error {
	if e == nil || item == nil || e.hub == nil {
		return nil
	}
	artifactRefs := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	return events.PublishByContract(ctx, e.hub, events.Envelope{
		ID:           fmt.Sprintf("%s/event/%s", item.ID, resultEventStateSuffix(item.State)),
		Version:      events.WorkorchEventVersion,
		Topic:        resultEventTopic(item.State),
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

func resultEventTopic(state common.ResultState) string {
	switch state {
	case common.ResultStateRejected:
		return resultevents.TopicResultRejected
	case common.ResultStateSuperseded:
		return resultevents.TopicResultSuperseded
	default:
		return resultevents.TopicResultAccepted
	}
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

func (e *eventSubsystem) publishArtifactCreated(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil || e.hub == nil {
		return nil
	}
	return events.PublishByContract(ctx, e.hub, events.Envelope{
		ID:           fmt.Sprintf("%s/event/created", item.ID),
		Version:      events.WorkorchEventVersion,
		Topic:        resultevents.TopicArtifactCreated,
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

func (e *eventSubsystem) publishPatchChanged(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, patch *runModel.Patch, topic string, failure error) error {
	if e.hub == nil || run == nil || patch == nil {
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
	if topic == orchestratorevents.TopicPatchReverted && patch.RevertApprovalID != "" {
		approvalID = patch.RevertApprovalID
	}
	errText := ""
	if failure != nil {
		errText = failure.Error()
	}
	return events.PublishByContract(ctx, e.hub, events.Envelope{
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

func (e *eventSubsystem) publishArtifactContinuationChanged(ctx context.Context, run *runModel.Run, step *runModel.Step, item *orchestratormodel.ArtifactContinuation, topic string, failure error) error {
	if e.hub == nil || run == nil || item == nil {
		return nil
	}
	payload := map[string]any{
		"id":               string(item.ID),
		"runId":            string(item.RunID),
		"artifactId":       item.ArtifactID,
		"targetRef":        item.TargetRef,
		"state":            string(item.State),
		"expectedSequence": item.ExpectedSequence,
		"finalReceived":    item.FinalReceived,
		"bytesWritten":     item.BytesWritten,
	}
	if item.RollingHash != "" {
		payload["rollingHash"] = item.RollingHash
	}
	if item.LastChunkHash != "" {
		payload["lastChunkHash"] = item.LastChunkHash
	}
	if len(item.ReceivedSequences) > 0 {
		payload["receivedSequences"] = append([]int(nil), item.ReceivedSequences...)
	}
	if len(item.CoverageTargets) > 0 {
		payload["coverageTargets"] = append([]string(nil), item.CoverageTargets...)
	}
	if len(item.EvidenceRefs) > 0 {
		payload["evidenceRefs"] = append([]string(nil), item.EvidenceRefs...)
	}
	if failure != nil {
		payload["failureReason"] = failure.Error()
	}
	stepID := common.StepID("")
	if step != nil {
		stepID = step.ID
	}
	return events.PublishByContract(ctx, e.hub, events.Envelope{
		ID:           fmt.Sprintf("%s/event/%s", item.ID, artifactContinuationEventName(topic)),
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "artifact_continuation",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		StepID:       string(stepID),
		Payload:      payload,
	})
}

func artifactContinuationEventName(topic string) string {
	switch topic {
	case orchestratorevents.TopicArtifactContinuationCreated:
		return "created"
	case orchestratorevents.TopicArtifactContinuationChunkRecorded:
		return "chunk-recorded"
	case orchestratorevents.TopicArtifactContinuationChunkRejected:
		return "chunk-rejected"
	case orchestratorevents.TopicArtifactContinuationAssembled:
		return "assembled"
	case orchestratorevents.TopicArtifactContinuationApplied:
		return "applied"
	case orchestratorevents.TopicArtifactContinuationFailed:
		return "failed"
	default:
		return "changed"
	}
}

func (e *eventSubsystem) publishPatchPreview(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, preview map[string]any) error {
	if e.hub == nil || run == nil || step == nil || len(preview) == 0 {
		return nil
	}
	toolCallID := common.ToolCallID("")
	if toolCall != nil {
		toolCallID = toolCall.ID
	}
	targetRef := stringFromAny(preview["targetRef"])
	patchType := stringFromAny(preview["patchType"])
	subjectID := fmt.Sprintf("%s/patch-preview", step.ID)
	return events.PublishByContract(ctx, e.hub, events.Envelope{
		ID:           fmt.Sprintf("%s/event/patch-previewed", subjectID),
		Version:      events.WorkorchEventVersion,
		Topic:        orchestratorevents.TopicPatchPreviewed,
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
	case json.Number:
		ret, _ := typed.Int64()
		return int(ret)
	default:
		return 0
	}
}

func (e *eventSubsystem) publishMemoryChanged(ctx context.Context, topic string, item *memorymodel.MemoryItem) error {
	if item == nil || e.hub == nil {
		return nil
	}
	return events.PublishByContract(ctx, e.hub, events.Envelope{
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

func (e *eventSubsystem) publishWorkspaceStateChanged(ctx context.Context, workspace *workspaceModel.Workspace, runID common.RunID) error {
	if e.hub == nil || workspace == nil {
		return nil
	}
	return events.PublishByContract(ctx, e.hub, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        workspaceevents.TopicWorkspaceStateChanged,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "workspace",
		SubjectID:    string(workspace.ID),
		WorkspaceID:  string(workspace.ID),
		RunID:        string(runID),
		Payload:      events.WorkspacePayload(workspace.Title, workspace.Description, workspace.DeliverableDirectory, workspace.InternalDirectory, workspace.State, workspace.ActiveRunID),
	})
}

func (e *eventSubsystem) publishStepState(ctx context.Context, run *runModel.Run, step *runModel.Step, topic string) error {
	if e.hub == nil || run == nil || step == nil {
		return nil
	}
	if err := events.PublishByContract(ctx, e.hub, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "step",
		SubjectID:    string(step.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		StepID:       string(step.ID),
		Payload:      events.StepPayload(step.Kind, step.State, runtimeStepEventInput(e.llmContextManager(), run, step), runtimeStepEventOutput(e.llmContextManager(), run, step)),
	}); err != nil {
		return err
	}
	if topic == runevents.TopicStepStateChanged {
		if err := e.publishLLMStreamProgressFromStep(ctx, run, step); err != nil {
			return err
		}
		if err := e.publishLLMUsageFromStep(ctx, run, step); err != nil {
			return err
		}
	}
	return nil
}

func (e *eventSubsystem) publishContractViolationRecorded(ctx context.Context, run *runModel.Run, step *runModel.Step) error {
	if e == nil || e.hub == nil || run == nil || step == nil {
		return nil
	}
	violation := contractViolationFromFailedOutput(step)
	if len(violation) == 0 {
		return nil
	}
	return e.publishContractViolationChanged(ctx, orchestratorevents.TopicOrchestratorContractViolationRecorded, run, step, "", "", violation, nil)
}

func (e *eventSubsystem) publishContractViolationRetryRequested(ctx context.Context, run *runModel.Run, sourceStep *runModel.Step, retryStepID common.StepID, violation map[string]any) error {
	if e == nil || e.hub == nil || run == nil || sourceStep == nil || retryStepID == "" {
		return nil
	}
	return e.publishContractViolationChanged(ctx, orchestratorevents.TopicOrchestratorContractViolationRetryRequested, run, sourceStep, retryStepID, "", violation, nil)
}

func (e *eventSubsystem) publishContractViolationEscalated(ctx context.Context, run *runModel.Run, sourceStep *runModel.Step, ticketID common.TicketID, violation map[string]any, context map[string]any) error {
	if e == nil || e.hub == nil || run == nil || sourceStep == nil {
		return nil
	}
	return e.publishContractViolationChanged(ctx, orchestratorevents.TopicOrchestratorContractViolationEscalated, run, sourceStep, "", ticketID, violation, context)
}

func (e *eventSubsystem) publishContractViolationChanged(
	ctx context.Context,
	topic string,
	run *runModel.Run,
	sourceStep *runModel.Step,
	retryStepID common.StepID,
	ticketID common.TicketID,
	violation map[string]any,
	context map[string]any,
) error {
	if e == nil || e.hub == nil || run == nil || sourceStep == nil || len(violation) == 0 {
		return nil
	}
	payload := cloneStepPayload(violation)
	payload["contractViolation"] = cloneStepPayload(violation)
	payload["workspaceId"] = string(run.WorkspaceID)
	payload["runId"] = string(run.ID)
	payload["sourceStepId"] = string(sourceStep.ID)
	payload["stepId"] = string(sourceStep.ID)
	payload["topic"] = topic
	payload["summary"] = contractViolationEventSummary(topic, violation)
	if retryStepID != "" {
		payload["retryStepId"] = string(retryStepID)
	}
	if ticketID != "" {
		payload["ticketId"] = string(ticketID)
	}
	if len(context) > 0 {
		payload["context"] = cloneStepPayload(context)
	}
	return events.PublishByContract(ctx, e.hub, events.Envelope{
		ID:           contractViolationEventID(topic, run, sourceStep, retryStepID, ticketID, violation),
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "contract_violation",
		SubjectID:    contractViolationSubjectID(sourceStep, violation),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		StepID:       string(sourceStep.ID),
		Payload:      payload,
	})
}

func contractViolationEventSummary(topic string, violation map[string]any) string {
	action := strings.TrimPrefix(strings.TrimSpace(topic), "/orchestrator/contract_violation/")
	if action == "" {
		action = "changed"
	}
	rule := strings.TrimSpace(stringFromMap(violation, "violatedRule"))
	if rule == "" {
		rule = "contract_violation"
	}
	return "contract violation " + action + ": " + rule
}

func contractViolationSubjectID(sourceStep *runModel.Step, violation map[string]any) string {
	if id := strings.TrimSpace(stringFromMap(violation, "id")); id != "" {
		return id
	}
	if sourceStep != nil {
		return fmt.Sprintf("%s/contract-violation", sourceStep.ID)
	}
	return "contract-violation"
}

func contractViolationEventID(topic string, run *runModel.Run, sourceStep *runModel.Step, retryStepID common.StepID, ticketID common.TicketID, violation map[string]any) string {
	subjectID := contractViolationSubjectID(sourceStep, violation)
	suffix := strings.TrimPrefix(strings.TrimSpace(topic), "/orchestrator/contract_violation/")
	if suffix == "" {
		suffix = "changed"
	}
	switch {
	case retryStepID != "":
		return fmt.Sprintf("%s/event/%s/%s", subjectID, suffix, retryStepID)
	case ticketID != "":
		return fmt.Sprintf("%s/event/%s/%s", subjectID, suffix, ticketID)
	case run != nil:
		return fmt.Sprintf("%s/event/%s/%s", subjectID, suffix, run.ID)
	default:
		return fmt.Sprintf("%s/event/%s", subjectID, suffix)
	}
}

func (e *eventSubsystem) publishRuntimeStepProgress(ctx context.Context, run *runModel.Run, step *runModel.Step, event string, data map[string]any) error {
	if e.hub == nil || run == nil || step == nil {
		return nil
	}
	if !strings.HasPrefix(strings.TrimSpace(step.Kind), "llm/") {
		return nil
	}
	providerID := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(data, "providerId"),
		stringFromMap(data, "providerID"),
	))
	model := strings.TrimSpace(stringFromMap(data, "model"))
	chunkCount := intFromAny(data["receivedEventCount"])
	if chunkCount == 0 {
		chunkCount = intFromAny(data["receivedDeltaCount"])
	}
	receivedBytes := intFromAny(data["receivedBytes"])
	elapsedMs := intFromAny(data["elapsedMs"])
	status := strings.TrimSpace(string(step.State))
	if status == "" {
		status = "running"
	}
	summary := strings.TrimSpace(firstNonEmptyString(stringFromMap(data, "summary"), event))
	return events.PublishByContract(ctx, e.hub, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        runevents.TopicRunLLMStreamProgress,
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
			summary,
		),
	})
}

func runtimeStepEventInput(llmContext *llmContextSubsystem, run *runModel.Run, step *runModel.Step) map[string]any {
	if run == nil || step == nil {
		return nil
	}
	if llmContext == nil {
		llmContext = newLLMContextSubsystem()
	}
	ret := map[string]any{
		"inputRef": runtimeRawPayloadRef(run.WorkspaceID, run.ID, "steps", string(step.ID), "input.json"),
	}
	if summary := llmContext.compactStepInput(step.Input); len(summary) > 0 {
		ret["summary"] = summary
	}
	return ret
}

func runtimeStepEventOutput(llmContext *llmContextSubsystem, run *runModel.Run, step *runModel.Step) map[string]any {
	if run == nil || step == nil || len(step.Output) == 0 {
		return nil
	}
	if llmContext == nil {
		llmContext = newLLMContextSubsystem()
	}
	ret := map[string]any{
		"outputRef": runtimeRawPayloadRef(run.WorkspaceID, run.ID, "steps", string(step.ID), "output.json"),
	}
	if summary := llmContext.compactStepOutput(step.Output); len(summary) > 0 {
		ret["summary"] = summary
	}
	return ret
}

func (e *eventSubsystem) publishLLMStreamProgressFromStep(ctx context.Context, run *runModel.Run, step *runModel.Step) error {
	if e.hub == nil || run == nil || step == nil || len(step.Output) == 0 {
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
	return events.PublishByContract(ctx, e.hub, events.Envelope{
		ID:           fmt.Sprintf("%s/event/llm-stream-progress", step.ID),
		Version:      events.WorkorchEventVersion,
		Topic:        runevents.TopicRunLLMStreamProgress,
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

func (e *eventSubsystem) publishLLMUsageFromStep(ctx context.Context, run *runModel.Run, step *runModel.Step) error {
	if e.hub == nil || run == nil || step == nil || len(step.Output) == 0 {
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
	if _, ok := usagePayload[events.PayloadKeyCacheUsage]; !ok {
		if cacheUsage := mapFromValue(usagePayload["cacheUsage"]); len(cacheUsage) > 0 {
			usagePayload[events.PayloadKeyCacheUsage] = cacheUsage
		}
	}
	if stringFromAny(usagePayload[events.PayloadKeyStatus]) == "" {
		usagePayload[events.PayloadKeyStatus] = string(step.State)
	}
	usageData, ok := events.DecodeLLMUsagePayload(usagePayload)
	if !ok {
		return nil
	}
	return events.PublishByContract(ctx, e.hub, events.Envelope{
		ID:           fmt.Sprintf("%s/event/llm-usage-recorded", step.ID),
		Version:      events.WorkorchEventVersion,
		Topic:        runevents.TopicRunLLMUsageRecorded,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "llm_usage",
		SubjectID:    string(step.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		StepID:       string(step.ID),
		Payload:      events.LLMUsagePayload(usageData),
	})
}
