package contextregistryports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	soulcontracts "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/contracts"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func CreateSoul(ctx context.Context, hub events.Hub, cmd contextregistrycore.Soul) (contextregistrycore.Soul, error) {
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicSoulCommandCreate, "soul", cmd.ID, map[string]any{
		events.PayloadKeyCommand: soulCommandFromView(cmd),
	})
	if err != nil {
		return contextregistrycore.Soul{}, err
	}
	return contextregistrycore.SoulFromPayload(response.Payload, events.PayloadKeySoul)
}

func UpdateSoul(ctx context.Context, hub events.Hub, cmd contextregistrycore.Soul) (contextregistrycore.Soul, error) {
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicSoulCommandUpdate, "soul", cmd.ID, map[string]any{
		events.PayloadKeyCommand: soulCommandFromView(cmd),
	})
	if err != nil {
		return contextregistrycore.Soul{}, err
	}
	return contextregistrycore.SoulFromPayload(response.Payload, events.PayloadKeySoul)
}

func UpdateSoulByID(ctx context.Context, hub events.Hub, soulID string, cmd contextregistrycore.Soul) (contextregistrycore.Soul, error) {
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

func DeleteSoul(ctx context.Context, hub events.Hub, soulID string) error {
	soulID = strings.TrimSpace(soulID)
	if soulID == "" {
		return common.ErrInvalidArgument
	}
	return PublishCommand(ctx, hub, events.TopicSoulCommandDelete, "soul", soulID, map[string]any{
		events.PayloadKeyCommand: soulcontracts.DeleteSoulCommand{SoulID: common.SoulID(soulID)},
	})
}

func ReadSoul(ctx context.Context, hub events.Hub, soulID string) (contextregistrycore.Soul, error) {
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicSoulCommandGet, "soul", soulID, map[string]any{
		events.PayloadKeySoulID: soulID,
	})
	if err != nil {
		return contextregistrycore.Soul{}, err
	}
	return contextregistrycore.SoulFromPayload(response.Payload, events.PayloadKeySoul)
}

func ReadSoulRecord(ctx context.Context, hub events.Hub, soulID common.SoulID) (*soulmodel.Soul, error) {
	return ownerreadports.ReadSoulByOwnerCommand(ctx, hub, soulID)
}

func ListSouls(ctx context.Context, hub events.Hub) ([]contextregistrycore.Soul, error) {
	response, err := PublishCommandWithResponse(ctx, hub, events.TopicSoulCommandList, "soul", "all", nil)
	if err != nil {
		return nil, err
	}
	return contextregistrycore.SoulsFromPayload(response.Payload, events.PayloadKeySouls)
}
