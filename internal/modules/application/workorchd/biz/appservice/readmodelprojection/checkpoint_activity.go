package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

func MapRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint) []projectioncontract.RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.RuntimeCheckpoint, 0, len(items))
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
