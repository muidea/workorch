package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

// RuntimeTargetParameterBinding is an owner-authored, catalog-bound preimage,
// not a provider choice or a transition instruction. A singleton target selects
// its exact literal. Prospective targets have no preimage and omit the parameter.
// Keeping the declaration separate from Values also forbids provider echoes
// when the selected operation creates a resource.
type RuntimeTargetParameterBinding struct {
	Kind   string
	Name   string
	Values []RuntimeTargetParameterValue
}

type RuntimeTargetParameterValue struct {
	TargetRef string
	Value     string
}

func cloneRuntimeTargetParameterBindings(values []RuntimeTargetParameterBinding) []RuntimeTargetParameterBinding {
	return cloneRuntimeStateRawSlice(values, func(value RuntimeTargetParameterBinding) RuntimeTargetParameterBinding {
		value.Values = cloneRuntimeStateRawSlice(value.Values, func(item RuntimeTargetParameterValue) RuntimeTargetParameterValue { return item })
		return value
	})
}

func normalizeRuntimeTargetParameterBindings(values []RuntimeTargetParameterBinding) []RuntimeTargetParameterBinding {
	ret := cloneRuntimeTargetParameterBindings(values)
	for i := range ret {
		ret[i].Kind = CanonicalRuntimeBlockKind(ret[i].Kind)
		ret[i].Name = strings.TrimSpace(ret[i].Name)
		for j := range ret[i].Values {
			ret[i].Values[j].TargetRef = strings.TrimSpace(ret[i].Values[j].TargetRef)
		}
		sort.Slice(ret[i].Values, func(a, b int) bool { return ret[i].Values[a].TargetRef < ret[i].Values[b].TargetRef })
		ret[i].Values = nilIfEmpty(ret[i].Values)
	}
	sort.Slice(ret, func(i, j int) bool {
		if ret[i].Kind != ret[j].Kind {
			return ret[i].Kind < ret[j].Kind
		}
		return ret[i].Name < ret[j].Name
	})
	return nilIfEmpty(ret)
}

func validateRuntimeTargetParameterBindings(values []RuntimeTargetParameterBinding, owner string, kinds, targets []string) error {
	seen := make(map[string]bool, len(values))
	for _, binding := range values {
		key := binding.Kind + "\x00" + binding.Name
		contract, ok := RuntimeOperationContractForKind(binding.Kind)
		descriptor, found := RuntimeOperationDescriptorForKind(binding.Kind)
		parameter, declared := runtimeCandidateWireParameter(contract.Parameters, binding.Name)
		if seen[key] || !ok || !found || descriptor.OwnerModuleID != owner ||
			!runtimeOperationKindsContain(kinds, binding.Kind) || !declared || parameter.Required ||
			parameter.Type != RuntimeOperationParameterTypeString || !contract.RequireTargetRefs ||
			descriptor.TargetCardinality != RuntimeOperationTargetCardinalityOne ||
			containsString(contract.ForbiddenParameters, binding.Name) || containsString(contract.RequireOneOfParameters, binding.Name) {
			return fmt.Errorf("invalid owner target parameter declaration %q/%q", binding.Kind, binding.Name)
		}
		seen[key] = true
		for _, variant := range contract.Variants {
			if containsString(variant.ForbiddenParameters, binding.Name) || containsString(variant.RequiredParameters, binding.Name) || containsString(variant.RequiredNonEmpty, binding.Name) {
				return fmt.Errorf("target parameter %q conflicts with variant %q", binding.Name, variant.ID)
			}
			for _, discriminator := range variant.Discriminators {
				if discriminator.Name == binding.Name {
					return fmt.Errorf("target parameter %q cannot select a variant", binding.Name)
				}
			}
		}
		if len(binding.Values) > RuntimeOwnerInventoryItemsMaxCount {
			return fmt.Errorf("target parameter %q exceeds owner inventory bound", binding.Name)
		}
		seenTargets := make(map[string]bool, len(binding.Values))
		for _, value := range binding.Values {
			if value.TargetRef == "" || seenTargets[value.TargetRef] || !containsString(targets, value.TargetRef) || value.Value == "" || len(value.Value) > 1024 {
				return fmt.Errorf("invalid or duplicate target parameter preimage for %q/%q", binding.Kind, binding.Name)
			}
			seenTargets[value.TargetRef] = true
		}
	}
	return nil
}

func runtimeCandidateTargetParameterBindings(kind string, target RuntimeOperationTargetAuthorization, catalogs []RuntimeOperationAdmissibility, owner string) []RuntimeTargetParameterBinding {
	catalog, found := runtimeCandidateAdmissibilityForKind(catalogs, owner, kind)
	if !found {
		return nil
	}
	var ret []RuntimeTargetParameterBinding
	for _, source := range catalog.TargetParameterBindings {
		if source.Kind != kind {
			continue
		}
		binding := RuntimeTargetParameterBinding{Kind: kind, Name: source.Name}
		if target.Policy != RuntimeOperationTargetPolicyProspectiveOwnerTarget {
			for _, value := range source.Values {
				if containsString(target.AllowedTargetRefs(), value.TargetRef) {
					binding.Values = append(binding.Values, value)
				}
			}
		}
		ret = append(ret, binding)
	}
	return normalizeRuntimeTargetParameterBindings(ret)
}

