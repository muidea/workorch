package workorchruntimecontract

import (
	"fmt"
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeDefinitionFocus is the immutable activation declaration for exactly
// one coverage work item. It intentionally cannot carry generation-specific
// target resolutions, operations or proof bindings. Profile-declared recipe
// and check constraints remain immutable inputs and are compiled into an exact
// per-CWI proof requirement before owner admissibility acquisition. A
// prospective target is selected only by a bounded semantic proposal and does
// not become Runtime authority until SM-2 seals its evidence binding.
type RuntimeDefinitionFocus struct {
	ID                                   string
	SliceID                              string
	AssessmentScope                      string
	AssessmentID                         string
	AssessmentText                       string
	CoverageWorkItemIDs                  []common.CoverageWorkItemID
	RequiredEvidenceKinds                []string
	AcceptedStepKinds                    []string
	EvidenceRole                         string
	TargetRole                           string
	TargetDiscoverySources               []string
	TargetSelectorRef                    string
	TargetSelectorOwnerRef               string
	TargetSelectorSubjectRef             string
	TargetSelectorCandidateRefs          []string
	ProspectiveTargetAllOfConstraintRefs []string
	RequireTargetGrounding               bool
	RequireDirectEvidence                bool
	RequireClaimEvidence                 bool
	RequireExecutableBehaviorEvidence    bool
	RequireCoverageTargets               bool
	RequireAssessment                    bool
	RequiredOwnerFacts                   []RuntimeOwnerFactRequirement
	OwnerFactConvergences                []RuntimeOwnerFactConvergence
	// These values are immutable Profile declaration inputs, not a selected
	// operation. EvidenceDeclarationRef is the Application-authored exact
	// identity of this admitted CWI; wildcard Profile declarations are quality
	// gates only and may not be copied here as semantic proof authority.
	EvidenceDeclarationRef             string
	ValidationRecipeID                 string
	RequiredCheckRefs                  []string
	ValidationFailureRepairConvergence *RuntimeValidationFailureRepairConvergence
	LineageAssertions                  []RuntimeLineageAssertionRule `json:",omitempty"`
	// ImplementationWorkPolicy is an exact-CWI projection of the sealed
	// Profile declaration, not a working binding or executable operation.
	ImplementationWorkPolicy *RuntimeImplementationWorkPolicy `json:",omitempty"`
	VerificationPolicy       *RuntimeVerificationPolicy       `json:",omitempty"`
	MetricConstraints        []string
	ArtifactRefs             []string
}

// CanonicalRuntimeDefinitionEvidenceDeclarationRef returns the sole
// Definition-owned declaration identity for one admitted source item. Profile
// wildcard declarations are intentionally not accepted as Runtime authority;
// the Application must project the exact admitted source through this helper.
func CanonicalRuntimeDefinitionEvidenceDeclarationRef(sourceScope, sourceID string) string {
	sourceScope = strings.TrimSpace(sourceScope)
	sourceID = strings.TrimSpace(sourceID)
	if sourceScope == "" || sourceID == "" {
		return ""
	}
	return "definition/" + sourceScope + "/" + sourceID
}

func (f RuntimeDefinitionFocus) Normalized() RuntimeDefinitionFocus {
	f.ID = strings.TrimSpace(f.ID)
	f.SliceID = strings.TrimSpace(f.SliceID)
	f.AssessmentScope = strings.TrimSpace(f.AssessmentScope)
	f.AssessmentID = strings.TrimSpace(f.AssessmentID)
	f.AssessmentText = strings.TrimSpace(f.AssessmentText)
	f.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(f.CoverageWorkItemIDs)
	f.RequiredEvidenceKinds = normalizeStrings(f.RequiredEvidenceKinds)
	f.AcceptedStepKinds = canonicalizeOperationKinds(f.AcceptedStepKinds)
	f.EvidenceRole = strings.TrimSpace(f.EvidenceRole)
	f.TargetRole = strings.TrimSpace(f.TargetRole)
	f.TargetDiscoverySources = normalizeStrings(f.TargetDiscoverySources)
	f.TargetSelectorRef = strings.TrimSpace(f.TargetSelectorRef)
	f.TargetSelectorOwnerRef = strings.TrimSpace(f.TargetSelectorOwnerRef)
	f.TargetSelectorSubjectRef = strings.TrimSpace(f.TargetSelectorSubjectRef)
	f.TargetSelectorCandidateRefs = normalizeStrings(f.TargetSelectorCandidateRefs)
	f.ProspectiveTargetAllOfConstraintRefs = NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(f.ProspectiveTargetAllOfConstraintRefs)
	f.RequiredOwnerFacts = NormalizeRuntimeOwnerFactRequirements(f.RequiredOwnerFacts)
	f.LineageAssertions = append([]RuntimeLineageAssertionRule(nil), f.LineageAssertions...)
	f.OwnerFactConvergences = NormalizeRuntimeOwnerFactConvergences(f.OwnerFactConvergences)
	f.EvidenceDeclarationRef = strings.TrimSpace(f.EvidenceDeclarationRef)
	f.ValidationRecipeID = strings.TrimSpace(f.ValidationRecipeID)
	f.RequiredCheckRefs = normalizeRuntimeValidationProofCheckRefs(f.RequiredCheckRefs)
	f.ValidationFailureRepairConvergence = cloneRuntimeValidationFailureRepairConvergence(f.ValidationFailureRepairConvergence)
	if f.ImplementationWorkPolicy != nil {
		value := f.ImplementationWorkPolicy.Normalized()
		f.ImplementationWorkPolicy = &value
	}
	if f.VerificationPolicy != nil {
		value := f.VerificationPolicy.Normalized()
		f.VerificationPolicy = &value
	}
	f.MetricConstraints = normalizeStrings(f.MetricConstraints)
	f.ArtifactRefs = normalizeStrings(f.ArtifactRefs)
	return f
}

func (f RuntimeDefinitionFocus) Clone() RuntimeDefinitionFocus {
	f = f.Normalized()
	f.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), f.CoverageWorkItemIDs...)
	f.RequiredEvidenceKinds = append([]string(nil), f.RequiredEvidenceKinds...)
	f.AcceptedStepKinds = append([]string(nil), f.AcceptedStepKinds...)
	f.TargetDiscoverySources = append([]string(nil), f.TargetDiscoverySources...)
	f.TargetSelectorCandidateRefs = append([]string(nil), f.TargetSelectorCandidateRefs...)
	f.ProspectiveTargetAllOfConstraintRefs = append([]string(nil), f.ProspectiveTargetAllOfConstraintRefs...)
	f.RequiredOwnerFacts = append([]RuntimeOwnerFactRequirement(nil), f.RequiredOwnerFacts...)
	f.OwnerFactConvergences = CloneRuntimeOwnerFactConvergences(f.OwnerFactConvergences)
	f.RequiredCheckRefs = append([]string(nil), f.RequiredCheckRefs...)
	f.ValidationFailureRepairConvergence = cloneRuntimeValidationFailureRepairConvergence(f.ValidationFailureRepairConvergence)
	f.VerificationPolicy = cloneRuntimeVerificationPolicy(f.VerificationPolicy)
	f.MetricConstraints = append([]string(nil), f.MetricConstraints...)
	f.ArtifactRefs = append([]string(nil), f.ArtifactRefs...)
	return f
}

