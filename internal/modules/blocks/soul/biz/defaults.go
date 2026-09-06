package soul

import (
	"context"
	"errors"

	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// Seeding and automatic reviewed-version upgrades remain Soul-owner work.
// Existing Run snapshots are never consulted or rewritten by this operation.
func (s *service) ensureCurrentBuiltinSoulDefaults(ctx context.Context) error {
	if err := ensureBuiltinSoulDefaults(ctx, s); err != nil {
		return err
	}
	defaults, err := workorchdefaults.BuiltinSoulDefaults()
	if err != nil {
		return err
	}
	for _, raw := range defaults {
		stored, err := s.souls.Get(ctx, common.SoulID(raw.ID))
		if errors.Is(err, common.ErrNotFound) {
			continue // a concurrent delete is not consent to recreate again
		}
		if err != nil {
			return err
		}
		current, err := normalizedStoredSoul(stored)
		if err != nil {
			continue // unknown legacy identity is not an upgrade baseline
		}
		target, err := normalizedSoulFromCommand(soulDefaultCreateCommand(raw))
		if err != nil {
			return err
		}
		item := builtinSoulDefaultObservation(current, target)
		if !item.UpgradeAvailable {
			continue
		}
		_, err = s.submitBuiltinDefaultUpgrade(ctx, soulBuiltinUpgradeEvent{SoulID: item.SoulID, ExpectedContentHash: item.CurrentContentHash, TargetContentHash: item.BuiltinContentHash})
		if err != nil && !errors.Is(err, common.ErrConflict) {
			return err
		}
	}
	return nil
}

func ensureBuiltinSoulDefaults(ctx context.Context, service *service) error {
	if service == nil || service.souls == nil {
		return common.ErrInvalidArgument
	}
	defaults, err := workorchdefaults.BuiltinSoulDefaults()
	if err != nil {
		return err
	}
	return ensureSoulDefaults(ctx, service, defaults)
}

func ensureSoulDefaults(ctx context.Context, service *service, defaults []workorchmanagementcontract.Soul) error {
	if service == nil || service.souls == nil {
		return common.ErrInvalidArgument
	}
	for _, item := range defaults {
		cmd := soulDefaultCreateCommand(item)
		validated, err := normalizedSoulFromCommand(cmd)
		if err != nil {
			return err
		}
		id := validated.ID
		if _, err := service.souls.Get(ctx, id); err == nil {
			continue
		} else if !errors.Is(err, common.ErrNotFound) {
			return err
		}
		_, err = service.createSoul(ctx, cmd, soulmodel.SoulSourceBuiltinSeed)
		if err != nil {
			if !errors.Is(err, common.ErrConflict) {
				return err
			}
			// A concurrent ensure may have created the seed after the read. Treat
			// that as success only when the resource is now observable.
			if _, getErr := service.souls.Get(ctx, id); getErr != nil {
				return getErr
			}
		}
	}
	return nil
}

func soulDefaultCreateCommand(item workorchmanagementcontract.Soul) soulevents.CreateSoulCommand {
	return soulevents.CreateSoulCommand{
		SoulID: common.SoulID(item.ID), Name: item.Name, Description: item.Description,
		BusinessDomains: append([]string(nil), item.BusinessDomains...), SoulMD: item.SoulMD,
		AllowedSkillRefs: append([]string(nil), item.AllowedSkillRefs...), AllowedMCPServerRefs: append([]string(nil), item.AllowedMCPServerRefs...),
		AllowedCapabilityRefs: append([]string(nil), item.AllowedCapabilityRefs...), AllowedCapabilityCategories: append([]string(nil), item.AllowedCapabilityCategories...),
		AllowedToolKinds: append([]string(nil), item.AllowedToolKinds...),
	}
}
