package orchestrator

import (
	"context"
	"sort"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type completionEvidenceRefCatalog struct {
	runID                     common.RunID
	aliases                   map[string]string
	steps                     map[string]*runModel.Step
	patches                   map[string]bool
	stepOrder                 map[string]int
	stepScope                 map[string]string
	latestUsableStepByScope   map[string]string
	latestUsableOrderByScope  map[string]int
	supersededFailureStepRefs map[string]string
}

func (o *RuntimeOrchestrator) normalizeCompletionGateDoDAssessmentEvidenceRefs(ctx context.Context, run *runModel.Run, output map[string]any) (map[string]any, error) {
	if o == nil || run == nil || output == nil || !requiresDoDAssessmentCompletionGate(run) {
		return output, nil
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return output, nil
	}
	catalog, err := completionEvidenceRefCatalogForRun(ctx, o.steps, o.evidence, run)
	if err != nil {
		return nil, err
	}
	normalized, changed := normalizeDoDAssessmentEvidenceRefs(ctx, o.steps, o.evidence, catalog, assessment)
	if !changed {
		return output, nil
	}
	ret := cloneStepPayload(output)
	if _, ok := ret[stepOutputKeyDoDAssessment]; ok {
		ret[stepOutputKeyDoDAssessment] = normalized
		return ret, nil
	}
	decision := mapFromValue(ret[stepOutputKeyLLMDecision])
	if len(decision) == 0 {
		return output, nil
	}
	decision[stepOutputKeyDoDAssessment] = normalized
	ret[stepOutputKeyLLMDecision] = decision
	return ret, nil
}

func completionEvidenceRefCatalogForRun(ctx context.Context, steps repository.StepRepository, evidence RuntimeEvidenceRepositories, run *runModel.Run) (completionEvidenceRefCatalog, error) {
	runID := common.RunID("")
	if run != nil {
		runID = run.ID
	}
	catalog := completionEvidenceRefCatalog{
		runID:                     runID,
		aliases:                   map[string]string{},
		steps:                     map[string]*runModel.Step{},
		patches:                   map[string]bool{},
		stepOrder:                 map[string]int{},
		stepScope:                 map[string]string{},
		latestUsableStepByScope:   map[string]string{},
		latestUsableOrderByScope:  map[string]int{},
		supersededFailureStepRefs: map[string]string{},
	}
	if steps == nil || run == nil || run.ID == "" {
		return catalog, nil
	}
	runSteps, err := steps.ListByRun(ctx, run.ID)
	if err != nil {
		return catalog, err
	}
	runSteps = orderedRuntimeContextSteps(runSteps)
	for idx, step := range runSteps {
		if step == nil || step.ID == "" {
			continue
		}
		canonical := string(step.ID)
		catalog.steps[canonical] = step
		catalog.stepOrder[canonical] = idx
		if scope := completionEvidenceStepScope(step); scope != "" {
			catalog.stepScope[canonical] = scope
			if completionEvidenceStepUsable(step) {
				catalog.latestUsableStepByScope[scope] = canonical
				catalog.latestUsableOrderByScope[scope] = idx
			}
		}
		catalog.addAlias(canonical, canonical)
		catalog.addAlias(canonical+"/tool-call/runtime-execute", canonical)
		for _, rawAlias := range completionEvidenceStepAliases(run.ID, step) {
			catalog.addAlias(rawAlias, canonical)
		}
	}
	for _, step := range runSteps {
		if step == nil || step.ID == "" || step.State != common.StepStateFailed {
			continue
		}
		canonical := string(step.ID)
		scope := catalog.stepScope[canonical]
		if scope == "" {
			continue
		}
		latest := catalog.latestUsableStepByScope[scope]
		if latest == "" {
			continue
		}
		if catalog.latestUsableOrderByScope[scope] > catalog.stepOrder[canonical] {
			catalog.supersededFailureStepRefs[canonical] = latest
		}
	}
	for _, step := range runSteps {
		if step == nil || step.ID == "" {
			continue
		}
		canonical := string(step.ID)
		for _, rawAlias := range completionEvidenceFollowUpNodeAliases(run.ID, step, runSteps) {
			catalog.addAlias(rawAlias, canonical)
		}
	}
	if evidence.Patches != nil {
		patches, err := evidence.Patches.ListByRun(ctx, run.ID)
		if err != nil {
			return catalog, err
		}
		for _, patch := range patches {
			if patch == nil || patch.ID == "" || patch.State != common.PatchStateApplied {
				continue
			}
			canonical := string(patch.ID)
			catalog.patches[canonical] = true
			catalog.addAlias(canonical, canonical)
			for _, rawAlias := range completionEvidencePatchAliases(run.ID, patch.StepID, canonical, catalog.steps[string(patch.StepID)]) {
				catalog.addAlias(rawAlias, canonical)
			}
		}
	}
	return catalog, nil
}

func completionEvidenceStepAliases(runID common.RunID, step *runModel.Step) []string {
	if runID == "" || step == nil {
		return nil
	}
	aliases := make([]string, 0, 8)
	add := func(value string) {
		value = strings.Trim(strings.TrimSpace(value), "/")
		if value == "" {
			return
		}
		aliases = append(aliases, value)
		aliases = append(aliases, string(runID)+"/"+value)
	}
	for _, key := range []string{"graphNodeId", "nodeId", "id"} {
		for _, variant := range completionEvidenceAliasLabelVariants(stringFromMap(step.Input, key)) {
			add(variant)
		}
	}
	last := completionEvidenceLastPathPart(string(step.ID))
	for _, variant := range completionEvidenceAliasLabelVariants(last) {
		add(variant)
	}
	return aliases
}

func completionEvidenceFollowUpNodeAliases(runID common.RunID, step *runModel.Step, steps []*runModel.Step) []string {
	if runID == "" || step == nil || step.ID == "" {
		return nil
	}
	nodeLabels := completionEvidenceStepNodeLabels(step)
	if len(nodeLabels) == 0 {
		return nil
	}
	ret := make([]string, 0, 4)
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.Trim(strings.TrimSpace(value), "/")
		if value == "" || seen[value] {
			return
		}
		ret = append(ret, value)
		seen[value] = true
	}
	for _, owner := range steps {
		if owner == nil || owner.ID == "" || owner.Kind != llmWorkspaceExecutionStepKind {
			continue
		}
		ownerLast := completionEvidenceLastPathPart(string(owner.ID))
		if ownerLast == "" {
			continue
		}
		for _, ownerVariant := range completionEvidenceAliasLabelVariants(ownerLast) {
			for _, nodeLabel := range nodeLabels {
				if !completionEvidenceOwnerLooksScopedToNode(ownerVariant, nodeLabel) {
					continue
				}
				add(ownerVariant)
				add(string(runID) + "/" + ownerVariant)
			}
		}
	}
	return ret
}

