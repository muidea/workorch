package orchestrator

import (
	"path/filepath"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func preserveStateChangingSchemaInvalidRepairContract(input map[string]any, failedStep *runModel.Step) bool {
	if len(input) == 0 || failedStep == nil || !stateChangingSchemaInvalidRepair(failedStep) || !inputRequiresStateChangingOutput(failedStep.Input) {
		return false
	}
	if boolValue(input["stateChangingOutputRequired"]) && strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveStateChangingOutput {
		return false
	}
	input["schemaInvalidRepairMode"] = "single_valid_json_decision"
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["stateChangingOutputRequired"] = true
	delete(input, "boundedValidationRequired")
	delete(input, "concreteFailureEvidenceRequired")
	delete(input, "evidenceRepairRequired")
	delete(input, stepInputKeyAssessmentOnly)
	delete(input, "assessmentOnlyReason")
	inheritPriorRepairContext(input, failedStep)
	pkg := stateChangingRepairActionPackageForConvergence(input, failedStep)
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	pkg["schemaInvalidRecovery"] = map[string]any{
		"required": true,
		"reason":   "previous_llm_decision_json_invalid",
		"requirements": []string{
			"return exactly one valid JSON object",
			"do not reuse rejected raw output previews",
			"do not include markdown, comments, duplicate objects, trailing braces, or partial arrays",
			"make every executionGraph node a complete JSON object",
		},
	}
	required := enforceStateChangingRequiredOutputForConvergence(pkg)
	required["mustReturnValidJSON"] = true
	required["mustNotReuseRejectedJSON"] = true
	required["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(required["forbiddenKinds"]),
		"malformed_json",
		"markdown_wrapped_json",
		"raw_rejected_json_reuse",
		"partial_json_array",
	))
	pkg["requiredOutput"] = required
	input["repairActionPackage"] = pkg
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Previous repair output was rejected because the LLM decision JSON was invalid; keep the existing state-changing repair objective and rebuild one valid JSON object from the current target snapshot.",
		"Do not downgrade this repair to validation or assessment-only; preserve the state-changing contract until the target artifact is produced.",
		"Do not copy any rawContentPreview or repairRawContentPreview from the rejected output; do not include markdown, comments, duplicate JSON objects, trailing braces, or partial arrays.",
	)
	return true
}

func preserveAssessmentOnlySchemaInvalidRepairContract(input map[string]any, failedStep *runModel.Step) bool {
	if len(input) == 0 || !assessmentOnlySchemaInvalidRepairRequired(failedStep) {
		return false
	}
	setRuntimeStage(input, runtimeStageVerification)
	input[stepInputKeySoulStage] = soulStageValidation
	input[stepInputKeyAssessmentOnly] = true
	input["completionFollowUp"] = true
	input["repairObjective"] = "complete_dod_goal_coverage_assessment"
	if strings.TrimSpace(stringFromMap(input, "assessmentOnlyReason")) == "" {
		input["assessmentOnlyReason"] = firstNonEmptyString(
			stringFromMap(failedStep.Input, "assessmentOnlyReason"),
			"assessment_schema_invalid_repair",
		)
	}
	for _, key := range []string{
		"missingDoDItems",
		"missingGoalCoverage",
		"missingDeliverableCoverage",
		"requiredDeliverableCoverage",
		"coverageTargetSet",
		"latestClosureEvidence",
		"supersededFailureEvidence",
	} {
		if _, ok := input[key]; !ok && failedStep.Input[key] != nil {
			input[key] = cloneStepInputValue(failedStep.Input[key])
		}
	}
	input["schemaInvalidRepairMode"] = "single_valid_json_decision"
	input["assessmentSchemaInvalidRecovery"] = map[string]any{
		"required": true,
		"reason":   "previous_assessment_decision_json_invalid",
		"requirements": []string{
			"return exactly one valid JSON object",
			"preserve the assessment-only contract",
			"return dodAssessment or a real governance blocker only",
			"do not include executionGraph, shellCommands, patchRequests, mcpToolCalls, memoryWrites, or other executable work",
			"do not reuse rejected raw output previews",
			"do not include markdown, comments, duplicate objects, trailing braces, or partial arrays",
		},
	}
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Previous assessment-only output was rejected because the LLM decision JSON was invalid; only fix the JSON/schema shape and preserve the assessment-only coverage contract.",
		"Return one valid JSON object containing a completed or failed dodAssessment, or a real governance blocker; do not return executable work.",
		"Do not copy rawContentPreview or repairRawContentPreview from the rejected output.",
	)
	delete(input, "stateChangingOutputRequired")
	delete(input, "concreteFailureEvidenceRequired")
	delete(input, "boundedValidationRequired")
	delete(input, "evidenceRepairRequired")
	delete(input, "evidenceRepairSource")
	return true
}

