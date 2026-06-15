package orchestrator

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
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
		if profileRequirementRequiresStateChangingEvidence(requirement) {
			kinds = append(kinds, "state_change")
		}
		if requirement.RequireDirect || requirement.RequireClaimEvidence {
			kinds = append(kinds, "validation")
		}
	}
	if runExecutionContractRequiresStateChange(run) && coverageSourceCanCarryStateChange(source) {
		kinds = append(kinds, "state_change")
	}
	return compactUniqueStrings(kinds)
}

func coverageDefaultRequiredEvidenceKinds(source coverageWorkItemSource) []string {
	target := strings.ToLower(strings.TrimSpace(source.TargetText))
	switch {
	case coverageTextRequiresDecisionEvidence(target):
		return []string{"decision"}
	case coverageTextRequiresApprovalEvidence(target):
		return []string{"approval"}
	default:
		return []string{"validation"}
	}
}

func coverageRequiredEvidenceCountsForSource(source coverageWorkItemSource, requiredKinds []string) map[string]int {
	ret := map[string]int{}
	target := strings.ToLower(strings.TrimSpace(source.TargetText))
	for _, kind := range requiredKinds {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		switch kind {
		case "decision":
			ret[kind] = coverageGovernanceEvidenceMinCount(target, "decision")
		case "approval":
			ret[kind] = coverageGovernanceEvidenceMinCount(target, "approval")
		default:
			ret[kind] = 1
		}
	}
	return ret
}

func coverageGovernanceEvidenceMinCount(text string, kind string) int {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return 1
	}
	kindTerms := []string{kind}
	switch kind {
	case "decision":
		kindTerms = []string{"decision", "decisions", "ticket", "tickets", "决策", "工单"}
	case "approval":
		kindTerms = []string{"approval", "approvals", "approved", "审批", "批准"}
	}
	for _, term := range kindTerms {
		if count := coverageCountBeforeTerm(text, term); count > 0 {
			return count
		}
	}
	return 1
}

func coverageCountBeforeTerm(text string, term string) int {
	term = strings.ToLower(strings.TrimSpace(term))
	if text == "" || term == "" || !strings.Contains(text, term) {
		return 0
	}
	pattern := regexp.MustCompile(`(?i)(\d+|one|two|three|four|five|six|seven|eight|nine|ten|一|一个|一条|一项|两|两个|两条|两项|二|二个|二条|二项|三|三个|三条|三项)\s*(?:[a-z_-]+\s+){0,3}` + regexp.QuoteMeta(term))
	match := pattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return 0
	}
	return coverageCountWordValue(match[1])
}

func coverageCountWordValue(raw string) int {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return 0
	}
	if count, err := strconv.Atoi(raw); err == nil {
		return count
	}
	switch raw {
	case "one", "一", "一个", "一条", "一项":
		return 1
	case "two", "两", "两个", "两条", "两项", "二", "二个", "二条", "二项":
		return 2
	case "three", "三", "三个", "三条", "三项":
		return 3
	case "four":
		return 4
	case "five":
		return 5
	case "six":
		return 6
	case "seven":
		return 7
	case "eight":
		return 8
	case "nine":
		return 9
	case "ten":
		return 10
	default:
		return 0
	}
}

func coverageTextRequiresDecisionEvidence(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	return strings.Contains(text, "decision") ||
		strings.Contains(text, "决策") ||
		strings.Contains(text, "工单")
}

func coverageTextRequiresApprovalEvidence(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	return strings.Contains(text, "approval") ||
		strings.Contains(text, "approved") ||
		strings.Contains(text, "审批") ||
		strings.Contains(text, "批准")
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
	required := stringSliceFromValue(item["requiredEvidenceKinds"])
	if len(required) == 0 {
		required = []string{"validation"}
	}
	return coverageMissingEvidenceKindsFromDetails(required, mapStringSliceValue(item["acceptedEvidenceByKind"]), mapIntValue(item["requiredEvidenceCounts"]))
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
		WorkItemIDs:  coverageWorkItemIDsFromStep(step),
		TargetTokens: coverageTargetTokensFromStep(step),
		EvidenceRefs: coverageStepEvidenceRefs(step),
		Accepted:     step.State == common.StepStateCompleted,
	}
	if stepKindCanProduceStateChangingEvidence(step.Kind) {
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
				TargetTokens:   tokens,
				EvidenceKinds:  coverageAssessmentEvidenceKinds(item),
				EvidenceRefs:   refs,
				Accepted:       accepted,
				CloseSatisfied: accepted,
				Reason:         firstNonEmptyString(stringFromMap(item, "reason"), stringFromMap(item, "failureReason"), stringFromMap(item, "summary")),
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

func coverageAssessmentEvidenceKinds(item map[string]any) []string {
	if len(item) == 0 {
		return []string{"validation"}
	}
	text := strings.Join([]string{
		stringFromMap(item, "item"),
		stringFromMap(item, "goal"),
		stringFromMap(item, "goalItem"),
		stringFromMap(item, "requirement"),
		stringFromMap(item, "targetText"),
		stringFromMap(item, "summary"),
	}, " ")
	switch {
	case coverageTextRequiresDecisionEvidence(text):
		return []string{"decision"}
	case coverageTextRequiresApprovalEvidence(text):
		return []string{"approval"}
	default:
		return []string{"validation"}
	}
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
		filteredRefs := coverageEvidenceRefsForKind(kind, refs)
		if len(filteredRefs) == 0 {
			continue
		}
		hasApplicableEvidence = true
		before := stableCoverageJSON(byKind[kind])
		byKind[kind] = compactUniqueStrings(append(byKind[kind], filteredRefs...))
		if before != stableCoverageJSON(byKind[kind]) {
			changed = true
		}
	}
	if !hasApplicableEvidence {
		return false
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
