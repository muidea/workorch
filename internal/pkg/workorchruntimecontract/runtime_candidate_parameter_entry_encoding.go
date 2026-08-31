package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

// RuntimeCandidatePayloadViolationKind identifies a provider-wire defect
// observed by the strict candidate decoder. It is a structural fact only;
// SM-3 remains the sole owner of correction/terminal selection.
type RuntimeCandidatePayloadViolationKind string

const (
	RuntimeCandidatePayloadViolationParameterEntryEncoding RuntimeCandidatePayloadViolationKind = "parameter_entry_encoding"
)

const maxRuntimeCandidateParameterEntryEncodings = 32

// RuntimeCandidateMisplacedParameter is the provider-safe identity of one
// declared logical parameter that was encoded as an object member of another
// named parameter entry. No provider-authored value is representable here.
type RuntimeCandidateMisplacedParameter struct {
	Name string                        `json:"name"`
	Type RuntimeOperationParameterType `json:"type"`
}

func (p RuntimeCandidateMisplacedParameter) Normalized() RuntimeCandidateMisplacedParameter {
	p.Name = strings.TrimSpace(p.Name)
	p.Type = RuntimeOperationParameterType(strings.TrimSpace(string(p.Type)))
	return p
}

func (p RuntimeCandidateMisplacedParameter) Validate() error {
	p = p.Normalized()
	if p.Name == "" {
		return fmt.Errorf("runtime candidate misplaced parameter requires a name")
	}
	if _, found := RuntimeOperationParameterWireValueField(p.Type); !found {
		return fmt.Errorf("runtime candidate misplaced parameter %q has undeclared type %q", p.Name, p.Type)
	}
	return nil
}

// RuntimeCandidateParameterEntryEncodingViolation records one malformed
// parameters[] entry without retaining any raw provider value. OperationIndex
// and ParameterIndex bind the fact to the rejected document; logical parameter
// ordering remains irrelevant and is normalized by name.
type RuntimeCandidateParameterEntryEncodingViolation struct {
	OperationIndex         int                                  `json:"operationIndex"`
	ParameterIndex         int                                  `json:"parameterIndex"`
	OperationKind          string                               `json:"operationKind"`
	ContainerParameterName string                               `json:"containerParameterName"`
	MisplacedParameters    []RuntimeCandidateMisplacedParameter `json:"misplacedParameters"`
}

func (v RuntimeCandidateParameterEntryEncodingViolation) Normalized() RuntimeCandidateParameterEntryEncodingViolation {
	v.OperationKind = CanonicalRuntimeBlockKind(v.OperationKind)
	v.ContainerParameterName = strings.TrimSpace(v.ContainerParameterName)
	v.MisplacedParameters = append([]RuntimeCandidateMisplacedParameter(nil), v.MisplacedParameters...)
	for index := range v.MisplacedParameters {
		v.MisplacedParameters[index] = v.MisplacedParameters[index].Normalized()
	}
	sort.Slice(v.MisplacedParameters, func(left, right int) bool {
		if v.MisplacedParameters[left].Name == v.MisplacedParameters[right].Name {
			return v.MisplacedParameters[left].Type < v.MisplacedParameters[right].Type
		}
		return v.MisplacedParameters[left].Name < v.MisplacedParameters[right].Name
	})
	return v
}

func (v RuntimeCandidateParameterEntryEncodingViolation) Clone() RuntimeCandidateParameterEntryEncodingViolation {
	v = v.Normalized()
	v.MisplacedParameters = append([]RuntimeCandidateMisplacedParameter(nil), v.MisplacedParameters...)
	return v
}

func (v RuntimeCandidateParameterEntryEncodingViolation) Validate() error {
	v = v.Normalized()
	if v.OperationIndex < 0 || v.ParameterIndex < 0 || v.ParameterIndex >= maxRuntimeCandidateParameterEntryEncodings || v.OperationKind == "" || v.ContainerParameterName == "" {
		return fmt.Errorf("runtime candidate parameter-entry encoding requires bounded indexes, operation kind and container parameter")
	}
	if len(v.MisplacedParameters) == 0 || len(v.MisplacedParameters) > maxRuntimeCandidateParameterEntryEncodings {
		return fmt.Errorf("runtime candidate parameter-entry encoding requires a bounded misplaced-parameter set")
	}
	seen := map[string]struct{}{}
	for _, parameter := range v.MisplacedParameters {
		if err := parameter.Validate(); err != nil {
			return err
		}
		if parameter.Name == v.ContainerParameterName {
			return fmt.Errorf("runtime candidate parameter-entry encoding cannot move its container parameter %q", parameter.Name)
		}
		if _, found := seen[parameter.Name]; found {
			return fmt.Errorf("runtime candidate parameter-entry encoding duplicates parameter %q", parameter.Name)
		}
		seen[parameter.Name] = struct{}{}
	}
	return nil
}

