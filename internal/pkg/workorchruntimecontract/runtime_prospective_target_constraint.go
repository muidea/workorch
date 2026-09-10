package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent = "workorch.runtime.prospective-target-satisfaction-constraint.v2"
	RuntimeProspectiveTargetSatisfactionConstraintHashPrefix    = "runtime-prospective-target-satisfaction-constraint-"
	// RuntimeProspectiveTargetRefsMaxCount is the shared provider, admission,
	// delta and owner-satisfaction bound for one prospective proposal.
	RuntimeProspectiveTargetRefsMaxCount                = 8
	RuntimeProspectiveTargetAllOfConstraintRefsMaxCount = 8
)

// NormalizeRuntimeProspectiveTargetAllOfConstraintRefs canonicalizes the
// immutable pre-seal constraint declaration. Every returned ref is mandatory:
// this contract deliberately has no one-of/any-of interpretation.
func NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(values []string) []string {
	return normalizeStrings(values)
}

// ValidateRuntimeProspectiveTargetAllOfConstraintRefs keeps the pre-seal
// declaration finite. Capability owners still decide whether they support the
// exact all-of set; this pure validation never interprets an owner constraint.
func ValidateRuntimeProspectiveTargetAllOfConstraintRefs(values []string) error {
	values = NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(values)
	if len(values) > RuntimeProspectiveTargetAllOfConstraintRefsMaxCount {
		return fmt.Errorf("runtime prospective target all-of declaration exceeds %d constraints", RuntimeProspectiveTargetAllOfConstraintRefsMaxCount)
	}
	return nil
}

// SameRuntimeProspectiveTargetAllOfConstraintRefs compares the canonical
// mandatory constraint set carried across Definition, prerequisite and owner
// observation contracts.
func SameRuntimeProspectiveTargetAllOfConstraintRefs(left, right []string) bool {
	return sameStrings(
		NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(left),
		NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(right),
	)
}

// RuntimeProspectiveTargetPredicateKind is a closed, owner-neutral predicate
// vocabulary over one prospective workspace-relative target. Predicates are
// facts authored by the owner that can judge the resulting capability input;
// they never select an exact target or a Runtime transition.
type RuntimeProspectiveTargetPredicateKind string

