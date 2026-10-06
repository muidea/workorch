package definitionadmission

import (
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestSpecificationDoDSourcesPreservesConfirmationAfterImageOrder(t *testing.T) {
	items := []workspacemodel.DefinitionCandidateItem{
		{CandidateItemRef: "derived-dod:generated:000001"},
		{CandidateItemRef: "derived-dod:template:000001"},
		{CandidateItemRef: "derived-dod:user:000002"},
		{CandidateItemRef: "derived-dod:user:000001"},
	}
	mapping, ok := specificationDoDSources(items)
	if !ok || mapping[items[3].CandidateItemRef] != "dod-001" || mapping[items[2].CandidateItemRef] != "dod-002" || mapping[items[1].CandidateItemRef] != "dod-003" || mapping[items[0].CandidateItemRef] != "dod-004" {
		t.Fatalf("confirmation after-image source mapping = %v (ok=%t)", mapping, ok)
	}
	items[3].CandidateItemRef = "derived-dod:user:000003"
	if _, ok := specificationDoDSources(items); ok {
		t.Fatal("noncontiguous derived source order was accepted")
	}
}

func TestSpecificationDoDSourcesRejectsNamespaceMixAndMalformedFormalRefs(t *testing.T) {
	for _, refs := range [][]string{
		{"dod:formal-source:item:000001", "derived-dod:user:000001"},
		{"dod:formal-source:item:000002"},
		{"dod:formal-source:item:000001", "dod:formal-source:item:000001"},
		{"dod:formal-source:item:1"},
		{"dod:formal-source:item:000001:extra"},
		{"dod"},
	} {
		items := make([]workspacemodel.DefinitionCandidateItem, 0, len(refs))
		for _, ref := range refs {
			items = append(items, workspacemodel.DefinitionCandidateItem{CandidateItemRef: ref})
		}
		if _, ok := specificationDoDSources(items); ok {
			t.Fatalf("malformed source identity graph was accepted: %v", refs)
		}
	}
}
