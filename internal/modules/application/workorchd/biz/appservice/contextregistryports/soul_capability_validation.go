package contextregistryports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ValidateSoulCapabilitySelection(ctx context.Context, hub magicevent.Hub, soulID common.SoulID, skillIDs []string, mcpServerIDs []string, capabilityIDs []string, capabilityCategories []string, toolKinds []string) error {
	if soulID == "" {
		return nil
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	command := soulevents.ValidateCapabilitySelectionCommand{
		SoulID:               soulID,
		SkillRefs:            contextregistrycore.CloneStrings(skillIDs),
		MCPServerRefs:        contextregistrycore.CloneStrings(mcpServerIDs),
		CapabilityRefs:       contextregistrycore.CloneStrings(capabilityIDs),
		CapabilityCategories: contextregistrycore.CloneStrings(capabilityCategories),
		ToolKinds:            contextregistrycore.CloneStrings(toolKinds),
	}
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandValidateCapability, sourceModule, soulevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, soulevents.Contracts(), soulevents.TopicSoulCommandValidateCapability, "soul", string(soulID), "", "", "")
	result, err := ownerevent.Result[soulevents.ValidateCapabilitySelectionResult](hub.Send(event), soulevents.TopicSoulCommandValidateCapability)
	if err != nil {
		return err
	}
	if !result.Validated || result.SoulID != soulID {
		return common.ErrConflict
	}
	return nil
}
