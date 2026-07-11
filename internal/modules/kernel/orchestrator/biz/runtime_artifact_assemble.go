package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) handleArtifactOutputContinuation(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	workspace *workspaceModel.Workspace,
) (bool, error) {
	if o == nil || run == nil || step == nil || output == nil || !outputModeIsIncrementalArtifact(step.Input) {
		return false, nil
	}
	if o.evidence.ArtifactContinuations == nil {
		return false, fmt.Errorf("%w: artifact continuation repository is required", common.ErrInvalidArgument)
	}
	switch artifactOutputPhaseFromInput(step.Input) {
	case artifactOutputPhaseContext:
		return false, nil
	case artifactOutputPhaseChunk:
		return true, o.recordArtifactContinuationChunk(ctx, run, step, toolCall, output, workspace)
	case artifactOutputPhaseChunkRepair:
		return true, o.recordArtifactContinuationChunkRepair(ctx, run, step, toolCall, output)
	default:
		return true, o.createArtifactContinuationsFromPlan(ctx, run, step, toolCall, output, workspace)
	}
}

func (o *RuntimeOrchestrator) createArtifactContinuationsFromPlan(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, output map[string]any, workspace *workspaceModel.Workspace) error {
	plan, ok, err := artifactOutputMap(output, "artifactPlan")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: artifactPlan is required for artifact continuation", common.ErrInvalidArgument)
	}
	artifacts := mapSliceFromValue(plan["artifacts"])
	if len(artifacts) == 0 {
		return fmt.Errorf("%w: artifactPlan.artifacts is required", common.ErrInvalidArgument)
	}
	created := make([]map[string]any, 0, len(artifacts))
	createdItems := make([]*orchestratormodel.ArtifactContinuation, 0, len(artifacts))
	now := runtimeEvidenceTimestamp()
	for idx, artifact := range artifacts {
		artifactID := strings.TrimSpace(stringFromMap(artifact, "artifactId"))
		targetRef := strings.TrimSpace(stringFromMap(artifact, "targetRef"))
		operation := strings.TrimSpace(stringFromMap(artifact, "operation"))
		continuationID := artifactContinuationID(run.ID, step.ID, artifactID, idx)
		item := &orchestratormodel.ArtifactContinuation{
			ID:               continuationID,
			WorkspaceID:      run.WorkspaceID,
			RunID:            run.ID,
			RootStepID:       step.ID,
			CurrentStepID:    step.ID,
			ArtifactID:       artifactID,
			TargetRef:        targetRef,
			Operation:        operation,
			OutputMode:       outputModeIncrementalArtifact,
			CoverageTargets:  stringSliceFromValue(artifact["coverageTargets"]),
			State:            common.ArtifactContinuationStateChunking,
			ExpectedSequence: 1,
			CreatedAt:        now,
			UpdatedAt:        now,
			Metadata: map[string]any{
				"estimatedChunks": intFromAny(artifact["estimatedChunks"]),
				"estimatedBytes":  intFromAny(artifact["estimatedBytes"]),
			},
		}
		if err := o.evidence.ArtifactContinuations.Create(ctx, item); err != nil {
			return err
		}
		if err := o.evidence.ArtifactContinuations.SavePlan(ctx, item.ID, artifact); err != nil {
			return err
		}
		if err := o.eventPublisher().publishArtifactContinuationChanged(ctx, run, step, item, orchestratorevents.TopicArtifactContinuationCreated, nil); err != nil {
			return err
		}
		created = append(created, artifactContinuationOutputSummary(item))
		createdItems = append(createdItems, item)
	}
	output["artifactContinuations"] = created
	output["stateChangingEvidenceDeferredUntilApply"] = true
	if err := o.completeArtifactContinuationLLMStep(ctx, run, step, toolCall, output); err != nil {
		return err
	}
	if len(createdItems) == 0 {
		return nil
	}
	return o.RequestUniqueDispatchStep(ctx, run, artifactContinuationLLMStepID(run.ID, createdItems[0]), initialStepKind(run), artifactContinuationNextStepInput(step, createdItems[0]), workspaceIDForDispatch(run, workspace))
}

