package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

// RuntimeValidationSurfaceReadiness separates an owner's callable operation
// catalog from the validation recipes that are executable with the facts
// currently sealed into the catalog. It is a factual owner observation and
// never selects a Runtime action or state transition.
type RuntimeValidationSurfaceReadiness string

const (
	RuntimeValidationSurfaceReady                RuntimeValidationSurfaceReadiness = "ready"
	RuntimeValidationSurfacePrerequisiteRequired RuntimeValidationSurfaceReadiness = "prerequisite_required"
	RuntimeValidationSurfaceUnavailable          RuntimeValidationSurfaceReadiness = "unavailable"
)

// RuntimeValidationProofApplicability is the capability owner's declarative
// statement about whether a technically ready surface may be used as direct
// evidence for a CWI. It is independent from callability and input readiness:
// an auxiliary surface may execute successfully without being allowed to
// close direct validation coverage.
type RuntimeValidationProofApplicability string

const (
	RuntimeValidationProofDirect      RuntimeValidationProofApplicability = "direct"
	RuntimeValidationProofAuxiliary   RuntimeValidationProofApplicability = "auxiliary"
	RuntimeValidationProofNotDeclared RuntimeValidationProofApplicability = "not_declared"
)

// RuntimeValidationSurface is the owner-neutral, immutable description of one
// validation proof surface. ValidationRecipeRefs remains the provider-visible
// ready-and-direct set; this value preserves why another callable surface was
// not exposed without leaking owner implementation or choosing a retry.
type RuntimeValidationSurface struct {
	ID                 string
	OwnerModuleID      string
	OperationKind      string
	Readiness          RuntimeValidationSurfaceReadiness
	ReadinessReason    string
	InputSchemaHash    string
	InputBindingRef    string
	PrerequisiteRef    string
	CheckRefs          []string
	ProofApplicability RuntimeValidationProofApplicability
	EffectScope        RuntimeEffectScope
	IdempotencyClass   RuntimeOperationIdempotencyClass
}

func (s RuntimeValidationSurface) Normalized() RuntimeValidationSurface {
	s.ID = strings.TrimSpace(s.ID)
	s.OwnerModuleID = strings.TrimSpace(s.OwnerModuleID)
	s.OperationKind = CanonicalRuntimeBlockKind(s.OperationKind)
	s.Readiness = RuntimeValidationSurfaceReadiness(strings.TrimSpace(string(s.Readiness)))
	s.ReadinessReason = strings.TrimSpace(s.ReadinessReason)
	s.InputSchemaHash = strings.TrimSpace(s.InputSchemaHash)
	s.InputBindingRef = strings.TrimSpace(s.InputBindingRef)
	s.PrerequisiteRef = strings.TrimSpace(s.PrerequisiteRef)
	s.CheckRefs = normalizeStrings(s.CheckRefs)
	s.ProofApplicability = RuntimeValidationProofApplicability(strings.TrimSpace(string(s.ProofApplicability)))
	s.EffectScope = NormalizeRuntimeEffectScope(s.EffectScope)
	s.IdempotencyClass = NormalizeRuntimeOperationIdempotencyClass(s.IdempotencyClass)
	return s
}

func (s RuntimeValidationSurface) Clone() RuntimeValidationSurface {
	s = s.Normalized()
	s.CheckRefs = append([]string(nil), s.CheckRefs...)
	return s
}

