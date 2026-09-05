package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// RuntimeOperationValueRuleKind identifies a pure, manifest-sealed value
// relation that can be checked without owner I/O. Capability-specific parsing
// and mutable target applicability remain RuntimeOperationCandidateConstraint
// facts returned by the receiving owner.
type RuntimeOperationValueRuleKind string

const (
	RuntimeOperationValueRuleNonEmpty        RuntimeOperationValueRuleKind = "non_empty"
	RuntimeOperationValueRuleIntegerRange    RuntimeOperationValueRuleKind = "integer_range"
	RuntimeOperationValueRuleStringByteRange RuntimeOperationValueRuleKind = "string_byte_range"
	RuntimeOperationValueRuleBooleanEquals   RuntimeOperationValueRuleKind = "boolean_equals"
	RuntimeOperationValueRuleDistinct        RuntimeOperationValueRuleKind = "distinct_parameters"
	RuntimeOperationValueRuleIntegerOrdered  RuntimeOperationValueRuleKind = "ordered_integer_parameters"
)

// RuntimeOperationValueRule is executable catalog data. Ref is the stable
// identity shared by the operation contract, candidate manifest, strict local
// decoder and the one bounded SM-3 correction witness.
type RuntimeOperationValueRule struct {
	Ref             string                        `json:"ref"`
	Kind            RuntimeOperationValueRuleKind `json:"kind"`
	ParameterNames  []string                      `json:"parameterNames"`
	HasMinimum      bool                          `json:"hasMinimum,omitempty"`
	Minimum         int64                         `json:"minimum,omitempty"`
	HasMaximum      bool                          `json:"hasMaximum,omitempty"`
	Maximum         int64                         `json:"maximum,omitempty"`
	ExpectedBoolean string                        `json:"expectedBoolean,omitempty"`
}

type RuntimeOperationValueActualShape string

const (
	RuntimeOperationValueActualShapeEmpty             RuntimeOperationValueActualShape = "empty_value"
	RuntimeOperationValueActualShapeBelowMinimum      RuntimeOperationValueActualShape = "below_minimum"
	RuntimeOperationValueActualShapeAboveMaximum      RuntimeOperationValueActualShape = "above_maximum"
	RuntimeOperationValueActualShapeUnexpectedBoolean RuntimeOperationValueActualShape = "unexpected_boolean"
	RuntimeOperationValueActualShapeEqualParameters   RuntimeOperationValueActualShape = "equal_parameter_values"
	RuntimeOperationValueActualShapeUnorderedIntegers RuntimeOperationValueActualShape = "unordered_integer_values"
)

type RuntimeOperationValueRequiredShape string

const (
	RuntimeOperationValueRequiredShapeNonEmpty        RuntimeOperationValueRequiredShape = "non_empty_value"
	RuntimeOperationValueRequiredShapeIntegerRange    RuntimeOperationValueRequiredShape = "integer_within_declared_range"
	RuntimeOperationValueRequiredShapeStringByteRange RuntimeOperationValueRequiredShape = "string_within_declared_byte_range"
	RuntimeOperationValueRequiredShapeBooleanEquals   RuntimeOperationValueRequiredShape = "declared_boolean_value"
	RuntimeOperationValueRequiredShapeDistinct        RuntimeOperationValueRequiredShape = "distinct_parameter_values"
	RuntimeOperationValueRequiredShapeIntegerOrdered  RuntimeOperationValueRequiredShape = "ordered_integer_values"
)

// RuntimeOperationValueRuleViolation contains only closed, provider-safe
// shapes. Actual parameter values stay in the Orchestrator-private rejected
// proposal and never enter C4.
type RuntimeOperationValueRuleViolation struct {
	RuleRef       string
	OperationKind string
	VariantID     string
	ParameterName string
	ActualShape   RuntimeOperationValueActualShape
	RequiredShape RuntimeOperationValueRequiredShape
	HasMinimum    bool
	Minimum       int64
	HasMaximum    bool
	Maximum       int64
	ExpectedBool  string
}

