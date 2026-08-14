package workorchruntimecontract

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

// CompileRuntimeDefinitionFocuses projects bootstrap focus inputs onto the
// immutable declaration surface. Generation-specific target, operation,
// recipe and binding fields are intentionally discarded.
func CompileRuntimeDefinitionFocuses(values []CoverageFocus) []CoverageFocus {
	ret := make([]CoverageFocus, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		declaration := CoverageFocus{
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
			RequireDirectEvidence:             value.RequireDirectEvidence,
			RequireClaimEvidence:              value.RequireClaimEvidence,
			RequireExecutableBehaviorEvidence: value.RequireExecutableBehaviorEvidence,
			RequireCoverageTargets:            value.RequireCoverageTargets,
			RequireAssessment:                 value.RequireAssessment,
			MetricConstraints:                 append([]string(nil), value.MetricConstraints...),
			ArtifactRefs:                      append([]string(nil), value.ArtifactRefs...),
		}
		if value.TargetSelectorRef != "" && value.TargetSelectorOwnerRef != "" {
			declaration.TargetSelectorRef = value.TargetSelectorRef
			declaration.TargetSelectorOwnerRef = value.TargetSelectorOwnerRef
			declaration.TargetSelectorSubjectRef = value.TargetSelectorSubjectRef
			declaration.TargetSelectorCandidateRefs = append([]string(nil), value.TargetSelectorCandidateRefs...)
		}
		ret = append(ret, declaration.Normalized())
	}
	return ret
}

// ValidateRuntimeDefinitionFocuses verifies the immutable activation
// declaration inventory. It deliberately rejects semantic binding fields:
// target, operation and proof authority belongs to an SM-2 binding generation.
func ValidateRuntimeDefinitionFocuses(seed RuntimeDefinitionSeed, focuses []CoverageFocus) error {
	seed = seed.Clone()
	focuses = normalizeCoverageFocusQueue(focuses)
	if len(focuses) == 0 {
		return fmt.Errorf("runtime definition focuses require one declaration per coverage item")
	}
	items := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(seed.CoverageItems))
	for _, item := range seed.CoverageItems {
		items[item.ID] = item
	}
	seen := make(map[common.CoverageWorkItemID]struct{}, len(items))
	for _, focus := range focuses {
		if focus.ID == "" || focus.SliceID == "" || len(focus.CoverageWorkItemIDs) != 1 || (len(focus.RequiredEvidenceKinds) == 0 && !focus.RequireAssessment) {
			return fmt.Errorf("runtime definition focus requires identity, slice, one coverage item and evidence or assessment")
		}
		coverageID := focus.CoverageWorkItemIDs[0]
		item, found := items[coverageID]
		if !found || item.SliceID != focus.SliceID {
			return fmt.Errorf("runtime definition focus %q references mismatched coverage %q", focus.ID, coverageID)
		}
		if _, duplicate := seen[coverageID]; duplicate {
			return fmt.Errorf("runtime definition focuses duplicate coverage %q", coverageID)
		}
		seen[coverageID] = struct{}{}
		if len(focus.CoverageEvidenceBindings) != 0 || focus.EvidenceDeclarationRef != "" || len(focus.TargetRefs) != 0 || focus.ProspectiveTargetRef != "" || len(focus.AcceptedStepKinds) != 0 || len(focus.AcceptedOperationVariants) != 0 || len(focus.RequiredCheckRefs) != 0 || focus.ValidationRecipeID != "" {
			return fmt.Errorf("runtime definition focus %q contains generation-specific semantic authority", focus.ID)
		}
		if focus.TargetSelectorRef != "" && focus.TargetSelectorOwnerRef == "" {
			return fmt.Errorf("runtime definition focus %q selector requires owner", focus.ID)
		}
	}
	if len(seen) != len(items) {
		return fmt.Errorf("runtime definition focuses do not cover every definition seed item")
	}
	return nil
}
