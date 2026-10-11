package workorchruntimecontract

import (
	"fmt"
	"reflect"
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
	Requirement                             RuntimeOwnerFactRequirement
	ProspectiveSatisfactionContractRef      string
	ProspectiveTargetSatisfactionConstraint *RuntimeProspectiveTargetSatisfactionConstraint
	DeclarationTargetSelectorRef            string
	DeclarationTargetSelectorOwnerRef       string
	DeclarationTargetSelectorSubjectRef     string
	AcquisitionOwnerModuleID                string
	AcquisitionOperationKind                string
	AcquisitionOperationVariantID           string
	AcquisitionTargetPolicy                 RuntimeOperationTargetPolicy
	AcquisitionTargetSelectorRef            string
	AcquisitionConstraintRefs               []string
	OwnerCatalogSnapshotID                  string
	OwnerCatalogObservedRevision            string
	ReobserveOwnerModuleID                  string
	ReobserveFactKind                       RuntimeOwnerFactKind
	MaxAcquisitionAttempts                  int
}

// RuntimeSemanticEvidenceObligationPurpose identifies which immutable proof
// contract is current for one CWI. DefinitionProof means the Definition's
// terminal evidence contract is directly actionable. OwnerFactAcquisition
// means the terminal contract remains unchanged, but the current bounded proof
// must first establish the manifest-sealed owner-fact acquisition postcondition.
// This is a pure derived vocabulary; it is not a Runtime state machine.
type RuntimeSemanticEvidenceObligationPurpose string

const (
	RuntimeSemanticEvidenceObligationImplementationWork      RuntimeSemanticEvidenceObligationPurpose = "implementation_work"
	RuntimeSemanticEvidenceObligationDefinitionProof         RuntimeSemanticEvidenceObligationPurpose = "definition_proof"
	RuntimeSemanticEvidenceObligationOwnerFactAcquisition    RuntimeSemanticEvidenceObligationPurpose = "owner_fact_acquisition"
	RuntimeSemanticEvidenceObligationValidationFailureRepair RuntimeSemanticEvidenceObligationPurpose = "validation_failure_repair"
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
	if obligation.ValidationFailureRepair != nil {
		ret.Purpose = RuntimeSemanticEvidenceObligationValidationFailureRepair
		ret.CurrentRequiredEvidenceKinds = []string{EvidenceKindStateChange}
	}
	if obligation.ImplementationWork != nil {
		ret.Purpose = RuntimeSemanticEvidenceObligationImplementationWork
		ret.CurrentRequiredEvidenceKinds = []string{EvidenceKindStateChange}
	}
	return ret, nil
}

// RuntimeSemanticCurrentDeclarationRef returns the only declaration identity
// a provider may echo for the current proof purpose.
func RuntimeSemanticCurrentDeclarationRef(obligation RuntimeSemanticProofObligation) string {
	obligation = obligation.Normalized()
	if obligation.ValidationFailureRepair != nil {
		return obligation.ValidationFailureRepair.Normalized().ConvergenceID
	}
	return obligation.DeclarationRef
}

