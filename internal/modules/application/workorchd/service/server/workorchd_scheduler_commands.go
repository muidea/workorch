package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type workspaceRestartRequest struct {
	WorkspaceID string
	Priority    int
	Command     appservice.ActivateWorkspaceExecutionCommand
}

type workspaceRunRequest struct {
	WorkspaceID           string
	RunID                 string
	Intent                string
	Action                string
	Reason                string
	RequestID             string
	DefinitionRevision    string
	DecisionID            string
	Priority              int
	ExecutionContextPatch map[string]any
	Command               appservice.ActivateWorkspaceExecutionCommand
}

type runContinuationRequest struct {
	WorkspaceID           string
	RunID                 string
	Action                string
	Priority              int
	ExecutionContextPatch map[string]any
}

func (s *serverState) publishGovernanceContinuationRequested(ctx context.Context, refs []governanceRunResumeRef, priority int) error {
	for _, ref := range uniqueGovernanceRunResumeRefs(refs) {
		intent := strings.TrimSpace(ref.Intent)
		if intent == "" {
			intent = "resume"
		}
		reason := strings.TrimSpace(ref.Reason)
		if reason == "" {
			reason = "governance_resolved"
		}
		if err := s.publishWorkorchDSchedulerCommand(ctx, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, ref.WorkspaceID, ref.RunID, map[string]any{
			events.PayloadKeyWorkspaceID: ref.WorkspaceID,
			events.PayloadKeyRunID:       ref.RunID,
			events.PayloadKeyIntent:      intent,
			events.PayloadKeyReason:      reason,
			events.PayloadKeyRequestID:   ref.RequestID,
			events.PayloadKeySource:      "governance",
			events.PayloadKeyDecisionID:  ref.DecisionID,
			events.PayloadKeyPriority:    priority,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *serverState) publishWorkorchDSchedulerCommand(ctx context.Context, topic string, workspaceID string, runID string, payload map[string]any) error {
	if s == nil || s.schedulerHub == nil || strings.TrimSpace(topic) == "" {
		return common.ErrInvalidArgument
	}
	payload = ensureSchedulerCommandRequestID(topic, workspaceID, runID, payload)
	payload[events.PayloadKeySource] = schedulerCommandSource(schedulerStringValue(payload, events.PayloadKeySource), "http")
	envelope := events.Envelope{
		Topic:        topic,
		SourceModule: "workorchd/http",
		SubjectType:  schedulerSubjectType(topic),
		SubjectID:    schedulerSubjectID(topic, workspaceID, runID, payload),
		WorkspaceID:  strings.TrimSpace(workspaceID),
		RunID:        strings.TrimSpace(runID),
		Payload:      cloneSchedulerPayload(payload),
	}
	if err := s.schedulerHub.Publish(ctx, envelope); err != nil {
		if response, ok := events.ResponseEnvelope(err); ok {
			if validateErr := validateWorkorchDSchedulerAcceptedResponse(topic, response); validateErr != nil {
				s.recordSchedulerCommandResult(topic, "not_accepted")
				return validateErr
			}
			s.recordSchedulerCommandResult(topic, "accepted")
			return nil
		}
		s.recordSchedulerCommandResult(topic, "failed")
		return err
	}
	s.recordSchedulerCommandResult(topic, "missing_response")
	return fmt.Errorf("%w: scheduler command %s returned no accepted response", common.ErrConflict, topic)
}

func schedulerCommandSource(source string, fallback string) string {
	source = strings.TrimSpace(source)
	if source != "" {
		return source
	}
	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		return fallback
	}
	return "unknown"
}

func firstNonEmptySchedulerString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func ensureSchedulerCommandRequestID(topic string, workspaceID string, runID string, payload map[string]any) map[string]any {
	ret := cloneSchedulerPayload(payload)
	if ret == nil {
		ret = map[string]any{}
	}
	if strings.TrimSpace(schedulerStringValue(ret, events.PayloadKeyRequestID)) != "" {
		return ret
	}
	parts := []string{
		"workorchd",
		schedulerKindFromTopic(topic),
		strings.TrimSpace(workspaceID),
		strings.TrimSpace(runID),
		fmt.Sprintf("%d", time.Now().UTC().UnixNano()),
	}
	for idx, part := range parts {
		if part == "" {
			parts[idx] = "none"
		}
	}
	ret[events.PayloadKeyRequestID] = strings.Join(parts, "-")
	return ret
}

func prepareHTTPSchedulerCommandPayload(topic string, workspaceID string, runID string, payload map[string]any) map[string]any {
	ret := ensureSchedulerCommandRequestID(topic, workspaceID, runID, payload)
	ret[events.PayloadKeySource] = schedulerCommandSource(schedulerStringValue(ret, events.PayloadKeySource), "http")
	return ret
}

func schedulerAcceptedHTTPBody(config Config, workspaceID string, runID string, action string, payload map[string]any) map[string]any {
	body := map[string]any{
		"service":     "workorch",
		"backend":     config.Backend,
		"dataRoot":    config.DataRoot,
		"accepted":    true,
		"workspaceId": workspaceID,
	}
	if strings.TrimSpace(runID) != "" {
		body["runId"] = runID
	}
	if value := strings.TrimSpace(action); value != "" {
		body["action"] = value
	}
	for _, key := range []string{
		events.PayloadKeyIntent,
		events.PayloadKeyRequestID,
		events.PayloadKeySource,
		events.PayloadKeyReason,
		events.PayloadKeyDefinitionRevision,
		events.PayloadKeyDecisionID,
	} {
		if value := schedulerStringValue(payload, key); value != "" {
			body[key] = value
		}
	}
	return body
}

func validateWorkorchDSchedulerAcceptedResponse(topic string, response events.Envelope) error {
	if strings.TrimSpace(response.Topic) != strings.TrimSpace(topic)+"/accepted" {
		return fmt.Errorf("%w: scheduler command %s returned unexpected response topic %s", common.ErrConflict, topic, response.Topic)
	}
	if accepted, _ := response.Payload["accepted"].(bool); !accepted {
		return fmt.Errorf("%w: scheduler command %s was not accepted", common.ErrConflict, topic)
	}
	if strings.TrimSpace(response.SourceModule) != "workorchd/scheduler" {
		return fmt.Errorf("%w: scheduler command %s returned response from %s", common.ErrConflict, topic, response.SourceModule)
	}
	return nil
}

func cloneSchedulerPayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func schedulerSubjectType(topic string) string {
	switch topic {
	case workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested:
		return "workspace-run"
	default:
		return "workorchd-scheduler-command"
	}
}

func schedulerKindFromTopic(topic string) string {
	switch topic {
	case workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested:
		return schedulerKindWorkspaceRun
	default:
		return "workorchd_scheduler_command"
	}
}

func (s *serverState) recordSchedulerCommandResult(topic string, result string) {
	if s == nil || s.schedulerDiagnostics == nil {
		return
	}
	s.schedulerDiagnostics.recordCommand(schedulerKindFromTopic(topic), result)
}

func (s *serverState) recordSchedulerCommandEnqueueFailure(topic string, workspaceID string, runID string, detail string) {
	if s == nil || s.schedulerDiagnostics == nil {
		return
	}
	kind := schedulerKindFromTopic(topic)
	s.schedulerDiagnostics.recordCommandEnqueueFailure(kind, schedulerReasonEnqueueFailed)
	s.schedulerDiagnostics.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID:    workspaceID,
		RunID:          runID,
		Kind:           kind,
		State:          schedulerDiagnosticStateFailed,
		Reason:         schedulerReasonEnqueueFailed,
		Detail:         detail,
		Recoverability: schedulerRecoverabilityManualRetry,
		DedupeKey:      schedulerDedupeKey(workspaceID, runID, kind, schedulerReasonEnqueueFailed),
	})
}

