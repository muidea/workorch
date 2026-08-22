package biz

import (
	projectioncomponent "github.com/muidea/workorch/internal/modules/application/workorchd/biz/projection"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
)

// executionProjection is the application-local fan-out for execution facts.
// The projection component is the only derived-store writer; Service receives
// a separate diagnostics-only observation. Keeping the two responsibilities
// explicit prevents the HTTP transport from becoming an accidental projection
// owner while preserving its metrics/read endpoints.
type executionProjection struct {
	projection *projectioncomponent.Component
	transport  *server.Service
}

func (p executionProjection) RecordWorkspaceExecutionOutcome(topic string, command workorchdevents.WorkspaceRunRequestedCommand, outcome workorchdevents.WorkspaceRunRequestedResult) {
	if p.projection != nil {
		p.projection.RecordWorkspaceExecutionOutcome(topic, command, outcome)
	}
	if p.transport != nil {
		p.transport.RecordWorkspaceExecutionTransportOutcome(topic, command, outcome)
	}
}

// RecordWorkspaceExecutionFailure records transport/contract diagnostics only.
// Without a valid Workspace-owner result there is no immutable runCommand
// outcome to project.
func (p executionProjection) RecordWorkspaceExecutionFailure(topic string, command workorchdevents.WorkspaceRunRequestedCommand, cause error) {
	if p.transport != nil {
		p.transport.RecordWorkspaceExecutionTransportFailure(topic, command, cause)
	}
}
