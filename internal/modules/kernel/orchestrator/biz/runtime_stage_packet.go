package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const stepInputKeyStagePacket = "stagePacket"

func attachStagePacketToStepInput(run *runModel.Run, stepID common.StepID, input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	input = normalizeForcedNarrowManagedValidationInput(input)
	input = ensureStagePacketCurrentFailureInput(input)
	packet := buildStagePacketFromStepInput(run, stepID, input)
	if !packet.HasScope() {
		return input
	}
	input[stepInputKeyStagePacket] = packet.ToMap()
	return input
}

func ensureStagePacketCurrentFailureInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	if len(mapFromValue(input["currentFailure"])) > 0 {
		return input
	}
	failure := stagePacketCurrentFailure(input)
	if len(failure) == 0 {
		return input
	}
	input["currentFailure"] = failure
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) > 0 && len(mapFromValue(pkg["currentFailure"])) == 0 {
		pkg["currentFailure"] = failure
		input["repairActionPackage"] = pkg
	}
	return input
}

func buildStagePacketFromStepInput(run *runModel.Run, stepID common.StepID, input map[string]any) runtimemodel.StagePacket {
	if run == nil {
		return runtimemodel.StagePacket{}
	}
	packet := runtimemodel.StagePacket{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      stepID,
		Definition: runtimemodel.StagePacketDefinition{
			Revision: stagePacketDefinitionRevision(run, input),
			GoalID:   run.GoalID,
			DoDID:    run.DoDID,
		},
		TemplatePolicyRefs:      stagePacketTemplatePolicyRefs(run, input),
		ActiveStage:             stagePacketActiveStage(input),
		ActiveFocus:             stagePacketActiveFocus(input),
		NextAction:              stagePacketNextAction(run, input),
		RequiredOutput:          stagePacketRequiredOutput(input),
		AllowedActions:          stringSliceFromValue(input["allowedActions"]),
		ForbiddenActions:        stringSliceFromValue(input["forbiddenActions"]),
		RequiredEvidenceKinds:   stringSliceFromValue(input["requiredEvidenceKinds"]),
		AcceptedEvidenceRefs:    stagePacketEvidenceRefs(input, "acceptedEvidenceRefs"),
		RejectedEvidenceRefs:    stagePacketEvidenceRefs(input, "rejectedEvidenceRefs"),
		CurrentFailure:          stagePacketCurrentFailure(input),
		CurrentGap:              stagePacketCurrentGap(input),
		ProcessEvidenceTimeline: stagePacketProcessEvidenceTimeline(input),
		CompactSummaryRef:       stagePacketCompactSummaryRef(input),
		MemoryRefs:              stagePacketMemoryRefs(input),
		TargetSnapshots:         stagePacketTargetSnapshots(input),
	}
	return packet.Normalized()
}

