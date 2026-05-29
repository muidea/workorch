package orchestrator

import (
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	artifactOutputPhasePlan        = "artifact_plan"
	artifactOutputPhaseChunk       = "artifact_chunk"
	artifactOutputPhaseChunkRepair = "artifact_chunk_repair"
)

var artifactOutputForbiddenActionKeys = []string{
	stepOutputKeyExecutionGraph,
	stepOutputKeyDoDAssessment,
	stepOutputKeyMemoryWrites,
	stepOutputKeyPatches,
	"patchRequests",
	"shellCommands",
	"mcpToolCalls",
	stepOutputKeyNextStep,
	stepOutputKeyNextKind,
	stepOutputKeyNextInput,
}

func validateIncrementalArtifactOutputContract(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	if step == nil || output == nil {
		return nil
	}
	mode := outputModeFromInput(step.Input)
	if mode != outputModeIncrementalArtifact {
		if artifactOutputFieldPresent(output, "artifactPlan") ||
			artifactOutputFieldPresent(output, "artifactChunk") ||
			artifactOutputFieldPresent(output, "artifactChunkRepair") {
			return fmt.Errorf("%w: artifact output requires incrementalArtifact outputMode", common.ErrInvalidArgument)
		}
		return nil
	}
	phase := artifactOutputPhaseFromInput(step.Input)
	switch phase {
	case artifactOutputPhaseChunk:
		return validateArtifactChunkOutputContract(step, output)
	case artifactOutputPhaseChunkRepair:
		return validateArtifactChunkRepairOutputContract(step, output)
	default:
		return validateArtifactPlanOutputContract(run, step, output)
	}
}

func validateArtifactPlanOutputContract(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	if err := validateArtifactOutputForbiddenFields(output, append(artifactOutputForbiddenActionKeys, "artifactChunk", "artifactChunkRepair")); err != nil {
		return err
	}
	if err := validateArtifactOutputCompletionState(output); err != nil {
		return err
	}
	if err := validateArtifactOutputMode(output); err != nil {
		return err
	}
	plan, ok, err := artifactOutputMap(output, "artifactPlan")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: artifactPlan is required for incrementalArtifact artifact_plan output", common.ErrInvalidArgument)
	}
	if err := validateArtifactPlanHasNoPayload(plan); err != nil {
		return err
	}
	artifacts := mapSliceFromValue(plan["artifacts"])
	if len(artifacts) == 0 {
		return fmt.Errorf("%w: artifactPlan.artifacts is required", common.ErrInvalidArgument)
	}
	seenArtifactIDs := map[string]bool{}
	allowedCoverageTargets := artifactPlanAllowedCoverageTargets(run, step)
	for idx, artifact := range artifacts {
		if err := validateArtifactPlanHasNoPayload(artifact); err != nil {
			return fmt.Errorf("%w: artifactPlan.artifacts[%d] invalid: %v", common.ErrInvalidArgument, idx, err)
		}
		artifactID := strings.TrimSpace(stringFromMap(artifact, "artifactId"))
		if artifactID == "" {
			return fmt.Errorf("%w: artifactPlan.artifacts[%d].artifactId is required", common.ErrInvalidArgument, idx)
		}
		if seenArtifactIDs[artifactID] {
			return fmt.Errorf("%w: artifactPlan.artifacts[%d].artifactId is duplicated", common.ErrInvalidArgument, idx)
		}
		seenArtifactIDs[artifactID] = true
		targetRef := strings.TrimSpace(stringFromMap(artifact, "targetRef"))
		if targetRef == "" {
			return fmt.Errorf("%w: artifactPlan.artifacts[%d].targetRef is required", common.ErrInvalidArgument, idx)
		}
		if !runtimeRepairTargetRefAdmissible(targetRef) {
			return fmt.Errorf("%w: artifactPlan.artifacts[%d].targetRef is outside deliverable boundary", common.ErrInvalidArgument, idx)
		}
		if strings.TrimSpace(stringFromMap(artifact, "operation")) == "" {
			return fmt.Errorf("%w: artifactPlan.artifacts[%d].operation is required", common.ErrInvalidArgument, idx)
		}
		coverageTargets := stringSliceFromValue(artifact["coverageTargets"])
		if len(coverageTargets) == 0 {
			return fmt.Errorf("%w: artifactPlan.artifacts[%d].coverageTargets is required", common.ErrInvalidArgument, idx)
		}
		if err := validateArtifactPlanCoverageTargets(idx, coverageTargets, allowedCoverageTargets); err != nil {
			return err
		}
		if intFromAny(artifact["estimatedChunks"]) <= 0 {
			return fmt.Errorf("%w: artifactPlan.artifacts[%d].estimatedChunks must be positive", common.ErrInvalidArgument, idx)
		}
		if intFromAny(artifact["estimatedBytes"]) <= 0 {
			return fmt.Errorf("%w: artifactPlan.artifacts[%d].estimatedBytes must be positive", common.ErrInvalidArgument, idx)
		}
	}
	return nil
}

