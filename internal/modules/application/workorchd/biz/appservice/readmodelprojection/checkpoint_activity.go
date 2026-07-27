package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

func MapRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint) []readmodelcore.RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.RuntimeCheckpoint, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, projectRuntimeCheckpointWithData(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
