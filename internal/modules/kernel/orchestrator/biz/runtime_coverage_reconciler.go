package orchestrator

import (
	"context"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) reconcileCoverageValidationEvidence(ctx context.Context, run *runModel.Run) (bool, int) {
	if o == nil || run == nil {
		return false, 0
	}
	if o.evidence.ValidationResults == nil || o.steps == nil {
		return false, 0
	}
	validations, err := o.evidence.ValidationResults.ListByRun(ctx, run.ID)
	if err != nil {
		return false, 0
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return false, 0
	}
	return o.coverageLedger().reconcileValidationEvidence(run, validations, steps, ctx)
}

func logCoverageReconcilerBinding(runID common.RunID, bound int) {
	if bound <= 0 {
		return
	}
	// Lightweight observability hook; loggers are wired via the kernel but we
	// keep this dependency-free so the reconciler stays testable.
	_ = runID
	_ = bound
}

func (c *coverageSubsystem) reconcileValidationEvidence(
	run *runModel.Run,
	validations []*runModel.ValidationResult,
	steps []*runModel.Step,
	contexts ...context.Context,
) (bool, int) {
	if run == nil || len(run.ExecutionContext) == 0 {
		return false, 0
	}
	if c.ensure(run, contexts...) {
		// ensure may have added the items list — keep going
	}
	items := mapSliceFromValue(run.ExecutionContext[coverageWorkItemsKey])
	if len(items) == 0 {
		return false, 0
	}

	stepIndex := map[common.StepID]*runModel.Step{}
	for _, step := range steps {
		if step == nil || step.ID == "" {
			continue
		}
		stepIndex[step.ID] = step
	}

	bound := 0
	changed := false
	seenPairs := map[string]bool{}

	for _, validation := range validations {
		if !reconcileValidationAcceptable(validation) {
			continue
		}
		step, ok := stepIndex[validation.StepID]
		if !ok || step == nil {
			continue
		}
		if !reconcileStepIsBindable(step) {
			continue
		}
		tokens := reconcileCandidateTokensForStep(step)
		if len(tokens) == 0 {
			continue
		}
		for _, item := range items {
			if !reconcileItemNeedsValidationBinding(item) {
				continue
			}
			if reconcileProfileRejectsBehavior(run, item, step) {
				continue
			}
			pairKey := string(validation.ID) + "|" + stringFromMap(item, "id")
			if seenPairs[pairKey] {
				continue
			}
			update := coverageWorkItemUpdate{
				TargetTokens:  tokens,
				EvidenceRefs:  []string{string(validation.EvidenceRef)},
				EvidenceKinds: []string{"validation"},
			}
			if !coverageWorkItemMatchesUpdate(item, update) {
				continue
			}
			seenPairs[pairKey] = true
			beforeAccepted := coverageEvidenceCountForKind("validation", mapStringSliceValue(item["acceptedEvidenceByKind"])["validation"])
			if coverageApplyWorkItemEvidenceWithOptions(
				item,
				[]string{"validation"},
				[]string{string(validation.EvidenceRef)},
				true,
				true,
				string(validation.StepID),
				"reconciled from runtime_step validation",
				coverageEvidenceAcceptanceOptionsFromValidationStep(validation, step),
			) {
				afterAccepted := coverageEvidenceCountForKind("validation", mapStringSliceValue(item["acceptedEvidenceByKind"])["validation"])
				if afterAccepted > beforeAccepted {
					bound++
				}
				changed = true
			}
		}
	}

	if !changed {
		return false, 0
	}
	revision := coverageDefinitionRevision(run.ExecutionContext)
	run.ExecutionContext[coverageWorkItemsKey] = items
	run.ExecutionContext[coverageWorkItemContextKey] = coverageWorkItemContextFromItems(
		revision,
		items,
		coverageExcludedSourcesFromContext(run.ExecutionContext),
		coverageHistoricalItemsFromContext(run.ExecutionContext),
	)
	c.syncRepositoryProjection(coverageRepositoryContext(run, contexts...), run, items)
	return true, bound
}

func reconcileValidationAcceptable(v *runModel.ValidationResult) bool {
	if v == nil {
		return false
	}
	if strings.TrimSpace(v.ValidationKind) != "runtime_step" {
		return false
	}
	if v.State != common.ValidationResultStatePassed {
		return false
	}
	if !validationEvidenceCanSatisfyDoD(qualityFromValidationResult(v)) {
		return false
	}
	return true
}

func coverageEvidenceAcceptanceOptionsFromValidationResult(validation *runModel.ValidationResult) coverageEvidenceAcceptanceOptions {
	return coverageEvidenceAcceptanceOptionsFromValidationStep(validation, nil)
}

