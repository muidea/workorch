package workorchruntimecontract

import (
	"errors"
	"fmt"
	"strings"
)

const (
	runtimeOperationTargetRuleInvalidAuthorization = "invalid_sealed_target_authorization"
	runtimeOperationTargetRuleRequired             = "target_refs_required"
	runtimeOperationTargetRuleOutside              = "target_outside_sealed_set"
	runtimeOperationTargetRuleProspectiveSingleton = "prospective_exact_singleton_required"
)

// RuntimeOperationTargetAuthorizationError is the owner-neutral finding from
// comparing one operation target set with its sealed authorization. It is a
// pure diagnostic value: it neither selects a candidate correction nor grants
// receipt credit.
type RuntimeOperationTargetAuthorizationError struct {
	Rule     string
	Actual   string
	Allowed  []string
	Required []string
}

func (e *RuntimeOperationTargetAuthorizationError) Error() string {
	if e == nil {
		return "runtime operation target authorization is invalid"
	}
	parts := []string{"runtime operation target authorization", "rule=" + strings.TrimSpace(e.Rule)}
	if strings.TrimSpace(e.Actual) != "" {
		parts = append(parts, "actual="+strings.TrimSpace(e.Actual))
	}
	if len(e.Allowed) != 0 {
		parts = append(parts, "allowed="+strings.Join(normalizeStrings(e.Allowed), ","))
	}
	if len(e.Required) != 0 {
		parts = append(parts, "required="+strings.Join(normalizeStrings(e.Required), ","))
	}
	return strings.Join(parts, "; ")
}

// ValidateRuntimeOperationTargetsAgainstAuthorization is the single pure
// target-policy evaluator shared by strict candidate decoding, semantic
// candidate admission and owner-receipt proof matching.
//
// exact_target and a finite existing_owner_scope authorize any non-empty
// operation target subset whose every member belongs to the sealed set. A
// selector-only existing_owner_scope deliberately leaves final membership to
// owner feasibility. prospective_owner_target authorizes exactly one sealed
// prospective member per operation; completion of the full all-of set remains
// a separate dispatch-level receipt-set proof.
func ValidateRuntimeOperationTargetsAgainstAuthorization(
	authorization RuntimeOperationTargetAuthorization,
	targetRefs []string,
	requireTargets bool,
) error {
	authorization = authorization.Normalized()
	targetRefs = normalizeStrings(targetRefs)
	allowed := authorization.AllowedTargetRefs()

	requiredError := func() error {
		return &RuntimeOperationTargetAuthorizationError{
			Rule: runtimeOperationTargetRuleRequired, Actual: "missing_targetRefs",
			Allowed: append([]string(nil), allowed...), Required: []string{"targetRefs"},
		}
	}
	outsideError := func(targetRef string) error {
		return &RuntimeOperationTargetAuthorizationError{
			Rule: runtimeOperationTargetRuleOutside, Actual: targetRef,
			Allowed: append([]string(nil), allowed...),
		}
	}
	validateFiniteMembership := func() error {
		if len(targetRefs) == 0 {
			if requireTargets {
				return requiredError()
			}
			return nil
		}
		for _, targetRef := range targetRefs {
			if !containsString(allowed, targetRef) {
				return outsideError(targetRef)
			}
		}
		return nil
	}

	switch authorization.Policy {
	case RuntimeOperationTargetPolicyExactTarget:
		if requireTargets && len(allowed) == 0 {
			return &RuntimeOperationTargetAuthorizationError{
				Rule:   runtimeOperationTargetRuleInvalidAuthorization,
				Actual: "empty_exact_target_set", Required: []string{"sealed exact target refs"},
			}
		}
		return validateFiniteMembership()
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		if len(authorization.OwnerScopeTargetRefs) == 0 && len(authorization.OwnerSelectorRefs) == 0 {
			return &RuntimeOperationTargetAuthorizationError{
				Rule:   runtimeOperationTargetRuleInvalidAuthorization,
				Actual: "missing_owner_scope_and_selector", Required: []string{"owner scope target refs or owner selector refs"},
			}
		}
		if len(authorization.OwnerScopeTargetRefs) != 0 {
			return validateFiniteMembership()
		}
		if len(targetRefs) == 0 && requireTargets {
			return requiredError()
		}
		// The owner selector is closed but not a finite local allow-list. The
		// capability owner remains responsible for factual target feasibility.
		return nil
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if len(authorization.OwnerSelectorRefs) == 0 || len(allowed) == 0 {
			return &RuntimeOperationTargetAuthorizationError{
				Rule:     runtimeOperationTargetRuleInvalidAuthorization,
				Actual:   "incomplete_prospective_target_authorization",
				Required: []string{"owner selector refs", "sealed prospective target refs"},
			}
		}
		if len(targetRefs) == 0 {
			return requiredError()
		}
		if len(targetRefs) != 1 {
			return &RuntimeOperationTargetAuthorizationError{
				Rule:   runtimeOperationTargetRuleProspectiveSingleton,
				Actual: strings.Join(targetRefs, ","), Allowed: append([]string(nil), allowed...),
				Required: []string{"exactly one sealed prospective target ref"},
			}
		}
		return validateFiniteMembership()
	default:
		return &RuntimeOperationTargetAuthorizationError{
			Rule:   runtimeOperationTargetRuleInvalidAuthorization,
			Actual: fmt.Sprintf("unsupported_policy_%q", authorization.Policy),
			Required: []string{
				string(RuntimeOperationTargetPolicyExactTarget),
				string(RuntimeOperationTargetPolicyExistingOwnerScope),
				string(RuntimeOperationTargetPolicyProspectiveOwnerTarget),
			},
		}
	}
}

