package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeCapabilityAcquisitionRequest is the owner-neutral value consumed by
// capability-owner EventHub contracts. It is not itself an EventHub command.
type RuntimeCapabilityAcquisitionRequest struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	PrerequisiteID      string
	OwnerModuleID       string
	CapabilityClass     RuntimeCapabilityClass
	RequiredKinds       []string
}

// RuntimeCapabilityAcquisition is a pure owner capability observation. Each
// capability owner exposes it through its own concrete EventHub result.
type RuntimeCapabilityAcquisition struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	Capabilities        RuntimeCapabilities
	Observation         RuntimePrerequisiteObservation
}

// RuntimeCapabilityPrerequisiteFact is an immutable resource dependency
// published by the owner of one concrete operation kind. The fact carries no
// Runtime disposition: SM-2 compiles its typed Requirement and remains the
// sole authority that decides whether and when to acquire it.
type RuntimeCapabilityPrerequisiteFact struct {
	ID            string
	OwnerModuleID string
	OperationKind string
	Requirement   RuntimeRequirement
}

func (f RuntimeCapabilityPrerequisiteFact) Normalized() RuntimeCapabilityPrerequisiteFact {
	f.ID = strings.TrimSpace(f.ID)
	f.OwnerModuleID = strings.TrimSpace(f.OwnerModuleID)
	f.OperationKind = CanonicalRuntimeBlockKind(f.OperationKind)
	requirements := normalizeRuntimeRequirements([]RuntimeRequirement{f.Requirement})
	if len(requirements) == 1 {
		f.Requirement = requirements[0]
	} else {
		f.Requirement = RuntimeRequirement{}
	}
	f.Requirement.CapabilityStateKind = strings.TrimSpace(f.Requirement.CapabilityStateKind)
	return f
}

func (f RuntimeCapabilityPrerequisiteFact) Validate() error {
	f = f.Normalized()
	if f.ID == "" || f.OwnerModuleID == "" || f.OperationKind == "" {
		return fmt.Errorf("runtime capability prerequisite fact requires identity, owner and operation kind")
	}
	descriptor, ok := RuntimeOperationDescriptorForKind(f.OperationKind)
	if !ok || descriptor.OwnerModuleID != f.OwnerModuleID {
		return fmt.Errorf("runtime capability prerequisite fact operation %q does not belong to owner %q", f.OperationKind, f.OwnerModuleID)
	}
	if err := validateRuntimeCapabilityResourceRequirement(f.Requirement); err != nil {
		return fmt.Errorf("runtime capability prerequisite fact %q: %w", f.ID, err)
	}
	return nil
}

func validateRuntimeCapabilityResourceRequirement(requirement RuntimeRequirement) error {
	requirements := normalizeRuntimeRequirements([]RuntimeRequirement{requirement})
	if len(requirements) != 1 {
		return fmt.Errorf("requires one typed resource requirement")
	}
	requirement = requirements[0]
	requirement.CapabilityStateKind = strings.TrimSpace(requirement.CapabilityStateKind)
	if requirement.Scope != RuntimePrerequisiteScopeGlobal || requirement.OwnerModuleID == "" || requirement.Reason == "" {
		return fmt.Errorf("resource requirement must be global and name an owner and reason")
	}
	switch requirement.Kind {
	case PrerequisiteKindManifest, PrerequisiteKindValidationScratch:
		if requirement.SubjectRef != "" || requirement.CapabilityStateKind != "" || requirement.MinimumBytes != 0 {
			return fmt.Errorf("%s resource requirement contains unsupported qualifiers", requirement.Kind)
		}
	case PrerequisiteKindTargetSnapshot:
		if requirement.SubjectRef == "" || requirement.CapabilityStateKind != "" || requirement.MinimumBytes != 0 {
			return fmt.Errorf("target snapshot resource requirement requires only a subject")
		}
	case PrerequisiteKindCapabilityState:
		if requirement.SubjectRef == "" || requirement.CapabilityStateKind == "" || requirement.MinimumBytes != 0 {
			return fmt.Errorf("capability-state resource requirement requires subject and state kind")
		}
	case PrerequisiteKindProviderCeiling:
		if requirement.MinimumBytes <= 0 || requirement.SubjectRef != "" || requirement.CapabilityStateKind != "" {
			return fmt.Errorf("provider-ceiling resource requirement requires a positive byte floor")
		}
	default:
		return fmt.Errorf("unsupported capability resource prerequisite kind %q", requirement.Kind)
	}
	if requirement.CapabilityClass != "" || len(requirement.RequiredCapabilities) != 0 || requirement.TargetSelectorRef != "" || requirement.TargetSelectorOwnerRef != "" || requirement.TargetSelectorSubjectRef != "" || len(requirement.TargetSelectorCandidateRefs) != 0 || len(requirement.ProspectiveTargetAllOfConstraintRefs) != 0 || requirement.ProspectiveTargetRef != "" {
		return fmt.Errorf("resource requirement contains capability or target-selection authority")
	}
	return nil
}

