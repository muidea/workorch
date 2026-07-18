package result

import (
	"context"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	results   repository.ResultRepository
	artifacts repository.ArtifactRepository
}

func NewService(results repository.ResultRepository, artifacts repository.ArtifactRepository) *Service {
	return &Service{results: results, artifacts: artifacts}
}

func (s *Service) CreateResult(ctx context.Context, item *resultmodel.Result) error {
	if err := validateResult(item); err != nil {
		return err
	}
	return s.results.Create(ctx, item)
}

func (s *Service) GetResult(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.results.Get(ctx, id)
}

func (s *Service) ListResultsByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.results.ListByWorkspace(ctx, workspaceID)
}

func (s *Service) ListResultsByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.results.ListByRun(ctx, runID)
}

func (s *Service) UpdateResult(ctx context.Context, item *resultmodel.Result) error {
	if err := validateResult(item); err != nil {
		return err
	}
	return s.results.Update(ctx, item)
}

func (s *Service) DeleteResult(ctx context.Context, id common.ResultID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	return s.results.Delete(ctx, id)
}

func (s *Service) CreateArtifact(ctx context.Context, item *resultmodel.Artifact) error {
	if err := validateArtifact(item); err != nil {
		return err
	}
	return s.artifacts.Create(ctx, item)
}

func (s *Service) GetArtifact(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.artifacts.Get(ctx, id)
}

func (s *Service) ListArtifactsByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.artifacts.ListByWorkspace(ctx, workspaceID)
}

func (s *Service) ListArtifactsByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.artifacts.ListByRun(ctx, runID)
}

func (s *Service) ListArtifactsByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	if resultID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.artifacts.ListByResult(ctx, resultID)
}

func (s *Service) UpdateArtifact(ctx context.Context, item *resultmodel.Artifact) error {
	if err := validateArtifact(item); err != nil {
		return err
	}
	return s.artifacts.Update(ctx, item)
}

func (s *Service) DeleteArtifact(ctx context.Context, id common.ArtifactID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	return s.artifacts.Delete(ctx, id)
}

func validateResult(item *resultmodel.Result) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" || !isSupportedResultState(item.State) {
		return common.ErrInvalidArgument
	}
	if item.RuntimeCloseout != nil {
		if err := item.RuntimeCloseout.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func isSupportedResultState(state common.ResultState) bool {
	switch state {
	case common.ResultStateDraft, common.ResultStateAccepted, common.ResultStateRejected, common.ResultStateSuperseded:
		return true
	default:
		return false
	}
}

func validateArtifact(item *resultmodel.Artifact) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" || !isSupportedArtifactState(item.State) {
		return common.ErrInvalidArgument
	}
	return nil
}

func isSupportedArtifactState(state common.ArtifactState) bool {
	switch state {
	case common.ArtifactStateCreated, common.ArtifactStateAvailable, common.ArtifactStateFailed:
		return true
	default:
		return false
	}
}
