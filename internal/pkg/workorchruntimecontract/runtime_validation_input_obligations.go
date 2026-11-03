package workorchruntimecontract

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

func RuntimeValidationInputReviewProtocolSupported(ref string) bool {
	return ref == RuntimeValidationInputReviewProtocol
}

func validateRuntimeValidationInputObligations(check RuntimeValidationInputCheckReview) error {
	return validateRuntimeValidationInputObligationShape(check, true)
}

// Every report preserves exact content and canonical references.
func validateRuntimeValidationInputObligationShape(check RuntimeValidationInputCheckReview, canonicalReferences bool) error {
	if len(check.Obligations) == 0 || len(check.Obligations) > RuntimeValidationInputReviewMaxObligations {
		return runtimeInputViolation(".obligations", "invalid_obligation_count", fmt.Sprintf("1..%d complete obligation mappings", RuntimeValidationInputReviewMaxObligations), "input review requires a complete bounded obligation mapping")
	}
	seen := make(map[string]int, len(check.Obligations))
	conclusion := RuntimeValidationInputConsistent
	for index, obligation := range check.Obligations {
		path := fmt.Sprintf(".obligations[%d]", index)
		for _, field := range []struct{ name, value string }{{"criterionText", obligation.CriterionText}, {"reason", obligation.Reason}} {
			value := field.value
			if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) || len(value) > RuntimeValidationInputReviewMaxReasonBytes {
				return runtimeInputViolation(path+"."+field.name, "invalid_obligation_text", "nonempty bounded UTF-8 text without surrounding whitespace", "input obligation requires bounded exact criterion text and reasoning")
			}
		}
		if previous, exists := seen[obligation.CriterionText]; exists {
			return runtimeInputViolation(path+".criterionText", "duplicate_criterion_text", fmt.Sprintf("criterionText unique within this check; same excerpt already at obligations[%d]; express joint consumers in one obligation and retain all missing/conflicting/unknown findings in its finding and reason", previous), "input review repeats an obligation excerpt")
		}
		seen[obligation.CriterionText] = index
		switch obligation.Finding {
		case RuntimeValidationInputCovered:
		case RuntimeValidationInputMissing, RuntimeValidationInputConflicting:
			conclusion = RuntimeValidationInputConflict
		case RuntimeValidationInputUnknown:
			if conclusion != RuntimeValidationInputConflict {
				conclusion = RuntimeValidationInputInconclusive
			}
		default:
			return runtimeInputViolation(path+".finding", "unsupported_finding", "covered | missing | conflicting | unknown", "input obligation has an unsupported finding")
		}
		if len(obligation.InputTargetRefs) == 0 || (canonicalReferences && !reflect.DeepEqual(obligation.InputTargetRefs, normalizeStrings(obligation.InputTargetRefs))) {
			return runtimeInputViolation(path+".inputTargetRefs", "noncanonical_input_refs", "nonempty unique sorted refs from this check's inputTargetRefs", "input obligation requires canonical observed input references")
		}
		for _, ref := range obligation.InputTargetRefs {
			if !containsString(check.InputTargetRefs, ref) {
				return runtimeInputViolation(path+".inputTargetRefs", "foreign_input_ref", "refs from this check's inputTargetRefs only", "input obligation references content outside its check review")
			}
		}
	}
	if check.Conclusion != conclusion {
		return runtimeInputViolation(".conclusion", "contradictory_conclusion", conclusion, "input review conclusion contradicts its obligation observations")
	}
	return nil
}

// ValidateCriterionText proves complete textual provenance, not semantic
// entailment. No non-whitespace part of the immutable criterion may disappear
// from the mapping. The reviewer must still judge the actual assertions;
// quoting criterion text is not proof.
func (r RuntimeValidationInputReview) ValidateCriterionText(original string) error {
	for checkIndex, check := range r.Checks {
		path := fmt.Sprintf(".checks[%d]", checkIndex)
		if len(check.Obligations) == 0 {
			return runtimeInputViolation(path+".obligations", "missing_criterion_mapping", "complete exact original criterion excerpts", "criterion requires complete obligation excerpts")
		}

		covered := make([]bool, len(original))
		for index, obligation := range check.Obligations {
			if obligation.CriterionText == "" || !strings.Contains(original, obligation.CriterionText) {
				return runtimeInputViolation(fmt.Sprintf("%s.obligations[%d].criterionText", path, index), "foreign_criterion_excerpt", "exact excerpt of this check's original criterion", "input review obligation is not an exact excerpt of the original criterion")
			}
			for offset := 0; offset < len(original); {
				index := strings.Index(original[offset:], obligation.CriterionText)
				if index < 0 {
					break
				}
				start := offset + index
				for i := start; i < start+len(obligation.CriterionText); i++ {
					covered[i] = true
				}
				// Non-overlapping occurrences bound work by the original text
				// size per obligation, including adversarial repeated text.
				offset = start + len(obligation.CriterionText)
			}
		}
		for index, character := range original {
			if !unicode.IsSpace(character) && !covered[index] {
				return runtimeInputViolation(path+".obligations", "incomplete_criterion_mapping", "exact excerpts covering every non-whitespace original criterion character", "input review obligation mapping omits part of the original criterion")
			}
		}
	}
	return nil
}

// ValidateCoverageCriteria 为每个 exact source check 核对其独立原判据。
// CWI 自身及 Profile 附加检查仍使用共同 CWI 判据，不能以共同文本替代源义务。
func (r RuntimeValidationInputReview) ValidateCoverageCriteria(item RuntimeCoverageItem) error {
	for index, check := range r.Checks {
		criterion := item.TargetText
		for _, source := range item.SourceObligationChecks {
			if source.CheckRef == check.RequiredCheckRef {
				criterion = source.Criterion
				break
			}
		}
		if err := (RuntimeValidationInputReview{Checks: []RuntimeValidationInputCheckReview{check}}).ValidateCriterionText(criterion); err != nil {
			var violation *runtimeValidationInputViolation
			if errors.As(err, &violation) {
				copy := *violation
				copy.Path = strings.Replace(copy.Path, ".checks[0]", fmt.Sprintf(".checks[%d]", index), 1)
				copy.Cause = fmt.Errorf("input review check %q: %w", check.RequiredCheckRef, err)
				return &copy
			}
			return err
		}
	}
	return nil
}

// RuntimeValidationInputReviewOnlyCoverageOmissions classifies an already
// admitted report. It does not choose repair, grant write authority, or turn
// an omission into validation credit. Unknown facts and contradictions prohibit
// the additive-only classification, even when another obligation is missing.
func RuntimeValidationInputReviewOnlyCoverageOmissions(r RuntimeValidationInputReview) bool {
	missing := false
	for _, source := range r.DiagnosticSources {
		if source.Conclusion != RuntimeValidationInputConsistent {
			return false
		}
	}
	if len(r.Checks) == 0 {
		return false
	}
	for _, check := range r.Checks {
		if validateRuntimeValidationInputObligations(check) != nil {
			return false
		}
		for _, obligation := range check.Obligations {
			switch obligation.Finding {
			case RuntimeValidationInputCovered:
			case RuntimeValidationInputMissing:
				missing = true
			default:
				return false
			}
		}
	}
	return missing
}
