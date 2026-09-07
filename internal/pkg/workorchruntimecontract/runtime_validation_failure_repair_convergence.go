package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// RuntimeValidationFailureRepairConvergence is one immutable, Profile-owned
// declaration for recovering a rejected validation proof. Its semantics are
// fixed and conjunctive:
//
//	typed validation rejection -> state-change repair -> fresh validation
//
// Routes is a deterministic lookup table, not an alternative terminal
// relation. Every route has one exact trigger tuple and one repair surface;
// duplicate trigger tuples are rejected at pre-seal admission.
type RuntimeValidationFailureRepairConvergence struct {
	ID                     string
	Routes                 []RuntimeValidationFailureRepairRoute
	RequireFreshValidation bool
	// MaxAttempts bounds matching rejection-to-repair-to-revalidation cycles
	// for one CWI. Cycle one includes the rejection which opens the first repair;
	// it is not an additional charge on top of that repair cycle.
	MaxAttempts int
}

// RuntimeValidationFailureRepairRoute binds one exact owner diagnostic to one
// owner-backed state-change repair surface. Diagnostic prose, stdout/stderr and
// target guesses never participate in route selection.
type RuntimeValidationFailureRepairRoute struct {
	RequireInputAddition       bool `json:",omitempty"`
	ID                         string
	TriggerOwnerModuleID       string
	TriggerOperationKind       string
	TriggerDiagnosticClass     string
	TriggerEvidenceTargetType  string
	TriggerOwnerDiagnosticCode string

	RepairEvidenceKind   string
	RepairOwnerModuleID  string
	RepairOperationKinds []string
	RepairTargetPolicy   RuntimeOperationTargetPolicy
	RepairTargetRole     string
	// RepairDeclarationTargetSelector* is the Profile-owned semantic target
	// declaration. RepairOperationTargetSelectorRef is the capability owner's
	// concrete operation authorization. They are independent identities and are
	// never treated as aliases.
	RepairDeclarationTargetSelectorRef           string
	RepairDeclarationTargetSelectorOwnerRef      string
	RepairDeclarationTargetSelectorSubjectRef    string
	RepairDeclarationTargetSelectorCandidateRefs []string
	RepairOperationTargetSelectorRef             string
}

func (c RuntimeValidationFailureRepairConvergence) Normalized() RuntimeValidationFailureRepairConvergence {
	c.ID = strings.TrimSpace(c.ID)
	routes := make([]RuntimeValidationFailureRepairRoute, len(c.Routes))
	for index, raw := range c.Routes {
		routes[index] = raw.Normalized()
	}
	// Routes are an authority declaration, so normalization must never erase an
	// empty item or collapse duplicate identities before Validate can reject the
	// original pre-seal shape. Valid routes have unique IDs, which makes this
	// ordering deterministic; invalid equal-ID entries retain their source order
	// only until the mandatory validation fence rejects them.
	sort.SliceStable(routes, func(i, j int) bool { return routes[i].ID < routes[j].ID })
	c.Routes = nilIfEmpty(routes)
	return c
}

func (c RuntimeValidationFailureRepairConvergence) Clone() RuntimeValidationFailureRepairConvergence {
	c = c.Normalized()
	c.Routes = cloneRuntimeValidationFailureRepairRoutes(c.Routes)
	return c
}

func (c RuntimeValidationFailureRepairConvergence) Validate() error {
	c = c.Normalized()
	if c.ID == "" || len(c.Routes) == 0 {
		return fmt.Errorf("validation failure repair convergence requires identity and unique exact routes")
	}
	seenIDs := make(map[string]struct{}, len(c.Routes))
	seenTriggers := make(map[string]string, len(c.Routes))
	for _, route := range c.Routes {
		if route.ID == "" {
			return fmt.Errorf("validation failure repair convergence %q requires non-empty unique route identities", c.ID)
		}
		if _, duplicate := seenIDs[route.ID]; duplicate {
			return fmt.Errorf("validation failure repair convergence %q has duplicate route identity %q", c.ID, route.ID)
		}
		seenIDs[route.ID] = struct{}{}
		if err := route.Validate(); err != nil {
			return fmt.Errorf("validation failure repair convergence %q route %q: %w", c.ID, route.ID, err)
		}
		key := route.TriggerKey()
		if previous, found := seenTriggers[key]; found {
			return fmt.Errorf("validation failure repair convergence %q routes %q and %q duplicate exact trigger", c.ID, previous, route.ID)
		}
		seenTriggers[key] = route.ID
	}
	if !c.RequireFreshValidation {
		return fmt.Errorf("validation failure repair convergence %q must require fresh validation after repair", c.ID)
	}
	if c.MaxAttempts < 1 || c.MaxAttempts > MaxRepairAttemptsPerFocusLimit {
		return fmt.Errorf("validation failure repair convergence %q max attempts must be in 1..%d", c.ID, MaxRepairAttemptsPerFocusLimit)
	}
	return nil
}