func (v *RuntimeOperationValueRuleViolation) Error() string {
	if v == nil {
		return "runtime operation value rule is violated"
	}
	switch v.ActualShape {
	case RuntimeOperationValueActualShapeEmpty:
		return fmt.Sprintf("kind %q requires non-empty parameter %q", v.OperationKind, v.ParameterName)
	case RuntimeOperationValueActualShapeBelowMinimum, RuntimeOperationValueActualShapeAboveMaximum:
		switch {
		case v.ActualShape == RuntimeOperationValueActualShapeBelowMinimum && v.RequiredShape == RuntimeOperationValueRequiredShapeStringByteRange && v.HasMinimum:
			return fmt.Sprintf("kind %q parameter %q must be at least %d bytes", v.OperationKind, v.ParameterName, v.Minimum)
		case v.ActualShape == RuntimeOperationValueActualShapeAboveMaximum && v.RequiredShape == RuntimeOperationValueRequiredShapeStringByteRange && v.HasMaximum:
			return fmt.Sprintf("kind %q parameter %q must be at most %d bytes", v.OperationKind, v.ParameterName, v.Maximum)
		case v.HasMinimum && v.HasMaximum:
			return fmt.Sprintf("kind %q parameter %q must be between %d and %d", v.OperationKind, v.ParameterName, v.Minimum, v.Maximum)
		case v.HasMinimum:
			return fmt.Sprintf("kind %q parameter %q must be at least %d", v.OperationKind, v.ParameterName, v.Minimum)
		default:
			return fmt.Sprintf("kind %q parameter %q is outside its declared range", v.OperationKind, v.ParameterName)
		}
	case RuntimeOperationValueActualShapeUnexpectedBoolean:
		return fmt.Sprintf("kind %q requires %s=%s", v.OperationKind, v.ParameterName, v.ExpectedBool)
	case RuntimeOperationValueActualShapeEqualParameters:
		if v.VariantID != "" {
			return fmt.Sprintf("kind %q variant %q requires distinct parameters ending at %q", v.OperationKind, v.VariantID, v.ParameterName)
		}
		return fmt.Sprintf("kind %q requires a distinct %s value", v.OperationKind, v.ParameterName)
	case RuntimeOperationValueActualShapeUnorderedIntegers:
		return fmt.Sprintf("kind %q requires ordered integer parameters ending at %q", v.OperationKind, v.ParameterName)
	default:
		return "runtime operation value rule is violated"
	}
}

