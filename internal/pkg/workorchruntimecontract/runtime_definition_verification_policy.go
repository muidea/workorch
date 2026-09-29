package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// ResolveRuntimeDefinitionVerificationPolicy projects only matching sealed
// Profile declarations onto an exact admitted CWI. Wildcards constrain the
// admitted source but cannot introduce a source identity or receipt fact.
func ResolveRuntimeDefinitionVerificationPolicy(profiles []RuntimeProfileContract, scope, sourceID string) (*RuntimeVerificationPolicy, error) {
	var resolved *RuntimeVerificationPolicy
	for _, profile := range profiles {
		for _, requirement := range profile.EvidenceRequirements {
			if strings.TrimSpace(requirement.Scope) != strings.TrimSpace(scope) ||
				(strings.TrimSpace(requirement.ID) != strings.TrimSpace(sourceID) && strings.TrimSpace(requirement.ID) != "*") ||
				requirement.VerificationPolicy == nil {
				continue
			}
			if err := requirement.VerificationPolicy.Validate(); err != nil {
				return nil, err
			}
			if resolved != nil && !SameRuntimeVerificationPolicy(resolved, requirement.VerificationPolicy) {
				return nil, fmt.Errorf("conflicting verification policies for %s/%s", scope, sourceID)
			}
			value := requirement.VerificationPolicy.Normalized()
			resolved = &value
		}
	}
	return resolved, nil
}

// ValidateRuntimeDefinitionVerificationPolicies rejects missing/injected/drifted
// policy projections on fresh load rather than filling them from today's data.
func ValidateRuntimeDefinitionVerificationPolicies(focuses []RuntimeDefinitionFocus, profiles []RuntimeProfileContract) error {
	for _, focus := range focuses {
		expected, err := ResolveRuntimeDefinitionVerificationPolicy(profiles, focus.AssessmentScope, focus.AssessmentID)
		if err != nil {
			return err
		}
		if !SameRuntimeVerificationPolicy(focus.VerificationPolicy, expected) {
			return fmt.Errorf("definition focus %q verification policy differs from sealed Profile", focus.ID)
		}
	}
	return nil
}
