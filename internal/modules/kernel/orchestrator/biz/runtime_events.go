package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type eventSubsystem struct {
	notificationHub magicevent.Hub
	checkpoints     *checkpointSubsystem
	llmContext      *llmContextSubsystem
}

func newEventSubsystem(checkpoints *checkpointSubsystem, llmContext *llmContextSubsystem) *eventSubsystem {
	return &eventSubsystem{
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
	if e == nil || run == nil {
		return nil
	}
	return e.publishTypedRunState(ctx, runevents.TopicRunCreated, run)
}

func (e *eventSubsystem) publishRunStateChangedAtStep(ctx context.Context, run *runModel.Run, currentStepID common.StepID) error {
	if err := e.checkpointManager().recordRunCheckpointAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	if e == nil || run == nil {
		return nil
	}
	return e.publishTypedRunState(ctx, runevents.TopicRunStateChanged, run)
}

func (e *eventSubsystem) publishTypedRunState(ctx context.Context, topic string, run *runModel.Run) error {
	if e == nil || e.notificationHub == nil || run == nil {
		return nil
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, runevents.RunStateChangedDataFromModel(run))
	event.BindLaneKey(runtimeStateNotificationLaneKey(run.WorkspaceID, run.ID))
	e.notificationHub.Post(event)
	return nil
}

func runtimeStateNotificationLaneKey(workspaceID common.WorkspaceID, runID common.RunID) string {
	return events.EventHubLaneKeyFor(orchestratorevents.TopicOrchestratorCommandStartRun, events.EventHubLaneScope{
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	})
}

func (e *eventSubsystem) publishPatchChanged(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, patch *runModel.Patch, topic string, failure error) error {
	if e == nil || run == nil || patch == nil {
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
	data := orchestratorevents.PatchChangedData{
		PatchID: patch.ID, SubjectID: string(patch.ID), WorkspaceID: patch.WorkspaceID, RunID: patch.RunID,
		StepID: stepID, ToolCallID: toolCallID, PatchType: patch.PatchType, TargetRef: patch.TargetRef,
		Summary: patch.Summary, State: patch.State, AddedLines: addedLines, DeletedLines: deletedLines,
		ContentHash: contentHash, DiffHash: diffHash, ApprovalID: approvalID, Risk: patch.RiskLevel, Error: errText,
	}
	return e.publishTypedPatch(ctx, topic, data)
}

func (e *eventSubsystem) publishArtifactContinuationChanged(ctx context.Context, run *runModel.Run, step *runModel.Step, item *orchestratormodel.ArtifactContinuation, topic string, failure error) error {
	if e == nil || run == nil || item == nil {
		return nil
	}
	stepID := common.StepID("")
	if step != nil {
		stepID = step.ID
	}
	failureReason := ""
	if failure != nil {
		failureReason = failure.Error()
	}
	data := orchestratorevents.ArtifactContinuationChangedData{
		ContinuationID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: stepID,
		ArtifactID: item.ArtifactID, TargetRef: item.TargetRef, State: item.State,
		ExpectedSequence: item.ExpectedSequence, FinalReceived: item.FinalReceived, BytesWritten: item.BytesWritten,
		RollingHash: item.RollingHash, LastChunkHash: item.LastChunkHash,
		ReceivedSequences: append([]int(nil), item.ReceivedSequences...), CoverageTargets: append([]string(nil), item.CoverageTargets...),
		EvidenceRefs: append([]string(nil), item.EvidenceRefs...), FailureReason: failureReason,
	}
	if e.notificationHub == nil {
		return nil
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, orchestratorevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{SubjectType: "artifact_continuation", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID), StepID: string(stepID)}))
	e.notificationHub.Post(event)
	return nil
}

func (e *eventSubsystem) publishPatchPreview(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, preview map[string]any) error {
	if e == nil || run == nil || step == nil || len(preview) == 0 {
		return nil
	}
	toolCallID := common.ToolCallID("")
	if toolCall != nil {
		toolCallID = toolCall.ID
	}
	targetRef := stringFromAny(preview["targetRef"])
	patchType := stringFromAny(preview["patchType"])
	subjectID := fmt.Sprintf("%s/patch-preview", step.ID)
	data := orchestratorevents.PatchChangedData{
		SubjectID: subjectID, WorkspaceID: run.WorkspaceID, RunID: run.ID, StepID: step.ID, ToolCallID: toolCallID,
		PatchType: patchType, TargetRef: targetRef, Summary: stringFromAny(preview["summary"]), State: common.PatchStatePreviewed,
		AddedLines: intFromAny(preview["addedLines"]), DeletedLines: intFromAny(preview["deletedLines"]),
		ContentHash: stringFromAny(preview["contentHash"]), DiffHash: stringFromAny(preview["diffHash"]),
		ApprovalID: common.ApprovalID(stringFromAny(preview["approvalId"])), Risk: stringFromAny(preview["risk"]),
	}
	return e.publishTypedPatch(ctx, orchestratorevents.TopicPatchPreviewed, data)
}