func validateRuntimeCandidateTargetParameterBindings(operation RuntimeCandidateOperationManifest) error {
	if err := validateRuntimeTargetParameterBindings(operation.TargetParameterBindings, operation.OwnerModuleID, []string{operation.Kind}, operation.Target.AllowedTargetRefs()); err != nil {
		return err
	}
	for _, binding := range operation.TargetParameterBindings {
		if len(operation.AdmissibilitySnapshotRefs) == 0 || !operation.RequireTargetRefs || operation.TargetCardinality != RuntimeOperationTargetCardinalityOne {
			return fmt.Errorf("target parameter %q requires a sealed owner catalog and singleton target", binding.Name)
		}
		if operation.Target.Policy == RuntimeOperationTargetPolicyProspectiveOwnerTarget {
			if len(binding.Values) != 0 {
				return fmt.Errorf("prospective target parameter %q cannot carry an existing preimage", binding.Name)
			}
		} else if len(binding.Values) == 0 || len(binding.Values) != len(operation.Target.AllowedTargetRefs()) {
			return fmt.Errorf("target parameter %q requires complete bounded target preimages", binding.Name)
		}
	}
	return nil
}

// RuntimeCandidateTargetParameters projects only owner-derived literals for an
// authorized target choice. The SM-3 correction frontier uses this same pure
// projection when proving a different legal target; it cannot select literals
// or turn a provider-authored precondition into a locally repaired candidate.
func RuntimeCandidateTargetParameters(authorization RuntimeCandidateOperationManifest, targetRefs []string) ([]OperationParameter, error) {
	if err := validateRuntimeCandidateTargetParameterBindings(authorization); err != nil {
		return nil, err
	}
	if err := ValidateRuntimeOperationTargetsAgainstAuthorization(authorization.Target, targetRefs, authorization.RequireTargetRefs); err != nil {
		return nil, err
	}
	return runtimeTargetDerivedParameters(authorization.TargetParameterBindings, targetRefs)
}

func runtimeTargetDerivedParameters(bindings []RuntimeTargetParameterBinding, targetRefs []string) ([]OperationParameter, error) {
	if len(bindings) == 0 {
		return nil, nil
	}
	if len(targetRefs) != 1 || strings.TrimSpace(targetRefs[0]) == "" {
		return nil, fmt.Errorf("target parameter derivation requires one exact target")
	}
	var ret []OperationParameter
	for _, binding := range bindings {
		if len(binding.Values) == 0 {
			continue // Manifest-validated prospective target: no preimage.
		}
		found := false
		for _, value := range binding.Values {
			if value.TargetRef == targetRefs[0] {
				literal := value.Value
				ret = append(ret, OperationParameter{Name: binding.Name, String: &literal})
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("target has no sealed parameter preimage")
		}
	}
	return ret, nil
}

func validateRuntimeOperationTargetParameterPreimage(operation RuntimeOperation, authorization RuntimeCandidateOperationManifest) error {
	if err := validateRuntimeCandidateTargetParameterBindings(authorization); err != nil {
		return err
	}
	derived, err := runtimeTargetDerivedParameters(authorization.TargetParameterBindings, operation.TargetRefs)
	if err != nil {
		return err
	}
	for _, binding := range authorization.TargetParameterBindings {
		var expected *string
		for _, parameter := range derived {
			if parameter.Name == binding.Name {
				expected = parameter.String
			}
		}
		matches := 0
		for _, parameter := range operation.Parameters {
			if parameter.Name != binding.Name {
				continue
			}
			matches++
			typeOfValue, count := runtimeCandidatePayloadParameterType(parameter)
			if expected == nil || count != 1 || typeOfValue != RuntimeOperationParameterTypeString || parameter.String == nil || *parameter.String != *expected {
				return fmt.Errorf("target parameter %q does not match sealed preimage", binding.Name)
			}
		}
		if (expected != nil && matches != 1) || (expected == nil && matches != 0) {
			return fmt.Errorf("target parameter %q has missing or duplicate preimage", binding.Name)
		}
	}
	return nil
}

// Empty extensions intentionally add no bytes: existing sealed catalog,
// manifest and wire identities remain verifiable without archive migration.
func writeRuntimeTargetParameterBindingsHash(h hashStringWriter, bindings []RuntimeTargetParameterBinding) {
	if len(bindings) == 0 {
		return
	}
	writeHashString(h, "target-parameter-bindings/v1")
	writeHashString(h, fmt.Sprint(len(bindings)))
	for _, binding := range bindings {
		writeHashString(h, binding.Kind)
		writeHashString(h, binding.Name)
		writeHashString(h, fmt.Sprint(len(binding.Values)))
		for _, value := range binding.Values {
			writeHashString(h, value.TargetRef)
			writeHashString(h, value.Value)
		}
	}
}
