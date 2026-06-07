package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentports"
	"github.com/muidea/workorch/internal/pkg/events"
)

type ArtifactContent = contentcore.ArtifactContent

func ReadArtifactContentWithSession(ctx context.Context, session *RuntimeSession, artifactURI string) (ret ArtifactContent, err error) {
	return readArtifactContentWithSession(ctx, session, artifactURI, nil)
}

func ReadArtifactContentWithRunSnapshotWithSession(ctx context.Context, session *RuntimeSession, artifactURI string, snapshot RunSnapshot) (ret ArtifactContent, err error) {
	return readArtifactContentWithSession(ctx, session, artifactURI, snapshot.Artifacts)
}

func readArtifactContentWithSession(ctx context.Context, session *RuntimeSession, artifactURI string, cachedArtifacts []Artifact) (ret ArtifactContent, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (ArtifactContent, error) {
		reader := contentports.NewReader(hub, session.RepositoryBackend(), session.RepositoryDataDir())
		return reader.ReadArtifact(ctx, artifactURI, contentcore.ArtifactViews(cachedArtifacts))
	})
}

func ReadArtifactContentWithBackend(ctx context.Context, backend string, dataRoot string, artifactURI string) (ArtifactContent, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (ArtifactContent, error) {
		return ReadArtifactContentWithSession(ctx, session, artifactURI)
	})
}
