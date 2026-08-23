package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const DefinitionAdmissionCandidateSchemaVersionCurrent uint32 = 2

const DefinitionAdmissionCompilerVersionCurrent = "workorch-definition-admission-v2"

type DefinitionAdmissionDisposition string

const (
	DefinitionAdmissionAdmittedAllOf             DefinitionAdmissionDisposition = "admitted_all_of"
	DefinitionAdmissionClarificationRequired     DefinitionAdmissionDisposition = "definition_clarification_required"
	DefinitionAdmissionOneOfUnsupported          string                         = "definition_one_of_unsupported"
	DefinitionAdmissionPrerequisiteUnresolved    string                         = "declared_prerequisite_unresolved"
	DefinitionAdmissionTerminalPolicyUnsupported string                         = "definition_terminal_policy_unsupported"
)

type DefinitionCandidateItem struct {
	CandidateItemRef string
	Text             string
	SourceIdentity   string
}

type DefinitionTemplateSnapshot struct {
	TemplateRef  string
	Revision     string
	SnapshotHash string
}

type DefinitionAdmissionPolicyBinding struct {
	PolicyRef    string
	Revision     string
	SnapshotHash string
}

// DefinitionRequiredOwnerFactBinding is the exact pre-seal declaration of a
// profile-required capability-owner fact. SourceRef binds it to one immutable
// profile evidence requirement; it does not represent a live owner reading.
type DefinitionRequiredOwnerFactBinding struct {
	SourceRef   string
	Requirement runtimecontract.RuntimeOwnerFactRequirement
}

// DefinitionOwnerFactConvergenceBinding binds the static closure declaration
// for one required fact. The Workspace owner persists it as candidate input;
// only the pure compiler decides whether the declaration is complete.
type DefinitionOwnerFactConvergenceBinding struct {
	SourceRef   string
	Convergence runtimecontract.RuntimeOwnerFactConvergence
}

type DefinitionProposedRelationInput struct {
	RelationRef    string
	RelationKind   string
	SourceItemRefs []string
	TargetItemRefs []string
}

// DefinitionAdmissionCandidate is the Workspace-owned immutable pre-seal
// authoring attempt. Unsupported relation tokens may exist only here so the
// pure compiler can reject them deterministically before a formal Definition
// or Runtime activation is created.
type DefinitionAdmissionCandidate struct {
	SchemaVersion                     uint32
	WorkspaceID                       common.WorkspaceID
	CandidateID                       string
	CandidateRevision                 uint64
	BaseDefinitionRevision            string
	BaseStartupInputHash              string
	GoalInput                         DefinitionCandidateItem
	DoDInputs                         []DefinitionCandidateItem
	WorkspaceTemplateSnapshot         DefinitionTemplateSnapshot
	AdmissionPolicyBindings           []DefinitionAdmissionPolicyBinding
	RequiredOwnerFactBindings         []DefinitionRequiredOwnerFactBinding
	OwnerFactConvergenceBindings      []DefinitionOwnerFactConvergenceBinding
	RuntimeProfileContractsHash       string
	AdmissionCompilerVersion          string
	ResolvedProjectWorkspaceDirectory string
	ProposedRelationInputs            []DefinitionProposedRelationInput
	CandidateHash                     string
}

type DefinitionAdmissionResult struct {
	WorkspaceID                common.WorkspaceID
	CandidateID                string
	CandidateRevision          uint64
	CandidateHash              string
	Disposition                DefinitionAdmissionDisposition
	ViolationCode              string
	OffendingRefs              []string
	CanonicalViolation         string
	CanonicalViolationHash     string
	AdmissionCompilerVersion   string
	CanonicalAdmittedInputHash string
	AdmissionResultHash        string
}

type CandidateItemToItemRefMapping struct {
	CandidateItemRef string
	ItemRef          string
}

type WorkspaceStartupInputContract struct {
	WorkspaceID                       common.WorkspaceID
	DefinitionRevision                string
	GoalDefinitionRef                 string
	GoalTextHash                      string
	DoDDefinitionRefs                 []string
	DoDItemSetHash                    string
	WorkspaceTemplateRef              string
	TemplateRevision                  string
	TemplateSnapshotHash              string
	RuntimeProfileContractsHash       string
	ResolvedProjectWorkspaceDirectory string
	CandidateItemToItemRefMappings    []CandidateItemToItemRefMapping
	CandidateItemToItemRefMappingHash string
	StartupInputHash                  string
}

