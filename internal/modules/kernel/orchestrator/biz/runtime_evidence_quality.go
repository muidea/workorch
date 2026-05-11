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
	if o == nil || o.evidence.ValidationResults == nil || run == nil || assessment == nil {
		return nil
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if !ok {
		return nil
	}
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
	return nil
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
