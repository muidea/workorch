package contextregistryports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	soulcontracts "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ValidateSoulCapabilitySelection(ctx context.Context, hub events.Hub, soulID common.SoulID, skillIDs []string, mcpServerIDs []string, capabilityIDs []string, capabilityCategories []string, toolKinds []string) error {
	if soulID == "" {
		return nil
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	return PublishCommand(ctx, hub, events.TopicSoulCommandValidateCapability, "soul", string(soulID), map[string]any{
		events.PayloadKeyCommand: soulcontracts.ValidateCapabilitySelectionCommand{
			SoulID:               soulID,
			SkillRefs:            contextregistrycore.CloneStrings(skillIDs),
			MCPServerRefs:        contextregistrycore.CloneStrings(mcpServerIDs),
			CapabilityRefs:       contextregistrycore.CloneStrings(capabilityIDs),
			CapabilityCategories: contextregistrycore.CloneStrings(capabilityCategories),
			ToolKinds:            contextregistrycore.CloneStrings(toolKinds),
		},
	})
}
