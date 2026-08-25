package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeSemanticOwnerFactAcquisition is the exact, owner-fact-backed
// operation authorization for one missing all-of postcondition. The
// Definition selector identifies the declared target-selection scope; the
// operation selector identifies the capability owner's concrete authorization.
// They are independent identities and are never treated as aliases.
type RuntimeSemanticOwnerFactAcquisition struct {
	Requirement                        RuntimeOwnerFactRequirement
	ProspectiveSatisfactionContractRef string
	// ProspectiveTargetRef is copied from the immutable Definition declaration.
	// The provider may echo it but cannot select or replace it.
	ProspectiveTargetRef                string
	DeclarationTargetSelectorRef        string
	DeclarationTargetSelectorOwnerRef   string
	DeclarationTargetSelectorSubjectRef string
	AcquisitionOwnerModuleID            string
	AcquisitionOperationKind            string
	AcquisitionOperationVariantID       string
	AcquisitionTargetPolicy             RuntimeOperationTargetPolicy
	AcquisitionTargetSelectorRef        string
	AcquisitionConstraintRefs           []string
	OwnerCatalogSnapshotID              string
	OwnerCatalogObservedRevision        string
	ReobserveOwnerModuleID              string
	ReobserveFactKind                   RuntimeOwnerFactKind
	MaxAcquisitionAttempts              int
}

// RuntimeSemanticEvidenceObligationPurpose identifies which immutable proof
// contract is current for one CWI. DefinitionProof means the Definition's
// terminal evidence contract is directly actionable. OwnerFactAcquisition
// means the terminal contract remains unchanged, but the current bounded proof
// must first establish the manifest-sealed owner-fact acquisition postcondition.
// This is a pure derived vocabulary; it is not a Runtime state machine.
type RuntimeSemanticEvidenceObligationPurpose string

const (
	RuntimeSemanticEvidenceObligationDefinitionProof      RuntimeSemanticEvidenceObligationPurpose = "definition_proof"
	RuntimeSemanticEvidenceObligationOwnerFactAcquisition RuntimeSemanticEvidenceObligationPurpose = "owner_fact_acquisition"
)

// RuntimeSemanticEvidenceObligation is the single derived source of truth for
// evidence-kind validation and provider schema narrowing. The terminal kinds
// always retain the sealed Definition declaration. Current kinds describe only
// the proof that may be proposed in the current semantic generation.
type RuntimeSemanticEvidenceObligation struct {
	Purpose                       RuntimeSemanticEvidenceObligationPurpose
	TerminalRequiredEvidenceKinds []string
	CurrentRequiredEvidenceKinds  []string
}

func (o RuntimeSemanticEvidenceObligation) Clone() RuntimeSemanticEvidenceObligation {
	o.TerminalRequiredEvidenceKinds = append([]string(nil), o.TerminalRequiredEvidenceKinds...)
	o.CurrentRequiredEvidenceKinds = append([]string(nil), o.CurrentRequiredEvidenceKinds...)
	return o
}

// ResolveRuntimeSemanticEvidenceObligation derives the current evidence
// contract without mutating the sealed proof obligation. A missing owner fact
// is acquired only through state-change evidence; that intermediate evidence
// never weakens or replaces the Definition's terminal evidence kinds.
func ResolveRuntimeSemanticEvidenceObligation(obligation RuntimeSemanticProofObligation) (RuntimeSemanticEvidenceObligation, error) {
	obligation = obligation.Normalized()
	if err := obligation.Validate(); err != nil {
		return RuntimeSemanticEvidenceObligation{}, err
	}
	ret := RuntimeSemanticEvidenceObligation{
		Purpose:                       RuntimeSemanticEvidenceObligationDefinitionProof,
		TerminalRequiredEvidenceKinds: append([]string(nil), obligation.RequiredEvidenceKinds...),
		CurrentRequiredEvidenceKinds:  append([]string(nil), obligation.RequiredEvidenceKinds...),
	}
	if obligation.OwnerFactAcquisition != nil {
		ret.Purpose = RuntimeSemanticEvidenceObligationOwnerFactAcquisition
		ret.CurrentRequiredEvidenceKinds = []string{EvidenceKindStateChange}
	}
	return ret, nil
}