const (
	RuntimeProspectiveTargetPredicatePathSuffix          RuntimeProspectiveTargetPredicateKind = "target_ref_path_suffix"
	RuntimeProspectiveTargetPredicatePathPrefix          RuntimeProspectiveTargetPredicateKind = "target_ref_path_prefix"
	RuntimeProspectiveTargetPredicatePathExact           RuntimeProspectiveTargetPredicateKind = "target_ref_path_exact"
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
	case RuntimeProspectiveTargetPredicatePathExact:
		p.Value = strings.TrimPrefix(p.Value, "./")
		p.Value = path.Clean(p.Value)
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
	case RuntimeProspectiveTargetPredicatePathExact:
		if !runtimeProspectiveTargetRefValid(p.Value) {
			return fmt.Errorf("runtime prospective target exact path %q is not workspace-relative", p.Value)
		}
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

// RuntimeProspectiveTargetClause requires at least one member of the bounded
// proposal target set to satisfy every predicate in AllOf. All clauses in the
// enclosing constraint are mandatory; there is no alternative aggregation.
type RuntimeProspectiveTargetClause struct {
	ID    string
	AllOf []RuntimeProspectiveTargetPredicate
}

func (c RuntimeProspectiveTargetClause) Normalized() RuntimeProspectiveTargetClause {
	c.ID = strings.TrimSpace(c.ID)
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

func (c RuntimeProspectiveTargetClause) Clone() RuntimeProspectiveTargetClause {
	// Normalized allocates and canonicalizes the complete predicate slice, so a
	// second copy would only repeat the same deep traversal.
	return c.Normalized()
}

func (c RuntimeProspectiveTargetClause) Validate() error {
	c = c.Normalized()
	if c.ID == "" || len(c.AllOf) == 0 {
		return fmt.Errorf("runtime prospective target clause requires an id and all-of predicates")
	}
	seen := make(map[string]struct{}, len(c.AllOf))
	for _, predicate := range c.AllOf {
		if err := predicate.Validate(); err != nil {
			return err
		}
		key := string(predicate.Kind) + "\x00" + predicate.Value
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("runtime prospective target clause %q duplicates predicate %q", c.ID, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// RuntimeProspectiveTargetSatisfactionConstraint is one immutable all-of-only
// owner fact. It binds generic target predicates to the exact validation
// surface, owner-resolved recipe schema and owner catalog revision that
// authored them. An exact-path predicate may state an owner-required input
// identity (for example a root manifest), but the contract contains no
// proposal-selected target, retry, action or state.
type RuntimeProspectiveTargetSatisfactionConstraint struct {
	SchemaVersion                      string
	OwnerModuleID                      string
	ProspectiveSatisfactionContractRef string
	ValidationSurfaceID                string
	ValidationRecipeID                 string
	InputSchemaHash                    string
	CatalogObservedRevision            string
	AllOf                              []RuntimeProspectiveTargetClause
}

type RuntimeValidationProofPreparation struct {
	RequirementHash    string
	DefinitionRevision string
	CoverageWorkItemID common.CoverageWorkItemID
	DeclarationRef     string
	ValidationRecipeID string
	TargetRef          string
	CheckAttestations  []RuntimeValidationCheckAttestationBinding
	// Optional owner-authored content syntax. Core preserves it in the catalog
	// and provider projection; it is neither mutation nor acceptance authority.
	ContentContractRef  string `json:",omitempty"`
	ContentRequirements string `json:",omitempty"`
	// OriginalDiagnostic is owner-authored observation, never an action or a
	// repair authorization. Omission preserves historical catalog identities.
	OriginalDiagnostic *RuntimeOriginalDiagnostic `json:",omitempty"`
	// ProspectiveTargetSatisfactionConstraint is the complete, per-CWI target
	// contract authored by the capability owner. Keeping it on the preparation
	// record avoids turning one recipe-wide surface readiness into an all-CWI
	// union: each missing requirement can be acquired independently.
	ProspectiveTargetSatisfactionConstraint RuntimeProspectiveTargetSatisfactionConstraint
}

func (p RuntimeValidationProofPreparation) Normalized() RuntimeValidationProofPreparation {
	p.RequirementHash = strings.TrimSpace(p.RequirementHash)
	p.DefinitionRevision = strings.TrimSpace(p.DefinitionRevision)
	p.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(p.CoverageWorkItemID)))
	p.DeclarationRef = strings.TrimSpace(p.DeclarationRef)
	p.ValidationRecipeID = strings.TrimSpace(p.ValidationRecipeID)
	p.TargetRef = strings.TrimSpace(p.TargetRef)
	p.CheckAttestations = NormalizeRuntimeValidationCheckAttestationBindings(p.CheckAttestations)
	p.ContentContractRef = strings.TrimSpace(p.ContentContractRef)
	p.ContentRequirements = strings.TrimSpace(p.ContentRequirements)
	p.OriginalDiagnostic = p.OriginalDiagnostic.CloneOriginal()
	p.ProspectiveTargetSatisfactionConstraint = p.ProspectiveTargetSatisfactionConstraint.Normalized()
	return p
}

func (p RuntimeValidationProofPreparation) Clone() RuntimeValidationProofPreparation {
	// Normalized detaches both the attestation slice and the nested target
	// constraint. Keep Clone single-pass because this record is replicated in
	// every retained semantic generation.
	return p.Normalized()
}

func (p RuntimeValidationProofPreparation) Validate() error {
	rawAttestations := p.CheckAttestations
	p = p.Normalized()
	if p.RequirementHash == "" || p.DefinitionRevision == "" || p.CoverageWorkItemID == "" || p.DeclarationRef == "" ||
		p.ValidationRecipeID == "" || (p.TargetRef != "" && !runtimeProspectiveTargetRefValid(p.TargetRef)) || len(p.CheckAttestations) == 0 {
		return fmt.Errorf("runtime validation proof preparation requires exact requirement, CWI, recipe and native attestations")
	}
	if strings.Contains(p.DeclarationRef, "*") {
		return fmt.Errorf("runtime validation proof preparation declaration %q is not CWI-specific", p.DeclarationRef)
	}
	if (p.ContentContractRef == "") != (p.ContentRequirements == "") || len(p.ContentContractRef) > 512 || len(p.ContentRequirements) > 4096 || !utf8.ValidString(p.ContentContractRef+p.ContentRequirements) ||
		strings.IndexFunc(p.ContentContractRef+p.ContentRequirements, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
		return fmt.Errorf("runtime validation proof preparation requires bounded owner content contract and requirements together")
	}
	required := make([]string, 0, len(p.CheckAttestations))
	for _, attestation := range p.CheckAttestations {
		required = append(required, attestation.RequiredCheckRef)
	}
	if err := ValidateRuntimeValidationCheckAttestationBindings(rawAttestations, required); err != nil {
		return err
	}
	constraint := p.ProspectiveTargetSatisfactionConstraint
	if err := constraint.Validate(); err != nil {
		return fmt.Errorf("runtime validation proof preparation target constraint: %w", err)
	}
	if constraint.ValidationRecipeID != p.ValidationRecipeID {
		return fmt.Errorf("runtime validation proof preparation recipe does not match target constraint")
	}
	if p.OriginalDiagnostic != nil {
		if err := p.OriginalDiagnostic.Validate(); err != nil {
			return fmt.Errorf("runtime validation proof preparation diagnostic: %w", err)
		}
		if p.OriginalDiagnostic.OwnerModuleID != constraint.OwnerModuleID || p.OriginalDiagnostic.ObservedRevision != constraint.CatalogObservedRevision {
			return fmt.Errorf("runtime validation proof preparation diagnostic must match its owner and catalog revision")
		}
	}
	if p.TargetRef != "" {
		exactTargetDeclared := false
		for _, clause := range constraint.AllOf {
			for _, predicate := range clause.AllOf {
				if predicate.Kind == RuntimeProspectiveTargetPredicatePathExact && predicate.Value == p.TargetRef {
					exactTargetDeclared = true
				}
			}
		}
		if !exactTargetDeclared {
			return fmt.Errorf("runtime validation proof preparation target %q is absent from exact target constraint", p.TargetRef)
		}
	}
	return nil
}

func normalizeRuntimeValidationProofPreparations(values []RuntimeValidationProofPreparation) []RuntimeValidationProofPreparation {
	ret := make([]RuntimeValidationProofPreparation, 0, len(values))
	for _, value := range values {
		ret = append(ret, value.Clone())
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].RequirementHash != ret[j].RequirementHash {
			return ret[i].RequirementHash < ret[j].RequirementHash
		}
		return ret[i].TargetRef < ret[j].TargetRef
	})
	return nilIfEmpty(ret)
}

func CloneRuntimeValidationProofPreparations(values []RuntimeValidationProofPreparation) []RuntimeValidationProofPreparation {
	return normalizeRuntimeValidationProofPreparations(values)
}

func (c RuntimeProspectiveTargetSatisfactionConstraint) Normalized() RuntimeProspectiveTargetSatisfactionConstraint {
	c.SchemaVersion = strings.TrimSpace(c.SchemaVersion)
	c.OwnerModuleID = strings.TrimSpace(c.OwnerModuleID)
	c.ProspectiveSatisfactionContractRef = strings.TrimSpace(c.ProspectiveSatisfactionContractRef)
	c.ValidationSurfaceID = strings.TrimSpace(c.ValidationSurfaceID)
	c.ValidationRecipeID = strings.TrimSpace(c.ValidationRecipeID)
	c.InputSchemaHash = strings.TrimSpace(c.InputSchemaHash)
	c.CatalogObservedRevision = strings.TrimSpace(c.CatalogObservedRevision)
	values := make([]RuntimeProspectiveTargetClause, 0, len(c.AllOf))
	for _, raw := range c.AllOf {
		values = append(values, raw.Normalized())
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].ID < values[j].ID
	})
	c.AllOf = nilIfEmpty(values)
	return c
}

