package orchestrator

import (
	"context"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type completionEvidenceRefCatalog struct {
	runID   common.RunID
	aliases map[string]string
	steps   map[string]*runModel.Step
	patches map[string]bool
}

func (o *RuntimeOrchestrator) normalizeCompletionGateDoDAssessmentEvidenceRefs(ctx context.Context, run *runModel.Run, output map[string]any) (map[string]any, error) {
	if o == nil || run == nil || output == nil || !requiresDoDAssessmentCompletionGate(run) {
		return output, nil
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return output, nil
	}
	catalog, err := o.completionEvidenceRefCatalog(ctx, run)
	if err != nil {
		return nil, err
	}
	normalized, changed := o.normalizeDoDAssessmentEvidenceRefs(ctx, catalog, assessment)
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

func (o *RuntimeOrchestrator) completionEvidenceRefCatalog(ctx context.Context, run *runModel.Run) (completionEvidenceRefCatalog, error) {
	catalog := completionEvidenceRefCatalog{
		runID:   run.ID,
		aliases: map[string]string{},
		steps:   map[string]*runModel.Step{},
		patches: map[string]bool{},
	}
	if o == nil || o.steps == nil || run == nil || run.ID == "" {
		return catalog, nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return catalog, err
	}
	for _, step := range steps {
		if step == nil || step.ID == "" {
			continue
		}
		canonical := string(step.ID)
		catalog.steps[canonical] = step
		catalog.addAlias(canonical, canonical)
		catalog.addAlias(canonical+"/tool-call/runtime-execute", canonical)
		for _, rawAlias := range completionEvidenceStepAliases(run.ID, step) {
			catalog.addAlias(rawAlias, canonical)
		}
	}
	for _, step := range steps {
		if step == nil || step.ID == "" {
			continue
		}
		canonical := string(step.ID)
		for _, rawAlias := range completionEvidenceFollowUpNodeAliases(run.ID, step, steps) {
			catalog.addAlias(rawAlias, canonical)
		}
	}
	if o.evidence.Patches != nil {
		patches, err := o.evidence.Patches.ListByRun(ctx, run.ID)
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

func (o *RuntimeOrchestrator) normalizeDoDAssessmentEvidenceRefs(ctx context.Context, catalog completionEvidenceRefCatalog, assessment map[string]any) (map[string]any, bool) {
	if assessment == nil {
		return assessment, false
	}
	ret := cloneStepPayload(assessment)
	changed := false
	if refs, ok := normalizeEvidenceRefList(ctx, o, catalog, ret["evidenceRefs"]); ok {
		ret["evidenceRefs"] = refs
		changed = true
	}
	if items, ok := normalizeAssessmentItemsEvidenceRefs(ctx, o, catalog, ret["items"]); ok {
		ret["items"] = items
		changed = true
	}
	for _, key := range []string{"goalCoverage", "goalOutcomeCoverage", "acceptanceDimensions", "dimensionCoverage", "deliverableCoverage", "deliverables", "producedArtifacts"} {
		value := ret[key]
		normalized, ok := normalizeCoverageEvidenceRefs(ctx, o, catalog, value)
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

func normalizeCoverageEvidenceRefs(ctx context.Context, o *RuntimeOrchestrator, catalog completionEvidenceRefCatalog, value any) (any, bool) {
	if mapped := mapFromValue(value); len(mapped) > 0 {
		items, changed := normalizeAssessmentItemsEvidenceRefs(ctx, o, catalog, mapped["items"])
		if !changed {
			return value, false
		}
		ret := cloneStepPayload(mapped)
		ret["items"] = items
		return ret, true
	}
	items, changed := normalizeAssessmentItemsEvidenceRefs(ctx, o, catalog, value)
	if !changed {
		return value, false
	}
	return items, true
}

func normalizeAssessmentItemsEvidenceRefs(ctx context.Context, o *RuntimeOrchestrator, catalog completionEvidenceRefCatalog, value any) ([]map[string]any, bool) {
	items, ok := doDAssessmentItemMaps(value)
	if !ok || len(items) == 0 {
		return nil, false
	}
	ret := make([]map[string]any, 0, len(items))
	changed := false
	for _, item := range items {
		normalized := cloneStepPayload(item)
		if refs, ok := normalizeEvidenceRefList(ctx, o, catalog, normalized["evidenceRefs"]); ok {
			normalized["evidenceRefs"] = refs
			changed = true
		}
		ret = append(ret, normalized)
	}
	if !changed {
		return items, false
	}
	return ret, true
}

func normalizeEvidenceRefList(ctx context.Context, o *RuntimeOrchestrator, catalog completionEvidenceRefCatalog, value any) ([]string, bool) {
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
		canonical, ok := o.canonicalCompletionEvidenceRef(ctx, catalog, ref)
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

func (o *RuntimeOrchestrator) canonicalCompletionEvidenceRef(ctx context.Context, catalog completionEvidenceRefCatalog, ref string) (string, bool) {
	ref = strings.Trim(strings.TrimSpace(ref), "/")
	if ref == "" || !traceableEvidenceRef(ref) {
		return "", false
	}
	if canonical := catalog.aliases[ref]; canonical != "" {
		if catalog.patches[canonical] {
			return canonical, true
		}
		return canonical, completionEvidenceStepUsable(catalog.steps[canonical])
	}
	if verdict, ok, err := o.evidenceRefQualityVerdict(ctx, ref); err == nil && ok {
		return ref, validationEvidenceCanSatisfyDoD(verdict)
	}
	if ok, err := o.patchEvidenceRefUsable(ctx, ref); err == nil && ok {
		return ref, true
	}
	if step, ok, err := o.stepByEvidenceRef(ctx, ref); err == nil && ok {
		return string(step.ID), completionEvidenceStepUsable(step)
	}
	return "", false
}

func (o *RuntimeOrchestrator) patchEvidenceRefUsable(ctx context.Context, ref string) (bool, error) {
	if o == nil || o.evidence.Patches == nil {
		return false, nil
	}
	patch, err := o.evidence.Patches.Get(ctx, common.PatchID(ref))
	if err == nil {
		return patch != nil && patch.State == common.PatchStateApplied, nil
	}
	if err == common.ErrNotFound {
		return false, nil
	}
	return false, err
}

func (o *RuntimeOrchestrator) stepByEvidenceRef(ctx context.Context, ref string) (*runModel.Step, bool, error) {
	if o == nil || o.steps == nil {
		return nil, false, nil
	}
	step, err := o.steps.Get(ctx, common.StepID(ref))
	if err == nil {
		return step, true, nil
	}
	if err != common.ErrNotFound {
		return nil, false, err
	}
	for _, marker := range []string{"/validation/", "/tool-call/"} {
		if idx := strings.Index(ref, marker); idx > 0 {
			step, err := o.steps.Get(ctx, common.StepID(ref[:idx]))
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