func schedulerSubjectID(topic string, workspaceID string, runID string, payload map[string]any) string {
	switch topic {
	case workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested:
		return strings.TrimSpace(workspaceID)
	default:
		if id := strings.TrimSpace(runID); id != "" {
			return id
		}
	}
	if value, _ := payload[events.PayloadKeyRunID].(string); strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(workspaceID)
}

func (s *serverState) registerWorkorchDSchedulerCommandWatcher() {
	if s == nil || s.schedulerHub == nil {
		return
	}
	topics := []string{
		workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested,
	}
	for _, topic := range topics {
		topic := topic
		unsubscribe := s.schedulerHub.Subscribe(topic, func(ctx context.Context, envelope events.Envelope) error {
			if err := s.handleWorkorchDSchedulerCommand(ctx, envelope); err != nil {
				return err
			}
			s.projectAcceptedSchedulerCommand(envelope)
			return events.Response(events.Envelope{
				Topic:        envelope.Topic + "/accepted",
				SourceModule: "workorchd/scheduler",
				SubjectType:  envelope.SubjectType,
				SubjectID:    envelope.SubjectID,
				WorkspaceID:  envelope.WorkspaceID,
				RunID:        envelope.RunID,
				Payload: map[string]any{
					"accepted": true,
				},
			})
		})
		s.eventUnsubscribes = append(s.eventUnsubscribes, unsubscribe)
	}
}