func validateArtifactPlanCoverageTargets(idx int, targets []string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	for _, target := range targets {
		matched := false
		for _, candidate := range allowed {
			if coverageRepairTokenMatches(target, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%w: artifactPlan.artifacts[%d].coverageTargets contains unknown target %q", common.ErrInvalidArgument, idx, target)
		}
	}
	return nil
}

func artifactPlanAllowedCoverageTargets(run *runModel.Run, step *runModel.Step) []string {
	seen := map[string]bool{}
	ret := make([]string, 0)
	add := func(value string) {
		value = normalizeCoverageGapRepairToken(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	addMap := func(item map[string]any) {
		if len(item) == 0 {
			return
		}
		for _, key := range []string{
			"coverageTargetToken",
			"workItemId",
			"coverageWorkItemId",
			"goalRequirementId",
			"targetText",
			"item",
			"raw",
			"requirement",
			"goal",
			"goalItem",
			"id",
			"expectationId",
			"description",
			"title",
		} {
			add(stringFromMap(item, key))
		}
	}
	if step != nil && step.Input != nil {
		for _, target := range coverageBoundTargetTokens(step) {
			add(target)
		}
		for _, item := range mapSliceFromValue(step.Input[coverageWorkItemsKey]) {
			addMap(item)
		}
		if context := mapFromValue(step.Input[coverageWorkItemContextKey]); len(context) > 0 {
			for _, item := range mapSliceFromValue(context["items"]) {
				addMap(item)
			}
			for _, item := range mapSliceFromValue(context["currentFocusItems"]) {
				addMap(item)
			}
		}
	}
	if run != nil && run.ExecutionContext != nil {
		for _, item := range mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey]) {
			addMap(item)
		}
		if context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]); len(context) > 0 {
			for _, item := range mapSliceFromValue(context["items"]) {
				addMap(item)
			}
			for _, item := range mapSliceFromValue(context["currentFocusItems"]) {
				addMap(item)
			}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func validateArtifactChunkOutputContract(step *runModel.Step, output map[string]any) error {
	if err := validateArtifactOutputForbiddenFields(output, append(artifactOutputForbiddenActionKeys, "artifactPlan", "artifactChunkRepair")); err != nil {
		return err
	}
	if err := validateArtifactOutputCompletionState(output); err != nil {
		return err
	}
	if err := validateArtifactOutputMode(output); err != nil {
		return err
	}
	chunk, ok, err := artifactOutputMap(output, "artifactChunk")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: artifactChunk is required for incrementalArtifact artifact_chunk output", common.ErrInvalidArgument)
	}
	artifactID := strings.TrimSpace(stringFromMap(chunk, "artifactId"))
	targetRef := strings.TrimSpace(stringFromMap(chunk, "targetRef"))
	if artifactID == "" {
		return fmt.Errorf("%w: artifactChunk.artifactId is required", common.ErrInvalidArgument)
	}
	if targetRef == "" {
		return fmt.Errorf("%w: artifactChunk.targetRef is required", common.ErrInvalidArgument)
	}
	sequence := intFromAny(chunk["sequence"])
	if sequence <= 0 {
		return fmt.Errorf("%w: artifactChunk.sequence must be positive", common.ErrInvalidArgument)
	}
	content := stringRawFromMap(chunk, "content")
	if content == "" {
		return fmt.Errorf("%w: artifactChunk.content is required", common.ErrInvalidArgument)
	}
	chunkHash := strings.TrimSpace(stringFromMap(chunk, "chunkHash"))
	if chunkHash == "" {
		return fmt.Errorf("%w: artifactChunk.chunkHash is required", common.ErrInvalidArgument)
	}
	if !artifactChunkHashMatchesContent(chunkHash, content) {
		return fmt.Errorf("%w: artifactChunk.chunkHash does not match content", common.ErrInvalidArgument)
	}
	if _, ok := chunk["isFinal"].(bool); !ok {
		return fmt.Errorf("%w: artifactChunk.isFinal is required", common.ErrInvalidArgument)
	}
	if maxBytes := artifactMaxChunkBytesFromInput(step.Input); maxBytes > 0 && len([]byte(content)) > maxBytes {
		return fmt.Errorf("%w: artifactChunk.content exceeds maxArtifactChunkBytes", common.ErrInvalidArgument)
	}
	continuation := mapFromValue(step.Input["artifactContinuationContext"])
	if expected := strings.TrimSpace(stringFromMap(continuation, "artifactId")); expected != "" && expected != artifactID {
		return fmt.Errorf("%w: artifactChunk.artifactId does not match artifactContinuationContext", common.ErrInvalidArgument)
	}
	if expected := strings.TrimSpace(stringFromMap(continuation, "targetRef")); expected != "" && expected != targetRef {
		return fmt.Errorf("%w: artifactChunk.targetRef does not match artifactContinuationContext", common.ErrInvalidArgument)
	}
	if expected := intFromAny(continuation["expectedSequence"]); expected > 0 && expected != sequence {
		return fmt.Errorf("%w: artifactChunk.sequence does not match artifactContinuationContext.expectedSequence", common.ErrInvalidArgument)
	}
	if ok, err := validateArtifactContinuationDuplicateSequence(continuation, sequence, chunkHash); !ok {
		return err
	}
	if err := validateArtifactChunkContinuity(output, chunk, content, continuation); err != nil {
		return err
	}
	return nil
}

func validateArtifactChunkContinuity(output map[string]any, chunk map[string]any, content string, continuation map[string]any) error {
	previousTail := stringRawFromMap(continuation, "previousChunkTail")
	if strings.TrimSpace(previousTail) == "" {
		return nil
	}
	if content == previousTail || strings.HasPrefix(content, previousTail) {
		return fmt.Errorf("%w: artifactChunk.content repeats previousChunkTail", common.ErrInvalidArgument)
	}
	if overlap := previousTailPrefixOverlap(previousTail, content); overlap > 0 {
		appendArtifactChunkWarning(output, "partial_previous_chunk_tail_overlap")
	}
	startsAfter := stringRawFromMap(chunk, "startsAfter")
	if strings.TrimSpace(startsAfter) != "" && !artifactChunkTailMarkerMatches(startsAfter, previousTail) {
		appendArtifactChunkWarning(output, "startsAfter_does_not_match_previousChunkTail")
	}
	endsWith := stringRawFromMap(chunk, "endsWith")
	if strings.TrimSpace(endsWith) != "" && !strings.HasSuffix(content, endsWith) {
		appendArtifactChunkWarning(output, "endsWith_does_not_match_content_suffix")
	}
	return nil
}

func previousTailPrefixOverlap(previousTail string, content string) int {
	max := len(previousTail)
	if len(content) < max {
		max = len(content)
	}
	for size := max; size > 0; size-- {
		if strings.HasSuffix(previousTail, content[:size]) {
			return size
		}
	}
	return 0
}

func artifactChunkTailMarkerMatches(marker string, previousTail string) bool {
	marker = strings.TrimSpace(marker)
	previousTail = strings.TrimSpace(previousTail)
	if marker == "" || previousTail == "" {
		return false
	}
	return marker == previousTail || strings.HasSuffix(previousTail, marker)
}

func appendArtifactChunkWarning(output map[string]any, warning string) {
	warning = strings.TrimSpace(warning)
	if warning == "" || output == nil {
		return
	}
	existing := stringSliceFromValue(output["artifactChunkWarnings"])
	if stringSliceContains(existing, warning) {
		return
	}
	output["artifactChunkWarnings"] = append(existing, warning)
}

func artifactChunkHashMatchesContent(chunkHash string, content string) bool {
	chunkHash = strings.TrimSpace(strings.ToLower(chunkHash))
	if chunkHash == "" {
		return false
	}
	expected := sha256HexBytes([]byte(content))
	return chunkHash == expected || chunkHash == "sha256:"+expected
}

func stringRawFromMap(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return value
}

func validateArtifactContinuationDuplicateSequence(continuation map[string]any, sequence int, chunkHash string) (bool, error) {
	if sequence <= 0 || len(continuation) == 0 {
		return true, nil
	}
	for _, item := range intSliceFromValue(continuation["receivedSequences"]) {
		if item == sequence {
			if existing := artifactContinuationChunkHashForSequence(continuation, sequence); artifactChunkHashesEqual(existing, chunkHash) {
				return true, nil
			}
			return false, fmt.Errorf("%w: artifactChunk.sequence was already received with a different chunkHash", common.ErrInvalidArgument)
		}
	}
	return true, nil
}

func artifactContinuationChunkHashForSequence(continuation map[string]any, sequence int) string {
	if sequence <= 0 || len(continuation) == 0 {
		return ""
	}
	hashes := mapStringFromAny(continuation["receivedChunkHashes"])
	for _, key := range []string{fmt.Sprintf("%d", sequence), fmt.Sprintf("sequence-%d", sequence)} {
		if value := strings.TrimSpace(hashes[key]); value != "" {
			return value
		}
	}
	received := intSliceFromValue(continuation["receivedSequences"])
	if len(received) > 0 && received[len(received)-1] == sequence {
		return stringFromMap(continuation, "lastChunkHash")
	}
	return ""
}

func artifactChunkHashesEqual(left string, right string) bool {
	left = normalizeArtifactChunkHash(left)
	right = normalizeArtifactChunkHash(right)
	return left != "" && left == right
}

func normalizeArtifactChunkHash(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "sha256:")
	return value
}