func (c RuntimeProspectiveTargetSatisfactionConstraint) Clone() RuntimeProspectiveTargetSatisfactionConstraint {
	// Normalized already allocates every clause and predicate.
	return c.Normalized()
}

func (c RuntimeProspectiveTargetSatisfactionConstraint) Validate() error {
	c = c.Normalized()
	if c.SchemaVersion != RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent || c.OwnerModuleID == "" ||
		c.ProspectiveSatisfactionContractRef == "" || c.ValidationSurfaceID == "" || c.ValidationRecipeID == "" ||
		c.InputSchemaHash == "" || c.CatalogObservedRevision == "" || len(c.AllOf) == 0 {
		return fmt.Errorf("runtime prospective target satisfaction constraint requires current schema, owner, contract, surface, recipe, input schema, catalog revision and all-of target clauses")
	}
	if len(c.AllOf) > RuntimeProspectiveTargetRefsMaxCount {
		return fmt.Errorf("runtime prospective target satisfaction constraint exceeds %d all-of target clauses", RuntimeProspectiveTargetRefsMaxCount)
	}
	seen := make(map[string]struct{}, len(c.AllOf))
	for _, clause := range c.AllOf {
		if err := clause.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[clause.ID]; duplicate {
			return fmt.Errorf("runtime prospective target satisfaction constraint duplicates clause %q", clause.ID)
		}
		seen[clause.ID] = struct{}{}
	}
	return nil
}