func (o *RuntimeOrchestrator) recordArtifactContinuationChunk(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, output map[string]any, workspace *workspaceModel.Workspace) error {
	chunk, ok, err := artifactOutputMap(output, "artifactChunk")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: artifactChunk is required for artifact continuation", common.ErrInvalidArgument)
	}
	context := mapFromValue(step.Input["artifactContinuationContext"])
	continuationID := common.ArtifactContinuationID(strings.TrimSpace(firstNonEmptyString(
		stringFromMap(context, "id"),
		stringFromMap(context, "artifactContinuationId"),
		stringFromMap(step.Input, "artifactContinuationId"),
	)))
	if continuationID == "" {
		return fmt.Errorf("%w: artifact continuation id is required", common.ErrInvalidArgument)
	}
	item, err := o.evidence.ArtifactContinuations.Get(ctx, continuationID)
	if err != nil {
		return err
	}
	sequence := intFromAny(chunk["sequence"])
	content := stringRawFromMap(chunk, "content")
	contentHash := normalizeArtifactChunkHash(stringFromMap(chunk, "chunkHash"))
	if contentHash == "" {
		contentHash = sha256HexBytes([]byte(content))
	}
	now := runtimeEvidenceTimestamp()
	chunkMeta := &orchestratormodel.ArtifactContinuationChunk{
		ContinuationID:    item.ID,
		Sequence:          sequence,
		ArtifactID:        item.ArtifactID,
		TargetRef:         item.TargetRef,
		ContentHash:       contentHash,
		Bytes:             int64(len([]byte(content))),
		PreviousChunkTail: stringRawFromMap(context, "previousChunkTail"),
		CurrentChunkTail:  artifactContinuationTail(content, artifactPreviousTailBytesFromInput(step.Input)),
		StepRefs:          []common.StepID{step.ID},
		CreatedAt:         now,
	}
	if err := o.evidence.ArtifactContinuations.SaveChunk(ctx, chunkMeta, content); err != nil {
		return err
	}
	item.ReceivedSequences = appendArtifactContinuationSequence(item.ReceivedSequences, sequence)
	if item.ReceivedChunkHashes == nil {
		item.ReceivedChunkHashes = map[int]string{}
	}
	item.ReceivedChunkHashes[sequence] = contentHash
	item.ExpectedSequence = nextArtifactContinuationSequence(item.ReceivedSequences)
	item.BytesWritten += int64(len([]byte(content)))
	item.LastChunkHash = contentHash
	item.PreviousChunkTail = chunkMeta.CurrentChunkTail
	item.CurrentStepID = step.ID
	item.OutputMode = outputModeIncrementalArtifact
	item.UpdatedAt = now
	if final, _ := chunk["isFinal"].(bool); final {
		item.FinalReceived = true
		item.State = common.ArtifactContinuationStateAssembling
	}
	if err := o.evidence.ArtifactContinuations.Update(ctx, item); err != nil {
		return err
	}
	if err := o.eventPublisher().publishArtifactContinuationChanged(ctx, run, step, item, orchestratorevents.TopicArtifactContinuationChunkRecorded, nil); err != nil {
		return err
	}
	output["artifactContinuationContext"] = artifactContinuationContext(item)
	output["artifactChunk"] = compactArtifactChunkForLLMContext(chunk)
	if decision := mapFromValue(output[stepOutputKeyLLMDecision]); len(decision) > 0 {
		if decisionChunk := mapFromValue(decision["artifactChunk"]); len(decisionChunk) > 0 {
			decision["artifactChunk"] = compactArtifactChunkForLLMContext(decisionChunk)
			output[stepOutputKeyLLMDecision] = decision
		}
	}
	if err := o.completeArtifactContinuationLLMStep(ctx, run, step, toolCall, output); err != nil {
		return err
	}
	if !item.FinalReceived {
		return o.RequestUniqueDispatchStep(ctx, run, artifactContinuationLLMStepID(run.ID, item), initialStepKind(run), artifactContinuationNextStepInput(step, item), workspaceIDForDispatch(run, workspace))
	}
	return o.RequestUniqueDispatchStep(ctx, run, artifactContinuationAssembleStepID(run.ID, item.ID), artifactAssembleStepKind, artifactContinuationInternalStepInput(item), workspaceIDForDispatch(run, workspace))
}

func (o *RuntimeOrchestrator) recordArtifactContinuationChunkRepair(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, output map[string]any) error {
	repair, ok, err := artifactOutputMap(output, "artifactChunkRepair")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: artifactChunkRepair is required for artifact continuation", common.ErrInvalidArgument)
	}
	context := mapFromValue(step.Input["artifactContinuationContext"])
	continuationID := common.ArtifactContinuationID(strings.TrimSpace(firstNonEmptyString(
		stringFromMap(context, "id"),
		stringFromMap(context, "artifactContinuationId"),
		stringFromMap(step.Input, "artifactContinuationId"),
	)))
	if continuationID == "" {
		return nil
	}
	item, err := o.evidence.ArtifactContinuations.Get(ctx, continuationID)
	if err != nil {
		return err
	}
	item.UpdatedAt = runtimeEvidenceTimestamp()
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	item.Metadata["lastChunkRepair"] = compactArtifactChunkRepairForLLMContext(repair)
	item.CurrentStepID = step.ID
	item.OutputMode = outputModeIncrementalArtifact
	if strings.TrimSpace(stringFromMap(repair, "nextAction")) == "blocker" {
		item.State = common.ArtifactContinuationStateFailed
		item.Metadata["failureReason"] = stringFromMap(repair, "failureReason")
		if err := o.evidence.ArtifactContinuations.Update(ctx, item); err != nil {
			return err
		}
		return o.eventPublisher().publishArtifactContinuationChanged(ctx, run, step, item, orchestratorevents.TopicArtifactContinuationFailed, fmt.Errorf("%s", stringFromMap(repair, "failureReason")))
	}
	if err := o.evidence.ArtifactContinuations.Update(ctx, item); err != nil {
		return err
	}
	output["artifactContinuationContext"] = artifactContinuationContext(item)
	return o.completeArtifactContinuationLLMStep(ctx, run, step, toolCall, output)
}