func stagePacketDefinitionRevision(run *runModel.Run, input map[string]any) string {
	for _, value := range []string{
		strings.TrimSpace(stringFromMap(input, "definitionRevision")),
		strings.TrimSpace(stringFromMap(mapFromValue(input["executionContext"]), "definitionRevision")),
		strings.TrimSpace(stringFromMap(coverageWorkItemContextFromInput(input), "definitionRevision")),
		strings.TrimSpace(stringFromMap(mapFromValue(input[gapBaselineKey]), "definitionRevision")),
	} {
		if value != "" {
			return value
		}
	}
	if run != nil {
		for _, value := range []string{
			strings.TrimSpace(run.DefinitionRevision),
			strings.TrimSpace(stringFromMap(run.ExecutionContext, "definitionRevision")),
			strings.TrimSpace(stringFromMap(mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]), "definitionRevision")),
			strings.TrimSpace(stringFromMap(mapFromValue(run.ExecutionContext[gapBaselineKey]), "definitionRevision")),
		} {
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func stagePacketNextAction(run *runModel.Run, input map[string]any) *runtimemodel.NextAction {
	action := currentActionNextActionFromInput(input)
	actionType := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(action, "type"),
		stringFromMap(action, "kind"),
	))
	if actionType == "" || run == nil {
		return nil
	}
	authorityFocus := dispatchAuthorityCurrentFocus(input)
	coverageFocus := dispatchCoverageFocusFromAuthority(input, authorityFocus)
	if len(coverageFocus) == 0 {
		coverageFocus = stagePacketActiveFocus(input)
	}
	next := runtimemodel.NextAction{
		WorkspaceID:           run.WorkspaceID,
		RunID:                 run.ID,
		Type:                  actionType,
		SubjectType:           strings.TrimSpace(stringFromMap(action, "subjectType")),
		SubjectID:             strings.TrimSpace(stringFromMap(action, "subjectId")),
		FocusCoverageItemIDs:  stagePacketNextActionCoverageIDs(action, coverageFocus),
		RequiredOutput:        stagePacketRequiredOutput(input),
		AllowedActions:        firstNonEmptyStringSlice(stringSliceFromValue(action["allowedActions"]), stringSliceFromValue(input["allowedActions"])),
		ForbiddenActions:      firstNonEmptyStringSlice(stringSliceFromValue(action["forbiddenActions"]), stringSliceFromValue(input["forbiddenActions"])),
		RequiredEvidenceKinds: firstNonEmptyStringSlice(stringSliceFromValue(action["requiredEvidenceKinds"]), stringSliceFromValue(input["requiredEvidenceKinds"])),
		Stage:                 firstNonEmptyString(stringFromMap(action, "stage"), stagePacketActiveStage(input)),
		StageMode:             firstNonEmptyString(stringFromMap(action, "stageMode"), stringFromMap(input, stepInputKeyRuntimeStageMode)),
		Reason:                strings.TrimSpace(stringFromMap(action, "reason")),
	}
	if next.SubjectID == "" {
		next.SubjectID = strings.TrimSpace(firstNonEmptyString(
			stringFromMap(coverageFocus, "id"),
			stringFromMap(action, "subjectId"),
		))
	}
	if next.SubjectType == "" && next.SubjectID != "" {
		next.SubjectType = "coverage_focus"
	}
	normalized := next.Normalized()
	if normalized.Type == "" {
		return nil
	}
	return &normalized
}

func stagePacketNextActionCoverageIDs(action map[string]any, coverageFocus map[string]any) []common.CoverageWorkItemID {
	values := stringSliceFromValue(action["focusCoverageItemIds"])
	if len(values) == 0 {
		values = stringSliceFromValue(action["coverageWorkItemIds"])
	}
	if len(values) == 0 {
		values = dispatchFocusWorkItemIDs(coverageFocus)
	}
	if len(values) == 0 {
		return nil
	}
	ret := make([]common.CoverageWorkItemID, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			ret = append(ret, common.CoverageWorkItemID(value))
		}
	}
	return ret
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func stagePacketActiveStage(input map[string]any) string {
	for _, value := range []string{
		strings.TrimSpace(stringFromMap(input, stepInputKeyRuntimeStage)),
		strings.TrimSpace(stringFromMap(input, stepInputKeySoulStage)),
		strings.TrimSpace(stringFromMap(mapFromValue(input["executionContext"]), stepInputKeyRuntimeStage)),
	} {
		if value != "" {
			return value
		}
	}
	return ""
}

func stagePacketTemplatePolicyRefs(run *runModel.Run, input map[string]any) []string {
	var refs []string
	for _, source := range []map[string]any{
		input,
		mapFromValue(input["executionContext"]),
		mapFromValue(input[stepInputKeyExecutionContextPackage]),
		mapFromValue(mapFromValue(input[stepInputKeyExecutionContextPackage])["selectedContext"]),
	} {
		refs = append(refs, stagePacketRefsFromKeys(source, "templatePolicyRefs", "policyRefs")...)
	}
	if run != nil {
		for _, source := range []map[string]any{
			run.ExecutionContext,
			mapFromValue(run.ExecutionContext[stepInputKeyExecutionContextPackage]),
			mapFromValue(mapFromValue(run.ExecutionContext[stepInputKeyExecutionContextPackage])["selectedContext"]),
		} {
			refs = append(refs, stagePacketRefsFromKeys(source, "templatePolicyRefs", "policyRefs")...)
		}
	}
	return compactUniqueStrings(refs)
}

