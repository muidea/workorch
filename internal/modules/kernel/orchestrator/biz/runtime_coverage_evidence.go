package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func coverageRequiredEvidenceKindsForSource(run *runModel.Run, source coverageWorkItemSource) []string {
	kinds := coverageDefaultRequiredEvidenceKinds(source)
	target := map[string]any{
		"kind":                     source.SourceScope,
		"source":                   source.Source,
		"id":                       source.SourceRequirementID,
		"targetText":               source.TargetText,
		"coverageTargetToken":      source.SourceRequirementID,
		"sourceRequirementAliases": source.SourceAliases,
	}
	for _, requirement := range coverageTargetEvidenceKindRequirements(run, []map[string]any{target}) {
		kinds = append(kinds, evidenceKindRequirementEvidenceKinds(requirement)...)
	}
	if runExecutionContractRequiresStateChange(run) && coverageSourceCanCarryStateChange(source) {
		kinds = append(kinds, "state_change")
	}
	return compactUniqueStrings(kinds)
}

func coverageDefaultRequiredEvidenceKinds(source coverageWorkItemSource) []string {
	if kinds := compactUniqueStrings(source.RequiredEvidenceKinds); len(kinds) > 0 {
		return kinds
	}
	return []string{"validation"}
}

func coverageRequiredEvidenceCountsForSource(source coverageWorkItemSource, requiredKinds []string) map[string]int {
	ret := map[string]int{}
	for _, kind := range requiredKinds {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		if count := source.RequiredEvidenceCounts[kind]; count > 0 {
			ret[kind] = count
			continue
		}
		ret[kind] = 1
	}
	return ret
}

func coverageSourceCanCarryStateChange(source coverageWorkItemSource) bool {
	if normalizeEvidenceKindRequirementScope(source.SourceScope) != "deliverable" {
		return false
	}
	return normalizedCoverageEvidenceRole(source.EvidenceRole) == "primary_state_change"
}

func normalizedCoverageEvidenceRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	role = strings.ReplaceAll(role, "-", "_")
	role = strings.ReplaceAll(role, " ", "_")
	switch role {
	case "primary", "primary_state_change", "implementation", "direct_state_change":
		return "primary_state_change"
	case "supplemental", "supplemental_evidence", "evidence", "ledger", "report", "snapshot", "validation":
		return "supplemental_evidence"
	default:
		return ""
	}
}

func coverageMissingEvidenceKinds(item map[string]any) []string {
	return deriveCoverageWorkItemState(item).MissingEvidenceKinds
}

func coverageMissingEvidenceKindsFromDetails(requiredKinds []string, acceptedByKind map[string][]string, requiredCounts map[string]int) []string {
	if len(requiredKinds) == 0 {
		requiredKinds = []string{"validation"}
	}
	missing := make([]string, 0, len(requiredKinds))
	for _, kind := range requiredKinds {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		requiredCount := requiredCounts[kind]
		if requiredCount <= 0 {
			requiredCount = 1
		}
		if coverageEvidenceCountForKind(kind, acceptedByKind[kind]) < requiredCount {
			missing = append(missing, kind)
		}
	}
	return compactUniqueStrings(missing)
}

func coverageEvidenceCountForKind(kind string, refs []string) int {
	refs = compactUniqueStrings(refs)
	switch strings.TrimSpace(kind) {
	case "decision":
		countable := make([]string, 0, len(refs))
		for _, ref := range refs {
			normalized := strings.ToLower(strings.TrimSpace(ref))
			if normalized == "" || strings.HasPrefix(normalized, "resolution-") || strings.Contains(normalized, "/resolution/") {
				continue
			}
			if coverageEvidenceRefLooksDecision(normalized) {
				countable = append(countable, normalized)
			}
		}
		return len(compactUniqueStrings(countable))
	case "approval":
		return len(filterCoverageEvidenceRefs(refs, coverageEvidenceRefLooksApproval))
	default:
		return len(refs)
	}
}

func coverageCurrentGap(missingKinds []string) string {
	if len(missingKinds) == 0 {
		return ""
	}
	return "missing accepted evidence binding for required evidence kind: " + strings.Join(missingKinds, ", ")
}

func coverageEvidenceRefsByKind(existing map[string]any, key string, requiredKinds []string) map[string][]string {
	ret := mapStringSliceValue(existing[key])
	if len(ret) > 0 {
		return ret
	}
	if len(requiredKinds) == 1 {
		legacyKey := "acceptedEvidenceRefs"
		if key == "rejectedEvidenceByKind" {
			legacyKey = "rejectedEvidenceRefs"
		}
		if refs := stringSliceFromValue(existing[legacyKey]); len(refs) > 0 {
			ret[requiredKinds[0]] = compactUniqueStrings(refs)
		}
	}
	return ret
}