func completionEvidenceStepNodeLabels(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	labels := make([]string, 0, 8)
	seen := map[string]bool{}
	add := func(value string) {
		for _, variant := range completionEvidenceAliasLabelVariants(value) {
			if variant == "" || seen[variant] {
				continue
			}
			labels = append(labels, variant)
			seen[variant] = true
		}
	}
	for _, key := range []string{"graphNodeId", "nodeId", "id"} {
		add(stringFromMap(step.Input, key))
	}
	add(completionEvidenceLastPathPart(string(step.ID)))
	return labels
}

func completionEvidenceOwnerLooksScopedToNode(owner string, node string) bool {
	owner = strings.Trim(strings.TrimSpace(owner), "/")
	node = strings.Trim(strings.TrimSpace(node), "/")
	if owner == "" || node == "" || owner == node {
		return false
	}
	return strings.HasSuffix(owner, "-"+node) || strings.HasSuffix(owner, "_"+node)
}

func completionEvidenceAliasLabelVariants(value string) []string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return nil
	}
	if strings.Contains(value, "/") {
		value = completionEvidenceLastPathPart(value)
	}
	candidates := []string{
		value,
		strings.ReplaceAll(value, "-", "_"),
		strings.ReplaceAll(value, "_", "-"),
	}
	stripped := completionEvidenceStripSequencePrefix(value)
	if stripped != "" && stripped != value {
		candidates = append(candidates,
			stripped,
			strings.ReplaceAll(stripped, "-", "_"),
			strings.ReplaceAll(stripped, "_", "-"),
		)
	}
	ret := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = strings.Trim(strings.TrimSpace(candidate), "/")
		if candidate == "" || seen[candidate] {
			continue
		}
		ret = append(ret, candidate)
		seen[candidate] = true
	}
	return ret
}

