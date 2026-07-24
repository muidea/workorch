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
	CoverageDependencies    []RuntimeCoverageDependency
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
	ID           string
	Scope        string
	EvidenceRole string
	// TargetRole identifies the declared target role of direct evidence. It is
	// a profile-owned semantic label; Runtime V2 only transports and compares it.
	TargetRole             string
	TargetRefs             []string
	TargetDiscoverySources []string
	TargetSelectorRef      string
	// TargetSelectorOwnerRef and TargetSelectorSubjectRef bind a generic File
	// selection query to the profile declaration which owns its semantics. File
	// only evaluates the bounded candidate refs; it never interprets these values.
	TargetSelectorOwnerRef      string
	TargetSelectorSubjectRef    string
	TargetSelectorCandidateRefs []string
	// ProspectiveTargetRef is a profile-declared, canonical workspace-relative
	// path for an empty-workspace durable create. It is sealed during activation
	// and verified by the File owner; Runtime V2 never derives it from prose.
	ProspectiveTargetRef              string
	RequireTargetGrounding            bool
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

// RuntimeCoverageDependency is a profile-declared relationship between two
// compiled coverage sources. Runtime V2 transports opaque source IDs and only
// applies the sealed evidence-kind trigger; it never infers a dependency from
// a file path, language, command name, or assessment prose.
type RuntimeCoverageDependency struct {
	ID                     string
	SourceScope            string
	SourceRequirementID    string
	DependentScope         string
	DependentRequirementID string
	TriggerEvidenceKinds   []string
}

func NormalizeRuntimeProfileContracts(values []RuntimeProfileContract) []RuntimeProfileContract {
	byID := map[string]RuntimeProfileContract{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.AcceptanceDimensions = normalizeRuntimeAcceptanceDimensions(value.AcceptanceDimensions)
		value.DeliverableExpectations = normalizeRuntimeDeliverableExpectations(value.DeliverableExpectations)
		value.EvidenceRequirements = normalizeRuntimeProfileEvidenceRequirements(value.EvidenceRequirements)
		value.CoverageDependencies = normalizeRuntimeCoverageDependencies(value.CoverageDependencies)
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
			if err := ValidateRuntimeTargetGrounding(item.RequireTargetGrounding, item.TargetRole, item.TargetRefs, item.TargetDiscoverySources, item.TargetSelectorRef); err != nil {
				return fmt.Errorf("runtime profile evidence requirement %q: %w", item.ID, err)
			}
			if item.ProspectiveTargetRef != "" && (!item.RequireTargetGrounding || item.TargetSelectorRef == "") {
				return fmt.Errorf("runtime profile evidence requirement %q prospective target requires target grounding and selector", item.ID)
			}
			if len(item.TargetSelectorCandidateRefs) > 0 {
				if !item.RequireTargetGrounding || item.TargetSelectorRef == "" || item.TargetSelectorSubjectRef == "" {
					return fmt.Errorf("runtime profile evidence requirement %q candidate selector requires grounding, selector and subject", item.ID)
				}
				if item.ProspectiveTargetRef != "" && !runtimeProfileContainsTargetRef(item.TargetSelectorCandidateRefs, item.ProspectiveTargetRef) {
					return fmt.Errorf("runtime profile evidence requirement %q prospective target must be declared as a selector candidate", item.ID)
				}
			}
			for _, state := range item.CapabilityStateRequirements {
				if !runtimeProfileCapabilityStateOwnerMatchesAcceptedStepKind(state.OwnerModuleID, item.AcceptedStepKinds) {
					return fmt.Errorf("runtime profile evidence requirement %q capability-state owner %q does not own an accepted operation kind", item.ID, state.OwnerModuleID)
				}
			}
		}
		for _, dependency := range profile.CoverageDependencies {
			if dependency.ID == "" || dependency.SourceScope == "" || dependency.SourceRequirementID == "" ||
				dependency.DependentScope == "" || dependency.DependentRequirementID == "" {
				return fmt.Errorf("runtime coverage dependency requires identity, source and dependent bindings")
			}
			if dependency.SourceScope == dependency.DependentScope && dependency.SourceRequirementID == dependency.DependentRequirementID {
				return fmt.Errorf("runtime coverage dependency %q cannot target itself", dependency.ID)
			}
			if len(dependency.TriggerEvidenceKinds) == 0 {
				return fmt.Errorf("runtime coverage dependency %q requires trigger evidence kinds", dependency.ID)
			}
		}
	}
	return nil
}