type WorkspaceActivationAdmissionState string

const (
	WorkspaceActivationEligible                       WorkspaceActivationAdmissionState = "eligible"
	WorkspaceActivationPendingClarificationResolution WorkspaceActivationAdmissionState = "pending_clarification_resolution"
)

type WorkspaceActivationAdmission struct {
	WorkspaceID                        common.WorkspaceID
	CandidateID                        string
	CandidateRevision                  uint64
	CandidateHash                      string
	DefinitionRevision                 string
	StartupInputHash                   string
	State                              WorkspaceActivationAdmissionState
	BlockingTicketID                   common.TicketID
	ClarificationResolutionID          common.ResolutionID
	ClarificationResolutionBindingHash string
}

func (c DefinitionAdmissionCandidate) Normalized() DefinitionAdmissionCandidate {
	c.CandidateID = strings.TrimSpace(c.CandidateID)
	c.BaseDefinitionRevision = strings.TrimSpace(c.BaseDefinitionRevision)
	c.BaseStartupInputHash = strings.TrimSpace(c.BaseStartupInputHash)
	c.GoalInput = normalizeCandidateItem(c.GoalInput)
	c.DoDInputs = normalizeCandidateItems(c.DoDInputs)
	c.WorkspaceTemplateSnapshot.TemplateRef = strings.TrimSpace(c.WorkspaceTemplateSnapshot.TemplateRef)
	c.WorkspaceTemplateSnapshot.Revision = strings.TrimSpace(c.WorkspaceTemplateSnapshot.Revision)
	c.WorkspaceTemplateSnapshot.SnapshotHash = strings.TrimSpace(c.WorkspaceTemplateSnapshot.SnapshotHash)
	c.AdmissionPolicyBindings = normalizeAdmissionPolicyBindings(c.AdmissionPolicyBindings)
	c.RequiredOwnerFactBindings = normalizeDefinitionRequiredOwnerFactBindings(c.RequiredOwnerFactBindings)
	c.OwnerFactConvergenceBindings = normalizeDefinitionOwnerFactConvergenceBindings(c.OwnerFactConvergenceBindings)
	c.RuntimeProfileContractsHash = strings.TrimSpace(c.RuntimeProfileContractsHash)
	c.AdmissionCompilerVersion = strings.TrimSpace(c.AdmissionCompilerVersion)
	c.ResolvedProjectWorkspaceDirectory = strings.TrimSpace(c.ResolvedProjectWorkspaceDirectory)
	c.ProposedRelationInputs = normalizeProposedRelations(c.ProposedRelationInputs)
	c.CandidateHash = strings.TrimSpace(c.CandidateHash)
	return c
}

