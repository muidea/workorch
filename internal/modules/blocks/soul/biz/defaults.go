package soul

import (
	"context"
	"errors"

	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

func ensureBuiltinSoulDefaults(ctx context.Context, repo repository.SoulRepository) error {
	if repo == nil {
		return common.ErrInvalidArgument
	}
	defaults, err := workorchdefaults.BuiltinSoulDefaults()
	if err != nil {
		return err
	}
	for _, item := range defaults {
		id := common.SoulID(item.ID)
		if _, err := repo.Get(ctx, id); err == nil {
			continue
		} else if !errors.Is(err, common.ErrNotFound) {
			return err
		}
		value := &soulmodel.Soul{ID: common.SoulID(item.ID), Name: item.Name, Description: item.Description, BusinessDomains: append([]string(nil), item.BusinessDomains...), SoulMD: item.SoulMD, AllowedSkillRefs: append([]string(nil), item.AllowedSkillRefs...), AllowedMCPServerRefs: append([]string(nil), item.AllowedMCPServerRefs...), AllowedCapabilityRefs: append([]string(nil), item.AllowedCapabilityRefs...), AllowedCapabilityCategories: append([]string(nil), item.AllowedCapabilityCategories...), AllowedToolKinds: append([]string(nil), item.AllowedToolKinds...)}
		if err := repo.Create(ctx, value); err != nil {
			if !errors.Is(err, common.ErrConflict) {
				return err
			}
			// A concurrent ensure may have created the seed after the read. Treat
			// that as success only when the resource is now observable.
			if _, getErr := repo.Get(ctx, id); getErr != nil {
				return getErr
			}
		}
	}
	return nil
}