func (o *RuntimeOrchestrator) publishArtifactContinuationRejected(ctx context.Context, run *runModel.Run, step *runModel.Step, output map[string]any, cause error) error {
	if o == nil || o.evidence.ArtifactContinuations == nil || run == nil || step == nil || cause == nil {
		return nil
	}
	continuationID := common.ArtifactContinuationID(strings.TrimSpace(firstNonEmptyString(
		stringFromMap(mapFromValue(step.Input["artifactContinuationContext"]), "id"),
		stringFromMap(step.Input, "artifactContinuationId"),
	)))
	if continuationID == "" {
		return nil
	}
	item, err := o.evidence.ArtifactContinuations.Get(ctx, continuationID)
	if err != nil {
		return nil
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	item.Metadata["lastRejectedOutput"] = map[string]any{
		"stepId": string(step.ID),
		"reason": cause.Error(),
		"stage":  stringFromMap(output, "stage"),
	}
	item.CurrentStepID = step.ID
	item.OutputMode = outputModeIncrementalArtifact
	item.UpdatedAt = runtimeEvidenceTimestamp()
	if err := o.evidence.ArtifactContinuations.Update(ctx, item); err != nil {
		return err
	}
	return o.eventPublisher().publishArtifactContinuationChanged(ctx, run, step, item, orchestratorevents.TopicArtifactContinuationChunkRejected, cause)
}

func (o *RuntimeOrchestrator) resumeArtifactContinuationAfterContextGraph(ctx context.Context, run *runModel.Run, step *runModel.Step, output map[string]any, workspace *workspaceModel.Workspace) (bool, error) {
	if o == nil || o.evidence.ArtifactContinuations == nil || run == nil || step == nil || step.Input == nil {
		return false, nil
	}
	contextStep := step
	if outputModeFromInput(contextStep.Input) != outputModeIncrementalArtifact ||
		artifactOutputPhaseFromInput(contextStep.Input) != artifactOutputPhaseContext {
		rootStepID := graphRootStepID(step)
		if rootStepID == "" || rootStepID == step.ID || o.steps == nil {
			return false, nil
		}
		rootStep, err := o.steps.Get(ctx, rootStepID)
		if err != nil {
			return true, err
		}
		contextStep = rootStep
	}
	if contextStep == nil || contextStep.Input == nil ||
		outputModeFromInput(contextStep.Input) != outputModeIncrementalArtifact ||
		artifactOutputPhaseFromInput(contextStep.Input) != artifactOutputPhaseContext {
		return false, nil
	}
	if _, ok := runtimeGraphSpecFromStep(contextStep); !ok {
		return false, nil
	}
	continuationID := common.ArtifactContinuationID(strings.TrimSpace(firstNonEmptyString(
		stringFromMap(mapFromValue(contextStep.Input["artifactContinuationContext"]), "id"),
		stringFromMap(contextStep.Input, "artifactContinuationId"),
	)))
	if continuationID == "" {
		return true, fmt.Errorf("%w: artifact context continuation id is required", common.ErrInvalidArgument)
	}
	item, err := o.evidence.ArtifactContinuations.Get(ctx, continuationID)
	if err != nil {
		return true, err
	}
	if item.RunID != run.ID || item.WorkspaceID != run.WorkspaceID {
		return true, common.ErrConflict
	}
	if item.State != common.ArtifactContinuationStatePlanning &&
		item.State != common.ArtifactContinuationStateChunking {
		return true, nil
	}
	nextInput := artifactContinuationNextStepInput(contextStep, item)
	nextInput["artifactContextEvidence"] = compactStepOutputForLLMContext(output)
	return true, o.RequestUniqueDispatchStep(ctx, run, artifactContinuationLLMStepID(run.ID, item), initialStepKind(run), nextInput, workspaceIDForDispatch(run, workspace))
}

func (o *RuntimeOrchestrator) completeArtifactContinuationLLMStep(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, output map[string]any) error {
	if o == nil || o.steps == nil || step == nil {
		return nil
	}
	return o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output)
}

