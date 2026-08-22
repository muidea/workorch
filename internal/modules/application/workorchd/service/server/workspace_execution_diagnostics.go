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
	s.http.state.recordExecutionAdmissionResult(topic, string(outcome.Outcome))
}

func (s *Service) RecordWorkspaceExecutionTransportFailure(topic string, _ workorchdevents.WorkspaceRunRequestedCommand, cause error) {
	if s == nil || s.http == nil || s.http.state == nil {
		return
	}
	s.http.state.recordExecutionAdmissionResult(topic, "failed")
	if cause != nil {
		s.http.state.recordExecutionAdmissionFailure(executionAdmissionKindFromTopic(topic), executionAdmissionReasonSubmissionFailed)
	}
}

func (s *serverState) recordExecutionAdmissionResult(topic string, result string) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.executionAdmissionMetrics.recordCommand(executionAdmissionKindFromTopic(topic), result)
}

func (s *serverState) recordExecutionAdmissionFailure(kind string, reason string) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.executionAdmissionMetrics.recordFailure(kind, reason)
}