func coverageFlattenEvidenceRefs(byKind map[string][]string, fallback []string) []string {
	refs := append([]string{}, fallback...)
	kinds := make([]string, 0, len(byKind))
	for kind := range byKind {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		refs = append(refs, byKind[kind]...)
	}
	return compactUniqueStrings(refs)
}

func coverageWorkItemStepUpdate(step *runModel.Step) coverageWorkItemUpdate {
	if step == nil {
		return coverageWorkItemUpdate{}
	}
	update := coverageWorkItemUpdate{
		WorkItemIDs:       coverageWorkItemIDsFromStep(step),
		TargetTokens:      coverageTargetTokensFromStep(step),
		EvidenceRefs:      coverageStepEvidenceRefs(step),
		Accepted:          step.State == common.StepStateCompleted,
		AcceptanceOptions: coverageEvidenceAcceptanceOptionsFromStep(step),
	}
	if stepKindCanProduceStateChangingEvidence(step.Kind) || runtimeStepDeclaresStateChangingEvidence(step) {
		update.EvidenceKinds = append(update.EvidenceKinds, "state_change")
	}
	if coverageStepCanProduceValidationEvidence(step) {
		update.EvidenceKinds = append(update.EvidenceKinds, "validation")
	}
	update.EvidenceKinds = compactUniqueStrings(update.EvidenceKinds)
	return update
}

func coverageStepCanProduceValidationEvidence(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	if !validationEvidenceCanSatisfyDoD(evidenceQualityFromRuntimeStep(step)) {
		return false
	}
	switch strings.TrimSpace(step.Kind) {
	case "shell/command", "mcp/tool_call":
		return true
	}
	return len(mapFromValue(step.Input["validationEvidence"])) > 0 ||
		len(mapFromValue(step.Output["validationEvidence"])) > 0 ||
		len(stringSliceFromValue(step.Output["evidenceRefs"])) > 0
}

type coverageEvidenceAcceptanceOptions struct {
	EnforceProofStrength bool
	Quality              runtimeEvidenceQuality
	TargetRefs           []string
	ObservedClaims       []string
	ProofKind            string
	CoverageAlignment    string
	Tags                 []string
	Reason               string
	InheritMatchedTarget bool
}

func coverageEvidenceAcceptanceOptionsFromStep(step *runModel.Step) coverageEvidenceAcceptanceOptions {
	if step == nil {
		return coverageEvidenceAcceptanceOptions{}
	}
	evidence := coverageValidationEvidenceMetadataFromStep(step)
	quality := evidenceQualityFromRuntimeStep(step)
	tags := append([]string{}, quality.Tags...)
	tags = append(tags, stringSliceFromValue(evidence["tags"])...)
	tags = append(tags, stringSliceFromValue(evidence["evidenceTags"])...)
	targetRefs := append([]string{}, coverageValidationStructuredTargetRefsFromEvidence(evidence)...)
	targetRefs = append(targetRefs, coverageStructuredBindingRefsFromValue(runtimeStepCoverageTargetRefs(step))...)
	targetRefs = append(targetRefs, coverageStructuredBindingRefsFromValue(step.Input["structuredTargetRefs"])...)
	targetRefs = append(targetRefs, coverageStructuredBindingRefsFromValue(step.Output["structuredTargetRefs"])...)
	targetRefs = append(targetRefs, coverageStructuredBindingRefsFromValue(step.Input["targetRefs"])...)
	targetRefs = append(targetRefs, coverageStructuredBindingRefsFromValue(step.Output["targetRefs"])...)
	targetRefs = append(targetRefs, coverageStructuredBindingRefsFromValue(step.Input["coverageWorkItemIds"])...)
	targetRefs = append(targetRefs, coverageStructuredBindingRefsFromValue(step.Output["coverageWorkItemIds"])...)
	claims := coverageObservedClaimsFromEvidence(evidence)
	ret := coverageEvidenceAcceptanceOptions{
		EnforceProofStrength: coverageStepRequiresValidationProofStrength(step, evidence, targetRefs, claims),
		Quality:              quality,
		TargetRefs:           compactUniqueStrings(targetRefs),
		ObservedClaims:       compactUniqueStrings(claims),
		ProofKind:            firstNonEmptyString(stringFromMap(evidence, "proofKind"), stringFromMap(evidence, "mode")),
		CoverageAlignment:    stringFromMap(evidence, "coverageAlignment"),
		Tags:                 compactUniqueStrings(tags),
		Reason:               firstNonEmptyString(quality.Reason, stringFromMap(evidence, "reason"), stringFromMap(evidence, "evidenceReason")),
	}
	if coverageEvidenceTagsRequireProofStrength(ret.Tags) {
		ret.EnforceProofStrength = true
	}
	return ret
}