func completionEvidenceStripSequencePrefix(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return ""
	}
	idx := strings.Index(value, "-")
	if idx <= 0 {
		return value
	}
	for _, ch := range value[:idx] {
		if ch < '0' || ch > '9' {
			return value
		}
	}
	return strings.TrimSpace(value[idx+1:])
}

func completionEvidenceLastPathPart(ref string) string {
	ref = strings.Trim(strings.TrimSpace(ref), "/")
	if ref == "" {
		return ""
	}
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}

func completionEvidenceStepScope(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	if target := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(step.Input, "targetRef"),
		stringFromMap(step.Output, "targetRef"),
	)); target != "" {
		return "targetRef=" + normalizeRuntimeFailureTargetValue(target)
	}
	if scope := runtimeFailureTargetScopeFromStep(step); scope != "" {
		return scope
	}
	if nodeID := graphNodeID(step); nodeID != "" {
		return "graphNodeId=" + completionEvidenceStripSequencePrefix(strings.TrimSpace(nodeID))
	}
	last := completionEvidenceLastPathPart(string(step.ID))
	if last == "" {
		return ""
	}
	return "stepLabel=" + completionEvidenceStripSequencePrefix(last)
}

func completionEvidencePatchAliases(runID common.RunID, stepID common.StepID, patchID string, step *runModel.Step) []string {
	if runID == "" || patchID == "" {
		return nil
	}
	patchSuffix := ""
	if idx := strings.LastIndex(patchID, "/patch/"); idx >= 0 {
		patchSuffix = patchID[idx:]
	}
	if patchSuffix == "" {
		return nil
	}
	nodeAliases := completionEvidenceStepAliases(runID, step)
	if stepID != "" {
		nodeAliases = append(nodeAliases, completionEvidenceLastPathPart(string(stepID)))
	}
	ret := make([]string, 0, len(nodeAliases)*2)
	seen := map[string]bool{}
	for _, alias := range nodeAliases {
		alias = strings.Trim(strings.TrimSpace(alias), "/")
		if alias == "" {
			continue
		}
		candidate := strings.Trim(strings.TrimSpace(alias+patchSuffix), "/")
		if !seen[candidate] {
			ret = append(ret, candidate)
			seen[candidate] = true
		}
		if !strings.HasPrefix(alias, string(runID)+"/") {
			blockCandidate := string(runID) + "/block/" + alias + patchSuffix
			if !seen[blockCandidate] {
				ret = append(ret, blockCandidate)
				seen[blockCandidate] = true
			}
		}
	}
	return ret
}

func (c completionEvidenceRefCatalog) addAlias(alias string, canonical string) {
	alias = strings.Trim(strings.TrimSpace(alias), "/")
	canonical = strings.TrimSpace(canonical)
	if alias == "" || canonical == "" {
		return
	}
	if _, exists := c.aliases[alias]; exists {
		return
	}
	c.aliases[alias] = canonical
}

func normalizeDoDAssessmentEvidenceRefs(ctx context.Context, steps repository.StepRepository, evidence RuntimeEvidenceRepositories, catalog completionEvidenceRefCatalog, assessment map[string]any) (map[string]any, bool) {
	if assessment == nil {
		return assessment, false
	}
	ret := cloneStepPayload(assessment)
	changed := false
	fallbackRefs := completionEvidenceFallbackRefs(catalog)
	if refs, ok := normalizeEvidenceRefList(ctx, steps, evidence, catalog, ret["evidenceRefs"]); ok {
		ret["evidenceRefs"] = refs
		changed = true
	}
	if items, ok := normalizeAssessmentItemsEvidenceRefs(ctx, steps, evidence, catalog, ret["items"], fallbackRefs, false); ok {
		ret["items"] = items
		changed = true
	}
	for _, key := range []string{"goalCoverage", "goalOutcomeCoverage", "acceptanceDimensions", "dimensionCoverage", "deliverableCoverage", "deliverables", "producedArtifacts"} {
		value := ret[key]
		normalized, ok := normalizeCoverageEvidenceRefs(ctx, steps, evidence, catalog, value, fallbackRefs)
		if !ok {
			continue
		}
		ret[key] = normalized
		changed = true
	}
	if !changed {
		return assessment, false
	}
	return ret, true
}

