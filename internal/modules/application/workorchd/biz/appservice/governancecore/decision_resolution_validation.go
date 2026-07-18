package governancecore

import (
	"fmt"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func ValidateDecisionResolutionPayloadForTicket(ticket *decisionmodel.Ticket, payload workorcheventpayload.Payload) error {
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

// ValidateDecisionResolutionTypeForTicket is the application-side preflight
// for single and batch APIs. Decision owner repeats this check immediately
// before durable writes; keeping this early gate makes a mixed batch reject
// before any selected ticket can be resolved.
func ValidateDecisionResolutionTypeForTicket(ticket *decisionmodel.Ticket, resolutionType string) error {
	if ticket == nil || !decisioncommon.IsSupportedResolutionType(resolutionType) {
		return common.ErrInvalidArgument
	}
	if ticket.DecisionKind == "runtime_v2_governance" && len(ticket.AllowedRecoveryActions) == 0 {
		return fmt.Errorf("%w: runtime_v2_governance ticket has no allowed recovery actions", common.ErrInvalidArgument)
	}
	if len(ticket.AllowedRecoveryActions) == 0 {
		return nil
	}
	for _, action := range ticket.AllowedRecoveryActions {
		if action == resolutionType {
			return nil
		}
	}
	return fmt.Errorf("%w: resolution type %q is not allowed for ticket %s", common.ErrInvalidArgument, resolutionType, ticket.ID)
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
