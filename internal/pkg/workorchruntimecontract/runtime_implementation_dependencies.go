package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// Dependencies are semantic nominations, not existence, feasibility or permission
// facts. The complete current owner scope supplies those independent boundaries.
type RuntimeImplementationUncertainty struct {
	Kind       string   `json:"kind"`
	TargetRefs []string `json:"targetRefs,omitempty"`
}

const (
	RuntimeImplementationSemanticInformationMissing = "semantic_information_missing"
	RuntimeImplementationTargetScopeUnavailable     = "target_scope_unavailable"
)

func (u *RuntimeImplementationUncertainty) Clone() *RuntimeImplementationUncertainty {
	if u == nil {
		return nil
	}
	c := *u
	c.TargetRefs = slices.Clone(u.TargetRefs)
	return &c
}
func validateImplementationDependencyRefs(refs []string) error {
	if len(refs) > MaxRuntimeImplementationWorkTargets {
		return fmt.Errorf("dependency target set exceeds bounded work context")
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if !implementationWorkExactRef(ref) || seen[ref] {
			return fmt.Errorf("dependency targets require unique exact references")
		}
		seen[ref] = true
	}
	return nil
}
func validateRuntimeInputDependencies(c RuntimeValidationInputCheckReview) error {
	if err := validateImplementationDependencyRefs(c.DependencyTargetRefs); err != nil {
		return err
	}
	if len(c.DependencyTargetRefs) != 0 && c.Conclusion != RuntimeValidationInputConflict {
		return fmt.Errorf("unresolved dependencies require a conflicting input check")
	}
	return nil
}
func runtimeImplementationTargetRefsSchema() map[string]any {
	return map[string]any{"type": "array", "maxItems": MaxRuntimeImplementationWorkTargets, "uniqueItems": true, "items": map[string]any{"type": "string", "minLength": 1}}
}

// One membership rule for purpose claims, semantic proposals and sealed working
// bindings. Eligibility only permits proposing an operation; concrete capability
// feasibility, permissions, CAS and fences still belong to their original owners.
func implementationTargetEligible(scope RuntimeImplementationWorkScopeObservation, excluded []string, ref string, targetPolicy RuntimeOperationTargetPolicy) bool {
	p := scope.Request.Policy
	if !implementationWorkExactRef(ref) || slices.Contains(excluded, ref) || !slices.Contains(p.AllowedTargetPolicies, targetPolicy) || (len(p.TargetSelectorCandidateRefs) != 0 && !slices.Contains(p.TargetSelectorCandidateRefs, ref)) {
		return false
	}
	switch targetPolicy {
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		return len(p.ProspectiveTargetAllOfConstraintRefs) != 0 && !slices.Contains(scope.Request.ManifestBinding.Manifest.TargetRefs, ref)
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		return slices.Contains(scope.Selection.TargetRefs, ref)
	}
	return false
}
func implementationTargetPolicies(scope RuntimeImplementationWorkScopeObservation, excluded []string, ref string) []RuntimeOperationTargetPolicy {
	var result []RuntimeOperationTargetPolicy
	for _, p := range scope.Request.Policy.AllowedTargetPolicies {
		if implementationTargetEligible(scope, excluded, ref, p) {
			result = append(result, p)
		}
	}
	return result
}

// TargetOptions is a provider projection of current scope membership and the
// sealed candidate vocabulary, never an operation authorization.
type RuntimeImplementationTargetOptions struct {
	ExistingTargetRefs             []string                       `json:"existingTargetRefs"`
	ProspectiveTargetPolicies      []RuntimeOperationTargetPolicy `json:"prospectiveTargetPolicies"`
	ProspectiveCandidateRefs       []string                       `json:"prospectiveCandidateRefs,omitempty"`
	ProspectiveAllOfConstraintRefs []string                       `json:"prospectiveAllOfConstraintRefs,omitempty"`
}

func (w RuntimeSemanticImplementationWork) TargetOptions() RuntimeImplementationTargetOptions {
	p := w.Scope.Request.Policy
	o := RuntimeImplementationTargetOptions{}
	for _, ref := range w.Scope.Selection.TargetRefs {
		if implementationTargetEligible(w.Scope, w.ExcludedTargetRefs, ref, RuntimeOperationTargetPolicyExistingOwnerScope) {
			o.ExistingTargetRefs = append(o.ExistingTargetRefs, ref)
		}
	}
	if slices.Contains(p.AllowedTargetPolicies, RuntimeOperationTargetPolicyProspectiveOwnerTarget) {
		o.ProspectiveTargetPolicies = []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget}
		o.ProspectiveCandidateRefs = slices.Clone(p.TargetSelectorCandidateRefs)
		o.ProspectiveAllOfConstraintRefs = slices.Clone(p.ProspectiveTargetAllOfConstraintRefs)
	}
	return o
}