func coverageEvidenceAcceptanceOptionsFromValidationStep(validation *runModel.ValidationResult, step *runModel.Step) coverageEvidenceAcceptanceOptions {
	if validation == nil {
		return coverageEvidenceAcceptanceOptions{}
	}
	stepOptions := coverageEvidenceAcceptanceOptionsFromStep(step)
	quality := qualityFromValidationResult(validation)
	if step != nil && validationEvidenceCanSatisfyDoD(quality) && !validationEvidenceCanSatisfyDoD(stepOptions.Quality) {
		quality = stepOptions.Quality
	}
	targetRefs := append([]string{}, stepOptions.TargetRefs...)
	if targetRef := strings.TrimSpace(validation.TargetRef); targetRef != "" {
		targetRefs = append(targetRefs, targetRef)
	}
	claims := append([]string{}, stepOptions.ObservedClaims...)
	ret := coverageEvidenceAcceptanceOptions{
		EnforceProofStrength: true,
		Quality:              quality,
		TargetRefs:           compactUniqueStrings(targetRefs),
		ObservedClaims:       compactUniqueStrings(claims),
		ProofKind:            stepOptions.ProofKind,
		CoverageAlignment:    stepOptions.CoverageAlignment,
		Tags:                 compactUniqueStrings(append(append([]string{}, stepOptions.Tags...), validation.EvidenceTags...)),
		Reason:               firstNonEmptyString(quality.Reason, strings.TrimSpace(validation.EvidenceReason), stepOptions.Reason),
	}
	if coverageEvidenceTagsRequireProofStrength(ret.Tags) {
		ret.EnforceProofStrength = true
	}
	return ret
}

func reconcileStepIsBindable(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	switch strings.TrimSpace(step.Kind) {
	case "shell/command", "mcp/tool_call":
		return true
	}
	return false
}

func reconcileCandidateTokensForStep(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := normalizeCoverageGapRepairToken(value)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		ret = append(ret, value)
	}
	for _, value := range reconcileStepGraphLeafTokens(string(step.ID)) {
		add(value)
	}
	for _, value := range coverageTargetTokensFromStep(step) {
		add(value)
	}
	if ref := strings.TrimSpace(stringFromMap(step.Output, "targetRef")); ref != "" {
		add(ref)
	}
	if ref := strings.TrimSpace(stringFromMap(step.Input, "targetRef")); ref != "" {
		add(ref)
	}
	return ret
}

func reconcileStepGraphLeafTokens(stepID string) []string {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return nil
	}
	parts := strings.Split(stepID, "/")
	if len(parts) == 0 {
		return nil
	}
	ret := make([]string, 0, 4)
	leaf := strings.TrimSpace(parts[len(parts)-1])
	if leaf != "" {
		ret = append(ret, leaf)
		if idx := strings.Index(leaf, "-"); idx > 0 && idx < len(leaf)-1 {
			ret = append(ret, strings.TrimSpace(leaf[idx+1:]))
		}
	}
	if len(parts) >= 2 {
		parent := strings.TrimSpace(parts[len(parts)-2])
		if parent != "" {
			ret = append(ret, parent)
		}
	}
	return ret
}

func reconcileItemNeedsValidationBinding(item map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	status := strings.TrimSpace(stringFromMap(item, "status"))
	switch status {
	case "satisfied", "blocked", "skipped", "superseded":
		return false
	}
	required := stringSet(stringSliceFromValue(item["requiredEvidenceKinds"]))
	if !required["validation"] {
		return false
	}
	accepted := mapStringSliceValue(item["acceptedEvidenceByKind"])
	if len(accepted["validation"]) > 0 {
		return false
	}
	return true
}

func reconcileProfileRejectsBehavior(run *runModel.Run, item map[string]any, step *runModel.Step) bool {
	if run == nil || item == nil || step == nil {
		return false
	}
	requirements := profileEvidenceKindRequirements(run)
	if len(requirements) == 0 {
		return false
	}
	itemName := evidenceKindRequirementItemName(profileEvidenceKindRequirement{}, item)
	for _, requirement := range requirements {
		if !requirement.RequireExecutableBehaviorEvidence {
			continue
		}
		if !evidenceKindRequirementMatchesItem(requirement, item) {
			continue
		}
		if !evidenceRequirementTargetsRuntimeBehavior(requirement, itemName) {
			continue
		}
		if reason := behaviorEvidenceRejectReason(requirement, itemName, step); reason != "" {
			return true
		}
	}
	return false
}