func (a RuntimeSemanticOwnerFactAcquisition) Normalized() RuntimeSemanticOwnerFactAcquisition {
	a.Requirement = a.Requirement.Normalized()
	a.ProspectiveSatisfactionContractRef = strings.TrimSpace(a.ProspectiveSatisfactionContractRef)
	a.ProspectiveTargetRef = strings.TrimSpace(a.ProspectiveTargetRef)
	a.DeclarationTargetSelectorRef = strings.TrimSpace(a.DeclarationTargetSelectorRef)
	a.DeclarationTargetSelectorOwnerRef = strings.TrimSpace(a.DeclarationTargetSelectorOwnerRef)
	a.DeclarationTargetSelectorSubjectRef = strings.TrimSpace(a.DeclarationTargetSelectorSubjectRef)
	a.AcquisitionOwnerModuleID = strings.TrimSpace(a.AcquisitionOwnerModuleID)
	a.AcquisitionOperationKind = CanonicalRuntimeBlockKind(a.AcquisitionOperationKind)
	a.AcquisitionOperationVariantID = strings.TrimSpace(a.AcquisitionOperationVariantID)
	a.AcquisitionTargetPolicy = RuntimeOperationTargetPolicy(strings.TrimSpace(string(a.AcquisitionTargetPolicy)))
	a.AcquisitionTargetSelectorRef = strings.TrimSpace(a.AcquisitionTargetSelectorRef)
	a.AcquisitionConstraintRefs = normalizeStrings(a.AcquisitionConstraintRefs)
	a.OwnerCatalogSnapshotID = strings.TrimSpace(a.OwnerCatalogSnapshotID)
	a.OwnerCatalogObservedRevision = strings.TrimSpace(a.OwnerCatalogObservedRevision)
	a.ReobserveOwnerModuleID = strings.TrimSpace(a.ReobserveOwnerModuleID)
	a.ReobserveFactKind = RuntimeOwnerFactKind(strings.TrimSpace(string(a.ReobserveFactKind)))
	return a
}

func (a RuntimeSemanticOwnerFactAcquisition) Clone() RuntimeSemanticOwnerFactAcquisition {
	a = a.Normalized()
	a.AcquisitionConstraintRefs = append([]string(nil), a.AcquisitionConstraintRefs...)
	return a
}

func (a RuntimeSemanticOwnerFactAcquisition) Validate() error {
	a = a.Normalized()
	if err := a.Requirement.Validate(); err != nil {
		return err
	}
	if a.DeclarationTargetSelectorRef == "" || a.DeclarationTargetSelectorOwnerRef == "" || a.DeclarationTargetSelectorSubjectRef == "" ||
		a.AcquisitionOwnerModuleID == "" || a.AcquisitionOperationKind == "" || a.AcquisitionTargetPolicy == "" ||
		a.AcquisitionTargetSelectorRef == "" || a.OwnerCatalogSnapshotID == "" || a.OwnerCatalogObservedRevision == "" ||
		a.ReobserveOwnerModuleID != a.Requirement.OwnerModuleID || a.ReobserveFactKind != a.Requirement.FactKind || a.MaxAcquisitionAttempts != 1 {
		return fmt.Errorf("runtime semantic owner-fact acquisition requires declaration, operation authorization, catalog, re-observation and one attempt")
	}
	descriptor, found := RuntimeOperationDescriptorForKind(a.AcquisitionOperationKind)
	if !found || descriptor.OwnerModuleID != a.AcquisitionOwnerModuleID {
		return fmt.Errorf("runtime semantic owner-fact acquisition operation %q does not belong to owner %q", a.AcquisitionOperationKind, a.AcquisitionOwnerModuleID)
	}
	switch a.AcquisitionTargetPolicy {
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		if a.ProspectiveSatisfactionContractRef != "" || a.ProspectiveTargetRef != "" {
			return fmt.Errorf("runtime semantic existing-scope owner-fact acquisition cannot declare prospective satisfaction or target authority")
		}
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if a.ProspectiveSatisfactionContractRef == "" || a.ProspectiveTargetRef == "" {
			return fmt.Errorf("runtime semantic prospective owner-fact acquisition requires a prospective satisfaction contract and exact target")
		}
	default:
		return fmt.Errorf("runtime semantic owner-fact acquisition requires a selector-backed target policy, got %q", a.AcquisitionTargetPolicy)
	}
	if a.AcquisitionTargetPolicy == RuntimeOperationTargetPolicyProspectiveOwnerTarget && len(a.AcquisitionConstraintRefs) == 0 {
		return fmt.Errorf("runtime semantic owner-fact acquisition requires prospective owner constraints")
	}
	if a.AcquisitionTargetPolicy == RuntimeOperationTargetPolicyExistingOwnerScope && len(a.AcquisitionConstraintRefs) != 0 {
		return fmt.Errorf("runtime semantic existing-scope owner-fact acquisition cannot carry prospective constraints")
	}
	return nil
}