func (a RuntimeSemanticOwnerFactAcquisition) Normalized() RuntimeSemanticOwnerFactAcquisition {
	a.Requirement = a.Requirement.Normalized()
	a.ProspectiveSatisfactionContractRef = strings.TrimSpace(a.ProspectiveSatisfactionContractRef)
	if a.ProspectiveTargetSatisfactionConstraint != nil {
		value := a.ProspectiveTargetSatisfactionConstraint.Normalized()
		a.ProspectiveTargetSatisfactionConstraint = &value
	}
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
	if a.ProspectiveTargetSatisfactionConstraint != nil {
		value := a.ProspectiveTargetSatisfactionConstraint.Clone()
		a.ProspectiveTargetSatisfactionConstraint = &value
	}
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
		if a.ProspectiveSatisfactionContractRef != "" || a.ProspectiveTargetSatisfactionConstraint != nil {
			return fmt.Errorf("runtime semantic existing-scope owner-fact acquisition cannot declare prospective satisfaction")
		}
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if a.ProspectiveSatisfactionContractRef == "" || a.ProspectiveTargetSatisfactionConstraint == nil {
			return fmt.Errorf("runtime semantic prospective owner-fact acquisition requires a prospective satisfaction contract and typed all-of target constraint")
		}
		constraint := a.ProspectiveTargetSatisfactionConstraint.Normalized()
		if err := constraint.Validate(); err != nil {
			return err
		}
		if constraint.OwnerModuleID != a.Requirement.OwnerModuleID ||
			constraint.ProspectiveSatisfactionContractRef != a.ProspectiveSatisfactionContractRef {
			return fmt.Errorf("runtime semantic prospective target constraint does not match required owner fact")
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
	CoverageWorkItemID common.CoverageWorkItemID
	// DeclarationRef is derived from the sealed Definition source. It is not a
	// provider choice. ValidationProofRequirement further binds a Shell recipe
	// to the unique CWI check that may receive validation credit.
	DeclarationRef                                  string
	ValidationProofRequirement                      *RuntimeValidationProofRequirement
	RequiredEvidenceKinds                           []string
	DeclarationAcceptedStepKinds                    []string
	DeclarationEvidenceRole                         string
	DeclarationTargetRole                           string
	DeclarationTargetDiscoverySources               []string
	DeclarationTargetSelectorRef                    string
	DeclarationTargetSelectorOwnerRef               string
	DeclarationTargetSelectorSubjectRef             string
	DeclarationTargetSelectorCandidateRefs          []string
	DeclarationProspectiveTargetAllOfConstraintRefs []string
	RequireTargetGrounding                          bool
	RequireDirectEvidence                           bool
	RequireClaimEvidence                            bool
	RequireExecutableBehaviorEvidence               bool
	RequireCoverageTargets                          bool
	RequireAssessment                               bool
	RequiredOwnerFacts                              []RuntimeOwnerFactRequirement
	OwnerFactConvergences                           []RuntimeOwnerFactConvergence
	MetricConstraints                               []string
	ArtifactRefs                                    []string
	LineageAssertions                               []RuntimeLineageAssertionRule `json:",omitempty"`
	VerificationPolicy                              *RuntimeVerificationPolicy    `json:",omitempty"`
	OwnerFactAcquisition                            *RuntimeSemanticOwnerFactAcquisition
	ValidationFailureRepair                         *RuntimeSemanticValidationFailureRepair
	ImplementationWork                              *RuntimeSemanticImplementationWork
}

func (o RuntimeSemanticProofObligation) Normalized() RuntimeSemanticProofObligation {
	o.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(o.CoverageWorkItemID)))
	o.DeclarationRef = strings.TrimSpace(o.DeclarationRef)
	if o.ValidationProofRequirement != nil {
		value := o.ValidationProofRequirement.Clone()
		o.ValidationProofRequirement = &value
	}
	o.RequiredEvidenceKinds = normalizeStrings(o.RequiredEvidenceKinds)
	o.DeclarationAcceptedStepKinds = canonicalizeOperationKinds(o.DeclarationAcceptedStepKinds)
	o.DeclarationEvidenceRole = strings.TrimSpace(o.DeclarationEvidenceRole)
	o.DeclarationTargetRole = strings.TrimSpace(o.DeclarationTargetRole)
	o.DeclarationTargetDiscoverySources = normalizeStrings(o.DeclarationTargetDiscoverySources)
	o.DeclarationTargetSelectorRef = strings.TrimSpace(o.DeclarationTargetSelectorRef)
	o.DeclarationTargetSelectorOwnerRef = strings.TrimSpace(o.DeclarationTargetSelectorOwnerRef)
	o.DeclarationTargetSelectorSubjectRef = strings.TrimSpace(o.DeclarationTargetSelectorSubjectRef)
	o.DeclarationTargetSelectorCandidateRefs = normalizeStrings(o.DeclarationTargetSelectorCandidateRefs)
	o.DeclarationProspectiveTargetAllOfConstraintRefs = NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(o.DeclarationProspectiveTargetAllOfConstraintRefs)
	o.RequiredOwnerFacts = NormalizeRuntimeOwnerFactRequirements(o.RequiredOwnerFacts)
	o.LineageAssertions = append([]RuntimeLineageAssertionRule(nil), o.LineageAssertions...)
	o.OwnerFactConvergences = NormalizeRuntimeOwnerFactConvergences(o.OwnerFactConvergences)
	o.MetricConstraints = normalizeStrings(o.MetricConstraints)
	o.ArtifactRefs = normalizeStrings(o.ArtifactRefs)
	if o.VerificationPolicy != nil {
		value := o.VerificationPolicy.Normalized()
		o.VerificationPolicy = &value
	}
	if o.OwnerFactAcquisition != nil {
		value := o.OwnerFactAcquisition.Normalized()
		o.OwnerFactAcquisition = &value
	}
	if o.ImplementationWork != nil {
		value := o.ImplementationWork.Clone()
		o.ImplementationWork = &value
	}
	if o.ValidationFailureRepair != nil {
		value := o.ValidationFailureRepair.Normalized()
		o.ValidationFailureRepair = &value
	}
	return o
}

