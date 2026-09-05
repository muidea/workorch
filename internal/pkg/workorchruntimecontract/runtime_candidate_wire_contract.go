package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// RuntimeCandidateWireContractRevision identifies the manifest-derived
// provider proposal surface. The manifest remains the semantic authority; this
// projection only separates values the provider must choose from values SM-3
// can compile locally without ambiguity.
const RuntimeCandidateWireContractRevision = "runtime-candidate-wire-contract/v2"

// RuntimeCandidateWireOperationRule is one closed provider operation shape.
// Required-operation rules are selected by RequiredOperationRef and omit every
// field already fixed by that manifest slot. Auxiliary rules are selected by
// Kind and retain the provider-selectable operation vocabulary.
type RuntimeCandidateWireOperationRule struct {
	RequiredOperationRef           string
	Kind                           string
	VariantID                      string
	ProviderFields                 []string
	RequiredProviderFields         []string
	ProviderParameterNames         []string
	RequiredProviderParameterNames []string
	RequiredNonEmptyParameterNames []string
	RequireOneOfProviderParameters []string
	DynamicParameterPrefixes       []string
	AllowedBindingRefs             []string
	DerivedBindingRefs             []string
	DerivedTargetRefs              []string
	DerivedParameters              []OperationParameter
}

// RuntimeCandidateWireRuleIdentity binds one provider operation position to
// the exact manifest-derived wire rule selected before strict DTO decoding.
// It is diagnostic lineage only: it carries no candidate value, retry policy,
// aggregate state or transition authority.
type RuntimeCandidateWireRuleIdentity struct {
	WireContractHash     string `json:"wireContractHash"`
	OperationIndex       int    `json:"operationIndex"`
	RequiredOperationRef string `json:"requiredOperationRef,omitempty"`
	OperationKind        string `json:"operationKind"`
	VariantID            string `json:"variantId,omitempty"`
}

func (i RuntimeCandidateWireRuleIdentity) Normalized() RuntimeCandidateWireRuleIdentity {
	i.WireContractHash = strings.TrimSpace(i.WireContractHash)
	i.RequiredOperationRef = strings.TrimSpace(i.RequiredOperationRef)
	i.OperationKind = CanonicalRuntimeBlockKind(i.OperationKind)
	i.VariantID = strings.TrimSpace(i.VariantID)
	return i
}

func (i *RuntimeCandidateWireRuleIdentity) Clone() *RuntimeCandidateWireRuleIdentity {
	if i == nil {
		return nil
	}
	value := i.Normalized()
	return &value
}

func (i RuntimeCandidateWireRuleIdentity) Validate() error {
	i = i.Normalized()
	if i.WireContractHash == "" || i.OperationIndex < 0 || i.OperationIndex >= RuntimeCandidateOperationMaxCount || i.OperationKind == "" {
		return fmt.Errorf("runtime candidate wire rule identity requires contract hash, operation index and kind")
	}
	return nil
}

// RuntimeCandidateWireRuleIdentityForOperation constructs an exact identity
// only when rule is one of contract's sealed selectors. Callers cannot create
// diagnostic authority from a kind-shaped value that is absent from the wire
// contract.
func RuntimeCandidateWireRuleIdentityForOperation(contract RuntimeCandidateWireContract, rule RuntimeCandidateWireOperationRule, operationIndex int) (*RuntimeCandidateWireRuleIdentity, error) {
	contract = contract.Normalized()
	rule = rule.Normalized()
	if contract.Revision != RuntimeCandidateWireContractRevision || contract.Hash == "" ||
		contract.Hash != StableRuntimeCandidateWireContractHash(contract) || operationIndex < 0 {
		return nil, fmt.Errorf("runtime candidate wire rule identity requires a sealed wire contract")
	}
	selected, found := runtimeCandidateWireRuleForIdentitySelector(contract, rule.RequiredOperationRef, rule.Kind)
	if !found || !reflect.DeepEqual(selected.Normalized(), rule) {
		return nil, fmt.Errorf("runtime candidate wire rule identity does not select an exact contract rule")
	}
	identity := RuntimeCandidateWireRuleIdentity{
		WireContractHash: contract.Hash, OperationIndex: operationIndex,
		RequiredOperationRef: rule.RequiredOperationRef, OperationKind: rule.Kind, VariantID: rule.VariantID,
	}.Normalized()
	if err := identity.ValidateAgainstContract(contract); err != nil {
		return nil, err
	}
	return identity.Clone(), nil
}