func NormalizeRuntimeCapabilityPrerequisiteFacts(values []RuntimeCapabilityPrerequisiteFact) []RuntimeCapabilityPrerequisiteFact {
	byID := make(map[string]RuntimeCapabilityPrerequisiteFact, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ret := make([]RuntimeCapabilityPrerequisiteFact, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, byID[id])
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func ValidateRuntimeCapabilityPrerequisiteFacts(values []RuntimeCapabilityPrerequisiteFact) error {
	byID := make(map[string]RuntimeCapabilityPrerequisiteFact, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		if err := value.Validate(); err != nil {
			return err
		}
		if existing, ok := byID[value.ID]; ok && !sameRuntimeCapabilityPrerequisiteFact(existing, value) {
			return fmt.Errorf("runtime capability prerequisite fact %q has conflicting definitions", value.ID)
		}
		byID[value.ID] = value
	}
	return nil
}

func MergeRuntimeCapabilityPrerequisiteFacts(existing, incoming []RuntimeCapabilityPrerequisiteFact) ([]RuntimeCapabilityPrerequisiteFact, error) {
	combined := append(append([]RuntimeCapabilityPrerequisiteFact(nil), existing...), incoming...)
	if err := ValidateRuntimeCapabilityPrerequisiteFacts(combined); err != nil {
		return nil, err
	}
	return NormalizeRuntimeCapabilityPrerequisiteFacts(combined), nil
}

func sameRuntimeCapabilityPrerequisiteFact(left, right RuntimeCapabilityPrerequisiteFact) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.ID == right.ID && left.OwnerModuleID == right.OwnerModuleID && left.OperationKind == right.OperationKind && sameRuntimeRequirement(left.Requirement, right.Requirement)
}

// RuntimeCapabilityUnsupportedError reports that the sealed owner/class/kind
// tuple cannot be served. It is a factual prerequisite error, not a Runtime
// action, retry decision, or governance request.
type RuntimeCapabilityUnsupportedError struct {
	OwnerModuleID   string
	CapabilityClass RuntimeCapabilityClass
	RequiredKinds   []string
	Reason          string
}

func (e *RuntimeCapabilityUnsupportedError) Error() string {
	if e == nil {
		return "runtime capability unavailable"
	}
	ownerModuleID := strings.TrimSpace(e.OwnerModuleID)
	reason := strings.TrimSpace(e.Reason)
	kinds := append([]string(nil), e.RequiredKinds...)
	for idx := range kinds {
		kinds[idx] = strings.TrimSpace(kinds[idx])
	}
	sort.Strings(kinds)
	if reason == "" {
		reason = "unsupported"
	}
	return fmt.Sprintf("runtime capability %s for owner %s is %s: %s", e.CapabilityClass, ownerModuleID, reason, strings.Join(kinds, ","))
}

// RuntimeCapabilityCatalogUnavailableError reports that an owner could not
// expose the catalog required to author an admissibility fact.  It is
// deliberately distinct from RuntimeCapabilityUnsupportedError: the owner
// may support the operation, but no catalog-backed authorization exists yet.
// Callers must treat this as an explicit prerequisite/availability fact, never
// as permission to derive recipes from an advisory server policy.
type RuntimeCapabilityCatalogUnavailableError struct {
	OwnerModuleID string
	SubjectRef    string
	Reason        string
}

func (e *RuntimeCapabilityCatalogUnavailableError) Error() string {
	if e == nil {
		return "runtime capability catalog unavailable"
	}
	owner := strings.TrimSpace(e.OwnerModuleID)
	subject := strings.TrimSpace(e.SubjectRef)
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "owner_unavailable"
	}
	if subject == "" {
		subject = "catalog"
	}
	return fmt.Sprintf("runtime capability catalog %s for owner %s (%s): %s", subject, owner, reason, common.ErrInvalidArgument)
}

func (e *RuntimeCapabilityCatalogUnavailableError) Unwrap() error {
	return common.ErrInvalidArgument
}

// ResolveRuntimeCapabilityAcquisition validates a concrete owner request
// against that owner's immutable capability catalog. It performs no I/O and
// makes no Runtime transition or disposition decision.
func ResolveRuntimeCapabilityAcquisition(request RuntimeCapabilityAcquisitionRequest, ownerModuleID string, supportedKinds ...string) (RuntimeCapabilityAcquisition, error) {
	return ResolveRuntimeCapabilityAcquisitionWithPrerequisites(request, ownerModuleID, nil, supportedKinds...)
}