func preserveConcreteValidationSchemaInvalidRepairContract(input map[string]any, failedStep *runModel.Step) bool {
	if len(input) == 0 || failedStep == nil || !stateChangingSchemaInvalidRepair(failedStep) || !inputRequiresConcreteValidationEvidence(failedStep.Input) {
		return false
	}
	inheritConcreteValidationRepairContract(input, failedStep)
	input["schemaInvalidRepairMode"] = "single_valid_json_decision"
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	pkg["schemaInvalidRecovery"] = map[string]any{
		"required": true,
		"reason":   "previous_llm_decision_json_invalid",
		"requirements": []string{
			"return exactly one valid JSON object",
			"do not reuse rejected raw output previews",
			"do not include markdown, comments, duplicate objects, trailing braces, or partial arrays",
			"make every executionGraph node a complete JSON object",
		},
	}
	required := concreteValidationEvidenceRequiredOutputWithScratch(input, pkg, mapFromValue(pkg["requiredOutput"]))
	required["mustReturnValidJSON"] = true
	required["mustNotReuseRejectedJSON"] = true
	required["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(required["forbiddenKinds"]),
		"malformed_json",
		"markdown_wrapped_json",
		"raw_rejected_json_reuse",
		"partial_json_array",
	))
	pkg["requiredOutput"] = required
	input["repairActionPackage"] = pkg
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Previous validation-evidence repair output was rejected because the JSON shape was invalid; only fix the JSON/schema shape and preserve the concrete validation evidence contract.",
		"Do not downgrade to evidence acquisition, assessment-only, or state-changing patch output; return one valid JSON object whose executionGraph follows repairActionPackage.requiredOutput.",
	)
	return true
}

func runtimeRepairLastFailureDiagnostics(failedStep *runModel.Step) map[string]any {
	if failedStep == nil {
		return nil
	}
	if diagnostics := mapFromValue(failedStep.Output["lastFailureDiagnostics"]); len(diagnostics) > 0 {
		return compactLLMContextMap(diagnostics, 1)
	}
	ret := map[string]any{
		"stepId":           string(failedStep.ID),
		"stepKind":         failedStep.Kind,
		"failureClass":     strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass)),
		"failureSignature": strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature)),
		"stage":            strings.TrimSpace(stringFromMap(failedStep.Output, "stage")),
		"error": firstNonEmptyString(
			stringFromMap(failedStep.Output, "error"),
			stringFromMap(failedStep.Output, "reason"),
		),
	}
	if command := strings.TrimSpace(stringFromMap(failedStep.Output, "command")); command != "" {
		ret["command"] = command
	}
	if args := stringSliceFromValue(failedStep.Output["args"]); len(args) > 0 {
		ret["args"] = args
	}
	if exitCode, ok := failedStep.Output["exitCode"]; ok {
		ret["exitCode"] = exitCode
	}
	if stderr := strings.TrimSpace(stringFromMap(failedStep.Output, "stderr")); stderr != "" {
		ret["stderrPreview"] = truncateLLMContextString(stderr)
	}
	if stdout := strings.TrimSpace(stringFromMap(failedStep.Output, "stdout")); stdout != "" {
		ret["stdoutPreview"] = truncateLLMContextString(stdout)
	}
	for key, value := range ret {
		if strings.TrimSpace(stringFromAny(value)) == "" {
			delete(ret, key)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func currentFailureIsCoverageAssessmentOnly(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	class := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(step.Output, failureOutputKeyClass),
		stringFromMap(step.Output, "failureClass"),
	))
	stage := strings.TrimSpace(stringFromMap(step.Output, "stage"))
	if class == failureClassDoDCoverageMissing && (stage == "llm_dod_assessment" || stage == "llm_execution_graph_plan") {
		return true
	}
	diagnostics := mapFromValue(step.Output["runtimeDiagnostics"])
	if strings.TrimSpace(stringFromMap(diagnostics, "kind")) == failureClassDoDCoverageMissing {
		return true
	}
	return false
}