func (i RuntimeCandidateWireRuleIdentity) ValidateAgainstContract(contract RuntimeCandidateWireContract) error {
	i = i.Normalized()
	contract = contract.Normalized()
	if err := i.Validate(); err != nil {
		return err
	}
	if contract.Revision != RuntimeCandidateWireContractRevision || contract.Hash == "" ||
		contract.Hash != StableRuntimeCandidateWireContractHash(contract) || i.WireContractHash != contract.Hash {
		return fmt.Errorf("runtime candidate wire rule identity contract hash mismatch")
	}
	if contract.MaxOperationCount < 1 || i.OperationIndex >= contract.MaxOperationCount {
		return fmt.Errorf("runtime candidate wire rule identity operation index exceeds contract cardinality")
	}
	selected, found := runtimeCandidateWireRuleForIdentitySelector(contract, i.RequiredOperationRef, i.OperationKind)
	if !found {
		return fmt.Errorf("runtime candidate wire rule identity selector is not sealed")
	}
	selected = selected.Normalized()
	if selected.Kind != i.OperationKind || selected.RequiredOperationRef != i.RequiredOperationRef || selected.VariantID != i.VariantID {
		return fmt.Errorf("runtime candidate wire rule identity differs from selected contract rule")
	}
	return nil
}

func (i RuntimeCandidateWireRuleIdentity) ValidateAgainstManifest(manifest RuntimeCandidateContractManifest) error {
	_, err := i.RuleAgainstManifest(manifest)
	return err
}

// RuleAgainstManifest resolves the exact sealed provider rule named by this
// diagnostic identity. Consumers that authorize a correction must use this
// rule rather than widen it back to a kind-level operation manifest.
func (i RuntimeCandidateWireRuleIdentity) RuleAgainstManifest(manifest RuntimeCandidateContractManifest) (RuntimeCandidateWireOperationRule, error) {
	if err := ValidateRuntimeCandidateContractManifestRawCanonical(manifest); err != nil {
		return RuntimeCandidateWireOperationRule{}, fmt.Errorf("runtime candidate wire rule identity manifest: %w", err)
	}
	wire, err := BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		return RuntimeCandidateWireOperationRule{}, err
	}
	if err := i.ValidateAgainstContract(wire); err != nil {
		return RuntimeCandidateWireOperationRule{}, err
	}
	rule, found := runtimeCandidateWireRuleForIdentitySelector(wire, i.RequiredOperationRef, i.OperationKind)
	if !found {
		return RuntimeCandidateWireOperationRule{}, fmt.Errorf("runtime candidate wire rule identity selector is not sealed")
	}
	return rule.Clone(), nil
}

// ValidateRuntimeCandidateFailureDiagnosticWireRule proves that a persisted
// operation-scoped diagnostic still names the exact wire rule and provider
// operation position from the manifest that rejected it. This is a pure
// lineage check; only the Orchestrator owner decides what the rejection does.
func ValidateRuntimeCandidateFailureDiagnosticWireRule(manifest RuntimeCandidateContractManifest, diagnostic *RuntimeCandidateFailureDiagnostic) error {
	if diagnostic == nil || diagnostic.WireRule == nil {
		return fmt.Errorf("runtime candidate failure diagnostic has no wire rule identity")
	}
	diagnostic = diagnostic.Clone()
	if err := ValidateRuntimeCandidateContractManifestRawCanonical(manifest); err != nil {
		return err
	}
	if diagnostic.ManifestHash == "" || diagnostic.ManifestHash != manifest.Hash {
		return fmt.Errorf("runtime candidate failure diagnostic manifest identity mismatch")
	}
	if err := diagnostic.WireRule.ValidateAgainstManifest(manifest); err != nil {
		return err
	}
	operationIndex, found := runtimeCandidateDiagnosticOperationIndex(diagnostic.Path)
	if !found || operationIndex != diagnostic.WireRule.OperationIndex || operationIndex >= manifest.MaxOperationCount {
		return fmt.Errorf("runtime candidate failure diagnostic wire rule does not match its operation path")
	}
	return nil
}

// RuntimeCandidateFailureDiagnosticRequiresWireRule reports whether a schema
// observation is scoped to one provider operation whose exact manifest rule
// must be carried across the correction boundary. Root/container diagnostics
// occur before any operation rule can be selected and therefore remain
// intentionally unbound.
func RuntimeCandidateFailureDiagnosticRequiresWireRule(class RuntimeCandidateFailureClass, diagnostic *RuntimeCandidateFailureDiagnostic) bool {
	if RuntimeCandidateFailureClass(strings.TrimSpace(string(class))) != RuntimeCandidateFailureClassSchema || diagnostic == nil {
		return false
	}
	path := strings.TrimSpace(diagnostic.Path)
	if _, found := runtimeCandidateDiagnosticOperationIndex(path); !found {
		return false
	}
	end := strings.Index(path, "]")
	if end < 0 {
		return false
	}
	// The operation object and its two selectors can fail before an exact rule
	// exists. Every other nested field is decoded only after rule selection and
	// therefore requires the identity emitted at that boundary.
	switch strings.TrimSpace(path[end+1:]) {
	case "", ".kind", ".requiredOperationRef":
		return false
	default:
		return true
	}
}