func assembleArtifactContinuation(ctx context.Context, repo repository.ArtifactContinuationRepository, item *orchestratormodel.ArtifactContinuation) (*orchestratormodel.ArtifactContinuationAssembledMetadata, string, error) {
	if repo == nil || item == nil || item.ID == "" {
		return nil, "", fmt.Errorf("%w: artifact continuation repository and item are required", common.ErrInvalidArgument)
	}
	if !item.FinalReceived {
		return nil, "", fmt.Errorf("%w: artifact continuation final chunk has not been received", common.ErrInvalidArgument)
	}
	sequences, err := artifactContinuationAssembleSequences(item)
	if err != nil {
		return nil, "", err
	}

	var assembled strings.Builder
	sourceRefs := make([]string, 0, len(sequences))
	for _, sequence := range sequences {
		chunk, content, err := repo.GetChunk(ctx, item.ID, sequence)
		if err != nil {
			return nil, "", fmt.Errorf("get artifact chunk %03d: %w", sequence, err)
		}
		if err := validateArtifactContinuationAssembleChunk(item, chunk, content, sequence); err != nil {
			return nil, "", err
		}
		assembled.WriteString(content)
		sourceRefs = append(sourceRefs, fmt.Sprintf("%03d", sequence))
	}

	content := assembled.String()
	bytes := int64(len([]byte(content)))
	rollingHash := sha256HexBytes([]byte(content))
	if item.RollingHash != "" && !artifactChunkHashesEqual(item.RollingHash, rollingHash) {
		return nil, "", fmt.Errorf("%w: artifact continuation rollingHash does not match assembled content", common.ErrInvalidArgument)
	}
	if item.BytesWritten > 0 && item.BytesWritten != bytes {
		return nil, "", fmt.Errorf("%w: artifact continuation bytesWritten does not match assembled content", common.ErrInvalidArgument)
	}

	metadata := &orchestratormodel.ArtifactContinuationAssembledMetadata{
		ContinuationID:  item.ID,
		TargetRef:       item.TargetRef,
		Bytes:           bytes,
		RollingHash:     rollingHash,
		SourceChunkRefs: sourceRefs,
		CreatedAt:       runtimeEvidenceTimestamp(),
		Metadata: map[string]any{
			"artifactId": item.ArtifactID,
			"operation":  item.Operation,
		},
	}
	if err := repo.SaveAssembled(ctx, metadata, content); err != nil {
		return nil, "", err
	}
	item.State = common.ArtifactContinuationStateApplying
	item.BytesWritten = bytes
	item.RollingHash = rollingHash
	item.OutputMode = outputModeIncrementalArtifact
	item.UpdatedAt = runtimeEvidenceTimestamp()
	if err := repo.Update(ctx, item); err != nil {
		return nil, "", err
	}
	return metadata, content, nil
}

func artifactContinuationID(runID common.RunID, stepID common.StepID, artifactID string, index int) common.ArtifactContinuationID {
	artifactID = runtimeReadableIDPart(artifactID)
	if artifactID == "" {
		artifactID = fmt.Sprintf("%03d", index+1)
	}
	return common.ArtifactContinuationID(runtimeChildID(common.StepID(fmt.Sprintf("%s/artifact-continuation", runID)), runtimeChildID(common.StepID(runtimeReadableStepLabel(runID, stepID)), artifactID)))
}

func artifactContinuationOutputSummary(item *orchestratormodel.ArtifactContinuation) map[string]any {
	if item == nil {
		return nil
	}
	ret := map[string]any{
		"id":               string(item.ID),
		"artifactId":       item.ArtifactID,
		"targetRef":        item.TargetRef,
		"operation":        item.Operation,
		"state":            string(item.State),
		"expectedSequence": item.ExpectedSequence,
	}
	if len(item.CoverageTargets) > 0 {
		ret["coverageTargets"] = append([]string(nil), item.CoverageTargets...)
	}
	return ret
}

func artifactContinuationInternalStepInput(item *orchestratormodel.ArtifactContinuation) map[string]any {
	return map[string]any{
		"artifactContinuationId":      string(item.ID),
		"artifactContinuationContext": artifactContinuationContext(item),
	}
}

func artifactContinuationNextStepInput(previous *runModel.Step, item *orchestratormodel.ArtifactContinuation) map[string]any {
	input := map[string]any{}
	if previous != nil {
		for _, key := range []string{
			"stateChangingOutputRequired",
			"repairObjective",
			"repairActionPackage",
			"llmOutputPolicy",
			stepInputKeyExecutionContextPackage,
			coverageWorkItemsKey,
			coverageWorkItemContextKey,
			stepInputKeySoulStage,
		} {
			if value, ok := previous.Input[key]; ok {
				input[key] = value
			}
		}
	}
	normalizeArtifactContinuationChunkInput(input)
	input["stateChangingOutputRequired"] = true
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["outputMode"] = outputModeIncrementalArtifact
	input["artifactContinuationId"] = string(item.ID)
	input["artifactContinuationContext"] = artifactContinuationContext(item)
	input["artifactOutputContract"] = map[string]any{
		"mode":                  outputModeIncrementalArtifact,
		"phase":                 artifactOutputPhaseChunk,
		"artifactId":            item.ArtifactID,
		"targetRef":             item.TargetRef,
		"expectedSequence":      item.ExpectedSequence,
		"maxArtifactChunkBytes": artifactMaxChunkBytesFromInput(input),
		"schema": map[string]any{
			"completionState": "continue",
			"requiredFields": []string{
				"artifactChunk.artifactId",
				"artifactChunk.targetRef",
				"artifactChunk.sequence",
				"artifactChunk.content",
				"artifactChunk.chunkHash",
				"artifactChunk.isFinal",
			},
			"mutuallyExclusiveWith": []string{"executionGraph", "artifactPlan", "dodAssessment", "memoryWrites"},
		},
	}
	return input
}