func (r RuntimeValidationFailureRepairRoute) Normalized() RuntimeValidationFailureRepairRoute {
	r.ID = strings.TrimSpace(r.ID)
	r.TriggerOwnerModuleID = strings.TrimSpace(r.TriggerOwnerModuleID)
	r.TriggerOperationKind = CanonicalRuntimeBlockKind(r.TriggerOperationKind)
	r.TriggerDiagnosticClass = strings.TrimSpace(r.TriggerDiagnosticClass)
	r.TriggerEvidenceTargetType = strings.TrimSpace(r.TriggerEvidenceTargetType)
	r.TriggerOwnerDiagnosticCode = strings.TrimSpace(r.TriggerOwnerDiagnosticCode)
	r.RepairEvidenceKind = strings.TrimSpace(r.RepairEvidenceKind)
	r.RepairOwnerModuleID = strings.TrimSpace(r.RepairOwnerModuleID)
	r.RepairOperationKinds = canonicalizeOperationKinds(r.RepairOperationKinds)
	r.RepairTargetPolicy = RuntimeOperationTargetPolicy(strings.TrimSpace(string(r.RepairTargetPolicy)))
	r.RepairTargetRole = strings.TrimSpace(r.RepairTargetRole)
	r.RepairDeclarationTargetSelectorRef = strings.TrimSpace(r.RepairDeclarationTargetSelectorRef)
	r.RepairDeclarationTargetSelectorOwnerRef = strings.TrimSpace(r.RepairDeclarationTargetSelectorOwnerRef)
	r.RepairDeclarationTargetSelectorSubjectRef = strings.TrimSpace(r.RepairDeclarationTargetSelectorSubjectRef)
	r.RepairDeclarationTargetSelectorCandidateRefs = normalizeStrings(r.RepairDeclarationTargetSelectorCandidateRefs)
	r.RepairOperationTargetSelectorRef = strings.TrimSpace(r.RepairOperationTargetSelectorRef)
	return r
}

func (r RuntimeValidationFailureRepairRoute) Clone() RuntimeValidationFailureRepairRoute {
	r = r.Normalized()
	r.RepairOperationKinds = append([]string(nil), r.RepairOperationKinds...)
	r.RepairDeclarationTargetSelectorCandidateRefs = append([]string(nil), r.RepairDeclarationTargetSelectorCandidateRefs...)
	return r
}

func (r RuntimeValidationFailureRepairRoute) Validate() error {
	r = r.Normalized()
	if r.ID == "" || r.TriggerOwnerModuleID == "" || r.TriggerOperationKind == "" ||
		r.TriggerDiagnosticClass == "" || r.TriggerEvidenceTargetType == "" || r.TriggerOwnerDiagnosticCode == "" {
		return fmt.Errorf("requires identity and exact typed trigger owner, operation, diagnostic class, evidence target type and owner code")
	}
	trigger, found := RuntimeOperationDescriptorForKind(r.TriggerOperationKind)
	if !found || trigger.OwnerModuleID != r.TriggerOwnerModuleID || !RuntimeOperationKindSupportsCoverageEvidence(r.TriggerOperationKind, EvidenceKindValidation) {
		return fmt.Errorf("trigger operation %q is not validation proof owned by %q", r.TriggerOperationKind, r.TriggerOwnerModuleID)
	}
	if !runtimeValidationDiagnosticClassKnown(r.TriggerDiagnosticClass) {
		return fmt.Errorf("has unknown diagnostic class %q", r.TriggerDiagnosticClass)
	}
	if !runtimeValidationEvidenceTargetTypeKnown(r.TriggerEvidenceTargetType) {
		return fmt.Errorf("has unknown evidence target type %q", r.TriggerEvidenceTargetType)
	}
	if r.RepairEvidenceKind != EvidenceKindStateChange || r.RepairOwnerModuleID == "" || len(r.RepairOperationKinds) == 0 {
		return fmt.Errorf("requires owner-backed state_change repair operations")
	}
	for _, kind := range r.RepairOperationKinds {
		descriptor, found := RuntimeOperationDescriptorForKind(kind)
		if !found || descriptor.OwnerModuleID != r.RepairOwnerModuleID || !RuntimeOperationKindSupportsCoverageEvidence(kind, EvidenceKindStateChange) {
			return fmt.Errorf("operation %q is not state-change proof owned by %q", kind, r.RepairOwnerModuleID)
		}
	}
	if _, ok := RuntimeRepairSurfaceConfirmationProtocolForOperationKinds(r.RepairOwnerModuleID, r.RepairOperationKinds); !ok {
		return fmt.Errorf("repair owner %q operations do not publish one shared repair-surface confirmation protocol", r.RepairOwnerModuleID)
	}
	if r.RequireInputAddition && r.RepairTargetPolicy != RuntimeOperationTargetPolicyProspectiveOwnerTarget {
		return fmt.Errorf("validation input addition requires create-only prospective repair")
	}
	if r.RepairTargetRole == "" || r.RepairDeclarationTargetSelectorRef == "" || r.RepairDeclarationTargetSelectorOwnerRef == "" || r.RepairDeclarationTargetSelectorSubjectRef == "" || r.RepairOperationTargetSelectorRef == "" {
		return fmt.Errorf("requires an independent repair target role and complete selector identity")
	}
	if r.RepairDeclarationTargetSelectorRef == r.RepairOperationTargetSelectorRef {
		return fmt.Errorf("requires distinct declaration and operation selector identities")
	}
	switch r.RepairTargetPolicy {
	case RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		// Both policies are selector-backed. Exact targets are confirmed after seal.
	default:
		return fmt.Errorf("requires a selector-backed repair target policy, got %q", r.RepairTargetPolicy)
	}
	return nil
}

