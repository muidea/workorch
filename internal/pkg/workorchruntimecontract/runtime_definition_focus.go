package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeDefinitionFocus is the immutable activation declaration for exactly
// one coverage work item. It intentionally cannot carry generation-specific
// target resolutions, operations, recipes, checks or proof bindings. A
// prospective target is selected only by a bounded semantic proposal and does
// not become Runtime authority until SM-2 seals its evidence binding.
type RuntimeDefinitionFocus struct {
	ID                                 string
	SliceID                            string
	AssessmentScope                    string
	AssessmentID                       string
	AssessmentText                     string
	CoverageWorkItemIDs                []common.CoverageWorkItemID
	RequiredEvidenceKinds              []string
	EvidenceRole                       string
	TargetRole                         string
	TargetDiscoverySources             []string
	TargetSelectorRef                  string
	TargetSelectorOwnerRef             string
	TargetSelectorSubjectRef           string
	TargetSelectorCandidateRefs        []string
	RequireTargetGrounding             bool
	RequireDirectEvidence              bool
	RequireClaimEvidence               bool
	RequireExecutableBehaviorEvidence  bool
	RequireCoverageTargets             bool
	RequireAssessment                  bool
	RequiredOwnerFacts                 []RuntimeOwnerFactRequirement
	OwnerFactConvergences              []RuntimeOwnerFactConvergence
	ValidationFailureRepairConvergence *RuntimeValidationFailureRepairConvergence
	MetricConstraints                  []string
	ArtifactRefs                       []string
}

func (f RuntimeDefinitionFocus) Normalized() RuntimeDefinitionFocus {
	f.ID = strings.TrimSpace(f.ID)
	f.SliceID = strings.TrimSpace(f.SliceID)
	f.AssessmentScope = strings.TrimSpace(f.AssessmentScope)
	f.AssessmentID = strings.TrimSpace(f.AssessmentID)
	f.AssessmentText = strings.TrimSpace(f.AssessmentText)
	f.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(f.CoverageWorkItemIDs)
	f.RequiredEvidenceKinds = normalizeStrings(f.RequiredEvidenceKinds)
	f.EvidenceRole = strings.TrimSpace(f.EvidenceRole)
	f.TargetRole = strings.TrimSpace(f.TargetRole)
	f.TargetDiscoverySources = normalizeStrings(f.TargetDiscoverySources)
	f.TargetSelectorRef = strings.TrimSpace(f.TargetSelectorRef)
	f.TargetSelectorOwnerRef = strings.TrimSpace(f.TargetSelectorOwnerRef)
	f.TargetSelectorSubjectRef = strings.TrimSpace(f.TargetSelectorSubjectRef)
	f.TargetSelectorCandidateRefs = normalizeStrings(f.TargetSelectorCandidateRefs)
	f.RequiredOwnerFacts = NormalizeRuntimeOwnerFactRequirements(f.RequiredOwnerFacts)
	f.OwnerFactConvergences = NormalizeRuntimeOwnerFactConvergences(f.OwnerFactConvergences)
	f.ValidationFailureRepairConvergence = cloneRuntimeValidationFailureRepairConvergence(f.ValidationFailureRepairConvergence)
	f.MetricConstraints = normalizeStrings(f.MetricConstraints)
	f.ArtifactRefs = normalizeStrings(f.ArtifactRefs)
	return f
}