func normalizeRuntimeCandidateParameterEntryEncodings(values []RuntimeCandidateParameterEntryEncodingViolation) []RuntimeCandidateParameterEntryEncodingViolation {
	ret := append([]RuntimeCandidateParameterEntryEncodingViolation(nil), values...)
	for index := range ret {
		ret[index] = ret[index].Normalized()
	}
	sort.Slice(ret, func(left, right int) bool {
		if ret[left].OperationIndex != ret[right].OperationIndex {
			return ret[left].OperationIndex < ret[right].OperationIndex
		}
		if ret[left].ParameterIndex != ret[right].ParameterIndex {
			return ret[left].ParameterIndex < ret[right].ParameterIndex
		}
		if ret[left].OperationKind != ret[right].OperationKind {
			return ret[left].OperationKind < ret[right].OperationKind
		}
		return ret[left].ContainerParameterName < ret[right].ContainerParameterName
	})
	return ret
}

func cloneRuntimeCandidateParameterEntryEncodings(values []RuntimeCandidateParameterEntryEncodingViolation) []RuntimeCandidateParameterEntryEncodingViolation {
	ret := normalizeRuntimeCandidateParameterEntryEncodings(values)
	for index := range ret {
		ret[index] = ret[index].Clone()
	}
	return ret
}

// ValidateRuntimeCandidateParameterEntryEncodings validates both the bounded
// DTO and its exact sealed-manifest membership. Dynamic parameters are
// deliberately excluded: an encoding correction cannot create new authority.
func ValidateRuntimeCandidateParameterEntryEncodings(manifest RuntimeCandidateContractManifest, values []RuntimeCandidateParameterEntryEncodingViolation) error {
	manifest = manifest.Normalized()
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" ||
		manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) || manifest.MaxOperationCount <= 0 {
		return fmt.Errorf("runtime candidate parameter-entry correction requires a sealed operation manifest")
	}
	values = normalizeRuntimeCandidateParameterEntryEncodings(values)
	if err := validateRuntimeCandidateParameterEntryEncodingFacts(values); err != nil {
		return err
	}
	for _, value := range values {
		if manifest.MaxOperationCount > 0 && value.OperationIndex >= manifest.MaxOperationCount {
			return fmt.Errorf("runtime candidate parameter-entry operation index %d exceeds sealed cardinality", value.OperationIndex)
		}
		operation, found := runtimeCandidatePayloadOperationManifest(manifest, value.OperationKind)
		if !found {
			return fmt.Errorf("runtime candidate parameter-entry operation kind %q is outside sealed manifest", value.OperationKind)
		}
		container, found := runtimeCandidateManifestParameterContract(operation, value.ContainerParameterName)
		if !found || runtimeCandidatePayloadContains(operation.ForbiddenParameters, container.Name) {
			return fmt.Errorf("runtime candidate parameter-entry container %q is outside sealed operation %q", value.ContainerParameterName, value.OperationKind)
		}
		for _, misplaced := range value.MisplacedParameters {
			parameter, found := runtimeCandidateManifestParameterContract(operation, misplaced.Name)
			if !found || parameter.Type != misplaced.Type || runtimeCandidatePayloadContains(operation.ForbiddenParameters, misplaced.Name) {
				return fmt.Errorf("runtime candidate parameter-entry parameter %q/%s is outside sealed operation %q", misplaced.Name, misplaced.Type, value.OperationKind)
			}
		}
	}
	return nil
}

