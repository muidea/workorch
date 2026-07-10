package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	soulevents "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CreateSoul(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.Soul) (contextregistrycore.Soul, error) {
	if hub == nil {
		return contextregistrycore.Soul{}, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[soulevents.CreateSoulCommand, soulevents.SoulCommandResult](ctx, hub, soulevents.TopicSoulCommandCreate, soulevents.CommandDestination, "soul", cmd.ID, soulevents.CreateSoulCommand(soulCommandFromView(cmd)))
	if err != nil {
		return contextregistrycore.Soul{}, err
	}
	return soulFromCommandResult(result)
}

func UpdateSoul(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.Soul) (contextregistrycore.Soul, error) {
	if hub == nil {
		return contextregistrycore.Soul{}, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[soulevents.UpdateSoulCommand, soulevents.SoulCommandResult](ctx, hub, soulevents.TopicSoulCommandUpdate, soulevents.CommandDestination, "soul", cmd.ID, soulevents.UpdateSoulCommand(soulCommandFromView(cmd)))
	if err != nil {
		return contextregistrycore.Soul{}, err
	}
	return soulFromCommandResult(result)
}

func UpdateSoulByID(ctx context.Context, hub magicevent.Hub, soulID string, cmd contextregistrycore.Soul) (contextregistrycore.Soul, error) {
	soulID = strings.TrimSpace(soulID)
	cmd.ID = strings.TrimSpace(cmd.ID)
	if soulID == "" {
		return contextregistrycore.Soul{}, common.ErrInvalidArgument
	}
	if cmd.ID != "" && cmd.ID != soulID {
		return contextregistrycore.Soul{}, common.ErrInvalidArgument
	}
	cmd.ID = soulID
	return UpdateSoul(ctx, hub, cmd)
}

func DeleteSoul(ctx context.Context, hub magicevent.Hub, soulID string) error {
	soulID = strings.TrimSpace(soulID)
	if hub == nil || soulID == "" {
		return common.ErrInvalidArgument
	}
	_, err := sendMagicCommand[soulevents.DeleteSoulCommand, soulevents.SoulCommandResult](ctx, hub, soulevents.TopicSoulCommandDelete, soulevents.CommandDestination, "soul", soulID, soulevents.DeleteSoulCommand{SoulID: common.SoulID(soulID)})
	return err
}

func ReadSoul(ctx context.Context, hub magicevent.Hub, soulID string) (contextregistrycore.Soul, error) {
	if hub == nil {
		return contextregistrycore.Soul{}, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[soulevents.GetSoulCommand, soulevents.SoulCommandResult](ctx, hub, soulevents.TopicSoulCommandGet, soulevents.CommandDestination, "soul", soulID, soulevents.GetSoulCommand{SoulID: common.SoulID(soulID)})
	if err != nil {
		return contextregistrycore.Soul{}, err
	}
	return soulFromCommandResult(result)
}

func ReadSoulRecord(ctx context.Context, hub magicevent.Hub, soulID common.SoulID) (*soulmodel.Soul, error) {
	return ownerreadports.ReadSoulByOwnerCommand(ctx, hub, soulID)
}

func ListSouls(ctx context.Context, hub magicevent.Hub) ([]contextregistrycore.Soul, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	result, err := sendMagicCommand[soulevents.ListSoulsCommand, soulevents.SoulCommandResult](ctx, hub, soulevents.TopicSoulCommandList, soulevents.CommandDestination, "soul", "all", soulevents.ListSoulsCommand{})
	if err != nil {
		return nil, err
	}
	return soulsFromCommandResult(result)
}

func soulFromCommandResult(result soulevents.SoulCommandResult) (contextregistrycore.Soul, error) {
	if result.Soul == nil {
		return contextregistrycore.Soul{}, common.ErrInvalidArgument
	}
	return contextregistrycore.MapSoul(result.Soul), nil
}

func soulsFromCommandResult(result soulevents.SoulCommandResult) ([]contextregistrycore.Soul, error) {
	ret := make([]contextregistrycore.Soul, 0, len(result.Souls))
	for _, item := range result.Souls {
		ret = append(ret, contextregistrycore.MapSoul(item))
	}
	return ret, nil
}