// CoverageFocusDeclaration is a read-only adapter for pure helpers that were
// intentionally written over CoverageFocus before the declaration/generation
// type split. It never supplies generation-only fields or Profile convergence
// declarations; those remain on this immutable Definition value.
func (f RuntimeDefinitionFocus) CoverageFocusDeclaration() CoverageFocus {
	f = f.Clone()
	return CoverageFocus{
		ID: f.ID, SliceID: f.SliceID, AssessmentScope: f.AssessmentScope,
		AssessmentID: f.AssessmentID, AssessmentText: f.AssessmentText,
		CoverageWorkItemIDs:                  append([]common.CoverageWorkItemID(nil), f.CoverageWorkItemIDs...),
		RequiredEvidenceKinds:                append([]string(nil), f.RequiredEvidenceKinds...),
		AcceptedStepKinds:                    append([]string(nil), f.AcceptedStepKinds...),
		EvidenceRole:                         f.EvidenceRole,
		TargetRole:                           f.TargetRole,
		TargetDiscoverySources:               append([]string(nil), f.TargetDiscoverySources...),
		TargetSelectorRef:                    f.TargetSelectorRef,
		TargetSelectorOwnerRef:               f.TargetSelectorOwnerRef,
		TargetSelectorSubjectRef:             f.TargetSelectorSubjectRef,
		TargetSelectorCandidateRefs:          append([]string(nil), f.TargetSelectorCandidateRefs...),
		ProspectiveTargetAllOfConstraintRefs: append([]string(nil), f.ProspectiveTargetAllOfConstraintRefs...),
		RequireTargetGrounding:               f.RequireTargetGrounding,
		RequireDirectEvidence:                f.RequireDirectEvidence,
		RequireClaimEvidence:                 f.RequireClaimEvidence,
		RequireExecutableBehaviorEvidence:    f.RequireExecutableBehaviorEvidence,
		RequireCoverageTargets:               f.RequireCoverageTargets,
		RequireAssessment:                    f.RequireAssessment,
		RequiredOwnerFacts:                   append([]RuntimeOwnerFactRequirement(nil), f.RequiredOwnerFacts...),
		OwnerFactConvergences:                CloneRuntimeOwnerFactConvergences(f.OwnerFactConvergences),
		EvidenceDeclarationRef:               f.EvidenceDeclarationRef,
		ValidationRecipeID:                   f.ValidationRecipeID,
		RequiredCheckRefs:                    append([]string(nil), f.RequiredCheckRefs...),
		VerificationPolicy:                   cloneRuntimeVerificationPolicy(f.VerificationPolicy),
		MetricConstraints:                    append([]string(nil), f.MetricConstraints...),
		ArtifactRefs:                         append([]string(nil), f.ArtifactRefs...),
	}.Normalized()
}