func normalizeCoverageEvidenceRefs(ctx context.Context, steps repository.StepRepository, evidence RuntimeEvidenceRepositories, catalog completionEvidenceRefCatalog, value any, fallbackRefs []string) (any, bool) {
	if mapped := mapFromValue(value); len(mapped) > 0 {
		items, changed := normalizeAssessmentItemsEvidenceRefs(ctx, steps, evidence, catalog, mapped["items"], fallbackRefs, boolValue(mapped["satisfied"]))
		if !changed {
			return value, false
		}
		ret := cloneStepPayload(mapped)
		ret["items"] = items
		return ret, true
	}
	items, changed := normalizeAssessmentItemsEvidenceRefs(ctx, steps, evidence, catalog, value, fallbackRefs, false)
	if !changed {
		return value, false
	}
	return items, true
}

func normalizeAssessmentItemsEvidenceRefs(ctx context.Context, steps repository.StepRepository, evidence RuntimeEvidenceRepositories, catalog completionEvidenceRefCatalog, value any, fallbackRefs []string, inferSatisfiedFromParent bool) ([]map[string]any, bool) {
	items, ok := doDAssessmentItemMaps(value)
	if !ok || len(items) == 0 {
		return nil, false
	}
	ret := make([]map[string]any, 0, len(items))
	changed := false
	for _, item := range items {
		normalized := cloneStepPayload(item)
		if refs, ok := normalizeEvidenceRefList(ctx, steps, evidence, catalog, normalized["evidenceRefs"]); ok {
			normalized["evidenceRefs"] = refs
			changed = true
		}
		state := strings.ToLower(strings.TrimSpace(stringFromMap(normalized, "state")))
		itemFallbackRefs := fallbackRefs
		if refs := completionEvidenceCandidateRefs(ctx, steps, evidence, catalog, normalized); len(refs) > 0 {
			itemFallbackRefs = refs
		}
		if state == "" && inferSatisfiedFromParent && len(itemFallbackRefs) > 0 {
			normalized["state"] = "satisfied"
			state = "satisfied"
			changed = true
		}
		if completionAssessmentStateAllowsEvidenceFallback(state) &&
			len(evidenceRefsFromValue(normalized["evidenceRefs"])) == 0 &&
			len(stringSliceFromValue(normalized["artifactRefs"])) == 0 &&
			len(itemFallbackRefs) > 0 {
			normalized["evidenceRefs"] = append([]string(nil), itemFallbackRefs...)
			changed = true
		}
		ret = append(ret, normalized)
	}
	if !changed {
		return items, false
	}
	return ret, true
}

func completionAssessmentStateAllowsEvidenceFallback(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "satisfied", "available", "produced":
		return true
	default:
		return false
	}
}

func completionEvidenceCandidateRefs(ctx context.Context, steps repository.StepRepository, evidence RuntimeEvidenceRepositories, catalog completionEvidenceRefCatalog, item map[string]any) []string {
	if steps == nil {
		return nil
	}
	candidates := mapSliceFromValue(item["evidenceCandidates"])
	if len(candidates) == 0 {
		return nil
	}
	refs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if !completionEvidenceCandidateCanFallback(candidate) {
			continue
		}
		ref := strings.TrimSpace(firstNonEmptyString(
			stringFromMap(candidate, "evidenceRef"),
			stringFromMap(candidate, "stepId"),
		))
		if ref == "" {
			continue
		}
		canonical, ok := canonicalCompletionEvidenceRef(ctx, steps, evidence, catalog, ref)
		if !ok {
			continue
		}
		refs = append(refs, canonical)
	}
	return dedupeEvidenceRefs(refs)
}

func completionEvidenceCandidateCanFallback(candidate map[string]any) bool {
	if len(candidate) == 0 {
		return false
	}
	kind := strings.TrimSpace(stringFromMap(candidate, "kind"))
	if completionEvidenceKindIsWeakFallback(kind) {
		return false
	}
	if boolValue(candidate["zeroWork"]) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(stringFromMap(candidate, "evidenceQuality"))) {
	case "inconclusive", "invalid", "smoke", "unrelated":
		return false
	default:
		return true
	}
}