func stagePacketActiveFocus(input map[string]any) map[string]any {
	for _, source := range []map[string]any{
		mapFromValue(input[convergenceAuthorityKey]),
		coverageWorkItemContextFromInput(input),
		mapFromValue(input["repairActionPackage"]),
		mapFromValue(input["acceptance"]),
		mapFromValue(input[coverageFocusKey]),
		mapFromValue(mapFromValue(input["executionContext"])[coverageFocusKey]),
	} {
		if focus := mapFromValue(source["currentFocus"]); len(focus) > 0 {
			return cloneStepPayload(focus)
		}
		for _, key := range []string{"currentFocusItems", "coverageTargetSet", "repairTargetSet"} {
			for _, item := range mapSliceFromValue(source[key]) {
				if len(item) == 0 {
					continue
				}
				focus := cloneStepPayload(item)
				if stringFromMap(focus, "id") == "" {
					if id := firstNonEmptyString(stringFromMap(focus, "coverageWorkItemId"), stringFromMap(focus, "coverageTargetToken")); id != "" {
						focus["id"] = id
					}
				}
				return focus
			}
		}
		if len(source) > 0 && strings.TrimSpace(stringFromMap(source, "id")) != "" {
			return cloneStepPayload(source)
		}
	}
	return nil
}

func stagePacketRequiredOutput(input map[string]any) map[string]any {
	state := strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state"))
	if state == convergenceStateNeedGovernance {
		return nil
	}
	authorityFocus := dispatchAuthorityCurrentFocus(input)
	if state == convergenceStateNeedAssessment {
		return assessmentRequiredOutputForAuthorityFocus(dispatchCoverageFocusFromAuthority(input, authorityFocus))
	}
	for _, source := range []map[string]any{
		mapFromValue(input["repairActionPackage"]),
		mapFromValue(input),
		mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"]),
	} {
		if required := mapFromValue(source["requiredOutput"]); len(required) > 0 {
			if len(authorityFocus) > 0 {
				return normalizeRequiredOutputFocusBinding(required, dispatchCoverageFocusFromAuthority(input, authorityFocus))
			}
			return cloneStepPayload(required)
		}
	}
	return nil
}

func stagePacketEvidenceRefs(input map[string]any, key string) []string {
	if len(input) == 0 || strings.TrimSpace(key) == "" {
		return nil
	}
	refs := make([]string, 0)
	for _, source := range stagePacketEvidenceSources(input) {
		refs = append(refs, stringSliceFromValue(source[key])...)
		for _, nestedKey := range []string{"currentFocus", "activeFocus"} {
			refs = append(refs, stringSliceFromValue(mapFromValue(source[nestedKey])[key])...)
		}
		for _, listKey := range []string{"currentFocusItems", "ledgerSummary", "coverageTargetSet", "repairTargetSet"} {
			for _, item := range mapSliceFromValue(source[listKey]) {
				refs = append(refs, stringSliceFromValue(item[key])...)
			}
		}
	}
	return compactUniqueStrings(refs)
}

func stagePacketEvidenceSources(input map[string]any) []map[string]any {
	return []map[string]any{
		input,
		mapFromValue(input[convergenceAuthorityKey]),
		mapFromValue(input["executionContext"]),
		mapFromValue(mapFromValue(input["executionContext"])[convergenceAuthorityKey]),
		coverageWorkItemContextFromInput(input),
		mapFromValue(input["repairActionPackage"]),
		mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"]),
		mapFromValue(input["acceptance"]),
	}
}

