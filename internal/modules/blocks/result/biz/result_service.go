package result

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"

	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
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
	item, err := s.results.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateResult(item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *service) ListResultsByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	if workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	items, err := s.results.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return validateResultList(items)
}

func (s *service) ListResultsByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	if runID == "" {
		return nil, common.ErrInvalidArgument
	}
	items, err := s.results.ListByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	return validateResultList(items)
}

func validateResultList(items []*resultmodel.Result) ([]*resultmodel.Result, error) {
	for _, item := range items {
		if err := validateResult(item); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *service) UpdateResult(ctx context.Context, item *resultmodel.Result) error {
	if err := validateResult(item); err != nil {
		return err
	}
	current, err := s.GetResult(ctx, item.ID)
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

func (s *service) CreateArtifact(ctx context.Context, item *resultmodel.Artifact) error {
	if err := validateArtifact(item); err != nil {
		return err
	}
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, item.WorkspaceID)
	if err != nil {
		return err
	}
	parent, err := s.GetResult(ctx, item.ResultID)
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

func (s *service) ReadArtifactContent(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, uri string) (resultevents.ReadArtifactContentResult, error) {
	if s == nil || workspaceID == "" || runID == "" || strings.TrimSpace(uri) == "" {
		return resultevents.ReadArtifactContentResult{}, common.ErrInvalidArgument
	}
	items, err := s.artifacts.ListByRun(ctx, runID)
	if err != nil {
		return resultevents.ReadArtifactContentResult{}, err
	}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID || item.RunID != runID || strings.TrimSpace(item.URI) != strings.TrimSpace(uri) {
			continue
		}
		if item.State != common.ArtifactStateAvailable ||
			(item.ArtifactType != resultcommon.ArtifactTypeText && item.ArtifactType != resultcommon.ArtifactTypeReport) ||
			strings.TrimSpace(item.Summary) == "" {
			return resultevents.ReadArtifactContentResult{}, common.ErrNotFound
		}
		content := []byte(strings.TrimSpace(item.Summary) + "\n")
		if len(content) > resultevents.MaxArtifactContentBytes {
			return resultevents.ReadArtifactContentResult{}, fmt.Errorf("%w: artifact content exceeds %d bytes", common.ErrConflict, resultevents.MaxArtifactContentBytes)
		}
		contentType := strings.TrimSpace(item.ContentType)
		if contentType == "" {
			contentType = "text/plain; charset=utf-8"
		}
		ret := resultevents.ReadArtifactContentResult{
			ArtifactID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID,
			URI: strings.TrimSpace(item.URI), FileName: path.Base(strings.TrimSpace(item.URI)),
			ContentType: contentType, Content: append([]byte(nil), content...),
		}
		hash := sha256.Sum256(ret.Content)
		ret.ContentSHA256 = hex.EncodeToString(hash[:])
		if err := ret.Validate(); err != nil {
			return resultevents.ReadArtifactContentResult{}, err
		}
		return ret.Clone(), nil
	}
	return resultevents.ReadArtifactContentResult{}, common.ErrNotFound
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

func validateResult(item *resultmodel.Result) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" || !isSupportedResultState(item.State) {
		return common.ErrInvalidArgument
	}
	if item.RuntimeCloseout != nil {
		if err := item.RuntimeCloseout.ValidateRaw(); err != nil {
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
