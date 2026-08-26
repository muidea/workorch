package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/content"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
)

func ReadArtifactContentWithSession(ctx context.Context, session *runtimecore.Session, artifactURI string) (ret content.ArtifactContent, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (content.ArtifactContent, error) {
		reader := content.NewReader(hub)
		return reader.ReadArtifact(ctx, artifactURI)
	})
}

func ReadArtifactContentWithBackend(ctx context.Context, backend string, dataRoot string, artifactURI string) (content.ArtifactContent, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (content.ArtifactContent, error) {
		return ReadArtifactContentWithSession(ctx, session, artifactURI)
	})
}