func (v RuntimeOperationValueRuleViolation) Validate() error {
	v.RuleRef = strings.TrimSpace(v.RuleRef)
	v.OperationKind = CanonicalRuntimeBlockKind(v.OperationKind)
	v.VariantID = strings.TrimSpace(v.VariantID)
	v.ParameterName = strings.TrimSpace(v.ParameterName)
	if v.RuleRef == "" || v.OperationKind == "" || v.ParameterName == "" {
		return fmt.Errorf("runtime operation value-rule violation requires rule, operation and parameter identity")
	}
	switch v.ActualShape {
	case RuntimeOperationValueActualShapeEmpty,
		RuntimeOperationValueActualShapeBelowMinimum,
		RuntimeOperationValueActualShapeAboveMaximum,
		RuntimeOperationValueActualShapeUnexpectedBoolean,
		RuntimeOperationValueActualShapeEqualParameters,
		RuntimeOperationValueActualShapeUnorderedIntegers:
	default:
		return fmt.Errorf("runtime operation value-rule actual shape %q is not declared", v.ActualShape)
	}
	switch v.RequiredShape {
	case RuntimeOperationValueRequiredShapeNonEmpty,
		RuntimeOperationValueRequiredShapeIntegerRange,
		RuntimeOperationValueRequiredShapeStringByteRange,
		RuntimeOperationValueRequiredShapeBooleanEquals,
		RuntimeOperationValueRequiredShapeDistinct,
		RuntimeOperationValueRequiredShapeIntegerOrdered:
	default:
		return fmt.Errorf("runtime operation value-rule required shape %q is not declared", v.RequiredShape)
	}
	if !runtimeOperationValueShapesCompatible(v.ActualShape, v.RequiredShape) {
		return fmt.Errorf("runtime operation value-rule shapes %q -> %q are incompatible", v.ActualShape, v.RequiredShape)
	}
	if (!v.HasMinimum && v.Minimum != 0) || (!v.HasMaximum && v.Maximum != 0) {
		return fmt.Errorf("runtime operation value-rule violation has an inactive bound")
	}
	switch v.ActualShape {
	case RuntimeOperationValueActualShapeBelowMinimum:
		if !v.HasMinimum || v.ExpectedBool != "" {
			return fmt.Errorf("runtime operation value-rule below-minimum violation requires an active minimum")
		}
	case RuntimeOperationValueActualShapeAboveMaximum:
		if !v.HasMaximum || v.ExpectedBool != "" {
			return fmt.Errorf("runtime operation value-rule above-maximum violation requires an active maximum")
		}
	case RuntimeOperationValueActualShapeUnexpectedBoolean:
		if v.HasMinimum || v.HasMaximum || (v.ExpectedBool != "true" && v.ExpectedBool != "false") {
			return fmt.Errorf("runtime operation value-rule boolean violation requires the declared boolean")
		}
	default:
		if v.HasMinimum || v.HasMaximum || v.ExpectedBool != "" {
			return fmt.Errorf("runtime operation value-rule violation carries unrelated bound metadata")
		}
	}
	return nil
}

func runtimeOperationValueShapesCompatible(actual RuntimeOperationValueActualShape, required RuntimeOperationValueRequiredShape) bool {
	switch actual {
	case RuntimeOperationValueActualShapeEmpty:
		return required == RuntimeOperationValueRequiredShapeNonEmpty
	case RuntimeOperationValueActualShapeBelowMinimum, RuntimeOperationValueActualShapeAboveMaximum:
		return required == RuntimeOperationValueRequiredShapeIntegerRange || required == RuntimeOperationValueRequiredShapeStringByteRange
	case RuntimeOperationValueActualShapeUnexpectedBoolean:
		return required == RuntimeOperationValueRequiredShapeBooleanEquals
	case RuntimeOperationValueActualShapeEqualParameters:
		return required == RuntimeOperationValueRequiredShapeDistinct
	case RuntimeOperationValueActualShapeUnorderedIntegers:
		return required == RuntimeOperationValueRequiredShapeIntegerOrdered
	default:
		return false
	}
}

func (r RuntimeOperationValueRule) Normalized() RuntimeOperationValueRule {
	r.Ref = strings.TrimSpace(r.Ref)
	r.Kind = RuntimeOperationValueRuleKind(strings.TrimSpace(string(r.Kind)))
	parameters := make([]string, 0, len(r.ParameterNames))
	for _, parameter := range r.ParameterNames {
		if parameter = strings.TrimSpace(parameter); parameter != "" {
			parameters = append(parameters, parameter)
		}
	}
	r.ParameterNames = nilIfEmpty(parameters)
	r.ExpectedBoolean = strings.ToLower(strings.TrimSpace(r.ExpectedBoolean))
	return r
}

func (r RuntimeOperationValueRule) Clone() RuntimeOperationValueRule {
	r = r.Normalized()
	r.ParameterNames = append([]string(nil), r.ParameterNames...)
	return r
}

