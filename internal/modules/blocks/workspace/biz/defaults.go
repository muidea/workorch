package workspace

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

// Startup uses the same owner-private guarded upgrade as explicit resource
// management. Only exact reviewed builtin versions are eligible; custom and
// unknown content is left untouched. No Workspace or Run is read or rewritten.
func (m *Biz) ensureCurrentBuiltinWorkspaceTemplateDefaults(ctx context.Context) error {
	if m == nil || m.templates == nil {
		return common.ErrInvalidArgument
	}
	m.templateMutationMu.Lock()
	err := ensureBuiltinWorkspaceTemplateDefaults(ctx, m.templates)
	m.templateMutationMu.Unlock()
	if err != nil {
		return err
	}
	items, err := m.queryBuiltinTemplateDefaults(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !item.UpgradeAvailable {
			continue
		}
		_, err := m.submitBuiltinTemplateUpgrade(ctx, workspaceTemplateUpgradeEvent{TemplateID: item.TemplateID, ExpectedContentHash: item.CurrentContentHash, TargetContentHash: item.BuiltinContentHash})
		if err != nil && !errors.Is(err, common.ErrConflict) {
			return err
		}
	}
	return nil
}

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
