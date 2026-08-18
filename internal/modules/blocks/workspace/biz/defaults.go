package workspace

import (
	"context"
	"errors"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
		contextHints, err := workorcheventpayload.FromMap(item.ContextHints)
		if err != nil {
			return err
		}
		evidencePolicies, err := workorcheventpayload.FromMapSlice(item.EvidencePolicies)
		if err != nil {
			return err
		}
		value := &workspacemodel.WorkspaceTemplate{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: item.DefaultSoulID, SoulBindings: cloneStringMap(item.SoulBindings), ContextHints: contextHints, EvidencePolicies: evidencePolicies}
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