// RuntimeOperationTargetAuthorizationFromProofIdentity projects the exact same
// target policy used by candidate admission from a binding-derived proof
// identity. Transaction and coverage lineage stay outside this pure value.
func RuntimeOperationTargetAuthorizationFromProofIdentity(identity CoverageEvidenceProofIdentity) (RuntimeOperationTargetAuthorization, error) {
	identity = identity.Normalized()
	if err := identity.Validate(); err != nil {
		return RuntimeOperationTargetAuthorization{}, err
	}
	return runtimeOperationTargetAuthorizationFromProofTarget(
		identity.TargetPolicy,
		identity.TargetRefs,
		identity.TargetSelectorRef,
		identity.ProspectiveTargetRefs,
	)
}

// RuntimeOperationTargetAuthorizationFromCoverageEvidenceBinding projects the
// target half of a binding without requiring all allowed proof operations to
// share one owner. Receipt matching already has one concrete sealed operation;
// a multi-owner proof vocabulary must not turn target authorization into an
// unrelated proof-owner identity check.
func RuntimeOperationTargetAuthorizationFromCoverageEvidenceBinding(binding CoverageEvidenceBinding) (RuntimeOperationTargetAuthorization, error) {
	binding = binding.Normalized()
	if err := binding.Validate(); err != nil {
		return RuntimeOperationTargetAuthorization{}, err
	}
	policy, err := coverageEvidenceProofTargetPolicyForBinding(binding)
	if err != nil {
		return RuntimeOperationTargetAuthorization{}, err
	}
	return runtimeOperationTargetAuthorizationFromProofTarget(
		policy,
		binding.TargetRefs,
		binding.TargetSelectorRef,
		binding.ProspectiveTargetRefs,
	)
}

func runtimeOperationTargetAuthorizationFromProofTarget(
	policy CoverageEvidenceProofTargetPolicy,
	targetRefs []string,
	targetSelectorRef string,
	prospectiveTargetRefs []string,
) (RuntimeOperationTargetAuthorization, error) {
	authorization := RuntimeOperationTargetAuthorization{
		Policy: RuntimeOperationTargetPolicy(policy),
	}
	switch policy {
	case CoverageEvidenceProofTargetPolicyExactTarget:
		authorization.ExactTargetRefs = append([]string(nil), targetRefs...)
	case CoverageEvidenceProofTargetPolicyExistingOwnerScope:
		authorization.OwnerScopeTargetRefs = append([]string(nil), targetRefs...)
		if targetSelectorRef != "" {
			authorization.OwnerSelectorRefs = []string{targetSelectorRef}
		}
	case CoverageEvidenceProofTargetPolicyProspectiveOwnerTarget:
		authorization.OwnerSelectorRefs = []string{targetSelectorRef}
		authorization.ProspectiveTargetRefs = append([]string(nil), prospectiveTargetRefs...)
	default:
		return RuntimeOperationTargetAuthorization{}, fmt.Errorf("coverage evidence proof has unsupported target policy %q", policy)
	}
	return authorization.Normalized(), nil
}

func runtimeOperationTargetAuthorizationDiagnostic(err error) (actual string, allowed, required []string) {
	var finding *RuntimeOperationTargetAuthorizationError
	if !errors.As(err, &finding) || finding == nil {
		return "invalid_target_authorization", nil, nil
	}
	return finding.Actual, append([]string(nil), finding.Allowed...), append([]string(nil), finding.Required...)
}