// RuntimeSemanticProofObligation is the sole per-CWI semantic declaration
// carried by a sealed manifest. It replaces later reads of activation
// DefinitionFocuses during document admission and binding compilation.
type RuntimeSemanticProofObligation struct {
	CoverageWorkItemID                     common.CoverageWorkItemID
	RequiredEvidenceKinds                  []string
	DeclarationTargetRole                  string
	DeclarationTargetDiscoverySources      []string
	DeclarationTargetSelectorRef           string
	DeclarationTargetSelectorOwnerRef      string
	DeclarationTargetSelectorSubjectRef    string
	DeclarationTargetSelectorCandidateRefs []string
	DeclarationProspectiveTargetRef        string
	RequireTargetGrounding                 bool
	RequireAssessment                      bool
	RequiredOwnerFacts                     []RuntimeOwnerFactRequirement
	OwnerFactConvergences                  []RuntimeOwnerFactConvergence
	MetricConstraints                      []string
	ArtifactRefs                           []string
	OwnerFactAcquisition                   *RuntimeSemanticOwnerFactAcquisition
}

func (o RuntimeSemanticProofObligation) Normalized() RuntimeSemanticProofObligation {
	o.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(o.CoverageWorkItemID)))
	o.RequiredEvidenceKinds = normalizeStrings(o.RequiredEvidenceKinds)
	o.DeclarationTargetRole = strings.TrimSpace(o.DeclarationTargetRole)
	o.DeclarationTargetDiscoverySources = normalizeStrings(o.DeclarationTargetDiscoverySources)
	o.DeclarationTargetSelectorRef = strings.TrimSpace(o.DeclarationTargetSelectorRef)
	o.DeclarationTargetSelectorOwnerRef = strings.TrimSpace(o.DeclarationTargetSelectorOwnerRef)
	o.DeclarationTargetSelectorSubjectRef = strings.TrimSpace(o.DeclarationTargetSelectorSubjectRef)
	o.DeclarationTargetSelectorCandidateRefs = normalizeStrings(o.DeclarationTargetSelectorCandidateRefs)
	o.DeclarationProspectiveTargetRef = strings.TrimSpace(o.DeclarationProspectiveTargetRef)
	o.RequiredOwnerFacts = NormalizeRuntimeOwnerFactRequirements(o.RequiredOwnerFacts)
	o.OwnerFactConvergences = NormalizeRuntimeOwnerFactConvergences(o.OwnerFactConvergences)
	o.MetricConstraints = normalizeStrings(o.MetricConstraints)
	o.ArtifactRefs = normalizeStrings(o.ArtifactRefs)
	if o.OwnerFactAcquisition != nil {
		value := o.OwnerFactAcquisition.Normalized()
		o.OwnerFactAcquisition = &value
	}
	return o
}