func normalizeArtifactContinuationChunkInput(input map[string]any) {
	if len(input) == 0 {
		return
	}
	delete(input, "requestedArtifactContext")
	delete(input, "requestedArtifactOutputPhase")
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		return
	}
	required := mapFromValue(pkg["requiredOutput"])
	if len(required) == 0 {
		return
	}
	clearIncrementalArtifactRequiredOutput(required)
	required["outputMode"] = outputModeIncrementalArtifact
	required["mustReturnArtifactChunk"] = true
	required["stateChangingEvidenceDeferredUntilApply"] = true
	required["maxArtifactChunkBytes"] = artifactMaxChunkBytesFromInput(input)
	required["previousChunkTailBytes"] = artifactPreviousTailBytesFromInput(input)
	required["forbiddenTopLevelOutputs"] = []string{"executionGraph", "dodAssessment", "artifactPlan", "memoryWrites"}
	required["artifactChunkSchema"] = artifactChunkRequiredOutputSchema()
	delete(required, "mustIncludeStateChangingNode")
	pkg["requiredOutput"] = required
	input["repairActionPackage"] = pkg
}

func appendArtifactContinuationSequence(existing []int, sequence int) []int {
	if sequence <= 0 {
		return append([]int(nil), existing...)
	}
	ret := append([]int(nil), existing...)
	for _, item := range ret {
		if item == sequence {
			sort.Ints(ret)
			return ret
		}
	}
	ret = append(ret, sequence)
	sort.Ints(ret)
	return ret
}

func nextArtifactContinuationSequence(received []int) int {
	next := 1
	for _, sequence := range appendArtifactContinuationSequence(received, 0) {
		if sequence == next {
			next++
		}
	}
	return next
}

func artifactContinuationTail(content string, limit int) string {
	if limit <= 0 || len(content) == 0 {
		return ""
	}
	data := []byte(content)
	if len(data) <= limit {
		return content
	}
	return string(data[len(data)-limit:])
}