func runtimeCandidateWireRuleForIdentitySelector(contract RuntimeCandidateWireContract, requiredOperationRef, operationKind string) (RuntimeCandidateWireOperationRule, bool) {
	if strings.TrimSpace(requiredOperationRef) != "" {
		return RuntimeCandidateWireRuleForRequiredOperation(contract, requiredOperationRef)
	}
	return RuntimeCandidateWireRuleForAuxiliaryKind(contract, operationKind)
}

func (r RuntimeCandidateWireOperationRule) Normalized() RuntimeCandidateWireOperationRule {
	r.RequiredOperationRef = strings.TrimSpace(r.RequiredOperationRef)
	r.Kind = CanonicalRuntimeBlockKind(r.Kind)
	r.VariantID = strings.TrimSpace(r.VariantID)
	r.ProviderFields = runtimeCandidateWireFields(r.ProviderFields)
	r.RequiredProviderFields = runtimeCandidateWireFields(r.RequiredProviderFields)
	r.ProviderParameterNames = normalizeStrings(r.ProviderParameterNames)
	r.RequiredProviderParameterNames = normalizeStrings(r.RequiredProviderParameterNames)
	r.RequiredNonEmptyParameterNames = normalizeStrings(r.RequiredNonEmptyParameterNames)
	r.RequireOneOfProviderParameters = normalizeStrings(r.RequireOneOfProviderParameters)
	r.DynamicParameterPrefixes = normalizeStrings(r.DynamicParameterPrefixes)
	r.AllowedBindingRefs = normalizeStrings(r.AllowedBindingRefs)
	r.DerivedBindingRefs = normalizeStrings(r.DerivedBindingRefs)
	r.DerivedTargetRefs = normalizeStrings(r.DerivedTargetRefs)
	parameters := make([]OperationParameter, 0, len(r.DerivedParameters))
	for _, parameter := range r.DerivedParameters {
		parameter = normalizeRuntimeCandidateWireParameter(parameter)
		if parameter.Name != "" {
			parameters = append(parameters, parameter)
		}
	}
	sort.Slice(parameters, func(i, j int) bool { return parameters[i].Name < parameters[j].Name })
	r.DerivedParameters = parameters
	return r
}

func (r RuntimeCandidateWireOperationRule) Clone() RuntimeCandidateWireOperationRule {
	r = r.Normalized()
	r.ProviderFields = append([]string(nil), r.ProviderFields...)
	r.RequiredProviderFields = append([]string(nil), r.RequiredProviderFields...)
	r.ProviderParameterNames = append([]string(nil), r.ProviderParameterNames...)
	r.RequiredProviderParameterNames = append([]string(nil), r.RequiredProviderParameterNames...)
	r.RequiredNonEmptyParameterNames = append([]string(nil), r.RequiredNonEmptyParameterNames...)
	r.RequireOneOfProviderParameters = append([]string(nil), r.RequireOneOfProviderParameters...)
	r.DynamicParameterPrefixes = append([]string(nil), r.DynamicParameterPrefixes...)
	r.AllowedBindingRefs = append([]string(nil), r.AllowedBindingRefs...)
	r.DerivedBindingRefs = append([]string(nil), r.DerivedBindingRefs...)
	r.DerivedTargetRefs = append([]string(nil), r.DerivedTargetRefs...)
	r.DerivedParameters = cloneRuntimeOperationParameters(r.DerivedParameters)
	return r
}

func (r RuntimeCandidateWireOperationRule) RequiredSlot() bool {
	return strings.TrimSpace(r.RequiredOperationRef) != ""
}

func (r RuntimeCandidateWireOperationRule) DerivedParameterNames() []string {
	names := make([]string, 0, len(r.DerivedParameters))
	for _, parameter := range r.Normalized().DerivedParameters {
		names = append(names, parameter.Name)
	}
	return normalizeStrings(names)
}

// RuntimeCandidateWireContract is the single pure proposal-wire projection
// consumed by schema rendering, prompt rendering, strict decoding and fixture
// encoding. It contains no request/session state and cannot select retry,
// terminal or dispatch.
type RuntimeCandidateWireContract struct {
	Revision               string
	Hash                   string
	ManifestHash           string
	State                  ConvergenceState
	MaxOperationCount      int
	RequiredTopLevelFields []string
	AllowedTopLevelFields  []string
	OperationRules         []RuntimeCandidateWireOperationRule
}