func (o RuntimeSemanticProofObligation) Clone() RuntimeSemanticProofObligation {
	o = o.Normalized()
	o.RequiredEvidenceKinds = append([]string(nil), o.RequiredEvidenceKinds...)
	o.DeclarationTargetDiscoverySources = append([]string(nil), o.DeclarationTargetDiscoverySources...)
	o.DeclarationTargetSelectorCandidateRefs = append([]string(nil), o.DeclarationTargetSelectorCandidateRefs...)
	o.RequiredOwnerFacts = append([]RuntimeOwnerFactRequirement(nil), o.RequiredOwnerFacts...)
	o.OwnerFactConvergences = CloneRuntimeOwnerFactConvergences(o.OwnerFactConvergences)
	o.MetricConstraints = append([]string(nil), o.MetricConstraints...)
	o.ArtifactRefs = append([]string(nil), o.ArtifactRefs...)
	if o.OwnerFactAcquisition != nil {
		value := o.OwnerFactAcquisition.Clone()
		o.OwnerFactAcquisition = &value
	}
	return o
}

func (o RuntimeSemanticProofObligation) Validate() error {
	o = o.Normalized()
	if o.CoverageWorkItemID == "" || (len(o.RequiredEvidenceKinds) == 0 && !o.RequireAssessment) {
		return fmt.Errorf("runtime semantic proof obligation requires coverage and evidence or assessment")
	}
	if err := ValidateRuntimeOwnerFactRequirements(o.RequiredOwnerFacts); err != nil {
		return err
	}
	if err := ValidateRuntimeOwnerFactConvergences(o.RequiredOwnerFacts, o.OwnerFactConvergences); err != nil {
		return err
	}
	if o.DeclarationTargetSelectorRef != "" && (o.DeclarationTargetSelectorOwnerRef == "" || o.DeclarationTargetSelectorSubjectRef == "") {
		return fmt.Errorf("runtime semantic proof obligation declaration selector requires owner and subject")
	}
	if o.DeclarationProspectiveTargetRef != "" {
		if !o.RequireTargetGrounding || o.DeclarationTargetSelectorRef == "" || o.DeclarationTargetSelectorOwnerRef == "" || o.DeclarationTargetSelectorSubjectRef == "" {
			return fmt.Errorf("runtime semantic proof obligation prospective target requires grounding and a complete declaration selector")
		}
		if len(o.DeclarationTargetSelectorCandidateRefs) != 0 && !containsString(o.DeclarationTargetSelectorCandidateRefs, o.DeclarationProspectiveTargetRef) {
			return fmt.Errorf("runtime semantic proof obligation prospective target is outside declaration selector candidates")
		}
	}
	if o.OwnerFactAcquisition != nil {
		if err := o.OwnerFactAcquisition.Validate(); err != nil {
			return err
		}
		if o.OwnerFactAcquisition.DeclarationTargetSelectorRef != o.DeclarationTargetSelectorRef ||
			o.OwnerFactAcquisition.DeclarationTargetSelectorOwnerRef != o.DeclarationTargetSelectorOwnerRef ||
			o.OwnerFactAcquisition.DeclarationTargetSelectorSubjectRef != o.DeclarationTargetSelectorSubjectRef {
			return fmt.Errorf("runtime semantic owner-fact acquisition declaration does not match proof obligation")
		}
		if o.OwnerFactAcquisition.AcquisitionTargetPolicy == RuntimeOperationTargetPolicyProspectiveOwnerTarget &&
			o.OwnerFactAcquisition.ProspectiveTargetRef != o.DeclarationProspectiveTargetRef {
			return fmt.Errorf("runtime semantic owner-fact acquisition prospective target does not match proof obligation")
		}
	}
	return nil
}

func normalizeRuntimeSemanticProofObligations(values []RuntimeSemanticProofObligation) []RuntimeSemanticProofObligation {
	ret := append([]RuntimeSemanticProofObligation(nil), values...)
	for idx := range ret {
		ret[idx] = ret[idx].Normalized()
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].CoverageWorkItemID < ret[j].CoverageWorkItemID })
	return nilIfEmpty(ret)
}