func coverageStepRequiresValidationProofStrength(step *runModel.Step, evidence map[string]any, targetRefs []string, claims []string) bool {
	if len(evidence) > 0 || len(targetRefs) > 0 || len(claims) > 0 {
		return true
	}
	if step == nil {
		return false
	}
	switch strings.TrimSpace(step.Kind) {
	case "shell/command", "mcp/tool_call":
		return true
	default:
		return false
	}
}

func coverageValidationEvidenceMetadataFromStep(step *runModel.Step) map[string]any {
	if step == nil {
		return nil
	}
	if evidence := mapFromValue(step.Output["validationEvidence"]); len(evidence) > 0 {
		return evidence
	}
	return mapFromValue(step.Input["validationEvidence"])
}

func coverageValidationTargetRefsFromEvidence(evidence map[string]any) []string {
	if len(evidence) == 0 {
		return nil
	}
	refs := stringSliceFromValue(evidence["targetRefs"])
	refs = append(refs, stringSliceFromValue(evidence["coverageTargetRefs"])...)
	if ref := strings.TrimSpace(stringFromMap(evidence, "targetRef")); ref != "" {
		refs = append(refs, ref)
	}
	if ref := strings.TrimSpace(stringFromMap(evidence, "coverageTargetRef")); ref != "" {
		refs = append(refs, ref)
	}
	return compactUniqueStrings(refs)
}

func coverageValidationStructuredTargetRefsFromEvidence(evidence map[string]any) []string {
	if len(evidence) == 0 {
		return nil
	}
	refs := stringSliceFromValue(evidence["structuredTargetRefs"])
	refs = append(refs, stringSliceFromValue(evidence["structuredCoverageTargetRefs"])...)
	refs = append(refs, coverageStructuredBindingRefsFromValue(evidence["targetRefs"])...)
	refs = append(refs, coverageStructuredBindingRefsFromValue(evidence["coverageTargetRefs"])...)
	if ref := strings.TrimSpace(stringFromMap(evidence, "structuredTargetRef")); ref != "" {
		refs = append(refs, ref)
	}
	if ref := strings.TrimSpace(stringFromMap(evidence, "targetRef")); coverageBindingRefLooksStructured(ref) {
		refs = append(refs, ref)
	}
	if ref := strings.TrimSpace(stringFromMap(evidence, "coverageTargetRef")); coverageBindingRefLooksStructured(ref) {
		refs = append(refs, ref)
	}
	return compactUniqueStrings(refs)
}

func coverageObservedClaimsFromEvidence(evidence map[string]any) []string {
	if len(evidence) == 0 {
		return nil
	}
	claims := stringSliceFromValue(evidence["observedClaims"])
	claims = append(claims, stringSliceFromValue(evidence["claims"])...)
	for _, item := range mapSliceFromValue(evidence["observedClaims"]) {
		for _, key := range []string{"claim", "summary", "description", "observed", "observation"} {
			if value := strings.TrimSpace(stringFromMap(item, key)); value != "" {
				claims = append(claims, value)
				break
			}
		}
	}
	if claim := strings.TrimSpace(stringFromMap(evidence, "observedClaim")); claim != "" {
		claims = append(claims, claim)
	}
	return compactUniqueStrings(claims)
}

func coverageStepEvidenceRefs(step *runModel.Step) []string {
	if step == nil || step.ID == "" {
		return nil
	}
	refs := []string{string(step.ID)}
	refs = append(refs, evidenceRefsFromValue(step.Output["evidenceRefs"])...)
	if ref := strings.TrimSpace(stringFromMap(step.Output, "evidenceRef")); ref != "" {
		refs = append(refs, ref)
	}
	if coverageStepCanProduceValidationEvidence(step) {
		refs = append(refs, fmt.Sprintf("%s/validation/runtime-step", step.ID))
	}
	return compactUniqueStrings(refs)
}

func coverageWorkItemIDsFromStep(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	ids := make([]string, 0)
	addFromMap := func(payload map[string]any) {
		if len(payload) == 0 {
			return
		}
		ids = append(ids, stringSliceFromValue(payload["workItemIds"])...)
		ids = append(ids, stringSliceFromValue(payload["coverageWorkItemIds"])...)
		if id := strings.TrimSpace(stringFromMap(payload, "workItemId")); id != "" {
			ids = append(ids, id)
		}
		if id := strings.TrimSpace(stringFromMap(payload, "coverageWorkItemId")); id != "" {
			ids = append(ids, id)
		}
		for _, item := range mapSliceFromValue(payload["coverageTargets"]) {
			ids = append(ids, stringSliceFromValue(item["workItemIds"])...)
			if id := strings.TrimSpace(stringFromMap(item, "workItemId")); id != "" {
				ids = append(ids, id)
			}
		}
	}
	addFromMap(step.Input)
	addFromMap(step.Output)
	for _, patch := range mapSliceFromValue(step.Output["patches"]) {
		addFromMap(patch)
	}
	if decision := mapFromValue(step.Output[stepOutputKeyLLMDecision]); len(decision) > 0 {
		addFromMap(decision)
		for _, patch := range mapSliceFromValue(decision["patchRequests"]) {
			addFromMap(patch)
		}
	}
	return compactUniqueStrings(ids)
}

