package repository

import (
	"context"

	result "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ArtifactRepository interface {
	Create(context.Context, *result.Artifact) error
	Get(context.Context, common.ArtifactID) (*result.Artifact, error)
	ListByWorkspace(context.Context, common.WorkspaceID) ([]*result.Artifact, error)
	ListByRun(context.Context, common.RunID) ([]*result.Artifact, error)
	ListByResult(context.Context, common.ResultID) ([]*result.Artifact, error)
	Update(context.Context, *result.Artifact) error
	Delete(context.Context, common.ArtifactID) error
}
