package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func ListRunResultsWithSession(ctx context.Context, session *runtimecore.Session, query workorchquerycontract.RunResourcePageQuery) (ret readmodelcore.ResultPage, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (readmodelcore.ResultPage, error) {
		return readmodelports.ReadRunResultsPage(ctx, hub, query)
	})
}

func ListRunResultsWithBackend(ctx context.Context, backend string, dataRoot string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ResultPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.ResultPage, error) {
		return ListRunResultsWithSession(ctx, session, query)
	})
}

func ListRunPatchesWithSession(ctx context.Context, session *runtimecore.Session, query workorchquerycontract.RunResourcePageQuery) (ret readmodelcore.PatchPage, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (readmodelcore.PatchPage, error) {
		return readmodelports.ReadRunPatchesPage(ctx, hub, query)
	})
}

func ListRunPatchesWithBackend(ctx context.Context, backend string, dataRoot string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.PatchPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.PatchPage, error) {
		return ListRunPatchesWithSession(ctx, session, query)
	})
}

func ListRunValidationResultsWithSession(ctx context.Context, session *runtimecore.Session, query workorchquerycontract.RunResourcePageQuery) (ret readmodelcore.ValidationResultPage, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (readmodelcore.ValidationResultPage, error) {
		return readmodelports.ReadRunValidationResultsPage(ctx, hub, query)
	})
}

func ListRunValidationResultsWithBackend(ctx context.Context, backend string, dataRoot string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ValidationResultPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.ValidationResultPage, error) {
		return ListRunValidationResultsWithSession(ctx, session, query)
	})
}

func ListRunArtifactsWithSession(ctx context.Context, session *runtimecore.Session, query workorchquerycontract.RunResourcePageQuery) (ret readmodelcore.ArtifactPage, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (readmodelcore.ArtifactPage, error) {
		return readmodelports.ReadRunArtifactsPage(ctx, hub, query)
	})
}

func ListRunArtifactsWithBackend(ctx context.Context, backend string, dataRoot string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ArtifactPage, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.ArtifactPage, error) {
		return ListRunArtifactsWithSession(ctx, session, query)
	})
}
