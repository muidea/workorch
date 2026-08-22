package server

import (
	"context"
	"fmt"
	"strings"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (s *serverState) admitWorkspaceExecution(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedCommand, error) {
	if s == nil {
		return workorchdevents.WorkspaceRunRequestedCommand{}, common.ErrInvalidArgument
	}
	if s.workspaceExecutionAdmission != nil {
		return s.workspaceExecutionAdmission.AdmitWorkspaceExecution(ctx, topic, command)
	}
	return workorchdevents.WorkspaceRunRequestedCommand{}, fmt.Errorf("%w: workspace execution admission is not configured", common.ErrConflict)
}

func executionAdmissionAcceptedHTTPBody(config Config, action string, command workorchdevents.WorkspaceRunRequestedCommand) map[string]any {
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	body := map[string]any{
		"service":     "workorch",
		"backend":     config.Backend,
		"dataRoot":    config.DataRoot,
		"accepted":    true,
		"workspaceId": workspaceID,
	}
	if value := strings.TrimSpace(action); value != "" {
		body["action"] = value
	}
	for key, value := range map[string]string{
		recordpayload.PayloadKeyIntent:    command.Intent,
		recordpayload.PayloadKeyRequestID: command.RequestID,
		recordpayload.PayloadKeySource:    command.Source,
		recordpayload.PayloadKeyReason:    command.Reason,
	} {
		if value = strings.TrimSpace(value); value != "" {
			body[key] = value
		}
	}
	return body
}