func artifactPreviousTailBytesFromInput(input map[string]any) int {
	for _, source := range []map[string]any{
		mapFromValue(input["artifactOutputContract"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		mapFromValue(input["llmOutputPolicy"]),
	} {
		if value := intFromAny(source["previousChunkTailBytes"]); value > 0 {
			return value
		}
	}
	return defaultPreviousChunkTailBytes
}

func artifactContinuationAssembleSequences(item *orchestratormodel.ArtifactContinuation) ([]int, error) {
	if item == nil || len(item.ReceivedSequences) == 0 {
		return nil, fmt.Errorf("%w: artifact continuation receivedSequences is required", common.ErrInvalidArgument)
	}
	sequences := append([]int(nil), item.ReceivedSequences...)
	sort.Ints(sequences)
	for idx, sequence := range sequences {
		expected := idx + 1
		if sequence != expected {
			return nil, fmt.Errorf("%w: artifact continuation receivedSequences must be contiguous from 1", common.ErrInvalidArgument)
		}
		if idx > 0 && sequence == sequences[idx-1] {
			return nil, fmt.Errorf("%w: artifact continuation receivedSequences contains duplicate sequence", common.ErrInvalidArgument)
		}
	}
	return sequences, nil
}

func validateArtifactContinuationAssembleChunk(item *orchestratormodel.ArtifactContinuation, chunk *orchestratormodel.ArtifactContinuationChunk, content string, sequence int) error {
	if item == nil || chunk == nil {
		return fmt.Errorf("%w: artifact continuation chunk is required", common.ErrInvalidArgument)
	}
	if chunk.ContinuationID != item.ID {
		return fmt.Errorf("%w: artifact chunk continuationId does not match continuation", common.ErrInvalidArgument)
	}
	if chunk.Sequence != sequence {
		return fmt.Errorf("%w: artifact chunk sequence does not match continuation", common.ErrInvalidArgument)
	}
	if chunk.ArtifactID != item.ArtifactID {
		return fmt.Errorf("%w: artifact chunk artifactId does not match continuation", common.ErrInvalidArgument)
	}
	if chunk.TargetRef != item.TargetRef {
		return fmt.Errorf("%w: artifact chunk targetRef does not match continuation", common.ErrInvalidArgument)
	}
	if content == "" {
		return fmt.Errorf("%w: artifact chunk content is required", common.ErrInvalidArgument)
	}
	if strings.TrimSpace(chunk.ContentHash) == "" {
		return fmt.Errorf("%w: artifact chunk contentHash is required", common.ErrInvalidArgument)
	}
	if !artifactChunkHashMatchesContent(chunk.ContentHash, content) {
		return fmt.Errorf("%w: artifact chunk contentHash does not match content", common.ErrInvalidArgument)
	}
	if chunk.Bytes > 0 && chunk.Bytes != int64(len([]byte(content))) {
		return fmt.Errorf("%w: artifact chunk bytes does not match content", common.ErrInvalidArgument)
	}
	return nil
}

func (o *RuntimeOrchestrator) applyAssembledArtifactContinuation(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	item *orchestratormodel.ArtifactContinuation,
	deliverableDirectory string,
) error {
	if o == nil || o.evidence.ArtifactContinuations == nil || item == nil || item.ID == "" {
		return fmt.Errorf("%w: artifact continuation apply requires repository and item", common.ErrInvalidArgument)
	}
	if run == nil || step == nil || toolCall == nil {
		return fmt.Errorf("%w: artifact continuation apply requires run and step", common.ErrInvalidArgument)
	}
	if item.State != common.ArtifactContinuationStateApplying {
		return fmt.Errorf("%w: artifact continuation must be in applying state", common.ErrInvalidArgument)
	}
	metadata, content, err := o.evidence.ArtifactContinuations.GetAssembled(ctx, item.ID)
	if err != nil {
		return err
	}
	if err := validateArtifactContinuationApplyPayload(item, metadata, content); err != nil {
		return err
	}
	output := artifactContinuationApplyPatchOutput(item, metadata, content)
	handled, patchedOutput, err := o.applyPatchCandidatesThroughRuntimeBlocks(ctx, run, step, toolCall, output, &workspaceModel.Workspace{ID: run.WorkspaceID, DeliverableDirectory: deliverableDirectory})
	if patchedOutput != nil {
		output = patchedOutput
	}
	if err != nil || !handled {
		if err == nil {
			err = fmt.Errorf("%w: artifact continuation apply produced no patch candidate", common.ErrInvalidArgument)
		}
		item.State = common.ArtifactContinuationStateFailed
		item.CurrentStepID = step.ID
		item.OutputMode = outputModeIncrementalArtifact
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		item.Metadata["applyFailure"] = map[string]any{
			"targetRef":    item.TargetRef,
			"artifactId":   item.ArtifactID,
			"rollingHash":  metadata.RollingHash,
			"bytes":        metadata.Bytes,
			"failureError": err.Error(),
			"failedStepId": string(step.ID),
		}
		item.UpdatedAt = runtimeEvidenceTimestamp()
		_ = o.evidence.ArtifactContinuations.Update(ctx, item)
		return err
	}
	patchRefs := artifactContinuationApplyPatchRefs(step, output)
	metadata.ApplyEvidenceRefs = patchRefs
	if err := o.evidence.ArtifactContinuations.SaveAssembled(ctx, metadata, content); err != nil {
		return err
	}
	item.State = common.ArtifactContinuationStateApplied
	item.CurrentStepID = step.ID
	item.OutputMode = outputModeIncrementalArtifact
	item.EvidenceRefs = appendUniqueStrings(item.EvidenceRefs, patchRefs...)
	item.UpdatedAt = runtimeEvidenceTimestamp()
	if err := o.evidence.ArtifactContinuations.Update(ctx, item); err != nil {
		return err
	}
	if step.Output == nil {
		step.Output = map[string]any{}
	}
	step.Output["artifactContinuationApply"] = map[string]any{
		"id":                string(item.ID),
		"artifactId":        item.ArtifactID,
		"targetRef":         item.TargetRef,
		"state":             string(item.State),
		"bytes":             metadata.Bytes,
		"rollingHash":       metadata.RollingHash,
		"applyEvidenceRefs": patchRefs,
		"coverageTargets":   append([]string(nil), item.CoverageTargets...),
	}
	return nil
}

func validateArtifactContinuationApplyPayload(item *orchestratormodel.ArtifactContinuation, metadata *orchestratormodel.ArtifactContinuationAssembledMetadata, content string) error {
	if item == nil || metadata == nil {
		return fmt.Errorf("%w: artifact continuation assembled metadata is required", common.ErrInvalidArgument)
	}
	if metadata.ContinuationID != item.ID {
		return fmt.Errorf("%w: assembled continuationId does not match continuation", common.ErrInvalidArgument)
	}
	if metadata.TargetRef != item.TargetRef {
		return fmt.Errorf("%w: assembled targetRef does not match continuation", common.ErrInvalidArgument)
	}
	if content == "" {
		return fmt.Errorf("%w: assembled content is required", common.ErrInvalidArgument)
	}
	if metadata.Bytes > 0 && metadata.Bytes != int64(len([]byte(content))) {
		return fmt.Errorf("%w: assembled bytes does not match content", common.ErrInvalidArgument)
	}
	if strings.TrimSpace(metadata.RollingHash) == "" {
		return fmt.Errorf("%w: assembled rollingHash is required", common.ErrInvalidArgument)
	}
	if !artifactChunkHashMatchesContent(metadata.RollingHash, content) {
		return fmt.Errorf("%w: assembled rollingHash does not match content", common.ErrInvalidArgument)
	}
	return nil
}

func artifactContinuationApplyPatchOutput(item *orchestratormodel.ArtifactContinuation, metadata *orchestratormodel.ArtifactContinuationAssembledMetadata, content string) map[string]any {
	return map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"targetRef":           item.TargetRef,
			"patchType":           "file_write",
			"content":             content,
			"summary":             "artifact continuation apply",
			"coverageTargets":     append([]string(nil), item.CoverageTargets...),
			"artifactId":          item.ArtifactID,
			"continuationId":      string(item.ID),
			"allowReservedTarget": true,
			"assembledBytes":      metadata.Bytes,
			"assembledHash":       metadata.RollingHash,
			"sourceChunkRefs":     append([]string(nil), metadata.SourceChunkRefs...),
			"artifactApplyRef":    fmt.Sprintf("%s/apply", item.ID),
		}},
	}
}

