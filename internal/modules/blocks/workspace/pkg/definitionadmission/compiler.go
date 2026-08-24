// Package definitionadmission contains the Workspace-owner pure static
// Definition compiler. It has no EventHub, repository, LLM or capability
// dependency. Only the Workspace private reducer may use its result to select
// an admission state transition.
package definitionadmission

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

var allOfRelationKinds = map[string]struct{}{
	"all_of": {}, "depends_on": {}, "verification_of": {}, "invalidated_by": {}, "context_ref": {},
}

var unsupportedAlternativeKinds = map[string]struct{}{
	"one_of": {}, "any_of": {}, "any_settled": {}, "quorum": {}, "threshold": {}, "first_success": {},
}

// Compile performs only checks derivable from the immutable candidate. It
// does not inspect free text, call an LLM or probe runtime owners.
func Compile(candidate workspacemodel.DefinitionAdmissionCandidate) (workspacemodel.DefinitionAdmissionResult, error) {
	candidate = candidate.Normalized()
	if err := candidate.Validate(); err != nil {
		return workspacemodel.DefinitionAdmissionResult{}, err
	}
	if candidate.AdmissionCompilerVersion != workspacemodel.DefinitionAdmissionCompilerVersionCurrent {
		return workspacemodel.DefinitionAdmissionResult{}, fmt.Errorf("unsupported definition admission compiler %q", candidate.AdmissionCompilerVersion)
	}
	for _, relation := range candidate.ProposedRelationInputs {
		kind := strings.ToLower(strings.TrimSpace(relation.RelationKind))
		if _, unsupported := unsupportedAlternativeKinds[kind]; unsupported {
			return clarification(candidate, workspacemodel.DefinitionAdmissionOneOfUnsupported, []string{relation.RelationRef}, "unsupported_alternative_relation:"+kind), nil
		}
		if _, supported := allOfRelationKinds[kind]; !supported {
			return clarification(candidate, string(workspacemodel.DefinitionAdmissionClarificationRequired), []string{relation.RelationRef}, "unsupported_relation:"+kind), nil
		}
	}
	if cycle := hardDependencyCycle(candidate.ProposedRelationInputs); len(cycle) > 0 {
		return clarification(candidate, workspacemodel.DefinitionAdmissionPrerequisiteUnresolved, cycle, "hard_dependency_cycle"), nil
	}
	if unresolved := unresolvedOwnerFactConvergence(candidate); len(unresolved) > 0 {
		return clarification(candidate, workspacemodel.DefinitionAdmissionPrerequisiteUnresolved, unresolved, "owner_fact_convergence_incomplete"), nil
	}
	result := workspacemodel.DefinitionAdmissionResult{
		WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision,
		CandidateHash: candidate.CandidateHash, Disposition: workspacemodel.DefinitionAdmissionAdmittedAllOf,
		AdmissionCompilerVersion:   candidate.AdmissionCompilerVersion,
		CanonicalAdmittedInputHash: stableHash("admitted_all_of", candidate.CandidateHash, candidate.AdmissionCompilerVersion),
	}.WithHash()
	return result, result.ValidateFor(candidate)
}

func unresolvedOwnerFactConvergence(candidate workspacemodel.DefinitionAdmissionCandidate) []string {
	candidate = candidate.Normalized()
	required := make(map[string]workspacemodel.DefinitionRequiredOwnerFactBinding, len(candidate.RequiredOwnerFactBindings))
	unresolved := make([]string, 0)
	for _, binding := range candidate.RequiredOwnerFactBindings {
		key := definitionOwnerFactBindingKey(binding.SourceRef, binding.Requirement.OwnerModuleID, string(binding.Requirement.FactKind), binding.Requirement.OperationKind)
		if _, duplicate := required[key]; duplicate {
			unresolved = append(unresolved, binding.SourceRef)
			continue
		}
		required[key] = binding
	}
	counts := make(map[string]int, len(candidate.OwnerFactConvergenceBindings))
	convergenceBySource := make(map[string]workspacemodel.DefinitionOwnerFactConvergenceBinding, len(candidate.OwnerFactConvergenceBindings))
	for _, binding := range candidate.OwnerFactConvergenceBindings {
		value := binding.Convergence.Normalized()
		key := definitionOwnerFactBindingKey(binding.SourceRef, value.Requirement.OwnerModuleID, string(value.Requirement.FactKind), value.Requirement.OperationKind)
		counts[key]++
		if _, ok := required[key]; !ok || value.Validate() != nil {
			unresolved = append(unresolved, binding.SourceRef)
		}
		if _, duplicate := convergenceBySource[binding.SourceRef]; duplicate {
			unresolved = append(unresolved, binding.SourceRef)
		} else {
			convergenceBySource[binding.SourceRef] = binding
		}
	}
	for key, binding := range required {
		if counts[key] != 1 {
			unresolved = append(unresolved, binding.SourceRef)
		}
	}
	if cycle := ownerFactConvergenceCycle(convergenceBySource); len(cycle) > 0 {
		unresolved = append(unresolved, cycle...)
	}
	return normalizeStrings(unresolved)
}

