package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	RuntimeImplementationWorkProtocolV1 = "runtime-implementation-work/v1"
	MaxRuntimeImplementationWorkRounds  = 8
	MaxRuntimeImplementationWorkTargets = 8

	// RuntimeImplementationWorkFileWorkspaceDeliverableSelectorRef identifies
	// the bounded owner-backed implementation-work scope. Its presence does not
	// grant a selected target, operation, dispatch or terminal credit.
	// It is a capability identity, not software-development behavior: File owns
	// the neutral bounded workspace scope and all concrete target decisions.
	RuntimeImplementationWorkFileWorkspaceDeliverableSelectorRef = "file/workspace_deliverable"
	RuntimeImplementationWorkConstraintWorkspaceRelativePath     = "workspace-relative-path"
	RuntimeImplementationWorkConstraintNewTargetOwnerSelector    = "new-target-requires-owner-selector"
)

// ErrRuntimeImplementationWorkUnavailable is a capability boundary, not an
// invitation to borrow owner-fact acquisition or rejected-validation repair.
// A syntactically valid policy is not an executable protocol. Only the exact
// integrated owner/selector/operation combination passes this static gate.
// Full runtime readiness is independently required by the public closure gate.
var ErrRuntimeImplementationWorkUnavailable = errors.New("runtime implementation work protocol is not executable")

// RuntimeImplementationWorkAvailability is a detached, deterministic report
// for admission/live tooling. It is deliberately not Runtime authority and
// cannot select an operation or transition.
type RuntimeImplementationWorkAvailability struct {
	Available bool
	Blockers  []RuntimeImplementationWorkBlocker
}

type RuntimeImplementationWorkBlocker struct {
	ProfileID     string
	RequirementID string
	Reason        string
}

// ExplainRuntimeImplementationWorkAvailability exposes the exact reason an
// declared combination is unsupported; this is not a fullflow test result.
func ExplainRuntimeImplementationWorkAvailability(profiles []RuntimeProfileContract) RuntimeImplementationWorkAvailability {
	report := RuntimeImplementationWorkAvailability{Available: true}
	for _, profile := range NormalizeRuntimeProfileContracts(profiles) {
		for _, requirement := range profile.EvidenceRequirements {
			if requirement.ImplementationWorkPolicy == nil {
				continue
			}
			policy := requirement.ImplementationWorkPolicy
			reason := ErrRuntimeImplementationWorkUnavailable.Error() + ": unsupported owner scope or operation combination"
			if err := policy.Validate(); err != nil {
				reason = fmt.Sprintf("invalid implementation work declaration: %v", err)
			} else if implementationWorkProtocolSupported(*policy) {
				continue
			}
			report.Available = false
			report.Blockers = append(report.Blockers, RuntimeImplementationWorkBlocker{
				ProfileID: profile.ID, RequirementID: requirement.ID,
				Reason: reason,
			})
		}
	}
	return report
}

// Only the integrated owner selector and operation inventory are executable.
// This is not a target grant: every round still requires an exact owner scope,
// independent feasibility, a working commitment and a fresh terminal proof.
func implementationWorkProtocolSupported(policy RuntimeImplementationWorkPolicy) bool {
	p := policy.Normalized()
	if p.OwnerModuleID != RuntimeModuleFile || p.TargetSelectorRef != RuntimeImplementationWorkFileWorkspaceDeliverableSelectorRef || len(p.TargetSelectorCandidateRefs) != 0 {
		return false
	}
	write, edit := false, false
	for _, operation := range p.AllowedOperations {
		if operation.VariantID != "" || (operation.Kind != StepKindFileWrite && operation.Kind != StepKindFileEdit) {
			return false
		}
		write = write || operation.Kind == StepKindFileWrite
		edit = edit || operation.Kind == StepKindFileEdit
	}
	prospective := slices.Contains(p.AllowedTargetPolicies, RuntimeOperationTargetPolicyProspectiveOwnerTarget)
	existing := slices.Contains(p.AllowedTargetPolicies, RuntimeOperationTargetPolicyExactTarget) || slices.Contains(p.AllowedTargetPolicies, RuntimeOperationTargetPolicyExistingOwnerScope)
	// File write is create-only; edit is revision-bound to an existing target.
	// Do not advertise an impossible operation/policy pair as executable.
	if write != prospective || edit != existing {
		return false
	}
	if prospective {
		return SameRuntimeProspectiveTargetAllOfConstraintRefs(p.ProspectiveTargetAllOfConstraintRefs, []string{
			RuntimeImplementationWorkConstraintWorkspaceRelativePath,
			RuntimeImplementationWorkConstraintNewTargetOwnerSelector,
		})
	}
	return len(p.ProspectiveTargetAllOfConstraintRefs) == 0
}