func (s *serverState) projectAcceptedSchedulerCommand(envelope events.Envelope) {
	if s == nil || s.workspaceProjection == nil {
		return
	}
	workspaceID := schedulerString(envelope, events.PayloadKeyWorkspaceID, envelope.WorkspaceID)
	if strings.TrimSpace(workspaceID) == "" {
		return
	}
	s.workspaceProjection.ApplyRunCommandAccepted(workspaceviewcore.RunCommandAcceptedUpdate{
		WorkspaceID:        workspaceID,
		RunID:              schedulerString(envelope, events.PayloadKeyRunID, envelope.RunID),
		Intent:             acceptedSchedulerCommandIntent(envelope),
		Reason:             schedulerStringValue(envelope.Payload, events.PayloadKeyReason),
		RequestID:          schedulerStringValue(envelope.Payload, events.PayloadKeyRequestID),
		Source:             acceptedSchedulerCommandSource(envelope),
		DefinitionRevision: schedulerStringValue(envelope.Payload, events.PayloadKeyDefinitionRevision),
		DecisionID:         schedulerStringValue(envelope.Payload, events.PayloadKeyDecisionID),
		Revision:           envelope.Revision,
		AcceptedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func acceptedSchedulerCommandSource(envelope events.Envelope) string {
	if source := schedulerStringValue(envelope.Payload, events.PayloadKeySource); source != "" {
		return source
	}
	return strings.TrimSpace(envelope.SourceModule)
}

func acceptedSchedulerCommandIntent(envelope events.Envelope) string {
	if intent := schedulerStringValue(envelope.Payload, events.PayloadKeyIntent); intent != "" {
		return intent
	}
	return ""
}

func (s *serverState) handleWorkorchDSchedulerCommand(ctx context.Context, envelope events.Envelope) error {
	switch envelope.Topic {
	case workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested:
		req := workspaceRunRequestFromEnvelope(envelope)
		if strings.TrimSpace(req.WorkspaceID) == "" || strings.TrimSpace(req.Intent) == "" {
			return common.ErrInvalidArgument
		}
		switch req.Intent {
		case "resume":
			if strings.TrimSpace(req.RunID) != "" {
				action := strings.TrimSpace(req.Action)
				if action == "" {
					action = "resume"
				}
				if !s.runContinuations.scheduleRunContinuation(runContinuationRequest{
					WorkspaceID:           req.WorkspaceID,
					RunID:                 req.RunID,
					Action:                action,
					Priority:              req.Priority,
					ExecutionContextPatch: req.ExecutionContextPatch,
				}) {
					return schedulerCommandQueueError(envelope.Topic)
				}
				return nil
			}
			req.Command.Restart = false
			if !s.runContinuations.scheduleWorkspaceActivation(workspaceRestartRequest{
				WorkspaceID: req.WorkspaceID,
				Priority:    req.Priority,
				Command:     req.Command,
			}) {
				return schedulerCommandQueueError(envelope.Topic)
			}
			return nil
		case "restart":
			req.Command.Restart = true
			if !s.runContinuations.scheduleWorkspaceRestart(workspaceRestartRequest{
				WorkspaceID: req.WorkspaceID,
				Priority:    req.Priority,
				Command:     req.Command,
			}) {
				return schedulerCommandQueueError(envelope.Topic)
			}
			return nil
		case "replan":
			if strings.TrimSpace(req.RunID) == "" {
				return common.ErrInvalidArgument
			}
			if !s.scheduleGovernanceRunResumeRefs([]governanceRunResumeRef{{
				WorkspaceID: req.WorkspaceID,
				RunID:       req.RunID,
				Intent:      "replan",
				Reason:      req.Reason,
				RequestID:   req.RequestID,
				DecisionID:  req.DecisionID,
			}}, req.Priority) {
				return schedulerCommandQueueError(envelope.Topic)
			}
			return nil
		case "recover":
			if strings.TrimSpace(req.RunID) == "" {
				return common.ErrInvalidArgument
			}
			if !s.runContinuations.scheduleRunContinuation(runContinuationRequest{
				WorkspaceID:           req.WorkspaceID,
				RunID:                 req.RunID,
				Action:                "resume",
				Priority:              req.Priority,
				ExecutionContextPatch: req.ExecutionContextPatch,
			}) {
				return schedulerCommandQueueError(envelope.Topic)
			}
			return nil
		default:
			return common.ErrInvalidArgument
		}
	default:
		return common.ErrInvalidArgument
	}
}

func schedulerCommandQueueError(topic string) error {
	return fmt.Errorf("%w: scheduler command %s was not queued", common.ErrConflict, topic)
}

func workspaceRunRequestFromEnvelope(envelope events.Envelope) workspaceRunRequest {
	payload := envelope.Payload
	workspaceID := schedulerString(envelope, events.PayloadKeyWorkspaceID, envelope.WorkspaceID)
	runID := schedulerString(envelope, events.PayloadKeyRunID, envelope.RunID)
	priority := schedulerInt(payload, events.PayloadKeyPriority)
	cmd := appservice.ActivateWorkspaceExecutionCommand{
		GoalID:                schedulerStringValue(payload, events.PayloadKeyGoalID),
		DoDID:                 schedulerStringValue(payload, events.PayloadKeyDoDID),
		LLMProviderID:         schedulerStringValue(payload, events.PayloadKeyLLMProviderID),
		SoulID:                schedulerStringValue(payload, events.PayloadKeySoulID),
		SoulBindings:          schedulerStringMap(payload, "soulBindings"),
		SkillIDs:              schedulerStringSlice(payload, "skillIds"),
		MCPServerIDs:          schedulerStringSlice(payload, "mcpServerIds"),
		CapabilityIDs:         schedulerStringSlice(payload, "capabilityIds"),
		MemoryIDs:             schedulerStringSlice(payload, "memoryIds"),
		DialogLanguage:        schedulerStringValue(payload, "dialogLanguage"),
		Priority:              priority,
		ExecutionContextPatch: schedulerMap(payload, events.PayloadKeyExecutionContextPatch),
	}
	return workspaceRunRequest{
		WorkspaceID:           workspaceID,
		RunID:                 runID,
		Intent:                schedulerStringValue(payload, events.PayloadKeyIntent),
		Action:                schedulerStringValue(payload, events.PayloadKeyAction),
		Reason:                schedulerStringValue(payload, events.PayloadKeyReason),
		RequestID:             schedulerStringValue(payload, events.PayloadKeyRequestID),
		DefinitionRevision:    schedulerStringValue(payload, events.PayloadKeyDefinitionRevision),
		DecisionID:            schedulerStringValue(payload, events.PayloadKeyDecisionID),
		Priority:              priority,
		ExecutionContextPatch: schedulerMap(payload, events.PayloadKeyExecutionContextPatch),
		Command:               cmd,
	}
}

func schedulerString(envelope events.Envelope, key string, fallback string) string {
	if value := schedulerStringValue(envelope.Payload, key); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func schedulerStringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func schedulerInt(payload map[string]any, key string) int {
	if payload == nil {
		return 0
	}
	switch value := payload[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func schedulerMap(payload map[string]any, key string) map[string]any {
	if payload == nil {
		return nil
	}
	if value, ok := payload[key].(map[string]any); ok {
		return value
	}
	return nil
}

func schedulerStringMap(payload map[string]any, key string) map[string]string {
	if payload == nil {
		return nil
	}
	switch value := payload[key].(type) {
	case map[string]string:
		ret := make(map[string]string, len(value))
		for k, v := range value {
			ret[k] = v
		}
		return ret
	case map[string]any:
		ret := make(map[string]string, len(value))
		for k, v := range value {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				ret[k] = strings.TrimSpace(s)
			}
		}
		if len(ret) > 0 {
			return ret
		}
	}
	return nil
}

func schedulerStringSlice(payload map[string]any, key string) []string {
	if payload == nil {
		return nil
	}
	switch value := payload[key].(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		ret := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				ret = append(ret, strings.TrimSpace(s))
			}
		}
		if len(ret) > 0 {
			return ret
		}
	}
	return nil
}
