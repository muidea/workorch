package workspace

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

func ensureBuiltinWorkspaceTemplateDefaults(ctx context.Context, repo repository.WorkspaceTemplateRepository) error {
	if repo == nil {
		return common.ErrInvalidArgument
	}
	defaults, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		return err
	}
	for _, item := range defaults {
		if _, err := repo.Get(ctx, item.ID); err == nil {
			continue
		} else if !errors.Is(err, common.ErrNotFound) {
			return err
		}
		value, err := builtinWorkspaceTemplate(item)
		if err != nil {
			return err
		}
		if err := repo.Create(ctx, value); err != nil {
			if !errors.Is(err, common.ErrConflict) {
				return err
			}
			if _, getErr := repo.Get(ctx, item.ID); getErr != nil {
				return getErr
			}
		}
	}
	return nil
}