func stagePacketCurrentFailure(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	for _, source := range []map[string]any{
		input,
		mapFromValue(input["repairActionPackage"]),
		mapFromValue(input["executionContext"]),
		mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"]),
	} {
		if failure := mapFromValue(source["currentFailure"]); len(failure) > 0 {
			return compactLLMContextMap(failure, 1)
		}
	}
	for _, source := range []map[string]any{
		input,
		mapFromValue(input["executionContext"]),
	} {
		if recovery := mapFromValue(source[stepInputKeyContractViolationRecovery]); len(recovery) > 0 {
			return stagePacketCurrentFailureFromContractViolationRecovery(recovery)
		}
		if digestFailure := currentFailureFromProgressDigest(mapFromValue(source[stepInputKeyCurrentProgressDigest])); len(digestFailure) > 0 {
			return compactLLMContextMap(digestFailure, 1)
		}
	}
	if stagePacketShouldDeriveCoverageGapFailure(input) {
		for _, context := range []map[string]any{
			coverageWorkItemContextFromInput(input),
			mapFromValue(mapFromValue(input["repairActionPackage"])[coverageWorkItemContextKey]),
			mapFromValue(mapFromValue(input["executionContext"])[coverageWorkItemContextKey]),
			mapFromValue(mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"])[coverageWorkItemContextKey]),
		} {
			if failure := currentFailureFromCoverageWorkItemContext(context); len(failure) > 0 {
				return compactLLMContextMap(failure, 1)
			}
		}
		if focus := stagePacketActiveFocus(input); coverageWorkItemHasOpenGap(focus) {
			if failure := currentFailureFromCoverageWorkItemContext(map[string]any{"currentFocus": focus}); len(failure) > 0 {
				return compactLLMContextMap(failure, 1)
			}
		}
	}
	return nil
}

func stagePacketShouldDeriveCoverageGapFailure(input map[string]any) bool {
	if boolValue(input[stepInputKeyRuntimeRepairFollowUp]) ||
		boolValue(input["concreteFailureEvidenceRequired"]) {
		return true
	}
	if strings.TrimSpace(stringFromMap(input, "repairObjective")) != "" {
		return true
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if boolValue(required["mustProduceConcreteFailureEvidence"]) {
		return true
	}
	switch strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state")) {
	case convergenceStateNeedEvidence, convergenceStateNeedStateChange:
		return true
	default:
		return false
	}
}

func stagePacketCurrentFailureFromContractViolationRecovery(recovery map[string]any) map[string]any {
	if len(recovery) == 0 {
		return nil
	}
	ret := map[string]any{"source": "contractViolationRecovery"}
	for _, key := range []string{
		"violatedRule",
		"violationType",
		"recoverability",
		"retryHint",
		"failureClass",
		"failureSignature",
		"rejectedOutputRef",
		"expectedContract",
		"actualOutputSummary",
	} {
		if value, ok := recovery[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if len(ret) == 1 {
		return nil
	}
	return ret
}

func stagePacketCurrentGap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	for _, source := range []struct {
		name  string
		value any
	}{
		{name: "input.currentGap", value: input["currentGap"]},
		{name: "repairActionPackage.currentGap", value: mapFromValue(input["repairActionPackage"])["currentGap"]},
		{name: "executionContext.currentGap", value: mapFromValue(input["executionContext"])["currentGap"]},
		{name: "executionContext.repairActionPackage.currentGap", value: mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"])["currentGap"]},
	} {
		if gap := stagePacketCurrentGapFromValue(source.value, source.name); len(gap) > 0 {
			return gap
		}
	}
	authorityFocus := dispatchAuthorityCurrentFocus(input)
	for _, source := range []struct {
		name  string
		focus map[string]any
	}{
		{name: "authority.currentFocus", focus: authorityFocus},
		{name: "coverage.currentFocus", focus: dispatchCoverageFocusFromAuthority(input, authorityFocus)},
		{name: "stagePacket.activeFocus", focus: stagePacketActiveFocus(input)},
	} {
		if gap := stagePacketCurrentGapFromFocus(source.focus, source.name); len(gap) > 0 {
			return gap
		}
	}
	for _, source := range []struct {
		name   string
		digest map[string]any
	}{
		{name: "currentProgressDigest", digest: mapFromValue(input[stepInputKeyCurrentProgressDigest])},
		{name: "executionContext.currentProgressDigest", digest: mapFromValue(mapFromValue(input["executionContext"])[stepInputKeyCurrentProgressDigest])},
	} {
		if gap := stagePacketCurrentGapFromProgressDigest(source.digest, source.name); len(gap) > 0 {
			return gap
		}
	}
	return nil
}

func stagePacketCurrentGapFromValue(value any, source string) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			return nil
		}
		ret := compactLLMContextMap(typed, 1)
		if len(ret) == 0 {
			return nil
		}
		if strings.TrimSpace(stringFromMap(ret, "source")) == "" {
			ret["source"] = source
		}
		if len(ret) == 1 {
			return nil
		}
		return ret
	case string:
		return stagePacketCurrentGapFromText(typed, source)
	default:
		return nil
	}
}