func (c RuntimeCandidateWireContract) Normalized() RuntimeCandidateWireContract {
	c.Revision = strings.TrimSpace(c.Revision)
	c.Hash = strings.TrimSpace(c.Hash)
	c.ManifestHash = strings.TrimSpace(c.ManifestHash)
	c.State = ConvergenceState(strings.TrimSpace(string(c.State)))
	c.RequiredTopLevelFields = normalizeStrings(c.RequiredTopLevelFields)
	c.AllowedTopLevelFields = normalizeStrings(c.AllowedTopLevelFields)
	rules := make([]RuntimeCandidateWireOperationRule, 0, len(c.OperationRules))
	for _, rule := range c.OperationRules {
		rules = append(rules, rule.Normalized())
	}
	c.OperationRules = rules
	return c
}

func (c RuntimeCandidateWireContract) Clone() RuntimeCandidateWireContract {
	c = c.Normalized()
	c.RequiredTopLevelFields = append([]string(nil), c.RequiredTopLevelFields...)
	c.AllowedTopLevelFields = append([]string(nil), c.AllowedTopLevelFields...)
	rules := make([]RuntimeCandidateWireOperationRule, 0, len(c.OperationRules))
	for _, rule := range c.OperationRules {
		rules = append(rules, rule.Clone())
	}
	c.OperationRules = rules
	return c
}

func (c RuntimeCandidateWireContract) Validate(manifest RuntimeCandidateContractManifest) error {
	manifest = manifest.Normalized()
	if err := ValidateRuntimeCandidateContractManifestRawCanonical(manifest); err != nil {
		return fmt.Errorf("runtime candidate wire manifest: %w", err)
	}
	c = c.Normalized()
	if c.Revision != RuntimeCandidateWireContractRevision || c.Hash == "" || c.ManifestHash != manifest.Hash ||
		c.State != manifest.State || c.MaxOperationCount != manifest.MaxOperationCount ||
		c.Hash != StableRuntimeCandidateWireContractHash(c) {
		return fmt.Errorf("runtime candidate wire contract is invalid")
	}
	expected, err := BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(c, expected) {
		return fmt.Errorf("runtime candidate wire contract does not match sealed manifest")
	}
	return nil
}

// BuildRuntimeCandidateWireContract deterministically compiles the provider
// proposal surface from one sealed manifest. Fixed-field omission is a wire
// rule, not post-hoc repair: the strict decoder rejects provider echoes and
// then adds the sealed values before the ordinary manifest evaluator runs.
func BuildRuntimeCandidateWireContract(manifest RuntimeCandidateContractManifest) (RuntimeCandidateWireContract, error) {
	if err := ValidateRuntimeCandidateContractManifestRawCanonical(manifest); err != nil {
		return RuntimeCandidateWireContract{}, fmt.Errorf("runtime candidate wire manifest: %w", err)
	}
	manifest = manifest.Normalized()
	contract := RuntimeCandidateWireContract{
		Revision:               RuntimeCandidateWireContractRevision,
		ManifestHash:           manifest.Hash,
		State:                  manifest.State,
		MaxOperationCount:      manifest.MaxOperationCount,
		RequiredTopLevelFields: append([]string(nil), manifest.RequiredTopLevelFields...),
		AllowedTopLevelFields:  append([]string(nil), manifest.AllowedTopLevelFields...),
	}
	if manifest.State != ConvergenceStateNeedStateChange && manifest.State != ConvergenceStateNeedValidation {
		contract = contract.Normalized()
		contract.Hash = StableRuntimeCandidateWireContractHash(contract)
		return contract, nil
	}

	requiredKinds := make(map[string]struct{}, len(manifest.RequiredOperationSequence))
	for _, requirement := range manifest.RequiredOperationSequence {
		operation, found := runtimeCandidateWireOperationManifest(manifest, requirement.Kind)
		if !found {
			return RuntimeCandidateWireContract{}, fmt.Errorf("required operation %q has no manifest operation", requirement.RequirementID)
		}
		rule, err := runtimeCandidateRequiredWireRule(operation, requirement)
		if err != nil {
			return RuntimeCandidateWireContract{}, err
		}
		contract.OperationRules = append(contract.OperationRules, rule)
		requiredKinds[operation.Kind] = struct{}{}
	}
	for _, operation := range manifest.Operations {
		operation = operation.Normalized()
		if _, required := requiredKinds[operation.Kind]; required {
			continue
		}
		rule, err := runtimeCandidateAuxiliaryWireRule(operation)
		if err != nil {
			return RuntimeCandidateWireContract{}, err
		}
		contract.OperationRules = append(contract.OperationRules, rule)
	}
	contract = contract.Normalized()
	contract.Hash = StableRuntimeCandidateWireContractHash(contract)
	return contract, nil
}

