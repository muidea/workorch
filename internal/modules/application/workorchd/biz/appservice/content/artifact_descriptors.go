package content

import resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"

func ArtifactDescriptorFromModel(item *resultmodel.Artifact) *ArtifactDescriptor {
	if item == nil {
		return nil
	}
	return &ArtifactDescriptor{ContentType: item.ContentType}
}
