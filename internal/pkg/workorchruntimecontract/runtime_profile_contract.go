package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

type RuntimeProfileContract struct {
	ID                      string
	AcceptanceDimensions    []RuntimeAcceptanceDimension
	DeliverableExpectations []RuntimeDeliverableExpectation
	EvidenceRequirements    []RuntimeProfileEvidenceRequirement
}

type RuntimeAcceptanceDimension struct {
	ID          string
	Required    bool
	Description string
}

type RuntimeDeliverableExpectation struct {
	ID           string
	Required     bool
	EvidenceRole string
	Description  string
}

type RuntimeProfileEvidenceRequirement struct {
	ID                                string
	Scope                             string
	EvidenceRole                      string
	RequiredEvidenceKinds             []string
	RequireDirectEvidence             bool
	RequireClaimEvidence              bool
	RequireExecutableBehaviorEvidence bool
	RequireCoverageTargets            bool
	AcceptedStepKinds                 []string
	AcceptedOperationVariants         []RuntimeOperationVariantRef
	CapabilityStateRequirements       []RuntimeProfileCapabilityStateRequirement
	RejectedStepKinds                 []string
	Description                       string
}

type RuntimeProfileCapabilityStateRequirement struct {
	OwnerModuleID       string
	CapabilityStateKind string
	SubjectRef          string
}

func NormalizeRuntimeProfileContracts(values []RuntimeProfileContract) []RuntimeProfileContract {
	byID := map[string]RuntimeProfileContract{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.AcceptanceDimensions = normalizeRuntimeAcceptanceDimensions(value.AcceptanceDimensions)
		value.DeliverableExpectations = normalizeRuntimeDeliverableExpectations(value.DeliverableExpectations)
		value.EvidenceRequirements = normalizeRuntimeProfileEvidenceRequirements(value.EvidenceRequirements)
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	keys := make([]string, 0, len(byID))
	for key := range byID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeProfileContract, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byID[key])
	}
	return nilIfEmpty(ret)
}

func ValidateRuntimeProfileContracts(values []RuntimeProfileContract) error {
	for _, profile := range NormalizeRuntimeProfileContracts(values) {
		if profile.ID == "" {
			return fmt.Errorf("runtime profile contract requires id")
		}
		for _, item := range profile.AcceptanceDimensions {
			if item.ID == "" {
				return fmt.Errorf("runtime acceptance dimension requires id")
			}
		}
		for _, item := range profile.DeliverableExpectations {
			if item.ID == "" {
				return fmt.Errorf("runtime deliverable expectation requires id")
			}
		}
		for _, item := range profile.EvidenceRequirements {
			if item.ID == "" || item.Scope == "" {
				return fmt.Errorf("runtime profile evidence requirement requires id and scope")
			}
			for _, state := range item.CapabilityStateRequirements {
				if !runtimeProfileCapabilityStateOwnerMatchesAcceptedStepKind(state.OwnerModuleID, item.AcceptedStepKinds) {
					return fmt.Errorf("runtime profile evidence requirement %q capability-state owner %q does not own an accepted operation kind", item.ID, state.OwnerModuleID)
				}
			}
		}
	}
	return nil
}

// runtimeProfileCapabilityStateOwnerMatchesAcceptedStepKind keeps capability-state
// prerequisites bound to the same canonical owner contract as the evidence they
// unlock. Profiles declare operation kinds, not EventHub destinations; accepting
// an arbitrary owner string here would otherwise create a sealed prerequisite
// that no owner can receive at runtime.
func runtimeProfileCapabilityStateOwnerMatchesAcceptedStepKind(ownerModuleID string, acceptedStepKinds []string) bool {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	if ownerModuleID == "" {
		return false
	}
	for _, kind := range acceptedStepKinds {
		descriptor, ok := RuntimeOperationDescriptorForKind(CanonicalRuntimeBlockKind(kind))
		if ok && descriptor.OwnerModuleID == ownerModuleID {
			return true
		}
	}
	return false
}

func SameRuntimeProfileContracts(left []RuntimeProfileContract, right []RuntimeProfileContract) bool {
	left = NormalizeRuntimeProfileContracts(left)
	right = NormalizeRuntimeProfileContracts(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if !sameRuntimeProfileContract(left[idx], right[idx]) {
			return false
		}
	}
	return true
}

