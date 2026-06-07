package activationports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ValidateSoulRoutingCapabilitySelection(ctx context.Context, hub events.Hub, routing appmodelcore.SoulRouting, skillIDs []string, mcpServerIDs []string, capabilityIDs []string, capabilityCategories []string, toolKinds []string) error {
	if routing.Status != "resolved" {
		return nil
	}
	execution := routing.Bindings[appmodelcore.SoulBindingStageExecution]
	soulID := common.SoulID(strings.TrimSpace(execution.SoulID))
	if soulID == "" {
		soulID = common.SoulID(strings.TrimSpace(routing.EffectiveDefaultSoulID))
	}
	return contextregistryports.ValidateSoulCapabilitySelection(ctx, hub, soulID, skillIDs, mcpServerIDs, capabilityIDs, capabilityCategories, toolKinds)
}
