package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func MapPatches(items []*runtimeevidencemodel.Patch) []readmodelcore.Patch {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.Patch, 0, len(items))
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