func runtimeRepairShouldReturnToRejectedResultContinuation(run *runModel.Run, failedStep *runModel.Step) bool {
	if run == nil || failedStep == nil || failedStep.Input == nil {
		return false
	}
	if len(mapFromValue(run.ExecutionContext["rejectedResultContinuation"])) == 0 {
		return false
	}
	if strings.TrimSpace(string(failedStep.Kind)) != llmWorkspaceExecutionStepKind {
		return false
	}
	if runtimeRepairInputHasCurrentTargetSnapshotEvidence(failedStep.Input) {
		return false
	}
	failedInputContext := mapFromValue(failedStep.Input["executionContext"])
	if len(mapFromValue(failedInputContext["rejectedResultContinuation"])) == 0 {
		return false
	}
	replan := mapFromValue(failedInputContext["replanContinuation"])
	if len(replan) == 0 || !boolValue(replan["historicalOnly"]) {
		return false
	}
	if boolValue(failedStep.Input["replanStateChangingContinuation"]) {
		return true
	}
	replanTargets := repairPatchTargetRefsFromScopeText(firstNonEmptyString(
		stringFromMap(mapFromValue(replan["failureContext"]), "failureTargetScope"),
		stringFromMap(mapFromValue(replan["ticketContext"]), "failureTargetScope"),
		stringFromMap(mapFromValue(replan["failureContext"]), failureOutputKeySignature),
		stringFromMap(mapFromValue(replan["ticketContext"]), failureOutputKeySignature),
	))
	if len(replanTargets) == 0 {
		return false
	}
	failedTargets := compactUniqueStrings(append(failureTargetRepairRefs(failedStep), repairPatchTargetRefsFromScopeText(stringFromMap(failedStep.Output, "error"))...))
	if len(failedTargets) == 0 {
		return false
	}
	targetSet := map[string]bool{}
	for _, target := range replanTargets {
		target = normalizeRepairPatchTargetRef(target)
		if target != "" {
			targetSet[target] = true
		}
	}
	for _, target := range failedTargets {
		if targetSet[normalizeRepairPatchTargetRef(target)] {
			return true
		}
	}
	return false
}

func runtimeRepairInputHasCurrentTargetSnapshotEvidence(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	for _, key := range []string{
		"failureTargetRepairContext",
		"patchRepairContext",
		"readableResourceRepairContext",
		"rootCauseCandidateContext",
		"resourceShapeAlignmentContext",
	} {
		if repairContextHasAdmissibleTargetEvidence(input[key]) {
			return true
		}
	}
	pkg := mapFromValue(input["repairActionPackage"])
	for _, key := range []string{"targetSnapshots", "currentTargetSnapshots", "rootCauseCandidateContext", "resourceShapeAlignmentContext"} {
		if repairContextHasAdmissibleTargetEvidence(pkg[key]) {
			return true
		}
	}
	return false
}

func resetRuntimeRepairInputToRejectedResultContinuation(input map[string]any, run *runModel.Run, failedStep *runModel.Step) map[string]any {
	if input == nil {
		input = map[string]any{}
	}
	continuation := mapFromValue(run.ExecutionContext["rejectedResultContinuation"])
	timeline := processEvidenceTimelineFromRejectedResultContinuation(continuation)
	digest := currentProgressDigestFromRejectedResultContinuation(continuation, timeline)
	input[stepInputKeyProcessEvidenceTimeline] = compactProcessEvidenceTimelineForRuntimeContext(timeline)
	input[stepInputKeyCurrentProgressDigest] = digest
	input["currentExecutionProgress"] = compactLLMContextMap(digest, 0)
	if currentFailure := currentFailureFromProgressDigest(digest); len(currentFailure) > 0 {
		input["currentFailure"] = currentFailure
	}
	delete(input, "replanStateChangingContinuation")
	delete(input, "stateChangingOutputRequired")
	delete(input, "patchRepairContext")
	delete(input, "failureTargetRepairContext")
	delete(input, "targetRepairAttempts")
	delete(input, "strategyReassessment")
	input["repairObjective"] = repairObjectiveAcquireFailureEvidence
	input["concreteFailureEvidenceRequired"] = true
	input["historicalRepairAttempt"] = map[string]any{
		"stepId":       string(failedStep.ID),
		"kind":         failedStep.Kind,
		"state":        string(failedStep.State),
		"reason":       firstNonEmptyString(stringFromMap(failedStep.Output, "reason"), stringFromMap(failedStep.Output, "error")),
		"supersededBy": "rejectedResultContinuation",
	}
	pkg := map[string]any{
		"requiredOutput":              evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
		"evidenceAcquisitionRequired": true,
		"currentExecutionProgress":    compactLLMContextMap(digest, 0),
	}
	if currentFailure := currentFailureFromProgressDigest(digest); len(currentFailure) > 0 {
		pkg["currentFailure"] = currentFailure
	}
	input["repairActionPackage"] = pkg
	input["repairGuidance"] = appendRepairGuidance(concreteFailureEvidenceAcquisitionGuidance(),
		"Rejected result continuation supersedes the historical replan failure; treat the historical repair attempt as lineage only.",
		"Acquire or produce the current evidence required by rejectedResultContinuation before choosing a new state-changing target.",
	)
	return input
}

