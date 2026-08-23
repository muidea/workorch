package server

import (
	"context"
	"fmt"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	executioncontract "github.com/muidea/workorch/internal/pkg/workorchexecutioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (s *serverState) admitWorkspaceExecution(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	if s == nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	if s.workspaceExecutionAdmission != nil {
		return s.workspaceExecutionAdmission.AdmitWorkspaceExecution(ctx, topic, command)
	}
	return workorchdevents.WorkspaceRunRequestedResult{}, fmt.Errorf("%w: workspace execution admission is not configured", common.ErrConflict)
}

func executionAdmissionHTTPResponse(config Config, action string, command workorchdevents.WorkspaceRunRequestedCommand, result workorchdevents.WorkspaceRunRequestedResult) (executioncontract.WorkspaceExecutionAdmissionResponse, int, error) {
	return executioncontract.NewWorkspaceExecutionAdmissionResponse(
		"workorch",
		config.Backend,
		config.DataRoot,
		action,
		command.Reason,
		command.Source,
		runtimecontract.RuntimeExecutionIntentAdmission(result),
	)
}
