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
	"strconv"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
	if candidate.Specification != nil {
		if refs := specificationAdmissionViolations(candidate); len(refs) > 0 {
			return clarification(candidate, workspacemodel.DefinitionAdmissionSpecificationIncomplete, refs, "goal_specification_incomplete"), nil
		}
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
	profilePrerequisiteRefs := append(unresolvedOwnerFactConvergence(candidate), mismatchedProfileOwnerFactBindings(candidate)...)
	if unresolved := normalizeStrings(profilePrerequisiteRefs); len(unresolved) > 0 {
		return clarification(candidate, workspacemodel.DefinitionAdmissionProfilePrerequisiteUnresolved, unresolved, "profile_owner_fact_convergence_incomplete"), nil
	}
	if violations := runtimecontract.RuntimeProfileProofClosureViolations(candidate.RuntimeProfileContracts); len(violations) > 0 {
		refs := make([]string, 0, len(violations))
		rules := make([]string, 0, len(violations))
		for _, violation := range violations {
			refs = append(refs, violation.SourceRef())
			rules = append(rules, violation.SourceRef()+"="+violation.Rule)
		}
		return clarification(candidate, workspacemodel.DefinitionAdmissionProfileContractInvalid, refs, "profile_proof_closure_incomplete:"+strings.Join(normalizeStrings(rules), ",")), nil
	}
	if err := runtimecontract.ValidateRuntimeProfileContracts(candidate.RuntimeProfileContracts); err != nil {
		return clarification(candidate, workspacemodel.DefinitionAdmissionProfileContractInvalid, runtimeProfileRefs(candidate.RuntimeProfileContracts), "runtime_profile_contract_invalid"), nil
	}
	if err := runtimecontract.ValidateRuntimeImplementationWorkAvailability(candidate.RuntimeProfileContracts); err != nil {
		return clarification(candidate, workspacemodel.DefinitionAdmissionProfileContractInvalid, runtimeProfileRefs(candidate.RuntimeProfileContracts), "implementation_work_protocol_unavailable"), nil
	}
	result := workspacemodel.DefinitionAdmissionResult{
		WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision,
		CandidateHash: candidate.CandidateHash, Disposition: workspacemodel.DefinitionAdmissionAdmittedAllOf,
		AdmissionCompilerVersion:   candidate.AdmissionCompilerVersion,
		CanonicalAdmittedInputHash: stableHash("admitted_all_of", candidate.CandidateHash, candidate.AdmissionCompilerVersion),
	}.WithHash()
	return result, result.ValidateFor(candidate)
}

func specificationAdmissionViolations(candidate workspacemodel.DefinitionAdmissionCandidate) []string {
	spec := candidate.Specification.Normalized()
	violations := make([]string, 0)
	if len(spec.Dimensions) == 0 {
		violations = append(violations, "dimension/missing")
	}
	if len(spec.Requirements) == 0 {
		violations = append(violations, "requirement/missing")
	}
	for _, resource := range spec.Resources {
		if resource.Required && (!resource.Available || resource.SnapshotHash == "") {
			violations = append(violations, "resource/"+resource.ID)
		}
	}
	for _, dimension := range spec.Dimensions {
		if !dimension.Required {
			continue
		}
		if !dimension.Activated {
			violations = append(violations, "dimension/"+dimension.ID)
		}
		found := false
		for _, requirement := range spec.Requirements {
			if requirement.DimensionID == dimension.ID && requirement.Required {
				found = true
			}
		}
		if !found {
			violations = append(violations, "dimension/"+dimension.ID+"/requirement")
		}
	}
	mappedDoD := make(map[string]bool, len(candidate.DoDInputs))
	goalSourceCount := 0
	for _, requirement := range spec.Requirements {
		for _, ref := range requirement.SourceItemRefs {
			if ref == spec.GoalItemRef {
				goalSourceCount++
			}
		}
	}
	for _, requirement := range spec.Requirements {
		if !requirement.Required {
			continue
		}
		if requirement.Text == "" || len(requirement.SourceItemRefs) == 0 {
			violations = append(violations, "requirement/"+requirement.ID+"/source")
		}
		found := false
		for _, obligation := range spec.Obligations {
			if obligation.RequirementID == requirement.ID && obligation.Text != "" {
				found = true
				for _, ref := range obligation.DoDItemRefs {
					mappedDoD[ref] = true
				}
			}
		}
		// 独立 exact Goal 不是 DoD obligation；其来源、原文及六审仍必须完整。
		independentGoal := goalSourceCount == 1 && len(requirement.SourceItemRefs) == 1 && requirement.SourceItemRefs[0] == spec.GoalItemRef && requirement.SourceKind == workspacemodel.GoalRequirementUserExplicit && requirement.Text == candidate.GoalInput.Text
		if !found && !independentGoal {
			violations = append(violations, "requirement/"+requirement.ID+"/obligation")
		}
	}
	for _, obligation := range spec.Obligations {
		if specRequirementIsRequired(spec.Requirements, obligation.RequirementID) && len(obligation.DoDItemRefs) == 0 {
			violations = append(violations, "obligation/"+obligation.ID+"/dod_mapping")
		}
		found := false
		for _, verification := range spec.Verifications {
			if verification.ObligationID != obligation.ID {
				continue
			}
			if verification.Declaration != "" && verification.Status == workspacemodel.GoalVerificationDeclared && specificationVerificationProfileMatches(candidate, verification) {
				found = true
			} else {
				violations = append(violations, "verification/"+verification.ID)
			}
		}
		if !found {
			violations = append(violations, "obligation/"+obligation.ID+"/verification")
		}
	}
	for _, dod := range candidate.DoDInputs {
		if !mappedDoD[dod.CandidateItemRef] {
			violations = append(violations, "dod/"+dod.CandidateItemRef)
		}
	}
	for _, review := range spec.Reviews {
		if review.Status != workspacemodel.GoalReviewPass {
			violations = append(violations, "review/"+review.Kind)
		}
	}
	return normalizeStrings(violations)
}

