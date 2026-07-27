package workspace

import (
	"context"
	"errors"

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
		value := &repository.WorkspaceTemplate{ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText, DoDItems: append([]string(nil), item.DoDItems...), DefaultSoulID: item.DefaultSoulID, SoulBindings: cloneStringMap(item.SoulBindings), ContextHints: item.ContextHints.Clone(), EvidencePolicies: cloneDefaultPayloads(item.EvidencePolicies)}
		if err := repo.Update(ctx, value); err != nil {
			if !errors.Is(err, common.ErrNotFound) {
				return err
			}
			if err := repo.Create(ctx, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func cloneDefaultPayloads(values []workorcheventpayload.Payload) []workorcheventpayload.Payload {
	if values == nil {
		return nil
	}
	cloned := make([]workorcheventpayload.Payload, 0, len(values))
	for _, value := range values {
		cloned = append(cloned, value.Clone())
	}
	return cloned
}
