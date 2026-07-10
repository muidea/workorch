package readmodelports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type ResultReader struct {
	Hub magicevent.Hub
}

func (r ResultReader) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	return ownerreadports.ListResultsByWorkspaceOwnerCommand(ctx, r.Hub, workspaceID)
}

func (r ResultReader) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	return ownerreadports.ListResultsByRunOwnerCommand(ctx, r.Hub, runID)
}

type ArtifactReader struct {
	Hub magicevent.Hub
}

func (r ArtifactReader) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	return ownerreadports.ListArtifactsByWorkspaceOwnerCommand(ctx, r.Hub, workspaceID)
}

func (r ArtifactReader) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	return ownerreadports.ListArtifactsByRunOwnerCommand(ctx, r.Hub, runID)
}

func (r ArtifactReader) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	return ownerreadports.ListArtifactsByResultOwnerCommand(ctx, r.Hub, resultID)
}

type ArtifactContinuationReader struct {
	Hub magicevent.Hub
}

func (r ArtifactContinuationReader) ListByRun(ctx context.Context, runID common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	return ownerreadports.ListArtifactContinuationsByRunOwnerCommand(ctx, r.Hub, runID)
}

type CoverageWorkItemReader struct {
	Hub magicevent.Hub
}

func (r CoverageWorkItemReader) ListByRun(ctx context.Context, runID common.RunID) ([]*orchestratormodel.CoverageWorkItem, error) {
	return ownerreadports.ListCoverageWorkItemsByRunOwnerCommand(ctx, r.Hub, runID)
}