func inheritPriorRepairContext(input map[string]any, failedStep *runModel.Step) {
	if input == nil || failedStep == nil || failedStep.Input == nil {
		return
	}
	inherit := func(key string) bool {
		if len(mapSliceFromValue(input[key])) > 0 {
			return false
		}
		context := mapSliceFromValue(failedStep.Input[key])
		if len(context) == 0 {
			return false
		}
		input[key] = context
		return true
	}
	inheritMap := func(key string) bool {
		if len(mapFromValue(input[key])) > 0 {
			return false
		}
		context := mapFromValue(failedStep.Input[key])
		if len(context) == 0 {
			return false
		}
		input[key] = cloneStepPayload(context)
		return true
	}
	inheritedTarget := inherit("failureTargetRepairContext")
	inheritedPatch := inherit("patchRepairContext")
	inheritedReadable := inherit("readableResourceRepairContext")
	inheritedCoverageContext := inheritMap(coverageWorkItemContextKey)
	inheritedManifest := inherit(stepInputKeyWorkspaceFileManifest)
	inheritedStrategy := false
	if len(mapFromValue(input["strategyReassessment"])) == 0 {
		if reassessment := mapFromValue(failedStep.Input["strategyReassessment"]); len(reassessment) > 0 {
			input["strategyReassessment"] = reassessment
			inheritedStrategy = true
		}
	}
	if len(mapFromValue(input["repairActionPackage"])) == 0 {
		if pkg := mapFromValue(failedStep.Input["repairActionPackage"]); len(pkg) > 0 {
			input["repairActionPackage"] = cloneStepPayload(pkg)
		}
	}
	inheritedRootCauseCandidates := inherit("rootCauseCandidateContext")
	if _, ok := input["targetRepairAttempts"]; !ok {
		if attempts := int64FromValue(failedStep.Input["targetRepairAttempts"]); attempts > 0 {
			input["targetRepairAttempts"] = attempts
		}
	}
	if strings.TrimSpace(stringFromMap(input, "failureTargetScope")) == "" {
		if scope := strings.TrimSpace(stringFromMap(failedStep.Input, "failureTargetScope")); scope != "" {
			input["failureTargetScope"] = scope
		}
	}
	if inheritedTarget || inheritedPatch || inheritedReadable || inheritedCoverageContext || inheritedManifest {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Preserve inherited repair context from the previous repair attempt; it contains the current target snapshots needed for direct state-changing recovery.",
			"Use inherited target snapshots and workspace manifest to produce accepted state-changing artifacts instead of retrying rejected undeclared tool or read-only recovery output.",
			"When a snapshot exposes contentHash, use that literal hash as baseHash or omit baseHash; do not emit template or interpolation syntax in patch fields.",
			"Do not generate file_write from inherited snapshots unless the snapshot is complete and not redacted; use exact minimal diffs or file/read when content is incomplete.",
		)
	}
	if inheritedStrategy || inheritedRootCauseCandidates {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Preserve inherited strategy reassessment from the previous repair attempt; a runtime contract failure on the repair wrapper does not clear the original repeated failed-surface repair contract.",
			"Use inherited root-cause candidate context to choose the next repair target; do not patch the same failed surface again unless surfaceContractCorrection explains why that surface itself is the correction target.",
		)
	}
}

func inheritCoverageRepairContext(input map[string]any, failedStep *runModel.Step) bool {
	if input == nil || failedStep == nil {
		return false
	}
	inherited := false
	inheritSlice := func(key string) {
		if len(mapSliceFromValue(input[key])) > 0 {
			return
		}
		if values := mapSliceFromValue(failedStep.Input[key]); len(values) > 0 {
			input[key] = cloneMapSlice(values)
			inherited = true
			return
		}
		if values := mapSliceFromValue(failedStep.Output[key]); len(values) > 0 {
			input[key] = cloneMapSlice(values)
			inherited = true
			return
		}
		if failedOutput := mapFromValue(failedStep.Output["failedStepOutput"]); len(failedOutput) > 0 {
			if values := mapSliceFromValue(failedOutput[key]); len(values) > 0 {
				input[key] = cloneMapSlice(values)
				inherited = true
			}
		}
		if diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"]); len(diagnostics) > 0 {
			if values := mapSliceFromValue(diagnostics[key]); len(values) > 0 {
				input[key] = cloneMapSlice(values)
				inherited = true
			}
		}
	}
	inheritSlice("missingDoDItems")
	inheritSlice("missingGoalCoverage")
	inheritSlice("missingDeliverableCoverage")
	if !inherited {
		return false
	}
	input["coverageRepairContextInherited"] = true
	input["coverageRepairRequired"] = true
	return true
}