// RuntimeImplementationWorkPolicy is an immutable Profile proposal boundary.
// It never contains a selected target, owner observation, next action, credit
// or retry state. Terminal proof remains on the original evidence requirement.
// The first protocol bounds work to one declared resource owner and requires
// fresh observation and validation, without permitting published-input edits.
type RuntimeImplementationWorkPolicy struct {
	ProtocolRef                          string                         `json:"protocolRef"`
	OwnerModuleID                        string                         `json:"ownerModuleId"`
	AllowedOperations                    []RuntimeOperationVariantRef   `json:"allowedOperations"`
	AllowedTargetPolicies                []RuntimeOperationTargetPolicy `json:"allowedTargetPolicies"`
	TargetSelectorRef                    string                         `json:"targetSelectorRef"`
	TargetSelectorOwnerRef               string                         `json:"targetSelectorOwnerRef"`
	TargetSelectorSubjectRef             string                         `json:"targetSelectorSubjectRef"`
	TargetSelectorCandidateRefs          []string                       `json:"targetSelectorCandidateRefs"`
	ProspectiveTargetAllOfConstraintRefs []string                       `json:"prospectiveTargetAllOfConstraintRefs"`
	MaxRoundsPerCWI                      int                            `json:"maxRoundsPerCWI"`
	MaxTargetsPerRound                   int                            `json:"maxTargetsPerRound"`
	RequireFreshObservation              bool                           `json:"requireFreshObservation"`
	RequireFreshValidation               bool                           `json:"requireFreshValidation"`
	ProtectValidationInputs              bool                           `json:"protectValidationInputs"`
}

// Clone preserves even invalid raw declarations; admission, not cloning, owns
// their rejection. In particular, blanks and duplicates must not disappear.
func (p RuntimeImplementationWorkPolicy) Clone() RuntimeImplementationWorkPolicy {
	p.AllowedOperations = slices.Clone(p.AllowedOperations)
	p.AllowedTargetPolicies = slices.Clone(p.AllowedTargetPolicies)
	p.TargetSelectorCandidateRefs = slices.Clone(p.TargetSelectorCandidateRefs)
	p.ProspectiveTargetAllOfConstraintRefs = slices.Clone(p.ProspectiveTargetAllOfConstraintRefs)
	return p
}

func cloneRuntimeImplementationWorkPolicy(p *RuntimeImplementationWorkPolicy) *RuntimeImplementationWorkPolicy {
	if p == nil {
		return nil
	}
	v := p.Clone()
	return &v
}

func (p RuntimeImplementationWorkPolicy) Normalized() RuntimeImplementationWorkPolicy {
	p = p.Clone()
	p.ProtocolRef = strings.TrimSpace(p.ProtocolRef)
	p.OwnerModuleID = strings.TrimSpace(p.OwnerModuleID)
	p.TargetSelectorRef = strings.TrimSpace(p.TargetSelectorRef)
	p.TargetSelectorOwnerRef = strings.TrimSpace(p.TargetSelectorOwnerRef)
	p.TargetSelectorSubjectRef = strings.TrimSpace(p.TargetSelectorSubjectRef)
	for i := range p.AllowedOperations {
		p.AllowedOperations[i].Kind = CanonicalRuntimeBlockKind(p.AllowedOperations[i].Kind)
		p.AllowedOperations[i].VariantID = strings.TrimSpace(p.AllowedOperations[i].VariantID)
	}
	sort.SliceStable(p.AllowedOperations, func(i, j int) bool {
		l, r := p.AllowedOperations[i], p.AllowedOperations[j]
		if l.Kind != r.Kind {
			return l.Kind < r.Kind
		}
		return l.VariantID < r.VariantID
	})
	for i := range p.AllowedTargetPolicies {
		p.AllowedTargetPolicies[i] = RuntimeOperationTargetPolicy(strings.TrimSpace(string(p.AllowedTargetPolicies[i])))
	}
	slices.Sort(p.AllowedTargetPolicies)
	for _, refs := range [][]string{p.TargetSelectorCandidateRefs, p.ProspectiveTargetAllOfConstraintRefs} {
		for i := range refs {
			refs[i] = strings.TrimSpace(refs[i])
		}
		slices.Sort(refs)
	}
	p.AllowedOperations = nilIfEmpty(p.AllowedOperations)
	p.AllowedTargetPolicies = nilIfEmpty(p.AllowedTargetPolicies)
	p.TargetSelectorCandidateRefs = nilIfEmpty(p.TargetSelectorCandidateRefs)
	p.ProspectiveTargetAllOfConstraintRefs = nilIfEmpty(p.ProspectiveTargetAllOfConstraintRefs)
	return p
}

