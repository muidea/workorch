package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

func dodCoverageArtifactRefs(item workorchresultcontract.DoDCoverageItem) []common.ArtifactID {
	artifactRefs := make([]common.ArtifactID, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, ref)
		}
	}
	return artifactRefs
}
