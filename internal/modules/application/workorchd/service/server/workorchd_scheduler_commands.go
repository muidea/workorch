package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// WorkspaceRunScheduler is the narrow application Biz port used by the HTTP
// adapter. The server neither owns nor directly uses EventHub.
type WorkspaceRunScheduler interface {
	SubmitWorkspaceRun(context.Context, workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error)
}

func (s *serverState) updateWorkspaceLLMTarget(ctx context.Context, workspaceID string, target *runtimecontract.ProviderModelTarget) (appmodelcore.Workspace, error) {
	var ret appmodelcore.Workspace
	err := s.withSharedSession(ctx, func(session *runtimecore.Session) error {
		var err error
		ret, err = appservice.UpdateWorkspaceLLMTargetWithSession(ctx, session, workspaceID, target)
		return err
	})
	return ret, err
}

func (s *serverState) publishWorkorchDSchedulerCommand(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	if s == nil || s.workspaceRunScheduler == nil || strings.TrimSpace(topic) == "" {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	command = prepareWorkspaceRunSchedulerCommand(topic, command)
	ownerResult, err := s.workspaceRunScheduler.SubmitWorkspaceRun(ctx, command)
	if err != nil {
		s.recordSchedulerCommandResult(topic, "failed")
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	s.recordSchedulerCommandResult(topic, string(ownerResult.Outcome))
	s.projectOwnerSchedulerOutcome(command, ownerResult)
	return ownerResult, nil
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
	command = prepareWorkspaceRunSchedulerCommand(topic, command)
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	s.projectQueuedSchedulerCommand(command)
	if !s.scheduleBackground("workspace_execution_activation", func(ctx context.Context) {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))
		if _, err := s.publishWorkorchDSchedulerCommand(ctx, topic, command); err != nil {
			s.projectRejectedSchedulerCommand(command, err)
			s.recordWorkspaceRunFailure(command, schedulerKindWorkspaceActivation, schedulerReasonWorkspaceActivationFailed, err)
		}
	}) {
		err := fmt.Errorf("%w: workspace execution activation could not be queued", common.ErrConflict)
		s.recordSchedulerCommandEnqueueFailure(topic, workspaceID, "", err.Error())
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

// prepareWorkspaceRunSchedulerCommand establishes one immutable request
// correlation for HTTP, TUI and local EventHub scheduler entries.
func prepareWorkspaceRunSchedulerCommand(topic string, command workorchdevents.WorkspaceRunRequestedCommand) workorchdevents.WorkspaceRunRequestedCommand {
	if strings.TrimSpace(command.RequestID) == "" {
		parts := []string{
			"workorchd",
			schedulerKindFromTopic(topic),
			strings.TrimSpace(string(command.WorkspaceID)),
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
	body := map[string]any{
		"service":     "workorch",
		"backend":     config.Backend,
		"dataRoot":    config.DataRoot,
		"accepted":    true,
		"workspaceId": workspaceID,
	}
	if value := strings.TrimSpace(action); value != "" {
		body["action"] = value
	}
	for key, value := range map[string]string{
		recordpayload.PayloadKeyIntent:    command.Intent,
		recordpayload.PayloadKeyRequestID: command.RequestID,
		recordpayload.PayloadKeySource:    command.Source,
		recordpayload.PayloadKeyReason:    command.Reason,
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

// recordWorkspaceRunFailure captures an application activation failure even
// when no Run was created. It is a scheduler/read-model diagnostic only: it
// neither creates a Run nor changes Runtime authority.
func (s *serverState) recordWorkspaceRunFailure(command workorchdevents.WorkspaceRunRequestedCommand, kind string, reason string, cause error) {
	if s == nil || s.schedulerDiagnostics == nil || cause == nil {
		return
	}
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	if workspaceID == "" {
		return
	}
	if strings.TrimSpace(kind) == "" {
		kind = schedulerKindWorkspaceActivation
	}
	if strings.TrimSpace(reason) == "" {
		reason = schedulerReasonWorkspaceActivationFailed
	}
	s.schedulerDiagnostics.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID:    workspaceID,
		Kind:           kind,
		State:          schedulerDiagnosticStateFailed,
		Reason:         reason,
		Detail:         cause.Error(),
		Recoverability: schedulerRecoverabilityManualRetry,
		DedupeKey:      schedulerDedupeKey(workspaceID, "", kind, reason),
	})
}

func schedulerSubjectID(topic string, command workorchdevents.WorkspaceRunRequestedCommand) string {
	if topic == workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested {
		return strings.TrimSpace(string(command.WorkspaceID))
	}
	return strings.TrimSpace(string(command.WorkspaceID))
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
		WorkspaceID:    workspaceID,
		RunID:          "",
		Intent:         strings.TrimSpace(command.Intent),
		Reason:         strings.TrimSpace(command.Reason),
		RequestID:      strings.TrimSpace(command.RequestID),
		IdempotencyKey: schedulerCommandIdempotencyKey(command),
		Source:         strings.TrimSpace(command.Source),
		AcceptedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// projectOwnerSchedulerOutcome records the Workspace owner's actual admission
// disposition.  Queued is merely HTTP transport state; it is never evidence
// that a Run was activated.
func (s *serverState) projectOwnerSchedulerOutcome(command workorchdevents.WorkspaceRunRequestedCommand, outcome workorchdevents.WorkspaceRunRequestedResult) {
	if s == nil || s.workspaceProjection == nil {
		return
	}
	workspaceID := strings.TrimSpace(string(outcome.WorkspaceID))
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(string(command.WorkspaceID))
	}
	if workspaceID == "" {
		return
	}
	disposition := string(outcome.Outcome)
	diagnostic := runtimecontract.RuntimeExecutionIntentReasonSummary(outcome.Reason)
	if outcome.Outcome == runtimecontract.RuntimeExecutionIntentReconciliationRequested || outcome.Outcome == runtimecontract.RuntimeExecutionIntentReplacementPending {
		s.workspaceProjection.ApplyRunCommandAccepted(workspaceviewcore.RunCommandAcceptedUpdate{
			WorkspaceID: workspaceID, RunID: string(outcome.RunID), Intent: command.Intent,
			Reason: command.Reason, Disposition: disposition, Diagnostic: diagnostic, RequestID: command.RequestID, IdempotencyKey: schedulerCommandIdempotencyKey(command), Source: command.Source,
			AcceptedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		return
	}
	s.workspaceProjection.ApplyRunCommandRejected(workspaceviewcore.RunCommandRejectedUpdate{
		WorkspaceID: workspaceID, RunID: string(outcome.RunID), Intent: command.Intent,
		Reason: command.Reason, Disposition: disposition, Diagnostic: diagnostic, RequestID: command.RequestID, IdempotencyKey: schedulerCommandIdempotencyKey(command), Source: command.Source,
		RejectedAt: time.Now().UTC().Format(time.RFC3339Nano), Error: diagnostic,
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
		WorkspaceID:    workspaceID,
		RunID:          "",
		Intent:         strings.TrimSpace(command.Intent),
		Reason:         strings.TrimSpace(command.Reason),
		RequestID:      strings.TrimSpace(command.RequestID),
		IdempotencyKey: schedulerCommandIdempotencyKey(command),
		Source:         strings.TrimSpace(command.Source),
		QueuedAt:       time.Now().UTC().Format(time.RFC3339Nano),
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
		WorkspaceID:    workspaceID,
		RunID:          "",
		Intent:         strings.TrimSpace(command.Intent),
		Reason:         strings.TrimSpace(command.Reason),
		RequestID:      strings.TrimSpace(command.RequestID),
		IdempotencyKey: schedulerCommandIdempotencyKey(command),
		Source:         strings.TrimSpace(command.Source),
		RejectedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Error:          errText,
	})
}

func schedulerCommandIdempotencyKey(command workorchdevents.WorkspaceRunRequestedCommand) string {
	switch strings.TrimSpace(command.Intent) {
	case "resume":
		if strings.TrimSpace(string(command.WorkspaceID)) == "" {
			return ""
		}
		return "resume/" + strings.TrimSpace(string(command.WorkspaceID))
	case "restart":
		if strings.TrimSpace(string(command.WorkspaceID)) == "" {
			return ""
		}
		return "restart/" + strings.TrimSpace(string(command.WorkspaceID))
	default:
		if strings.TrimSpace(string(command.WorkspaceID)) == "" {
			return ""
		}
		return strings.TrimSpace(command.Intent) + "/" + strings.TrimSpace(string(command.WorkspaceID))
	}
}

func (s *serverState) handleWorkorchDSchedulerCommand(ctx context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	if s == nil || s.workspaceRunScheduler == nil || strings.TrimSpace(string(command.WorkspaceID)) == "" || strings.TrimSpace(command.RequestID) == "" {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	// The server is only an asynchronous protocol adapter. It cannot choose a
	// Run path or issue lifecycle commands; the application Biz forwards this
	// request to Workspace owner as a typed intent.
	return s.workspaceRunScheduler.SubmitWorkspaceRun(ctx, command)
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