func coverageWorkItemAssessmentUpdates(step *runModel.Step, assessment map[string]any) []coverageWorkItemUpdate {
	if step == nil || len(assessment) == 0 {
		return nil
	}
	updates := make([]coverageWorkItemUpdate, 0)
	addItems := func(items []map[string]any) {
		for _, item := range items {
			tokens := coverageTokensFromAssessmentItem(item)
			if len(tokens) == 0 {
				continue
			}
			refs := evidenceRefsFromValue(item["evidenceRefs"])
			if len(refs) == 0 {
				refs = []string{string(step.ID)}
			}
			state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
			accepted := state == "satisfied"
			update := coverageWorkItemUpdate{
				WorkItemIDs:       coverageAssessmentWorkItemIDs(item),
				TargetTokens:      tokens,
				EvidenceKinds:     coverageAssessmentEvidenceKinds(item),
				EvidenceRefs:      refs,
				Accepted:          accepted,
				CloseSatisfied:    accepted,
				Reason:            firstNonEmptyString(stringFromMap(item, "reason"), stringFromMap(item, "failureReason"), stringFromMap(item, "summary")),
				AcceptanceOptions: coverageAssessmentAcceptanceOptions(item),
			}
			updates = append(updates, update)
		}
	}
	if items, ok := doDAssessmentItemMaps(assessment["items"]); ok {
		addItems(items)
	}
	for _, key := range []string{"goalCoverage", "deliverableCoverage", "acceptanceDimensions"} {
		coverage := mapFromValue(assessment[key])
		if items, ok := doDAssessmentItemMaps(coverage["items"]); ok {
			addItems(items)
		}
	}
	return updates
}

func coverageAssessmentWorkItemIDs(item map[string]any) []string {
	if len(item) == 0 {
		return nil
	}
	ids := []string{
		stringFromMap(item, "workItemId"),
		stringFromMap(item, "coverageWorkItemId"),
	}
	ids = append(ids, stringSliceFromValue(item["workItemIds"])...)
	ids = append(ids, stringSliceFromValue(item["coverageWorkItemIds"])...)
	return compactUniqueStrings(ids)
}

func coverageAssessmentAcceptanceOptions(item map[string]any) coverageEvidenceAcceptanceOptions {
	if len(item) == 0 {
		return coverageEvidenceAcceptanceOptions{}
	}
	targetRefs := coverageAssessmentWorkItemIDs(item)
	for _, key := range []string{
		"structuredTargetRefs",
		"targetRefs",
		"coverageTargetIds",
		"sourceRequirementIds",
	} {
		targetRefs = append(targetRefs, coverageStructuredBindingRefsFromValue(item[key])...)
	}
	for _, key := range []string{
		"coverageTargetToken",
		"goalRequirementId",
		"id",
		"goalItem",
		"deliverable",
		"path",
		"targetRef",
	} {
		if value := strings.TrimSpace(stringFromMap(item, key)); coverageBindingRefLooksStructured(value) {
			targetRefs = append(targetRefs, value)
		}
	}
	claims := stringSliceFromValue(item["observedClaims"])
	for _, key := range []string{"observedClaim", "summary", "reason", "item"} {
		if value := strings.TrimSpace(stringFromMap(item, key)); value != "" {
			claims = append(claims, value)
		}
	}
	return coverageEvidenceAcceptanceOptions{
		Quality:              runtimeEvidenceQuality{Quality: common.EvidenceQualityValid},
		TargetRefs:           compactUniqueStrings(targetRefs),
		ObservedClaims:       compactUniqueStrings(claims),
		ProofKind:            "dod_assessment",
		InheritMatchedTarget: true,
	}
}

func coverageAssessmentEvidenceKinds(item map[string]any) []string {
	if kinds := coverageAssessmentEvidenceKindsExplicit(item); len(kinds) > 0 {
		return kinds
	}
	return []string{"validation"}
}

func coverageAssessmentEvidenceKindsExplicit(item map[string]any) []string {
	if len(item) == 0 {
		return nil
	}
	for _, key := range []string{"evidenceKinds", "requiredEvidenceKinds"} {
		if kinds := compactUniqueStrings(stringSliceFromValue(item[key])); len(kinds) > 0 {
			return kinds
		}
	}
	if kind := strings.TrimSpace(stringFromMap(item, "evidenceKind")); kind != "" {
		return []string{kind}
	}
	return nil
}