func validateRuntimeCandidateParameterEntryEncodingFacts(values []RuntimeCandidateParameterEntryEncodingViolation) error {
	values = normalizeRuntimeCandidateParameterEntryEncodings(values)
	if len(values) == 0 || len(values) > maxRuntimeCandidateParameterEntryEncodings {
		return fmt.Errorf("runtime candidate parameter-entry correction requires a bounded encoding set")
	}
	total := 0
	seenEntries := map[string]struct{}{}
	operationKinds := map[int]string{}
	containerNames := map[string]struct{}{}
	misplacedNames := map[string]struct{}{}
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
		entryKey := fmt.Sprintf("%d/%d", value.OperationIndex, value.ParameterIndex)
		if _, found := seenEntries[entryKey]; found {
			return fmt.Errorf("runtime candidate parameter-entry correction duplicates source entry %s", entryKey)
		}
		seenEntries[entryKey] = struct{}{}
		if kind, found := operationKinds[value.OperationIndex]; found && kind != value.OperationKind {
			return fmt.Errorf("runtime candidate parameter-entry operation %d has conflicting kinds", value.OperationIndex)
		}
		operationKinds[value.OperationIndex] = value.OperationKind
		containerKey := fmt.Sprintf("%d/%s", value.OperationIndex, value.ContainerParameterName)
		if _, found := containerNames[containerKey]; found {
			return fmt.Errorf("runtime candidate parameter-entry correction duplicates container %q", value.ContainerParameterName)
		}
		if _, found := misplacedNames[containerKey]; found {
			return fmt.Errorf("runtime candidate parameter-entry container %q is also misplaced", value.ContainerParameterName)
		}
		containerNames[containerKey] = struct{}{}
		total += len(value.MisplacedParameters)
		if total > maxRuntimeCandidateParameterEntryEncodings {
			return fmt.Errorf("runtime candidate parameter-entry correction exceeds misplaced-parameter bound")
		}
		for _, parameter := range value.MisplacedParameters {
			parameterKey := fmt.Sprintf("%d/%s", value.OperationIndex, parameter.Name)
			if _, found := containerNames[parameterKey]; found {
				return fmt.Errorf("runtime candidate misplaced parameter %q is also a container", parameter.Name)
			}
			if _, found := misplacedNames[parameterKey]; found {
				return fmt.Errorf("runtime candidate parameter-entry correction duplicates misplaced parameter %q", parameter.Name)
			}
			misplacedNames[parameterKey] = struct{}{}
		}
	}
	return nil
}

func runtimeCandidateManifestParameterContract(operation RuntimeCandidateOperationManifest, name string) (RuntimeOperationParameterContract, bool) {
	name = strings.TrimSpace(name)
	for _, parameter := range operation.Normalized().Parameters {
		if parameter.Name == name {
			return parameter, true
		}
	}
	return RuntimeOperationParameterContract{}, false
}

// RuntimeOperationParameterWireValueField is the canonical provider-wire key
// for one declared parameter type.
func RuntimeOperationParameterWireValueField(parameterType RuntimeOperationParameterType) (string, bool) {
	switch parameterType {
	case RuntimeOperationParameterTypeString:
		return "string", true
	case RuntimeOperationParameterTypeInteger:
		return "integer", true
	case RuntimeOperationParameterTypeBoolean:
		return "boolean", true
	case RuntimeOperationParameterTypeStringList:
		return "stringList", true
	default:
		return "", false
	}
}

// RuntimeOperationParameterMatchesContractType compares one decoded named
// parameter with a sealed contract type without interpreting its value.
func RuntimeOperationParameterMatchesContractType(parameter OperationParameter, parameterType RuntimeOperationParameterType) bool {
	actual, count := runtimeCandidatePayloadParameterType(parameter)
	return count == 1 && actual == parameterType
}

func sameRuntimeCandidateParameterEntryEncodings(left, right []RuntimeCandidateParameterEntryEncodingViolation) bool {
	left = normalizeRuntimeCandidateParameterEntryEncodings(left)
	right = normalizeRuntimeCandidateParameterEntryEncodings(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].OperationIndex != right[index].OperationIndex ||
			left[index].ParameterIndex != right[index].ParameterIndex ||
			left[index].OperationKind != right[index].OperationKind ||
			left[index].ContainerParameterName != right[index].ContainerParameterName ||
			len(left[index].MisplacedParameters) != len(right[index].MisplacedParameters) {
			return false
		}
		for parameterIndex := range left[index].MisplacedParameters {
			if left[index].MisplacedParameters[parameterIndex] != right[index].MisplacedParameters[parameterIndex] {
				return false
			}
		}
	}
	return true
}

func writeHashRuntimeCandidateParameterEntryEncodings(h hashStringWriter, values []RuntimeCandidateParameterEntryEncodingViolation) {
	values = normalizeRuntimeCandidateParameterEntryEncodings(values)
	writeHashString(h, fmt.Sprintf("%d", len(values)))
	for _, value := range values {
		writeHashString(h, fmt.Sprintf("%d", value.OperationIndex))
		writeHashString(h, fmt.Sprintf("%d", value.ParameterIndex))
		writeHashString(h, value.OperationKind)
		writeHashString(h, value.ContainerParameterName)
		writeHashString(h, fmt.Sprintf("%d", len(value.MisplacedParameters)))
		for _, parameter := range value.MisplacedParameters {
			writeHashString(h, parameter.Name)
			writeHashString(h, string(parameter.Type))
		}
	}
}