// ResolveRuntimeCapabilityAcquisitionWithPrerequisites additionally seals the
// subset of owner-published resource dependencies applicable to the requested
// operation kinds. Matching is generic and contains no capability-specific
// branch in Runtime core.
func ResolveRuntimeCapabilityAcquisitionWithPrerequisites(request RuntimeCapabilityAcquisitionRequest, ownerModuleID string, prerequisiteFacts []RuntimeCapabilityPrerequisiteFact, supportedKinds ...string) (RuntimeCapabilityAcquisition, error) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	if request.WorkspaceID == "" || request.RunID == "" || request.AuthoritySnapshotID == "" || request.PrerequisiteID == "" || strings.TrimSpace(request.OwnerModuleID) != ownerModuleID || len(request.RequiredKinds) == 0 {
		return RuntimeCapabilityAcquisition{}, fmt.Errorf("invalid runtime capability prerequisite request")
	}
	supported := make(map[string]struct{}, len(supportedKinds))
	for _, kind := range supportedKinds {
		kind = strings.TrimSpace(kind)
		if kind != "" {
			supported[kind] = struct{}{}
		}
	}
	resolved := make([]string, 0, len(request.RequiredKinds))
	seen := map[string]struct{}{}
	for _, kind := range request.RequiredKinds {
		kind = strings.TrimSpace(kind)
		if _, ok := supported[kind]; !ok {
			return RuntimeCapabilityAcquisition{}, &RuntimeCapabilityUnsupportedError{
				OwnerModuleID: ownerModuleID, CapabilityClass: request.CapabilityClass, RequiredKinds: []string{kind}, Reason: "unsupported_kind",
			}
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		resolved = append(resolved, kind)
	}
	capabilities := RuntimeCapabilities{}
	switch request.CapabilityClass {
	case RuntimeCapabilityClassStateChange:
		capabilities.StateChangeKinds = resolved
	case RuntimeCapabilityClassValidation:
		capabilities.ValidationKinds = resolved
	case RuntimeCapabilityClassGovernance:
		capabilities.GovernanceKinds = resolved
	default:
		return RuntimeCapabilityAcquisition{}, fmt.Errorf("invalid runtime capability class %q", request.CapabilityClass)
	}
	if err := ValidateRuntimeCapabilityPrerequisiteFacts(prerequisiteFacts); err != nil {
		return RuntimeCapabilityAcquisition{}, err
	}
	requestedKinds := stringSet(resolved)
	for _, fact := range NormalizeRuntimeCapabilityPrerequisiteFacts(prerequisiteFacts) {
		if fact.OwnerModuleID != ownerModuleID {
			return RuntimeCapabilityAcquisition{}, fmt.Errorf("runtime capability prerequisite fact %q has owner %q, want %q", fact.ID, fact.OwnerModuleID, ownerModuleID)
		}
		if _, requested := requestedKinds[fact.OperationKind]; requested {
			capabilities.PrerequisiteFacts = append(capabilities.PrerequisiteFacts, fact)
		}
	}
	observation := runtimeCapabilityAcquisitionObservation(request, ownerModuleID, resolved, capabilities.PrerequisiteFacts)
	return RuntimeCapabilityAcquisition{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.AuthoritySnapshotID,
		Capabilities: capabilities, Observation: observation,
	}, nil
}

