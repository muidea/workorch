package governancecore

import (
	"fmt"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ValidateDecisionResolutionPayloadForTicket(ticket *decisionmodel.Ticket, payload map[string]any) error {
	if ticket == nil {
		return nil
	}
	if IsWorkorchRuntimeLLMProviderRecoveryTicket(ticket) && ContainsInlineLLMProviderConfig(payload) {
		return fmt.Errorf("%w: WorkOrch runtime LLM provider configuration must be changed in WorkOrch system configuration, not pasted into a decision answer", common.ErrInvalidArgument)
	}
	if IsRuntimeRepairExhaustionTicket(ticket) && IsAmbiguousRuntimeRepairExhaustionResolution(payload) {
		return fmt.Errorf("%w: runtime repair exhaustion requires an explicit recovery action: retry_same_step, replan, provide_information, or terminate", common.ErrInvalidArgument)
	}
	return nil
}

func EnsureDecisionTicketWorkspaceScope(runtimeWorkspaceID common.WorkspaceID, item *decisionmodel.Ticket) error {
	if item == nil {
		return common.ErrNotFound
	}
	if runtimeWorkspaceID == "" || item.WorkspaceID == runtimeWorkspaceID {
		return nil
	}
	return common.ErrNotFound
}
