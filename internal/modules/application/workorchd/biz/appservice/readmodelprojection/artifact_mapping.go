package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func MapArtifacts(items []*resultmodel.Artifact) []projectioncontract.Artifact {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.Artifact, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, artifactFromModel(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
