package content

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *Reader) ReadArtifact(ctx context.Context, artifactURI string) (ArtifactContent, error) {
	if r == nil || r.eventHub == nil {
		return ArtifactContent{}, common.ErrNotFound
	}
	ref, err := ParseArtifactURI(artifactURI)
	if err != nil {
		return ArtifactContent{}, err
	}
	readCtx, err := common.BindRuntimeWorkspaceContext(ctx, ref.WorkspaceID)
	if err != nil {
		return ArtifactContent{}, err
	}
	content, err := ownerreadports.ReadArtifactContentByResultOwnerCommand(readCtx, r.eventHub, ref.WorkspaceID, ref.RunID, ref.URI)
	if err != nil {
		return ArtifactContent{}, err
	}
	return ArtifactContent{
		URI:           content.URI,
		FileName:      content.FileName,
		ContentType:   strings.TrimSpace(content.ContentType),
		Content:       append([]byte(nil), content.Content...),
		ContentSHA256: content.ContentSHA256,
	}, nil
}