func completionEvidenceFallbackRefs(catalog completionEvidenceRefCatalog) []string {
	type orderedRef struct {
		order int
		ref   string
	}
	refs := make([]orderedRef, 0, len(catalog.steps)+len(catalog.patches))
	for ref := range catalog.patches {
		refs = append(refs, orderedRef{order: len(catalog.stepOrder) + len(refs) + 1, ref: ref})
	}
	for ref, step := range catalog.steps {
		if !completionEvidenceStepFallbackUsable(step) {
			continue
		}
		refs = append(refs, orderedRef{order: catalog.stepOrder[ref], ref: ref})
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].order != refs[j].order {
			return refs[i].order > refs[j].order
		}
		return refs[i].ref < refs[j].ref
	})
	limit := min(len(refs), 3)
	ret := make([]string, 0, limit)
	for idx := 0; idx < limit; idx++ {
		ret = append(ret, refs[idx].ref)
	}
	return dedupeEvidenceRefs(ret)
}

func completionEvidenceStepFallbackUsable(step *runModel.Step) bool {
	if !completionEvidenceStepUsable(step) {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(step.Kind), "llm/") {
		return false
	}
	if completionEvidenceKindIsWeakFallback(step.Kind) {
		return false
	}
	if boolValue(step.Output["zeroWork"]) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(stringFromMap(step.Output, "evidenceQuality"))) {
	case "inconclusive", "invalid", "smoke", "unrelated":
		return false
	default:
		return true
	}
}

func completionEvidenceKindIsWeakFallback(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "", "file/read", "file/list", "mcp/resource_read", "mcp/prompt_get":
		return true
	default:
		return false
	}
}

func dedupeEvidenceRefs(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		ret = append(ret, ref)
	}
	return ret
}

func normalizeEvidenceRefList(ctx context.Context, steps repository.StepRepository, evidence RuntimeEvidenceRepositories, catalog completionEvidenceRefCatalog, value any) ([]string, bool) {
	refs := evidenceRefsFromValue(value)
	if len(refs) == 0 {
		return nil, false
	}
	ret := make([]string, 0, len(refs))
	seen := map[string]bool{}
	changed := false
	for _, raw := range refs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			changed = true
			continue
		}
		canonical, ok := canonicalCompletionEvidenceRef(ctx, steps, evidence, catalog, ref)
		if !ok {
			changed = true
			continue
		}
		if canonical != ref {
			changed = true
		}
		if seen[canonical] {
			changed = true
			continue
		}
		seen[canonical] = true
		ret = append(ret, canonical)
	}
	if !changed {
		return refs, false
	}
	return ret, true
}

func canonicalCompletionEvidenceRef(ctx context.Context, steps repository.StepRepository, evidence RuntimeEvidenceRepositories, catalog completionEvidenceRefCatalog, ref string) (string, bool) {
	ref = strings.Trim(strings.TrimSpace(ref), "/")
	if ref == "" || !traceableEvidenceRef(ref) {
		return "", false
	}
	if canonical := catalog.aliases[ref]; canonical != "" {
		if catalog.patches[canonical] {
			return canonical, true
		}
		if completionEvidenceStepUsable(catalog.steps[canonical]) {
			return canonical, true
		}
		if replacement := catalog.supersededFailureStepRefs[canonical]; replacement != "" {
			return replacement, true
		}
		return canonical, false
	}
	if verdict, ok, err := evidenceRefQualityVerdictFromRepository(ctx, evidence.ValidationResults, ref); err == nil && ok {
		return ref, validationEvidenceCanSatisfyDoD(verdict)
	}
	if ok, err := patchEvidenceRefUsable(ctx, evidence.Patches, ref); err == nil && ok {
		return ref, true
	}
	if step, ok, err := stepByEvidenceRef(ctx, steps, ref); err == nil && ok {
		canonical := string(step.ID)
		if completionEvidenceStepUsable(step) {
			return canonical, true
		}
		if replacement := catalog.supersededFailureStepRefs[canonical]; replacement != "" {
			return replacement, true
		}
		if step.RunID == catalog.runID {
			if scope := completionEvidenceStepScope(step); scope != "" {
				if replacement := catalog.latestUsableStepByScope[scope]; replacement != "" &&
					catalog.latestUsableOrderByScope[scope] > catalog.stepOrder[canonical] {
					return replacement, true
				}
			}
		}
		return canonical, false
	}
	return "", false
}

