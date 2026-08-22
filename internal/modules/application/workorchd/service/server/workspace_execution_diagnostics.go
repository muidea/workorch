package server

import workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"

// The transport diagnostics port is deliberately separate from the
// projection writer. Biz-owned execution admission sends derived facts to the
// projection component and may send these process-local counters to Service;
// Service never turns a diagnostic into an owner command.

func (s *Service) RecordWorkspaceExecutionTransportOutcome(topic string, _ workorchdevents.WorkspaceRunRequestedCommand, outcome workorchdevents.WorkspaceRunRequestedResult) {
	if s == nil || s.http == nil || s.http.state == nil {
		return
	}
	s.http.state.recordSchedulerCommandResult(topic, string(outcome.Outcome))
}

func (s *Service) RecordWorkspaceExecutionTransportRejected(topic string, command workorchdevents.WorkspaceRunRequestedCommand, cause error) {
	if s == nil || s.http == nil || s.http.state == nil {
		return
	}
	s.http.state.recordSchedulerCommandResult(topic, "failed")
	s.http.state.recordWorkspaceRunFailure(command, schedulerKindWorkspaceActivation, schedulerReasonWorkspaceActivationFailed, cause)
}