func specRequirementIsRequired(requirements []workspacemodel.GoalSpecificationRequirement, id string) bool {
	for _, requirement := range requirements {
		if requirement.ID == id {
			return requirement.Required
		}
	}
	return false
}

// The graph is Workspace-owned. Resolve every DoD of this obligation to its
// formal after-image source before asking the neutral compiler about applicability.
func specificationVerificationProfileMatches(candidate workspacemodel.DefinitionAdmissionCandidate, verification workspacemodel.GoalSpecificationVerification) bool {
	return SpecificationVerificationApplies(*candidate.Specification, verification, candidate.DoDInputs, candidate.RuntimeProfileContracts)
}

// Candidate normalization sorts by ref. That order is not the confirmed DoD
// after-image order: user, template, generated are materialized in that order.
// Formal references encode their original ordinal. Unknown identities fail closed.
func specificationDoDSources(items []workspacemodel.DefinitionCandidateItem) (map[string]string, bool) {
	result := make(map[string]string, len(items))
	groups := map[string]map[int]string{"user": {}, "template": {}, "generated": {}}
	derived, formal := false, false
	formalOrdinals := make(map[int]bool)
	for _, item := range items {
		if strings.HasPrefix(item.CandidateItemRef, "derived-dod:") {
			derived = true
			parts := strings.Split(item.CandidateItemRef, ":")
			if len(parts) != 3 {
				return nil, false
			}
			group, exists := groups[parts[1]]
			ordinal, err := strconv.Atoi(parts[2])
			if !exists || err != nil || ordinal < 1 || parts[2] != fmt.Sprintf("%06d", ordinal) || group[ordinal] != "" {
				return nil, false
			}
			group[ordinal] = item.CandidateItemRef
			continue
		}
		formal = true
		parts := strings.Split(item.CandidateItemRef, ":item:")
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "dod:") || len(parts[0]) <= len("dod:") {
			return nil, false
		}
		ordinal, err := strconv.Atoi(parts[1])
		if err != nil || ordinal < 1 || parts[1] != fmt.Sprintf("%06d", ordinal) || formalOrdinals[ordinal] {
			return nil, false
		}
		formalOrdinals[ordinal] = true
		result[item.CandidateItemRef] = fmt.Sprintf("dod-%03d", ordinal)
	}
	if derived && formal {
		return nil, false
	}
	if derived {
		ordinal := 0
		for _, kind := range []string{"user", "template", "generated"} {
			for n := 1; n <= len(groups[kind]); n++ {
				ref := groups[kind][n]
				if ref == "" {
					return nil, false
				}
				ordinal++
				result[ref] = fmt.Sprintf("dod-%03d", ordinal)
			}
		}
	} else {
		for n := 1; n <= len(items); n++ {
			if !formalOrdinals[n] {
				return nil, false
			}
		}
	}
	return result, len(result) == len(items)
}