func artifactContinuationApplyPatchRefs(step *runModel.Step, output map[string]any) []string {
	if step == nil || step.ID == "" {
		return nil
	}
	requests := mapSliceFromValue(output[stepOutputKeyPatches])
	if len(requests) == 0 {
		return nil
	}
	ret := make([]string, 0, len(requests))
	for idx := range requests {
		ret = append(ret, fmt.Sprintf("%s/patch/%d", step.ID, idx+1))
	}
	return ret
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values)+len(additions))
	ret := make([]string, 0, len(values)+len(additions))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		ret = append(ret, value)
	}
	return ret
}

func (o *RuntimeOrchestrator) dispatchInternalArtifactContinuationStep(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	workspace *workspaceModel.Workspace,
) (bool, error) {
	switch {
	case run == nil || step == nil:
		return false, nil
	case step.Kind != artifactAssembleStepKind && step.Kind != artifactApplyStepKind:
		return false, nil
	case o == nil || o.evidence.ArtifactContinuations == nil:
		return true, common.ErrInvalidArgument
	}
	continuationID := common.ArtifactContinuationID(strings.TrimSpace(stringFromMap(step.Input, "artifactContinuationId")))
	if continuationID == "" {
		return true, common.ErrInvalidArgument
	}
	item, err := o.evidence.ArtifactContinuations.Get(ctx, continuationID)
	if err != nil {
		return true, err
	}
	if item.RunID != run.ID || item.WorkspaceID != run.WorkspaceID {
		return true, common.ErrConflict
	}
	item.CurrentStepID = step.ID
	item.OutputMode = outputModeIncrementalArtifact
	_ = o.evidence.ArtifactContinuations.Update(ctx, item)
	output := map[string]any{}
	switch step.Kind {
	case artifactAssembleStepKind:
		metadata, _, err := assembleArtifactContinuation(ctx, o.evidence.ArtifactContinuations, item)
		if err != nil {
			output = artifactContinuationStepFailureOutput(step.Kind, item, err)
			_ = o.eventPublisher().publishArtifactContinuationChanged(ctx, run, step, item, orchestratorevents.TopicArtifactContinuationFailed, err)
			return true, o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "artifact continuation assemble failed", err)
		}
		fresh, _ := o.evidence.ArtifactContinuations.Get(ctx, item.ID)
		if fresh != nil {
			item = fresh
		}
		if err := o.eventPublisher().publishArtifactContinuationChanged(ctx, run, step, item, orchestratorevents.TopicArtifactContinuationAssembled, nil); err != nil {
			return true, err
		}
		output["artifactContinuationAssemble"] = map[string]any{
			"id":              string(item.ID),
			"artifactId":      item.ArtifactID,
			"targetRef":       item.TargetRef,
			"state":           string(common.ArtifactContinuationStateApplying),
			"bytes":           metadata.Bytes,
			"rollingHash":     metadata.RollingHash,
			"sourceChunkRefs": append([]string(nil), metadata.SourceChunkRefs...),
		}
	case artifactApplyStepKind:
		if workspace == nil {
			return true, common.ErrInvalidArgument
		}
		if err := o.applyAssembledArtifactContinuation(ctx, run, step, toolCall, item, workspace.DeliverableDirectory); err != nil {
			output = artifactContinuationStepFailureOutput(step.Kind, item, err)
			if len(step.Output) > 0 {
				for key, value := range step.Output {
					output[key] = value
				}
			}
			fresh, _ := o.evidence.ArtifactContinuations.Get(ctx, item.ID)
			if fresh != nil {
				item = fresh
			}
			_ = o.eventPublisher().publishArtifactContinuationChanged(ctx, run, step, item, orchestratorevents.TopicArtifactContinuationFailed, err)
			return true, o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "artifact continuation apply failed", err)
		}
		fresh, _ := o.evidence.ArtifactContinuations.Get(ctx, item.ID)
		if fresh != nil {
			item = fresh
		}
		if err := o.eventPublisher().publishArtifactContinuationChanged(ctx, run, step, item, orchestratorevents.TopicArtifactContinuationApplied, nil); err != nil {
			return true, err
		}
		output = cloneStepPayload(step.Output)
	default:
		return false, nil
	}
	return true, o.completeInternalArtifactContinuationStep(ctx, run, step, toolCall, output, workspace)
}

