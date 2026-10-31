package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// These are review responsibilities, not receipt facts or acceptance credit.
func RuntimeValidationProofConsumerRefs(subject RuntimeValidationInputReviewSubject) []string {
	refs := []string{RuntimeValidationInputConsumer}
	for _, assertion := range subject.LineageAssertions {
		refs = append(refs, assertion.Rule.Ref)
	}
	slices.Sort(refs)
	return refs
}

func validateRuntimeValidationCompositionSubject(subject RuntimeValidationInputReviewSubject, binding RuntimeValidationProofRequirementBinding) error {

	if err := ValidateRuntimeLineageAssertions(subject.LineageAssertions, binding.CoverageWorkItemID, binding.DeclarationRef); err != nil {
		return err
	}
	for _, assertion := range subject.LineageAssertions {
		if !boundedValidationExecutionRef(assertion.Rule.Ref, 512) {
			return fmt.Errorf("composition requires bounded provider-safe consumer references")
		}
		if assertion.Rule.Ref == RuntimeValidationInputConsumer {
			return fmt.Errorf("lineage rule aliases reserved validation input consumer")
		}
	}
	return nil
}

func validateRuntimeValidationCompositionCheck(check RuntimeValidationInputCheckReview, subject RuntimeValidationInputReviewSubject) error {

	if len(subject.LineageAssertions) == 0 {
		allAbsent := true
		for _, obligation := range check.Obligations {
			allAbsent = allAbsent && obligation.ProofConsumerRefs == nil
		}
		if allAbsent {
			return nil
		}
	}
	allowed := RuntimeValidationProofConsumerRefs(subject)
	used := make(map[string]bool, len(allowed))
	for _, obligation := range check.Obligations {
		refs := obligation.ProofConsumerRefs
		if len(refs) == 0 || !reflect.DeepEqual(refs, normalizeStrings(refs)) {
			return fmt.Errorf("composite obligation requires canonical exact proof consumers")
		}
		for _, ref := range refs {
			if ref == "" || ref != strings.TrimSpace(ref) || !slices.Contains(allowed, ref) {
				return fmt.Errorf("composite obligation references an undeclared proof consumer")
			}
			used[ref] = true
		}
	}
	for _, ref := range allowed {
		if !used[ref] {
			return fmt.Errorf("composite check omits required proof consumer %q", ref)
		}
	}
	return nil
}

// Independently compare owner-supplied declarations with the original compiled
// requirement. Rehashing a substituted subject does not establish provenance.
func runtimeValidationCompositionMatchesRequirement(subject *RuntimeValidationInputReviewSubject, requirement RuntimeValidationProofRequirement) bool {
	return subject == nil ||
		slices.Equal(subject.LineageAssertions, requirement.LineageAssertions)
}
