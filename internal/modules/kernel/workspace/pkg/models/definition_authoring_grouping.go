package workspace

import (
	"fmt"
	"slices"
)

// BindAuthoringSourceGroups computes representation, not semantic coverage.
// Grouping is bounded by the existing 32-source structural limit. Each exact
// obligation and its independent verification remain present and all-of.
func BindAuthoringSourceGroups(d DefinitionDerivationRequest, m DefinitionAuthoringMapping) (DefinitionAuthoringMapping, error) {
	m = m.Clone()
	if d.Proposal == nil {
		return m, fmt.Errorf("source grouping requires derived sources")
	}
	items, err := d.Proposal.CandidateItems(d.Input, &d.Template)
	if err != nil {
		return m, err
	}
	resources, err := AuthoringSpecificationResources(d.Input, d.Resources)
	if err != nil {
		return m, err
	}
	ids := []string{}
	for _, r := range resources {
		ids = append(ids, r.ID)
	}
	m.GoalRequirement = DefinitionAuthoringGoalMapping{DimensionRef: "sources", ResourceIDs: slices.Clone(ids)}
	m.Dimensions = []DefinitionAuthoringDimensionMapping{{Ref: "sources", ResourceIDs: slices.Clone(ids)}}
	m.Requirements = []DefinitionAuthoringRequirementMapping{}
	groupFor := map[string]string{}
	for _, kind := range []string{GoalRequirementUserExplicit, GoalRequirementTemplateRequired, GoalRequirementDerived} {
		refs := []string{}
		for _, item := range items {
			if authoringItemKind(item) == kind {
				refs = append(refs, item.CandidateItemRef)
			}
		}
		for offset := 0; offset < len(refs); offset += 32 {
			end := min(offset+32, len(refs))
			ref := fmt.Sprintf("source-%s-%03d", kind, offset/32+1)
			m.Requirements = append(m.Requirements, DefinitionAuthoringRequirementMapping{Ref: ref, DimensionRef: "sources", SourceItemRefs: slices.Clone(refs[offset:end]), ResourceIDs: slices.Clone(ids), Text: "Source obligations: " + kind})
			for _, source := range refs[offset:end] {
				groupFor[source] = ref
			}
		}
	}
	for i := range m.Obligations {
		m.Obligations[i].RequirementRef = groupFor[m.Obligations[i].CandidateItemRef]
	}
	return m, nil
}