func mapStringFromAny(value any) map[string]string {
	switch typed := value.(type) {
	case map[string]string:
		ret := make(map[string]string, len(typed))
		for key, value := range typed {
			ret[key] = value
		}
		return ret
	case map[int]string:
		ret := make(map[string]string, len(typed))
		for key, value := range typed {
			ret[fmt.Sprintf("%d", key)] = value
		}
		return ret
	case map[string]any:
		ret := make(map[string]string, len(typed))
		for key, value := range typed {
			text, _ := value.(string)
			if strings.TrimSpace(text) != "" {
				ret[key] = text
			}
		}
		return ret
	default:
		return nil
	}
}

func intSliceFromValue(value any) []int {
	switch typed := value.(type) {
	case []int:
		return append([]int(nil), typed...)
	case []any:
		ret := make([]int, 0, len(typed))
		for _, item := range typed {
			next := intFromAny(item)
			if next > 0 {
				ret = append(ret, next)
			}
		}
		return ret
	default:
		return nil
	}
}

func validateArtifactChunkRepairOutputContract(step *runModel.Step, output map[string]any) error {
	if err := validateArtifactOutputForbiddenFields(output, append(artifactOutputForbiddenActionKeys, "artifactPlan", "artifactChunk")); err != nil {
		return err
	}
	if err := validateArtifactRepairCompletionState(output); err != nil {
		return err
	}
	if err := validateArtifactOutputMode(output); err != nil {
		return err
	}
	repair, ok, err := artifactOutputMap(output, "artifactChunkRepair")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: artifactChunkRepair is required for incrementalArtifact artifact_chunk_repair output", common.ErrInvalidArgument)
	}
	if err := validateArtifactRepairHasNoPayload(repair); err != nil {
		return err
	}
	artifactID := strings.TrimSpace(stringFromMap(repair, "artifactId"))
	targetRef := strings.TrimSpace(stringFromMap(repair, "targetRef"))
	if artifactID == "" {
		return fmt.Errorf("%w: artifactChunkRepair.artifactId is required", common.ErrInvalidArgument)
	}
	if targetRef == "" {
		return fmt.Errorf("%w: artifactChunkRepair.targetRef is required", common.ErrInvalidArgument)
	}
	sequence := intFromAny(repair["sequence"])
	if sequence <= 0 {
		return fmt.Errorf("%w: artifactChunkRepair.sequence must be positive", common.ErrInvalidArgument)
	}
	if strings.TrimSpace(stringFromMap(repair, "failureReason")) == "" {
		return fmt.Errorf("%w: artifactChunkRepair.failureReason is required", common.ErrInvalidArgument)
	}
	nextAction := strings.TrimSpace(stringFromMap(repair, "nextAction"))
	switch nextAction {
	case "retry_current_sequence", "replan_artifact", "blocker":
	default:
		return fmt.Errorf("%w: artifactChunkRepair.nextAction is invalid", common.ErrInvalidArgument)
	}
	if len(stringSliceFromValue(repair["constraints"])) == 0 {
		return fmt.Errorf("%w: artifactChunkRepair.constraints is required", common.ErrInvalidArgument)
	}
	continuation := mapFromValue(step.Input["artifactContinuationContext"])
	if expected := strings.TrimSpace(stringFromMap(continuation, "artifactId")); expected != "" && expected != artifactID {
		return fmt.Errorf("%w: artifactChunkRepair.artifactId does not match artifactContinuationContext", common.ErrInvalidArgument)
	}
	if expected := strings.TrimSpace(stringFromMap(continuation, "targetRef")); expected != "" && expected != targetRef {
		return fmt.Errorf("%w: artifactChunkRepair.targetRef does not match artifactContinuationContext", common.ErrInvalidArgument)
	}
	if expected := intFromAny(continuation["expectedSequence"]); expected > 0 && expected != sequence {
		return fmt.Errorf("%w: artifactChunkRepair.sequence does not match artifactContinuationContext.expectedSequence", common.ErrInvalidArgument)
	}
	return nil
}

