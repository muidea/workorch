package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func MapArtifactContinuations(items []*orchestratormodel.ArtifactContinuation) []projectioncontract.ArtifactContinuation {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.ArtifactContinuation, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, ProjectArtifactContinuation(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