func coverageTokensFromAssessmentItem(item map[string]any) []string {
	if len(item) == 0 {
		return nil
	}
	tokens := make([]string, 0)
	for _, key := range []string{
		"workItemId",
		"coverageWorkItemId",
		"coverageTargetToken",
		"goalRequirementId",
		"id",
		"item",
		"goal",
		"goalItem",
		"requirement",
		"deliverable",
		"name",
		"path",
		"targetText",
	} {
		tokens = append(tokens, stringFromMap(item, key))
	}
	tokens = append(tokens, stringSliceFromValue(item["workItemIds"])...)
	tokens = append(tokens, stringSliceFromValue(item["coverageWorkItemIds"])...)
	return compactUniqueStrings(tokens)
}

func coverageWorkItemMatchesUpdate(item map[string]any, update coverageWorkItemUpdate) bool {
	if len(item) == 0 {
		return false
	}
	id := strings.TrimSpace(stringFromMap(item, "id"))
	for _, candidate := range update.WorkItemIDs {
		if strings.TrimSpace(candidate) == id {
			return true
		}
	}
	candidates := []string{
		stringFromMap(item, "normalizedSourceKey"),
		stringFromMap(item, "sourceRequirementId"),
		stringFromMap(item, "targetText"),
		stringFromMap(item, "sourcePath"),
	}
	candidates = append(candidates, stringSliceFromValue(item["sourceRequirementAliases"])...)
	for _, token := range update.TargetTokens {
		for _, candidate := range candidates {
			if coverageRepairTokenMatches(token, candidate) {
				return true
			}
		}
	}
	return false
}

func coverageApplyWorkItemEvidence(item map[string]any, kinds []string, refs []string, accepted bool, closeSatisfied bool, stepID string, reason string) bool {
	return coverageApplyWorkItemEvidenceWithOptions(item, kinds, refs, accepted, closeSatisfied, stepID, reason, coverageEvidenceAcceptanceOptions{})
}

