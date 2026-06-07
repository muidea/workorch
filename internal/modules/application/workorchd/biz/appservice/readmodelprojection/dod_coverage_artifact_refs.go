package readmodelprojection

import resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"

func dodCoverageArtifactRefs(item resultmodel.DoDCoverageItem) []string {
	artifactRefs := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	return artifactRefs
}