func (c DefinitionAdmissionCandidate) Validate() error {
	c = c.Normalized()
	if c.SchemaVersion != DefinitionAdmissionCandidateSchemaVersionCurrent || c.WorkspaceID == "" || c.CandidateID == "" || c.CandidateRevision == 0 {
		return fmt.Errorf("definition admission candidate requires current schema, workspace and immutable candidate identity")
	}
	if (c.BaseDefinitionRevision == "") != (c.BaseStartupInputHash == "") {
		return fmt.Errorf("definition admission candidate base definition and startup hash must be paired")
	}
	if c.GoalInput.CandidateItemRef == "" || c.GoalInput.Text == "" || c.GoalInput.SourceIdentity == "" || len(c.DoDInputs) == 0 {
		return fmt.Errorf("definition admission candidate requires exact Goal and DoD inputs")
	}
	seen := map[string]struct{}{c.GoalInput.CandidateItemRef: {}}
	for _, item := range c.DoDInputs {
		if item.CandidateItemRef == "" || item.Text == "" || item.SourceIdentity == "" {
			return fmt.Errorf("definition admission candidate contains incomplete DoD input")
		}
		if _, duplicate := seen[item.CandidateItemRef]; duplicate {
			return fmt.Errorf("definition admission candidate item ref %q is duplicated", item.CandidateItemRef)
		}
		seen[item.CandidateItemRef] = struct{}{}
	}
	if c.WorkspaceTemplateSnapshot.TemplateRef == "" || c.WorkspaceTemplateSnapshot.Revision == "" || c.WorkspaceTemplateSnapshot.SnapshotHash == "" || c.RuntimeProfileContractsHash == "" || c.AdmissionCompilerVersion == "" || c.ResolvedProjectWorkspaceDirectory == "" {
		return fmt.Errorf("definition admission candidate requires template snapshot, compiler and resolved directory")
	}
	policyRefs := map[string]struct{}{}
	for _, binding := range c.AdmissionPolicyBindings {
		if binding.PolicyRef == "" || binding.Revision == "" || binding.SnapshotHash == "" {
			return fmt.Errorf("definition admission policy binding is incomplete")
		}
		if _, duplicate := policyRefs[binding.PolicyRef]; duplicate {
			return fmt.Errorf("definition admission policy binding %q is duplicated", binding.PolicyRef)
		}
		policyRefs[binding.PolicyRef] = struct{}{}
	}
	for _, binding := range c.RequiredOwnerFactBindings {
		if binding.SourceRef == "" {
			return fmt.Errorf("definition required owner fact requires source ref")
		}
		if err := binding.Requirement.Validate(); err != nil {
			return fmt.Errorf("definition required owner fact %q: %w", binding.SourceRef, err)
		}
	}
	for _, binding := range c.OwnerFactConvergenceBindings {
		if binding.SourceRef == "" {
			return fmt.Errorf("definition owner-fact convergence requires source ref")
		}
		if err := binding.Convergence.Requirement.Validate(); err != nil {
			return fmt.Errorf("definition owner-fact convergence %q: %w", binding.SourceRef, err)
		}
	}
	for _, relation := range c.ProposedRelationInputs {
		if relation.RelationRef == "" || relation.RelationKind == "" || len(relation.SourceItemRefs) == 0 || len(relation.TargetItemRefs) == 0 {
			return fmt.Errorf("definition admission candidate relation is incomplete")
		}
		for _, ref := range append(append([]string(nil), relation.SourceItemRefs...), relation.TargetItemRefs...) {
			if _, ok := seen[ref]; !ok {
				return fmt.Errorf("definition admission candidate relation references unknown item %q", ref)
			}
		}
	}
	want := c.computeHash()
	if c.CandidateHash == "" || c.CandidateHash != want {
		return fmt.Errorf("definition admission candidate hash mismatch")
	}
	return nil
}

func (c DefinitionAdmissionCandidate) WithHash() DefinitionAdmissionCandidate {
	c = c.Normalized()
	c.CandidateHash = c.computeHash()
	return c
}

func (c DefinitionAdmissionCandidate) Clone() DefinitionAdmissionCandidate {
	c = c.Normalized()
	c.DoDInputs = append([]DefinitionCandidateItem(nil), c.DoDInputs...)
	c.AdmissionPolicyBindings = append([]DefinitionAdmissionPolicyBinding(nil), c.AdmissionPolicyBindings...)
	c.RequiredOwnerFactBindings = append([]DefinitionRequiredOwnerFactBinding(nil), c.RequiredOwnerFactBindings...)
	c.OwnerFactConvergenceBindings = append([]DefinitionOwnerFactConvergenceBinding(nil), c.OwnerFactConvergenceBindings...)
	for idx := range c.OwnerFactConvergenceBindings {
		c.OwnerFactConvergenceBindings[idx].Convergence.DependsOnRequirementRefs = append([]string(nil), c.OwnerFactConvergenceBindings[idx].Convergence.DependsOnRequirementRefs...)
	}
	c.ProposedRelationInputs = append([]DefinitionProposedRelationInput(nil), c.ProposedRelationInputs...)
	for idx := range c.ProposedRelationInputs {
		c.ProposedRelationInputs[idx].SourceItemRefs = append([]string(nil), c.ProposedRelationInputs[idx].SourceItemRefs...)
		c.ProposedRelationInputs[idx].TargetItemRefs = append([]string(nil), c.ProposedRelationInputs[idx].TargetItemRefs...)
	}
	return c
}

