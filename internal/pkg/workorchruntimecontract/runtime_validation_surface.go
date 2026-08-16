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

// RuntimeValidationSurface is the owner-neutral, immutable description of one
// validation proof surface. ValidationRecipeRefs remains the provider-visible
// ready-only set; this value preserves why another callable surface was not
// exposed without leaking owner implementation or choosing a retry.
type RuntimeValidationSurface struct {
	ID               string
	OwnerModuleID    string
	OperationKind    string
	Readiness        RuntimeValidationSurfaceReadiness
	ReadinessReason  string
	InputSchemaHash  string
	InputBindingRef  string
	PrerequisiteRef  string
	CheckRefs        []string
	EffectScope      RuntimeEffectScope
	IdempotencyClass RuntimeOperationIdempotencyClass
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

func RuntimeReadyValidationRecipeRefs(values []RuntimeValidationSurface) []string {
	ret := make([]string, 0, len(values))
	for _, value := range NormalizeRuntimeValidationSurfaces(values) {
		if value.Readiness == RuntimeValidationSurfaceReady {
			ret = append(ret, value.ID)
		}
	}
	return normalizeStrings(ret)
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