func StableRuntimeProspectiveTargetSatisfactionConstraintHash(value RuntimeProspectiveTargetSatisfactionConstraint) string {
	value = value.Normalized()
	return stableRuntimeProspectiveTargetSatisfactionConstraintHashCanonical(value)
}

// stableRuntimeProspectiveTargetSatisfactionConstraintHashCanonical hashes a
// value already normalized by its enclosing catalog. It stays private so all
// external callers retain the canonicalization boundary.
func stableRuntimeProspectiveTargetSatisfactionConstraintHashCanonical(value RuntimeProspectiveTargetSatisfactionConstraint) string {
	h := sha256.New()
	writeHashString(h, RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent)
	writeHashString(h, value.OwnerModuleID)
	writeHashString(h, value.ProspectiveSatisfactionContractRef)
	writeHashString(h, value.ValidationSurfaceID)
	writeHashString(h, value.ValidationRecipeID)
	writeHashString(h, value.InputSchemaHash)
	writeHashString(h, value.CatalogObservedRevision)
	for _, clause := range value.AllOf {
		writeHashString(h, clause.ID)
		for _, predicate := range clause.AllOf {
			writeHashString(h, string(predicate.Kind))
			writeHashString(h, predicate.Value)
		}
	}
	return RuntimeProspectiveTargetSatisfactionConstraintHashPrefix + hex.EncodeToString(h.Sum(nil))[:20]
}

// RuntimeProspectiveTargetSatisfiesConstraint is the length-one form used by
// ordinary prospective proposals. Acquisition proposals use the bounded-set
// evaluator below.
func RuntimeProspectiveTargetSatisfiesConstraint(targetRef string, constraint RuntimeProspectiveTargetSatisfactionConstraint) bool {
	return RuntimeProspectiveTargetsSatisfyConstraint([]string{targetRef}, constraint)
}

// RuntimeProspectiveTargetsSatisfyConstraint evaluates one immutable,
// all-of-only target-set fact without I/O or owner-specific semantics. Every
// clause must be covered by at least one target and every proposed target must
// contribute to at least one clause; redundant or unrelated writes cannot be
// smuggled into the acquisition set.
func RuntimeProspectiveTargetsSatisfyConstraint(targetRefs []string, constraint RuntimeProspectiveTargetSatisfactionConstraint) bool {
	if err := constraint.Validate(); err != nil {
		return false
	}
	rawTargetRefs := append([]string(nil), targetRefs...)
	targetRefs = normalizeStrings(targetRefs)
	if len(rawTargetRefs) != len(targetRefs) {
		return false
	}
	if len(targetRefs) == 0 || len(targetRefs) > RuntimeProspectiveTargetRefsMaxCount {
		return false
	}
	for _, targetRef := range targetRefs {
		if !runtimeProspectiveTargetRefValid(targetRef) {
			return false
		}
	}
	constraint = constraint.Normalized()
	clauseMatchCounts := make([]int, len(constraint.AllOf))
	targetMatches := make([][]bool, len(targetRefs))
	for targetIndex, targetRef := range targetRefs {
		targetMatches[targetIndex] = make([]bool, len(constraint.AllOf))
		for clauseIndex, clause := range constraint.AllOf {
			if runtimeProspectiveTargetMatchesClause(targetRef, clause) {
				targetMatches[targetIndex][clauseIndex] = true
				clauseMatchCounts[clauseIndex]++
			}
		}
	}
	for _, count := range clauseMatchCounts {
		if count == 0 {
			return false
		}
	}
	for _, matches := range targetMatches {
		necessary := false
		for clauseIndex, matched := range matches {
			if matched && clauseMatchCounts[clauseIndex] == 1 {
				necessary = true
				break
			}
		}
		if !necessary {
			return false
		}
	}
	return true
}

