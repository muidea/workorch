package server

import (
	"context"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

// WorkspaceExecutionAdmission is the one application ingress used by HTTP
// for workspace resume/restart. It returns only after the Workspace owner has
// synchronously admitted the normalized immutable command.
type WorkspaceExecutionAdmission interface {
	AdmitWorkspaceExecution(context.Context, string, workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error)
}
