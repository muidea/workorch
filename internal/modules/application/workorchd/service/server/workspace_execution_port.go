package server

import (
	"context"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

// WorkspaceExecutionAdmission is the one application ingress used by HTTP
// for workspace resume/restart. It returns the normalized immutable command
// that is safe to expose in the HTTP 202 response.
type WorkspaceExecutionAdmission interface {
	EnqueueWorkspaceExecution(context.Context, string, workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedCommand, error)
}