func coverageApplyWorkItemEvidenceWithOptions(item map[string]any, kinds []string, refs []string, accepted bool, closeSatisfied bool, stepID string, reason string, options coverageEvidenceAcceptanceOptions) bool {
	if len(item) == 0 || len(kinds) == 0 || len(refs) == 0 {
		return false
	}
	required := stringSet(stringSliceFromValue(item["requiredEvidenceKinds"]))
	changed := false
	key := "rejectedEvidenceByKind"
	flatKey := "rejectedEvidenceRefs"
	if accepted {
		key = "acceptedEvidenceByKind"
		flatKey = "acceptedEvidenceRefs"
	}
	byKind := mapStringSliceValue(item[key])
	hasApplicableEvidence := false
	for _, kind := range kinds {
		kind = strings.TrimSpace(kind)
		if kind == "" || !required[kind] {
			continue
		}
		if accepted && options.InheritMatchedTarget {
			options.TargetRefs = compactUniqueStrings(append(options.TargetRefs, coverageAcceptedBindingRefsForItem(item)...))
		}
		if accepted && !coverageEvidenceSufficientForKind(item, kind, options) {
			recordCoverageInsufficientEvidence(item, kind, refs, stepID, firstNonEmptyString(options.Reason, coverageEvidenceInsufficientReason(item, kind, options), reason))
			changed = true
			continue
		}
		filteredRefs := coverageEvidenceRefsForKind(kind, refs)
		if len(filteredRefs) == 0 {
			continue
		}
		hasApplicableEvidence = true
		var bindingChanged bool
		before := stableCoverageJSON(byKind[kind])
		byKind[kind] = compactUniqueStrings(append(byKind[kind], filteredRefs...))
		if before != stableCoverageJSON(byKind[kind]) {
			changed = true
		}
		if accepted {
			bindingChanged = recordCoverageAcceptedEvidenceBinding(item, kind, filteredRefs, stepID)
			if bindingChanged {
				changed = true
			}
		}
	}
	if !hasApplicableEvidence {
		return changed
	}
	item[key] = byKind
	item[flatKey] = coverageFlattenEvidenceRefs(byKind, stringSliceFromValue(item[flatKey]))
	if stepID != "" {
		item["updatedByStepId"] = stepID
	}
	item["updatedAt"] = runtimeEvidenceTimestamp()
	if strings.TrimSpace(reason) != "" {
		item["reason"] = reason
	}
	missing := coverageMissingEvidenceKinds(item)
	item["missingEvidenceKinds"] = missing
	item["currentGap"] = coverageCurrentGap(missing)
	if accepted {
		if len(missing) == 0 {
			if closeSatisfied {
				if strings.TrimSpace(stringFromMap(item, "status")) != "satisfied" {
					changed = true
				}
				if strings.TrimSpace(stringFromMap(item, "currentGap")) != "" {
					changed = true
				}
				item["status"] = "satisfied"
				item["missingEvidenceKinds"] = []string{}
				item["currentGap"] = ""
			} else if strings.TrimSpace(stringFromMap(item, "status")) != "satisfied" {
				if strings.TrimSpace(stringFromMap(item, "status")) != "evidence_ready" {
					changed = true
					item["status"] = "evidence_ready"
				}
			}
		} else if strings.TrimSpace(stringFromMap(item, "status")) == "" || strings.TrimSpace(stringFromMap(item, "status")) == "open" {
			changed = true
			item["status"] = "in_progress"
		}
	} else {
		// Rejection branch. A rejection is recorded in the rejection lineage, but the
		// cwi status demotion depends on whether accepted evidence already satisfies the
		// required kinds:
		//   - accepted covers all required kinds  -> the cwi is already "evidence_ready"
		//     or "satisfied"; the rejection is informational only. Do not demote.
		//   - accepted covers some required kinds -> the cwi is in flight; use
		//     "in_progress" rather than "failed" so the completion gate still sees it as
		//     actively progressing toward a satisfied state.
		//   - accepted covers no required kind    -> the cwi is genuinely failing; keep
		//     "failed" as the authoritative LLM/runtime verdict.
		// The previous behavior unconditionally demoted to "failed" on any rejection,
		// which caused cwi items with valid accepted evidence to be misclassified and
		// tripped the dodAssessment satisfaction gate.
		acceptedByKind := mapStringSliceValue(item["acceptedEvidenceByKind"])
		requiredCounts := mapIntValue(item["requiredEvidenceCounts"])
		acceptedCoversAll := true
		acceptedCoversAny := false
		for kind := range required {
			requiredCount := requiredCounts[kind]
			if requiredCount <= 0 {
				requiredCount = 1
			}
			count := coverageEvidenceCountForKind(kind, acceptedByKind[kind])
			if count > 0 {
				acceptedCoversAny = true
			}
			if count < requiredCount {
				acceptedCoversAll = false
			}
		}
		switch {
		case acceptedCoversAll:
			// Already evidence_ready / satisfied; record the rejection as lineage
			// but keep the current status. Only the gap text reflects the rejection.
			if strings.TrimSpace(reason) != "" {
				if strings.TrimSpace(stringFromMap(item, "currentGap")) != reason {
					changed = true
				}
				item["currentGap"] = reason
			}
		case acceptedCoversAny:
			// Partial progress; surface as in_progress so the gate can keep iterating.
			if strings.TrimSpace(stringFromMap(item, "status")) != "in_progress" {
				changed = true
			}
			item["status"] = "in_progress"
			if strings.TrimSpace(reason) != "" {
				if strings.TrimSpace(stringFromMap(item, "currentGap")) != reason {
					changed = true
				}
				item["currentGap"] = reason
			}
		default:
			if strings.TrimSpace(stringFromMap(item, "status")) != "failed" {
				changed = true
			}
			item["status"] = "failed"
			if strings.TrimSpace(reason) != "" {
				if strings.TrimSpace(stringFromMap(item, "currentGap")) != reason {
					changed = true
				}
				item["currentGap"] = reason
			}
		}
	}
	if !changed {
		return false
	}
	return true
}

func recordCoverageAcceptedEvidenceBinding(item map[string]any, kind string, refs []string, stepID string) bool {
	if len(item) == 0 || strings.TrimSpace(kind) == "" || len(refs) == 0 {
		return false
	}
	byKind := mapStringSliceValue(item["acceptedEvidenceBindingByKind"])
	before := stableCoverageJSON(byKind[kind])
	byKind[kind] = compactUniqueStrings(append(byKind[kind], refs...))
	changed := before != stableCoverageJSON(byKind[kind])
	item["acceptedEvidenceBindingByKind"] = byKind
	item["acceptedEvidenceBindingRefs"] = coverageFlattenEvidenceRefs(byKind, stringSliceFromValue(item["acceptedEvidenceBindingRefs"]))
	if stepID != "" {
		item["acceptedEvidenceBindingStepId"] = stepID
	}
	if recordCoverageEvidenceBindingDecisions(item, kind, refs, stepID, "accepted_binding", "accepted evidence satisfies required coverage evidence kind") {
		changed = true
	}
	return changed
}

func coverageEvidenceSufficientForKind(item map[string]any, kind string, options coverageEvidenceAcceptanceOptions) bool {
	switch strings.TrimSpace(kind) {
	case "state_change":
		return stateChangeEvidenceCanSatisfyCoverage(item, options)
	case "validation":
	default:
		return true
	}
	if !validationEvidenceCanSatisfyDoD(options.Quality) {
		return false
	}
	if coverageEvidenceTagsAreDiagnosticOnly(options.Tags) {
		return false
	}
	if !coverageEvidenceTargetAligned(item, options.TargetRefs) {
		return false
	}
	if len(options.ObservedClaims) == 0 {
		return false
	}
	return true
}