func patchEvidenceRefUsable(ctx context.Context, patches repository.PatchRepository, ref string) (bool, error) {
	if patches == nil {
		return false, nil
	}
	patch, err := patches.Get(ctx, common.PatchID(ref))
	if err == nil {
		return patch != nil && patch.State == common.PatchStateApplied, nil
	}
	if err == common.ErrNotFound {
		return false, nil
	}
	return false, err
}

func stepByEvidenceRef(ctx context.Context, steps repository.StepRepository, ref string) (*runModel.Step, bool, error) {
	if steps == nil {
		return nil, false, nil
	}
	step, err := steps.Get(ctx, common.StepID(ref))
	if err == nil {
		return step, true, nil
	}
	if err != common.ErrNotFound {
		return nil, false, err
	}
	for _, marker := range []string{"/validation/", "/tool-call/"} {
		if idx := strings.Index(ref, marker); idx > 0 {
			step, err := steps.Get(ctx, common.StepID(ref[:idx]))
			if err == nil {
				return step, true, nil
			}
			if err != common.ErrNotFound {
				return nil, false, err
			}
		}
	}
	return nil, false, nil
}

func completionEvidenceStepUsable(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	switch step.State {
	case common.StepStateCompleted:
		return true
	default:
		return false
	}
}

func (o *RuntimeOrchestrator) latestCompletionClosureEvidence(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) map[string]any {
	if o == nil || run == nil || o.steps == nil {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	steps = orderedRuntimeContextSteps(steps)
	type closureItem struct {
		order int
		item  map[string]any
	}
	latestByScope := map[string]closureItem{}
	for idx, step := range steps {
		if step == nil || step.ID == "" || step.State != common.StepStateCompleted {
			continue
		}
		scope := completionEvidenceStepScope(step)
		if scope == "" {
			continue
		}
		latestByScope[scope] = closureItem{order: idx, item: completionClosureStepEvidenceItem(step, idx, scope)}
	}
	failures := make([]map[string]any, 0)
	for idx, step := range steps {
		if step == nil || step.ID == "" || step.State != common.StepStateFailed {
			continue
		}
		scope := completionEvidenceStepScope(step)
		if scope == "" {
			continue
		}
		latest, ok := latestByScope[scope]
		if !ok || latest.order <= idx {
			continue
		}
		item := completionClosureStepEvidenceItem(step, idx, scope)
		item["supersededBy"] = stringFromMap(latest.item, "evidenceRef")
		failures = append(failures, item)
	}
	latest := make([]closureItem, 0, len(latestByScope))
	focusScope := ""
	if failedStep != nil {
		focusScope = completionEvidenceStepScope(failedStep)
	}
	for scope, item := range latestByScope {
		if focusScope != "" && scope == focusScope {
			item.item["focus"] = true
		}
		latest = append(latest, item)
	}
	sort.SliceStable(latest, func(i, j int) bool {
		leftFocus, _ := latest[i].item["focus"].(bool)
		rightFocus, _ := latest[j].item["focus"].(bool)
		if leftFocus != rightFocus {
			return !leftFocus
		}
		return latest[i].order > latest[j].order
	})
	latestLimit := min(len(latest), 8)
	latestItems := make([]map[string]any, 0, latestLimit)
	for idx := 0; idx < latestLimit; idx++ {
		latestItems = append(latestItems, latest[idx].item)
	}
	patchItems := latestCompletionPatchEvidence(ctx, o.evidence, run, failedStep)
	validationItems := latestCompletionValidationEvidence(ctx, o.steps, o.evidence, run, failedStep)
	failureLimit := min(len(failures), 8)
	ret := map[string]any{}
	if len(latestItems) > 0 {
		ret["latestEvidence"] = latestItems
	}
	if len(patchItems) > 0 {
		ret["latestPatchEvidence"] = patchItems
	}
	if len(validationItems) > 0 {
		ret["latestValidationEvidence"] = validationItems
	}
	if failureLimit > 0 {
		ret["supersededFailures"] = failures[len(failures)-failureLimit:]
	}
	if len(ret) == 0 {
		return nil
	}
	ret["selectionRule"] = "Prefer later successful evidence in the same neutral scope over earlier failed evidence; older failures remain lineage only."
	return ret
}

func completionClosureStepEvidenceItem(step *runModel.Step, order int, scope string) map[string]any {
	item := map[string]any{
		"evidenceRef": string(step.ID),
		"kind":        step.Kind,
		"state":       string(step.State),
		"order":       order,
		"scope":       scope,
	}
	if nodeID := graphNodeID(step); nodeID != "" {
		item["graphNodeId"] = nodeID
	}
	if target := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(step.Input, "targetRef"),
		stringFromMap(step.Output, "targetRef"),
	)); target != "" {
		item["targetRef"] = target
	}
	if output := compactStepOutputForLLMContext(step.Output); len(output) > 0 {
		item["output"] = output
	}
	return item
}

