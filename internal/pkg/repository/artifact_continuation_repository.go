package repository

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type ArtifactContinuationRepository interface {
	Create(context.Context, *orchestratormodel.ArtifactContinuation) error
	Get(context.Context, common.ArtifactContinuationID) (*orchestratormodel.ArtifactContinuation, error)
	ListByRun(context.Context, common.RunID) ([]*orchestratormodel.ArtifactContinuation, error)
	ListOpenByRun(context.Context, common.RunID) ([]*orchestratormodel.ArtifactContinuation, error)
	Update(context.Context, *orchestratormodel.ArtifactContinuation) error
	Delete(context.Context, common.ArtifactContinuationID) error
	SavePlan(context.Context, common.ArtifactContinuationID, map[string]any) error
	GetPlan(context.Context, common.ArtifactContinuationID) (map[string]any, error)
	SaveChunk(context.Context, *orchestratormodel.ArtifactContinuationChunk, string) error
	GetChunk(context.Context, common.ArtifactContinuationID, int) (*orchestratormodel.ArtifactContinuationChunk, string, error)
	ListChunks(context.Context, common.ArtifactContinuationID) ([]*orchestratormodel.ArtifactContinuationChunk, error)
	SaveAssembled(context.Context, *orchestratormodel.ArtifactContinuationAssembledMetadata, string) error
	GetAssembled(context.Context, common.ArtifactContinuationID) (*orchestratormodel.ArtifactContinuationAssembledMetadata, string, error)
}
