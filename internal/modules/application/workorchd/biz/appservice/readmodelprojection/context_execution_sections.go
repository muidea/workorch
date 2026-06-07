package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func ContextExecutionSectionsSection(items []readmodelcore.ExecutionSection) readmodelcore.ContextSection {
	if len(items) == 0 {
		return readmodelcore.ContextSection{}
	}
	state, refs := contextExecutionSectionStateAndEvidenceRefs(items)
	return readmodelcore.ContextSection{
		ID:           "execution_sections",
		Kind:         "execution_sections",
		Title:        "Execution sections",
		Priority:     84,
		State:        state,
		Summary:      strconv.Itoa(len(items)) + " execution sections",
		EvidenceRefs: compactUniqueStrings(refs),
		Data: map[string]any{
			"executionSections": items,
		},
	}
}