func (r RuntimeOperationValueRule) Validate() error {
	r = r.Normalized()
	if r.Ref == "" {
		return fmt.Errorf("runtime operation value rule requires a ref")
	}
	seen := make(map[string]struct{}, len(r.ParameterNames))
	for _, name := range r.ParameterNames {
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("runtime operation value rule %q repeats parameter %q", r.Ref, name)
		}
		seen[name] = struct{}{}
	}
	switch r.Kind {
	case RuntimeOperationValueRuleNonEmpty:
		if len(r.ParameterNames) != 1 || r.HasMinimum || r.Minimum != 0 || r.HasMaximum || r.Maximum != 0 || r.ExpectedBoolean != "" {
			return fmt.Errorf("runtime operation value rule %q has invalid non-empty shape", r.Ref)
		}
	case RuntimeOperationValueRuleIntegerRange, RuntimeOperationValueRuleStringByteRange:
		if len(r.ParameterNames) != 1 || (!r.HasMinimum && !r.HasMaximum) ||
			(!r.HasMinimum && r.Minimum != 0) || (!r.HasMaximum && r.Maximum != 0) ||
			(r.HasMinimum && r.HasMaximum && r.Maximum < r.Minimum) || r.ExpectedBoolean != "" {
			return fmt.Errorf("runtime operation value rule %q has invalid range shape", r.Ref)
		}
	case RuntimeOperationValueRuleBooleanEquals:
		if len(r.ParameterNames) != 1 || (r.ExpectedBoolean != "true" && r.ExpectedBoolean != "false") || r.HasMinimum || r.Minimum != 0 || r.HasMaximum || r.Maximum != 0 {
			return fmt.Errorf("runtime operation value rule %q has invalid boolean shape", r.Ref)
		}
	case RuntimeOperationValueRuleDistinct, RuntimeOperationValueRuleIntegerOrdered:
		if len(r.ParameterNames) != 2 || r.HasMinimum || r.Minimum != 0 || r.HasMaximum || r.Maximum != 0 || r.ExpectedBoolean != "" {
			return fmt.Errorf("runtime operation value rule %q has invalid relation shape", r.Ref)
		}
	default:
		return fmt.Errorf("runtime operation value rule kind %q is not declared", r.Kind)
	}
	return nil
}

func (r RuntimeOperationValueRule) RequiredShape() RuntimeOperationValueRequiredShape {
	switch r.Normalized().Kind {
	case RuntimeOperationValueRuleNonEmpty:
		return RuntimeOperationValueRequiredShapeNonEmpty
	case RuntimeOperationValueRuleIntegerRange:
		return RuntimeOperationValueRequiredShapeIntegerRange
	case RuntimeOperationValueRuleStringByteRange:
		return RuntimeOperationValueRequiredShapeStringByteRange
	case RuntimeOperationValueRuleBooleanEquals:
		return RuntimeOperationValueRequiredShapeBooleanEquals
	case RuntimeOperationValueRuleDistinct:
		return RuntimeOperationValueRequiredShapeDistinct
	case RuntimeOperationValueRuleIntegerOrdered:
		return RuntimeOperationValueRequiredShapeIntegerOrdered
	default:
		return ""
	}
}

// AcceptsViolationShapes reports whether a closed diagnostic shape could be
// produced by this exact rule. It lets cross-owner consumers validate a
// detached witness without receiving the rejected parameter value.
func (r RuntimeOperationValueRule) AcceptsViolationShapes(actual RuntimeOperationValueActualShape, required RuntimeOperationValueRequiredShape) bool {
	r = r.Normalized()
	if r.Validate() != nil || r.RequiredShape() != required {
		return false
	}
	switch r.Kind {
	case RuntimeOperationValueRuleNonEmpty:
		return actual == RuntimeOperationValueActualShapeEmpty
	case RuntimeOperationValueRuleIntegerRange, RuntimeOperationValueRuleStringByteRange:
		return (actual == RuntimeOperationValueActualShapeBelowMinimum && r.HasMinimum) ||
			(actual == RuntimeOperationValueActualShapeAboveMaximum && r.HasMaximum)
	case RuntimeOperationValueRuleBooleanEquals:
		return actual == RuntimeOperationValueActualShapeUnexpectedBoolean
	case RuntimeOperationValueRuleDistinct:
		return actual == RuntimeOperationValueActualShapeEqualParameters
	case RuntimeOperationValueRuleIntegerOrdered:
		return actual == RuntimeOperationValueActualShapeUnorderedIntegers
	default:
		return false
	}
}

