package soul

import (
	"context"
	"errors"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

func (m *Biz) handleBuiltinDefaultsCommand(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if m == nil || m.service == nil || ev == nil || ev.Source() != applicationContextRegistrySource {
		setMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return
	}
	switch ev.ID() {
	case soulevents.TopicSoulCommandQueryBuiltinDefaults:
		if _, ok := ev.Data().(soulevents.QueryBuiltinSoulDefaultsCommand); !ok {
			setMagicResult(result, soulevents.QueryBuiltinSoulDefaultsResult{}, common.ErrInvalidArgument)
			return
		}
		items, err := m.service.queryBuiltinDefaults(ev.Context())
		setMagicResult(result, soulevents.QueryBuiltinSoulDefaultsResult{Defaults: items}, err)
	case soulevents.TopicSoulCommandUpgradeBuiltinDefault:
		cmd, ok := ev.Data().(soulevents.UpgradeBuiltinSoulDefaultCommand)
		if !ok {
			setMagicResult(result, soulevents.UpgradeBuiltinSoulDefaultResult{}, common.ErrInvalidArgument)
			return
		}
		value, err := m.service.submitBuiltinDefaultUpgrade(ev.Context(), soulBuiltinUpgradeEvent{SoulID: cmd.SoulID, ExpectedContentHash: cmd.ExpectedContentHash, TargetContentHash: cmd.TargetContentHash})
		setMagicResult(result, value, err)
	default:
		setMagicResult(result, struct{}{}, common.ErrInvalidArgument)
	}
}

func (s *service) queryBuiltinDefaults(ctx context.Context) ([]soulevents.BuiltinSoulDefault, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	defaults, err := workorchdefaults.BuiltinSoulDefaults()
	if err != nil {
		return nil, err
	}
	items := make([]soulevents.BuiltinSoulDefault, 0, len(defaults))
	for _, raw := range defaults {
		target, err := normalizedSoulFromCommand(soulDefaultCreateCommand(raw))
		if err != nil {
			return nil, err
		}
		current, err := s.getSoul(ctx, target.ID)
		if err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
		items = append(items, builtinSoulDefaultObservation(current, target))
	}
	return items, nil
}

func builtinSoulDefaultObservation(current, target *soulmodel.Soul) soulevents.BuiltinSoulDefault {
	value := soulevents.BuiltinSoulDefault{SoulID: target.ID, BuiltinContentHash: soulmodel.StableSoulContentHash(target)}
	if current == nil {
		return value
	}
	value.Present = true
	value.CurrentContentHash = current.ContentHash
	if current.SourceProvenance != soulmodel.SoulSourceBuiltinSeed {
		return value
	}
	if current.ContentHash == value.BuiltinContentHash {
		value.CurrentBuiltinRevision = value.BuiltinContentHash
		return value
	}
	value.CurrentBuiltinRevision = workorchdefaults.KnownBuiltinContentRevision("soul", string(current.ID), current.ContentHash)
	value.UpgradeAvailable = value.CurrentBuiltinRevision != ""
	return value
}

func (s *service) upgradeBuiltinDefault(ctx context.Context, event soulBuiltinUpgradeEvent) (soulevents.UpgradeBuiltinSoulDefaultResult, error) {
	defaults, err := workorchdefaults.BuiltinSoulDefaults()
	if err != nil {
		return soulevents.UpgradeBuiltinSoulDefaultResult{}, err
	}
	for _, raw := range defaults {
		if common.SoulID(raw.ID) != event.SoulID {
			continue
		}
		cmd := soulDefaultCreateCommand(raw)
		target, err := normalizedSoulFromCommand(cmd)
		if err != nil {
			return soulevents.UpgradeBuiltinSoulDefaultResult{}, err
		}
		current, err := s.getSoul(ctx, event.SoulID)
		if err != nil {
			return soulevents.UpgradeBuiltinSoulDefaultResult{}, err
		}
		value := builtinSoulDefaultObservation(current, target)
		if event.TargetContentHash != value.BuiltinContentHash {
			return soulevents.UpgradeBuiltinSoulDefaultResult{}, fmt.Errorf("%w: builtin Soul target changed", common.ErrConflict)
		}
		// Exact replay after a committed write/response loss is a no-op, but an
		// arbitrary expected hash cannot impersonate a reviewed upgrade request.
		knownExpected := event.ExpectedContentHash == value.BuiltinContentHash || workorchdefaults.KnownBuiltinContentRevision("soul", string(event.SoulID), event.ExpectedContentHash) != ""
		if current.SourceProvenance != soulmodel.SoulSourceBuiltinSeed || !knownExpected ||
			(value.CurrentContentHash != value.BuiltinContentHash && (!value.UpgradeAvailable || event.ExpectedContentHash != value.CurrentContentHash)) {
			return soulevents.UpgradeBuiltinSoulDefaultResult{}, fmt.Errorf("%w: Soul is customized, unknown, or changed since preview", common.ErrConflict)
		}
		if value.CurrentContentHash == value.BuiltinContentHash {
			return soulevents.UpgradeBuiltinSoulDefaultResult{Default: value}, nil
		}
		updated, err := s.updateSoulLocked(ctx, cmd)
		if err != nil {
			return soulevents.UpgradeBuiltinSoulDefaultResult{}, err
		}
		return soulevents.UpgradeBuiltinSoulDefaultResult{Default: builtinSoulDefaultObservation(updated, target), Upgraded: true}, nil
	}
	return soulevents.UpgradeBuiltinSoulDefaultResult{}, common.ErrNotFound
}
