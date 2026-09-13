package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// ResolveRuntimeDefinitionImplementationWorkPolicy instantiates only matching
// Profile declarations for an exact admitted source. It is a pure projection:
// no owner observation, work selection, budget reservation or binding exists
// here. Identical wildcard/exact declarations agree; neither precedence nor a
// union may broaden their boundary. An absent exact policy does not cancel a
// matching wildcard policy.
func ResolveRuntimeDefinitionImplementationWorkPolicy(profiles []RuntimeProfileContract, scope, sourceID string) (*RuntimeImplementationWorkPolicy, error) {
	scope, sourceID = strings.TrimSpace(scope), strings.TrimSpace(sourceID)
	var resolved *RuntimeImplementationWorkPolicy
	for _, profile := range profiles {
		for _, requirement := range profile.EvidenceRequirements {
			if strings.TrimSpace(requirement.Scope) != scope ||
				(strings.TrimSpace(requirement.ID) != sourceID && strings.TrimSpace(requirement.ID) != "*") ||
				requirement.ImplementationWorkPolicy == nil {
				continue
			}
			if !validRuntimeImplementationWorkRef(scope) || !validRuntimeImplementationWorkRef(sourceID) {
				return nil, fmt.Errorf("implementation work projection requires an exact bounded Definition source")
			}
			policy := requirement.ImplementationWorkPolicy
			if err := policy.Validate(); err != nil {
				return nil, fmt.Errorf("implementation work Profile %q source %s/%s: %w", profile.ID, scope, sourceID, err)
			}
			if resolved != nil && !SameRuntimeImplementationWorkPolicy(resolved, policy) {
				return nil, fmt.Errorf("implementation work has conflicting Profile declarations for %s/%s", scope, sourceID)
			}
			value := policy.Normalized()
			resolved = &value
		}
	}
	return resolved, nil
}

func validateRuntimeDefinitionImplementationWorkPolicy(focus RuntimeDefinitionFocus) error {
	if focus.ImplementationWorkPolicy == nil {
		return nil
	}
	if len(focus.CoverageWorkItemIDs) != 1 || len(focus.RequiredEvidenceKinds) != 1 || focus.RequiredEvidenceKinds[0] != EvidenceKindValidation {
		return fmt.Errorf("runtime definition focus %q implementation work requires one validation-only CWI", focus.ID)
	}
	if err := focus.ImplementationWorkPolicy.Validate(); err != nil {
		return fmt.Errorf("runtime definition focus %q: %w", focus.ID, err)
	}
	return nil
}

// ValidateRuntimeDefinitionImplementationWorkPolicies independently checks the
// Application projection against its sealed Profile source. The existing
// Definition validator binds each focus to the seed's exact CWI/source/text;
// this check additionally rejects policy injection, omission and drift. It
// never fills missing fields during load or grants executable support.
func ValidateRuntimeDefinitionImplementationWorkPolicies(focuses []RuntimeDefinitionFocus, profiles []RuntimeProfileContract) error {
	for _, focus := range focuses {
		if err := validateRuntimeDefinitionImplementationWorkPolicy(focus); err != nil {
			return err
		}
		expected, err := ResolveRuntimeDefinitionImplementationWorkPolicy(profiles, focus.AssessmentScope, focus.AssessmentID)
		if err != nil {
			return err
		}
		if !SameRuntimeImplementationWorkPolicy(focus.ImplementationWorkPolicy, expected) {
			return fmt.Errorf("runtime definition focus %q implementation work differs from its exact sealed Profile source", focus.ID)
		}
	}
	return nil
}