func (c DefinitionAdmissionCandidate) computeHash() string {
	c.CandidateHash = ""
	h := sha256.New()
	writeDefinitionHash(h, fmt.Sprintf("%d", c.SchemaVersion), string(c.WorkspaceID), c.CandidateID, fmt.Sprintf("%d", c.CandidateRevision), c.BaseDefinitionRevision, c.BaseStartupInputHash)
	writeDefinitionHash(h, c.GoalInput.CandidateItemRef, c.GoalInput.Text, c.GoalInput.SourceIdentity)
	for _, item := range c.DoDInputs {
		writeDefinitionHash(h, item.CandidateItemRef, item.Text, item.SourceIdentity)
	}
	writeDefinitionHash(h, c.WorkspaceTemplateSnapshot.TemplateRef, c.WorkspaceTemplateSnapshot.Revision, c.WorkspaceTemplateSnapshot.SnapshotHash, c.RuntimeProfileContractsHash, c.AdmissionCompilerVersion, c.ResolvedProjectWorkspaceDirectory)
	for _, binding := range c.AdmissionPolicyBindings {
		writeDefinitionHash(h, binding.PolicyRef, binding.Revision, binding.SnapshotHash)
	}
	for _, binding := range c.RequiredOwnerFactBindings {
		writeDefinitionHash(h, binding.SourceRef, binding.Requirement.OwnerModuleID, string(binding.Requirement.FactKind), binding.Requirement.OperationKind)
	}
	for _, binding := range c.OwnerFactConvergenceBindings {
		value := binding.Convergence
		writeDefinitionHash(h, binding.SourceRef, value.Requirement.OwnerModuleID, string(value.Requirement.FactKind), value.Requirement.OperationKind,
			fmt.Sprintf("%t", value.RequirePreexisting), value.MissingTrigger, value.AcquisitionOwnerModuleID, value.AcquisitionOperationKind,
			value.TargetSelectorRef, value.TargetSelectorOwnerRef, value.TargetSelectorSubjectRef, value.ReobserveOwnerModuleID,
			string(value.ReobserveFactKind), fmt.Sprintf("%d", value.MaxAcquisitionAttempts))
		writeDefinitionHash(h, value.DependsOnRequirementRefs...)
	}
	for _, relation := range c.ProposedRelationInputs {
		writeDefinitionHash(h, relation.RelationRef, relation.RelationKind)
		writeDefinitionHash(h, relation.SourceItemRefs...)
		writeDefinitionHash(h, relation.TargetItemRefs...)
	}
	return "definition-candidate-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func (r DefinitionAdmissionResult) Normalized() DefinitionAdmissionResult {
	r.CandidateID = strings.TrimSpace(r.CandidateID)
	r.CandidateHash = strings.TrimSpace(r.CandidateHash)
	r.Disposition = DefinitionAdmissionDisposition(strings.TrimSpace(string(r.Disposition)))
	r.ViolationCode = strings.TrimSpace(r.ViolationCode)
	r.OffendingRefs = normalizeDefinitionStrings(r.OffendingRefs)
	r.CanonicalViolation = strings.TrimSpace(r.CanonicalViolation)
	r.CanonicalViolationHash = strings.TrimSpace(r.CanonicalViolationHash)
	r.AdmissionCompilerVersion = strings.TrimSpace(r.AdmissionCompilerVersion)
	r.CanonicalAdmittedInputHash = strings.TrimSpace(r.CanonicalAdmittedInputHash)
	r.AdmissionResultHash = strings.TrimSpace(r.AdmissionResultHash)
	return r
}