func structuredFailureSignalsContainShapeConflict(signals map[string]any) bool {
	if structuredFailureSignalsAreRuntimeContractOnly(signals) {
		return false
	}
	for _, key := range []string{"messages", "identifiers"} {
		for _, value := range stringSliceFromValue(signals[key]) {
			if textContainsShapeConflictDiagnostic(value) {
				return true
			}
		}
	}
	return false
}

func primaryDiagnosticRepairContractFromSignals(signals map[string]any) map[string]any {
	if len(signals) == 0 || !structuredFailureSignalsContainShapeConflict(signals) {
		return nil
	}
	lineRefs := mapSliceFromValue(signals["lineRefs"])
	targetRefs := primaryDiagnosticTargetRefs(signals, lineRefs)
	identifiers := stringSliceFromValue(signals["identifiers"])
	messages := stringSliceFromValue(signals["messages"])
	if len(lineRefs) == 0 && len(targetRefs) == 0 {
		return nil
	}
	ret := map[string]any{
		"required": true,
		"reason":   "line_or_shape_diagnostic_blocks_current_validation",
		"requirements": []string{
			"address line-referenced resource shape blockers before secondary changes",
			"patch the diagnostic target lines or declare a matching primaryDiagnosticResolution/addressesFailureSignals contract",
			"when patching a related target, declare relatedFailureTargets and reference the diagnostic identifiers or line refs being unblocked",
		},
	}
	if len(targetRefs) > 0 {
		ret["targetRefs"] = targetRefs
	}
	if len(lineRefs) > 0 {
		ret["lineRefs"] = lineRefs
	}
	if len(identifiers) > 0 {
		ret["identifiers"] = limitedStrings(compactUniqueStrings(identifiers), maxLLMContextSliceItems)
	}
	if len(messages) > 0 {
		ret["messages"] = limitedStrings(compactUniqueStrings(messages), 5)
	}
	return ret
}

func structuredFailureSignalsAreRuntimeContractOnly(signals map[string]any) bool {
	if len(signals) == 0 || len(mapSliceFromValue(signals["lineRefs"])) > 0 {
		return false
	}
	identifiers := lowerStringSet(stringSliceFromValue(signals["identifiers"]))
	for _, externalClass := range []string{
		failureClassShellCommand,
		failureClassPatchApply,
		failureClassMCPCapabilityDenied,
		failureClassMCPRuntime,
	} {
		if identifiers[strings.ToLower(externalClass)] {
			return false
		}
	}
	messages := strings.ToLower(strings.Join(stringSliceFromValue(signals["messages"]), "\n"))
	if messages == "" {
		return false
	}
	if !(identifiers[failureClassRuntime] || identifiers[failureClassLLMDecisionInvalid]) {
		return false
	}
	for _, marker := range []string{
		"state-changing llm repair step returned",
		"state-changing execution-stage llm graph node returned",
		"executiongraph patch/apply input invalid",
		"executiongraph patch/apply must not depend",
		"patch/apply must not depend on same-graph file/read",
		"patch text_replace oldtext/newtext must describe",
		"oldtext/newtext must describe a substantive change",
		"primary diagnostic repair requires patch/apply",
		"strategyreassessment.required",
		"invalid llm decision json",
		"json: unknown field",
		"concrete validation nodes must declare",
		"validationevidence.",
		"dodcoverage.",
		"dodassessment.",
		"repairactionpackage.",
	} {
		if strings.Contains(messages, marker) {
			return true
		}
	}
	if (identifiers["workspace_execution"] || identifiers["executiongraph"]) &&
		(identifiers["llm_execution_graph_plan"] || identifiers["llm_schema_invalid"]) {
		return true
	}
	return false
}

func lowerStringSet(values []string) map[string]bool {
	ret := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			ret[value] = true
		}
	}
	return ret
}

func primaryDiagnosticTargetRefs(signals map[string]any, lineRefs []map[string]any) []string {
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeStructuredFailureTargetRef(value)
		if value == "" || !runtimeRepairTargetRefAdmissible(value) || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, ref := range lineRefs {
		add(stringFromMap(ref, "targetRef"))
	}
	for _, value := range stringSliceFromValue(signals["targetRefs"]) {
		add(value)
	}
	return limitedStrings(ret, maxLLMContextSliceItems)
}