func stateChangeEvidenceCanSatisfyCoverage(item map[string]any, options coverageEvidenceAcceptanceOptions) bool {
	if len(item) == 0 {
		return false
	}
	if !validationEvidenceCanSatisfyDoD(options.Quality) {
		return false
	}
	if options.Quality.ZeroWork {
		return false
	}
	if coverageEvidenceTagsAreDiagnosticOnly(options.Tags) {
		return false
	}
	return coverageEvidenceTargetAligned(item, options.TargetRefs)
}

func coverageEvidenceTargetAligned(item map[string]any, targetRefs []string) bool {
	if len(item) == 0 || len(targetRefs) == 0 {
		return false
	}
	return coverageStrictTargetRefsIntersect(coverageStructuredBindingRefsFromValue(targetRefs), coverageAcceptedBindingRefsForItem(item))
}

func coverageAcceptedBindingRefsForItem(item map[string]any) []string {
	if len(item) == 0 {
		return nil
	}
	refs := []string{
		stringFromMap(item, "id"),
		stringFromMap(item, "repositoryId"),
		stringFromMap(item, "coverageWorkItemId"),
		stringFromMap(item, "coverageTargetId"),
		stringFromMap(item, "normalizedSourceKey"),
		stringFromMap(item, "sourceRequirementId"),
		stringFromMap(item, "targetRef"),
	}
	refs = append(refs, stringSliceFromValue(item["coverageWorkItemIds"])...)
	refs = append(refs, stringSliceFromValue(item["coverageTargetIds"])...)
	refs = append(refs, coverageStructuredBindingRefsFromValue(item["structuredTargetRefs"])...)
	refs = append(refs, coverageStructuredBindingRefsFromValue(item["targetRefs"])...)
	refs = append(refs, coverageStructuredBindingRefsFromValue(item["sourceRequirementAliases"])...)
	return compactUniqueStrings(refs)
}

func coverageStructuredBindingRefsFromValue(value any) []string {
	refs := make([]string, 0)
	for _, ref := range stringSliceFromValue(value) {
		if coverageBindingRefLooksStructured(ref) {
			refs = append(refs, ref)
		}
	}
	return compactUniqueStrings(refs)
}

func coverageBindingRefLooksStructured(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 160 {
		return false
	}
	return len(strings.Fields(value)) <= 1
}

func coverageEvidenceTagsRequireProofStrength(tags []string) bool {
	return coverageEvidenceTagsContain(tags,
		"generic_test_fallback",
		"all_tests_fallback",
		"target_unaligned",
		"controlled_substitute_unproven",
		"discovery_only",
		"missing_observed_claims",
		"missing_target_refs",
		"diagnostic_only",
	)
}

func coverageEvidenceTagsAreDiagnosticOnly(tags []string) bool {
	return coverageEvidenceTagsContain(tags,
		"target_unaligned",
		"controlled_substitute_unproven",
		"discovery_only",
		"missing_observed_claims",
		"missing_target_refs",
		"diagnostic_only",
	)
}

func coverageEvidenceTagsContain(tags []string, markers ...string) bool {
	if len(tags) == 0 || len(markers) == 0 {
		return false
	}
	markerSet := stringSet(markers)
	for _, tag := range tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		normalized = strings.ReplaceAll(normalized, "-", "_")
		normalized = strings.ReplaceAll(normalized, " ", "_")
		if markerSet[normalized] {
			return true
		}
	}
	return false
}

func coverageEvidenceInsufficientReason(item map[string]any, kind string, options coverageEvidenceAcceptanceOptions) string {
	switch strings.TrimSpace(kind) {
	case "state_change":
		switch {
		case !validationEvidenceCanSatisfyDoD(options.Quality):
			return firstNonEmptyString(options.Quality.Reason, "state-changing evidence quality is not sufficient for coverage")
		case options.Quality.ZeroWork:
			return firstNonEmptyString(options.Quality.Reason, "state-changing evidence reported zero work")
		case coverageEvidenceTagsAreDiagnosticOnly(options.Tags):
			return "state-changing evidence is diagnostic or target-unaligned"
		case !coverageEvidenceTargetAligned(item, options.TargetRefs):
			return "state-changing evidence is missing target-aligned refs"
		default:
			return "state-changing evidence is not sufficient for coverage"
		}
	case "validation":
	default:
		return "evidence is not sufficient for coverage"
	}
	switch {
	case !validationEvidenceCanSatisfyDoD(options.Quality):
		return firstNonEmptyString(options.Quality.Reason, "validation evidence quality is not sufficient for coverage")
	case coverageEvidenceTagsAreDiagnosticOnly(options.Tags):
		return "validation evidence is diagnostic or target-unaligned"
	case !coverageEvidenceTargetAligned(item, options.TargetRefs):
		return "validation evidence is missing target-aligned refs"
	case len(options.ObservedClaims) == 0:
		return "validation evidence is missing observed claims"
	default:
		return "validation evidence is not sufficient for coverage"
	}
}

