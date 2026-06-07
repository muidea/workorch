package contentcore

import resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"

func ArtifactDescriptorFromModel(item *resultmodel.Artifact) *ArtifactDescriptor {
	if item == nil {
		return nil
	}
	return &ArtifactDescriptor{ContentType: item.ContentType}
}