func definitionOwnerFactBindingKey(sourceRef, owner, kind, operation string) string {
	return strings.Join([]string{strings.TrimSpace(sourceRef), strings.TrimSpace(owner), strings.TrimSpace(kind), strings.TrimSpace(operation)}, "|")
}

func ownerFactConvergenceCycle(values map[string]workspacemodel.DefinitionOwnerFactConvergenceBinding) []string {
	state := make(map[string]uint8, len(values))
	stack := make([]string, 0, len(values))
	var visit func(string) []string
	visit = func(ref string) []string {
		switch state[ref] {
		case 1:
			for idx := range stack {
				if stack[idx] == ref {
					return append([]string(nil), stack[idx:]...)
				}
			}
			return []string{ref}
		case 2:
			return nil
		}
		value, ok := values[ref]
		if !ok {
			return []string{ref}
		}
		state[ref] = 1
		stack = append(stack, ref)
		for _, dependency := range value.Convergence.Normalized().DependsOnRequirementRefs {
			if cycle := visit(dependency); len(cycle) > 0 {
				return cycle
			}
		}
		stack = stack[:len(stack)-1]
		state[ref] = 2
		return nil
	}
	refs := make([]string, 0, len(values))
	for ref := range values {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		if cycle := visit(ref); len(cycle) > 0 {
			return normalizeStrings(cycle)
		}
	}
	return nil
}

func clarification(candidate workspacemodel.DefinitionAdmissionCandidate, code string, refs []string, signature string) workspacemodel.DefinitionAdmissionResult {
	refs = normalizeStrings(refs)
	canonical := strings.Join(append([]string{code, signature}, refs...), "\x00")
	return (workspacemodel.DefinitionAdmissionResult{
		WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision,
		CandidateHash: candidate.CandidateHash, Disposition: workspacemodel.DefinitionAdmissionClarificationRequired,
		ViolationCode: code, OffendingRefs: refs, CanonicalViolation: canonical,
		CanonicalViolationHash:   stableHash("definition-violation", canonical),
		AdmissionCompilerVersion: candidate.AdmissionCompilerVersion,
	}).WithHash()
}

func hardDependencyCycle(relations []workspacemodel.DefinitionProposedRelationInput) []string {
	edges := map[string][]string{}
	for _, relation := range relations {
		kind := strings.ToLower(strings.TrimSpace(relation.RelationKind))
		if kind != "depends_on" && kind != "verification_of" && kind != "all_of" {
			continue
		}
		for _, dependent := range relation.TargetItemRefs {
			edges[dependent] = append(edges[dependent], relation.SourceItemRefs...)
		}
	}
	state := map[string]uint8{}
	stack := []string{}
	var visit func(string) []string
	visit = func(node string) []string {
		switch state[node] {
		case 1:
			for idx := range stack {
				if stack[idx] == node {
					return normalizeStrings(append([]string(nil), stack[idx:]...))
				}
			}
			return []string{node}
		case 2:
			return nil
		}
		state[node] = 1
		stack = append(stack, node)
		for _, next := range normalizeStrings(edges[node]) {
			if cycle := visit(next); len(cycle) > 0 {
				return cycle
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = 2
		return nil
	}
	nodes := make([]string, 0, len(edges))
	for node := range edges {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		if cycle := visit(node); len(cycle) > 0 {
			return cycle
		}
	}
	return nil
}

func stableHash(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = fmt.Fprintf(h, "%08x%s", len(part), part)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func normalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}