func runtimeCandidateRequiredWireRule(operation RuntimeCandidateOperationManifest, requirement RuntimeRequiredOperationVariant) (RuntimeCandidateWireOperationRule, error) {
	operation = operation.Normalized()
	normalizedRequirements := normalizeRequiredOperationVariants([]RuntimeRequiredOperationVariant{requirement})
	if len(normalizedRequirements) != 1 {
		return RuntimeCandidateWireOperationRule{}, fmt.Errorf("required operation rule is empty")
	}
	requirement = normalizedRequirements[0]
	rule := RuntimeCandidateWireOperationRule{
		RequiredOperationRef:     requirement.RequirementID,
		Kind:                     operation.Kind,
		VariantID:                requirement.VariantID,
		ProviderFields:           []string{"requiredOperationRef"},
		RequiredProviderFields:   []string{"requiredOperationRef"},
		DynamicParameterPrefixes: append([]string(nil), operation.DynamicParameterPrefixes...),
	}
	var variant *RuntimeOperationVariantContract
	if requirement.VariantID != "" {
		value, found := runtimeCandidateWireVariant(operation.Variants, requirement.VariantID)
		if !found {
			return RuntimeCandidateWireOperationRule{}, fmt.Errorf("required operation %q has no sealed variant %q", requirement.RequirementID, requirement.VariantID)
		}
		variant = &value
	}
	var err error
	rule.DerivedParameters, rule.ProviderParameterNames, rule.RequiredProviderParameterNames,
		rule.RequiredNonEmptyParameterNames, rule.RequireOneOfProviderParameters, err =
		runtimeCandidateWireParameterRule(operation, variant, "required operation "+requirement.RequirementID)
	if err != nil {
		return RuntimeCandidateWireOperationRule{}, err
	}
	if len(rule.ProviderParameterNames) != 0 || operation.AllowsDynamicParameters {
		rule.ProviderFields = append(rule.ProviderFields, "parameters")
		if len(rule.RequiredProviderParameterNames) != 0 || len(rule.RequireOneOfProviderParameters) != 0 {
			rule.RequiredProviderFields = append(rule.RequiredProviderFields, "parameters")
		}
	}

	authorization, found := runtimeCandidateWireBindingAuthorization(operation.BindingAuthorizations, requirement.RequirementID, requirement.VariantID)
	if !found {
		return RuntimeCandidateWireOperationRule{}, fmt.Errorf("required operation %q has no binding authorization", requirement.RequirementID)
	}
	rule.AllowedBindingRefs = append([]string(nil), authorization.AllowedBindingRefs...)
	switch len(authorization.AllowedBindingRefs) {
	case 0:
	case 1:
		rule.DerivedBindingRefs = append([]string(nil), authorization.AllowedBindingRefs...)
	default:
		rule.ProviderFields = append(rule.ProviderFields, "bindingRefs")
		rule.RequiredProviderFields = append(rule.RequiredProviderFields, "bindingRefs")
	}

	if strings.TrimSpace(requirement.TargetRef) != "" {
		rule.DerivedTargetRefs = []string{strings.TrimSpace(requirement.TargetRef)}
	} else if operation.RequireTargetRefs && operation.TargetCardinality == RuntimeOperationTargetCardinalityOne {
		allowedTargets := operation.Target.AllowedTargetRefs()
		if len(allowedTargets) == 1 {
			rule.DerivedTargetRefs = append([]string(nil), allowedTargets...)
		}
	}
	if len(rule.DerivedTargetRefs) == 0 && runtimeCandidateWireContains(operation.AllowedOperationFields, "targetRefs") && runtimeCandidateWireTargetSelectable(operation) {
		rule.ProviderFields = append(rule.ProviderFields, "targetRefs")
		if operation.RequireTargetRefs {
			rule.RequiredProviderFields = append(rule.RequiredProviderFields, "targetRefs")
		}
	}
	return rule.Normalized(), nil
}

