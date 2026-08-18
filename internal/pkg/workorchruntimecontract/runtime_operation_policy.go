package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

// RuntimeOperationPolicyFact is capability-owner-authored effect and replay
// metadata for one technical operation surface. Runtime matches only the typed
// parameter predicates and never branches on an owner, protocol, tool name, or
// risk label to derive these semantics.
type RuntimeOperationPolicyFact struct {
	ID               string
	OwnerModuleID    string
	OperationKind    string
	SubjectRef       string
	ParameterMatches []RuntimeOperationParameterMatch
	EffectScope      RuntimeEffectScope
	IdempotencyClass RuntimeOperationIdempotencyClass
}

// RuntimeOperationParameterMatch is a closed scalar matcher. It is deliberately
// narrower than OperationParameter: an owner policy may identify a concrete
// operation variant, but it cannot carry executable payload or invent fields.
type RuntimeOperationParameterMatch struct {
	Name    string
	String  *string
	Boolean *bool
}

func (m RuntimeOperationParameterMatch) Normalized() RuntimeOperationParameterMatch {
	m.Name = strings.TrimSpace(m.Name)
	if m.String != nil {
		value := strings.TrimSpace(*m.String)
		m.String = &value
	}
	if m.Boolean != nil {
		value := *m.Boolean
		m.Boolean = &value
	}
	return m
}

func (m RuntimeOperationParameterMatch) Validate() error {
	m = m.Normalized()
	if m.Name == "" || (m.String == nil) == (m.Boolean == nil) {
		return fmt.Errorf("runtime operation parameter match requires a name and exactly one scalar value")
	}
	if m.String != nil && *m.String == "" {
		return fmt.Errorf("runtime operation parameter match %q requires a non-empty string", m.Name)
	}
	return nil
}

func (f RuntimeOperationPolicyFact) Normalized() RuntimeOperationPolicyFact {
	f.ID = strings.TrimSpace(f.ID)
	f.OwnerModuleID = strings.TrimSpace(f.OwnerModuleID)
	f.OperationKind = CanonicalRuntimeBlockKind(f.OperationKind)
	f.SubjectRef = strings.TrimSpace(f.SubjectRef)
	if len(f.ParameterMatches) != 0 {
		matches := make([]RuntimeOperationParameterMatch, 0, len(f.ParameterMatches))
		for _, raw := range f.ParameterMatches {
			value := raw.Normalized()
			if value.Name != "" {
				matches = append(matches, value)
			}
		}
		sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
		f.ParameterMatches = nilIfEmpty(matches)
	}
	f.EffectScope = NormalizeRuntimeEffectScope(f.EffectScope)
	f.IdempotencyClass = NormalizeRuntimeOperationIdempotencyClass(f.IdempotencyClass)
	return f
}

func (f RuntimeOperationPolicyFact) Validate() error {
	raw := f
	f = f.Normalized()
	if f.ID == "" || f.OwnerModuleID == "" || f.OperationKind == "" {
		return fmt.Errorf("runtime operation policy fact requires identity, owner and operation kind")
	}
	descriptor, found := RuntimeOperationDescriptorForKind(f.OperationKind)
	if !found || descriptor.OwnerModuleID != f.OwnerModuleID {
		return fmt.Errorf("runtime operation policy fact %q does not belong to owner %q", f.OperationKind, f.OwnerModuleID)
	}
	if !ValidRuntimeEffectScope(RuntimeEffectScope(strings.TrimSpace(string(raw.EffectScope)))) ||
		strings.TrimSpace(string(raw.IdempotencyClass)) == "" ||
		NormalizeRuntimeOperationIdempotencyClass(raw.IdempotencyClass) != RuntimeOperationIdempotencyClass(strings.TrimSpace(string(raw.IdempotencyClass))) {
		return fmt.Errorf("runtime operation policy fact %q has invalid effect or idempotency", f.ID)
	}
	seen := make(map[string]struct{}, len(f.ParameterMatches))
	for _, match := range f.ParameterMatches {
		if err := match.Validate(); err != nil {
			return fmt.Errorf("runtime operation policy fact %q: %w", f.ID, err)
		}
		if _, duplicate := seen[match.Name]; duplicate {
			return fmt.Errorf("runtime operation policy fact %q duplicates parameter match %q", f.ID, match.Name)
		}
		seen[match.Name] = struct{}{}
	}
	return nil
}

func NormalizeRuntimeOperationPolicyFacts(values []RuntimeOperationPolicyFact) []RuntimeOperationPolicyFact {
	byID := make(map[string]RuntimeOperationPolicyFact, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ret := make([]RuntimeOperationPolicyFact, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, byID[id])
	}
	return nilIfEmpty(ret)
}

func ValidateRuntimeOperationPolicyFacts(values []RuntimeOperationPolicyFact) error {
	seenIDs := make(map[string]struct{}, len(values))
	seenSelectors := make(map[string]RuntimeOperationPolicyFact, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		if _, duplicate := seenIDs[value.ID]; duplicate {
			return fmt.Errorf("runtime operation policy fact %q is duplicated", value.ID)
		}
		seenIDs[value.ID] = struct{}{}
		if err := raw.Validate(); err != nil {
			return err
		}
		selector := runtimeOperationPolicySelectorKey(value)
		if previous, duplicate := seenSelectors[selector]; duplicate &&
			(previous.EffectScope != value.EffectScope || previous.IdempotencyClass != value.IdempotencyClass) {
			return fmt.Errorf("runtime operation policy facts %q and %q conflict for one selector", previous.ID, value.ID)
		}
		seenSelectors[selector] = value
	}
	return nil
}

