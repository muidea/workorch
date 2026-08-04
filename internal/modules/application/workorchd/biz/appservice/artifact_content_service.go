package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ReadArtifactContentWithSession(ctx context.Context, session *runtimecore.Session, artifactURI string) (ret contentcore.ArtifactContent, err error) {
	return readArtifactContentWithSession(ctx, session, artifactURI, nil)
}

func ReadArtifactContentWithRunSnapshotWithSession(ctx context.Context, session *runtimecore.Session, artifactURI string, snapshot readmodelcore.RunSnapshot) (ret contentcore.ArtifactContent, err error) {
	return readArtifactContentWithSession(ctx, session, artifactURI, snapshot.Artifacts)
}

func readArtifactContentWithSession(ctx context.Context, session *runtimecore.Session, artifactURI string, cachedArtifacts []workorchprojectioncontract.Artifact) (ret contentcore.ArtifactContent, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (contentcore.ArtifactContent, error) {
		reader := contentports.NewReader(hub, session.RepositoryBackend(), session.RepositoryDataDir())
		return reader.ReadArtifact(ctx, artifactURI, contentcore.ArtifactViews(cachedArtifacts))
	})
}

func ReadArtifactContentWithBackend(ctx context.Context, backend string, dataRoot string, artifactURI string) (contentcore.ArtifactContent, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (contentcore.ArtifactContent, error) {
		return ReadArtifactContentWithSession(ctx, session, artifactURI)
	})
}