func runtimeCandidateAuxiliaryWireRule(operation RuntimeCandidateOperationManifest) (RuntimeCandidateWireOperationRule, error) {
	operation = operation.Normalized()
	rule := RuntimeCandidateWireOperationRule{
		Kind:                     operation.Kind,
		ProviderFields:           []string{"kind"},
		RequiredProviderFields:   []string{"kind"},
		DynamicParameterPrefixes: append([]string(nil), operation.DynamicParameterPrefixes...),
	}
	var variant *RuntimeOperationVariantContract
	if len(operation.Variants) == 1 {
		value := cloneRuntimeOperationVariantContract(operation.Variants[0])
		variant = &value
		rule.VariantID = value.ID
	}
	var err error
	rule.DerivedParameters, rule.ProviderParameterNames, rule.RequiredProviderParameterNames,
		rule.RequiredNonEmptyParameterNames, rule.RequireOneOfProviderParameters, err =
		runtimeCandidateWireParameterRule(operation, variant, "auxiliary operation "+operation.Kind)
	if err != nil {
		return RuntimeCandidateWireOperationRule{}, err
	}
	if len(rule.ProviderParameterNames) != 0 || operation.AllowsDynamicParameters {
		rule.ProviderFields = append(rule.ProviderFields, "parameters")
		if len(rule.RequiredProviderParameterNames) != 0 || len(rule.RequireOneOfProviderParameters) != 0 || (variant == nil && len(operation.Variants) != 0) {
			rule.RequiredProviderFields = append(rule.RequiredProviderFields, "parameters")
		}
	}

	authorizations, err := runtimeCandidateWireAuxiliaryBindingAuthorizations(operation)
	if err != nil {
		return RuntimeCandidateWireOperationRule{}, err
	}
	rule.AllowedBindingRefs = runtimeCandidateWireBindingRefs(authorizations)
	if bindingRef, fixed := runtimeCandidateWireCommonSingletonBinding(authorizations); fixed {
		rule.DerivedBindingRefs = []string{bindingRef}
	} else if len(rule.AllowedBindingRefs) != 0 {
		rule.ProviderFields = append(rule.ProviderFields, "bindingRefs")
		if runtimeCandidateWireAllAuthorizationsRequireBinding(authorizations) {
			rule.RequiredProviderFields = append(rule.RequiredProviderFields, "bindingRefs")
		}
	}

	if operation.RequireTargetRefs && operation.TargetCardinality == RuntimeOperationTargetCardinalityOne {
		allowedTargets := operation.Target.AllowedTargetRefs()
		if len(allowedTargets) == 1 {
			rule.DerivedTargetRefs = append([]string(nil), allowedTargets...)
		}
	}
	if len(rule.DerivedTargetRefs) == 0 && runtimeCandidateWireContains(operation.AllowedOperationFields, "targetRefs") && runtimeCandidateWireTargetSelectable(operation) {
		rule.ProviderFields = append(rule.ProviderFields, "targetRefs")
		if operation.RequireTargetRefs {
			rule.RequiredProviderFields = append(rule.RequiredProviderFields, "targetRefs")
		}
	}
	return rule.Normalized(), nil
}

func runtimeCandidateWireParameterRule(
	operation RuntimeCandidateOperationManifest,
	variant *RuntimeOperationVariantContract,
	context string,
) (derived []OperationParameter, provider, required, requiredNonEmpty, requireOneOf []string, err error) {
	derivedNames := make(map[string]struct{})
	if variant != nil {
		for _, discriminator := range variant.Discriminators {
			parameter, found := runtimeCandidateWireParameter(operation.Parameters, discriminator.Name)
			if !found || parameter.Type != RuntimeOperationParameterTypeString || strings.TrimSpace(discriminator.EqualsString) == "" {
				return nil, nil, nil, nil, nil, fmt.Errorf("%s has non-derivable variant discriminator %q", context, discriminator.Name)
			}
			value := strings.TrimSpace(discriminator.EqualsString)
			derived = append(derived, OperationParameter{Name: parameter.Name, String: &value})
			derivedNames[parameter.Name] = struct{}{}
		}
	}
	for _, parameter := range operation.Parameters {
		if _, fixed := derivedNames[parameter.Name]; fixed {
			continue
		}
		if variant != nil && runtimeCandidateWireContains(variant.ForbiddenParameters, parameter.Name) {
			continue
		}
		provider = append(provider, parameter.Name)
		if parameter.Required || (variant != nil && (runtimeCandidateWireContains(variant.RequiredParameters, parameter.Name) || runtimeCandidateWireContains(variant.RequiredNonEmpty, parameter.Name))) {
			required = append(required, parameter.Name)
		}
		if runtimeOperationValueRulesRequireNonEmpty(operation.ValueRules, variant, parameter.Name) ||
			(variant != nil && runtimeCandidateWireContains(variant.RequiredNonEmpty, parameter.Name)) {
			requiredNonEmpty = append(requiredNonEmpty, parameter.Name)
		}
	}
	if len(operation.RequireOneOfParameters) != 0 {
		derivedSatisfies := false
		for _, name := range operation.RequireOneOfParameters {
			if _, fixed := derivedNames[name]; fixed {
				derivedSatisfies = true
				break
			}
		}
		if !derivedSatisfies {
			for _, name := range operation.RequireOneOfParameters {
				if _, fixed := derivedNames[name]; !fixed {
					requireOneOf = append(requireOneOf, name)
				}
			}
		}
	}
	return derived, provider, required, requiredNonEmpty, requireOneOf, nil
}

