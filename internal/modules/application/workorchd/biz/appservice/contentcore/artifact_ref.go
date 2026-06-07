package contentcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func ParseArtifactURI(artifactURI string) (ArtifactRef, error) {
	artifactURI = strings.TrimSpace(artifactURI)
	const prefix = "workorch://workspaces/"
	if artifactURI == "" || !strings.HasPrefix(artifactURI, prefix) {
		return ArtifactRef{}, common.ErrInvalidArgument
	}
	rest := strings.TrimPrefix(artifactURI, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) < 4 || parts[0] == "" || parts[1] != "artifacts" || parts[2] == "" {
		return ArtifactRef{}, common.ErrInvalidArgument
	}
	relativePath := strings.Join(parts[3:], "/")
	if strings.TrimSpace(relativePath) == "" {
		return ArtifactRef{}, common.ErrInvalidArgument
	}
	for _, part := range parts[3:] {
		if part == "" || part == "." || part == ".." {
			return ArtifactRef{}, common.ErrInvalidArgument
		}
	}
	return ArtifactRef{
		URI:          artifactURI,
		WorkspaceID:  common.WorkspaceID(parts[0]),
		RunID:        common.RunID(parts[2]),
		RelativePath: relativePath,
	}, nil
}