func (r DefinitionAdmissionResult) ValidateFor(candidate DefinitionAdmissionCandidate) error {
	r = r.Normalized()
	candidate = candidate.Normalized()
	if err := candidate.Validate(); err != nil {
		return err
	}
	if r.WorkspaceID != candidate.WorkspaceID || r.CandidateID != candidate.CandidateID || r.CandidateRevision != candidate.CandidateRevision || r.CandidateHash != candidate.CandidateHash || r.AdmissionCompilerVersion != candidate.AdmissionCompilerVersion {
		return fmt.Errorf("definition admission result does not bind the exact candidate")
	}
	switch r.Disposition {
	case DefinitionAdmissionAdmittedAllOf:
		if r.CanonicalAdmittedInputHash == "" || r.ViolationCode != "" || r.CanonicalViolation != "" || r.CanonicalViolationHash != "" || len(r.OffendingRefs) != 0 {
			return fmt.Errorf("admitted all-of result has invalid clarification fields")
		}
	case DefinitionAdmissionClarificationRequired:
		if r.ViolationCode == "" || r.CanonicalViolation == "" || r.CanonicalViolationHash == "" || len(r.OffendingRefs) == 0 || r.CanonicalAdmittedInputHash != "" {
			return fmt.Errorf("clarification result requires typed violation and offending refs")
		}
	default:
		return fmt.Errorf("unsupported definition admission disposition %q", r.Disposition)
	}
	if r.AdmissionResultHash == "" || r.AdmissionResultHash != r.computeHash() {
		return fmt.Errorf("definition admission result hash mismatch")
	}
	return nil
}

func (r DefinitionAdmissionResult) WithHash() DefinitionAdmissionResult {
	r = r.Normalized()
	r.AdmissionResultHash = r.computeHash()
	return r
}

func (r DefinitionAdmissionResult) computeHash() string {
	r.AdmissionResultHash = ""
	h := sha256.New()
	writeDefinitionHash(h, string(r.WorkspaceID), r.CandidateID, fmt.Sprintf("%d", r.CandidateRevision), r.CandidateHash, string(r.Disposition), r.ViolationCode)
	writeDefinitionHash(h, r.OffendingRefs...)
	writeDefinitionHash(h, r.CanonicalViolation, r.CanonicalViolationHash, r.AdmissionCompilerVersion, r.CanonicalAdmittedInputHash)
	return "definition-admission-result-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func (s WorkspaceStartupInputContract) Normalized() WorkspaceStartupInputContract {
	s.DefinitionRevision = strings.TrimSpace(s.DefinitionRevision)
	s.GoalDefinitionRef = strings.TrimSpace(s.GoalDefinitionRef)
	s.GoalTextHash = strings.TrimSpace(s.GoalTextHash)
	s.DoDDefinitionRefs = normalizeDefinitionStrings(s.DoDDefinitionRefs)
	s.DoDItemSetHash = strings.TrimSpace(s.DoDItemSetHash)
	s.WorkspaceTemplateRef = strings.TrimSpace(s.WorkspaceTemplateRef)
	s.TemplateRevision = strings.TrimSpace(s.TemplateRevision)
	s.TemplateSnapshotHash = strings.TrimSpace(s.TemplateSnapshotHash)
	s.RuntimeProfileContractsHash = strings.TrimSpace(s.RuntimeProfileContractsHash)
	s.ResolvedProjectWorkspaceDirectory = strings.TrimSpace(s.ResolvedProjectWorkspaceDirectory)
	s.CandidateItemToItemRefMappings = normalizeItemMappings(s.CandidateItemToItemRefMappings)
	s.CandidateItemToItemRefMappingHash = strings.TrimSpace(s.CandidateItemToItemRefMappingHash)
	s.StartupInputHash = strings.TrimSpace(s.StartupInputHash)
	return s
}

func (s WorkspaceStartupInputContract) Validate() error {
	s = s.Normalized()
	if s.WorkspaceID == "" || s.DefinitionRevision == "" || s.GoalDefinitionRef == "" || s.GoalTextHash == "" || len(s.DoDDefinitionRefs) == 0 || s.DoDItemSetHash == "" || s.WorkspaceTemplateRef == "" || s.TemplateRevision == "" || s.TemplateSnapshotHash == "" || s.RuntimeProfileContractsHash == "" || s.ResolvedProjectWorkspaceDirectory == "" || len(s.CandidateItemToItemRefMappings) == 0 {
		return fmt.Errorf("workspace startup input contract is incomplete")
	}
	if s.CandidateItemToItemRefMappingHash != stableItemMappingHash(s.CandidateItemToItemRefMappings) {
		return fmt.Errorf("workspace startup input mapping hash mismatch")
	}
	if s.StartupInputHash == "" || s.StartupInputHash != s.computeHash() {
		return fmt.Errorf("workspace startup input hash mismatch")
	}
	return nil
}

