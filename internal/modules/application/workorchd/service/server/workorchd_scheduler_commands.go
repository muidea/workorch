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
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type workspaceRunRequest struct {
	WorkspaceID        string
	RunID              string
	Intent             string
	Action             string
	Reason             string
	RequestID          string
	DefinitionRevision string
	DecisionID         string
	Priority           int
	Command            appservice.ActivateWorkspaceExecutionCommand
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
	case "resume":
		if strings.TrimSpace(req.WorkspaceID) == "" || strings.TrimSpace(req.RunID) == "" {
			return ""
		}
		return strings.Join([]string{"resume", strings.TrimSpace(req.WorkspaceID), strings.TrimSpace(req.RunID)}, "/")
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
			return s.sendRuntimeV2RunOwnerCommand(ctx, req.WorkspaceID, req.RunID, action)
		}
		req.Command.Restart = false
		return s.executeWorkspaceRuntimeV2Activation(ctx, req)
	case "restart":
		if strings.TrimSpace(req.RunID) != "" {
			return common.ErrInvalidArgument
		}
		req.Command.Restart = true
		return s.executeWorkspaceRuntimeV2Activation(ctx, req)
	default:
		return common.ErrInvalidArgument
	}
}

func (s *serverState) executeWorkspaceRuntimeV2Activation(ctx context.Context, req workspaceRunRequest) error {
	if s == nil || strings.TrimSpace(req.WorkspaceID) == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	runCtx := appservice.ContextWithCommandHub(ctx, s.commandHub)
	runCtx = common.ContextWithRuntimeWorkspaceID(runCtx, common.WorkspaceID(workspaceID))
	release, err := s.acquireRunSlotWithPriority(runCtx, req.Priority)
	if err != nil {
		s.metrics.runCanceled.Add(1)
		return err
	}
	defer release()
	req.Command.Priority = req.Priority
	activation, err := s.resumeWorkspaceExecution(runCtx, s.config.Backend, s.config.DataRoot, workspaceID, req.Command)
	if err != nil {
		s.notifyRunStatusChanged()
		return err
	}
	s.metrics.observeRunOutcome(activation.Run)
	s.notifyRunStatusChangedForRun(activation.Run)
	return nil
}

func (s *serverState) sendRuntimeV2RunOwnerCommand(ctx context.Context, workspaceID string, runID string, action string) error {
	if s == nil || s.commandHub == nil || strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(runID) == "" {
		return common.ErrInvalidArgument
	}
	workspaceScope := common.WorkspaceID(strings.TrimSpace(workspaceID))
	runScope := common.RunID(strings.TrimSpace(runID))
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceScope)
	var topic string
	var command any
	switch strings.TrimSpace(action) {
	case "start":
		topic = orchestratorevents.TopicOrchestratorCommandStartRun
		command = orchestratorevents.StartRunCommand{RunID: runScope}
	case "resume":
		topic = orchestratorevents.TopicOrchestratorCommandResumeRun
		command = orchestratorevents.ResumeRunCommand{WorkspaceID: workspaceScope, RunID: runScope}
	default:
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(topic, "workorchd/scheduler", orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{SubjectType: "run", SubjectID: string(runScope), WorkspaceID: string(workspaceScope), RunID: string(runScope)}))
	response := s.commandHub.Send(event)
	if response == nil {
		return fmt.Errorf("%w: Runtime V2 run command %s returned no response", common.ErrConflict, topic)
	}
	if err := response.Error(); err != nil {
		return err
	}
	data, err := response.Get()
	if err != nil {
		return err
	}
	if _, ok := data.(orchestratorevents.OrchestratorCommandResult); !ok {
		return fmt.Errorf("%w: Runtime V2 run command %s returned %T", common.ErrInvalidArgument, topic, data)
	}
	return nil
}

func workspaceRunRequestFromCommand(command workorchdevents.WorkspaceRunRequestedCommand) workspaceRunRequest {
	cmd := appservice.ActivateWorkspaceExecutionCommand{
		GoalID:         string(command.GoalID),
		DoDID:          string(command.DoDID),
		LLMProviderID:  command.LLMProviderID,
		SoulID:         string(command.SoulID),
		SoulBindings:   cloneSchedulerStringMap(command.SoulBindings),
		SkillIDs:       schedulerIDStrings(command.SkillIDs),
		MCPServerIDs:   schedulerIDStrings(command.MCPServerIDs),
		CapabilityIDs:  schedulerIDStrings(command.CapabilityIDs),
		MemoryIDs:      schedulerIDStrings(command.MemoryIDs),
		DialogLanguage: command.DialogLanguage,
		Priority:       command.Priority,
	}
	return workspaceRunRequest{
		WorkspaceID:        string(command.WorkspaceID),
		RunID:              string(command.RunID),
		Intent:             command.Intent,
		Action:             command.Action,
		Reason:             command.Reason,
		RequestID:          command.RequestID,
		DefinitionRevision: command.DefinitionRevision,
		DecisionID:         command.DecisionID,
		Priority:           command.Priority,
		Command:            cmd,
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
