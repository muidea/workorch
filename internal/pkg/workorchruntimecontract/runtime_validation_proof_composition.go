package workorchruntimecontract

import (
	"fmt"
	"slices"
	"strings"
)

const RuntimeDefinitionAssessmentConsumer = "definition_assessment"

// Normal semantic proof binding requires independent Definition assessment.
// This additional consumer only assigns evaluative responsibility within it.
// It never replaces the mandatory input or lineage consumers and never grants credit at input review.
func RuntimeValidationAllowedProofConsumerRefs(subject RuntimeValidationInputReviewSubject) []string {
	refs := RuntimeValidationProofConsumerRefs(subject)
	refs = append(refs, RuntimeDefinitionAssessmentConsumer)
	slices.Sort(refs)
	return refs
}

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
		if assertion.Rule.Ref == RuntimeValidationInputConsumer || assertion.Rule.Ref == RuntimeDefinitionAssessmentConsumer {
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
	allowed := RuntimeValidationAllowedProofConsumerRefs(subject)
	used := make(map[string]bool, len(allowed))
	for index, obligation := range check.Obligations {
		refs := obligation.ProofConsumerRefs
		path := fmt.Sprintf(".obligations[%d].proofConsumerRefs", index)
		if len(refs) == 0 || len(refs) > len(allowed) {
			return runtimeInputViolation(path, "invalid_consumer_count", "nonempty bounded subset of declared allowedProofConsumerRefs", "composite obligation requires nonempty bounded proof consumers")
		}
		// Consumers are a set, as declared by the provider schema. Preserve the
		// report's exact order for provenance/hash checks; do not sort or repair it.
		seen := make(map[string]bool, len(refs))
		for _, ref := range refs {
			if ref == "" || ref != strings.TrimSpace(ref) || !slices.Contains(allowed, ref) {
				return runtimeInputViolation(path, "undeclared_proof_consumer", "exact declared allowedProofConsumerRefs only", "composite obligation references an undeclared proof consumer")
			}
			if seen[ref] {
				return runtimeInputViolation(path, "duplicate_proof_consumer", "unique proofConsumerRefs in any order", "composite obligation repeats a proof consumer")
			}
			seen[ref] = true
			used[ref] = true
		}
	}
	for _, ref := range RuntimeValidationProofConsumerRefs(subject) {
		if !used[ref] {
			return runtimeInputViolation(".obligations", "missing_proof_consumer", "complete declared requiredProofConsumerRefs across this check's obligations, including "+ref, fmt.Sprintf("composite check omits required proof consumer %q", ref))
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