func (s RuntimeValidationSurface) Validate() error {
	raw := s
	s = s.Normalized()
	if s.ID == "" || s.OwnerModuleID == "" || s.OperationKind == "" || s.InputSchemaHash == "" || len(s.CheckRefs) == 0 {
		return fmt.Errorf("runtime validation surface requires identity, owner, operation, input schema and checks")
	}
	descriptor, found := RuntimeOperationDescriptorForKind(s.OperationKind)
	if !found || descriptor.OwnerModuleID != s.OwnerModuleID {
		return fmt.Errorf("runtime validation surface operation %q does not belong to owner %q", s.OperationKind, s.OwnerModuleID)
	}
	if !ValidRuntimeEffectScope(RuntimeEffectScope(strings.TrimSpace(string(raw.EffectScope)))) ||
		strings.TrimSpace(string(raw.IdempotencyClass)) == "" ||
		NormalizeRuntimeOperationIdempotencyClass(raw.IdempotencyClass) != RuntimeOperationIdempotencyClass(strings.TrimSpace(string(raw.IdempotencyClass))) {
		return fmt.Errorf("runtime validation surface %q has invalid effect or idempotency", s.ID)
	}
	switch s.ProofApplicability {
	case RuntimeValidationProofDirect, RuntimeValidationProofAuxiliary, RuntimeValidationProofNotDeclared:
	default:
		return fmt.Errorf("runtime validation surface %q has invalid proof applicability %q", s.ID, s.ProofApplicability)
	}
	switch s.Readiness {
	case RuntimeValidationSurfaceReady:
		if s.InputBindingRef == "" || s.PrerequisiteRef != "" || s.ReadinessReason != "" {
			return fmt.Errorf("ready runtime validation surface %q requires only an input binding", s.ID)
		}
	case RuntimeValidationSurfacePrerequisiteRequired:
		if s.InputBindingRef != "" || s.PrerequisiteRef == "" || s.ReadinessReason == "" {
			return fmt.Errorf("prerequisite runtime validation surface %q requires only prerequisite lineage", s.ID)
		}
	case RuntimeValidationSurfaceUnavailable:
		if s.InputBindingRef != "" || s.PrerequisiteRef != "" || s.ReadinessReason == "" {
			return fmt.Errorf("unavailable runtime validation surface %q requires only a factual reason", s.ID)
		}
	default:
		return fmt.Errorf("runtime validation surface %q has unknown readiness %q", s.ID, s.Readiness)
	}
	return nil
}

func NormalizeRuntimeValidationSurfaces(values []RuntimeValidationSurface) []RuntimeValidationSurface {
	byID := make(map[string]RuntimeValidationSurface, len(values))
	for _, raw := range values {
		value := raw.Clone()
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ret := make([]RuntimeValidationSurface, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, byID[id])
	}
	return nilIfEmpty(ret)
}

// RuntimeReadyValidationRecipeRefs reports technical readiness for owner audit
// and diagnostics. Provider selection must use
// RuntimeSelectableValidationRecipeRefs instead.
func RuntimeReadyValidationRecipeRefs(values []RuntimeValidationSurface) []string {
	ret := make([]string, 0, len(values))
	for _, value := range NormalizeRuntimeValidationSurfaces(values) {
		if value.Readiness == RuntimeValidationSurfaceReady {
			ret = append(ret, value.ID)
		}
	}
	return normalizeStrings(ret)
}

// RuntimeSelectableValidationRecipeRefs returns the exact provider-selectable
// proof surface. A successful/ready auxiliary, durable, or replay-unknown probe
// remains audit evidence but cannot be promoted into a semantic recipe or
// receive direct CWI credit. At-most-once validation remains selectable, but an
// ambiguous claimed execution must converge as outcome_unknown, never replay.
func RuntimeSelectableValidationRecipeRefs(values []RuntimeValidationSurface) []string {
	ret := make([]string, 0, len(values))
	for _, value := range NormalizeRuntimeValidationSurfaces(values) {
		if value.Readiness == RuntimeValidationSurfaceReady && value.ProofApplicability == RuntimeValidationProofDirect &&
			RuntimeEffectIsNonDurable(value.EffectScope) && runtimeValidationSurfaceHasExplicitReplayPolicy(value.IdempotencyClass) {
			ret = append(ret, value.ID)
		}
	}
	return normalizeStrings(ret)
}

func runtimeValidationSurfaceHasExplicitReplayPolicy(value RuntimeOperationIdempotencyClass) bool {
	switch NormalizeRuntimeOperationIdempotencyClass(value) {
	case RuntimeOperationIdempotencySafeToReplay, RuntimeOperationIdempotencyAtMostOnce, RuntimeOperationIdempotencyOwnerManaged:
		return true
	default:
		return false
	}
}

func RuntimeValidationSurfaceForID(values []RuntimeValidationSurface, id string) (RuntimeValidationSurface, bool) {
	id = strings.TrimSpace(id)
	for _, value := range NormalizeRuntimeValidationSurfaces(values) {
		if value.ID == id {
			return value.Clone(), true
		}
	}
	return RuntimeValidationSurface{}, false
}