// RuntimeRepairSurfaceConfirmationProtocolForOperationKinds returns the one
// concrete owner protocol shared by every operation in a repair route. It is a
// pure descriptor lookup: it neither selects a Runtime action nor dispatches a
// cross-owner command.
func RuntimeRepairSurfaceConfirmationProtocolForOperationKinds(ownerModuleID string, operationKinds []string) (RuntimeRepairSurfaceConfirmationProtocol, bool) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	kinds := canonicalizeOperationKinds(operationKinds)
	if ownerModuleID == "" || len(kinds) == 0 {
		return RuntimeRepairSurfaceConfirmationProtocolNone, false
	}
	protocol := RuntimeRepairSurfaceConfirmationProtocolNone
	for _, kind := range kinds {
		descriptor, found := RuntimeOperationDescriptorForKind(kind)
		if !found || descriptor.OwnerModuleID != ownerModuleID ||
			descriptor.RepairSurfaceConfirmationProtocol == RuntimeRepairSurfaceConfirmationProtocolNone ||
			!RuntimeRepairSurfaceConfirmationProtocolSupported(descriptor.RepairSurfaceConfirmationProtocol) {
			return RuntimeRepairSurfaceConfirmationProtocolNone, false
		}
		if protocol != RuntimeRepairSurfaceConfirmationProtocolNone && protocol != descriptor.RepairSurfaceConfirmationProtocol {
			return RuntimeRepairSurfaceConfirmationProtocolNone, false
		}
		protocol = descriptor.RepairSurfaceConfirmationProtocol
	}
	return protocol, protocol != RuntimeRepairSurfaceConfirmationProtocolNone
}

func (r RuntimeValidationFailureRepairRoute) TriggerKey() string {
	r = r.Normalized()
	return strings.Join([]string{r.TriggerOwnerModuleID, r.TriggerOperationKind, r.TriggerDiagnosticClass, r.TriggerEvidenceTargetType, r.TriggerOwnerDiagnosticCode}, "|")
}

func (r RuntimeValidationFailureRepairRoute) MatchesTrigger(ownerModuleID, operationKind, diagnosticClass, evidenceTargetType, ownerDiagnosticCode string) bool {
	r = r.Normalized()
	if r.Validate() != nil {
		return false
	}
	return r.TriggerOwnerModuleID == strings.TrimSpace(ownerModuleID) &&
		r.TriggerOperationKind == CanonicalRuntimeBlockKind(operationKind) &&
		r.TriggerDiagnosticClass == strings.TrimSpace(diagnosticClass) &&
		r.TriggerEvidenceTargetType == strings.TrimSpace(evidenceTargetType) &&
		r.TriggerOwnerDiagnosticCode == strings.TrimSpace(ownerDiagnosticCode)
}

// RequiresStructuredObservation reports whether the route can only repair an
// already-existing owner resource. Such a route cannot ask a semantic provider
// to invent or select a target: the validation owner must publish bounded
// candidates and the resource owner must confirm them first.
func (r RuntimeValidationFailureRepairRoute) RequiresStructuredObservation() bool {
	r = r.Normalized()
	return r.RepairTargetPolicy == RuntimeOperationTargetPolicyExistingOwnerScope
}

// RouteForTrigger returns the unique exact route. Validate rejects duplicate
// keys, so callers never choose between alternatives.
func (c RuntimeValidationFailureRepairConvergence) RouteForTrigger(ownerModuleID, operationKind, diagnosticClass, evidenceTargetType, ownerDiagnosticCode string) (RuntimeValidationFailureRepairRoute, bool) {
	c = c.Normalized()
	if c.Validate() != nil {
		return RuntimeValidationFailureRepairRoute{}, false
	}
	for _, route := range c.Routes {
		if route.MatchesTrigger(ownerModuleID, operationKind, diagnosticClass, evidenceTargetType, ownerDiagnosticCode) {
			return route.Clone(), true
		}
	}
	return RuntimeValidationFailureRepairRoute{}, false
}

