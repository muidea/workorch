package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// A nil session retains the standalone public gate. Internal callers reuse
// only exact raw catalogs fully validated within the same enclosing call.
func validateWorkRoundCatalogInSession(raw RuntimeOperationAdmissibility, session *runtimeSemanticValidationSession) (RuntimeOperationAdmissibility, error) {
	if session != nil {
		return validateRuntimeAdmissibilityInSemanticSession(raw, session)
	}
	if err := raw.Validate(); err != nil {
		return RuntimeOperationAdmissibility{}, err
	}
	return raw.Normalized(), nil
}

func validateWorkRoundInputCatalogsInSession(values []RuntimeOperationAdmissibility, session *runtimeSemanticValidationSession) error {
	if session == nil {
		return ValidateRuntimeOperationAdmissibilities(values)
	}
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		if _, err := validateRuntimeAdmissibilityInSemanticSession(raw, session); err != nil {
			return err
		}
		id := strings.TrimSpace(raw.PrerequisiteID)
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("runtime operation admissibility duplicates prerequisite %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
