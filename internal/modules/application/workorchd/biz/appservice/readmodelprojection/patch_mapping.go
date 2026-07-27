package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

func MapPatches(items []*runmodel.Patch) []readmodelcore.Patch {
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