func StableRuntimeValidationFailureRepairConvergenceHash(value RuntimeValidationFailureRepairConvergence) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-validation-failure-repair-convergence-v2")
	writeHashString(h, value.ID)
	for _, route := range value.Routes {
		writeHashString(h, StableRuntimeValidationFailureRepairRouteHash(route))
	}
	writeHashString(h, fmt.Sprintf("%t", value.RequireFreshValidation))
	writeHashString(h, fmt.Sprintf("%d", value.MaxAttempts))
	return "runtime-validation-failure-repair-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func StableRuntimeValidationFailureRepairRouteHash(value RuntimeValidationFailureRepairRoute) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-validation-failure-repair-route")
	writeHashString(h, value.ID)
	writeHashString(h, value.TriggerOwnerModuleID)
	writeHashString(h, value.TriggerOperationKind)
	writeHashString(h, value.TriggerDiagnosticClass)
	writeHashString(h, value.TriggerEvidenceTargetType)
	writeHashString(h, value.TriggerOwnerDiagnosticCode)
	writeHashString(h, value.RepairEvidenceKind)
	writeHashString(h, value.RepairOwnerModuleID)
	writeHashStrings(h, value.RepairOperationKinds)
	writeHashString(h, string(value.RepairTargetPolicy))
	writeHashString(h, value.RepairTargetRole)
	writeHashString(h, value.RepairDeclarationTargetSelectorRef)
	writeHashString(h, value.RepairDeclarationTargetSelectorOwnerRef)
	writeHashString(h, value.RepairDeclarationTargetSelectorSubjectRef)
	writeHashStrings(h, value.RepairDeclarationTargetSelectorCandidateRefs)
	writeHashString(h, value.RepairOperationTargetSelectorRef)
	if value.RequireInputAddition {
		writeHashString(h, "require-validation-input-addition/v1")
	}
	return "runtime-validation-failure-repair-route-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func SameRuntimeValidationFailureRepairConvergence(left, right *RuntimeValidationFailureRepairConvergence) bool {
	if left == nil || right == nil {
		return left == right
	}
	return StableRuntimeValidationFailureRepairConvergenceHash(*left) == StableRuntimeValidationFailureRepairConvergenceHash(*right)
}

func cloneRuntimeValidationFailureRepairConvergence(value *RuntimeValidationFailureRepairConvergence) *RuntimeValidationFailureRepairConvergence {
	if value == nil {
		return nil
	}
	clone := value.Clone()
	return &clone
}

func cloneRuntimeValidationFailureRepairRoutes(values []RuntimeValidationFailureRepairRoute) []RuntimeValidationFailureRepairRoute {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeValidationFailureRepairRoute, len(values))
	for index, value := range values {
		ret[index] = value.Clone()
	}
	return ret
}

func runtimeValidationDiagnosticClassKnown(value string) bool {
	switch strings.TrimSpace(value) {
	case ValidationDiagnosticCommandInvalid,
		ValidationDiagnosticEnvironmentMissing,
		ValidationDiagnosticDependencyMismatch,
		ValidationDiagnosticToolchainMismatch,
		ValidationDiagnosticBuildFailed,
		ValidationDiagnosticTestFailed,
		ValidationDiagnosticTimeout,
		ValidationDiagnosticOutputInconclusive,
		ValidationDiagnosticEvidenceBindingMissing,
		ValidationDiagnosticContractViolation,
		ValidationDiagnosticEvidenceMetadataMissing,
		ValidationDiagnosticScratchRequired,
		ValidationDiagnosticScratchEscape,
		ValidationDiagnosticProcessControlForbidden,
		ValidationDiagnosticDetachedProcessForbidden,
		ValidationDiagnosticShellSafetyUnproven,
		ValidationDiagnosticManagedValidationRequired,
		ValidationDiagnosticImplementationGap,
		ValidationDiagnosticExternalDependency,
		ValidationDiagnosticProviderTransient,
		ValidationDiagnosticRuntimeMisalignment:
		return true
	default:
		return false
	}
}

func runtimeValidationEvidenceTargetTypeKnown(value string) bool {
	switch strings.TrimSpace(value) {
	case ValidationEvidenceTargetCommandContract,
		ValidationEvidenceTargetEnvironment,
		ValidationEvidenceTargetDependencyContract,
		ValidationEvidenceTargetToolchainContract,
		ValidationEvidenceTargetValidationOutput,
		ValidationEvidenceTargetBehaviorEvidence,
		ValidationEvidenceTargetCoverageBinding,
		ValidationEvidenceTargetFailureSurface:
		return true
	default:
		return false
	}
}