func normalizeRuntimeAcceptanceDimensions(values []RuntimeAcceptanceDimension) []RuntimeAcceptanceDimension {
	byID := map[string]RuntimeAcceptanceDimension{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Description = strings.TrimSpace(value.Description)
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	return sortedMapValues(byID)
}

func normalizeRuntimeDeliverableExpectations(values []RuntimeDeliverableExpectation) []RuntimeDeliverableExpectation {
	byID := map[string]RuntimeDeliverableExpectation{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.EvidenceRole = strings.TrimSpace(value.EvidenceRole)
		value.Description = strings.TrimSpace(value.Description)
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	return sortedMapValues(byID)
}

func normalizeRuntimeProfileEvidenceRequirements(values []RuntimeProfileEvidenceRequirement) []RuntimeProfileEvidenceRequirement {
	byKey := map[string]RuntimeProfileEvidenceRequirement{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Scope = strings.TrimSpace(value.Scope)
		value.EvidenceRole = strings.TrimSpace(value.EvidenceRole)
		value.RequiredEvidenceKinds = normalizeStrings(value.RequiredEvidenceKinds)
		value.AcceptedStepKinds = normalizeStrings(value.AcceptedStepKinds)
		value.AcceptedOperationVariants = NormalizeRuntimeOperationVariantRefs(value.AcceptedOperationVariants)
		value.CapabilityStateRequirements = normalizeRuntimeProfileCapabilityStateRequirements(value.CapabilityStateRequirements)
		value.RejectedStepKinds = normalizeStrings(value.RejectedStepKinds)
		value.Description = strings.TrimSpace(value.Description)
		if value.ID != "" && value.Scope != "" {
			byKey[value.Scope+"|"+value.ID] = value
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeProfileEvidenceRequirement, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byKey[key])
	}
	return nilIfEmpty(ret)
}

func normalizeRuntimeProfileCapabilityStateRequirements(values []RuntimeProfileCapabilityStateRequirement) []RuntimeProfileCapabilityStateRequirement {
	byKey := map[string]RuntimeProfileCapabilityStateRequirement{}
	for _, value := range values {
		value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
		value.CapabilityStateKind = strings.TrimSpace(value.CapabilityStateKind)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		if value.OwnerModuleID != "" && value.CapabilityStateKind != "" && value.SubjectRef != "" {
			byKey[value.OwnerModuleID+"|"+value.CapabilityStateKind+"|"+value.SubjectRef] = value
		}
	}
	return sortedMapValues(byKey)
}

func sameRuntimeProfileContract(left RuntimeProfileContract, right RuntimeProfileContract) bool {
	leftValues := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{left})
	rightValues := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{right})
	if len(leftValues) != 1 || len(rightValues) != 1 {
		return len(leftValues) == len(rightValues)
	}
	left, right = leftValues[0], rightValues[0]
	if left.ID != right.ID || len(left.AcceptanceDimensions) != len(right.AcceptanceDimensions) ||
		len(left.DeliverableExpectations) != len(right.DeliverableExpectations) || len(left.EvidenceRequirements) != len(right.EvidenceRequirements) {
		return false
	}
	for idx := range left.AcceptanceDimensions {
		if left.AcceptanceDimensions[idx] != right.AcceptanceDimensions[idx] {
			return false
		}
	}
	for idx := range left.DeliverableExpectations {
		if left.DeliverableExpectations[idx] != right.DeliverableExpectations[idx] {
			return false
		}
	}
	for idx := range left.EvidenceRequirements {
		l, r := left.EvidenceRequirements[idx], right.EvidenceRequirements[idx]
		if l.ID != r.ID || l.Scope != r.Scope || l.EvidenceRole != r.EvidenceRole || !sameStrings(l.RequiredEvidenceKinds, r.RequiredEvidenceKinds) || l.RequireDirectEvidence != r.RequireDirectEvidence ||
			l.RequireClaimEvidence != r.RequireClaimEvidence || l.RequireExecutableBehaviorEvidence != r.RequireExecutableBehaviorEvidence ||
			l.RequireCoverageTargets != r.RequireCoverageTargets || l.Description != r.Description ||
			!sameStrings(l.AcceptedStepKinds, r.AcceptedStepKinds) || !SameRuntimeOperationVariantRefs(l.AcceptedOperationVariants, r.AcceptedOperationVariants) || !sameStrings(l.RejectedStepKinds, r.RejectedStepKinds) {
			return false
		}
		if len(l.CapabilityStateRequirements) != len(r.CapabilityStateRequirements) {
			return false
		}
		for idx := range l.CapabilityStateRequirements {
			if l.CapabilityStateRequirements[idx] != r.CapabilityStateRequirements[idx] {
				return false
			}
		}
	}
	return true
}
