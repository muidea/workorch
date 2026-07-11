package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
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
		if err := s.publishWorkorchDSchedulerCommand(ctx, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
			WorkspaceID: common.WorkspaceID(ref.WorkspaceID),
			RunID:       common.RunID(ref.RunID),
			Intent:      intent,
			Reason:      reason,
			RequestID:   ref.RequestID,
			Source:      "governance",
			DecisionID:  ref.DecisionID,
			Priority:    priority,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *serverState) publishWorkorchDSchedulerCommand(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) error {
	if s == nil || s.schedulerHub == nil || strings.TrimSpace(topic) == "" {
		return common.ErrInvalidArgument
	}
	command = prepareHTTPSchedulerCommand(topic, command)
	event := magicevent.NewEventWithContext(topic, "workorchd/http", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{SubjectType: schedulerSubjectType(topic), SubjectID: schedulerSubjectID(topic, command), WorkspaceID: strings.TrimSpace(string(command.WorkspaceID)), RunID: strings.TrimSpace(string(command.RunID))}))
	response := s.schedulerHub.Send(event)
	if response == nil {
		s.recordSchedulerCommandResult(topic, "missing_response")
		return fmt.Errorf("%w: scheduler command %s returned no accepted response", common.ErrConflict, topic)
	}
	if err := response.Error(); err != nil {
		s.recordSchedulerCommandResult(topic, "failed")
		if strings.Contains(err.Message, "missing observer") {
			return fmt.Errorf("%w: scheduler command %s returned no accepted response", common.ErrConflict, topic)
		}
		return err
	}
	data, err := response.Get()
	if err != nil {
		s.recordSchedulerCommandResult(topic, "failed")
		return err
	}
	result, ok := data.(workorchdevents.WorkspaceRunRequestedResult)
	if !ok || !result.Accepted {
		s.recordSchedulerCommandResult(topic, "not_accepted")
		return fmt.Errorf("%w: scheduler command %s was not accepted", common.ErrConflict, topic)
	}
	s.recordSchedulerCommandResult(topic, "accepted")
	return nil
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

func prepareHTTPSchedulerCommand(topic string, command workorchdevents.WorkspaceRunRequestedCommand) workorchdevents.WorkspaceRunRequestedCommand {
	if strings.TrimSpace(command.RequestID) == "" {
		parts := []string{
			"workorchd",
			schedulerKindFromTopic(topic),
			strings.TrimSpace(string(command.WorkspaceID)),
			strings.TrimSpace(string(command.RunID)),
			fmt.Sprintf("%d", time.Now().UTC().UnixNano()),
		}
		for idx, part := range parts {
			if part == "" {
				parts[idx] = "none"
			}
		}
		command.RequestID = strings.Join(parts, "-")
	}
	command.Source = schedulerCommandSource(command.Source, "http")
	return command
}

func schedulerAcceptedHTTPBody(config Config, action string, command workorchdevents.WorkspaceRunRequestedCommand) map[string]any {
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	runID := strings.TrimSpace(string(command.RunID))
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
	for key, value := range map[string]string{
		events.PayloadKeyIntent:             command.Intent,
		events.PayloadKeyRequestID:          command.RequestID,
		events.PayloadKeySource:             command.Source,
		events.PayloadKeyReason:             command.Reason,
		events.PayloadKeyDefinitionRevision: command.DefinitionRevision,
		events.PayloadKeyDecisionID:         command.DecisionID,
	} {
		if value = strings.TrimSpace(value); value != "" {
			body[key] = value
		}
	}
	return body
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

func schedulerSubjectID(topic string, command workorchdevents.WorkspaceRunRequestedCommand) string {
	switch topic {
	case workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested:
		return strings.TrimSpace(string(command.WorkspaceID))
	default:
		if id := strings.TrimSpace(string(command.RunID)); id != "" {
			return id
		}
	}
	return strings.TrimSpace(string(command.WorkspaceID))
}

func (s *serverState) registerWorkorchDSchedulerCommandWatcher() {
	if s == nil || s.schedulerHub == nil {
		return
	}
	topics := []string{
		workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested,
	}
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, s.schedulerHub)
	for _, topic := range topics {
		topic := topic
		observer.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
			command, ok := ev.Data().(workorchdevents.WorkspaceRunRequestedCommand)
			if !ok {
				result.Set(nil, cd.NewError(cd.IllegalParam, common.ErrInvalidArgument.Error()))
				return
			}
			ctx := ev.Context()
			if err := s.handleWorkorchDSchedulerCommand(ctx, command); err != nil {
				s.projectRejectedSchedulerCommand(command, err)
				result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
				return
			}
			s.projectAcceptedSchedulerCommand(command)
			result.Set(workorchdevents.WorkspaceRunRequestedResult{Accepted: true}, nil)
		})
		s.eventUnsubscribes = append(s.eventUnsubscribes, func() { observer.Unsubscribe(topic) })
	}
}

func schedulerIDs[ID ~string](values []string) []ID {
	ret := make([]ID, 0, len(values))
	for _, value := range values {
		ret = append(ret, ID(value))
	}
	return ret
}

