package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ListRunResultsWithSession(ctx context.Context, session *RuntimeSession, query ResultQuery) (ret ResultPage, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (ResultPage, error) {
		return readmodelports.ReadRunResultsPage(ctx, hub, query)
	})
}

func ListRunResultsWithBackend(ctx context.Context, backend string, dataRoot string, query ResultQuery) (ResultPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (ResultPage, error) {
		return ListRunResultsWithSession(ctx, session, query)
	})
}

func ListRunPatchesWithSession(ctx context.Context, session *RuntimeSession, query PatchQuery) (ret PatchPage, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (PatchPage, error) {
		return readmodelports.ReadRunPatchesPage(ctx, hub, query)
	})
}

func ListRunPatchesWithBackend(ctx context.Context, backend string, dataRoot string, query PatchQuery) (PatchPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (PatchPage, error) {
		return ListRunPatchesWithSession(ctx, session, query)
	})
}

func ListRunValidationResultsWithSession(ctx context.Context, session *RuntimeSession, query ValidationResultQuery) (ret ValidationResultPage, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (ValidationResultPage, error) {
		return readmodelports.ReadRunValidationResultsPage(ctx, hub, query)
	})
}

func ListRunValidationResultsWithBackend(ctx context.Context, backend string, dataRoot string, query ValidationResultQuery) (ValidationResultPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (ValidationResultPage, error) {
		return ListRunValidationResultsWithSession(ctx, session, query)
	})
}

func ListRunArtifactsWithSession(ctx context.Context, session *RuntimeSession, query ArtifactQuery) (ret ArtifactPage, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (ArtifactPage, error) {
		return readmodelports.ReadRunArtifactsPage(ctx, hub, query)
	})
}

func ListRunArtifactsWithBackend(ctx context.Context, backend string, dataRoot string, query ArtifactQuery) (ArtifactPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (ArtifactPage, error) {
		return ListRunArtifactsWithSession(ctx, session, query)
	})
}
