package result

import (
	"context"
	"errors"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type service struct {
	results   repository.ResultRepository
	artifacts repository.ArtifactRepository
}

func newService(results repository.ResultRepository, artifacts repository.ArtifactRepository) *service {
	return &service{results: results, artifacts: artifacts}
}

func (s *service) CreateResult(ctx context.Context, item *resultmodel.Result) error {
	if err := validateResult(item); err != nil {
		return err
	}
	return s.results.Create(ctx, item)
}

func (s *service) GetResult(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.results.Get(ctx, id)
}

func (s *service) ListResultsByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.results.ListByWorkspace(ctx, workspaceID)
}

func (s *service) ListResultsByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.results.ListByRun(ctx, runID)
}

func (s *service) UpdateResult(ctx context.Context, item *resultmodel.Result) error {
	if err := validateResult(item); err != nil {
		return err
	}
	current, err := s.results.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	if current.WorkspaceID != item.WorkspaceID || current.RunID != item.RunID || !validResultTransition(current.State, item.State) {
		return common.ErrConflict
	}
	if (current.TerminalDelivery != nil || item.TerminalDelivery != nil) && !sameTerminalResult(current, item) {
		return common.ErrConflict
	}
	return s.results.CompareAndSwap(ctx, item, item.Revision)
}

func (s *service) DeleteResult(ctx context.Context, workspaceID common.WorkspaceID, id common.ResultID) error {
	if workspaceID == "" || id == "" {
		return common.ErrInvalidArgument
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return err
	}
	current, err := s.results.Get(ctx, id)
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return err
	}
	if current != nil && current.WorkspaceID != workspaceID {
		return common.ErrConflict
	}
	artifacts, err := s.artifacts.ListByResult(ctx, id)
	if err != nil {
		return err
	}
	if current == nil && len(artifacts) == 0 {
		return common.ErrNotFound
	}
	// Delete the aggregate root first, then its formal children. If a child
	// delete fails, a retry sees the missing root and resumes orphan cleanup.
	// This prevents a successful command from leaving a Result that references
	// child metadata which no longer exists.
	if current != nil {
		if err := s.results.Delete(ctx, id); err != nil {
			return err
		}
	}
	for _, artifact := range artifacts {
		if artifact == nil || artifact.WorkspaceID != workspaceID || artifact.ResultID != id {
			return common.ErrConflict
		}
		if err := s.artifacts.Delete(ctx, artifact.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (s *service) CreateArtifact(ctx context.Context, item *resultmodel.Artifact) error {
	if err := validateArtifact(item); err != nil {
		return err
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, item.WorkspaceID)
	if err != nil {
		return err
	}
	parent, err := s.results.Get(ctx, item.ResultID)
	if err != nil {
		return err
	}
	if parent.WorkspaceID != item.WorkspaceID || parent.RunID != item.RunID {
		return common.ErrConflict
	}
	return s.artifacts.Create(ctx, item)
}

// createTerminalArtifact permits the terminal-delivery saga to persist child
// metadata while its parent Result is still draft/pending. Public owner writes
// never bypass the parent-scope check in CreateArtifact.
func (s *service) createTerminalArtifact(ctx context.Context, item *resultmodel.Artifact) error {
	if err := validateArtifact(item); err != nil {
		return err
	}
	return s.artifacts.Create(ctx, item)
}

func (s *service) GetArtifact(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.artifacts.Get(ctx, id)
}

func (s *service) ListArtifactsByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.artifacts.ListByWorkspace(ctx, workspaceID)
}

func (s *service) ListArtifactsByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.artifacts.ListByRun(ctx, runID)
}

func (s *service) ListArtifactsByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	if resultID == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.artifacts.ListByResult(ctx, resultID)
}

func (s *service) UpdateArtifact(ctx context.Context, item *resultmodel.Artifact) error {
	if err := validateArtifact(item); err != nil {
		return err
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, item.WorkspaceID)
	if err != nil {
		return err
	}
	current, err := s.artifacts.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	if current.WorkspaceID != item.WorkspaceID || current.RunID != item.RunID || current.ResultID != item.ResultID {
		return common.ErrConflict
	}
	if (current.TerminalDeliveryID != "" || item.TerminalDeliveryID != "" || current.MaterialHash != "" || item.MaterialHash != "") && !sameTerminalArtifact(current, item) {
		return common.ErrConflict
	}
	return s.artifacts.Update(ctx, item)
}

func (s *service) DeleteArtifact(ctx context.Context, workspaceID common.WorkspaceID, id common.ArtifactID) error {
	if workspaceID == "" || id == "" {
		return common.ErrInvalidArgument
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return err
	}
	current, err := s.artifacts.Get(ctx, id)
	if err != nil {
		return err
	}
	if current.WorkspaceID != workspaceID {
		return common.ErrConflict
	}
	if _, err := s.results.Get(ctx, current.ResultID); err == nil {
		return common.ErrConflict
	} else if !errors.Is(err, common.ErrNotFound) {
		return err
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

func validResultTransition(from, to common.ResultState) bool {
	if from == to {
		return true
	}
	switch from {
	case common.ResultStateDraft:
		return to == common.ResultStateAccepted || to == common.ResultStateRejected
	case common.ResultStateAccepted, common.ResultStateRejected:
		return to == common.ResultStateSuperseded
	default:
		return false
	}
}

func validateArtifact(item *resultmodel.Artifact) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" || item.ResultID == "" || !isSupportedArtifactState(item.State) {
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