func (o RuntimeSemanticProofObligation) Clone() RuntimeSemanticProofObligation {
	o = o.Normalized()
	// acquisition 的嵌套路径规范化并非对所有原始输入幂等，保留原 Clone 次序。
	if o.OwnerFactAcquisition != nil {
		value := o.OwnerFactAcquisition.Clone()
		o.OwnerFactAcquisition = &value
	}
	return o
}

func (o RuntimeSemanticProofObligation) Validate() error {
	if err := ValidateRuntimeLineageAssertionRules(o.LineageAssertions); err != nil {
		return err
	}
	if o.VerificationPolicy != nil {
		if err := o.VerificationPolicy.Validate(); err != nil {
			return err
		}
	}
	if o.ValidationProofRequirement != nil {
		if err := o.ValidationProofRequirement.Validate(); err != nil {
			return err
		}
	}
	o = o.Normalized()
	if o.CoverageWorkItemID == "" || o.DeclarationRef == "" || strings.Contains(o.DeclarationRef, "*") || (len(o.RequiredEvidenceKinds) == 0 && !o.RequireAssessment) {
		return fmt.Errorf("runtime semantic proof obligation requires coverage, exact declaration and evidence or assessment")
	}
	if o.ValidationProofRequirement != nil {
		requirement := o.ValidationProofRequirement.Normalized()
		if err := requirement.Validate(); err != nil {
			return err
		}
		if requirement.CoverageWorkItemID != o.CoverageWorkItemID || requirement.DeclarationRef != o.DeclarationRef || !containsString(o.RequiredEvidenceKinds, EvidenceKindValidation) {
			return fmt.Errorf("runtime semantic validation proof requirement does not match obligation")
		}
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
	if len(o.DeclarationTargetSelectorCandidateRefs) != 0 && (!o.RequireTargetGrounding || o.DeclarationTargetSelectorRef == "" || o.DeclarationTargetSelectorOwnerRef == "" || o.DeclarationTargetSelectorSubjectRef == "") {
		return fmt.Errorf("runtime semantic proof obligation declaration selector candidates require target grounding and a complete selector")
	}
	if len(o.DeclarationProspectiveTargetAllOfConstraintRefs) != 0 {
		if !o.RequireTargetGrounding || o.DeclarationTargetSelectorRef == "" || o.DeclarationTargetSelectorOwnerRef == "" || o.DeclarationTargetSelectorSubjectRef == "" {
			return fmt.Errorf("runtime semantic proof obligation prospective target all-of declaration requires target grounding and a complete selector")
		}
		if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(o.DeclarationProspectiveTargetAllOfConstraintRefs); err != nil {
			return err
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
	}
	if o.ImplementationWork != nil {
		if o.OwnerFactAcquisition != nil || o.ValidationFailureRepair != nil || o.ImplementationWork.Scope.Request.CoverageWorkItemID != o.CoverageWorkItemID {
			return fmt.Errorf("runtime semantic implementation work purpose cannot acquire owner facts or bypass exact repair")
		}
		if err := o.ImplementationWork.Validate(); err != nil {
			return err
		}
	}
	if o.OwnerFactAcquisition != nil && o.ValidationFailureRepair != nil {
		return fmt.Errorf("runtime semantic proof obligation cannot acquire an owner fact and repair a validation failure in one generation")
	}
	if o.ValidationFailureRepair != nil {
		if len(o.RequiredEvidenceKinds) != 1 || o.RequiredEvidenceKinds[0] != EvidenceKindValidation {
			return fmt.Errorf("runtime semantic validation repair must preserve validation-only terminal evidence")
		}
		if err := o.ValidationFailureRepair.Validate(); err != nil {
			return err
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
			l.DeclarationRef != r.DeclarationRef ||
			!sameOptionalRuntimeValidationProofRequirement(l.ValidationProofRequirement, r.ValidationProofRequirement) ||
			!sameStrings(l.RequiredEvidenceKinds, r.RequiredEvidenceKinds) ||
			!sameStrings(l.DeclarationAcceptedStepKinds, r.DeclarationAcceptedStepKinds) ||
			l.DeclarationEvidenceRole != r.DeclarationEvidenceRole ||
			l.DeclarationTargetRole != r.DeclarationTargetRole ||
			!sameStrings(l.DeclarationTargetDiscoverySources, r.DeclarationTargetDiscoverySources) ||
			l.DeclarationTargetSelectorRef != r.DeclarationTargetSelectorRef ||
			l.DeclarationTargetSelectorOwnerRef != r.DeclarationTargetSelectorOwnerRef ||
			l.DeclarationTargetSelectorSubjectRef != r.DeclarationTargetSelectorSubjectRef ||
			!sameStrings(l.DeclarationTargetSelectorCandidateRefs, r.DeclarationTargetSelectorCandidateRefs) ||
			!SameRuntimeProspectiveTargetAllOfConstraintRefs(l.DeclarationProspectiveTargetAllOfConstraintRefs, r.DeclarationProspectiveTargetAllOfConstraintRefs) ||
			l.RequireTargetGrounding != r.RequireTargetGrounding ||
			l.RequireDirectEvidence != r.RequireDirectEvidence ||
			l.RequireClaimEvidence != r.RequireClaimEvidence ||
			l.RequireExecutableBehaviorEvidence != r.RequireExecutableBehaviorEvidence ||
			l.RequireCoverageTargets != r.RequireCoverageTargets ||
			l.RequireAssessment != r.RequireAssessment ||
			!sameRuntimeOwnerFactRequirements(l.RequiredOwnerFacts, r.RequiredOwnerFacts) ||
			!SameRuntimeOwnerFactConvergences(l.OwnerFactConvergences, r.OwnerFactConvergences) ||
			!sameStrings(l.MetricConstraints, r.MetricConstraints) || !sameStrings(l.ArtifactRefs, r.ArtifactRefs) ||
			!reflect.DeepEqual(l.LineageAssertions, r.LineageAssertions) || !SameRuntimeVerificationPolicy(l.VerificationPolicy, r.VerificationPolicy) ||
			!sameRuntimeSemanticOwnerFactAcquisition(l.OwnerFactAcquisition, r.OwnerFactAcquisition) ||
			!reflect.DeepEqual(l.ImplementationWork, r.ImplementationWork) ||
			!sameRuntimeSemanticValidationFailureRepair(l.ValidationFailureRepair, r.ValidationFailureRepair) {
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
		sameRuntimeProspectiveTargetSatisfactionConstraints(l.ProspectiveTargetSatisfactionConstraint, r.ProspectiveTargetSatisfactionConstraint) &&
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

func sameOptionalRuntimeValidationProofRequirement(left, right *RuntimeValidationProofRequirement) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return sameRuntimeValidationProofRequirement(*left, *right)
}

func sameRuntimeProspectiveTargetSatisfactionConstraints(left, right *RuntimeProspectiveTargetSatisfactionConstraint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return StableRuntimeProspectiveTargetSatisfactionConstraintHash(*left) == StableRuntimeProspectiveTargetSatisfactionConstraintHash(*right)
}

func runtimeSemanticProofObligationDefinitionSource(o RuntimeSemanticProofObligation) RuntimeDefinitionFocus {
	o = o.Normalized()
	return RuntimeDefinitionFocus{
		ID: func() string {
			if o.ValidationFailureRepair == nil {
				return ""
			}
			return o.ValidationFailureRepair.Normalized().FailedFocusID
		}(),
		CoverageWorkItemIDs:    []common.CoverageWorkItemID{o.CoverageWorkItemID},
		RequiredEvidenceKinds:  append([]string(nil), o.RequiredEvidenceKinds...),
		AcceptedStepKinds:      append([]string(nil), o.DeclarationAcceptedStepKinds...),
		EvidenceDeclarationRef: o.DeclarationRef,
		ValidationRecipeID: func() string {
			if o.ValidationProofRequirement == nil {
				return ""
			}
			return o.ValidationProofRequirement.ValidationRecipeID
		}(),
		RequiredCheckRefs: func() []string {
			if o.ValidationProofRequirement == nil {
				return nil
			}
			return append([]string(nil), o.ValidationProofRequirement.RequiredCheckRefs...)
		}(),
		EvidenceRole:                         o.DeclarationEvidenceRole,
		TargetRole:                           o.DeclarationTargetRole,
		TargetDiscoverySources:               append([]string(nil), o.DeclarationTargetDiscoverySources...),
		TargetSelectorRef:                    o.DeclarationTargetSelectorRef,
		TargetSelectorOwnerRef:               o.DeclarationTargetSelectorOwnerRef,
		TargetSelectorSubjectRef:             o.DeclarationTargetSelectorSubjectRef,
		TargetSelectorCandidateRefs:          append([]string(nil), o.DeclarationTargetSelectorCandidateRefs...),
		ProspectiveTargetAllOfConstraintRefs: append([]string(nil), o.DeclarationProspectiveTargetAllOfConstraintRefs...),
		RequireTargetGrounding:               o.RequireTargetGrounding,
		RequireDirectEvidence:                o.RequireDirectEvidence,
		RequireClaimEvidence:                 o.RequireClaimEvidence,
		RequireExecutableBehaviorEvidence:    o.RequireExecutableBehaviorEvidence,
		RequireCoverageTargets:               o.RequireCoverageTargets,
		RequireAssessment:                    o.RequireAssessment,
		RequiredOwnerFacts:                   append([]RuntimeOwnerFactRequirement(nil), o.RequiredOwnerFacts...),
		OwnerFactConvergences:                CloneRuntimeOwnerFactConvergences(o.OwnerFactConvergences),
		ValidationFailureRepairConvergence: func() *RuntimeValidationFailureRepairConvergence {
			if o.ValidationFailureRepair == nil {
				return nil
			}
			value := o.ValidationFailureRepair.convergence()
			return &value
		}(),
		MetricConstraints:  append([]string(nil), o.MetricConstraints...),
		ArtifactRefs:       append([]string(nil), o.ArtifactRefs...),
		VerificationPolicy: cloneRuntimeVerificationPolicy(o.VerificationPolicy),
	}.Normalized()
}

func runtimeSemanticProofObligationSourceFocus(o RuntimeSemanticProofObligation) CoverageFocus {
	o = o.Normalized()
	focus := runtimeSemanticProofObligationDefinitionSource(o).CoverageFocusDeclaration()
	if o.ValidationFailureRepair == nil {
		return focus
	}
	repair := o.ValidationFailureRepair.Normalized()
	focus.RequiredEvidenceKinds = []string{EvidenceKindStateChange}
	focus.TargetRole = repair.RepairTargetRole
	focus.TargetSelectorRef = repair.RepairDeclarationTargetSelectorRef
	focus.TargetSelectorOwnerRef = repair.RepairDeclarationTargetSelectorOwnerRef
	focus.TargetSelectorSubjectRef = repair.RepairDeclarationTargetSelectorSubjectRef
	focus.TargetSelectorCandidateRefs = append([]string(nil), repair.RepairDeclarationTargetSelectorCandidateRefs...)
	focus.AcceptedStepKinds = append([]string(nil), repair.RepairOperationKinds...)
	return focus.Normalized()
}

func writeRuntimeSemanticProofObligationsHash(h hashStringWriter, values []RuntimeSemanticProofObligation) {
	for _, raw := range normalizeRuntimeSemanticProofObligations(values) {
		value := raw.Normalized()
		writeHashString(h, string(value.CoverageWorkItemID))
		writeHashString(h, value.DeclarationRef)
		if value.ValidationProofRequirement == nil {
			writeHashString(h, "no-validation-proof-requirement")
		} else {
			writeHashString(h, value.ValidationProofRequirement.Hash)
		}
		writeHashStrings(h, value.RequiredEvidenceKinds)
		writeHashStrings(h, value.DeclarationAcceptedStepKinds)
		writeHashString(h, value.DeclarationEvidenceRole)
		writeHashString(h, value.DeclarationTargetRole)
		writeHashStrings(h, value.DeclarationTargetDiscoverySources)
		writeHashString(h, value.DeclarationTargetSelectorRef)
		writeHashString(h, value.DeclarationTargetSelectorOwnerRef)
		writeHashString(h, value.DeclarationTargetSelectorSubjectRef)
		writeHashStrings(h, value.DeclarationTargetSelectorCandidateRefs)
		writeHashStrings(h, value.DeclarationProspectiveTargetAllOfConstraintRefs)
		writeHashString(h, strconv.FormatBool(value.RequireTargetGrounding))
		writeHashString(h, strconv.FormatBool(value.RequireDirectEvidence))
		writeHashString(h, strconv.FormatBool(value.RequireClaimEvidence))
		writeHashString(h, strconv.FormatBool(value.RequireExecutableBehaviorEvidence))
		writeHashString(h, strconv.FormatBool(value.RequireCoverageTargets))
		writeHashString(h, strconv.FormatBool(value.RequireAssessment))
		for _, requirement := range value.RequiredOwnerFacts {
			writeHashString(h, RuntimeOwnerFactRequirementRef(requirement))
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
		for _, rule := range value.LineageAssertions {
			writeHashString(h, "lineage-assertion")
			writeHashString(h, StableRuntimeLineageAssertionRuleHash(rule))
		}
		if value.VerificationPolicy != nil {
			writeHashString(h, "verification-policy")
			writeHashString(h, StableRuntimeVerificationPolicyHash(*value.VerificationPolicy))
		}
		if value.OwnerFactAcquisition == nil {
			writeHashString(h, "no-owner-fact-acquisition")
		} else {
			acquisition := value.OwnerFactAcquisition.Normalized()
			writeHashString(h, RuntimeOwnerFactRequirementRef(acquisition.Requirement))
			writeHashString(h, acquisition.ProspectiveSatisfactionContractRef)
			if acquisition.ProspectiveTargetSatisfactionConstraint == nil {
				writeHashString(h, "no-prospective-target-satisfaction-constraint")
			} else {
				writeHashString(h, StableRuntimeProspectiveTargetSatisfactionConstraintHash(*acquisition.ProspectiveTargetSatisfactionConstraint))
			}
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
		if value.ImplementationWork != nil {
			writeHashString(h, StableRuntimeSemanticImplementationWorkHash(*value.ImplementationWork))
		}
		if value.ValidationFailureRepair == nil {
			writeHashString(h, "no-validation-failure-repair")
		} else {
			writeHashString(h, StableRuntimeSemanticValidationFailureRepairHash(*value.ValidationFailureRepair))
		}
	}
}

// compileRuntimeSemanticDefinitionProofObligation copies only immutable
// Definition/CWI facts into the terminal obligation. Owner catalogs may add a
// current acquisition or repair route later, but cannot alter this source.
func compileRuntimeSemanticDefinitionProofObligation(definitionRevision string, item RuntimeCoverageItem, source RuntimeDefinitionFocus) (RuntimeSemanticProofObligation, error) {
	item = item.Normalized()
	source = source.Normalized()
	if item.ID == "" || len(source.CoverageWorkItemIDs) != 1 || source.CoverageWorkItemIDs[0] != item.ID {
		return RuntimeSemanticProofObligation{}, fmt.Errorf("runtime semantic proof obligation has mismatched Definition source for coverage %q", item.ID)
	}
	declarationRef := strings.TrimSpace(source.EvidenceDeclarationRef)
	if declarationRef == "" || strings.Contains(declarationRef, "*") {
		return RuntimeSemanticProofObligation{}, fmt.Errorf("runtime semantic proof obligation coverage %q requires an exact admitted-CWI declaration", item.ID)
	}
	obligation := RuntimeSemanticProofObligation{
		CoverageWorkItemID:                              item.ID,
		DeclarationRef:                                  declarationRef,
		RequiredEvidenceKinds:                           append([]string(nil), source.RequiredEvidenceKinds...),
		DeclarationAcceptedStepKinds:                    append([]string(nil), source.AcceptedStepKinds...),
		DeclarationEvidenceRole:                         source.EvidenceRole,
		DeclarationTargetRole:                           source.TargetRole,
		DeclarationTargetDiscoverySources:               append([]string(nil), source.TargetDiscoverySources...),
		DeclarationTargetSelectorRef:                    source.TargetSelectorRef,
		DeclarationTargetSelectorOwnerRef:               source.TargetSelectorOwnerRef,
		DeclarationTargetSelectorSubjectRef:             source.TargetSelectorSubjectRef,
		DeclarationTargetSelectorCandidateRefs:          append([]string(nil), source.TargetSelectorCandidateRefs...),
		DeclarationProspectiveTargetAllOfConstraintRefs: append([]string(nil), source.ProspectiveTargetAllOfConstraintRefs...),
		RequireTargetGrounding:                          source.RequireTargetGrounding,
		RequireDirectEvidence:                           source.RequireDirectEvidence,
		RequireClaimEvidence:                            source.RequireClaimEvidence,
		RequireExecutableBehaviorEvidence:               source.RequireExecutableBehaviorEvidence,
		RequireCoverageTargets:                          source.RequireCoverageTargets,
		RequireAssessment:                               source.RequireAssessment,
		RequiredOwnerFacts:                              append([]RuntimeOwnerFactRequirement(nil), source.RequiredOwnerFacts...),
		OwnerFactConvergences:                           CloneRuntimeOwnerFactConvergences(source.OwnerFactConvergences),
		MetricConstraints:                               append([]string(nil), source.MetricConstraints...),
		ArtifactRefs:                                    append([]string(nil), source.ArtifactRefs...),
		LineageAssertions:                               append([]RuntimeLineageAssertionRule(nil), source.LineageAssertions...),
		VerificationPolicy:                              cloneRuntimeVerificationPolicy(source.VerificationPolicy),
	}
	validationRequirements, err := CompileRuntimeValidationProofRequirements(definitionRevision, []RuntimeCoverageItem{item}, []RuntimeDefinitionFocus{source})
	if err != nil {
		return RuntimeSemanticProofObligation{}, err
	}
	if requirement, found := RuntimeValidationProofRequirementForCoverage(validationRequirements, item.ID); found {
		value := requirement.Clone()
		obligation.ValidationProofRequirement = &value
	}
	obligation = obligation.Normalized()
	if err := obligation.Validate(); err != nil {
		return RuntimeSemanticProofObligation{}, fmt.Errorf("runtime semantic proof obligation for coverage %q: %w", item.ID, err)
	}
	return obligation, nil
}

type runtimeSemanticProofCatalogs struct {
	values  []RuntimeOperationAdmissibility
	byOwner map[string]RuntimeOperationAdmissibility
}

func validateRuntimeSemanticProofCatalogs(catalogs []RuntimeOperationAdmissibility) (runtimeSemanticProofCatalogs, error) {
	if err := validateRuntimeSemanticOwnerCatalogCollection(catalogs); err != nil {
		return runtimeSemanticProofCatalogs{}, err
	}
	// 集合门禁已完整校验每条 raw catalog；本次只构造 detached 规范化索引。
	validated := runtimeSemanticProofCatalogs{
		values:  NormalizeRuntimeOperationAdmissibilities(catalogs),
		byOwner: make(map[string]RuntimeOperationAdmissibility, len(catalogs)),
	}
	for _, value := range validated.values {
		validated.byOwner[value.OwnerModuleID] = value
	}
	return validated, nil
}

func compileRuntimeSemanticProofObligations(definitionRevision string, sourceFocuses []RuntimeDefinitionFocus, coverageItems []RuntimeCoverageItem, catalogs []RuntimeOperationAdmissibility) ([]RuntimeSemanticProofObligation, error) {
	sourceByCoverage, err := runtimeSemanticBindingSourceFocuses(sourceFocuses)
	if err != nil {
		return nil, err
	}
	validated, err := validateRuntimeSemanticProofCatalogs(catalogs)
	if err != nil {
		return nil, err
	}
	return compileRuntimeSemanticProofObligationsWithValidatedCatalogs(definitionRevision, sourceByCoverage, coverageItems, validated)
}

func compileRuntimeSemanticProofObligationsWithValidatedCatalogs(definitionRevision string, sourceByCoverage map[common.CoverageWorkItemID]RuntimeDefinitionFocus, coverageItems []RuntimeCoverageItem, validated runtimeSemanticProofCatalogs) ([]RuntimeSemanticProofObligation, error) {
	catalogs, catalogByOwner := validated.values, validated.byOwner
	ret := make([]RuntimeSemanticProofObligation, 0, len(coverageItems))
	for _, item := range NormalizeRuntimeCoverageItems(coverageItems) {
		source, found := sourceByCoverage[item.ID]
		if !found {
			return nil, fmt.Errorf("runtime semantic proof obligation has no Definition focus for coverage %q", item.ID)
		}
		source = source.Normalized()
		obligation, err := compileRuntimeSemanticDefinitionProofObligation(definitionRevision, item, source)
		if err != nil {
			return nil, err
		}
		if requirement := obligation.ValidationProofRequirement; requirement != nil {
			applicable := 0
			for _, catalog := range catalogs {
				if _, _, ok := RuntimeValidationSurfaceDeclaringProofRequirement(catalog.ValidationSurfaces, *requirement); ok {
					applicable++
				}
			}
			if applicable > 1 {
				return nil, fmt.Errorf("runtime validation proof requirement for coverage %q has ambiguous cross-owner applicability, got %d catalogs", item.ID, applicable)
			}
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
			if convergence.AcquisitionTargetPolicy == RuntimeOperationTargetPolicyProspectiveOwnerTarget && len(source.ProspectiveTargetAllOfConstraintRefs) != 0 &&
				!SameRuntimeProspectiveTargetAllOfConstraintRefs(constraints, source.ProspectiveTargetAllOfConstraintRefs) {
				return nil, fmt.Errorf("runtime semantic proof obligation for coverage %q acquisition constraints do not match the Definition all-of declaration", item.ID)
			}
			var prospectiveConstraint *RuntimeProspectiveTargetSatisfactionConstraint
			if convergence.AcquisitionTargetPolicy == RuntimeOperationTargetPolicyProspectiveOwnerTarget {
				requiredOwnerCatalog, found := catalogByOwner[convergence.Requirement.OwnerModuleID]
				if !found {
					return nil, fmt.Errorf("runtime semantic proof obligation for coverage %q has no required owner dependency catalog", item.ID)
				}
				// requiredOwnerCatalog 来自本次 raw 门禁后的 detached 索引；不重复校验整份目录。
				if err := convergence.Requirement.Validate(); err != nil {
					return nil, fmt.Errorf("runtime semantic proof obligation for coverage %q prospective target constraint: %w", item.ID, err)
				}
				resolved, _, resolveErr := resolveRuntimeProspectiveTargetSatisfactionConstraintNormalized(
					requiredOwnerCatalog,
					convergence.Requirement.Normalized(),
					strings.TrimSpace(convergence.ProspectiveSatisfactionContractRef),
				)
				if resolveErr != nil {
					return nil, fmt.Errorf("runtime semantic proof obligation for coverage %q prospective target constraint: %w", item.ID, resolveErr)
				}
				resolved = resolved.Clone()
				prospectiveConstraint = &resolved
			}
			obligation.OwnerFactAcquisition = &RuntimeSemanticOwnerFactAcquisition{
				Requirement:                             convergence.Requirement,
				ProspectiveSatisfactionContractRef:      convergence.ProspectiveSatisfactionContractRef,
				ProspectiveTargetSatisfactionConstraint: prospectiveConstraint,
				DeclarationTargetSelectorRef:            convergence.TargetSelectorRef,
				DeclarationTargetSelectorOwnerRef:       convergence.TargetSelectorOwnerRef,
				DeclarationTargetSelectorSubjectRef:     convergence.TargetSelectorSubjectRef,
				AcquisitionOwnerModuleID:                convergence.AcquisitionOwnerModuleID,
				AcquisitionOperationKind:                convergence.AcquisitionOperationKind,
				AcquisitionOperationVariantID:           convergence.AcquisitionOperationVariantID,
				AcquisitionTargetPolicy:                 convergence.AcquisitionTargetPolicy,
				AcquisitionTargetSelectorRef:            convergence.AcquisitionTargetSelectorRef,
				AcquisitionConstraintRefs:               constraints,
				OwnerCatalogSnapshotID:                  catalog.SnapshotID,
				OwnerCatalogObservedRevision:            catalog.ObservedRevision,
				ReobserveOwnerModuleID:                  convergence.ReobserveOwnerModuleID,
				ReobserveFactKind:                       convergence.ReobserveFactKind,
				MaxAcquisitionAttempts:                  convergence.MaxAcquisitionAttempts,
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
