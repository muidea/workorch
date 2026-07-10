package readmodelports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
)

func ReadRunResultsPage(ctx context.Context, hub magicevent.Hub, query readmodelcore.ResultQuery) (readmodelcore.ResultPage, error) {
	ctx, runID, err := runScopedContext(ctx, hub, query.WorkspaceID, query.RunID)
	if err != nil {
		return readmodelcore.ResultPage{}, err
	}
	items, err := ownerreadports.ListResultsByRunOwnerCommand(ctx, hub, runID)
	if err != nil {
		return readmodelcore.ResultPage{}, err
	}
	return readmodelprojection.PageRunResults(items, query), nil
}

func ReadRunPatchesPage(ctx context.Context, hub magicevent.Hub, query readmodelcore.PatchQuery) (readmodelcore.PatchPage, error) {
	ctx, runID, err := runScopedContext(ctx, hub, query.WorkspaceID, query.RunID)
	if err != nil {
		return readmodelcore.PatchPage{}, err
	}
	items, err := ownerreadports.ListPatchesByRunOwnerCommand(ctx, hub, runID)
	if err != nil {
		return readmodelcore.PatchPage{}, err
	}
	return readmodelprojection.PageRunPatches(items, query), nil
}

func ReadRunValidationResultsPage(ctx context.Context, hub magicevent.Hub, query readmodelcore.ValidationResultQuery) (readmodelcore.ValidationResultPage, error) {
	ctx, runID, err := runScopedContext(ctx, hub, query.WorkspaceID, query.RunID)
	if err != nil {
		return readmodelcore.ValidationResultPage{}, err
	}
	items, err := ownerreadports.ListValidationResultsByRunOwnerCommand(ctx, hub, runID)
	if err != nil {
		return readmodelcore.ValidationResultPage{}, err
	}
	return readmodelprojection.PageRunValidationResults(items, query), nil
}

func ReadRunArtifactsPage(ctx context.Context, hub magicevent.Hub, query readmodelcore.ArtifactQuery) (readmodelcore.ArtifactPage, error) {
	ctx, runID, err := runScopedContext(ctx, hub, query.WorkspaceID, query.RunID)
	if err != nil {
		return readmodelcore.ArtifactPage{}, err
	}
	items, err := ownerreadports.ListArtifactsByRunOwnerCommand(ctx, hub, runID)
	if err != nil {
		return readmodelcore.ArtifactPage{}, err
	}
	return readmodelprojection.PageRunArtifacts(items, query), nil
}