func (s WorkspaceStartupInputContract) WithHash() WorkspaceStartupInputContract {
	s = s.Normalized()
	s.CandidateItemToItemRefMappingHash = stableItemMappingHash(s.CandidateItemToItemRefMappings)
	s.StartupInputHash = s.computeHash()
	return s
}

func (s WorkspaceStartupInputContract) Clone() WorkspaceStartupInputContract {
	s = s.Normalized()
	s.DoDDefinitionRefs = append([]string(nil), s.DoDDefinitionRefs...)
	s.CandidateItemToItemRefMappings = append([]CandidateItemToItemRefMapping(nil), s.CandidateItemToItemRefMappings...)
	return s
}

func (s WorkspaceStartupInputContract) computeHash() string {
	s.StartupInputHash = ""
	h := sha256.New()
	writeDefinitionHash(h, string(s.WorkspaceID), s.DefinitionRevision, s.GoalDefinitionRef, s.GoalTextHash)
	writeDefinitionHash(h, s.DoDDefinitionRefs...)
	writeDefinitionHash(h, s.DoDItemSetHash, s.WorkspaceTemplateRef, s.TemplateRevision, s.TemplateSnapshotHash, s.RuntimeProfileContractsHash, s.ResolvedProjectWorkspaceDirectory, s.CandidateItemToItemRefMappingHash)
	return "workspace-startup-input-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func (a WorkspaceActivationAdmission) Normalized() WorkspaceActivationAdmission {
	a.CandidateID = strings.TrimSpace(a.CandidateID)
	a.CandidateHash = strings.TrimSpace(a.CandidateHash)
	a.DefinitionRevision = strings.TrimSpace(a.DefinitionRevision)
	a.StartupInputHash = strings.TrimSpace(a.StartupInputHash)
	a.State = WorkspaceActivationAdmissionState(strings.TrimSpace(string(a.State)))
	a.ClarificationResolutionBindingHash = strings.TrimSpace(a.ClarificationResolutionBindingHash)
	return a
}

func (a WorkspaceActivationAdmission) Validate(startup WorkspaceStartupInputContract) error {
	a = a.Normalized()
	startup = startup.Normalized()
	if a.WorkspaceID == "" || a.CandidateID == "" || a.CandidateRevision == 0 || a.CandidateHash == "" || a.DefinitionRevision == "" || a.StartupInputHash == "" || a.WorkspaceID != startup.WorkspaceID || a.DefinitionRevision != startup.DefinitionRevision || a.StartupInputHash != startup.StartupInputHash {
		return fmt.Errorf("workspace activation admission does not bind startup input")
	}
	switch a.State {
	case WorkspaceActivationEligible:
		if a.BlockingTicketID != "" && (a.ClarificationResolutionID == "" || a.ClarificationResolutionBindingHash == "") {
			return fmt.Errorf("eligible clarified activation requires resolution acknowledgement")
		}
	case WorkspaceActivationPendingClarificationResolution:
		if a.BlockingTicketID == "" || a.ClarificationResolutionID != "" || a.ClarificationResolutionBindingHash != "" {
			return fmt.Errorf("pending activation admission requires only blocking ticket identity")
		}
	default:
		return fmt.Errorf("unsupported workspace activation admission state %q", a.State)
	}
	return nil
}

func normalizeCandidateItem(item DefinitionCandidateItem) DefinitionCandidateItem {
	item.CandidateItemRef = strings.TrimSpace(item.CandidateItemRef)
	item.Text = strings.TrimSpace(item.Text)
	item.SourceIdentity = strings.TrimSpace(item.SourceIdentity)
	return item
}

func normalizeCandidateItems(items []DefinitionCandidateItem) []DefinitionCandidateItem {
	ret := append([]DefinitionCandidateItem(nil), items...)
	for idx := range ret {
		ret[idx] = normalizeCandidateItem(ret[idx])
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].CandidateItemRef < ret[j].CandidateItemRef })
	return ret
}

func normalizeAdmissionPolicyBindings(values []DefinitionAdmissionPolicyBinding) []DefinitionAdmissionPolicyBinding {
	ret := append([]DefinitionAdmissionPolicyBinding(nil), values...)
	for idx := range ret {
		ret[idx].PolicyRef = strings.TrimSpace(ret[idx].PolicyRef)
		ret[idx].Revision = strings.TrimSpace(ret[idx].Revision)
		ret[idx].SnapshotHash = strings.TrimSpace(ret[idx].SnapshotHash)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].PolicyRef < ret[j].PolicyRef })
	return ret
}