func runtimeProspectiveTargetMatchesClause(targetRef string, clause RuntimeProspectiveTargetClause) bool {
	clean := path.Clean(targetRef)
	for _, predicate := range clause.Normalized().AllOf {
		switch predicate.Kind {
		case RuntimeProspectiveTargetPredicatePathSuffix:
			if !strings.HasSuffix(clean, predicate.Value) {
				return false
			}
		case RuntimeProspectiveTargetPredicatePathPrefix:
			if !strings.HasPrefix(clean, predicate.Value) {
				return false
			}
		case RuntimeProspectiveTargetPredicatePathExact:
			if clean != predicate.Value {
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

func runtimeProspectiveTargetRefValid(targetRef string) bool {
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" || strings.Contains(targetRef, "\\") || strings.HasPrefix(targetRef, "/") {
		return false
	}
	for _, segment := range strings.Split(targetRef, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	clean := path.Clean(targetRef)
	return clean == targetRef && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.HasPrefix(clean, "/")
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
			!RuntimeEffectIsNonDurable(surface.EffectScope) || !runtimeValidationSurfaceHasExplicitReplayPolicy(surface.IdempotencyClass) {
			continue
		}
		constraint, scoped := runtimeProspectiveConstraintForOwnerFactRequirement(surface, requirement, contractRef)
		if !scoped {
			continue
		}
		matched = append(matched, constraint)
	}
	if len(matched) != 1 {
		return RuntimeProspectiveTargetSatisfactionConstraint{}, fmt.Errorf("runtime prospective target satisfaction contract %q requires exactly one all-of surface, got %d", contractRef, len(matched))
	}
	return matched[0].Clone(), nil
}

// ResolveRuntimeValidationProofPreparation returns the one capability-owner
// preparation record for an exact missing CWI proof requirement. It never
// interprets the opaque native check protocol and cannot turn the preparation
// into evidence credit.
func ResolveRuntimeValidationProofPreparation(catalog RuntimeOperationAdmissibility, requirement RuntimeOwnerFactRequirement, contractRef string) (RuntimeValidationProofPreparation, error) {
	catalog = catalog.Normalized()
	requirement = requirement.Normalized()
	contractRef = strings.TrimSpace(contractRef)
	if err := catalog.Validate(); err != nil {
		return RuntimeValidationProofPreparation{}, err
	}
	if err := requirement.Validate(); err != nil {
		return RuntimeValidationProofPreparation{}, err
	}
	if requirement.ValidationProofRequirementHash == "" || contractRef == "" || catalog.OwnerModuleID != requirement.OwnerModuleID {
		return RuntimeValidationProofPreparation{}, fmt.Errorf("runtime validation proof preparation requires exact owner-fact requirement and contract")
	}
	var matched []RuntimeValidationProofPreparation
	for _, surface := range catalog.ValidationSurfaces {
		surface = surface.Normalized()
		if surface.OwnerModuleID != requirement.OwnerModuleID || surface.OperationKind != requirement.OperationKind ||
			!RuntimeEffectIsNonDurable(surface.EffectScope) || !runtimeValidationSurfaceHasExplicitReplayPolicy(surface.IdempotencyClass) {
			continue
		}
		for _, preparation := range surface.ProofRequirementPreparations {
			if preparation.RequirementHash == requirement.ValidationProofRequirementHash &&
				preparation.ProspectiveTargetSatisfactionConstraint.ProspectiveSatisfactionContractRef == contractRef {
				matched = append(matched, preparation.Clone())
			}
		}
	}
	if len(matched) != 1 {
		return RuntimeValidationProofPreparation{}, fmt.Errorf("runtime validation proof requirement %q requires exactly one preparation, got %d", requirement.ValidationProofRequirementHash, len(matched))
	}
	return matched[0].Clone(), nil
}

func runtimeProspectiveConstraintForOwnerFactRequirement(surface RuntimeValidationSurface, requirement RuntimeOwnerFactRequirement, contractRef string) (RuntimeProspectiveTargetSatisfactionConstraint, bool) {
	surface = surface.Clone()
	requirement = requirement.Normalized()
	contractRef = strings.TrimSpace(contractRef)
	if requirement.ValidationProofRequirementHash == "" {
		if surface.Readiness != RuntimeValidationSurfacePrerequisiteRequired || surface.ProspectiveTargetSatisfactionConstraint == nil ||
			surface.ProspectiveSatisfactionContractRef != contractRef {
			return RuntimeProspectiveTargetSatisfactionConstraint{}, false
		}
		return surface.ProspectiveTargetSatisfactionConstraint.Clone(), true
	}
	for _, preparation := range surface.ProofRequirementPreparations {
		if preparation.RequirementHash == requirement.ValidationProofRequirementHash {
			constraint := preparation.ProspectiveTargetSatisfactionConstraint.Clone()
			if constraint.ProspectiveSatisfactionContractRef != contractRef {
				return RuntimeProspectiveTargetSatisfactionConstraint{}, false
			}
			return constraint, true
		}
	}
	return RuntimeProspectiveTargetSatisfactionConstraint{}, false
}