func artifactOutputPhaseFromInput(input map[string]any) string {
	contract := mapFromValue(input["artifactOutputContract"])
	phase := strings.TrimSpace(stringFromMap(contract, "phase"))
	if phase != "" {
		return phase
	}
	continuation := mapFromValue(input["artifactContinuationContext"])
	switch strings.TrimSpace(stringFromMap(continuation, "state")) {
	case string(common.ArtifactContinuationStateChunking):
		return artifactOutputPhaseChunk
	default:
		return artifactOutputPhasePlan
	}
}

func artifactOutputMap(output map[string]any, key string) (map[string]any, bool, error) {
	if value, ok := output[key]; ok {
		mapped := mapFromValue(value)
		if len(mapped) == 0 {
			return nil, true, fmt.Errorf("%w: %s must be an object", common.ErrInvalidArgument, key)
		}
		return mapped, true, nil
	}
	decision := mapFromValue(output[stepOutputKeyLLMDecision])
	if value, ok := decision[key]; ok {
		mapped := mapFromValue(value)
		if len(mapped) == 0 {
			return nil, true, fmt.Errorf("%w: llmDecision.%s must be an object", common.ErrInvalidArgument, key)
		}
		return mapped, true, nil
	}
	return nil, false, nil
}

func artifactOutputFieldPresent(output map[string]any, key string) bool {
	if output == nil {
		return false
	}
	if _, ok := output[key]; ok {
		return true
	}
	decision := mapFromValue(output[stepOutputKeyLLMDecision])
	_, ok := decision[key]
	return ok
}

