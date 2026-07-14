package contextregistryports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CreateSoul(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.Soul) (contextregistrycore.Soul, error) {
	if hub == nil {
		return contextregistrycore.Soul{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandCreate, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.CreateSoulCommand(soulCommandFromView(cmd)))
	ownerevent.BindLane(event, soulevents.TopicSoulCommandCreate, "soul", cmd.ID, "", "", "")
	result, err := ownerevent.Soul(hub.Send(event), soulevents.TopicSoulCommandCreate)
	if err != nil {
		return contextregistrycore.Soul{}, err
	}
	return soulFromCommandResult(result)
}

func UpdateSoul(ctx context.Context, hub magicevent.Hub, cmd contextregistrycore.Soul) (contextregistrycore.Soul, error) {
	if hub == nil {
		return contextregistrycore.Soul{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandUpdate, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.UpdateSoulCommand(soulCommandFromView(cmd)))
	ownerevent.BindLane(event, soulevents.TopicSoulCommandUpdate, "soul", cmd.ID, "", "", "")
	result, err := ownerevent.Soul(hub.Send(event), soulevents.TopicSoulCommandUpdate)
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
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandDelete, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.DeleteSoulCommand{SoulID: common.SoulID(soulID)})
	ownerevent.BindLane(event, soulevents.TopicSoulCommandDelete, "soul", soulID, "", "", "")
	_, err := ownerevent.Soul(hub.Send(event), soulevents.TopicSoulCommandDelete)
	return err
}

func ReadSoul(ctx context.Context, hub magicevent.Hub, soulID string) (contextregistrycore.Soul, error) {
	if hub == nil {
		return contextregistrycore.Soul{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandGet, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.GetSoulCommand{SoulID: common.SoulID(soulID)})
	ownerevent.BindLane(event, soulevents.TopicSoulCommandGet, "soul", soulID, "", "", "")
	result, err := ownerevent.Soul(hub.Send(event), soulevents.TopicSoulCommandGet)
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
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandList, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.ListSoulsCommand{})
	ownerevent.BindLane(event, soulevents.TopicSoulCommandList, "soul", "all", "", "", "")
	result, err := ownerevent.Soul(hub.Send(event), soulevents.TopicSoulCommandList)
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
