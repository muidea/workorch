package contentcore

import (
	"strings"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func FindArtifactByURI(items []*resultmodel.Artifact, uri string) *resultmodel.Artifact {
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.URI) == uri {
			return item
		}
	}
	return nil
}

func FindArtifactViewByURI(items []ArtifactView, ref ArtifactRef) *ArtifactDescriptor {
	if len(items) == 0 {
		return nil
	}
	for _, item := range items {
		if strings.TrimSpace(item.URI) == ref.URI &&
			common.WorkspaceID(item.WorkspaceID) == ref.WorkspaceID &&
			common.RunID(item.RunID) == ref.RunID {
			return &ArtifactDescriptor{ContentType: item.ContentType}
		}
	}
	return nil
}