func artifactContinuationStepFailureOutput(kind string, item *orchestratormodel.ArtifactContinuation, err error) map[string]any {
	output := map[string]any{
		"status": "failed",
		"stage":  kind,
	}
	if err != nil {
		output["error"] = err.Error()
	}
	if item != nil {
		output["artifactContinuation"] = map[string]any{
			"id":         string(item.ID),
			"artifactId": item.ArtifactID,
			"targetRef":  item.TargetRef,
			"state":      string(item.State),
		}
	}
	return output
}

func (o *RuntimeOrchestrator) completeInternalArtifactContinuationStep(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	workspace *workspaceModel.Workspace,
) error {
	if err := o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
		return err
	}
	if step.Kind == artifactAssembleStepKind {
		return o.RequestUniqueDispatchStep(ctx, run, artifactContinuationApplyStepID(run.ID, common.ArtifactContinuationID(strings.TrimSpace(stringFromMap(step.Input, "artifactContinuationId")))), artifactApplyStepKind, step.Input, workspaceIDForDispatch(run, workspace))
	}
	if shouldDispatchLLMCompletionFollowUp(run, step, output) {
		return o.RequestUniqueDispatchStep(ctx, run, llmCompletionFollowUpStepID(run.ID, step), initialStepKind(run), o.graphManager().llmCompletionFollowUpStepInput(ctx, run, step), workspaceIDForDispatch(run, workspace))
	}
	return nil
}

func artifactContinuationAssembleStepID(runID common.RunID, id common.ArtifactContinuationID) common.StepID {
	return common.StepID(fmt.Sprintf("%s/artifact-assemble-%s", runID, runtimeChildID(common.StepID(id), "")))
}

func artifactContinuationApplyStepID(runID common.RunID, id common.ArtifactContinuationID) common.StepID {
	return common.StepID(fmt.Sprintf("%s/artifact-apply-%s", runID, runtimeChildID(common.StepID(id), "")))
}

func artifactContinuationRecoverySpecs(
	ctx context.Context,
	steps repository.StepRepository,
	continuations repository.ArtifactContinuationRepository,
	run *runModel.Run,
) ([]stepDispatchSpec, bool, error) {
	if run == nil || steps == nil || continuations == nil {
		return nil, false, nil
	}
	items, err := continuations.ListOpenByRun(ctx, run.ID)
	if err != nil {
		return nil, true, err
	}
	if len(items) == 0 || items[0] == nil {
		return nil, false, nil
	}
	item := items[0]
	input := map[string]any{
		"artifactContinuationId":      string(item.ID),
		"artifactContinuationContext": artifactContinuationContext(item),
	}
	switch item.State {
	case common.ArtifactContinuationStatePlanning, common.ArtifactContinuationStateChunking:
		stepID, err := uniqueDispatchStepID(ctx, steps, run.ID, artifactContinuationLLMStepID(run.ID, item))
		if err != nil {
			return nil, true, err
		}
		input["stateChangingOutputRequired"] = true
		input["repairObjective"] = repairObjectiveStateChangingOutput
		return []stepDispatchSpec{{
			ID:    stepID,
			Kind:  initialStepKind(run),
			Input: input,
		}}, true, nil
	case common.ArtifactContinuationStateAssembling:
		stepID, err := uniqueDispatchStepID(ctx, steps, run.ID, artifactContinuationAssembleStepID(run.ID, item.ID))
		if err != nil {
			return nil, true, err
		}
		return []stepDispatchSpec{{
			ID:    stepID,
			Kind:  artifactAssembleStepKind,
			Input: input,
		}}, true, nil
	case common.ArtifactContinuationStateApplying:
		stepID, err := uniqueDispatchStepID(ctx, steps, run.ID, artifactContinuationApplyStepID(run.ID, item.ID))
		if err != nil {
			return nil, true, err
		}
		return []stepDispatchSpec{{
			ID:    stepID,
			Kind:  artifactApplyStepKind,
			Input: input,
		}}, true, nil
	default:
		return nil, false, nil
	}
}

func artifactContinuationLLMStepID(runID common.RunID, item *orchestratormodel.ArtifactContinuation) common.StepID {
	id := common.ArtifactContinuationID("artifact-continuation")
	if item != nil && item.ID != "" {
		id = item.ID
	}
	return common.StepID(fmt.Sprintf("%s/artifact-continue-%s", runID, runtimeChildID(common.StepID(id), "")))
}