func runtimeOperationValueRulesRequireNonEmpty(operationRules []RuntimeOperationValueRule, variant *RuntimeOperationVariantContract, parameterName string) bool {
	rules := append([]RuntimeOperationValueRule(nil), operationRules...)
	if variant != nil {
		rules = append(rules, variant.ValueRules...)
	}
	for _, rule := range NormalizeRuntimeOperationValueRules(rules) {
		if len(rule.ParameterNames) != 1 || rule.ParameterNames[0] != parameterName {
			continue
		}
		if rule.Kind == RuntimeOperationValueRuleNonEmpty ||
			(rule.Kind == RuntimeOperationValueRuleStringByteRange && rule.HasMinimum && rule.Minimum > 0) {
			return true
		}
	}
	return false
}

func runtimeCandidateWireAuxiliaryBindingAuthorizations(operation RuntimeCandidateOperationManifest) ([]RuntimeOperationBindingAuthorization, error) {
	if len(operation.BindingAuthorizations) == 0 {
		return nil, nil
	}
	variantIDs := []string{""}
	if len(operation.Variants) != 0 {
		variantIDs = make([]string, 0, len(operation.Variants))
		for _, variant := range operation.Variants {
			variantIDs = append(variantIDs, variant.ID)
		}
	}
	ret := make([]RuntimeOperationBindingAuthorization, 0, len(variantIDs))
	for _, variantID := range variantIDs {
		authorization, found := runtimeCandidateWireBindingAuthorization(operation.BindingAuthorizations, "", variantID)
		if !found {
			return nil, fmt.Errorf("auxiliary operation %q has no binding authorization for variant %q", operation.Kind, variantID)
		}
		ret = append(ret, authorization)
	}
	return ret, nil
}

func runtimeCandidateWireCommonSingletonBinding(authorizations []RuntimeOperationBindingAuthorization) (string, bool) {
	if len(authorizations) == 0 {
		return "", false
	}
	wanted := ""
	for _, authorization := range authorizations {
		authorization = authorization.Normalized()
		if len(authorization.AllowedBindingRefs) != 1 {
			return "", false
		}
		if wanted == "" {
			wanted = authorization.AllowedBindingRefs[0]
			continue
		}
		if authorization.AllowedBindingRefs[0] != wanted {
			return "", false
		}
	}
	return wanted, wanted != ""
}

func runtimeCandidateWireAllAuthorizationsRequireBinding(authorizations []RuntimeOperationBindingAuthorization) bool {
	if len(authorizations) == 0 {
		return false
	}
	for _, authorization := range authorizations {
		if len(authorization.Normalized().AllowedBindingRefs) == 0 {
			return false
		}
	}
	return true
}

func runtimeCandidateWireTargetSelectable(operation RuntimeCandidateOperationManifest) bool {
	target := operation.Target.Normalized()
	if len(target.AllowedTargetRefs()) != 0 {
		return true
	}
	return target.Policy == RuntimeOperationTargetPolicyExistingOwnerScope && len(target.OwnerSelectorRefs) != 0
}

func RuntimeCandidateWireRuleForRequiredOperation(contract RuntimeCandidateWireContract, requiredOperationRef string) (RuntimeCandidateWireOperationRule, bool) {
	requiredOperationRef = strings.TrimSpace(requiredOperationRef)
	for _, rule := range contract.Normalized().OperationRules {
		if rule.RequiredOperationRef == requiredOperationRef {
			return rule.Clone(), true
		}
	}
	return RuntimeCandidateWireOperationRule{}, false
}

func RuntimeCandidateWireRuleForAuxiliaryKind(contract RuntimeCandidateWireContract, kind string) (RuntimeCandidateWireOperationRule, bool) {
	kind = CanonicalRuntimeBlockKind(kind)
	for _, rule := range contract.Normalized().OperationRules {
		if rule.RequiredOperationRef == "" && rule.Kind == kind {
			return rule.Clone(), true
		}
	}
	return RuntimeCandidateWireOperationRule{}, false
}