func (f RuntimeDefinitionFocus) Clone() RuntimeDefinitionFocus {
	f = f.Normalized()
	f.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), f.CoverageWorkItemIDs...)
	f.RequiredEvidenceKinds = append([]string(nil), f.RequiredEvidenceKinds...)
	f.TargetDiscoverySources = append([]string(nil), f.TargetDiscoverySources...)
	f.TargetSelectorCandidateRefs = append([]string(nil), f.TargetSelectorCandidateRefs...)
	f.RequiredOwnerFacts = append([]RuntimeOwnerFactRequirement(nil), f.RequiredOwnerFacts...)
	f.OwnerFactConvergences = CloneRuntimeOwnerFactConvergences(f.OwnerFactConvergences)
	f.ValidationFailureRepairConvergence = cloneRuntimeValidationFailureRepairConvergence(f.ValidationFailureRepairConvergence)
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
		CoverageWorkItemIDs:               append([]common.CoverageWorkItemID(nil), f.CoverageWorkItemIDs...),
		RequiredEvidenceKinds:             append([]string(nil), f.RequiredEvidenceKinds...),
		EvidenceRole:                      f.EvidenceRole,
		TargetRole:                        f.TargetRole,
		TargetDiscoverySources:            append([]string(nil), f.TargetDiscoverySources...),
		TargetSelectorRef:                 f.TargetSelectorRef,
		TargetSelectorOwnerRef:            f.TargetSelectorOwnerRef,
		TargetSelectorSubjectRef:          f.TargetSelectorSubjectRef,
		TargetSelectorCandidateRefs:       append([]string(nil), f.TargetSelectorCandidateRefs...),
		RequireTargetGrounding:            f.RequireTargetGrounding,
		RequireDirectEvidence:             f.RequireDirectEvidence,
		RequireClaimEvidence:              f.RequireClaimEvidence,
		RequireExecutableBehaviorEvidence: f.RequireExecutableBehaviorEvidence,
		RequireCoverageTargets:            f.RequireCoverageTargets,
		RequireAssessment:                 f.RequireAssessment,
		RequiredOwnerFacts:                append([]RuntimeOwnerFactRequirement(nil), f.RequiredOwnerFacts...),
		OwnerFactConvergences:             CloneRuntimeOwnerFactConvergences(f.OwnerFactConvergences),
		MetricConstraints:                 append([]string(nil), f.MetricConstraints...),
		ArtifactRefs:                      append([]string(nil), f.ArtifactRefs...),
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
			ID:                                value.ID,
			SliceID:                           value.SliceID,
			AssessmentScope:                   value.AssessmentScope,
			AssessmentID:                      value.AssessmentID,
			AssessmentText:                    value.AssessmentText,
			CoverageWorkItemIDs:               append([]common.CoverageWorkItemID(nil), value.CoverageWorkItemIDs...),
			RequiredEvidenceKinds:             append([]string(nil), value.RequiredEvidenceKinds...),
			EvidenceRole:                      value.EvidenceRole,
			TargetRole:                        value.TargetRole,
			TargetDiscoverySources:            append([]string(nil), value.TargetDiscoverySources...),
			TargetSelectorRef:                 value.TargetSelectorRef,
			TargetSelectorOwnerRef:            value.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef:          value.TargetSelectorSubjectRef,
			TargetSelectorCandidateRefs:       append([]string(nil), value.TargetSelectorCandidateRefs...),
			RequireTargetGrounding:            value.RequireTargetGrounding,
			RequireDirectEvidence:             value.RequireDirectEvidence,
			RequireClaimEvidence:              value.RequireClaimEvidence,
			RequireExecutableBehaviorEvidence: value.RequireExecutableBehaviorEvidence,
			RequireCoverageTargets:            value.RequireCoverageTargets,
			RequireAssessment:                 value.RequireAssessment,
			RequiredOwnerFacts:                append([]RuntimeOwnerFactRequirement(nil), value.RequiredOwnerFacts...),
			OwnerFactConvergences:             CloneRuntimeOwnerFactConvergences(value.OwnerFactConvergences),
			MetricConstraints:                 append([]string(nil), value.MetricConstraints...),
			ArtifactRefs:                      append([]string(nil), value.ArtifactRefs...),
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
	seed = seed.Clone()
	focuses = NormalizeRuntimeDefinitionFocuses(focuses)
	if len(focuses) == 0 {
		return fmt.Errorf("runtime definition focuses require one declaration per coverage item")
	}
	items := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(seed.CoverageItems))
	for _, item := range seed.CoverageItems {
		items[item.ID] = item
	}
	seenIDs := make(map[string]struct{}, len(focuses))
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(items))
	for _, focus := range focuses {
		if focus.ID == "" || focus.SliceID == "" || len(focus.CoverageWorkItemIDs) != 1 || (len(focus.RequiredEvidenceKinds) == 0 && !focus.RequireAssessment) {
			return fmt.Errorf("runtime definition focus requires identity, slice, one coverage item and evidence or assessment")
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
		if _, duplicate := seenCoverage[coverageID]; duplicate {
			return fmt.Errorf("runtime definition focuses duplicate coverage %q", coverageID)
		}
		seenCoverage[coverageID] = struct{}{}
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
		sameStrings(left.RequiredEvidenceKinds, right.RequiredEvidenceKinds) && left.EvidenceRole == right.EvidenceRole &&
		left.TargetRole == right.TargetRole && sameStrings(left.TargetDiscoverySources, right.TargetDiscoverySources) &&
		left.TargetSelectorRef == right.TargetSelectorRef && left.TargetSelectorOwnerRef == right.TargetSelectorOwnerRef &&
		left.TargetSelectorSubjectRef == right.TargetSelectorSubjectRef && sameStrings(left.TargetSelectorCandidateRefs, right.TargetSelectorCandidateRefs) &&
		left.RequireTargetGrounding == right.RequireTargetGrounding && left.RequireDirectEvidence == right.RequireDirectEvidence &&
		left.RequireClaimEvidence == right.RequireClaimEvidence && left.RequireExecutableBehaviorEvidence == right.RequireExecutableBehaviorEvidence &&
		left.RequireCoverageTargets == right.RequireCoverageTargets && left.RequireAssessment == right.RequireAssessment &&
		sameRuntimeOwnerFactRequirements(left.RequiredOwnerFacts, right.RequiredOwnerFacts) &&
		SameRuntimeOwnerFactConvergences(left.OwnerFactConvergences, right.OwnerFactConvergences) &&
		SameRuntimeValidationFailureRepairConvergence(left.ValidationFailureRepairConvergence, right.ValidationFailureRepairConvergence) &&
		sameStrings(left.MetricConstraints, right.MetricConstraints) && sameStrings(left.ArtifactRefs, right.ArtifactRefs)
}
