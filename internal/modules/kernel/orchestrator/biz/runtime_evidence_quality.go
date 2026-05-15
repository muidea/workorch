package orchestrator

import (
	"context"
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type runtimeEvidenceQuality struct {
	Quality  common.EvidenceQuality
	Reason   string
	ZeroWork bool
	Tags     []string
}

type doDAssessmentEvidenceQualityError struct {
	cause error
	items []map[string]any
}

type profileEvidenceKindRequirement struct {
	Scope             string
	ID                string
	AcceptedStepKinds []string
	RejectedStepKinds []string
	Description       string
}

func (e *doDAssessmentEvidenceQualityError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *doDAssessmentEvidenceQualityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *doDAssessmentEvidenceQualityError) RuntimeFailureDiagnostics() map[string]any {
	if e == nil || len(e.items) == 0 {
		return nil
	}
	return map[string]any{
		"kind":            "dod_evidence_quality_gap",
		"missingDoDItems": e.items,
	}
}

func runtimeStepEvidenceQuality(run *runModel.Run, step *runModel.Step, state common.ValidationResultState, output map[string]any) runtimeEvidenceQuality {
	if state == common.ValidationResultStateFailed {
		return runtimeEvidenceQuality{Quality: common.EvidenceQualityInvalid, Reason: "runtime step failed"}
	}
	return evidenceQualityFromRuntimeOutput(output)
}

func (o *RuntimeOrchestrator) validateDoDAssessmentEvidenceQuality(ctx context.Context, run *runModel.Run, assessment map[string]any) error {
	if o == nil || run == nil || assessment == nil {
		return nil
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if ok && o.evidence.ValidationResults != nil {
		for _, item := range items {
			if strings.TrimSpace(stringFromMap(item, "state")) != "satisfied" {
				continue
			}
			for _, ref := range evidenceRefsFromValue(item["evidenceRefs"]) {
				verdict, ok, err := o.evidenceRefQualityVerdict(ctx, ref)
				if err != nil {
					return err
				}
				if !ok || validationEvidenceCanSatisfyDoD(verdict) {
					continue
				}
				itemName := strings.TrimSpace(stringFromMap(item, "item"))
				reason := strings.TrimSpace(verdict.Reason)
				cause := fmt.Errorf("%w: satisfied dodAssessment item %q references %s evidence %q: %s", common.ErrInvalidArgument, itemName, verdict.Quality, ref, reason)
				return &doDAssessmentEvidenceQualityError{
					cause: cause,
					items: []map[string]any{{
						"kind":         "leaf",
						"item":         itemName,
						"state":        "failed",
						"source":       "evidenceQuality",
						"evidenceRefs": []string{ref},
						"reason":       firstNonEmptyString(reason, fmt.Sprintf("Referenced evidence quality is %s.", verdict.Quality)),
						"impact":       "This DoD item cannot be marked satisfied until concrete verification evidence is produced.",
						"suggestedNextActions": []string{
							"Add or run effective verification that exercises this DoD item.",
							"Replace weak, zero-work, or inconclusive evidence with a passing command that executes real assertions.",
						},
					}},
				}
			}
		}
	}
	if err := o.validateDoDAssessmentEvidenceKindRequirements(ctx, run, assessment); err != nil {
		return err
	}
	return nil
}

func (o *RuntimeOrchestrator) validateDoDAssessmentEvidenceKindRequirements(ctx context.Context, run *runModel.Run, assessment map[string]any) error {
	requirements := profileEvidenceKindRequirements(run)
	if len(requirements) == 0 || o == nil || o.steps == nil {
		return nil
	}
	for _, requirement := range requirements {
		items := assessmentItemsForEvidenceKindRequirement(requirement.Scope, assessment)
		if len(items) == 0 {
			continue
		}
		for _, item := range items {
			if strings.ToLower(strings.TrimSpace(stringFromMap(item, "state"))) != "satisfied" {
				continue
			}
			if !evidenceKindRequirementMatchesItem(requirement, item) {
				continue
			}
			if err := o.validateEvidenceRefsStepKinds(ctx, requirement, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func profileEvidenceKindRequirements(run *runModel.Run) []profileEvidenceKindRequirement {
	if run == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	executionContract := mapFromValue(run.ExecutionContext["executionContract"])
	profileContracts := mapFromValue(executionContract["profileContracts"])
	if len(profileContracts) == 0 {
		return nil
	}
	ret := make([]profileEvidenceKindRequirement, 0)
	for _, rawProfile := range profileContracts {
		profile := mapFromValue(rawProfile)
		if len(profile) == 0 {
			continue
		}
		for _, raw := range mapSliceFromValue(profile["evidenceRequirements"]) {
			id := strings.TrimSpace(stringFromMap(raw, "id"))
			if id == "" {
				id = strings.TrimSpace(stringFromMap(raw, "dimension"))
			}
			scope := strings.TrimSpace(firstNonEmptyString(
				stringFromMap(raw, "scope"),
				stringFromMap(raw, "appliesTo"),
			))
			if id == "" || scope == "" {
				continue
			}
			req := profileEvidenceKindRequirement{
				Scope:             scope,
				ID:                id,
				AcceptedStepKinds: stringSliceFromValue(raw["acceptedStepKinds"]),
				RejectedStepKinds: stringSliceFromValue(raw["rejectedStepKinds"]),
				Description:       strings.TrimSpace(stringFromMap(raw, "description")),
			}
			if len(req.AcceptedStepKinds) == 0 && len(req.RejectedStepKinds) == 0 {
				continue
			}
			ret = append(ret, req)
		}
	}
	return ret
}

func assessmentItemsForEvidenceKindRequirement(scope string, assessment map[string]any) []map[string]any {
	switch normalizeEvidenceKindRequirementScope(scope) {
	case "acceptance_dimension":
		return acceptanceDimensionItemsFromAssessment(assessment)
	case "deliverable":
		return deliverableCoverageItemsFromAssessment(assessment)
	case "goal_coverage":
		coverage := mapFromValue(assessment["goalCoverage"])
		if len(coverage) == 0 {
			coverage = mapFromValue(assessment["goalOutcomeCoverage"])
		}
		return mapSliceFromValue(coverage["items"])
	case "dod_item":
		items, ok := doDAssessmentItemMaps(assessment["items"])
		if !ok {
			return nil
		}
		return items
	default:
		return nil
	}
}

func normalizeEvidenceKindRequirementScope(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	scope = strings.ReplaceAll(scope, "-", "_")
	scope = strings.ReplaceAll(scope, " ", "_")
	switch scope {
	case "acceptance_dimension", "acceptance_dimensions", "dimension", "dimension_coverage", "acceptancedimension":
		return "acceptance_dimension"
	case "deliverable", "deliverables", "deliverable_coverage", "produced_artifacts":
		return "deliverable"
	case "goal", "goal_coverage", "goal_outcome_coverage":
		return "goal_coverage"
	case "dod", "dod_item", "items", "assessment_item":
		return "dod_item"
	default:
		return scope
	}
}

func evidenceKindRequirementMatchesItem(requirement profileEvidenceKindRequirement, item map[string]any) bool {
	want := strings.TrimSpace(requirement.ID)
	if want == "" {
		return false
	}
	for _, key := range []string{"id", "dimension", "key", "name", "deliverable", "path", "goal", "goalItem", "item", "requirement"} {
		if strings.TrimSpace(stringFromMap(item, key)) == want {
			return true
		}
	}
	return false
}

func (o *RuntimeOrchestrator) validateEvidenceRefsStepKinds(ctx context.Context, requirement profileEvidenceKindRequirement, item map[string]any) error {
	refs := evidenceRefsFromValue(item["evidenceRefs"])
	if len(refs) == 0 {
		return nil
	}
	accepted := mapStringSet(requirement.AcceptedStepKinds)
	rejected := mapStringSet(requirement.RejectedStepKinds)
	hasAcceptedRequirement := len(accepted) > 0
	hasAcceptedEvidence := false
	knownEvidenceRefs := 0
	rejectedRefs := make([]string, 0)
	rejectedKind := ""
	itemName := evidenceKindRequirementItemName(requirement, item)
	for _, ref := range refs {
		kind, ok, err := o.evidenceRefStepKind(ctx, ref)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		knownEvidenceRefs++
		if rejected[kind] {
			rejectedRefs = append(rejectedRefs, ref)
			if rejectedKind == "" {
				rejectedKind = kind
			}
		}
		if accepted[kind] {
			hasAcceptedEvidence = true
		}
	}
	if hasAcceptedRequirement && hasAcceptedEvidence {
		return nil
	}
	if len(rejectedRefs) > 0 {
		reason := fmt.Sprintf("step kind %q is not sufficient for this evidence requirement", rejectedKind)
		if hasAcceptedRequirement {
			reason = fmt.Sprintf("step kind %q is not sufficient by itself for this evidence requirement", rejectedKind)
		}
		return evidenceKindRequirementError(requirement, itemName, strings.Join(rejectedRefs, ", "), rejectedKind, reason)
	}
	if hasAcceptedRequirement && knownEvidenceRefs > 0 && !hasAcceptedEvidence {
		return evidenceKindRequirementError(requirement, itemName, strings.Join(refs, ", "), "", fmt.Sprintf("requires evidence from step kinds: %s", strings.Join(requirement.AcceptedStepKinds, ", ")))
	}
	return nil
}

func (o *RuntimeOrchestrator) evidenceRefStepKind(ctx context.Context, ref string) (string, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || o == nil || o.steps == nil {
		return "", false, nil
	}
	if step, ok, err := o.stepKindByRef(ctx, ref); err != nil || ok {
		if step == nil {
			return "", ok, err
		}
		return strings.TrimSpace(step.Kind), true, nil
	}
	if o.evidence.ValidationResults != nil {
		if item, err := o.evidence.ValidationResults.Get(ctx, common.ValidationResultID(ref)); err == nil {
			if step, ok, err := o.stepKindByRef(ctx, string(item.StepID)); err != nil || ok {
				if step == nil {
					return "", ok, err
				}
				return strings.TrimSpace(step.Kind), true, nil
			}
		} else if err != common.ErrNotFound {
			return "", false, err
		}
	}
	for _, marker := range []string{"/validation/", "/tool-call/"} {
		if idx := strings.Index(ref, marker); idx > 0 {
			if step, ok, err := o.stepKindByRef(ctx, ref[:idx]); err != nil || ok {
				if step == nil {
					return "", ok, err
				}
				return strings.TrimSpace(step.Kind), true, nil
			}
		}
	}
	return "", false, nil
}

func (o *RuntimeOrchestrator) stepKindByRef(ctx context.Context, ref string) (*runModel.Step, bool, error) {
	step, err := o.steps.Get(ctx, common.StepID(ref))
	if err == nil {
		return step, true, nil
	}
	if err == common.ErrNotFound {
		return nil, false, nil
	}
	return nil, false, err
}

func evidenceKindRequirementItemName(requirement profileEvidenceKindRequirement, item map[string]any) string {
	for _, key := range []string{"id", "dimension", "key", "name", "deliverable", "path", "goal", "goalItem", "item", "requirement"} {
		if value := strings.TrimSpace(stringFromMap(item, key)); value != "" {
			return value
		}
	}
	return requirement.ID
}

func evidenceKindRequirementError(requirement profileEvidenceKindRequirement, itemName, ref, kind, reason string) error {
	message := fmt.Sprintf("satisfied dodAssessment %s item %q references insufficient evidence", normalizeEvidenceKindRequirementScope(requirement.Scope), itemName)
	if ref != "" {
		message += fmt.Sprintf(" %q", ref)
	}
	if kind != "" {
		message += fmt.Sprintf(" with step kind %q", kind)
	}
	if reason != "" {
		message += ": " + reason
	}
	cause := fmt.Errorf("%w: %s", common.ErrInvalidArgument, message)
	return &doDAssessmentEvidenceQualityError{
		cause: cause,
		items: []map[string]any{{
			"kind":         "leaf",
			"item":         itemName,
			"state":        "failed",
			"source":       "evidenceKind",
			"evidenceRefs": []string{ref},
			"reason":       firstNonEmptyString(reason, requirement.Description, "Referenced evidence step kind cannot satisfy this claim."),
			"impact":       "This claim cannot be marked satisfied until it is supported by evidence of an appropriate runtime action.",
			"suggestedNextActions": []string{
				"Run or produce concrete evidence matching the required runtime action.",
				"Replace read-only or unrelated evidence with a step that verifies the claimed behavior or side effect.",
			},
		}},
	}
}

func mapStringSet(values []string) map[string]bool {
	ret := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		ret[value] = true
	}
	return ret
}

func (o *RuntimeOrchestrator) evidenceRefQualityVerdict(ctx context.Context, ref string) (runtimeEvidenceQuality, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || o == nil || o.evidence.ValidationResults == nil {
		return runtimeEvidenceQuality{}, false, nil
	}
	if item, err := o.evidence.ValidationResults.Get(ctx, common.ValidationResultID(ref)); err == nil {
		return qualityFromValidationResult(item), true, nil
	} else if err != common.ErrNotFound {
		return runtimeEvidenceQuality{}, false, err
	}
	items, err := o.evidence.ValidationResults.ListByStep(ctx, common.StepID(ref))
	if err != nil {
		return runtimeEvidenceQuality{}, false, err
	}
	if len(items) == 0 {
		return runtimeEvidenceQuality{}, false, nil
	}
	for _, item := range items {
		quality := qualityFromValidationResult(item)
		if !validationEvidenceCanSatisfyDoD(quality) {
			return quality, true, nil
		}
	}
	return qualityFromValidationResult(items[0]), true, nil
}

func evidenceQualityFromRuntimeOutput(output map[string]any) runtimeEvidenceQuality {
	if output == nil {
		return runtimeEvidenceQuality{Quality: common.EvidenceQualityValid}
	}
	quality := common.EvidenceQuality(strings.TrimSpace(stringFromMap(output, "evidenceQuality")))
	if quality == "" {
		quality = common.EvidenceQualityValid
	}
	return runtimeEvidenceQuality{
		Quality:  quality,
		Reason:   strings.TrimSpace(stringFromMap(output, "evidenceReason")),
		ZeroWork: boolFromValue(output["zeroWork"]),
		Tags:     stringSliceFromValue(output["evidenceTags"]),
	}
}

func qualityFromValidationResult(item *runModel.ValidationResult) runtimeEvidenceQuality {
	if item == nil {
		return runtimeEvidenceQuality{Quality: common.EvidenceQualityInvalid, Reason: "validation result is missing"}
	}
	quality := item.EvidenceQuality
	if quality == "" {
		if item.State == common.ValidationResultStateFailed {
			quality = common.EvidenceQualityInvalid
		} else {
			quality = common.EvidenceQualityValid
		}
	}
	return runtimeEvidenceQuality{
		Quality:  quality,
		Reason:   item.EvidenceReason,
		ZeroWork: item.ZeroWork,
		Tags:     append([]string(nil), item.EvidenceTags...),
	}
}

func validationEvidenceCanSatisfyDoD(item runtimeEvidenceQuality) bool {
	if item.ZeroWork {
		return false
	}
	switch item.Quality {
	case "", common.EvidenceQualityValid:
		return true
	default:
		return false
	}
}

func injectRuntimeEvidencePolicies(input map[string]any, executionContext map[string]any) {
	if input == nil || input["evidencePolicies"] != nil || executionContext == nil {
		return
	}
	policies := mapSliceFromValue(executionContext["evidencePolicies"])
	if len(policies) == 0 {
		return
	}
	input["evidencePolicies"] = policies
}