func textContainsShapeConflictDiagnostic(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	for _, token := range []string{
		"undefined:",
		"undefined ",
		"unknown field",
		"type mismatch",
		"cannot use",
		"missing field",
		"missing method",
		"does not implement",
		"unresolved",
		"unresolved reference",
		"no such field",
		"no such method",
		"schema key",
	} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

func stateChangingRepairGuidance() []string {
	return []string{
		"Return concrete state-changing output with complete content, or return a real governance blocker when external information, permission, approval, credential, or configuration is required.",
		"If multiple dependent operations are needed, executionGraph must contain an auditable state-changing operation accepted by repairActionPackage.requiredOutput; patch/apply is preferred when it can represent the change.",
		"Do not return read-only, inspection-only, or LLM-gated executionGraph.",
		"When current target snapshots are already provided in failureTargetRepairContext, patchRepairContext, or repairActionPackage.targetSnapshots, do not add file/read before patch/apply in the same executionGraph; use the provided snapshot directly or return a real governance blocker if it is insufficient.",
		"patch/apply supports file_write, unified_diff, and text_replace; file_write content must be non-empty and must not be used as a delete operation.",
		"For patch/apply text_replace, input.oldText must be an exact non-empty literal string from the current target snapshot and input.newText must be an exact literal string that may be empty only to delete oldText; for file_write, input.content must be complete non-empty literal content; for unified_diff, input.diff must be a complete standard unified diff with explicit line ranges.",
		"When changing an existing target file, prefer text_replace with exact oldText/newText or the smallest unified_diff over file_write; use file_write primarily to create missing targets or replace a known full file snapshot intentionally.",
		"unified_diff patches must include explicit hunk line ranges such as @@ -12,3 +12,4 @@; never emit a bare @@ hunk.",
		"Do not use shell/command or mcp/tool_call to modify workspace files when a patch/apply state-change artifact can represent the change; when acceptedKinds explicitly allows them, declare stateChanging=true plus targetRef, relatedFailureTargets, or stateChangingReason, then add bounded verification that depends on the operation.",
		"If a shell/command or mcp/tool_call is the only practical way to materialize derived workspace state, it must explicitly declare stateChanging=true, targetRef, stateChangingReason, and relatedFailureTargets/targetRefs/affectedTargets; coverageTargets or summary alone are not an auditable state-changing declaration.",
		"For scoped repair verification, shell/command input must declare targetRef, relatedFailureTargets, verificationScope, or boundedReason, or depend on a patch/apply node that directly targets the current failure scope.",
		"Any MCP tool call that synchronizes modified file content must include the complete current file content in input.content together with the target path; do not emit path-only, root-only, or partial sync requests.",
		"When failedStepOutput identifies concrete target resources, treat those target snapshots as authoritative and repair the target directly instead of changing unrelated prerequisite metadata.",
		"When repair target snapshots or failure target scope are present, patch/apply must target that failure scope/current repair snapshot, or explicitly declare relatedFailureTargets that cite the current failure target being validated or unblocked; unrelated marker/no-op patches are invalid.",
		"When a workspace already contains known resources, reconcile the repair with those existing resources and prefer the smallest coherent state change over broad replacement.",
		"When repair context contains redacted or truncated content, do not copy it into patch/apply; first read the target or produce a minimal exact diff from visible current evidence.",
		"When fixing failures caused by missing names, invalid attributes, or contract-entry mismatches, patch against names that already exist in current workspace resources unless the Goal explicitly requires creating a new named element.",
		"When failure diagnostics include concrete line refs or resource-shape mismatches, fix the reported execution/validation blockers in the failure target before secondary validation, formatting, or evidence refinements.",
		"Do not return self-inspection, run snapshot, observability, or context rehydration graphs when effective Goal and DoD are already present.",
		"If a target file is missing in an empty or new workspace, create it with patch/apply file_write instead of reading it again.",
		"Do not use placeholders such as __FROM_PREVIOUS_STEP_OUTPUT__, <<...>>, ${...}, or {{...}} in patch fields; use concrete literal values or omit optional fields.",
	}
}

func concreteFailureEvidenceAcquisitionGuidance() []string {
	return []string{
		"Acquire bounded concrete failure evidence before producing a patch.",
		"Use only targeted file/list, file/read, shell/command, mcp/tool_call, or equivalent concrete verification capabilities.",
		"Every executionGraph node must be an evidence acquisition node from repairActionPackage.requiredOutput.acceptedKinds; do not include patch/apply, patch/preview, or llm/workspace_execution in this step.",
		"Do not return patch/apply until current failure evidence identifies a concrete target, line ref, repair target set, coverage target, or current target snapshot.",
		"Do not use self-inspection, run snapshot, observability-only, context rehydration, or nested LLM graphs as evidence acquisition.",
		"If bounded verification produces target refs, line refs, stdout/stderr, exit code, or current snapshots, let the next repair use that evidence for target-scoped state-changing repair.",
	}
}

func repairStrategyReassessmentFromInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	targetAttempts := int64FromValue(input["targetRepairAttempts"])
	repairAttempt := int64FromValue(input[stepInputKeyRuntimeRepairAttempt])
	targetScope := strings.TrimSpace(stringFromMap(input, "failureTargetScope"))
	failureClass := strings.TrimSpace(stringFromMap(input, failureOutputKeyClass))
	failureSignature := strings.TrimSpace(stringFromMap(input, failureOutputKeySignature))
	if targetAttempts < 2 && !(repairAttempt >= 3 && (targetScope != "" || failureClass != "" || failureSignature != "")) {
		return nil
	}
	reason := "repeated_repair_failure"
	if targetAttempts >= 2 {
		reason = "same_target_scope_repeated_failure"
	}
	failedSurface := repairStrategyFailedSurfaceFromInput(input)
	ret := map[string]any{
		"required":             true,
		"reason":               reason,
		"failedSurface":        failedSurface,
		"repairAttempt":        repairAttempt,
		"targetRepairAttempts": targetAttempts,
		"requiredDecisionFields": []string{
			"failedSurface",
			"rootCauseTarget",
			"repairTargetDecision",
			"repairMode",
			"forbiddenRepair",
			"verificationPlan",
		},
		"requirements": []string{
			"choose repairMode as continue_same_target_repair, shift_root_cause_target, or return blocker/failed assessment",
			"explain whether the next change targets state-change artifact, evidence, verification, or blocker handling",
			"when continuing the same target, address the current primary diagnostics using current target evidence and bounded verification",
			"when shifting root-cause target, use a neutral grounded candidate and relate it to the failed surface",
			"do not weaken still-valid Goal or DoD constraints to make verification pass",
		},
	}
	if failureClass != "" {
		ret["failureClass"] = failureClass
	}
	if targetScope != "" {
		ret["failureTargetScope"] = targetScope
	}
	return ret
}

