package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ContextTaskDecompositionSection(items []projectioncontract.TaskDecomposition) readmodelcore.ContextSection {
	if len(items) == 0 {
		return readmodelcore.ContextSection{}
	}
	refs := make([]string, 0, len(items))
	state := "completed"
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
		if item.State == "failed" {
			state = "failed"
			continue
		}
		if item.State != "completed" && state != "failed" {
			state = "running"
		}
	}
	return readmodelcore.ContextSection{
		ID:           "task_decomposition",
		Kind:         "task_decomposition",
		Title:        "Task decomposition",
		Priority:     85,
		State:        state,
		Summary:      strconv.Itoa(len(items)) + " decomposition graphs",
		EvidenceRefs: refs,
		Data: map[string]any{
			"taskDecompositions": items,
		},
	}
}