func stagePacketCurrentGapFromFocus(focus map[string]any, source string) map[string]any {
	if len(focus) == 0 {
		return nil
	}
	ret := map[string]any{"source": source}
	if id := strings.TrimSpace(stringFromMap(focus, "id")); id != "" {
		ret["focusId"] = id
	}
	if summary := stagePacketCurrentGapText(stringFromMap(focus, "currentGap")); summary != "" {
		ret["summary"] = summary
	}
	if missing := compactUniqueStrings(stringSliceFromValue(focus["missingEvidenceKinds"])); len(missing) > 0 {
		ret["missingEvidenceKinds"] = missing
	}
	if ids := compactUniqueStrings(dispatchFocusWorkItemIDs(focus)); len(ids) > 0 {
		ret["coverageWorkItemIds"] = ids
	}
	if refs := compactUniqueStrings(stringSliceFromValue(focus["structuredTargetRefs"])); len(refs) > 0 {
		ret["structuredTargetRefs"] = refs
	}
	if refs := compactUniqueStrings(stringSliceFromValue(focus["targetRefs"])); len(refs) > 0 {
		ret["targetRefs"] = refs
	}
	if len(ret) == 1 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func stagePacketCurrentGapFromProgressDigest(digest map[string]any, source string) map[string]any {
	if len(digest) == 0 {
		return nil
	}
	ret := map[string]any{"source": source}
	if summary := stagePacketCurrentGapText(stringFromMap(digest, "currentGap")); summary != "" {
		ret["summary"] = summary
	}
	if missing := compactUniqueStrings(stringSliceFromValue(digest["missingEvidenceKinds"])); len(missing) > 0 {
		ret["missingEvidenceKinds"] = missing
	}
	if ids := compactUniqueStrings(stringSliceFromValue(digest["coverageWorkItemIds"])); len(ids) > 0 {
		ret["coverageWorkItemIds"] = ids
	}
	if refs := compactUniqueStrings(stringSliceFromValue(digest["structuredTargetRefs"])); len(refs) > 0 {
		ret["structuredTargetRefs"] = refs
	}
	if refs := compactUniqueStrings(stringSliceFromValue(digest["targetRefs"])); len(refs) > 0 {
		ret["targetRefs"] = refs
	}
	if len(ret) == 1 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func stagePacketCurrentGapFromText(value string, source string) map[string]any {
	summary := stagePacketCurrentGapText(value)
	if summary == "" {
		return nil
	}
	return map[string]any{
		"source":  source,
		"summary": summary,
	}
}

func stagePacketCurrentGapText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "none_observed_in_recent_process_evidence" {
		return ""
	}
	return truncateLLMContextString(value)
}

func stagePacketProcessEvidenceTimeline(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	for _, value := range []any{
		input[stepInputKeyProcessEvidenceTimeline],
		mapFromValue(input["executionContext"])[stepInputKeyProcessEvidenceTimeline],
	} {
		if timeline := mapSliceFromValue(value); len(timeline) > 0 {
			return compactProcessEvidenceTimelineForRuntimeContext(timeline)
		}
	}
	return nil
}

func stagePacketCompactSummaryRef(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	for _, source := range []map[string]any{
		input,
		mapFromValue(input["executionContext"]),
		mapFromValue(input[stepInputKeyExecutionContextPackage]),
	} {
		for _, key := range []string{"compactSummaryRef", "historicalSummaryRef", "summaryRef"} {
			if value := strings.TrimSpace(stringFromMap(source, key)); value != "" {
				return value
			}
		}
	}
	for _, item := range stagePacketProcessEvidenceTimeline(input) {
		if strings.TrimSpace(stringFromMap(item, "summaryKind")) != "compact_history" {
			continue
		}
		if value := strings.TrimSpace(stringFromMap(item, "sourceRef")); value != "" {
			return value
		}
	}
	return ""
}

func stagePacketMemoryRefs(input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}
	var refs []string
	for _, source := range []map[string]any{
		input,
		mapFromValue(input["executionContext"]),
		mapFromValue(input[stepInputKeyExecutionContextPackage]),
		mapFromValue(mapFromValue(input[stepInputKeyExecutionContextPackage])["memoryRetrieval"]),
		mapFromValue(mapFromValue(input[stepInputKeyExecutionContextPackage])["selectedContext"]),
		mapFromValue(input["memoryRetrieval"]),
		mapFromValue(input["selectedContext"]),
	} {
		refs = append(refs, stagePacketRefsFromKeys(source, "memoryRefs", "selectedMemoryIds", "retrievedMemoryIds")...)
		refs = append(refs, stagePacketRefsFromItems(source["memories"], "id", "memoryId", "ref", "sourceRef")...)
	}
	return compactUniqueStrings(refs)
}

