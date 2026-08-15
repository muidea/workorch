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

func (p executionProjection) RecordWorkspaceExecutionQueued(command workorchdevents.WorkspaceRunRequestedCommand) {
	if p.projection != nil {
		p.projection.RecordWorkspaceExecutionQueued(command)
	}
	if p.transport != nil {
		p.transport.RecordWorkspaceExecutionTransportQueued(command)
	}
}

func (p executionProjection) RecordWorkspaceExecutionOutcome(topic string, command workorchdevents.WorkspaceRunRequestedCommand, outcome workorchdevents.WorkspaceRunRequestedResult) {
	if p.projection != nil {
		p.projection.RecordWorkspaceExecutionOutcome(topic, command, outcome)
	}
	if p.transport != nil {
		p.transport.RecordWorkspaceExecutionTransportOutcome(topic, command, outcome)
	}
}

func (p executionProjection) RecordWorkspaceExecutionRejected(topic string, command workorchdevents.WorkspaceRunRequestedCommand, cause error) {
	if p.projection != nil {
		p.projection.RecordWorkspaceExecutionRejected(topic, command, cause)
	}
	if p.transport != nil {
		p.transport.RecordWorkspaceExecutionTransportRejected(topic, command, cause)
	}
}

func (p executionProjection) RecordWorkspaceExecutionEnqueueFailure(topic string, command workorchdevents.WorkspaceRunRequestedCommand, cause error) {
	if p.projection != nil {
		p.projection.RecordWorkspaceExecutionEnqueueFailure(topic, command, cause)
	}
	if p.transport != nil {
		p.transport.RecordWorkspaceExecutionTransportEnqueueFailure(topic, command, cause)
	}
}