func cloneRuntimeSemanticProofObligations(values []RuntimeSemanticProofObligation) []RuntimeSemanticProofObligation {
	ret := make([]RuntimeSemanticProofObligation, 0, len(values))
	for _, value := range values {
		ret = append(ret, value.Clone())
	}
	return nilIfEmpty(ret)
}

func sameRuntimeSemanticProofObligations(left, right []RuntimeSemanticProofObligation) bool {
	left = normalizeRuntimeSemanticProofObligations(left)
	right = normalizeRuntimeSemanticProofObligations(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		l, r := left[idx], right[idx]
		if l.CoverageWorkItemID != r.CoverageWorkItemID ||
			!sameStrings(l.RequiredEvidenceKinds, r.RequiredEvidenceKinds) ||
			l.DeclarationTargetRole != r.DeclarationTargetRole ||
			!sameStrings(l.DeclarationTargetDiscoverySources, r.DeclarationTargetDiscoverySources) ||
			l.DeclarationTargetSelectorRef != r.DeclarationTargetSelectorRef ||
			l.DeclarationTargetSelectorOwnerRef != r.DeclarationTargetSelectorOwnerRef ||
			l.DeclarationTargetSelectorSubjectRef != r.DeclarationTargetSelectorSubjectRef ||
			!sameStrings(l.DeclarationTargetSelectorCandidateRefs, r.DeclarationTargetSelectorCandidateRefs) ||
			l.DeclarationProspectiveTargetRef != r.DeclarationProspectiveTargetRef ||
			l.RequireTargetGrounding != r.RequireTargetGrounding || l.RequireAssessment != r.RequireAssessment ||
			!sameRuntimeOwnerFactRequirements(l.RequiredOwnerFacts, r.RequiredOwnerFacts) ||
			!SameRuntimeOwnerFactConvergences(l.OwnerFactConvergences, r.OwnerFactConvergences) ||
			!sameStrings(l.MetricConstraints, r.MetricConstraints) || !sameStrings(l.ArtifactRefs, r.ArtifactRefs) ||
			!sameRuntimeSemanticOwnerFactAcquisition(l.OwnerFactAcquisition, r.OwnerFactAcquisition) {
			return false
		}
	}
	return true
}

func sameRuntimeSemanticOwnerFactAcquisition(left, right *RuntimeSemanticOwnerFactAcquisition) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := left.Normalized(), right.Normalized()
	return l.Requirement == r.Requirement &&
		l.ProspectiveSatisfactionContractRef == r.ProspectiveSatisfactionContractRef &&
		l.ProspectiveTargetRef == r.ProspectiveTargetRef &&
		l.DeclarationTargetSelectorRef == r.DeclarationTargetSelectorRef &&
		l.DeclarationTargetSelectorOwnerRef == r.DeclarationTargetSelectorOwnerRef &&
		l.DeclarationTargetSelectorSubjectRef == r.DeclarationTargetSelectorSubjectRef &&
		l.AcquisitionOwnerModuleID == r.AcquisitionOwnerModuleID &&
		l.AcquisitionOperationKind == r.AcquisitionOperationKind &&
		l.AcquisitionOperationVariantID == r.AcquisitionOperationVariantID &&
		l.AcquisitionTargetPolicy == r.AcquisitionTargetPolicy &&
		l.AcquisitionTargetSelectorRef == r.AcquisitionTargetSelectorRef &&
		sameStrings(l.AcquisitionConstraintRefs, r.AcquisitionConstraintRefs) &&
		l.OwnerCatalogSnapshotID == r.OwnerCatalogSnapshotID &&
		l.OwnerCatalogObservedRevision == r.OwnerCatalogObservedRevision &&
		l.ReobserveOwnerModuleID == r.ReobserveOwnerModuleID &&
		l.ReobserveFactKind == r.ReobserveFactKind &&
		l.MaxAcquisitionAttempts == r.MaxAcquisitionAttempts
}

