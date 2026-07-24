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
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// WorkspaceRunScheduler is the narrow application Biz port used by the HTTP
// adapter. The server neither owns nor directly uses EventHub.
type WorkspaceRunScheduler interface {
	SubmitWorkspaceRun(context.Context, workorchdevents.WorkspaceRunRequestedCommand) error
	DispatchRuntimeRun(context.Context, string, string, string) error
}

func (s *serverState) updateWorkspaceLLMTarget(ctx context.Context, workspaceID string, target *runtimecontract.ProviderModelTarget) (appservice.Workspace, error) {
	var ret appservice.Workspace
	err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
		var err error
		ret, err = appservice.UpdateWorkspaceLLMTargetWithSession(ctx, session, workspaceID, target)
		return err
	})
	return ret, err
}

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
	if s == nil || s.workspaceRunScheduler == nil || strings.TrimSpace(topic) == "" {
		return common.ErrInvalidArgument
	}
	command = prepareHTTPSchedulerCommand(topic, command)
	if err := s.workspaceRunScheduler.SubmitWorkspaceRun(ctx, command); err != nil {
		s.recordSchedulerCommandResult(topic, "failed")
		return err
	}
	s.recordSchedulerCommandResult(topic, "accepted")
	return nil
}

// enqueueWorkorchDSchedulerCommand is the HTTP acceptance boundary. It records
// the command as queued before submitting background work so a 202 response
// never implies that activation bootstrap already completed. The background
// task uses the process context plus explicit workspace scope; it must not
// retain the request context after the handler returns.
func (s *serverState) enqueueWorkorchDSchedulerCommand(topic string, command workorchdevents.WorkspaceRunRequestedCommand) error {
	if s == nil || s.workspaceRunScheduler == nil || strings.TrimSpace(topic) == "" {
		return common.ErrInvalidArgument
	}
	command = prepareHTTPSchedulerCommand(topic, command)
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	s.projectQueuedSchedulerCommand(command)
	if !s.scheduleBackground("workspace_execution_activation", func(ctx context.Context) {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))
		if err := s.publishWorkorchDSchedulerCommand(ctx, topic, command); err != nil {
			s.projectRejectedSchedulerCommand(command, err)
		}
	}) {
		err := fmt.Errorf("%w: workspace execution activation could not be queued", common.ErrConflict)
		s.recordSchedulerCommandEnqueueFailure(topic, workspaceID, strings.TrimSpace(string(command.RunID)), err.Error())
		s.projectRejectedSchedulerCommand(command, err)
		return err
	}
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

func (s *serverState) projectQueuedSchedulerCommand(command workorchdevents.WorkspaceRunRequestedCommand) {
	if s == nil || s.workspaceProjection == nil {
		return
	}
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	if workspaceID == "" {
		return
	}
	s.workspaceProjection.ApplyRunCommandQueued(workspaceviewcore.RunCommandQueuedUpdate{
		WorkspaceID:        workspaceID,
		RunID:              strings.TrimSpace(string(command.RunID)),
		Intent:             strings.TrimSpace(command.Intent),
		Reason:             strings.TrimSpace(command.Reason),
		RequestID:          strings.TrimSpace(command.RequestID),
		IdempotencyKey:     schedulerCommandIdempotencyKey(command),
		Source:             strings.TrimSpace(command.Source),
		DefinitionRevision: strings.TrimSpace(command.DefinitionRevision),
		DecisionID:         strings.TrimSpace(command.DecisionID),
		QueuedAt:           time.Now().UTC().Format(time.RFC3339Nano),
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
	runCtx := ctx
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
	// Activation constructs a fresh bootstrap or returns the existing active
	// Run.  In both cases the scheduler must hand the persisted scope back to
	// the Runtime V2 owner.  Without this command a Web/TUI `resume` merely
	// rereads the active Run and no drive is scheduled.  Start/resume races are
	// intentionally harmless: the Orchestrator owns admission and run-scoped
	// singleflight, so a newly-created Run reuses its already-scheduled drive.
	if strings.TrimSpace(activation.Run.ID) != "" && activation.Run.State == string(common.RunStateActive) {
		if err := s.sendRuntimeV2RunOwnerCommand(runCtx, workspaceID, activation.Run.ID, "resume"); err != nil {
			s.notifyRunStatusChangedForRun(activation.Run)
			return err
		}
	}
	s.notifyRunStatusChangedForRun(activation.Run)
	return nil
}

func (s *serverState) sendRuntimeV2RunOwnerCommand(ctx context.Context, workspaceID string, runID string, action string) error {
	if s == nil || s.workspaceRunScheduler == nil || strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(runID) == "" {
		return common.ErrInvalidArgument
	}
	return s.workspaceRunScheduler.DispatchRuntimeRun(ctx, workspaceID, runID, action)
}

func workspaceRunRequestFromCommand(command workorchdevents.WorkspaceRunRequestedCommand) workspaceRunRequest {
	cmd := appservice.ActivateWorkspaceExecutionCommand{
		GoalID:         string(command.GoalID),
		DoDID:          string(command.DoDID),
		LLMTarget:      command.LLMTarget,
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
