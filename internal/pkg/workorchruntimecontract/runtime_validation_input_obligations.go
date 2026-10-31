package workorchruntimecontract

import (
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
		return fmt.Errorf("input review requires a complete bounded obligation mapping")
	}
	seen := make(map[string]bool, len(check.Obligations))
	conclusion := RuntimeValidationInputConsistent
	for _, obligation := range check.Obligations {
		for _, value := range []string{obligation.CriterionText, obligation.Reason} {
			if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) || len(value) > RuntimeValidationInputReviewMaxReasonBytes {
				return fmt.Errorf("input obligation requires bounded exact criterion text and reasoning")
			}
		}
		if seen[obligation.CriterionText] {
			return fmt.Errorf("input review repeats an obligation excerpt")
		}
		seen[obligation.CriterionText] = true
		switch obligation.Finding {
		case RuntimeValidationInputCovered:
		case RuntimeValidationInputMissing, RuntimeValidationInputConflicting:
			conclusion = RuntimeValidationInputConflict
		case RuntimeValidationInputUnknown:
			if conclusion != RuntimeValidationInputConflict {
				conclusion = RuntimeValidationInputInconclusive
			}
		default:
			return fmt.Errorf("input obligation has an unsupported finding")
		}
		if len(obligation.InputTargetRefs) == 0 || (canonicalReferences && !reflect.DeepEqual(obligation.InputTargetRefs, normalizeStrings(obligation.InputTargetRefs))) {
			return fmt.Errorf("input obligation requires canonical observed input references")
		}
		for _, ref := range obligation.InputTargetRefs {
			if !containsString(check.InputTargetRefs, ref) {
				return fmt.Errorf("input obligation references content outside its check review")
			}
		}
	}
	if check.Conclusion != conclusion {
		return fmt.Errorf("input review conclusion contradicts its obligation observations")
	}
	return nil
}

// ValidateCriterionText proves complete textual provenance, not semantic
// entailment. No non-whitespace part of the immutable criterion may disappear
// from the mapping. The reviewer must still judge the actual assertions;
// quoting criterion text is not proof.
func (r RuntimeValidationInputReview) ValidateCriterionText(original string) error {
	for _, check := range r.Checks {
		if len(check.Obligations) == 0 {
			return fmt.Errorf("criterion requires complete obligation excerpts")
		}

		covered := make([]bool, len(original))
		for _, obligation := range check.Obligations {
			if obligation.CriterionText == "" || !strings.Contains(original, obligation.CriterionText) {
				return fmt.Errorf("input review obligation is not an exact excerpt of the original criterion")
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
				return fmt.Errorf("input review obligation mapping omits part of the original criterion")
			}
		}
	}
	return nil
}

// ValidateCoverageCriteria 为每个 exact source check 核对其独立原判据。
// CWI 自身及 Profile 附加检查仍使用共同 CWI 判据，不能以共同文本替代源义务。
func (r RuntimeValidationInputReview) ValidateCoverageCriteria(item RuntimeCoverageItem) error {
	for _, check := range r.Checks {
		criterion := item.TargetText
		for _, source := range item.SourceObligationChecks {
			if source.CheckRef == check.RequiredCheckRef {
				criterion = source.Criterion
				break
			}
		}
		if err := (RuntimeValidationInputReview{Checks: []RuntimeValidationInputCheckReview{check}}).ValidateCriterionText(criterion); err != nil {
			return fmt.Errorf("input review check %q: %w", check.RequiredCheckRef, err)
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