func latestCompletionPatchEvidence(ctx context.Context, evidence RuntimeEvidenceRepositories, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if run == nil || evidence.Patches == nil {
		return nil
	}
	patches, err := evidence.Patches.ListByRun(ctx, run.ID)
	if err != nil || len(patches) == 0 {
		return nil
	}
	focusScope := ""
	if failedStep != nil {
		focusScope = completionEvidenceStepScope(failedStep)
	}
	ret := make([]map[string]any, 0, min(len(patches), 8))
	for _, patch := range patches {
		if patch == nil || patch.ID == "" || patch.State != common.PatchStateApplied {
			continue
		}
		scope := ""
		if target := strings.TrimSpace(patch.TargetRef); target != "" {
			scope = "targetRef=" + normalizeRuntimeFailureTargetValue(target)
		}
		item := map[string]any{
			"evidenceRef": string(patch.ID),
			"kind":        "patch/" + strings.TrimSpace(patch.PatchType),
			"state":       string(patch.State),
			"stepId":      string(patch.StepID),
			"targetRef":   strings.TrimSpace(patch.TargetRef),
			"scope":       scope,
			"summary":     strings.TrimSpace(patch.Summary),
		}
		if focusScope != "" && scope != "" && scope == focusScope {
			item["focus"] = true
		}
		ret = append(ret, item)
		if len(ret) >= 8 {
			break
		}
	}
	return ret
}

func latestCompletionValidationEvidence(ctx context.Context, steps repository.StepRepository, evidence RuntimeEvidenceRepositories, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if run == nil || evidence.ValidationResults == nil {
		return nil
	}
	items, err := evidence.ValidationResults.ListByRun(ctx, run.ID)
	if err != nil || len(items) == 0 {
		return nil
	}
	focusScope := ""
	if failedStep != nil {
		focusScope = completionEvidenceStepScope(failedStep)
	}
	ret := make([]map[string]any, 0, min(len(items), 8))
	for _, result := range items {
		if result == nil || result.ID == "" || result.State != common.ValidationResultStatePassed {
			continue
		}
		if !validationEvidenceCanSatisfyDoD(qualityFromValidationResult(result)) {
			continue
		}
		scope := ""
		if target := strings.TrimSpace(result.TargetRef); target != "" {
			scope = "targetRef=" + normalizeRuntimeFailureTargetValue(target)
		} else if result.StepID != "" {
			if step, ok, err := stepByEvidenceRef(ctx, steps, string(result.StepID)); err == nil && ok {
				scope = completionEvidenceStepScope(step)
			}
		}
		item := map[string]any{
			"evidenceRef": string(result.ID),
			"kind":        "validation/" + strings.TrimSpace(result.ValidationKind),
			"state":       string(result.State),
			"stepId":      string(result.StepID),
			"targetRef":   strings.TrimSpace(result.TargetRef),
			"scope":       scope,
			"summary":     strings.TrimSpace(result.Summary),
		}
		if evidenceRef := strings.TrimSpace(result.EvidenceRef); evidenceRef != "" {
			item["sourceEvidenceRef"] = evidenceRef
		}
		if focusScope != "" && scope != "" && scope == focusScope {
			item["focus"] = true
		}
		ret = append(ret, item)
		if len(ret) >= 8 {
			break
		}
	}
	return ret
}