// CompileProfileOwnerFactBindings derives the exact pre-seal manifest from a
// normalized Profile snapshot. Application may use this pure helper to author
// a candidate, while Workspace Compile independently derives and compares the
// same manifest before selecting an owner-local admission transition.
func CompileProfileOwnerFactBindings(profiles []runtimecontract.RuntimeProfileContract) ([]workspacemodel.DefinitionRequiredOwnerFactBinding, []workspacemodel.DefinitionOwnerFactConvergenceBinding) {
	required := make([]workspacemodel.DefinitionRequiredOwnerFactBinding, 0)
	convergences := make([]workspacemodel.DefinitionOwnerFactConvergenceBinding, 0)
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
		for _, evidence := range profile.EvidenceRequirements {
			sourceByRequirement := make(map[string]string, len(evidence.RequiredOwnerFacts))
			for _, fact := range evidence.RequiredOwnerFacts {
				sourceRef := definitionOwnerFactSourceRef(profile.ID, evidence.Scope, evidence.ID, fact)
				required = append(required, workspacemodel.DefinitionRequiredOwnerFactBinding{SourceRef: sourceRef, Requirement: fact})
				sourceByRequirement[runtimecontract.RuntimeOwnerFactRequirementRef(fact)] = sourceRef
			}
			for _, convergence := range evidence.OwnerFactConvergences {
				sourceRef := definitionOwnerFactSourceRef(profile.ID, evidence.Scope, evidence.ID, convergence.Requirement)
				convergence = convergence.Normalized()
				for idx, dependency := range convergence.DependsOnRequirementRefs {
					if mapped := sourceByRequirement[dependency]; mapped != "" {
						convergence.DependsOnRequirementRefs[idx] = mapped
					}
				}
				convergences = append(convergences, workspacemodel.DefinitionOwnerFactConvergenceBinding{SourceRef: sourceRef, Convergence: convergence})
			}
		}
	}
	sort.Slice(required, func(i, j int) bool {
		return definitionRequiredOwnerFactKey(required[i]) < definitionRequiredOwnerFactKey(required[j])
	})
	sort.Slice(convergences, func(i, j int) bool {
		return definitionOwnerFactConvergenceKey(convergences[i]) < definitionOwnerFactConvergenceKey(convergences[j])
	})
	return required, convergences
}

func mismatchedProfileOwnerFactBindings(candidate workspacemodel.DefinitionAdmissionCandidate) []string {
	expectedRequired, expectedConvergences := CompileProfileOwnerFactBindings(candidate.RuntimeProfileContracts)
	refs := make([]string, 0)
	requiredCounts := make(map[string]int, len(candidate.RequiredOwnerFactBindings))
	for _, binding := range candidate.RequiredOwnerFactBindings {
		requiredCounts[definitionRequiredOwnerFactKey(binding)]++
	}
	expectedRequiredKeys := make(map[string]struct{}, len(expectedRequired))
	for _, binding := range expectedRequired {
		key := definitionRequiredOwnerFactKey(binding)
		expectedRequiredKeys[key] = struct{}{}
		if requiredCounts[key] != 1 {
			refs = append(refs, binding.SourceRef)
		}
	}
	for _, binding := range candidate.RequiredOwnerFactBindings {
		if _, expected := expectedRequiredKeys[definitionRequiredOwnerFactKey(binding)]; !expected {
			refs = append(refs, binding.SourceRef)
		}
	}

	actualConvergences := make(map[string][]runtimecontract.RuntimeOwnerFactConvergence, len(candidate.OwnerFactConvergenceBindings))
	for _, binding := range candidate.OwnerFactConvergenceBindings {
		key := definitionOwnerFactConvergenceKey(binding)
		actualConvergences[key] = append(actualConvergences[key], binding.Convergence)
	}
	expectedConvergenceKeys := make(map[string]struct{}, len(expectedConvergences))
	for _, binding := range expectedConvergences {
		key := definitionOwnerFactConvergenceKey(binding)
		expectedConvergenceKeys[key] = struct{}{}
		actual := actualConvergences[key]
		if len(actual) != 1 || !runtimecontract.SameRuntimeOwnerFactConvergences(actual, []runtimecontract.RuntimeOwnerFactConvergence{binding.Convergence}) {
			refs = append(refs, binding.SourceRef)
		}
	}
	for _, binding := range candidate.OwnerFactConvergenceBindings {
		if _, expected := expectedConvergenceKeys[definitionOwnerFactConvergenceKey(binding)]; !expected {
			refs = append(refs, binding.SourceRef)
		}
	}
	return normalizeStrings(refs)
}

func definitionRequiredOwnerFactKey(binding workspacemodel.DefinitionRequiredOwnerFactBinding) string {
	value := binding.Requirement.Normalized()
	return definitionOwnerFactBindingKey(binding.SourceRef, value.OwnerModuleID, string(value.FactKind), value.OperationKind)
}

func definitionOwnerFactConvergenceKey(binding workspacemodel.DefinitionOwnerFactConvergenceBinding) string {
	value := binding.Convergence.Normalized().Requirement
	return definitionOwnerFactBindingKey(binding.SourceRef, value.OwnerModuleID, string(value.FactKind), value.OperationKind)
}

func definitionOwnerFactSourceRef(profileID, scope, requirementID string, fact runtimecontract.RuntimeOwnerFactRequirement) string {
	fact = fact.Normalized()
	return strings.Join([]string{"profile", strings.TrimSpace(profileID), strings.TrimSpace(scope), strings.TrimSpace(requirementID), fact.OwnerModuleID, string(fact.FactKind), fact.OperationKind}, "/")
}

func runtimeProfileRefs(profiles []runtimecontract.RuntimeProfileContract) []string {
	refs := make([]string, 0, len(profiles))
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
		refs = append(refs, "profile/"+profile.ID)
	}
	return normalizeStrings(refs)
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
