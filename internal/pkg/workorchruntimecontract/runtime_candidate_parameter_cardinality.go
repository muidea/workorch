package workorchruntimecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
)

const (
	RuntimeCandidateParameterCardinalityV1                                                    = "runtime-candidate-parameter-cardinality/v1"
	RuntimeCandidatePayloadViolationParameterCardinality RuntimeCandidatePayloadViolationKind = "parameter_cardinality"
	RuntimeCandidateParameterNamesUnique                                                      = "one_typed_entry_per_parameter_name"
	RuntimeCandidateParameterNamesRepeated                                                    = "repeated_named_parameter_entries"
	// This is a diagnostic collection bound, not an execution/retry allowance.
	maxRuntimeCandidateCardinalityParameters = 256
)

// ParameterCardinalities is the existing wire invariant expressed for schema
// renderers as well as the strict decoder. Dynamic names remain locally checked.
func (r RuntimeCandidateWireOperationRule) ParameterCardinalities() []RuntimeCandidateParameterCardinality {
	ret := make([]RuntimeCandidateParameterCardinality, 0, len(r.ProviderParameterNames))
	for _, name := range r.ProviderParameterNames {
		minimum := 0
		if runtimeCandidateWireContains(r.RequiredProviderParameterNames, name) {
			minimum = 1
		}
		ret = append(ret, RuntimeCandidateParameterCardinality{Name: name, Minimum: minimum, Maximum: 1})
	}
	return ret
}

type RuntimeCandidateParameterCardinality struct {
	Name             string
	Minimum, Maximum int
}

// The complete bounded document shape is retained, without provider values.
// Single-occurrence value hashes freeze unaffected parameters; only repeated
// declared parameter values may be regenerated. Index coverage proves that the
// collection has no silently omitted parameter entry or operation.
type RuntimeCandidateParameterCardinalityViolation struct {
	Revision       string                                          `json:"revision"`
	DocumentSHA256 string                                          `json:"documentSha256"`
	DocumentBytes  int                                             `json:"documentBytes"`
	OperationCount int                                             `json:"operationCount"`
	Operations     []RuntimeCandidateParameterCardinalityOperation `json:"operations"`
}

type RuntimeCandidateParameterCardinalityOperation struct {
	WireRule       RuntimeCandidateWireRuleIdentity       `json:"wireRule"`
	ScopeHash      string                                 `json:"scopeHash"`
	ParameterCount int                                    `json:"parameterCount"`
	Parameters     []RuntimeCandidateParameterOccurrences `json:"parameters"`
}

type RuntimeCandidateParameterOccurrences struct {
	Name      string                        `json:"name"`
	Type      RuntimeOperationParameterType `json:"type"`
	Indexes   []int                         `json:"indexes"`
	ValueHash string                        `json:"valueHash,omitempty"`
}

func (v *RuntimeCandidateParameterCardinalityViolation) Clone() *RuntimeCandidateParameterCardinalityViolation {
	if v == nil {
		return nil
	}
	out := *v
	out.Operations = append([]RuntimeCandidateParameterCardinalityOperation(nil), v.Operations...)
	for i := range out.Operations {
		out.Operations[i].Parameters = append([]RuntimeCandidateParameterOccurrences(nil), v.Operations[i].Parameters...)
		for j := range out.Operations[i].Parameters {
			out.Operations[i].Parameters[j].Indexes = append([]int(nil), v.Operations[i].Parameters[j].Indexes...)
		}
	}
	return &out
}