func runtimeSemanticProofObligationSourceFocus(o RuntimeSemanticProofObligation) CoverageFocus {
	o = o.Normalized()
	return CoverageFocus{
		CoverageWorkItemIDs:         []common.CoverageWorkItemID{o.CoverageWorkItemID},
		RequiredEvidenceKinds:       append([]string(nil), o.RequiredEvidenceKinds...),
		TargetRole:                  o.DeclarationTargetRole,
		TargetDiscoverySources:      append([]string(nil), o.DeclarationTargetDiscoverySources...),
		TargetSelectorRef:           o.DeclarationTargetSelectorRef,
		TargetSelectorOwnerRef:      o.DeclarationTargetSelectorOwnerRef,
		TargetSelectorSubjectRef:    o.DeclarationTargetSelectorSubjectRef,
		TargetSelectorCandidateRefs: append([]string(nil), o.DeclarationTargetSelectorCandidateRefs...),
		ProspectiveTargetRef:        o.DeclarationProspectiveTargetRef,
		RequireTargetGrounding:      o.RequireTargetGrounding,
		RequireAssessment:           o.RequireAssessment,
		RequiredOwnerFacts:          append([]RuntimeOwnerFactRequirement(nil), o.RequiredOwnerFacts...),
		OwnerFactConvergences:       CloneRuntimeOwnerFactConvergences(o.OwnerFactConvergences),
		MetricConstraints:           append([]string(nil), o.MetricConstraints...),
		ArtifactRefs:                append([]string(nil), o.ArtifactRefs...),
	}.Normalized()
}

func writeRuntimeSemanticProofObligationsHash(h hashStringWriter, values []RuntimeSemanticProofObligation) {
	for _, raw := range normalizeRuntimeSemanticProofObligations(values) {
		value := raw.Normalized()
		writeHashString(h, string(value.CoverageWorkItemID))
		writeHashStrings(h, value.RequiredEvidenceKinds)
		writeHashString(h, value.DeclarationTargetRole)
		writeHashStrings(h, value.DeclarationTargetDiscoverySources)
		writeHashString(h, value.DeclarationTargetSelectorRef)
		writeHashString(h, value.DeclarationTargetSelectorOwnerRef)
		writeHashString(h, value.DeclarationTargetSelectorSubjectRef)
		writeHashStrings(h, value.DeclarationTargetSelectorCandidateRefs)
		writeHashString(h, value.DeclarationProspectiveTargetRef)
		writeHashString(h, strconv.FormatBool(value.RequireTargetGrounding))
		writeHashString(h, strconv.FormatBool(value.RequireAssessment))
		for _, requirement := range value.RequiredOwnerFacts {
			writeHashString(h, requirement.OwnerModuleID)
			writeHashString(h, string(requirement.FactKind))
			writeHashString(h, requirement.OperationKind)
		}
		for _, convergence := range value.OwnerFactConvergences {
			convergence = convergence.Normalized()
			writeHashString(h, RuntimeOwnerFactRequirementRef(convergence.Requirement))
			writeHashString(h, strconv.FormatBool(convergence.RequirePreexisting))
			writeHashString(h, convergence.ProspectiveSatisfactionContractRef)
			writeHashString(h, convergence.MissingTrigger)
			writeHashString(h, convergence.AcquisitionOwnerModuleID)
			writeHashString(h, convergence.AcquisitionOperationKind)
			writeHashString(h, convergence.AcquisitionOperationVariantID)
			writeHashString(h, string(convergence.AcquisitionTargetPolicy))
			writeHashString(h, convergence.AcquisitionTargetSelectorRef)
			writeHashString(h, convergence.TargetSelectorRef)
			writeHashString(h, convergence.TargetSelectorOwnerRef)
			writeHashString(h, convergence.TargetSelectorSubjectRef)
			writeHashString(h, convergence.ReobserveOwnerModuleID)
			writeHashString(h, string(convergence.ReobserveFactKind))
			writeHashString(h, strconv.Itoa(convergence.MaxAcquisitionAttempts))
			writeHashStrings(h, convergence.DependsOnRequirementRefs)
		}
		writeHashStrings(h, value.MetricConstraints)
		writeHashStrings(h, value.ArtifactRefs)
		if value.OwnerFactAcquisition == nil {
			writeHashString(h, "no-owner-fact-acquisition")
			continue
		}
		acquisition := value.OwnerFactAcquisition.Normalized()
		writeHashString(h, RuntimeOwnerFactRequirementRef(acquisition.Requirement))
		writeHashString(h, acquisition.ProspectiveSatisfactionContractRef)
		writeHashString(h, acquisition.ProspectiveTargetRef)
		writeHashString(h, acquisition.DeclarationTargetSelectorRef)
		writeHashString(h, acquisition.DeclarationTargetSelectorOwnerRef)
		writeHashString(h, acquisition.DeclarationTargetSelectorSubjectRef)
		writeHashString(h, acquisition.AcquisitionOwnerModuleID)
		writeHashString(h, acquisition.AcquisitionOperationKind)
		writeHashString(h, acquisition.AcquisitionOperationVariantID)
		writeHashString(h, string(acquisition.AcquisitionTargetPolicy))
		writeHashString(h, acquisition.AcquisitionTargetSelectorRef)
		writeHashStrings(h, acquisition.AcquisitionConstraintRefs)
		writeHashString(h, acquisition.OwnerCatalogSnapshotID)
		writeHashString(h, acquisition.OwnerCatalogObservedRevision)
		writeHashString(h, acquisition.ReobserveOwnerModuleID)
		writeHashString(h, string(acquisition.ReobserveFactKind))
		writeHashString(h, strconv.Itoa(acquisition.MaxAcquisitionAttempts))
	}
}

