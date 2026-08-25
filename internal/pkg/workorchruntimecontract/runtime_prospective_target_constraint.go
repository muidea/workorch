package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
)

const (
	RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent = "workorch.runtime.prospective-target-satisfaction-constraint.v1"
	RuntimeProspectiveTargetSatisfactionConstraintHashPrefix    = "runtime-prospective-target-satisfaction-constraint-"
)

// RuntimeProspectiveTargetPredicateKind is a closed, owner-neutral predicate
// vocabulary over one prospective workspace-relative target. Predicates are
// facts authored by the owner that can judge the resulting capability input;
// they never select an exact target or a Runtime transition.
type RuntimeProspectiveTargetPredicateKind string

const (
	RuntimeProspectiveTargetPredicatePathSuffix          RuntimeProspectiveTargetPredicateKind = "target_ref_path_suffix"
	RuntimeProspectiveTargetPredicatePathPrefix          RuntimeProspectiveTargetPredicateKind = "target_ref_path_prefix"
	RuntimeProspectiveTargetPredicatePathSegmentNotEqual RuntimeProspectiveTargetPredicateKind = "target_ref_path_segment_not_equals"
)

// RuntimeProspectiveTargetPredicate is one conjunct in an owner-authored
// prospective target contract. There is intentionally no aggregation field:
// every predicate is interpreted as part of the enclosing AllOf set.
type RuntimeProspectiveTargetPredicate struct {
	Kind  RuntimeProspectiveTargetPredicateKind
	Value string
}

func (p RuntimeProspectiveTargetPredicate) Normalized() RuntimeProspectiveTargetPredicate {
	p.Kind = RuntimeProspectiveTargetPredicateKind(strings.TrimSpace(string(p.Kind)))
	p.Value = strings.TrimSpace(strings.ReplaceAll(p.Value, "\\", "/"))
	switch p.Kind {
	case RuntimeProspectiveTargetPredicatePathPrefix:
		p.Value = strings.TrimPrefix(p.Value, "./")
		p.Value = strings.Trim(p.Value, "/")
		if p.Value != "" {
			p.Value += "/"
		}
	case RuntimeProspectiveTargetPredicatePathSegmentNotEqual:
		p.Value = strings.Trim(p.Value, "/")
	}
	return p
}

func (p RuntimeProspectiveTargetPredicate) Validate() error {
	p = p.Normalized()
	if p.Value == "" {
		return fmt.Errorf("runtime prospective target predicate requires a value")
	}
	switch p.Kind {
	case RuntimeProspectiveTargetPredicatePathSuffix:
		return nil
	case RuntimeProspectiveTargetPredicatePathPrefix:
		if p.Value == "/" || p.Value == "../" || strings.Contains(p.Value, "/../") || strings.HasPrefix(p.Value, "../") || strings.HasPrefix(p.Value, "./") {
			return fmt.Errorf("runtime prospective target path prefix %q is not workspace-relative", p.Value)
		}
		return nil
	case RuntimeProspectiveTargetPredicatePathSegmentNotEqual:
		if strings.Contains(p.Value, "/") || p.Value == "." {
			return fmt.Errorf("runtime prospective target excluded segment %q is invalid", p.Value)
		}
		return nil
	default:
		return fmt.Errorf("runtime prospective target predicate has unknown kind %q", p.Kind)
	}
}

// RuntimeProspectiveTargetSatisfactionConstraint is one immutable all-of-only
// owner fact. It binds generic target predicates to the exact validation
// surface, activation-sealed recipe schema and owner catalog revision that
// authored them. It contains no exact target, retry, action or state.
type RuntimeProspectiveTargetSatisfactionConstraint struct {
	SchemaVersion                      string
	OwnerModuleID                      string
	ProspectiveSatisfactionContractRef string
	ValidationSurfaceID                string
	ValidationRecipeID                 string
	InputSchemaHash                    string
	CatalogObservedRevision            string
	AllOf                              []RuntimeProspectiveTargetPredicate
}

func (c RuntimeProspectiveTargetSatisfactionConstraint) Normalized() RuntimeProspectiveTargetSatisfactionConstraint {
	c.SchemaVersion = strings.TrimSpace(c.SchemaVersion)
	c.OwnerModuleID = strings.TrimSpace(c.OwnerModuleID)
	c.ProspectiveSatisfactionContractRef = strings.TrimSpace(c.ProspectiveSatisfactionContractRef)
	c.ValidationSurfaceID = strings.TrimSpace(c.ValidationSurfaceID)
	c.ValidationRecipeID = strings.TrimSpace(c.ValidationRecipeID)
	c.InputSchemaHash = strings.TrimSpace(c.InputSchemaHash)
	c.CatalogObservedRevision = strings.TrimSpace(c.CatalogObservedRevision)
	values := make([]RuntimeProspectiveTargetPredicate, 0, len(c.AllOf))
	for _, raw := range c.AllOf {
		values = append(values, raw.Normalized())
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Kind != values[j].Kind {
			return values[i].Kind < values[j].Kind
		}
		return values[i].Value < values[j].Value
	})
	c.AllOf = nilIfEmpty(values)
	return c
}

func (c RuntimeProspectiveTargetSatisfactionConstraint) Clone() RuntimeProspectiveTargetSatisfactionConstraint {
	c = c.Normalized()
	c.AllOf = append([]RuntimeProspectiveTargetPredicate(nil), c.AllOf...)
	return c
}