// RuntimeFocusSeed returns the non-authoritative queue cursor used before the
// first SM-2 semantic generation. Definition declarations remain available
// only through RuntimeFacts.DefinitionFocuses.
func (f RuntimeDefinitionFocus) RuntimeFocusSeed() CoverageFocus {
	f = f.Normalized()
	return CoverageFocus{
		ID:                    f.ID,
		SliceID:               f.SliceID,
		AssessmentScope:       f.AssessmentScope,
		AssessmentID:          f.AssessmentID,
		AssessmentText:        f.AssessmentText,
		CoverageWorkItemIDs:   append([]common.CoverageWorkItemID(nil), f.CoverageWorkItemIDs...),
		RequiredEvidenceKinds: append([]string(nil), f.RequiredEvidenceKinds...),
		RequireAssessment:     f.RequireAssessment,
		VerificationPolicy:    cloneRuntimeVerificationPolicy(f.VerificationPolicy),
	}.Normalized()
}

// CompileRuntimeDefinitionFocuses projects focus-shaped activation output onto
// the immutable declaration surface. Profile-only declarations that have no
// mutable focus field are attached by the activation compiler directly to the
// returned RuntimeDefinitionFocus. Generation-specific authority has no field
// in this type and therefore cannot leak into a fresh Runtime aggregate.
func CompileRuntimeDefinitionFocuses(values []CoverageFocus) []RuntimeDefinitionFocus {
	ret := make([]RuntimeDefinitionFocus, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		ret = append(ret, RuntimeDefinitionFocus{
			ID:                                   value.ID,
			SliceID:                              value.SliceID,
			AssessmentScope:                      value.AssessmentScope,
			AssessmentID:                         value.AssessmentID,
			AssessmentText:                       value.AssessmentText,
			CoverageWorkItemIDs:                  append([]common.CoverageWorkItemID(nil), value.CoverageWorkItemIDs...),
			RequiredEvidenceKinds:                append([]string(nil), value.RequiredEvidenceKinds...),
			AcceptedStepKinds:                    append([]string(nil), value.AcceptedStepKinds...),
			EvidenceRole:                         value.EvidenceRole,
			TargetRole:                           value.TargetRole,
			TargetDiscoverySources:               append([]string(nil), value.TargetDiscoverySources...),
			TargetSelectorRef:                    value.TargetSelectorRef,
			TargetSelectorOwnerRef:               value.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef:             value.TargetSelectorSubjectRef,
			TargetSelectorCandidateRefs:          append([]string(nil), value.TargetSelectorCandidateRefs...),
			ProspectiveTargetAllOfConstraintRefs: append([]string(nil), value.ProspectiveTargetAllOfConstraintRefs...),
			RequireTargetGrounding:               value.RequireTargetGrounding,
			RequireDirectEvidence:                value.RequireDirectEvidence,
			RequireClaimEvidence:                 value.RequireClaimEvidence,
			RequireExecutableBehaviorEvidence:    value.RequireExecutableBehaviorEvidence,
			RequireCoverageTargets:               value.RequireCoverageTargets,
			RequireAssessment:                    value.RequireAssessment,
			RequiredOwnerFacts:                   append([]RuntimeOwnerFactRequirement(nil), value.RequiredOwnerFacts...),
			OwnerFactConvergences:                CloneRuntimeOwnerFactConvergences(value.OwnerFactConvergences),
			EvidenceDeclarationRef:               value.EvidenceDeclarationRef,
			ValidationRecipeID:                   value.ValidationRecipeID,
			RequiredCheckRefs:                    append([]string(nil), value.RequiredCheckRefs...),
			VerificationPolicy:                   cloneRuntimeVerificationPolicy(value.VerificationPolicy),
			MetricConstraints:                    append([]string(nil), value.MetricConstraints...),
			ArtifactRefs:                         append([]string(nil), value.ArtifactRefs...),
		}.Normalized())
	}
	return nilIfEmpty(ret)
}

