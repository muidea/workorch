package server

import (
	"context"
	"strings"

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

type runContinuationRequest struct {
	WorkspaceID           string
	RunID                 string
	Action                string
	Priority              int
	ExecutionContextPatch map[string]any
}

func (s *serverState) publishGovernanceContinuationRequested(ctx context.Context, refs []governanceRunResumeRef, priority int) error {
	for _, ref := range uniqueGovernanceRunResumeRefs(refs) {
		if err := s.publishWorkorchDSchedulerCommand(ctx, workorchdevents.TopicWorkorchDCommandGovernanceContinuationRequested, ref.WorkspaceID, ref.RunID, map[string]any{
			events.PayloadKeyWorkspaceID: ref.WorkspaceID,
			events.PayloadKeyRunID:       ref.RunID,
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
		if _, ok := events.ResponseEnvelope(err); ok {
			return nil
		}
		return err
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
	case workorchdevents.TopicWorkorchDCommandWorkspaceActivationRequested:
		return "workspace-activation"
	case workorchdevents.TopicWorkorchDCommandRunContinuationRequested:
		return "run-continuation"
	case workorchdevents.TopicWorkorchDCommandGovernanceContinuationRequested:
		return "governance-continuation"
	case workorchdevents.TopicWorkorchDCommandReplanContinuationRequested:
		return "replan-continuation"
	case workorchdevents.TopicWorkorchDCommandWorkspaceRestartRequested:
		return "workspace-restart"
	default:
		return "workorchd-scheduler-command"
	}
}

func schedulerSubjectID(topic string, workspaceID string, runID string, payload map[string]any) string {
	switch topic {
	case workorchdevents.TopicWorkorchDCommandWorkspaceRestartRequested:
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
		workorchdevents.TopicWorkorchDCommandWorkspaceActivationRequested,
		workorchdevents.TopicWorkorchDCommandRunContinuationRequested,
		workorchdevents.TopicWorkorchDCommandGovernanceContinuationRequested,
		workorchdevents.TopicWorkorchDCommandReplanContinuationRequested,
		workorchdevents.TopicWorkorchDCommandWorkspaceRestartRequested,
	}
	for _, topic := range topics {
		topic := topic
		unsubscribe := s.schedulerHub.Subscribe(topic, func(ctx context.Context, envelope events.Envelope) error {
			if err := s.handleWorkorchDSchedulerCommand(ctx, envelope); err != nil {
				return err
			}
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

func (s *serverState) handleWorkorchDSchedulerCommand(ctx context.Context, envelope events.Envelope) error {
	switch envelope.Topic {
	case workorchdevents.TopicWorkorchDCommandWorkspaceActivationRequested:
		req := workspaceRestartRequestFromEnvelope(envelope)
		req.Command.Restart = false
		if strings.TrimSpace(req.WorkspaceID) == "" {
			return common.ErrInvalidArgument
		}
		_ = s.runContinuations.scheduleWorkspaceActivation(req)
		return nil
	case workorchdevents.TopicWorkorchDCommandRunContinuationRequested:
		req := runContinuationRequestFromEnvelope(envelope)
		if strings.TrimSpace(req.WorkspaceID) == "" || strings.TrimSpace(req.RunID) == "" || strings.TrimSpace(req.Action) == "" {
			return common.ErrInvalidArgument
		}
		_ = s.runContinuations.scheduleRunContinuation(req)
		return nil
	case workorchdevents.TopicWorkorchDCommandGovernanceContinuationRequested:
		workspaceID := schedulerString(envelope, events.PayloadKeyWorkspaceID, envelope.WorkspaceID)
		runID := schedulerString(envelope, events.PayloadKeyRunID, envelope.RunID)
		if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(runID) == "" {
			return common.ErrInvalidArgument
		}
		s.scheduleGovernanceRunResumeRefs([]governanceRunResumeRef{{WorkspaceID: workspaceID, RunID: runID}}, schedulerInt(envelope.Payload, events.PayloadKeyPriority))
		return nil
	case workorchdevents.TopicWorkorchDCommandReplanContinuationRequested:
		workspaceID := schedulerString(envelope, events.PayloadKeyWorkspaceID, envelope.WorkspaceID)
		runID := schedulerString(envelope, events.PayloadKeyRunID, envelope.RunID)
		if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(runID) == "" {
			return common.ErrInvalidArgument
		}
		s.scheduleReplanContinuationIfNeeded(ctx, workspaceID, runID, schedulerInt(envelope.Payload, events.PayloadKeyPriority))
		return nil
	case workorchdevents.TopicWorkorchDCommandWorkspaceRestartRequested:
		req := workspaceRestartRequestFromEnvelope(envelope)
		if strings.TrimSpace(req.WorkspaceID) == "" {
			return common.ErrInvalidArgument
		}
		_ = s.runContinuations.scheduleWorkspaceRestart(req)
		return nil
	default:
		return common.ErrInvalidArgument
	}
}

func runContinuationRequestFromEnvelope(envelope events.Envelope) runContinuationRequest {
	payload := envelope.Payload
	return runContinuationRequest{
		WorkspaceID:           schedulerString(envelope, events.PayloadKeyWorkspaceID, envelope.WorkspaceID),
		RunID:                 schedulerString(envelope, events.PayloadKeyRunID, envelope.RunID),
		Action:                schedulerStringValue(payload, events.PayloadKeyAction),
		Priority:              schedulerInt(payload, events.PayloadKeyPriority),
		ExecutionContextPatch: schedulerMap(payload, events.PayloadKeyExecutionContextPatch),
	}
}

func workspaceRestartRequestFromEnvelope(envelope events.Envelope) workspaceRestartRequest {
	payload := envelope.Payload
	workspaceID := schedulerString(envelope, events.PayloadKeyWorkspaceID, envelope.WorkspaceID)
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
		Restart:               true,
		ExecutionContextPatch: schedulerMap(payload, events.PayloadKeyExecutionContextPatch),
	}
	return workspaceRestartRequest{WorkspaceID: workspaceID, Priority: priority, Command: cmd}
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