func StableRuntimeCandidateWireContractHash(contract RuntimeCandidateWireContract) string {
	contract = contract.Normalized()
	h := sha256.New()
	writeHashString(h, RuntimeCandidateWireContractRevision)
	writeHashString(h, contract.ManifestHash)
	writeHashString(h, string(contract.State))
	writeHashString(h, fmt.Sprintf("%d", contract.MaxOperationCount))
	for _, value := range contract.RequiredTopLevelFields {
		writeHashString(h, value)
	}
	for _, value := range contract.AllowedTopLevelFields {
		writeHashString(h, value)
	}
	for index, rule := range contract.OperationRules {
		rule = rule.Normalized()
		writeHashString(h, fmt.Sprintf("rule/%d", index))
		writeHashString(h, rule.RequiredOperationRef)
		writeHashString(h, rule.Kind)
		writeHashString(h, rule.VariantID)
		for _, values := range [][]string{
			rule.ProviderFields, rule.RequiredProviderFields, rule.ProviderParameterNames,
			rule.RequiredProviderParameterNames, rule.RequiredNonEmptyParameterNames, rule.RequireOneOfProviderParameters,
			rule.DynamicParameterPrefixes, rule.AllowedBindingRefs, rule.DerivedBindingRefs, rule.DerivedTargetRefs,
		} {
			writeHashString(h, "list")
			for _, value := range values {
				writeHashString(h, value)
			}
		}
		for _, parameter := range rule.DerivedParameters {
			writeHashString(h, parameter.Name)
			writeRuntimeCandidateWireParameterHash(h, parameter)
		}
	}
	return "runtime-candidate-wire-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func runtimeCandidateWireFields(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	order := []string{"requiredOperationRef", "kind", "bindingRefs", "targetRefs", "parameters"}
	ret := make([]string, 0, len(set))
	for _, value := range order {
		if _, found := set[value]; found {
			ret = append(ret, value)
			delete(set, value)
		}
	}
	remainder := make([]string, 0, len(set))
	for value := range set {
		remainder = append(remainder, value)
	}
	sort.Strings(remainder)
	return append(ret, remainder...)
}

func runtimeCandidateWireOperationManifest(manifest RuntimeCandidateContractManifest, kind string) (RuntimeCandidateOperationManifest, bool) {
	kind = CanonicalRuntimeBlockKind(kind)
	for _, operation := range manifest.Operations {
		if operation.Kind == kind {
			return operation.Normalized(), true
		}
	}
	return RuntimeCandidateOperationManifest{}, false
}

func runtimeCandidateWireVariant(values []RuntimeOperationVariantContract, variantID string) (RuntimeOperationVariantContract, bool) {
	variantID = strings.TrimSpace(variantID)
	for _, variant := range values {
		if variant.ID == variantID {
			return cloneRuntimeOperationVariantContract(variant), true
		}
	}
	return RuntimeOperationVariantContract{}, false
}

func runtimeCandidateWireParameter(values []RuntimeOperationParameterContract, name string) (RuntimeOperationParameterContract, bool) {
	name = strings.TrimSpace(name)
	for _, parameter := range values {
		if parameter.Name == name {
			return parameter, true
		}
	}
	return RuntimeOperationParameterContract{}, false
}

func runtimeCandidateWireBindingAuthorization(values []RuntimeOperationBindingAuthorization, requiredOperationRef, variantID string) (RuntimeOperationBindingAuthorization, bool) {
	requiredOperationRef = strings.TrimSpace(requiredOperationRef)
	variantID = strings.TrimSpace(variantID)
	for _, authorization := range values {
		authorization = authorization.Normalized()
		if authorization.RequiredOperationRef == requiredOperationRef && authorization.VariantID == variantID {
			return authorization, true
		}
	}
	return RuntimeOperationBindingAuthorization{}, false
}

func runtimeCandidateWireBindingRefs(values []RuntimeOperationBindingAuthorization) []string {
	ret := make([]string, 0)
	for _, authorization := range values {
		ret = append(ret, authorization.AllowedBindingRefs...)
	}
	return normalizeStrings(ret)
}

func runtimeCandidateWireContains(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

func cloneRuntimeOperationParameters(values []OperationParameter) []OperationParameter {
	ret := make([]OperationParameter, len(values))
	for index, value := range values {
		ret[index] = value
		ret[index].StringList = append([]string(nil), value.StringList...)
	}
	return ret
}

func normalizeRuntimeCandidateWireParameter(value OperationParameter) OperationParameter {
	value.Name = strings.TrimSpace(value.Name)
	if value.String != nil {
		normalized := strings.TrimSpace(*value.String)
		value.String = &normalized
	}
	value.StringList = normalizeOperationParameterStringList(value.Name, value.StringList)
	return value
}

func writeRuntimeCandidateWireParameterHash(h interface{ Write([]byte) (int, error) }, value OperationParameter) {
	value = normalizeRuntimeCandidateWireParameter(value)
	writeHashString(h, value.Name)
	switch {
	case value.String != nil:
		writeHashString(h, "string")
		writeHashString(h, *value.String)
	case value.Integer != nil:
		writeHashString(h, "integer")
		writeHashString(h, fmt.Sprintf("%d", *value.Integer))
	case value.Boolean != nil:
		writeHashString(h, "boolean")
		writeHashString(h, fmt.Sprintf("%t", *value.Boolean))
	case value.StringList != nil:
		writeHashString(h, "string_list")
		for _, item := range value.StringList {
			writeHashString(h, item)
		}
	default:
		writeHashString(h, "missing")
	}
}