func runtimeProfileContainsTargetRef(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
		value.TargetRole = strings.TrimSpace(value.TargetRole)
		value.TargetRefs = normalizeStrings(value.TargetRefs)
		value.TargetDiscoverySources = normalizeStrings(value.TargetDiscoverySources)
		value.TargetSelectorRef = strings.TrimSpace(value.TargetSelectorRef)
		value.TargetSelectorOwnerRef = strings.TrimSpace(value.TargetSelectorOwnerRef)
		value.TargetSelectorSubjectRef = strings.TrimSpace(value.TargetSelectorSubjectRef)
		value.TargetSelectorCandidateRefs = normalizeStrings(value.TargetSelectorCandidateRefs)
		value.ProspectiveTargetRef = strings.TrimSpace(value.ProspectiveTargetRef)
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

func normalizeRuntimeCoverageDependencies(values []RuntimeCoverageDependency) []RuntimeCoverageDependency {
	byKey := map[string]RuntimeCoverageDependency{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.SourceScope = strings.TrimSpace(value.SourceScope)
		value.SourceRequirementID = strings.TrimSpace(value.SourceRequirementID)
		value.DependentScope = strings.TrimSpace(value.DependentScope)
		value.DependentRequirementID = strings.TrimSpace(value.DependentRequirementID)
		value.TriggerEvidenceKinds = normalizeStrings(value.TriggerEvidenceKinds)
		key := strings.Join([]string{value.ID, value.SourceScope, value.SourceRequirementID, value.DependentScope, value.DependentRequirementID}, "|")
		byKey[key] = value
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
		len(left.DeliverableExpectations) != len(right.DeliverableExpectations) || len(left.EvidenceRequirements) != len(right.EvidenceRequirements) ||
		len(left.CoverageDependencies) != len(right.CoverageDependencies) {
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
		if l.ID != r.ID || l.Scope != r.Scope || l.EvidenceRole != r.EvidenceRole || l.TargetRole != r.TargetRole || l.TargetSelectorRef != r.TargetSelectorRef || l.TargetSelectorOwnerRef != r.TargetSelectorOwnerRef || l.TargetSelectorSubjectRef != r.TargetSelectorSubjectRef || l.ProspectiveTargetRef != r.ProspectiveTargetRef ||
			!sameStrings(l.TargetRefs, r.TargetRefs) || !sameStrings(l.TargetDiscoverySources, r.TargetDiscoverySources) || !sameStrings(l.TargetSelectorCandidateRefs, r.TargetSelectorCandidateRefs) || l.RequireTargetGrounding != r.RequireTargetGrounding ||
			!sameStrings(l.RequiredEvidenceKinds, r.RequiredEvidenceKinds) || l.RequireDirectEvidence != r.RequireDirectEvidence ||
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
	for idx := range left.CoverageDependencies {
		l, r := left.CoverageDependencies[idx], right.CoverageDependencies[idx]
		if l.ID != r.ID || l.SourceScope != r.SourceScope || l.SourceRequirementID != r.SourceRequirementID ||
			l.DependentScope != r.DependentScope || l.DependentRequirementID != r.DependentRequirementID ||
			!sameStrings(l.TriggerEvidenceKinds, r.TriggerEvidenceKinds) {
			return false
		}
	}
	return true
}