func cardinalitySHA(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
func cardinalityValueHash(value any) string {
	body, _ := json.Marshal(value)
	return cardinalitySHA(body)
}
func cardinalityDigestValid(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

func cardinalityScopeHash(operation RuntimeOperation) string {
	operation = operation.Normalized()
	return cardinalityValueHash(struct {
		Kind, RequiredOperationRef string
		BindingRefs, TargetRefs    []string
	}{operation.Kind, operation.RequiredOperationRef, operation.BindingRefs, operation.TargetRefs})
}

func cardinalityParameterHash(parameter OperationParameter) string {
	// Use the same positional/set normalization as the executable DTO.
	operation := (RuntimeOperation{Parameters: []OperationParameter{parameter}}).Normalized()
	return cardinalityValueHash(operation.Parameters[0])
}

func (v *RuntimeCandidateParameterCardinalityViolation) Validate() error {
	if v == nil || v.Revision != RuntimeCandidateParameterCardinalityV1 || !cardinalityDigestValid(v.DocumentSHA256) || v.DocumentBytes <= 0 || v.OperationCount != len(v.Operations) || len(v.Operations) == 0 || len(v.Operations) > RuntimeCandidateOperationMaxCount {
		return fmt.Errorf("parameter cardinality requires a versioned complete document identity")
	}
	duplicates := 0
	for i, operation := range v.Operations {
		if operation.WireRule.Validate() != nil || operation.WireRule.OperationIndex != i || !cardinalityDigestValid(operation.ScopeHash) || operation.ParameterCount < 0 || operation.ParameterCount > maxRuntimeCandidateCardinalityParameters || len(operation.Parameters) > operation.ParameterCount {
			return fmt.Errorf("parameter cardinality operation identity or bound is invalid")
		}
		seen := make([]bool, operation.ParameterCount)
		last := ""
		for _, parameter := range operation.Parameters {
			if parameter.Name == "" || parameter.Name <= last || len(parameter.Indexes) == 0 {
				return fmt.Errorf("parameter cardinality names must be unique and canonical")
			}
			last = parameter.Name
			if _, found := RuntimeOperationParameterWireValueField(parameter.Type); !found {
				return fmt.Errorf("parameter cardinality has unknown type")
			}
			if len(parameter.Indexes) == 1 {
				if !cardinalityDigestValid(parameter.ValueHash) {
					return fmt.Errorf("single parameter requires a frozen value hash")
				}
			} else {
				duplicates++
				if parameter.ValueHash != "" {
					return fmt.Errorf("duplicate parameter must not select a retained value")
				}
			}
			previous := -1
			for _, index := range parameter.Indexes {
				if index <= previous || index >= len(seen) || seen[index] {
					return fmt.Errorf("parameter cardinality indexes overlap or exceed bounds")
				}
				seen[index], previous = true, index
			}
		}
		for _, found := range seen {
			if !found {
				return fmt.Errorf("parameter cardinality collection is incomplete")
			}
		}
	}
	if duplicates == 0 {
		return fmt.Errorf("parameter cardinality has no repeated parameter")
	}
	return nil
}

func ValidateRuntimeCandidateParameterCardinality(manifest RuntimeCandidateContractManifest, v *RuntimeCandidateParameterCardinalityViolation) error {
	if err := v.Validate(); err != nil {
		return err
	}
	wire, err := BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		return err
	}
	if len(v.Operations) > wire.MaxOperationCount {
		return fmt.Errorf("parameter cardinality exceeds manifest operation count")
	}
	refs := map[string]bool{}
	for _, operation := range v.Operations {
		if err := operation.WireRule.ValidateAgainstContract(wire); err != nil {
			return err
		}
		rule, found := runtimeCandidateWireRuleForIdentitySelector(wire, operation.WireRule.RequiredOperationRef, operation.WireRule.OperationKind)
		if !found {
			return fmt.Errorf("parameter cardinality rule is not sealed")
		}
		if rule.RequiredOperationRef != "" {
			if refs[rule.RequiredOperationRef] {
				return fmt.Errorf("parameter cardinality cannot repair duplicate operation slots")
			}
			refs[rule.RequiredOperationRef] = true
		}
		contract, _ := runtimeCandidatePayloadOperationManifest(manifest, rule.Kind)
		names := map[string]bool{}
		for _, parameter := range operation.Parameters {
			declared, found := runtimeCandidateManifestParameterContract(contract, parameter.Name)
			if !found || declared.Type != parameter.Type || !runtimeCandidateWireContains(rule.ProviderParameterNames, parameter.Name) {
				return fmt.Errorf("parameter cardinality parameter is not provider-selectable in exact rule")
			}
			if len(parameter.Indexes) > 1 && cardinalityParameterSelectsVariant(contract, parameter.Name) {
				return fmt.Errorf("parameter cardinality cannot choose an ambiguous operation variant")
			}
			names[parameter.Name] = true
		}
		for _, name := range rule.RequiredProviderParameterNames {
			if !names[name] {
				return fmt.Errorf("parameter cardinality cannot mask missing required parameters")
			}
		}
	}
	for _, rule := range wire.OperationRules {
		if rule.RequiredSlot() && !refs[rule.RequiredOperationRef] {
			return fmt.Errorf("parameter cardinality cannot mask missing operation slots")
		}
	}
	return nil
}

func cardinalityParameterSelectsVariant(contract RuntimeCandidateOperationManifest, name string) bool {
	for _, variant := range contract.Variants {
		for _, discriminator := range variant.Discriminators {
			if discriminator.Name == name {
				return true
			}
		}
	}
	return false
}

// MatchesDiagnostic preserves the original first-error identity. The witness
// adds the full set of repeated names; it never changes first-error ordering.
func (v *RuntimeCandidateParameterCardinalityViolation) MatchesDiagnostic(d *RuntimeCandidateFailureDiagnostic) bool {
	if v.Validate() != nil || d == nil || d.WireRule == nil {
		return false
	}
	for _, operation := range v.Operations {
		index, name := maxRuntimeCandidateCardinalityParameters, ""
		for _, parameter := range operation.Parameters {
			if len(parameter.Indexes) > 1 && parameter.Indexes[1] < index {
				index, name = parameter.Indexes[1], parameter.Name
			}
		}
		if name != "" {
			return *d.WireRule == operation.WireRule && d.Path == fmt.Sprintf("$.operations[%d].parameters[%d].name", operation.WireRule.OperationIndex, index) && d.Actual == "duplicate_"+name
		}
	}
	return false
}

// ValidateRuntimeCandidateParameterCardinalityCorrection checks the permitted
// difference, after normal manifest admission. It cannot add/remove operations,
// change targets/bindings, or modify any single-occurrence provider parameter.
func ValidateRuntimeCandidateParameterCardinalityCorrection(manifest RuntimeCandidateContractManifest, v *RuntimeCandidateParameterCardinalityViolation, operations []RuntimeOperation) error {
	if err := ValidateRuntimeCandidateParameterCardinality(manifest, v); err != nil {
		return err
	}
	if len(operations) != len(v.Operations) {
		return fmt.Errorf("cardinality correction changed operation count")
	}
	for i, original := range v.Operations {
		operation := operations[i].Normalized()
		if operation.Kind != original.WireRule.OperationKind || operation.RequiredOperationRef != original.WireRule.RequiredOperationRef || cardinalityScopeHash(operation) != original.ScopeHash {
			return fmt.Errorf("cardinality correction changed operation scope")
		}
		rule, err := original.WireRule.RuleAgainstManifest(manifest)
		if err != nil {
			return err
		}
		parameters := map[string]OperationParameter{}
		for _, parameter := range operation.Parameters {
			if !runtimeCandidateWireContains(rule.ProviderParameterNames, parameter.Name) {
				continue
			} // derived values are checked by manifest admission
			if _, duplicate := parameters[parameter.Name]; duplicate {
				return fmt.Errorf("cardinality correction still repeats parameters")
			}
			parameters[parameter.Name] = parameter
		}
		if len(parameters) != len(original.Parameters) {
			return fmt.Errorf("cardinality correction changed parameter set")
		}
		for _, before := range original.Parameters {
			after, found := parameters[before.Name]
			if !found || !RuntimeOperationParameterMatchesContractType(after, before.Type) || (len(before.Indexes) == 1 && cardinalityParameterHash(after) != before.ValueHash) {
				return fmt.Errorf("cardinality correction changed a frozen parameter or omitted a repeated name")
			}
		}
		// No dynamic provider entry may be introduced by this closed-name witness.
		for _, parameter := range operation.Parameters {
			if _, found := parameters[parameter.Name]; found {
				continue
			}
			derived := runtimeCandidateWireContains(rule.DerivedParameterNames(), parameter.Name)
			for _, binding := range rule.TargetParameterBindings {
				derived = derived || binding.Name == parameter.Name
			}
			if !derived {
				return fmt.Errorf("cardinality correction introduced an unobserved parameter")
			}
		}
	}
	return nil
}

func writeHashRuntimeCandidateParameterCardinality(h hashStringWriter, value *RuntimeCandidateParameterCardinalityViolation) {
	// Empty extension preserves every pre-existing correction identity.
	if value == nil {
		return
	}
	writeHashString(h, RuntimeCandidateParameterCardinalityV1)
	body, _ := json.Marshal(value)
	writeHashString(h, string(body))
}

func sameRuntimeCandidateParameterCardinality(left, right *RuntimeCandidateParameterCardinalityViolation) bool {
	return reflect.DeepEqual(left, right)
}

// Only fully typed, closed-name documents qualify. Unknown fields, bad types,
// missing slots/parameters or a collection exceeding its bound receive their
// original strict rejection without a partial correction fact.
func runtimeCandidatePayloadParameterCardinality(manifest RuntimeCandidateContractManifest, wire RuntimeCandidateWireContract, payload []byte, envelope runtimeCandidateProviderWireEnvelope) *RuntimeCandidateParameterCardinalityViolation {
	if !runtimeCandidateCardinalityUniqueObjectKeys(payload) {
		return nil
	}
	value := &RuntimeCandidateParameterCardinalityViolation{Revision: RuntimeCandidateParameterCardinalityV1, DocumentSHA256: cardinalitySHA(payload), DocumentBytes: len(payload), OperationCount: len(envelope.Operations)}
	for i, raw := range envelope.Operations {
		header, identity, err := compileRuntimeCandidateProviderWireOperationHeader(wire, raw, i)
		if err != nil {
			return nil
		}
		body, _ := json.Marshal(header)
		var typed runtimeCandidateOperation
		if strictDecodeRuntimeCandidatePayload(body, &typed) != nil || len(typed.Parameters) > maxRuntimeCandidateCardinalityParameters {
			return nil
		}
		rule, _ := runtimeCandidateWireRuleForIdentitySelector(wire, identity.RequiredOperationRef, identity.OperationKind)
		contract, _ := runtimeCandidatePayloadOperationManifest(manifest, rule.Kind)
		// A structural witness cannot fix a second missing/invalid authority
		// selector. Validate header choices directly, without constructing a
		// deduplicated candidate or choosing any repeated value.
		for _, field := range []string{"targetRefs", "bindingRefs"} {
			if raw, present := header[field]; present && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return nil
			}
		}
		if ValidateRuntimeOperationTargetsAgainstAuthorization(contract.Target, typed.TargetRefs, contract.RequireTargetRefs) != nil {
			return nil
		}
		if len(rule.AllowedBindingRefs) > 0 && len(typed.BindingRefs) != 1 {
			return nil
		}
		for _, ref := range typed.BindingRefs {
			if !runtimeCandidateWireContains(rule.AllowedBindingRefs, ref) {
				return nil
			}
		}
		var rawParameters []map[string]json.RawMessage
		if raw, present := header["parameters"]; present && json.Unmarshal(raw, &rawParameters) != nil {
			return nil
		}
		operation := RuntimeCandidateParameterCardinalityOperation{WireRule: *identity, ParameterCount: len(typed.Parameters), ScopeHash: cardinalityScopeHash(RuntimeOperation{Kind: typed.Kind, RequiredOperationRef: typed.RequiredOperationRef, BindingRefs: typed.BindingRefs, TargetRefs: typed.TargetRefs})}
		byName := map[string]*RuntimeCandidateParameterOccurrences{}
		for index, entry := range typed.Parameters {
			parameter := OperationParameter{Name: entry.Name, String: entry.String, Integer: entry.Integer, Boolean: entry.Boolean, StringList: entry.StringList}
			declared, found := runtimeCandidateManifestParameterContract(contract, entry.Name)
			if !found || !runtimeCandidateWireContains(rule.ProviderParameterNames, entry.Name) || !RuntimeOperationParameterMatchesContractType(parameter, declared.Type) {
				return nil
			}
			field, _ := RuntimeOperationParameterWireValueField(declared.Type)
			if len(rawParameters[index]) != 2 || len(rawParameters[index][field]) == 0 || bytes.Equal(bytes.TrimSpace(rawParameters[index][field]), []byte("null")) {
				return nil
			}
			group := byName[entry.Name]
			if group == nil {
				group = &RuntimeCandidateParameterOccurrences{Name: entry.Name, Type: declared.Type, ValueHash: cardinalityParameterHash(parameter)}
				byName[entry.Name] = group
			}
			group.Indexes = append(group.Indexes, index)
			if len(group.Indexes) > 1 {
				group.ValueHash = ""
			}
		}
		for _, group := range byName {
			operation.Parameters = append(operation.Parameters, *group)
		}
		sort.Slice(operation.Parameters, func(i, j int) bool { return operation.Parameters[i].Name < operation.Parameters[j].Name })
		value.Operations = append(value.Operations, operation)
	}
	if ValidateRuntimeCandidateParameterCardinality(manifest, value) != nil {
		return nil
	}
	return value
}

// Duplicate JSON object keys are ambiguous even if encoding/json accepts them.
// They cannot be used as the source of this stricter diagnostic protocol.
func runtimeCandidateCardinalityUniqueObjectKeys(payload []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var visit func(int) bool
	visit = func(depth int) bool {
		if depth > 64 {
			return false
		}
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		delim, nested := token.(json.Delim)
		if !nested {
			return true
		}
		if delim != '{' && delim != '[' {
			return false
		}
		seen := map[string]bool{}
		for decoder.More() {
			if delim == '{' {
				key, err := decoder.Token()
				if err != nil {
					return false
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return false
				}
				seen[name] = true
			}
			if !visit(depth + 1) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && ((delim == '{' && end == json.Delim('}')) || (delim == '[' && end == json.Delim(']')))
	}
	if !visit(0) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}