func (s *serverState) projectAcceptedSchedulerCommand(command workorchdevents.WorkspaceRunRequestedCommand) {
	if s == nil || s.workspaceProjection == nil {
		return
	}
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	if strings.TrimSpace(workspaceID) == "" {
		return
	}
	s.workspaceProjection.ApplyRunCommandAccepted(workspaceviewcore.RunCommandAcceptedUpdate{
		WorkspaceID:        workspaceID,
		RunID:              strings.TrimSpace(string(command.RunID)),
		Intent:             strings.TrimSpace(command.Intent),
		Reason:             strings.TrimSpace(command.Reason),
		RequestID:          strings.TrimSpace(command.RequestID),
		IdempotencyKey:     schedulerCommandIdempotencyKey(command),
		Source:             strings.TrimSpace(command.Source),
		DefinitionRevision: strings.TrimSpace(command.DefinitionRevision),
		DecisionID:         strings.TrimSpace(command.DecisionID),
		AcceptedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *serverState) projectRejectedSchedulerCommand(command workorchdevents.WorkspaceRunRequestedCommand, cause error) {
	if s == nil || s.workspaceProjection == nil {
		return
	}
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	if strings.TrimSpace(workspaceID) == "" {
		return
	}
	errText := ""
	if cause != nil {
		errText = cause.Error()
	}
	s.workspaceProjection.ApplyRunCommandRejected(workspaceviewcore.RunCommandRejectedUpdate{
		WorkspaceID:        workspaceID,
		RunID:              strings.TrimSpace(string(command.RunID)),
		Intent:             strings.TrimSpace(command.Intent),
		Reason:             strings.TrimSpace(command.Reason),
		RequestID:          strings.TrimSpace(command.RequestID),
		IdempotencyKey:     schedulerCommandIdempotencyKey(command),
		Source:             strings.TrimSpace(command.Source),
		DefinitionRevision: strings.TrimSpace(command.DefinitionRevision),
		DecisionID:         strings.TrimSpace(command.DecisionID),
		RejectedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Error:              errText,
	})
}

func schedulerCommandIdempotencyKey(command workorchdevents.WorkspaceRunRequestedCommand) string {
	req := workspaceRunRequestFromCommand(command)
	switch strings.TrimSpace(req.Intent) {
	case "replan":
		return governanceRunResumeKey(governanceRunResumeRef{
			WorkspaceID: req.WorkspaceID,
			RunID:       req.RunID,
			Intent:      "replan",
			DecisionID:  req.DecisionID,
		})
	case "resume":
		return governanceRunResumeKey(governanceRunResumeRef{
			WorkspaceID: req.WorkspaceID,
			RunID:       req.RunID,
			Intent:      req.Intent,
		})
	case "restart":
		if strings.TrimSpace(req.WorkspaceID) == "" {
			return ""
		}
		return "restart/" + strings.TrimSpace(req.WorkspaceID)
	default:
		if strings.TrimSpace(req.WorkspaceID) == "" {
			return ""
		}
		return strings.TrimSpace(req.Intent) + "/" + strings.TrimSpace(req.WorkspaceID)
	}
}

func (s *serverState) handleWorkorchDSchedulerCommand(ctx context.Context, command workorchdevents.WorkspaceRunRequestedCommand) error {
	req := workspaceRunRequestFromCommand(command)
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
				return schedulerCommandQueueError(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)
			}
			return nil
		}
		req.Command.Restart = false
		if !s.runContinuations.scheduleWorkspaceActivation(workspaceRestartRequest{
			WorkspaceID: req.WorkspaceID,
			Priority:    req.Priority,
			Command:     req.Command,
		}) {
			return schedulerCommandQueueError(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)
		}
		return nil
	case "restart":
		if strings.TrimSpace(req.RunID) != "" {
			return common.ErrInvalidArgument
		}
		req.Command.Restart = true
		if !s.runContinuations.scheduleWorkspaceRestart(workspaceRestartRequest{
			WorkspaceID: req.WorkspaceID,
			Priority:    req.Priority,
			Command:     req.Command,
		}) {
			return schedulerCommandQueueError(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)
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
			return schedulerCommandQueueError(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)
		}
		return nil
	default:
		return common.ErrInvalidArgument
	}
}

func schedulerCommandQueueError(topic string) error {
	return fmt.Errorf("%w: scheduler command %s was not queued", common.ErrConflict, topic)
}

func workspaceRunRequestFromCommand(command workorchdevents.WorkspaceRunRequestedCommand) workspaceRunRequest {
	executionContextPatch := command.ExecutionContextPatch.Map()
	cmd := appservice.ActivateWorkspaceExecutionCommand{
		GoalID:                string(command.GoalID),
		DoDID:                 string(command.DoDID),
		LLMProviderID:         command.LLMProviderID,
		SoulID:                string(command.SoulID),
		SoulBindings:          cloneSchedulerStringMap(command.SoulBindings),
		SkillIDs:              schedulerIDStrings(command.SkillIDs),
		MCPServerIDs:          schedulerIDStrings(command.MCPServerIDs),
		CapabilityIDs:         schedulerIDStrings(command.CapabilityIDs),
		MemoryIDs:             schedulerIDStrings(command.MemoryIDs),
		DialogLanguage:        command.DialogLanguage,
		Priority:              command.Priority,
		ExecutionContextPatch: executionContextPatch,
	}
	return workspaceRunRequest{
		WorkspaceID:           string(command.WorkspaceID),
		RunID:                 string(command.RunID),
		Intent:                command.Intent,
		Action:                command.Action,
		Reason:                command.Reason,
		RequestID:             command.RequestID,
		DefinitionRevision:    command.DefinitionRevision,
		DecisionID:            command.DecisionID,
		Priority:              command.Priority,
		ExecutionContextPatch: executionContextPatch,
		Command:               cmd,
	}
}

func schedulerIDStrings[ID ~string](values []ID) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		ret = append(ret, string(value))
	}
	return ret
}

func cloneSchedulerStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	ret := make(map[string]string, len(values))
	for key, value := range values {
		ret[key] = value
	}
	return ret
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