func RuntimeOperationPolicyForOperation(operation RuntimeOperation, policies []RuntimeOperationPolicyFact) (RuntimeOperationPolicyFact, bool) {
	operation = operation.Normalized()
	bestSpecificity := -1
	var best RuntimeOperationPolicyFact
	conflict := false
	for _, raw := range NormalizeRuntimeOperationPolicyFacts(policies) {
		policy := raw.Normalized()
		if policy.OperationKind != CanonicalRuntimeBlockKind(operation.Kind) || !runtimeOperationPolicyMatches(operation, policy) {
			continue
		}
		specificity := len(policy.ParameterMatches)
		if specificity < bestSpecificity {
			continue
		}
		if specificity > bestSpecificity {
			best = policy
			bestSpecificity = specificity
			conflict = false
			continue
		}
		if best.EffectScope != policy.EffectScope || best.IdempotencyClass != policy.IdempotencyClass {
			conflict = true
		}
	}
	if bestSpecificity < 0 || conflict {
		return RuntimeOperationPolicyFact{}, false
	}
	return best, true
}

func RuntimeOperationPoliciesFromAdmissibilities(values []RuntimeOperationAdmissibility) []RuntimeOperationPolicyFact {
	var ret []RuntimeOperationPolicyFact
	for _, inventory := range NormalizeRuntimeOperationAdmissibilities(values) {
		ret = append(ret, inventory.OperationPolicies...)
	}
	return NormalizeRuntimeOperationPolicyFacts(ret)
}

// EffectiveRuntimeOperationPolicies makes the current owner catalog
// authoritative for every owner/kind it contains. Activation facts remain a
// bootstrap fallback only until the corresponding current catalog is sealed.
func EffectiveRuntimeOperationPolicies(activation []RuntimeOperationPolicyFact, catalogs []RuntimeOperationAdmissibility) []RuntimeOperationPolicyFact {
	activation = NormalizeRuntimeOperationPolicyFacts(activation)
	catalogs = NormalizeRuntimeOperationAdmissibilities(catalogs)
	currentOwnerKinds := make(map[string]struct{})
	var ret []RuntimeOperationPolicyFact
	for _, catalog := range catalogs {
		for _, kind := range catalog.OperationKinds {
			currentOwnerKinds[catalog.OwnerModuleID+"|"+CanonicalRuntimeBlockKind(kind)] = struct{}{}
		}
		ret = append(ret, catalog.OperationPolicies...)
	}
	for _, policy := range activation {
		if _, current := currentOwnerKinds[policy.OwnerModuleID+"|"+policy.OperationKind]; current {
			continue
		}
		ret = append(ret, policy)
	}
	return NormalizeRuntimeOperationPolicyFacts(ret)
}

// RuntimeActivationForFacts returns a detached activation view whose policy
// facts reflect the current owner catalogs. It does not mutate the persisted
// activation seed or any owner state.
func RuntimeActivationForFacts(facts RuntimeFacts) RuntimeActivationContext {
	facts = facts.Normalized()
	activation := facts.Activation.Clone()
	activation.OperationPolicies = EffectiveRuntimeOperationPolicies(
		activation.OperationPolicies,
		facts.Resources.OperationAdmissibilities,
	)
	return activation.Normalized()
}

func runtimeOperationPolicyMatches(operation RuntimeOperation, policy RuntimeOperationPolicyFact) bool {
	for _, match := range policy.ParameterMatches {
		matched := false
		for _, parameter := range operation.Parameters {
			if strings.TrimSpace(parameter.Name) != match.Name {
				continue
			}
			switch {
			case match.String != nil && parameter.String != nil:
				matched = strings.TrimSpace(*parameter.String) == *match.String
			case match.Boolean != nil && parameter.Boolean != nil:
				matched = *parameter.Boolean == *match.Boolean
			}
			break
		}
		if !matched {
			return false
		}
	}
	return true
}

func runtimeOperationPolicySelectorKey(value RuntimeOperationPolicyFact) string {
	value = value.Normalized()
	parts := []string{value.OwnerModuleID, value.OperationKind, value.SubjectRef}
	for _, match := range value.ParameterMatches {
		part := match.Name + "="
		if match.String != nil {
			part += "s:" + *match.String
		} else if match.Boolean != nil {
			part += fmt.Sprintf("b:%t", *match.Boolean)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "|")
}

func sameRuntimeOperationPolicyFact(left, right RuntimeOperationPolicyFact) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.ID == right.ID && left.OwnerModuleID == right.OwnerModuleID && left.OperationKind == right.OperationKind &&
		left.SubjectRef == right.SubjectRef && left.EffectScope == right.EffectScope && left.IdempotencyClass == right.IdempotencyClass &&
		runtimeOperationPolicySelectorKey(left) == runtimeOperationPolicySelectorKey(right)
}
