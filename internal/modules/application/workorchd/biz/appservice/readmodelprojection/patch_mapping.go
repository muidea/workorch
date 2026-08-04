package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func MapPatches(items []*runtimeevidencemodel.Patch) []projectioncontract.Patch {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.Patch, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, ProjectPatch(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