func (c RuntimeProspectiveTargetSatisfactionConstraint) Validate() error {
	c = c.Normalized()
	if c.SchemaVersion != RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent || c.OwnerModuleID == "" ||
		c.ProspectiveSatisfactionContractRef == "" || c.ValidationSurfaceID == "" || c.ValidationRecipeID == "" ||
		c.InputSchemaHash == "" || c.CatalogObservedRevision == "" || len(c.AllOf) == 0 {
		return fmt.Errorf("runtime prospective target satisfaction constraint requires current schema, owner, contract, surface, recipe, input schema, catalog revision and all-of predicates")
	}
	seen := make(map[string]struct{}, len(c.AllOf))
	for _, predicate := range c.AllOf {
		if err := predicate.Validate(); err != nil {
			return err
		}
		key := string(predicate.Kind) + "\x00" + predicate.Value
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("runtime prospective target satisfaction constraint duplicates predicate %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func StableRuntimeProspectiveTargetSatisfactionConstraintHash(value RuntimeProspectiveTargetSatisfactionConstraint) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent)
	writeHashString(h, value.OwnerModuleID)
	writeHashString(h, value.ProspectiveSatisfactionContractRef)
	writeHashString(h, value.ValidationSurfaceID)
	writeHashString(h, value.ValidationRecipeID)
	writeHashString(h, value.InputSchemaHash)
	writeHashString(h, value.CatalogObservedRevision)
	for _, predicate := range value.AllOf {
		writeHashString(h, string(predicate.Kind))
		writeHashString(h, predicate.Value)
	}
	return RuntimeProspectiveTargetSatisfactionConstraintHashPrefix + hex.EncodeToString(h.Sum(nil))[:20]
}

// RuntimeProspectiveTargetSatisfiesConstraint evaluates the immutable all-of
// fact without I/O or owner-specific semantics. A true result means every
// published predicate matches; transition and retry authority remains outside
// this pure helper.
func RuntimeProspectiveTargetSatisfiesConstraint(targetRef string, constraint RuntimeProspectiveTargetSatisfactionConstraint) bool {
	if err := constraint.Validate(); err != nil {
		return false
	}
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" || strings.Contains(targetRef, "\\") || strings.HasPrefix(targetRef, "/") {
		return false
	}
	for _, segment := range strings.Split(targetRef, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	clean := path.Clean(targetRef)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return false
	}
	for _, predicate := range constraint.Normalized().AllOf {
		switch predicate.Kind {
		case RuntimeProspectiveTargetPredicatePathSuffix:
			if !strings.HasSuffix(clean, predicate.Value) {
				return false
			}
		case RuntimeProspectiveTargetPredicatePathPrefix:
			if !strings.HasPrefix(clean, predicate.Value) {
				return false
			}
		case RuntimeProspectiveTargetPredicatePathSegmentNotEqual:
			for _, segment := range strings.Split(clean, "/") {
				if segment == predicate.Value {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}

// ResolveRuntimeProspectiveTargetSatisfactionConstraint extracts the one
// all-of contract that can satisfy a missing required-owner fact. More than
// one matching surface would be an implicit any-of and is rejected.
func ResolveRuntimeProspectiveTargetSatisfactionConstraint(catalog RuntimeOperationAdmissibility, requirement RuntimeOwnerFactRequirement, contractRef string) (RuntimeProspectiveTargetSatisfactionConstraint, error) {
	catalog = catalog.Normalized()
	requirement = requirement.Normalized()
	contractRef = strings.TrimSpace(contractRef)
	if err := catalog.Validate(); err != nil {
		return RuntimeProspectiveTargetSatisfactionConstraint{}, err
	}
	if err := requirement.Validate(); err != nil {
		return RuntimeProspectiveTargetSatisfactionConstraint{}, err
	}
	if contractRef == "" || catalog.OwnerModuleID != requirement.OwnerModuleID {
		return RuntimeProspectiveTargetSatisfactionConstraint{}, fmt.Errorf("runtime prospective target satisfaction constraint does not match required owner catalog")
	}
	var matched []RuntimeProspectiveTargetSatisfactionConstraint
	for _, surface := range catalog.ValidationSurfaces {
		surface = surface.Normalized()
		if surface.OwnerModuleID != requirement.OwnerModuleID || surface.OperationKind != requirement.OperationKind ||
			surface.Readiness != RuntimeValidationSurfacePrerequisiteRequired || surface.ProspectiveSatisfactionContractRef != contractRef ||
			surface.ProofApplicability != RuntimeValidationProofDirect || !RuntimeEffectIsNonDurable(surface.EffectScope) ||
			!runtimeValidationSurfaceHasExplicitReplayPolicy(surface.IdempotencyClass) || surface.ProspectiveTargetSatisfactionConstraint == nil {
			continue
		}
		matched = append(matched, surface.ProspectiveTargetSatisfactionConstraint.Clone())
	}
	if len(matched) != 1 {
		return RuntimeProspectiveTargetSatisfactionConstraint{}, fmt.Errorf("runtime prospective target satisfaction contract %q requires exactly one all-of surface, got %d", contractRef, len(matched))
	}
	return matched[0].Clone(), nil
}
