package readmodelprojection

import (
	"strconv"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ContextExecutionSectionsSection(items []projectioncontract.ExecutionSection) projectioncontract.ContextSection {
	if len(items) == 0 {
		return projectioncontract.ContextSection{}
	}
	state, refs := contextExecutionSectionStateAndEvidenceRefs(items)
	return projectioncontract.ContextSection{
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
