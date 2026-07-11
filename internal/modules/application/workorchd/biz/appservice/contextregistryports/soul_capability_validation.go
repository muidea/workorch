package contextregistryports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
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
	_, err := sendMagicCommand[soulevents.ValidateCapabilitySelectionCommand, soulevents.SoulCommandResult](
		ctx,
		hub,
		soulevents.TopicSoulCommandValidateCapability,
		soulevents.CommandDestination,
		"soul",
		string(soulID),
		soulevents.ValidateCapabilitySelectionCommand{
			SoulID:               soulID,
			SkillRefs:            contextregistrycore.CloneStrings(skillIDs),
			MCPServerRefs:        contextregistrycore.CloneStrings(mcpServerIDs),
			CapabilityRefs:       contextregistrycore.CloneStrings(capabilityIDs),
			CapabilityCategories: contextregistrycore.CloneStrings(capabilityCategories),
			ToolKinds:            contextregistrycore.CloneStrings(toolKinds),
		},
	)
	return err
}
