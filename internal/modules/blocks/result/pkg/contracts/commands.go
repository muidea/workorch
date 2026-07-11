package contracts

import (
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateResultCommand struct {
	Result *resultmodel.Result
}

type GetResultCommand struct {
	ResultID common.ResultID
}

type ListResultsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type ListResultsByRunCommand struct {
	RunID common.RunID
}

type UpdateResultCommand struct {
	Result *resultmodel.Result
}

type DeleteResultCommand struct {
	ResultID common.ResultID
}

type CreateArtifactCommand struct {
	Artifact *resultmodel.Artifact
}

type GetArtifactCommand struct {
	ArtifactID common.ArtifactID
}

type ListArtifactsByWorkspaceCommand struct {
	WorkspaceID common.WorkspaceID
}

type ListArtifactsByRunCommand struct {
	RunID common.RunID
}

type ListArtifactsByResultCommand struct {
	ResultID common.ResultID
}

type UpdateArtifactCommand struct {
	Artifact *resultmodel.Artifact
}

type DeleteArtifactCommand struct {
	ArtifactID common.ArtifactID
}
