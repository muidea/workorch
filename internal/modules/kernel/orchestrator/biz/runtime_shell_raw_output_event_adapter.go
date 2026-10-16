package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) handleRuntimeValidationOutput(event magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if m == nil || event == nil || event.Source() != orchestratorevents.RuntimeValidationOutputReadSource {
		result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
		return
	}
	command, ok := event.Data().(orchestratorevents.QueryRuntimeValidationOutputCommand)
	if !ok || command.Validate() != nil {
		result.Set(nil, runtimeOperationMagicFailure(common.ErrInvalidArgument))
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(event.Context(), command.WorkspaceID)
	if err != nil {
		result.Set(nil, runtimeOperationMagicFailure(err))
		return
	}
	value, err := m.queryRuntimeValidationRawOutput(ctx, shellevents.QueryRuntimeValidationRawOutputCommand{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, DispatchID: command.DispatchID,
		OperationID: command.OperationID, ExecutionKey: command.ExecutionKey, Fence: command.Fence,
		SourceReceiptHash: command.SourceReceiptHash, StageID: command.StageID, Stream: command.Stream,
	})
	if err != nil {
		result.Set(nil, runtimeOperationMagicFailure(err))
		return
	}
	output := orchestratorevents.QueryRuntimeValidationOutputResult{
		SourceReceiptHash: value.SourceReceiptHash, StageID: value.StageID, Stream: value.Stream,
		Identity: value.Identity.Clone(), SavedSHA256: value.SavedSHA256, Raw: append([]byte(nil), value.Raw...),
	}
	if err := output.ValidateFor(command); err != nil {
		result.Set(nil, runtimeOperationMagicFailure(err))
		return
	}
	result.Set(output, nil)
}

// queryRuntimeValidationRawOutput is the only production sender for the
// Shell-owned restricted raw-output read. The Shell owner independently
// revalidates the persisted receipt, stage, stream and storage reference; this
// adapter does not write Runtime state or grant evidence credit.
func (m *Biz) queryRuntimeValidationRawOutput(ctx context.Context, command shellevents.QueryRuntimeValidationRawOutputCommand) (shellevents.QueryRuntimeValidationRawOutputResult, error) {
	if m == nil || m.EventHub() == nil || command.Validate() != nil {
		return shellevents.QueryRuntimeValidationRawOutputResult{}, common.ErrInvalidArgument
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return shellevents.QueryRuntimeValidationRawOutputResult{}, err
	}
	event := magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeValidationRawOutput, ModuleID, runtimecontract.RuntimeModuleShell, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(eventcontract.QueryLaneKey(shellevents.TopicQueryRuntimeValidationRawOutput, eventcontract.LaneScope{
		WorkspaceID: string(command.WorkspaceID), RunID: string(command.RunID), SubjectID: command.ExecutionKey,
	}))
	result := m.SendEvent(event)
	if result == nil {
		return shellevents.QueryRuntimeValidationRawOutputResult{}, fmt.Errorf("Shell validation raw-output query returned no result")
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return shellevents.QueryRuntimeValidationRawOutputResult{}, resultErr
	}
	response, ok := data.(shellevents.QueryRuntimeValidationRawOutputResult)
	if !ok {
		return shellevents.QueryRuntimeValidationRawOutputResult{}, fmt.Errorf("Shell validation raw-output query returned invalid result type %T", data)
	}
	if err := response.ValidateFor(command); err != nil {
		return shellevents.QueryRuntimeValidationRawOutputResult{}, err
	}
	return response.Clone(), nil
}
