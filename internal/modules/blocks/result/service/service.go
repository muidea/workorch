package service

import (
	"context"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type Service interface {
	CreateResult(context.Context, *resultmodel.Result) error
	GetResult(context.Context, common.ResultID) (*resultmodel.Result, error)
	ListResultsByWorkspace(context.Context, common.WorkspaceID) ([]*resultmodel.Result, error)
	ListResultsByRun(context.Context, common.RunID) ([]*resultmodel.Result, error)
	UpdateResult(context.Context, *resultmodel.Result) error
	DeleteResult(context.Context, common.ResultID) error
	CreateArtifact(context.Context, *resultmodel.Artifact) error
	GetArtifact(context.Context, common.ArtifactID) (*resultmodel.Artifact, error)
	ListArtifactsByWorkspace(context.Context, common.WorkspaceID) ([]*resultmodel.Artifact, error)
	ListArtifactsByRun(context.Context, common.RunID) ([]*resultmodel.Artifact, error)
	ListArtifactsByResult(context.Context, common.ResultID) ([]*resultmodel.Artifact, error)
	UpdateArtifact(context.Context, *resultmodel.Artifact) error
	DeleteArtifact(context.Context, common.ArtifactID) error
}

type Dependencies struct {
	CreateResult             func(context.Context, *resultmodel.Result) error
	GetResult                func(context.Context, common.ResultID) (*resultmodel.Result, error)
	ListResultsByWorkspace   func(context.Context, common.WorkspaceID) ([]*resultmodel.Result, error)
	ListResultsByRun         func(context.Context, common.RunID) ([]*resultmodel.Result, error)
	UpdateResult             func(context.Context, *resultmodel.Result) error
	DeleteResult             func(context.Context, common.ResultID) error
	CreateArtifact           func(context.Context, *resultmodel.Artifact) error
	GetArtifact              func(context.Context, common.ArtifactID) (*resultmodel.Artifact, error)
	ListArtifactsByWorkspace func(context.Context, common.WorkspaceID) ([]*resultmodel.Artifact, error)
	ListArtifactsByRun       func(context.Context, common.RunID) ([]*resultmodel.Artifact, error)
	ListArtifactsByResult    func(context.Context, common.ResultID) ([]*resultmodel.Artifact, error)
	UpdateArtifact           func(context.Context, *resultmodel.Artifact) error
	DeleteArtifact           func(context.Context, common.ArtifactID) error
}

type Module struct {
	createResult             func(context.Context, *resultmodel.Result) error
	getResult                func(context.Context, common.ResultID) (*resultmodel.Result, error)
	listResultsByWorkspace   func(context.Context, common.WorkspaceID) ([]*resultmodel.Result, error)
	listResultsByRun         func(context.Context, common.RunID) ([]*resultmodel.Result, error)
	updateResult             func(context.Context, *resultmodel.Result) error
	deleteResult             func(context.Context, common.ResultID) error
	createArtifact           func(context.Context, *resultmodel.Artifact) error
	getArtifact              func(context.Context, common.ArtifactID) (*resultmodel.Artifact, error)
	listArtifactsByWorkspace func(context.Context, common.WorkspaceID) ([]*resultmodel.Artifact, error)
	listArtifactsByRun       func(context.Context, common.RunID) ([]*resultmodel.Artifact, error)
	listArtifactsByResult    func(context.Context, common.ResultID) ([]*resultmodel.Artifact, error)
	updateArtifact           func(context.Context, *resultmodel.Artifact) error
	deleteArtifact           func(context.Context, common.ArtifactID) error
}

func New(deps Dependencies) *Module {
	return &Module{
		createResult:             deps.CreateResult,
		getResult:                deps.GetResult,
		listResultsByWorkspace:   deps.ListResultsByWorkspace,
		listResultsByRun:         deps.ListResultsByRun,
		updateResult:             deps.UpdateResult,
		deleteResult:             deps.DeleteResult,
		createArtifact:           deps.CreateArtifact,
		getArtifact:              deps.GetArtifact,
		listArtifactsByWorkspace: deps.ListArtifactsByWorkspace,
		listArtifactsByRun:       deps.ListArtifactsByRun,
		listArtifactsByResult:    deps.ListArtifactsByResult,
		updateArtifact:           deps.UpdateArtifact,
		deleteArtifact:           deps.DeleteArtifact,
	}
}

func (m *Module) CreateResult(ctx context.Context, item *resultmodel.Result) error {
	if m == nil || m.createResult == nil {
		return common.ErrInvalidArgument
	}
	return m.createResult(ctx, item)
}

func (m *Module) GetResult(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	if m == nil || m.getResult == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.getResult(ctx, id)
}

func (m *Module) ListResultsByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	if m == nil || m.listResultsByWorkspace == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listResultsByWorkspace(ctx, workspaceID)
}

func (m *Module) ListResultsByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	if m == nil || m.listResultsByRun == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listResultsByRun(ctx, runID)
}

func (m *Module) UpdateResult(ctx context.Context, item *resultmodel.Result) error {
	if m == nil || m.updateResult == nil {
		return common.ErrInvalidArgument
	}
	return m.updateResult(ctx, item)
}

func (m *Module) DeleteResult(ctx context.Context, id common.ResultID) error {
	if m == nil || m.deleteResult == nil {
		return common.ErrInvalidArgument
	}
	return m.deleteResult(ctx, id)
}

func (m *Module) CreateArtifact(ctx context.Context, item *resultmodel.Artifact) error {
	if m == nil || m.createArtifact == nil {
		return common.ErrInvalidArgument
	}
	return m.createArtifact(ctx, item)
}

func (m *Module) GetArtifact(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	if m == nil || m.getArtifact == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.getArtifact(ctx, id)
}

func (m *Module) ListArtifactsByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	if m == nil || m.listArtifactsByWorkspace == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listArtifactsByWorkspace(ctx, workspaceID)
}

func (m *Module) ListArtifactsByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	if m == nil || m.listArtifactsByRun == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listArtifactsByRun(ctx, runID)
}

func (m *Module) ListArtifactsByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	if m == nil || m.listArtifactsByResult == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listArtifactsByResult(ctx, resultID)
}

func (m *Module) UpdateArtifact(ctx context.Context, item *resultmodel.Artifact) error {
	if m == nil || m.updateArtifact == nil {
		return common.ErrInvalidArgument
	}
	return m.updateArtifact(ctx, item)
}

func (m *Module) DeleteArtifact(ctx context.Context, id common.ArtifactID) error {
	if m == nil || m.deleteArtifact == nil {
		return common.ErrInvalidArgument
	}
	return m.deleteArtifact(ctx, id)
}