func validateArtifactOutputForbiddenFields(output map[string]any, forbidden []string) error {
	for _, key := range forbidden {
		if artifactOutputFieldPresent(output, key) {
			return fmt.Errorf("%w: incrementalArtifact output must not include %s", common.ErrInvalidArgument, key)
		}
	}
	return nil
}

func validateArtifactOutputCompletionState(output map[string]any) error {
	state := strings.TrimSpace(strings.ToLower(stringFromMap(output, "completionState")))
	if state == "" {
		state = llmDecisionCompletionStateFromOutput(output)
	}
	if state != "" && state != "continue" {
		return fmt.Errorf("%w: incrementalArtifact completionState must be continue", common.ErrInvalidArgument)
	}
	return nil
}

func validateArtifactRepairCompletionState(output map[string]any) error {
	state := strings.TrimSpace(strings.ToLower(stringFromMap(output, "completionState")))
	if state == "" {
		state = llmDecisionCompletionStateFromOutput(output)
	}
	if state != "" && state != "continue" && state != "blocked" {
		return fmt.Errorf("%w: artifactChunkRepair completionState must be continue or blocked", common.ErrInvalidArgument)
	}
	return nil
}

func validateArtifactOutputMode(output map[string]any) error {
	mode := strings.TrimSpace(stringFromMap(output, "outputMode"))
	if mode == "" {
		mode = strings.TrimSpace(stringFromMap(mapFromValue(output[stepOutputKeyLLMDecision]), "outputMode"))
	}
	if mode != "" && mode != outputModeIncrementalArtifact {
		return fmt.Errorf("%w: incrementalArtifact outputMode must be incrementalArtifact", common.ErrInvalidArgument)
	}
	return nil
}

func validateArtifactRepairHasNoPayload(item map[string]any) error {
	for key, value := range item {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "content", "body", "fullcontent", "patch", "diff", "artifactchunk", "chunks":
			if !artifactPlanPayloadEmpty(value) {
				return fmt.Errorf("%w: artifactChunkRepair must not include payload field %s", common.ErrInvalidArgument, key)
			}
		}
	}
	return nil
}

func validateArtifactPlanHasNoPayload(item map[string]any) error {
	for key, value := range item {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "content", "body", "fullcontent", "patch", "diff", "artifactchunk", "chunks":
			if !artifactPlanPayloadEmpty(value) {
				return fmt.Errorf("%w: artifactPlan must not include payload field %s", common.ErrInvalidArgument, key)
			}
		}
	}
	return nil
}

func artifactPlanPayloadEmpty(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case []map[string]any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func artifactMaxChunkBytesFromInput(input map[string]any) int {
	for _, source := range []map[string]any{
		mapFromValue(input["artifactOutputContract"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		mapFromValue(input["llmOutputPolicy"]),
	} {
		if value := intFromAny(source["maxArtifactChunkBytes"]); value > 0 {
			return value
		}
	}
	return defaultMaxArtifactChunkBytes
}
