package contextregistryports

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

const (
	soulReadPageLimit = 100
	soulReadMaxPages  = 1000
	soulReadMaxItems  = 100000
)

func CreateSoul(ctx context.Context, hub magicevent.Hub, cmd workorchmanagementcontract.SoulMutation) (workorchmanagementcontract.Soul, error) {
	if hub == nil {
		return workorchmanagementcontract.Soul{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandCreate, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.CreateSoulCommand(soulCommandFromMutation(cmd)))
	ownerevent.BindLane(event, soulevents.Contracts(), soulevents.TopicSoulCommandCreate, "soul", cmd.ID, "", "", "")
	result, err := ownerevent.Result[soulevents.CreateSoulResult](hub.Send(event), soulevents.TopicSoulCommandCreate)
	if err != nil {
		return workorchmanagementcontract.Soul{}, err
	}
	return soulFromRecord(result.Soul)
}

func UpdateSoul(ctx context.Context, hub magicevent.Hub, cmd workorchmanagementcontract.SoulMutation) (workorchmanagementcontract.Soul, error) {
	if hub == nil {
		return workorchmanagementcontract.Soul{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandUpdate, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.UpdateSoulCommand(soulCommandFromMutation(cmd)))
	ownerevent.BindLane(event, soulevents.Contracts(), soulevents.TopicSoulCommandUpdate, "soul", cmd.ID, "", "", "")
	result, err := ownerevent.Result[soulevents.UpdateSoulResult](hub.Send(event), soulevents.TopicSoulCommandUpdate)
	if err != nil {
		return workorchmanagementcontract.Soul{}, err
	}
	return soulFromRecord(result.Soul)
}

func UpdateSoulByID(ctx context.Context, hub magicevent.Hub, soulID string, cmd workorchmanagementcontract.SoulMutation) (workorchmanagementcontract.Soul, error) {
	soulID = strings.TrimSpace(soulID)
	cmd.ID = strings.TrimSpace(cmd.ID)
	if soulID == "" {
		return workorchmanagementcontract.Soul{}, common.ErrInvalidArgument
	}
	if cmd.ID != "" && cmd.ID != soulID {
		return workorchmanagementcontract.Soul{}, common.ErrInvalidArgument
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
	ownerevent.BindLane(event, soulevents.Contracts(), soulevents.TopicSoulCommandDelete, "soul", soulID, "", "", "")
	result, err := ownerevent.Result[soulevents.DeleteSoulResult](hub.Send(event), soulevents.TopicSoulCommandDelete)
	if err != nil {
		return err
	}
	if !result.Deleted || result.SoulID != common.SoulID(soulID) {
		return common.ErrConflict
	}
	return nil
}

func ReadSoul(ctx context.Context, hub magicevent.Hub, soulID string) (workorchmanagementcontract.Soul, error) {
	if hub == nil {
		return workorchmanagementcontract.Soul{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandGet, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.GetSoulCommand{SoulID: common.SoulID(soulID)})
	ownerevent.BindLane(event, soulevents.Contracts(), soulevents.TopicSoulCommandGet, "soul", soulID, "", "", "")
	result, err := ownerevent.Result[soulevents.GetSoulResult](hub.Send(event), soulevents.TopicSoulCommandGet)
	if err != nil {
		return workorchmanagementcontract.Soul{}, err
	}
	return soulFromRecord(result.Soul)
}

func ReadSoulRecord(ctx context.Context, hub magicevent.Hub, soulID common.SoulID) (*soulmodel.Soul, error) {
	return ownerreadports.ReadSoulByOwnerCommand(ctx, hub, soulID)
}

func ListSouls(ctx context.Context, hub magicevent.Hub) ([]workorchmanagementcontract.Soul, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]workorchmanagementcontract.Soul, 0)
	var cursor common.SoulID
	seen := make(map[common.SoulID]struct{})
	for page := 0; page < soulReadMaxPages; page++ {
		event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandList, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, soulevents.ListSoulsCommand{CursorAfterID: cursor, Limit: soulReadPageLimit})
		ownerevent.BindLane(event, soulevents.Contracts(), soulevents.TopicSoulCommandList, "soul", "all", "", "", "")
		result, err := ownerevent.Result[soulevents.ListSoulsResult](hub.Send(event), soulevents.TopicSoulCommandList)
		if err != nil {
			return nil, err
		}
		if len(result.Souls) > soulReadPageLimit || len(ret)+len(result.Souls) > soulReadMaxItems {
			return nil, common.ErrConflict
		}
		previous := cursor
		for _, item := range result.Souls {
			if item == nil || item.ID == "" || (previous != "" && item.ID <= previous) {
				return nil, common.ErrConflict
			}
			ret = append(ret, contextregistrycore.MapSoul(item))
			previous = item.ID
		}
		if result.NextCursor == "" {
			return ret, nil
		}
		if len(result.Souls) == 0 || result.NextCursor != result.Souls[len(result.Souls)-1].ID || result.NextCursor <= cursor {
			return nil, common.ErrConflict
		}
		if _, duplicate := seen[result.NextCursor]; duplicate {
			return nil, common.ErrConflict
		}
		seen[result.NextCursor] = struct{}{}
		cursor = result.NextCursor
	}
	return nil, fmt.Errorf("%w: Soul owner pagination exceeded %d pages", common.ErrConflict, soulReadMaxPages)
}

func soulFromRecord(item *soulmodel.Soul) (workorchmanagementcontract.Soul, error) {
	if item == nil {
		return workorchmanagementcontract.Soul{}, common.ErrInvalidArgument
	}
	return contextregistrycore.MapSoul(item), nil
}
