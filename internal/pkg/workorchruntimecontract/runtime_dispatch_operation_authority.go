package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
)

// RuntimeDispatchSealedOperationPolicies returns the complete immutable policy
// view used when this dispatch was sealed. Catalog policies remain members of
// their full owner-authored snapshot; OperationPolicies contains only the
// activation fallback selected for operations without a catalog reference.
func RuntimeDispatchSealedOperationPolicies(dispatch RuntimeDispatchRecord) []RuntimeOperationPolicyFact {
	ret := append([]RuntimeOperationPolicyFact(nil), dispatch.OperationPolicies...)
	for _, catalog := range dispatch.OperationAdmissibilities {
		ret = append(ret, catalog.OperationPolicies...)
	}
	return NormalizeRuntimeOperationPolicyFacts(ret)
}

// CloneRuntimeOperationAdmissibilities detaches and canonicalizes complete
// owner catalog snapshots before they cross the Run persistence boundary.
func CloneRuntimeOperationAdmissibilities(values []RuntimeOperationAdmissibility) []RuntimeOperationAdmissibility {
	return NormalizeRuntimeOperationAdmissibilities(values)
}

// CloneRuntimePrerequisiteObservations detaches and canonicalizes the exact
// owner observations retained by a sealed dispatch.
func CloneRuntimePrerequisiteObservations(values []RuntimePrerequisiteObservation) []RuntimePrerequisiteObservation {
	return NormalizeRuntimePrerequisiteObservations(values)
}

