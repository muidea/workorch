package readmodelprojection

import resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"

func resultArtifactRefs(item *resultmodel.Result) []string {
	if item == nil {
		return nil
	}
	artifactRefs := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	return artifactRefs
}
