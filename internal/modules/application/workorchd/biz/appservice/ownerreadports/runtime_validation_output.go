package ownerreadports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadRuntimeValidationOutput(ctx context.Context, hub magicevent.Hub, command orchestratorevents.QueryRuntimeValidationOutputCommand) (orchestratorevents.QueryRuntimeValidationOutputResult, error) {
	if hub == nil || command.Validate() != nil || workspaceScopeFromContext(ctx) != command.WorkspaceID {
		return orchestratorevents.QueryRuntimeValidationOutputResult{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorQueryRuntimeValidationOutput, orchestratorevents.RuntimeValidationOutputReadSource, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, orchestratorevents.Contracts(), event.ID(), "operation", command.ExecutionKey, command.WorkspaceID, command.RunID, "")
	response := hub.Send(event)
	if response == nil {
		return orchestratorevents.QueryRuntimeValidationOutputResult{}, fmt.Errorf("runtime validation output query returned no result")
	}
	data, err := response.Get()
	if err != nil {
		return orchestratorevents.QueryRuntimeValidationOutputResult{}, err
	}
	value, ok := data.(orchestratorevents.QueryRuntimeValidationOutputResult)
	if !ok {
		return orchestratorevents.QueryRuntimeValidationOutputResult{}, fmt.Errorf("runtime validation output query returned invalid result type %T", data)
	}
	if err := value.ValidateFor(command); err != nil {
		return orchestratorevents.QueryRuntimeValidationOutputResult{}, err
	}
	value.Raw = append([]byte(nil), value.Raw...)
	value.Identity = value.Identity.Clone()
	return value, nil
}