func (e *eventSubsystem) publishTypedPatch(ctx context.Context, topic string, data orchestratorevents.PatchChangedData) error {
	if e == nil || e.notificationHub == nil {
		return nil
	}
	subjectID := data.SubjectID
	if subjectID == "" {
		subjectID = string(data.PatchID)
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, orchestratorevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{SubjectType: "patch", SubjectID: subjectID, WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), StepID: string(data.StepID)}))
	e.notificationHub.Post(event)
	return nil
}

func patchLineStats(patch *runModel.Patch) (int, int) {
	if patch == nil {
		return 0, 0
	}
	if patch.AddedLines != 0 || patch.DeletedLines != 0 {
		return patch.AddedLines, patch.DeletedLines
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

func (e *eventSubsystem) publishWorkspaceStateChanged(ctx context.Context, workspace *workspaceModel.Workspace, runID common.RunID) error {
	if e == nil || workspace == nil {
		return nil
	}
	if e.notificationHub == nil {
		return nil
	}
	data := workspaceevents.WorkspaceChangedData{
		WorkspaceID:          workspace.ID,
		Title:                workspace.Title,
		Description:          workspace.Description,
		DeliverableDirectory: workspace.DeliverableDirectory,
		InternalDirectory:    workspace.InternalDirectory,
		State:                workspace.State,
		ActiveRunID:          workspace.ActiveRunID,
		RunID:                runID,
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceStateChanged, modulecommon.ModuleID, workspaceevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(runtimeStateNotificationLaneKey(workspace.ID, runID))
	e.notificationHub.Post(event)
	return nil
}

func (e *eventSubsystem) publishStepState(ctx context.Context, run *runModel.Run, step *runModel.Step, topic string) error {
	if e == nil || run == nil || step == nil {
		return nil
	}
	if e.notificationHub == nil {
		return nil
	}
	input, err := typedStepSnapshot(runtimeStepEventInput(e.llmContextManager(), run, step))
	if err != nil {
		return err
	}
	output, err := typedStepSnapshot(runtimeStepEventOutput(e.llmContextManager(), run, step))
	if err != nil {
		return err
	}
	data := runevents.StepChangedData{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      step.ID,
		Kind:        step.Kind,
		State:       step.State,
		Input:       input,
		Output:      output,
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{
		SubjectType: "step",
		SubjectID:   string(step.ID),
		WorkspaceID: string(run.WorkspaceID),
		RunID:       string(run.ID),
		StepID:      string(step.ID),
	}))
	e.notificationHub.Post(event)
	if topic == runevents.TopicStepStateChanged {
		if err := e.publishLLMStreamProgressFromStep(ctx, run, step); err != nil {
			return err
		}
		return e.publishLLMUsageFromStep(ctx, run, step)
	}
	return nil
}

func typedStepSnapshot(payload map[string]any) (runevents.StepSnapshot, error) {
	if len(payload) == 0 {
		return runevents.StepSnapshot{}, nil
	}
	ref := firstNonEmptyString(stringFromMap(payload, "inputRef"), stringFromMap(payload, "outputRef"))
	summary, _ := payload["summary"].(map[string]any)
	typedSummary, err := runevents.NotificationPayloadFromMap(summary)
	if err != nil {
		return runevents.StepSnapshot{}, err
	}
	return runevents.StepSnapshot{Ref: ref, Summary: typedSummary}, nil
}

func (e *eventSubsystem) publishContractViolationRecorded(ctx context.Context, run *runModel.Run, step *runModel.Step) error {
	if e == nil || run == nil || step == nil {
		return nil
	}
	violation := contractViolationFromFailedOutput(step)
	if len(violation) == 0 {
		return nil
	}
	return e.publishContractViolationChanged(ctx, orchestratorevents.TopicOrchestratorContractViolationRecorded, run, step, "", "", violation, nil)
}

func (e *eventSubsystem) publishContractViolationRetryRequested(ctx context.Context, run *runModel.Run, sourceStep *runModel.Step, retryStepID common.StepID, violation map[string]any) error {
	if e == nil || run == nil || sourceStep == nil || retryStepID == "" {
		return nil
	}
	return e.publishContractViolationChanged(ctx, orchestratorevents.TopicOrchestratorContractViolationRetryRequested, run, sourceStep, retryStepID, "", violation, nil)
}

func (e *eventSubsystem) publishContractViolationEscalated(ctx context.Context, run *runModel.Run, sourceStep *runModel.Step, ticketID common.TicketID, violation map[string]any, context map[string]any) error {
	if e == nil || run == nil || sourceStep == nil {
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
	if e == nil || run == nil || sourceStep == nil || len(violation) == 0 {
		return nil
	}
	if e.notificationHub == nil {
		return nil
	}
	typedViolation, err := orchestratorevents.StructuredPayloadFromMap(violation)
	if err != nil {
		return err
	}
	typedContext, err := orchestratorevents.StructuredPayloadFromMap(context)
	if err != nil {
		return err
	}
	data := orchestratorevents.ContractViolationData{
		SubjectID: contractViolationSubjectID(sourceStep, violation), WorkspaceID: run.WorkspaceID, RunID: run.ID,
		SourceStepID: sourceStep.ID, RetryStepID: retryStepID, TicketID: ticketID, Topic: topic,
		Summary: contractViolationEventSummary(topic, violation), Violation: typedViolation, Context: typedContext,
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, orchestratorevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{SubjectType: "contract_violation", SubjectID: data.SubjectID, WorkspaceID: string(run.WorkspaceID), RunID: string(run.ID), StepID: string(sourceStep.ID)}))
	e.notificationHub.Post(event)
	return nil
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

func (e *eventSubsystem) publishRuntimeStepProgress(ctx context.Context, run *runModel.Run, step *runModel.Step, event string, data map[string]any) error {
	if e == nil || run == nil || step == nil {
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
	return e.publishTypedLLMStreamProgress(ctx, runevents.LLMStreamProgressData{
		WorkspaceID:   run.WorkspaceID,
		RunID:         run.ID,
		StepID:        step.ID,
		ProviderID:    providerID,
		Model:         model,
		ChunkCount:    chunkCount,
		ReceivedBytes: receivedBytes,
		ElapsedMs:     elapsedMs,
		Status:        status,
		Summary:       summary,
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
	if e == nil || run == nil || step == nil || len(step.Output) == 0 {
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
	data := runevents.LLMStreamProgressData{
		WorkspaceID:   run.WorkspaceID,
		RunID:         run.ID,
		StepID:        step.ID,
		ProviderID:    providerID,
		Model:         model,
		ChunkCount:    chunkCount,
		ReceivedBytes: receivedBytes,
		ElapsedMs:     elapsedMs,
		Status:        status,
		Summary:       streamProgressSummary(step.Output),
	}
	return e.publishTypedLLMStreamProgress(ctx, data)
}

func (e *eventSubsystem) publishTypedLLMStreamProgress(ctx context.Context, data runevents.LLMStreamProgressData) error {
	if e == nil || e.notificationHub == nil {
		return nil
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunLLMStreamProgress, modulecommon.ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(events.EventHubLaneKeyFor(runevents.TopicRunLLMStreamProgress, events.EventHubLaneScope{
		SubjectType: "llm_stream_progress",
		SubjectID:   string(data.StepID),
		WorkspaceID: string(data.WorkspaceID),
		RunID:       string(data.RunID),
		StepID:      string(data.StepID),
	}))
	e.notificationHub.Post(event)
	return nil
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
	if e == nil || run == nil || step == nil || len(step.Output) == 0 {
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
	contextBudget, err := runevents.NotificationPayloadFromMap(usageData.ContextBudget)
	if err != nil {
		return err
	}
	cacheUsage, err := runevents.NotificationPayloadFromMap(usageData.CacheUsage)
	if err != nil {
		return err
	}
	cacheMarkers, err := runevents.NotificationPayloadFromMap(usageData.CacheMarkers)
	if err != nil {
		return err
	}
	typedData := runevents.LLMUsageRecordedData{
		WorkspaceID:   usageData.WorkspaceID,
		RunID:         usageData.RunID,
		StepID:        usageData.StepID,
		ProviderID:    usageData.ProviderID,
		Provider:      usageData.Provider,
		APISurface:    usageData.APISurface,
		Model:         usageData.Model,
		Operation:     usageData.Operation,
		InputTokens:   usageData.InputTokens,
		OutputTokens:  usageData.OutputTokens,
		TotalTokens:   usageData.TotalTokens,
		Estimated:     usageData.Estimated,
		UsageSource:   usageData.UsageSource,
		RequestBytes:  usageData.RequestBytes,
		ResponseBytes: usageData.ResponseBytes,
		Streaming:     usageData.Streaming,
		ContextBudget: contextBudget,
		CacheUsage:    cacheUsage,
		CacheMarkers:  cacheMarkers,
		StartedAt:     usageData.StartedAt,
		CompletedAt:   usageData.CompletedAt,
		DurationMs:    usageData.DurationMs,
		Status:        usageData.Status,
		FailureClass:  usageData.FailureClass,
	}
	if e.notificationHub == nil {
		return nil
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunLLMUsageRecorded, modulecommon.ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, typedData)
	event.BindLaneKey(events.EventHubLaneKeyFor(runevents.TopicRunLLMUsageRecorded, events.EventHubLaneScope{
		SubjectType: "llm_usage",
		SubjectID:   string(step.ID),
		WorkspaceID: string(run.WorkspaceID),
		RunID:       string(run.ID),
		StepID:      string(step.ID),
	}))
	e.notificationHub.Post(event)
	return nil
}
