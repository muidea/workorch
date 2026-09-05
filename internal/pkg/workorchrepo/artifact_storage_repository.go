package workorchrepo

import (
	"context"
	"path/filepath"
	"strings"

	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type artifactStorageRepository struct {
	base     repository.ArtifactRepository
	dataRoot string
}

func NewArtifactStorageRepository(base repository.ArtifactRepository, dataRoot string) repository.ArtifactRepository {
	if base == nil || strings.TrimSpace(dataRoot) == "" {
		return base
	}
	return &artifactStorageRepository{base: base, dataRoot: dataRoot}
}

func (r *artifactStorageRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	bound, _ := r.base.(repository.OwnerRepositoryBound)
	if bound == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return bound.OwnerRepositoryBinding()
}

func (r *artifactStorageRepository) Create(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil || item.WorkspaceID == "" {
		return r.base.Create(ctx, item)
	}
	return withWorkspaceDeletionMutation(ctx, r.dataRoot, "result", item.WorkspaceID, func(guarded context.Context) error {
		if err := r.prepareStoredArtifact(item); err != nil {
			return err
		}
		return r.base.Create(guarded, item)
	})
}

func (r *artifactStorageRepository) Get(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	return r.base.Get(ctx, id)
}

func (r *artifactStorageRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	return r.base.ListByWorkspace(ctx, workspaceID)
}

func (r *artifactStorageRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	return r.base.ListByRun(ctx, runID)
}

func (r *artifactStorageRepository) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	return r.base.ListByResult(ctx, resultID)
}

func (r *artifactStorageRepository) Update(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil || item.WorkspaceID == "" {
		return r.base.Update(ctx, item)
	}
	return withWorkspaceDeletionMutation(ctx, r.dataRoot, "result", item.WorkspaceID, func(guarded context.Context) error {
		if err := r.prepareStoredArtifact(item); err != nil {
			return err
		}
		return r.base.Update(guarded, item)
	})
}

func (r *artifactStorageRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	return r.base.Delete(ctx, id)
}

func (r *artifactStorageRepository) prepareStoredArtifact(item *resultmodel.Artifact) error {
	if r == nil || item == nil || item.State != common.ArtifactStateAvailable {
		return nil
	}
	if item.WorkspaceID == "" || item.RunID == "" || strings.TrimSpace(item.Summary) == "" {
		return nil
	}
	if item.ArtifactType != resultcommon.ArtifactTypeText && item.ArtifactType != resultcommon.ArtifactTypeReport {
		return nil
	}
	fileName := artifactFileName(item)
	dir := filepath.Join(r.dataRoot, "internal", "workspaces", string(item.WorkspaceID), "artifacts", string(item.RunID))
	if err := publishRepositoryFile(filepath.Join(dir, fileName), []byte(strings.TrimSpace(item.Summary)+"\n"), 0o644); err != nil {
		return err
	}
	item.URI = "workorch://workspaces/" + string(item.WorkspaceID) + "/artifacts/" + string(item.RunID) + "/" + fileName
	if strings.TrimSpace(item.ContentType) == "" {
		item.ContentType = "text/plain; charset=utf-8"
	}
	return nil
}
func artifactFileName(item *resultmodel.Artifact) string {
	if item == nil {
		return "artifact.txt"
	}
	if strings.Contains(string(item.ID), "result-summary") {
		return "result-summary.txt"
	}
	name := sanitizeArtifactFileName(item.Name)
	if name == "" {
		name = sanitizeArtifactFileName(string(item.ID))
	}
	if name == "" {
		name = "artifact"
	}
	if filepath.Ext(name) == "" {
		name += ".txt"
	}
	return name
}

func sanitizeArtifactFileName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, item := range value {
		if (item >= 'a' && item <= 'z') || (item >= '0' && item <= '9') || item == '.' || item == '_' {
			builder.WriteRune(item)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-._")
}
