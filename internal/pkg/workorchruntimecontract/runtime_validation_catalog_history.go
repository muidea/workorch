package workorchruntimecontract

import "fmt"

// Private to one complete current/history call. No success product, flag or
// session escapes this function; standalone catalog calls retain full gates.
type runtimeValidationCatalogRequirementPreimage struct {
	raw       []RuntimeValidationProofRequirement
	requested []RuntimeValidationProofRequirement
}

func cloneRuntimeValidationProofRequirementRaw(in RuntimeValidationProofRequirement) RuntimeValidationProofRequirement {
	in.RequiredCheckRefs = cloneRuntimeStateRawSlice(in.RequiredCheckRefs, func(value string) string { return value })
	in.SourceObligationChecks = CloneRuntimeSourceObligationChecks(in.SourceObligationChecks)
	in.LineageAssertions = cloneRuntimeStateRawSlice(in.LineageAssertions, func(value RuntimeLineageAssertion) RuntimeLineageAssertion { return value })
	return in
}

// ValidateRuntimeValidationProofRequirementCatalogHistory preserves the
// original current-then-individual-history checks and error labels. Only the
// unchanged Definition requirements, fully checked by the current gate, are
// reused. Every raw catalog and its participation gate are checked anew.
func ValidateRuntimeValidationProofRequirementCatalogHistory(requirements []RuntimeValidationProofRequirement, current, history []RuntimeOperationAdmissibility) error {
	if err := ValidateRuntimeValidationProofRequirementCatalogApplicability(requirements, current); err != nil {
		return fmt.Errorf("runtime aggregate current validation proof applicability: %w", err)
	}
	prior := runtimeValidationCatalogRequirementPreimage{
		raw:       cloneRuntimeStateRawSlice(requirements, cloneRuntimeValidationProofRequirementRaw),
		requested: NormalizeRuntimeValidationProofRequirements(requirements),
	}
	for index, catalog := range history {
		if err := validateRuntimeValidationProofRequirementCatalogApplicability(requirements, []RuntimeOperationAdmissibility{catalog}, &prior); err != nil {
			return fmt.Errorf("runtime aggregate sealed validation proof applicability %d: %w", index, err)
		}
	}
	return nil
}
