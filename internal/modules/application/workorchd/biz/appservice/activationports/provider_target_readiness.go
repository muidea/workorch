package activationports

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func EnsureProviderInferenceSelection(ctx context.Context, hub magicevent.Hub, selection runtimecontract.ProviderInferenceSelection) (runtimecontract.ProviderInferenceSelection, error) {
	selection = selection.Normalized()
	if hub == nil {
		return runtimecontract.ProviderInferenceSelection{}, common.ErrInvalidArgument
	}
	if err := selection.Validate(); err != nil {
		return runtimecontract.ProviderInferenceSelection{}, err
	}
	event := magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandValidateInferenceSelection,
		modulecommon.ModuleID,
		llmevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		llmevents.ValidateProviderInferenceSelectionCommand{Selection: selection, Operation: workorchconfig.LLMOperationResponses},
	)
	event.BindLaneKey(events.EventHubLaneKeyFor(llmevents.Contracts(), llmevents.TopicLLMCommandValidateInferenceSelection, eventcontract.LaneScope{
		SubjectType: "llm_selection", SubjectID: selection.Target.ProviderID + "/" + selection.Target.ModelID + "/" + selection.ReasoningEffort,
	}))
	result := hub.Send(event)
	if result == nil {
		return runtimecontract.ProviderInferenceSelection{}, common.ErrInvalidArgument
	}
	if resultErr := result.Error(); resultErr != nil {
		if resultErr.Cause != nil {
			return runtimecontract.ProviderInferenceSelection{}, resultErr.Cause
		}
		return runtimecontract.ProviderInferenceSelection{}, resultErr
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return runtimecontract.ProviderInferenceSelection{}, resultErr
	}
	response, ok := data.(llmevents.ValidateProviderInferenceSelectionResult)
	if !ok || !response.Selection.Same(selection) || strings.TrimSpace(response.CapabilityRevision) == "" {
		return runtimecontract.ProviderInferenceSelection{}, fmt.Errorf("%w: invalid inference selection validation result", common.ErrInvalidArgument)
	}
	return response.Selection, nil
}