// ValidateForRequest verifies the complete scoped capability fact. The
// prerequisite identity is canonical in Observation and is not repeated on
// the acquisition envelope.
func (a RuntimeCapabilityAcquisition) ValidateForRequest(request RuntimeCapabilityAcquisitionRequest) error {
	if a.WorkspaceID != request.WorkspaceID || a.RunID != request.RunID ||
		strings.TrimSpace(a.AuthoritySnapshotID) != strings.TrimSpace(request.AuthoritySnapshotID) {
		return fmt.Errorf("runtime capability acquisition scope does not match request")
	}
	observation := a.Observation.Normalized()
	if observation.PrerequisiteID != strings.TrimSpace(request.PrerequisiteID) ||
		observation.Kind != PrerequisiteKindCapability ||
		observation.OwnerModuleID != strings.TrimSpace(request.OwnerModuleID) {
		return fmt.Errorf("runtime capability acquisition observation does not match request")
	}
	if err := observation.Validate(); err != nil {
		return err
	}
	want := normalizeStrings(request.RequiredKinds)
	stateChange := normalizeStrings(a.Capabilities.StateChangeKinds)
	validation := normalizeStrings(a.Capabilities.ValidationKinds)
	governance := normalizeStrings(a.Capabilities.GovernanceKinds)
	prerequisiteFacts := NormalizeRuntimeCapabilityPrerequisiteFacts(a.Capabilities.PrerequisiteFacts)
	if err := ValidateRuntimeCapabilityPrerequisiteFacts(a.Capabilities.PrerequisiteFacts); err != nil {
		return err
	}
	var got []string
	switch request.CapabilityClass {
	case RuntimeCapabilityClassStateChange:
		got = stateChange
		if len(validation) != 0 || len(governance) != 0 {
			return fmt.Errorf("runtime capability acquisition returned capabilities outside requested class")
		}
	case RuntimeCapabilityClassValidation:
		got = validation
		if len(stateChange) != 0 || len(governance) != 0 {
			return fmt.Errorf("runtime capability acquisition returned capabilities outside requested class")
		}
	case RuntimeCapabilityClassGovernance:
		got = governance
		if len(stateChange) != 0 || len(validation) != 0 {
			return fmt.Errorf("runtime capability acquisition returned capabilities outside requested class")
		}
	default:
		return fmt.Errorf("invalid runtime capability class %q", request.CapabilityClass)
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		return fmt.Errorf("runtime capability acquisition does not match required kinds")
	}
	gotKinds := stringSet(got)
	for _, fact := range prerequisiteFacts {
		if fact.OwnerModuleID != strings.TrimSpace(request.OwnerModuleID) {
			return fmt.Errorf("runtime capability acquisition returned a prerequisite fact from another owner")
		}
		if _, ok := gotKinds[fact.OperationKind]; !ok {
			return fmt.Errorf("runtime capability acquisition returned a prerequisite fact outside requested kinds")
		}
	}
	expectedObservation := runtimeCapabilityAcquisitionObservation(request, strings.TrimSpace(request.OwnerModuleID), got, prerequisiteFacts).Normalized()
	if observation.ContentHash != expectedObservation.ContentHash || observation.ObservationID != expectedObservation.ObservationID || !sameRuntimeFreshnessDependencies(observation.Dependencies, expectedObservation.Dependencies) {
		return fmt.Errorf("runtime capability acquisition observation does not seal returned facts")
	}
	return nil
}

func runtimeCapabilityAcquisitionObservation(request RuntimeCapabilityAcquisitionRequest, ownerModuleID string, kinds []string, prerequisiteFacts []RuntimeCapabilityPrerequisiteFact) RuntimePrerequisiteObservation {
	kinds = append([]string(nil), kinds...)
	sort.Strings(kinds)
	h := sha256.New()
	writeHashString(h, ownerModuleID)
	writeHashString(h, string(request.CapabilityClass))
	writeHashStrings(h, kinds)
	for _, fact := range NormalizeRuntimeCapabilityPrerequisiteFacts(prerequisiteFacts) {
		writeHashString(h, fact.ID)
		writeHashString(h, fact.OwnerModuleID)
		writeHashString(h, fact.OperationKind)
		writeRuntimeRequirementHash(h, fact.Requirement)
	}
	digest := h.Sum(nil)
	revision := "capability-catalog-" + hex.EncodeToString(digest)[:20]
	observation := RuntimePrerequisiteObservation{
		PrerequisiteID: request.PrerequisiteID,
		Kind:           PrerequisiteKindCapability,
		OwnerModuleID:  ownerModuleID,
		ContentHash:    revision,
		Dependencies: []RuntimeFreshnessDependency{{
			Domain: RuntimeResourceOwnerCatalog, OwnerModuleID: ownerModuleID,
			SubjectRef: "runtime_capabilities", Revision: revision,
		}},
	}
	observation.ObservationID = StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func writeRuntimeRequirementHash(h hashStringWriter, requirement RuntimeRequirement) {
	requirements := normalizeRuntimeRequirements([]RuntimeRequirement{requirement})
	if len(requirements) != 1 {
		writeHashString(h, "")
		return
	}
	requirement = requirements[0]
	writeHashString(h, string(requirement.Kind))
	writeHashString(h, string(requirement.Scope))
	writeHashString(h, requirement.FocusID)
	writeHashString(h, requirement.OwnerModuleID)
	writeHashString(h, requirement.SubjectRef)
	writeHashString(h, requirement.TargetSelectorRef)
	writeHashString(h, requirement.TargetSelectorOwnerRef)
	writeHashString(h, requirement.TargetSelectorSubjectRef)
	writeHashStrings(h, requirement.TargetSelectorCandidateRefs)
	writeHashStrings(h, requirement.ProspectiveTargetAllOfConstraintRefs)
	writeHashString(h, requirement.ProspectiveTargetRef)
	writeHashString(h, string(requirement.CapabilityClass))
	writeHashStrings(h, requirement.RequiredCapabilities)
	writeHashString(h, strings.TrimSpace(requirement.CapabilityStateKind))
	writeHashString(h, fmt.Sprintf("%d", requirement.MinimumBytes))
	writeHashString(h, requirement.Reason)
}

func sameRuntimeFreshnessDependencies(left, right []RuntimeFreshnessDependency) bool {
	left = NormalizeRuntimeFreshnessDependencies(left)
	right = NormalizeRuntimeFreshnessDependencies(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