// StableRuntimeDispatchOperationAuthorityHash binds the full verified owner
// snapshot set and exact fallback policies. Catalog SnapshotID and observation
// ObservationID are content-addressed preimages whose validity is rechecked by
// ValidateRuntimeDispatchOperationAuthority before this hash is accepted.
func StableRuntimeDispatchOperationAuthorityHash(dispatch RuntimeDispatchRecord) string {
	catalogs := NormalizeRuntimeOperationAdmissibilities(dispatch.OperationAdmissibilities)
	observations := NormalizeRuntimePrerequisiteObservations(dispatch.OperationAdmissibilityObservations)
	policies := NormalizeRuntimeOperationPolicyFacts(dispatch.OperationPolicies)
	h := sha256.New()
	writeHashString(h, "runtime-dispatch-operation-authority/v1")
	writeHashString(h, fmt.Sprintf("%d", len(catalogs)))
	for _, catalog := range catalogs {
		writeHashString(h, catalog.PrerequisiteID)
		writeHashString(h, catalog.SnapshotID)
		writeHashString(h, catalog.AcquisitionBinding)
	}
	writeHashString(h, fmt.Sprintf("%d", len(observations)))
	for _, observation := range observations {
		writeHashString(h, observation.PrerequisiteID)
		writeHashString(h, observation.ObservationID)
		writeHashString(h, observation.ContentHash)
	}
	writeHashString(h, fmt.Sprintf("%d", len(policies)))
	for _, policy := range policies {
		writeHashString(h, runtimeOperationPolicySelectorKey(policy))
		writeHashString(h, policy.ID)
		writeHashString(h, string(policy.EffectScope))
		writeHashString(h, string(policy.IdempotencyClass))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateRuntimeDispatchOperationAuthority makes a dispatch self-verifying:
// every execution resolves effect/replay semantics only from its sealed full
// catalog, its exact fallback policy, or the static operation contract. No
// mutable current Runtime facts participate in this check.
func ValidateRuntimeDispatchOperationAuthority(dispatch RuntimeDispatchRecord) error {
	return validateRuntimeDispatchOperationAuthorityInIndex(dispatch, nil)
}

func validateRuntimeDispatchOperationAuthorityInIndex(dispatch RuntimeDispatchRecord, index *runtimeOwnerLineageIndex) error {
	catalogs := NormalizeRuntimeOperationAdmissibilities(dispatch.OperationAdmissibilities)
	observations := NormalizeRuntimePrerequisiteObservations(dispatch.OperationAdmissibilityObservations)
	policies := NormalizeRuntimeOperationPolicyFacts(dispatch.OperationPolicies)
	if !reflect.DeepEqual(dispatch.OperationAdmissibilities, catalogs) ||
		!reflect.DeepEqual(dispatch.OperationAdmissibilityObservations, observations) ||
		!reflect.DeepEqual(dispatch.OperationPolicies, policies) {
		return fmt.Errorf("runtime dispatch operation authority snapshots must be canonical")
	}

	catalogBySnapshot := make(map[string]RuntimeOperationAdmissibility, len(catalogs))
	observationByPrerequisite := make(map[string]RuntimePrerequisiteObservation, len(observations))
	for _, observation := range observations {
		if err := observation.Validate(); err != nil {
			return fmt.Errorf("runtime dispatch owner catalog observation: %w", err)
		}
		if observation.Kind != PrerequisiteKindOperationAdmissibility {
			return fmt.Errorf("runtime dispatch owner catalog observation %q has kind %q", observation.PrerequisiteID, observation.Kind)
		}
		observationByPrerequisite[observation.PrerequisiteID] = observation
	}
	for _, catalog := range catalogs {
		if err := index.catalog(catalog); err != nil {
			return fmt.Errorf("runtime dispatch owner catalog: %w", err)
		}
		if _, duplicate := catalogBySnapshot[catalog.SnapshotID]; duplicate {
			return fmt.Errorf("runtime dispatch duplicates owner catalog snapshot %q", catalog.SnapshotID)
		}
		if observation, found := observationByPrerequisite[catalog.PrerequisiteID]; found &&
			(observation.OwnerModuleID != catalog.OwnerModuleID || observation.ContentHash != catalog.SnapshotID) {
			return fmt.Errorf("runtime dispatch owner catalog %q observation mismatch", catalog.SnapshotID)
		}
		catalogBySnapshot[catalog.SnapshotID] = catalog
	}
	for _, observation := range observations {
		catalog, found := catalogBySnapshot[observation.ContentHash]
		if !found || catalog.PrerequisiteID != observation.PrerequisiteID || catalog.OwnerModuleID != observation.OwnerModuleID {
			return fmt.Errorf("runtime dispatch owner catalog observation %q has no exact catalog", observation.ObservationID)
		}
	}
	if err := ValidateRuntimeOperationPolicyFacts(policies); err != nil {
		return fmt.Errorf("runtime dispatch fallback operation policy: %w", err)
	}

	executionByOperation := make(map[string]RuntimeOperationExecutionRecord, len(dispatch.OperationExecutions))
	for _, execution := range dispatch.OperationExecutions {
		execution = execution.Normalized()
		executionByOperation[execution.OperationID] = execution
	}
	usedCatalogs := make(map[string]struct{}, len(catalogs))
	usedPolicies := make(map[string]struct{}, len(policies))
	for operationID := range runtimeCandidateOperationIDs(dispatch.Candidate) {
		operation, found := runtimeCandidateOperationForID(dispatch.Candidate, operationID)
		if !found {
			return fmt.Errorf("runtime dispatch operation %q is missing from candidate", operationID)
		}
		execution, found := executionByOperation[operationID]
		if !found {
			return fmt.Errorf("runtime dispatch operation %q has no execution authority", operationID)
		}
		var selectedPolicies []RuntimeOperationPolicyFact
		if execution.AdmissibilitySnapshotID != "" {
			catalog, found := catalogBySnapshot[execution.AdmissibilitySnapshotID]
			if !found || catalog.AcquisitionBinding != execution.AcquisitionBinding {
				return fmt.Errorf("runtime dispatch operation %q has no exact owner catalog snapshot", operationID)
			}
			descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind)
			if !known || descriptor.OwnerModuleID != catalog.OwnerModuleID || !runtimeOperationKindsContain(catalog.OperationKinds, operation.Kind) {
				return fmt.Errorf("runtime dispatch operation %q owner catalog scope mismatch", operationID)
			}
			if policy, policyFound := RuntimeOperationPolicyForOperation(operation, catalog.OperationPolicies); policyFound {
				selectedPolicies = []RuntimeOperationPolicyFact{policy}
			}
			usedCatalogs[catalog.SnapshotID] = struct{}{}
		} else if policy, policyFound := RuntimeOperationPolicyForOperation(operation, policies); policyFound {
			selectedPolicies = []RuntimeOperationPolicyFact{policy}
			usedPolicies[policy.ID] = struct{}{}
		}
		if effect := ResolveExpectedEffectScope(operation, selectedPolicies); effect == RuntimeEffectUnknown || effect != execution.ExpectedEffectScope {
			return fmt.Errorf("runtime dispatch operation %q sealed effect scope mismatch", operationID)
		}
		if idempotency := ResolveOperationIdempotencyClass(operation, selectedPolicies); idempotency != execution.IdempotencyClass {
			return fmt.Errorf("runtime dispatch operation %q sealed idempotency mismatch", operationID)
		}
	}
	if len(usedCatalogs) != len(catalogs) || len(usedPolicies) != len(policies) {
		return fmt.Errorf("runtime dispatch operation authority contains an unreferenced catalog or policy")
	}
	return nil
}