func (p RuntimeImplementationWorkPolicy) Validate() error {
	p = p.Normalized()
	if p.ProtocolRef != RuntimeImplementationWorkProtocolV1 {
		return fmt.Errorf("implementation work requires protocol %q", RuntimeImplementationWorkProtocolV1)
	}
	for _, ref := range []string{p.OwnerModuleID, p.TargetSelectorRef, p.TargetSelectorOwnerRef, p.TargetSelectorSubjectRef} {
		if !validRuntimeImplementationWorkRef(ref) {
			return fmt.Errorf("implementation work requires bounded exact owner and declaration selector identities")
		}
	}
	if p.MaxRoundsPerCWI < 1 || p.MaxRoundsPerCWI > MaxRuntimeImplementationWorkRounds ||
		p.MaxTargetsPerRound < 1 || p.MaxTargetsPerRound > MaxRuntimeImplementationWorkTargets {
		return fmt.Errorf("implementation work requires finite rounds and targets in 1..8")
	}
	if !p.RequireFreshObservation || !p.RequireFreshValidation || !p.ProtectValidationInputs {
		return fmt.Errorf("implementation work requires fresh observation, fresh validation and validation-input protection")
	}
	if len(p.AllowedOperations) == 0 || len(p.AllowedOperations) > 8 {
		return fmt.Errorf("implementation work requires 1..8 declared owner operations")
	}
	for i, op := range p.AllowedOperations {
		if i > 0 && op == p.AllowedOperations[i-1] {
			return fmt.Errorf("implementation work duplicates an operation declaration")
		}
		d, found := RuntimeOperationDescriptorForKind(op.Kind)
		if !found || d.OwnerModuleID != p.OwnerModuleID || !d.RequiresOwnerReceipt || d.DefaultEffectScope != RuntimeEffectWorkspaceDurable {
			return fmt.Errorf("implementation work operation %q requires the declared workspace-effect receipt owner", op.Kind)
		}
		contract, found := RuntimeOperationContractForKind(op.Kind)
		if !found || !contract.RequireTargetRefs {
			return fmt.Errorf("implementation work operation %q requires a target-bound static contract", op.Kind)
		}
		variantFound := len(contract.Variants) == 0 && op.VariantID == ""
		for _, variant := range contract.Variants {
			if variant.ID == op.VariantID {
				variantFound = true
			}
		}
		if !variantFound {
			return fmt.Errorf("implementation work operation %q has an undeclared variant", op.Kind)
		}
	}
	if len(p.AllowedTargetPolicies) == 0 || len(p.AllowedTargetPolicies) > 3 {
		return fmt.Errorf("implementation work requires bounded target policies")
	}
	for i, policy := range p.AllowedTargetPolicies {
		if i > 0 && policy == p.AllowedTargetPolicies[i-1] {
			return fmt.Errorf("implementation work duplicates a target policy")
		}
		switch policy {
		case RuntimeOperationTargetPolicyExactTarget, RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		default:
			return fmt.Errorf("implementation work has unsupported target policy %q", policy)
		}
	}
	for _, refs := range [][]string{p.TargetSelectorCandidateRefs, p.ProspectiveTargetAllOfConstraintRefs} {
		if len(refs) > 8 {
			return fmt.Errorf("implementation work declaration exceeds 8 scope refs")
		}
		for i, ref := range refs {
			if !validRuntimeImplementationWorkRef(ref) || (i > 0 && ref == refs[i-1]) {
				return fmt.Errorf("implementation work scope refs must be exact, bounded and unique")
			}
		}
	}
	prospective := slices.Contains(p.AllowedTargetPolicies, RuntimeOperationTargetPolicyProspectiveOwnerTarget)
	if prospective && len(p.TargetSelectorCandidateRefs) == 0 && len(p.ProspectiveTargetAllOfConstraintRefs) == 0 {
		return fmt.Errorf("implementation work prospective scope requires bounded candidates or mandatory all-of constraints")
	}
	if !prospective && len(p.ProspectiveTargetAllOfConstraintRefs) != 0 {
		return fmt.Errorf("implementation work prospective constraints require a prospective target policy")
	}
	return nil
}

func validRuntimeImplementationWorkRef(ref string) bool {
	return ref != "" && len(ref) <= 512 && utf8.ValidString(ref) && !strings.Contains(ref, "*") && strings.IndexFunc(ref, unicode.IsControl) < 0
}

func SameRuntimeImplementationWorkPolicy(left, right *RuntimeImplementationWorkPolicy) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return reflect.DeepEqual(left.Normalized(), right.Normalized())
}

func StableRuntimeImplementationWorkPolicyHash(p RuntimeImplementationWorkPolicy) string {
	data, _ := json.Marshal(p.Normalized())
	digest := sha256.Sum256(data)
	return "implementation-work:" + hex.EncodeToString(digest[:])
}

// ValidateRuntimeImplementationWorkAvailability separates declaration validity
// from executable support. Both Workspace promotion and Runtime activation use
// this gate; no adapter can silently omit a requested but unimplemented policy.
func ValidateRuntimeImplementationWorkAvailability(profiles []RuntimeProfileContract) error {
	report := ExplainRuntimeImplementationWorkAvailability(profiles)
	if len(report.Blockers) != 0 {
		blocker := report.Blockers[0]
		return fmt.Errorf("%w: profile %q requirement %q", ErrRuntimeImplementationWorkUnavailable, blocker.ProfileID, blocker.RequirementID)
	}
	return nil
}