func refreshRepairStrategyReassessment(input map[string]any) {
	if len(input) == 0 {
		return
	}
	refreshed := repairStrategyReassessmentFromInput(input)
	if len(refreshed) == 0 {
		return
	}
	current := mapFromValue(input["strategyReassessment"])
	if len(current) == 0 || repairStrategySurfaceMoreSpecific(refreshed, current) {
		input["strategyReassessment"] = refreshed
	}
}

func repairStrategyFailedSurfaceFromInput(input map[string]any) string {
	targetScope := strings.TrimSpace(stringFromMap(input, "failureTargetScope"))
	if targetScope != "" {
		return targetScope
	}
	targetRefs := repairStrategyTargetRefsFromInput(input)
	if len(targetRefs) == 1 {
		return "targetRef=" + targetRefs[0]
	}
	if len(targetRefs) > 1 {
		return "failureTargets=" + strings.Join(limitedStrings(targetRefs, maxLLMContextSliceItems), ",")
	}
	failureSignature := strings.TrimSpace(stringFromMap(input, failureOutputKeySignature))
	failureClass := strings.TrimSpace(stringFromMap(input, failureOutputKeyClass))
	return firstNonEmptyString(failureSignature, failureClass, strings.TrimSpace(stringFromMap(input, "failedStepKind")))
}