func stagePacketRefsFromKeys(source map[string]any, keys ...string) []string {
	if len(source) == 0 {
		return nil
	}
	var refs []string
	for _, key := range keys {
		refs = append(refs, stagePacketRefsFromValue(source[key])...)
	}
	return refs
}

func stagePacketRefsFromValue(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	case []string:
		return compactUniqueStrings(typed)
	case []any:
		var refs []string
		for _, item := range typed {
			refs = append(refs, stagePacketRefsFromValue(item)...)
		}
		return compactUniqueStrings(refs)
	case map[string]any:
		var refs []string
		for _, value := range typed {
			refs = append(refs, stagePacketRefsFromValue(value)...)
		}
		return compactUniqueStrings(refs)
	case []map[string]any:
		var refs []string
		for _, item := range typed {
			refs = append(refs, stagePacketRefsFromItems([]map[string]any{item}, "id", "ref", "policyRef", "sourceRef")...)
		}
		return compactUniqueStrings(refs)
	default:
		return nil
	}
}

func stagePacketRefsFromItems(value any, keys ...string) []string {
	items := mapSliceFromValue(value)
	if len(items) == 0 {
		return nil
	}
	var refs []string
	for _, item := range items {
		for _, key := range keys {
			if value := strings.TrimSpace(stringFromMap(item, key)); value != "" {
				refs = append(refs, value)
				break
			}
		}
	}
	return compactUniqueStrings(refs)
}

func stagePacketTargetSnapshots(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0)
	for _, source := range stagePacketSnapshotSources(input) {
		for _, key := range []string{"targetSnapshots", "failureTargetRepairContext", "patchRepairContext"} {
			ret = append(ret, mapSliceFromValue(source[key])...)
		}
	}
	return runtimemodel.NormalizeStagePacketTargetSnapshots(ret)
}

func stagePacketSnapshotSources(input map[string]any) []map[string]any {
	return []map[string]any{
		input,
		mapFromValue(input["executionContext"]),
		mapFromValue(input["repairActionPackage"]),
		mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"]),
	}
}