func recordCoverageInsufficientEvidence(item map[string]any, kind string, refs []string, stepID string, reason string) {
	if len(item) == 0 || strings.TrimSpace(kind) == "" || len(refs) == 0 {
		return
	}
	filteredRefs := coverageEvidenceRefsForKind(kind, refs)
	byKind := mapStringSliceValue(item["supportingEvidenceByKind"])
	byKind[kind] = compactUniqueStrings(append(byKind[kind], filteredRefs...))
	item["supportingEvidenceByKind"] = byKind
	item["supportingEvidenceRefs"] = coverageFlattenEvidenceRefs(byKind, stringSliceFromValue(item["supportingEvidenceRefs"]))
	if stepID != "" {
		item["updatedByStepId"] = stepID
	}
	item["updatedAt"] = runtimeEvidenceTimestamp()
	if strings.TrimSpace(reason) != "" {
		item["evidenceQualityReason"] = reason
		item["currentGap"] = reason
	}
	recordCoverageEvidenceBindingDecisions(item, kind, filteredRefs, stepID, "supporting_only", firstNonEmptyString(reason, "evidence did not satisfy required coverage proof strength"))
}

func recordCoverageEvidenceBindingDecisions(item map[string]any, kind string, refs []string, stepID string, status string, reason string) bool {
	if len(item) == 0 || strings.TrimSpace(kind) == "" || len(refs) == 0 || strings.TrimSpace(status) == "" {
		return false
	}
	coverageItemID := strings.TrimSpace(stringFromMap(item, "id"))
	existing := mapSliceFromValue(item["evidenceBindingDecisions"])
	seen := make(map[string]bool, len(existing))
	for _, decision := range existing {
		key := evidenceBindingDecisionKey(
			stringFromMap(decision, "evidenceRef"),
			stringFromMap(decision, "coverageItemId"),
			stringFromMap(decision, "evidenceKind"),
			stringFromMap(decision, "status"),
		)
		if key != "" {
			seen[key] = true
		}
	}
	changed := false
	for _, ref := range compactUniqueStrings(refs) {
		key := evidenceBindingDecisionKey(ref, coverageItemID, kind, status)
		if key == "" || seen[key] {
			continue
		}
		decision := map[string]any{
			"evidenceRef":    strings.TrimSpace(ref),
			"coverageItemId": coverageItemID,
			"evidenceKind":   strings.TrimSpace(kind),
			"status":         strings.TrimSpace(status),
			"reason":         strings.TrimSpace(reason),
			"recordedAt":     runtimeEvidenceTimestamp(),
		}
		if stepID != "" {
			decision["stepId"] = stepID
		}
		existing = append(existing, decision)
		seen[key] = true
		changed = true
	}
	if changed {
		item["evidenceBindingDecisions"] = existing
	}
	return changed
}

func evidenceBindingDecisionKey(ref, coverageItemID, kind, status string) string {
	ref = strings.TrimSpace(ref)
	coverageItemID = strings.TrimSpace(coverageItemID)
	kind = strings.TrimSpace(kind)
	status = strings.TrimSpace(status)
	if ref == "" || coverageItemID == "" || kind == "" || status == "" {
		return ""
	}
	return ref + "|" + coverageItemID + "|" + kind + "|" + status
}

func coverageEvidenceRefsForKind(kind string, refs []string) []string {
	kind = strings.TrimSpace(kind)
	if kind == "" || len(refs) == 0 {
		return nil
	}
	switch kind {
	case "decision":
		return filterCoverageEvidenceRefs(refs, coverageEvidenceRefLooksDecision)
	case "approval":
		return filterCoverageEvidenceRefs(refs, coverageEvidenceRefLooksApproval)
	default:
		return compactUniqueStrings(refs)
	}
}

func filterCoverageEvidenceRefs(refs []string, keep func(string) bool) []string {
	if keep == nil {
		return compactUniqueStrings(refs)
	}
	ret := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || !keep(ref) {
			continue
		}
		ret = append(ret, ref)
	}
	return compactUniqueStrings(ret)
}

func coverageEvidenceRefLooksDecision(ref string) bool {
	ref = strings.ToLower(strings.TrimSpace(ref))
	return strings.Contains(ref, "decision") ||
		strings.Contains(ref, "ticket") ||
		strings.Contains(ref, "resolution")
}

func coverageEvidenceRefLooksApproval(ref string) bool {
	ref = strings.ToLower(strings.TrimSpace(ref))
	return strings.Contains(ref, "approval")
}
