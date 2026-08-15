package content

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *Reader) ReadArtifact(ctx context.Context, artifactURI string, cachedArtifacts []ArtifactView) (ArtifactContent, error) {
	if r == nil || r.eventHub == nil || !strings.EqualFold(r.repositoryBackend, "file") || r.repositoryDataDir == "" {
		return ArtifactContent{}, common.ErrNotFound
	}
	ref, err := ParseArtifactURI(artifactURI)
	if err != nil {
		return ArtifactContent{}, err
	}
	artifact := FindArtifactViewByURI(cachedArtifacts, ref)
	if artifact == nil {
		readCtx := common.ContextWithRuntimeWorkspaceID(ctx, ref.WorkspaceID)
		artifacts, err := ownerreadports.ListArtifactsByRunOwnerCommand(readCtx, r.eventHub, ref.RunID)
		if err != nil {
			return ArtifactContent{}, err
		}
		artifact = ArtifactDescriptorFromModel(FindArtifactByURI(artifacts, ref.URI))
	}
	if artifact == nil {
		return ArtifactContent{}, common.ErrNotFound
	}
	root := filepath.Join(r.repositoryDataDir, "internal", "workspaces", string(ref.WorkspaceID), "artifacts", string(ref.RunID))
	cleanPath, err := SafeArtifactPath(root, ref.RelativePath)
	if err != nil {
		return ArtifactContent{}, err
	}
	body, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ArtifactContent{}, common.ErrNotFound
		}
		return ArtifactContent{}, err
	}
	return ArtifactContent{
		URI:         ref.URI,
		FileName:    filepath.Base(cleanPath),
		ContentType: FirstNonEmpty(artifact.ContentType, "application/octet-stream"),
		Content:     body,
	}, nil
}