func NormalizeRuntimeDefinitionFocuses(values []RuntimeDefinitionFocus) []RuntimeDefinitionFocus {
	ret := make([]RuntimeDefinitionFocus, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.ID != "" {
			ret = append(ret, value)
		}
	}
	return nilIfEmpty(ret)
}

func CloneRuntimeDefinitionFocuses(values []RuntimeDefinitionFocus) []RuntimeDefinitionFocus {
	values = NormalizeRuntimeDefinitionFocuses(values)
	ret := make([]RuntimeDefinitionFocus, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

func RuntimeFocusSeedsFromDefinitionFocuses(values []RuntimeDefinitionFocus) []CoverageFocus {
	values = NormalizeRuntimeDefinitionFocuses(values)
	ret := make([]CoverageFocus, 0, len(values))
	for _, value := range values {
		ret = append(ret, value.RuntimeFocusSeed())
	}
	return nilIfEmpty(ret)
}

// ValidateRuntimeDefinitionFocuses verifies the immutable activation
// declaration inventory and its exact owner-fact acquisition bindings.
func ValidateRuntimeDefinitionFocuses(seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus) error {
	for _, raw := range focuses {
		if duplicate := duplicateRuntimeValidationProofCheckRef(raw.RequiredCheckRefs); duplicate != "" {
			return fmt.Errorf("runtime definition focus %q duplicates check %q", strings.TrimSpace(raw.ID), duplicate)
		}
		if len(raw.CoverageWorkItemIDs) != len(normalizeCoverageWorkItemIDs(raw.CoverageWorkItemIDs)) {
			return fmt.Errorf("runtime definition focus %q has duplicate or empty coverage identity", strings.TrimSpace(raw.ID))
		}
	}
	seed = seed.Clone()
	focuses = NormalizeRuntimeDefinitionFocuses(focuses)
	if len(focuses) == 0 {
		return fmt.Errorf("runtime definition focuses require one declaration per coverage item")
	}
	items := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(seed.CoverageItems))
	for _, item := range seed.CoverageItems {
		if item.HistoricalAcceptance != nil && (seed.SourceObligations == nil ||
			ValidateRuntimeEvidencePurposeMappingCoverage(*seed.SourceObligations, seed.CoverageItems) != nil ||
			ValidateRuntimeSourceObligationSeed(seed) != nil) {
			return fmt.Errorf("runtime definition focus historical checks require original confirmed source mapping")
		}
		items[item.ID] = item
	}
	seenIDs := make(map[string]struct{}, len(focuses))
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(items))
	for _, focus := range focuses {
		if focus.ID == "" || focus.SliceID == "" || len(focus.CoverageWorkItemIDs) != 1 || (len(focus.RequiredEvidenceKinds) == 0 && !focus.RequireAssessment) {
			return fmt.Errorf("runtime definition focus requires identity, slice, one coverage item and evidence or assessment")
		}
		canonicalDeclarationRef := CanonicalRuntimeDefinitionEvidenceDeclarationRef(focus.AssessmentScope, focus.AssessmentID)
		if focus.EvidenceDeclarationRef != canonicalDeclarationRef {
			return fmt.Errorf("runtime definition focus %q declaration %q does not match canonical admitted-CWI declaration %q", focus.ID, focus.EvidenceDeclarationRef, canonicalDeclarationRef)
		}
		if _, duplicate := seenIDs[focus.ID]; duplicate {
			return fmt.Errorf("runtime definition focus %q is duplicated", focus.ID)
		}
		seenIDs[focus.ID] = struct{}{}
		coverageID := focus.CoverageWorkItemIDs[0]
		item, found := items[coverageID]
		if !found || item.SliceID != focus.SliceID {
			return fmt.Errorf("runtime definition focus %q references mismatched coverage %q", focus.ID, coverageID)
		}
		if focus.AssessmentScope != item.SourceScope || focus.AssessmentID != item.SourceID || focus.AssessmentText != item.TargetText {
			return fmt.Errorf("runtime definition focus %q assessment source does not exactly match definition seed coverage %q", focus.ID, coverageID)
		}
		// A confirmed mapped historical endpoint retains its original checks
		// as provenance only. The independently validated consumer seed owns
		// their ordinary all-of execution; no local validation is fabricated.
		if len(item.SourceObligationChecks) != 0 && item.HistoricalAcceptance == nil &&
			(!containsString(focus.RequiredEvidenceKinds, EvidenceKindValidation) ||
				!runtimeOperationKindsContain(focus.AcceptedStepKinds, RuntimeBlockKindManagedValidationSession)) {
			return fmt.Errorf("runtime definition focus %q source obligations require exact owner-attested validation checks", focus.ID)
		}
		if _, duplicate := seenCoverage[coverageID]; duplicate {
			return fmt.Errorf("runtime definition focuses duplicate coverage %q", coverageID)
		}
		seenCoverage[coverageID] = struct{}{}
		if err := validateRuntimeDefinitionImplementationWorkPolicy(focus); err != nil {
			return err
		}
		selectorParts := 0
		for _, value := range []string{focus.TargetSelectorRef, focus.TargetSelectorOwnerRef, focus.TargetSelectorSubjectRef} {
			if value != "" {
				selectorParts++
			}
		}
		if selectorParts != 0 && selectorParts != 3 {
			return fmt.Errorf("runtime definition focus %q selector requires ref, owner and subject", focus.ID)
		}
		if len(focus.TargetSelectorCandidateRefs) != 0 && (!focus.RequireTargetGrounding || selectorParts != 3) {
			return fmt.Errorf("runtime definition focus %q selector candidates require target grounding and a complete selector", focus.ID)
		}
		if len(focus.ProspectiveTargetAllOfConstraintRefs) != 0 {
			if !focus.RequireTargetGrounding || selectorParts != 3 {
				return fmt.Errorf("runtime definition focus %q prospective target all-of declaration requires target grounding and a complete selector", focus.ID)
			}
			if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(focus.ProspectiveTargetAllOfConstraintRefs); err != nil {
				return fmt.Errorf("runtime definition focus %q: %w", focus.ID, err)
			}
		}
		if focus.RequireTargetGrounding && focus.TargetRole == "" {
			return fmt.Errorf("runtime definition focus %q target grounding requires a target role", focus.ID)
		}
		if len(focus.TargetDiscoverySources) != 0 {
			if err := ValidateRuntimeTargetGrounding(focus.RequireTargetGrounding, focus.TargetRole, nil, focus.TargetDiscoverySources, focus.TargetSelectorRef); err != nil {
				return fmt.Errorf("runtime definition focus %q: %w", focus.ID, err)
			}
		}
		if err := ValidateRuntimeOwnerFactRequirements(focus.RequiredOwnerFacts); err != nil {
			return fmt.Errorf("runtime definition focus %q owner facts: %w", focus.ID, err)
		}
		if err := ValidateRuntimeOwnerFactConvergences(focus.RequiredOwnerFacts, focus.OwnerFactConvergences); err != nil {
			return fmt.Errorf("runtime definition focus %q owner-fact convergence: %w", focus.ID, err)
		}
		if focus.ValidationFailureRepairConvergence != nil {
			if focus.AssessmentScope != "dodItem" || len(focus.RequiredEvidenceKinds) != 1 || focus.RequiredEvidenceKinds[0] != EvidenceKindValidation {
				return fmt.Errorf("runtime definition focus %q validation failure repair requires one validation-only dodItem CWI", focus.ID)
			}
			if err := focus.ValidationFailureRepairConvergence.Validate(); err != nil {
				return fmt.Errorf("runtime definition focus %q: %w", focus.ID, err)
			}
		}
		if focus.ValidationRecipeID != "" && !containsString(focus.RequiredEvidenceKinds, EvidenceKindValidation) {
			return fmt.Errorf("runtime definition focus %q recipe requires validation evidence", focus.ID)
		}
		if !containsString(focus.RequiredEvidenceKinds, EvidenceKindValidation) && len(focus.RequiredCheckRefs) != 0 {
			return fmt.Errorf("runtime definition focus %q carries validation-only declarations without validation evidence", focus.ID)
		}
		for _, convergence := range focus.OwnerFactConvergences {
			if convergence.RequirePreexisting {
				continue
			}
			if focus.TargetSelectorRef != convergence.TargetSelectorRef ||
				focus.TargetSelectorOwnerRef != convergence.TargetSelectorOwnerRef ||
				focus.TargetSelectorSubjectRef != convergence.TargetSelectorSubjectRef {
				return fmt.Errorf("runtime definition focus %q owner-fact acquisition selector does not match its declaration", focus.ID)
			}
		}
		if err := ValidateRuntimeLineageAssertionRules(focus.LineageAssertions); err != nil {
			return err
		}
		if focus.VerificationPolicy != nil {
			if err := focus.VerificationPolicy.Validate(); err != nil {
				return fmt.Errorf("runtime definition focus %q: %w", focus.ID, err)
			}
		}
	}
	if len(seenCoverage) != len(items) {
		return fmt.Errorf("runtime definition focuses do not cover every definition seed item")
	}
	return nil
}