func normalizeDefinitionRequiredOwnerFactBindings(values []DefinitionRequiredOwnerFactBinding) []DefinitionRequiredOwnerFactBinding {
	ret := append([]DefinitionRequiredOwnerFactBinding(nil), values...)
	for idx := range ret {
		ret[idx].SourceRef = strings.TrimSpace(ret[idx].SourceRef)
		ret[idx].Requirement = ret[idx].Requirement.Normalized()
	}
	sort.Slice(ret, func(i, j int) bool {
		if ret[i].SourceRef != ret[j].SourceRef {
			return ret[i].SourceRef < ret[j].SourceRef
		}
		left := ret[i].Requirement
		right := ret[j].Requirement
		return left.OwnerModuleID+"|"+string(left.FactKind)+"|"+left.OperationKind < right.OwnerModuleID+"|"+string(right.FactKind)+"|"+right.OperationKind
	})
	return ret
}

func normalizeDefinitionOwnerFactConvergenceBindings(values []DefinitionOwnerFactConvergenceBinding) []DefinitionOwnerFactConvergenceBinding {
	ret := append([]DefinitionOwnerFactConvergenceBinding(nil), values...)
	for idx := range ret {
		ret[idx].SourceRef = strings.TrimSpace(ret[idx].SourceRef)
		ret[idx].Convergence = ret[idx].Convergence.Normalized()
	}
	sort.Slice(ret, func(i, j int) bool {
		if ret[i].SourceRef != ret[j].SourceRef {
			return ret[i].SourceRef < ret[j].SourceRef
		}
		left := ret[i].Convergence.Requirement
		right := ret[j].Convergence.Requirement
		return left.OwnerModuleID+"|"+string(left.FactKind)+"|"+left.OperationKind < right.OwnerModuleID+"|"+string(right.FactKind)+"|"+right.OperationKind
	})
	return ret
}

func normalizeProposedRelations(values []DefinitionProposedRelationInput) []DefinitionProposedRelationInput {
	ret := append([]DefinitionProposedRelationInput(nil), values...)
	for idx := range ret {
		ret[idx].RelationRef = strings.TrimSpace(ret[idx].RelationRef)
		ret[idx].RelationKind = strings.ToLower(strings.TrimSpace(ret[idx].RelationKind))
		ret[idx].SourceItemRefs = normalizeDefinitionStrings(ret[idx].SourceItemRefs)
		ret[idx].TargetItemRefs = normalizeDefinitionStrings(ret[idx].TargetItemRefs)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].RelationRef < ret[j].RelationRef })
	return ret
}

func normalizeDefinitionStrings(values []string) []string {
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

func normalizeItemMappings(values []CandidateItemToItemRefMapping) []CandidateItemToItemRefMapping {
	ret := append([]CandidateItemToItemRefMapping(nil), values...)
	for idx := range ret {
		ret[idx].CandidateItemRef = strings.TrimSpace(ret[idx].CandidateItemRef)
		ret[idx].ItemRef = strings.TrimSpace(ret[idx].ItemRef)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].CandidateItemRef < ret[j].CandidateItemRef })
	return ret
}

func stableItemMappingHash(values []CandidateItemToItemRefMapping) string {
	h := sha256.New()
	for _, value := range normalizeItemMappings(values) {
		writeDefinitionHash(h, value.CandidateItemRef, value.ItemRef)
	}
	return "definition-item-mapping-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func StableDefinitionTextHash(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return "definition-text-sha256:" + hex.EncodeToString(digest[:])
}

func StableDefinitionStringSetHash(values []string) string {
	h := sha256.New()
	writeDefinitionHash(h, normalizeDefinitionStrings(values)...)
	return "definition-set-sha256:" + hex.EncodeToString(h.Sum(nil))
}

type definitionHashWriter interface{ Write([]byte) (int, error) }

func writeDefinitionHash(h definitionHashWriter, values ...string) {
	for _, value := range values {
		_, _ = h.Write([]byte(fmt.Sprintf("%08x", len(value))))
		_, _ = h.Write([]byte(value))
	}
}