func compileRuntimeSemanticProofObligations(sourceFocuses []RuntimeDefinitionFocus, coverageItems []RuntimeCoverageItem, catalogs []RuntimeOperationAdmissibility) ([]RuntimeSemanticProofObligation, error) {
	sourceByCoverage, err := runtimeSemanticBindingSourceFocuses(sourceFocuses)
	if err != nil {
		return nil, err
	}
	catalogByOwner := make(map[string]RuntimeOperationAdmissibility, len(catalogs))
	for _, raw := range NormalizeRuntimeOperationAdmissibilities(catalogs) {
		if err := raw.Validate(); err != nil {
			return nil, err
		}
		catalogByOwner[raw.OwnerModuleID] = raw
	}
	ret := make([]RuntimeSemanticProofObligation, 0, len(coverageItems))
	for _, item := range NormalizeRuntimeCoverageItems(coverageItems) {
		source, found := sourceByCoverage[item.ID]
		if !found {
			return nil, fmt.Errorf("runtime semantic proof obligation has no Definition focus for coverage %q", item.ID)
		}
		source = source.Normalized()
		obligation := RuntimeSemanticProofObligation{
			CoverageWorkItemID:                     item.ID,
			RequiredEvidenceKinds:                  append([]string(nil), source.RequiredEvidenceKinds...),
			DeclarationTargetRole:                  source.TargetRole,
			DeclarationTargetDiscoverySources:      append([]string(nil), source.TargetDiscoverySources...),
			DeclarationTargetSelectorRef:           source.TargetSelectorRef,
			DeclarationTargetSelectorOwnerRef:      source.TargetSelectorOwnerRef,
			DeclarationTargetSelectorSubjectRef:    source.TargetSelectorSubjectRef,
			DeclarationTargetSelectorCandidateRefs: append([]string(nil), source.TargetSelectorCandidateRefs...),
			DeclarationProspectiveTargetRef:        source.ProspectiveTargetRef,
			RequireTargetGrounding:                 source.RequireTargetGrounding,
			RequireAssessment:                      source.RequireAssessment,
			RequiredOwnerFacts:                     append([]RuntimeOwnerFactRequirement(nil), source.RequiredOwnerFacts...),
			OwnerFactConvergences:                  CloneRuntimeOwnerFactConvergences(source.OwnerFactConvergences),
			MetricConstraints:                      append([]string(nil), source.MetricConstraints...),
			ArtifactRefs:                           append([]string(nil), source.ArtifactRefs...),
		}
		if len(RuntimeMissingOwnerFactRequirements(source.RequiredOwnerFacts, catalogs)) != 0 {
			convergence, required := ResolveRuntimeOwnerFactAcquisition(source.RequiredOwnerFacts, source.OwnerFactConvergences, catalogs)
			if !required {
				return nil, fmt.Errorf("runtime semantic proof obligation for coverage %q has a missing owner fact without ready acquisition authority", item.ID)
			}
			if source.TargetSelectorRef != convergence.TargetSelectorRef || source.TargetSelectorOwnerRef != convergence.TargetSelectorOwnerRef || source.TargetSelectorSubjectRef != convergence.TargetSelectorSubjectRef {
				return nil, fmt.Errorf("runtime semantic proof obligation for coverage %q has mismatched Definition acquisition binding", item.ID)
			}
			catalog, found := catalogByOwner[convergence.AcquisitionOwnerModuleID]
			if !found {
				return nil, fmt.Errorf("runtime semantic proof obligation for coverage %q has no acquisition owner catalog", item.ID)
			}
			variantIDs := []string(nil)
			if convergence.AcquisitionOperationVariantID != "" {
				variantIDs = []string{convergence.AcquisitionOperationVariantID}
			}
			allowed, selectors, constraints, factFound, allowErr := RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(
				catalog, convergence.AcquisitionOperationKind, variantIDs, convergence.AcquisitionTargetPolicy,
			)
			if allowErr != nil {
				return nil, allowErr
			}
			if !factFound || !allowed || !containsString(selectors, convergence.AcquisitionTargetSelectorRef) {
				return nil, fmt.Errorf("runtime semantic proof obligation for coverage %q acquisition is absent from owner operation-admissibility fact", item.ID)
			}
			if convergence.AcquisitionTargetPolicy != RuntimeOperationTargetPolicyProspectiveOwnerTarget {
				constraints = nil
			}
			prospectiveTargetRef := ""
			if convergence.AcquisitionTargetPolicy == RuntimeOperationTargetPolicyProspectiveOwnerTarget {
				prospectiveTargetRef = source.ProspectiveTargetRef
			}
			obligation.OwnerFactAcquisition = &RuntimeSemanticOwnerFactAcquisition{
				Requirement:                         convergence.Requirement,
				ProspectiveSatisfactionContractRef:  convergence.ProspectiveSatisfactionContractRef,
				ProspectiveTargetRef:                prospectiveTargetRef,
				DeclarationTargetSelectorRef:        convergence.TargetSelectorRef,
				DeclarationTargetSelectorOwnerRef:   convergence.TargetSelectorOwnerRef,
				DeclarationTargetSelectorSubjectRef: convergence.TargetSelectorSubjectRef,
				AcquisitionOwnerModuleID:            convergence.AcquisitionOwnerModuleID,
				AcquisitionOperationKind:            convergence.AcquisitionOperationKind,
				AcquisitionOperationVariantID:       convergence.AcquisitionOperationVariantID,
				AcquisitionTargetPolicy:             convergence.AcquisitionTargetPolicy,
				AcquisitionTargetSelectorRef:        convergence.AcquisitionTargetSelectorRef,
				AcquisitionConstraintRefs:           constraints,
				OwnerCatalogSnapshotID:              catalog.SnapshotID,
				OwnerCatalogObservedRevision:        catalog.ObservedRevision,
				ReobserveOwnerModuleID:              convergence.ReobserveOwnerModuleID,
				ReobserveFactKind:                   convergence.ReobserveFactKind,
				MaxAcquisitionAttempts:              convergence.MaxAcquisitionAttempts,
			}
		}
		obligation = obligation.Normalized()
		if err := obligation.Validate(); err != nil {
			return nil, fmt.Errorf("runtime semantic proof obligation for coverage %q: %w", item.ID, err)
		}
		ret = append(ret, obligation)
	}
	return normalizeRuntimeSemanticProofObligations(ret), nil
}