func sameRuntimeDefinitionFocus(left, right RuntimeDefinitionFocus) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.ID == right.ID && left.SliceID == right.SliceID &&
		left.AssessmentScope == right.AssessmentScope && left.AssessmentID == right.AssessmentID && left.AssessmentText == right.AssessmentText &&
		sameCoverageWorkItemIDs(left.CoverageWorkItemIDs, right.CoverageWorkItemIDs) &&
		sameStrings(left.RequiredEvidenceKinds, right.RequiredEvidenceKinds) && sameStrings(left.AcceptedStepKinds, right.AcceptedStepKinds) && left.EvidenceRole == right.EvidenceRole &&
		left.TargetRole == right.TargetRole && sameStrings(left.TargetDiscoverySources, right.TargetDiscoverySources) &&
		left.TargetSelectorRef == right.TargetSelectorRef && left.TargetSelectorOwnerRef == right.TargetSelectorOwnerRef &&
		left.TargetSelectorSubjectRef == right.TargetSelectorSubjectRef && sameStrings(left.TargetSelectorCandidateRefs, right.TargetSelectorCandidateRefs) && SameRuntimeProspectiveTargetAllOfConstraintRefs(left.ProspectiveTargetAllOfConstraintRefs, right.ProspectiveTargetAllOfConstraintRefs) &&
		left.RequireTargetGrounding == right.RequireTargetGrounding && left.RequireDirectEvidence == right.RequireDirectEvidence &&
		left.RequireClaimEvidence == right.RequireClaimEvidence && left.RequireExecutableBehaviorEvidence == right.RequireExecutableBehaviorEvidence &&
		left.RequireCoverageTargets == right.RequireCoverageTargets && left.RequireAssessment == right.RequireAssessment &&
		sameRuntimeOwnerFactRequirements(left.RequiredOwnerFacts, right.RequiredOwnerFacts) &&
		SameRuntimeOwnerFactConvergences(left.OwnerFactConvergences, right.OwnerFactConvergences) &&
		left.EvidenceDeclarationRef == right.EvidenceDeclarationRef && left.ValidationRecipeID == right.ValidationRecipeID && sameStrings(left.RequiredCheckRefs, right.RequiredCheckRefs) &&
		SameRuntimeValidationFailureRepairConvergence(left.ValidationFailureRepairConvergence, right.ValidationFailureRepairConvergence) &&
		SameRuntimeImplementationWorkPolicy(left.ImplementationWorkPolicy, right.ImplementationWorkPolicy) &&
		slices.Equal(left.LineageAssertions, right.LineageAssertions) && SameRuntimeVerificationPolicy(left.VerificationPolicy, right.VerificationPolicy) &&
		sameStrings(left.MetricConstraints, right.MetricConstraints) && sameStrings(left.ArtifactRefs, right.ArtifactRefs)
}