func repairStrategyTargetRefsFromInput(input map[string]any) []string {
	seen := map[string]bool{}
	var refs []string
	add := func(ref string) {
		ref = filepath.ToSlash(filepath.Clean(strings.TrimSpace(ref)))
		if ref == "" || ref == "." || seen[ref] {
			return
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	for _, ref := range stringSliceFromValue(input["repairTargetSet"]) {
		add(ref)
	}
	for _, key := range []string{"targetSnapshots", "currentTargetSnapshots", "failureTargetRepairContext", "patchRepairContext"} {
		for _, item := range mapSliceFromValue(input[key]) {
			add(stringFromMap(item, "targetRef"))
		}
	}
	return refs
}

func repairStrategySurfaceMoreSpecific(next map[string]any, current map[string]any) bool {
	nextSurface := strings.TrimSpace(stringFromMap(next, "failedSurface"))
	currentSurface := strings.TrimSpace(stringFromMap(current, "failedSurface"))
	if nextSurface == "" || nextSurface == currentSurface {
		return false
	}
	return len(strategySurfaceTargetRefs(nextSurface)) > 0 && len(strategySurfaceTargetRefs(currentSurface)) == 0
}

func concreteValidationEvidenceRepairGuidance() []string {
	return []string{
		"Do not repeat assessment-only validation.",
		"Produce concrete verification evidence for the failed Goal/DoD coverage before attempting state-changing repair.",
		"Return a bounded executionGraph that runs available verification capabilities through the deliverable entrypoint, targeted tests, or an equivalent controlled substitute; do not cite implementation patches as behavior proof.",
		"For API, CLI, service, proxy, streaming, error, token, persistence, console, or concurrency claims, prefer a local deterministic fake/stub/fixture plus the production entrypoint when real external prerequisites are unavailable.",
		"Each shell/command or mcp/tool_call intended to close behavior coverage must declare input.validationEvidence.mode and input.validationEvidence.observationType as runtime behavior, executable contract, dynamic execution, or controlled substitute evidence.",
		"Minimal validation node shape: {\"kind\":\"shell/command\",\"input\":{\"command\":\"sh\",\"args\":[\"-c\",\"printf verified\"],\"validationEvidence\":{\"mode\":\"runtime_behavior\",\"observationType\":\"runtime_observation\"}}}.",
		"If a dependency or environment prerequisite is unavailable, prefer a neutral local fixture, mock/stub, temporary config, isolated working directory, or alternate available port when that can validate the same contract without changing deliverables.",
		"Concrete validation shell commands must be bounded and non-destructive: do not use nohup, setsid, background '&' service launches, kill/pkill/killall/fuser -k, or approval requests for process termination; use a foreground timeout-controlled harness or controlled substitute instead.",
		"Do not use read-only inspection, self-inspection, run snapshot, observability, or context rehydration graphs as evidence repair.",
		"If a verification command or tool later fails with concrete diagnostics, that failure may drive a separate state-changing repair.",
	}
}

func narrowStateChangingRecoveryInput(input map[string]any, failedStep *runModel.Step, execErr error, source string) {
	if input == nil || failedStep == nil || !boolValue(input["stateChangingOutputRequired"]) {
		return
	}
	class := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(input, failureOutputKeyClass),
		stringFromMap(failedStep.Output, failureOutputKeyClass),
	))
	stage := strings.TrimSpace(stringFromMap(failedStep.Output, "stage"))
	errorText := strings.Join([]string{
		errorString(execErr),
		stringFromMap(failedStep.Output, "error"),
		stringFromMap(failedStep.Output, "reason"),
	}, "\n")
	if !providerTransientStreamFailure(class, stage) && !stateChangingReadOnlyGraphFailureText(errorText) {
		return
	}
	input["stateChangingRecoveryMode"] = "direct_state_change"
	input["stateChangingRecoverySource"] = strings.TrimSpace(source)
	input["providerTimeoutStateChangingRetry"] = class == failureClassProviderTimeout || stage == "llm_provider_timeout"
	input["forbiddenRecoveryOutputs"] = []string{
		"read_only_graph",
		"self_inspection_graph",
		"run_snapshot_graph",
		"observability_graph",
		"context_rehydration_graph",
		"llm_gated_replan_graph",
	}
	input["requiredRecoveryOutputs"] = []string{
		"state_changing_artifact",
		"failed_dod_assessment",
		"real_decision_or_approval_blocker",
	}
	input["failedStepOutput"] = compactStateChangingRecoveryFailureOutput(failedStep.Output)
	delete(input, "previousStepOutput")
	delete(input, "failedExecutionGraph")
	delete(input, stepInputKeyGraphEvidence)
	delete(input, "runEvidenceSummary")
	if strings.TrimSpace(source) == "provider_retry" {
		delete(input, stepInputKeyWorkspaceFileManifest)
	}
	delete(input, stepOutputKeyExecutionGraph)
}

func providerTransientStreamFailure(class, stage string) bool {
	switch strings.TrimSpace(class) {
	case failureClassProviderTimeout, failureClassProviderStreamIncomplete:
		return true
	}
	switch strings.TrimSpace(stage) {
	case "llm_provider_timeout", "llm_provider_stream_incomplete":
		return true
	default:
		return false
	}
}