func NormalizeRuntimeOperationValueRules(values []RuntimeOperationValueRule) []RuntimeOperationValueRule {
	if len(values) == 0 {
		return nil
	}
	byRef := make(map[string]RuntimeOperationValueRule, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.Ref != "" {
			byRef[value.Ref] = value
		}
	}
	refs := make([]string, 0, len(byRef))
	for ref := range byRef {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	ret := make([]RuntimeOperationValueRule, 0, len(refs))
	for _, ref := range refs {
		ret = append(ret, byRef[ref].Clone())
	}
	return nilIfEmpty(ret)
}

func RuntimeOperationValueRuleID(kind, variantID string, ruleKind RuntimeOperationValueRuleKind, parameterNames ...string) string {
	parts := []string{"operation_value", CanonicalRuntimeBlockKind(kind)}
	if variantID = strings.TrimSpace(variantID); variantID != "" {
		parts = append(parts, variantID)
	}
	parts = append(parts, string(ruleKind))
	for _, name := range parameterNames {
		if name = strings.TrimSpace(name); name != "" {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, "/")
}

func runtimeNonEmptyValueRule(kind, variantID, parameter string) RuntimeOperationValueRule {
	return RuntimeOperationValueRule{Ref: RuntimeOperationValueRuleID(kind, variantID, RuntimeOperationValueRuleNonEmpty, parameter), Kind: RuntimeOperationValueRuleNonEmpty, ParameterNames: []string{parameter}}
}

func runtimeIntegerRangeValueRule(kind, variantID, parameter string, minimum, maximum int64, hasMaximum bool) RuntimeOperationValueRule {
	return RuntimeOperationValueRule{Ref: RuntimeOperationValueRuleID(kind, variantID, RuntimeOperationValueRuleIntegerRange, parameter), Kind: RuntimeOperationValueRuleIntegerRange, ParameterNames: []string{parameter}, HasMinimum: true, Minimum: minimum, HasMaximum: hasMaximum, Maximum: maximum}
}

func runtimeStringByteRangeValueRule(kind, variantID, parameter string, minimum, maximum int64) RuntimeOperationValueRule {
	return RuntimeOperationValueRule{Ref: RuntimeOperationValueRuleID(kind, variantID, RuntimeOperationValueRuleStringByteRange, parameter), Kind: RuntimeOperationValueRuleStringByteRange, ParameterNames: []string{parameter}, HasMinimum: minimum > 0, Minimum: minimum, HasMaximum: maximum > 0, Maximum: maximum}
}

func runtimeBooleanEqualsValueRule(kind, variantID, parameter string, expected bool) RuntimeOperationValueRule {
	return RuntimeOperationValueRule{Ref: RuntimeOperationValueRuleID(kind, variantID, RuntimeOperationValueRuleBooleanEquals, parameter), Kind: RuntimeOperationValueRuleBooleanEquals, ParameterNames: []string{parameter}, ExpectedBoolean: strconv.FormatBool(expected)}
}

func runtimeDistinctValueRule(kind, variantID, left, right string) RuntimeOperationValueRule {
	return RuntimeOperationValueRule{Ref: RuntimeOperationValueRuleID(kind, variantID, RuntimeOperationValueRuleDistinct, left, right), Kind: RuntimeOperationValueRuleDistinct, ParameterNames: []string{left, right}}
}

func runtimeIntegerOrderedValueRule(kind, variantID, left, right string) RuntimeOperationValueRule {
	return RuntimeOperationValueRule{Ref: RuntimeOperationValueRuleID(kind, variantID, RuntimeOperationValueRuleIntegerOrdered, left, right), Kind: RuntimeOperationValueRuleIntegerOrdered, ParameterNames: []string{left, right}}
}

func validateRuntimeOperationValueRuleDeclarations(kind string, parameters []RuntimeOperationParameterContract, operationRules []RuntimeOperationValueRule, variants []RuntimeOperationVariantContract) error {
	parameterTypes := make(map[string]RuntimeOperationParameterType, len(parameters))
	for _, parameter := range parameters {
		parameterTypes[strings.TrimSpace(parameter.Name)] = parameter.Type
	}
	validateRules := func(variantID string, rules []RuntimeOperationValueRule) error {
		seen := make(map[string]struct{}, len(rules))
		for _, rule := range rules {
			rule = rule.Normalized()
			if err := rule.Validate(); err != nil {
				return err
			}
			if _, duplicate := seen[rule.Ref]; duplicate {
				return fmt.Errorf("runtime operation value rule %q is duplicated", rule.Ref)
			}
			seen[rule.Ref] = struct{}{}
			if rule.Ref != RuntimeOperationValueRuleID(kind, variantID, rule.Kind, rule.ParameterNames...) {
				return fmt.Errorf("runtime operation value rule %q identity does not match its declaration", rule.Ref)
			}
			if rule.Kind == RuntimeOperationValueRuleDistinct {
				leftType, leftFound := parameterTypes[rule.ParameterNames[0]]
				rightType, rightFound := parameterTypes[rule.ParameterNames[1]]
				if leftFound && rightFound && leftType != rightType {
					return fmt.Errorf("runtime operation value rule %q requires parameters of the same type", rule.Ref)
				}
			}
			for _, name := range rule.ParameterNames {
				parameterType, found := parameterTypes[name]
				if !found {
					return fmt.Errorf("runtime operation value rule %q references undeclared parameter %q", rule.Ref, name)
				}
				switch rule.Kind {
				case RuntimeOperationValueRuleIntegerRange, RuntimeOperationValueRuleIntegerOrdered:
					if parameterType != RuntimeOperationParameterTypeInteger {
						return fmt.Errorf("runtime operation value rule %q requires integer parameter %q", rule.Ref, name)
					}
				case RuntimeOperationValueRuleStringByteRange:
					if parameterType != RuntimeOperationParameterTypeString {
						return fmt.Errorf("runtime operation value rule %q requires string parameter %q", rule.Ref, name)
					}
				case RuntimeOperationValueRuleBooleanEquals:
					if parameterType != RuntimeOperationParameterTypeBoolean {
						return fmt.Errorf("runtime operation value rule %q requires boolean parameter %q", rule.Ref, name)
					}
				case RuntimeOperationValueRuleNonEmpty:
					if parameterType != RuntimeOperationParameterTypeString && parameterType != RuntimeOperationParameterTypeStringList {
						return fmt.Errorf("runtime operation value rule %q requires string or string-list parameter %q", rule.Ref, name)
					}
				}
			}
		}
		return nil
	}
	if err := validateRules("", operationRules); err != nil {
		return err
	}
	for _, variant := range variants {
		if err := validateRules(strings.TrimSpace(variant.ID), variant.ValueRules); err != nil {
			return err
		}
	}
	return nil
}

func rebindRuntimeOperationValueRules(kind, variantID string, rules []RuntimeOperationValueRule) []RuntimeOperationValueRule {
	ret := make([]RuntimeOperationValueRule, 0, len(rules))
	for _, rule := range rules {
		rule = rule.Normalized()
		rule.Ref = RuntimeOperationValueRuleID(kind, variantID, rule.Kind, rule.ParameterNames...)
		ret = append(ret, rule)
	}
	return NormalizeRuntimeOperationValueRules(ret)
}

func runtimeOperationSelectedValueRules(operation RuntimeOperation, kind string, operationRules []RuntimeOperationValueRule, variants []RuntimeOperationVariantContract) (string, []RuntimeOperationValueRule, error) {
	operation = operation.Normalized()
	kind = CanonicalRuntimeBlockKind(kind)
	if operation.Kind != kind {
		return "", nil, fmt.Errorf("runtime operation value rules require a matching operation kind")
	}
	variantID := ""
	rules := append([]RuntimeOperationValueRule(nil), operationRules...)
	if len(variants) != 0 {
		variant, err := MatchRuntimeOperationVariant(operation, RuntimeOperationContract{Kind: kind, Variants: variants})
		if err != nil {
			return "", nil, err
		}
		variantID = variant.ID
		rules = append(rules, variant.ValueRules...)
	}
	return variantID, NormalizeRuntimeOperationValueRules(rules), nil
}

func RuntimeOperationValueRulesForOperation(operation RuntimeOperation, authorization RuntimeCandidateOperationManifest) (string, []RuntimeOperationValueRule, error) {
	authorization = authorization.Normalized()
	if err := validateRuntimeOperationValueRuleDeclarations(authorization.Kind, authorization.Parameters, authorization.ValueRules, authorization.Variants); err != nil {
		return "", nil, err
	}
	return runtimeOperationSelectedValueRules(operation, authorization.Kind, authorization.ValueRules, authorization.Variants)
}

func RuntimeOperationValueRuleForOperationRef(operation RuntimeOperation, authorization RuntimeCandidateOperationManifest, ref string) (RuntimeOperationValueRule, bool) {
	_, rules, err := RuntimeOperationValueRulesForOperation(operation, authorization)
	if err != nil {
		return RuntimeOperationValueRule{}, false
	}
	ref = strings.TrimSpace(ref)
	for _, rule := range rules {
		if rule.Ref == ref {
			return rule.Clone(), true
		}
	}
	return RuntimeOperationValueRule{}, false
}

func RuntimeOperationValueRuleForManifestRef(manifest RuntimeCandidateContractManifest, operationKind, ref string) (RuntimeOperationValueRule, bool) {
	operationKind = CanonicalRuntimeBlockKind(operationKind)
	ref = strings.TrimSpace(ref)
	var found RuntimeOperationValueRule
	for _, operation := range manifest.Normalized().Operations {
		if operation.Kind != operationKind {
			continue
		}
		sets := [][]RuntimeOperationValueRule{operation.ValueRules}
		for _, variant := range operation.Variants {
			sets = append(sets, variant.ValueRules)
		}
		for _, rules := range sets {
			for _, rule := range NormalizeRuntimeOperationValueRules(rules) {
				if rule.Ref != ref {
					continue
				}
				if found.Ref != "" && found.Ref == rule.Ref && strings.Join(runtimeOperationValueRuleHashStrings(found), "\x00") != strings.Join(runtimeOperationValueRuleHashStrings(rule), "\x00") {
					return RuntimeOperationValueRule{}, false
				}
				found = rule
			}
		}
	}
	if found.Ref == "" || found.Validate() != nil {
		return RuntimeOperationValueRule{}, false
	}
	return found.Clone(), true
}

func RuntimeOperationValueRuleViolationForOperation(operation RuntimeOperation, authorization RuntimeCandidateOperationManifest) (*RuntimeOperationValueRuleViolation, error) {
	variantID, rules, err := RuntimeOperationValueRulesForOperation(operation, authorization)
	if err != nil {
		return nil, err
	}
	return runtimeOperationValueRuleViolation(operation, variantID, rules), nil
}

func runtimeOperationValueRuleViolation(operation RuntimeOperation, variantID string, rules []RuntimeOperationValueRule) *RuntimeOperationValueRuleViolation {
	operation = operation.Normalized()
	values := operationParameterValues(operation.Parameters)
	for _, rule := range NormalizeRuntimeOperationValueRules(rules) {
		violation := RuntimeOperationValueRuleViolation{
			RuleRef: rule.Ref, OperationKind: operation.Kind, VariantID: variantID,
			ParameterName: rule.ParameterNames[len(rule.ParameterNames)-1], HasMinimum: rule.HasMinimum,
			Minimum: rule.Minimum, HasMaximum: rule.HasMaximum, Maximum: rule.Maximum, ExpectedBool: rule.ExpectedBoolean,
		}
		switch rule.Kind {
		case RuntimeOperationValueRuleNonEmpty:
			value, present := values[rule.ParameterNames[0]]
			if present && !operationParameterHasValue(value) {
				violation.ActualShape, violation.RequiredShape = RuntimeOperationValueActualShapeEmpty, RuntimeOperationValueRequiredShapeNonEmpty
				return &violation
			}
		case RuntimeOperationValueRuleIntegerRange:
			value, present := parameterInteger(values[rule.ParameterNames[0]])
			if !present {
				continue
			}
			if rule.HasMinimum && value < rule.Minimum {
				violation.ActualShape, violation.RequiredShape = RuntimeOperationValueActualShapeBelowMinimum, RuntimeOperationValueRequiredShapeIntegerRange
				return &violation
			}
			if rule.HasMaximum && value > rule.Maximum {
				violation.ActualShape, violation.RequiredShape = RuntimeOperationValueActualShapeAboveMaximum, RuntimeOperationValueRequiredShapeIntegerRange
				return &violation
			}
		case RuntimeOperationValueRuleStringByteRange:
			value, present := values[rule.ParameterNames[0]]
			if !present || value.String == nil {
				continue
			}
			rawByteLength := int64(len(*value.String))
			semanticByteLength := rawByteLength
			if rule.HasMinimum && rule.Minimum > 0 && strings.TrimSpace(*value.String) == "" {
				semanticByteLength = 0
			}
			if rule.HasMinimum && semanticByteLength < rule.Minimum {
				violation.ActualShape, violation.RequiredShape = RuntimeOperationValueActualShapeBelowMinimum, RuntimeOperationValueRequiredShapeStringByteRange
				return &violation
			}
			if rule.HasMaximum && rawByteLength > rule.Maximum {
				violation.ActualShape, violation.RequiredShape = RuntimeOperationValueActualShapeAboveMaximum, RuntimeOperationValueRequiredShapeStringByteRange
				return &violation
			}
		case RuntimeOperationValueRuleBooleanEquals:
			value, present := parameterBoolean(values[rule.ParameterNames[0]])
			if !present {
				continue
			}
			expected, _ := strconv.ParseBool(rule.ExpectedBoolean)
			if value != expected {
				violation.ActualShape, violation.RequiredShape = RuntimeOperationValueActualShapeUnexpectedBoolean, RuntimeOperationValueRequiredShapeBooleanEquals
				return &violation
			}
		case RuntimeOperationValueRuleDistinct:
			left, leftOK := values[rule.ParameterNames[0]]
			right, rightOK := values[rule.ParameterNames[1]]
			if leftOK && rightOK && operationParameterRawValue(left) == operationParameterRawValue(right) {
				violation.ActualShape, violation.RequiredShape = RuntimeOperationValueActualShapeEqualParameters, RuntimeOperationValueRequiredShapeDistinct
				return &violation
			}
		case RuntimeOperationValueRuleIntegerOrdered:
			left, leftOK := parameterInteger(values[rule.ParameterNames[0]])
			right, rightOK := parameterInteger(values[rule.ParameterNames[1]])
			if leftOK && rightOK && right < left {
				violation.ActualShape, violation.RequiredShape = RuntimeOperationValueActualShapeUnorderedIntegers, RuntimeOperationValueRequiredShapeIntegerOrdered
				return &violation
			}
		}
	}
	return nil
}

func runtimeOperationValueRuleHashStrings(rule RuntimeOperationValueRule) []string {
	rule = rule.Normalized()
	values := []string{rule.Ref, string(rule.Kind)}
	values = append(values, rule.ParameterNames...)
	values = append(values,
		strconv.FormatBool(rule.HasMinimum), strconv.FormatInt(rule.Minimum, 10),
		strconv.FormatBool(rule.HasMaximum), strconv.FormatInt(rule.Maximum, 10), rule.ExpectedBoolean,
	)
	return values
}
