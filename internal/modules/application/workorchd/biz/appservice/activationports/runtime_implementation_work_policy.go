package activationports

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// Presence must survive decoding, even when a direct caller bypasses the raw
// shape gate. Invalid input becomes an invalid typed declaration, never nil.
func runtimeImplementationWorkPolicy(input map[string]any) *runtimecontract.RuntimeImplementationWorkPolicy {
	raw, present := input["implementationWorkPolicy"]
	if !present {
		return nil
	}
	object, ok := raw.(map[string]any)
	if !ok || object == nil {
		return &runtimecontract.RuntimeImplementationWorkPolicy{}
	}
	if err := validateRuntimeProfileObject(object, "implementationWork", "implementationWorkPolicy"); err != nil {
		return &runtimecontract.RuntimeImplementationWorkPolicy{}
	}
	policy := &runtimecontract.RuntimeImplementationWorkPolicy{
		ProtocolRef:                          runtimeString(object["protocolRef"]),
		OwnerModuleID:                        runtimeString(object["ownerModuleId"]),
		TargetSelectorRef:                    runtimeString(object["targetSelectorRef"]),
		TargetSelectorOwnerRef:               runtimeString(object["targetSelectorOwnerRef"]),
		TargetSelectorSubjectRef:             runtimeString(object["targetSelectorSubjectRef"]),
		TargetSelectorCandidateRefs:          runtimeStrings(object["targetSelectorCandidateRefs"]),
		ProspectiveTargetAllOfConstraintRefs: runtimeStrings(object["prospectiveTargetAllOfConstraintRefs"]),
		MaxRoundsPerCWI:                      runtimeInt(object["maxRoundsPerCWI"]),
		MaxTargetsPerRound:                   runtimeInt(object["maxTargetsPerRound"]),
		RequireFreshObservation:              runtimeBool(object["requireFreshObservation"]),
		RequireFreshValidation:               runtimeBool(object["requireFreshValidation"]),
		ProtectValidationInputs:              runtimeBool(object["protectValidationInputs"]),
	}
	for _, item := range runtimeMapSlice(object["allowedOperations"]) {
		policy.AllowedOperations = append(policy.AllowedOperations, runtimecontract.RuntimeOperationVariantRef{
			Kind: runtimeString(item["kind"]), VariantID: runtimeString(item["variantId"]),
		})
	}
	for _, value := range runtimeStrings(object["allowedTargetPolicies"]) {
		policy.AllowedTargetPolicies = append(policy.AllowedTargetPolicies, runtimecontract.RuntimeOperationTargetPolicy(value))
	}
	return policy
}