type RuntimeImplementationDependencyTask struct {
	RequiredCheckRef       string                         `json:"requiredCheckRef"`
	TargetRef              string                         `json:"targetRef"`
	PresentInOwnerManifest bool                           `json:"presentInOwnerManifest"`
	CandidatePolicies      []RuntimeOperationTargetPolicy `json:"candidatePolicies"`
}

func (w RuntimeSemanticImplementationWork) DependencyTasks() []RuntimeImplementationDependencyTask {
	v := w.Scope.Request.InputRevision
	if v == nil {
		return nil
	}
	var tasks []RuntimeImplementationDependencyTask
	for _, c := range v.Report.Checks {
		for _, ref := range c.DependencyTargetRefs {
			tasks = append(tasks, RuntimeImplementationDependencyTask{RequiredCheckRef: c.RequiredCheckRef, TargetRef: ref, PresentInOwnerManifest: slices.Contains(w.Scope.Request.ManifestBinding.Manifest.TargetRefs, ref), CandidatePolicies: implementationTargetPolicies(w.Scope, w.ExcludedTargetRefs, ref)})
		}
	}
	return tasks
}
func validateImplementationResourceClaims(d RuntimeSemanticPlanDocument, w *RuntimeSemanticImplementationWork) error {
	i := d.ImplementationIntent
	if err := validateImplementationDependencyRefs(i.DependencyTargetRefs); err != nil {
		return err
	}
	if len(i.DependencyTargetRefs) != 0 && d.ImplementationPurpose != RuntimeImplementationWorkPurposeEffect {
		return fmt.Errorf("only effect purpose can select dependency targets")
	}
	if len(i.DependencyTargetRefs) > w.Scope.Request.Policy.MaxTargetsPerRound {
		return fmt.Errorf("selected dependencies exceed round limit")
	}
	for _, ref := range i.DependencyTargetRefs {
		if len(implementationTargetPolicies(w.Scope, w.ExcludedTargetRefs, ref)) == 0 {
			return fmt.Errorf("selected dependency is outside current work policy")
		}
	}
	u := i.Uncertainty
	if d.ImplementationPurpose != RuntimeImplementationWorkPurposeInconclusive {
		if u != nil {
			return fmt.Errorf("uncertainty requires inconclusive purpose")
		}
		return nil
	}
	if u == nil {
		return fmt.Errorf("inconclusive purpose requires typed uncertainty")
	}
	if err := validateImplementationDependencyRefs(u.TargetRefs); err != nil {
		return err
	}
	switch u.Kind {
	case RuntimeImplementationSemanticInformationMissing:
		// This is a semantic insufficiency, never a claim about scope permission.
		return nil
	case RuntimeImplementationTargetScopeUnavailable:
		if len(u.TargetRefs) == 0 {
			return fmt.Errorf("scope uncertainty requires exact affected targets")
		}
		if w.EffectAvailable {
			for _, ref := range u.TargetRefs {
				if len(implementationTargetPolicies(w.Scope, w.ExcludedTargetRefs, ref)) != 0 {
					return &RuntimeSemanticProposalViolationError{ViolationFact: RuntimeSemanticContractViolation{Kind: RuntimeSemanticContractViolationImplementationIntent, CoverageWorkItemID: w.Scope.Request.CoverageWorkItemID, Path: "$.implementationIntent.uncertainty.targetRefs", Rule: RuntimeSemanticConstraintRelational, Actual: "owner_eligible_target_reported_unavailable", Required: []string{ref, "propose bounded dependency work under the current owner policy"}}, Cause: fmt.Errorf("scope uncertainty contradicts owner candidate eligibility for %q", ref)}
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown implementation uncertainty kind")
	}
}
func validateSelectedDependencyProposals(intent *RuntimeImplementationIntent, proposals []RuntimeSemanticProofProposal) error {
	if intent == nil {
		return nil
	}
	for _, ref := range intent.DependencyTargetRefs {
		found := false
		for _, p := range proposals {
			found = found || slices.Contains(RuntimeSemanticProposalTargetRefs(p), ref)
		}
		if !found {
			return fmt.Errorf("implementation proof omits selected dependency %q", ref)
		}
	}
	return nil
}

func validateRuntimeDependencyRefsPayload(raw json.RawMessage) error {
	var refs []string
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &refs) != nil {
		return fmt.Errorf("dependency targets must be an array")
	}
	return validateImplementationDependencyRefs(refs)
}
