package contentports

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *Reader) ReadArtifact(ctx context.Context, artifactURI string, cachedArtifacts []contentcore.ArtifactView) (contentcore.ArtifactContent, error) {
	if r == nil || r.eventHub == nil || !strings.EqualFold(r.repositoryBackend, "file") || r.repositoryDataDir == "" {
		return contentcore.ArtifactContent{}, common.ErrNotFound
	}
	ref, err := contentcore.ParseArtifactURI(artifactURI)
	if err != nil {
		return contentcore.ArtifactContent{}, err
	}
	artifact := contentcore.FindArtifactViewByURI(cachedArtifacts, ref)
	if artifact == nil {
		artifacts, err := ownerreadports.ListArtifactsByRunOwnerCommand(ctx, r.eventHub, ref.RunID)
		if err != nil {
			return contentcore.ArtifactContent{}, err
		}
		artifact = contentcore.ArtifactDescriptorFromModel(contentcore.FindArtifactByURI(artifacts, ref.URI))
	}
	if artifact == nil {
		return contentcore.ArtifactContent{}, common.ErrNotFound
	}
	root := filepath.Join(r.repositoryDataDir, "internal", "workspaces", string(ref.WorkspaceID), "artifacts", string(ref.RunID))
	cleanPath, err := contentcore.SafeArtifactPath(root, ref.RelativePath)
	if err != nil {
		return contentcore.ArtifactContent{}, err
	}
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return contentcore.ArtifactContent{}, common.ErrNotFound
		}
		return contentcore.ArtifactContent{}, err
	}
	return contentcore.ArtifactContent{
		URI:         ref.URI,
		FileName:    filepath.Base(cleanPath),
		ContentType: contentcore.FirstNonEmpty(artifact.ContentType, "application/octet-stream"),
		Content:     content,
	}, nil
}
